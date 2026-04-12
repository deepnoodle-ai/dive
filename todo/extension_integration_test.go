package todo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// scriptedLLM is a minimal test double that returns pre-programmed LLM
// responses in order. Each call to Generate consumes the next script entry
// and records the messages it received so tests can introspect what the
// agent passed to the model (e.g. whether a system-reminder block was
// injected into the first user message).
type scriptedLLM struct {
	mu       sync.Mutex
	script   []scriptedTurn
	idx      int
	received [][]*llm.Message
}

type scriptedTurn struct {
	text     string
	toolUses []*llm.ToolUseContent
}

func (s *scriptedLLM) Name() string { return "scripted" }

func (s *scriptedLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := &llm.Config{}
	cfg.Apply(opts...)
	snap := make([]*llm.Message, len(cfg.Messages))
	for i, m := range cfg.Messages {
		snap[i] = m.Copy()
	}
	s.received = append(s.received, snap)
	if s.idx >= len(s.script) {
		return nil, fmt.Errorf("scriptedLLM: unexpected call %d (scripted %d turns)", s.idx+1, len(s.script))
	}
	turn := s.script[s.idx]
	s.idx++
	var content []llm.Content
	stop := "stop"
	if len(turn.toolUses) > 0 {
		for _, tu := range turn.toolUses {
			content = append(content, tu)
		}
		stop = "tool_use"
	} else {
		content = append(content, &llm.TextContent{Text: turn.text})
	}
	return &llm.Response{
		ID:         fmt.Sprintf("resp_%d", s.idx),
		Model:      s.Name(),
		Role:       llm.Assistant,
		Content:    content,
		Type:       "message",
		StopReason: stop,
		Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
	}, nil
}

func (s *scriptedLLM) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idx
}

// lastReceived returns the messages from the most recent Generate call.
func (s *scriptedLLM) lastReceived() []*llm.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.received) == 0 {
		return nil
	}
	return s.received[len(s.received)-1]
}

// todoWriteToolUse builds a TodoWrite tool_use content block with a
// JSON-serialized WriteInput payload.
func todoWriteToolUse(id string, items []TodoItem) *llm.ToolUseContent {
	input, _ := json.Marshal(WriteInput{Todos: items})
	return &llm.ToolUseContent{ID: id, Name: ToolName, Input: input}
}

// findStateBlockInMessages returns the first <todo-state> text-content block
// found anywhere in messages, or "" if none exists. Useful for asserting on
// what was durably written to the session / the compaction output.
func findStateBlockInMessages(messages []*llm.Message) string {
	for _, msg := range messages {
		for _, c := range msg.Content {
			tc, ok := c.(*llm.TextContent)
			if !ok {
				continue
			}
			if strings.Contains(tc.Text, stateBlockStart) {
				return tc.Text
			}
		}
	}
	return ""
}

// firstUserMessageText collects all TextContent blocks in the first user
// message into a single string, so tests can assert on reminder injection.
func firstUserMessageText(messages []*llm.Message) string {
	for _, msg := range messages {
		if msg.Role != llm.User {
			continue
		}
		var b strings.Builder
		for _, c := range msg.Content {
			if tc, ok := c.(*llm.TextContent); ok {
				b.WriteString(tc.Text)
				b.WriteString("\n")
			}
		}
		return b.String()
	}
	return ""
}

