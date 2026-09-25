package dive_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	. "github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// suspendingTool suspends every call, recording the turn ID its context
// carries.
func suspendingTool(name string, turnIDs *[]string) *funcTool {
	var mu sync.Mutex
	return &funcTool{name: name, call: func(ctx context.Context, input any) (*ToolResult, error) {
		if turnIDs != nil {
			mu.Lock()
			*turnIDs = append(*turnIDs, TurnID(ctx))
			mu.Unlock()
		}
		return NewSuspendResult("waiting for "+name, nil), nil
	}}
}

// Every response carries the turn's identity and record, with a revision
// only when a TurnStore saved it.
func TestTurnRecordFields(t *testing.T) {
	var ran atomicCounter
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "work", `{}`)),
		textResponse("end_turn", "done"),
	}}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{ran.tool("work")}})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	turn := resp.Turn
	assert.True(t, strings.HasPrefix(turn.ID, "turn_"))
	assert.Equal(t, turn.Schema, TurnSchema)
	assert.Equal(t, turn.Status, ResponseStatusCompleted)
	assert.Equal(t, turn.Origin.Kind, TurnOriginInput)
	assert.Equal(t, turn.Revision, uint64(0))
	assert.Equal(t, turn.ToolCalls, []ToolCallRecord{{ID: "toolu_1", Name: "work", State: ToolCallStateCompleted}})
	assert.Equal(t, ran.get(), []string{turn.ID})

	sess := session.New("record-fields")
	mock = &responseLLM{responses: []*llm.Response{textResponse("end_turn", "hi")}}
	agent, err = NewAgent(AgentOptions{Model: mock, Session: sess})
	assert.NoError(t, err)
	resp, err = agent.CreateResponse(context.Background(), WithInput("hello"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Turn.Revision, uint64(1))
	assert.Equal(t, sess.Revision(), uint64(1))
}

// atomicCounter is a tool that records the turn ID of each call.
type atomicCounter struct {
	mu  sync.Mutex
	ids []string
}

func (c *atomicCounter) tool(name string) *funcTool {
	return &funcTool{name: name, call: func(ctx context.Context, input any) (*ToolResult, error) {
		c.mu.Lock()
		c.ids = append(c.ids, TurnID(ctx))
		c.mu.Unlock()
		return NewToolResultText(name + " ran"), nil
	}}
}

func (c *atomicCounter) get() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.ids...)
}

// A resume keeps the suspended turn's ID; its tools see it, and the
// suspended record lists the waiting call.
func TestTurnIDStableAcrossResume(t *testing.T) {
	var ids []string
	var ran atomicCounter
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "approve", `{}`)),
		callResponse("tool_use", toolUse("toolu_2", "work", `{}`)),
		textResponse("end_turn", "done"),
	}}
	sess := session.New("stable-id")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Tools: []Tool{suspendingTool("approve", &ids), ran.tool("work")}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	turnID := resp.Turn.ID
	assert.Equal(t, resp.Suspension.TurnID, turnID)
	assert.Equal(t, resp.Turn.Status, ResponseStatusSuspended)
	assert.Equal(t, resp.Turn.ToolCalls, []ToolCallRecord{{ID: "toolu_1", Name: "approve", State: ToolCallStateWaiting}})
	assert.Equal(t, ids, []string{turnID})
	assert.Equal(t, sess.LoadSuspension().TurnID, turnID)

	resp, err = agent.CreateResponse(context.Background(), WithToolResults(map[string]*ToolResult{"toolu_1": NewToolResultText("yes")}))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.Turn.ID, turnID)
	assert.Equal(t, ran.get(), []string{turnID})
	assert.Len(t, resp.Turn.ToolCalls, 2)
	assert.Equal(t, sess.EventCount(), 1)
}

