package dive_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// These tests pin what each exit after the turn boundary returns.

// itemRecorder collects the items an event callback receives.
type itemRecorder struct {
	mu    sync.Mutex
	items []*ResponseItem
}

func (r *itemRecorder) callback(ctx context.Context, item *ResponseItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, item)
	return nil
}

func (r *itemRecorder) last() *ResponseItem {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.items) == 0 {
		return nil
	}
	return r.items[len(r.items)-1]
}

// assertIncomplete checks the shared shape of an incomplete return: the
// error wraps a *GenerationError whose Response is resp, and the turn is
// closed by the outcome reminder.
func assertIncomplete(t *testing.T, resp *Response, err error, reason TurnReason, next TurnNext) *GenerationError {
	t.Helper()
	assert.Error(t, err)
	assert.NotNil(t, resp)
	var genErr *GenerationError
	assert.True(t, errors.As(err, &genErr))
	assert.True(t, genErr.Response == resp)
	assert.Equal(t, resp.Status, ResponseStatusIncomplete)
	assert.NotNil(t, resp.FinishedAt)
	assert.NotNil(t, resp.Turn)
	assert.NotNil(t, resp.Turn.Outcome)
	assert.Equal(t, resp.Turn.Outcome.Reason, reason)
	assert.Equal(t, resp.Turn.Outcome.Next, next)
	assert.Equal(t, resp.Turn.Outcome.Error, err.Error())
	assert.NotNil(t, resp.Turn.Usage)
	assertClosedBy(t, resp.OutputMessages, reason)
	assertClosedBy(t, resp.Turn.Messages, reason)
	assertClosedBy(t, genErr.OutputMessages, reason)
	return genErr
}

// assertClosedBy checks that messages end in the outcome reminder of an
// incomplete turn with this reason.
func assertClosedBy(t *testing.T, messages []*llm.Message, reason TurnReason) {
	t.Helper()
	assert.True(t, len(messages) > 0)
	outcome, ok := FindTurnOutcome(messages[len(messages)-1])
	assert.True(t, ok, "the last message is the outcome reminder")
	assert.Equal(t, outcome.Reason, reason)
}

// A model error after a Stop-hook continuation reports the whole turn: both
// rounds' output, the continuation reminder, every item and the usage of
// every model call.
func TestGenerationErrorAfterStopContinuationCarriesWholeTurn(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			{text: "first round", usage: llm.Usage{InputTokens: 10, OutputTokens: 4}},
			// The script ends here, so the continuation's model call fails.
		},
	}
	sess := session.New("stop-continue-error")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Session: sess,
		Hooks: Hooks{
			Stop: []StopHook{
				func(ctx context.Context, hctx *HookContext) (*StopDecision, error) {
					return &StopDecision{Continue: true, Reason: "keep going"}, nil
				},
			},
		},
	})
	assert.NoError(t, err)

	var rec itemRecorder
	resp, err := agent.CreateResponse(context.Background(), WithInput("start"), WithEventCallback(rec.callback))
	genErr := assertIncomplete(t, resp, err, TurnReasonProviderError, TurnNextContinue)
	assert.Equal(t, mock.Calls(), 1)

	assert.Len(t, genErr.OutputMessages, 3)
	assert.Equal(t, genErr.OutputMessages[0].Text(), "first round")
	_, ok := FindReminder(genErr.OutputMessages[1], "stop-continuation")
	assert.True(t, ok)

	assert.Len(t, genErr.Items, 1)
	assert.Equal(t, genErr.Items[0].Type, ResponseItemTypeMessage)
	assert.Equal(t, genErr.Usage.InputTokens, 10)
	assert.Equal(t, genErr.Usage.OutputTokens, 4)

	// The response carries the same turn; Turn.Messages adds the input.
	assert.Len(t, resp.OutputMessages, 3)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, resp.Usage.InputTokens, 10)
	assert.Len(t, resp.Turn.Messages, 4)
	assert.Equal(t, resp.Turn.Messages[0].Text(), "start")
	assert.Equal(t, resp.Turn.Usage.InputTokens, 10)

	// The terminal item mirrors the turn and is not one of the items.
	last := rec.last()
	assert.Equal(t, last.Type, ResponseItemTypeTurnEnded)
	assert.True(t, last.Turn == resp.Turn)

	// The closed turn is saved.
	assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 4)
}