// TestIntegration_StatePersistsAcrossTurns verifies the end-to-end flow:
// a TodoWrite call during turn one writes a hidden <todo-state> block into
// the tool_result message; that block is saved to the session and is still
// present on turn two, where findLatestState can recover the todos.
func TestIntegration_StatePersistsAcrossTurns(t *testing.T) {
	items := []TodoItem{
		{Content: "Draft spec", Status: TodoStatusInProgress, ActiveForm: "Drafting spec"},
		{Content: "Ship feature", Status: TodoStatusPending, ActiveForm: "Shipping feature"},
	}
	mock := &scriptedLLM{
		script: []scriptedTurn{
			// Turn 1: tool_use then final text.
			{toolUses: []*llm.ToolUseContent{todoWriteToolUse("tu-1", items)}},
			{text: "todos written"},
			// Turn 2: plain text reply.
			{text: "still tracking"},
			// Turn 3: plain text reply.
			{text: "all clear"},
		},
	}
	sess := session.New("integration-s1")
	ext := New(WithReminderTurns(0)) // disable reminder side-effects for this test
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model:      mock,
		Session:    sess,
		Extensions: []dive.Extension{ext},
	})
	assert.NoError(t, err)

	// Turn one: LLM calls TodoWrite, agent runs the tool, LLM emits final text.
	resp, err := agent.CreateResponse(context.Background(), dive.WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, dive.ResponseStatusCompleted)

	stored, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.NotEqual(t, findStateBlockInMessages(stored), "", "state block should be persisted in the session after a successful TodoWrite")

	got, turnsAfterTurn1, found := LatestState(stored)
	assert.True(t, found)
	// After turn 1 the final assistant message ("todos written") sits after
	// the tool_result block, so turnsSince counts exactly that one turn.
	assert.Equal(t, turnsAfterTurn1, 1)
	assert.Len(t, got, 2)
	assert.Equal(t, got[0].Content, "Draft spec")

	// Turn two: plain assistant reply, no tool call. Existing state must survive.
	_, err = agent.CreateResponse(context.Background(), dive.WithInput("thanks"))
	assert.NoError(t, err)

	stored, err = sess.Messages(context.Background())
	assert.NoError(t, err)
	got, turnsAfterTurn2, found := LatestState(stored)
	assert.True(t, found)
	assert.True(t, turnsAfterTurn2 > turnsAfterTurn1, "turnsSince should advance after a plain assistant turn")
	assert.Len(t, got, 2)
	assert.Equal(t, got[0].Content, "Draft spec")

	// Turn three: one more plain turn; state still findable, list unchanged.
	_, err = agent.CreateResponse(context.Background(), dive.WithInput("one more thing"))
	assert.NoError(t, err)
	stored, _ = sess.Messages(context.Background())
	got, turnsAfterTurn3, found := LatestState(stored)
	assert.True(t, found)
	assert.True(t, turnsAfterTurn3 > turnsAfterTurn2, "turnsSince should advance again")
	assert.Len(t, got, 2)
}

// TestIntegration_ReminderSelfClearsAfterSecondWrite exercises the full
// reminder lifecycle through the real generation loop: a TodoWrite, several
// plain turns that push turnsSince past the threshold (so the next
// PreGeneration call injects a reminder), then a second TodoWrite that
// resets the counter and must cause the reminder to be removed on the next
// generation.
func TestIntegration_ReminderSelfClearsAfterSecondWrite(t *testing.T) {
	items1 := []TodoItem{{Content: "Task A", Status: TodoStatusInProgress, ActiveForm: "Doing A"}}
	items2 := []TodoItem{{Content: "Task A", Status: TodoStatusCompleted, ActiveForm: "Doing A"}}

	mock := &scriptedLLM{
		script: []scriptedTurn{
			{toolUses: []*llm.ToolUseContent{todoWriteToolUse("tu-1", items1)}}, // turn 1 LLM call 1 (tool use)
			{text: "ok wrote todos"}, // turn 1 LLM call 2 (final)
			{text: "turn 2 reply"},   // turn 2
			{text: "turn 3 reply"},   // turn 3
			{text: "turn 4 reply"},   // turn 4 — reminder should now be injected on NEXT PreGeneration
			{text: "turn 5 reply"},   // turn 5 (receives reminder)
			{toolUses: []*llm.ToolUseContent{todoWriteToolUse("tu-2", items2)}}, // turn 6 LLM call 1 (second TodoWrite)
			{text: "ack"},            // turn 6 LLM call 2 (final)
			{text: "turn 7 reply"},   // turn 7 (should NOT have reminder)
		},
	}

	sess := session.New("integration-reminder")
	ext := New(WithReminderTurns(3))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model:      mock,
		Session:    sess,
		Extensions: []dive.Extension{ext},
	})
	assert.NoError(t, err)

	mustTurn := func(input string) {
		t.Helper()
		_, err := agent.CreateResponse(context.Background(), dive.WithInput(input))
		assert.NoError(t, err)
	}

	mustTurn("start")
	mustTurn("turn 2")
	mustTurn("turn 3")
	mustTurn("turn 4")

	// By turn 5, at least 3 assistant turns have happened after the TodoWrite
	// state block, so the reminder must have been injected into the first
	// user message visible on the LLM call.
	mustTurn("turn 5")
	preTurn5 := mock.received[len(mock.received)-1]
	assert.True(t, dive.HasSystemReminder(preTurn5, reminderName), "reminder should be injected on turn 5")

	// Turn 6 calls TodoWrite again. The reminder was still present when the
	// generation started (LLM call 7), but after the state-capture hook runs
	// the NEXT generation should see it removed.
	mustTurn("turn 6")

	// Turn 7: plain reply. The reminder must be gone from the first user
	// message because a fresh TodoWrite just ran with turnsSince=0.
	mustTurn("turn 7")
	preTurn7 := mock.received[len(mock.received)-1]
	assert.False(t, dive.HasSystemReminder(preTurn7, reminderName), "reminder should be cleared after a fresh TodoWrite")

	// Sanity: the latest persisted state reflects the second write.
	stored, _ := sess.Messages(context.Background())
	got, _, found := LatestState(stored)
	assert.True(t, found)
	assert.Len(t, got, 1)
	assert.Equal(t, TodoStatusCompleted, got[0].Status)
}