// A resume request is checked against the session's open turn and revision
// before anything runs.
func TestResumeRequestChecks(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "approve", `{}`)),
		textResponse("end_turn", "done"),
	}}
	sess := session.New("resume-request")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Tools: []Tool{suspendingTool("approve", nil)}})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	results := map[string]*ToolResult{"toolu_1": NewToolResultText("yes")}

	_, err = agent.CreateResponse(context.Background(), WithResumeRequest(ResumeRequest{
		TurnID: resp.Turn.ID, ExpectedRevision: resp.Turn.Revision + 1, ToolResults: results,
	}))
	assert.True(t, errors.Is(err, ErrRevisionConflict))
	_, err = agent.CreateResponse(context.Background(), WithResumeRequest(ResumeRequest{
		TurnID: "turn_other", ExpectedRevision: resp.Turn.Revision, ToolResults: results,
	}))
	assert.True(t, errors.Is(err, ErrRevisionConflict))
	assert.Equal(t, mock.calls(), 1)
	assert.True(t, sess.IsSuspended())

	resp, err = agent.CreateResponse(context.Background(), WithResumeRequest(ResumeRequest{
		TurnID: resp.Turn.ID, ExpectedRevision: resp.Turn.Revision, ToolResults: results,
	}))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)

	// A session that is not a TurnStore has no revision to check.
	plain, err := NewAgent(AgentOptions{Model: mock, Session: &plainSession{id: "plain"}})
	assert.NoError(t, err)
	_, err = plain.CreateResponse(context.Background(), WithResumeRequest(ResumeRequest{ExpectedRevision: 1, ToolResults: results}))
	assert.ErrorContains(t, err, "TurnStore")
}

// A result a resume already accepted may be sent again and is skipped; a
// different result for the same call is refused.
func TestResentAndConflictingToolResults(t *testing.T) {
	mock := &scriptedLLM{script: []scriptedTurn{
		toolUseAssistantTurn(
			newScriptedToolUse("toolu_a", "tool_a", `{}`),
			newScriptedToolUse("toolu_b", "tool_b", `{}`),
		),
		finalTextTurn("done"),
	}}
	var postToolUse int
	sess := session.New("resent-results")
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{suspendingTool("tool_a", nil), suspendingTool("tool_b", nil)},
		Session:               sess,
		ParallelToolExecution: true,
		Hooks: Hooks{PostToolUse: []PostToolUseHook{func(ctx context.Context, hctx *HookContext) error {
			postToolUse++
			return nil
		}}},
	})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)

	supplied := map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}
	resp, err := agent.CreateResponse(context.Background(), WithToolResults(supplied))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, postToolUse, 1)
	assert.Len(t, resp.Suspension.CompletedToolCalls, 1)
	assert.Equal(t, resp.Suspension.CompletedToolCalls[0].ID, "toolu_a")

	_, err = agent.CreateResponse(context.Background(), WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A changed")}))
	assert.True(t, errors.Is(err, ErrConflictingToolResult))
	assert.True(t, errors.Is(err, ErrUnknownPendingToolCall))

	// Sent again with the remaining call's result: toolu_a is skipped, so
	// its hook does not run twice, and the turn completes.
	resp, err = agent.CreateResponse(context.Background(), WithToolResults(map[string]*ToolResult{
		"toolu_a": NewToolResultText("A done"),
		"toolu_b": NewToolResultText("B done"),
	}))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, postToolUse, 2)
	results := 0
	for _, msg := range resp.Turn.Messages {
		for _, c := range msg.Content {
			if _, ok := c.(*llm.ToolResultContent); ok {
				results++
			}
		}
	}
	assert.Equal(t, results, 2)
}

// A partial resume saves the results it accepted before emitting their
// items, so a callback error keeps them and the same resume can be sent
// again.
func TestPartialResumeCallbackErrorKeepsResults(t *testing.T) {
	mock := &scriptedLLM{script: []scriptedTurn{
		toolUseAssistantTurn(
			newScriptedToolUse("toolu_a", "tool_a", `{}`),
			newScriptedToolUse("toolu_b", "tool_b", `{}`),
		),
	}}
	sess := session.New("partial-callback")
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{suspendingTool("tool_a", nil), suspendingTool("tool_b", nil)},
		Session:               sess,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)

	callbackErr := errors.New("consumer failed")
	supplied := map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}
	resp, err := agent.CreateResponse(context.Background(), WithToolResults(supplied),
		WithEventCallback(func(ctx context.Context, item *ResponseItem) error { return callbackErr }))
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, callbackErr))
	state := sess.LoadSuspension()
	assert.Len(t, state.PendingToolCalls, 1)
	assert.Equal(t, state.PendingToolCalls[0].ID, "toolu_b")

	resp, err = agent.CreateResponse(context.Background(), WithToolResults(supplied))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, resp.Suspension.PendingToolCalls[0].ID, "toolu_b")
}