// A callback error on the assistant message item still reports the message
// and its usage: both are recorded before the callback runs.
func TestGenerationErrorWhenCallbackRejectsAssistantMessage(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			{text: "hello", usage: llm.Usage{InputTokens: 7, OutputTokens: 3}},
		},
	}
	agent, err := NewAgent(AgentOptions{Model: mock})
	assert.NoError(t, err)

	errRejected := errors.New("rejected")
	var turnEnded *ResponseItem
	resp, err := agent.CreateResponse(context.Background(),
		WithInput("hi"),
		WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
			switch item.Type {
			case ResponseItemTypeMessage:
				return errRejected
			case ResponseItemTypeTurnEnded:
				turnEnded = item
			}
			return nil
		}),
	)
	assert.True(t, errors.Is(err, errRejected))
	genErr := assertIncomplete(t, resp, err, TurnReasonCallbackError, TurnNextContinue)
	assert.Len(t, genErr.OutputMessages, 2)
	assert.Equal(t, genErr.OutputMessages[0].Text(), "hello")
	assert.Len(t, genErr.Items, 1)
	assert.Equal(t, genErr.Usage.InputTokens, 7)
	assert.Equal(t, genErr.Usage.OutputTokens, 3)
	assert.Equal(t, resp.StopReason, "stop")

	// The callback that failed still receives the terminal item.
	assert.NotNil(t, turnEnded)
}

// A model error after a full resume reports the caller-supplied results
// emitted in the resume phase, with zero usage. The closed turn replaces the
// suspended one, so the session is no longer suspended.
func TestGenerationErrorAfterResumeCarriesResumeItems(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "approve", `{}`)),
			// The script ends here, so the model call after the resume fails.
		},
	}
	tool := &scriptedTool{name: "approve", outcomes: []toolOutcome{{result: NewSuspendResult("wait", nil)}}}
	sess := session.New("resume-error")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)

	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("approved")}),
	)
	genErr := assertIncomplete(t, resp, err, TurnReasonProviderError, TurnNextContinue)
	assert.Len(t, genErr.Items, 1)
	assert.Equal(t, genErr.Items[0].Type, ResponseItemTypeToolCallResult)
	assert.Equal(t, genErr.Items[0].ToolCallResult.ID, "toolu_a")
	assert.Len(t, genErr.OutputMessages, 1)
	assert.NotNil(t, genErr.Usage)
	assert.Equal(t, genErr.Usage.InputTokens, 0)
	assert.Nil(t, resp.Usage)

	// Turn.Messages is the suspended turn with the supplied result merged,
	// then the outcome reminder.
	assert.Len(t, resp.Turn.Messages, 4)
	assert.Equal(t, resp.Turn.Messages[0].Text(), "start")
	assert.Equal(t, resp.Turn.Messages[2].Role, llm.User)

	assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
	assert.False(t, sessIsSuspended(sess))
	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 4)
}

// A partial resume never calls the model, so it reports no usage and no
// output; its items are the results the caller supplied.
func TestPartialResumeReportsNoUsage(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a", nil)}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait b", nil)}}}
	sess := session.New("partial-usage")
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{toolA, toolB},
		Session:               sess,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.NotNil(t, resp.Usage)

	var rec itemRecorder
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
		WithEventCallback(rec.callback),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Nil(t, resp.Usage)
	assert.Len(t, resp.OutputMessages, 0)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, resp.Items[0].ToolCallResult.ID, "toolu_a")
	assert.NotNil(t, resp.FinishedAt)
	assert.Equal(t, mock.Calls(), 1)

	assert.True(t, resp.Turn.Suspension == resp.Suspension)
	assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
	assert.Len(t, resp.Turn.Messages, 3)

	// A partial resume skips the suspended item but ends with turn_ended.
	assert.Len(t, rec.items, 2)
	assert.Equal(t, rec.items[0].Type, ResponseItemTypeToolCallResult)
	assert.Equal(t, rec.items[1].Type, ResponseItemTypeTurnEnded)
}

// A partial resume that fails leaves the turn suspended and returns no
// response; the error still carries the items so far.
func TestPartialResumeFailureReturnsNoResponse(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a", nil)}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait b", nil)}}}
	sess := session.New("partial-failure")
	abort := false
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{toolA, toolB},
		Session:               sess,
		ParallelToolExecution: true,
		Hooks: Hooks{
			PostGeneration: []PostGenerationHook{
				func(ctx context.Context, hctx *HookContext) error {
					if abort {
						return AbortGeneration("no")
					}
					return nil
				},
			},
		},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)

	abort = true
	var rec itemRecorder
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
		WithEventCallback(rec.callback),
	)
	assert.Nil(t, resp)
	var genErr *GenerationError
	assert.True(t, errors.As(err, &genErr))
	assert.Nil(t, genErr.Response)
	assert.Len(t, genErr.Items, 1)
	var abortErr *HookAbortError
	assert.True(t, errors.As(err, &abortErr))

	// No terminal item: the turn has not ended.
	assert.Len(t, rec.items, 1)
	assert.Equal(t, rec.items[0].Type, ResponseItemTypeToolCallResult)
	assert.True(t, sessIsSuspended(sess))
}

