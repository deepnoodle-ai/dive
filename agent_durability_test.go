package dive_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// recordingStore records every turn checkpointed through it, and fails the
// checkpoint numbered failAt (from 1) when it is set.
type recordingStore struct {
	*session.Session
	mu     sync.Mutex
	turns  []*Turn
	failAt int
}

func (s *recordingStore) CheckpointTurn(ctx context.Context, expected uint64, turn *Turn) (uint64, error) {
	s.mu.Lock()
	cp := *turn
	cp.Messages = append([]*llm.Message(nil), turn.Messages...)
	cp.ToolCalls = append([]ToolCallRecord(nil), turn.ToolCalls...)
	s.turns = append(s.turns, &cp)
	fail := s.failAt == len(s.turns)
	s.mu.Unlock()
	if fail {
		return 0, errors.New("disk full")
	}
	return s.Session.CheckpointTurn(ctx, expected, turn)
}

func (s *recordingStore) recorded() []*Turn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*Turn(nil), s.turns...)
}

// states returns the state of each call in a record, in order.
func states(turn *Turn) []ToolCallState {
	var out []ToolCallState
	for _, record := range turn.ToolCalls {
		out = append(out, record.State)
	}
	return out
}

// readOnly marks a tool read-only.
func readOnly(tool *funcTool) *funcTool {
	tool.ann = &ToolAnnotations{ReadOnlyHint: true}
	return tool
}

// With CheckpointSteps the turn is recorded before the first model call,
// after a response whose calls will run, as each call starts and as each
// result arrives, and then finished.
func TestStepCheckpoints(t *testing.T) {
	var ran atomic.Int32
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "work", `{}`), toolUse("toolu_2", "look", `{}`)),
		textResponse("end_turn", "done"),
	}}
	store := &recordingStore{Session: session.New("steps")}
	agent, err := NewAgent(AgentOptions{
		Model:      mock,
		Session:    store,
		Tools:      []Tool{countingTool("work", &ran), countingTool("look", &ran)},
		Durability: DurabilityOptions{CheckpointSteps: true},
	})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)

	turns := store.recorded()
	assert.Len(t, turns, 7)
	running, notStarted, completed := ToolCallStateRunning, ToolCallStateNotStarted, ToolCallStateCompleted
	want := []struct {
		messages int
		states   []ToolCallState
	}{
		{1, nil},
		{2, []ToolCallState{notStarted, notStarted}},
		{2, []ToolCallState{running, notStarted}},
		{3, []ToolCallState{completed, notStarted}},
		{3, []ToolCallState{completed, running}},
		{3, []ToolCallState{completed, completed}},
	}
	for i, w := range want {
		assert.Equal(t, turns[i].Status, ResponseStatusRunning, "step %d", i)
		assert.Equal(t, turns[i].ID, resp.Turn.ID)
		assert.Len(t, turns[i].Messages, w.messages, "step %d", i)
		assert.Equal(t, states(turns[i]), w.states, "step %d", i)
	}
	assert.Equal(t, turns[6].Status, ResponseStatusCompleted)
	assert.Len(t, turns[6].Messages, 4)
	assert.Equal(t, resp.Turn.Revision, uint64(7))
	assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
}

// A parallel batch records every call it starts in one checkpoint, and each
// result as it arrives.
func TestParallelStepCheckpoints(t *testing.T) {
	var ran atomic.Int32
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "work", `{}`), toolUse("toolu_2", "look", `{}`)),
		textResponse("end_turn", "done"),
	}}
	store := &recordingStore{Session: session.New("parallel-steps")}
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Session:               store,
		Tools:                 []Tool{countingTool("work", &ran), countingTool("look", &ran)},
		ParallelToolExecution: true,
		Durability:            DurabilityOptions{CheckpointSteps: true},
	})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)

	turns := store.recorded()
	assert.Len(t, turns, 6)
	assert.Equal(t, states(turns[2]), []ToolCallState{ToolCallStateRunning, ToolCallStateRunning})
	one := states(turns[3])
	assert.Contains(t, one, ToolCallStateRunning)
	assert.Contains(t, one, ToolCallStateCompleted)
	assert.Equal(t, states(turns[4]), []ToolCallState{ToolCallStateCompleted, ToolCallStateCompleted})
	assert.Len(t, turns[4].Messages, 3)
	assert.Equal(t, turns[5].Status, ResponseStatusCompleted)
}