// CancelSuspendedTurn closes a suspended turn without a model call: the
// pending call is unknown, the completed sibling keeps its result, and the
// session moves on.
func TestCancelSuspendedTurn(t *testing.T) {
	var ran atomicCounter
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "approve", `{}`), toolUse("toolu_2", "look", `{}`)),
		textResponse("end_turn", "next answer"),
	}}
	var incompleteHooks int
	sess := session.New("cancel-suspended")
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Session:               sess,
		Tools:                 []Tool{suspendingTool("approve", nil), ran.tool("look")},
		ParallelToolExecution: true,
		Hooks: Hooks{OnIncompleteTurn: []IncompleteTurnHook{func(ctx context.Context, hctx *HookContext) (*IncompleteTurnDecision, error) {
			incompleteHooks++
			return nil, nil
		}}},
	})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	turnID := resp.Turn.ID

	var rec itemRecorder
	resp, err = agent.CancelSuspendedTurn(context.Background(), WithEventCallback(rec.callback))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusIncomplete)
	assert.Equal(t, resp.Turn.ID, turnID)
	assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
	outcome := resp.Turn.Outcome
	assert.Equal(t, outcome.Reason, TurnReasonCanceled)
	assert.Equal(t, outcome.Next, TurnNextReconcile)
	assert.Equal(t, outcome.ToolCalls, []ToolCallRecord{
		{ID: "toolu_1", Name: "approve", State: ToolCallStateUnknown},
		{ID: "toolu_2", Name: "look", State: ToolCallStateCompleted},
	})
	assert.Equal(t, mock.calls(), 1)
	assert.Equal(t, incompleteHooks, 1)
	assert.False(t, sess.IsSuspended())
	assert.Equal(t, sess.EventCount(), 1)
	assert.Equal(t, rec.last().Type, ResponseItemTypeTurnEnded)
	assert.Equal(t, countToolResultItems(rec.items, "toolu_1"), 1)

	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, llm.AnswerUnansweredToolCalls(saved), len(saved))
	recorded, ok := FindLatestTurnOutcome(saved)
	assert.True(t, ok)
	assert.Equal(t, recorded.Reason, TurnReasonCanceled)

	// The conversation moves on with new input.
	resp, err = agent.CreateResponse(context.Background(), WithInput("something else"))
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "next answer")
	assert.Equal(t, ran.get(), []string{turnID})

	_, err = agent.CancelSuspendedTurn(context.Background())
	assert.True(t, errors.Is(err, ErrNoSuspendedTurn))
}

// A stateless caller cancels the suspended turn it holds and appends the
// closed turn to its history.
func TestCancelSuspendedTurnStateless(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "approve", `{}`)),
	}}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{suspendingTool("approve", nil)}})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	state := resp.Suspension

	resp, err = agent.CancelSuspendedTurn(context.Background(), WithResume(state, nil))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusIncomplete)
	assert.Equal(t, resp.Turn.ID, state.TurnID)
	assert.Equal(t, resp.Turn.Persistence, PersistenceNone)
	// go, the call, its unknown result, the outcome.
	assert.Len(t, resp.Turn.Messages, 4)
	assert.Equal(t, len(llm.AnswerUnansweredToolCalls(resp.Turn.Messages)), 4)

	_, err = agent.CancelSuspendedTurn(context.Background(),
		WithResume(state, map[string]*ToolResult{"toolu_1": NewToolResultText("x")}))
	assert.Error(t, err)
}