// TestIntegration_EmptyListDoesNotTriggerStaleReminder verifies that an
// explicit empty TodoWrite (user said "I'm done") does not cause a stale
// reminder to be injected later, no matter how many turns pass.
func TestIntegration_EmptyListDoesNotTriggerStaleReminder(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			{toolUses: []*llm.ToolUseContent{todoWriteToolUse("tu-1", []TodoItem{})}},
			{text: "cleared list"},
			{text: "turn 2"},
			{text: "turn 3"},
			{text: "turn 4"},
			{text: "turn 5"},
		},
	}
	sess := session.New("integration-empty")
	ext := New(WithReminderTurns(2))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model:      mock,
		Session:    sess,
		Extensions: []dive.Extension{ext},
	})
	assert.NoError(t, err)

	for _, input := range []string{"start", "turn2", "turn3", "turn4", "turn5"} {
		_, err := agent.CreateResponse(context.Background(), dive.WithInput(input))
		assert.NoError(t, err)
	}

	for i, received := range mock.received {
		if dive.HasSystemReminder(received, reminderName) {
			t.Fatalf("reminder unexpectedly injected at LLM call %d with an empty todo list", i+1)
		}
	}
}

// TestIntegration_MalformedStateBlockFailsSafe pre-seeds a session with a
// tool_result-shaped user message whose trailing text content contains a
// well-shaped but JSON-broken <todo-state> block. The agent must not panic,
// must not inject a reminder off of the corrupted state, and a subsequent
// successful TodoWrite must restore correct state that findLatestState can
// read back.
func TestIntegration_MalformedStateBlockFailsSafe(t *testing.T) {
	items := []TodoItem{{Content: "Recovered", Status: TodoStatusInProgress, ActiveForm: "Recovering"}}

	// First, sanity-check the core parser: garbage payload returns !ok and
	// findLatestState returns no match and does not panic.
	garbage := stateBlockStart + "not-json-at-all" + stateBlockEnd
	_, parseOK := parseStateBlock(garbage)
	assert.False(t, parseOK)

	// Seed a session by saving a fake turn whose assistant message is plain
	// text and whose following user message is a plausible tool_result-ish
	// message carrying the malformed state block as a trailing text content.
	sess := session.New("integration-malformed")
	seedMessages := []*llm.Message{
		llm.NewUserTextMessage("start"),
		{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "noop"}}},
		{Role: llm.User, Content: []llm.Content{&llm.TextContent{Text: garbage}}},
	}
	assert.NoError(t, sess.SaveTurn(context.Background(), seedMessages, &llm.Usage{}))

	// An arbitrary extra malformed variant without the closing tag — must
	// also not trip the parser.
	_, parseOK = parseStateBlock(stateBlockStart + `{"todos":[`)
	assert.False(t, parseOK)

	got, _, found := LatestState([]*llm.Message{
		{Role: llm.User, Content: []llm.Content{&llm.TextContent{Text: stateBlockStart + `{"todos":[` /* no close */}}},
	})
	assert.False(t, found)
	assert.Nil(t, got)

	// Now run the agent. The scripted LLM first replies plainly (we assert
	// the pre-generation reminder hook did NOT inject a reminder even though
	// a corrupt block sits in history), then on the next turn it issues a
	// real TodoWrite so we can confirm recovery.
	mock := &scriptedLLM{
		script: []scriptedTurn{
			{text: "plain reply"},
			{toolUses: []*llm.ToolUseContent{todoWriteToolUse("tu-1", items)}},
			{text: "wrote"},
		},
	}
	ext := New(WithReminderTurns(1)) // very eager threshold
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model:      mock,
		Session:    sess,
		Extensions: []dive.Extension{ext},
	})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), dive.WithInput("q1"))
	assert.NoError(t, err)

	// The LLM call MUST NOT contain a reminder — the corrupted block must
	// not be mis-parsed as "empty list" or any other valid state.
	received := mock.received[0]
	assert.False(t, dive.HasSystemReminder(received, reminderName), "malformed state must not cause a reminder to be injected")

	// Second turn: the real TodoWrite restores valid state.
	_, err = agent.CreateResponse(context.Background(), dive.WithInput("q2"))
	assert.NoError(t, err)

	stored, _ := sess.Messages(context.Background())
	latest, _, found := LatestState(stored)
	assert.True(t, found)
	assert.Len(t, latest, 1)
	assert.Equal(t, "Recovered", latest[0].Content)
}