// A process that exits while a tool runs leaves the turn running in the
// file. The next invocation closes it with process_exit: the running call is
// unknown, and a call that is not read-only needs reconciling.
func TestProcessExitRecovery(t *testing.T) {
	ctx := context.Background()
	dir, crashed := t.TempDir(), t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	sess, err := store.Open(ctx, "crash")
	assert.NoError(t, err)

	// The write tool copies the session file as it stands while the tool
	// runs: the state a process exiting at that moment leaves.
	write := &funcTool{name: "write", call: func(ctx context.Context, input any) (*ToolResult, error) {
		data, err := os.ReadFile(filepath.Join(dir, "crash.jsonl"))
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(crashed, "crash.jsonl"), data, 0644); err != nil {
			return nil, err
		}
		return NewToolResultText("written"), nil
	}}
	var ran atomic.Int32
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "look", `{}`), toolUse("toolu_2", "write", `{}`)),
		textResponse("end_turn", "done"),
	}}
	agent, err := NewAgent(AgentOptions{
		Model:      mock,
		Session:    sess,
		Tools:      []Tool{readOnly(countingTool("look", &ran)), write},
		Durability: DurabilityOptions{CheckpointSteps: true},
	})
	assert.NoError(t, err)
	first, err := agent.CreateResponse(ctx, WithInput("look, then write"))
	assert.NoError(t, err)

	// The other process opens the copy.
	other, err := session.NewFileStore(crashed)
	assert.NoError(t, err)
	restarted, err := other.Open(ctx, "crash")
	assert.NoError(t, err)
	snap, err := restarted.Load(ctx)
	assert.NoError(t, err)
	assert.Equal(t, snap.OpenTurn.Status, ResponseStatusRunning)
	assert.Equal(t, snap.OpenTurn.ID, first.Turn.ID)
	assert.Equal(t, states(snap.OpenTurn), []ToolCallState{ToolCallStateCompleted, ToolCallStateRunning})

	mock2 := &responseLLM{responses: []*llm.Response{textResponse("end_turn", "checked, and continued")}}
	agent2, err := NewAgent(AgentOptions{
		Model:           mock2,
		Session:         restarted,
		Tools:           []Tool{readOnly(countingTool("look", &ran)), write},
		IncompleteTurns: IncompleteTurnOptions{RequireReconcile: true},
	})
	assert.NoError(t, err)
	_, err = agent2.CreateResponse(ctx, WithInput("anything else?"))
	assert.True(t, errors.Is(err, ErrUnreconciledToolCalls))
	assert.Equal(t, mock2.calls(), 0)

	snap, err = restarted.Load(ctx)
	assert.NoError(t, err)
	turn := snap.OpenTurn
	assert.Equal(t, turn.Status, ResponseStatusIncomplete)
	assert.Equal(t, turn.Outcome.Reason, TurnReasonProcessExit)
	assert.Equal(t, turn.Outcome.Next, TurnNextReconcile)
	assert.Equal(t, states(turn), []ToolCallState{ToolCallStateCompleted, ToolCallStateUnknown})

	// Continuing folds into the recovered turn; the model sees the unknown
	// result.
	resp, err := agent2.CreateResponse(ctx, WithContinue())
	assert.NoError(t, err)
	assert.Equal(t, resp.Turn.ID, first.Turn.ID)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	var sawUnknown bool
	for _, msg := range mock2.request(0) {
		for _, c := range msg.Content {
			if r, ok := c.(*llm.ToolResultContent); ok && r.ToolUseID == "toolu_2" && r.IsError {
				sawUnknown = true
			}
		}
	}
	assert.True(t, sawUnknown)
}