// A callback error on the terminal suspended item is logged: the suspended
// turn was already saved.
func TestSuspendedItemCallbackError(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "approve", `{}`)),
		},
	}
	tool := &scriptedTool{name: "approve", outcomes: []toolOutcome{{result: NewSuspendResult("wait", nil)}}}
	sess := session.New("suspended-item-error")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(),
		WithInput("start"),
		WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
			if item.Type == ResponseItemTypeSuspended {
				return errors.New("rejected")
			}
			return nil
		}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
	assert.True(t, sessIsSuspended(sess))
}

// An OnSuspend abort ends the turn incomplete; the suspension is not
// reported, since nothing recorded it. The suspending call may have
// dispatched its request, so it is unknown.
func TestOnSuspendAbortIsIncomplete(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "approve", `{}`)),
		},
	}
	tool := &scriptedTool{name: "approve", outcomes: []toolOutcome{{result: NewSuspendResult("wait", nil)}}}
	sess := session.New("on-suspend-abort")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{tool},
		Session: sess,
		Hooks: Hooks{
			OnSuspend: []OnSuspendHook{
				func(ctx context.Context, hctx *HookContext) error {
					return AbortGeneration("not now")
				},
			},
		},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assertIncomplete(t, resp, err, TurnReasonHookAbort, TurnNextReconcile)
	assert.Equal(t, resp.Turn.Outcome.Hook, "OnSuspend")
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{{ID: "toolu_a", Name: "approve", State: ToolCallStateUnknown}})
	assert.Nil(t, resp.Suspension)
	assert.Nil(t, resp.Turn.Suspension)
	assert.False(t, sessIsSuspended(sess))

	// The saved turn answers the call as unknown and ends in the outcome.
	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 4)
	assert.True(t, strings.Contains(resultText(t, lastToolResults(t, saved)["toolu_a"]), "Unknown result:"))
	assertClosedBy(t, saved, TurnReasonHookAbort)
}

// A completed turn reports what it saved.
func TestCompletedTurnReportsTurn(t *testing.T) {
	mock := &scriptedLLM{script: []scriptedTurn{{text: "hello", usage: llm.Usage{InputTokens: 5}}}}
	sess := session.New("completed-turn")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess})
	assert.NoError(t, err)

	var rec itemRecorder
	resp, err := agent.CreateResponse(context.Background(), WithInput("hi"), WithEventCallback(rec.callback))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Nil(t, resp.Turn.Outcome)
	assert.Nil(t, resp.Turn.Suspension)
	assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
	assert.Len(t, resp.Turn.Messages, 2)
	assert.Equal(t, resp.Turn.Messages[0].Text(), "hi")
	assert.Equal(t, resp.Turn.Messages[1].Text(), "hello")
	assert.Equal(t, resp.Turn.Usage.InputTokens, 5)

	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 2)

	// turn_ended is the last item emitted and is not in resp.Items.
	assert.Equal(t, rec.last().Type, ResponseItemTypeTurnEnded)
	assert.True(t, rec.last().Turn == resp.Turn)
	for _, item := range resp.Items {
		assert.NotEqual(t, item.Type, ResponseItemTypeTurnEnded)
	}
}

// Without a session nothing is persisted.
func TestStatelessTurnPersistenceNone(t *testing.T) {
	mock := &scriptedLLM{script: []scriptedTurn{finalTextTurn("hello")}}
	agent, err := NewAgent(AgentOptions{Model: mock})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Turn.Persistence, PersistenceNone)
	assert.Len(t, resp.Turn.Messages, 2)
}

// failingSaveSession is a Session whose SaveTurn fails.
type failingSaveSession struct{ err error }

func (s *failingSaveSession) ID() string { return "failing-save" }
func (s *failingSaveSession) Messages(context.Context) ([]*llm.Message, error) {
	return nil, nil
}
func (s *failingSaveSession) SaveTurn(context.Context, []*llm.Message, *llm.Usage) error {
	return s.err
}

