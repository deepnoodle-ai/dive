package arena

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
)

// localDecider fills a seat with a local Dive agent. It is built from the single
// shared template — identical system prompt, identical tool set, identical
// referee hook — with only the model and its own session differing from every
// other local player. This is the Phase 1 path: the model submits its action by
// calling one of the typed action tools, the referee hook (a PreToolUse hook)
// rejects illegal moves and forces a retry, and the tool records the action.
type localDecider struct {
	id     string
	agent  *dive.Agent
	policy policy
	log    func(string)

	mu  sync.Mutex
	cur *localTurn
}

// localTurn is the active turn's state, shared between the action tools (which
// write the captured action) and the referee hook (which validates the call).
// Each player has its own, so players could even act concurrently.
type localTurn struct {
	kind       actionKind
	candidates map[string]bool
	candList   []string // for human-readable error messages
	captured   *capturedAction
	submitted  bool
}

// newLocalDecider builds the agent for a local seat. The system prompt, tools,
// and referee are all identical across players — see SystemPrompt.
func newLocalDecider(id string, model llm.LLM, pol policy, log func(string)) (*localDecider, error) {
	d := &localDecider{id: id, policy: pol, log: log}
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         id,
		SystemPrompt: SystemPrompt,
		Model:        model,
		Session:      session.New("colosseum-" + id),
		Tools:        d.actionTools(),
		Hooks: dive.Hooks{
			PreToolUse: []dive.PreToolUseHook{d.referee},
		},
		// Bound the per-turn tool loop: enough iterations to recover from a few
		// rejected illegal moves, but not so many that a confused model burns
		// tokens forever.
		ToolIterationLimit: 6,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent for %s: %w", id, err)
	}
	d.agent = agent
	return d, nil
}

func (d *localDecider) decide(ctx context.Context, t turn, situation string) (*capturedAction, *llm.Usage, error) {
	instruction := localTask(t.kind, t.role, t.candidates)
	input := situation + "\n\n" + instruction

	d.beginTurn(t)
	defer d.endTurn()

	total := &llm.Usage{}
	var lastErr error
	for attempt := 0; attempt <= d.policy.maxRetries; attempt++ {
		cctx, cancel := context.WithTimeout(ctx, d.policy.turnTimeout)
		resp, err := d.agent.CreateResponse(cctx, dive.WithInput(input))
		cancel()
		if resp != nil && resp.Usage != nil {
			total.Add(resp.Usage)
		}
		// A valid action may have been captured even if the trailing
		// acknowledgement call later errored — prefer it over retrying.
		if act := d.captured(); act != nil {
			return act, total, nil
		}
		if err != nil {
			lastErr = err
			if d.log != nil {
				d.log(fmt.Sprintf("%s turn error (attempt %d/%d): %v",
					d.id, attempt+1, d.policy.maxRetries+1, err))
			}
			if attempt < d.policy.maxRetries {
				backoff(ctx, attempt)
			}
			continue // reuse the same input; nothing was saved to the session
		}
		// No transport error but no action either: nudge once more. The prior
		// turn is now in the session, so a short reminder is enough.
		lastErr = fmt.Errorf("model produced no valid action")
		input = "You did not submit a valid action that turn. " + instruction
	}
	return nil, total, lastErr
}

// --- turn / capture plumbing ---

func (d *localDecider) beginTurn(t turn) {
	d.mu.Lock()
	d.cur = &localTurn{kind: t.kind, candidates: setOf(t.candidates), candList: t.candidates}
	d.mu.Unlock()
}

func (d *localDecider) endTurn() {
	d.mu.Lock()
	d.cur = nil
	d.mu.Unlock()
}

func (d *localDecider) captured() *capturedAction {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cur == nil {
		return nil
	}
	return d.cur.captured
}

// capture is called by the action tools after the referee approves a call.
func (d *localDecider) capture(a capturedAction) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cur != nil && !d.cur.submitted {
		d.cur.captured = &a
		d.cur.submitted = true
	}
}