// A running call whose tool is read-only is unknown, but needs no
// reconciling: new input starts a new turn.
func TestProcessExitReadOnlyCall(t *testing.T) {
	ctx := context.Background()
	sess := session.New("read-only-exit")
	running := &Turn{
		Schema: TurnSchema,
		ID:     "turn_crashed",
		Status: ResponseStatusRunning,
		Messages: []*llm.Message{
			llm.NewUserTextMessage("look"),
			{Role: llm.Assistant, Content: []llm.Content{toolUse("toolu_1", "look", `{}`)}},
		},
		Usage:     &llm.Usage{InputTokens: 3},
		ToolCalls: []ToolCallRecord{{ID: "toolu_1", Name: "look", State: ToolCallStateRunning}},
	}
	_, err := sess.CheckpointTurn(ctx, 0, running)
	assert.NoError(t, err)

	var ran atomic.Int32
	mock := &responseLLM{responses: []*llm.Response{textResponse("end_turn", "hello")}}
	agent, err := NewAgent(AgentOptions{
		Model:           mock,
		Session:         sess,
		Tools:           []Tool{readOnly(countingTool("look", &ran))},
		IncompleteTurns: IncompleteTurnOptions{RequireReconcile: true},
	})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(ctx, WithInput("hi"))
	assert.NoError(t, err)
	assert.NotEqual(t, resp.Turn.ID, "turn_crashed")

	turns, err := sess.Turns(ctx)
	assert.NoError(t, err)
	assert.Len(t, turns, 2)
	assert.Equal(t, turns[0].Outcome.Reason, TurnReasonProcessExit)
	assert.Equal(t, turns[0].Outcome.Next, TurnNextContinue)
	assert.True(t, turns[0].Superseded)
	assert.True(t, requestHasReminder(mock.request(0), ReminderNameTurnIncomplete))
}

// A step checkpoint that fails stops the turn before the step: the tool
// never runs.
func TestStepCheckpointFailureStopsBeforeTool(t *testing.T) {
	var ran atomic.Int32
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "work", `{}`)),
	}}
	store := &recordingStore{Session: session.New("step-failure"), failAt: 3}
	agent, err := NewAgent(AgentOptions{
		Model:      mock,
		Session:    store,
		Tools:      []Tool{countingTool("work", &ran)},
		Durability: DurabilityOptions{CheckpointSteps: true},
	})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.Error(t, err)
	assert.Equal(t, ran.Load(), int32(0))
	assert.Equal(t, resp.Status, ResponseStatusIncomplete)
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{{ID: "toolu_1", Name: "work", State: ToolCallStateNotStarted}})
	assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
}

// A full resume with step checkpoints records the supplied results before
// their items are emitted.
func TestResumeStepCheckpoint(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "approve", `{}`)),
		textResponse("end_turn", "approved"),
	}}
	store := &recordingStore{Session: session.New("resume-steps")}
	agent, err := NewAgent(AgentOptions{
		Model:      mock,
		Session:    store,
		Tools:      []Tool{suspendingTool("approve", nil)},
		Durability: DurabilityOptions{CheckpointSteps: true},
	})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	before := len(store.recorded())

	var atResult int
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_1": NewToolResultText("yes")}),
		WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
			if item.Type == ResponseItemTypeToolCallResult {
				atResult = len(store.recorded())
			}
			return nil
		}))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	turns := store.recorded()
	assert.Equal(t, atResult, before+1)
	assert.Equal(t, turns[before].Status, ResponseStatusRunning)
	assert.Equal(t, states(turns[before]), []ToolCallState{ToolCallStateCompleted})
	assert.False(t, store.IsSuspended())
}

// Durability options a session cannot honour fail the call.
func TestDurabilityNeedsCapableSession(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{textResponse("end_turn", "hi")}}
	agent, err := NewAgent(AgentOptions{
		Model:      mock,
		Session:    &plainSession{},
		Durability: DurabilityOptions{CheckpointSteps: true},
	})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.Error(t, err)

	agent, err = NewAgent(AgentOptions{
		Model:      mock,
		Session:    &plainSession{},
		Durability: DurabilityOptions{Claim: true},
	})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.Error(t, err)
	assert.Equal(t, mock.calls(), 0)

	// Without a session there is nothing to make durable.
	agent, err = NewAgent(AgentOptions{
		Model:      mock,
		Durability: DurabilityOptions{CheckpointSteps: true, Claim: true},
	})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
}