// With RequireReconcile, new input after a turn with an unknown call is
// refused until the turn is continued.
func TestRequireReconcile(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "approve", `{}`)),
		textResponse("end_turn", "checked"),
		textResponse("end_turn", "next"),
	}}
	sess := session.New("require-reconcile")
	agent, err := NewAgent(AgentOptions{
		Model:           mock,
		Session:         sess,
		Tools:           []Tool{suspendingTool("approve", nil)},
		IncompleteTurns: IncompleteTurnOptions{RequireReconcile: true},
	})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	resp, err := agent.CancelSuspendedTurn(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, resp.Turn.Outcome.Next, TurnNextReconcile)

	_, err = agent.CreateResponse(context.Background(), WithInput("new topic"))
	assert.True(t, errors.Is(err, ErrUnreconciledToolCalls))
	assert.Equal(t, mock.calls(), 1)

	resp, err = agent.CreateResponse(context.Background(), WithContinue())
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "checked")
	resp, err = agent.CreateResponse(context.Background(), WithInput("new topic"))
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "next")
}

// New input after an incomplete turn starts a new turn and leaves the
// incomplete one superseded, its record intact.
func TestNewInputSupersedesIncompleteTurn(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		textResponse("max_tokens", "The answer is"),
		textResponse("end_turn", "Something else."),
	}}
	sess := session.New("supersede")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess})
	assert.NoError(t, err)
	first, err := agent.CreateResponse(context.Background(), WithInput("question"))
	assert.NoError(t, err)
	second, err := agent.CreateResponse(context.Background(), WithInput("never mind"))
	assert.NoError(t, err)
	assert.NotEqual(t, second.Turn.ID, first.Turn.ID)

	// The model saw the incomplete turn with its outcome.
	outcome, ok := FindLatestTurnOutcome(mock.request(1))
	assert.True(t, ok)
	assert.Equal(t, outcome.Reason, TurnReasonOutputLimit)

	turns, err := sess.Turns(context.Background())
	assert.NoError(t, err)
	assert.Len(t, turns, 2)
	assert.True(t, turns[0].Superseded)
	assert.Equal(t, turns[0].ID, first.Turn.ID)
	assert.Equal(t, turns[1].ID, second.Turn.ID)
}

// A write to the session during the turn makes its checkpoint a conflict:
// the turn is returned with Persistence failed.
func TestCheckpointConflictFailsPersistence(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		textResponse("end_turn", "one"),
		textResponse("end_turn", "two"),
	}}
	sess := session.New("conflict")
	compact := false
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Session: sess,
		Hooks: Hooks{PreGeneration: []PreGenerationHook{func(ctx context.Context, hctx *HookContext) error {
			if !compact {
				return nil
			}
			return sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
				return []*llm.Message{llm.NewUserTextMessage("summary")}, nil
			})
		}}},
	})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("first"))
	assert.NoError(t, err)

	compact = true
	resp, err := agent.CreateResponse(context.Background(), WithInput("second"))
	assert.True(t, errors.Is(err, ErrRevisionConflict))
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.Turn.Persistence, PersistenceFailed)
	assert.Equal(t, resp.Turn.Revision, uint64(0))
}

// A background task records the turn that started it, and the turn its
// results start links back to that turn.
func TestBackgroundResultsTurnOrigin(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "build", `{}`)),
		textResponse("end_turn", "started"),
		textResponse("end_turn", "built"),
	}}
	build := &funcTool{name: "build", call: func(ctx context.Context, input any) (*ToolResult, error) {
		return NewBackgroundResult(ctx, "building", func(ctx context.Context) (string, error) {
			return "ok", nil
		}), nil
	}}
	sess := session.New("background-origin")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Tools: []Tool{build}})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("build it"))
	assert.NoError(t, err)
	assert.Len(t, resp.BackgroundTasks, 1)
	assert.Equal(t, resp.BackgroundTasks[0].TurnID, resp.Turn.ID)

	next, err := ContinueWithBackground(context.Background(), agent, resp)
	assert.NoError(t, err)
	assert.NotEqual(t, next.Turn.ID, resp.Turn.ID)
	assert.Equal(t, next.Turn.Origin.Kind, TurnOriginBackground)
	assert.Equal(t, next.Turn.Origin.TurnID, resp.Turn.ID)
}