// A completed turn whose save fails is returned with the error; the
// session may or may not hold it.
func TestCompletedTurnSaveFailure(t *testing.T) {
	mock := &scriptedLLM{script: []scriptedTurn{finalTextTurn("hello")}}
	errDisk := errors.New("disk full")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: &failingSaveSession{err: errDisk}})
	assert.NoError(t, err)

	var rec itemRecorder
	resp, err := agent.CreateResponse(context.Background(), WithInput("hi"), WithEventCallback(rec.callback))
	assert.True(t, errors.Is(err, errDisk))
	assert.NotNil(t, resp)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.Turn.Persistence, PersistenceUnknown)
	assert.Equal(t, resp.OutputText(), "hello")
	assert.Equal(t, rec.last().Type, ResponseItemTypeTurnEnded)
}

// blockingLLM waits for its context to end and returns its error.
type blockingLLM struct{}

func (blockingLLM) Name() string { return "blocking" }
func (blockingLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCancelledTurnIsIncomplete(t *testing.T) {
	agent, err := NewAgent(AgentOptions{Model: blockingLLM{}})
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	var rec itemRecorder
	resp, err := agent.CreateResponse(ctx, WithInput("hi"), WithEventCallback(rec.callback))
	assert.True(t, errors.Is(err, context.Canceled))
	assertIncomplete(t, resp, err, TurnReasonCanceled, TurnNextInput)

	// The terminal item is still delivered after the cancellation.
	assert.Equal(t, rec.last().Type, ResponseItemTypeTurnEnded)
}

func TestResponseTimeoutIsDeadline(t *testing.T) {
	agent, err := NewAgent(AgentOptions{Model: blockingLLM{}, ResponseTimeout: 10 * time.Millisecond})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
	assertIncomplete(t, resp, err, TurnReasonDeadline, TurnNextContinue)
}

// brokenStreamLLM streams its events and then fails with err.
type brokenStreamLLM struct {
	events []*llm.Event
	err    error
}

func (m *brokenStreamLLM) Name() string { return "broken-stream" }
func (m *brokenStreamLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	return nil, errors.New("not used")
}
func (m *brokenStreamLLM) Stream(ctx context.Context, opts ...llm.Option) (llm.StreamIterator, error) {
	return &brokenStreamIterator{events: m.events, err: m.err}, nil
}

type brokenStreamIterator struct {
	events []*llm.Event
	err    error
	next   int
}

func (it *brokenStreamIterator) Next() bool {
	if it.next >= len(it.events) {
		return false
	}
	it.next++
	return true
}
func (it *brokenStreamIterator) Event() *llm.Event { return it.events[it.next-1] }
func (it *brokenStreamIterator) Err() error        { return it.err }
func (it *brokenStreamIterator) Close() error      { return nil }

// A stream that fails after delivering events was interrupted; one that
// fails before any event is a provider error.
func TestStreamFailureReasons(t *testing.T) {
	errNetwork := errors.New("connection reset")
	start := &llm.Event{
		Type:    llm.EventTypeMessageStart,
		Message: &llm.Response{ID: "msg_1", Role: llm.Assistant, Model: "broken-stream"},
	}

	agent, err := NewAgent(AgentOptions{Model: &brokenStreamLLM{events: []*llm.Event{start}, err: errNetwork}})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.True(t, errors.Is(err, errNetwork))
	assertIncomplete(t, resp, err, TurnReasonStreamInterrupted, TurnNextContinue)

	agent, err = NewAgent(AgentOptions{Model: &brokenStreamLLM{err: errNetwork}})
	assert.NoError(t, err)
	resp, err = agent.CreateResponse(context.Background(), WithInput("hi"))
	assertIncomplete(t, resp, err, TurnReasonProviderError, TurnNextContinue)
}

// A final Stop hook's changes to the response reach the PostGeneration
// hooks, the saved turn and the return.
func TestStopHookResponseEditsSurvive(t *testing.T) {
	mock := &scriptedLLM{script: []scriptedTurn{finalTextTurn("secret answer")}}
	sess := session.New("stop-hook-edits")
	var postGenText string
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Session: sess,
		Hooks: Hooks{
			Stop: []StopHook{
				func(ctx context.Context, hctx *HookContext) (*StopDecision, error) {
					hctx.Response.StopReason = "edited"
					hctx.Response.OutputMessages = []*llm.Message{llm.NewAssistantTextMessage("redacted")}
					return nil, nil
				},
			},
			PostGeneration: []PostGenerationHook{
				func(ctx context.Context, hctx *HookContext) error {
					postGenText = hctx.Response.OutputMessages[0].Text()
					return nil
				},
			},
		},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.NoError(t, err)
	assert.Equal(t, resp.StopReason, "edited")
	assert.Len(t, resp.OutputMessages, 1)
	assert.Equal(t, resp.OutputMessages[0].Text(), "redacted")
	assert.Equal(t, postGenText, "redacted")
	assert.Equal(t, resp.Turn.Messages[1].Text(), "redacted")

	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 2)
	assert.Equal(t, saved[1].Text(), "redacted")
}