// An invocation claims its session, fails before doing anything when
// another owner holds it, and releases its claim at the end.
func TestClaimSession(t *testing.T) {
	ctx := context.Background()
	sess := session.New("claimed")
	mock := &responseLLM{responses: []*llm.Response{textResponse("end_turn", "hi")}}
	var claimedDuring bool
	agent, err := NewAgent(AgentOptions{
		Model:      mock,
		Session:    sess,
		Durability: DurabilityOptions{Claim: true, Owner: "agent"},
		Hooks: Hooks{PreGeneration: []PreGenerationHook{func(ctx context.Context, hctx *HookContext) error {
			claimedDuring = errors.Is(sess.ClaimSession(ctx, "other", time.Minute), ErrSessionClaimed)
			return nil
		}}},
	})
	assert.NoError(t, err)

	assert.NoError(t, sess.ClaimSession(ctx, "other", time.Minute))
	_, err = agent.CreateResponse(ctx, WithInput("hi"))
	assert.True(t, errors.Is(err, ErrSessionClaimed))
	assert.Equal(t, mock.calls(), 0)
	assert.NoError(t, sess.ReleaseSession(ctx, "other"))

	resp, err := agent.CreateResponse(ctx, WithInput("hi"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.True(t, claimedDuring)
	assert.NoError(t, sess.ClaimSession(ctx, "other", time.Minute), "released")
}

// lossySession fails every claim after the first, as a session whose claim
// another owner took.
type lossySession struct {
	*session.Session
	claims atomic.Int32
}

func (s *lossySession) ClaimSession(ctx context.Context, owner string, ttl time.Duration) error {
	if s.claims.Add(1) > 1 {
		return ErrSessionClaimed
	}
	return s.Session.ClaimSession(ctx, owner, ttl)
}

// An invocation whose claim cannot be renewed is cancelled, and says why.
func TestClaimLostCancelsTurn(t *testing.T) {
	sess := &lossySession{Session: session.New("lossy")}
	wait := &funcTool{name: "wait", call: func(ctx context.Context, input any) (*ToolResult, error) {
		<-ctx.Done()
		return NewToolResultError("stopped"), nil
	}}
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "wait", `{}`)),
	}}
	agent, err := NewAgent(AgentOptions{
		Model:      mock,
		Session:    sess,
		Tools:      []Tool{wait},
		Durability: DurabilityOptions{Claim: true, ClaimTTL: 30 * time.Millisecond},
	})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.True(t, errors.Is(err, context.Canceled))
	assert.True(t, errors.Is(err, ErrSessionClaimed))
	assert.Equal(t, resp.Turn.Outcome.Reason, TurnReasonCanceled)
}

// failingToolset cannot resolve its tools.
type failingToolset struct{}

func (failingToolset) Name() string { return "failing" }
func (failingToolset) Tools(ctx context.Context) ([]Tool, error) {
	return nil, errors.New("toolset unavailable")
}

// A turn left running is closed even when the tools cannot be resolved; its
// running calls then all need reconciling, read-only or not.
func TestProcessExitRecoveryWithoutTools(t *testing.T) {
	ctx := context.Background()
	sess := session.New("no-tools-exit")
	_, err := sess.CheckpointTurn(ctx, 0, &Turn{
		Schema: TurnSchema,
		ID:     "turn_crashed",
		Status: ResponseStatusRunning,
		Messages: []*llm.Message{
			llm.NewUserTextMessage("look"),
			{Role: llm.Assistant, Content: []llm.Content{toolUse("toolu_1", "look", `{}`)}},
		},
		ToolCalls: []ToolCallRecord{{ID: "toolu_1", Name: "look", State: ToolCallStateRunning}},
	})
	assert.NoError(t, err)

	var ran atomic.Int32
	agent, err := NewAgent(AgentOptions{
		Model:    &responseLLM{responses: []*llm.Response{textResponse("end_turn", "hi")}},
		Session:  sess,
		Tools:    []Tool{readOnly(countingTool("look", &ran))},
		Toolsets: []Toolset{failingToolset{}},
	})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(ctx, WithInput("again"))
	assert.Error(t, err)

	turns, err := sess.Turns(ctx)
	assert.NoError(t, err)
	assert.Len(t, turns, 2)
	assert.Equal(t, turns[0].Outcome.Reason, TurnReasonProcessExit)
	assert.Equal(t, turns[0].Outcome.Next, TurnNextReconcile)
	assert.Equal(t, states(turns[0]), []ToolCallState{ToolCallStateUnknown})
}
