package arena

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/a2a"
	"github.com/deepnoodle-ai/dive/llm"
)

// remoteDecider fills a seat with an agent reached over A2A — the "bring your
// own agent to the arena" path. The game master sends the same situation it
// sends local players, plus an instruction to reply with a single JSON action
// object, and parses the reply. The remote agent runs entirely on whatever
// host the contributor controls; the only contract is the JSON action format.
//
// One RemoteAgent is reused for all of a player's turns: a2a.RemoteAgent keeps
// the context id across SendText calls, so the remote host (with a
// SessionProvider) maintains a continuing, private game memory over the wire.
type remoteDecider struct {
	id     string
	url    string
	remote *a2a.RemoteAgent
	policy policy
	log    func(string)
}

func newRemoteDecider(ctx context.Context, id, url string, pol policy, log func(string)) (*remoteDecider, error) {
	remote, err := a2a.NewRemoteAgentFromURL(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect to remote agent %s at %s: %w", id, url, err)
	}
	return &remoteDecider{id: id, url: url, remote: remote, policy: pol, log: log}, nil
}

func (d *remoteDecider) decide(ctx context.Context, t turn, situation string) (*capturedAction, *llm.Usage, error) {
	instruction := remoteTask(t.kind, t.role, t.candidates)
	input := situation + "\n\n" + instruction
	candidates := setOf(t.candidates)

	var lastErr error
	for attempt := 0; attempt <= d.policy.maxRetries; attempt++ {
		cctx, cancel := context.WithTimeout(ctx, d.policy.turnTimeout)
		result, err := d.remote.SendText(cctx, input)
		cancel()
		if err != nil {
			lastErr = err
			if d.log != nil {
				d.log(fmt.Sprintf("%s (remote) error (attempt %d/%d): %v",
					d.id, attempt+1, d.policy.maxRetries+1, err))
			}
			if attempt < d.policy.maxRetries {
				backoff(ctx, attempt)
			}
			continue
		}
		action, perr := parseRemoteAction(result.Text, t.kind, candidates, t.candidates)
		if perr == nil {
			return action, nil, nil // A2A does not surface token usage
		}
		lastErr = perr
		if d.log != nil {
			d.log(fmt.Sprintf("%s (remote) invalid action (attempt %d/%d): %v",
				d.id, attempt+1, d.policy.maxRetries+1, perr))
		}
		input = fmt.Sprintf("Your previous response was invalid: %v\n\n%s", perr, instruction)
	}
	return nil, nil, lastErr
}

// remoteAction is the JSON contract a remote agent must reply with.
type remoteAction struct {
	Action    string `json:"action"`
	Target    string `json:"target"`
	Message   string `json:"message"`
	Reasoning string `json:"reasoning"`
}

// parseRemoteAction extracts and validates the JSON action from a remote
// agent's reply, validating it against the same rules the local referee applies.
func parseRemoteAction(text string, kind actionKind, candidates map[string]bool, candList []string) (*capturedAction, error) {
	raw, ok := extractJSONObject(text)
	if !ok {
		return nil, fmt.Errorf("response did not contain a JSON object")
	}
	var a remoteAction
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return nil, fmt.Errorf("response was not valid JSON: %v", err)
	}
	if a.Action != "" && a.Action != kind.toolName() {
		return nil, fmt.Errorf("expected action %q, got %q", kind.toolName(), a.Action)
	}
	if strings.TrimSpace(a.Reasoning) == "" {
		return nil, fmt.Errorf("missing 'reasoning'")
	}
	switch kind {
	case actSpeak:
		if strings.TrimSpace(a.Message) == "" {
			return nil, fmt.Errorf("missing 'message'")
		}
		return &capturedAction{kind: actSpeak, message: a.Message, reasoning: a.Reasoning}, nil
	default: // vote / night
		if !candidates[a.Target] {
			return nil, fmt.Errorf("invalid target %q; choose one of: %s",
				a.Target, strings.Join(candList, ", "))
		}
		return &capturedAction{kind: kind, target: a.Target, reasoning: a.Reasoning}, nil
	}
}

// extractJSONObject returns the first balanced {...} object in s, tolerating
// surrounding prose or markdown code fences that models sometimes add.
func extractJSONObject(s string) (string, bool) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", false
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case c == '\\' && inStr:
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
			// skip
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}