// commitThenFailSession saves a suspended turn and then reports an error, as
// a store does when it fails after its write landed.
type commitThenFailSession struct {
	*session.Session
	fail bool
}

func (s *commitThenFailSession) SaveSuspendedTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage, state *SuspensionState) error {
	if err := s.Session.SaveSuspendedTurn(ctx, messages, usage, state); err != nil {
		return err
	}
	if s.fail {
		return errors.New("sync failed after write")
	}
	return nil
}

// A partial resume whose save fails after it landed returns no response;
// the reloaded suspension shows the result was taken, and resubmitting it
// is refused with ErrUnknownPendingToolCall.
func TestPartialResumeSaveFailureAfterCommit(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
			finalTextTurn("done"),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a", nil)}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait b", nil)}}}
	sess := &commitThenFailSession{Session: session.New("commit-then-fail")}
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{toolA, toolB},
		Session:               sess,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)

	sess.fail = true
	supplied := map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}
	resp, err = agent.CreateResponse(context.Background(), WithToolResults(supplied))
	assert.Nil(t, resp)
	var genErr *GenerationError
	assert.True(t, errors.As(err, &genErr))
	assert.Nil(t, genErr.Response)
	assert.ErrorContains(t, err, "save suspended turn")

	// The reloaded suspension holds only the call still pending.
	sess.fail = false
	state := sess.LoadSuspension()
	assert.NotNil(t, state)
	assert.Len(t, state.PendingToolCalls, 1)
	assert.Equal(t, state.PendingToolCalls[0].ID, "toolu_b")

	_, err = agent.CreateResponse(context.Background(), WithToolResults(supplied))
	assert.True(t, errors.Is(err, ErrUnknownPendingToolCall))

	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_b": NewToolResultText("B done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
}

// A background task started before a later failure in the same batch is
// still reported on the incomplete response.
func TestIncompleteResponseKeepsStartedBackgroundTasks(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_bg", "start_job", `{}`),
				newScriptedToolUse("toolu_x", "blocked", `{}`),
			),
		},
	}
	release := make(chan struct{})
	defer close(release)
	startJob := &funcTool{
		name: "start_job",
		call: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewBackgroundResult(ctx, "long job", func(ctx context.Context) (string, error) {
				<-release
				return "finished", nil
			}), nil
		},
	}
	blocked := &funcTool{
		name: "blocked",
		call: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewToolResultText("unreachable"), nil
		},
	}
	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{startJob, blocked},
		Hooks: Hooks{
			PreToolUse: []PreToolUseHook{
				func(ctx context.Context, hctx *HookContext) error {
					if hctx.Tool.Name() == "blocked" {
						return AbortGeneration("stop here")
					}
					return nil
				},
			},
		},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assertIncomplete(t, resp, err, TurnReasonHookAbort, TurnNextInput)
	assert.Len(t, resp.BackgroundTasks, 1)
	assert.Equal(t, resp.BackgroundTasks[0].ToolUseID, "toolu_bg")
}

// PreGeneration and PreIteration aborts name their hook.
func TestEarlyHookAbortsNameTheHook(t *testing.T) {
	for _, hookType := range []string{"PreGeneration", "PreIteration"} {
		abort := func(ctx context.Context, hctx *HookContext) error { return AbortGeneration("no") }
		var hooks Hooks
		if hookType == "PreGeneration" {
			hooks.PreGeneration = []PreGenerationHook{abort}
		} else {
			hooks.PreIteration = []PreIterationHook{abort}
		}
		agent, err := NewAgent(AgentOptions{Model: &scriptedLLM{}, Hooks: hooks})
		assert.NoError(t, err)
		resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
		assertIncomplete(t, resp, err, TurnReasonHookAbort, TurnNextInput)
		assert.Equal(t, resp.Turn.Outcome.Hook, hookType)
	}
}

// funcTool is a tool that runs call.
type funcTool struct {
	name string
	call func(ctx context.Context, input any) (*ToolResult, error)
	ann  *ToolAnnotations
}

func (t *funcTool) Name() string                  { return t.name }
func (t *funcTool) Description() string           { return "test tool" }
func (t *funcTool) Schema() *Schema               { return &Schema{Type: Object} }
func (t *funcTool) Annotations() *ToolAnnotations { return t.ann }
func (t *funcTool) Call(ctx context.Context, input any) (*ToolResult, error) {
	return t.call(ctx, input)
}