// --- the shared action tools ---

func (d *localDecider) actionTools() []dive.Tool {
	const ack = "Action recorded. Your turn is over — do not call any more tools; reply with a brief acknowledgement."

	speak := dive.FuncTool("speak",
		"Address the village out loud during the day discussion.",
		func(ctx context.Context, in *SpeakInput) (*dive.ToolResult, error) {
			d.capture(capturedAction{kind: actSpeak, message: in.Message, reasoning: in.Reasoning})
			return dive.NewToolResultText(ack), nil
		})

	vote := dive.FuncTool("vote",
		"Cast your public vote for the player to eliminate.",
		func(ctx context.Context, in *VoteInput) (*dive.ToolResult, error) {
			d.capture(capturedAction{kind: actVote, target: in.Target, reasoning: in.Reasoning})
			return dive.NewToolResultText(ack), nil
		})

	night := dive.FuncTool("night_action",
		"Take your secret night action (kill, inspect, or protect, depending on your role).",
		func(ctx context.Context, in *NightActionInput) (*dive.ToolResult, error) {
			d.capture(capturedAction{kind: actNight, target: in.Target, reasoning: in.Reasoning})
			return dive.NewToolResultText(ack), nil
		})

	return []dive.Tool{speak, vote, night}
}

// --- the referee (a PreToolUse hook) ---

// referee rejects illegal moves by returning an error, which Dive feeds back to
// the model as the tool result, forcing a corrected retry. It validates against
// the turn's legal candidate list (the authoritative set the game master
// computed), so it needs no game-state coupling.
func (d *localDecider) referee(ctx context.Context, hctx *dive.HookContext) error {
	d.mu.Lock()
	cur := d.cur
	d.mu.Unlock()
	if cur == nil {
		return fmt.Errorf("there is no active turn right now")
	}
	if cur.submitted {
		return fmt.Errorf("you have already submitted your action this turn; do not call any more tools")
	}

	tool := hctx.Call.Name
	switch cur.kind {
	case actSpeak:
		if tool != "speak" {
			return fmt.Errorf("it is the discussion phase: use the 'speak' tool, not %q", tool)
		}
		var in SpeakInput
		if err := json.Unmarshal(hctx.Call.Input, &in); err != nil {
			return fmt.Errorf("could not parse your speak action: %v", err)
		}
		if strings.TrimSpace(in.Message) == "" {
			return fmt.Errorf("your 'message' is empty; say something to the village")
		}
		return requireReasoning(in.Reasoning)

	case actVote:
		if tool != "vote" {
			return fmt.Errorf("it is the voting phase: use the 'vote' tool, not %q", tool)
		}
		var in VoteInput
		if err := json.Unmarshal(hctx.Call.Input, &in); err != nil {
			return fmt.Errorf("could not parse your vote: %v", err)
		}
		if err := cur.checkTarget(in.Target); err != nil {
			return err
		}
		return requireReasoning(in.Reasoning)

	case actNight:
		if tool != "night_action" {
			return fmt.Errorf("it is the night phase: use the 'night_action' tool, not %q", tool)
		}
		var in NightActionInput
		if err := json.Unmarshal(hctx.Call.Input, &in); err != nil {
			return fmt.Errorf("could not parse your night action: %v", err)
		}
		if err := cur.checkTarget(in.Target); err != nil {
			return err
		}
		return requireReasoning(in.Reasoning)
	}
	return fmt.Errorf("unexpected turn kind %q", cur.kind)
}

func (lt *localTurn) checkTarget(target string) error {
	if !lt.candidates[target] {
		return fmt.Errorf("invalid target %q; choose exactly one of: %s",
			target, strings.Join(lt.candList, ", "))
	}
	return nil
}

func requireReasoning(reasoning string) error {
	if strings.TrimSpace(reasoning) == "" {
		return fmt.Errorf("you must include your private 'reasoning'")
	}
	return nil
}