// RequireReconcile reads the session's latest turn record, not the active
// history: a compaction whose summary drops the outcome does not let new
// input past a turn with an unknown call. Continuing clears it.
func TestRequireReconcileSurvivesCompaction(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "approve", `{}`)),
		textResponse("end_turn", "checked"),
		textResponse("end_turn", "next"),
	}}
	sess := session.New("reconcile-compaction")
	agent, err := NewAgent(AgentOptions{
		Model:           mock,
		Session:         sess,
		Tools:           []Tool{suspendingTool("approve", nil)},
		IncompleteTurns: IncompleteTurnOptions{RequireReconcile: true},
	})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	resp, err := agent.CancelSuspendedTurn(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, resp.Turn.Outcome.Next, TurnNextReconcile)

	assert.NoError(t, sess.Compact(context.Background(), func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
		return []*llm.Message{llm.NewUserTextMessage("summary without the outcome")}, nil
	}))
	history, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	_, ok := FindLatestTurnOutcome(history)
	assert.False(t, ok)

	_, err = agent.CreateResponse(context.Background(), WithInput("new topic"))
	assert.True(t, errors.Is(err, ErrUnreconciledToolCalls))
	assert.Equal(t, mock.calls(), 1)

	resp, err = agent.CreateResponse(context.Background(), WithContinue())
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "checked")
	resp, err = agent.CreateResponse(context.Background(), WithInput("new topic"))
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "next")
}

// A resume request with no results on a session with nothing suspended
// fails; it never starts a turn.
func TestResumeRequestWithoutSuspension(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		textResponse("max_tokens", "The answer is"),
	}}
	sess := session.New("request-no-suspension")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("question"))
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithResumeRequest(ResumeRequest{
		TurnID: resp.Turn.ID, ExpectedRevision: resp.Turn.Revision,
	}))
	assert.True(t, errors.Is(err, ErrNoSuspendedTurn))
	assert.Equal(t, mock.calls(), 1)
	assert.Equal(t, sess.EventCount(), 1)
}

// A resume request with no results on a suspended session re-saves the
// suspension unchanged: no hook runs and no model is called.
func TestResumeRequestWithoutResultsStaysSuspended(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "approve", `{}`)),
	}}
	sess := session.New("request-no-results")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Tools: []Tool{suspendingTool("approve", nil)}})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)

	resp, err = agent.CreateResponse(context.Background(), WithResumeRequest(ResumeRequest{
		TurnID: resp.Turn.ID, ExpectedRevision: resp.Turn.Revision,
	}))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Len(t, resp.Suspension.PendingToolCalls, 1)
	assert.Equal(t, resp.Suspension.PendingToolCalls[0].ID, "toolu_1")
	assert.Equal(t, mock.calls(), 1)
	assert.True(t, sess.IsSuspended())
}

// A zero-value resume request is still a resume: it fails with nothing
// suspended, and re-saves a suspension unchanged.
func TestZeroResumeRequest(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		textResponse("end_turn", "hi"),
		callResponse("tool_use", toolUse("toolu_1", "approve", `{}`)),
	}}
	sess := session.New("zero-request")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Tools: []Tool{suspendingTool("approve", nil)}})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("hello"))
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithResumeRequest(ResumeRequest{}))
	assert.True(t, errors.Is(err, ErrNoSuspendedTurn))
	assert.Equal(t, mock.calls(), 1)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	resp, err = agent.CreateResponse(context.Background(), WithResumeRequest(ResumeRequest{}))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, resp.Suspension.PendingToolCalls[0].ID, "toolu_1")
	assert.Equal(t, mock.calls(), 2)

	// A session that is not a TurnStore cannot take one.
	plain, err := NewAgent(AgentOptions{Model: mock, Session: &plainSession{id: "plain"}})
	assert.NoError(t, err)
	_, err = plain.CreateResponse(context.Background(), WithResumeRequest(ResumeRequest{}))
	assert.ErrorContains(t, err, "TurnStore")
}