// TestIntegration_SuspendResumePreservesTodoState covers the suspend/resume
// path: in a single iteration the LLM calls BOTH TodoWrite and a suspending
// approval tool. The TodoWrite completes and writes a <todo-state> block
// into the partial tool_result that gets persisted on suspend; after the
// caller supplies the approval result and the agent resumes to completion,
// the state block must still be present in the saved session and recoverable
// via findLatestState.
func TestIntegration_SuspendResumePreservesTodoState(t *testing.T) {
	items := []TodoItem{{Content: "Deploy", Status: TodoStatusInProgress, ActiveForm: "Deploying"}}

	approve := &approveTool{}

	mock := &scriptedLLM{
		script: []scriptedTurn{
			// Single assistant turn with two tool calls.
			{toolUses: []*llm.ToolUseContent{
				todoWriteToolUse("tu-todo", items),
				{ID: "tu-approve", Name: "approve", Input: []byte(`{"req":"deploy"}`)},
			}},
			// After resume with the approval result the LLM finalizes.
			{text: "deployed"},
		},
	}

	sess := session.New("integration-suspend")
	ext := New(WithReminderTurns(5))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model:      mock,
		Session:    sess,
		Tools:      []dive.Tool{approve},
		Extensions: []dive.Extension{ext},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), dive.WithInput("ship it"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, dive.ResponseStatusSuspended)
	assert.True(t, sess.LoadSuspension() != nil)

	// The suspended turn already has the <todo-state> block in it, because
	// the TodoWrite completed before the approval tool suspended.
	suspState := sess.LoadSuspension()
	assert.NotEqual(t, "", findStateBlockInMessages(suspState.TurnMessages), "state block should survive the partial suspend save")

	// Resume with an approval result.
	resp, err = agent.CreateResponse(context.Background(),
		dive.WithToolResults(map[string]*dive.ToolResult{
			"tu-approve": dive.NewToolResultText("approved"),
		}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, dive.ResponseStatusCompleted)
	assert.False(t, sess.LoadSuspension() != nil)

	stored, _ := sess.Messages(context.Background())
	latest, _, found := LatestState(stored)
	assert.True(t, found)
	assert.Len(t, latest, 1)
	assert.Equal(t, "Deploy", latest[0].Content)
}

// approveTool is a tiny dive.Tool that unconditionally suspends. Used in
// the suspend/resume integration test to force a partial tool_result save.
type approveTool struct{}

func (a *approveTool) Name() string        { return "approve" }
func (a *approveTool) Description() string { return "ask a human to approve an action" }
func (a *approveTool) Schema() *dive.Schema {
	return &dive.Schema{Type: dive.Object}
}
func (a *approveTool) Annotations() *dive.ToolAnnotations { return nil }
func (a *approveTool) Call(_ context.Context, _ any) (*dive.ToolResult, error) {
	return dive.NewSuspendResult("waiting on approver", nil), nil
}

// TestIntegration_StateCaptureIgnoresFailedToolResults ensures that the
// state-capture hook never persists a "successful" state block when the
// underlying TodoWrite call reported a protocol-level error. The PostToolUse
// path already won't be called on error (PostToolUseFailure fires instead),
// but we belt-and-suspenders test that if a bad invocation somehow reached
// the capture hook it is still a no-op.
func TestIntegration_StateCaptureIgnoresFailedToolResults(t *testing.T) {
	ext := New()
	hooks := ext.Hooks()
	assert.Len(t, hooks.PostToolUse, 1)
	hook := hooks.PostToolUse[0]

	// Simulate calling the PostToolUse hook with a failed (IsError) result.
	call := todoWriteCall(t, []TodoItem{
		{Content: "Task", Status: TodoStatusInProgress, ActiveForm: "Doing task"},
	})
	hctx := &dive.HookContext{
		Tool: NewTool(),
		Call: call,
		Result: &dive.ToolCallResult{
			ID:     "tu-1",
			Result: dive.NewToolResultError("bad input"),
		},
	}
	assert.NoError(t, hook(context.Background(), hctx))
	assert.Equal(t, "", hctx.AdditionalContext, "failed TodoWrite must not emit a state block")

	// And with a missing Call (defensive).
	hctx = &dive.HookContext{
		Tool:   NewTool(),
		Result: &dive.ToolCallResult{ID: "tu-1", Result: dive.NewToolResultText("ok")},
	}
	assert.NoError(t, hook(context.Background(), hctx))
	assert.Equal(t, "", hctx.AdditionalContext)
}
