package dive_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// These tests pin how an incomplete turn is kept: closed, reported, passed
// to OnIncompleteTurn hooks and saved, and how the model's own stops and
// WithContinue behave.

// responseLLM returns its scripted responses in order and records every
// request's messages. A nil response in the script is an error.
type responseLLM struct {
	mu        sync.Mutex
	responses []*llm.Response
	errs      []error
	requests  [][]*llm.Message
}

func (m *responseLLM) Name() string { return "response-llm" }

func (m *responseLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := &llm.Config{}
	cfg.Apply(opts...)
	msgs := make([]*llm.Message, len(cfg.Messages))
	for i, msg := range cfg.Messages {
		msgs[i] = msg.Copy()
	}
	m.requests = append(m.requests, msgs)
	i := len(m.requests) - 1
	if i >= len(m.responses) {
		return nil, fmt.Errorf("responseLLM: unexpected call %d", i+1)
	}
	if m.responses[i] == nil {
		return nil, m.errs[i]
	}
	resp := *m.responses[i]
	resp.Content = append([]llm.Content(nil), resp.Content...)
	return &resp, nil
}

func (m *responseLLM) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

func (m *responseLLM) request(i int) []*llm.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requests[i]
}

func textResponse(stopReason, text string) *llm.Response {
	return &llm.Response{
		Role:       llm.Assistant,
		Content:    []llm.Content{&llm.TextContent{Text: text}},
		StopReason: stopReason,
		Usage:      llm.Usage{InputTokens: 3, OutputTokens: 2},
	}
}

func callResponse(stopReason string, content ...llm.Content) *llm.Response {
	return &llm.Response{
		Role:       llm.Assistant,
		Content:    content,
		StopReason: stopReason,
		Usage:      llm.Usage{InputTokens: 3, OutputTokens: 2},
	}
}

func toolUse(id, name, input string) *llm.ToolUseContent {
	return &llm.ToolUseContent{ID: id, Name: name, Input: []byte(input)}
}

// countingTool records how many times it ran.
func countingTool(name string, ran *atomic.Int32) *funcTool {
	return &funcTool{name: name, call: func(ctx context.Context, input any) (*ToolResult, error) {
		ran.Add(1)
		return NewToolResultText(name + " ran"), nil
	}}
}

// requestHasReminder reports whether a request carries a reminder by name.
func requestHasReminder(msgs []*llm.Message, name string) bool {
	_, ok := FindLatestReminder(msgs, name)
	return ok
}

// A final answer cut at max_tokens is incomplete without an error, recorded
// and saved; WithContinue continues it with the turn-continue reminder in
// the request only.
func TestOutputLimitIsIncompleteAndContinues(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		textResponse("max_tokens", "The answer is"),
		textResponse("end_turn", " forty-two."),
	}}
	sess := session.New("output-limit")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("question"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusIncomplete)
	assert.Equal(t, resp.StopReason, "max_tokens")
	assert.Equal(t, resp.Turn.Outcome.Reason, TurnReasonOutputLimit)
	assert.Equal(t, resp.Turn.Outcome.Next, TurnNextContinue)
	assert.Equal(t, resp.Turn.Outcome.Error, "")
	assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
	assertClosedBy(t, resp.OutputMessages, TurnReasonOutputLimit)
	assert.Equal(t, resp.OutputMessages[0].Text(), "The answer is")

	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 3)
	assertClosedBy(t, saved, TurnReasonOutputLimit)

	resp, err = agent.CreateResponse(context.Background(), WithContinue())
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.OutputText(), " forty-two.")
	assert.Len(t, resp.Turn.Messages, 1)
	assert.True(t, requestHasReminder(mock.request(1), ReminderNameTurnContinue))

	// The continuation is its own event, holding its output alone.
	saved, err = sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 4)
	assert.False(t, requestHasReminder(saved, ReminderNameTurnContinue))
	assert.Equal(t, sess.EventCount(), 2)
}

// A response that filled the context window is incomplete with
// context_limit and advises new input; its tool call is not run.
func TestContextLimitIsIncomplete(t *testing.T) {
	var ran atomic.Int32
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("model_context_window_exceeded", &llm.TextContent{Text: "partial"}, toolUse("toolu_1", "work", `{}`)),
	}}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{countingTool("work", &ran)}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusIncomplete)
	assert.Equal(t, resp.Turn.Outcome.Reason, TurnReasonContextLimit)
	assert.Equal(t, resp.Turn.Outcome.Next, TurnNextInput)
	assert.Equal(t, ran.Load(), int32(0))
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{{ID: "toolu_1", Name: "work", State: ToolCallStateNotStarted}})
	assertEveryCallAnswered(t, resp.Turn.Messages)
}

// max_tokens inside a tool call: the truncated call is dropped from the
// message, a complete sibling is answered "not run", and no tool runs.
func TestOutputLimitDropsTruncatedToolCall(t *testing.T) {
	var ran atomic.Int32
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("max_tokens",
			toolUse("toolu_1", "work", `{"a":1}`),
			toolUse("toolu_2", "work", `{"a":`),
		),
	}}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{countingTool("work", &ran)}})
	assert.NoError(t, err)

	var rec itemRecorder
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"), WithEventCallback(rec.callback))
	assert.NoError(t, err)
	assert.Equal(t, resp.Turn.Outcome.Reason, TurnReasonOutputLimit)
	assert.Equal(t, ran.Load(), int32(0))

	assistant := resp.OutputMessages[0]
	assert.Len(t, assistant.Content, 1)
	assert.Equal(t, assistant.Content[0].(*llm.ToolUseContent).ID, "toolu_1")
	results := lastToolResults(t, resp.Turn.Messages)
	assert.Contains(t, resultText(t, results["toolu_1"]), ToolCallNotRunText)
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{{ID: "toolu_1", Name: "work", State: ToolCallStateNotStarted}})
	assert.Equal(t, callItems(rec.items)["toolu_1"], announcedAndAnswered)

	// The closed turn encodes: every call is answered.
	assertEveryCallAnswered(t, resp.Turn.Messages)
}

// A refusal is a completed turn: its tool call is answered "not run", and no
// outcome reminder is recorded.
func TestRefusalRunsNoToolAndCompletes(t *testing.T) {
	var ran atomic.Int32
	details := &llm.StopDetails{Type: "cyber"}
	refusal := callResponse("refusal", &llm.TextContent{Text: "I can't"}, toolUse("toolu_1", "work", `{}`))
	refusal.StopDetails = details
	mock := &responseLLM{responses: []*llm.Response{refusal}}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{countingTool("work", &ran)}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.StopReason, "refusal")
	assert.Equal(t, resp.StopDetails, details)
	assert.Equal(t, ran.Load(), int32(0))
	assert.Nil(t, resp.Turn.Outcome)
	assert.Len(t, resp.OutputMessages, 2)
	assert.Contains(t, resultText(t, lastToolResults(t, resp.OutputMessages)["toolu_1"]), ToolCallNotRunText)
	_, found := FindLatestTurnOutcome(resp.Turn.Messages)
	assert.False(t, found)
	assert.Equal(t, mock.calls(), 1)
}

// A stop reason Dive does not recognize never runs a call: with a call the
// turn is provider_stopped; with text only it is completed.
func TestUnrecognizedStopReason(t *testing.T) {
	var ran atomic.Int32
	mock := &responseLLM{responses: []*llm.Response{callResponse("some_new_reason", toolUse("toolu_1", "work", `{}`))}}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{countingTool("work", &ran)}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusIncomplete)
	assert.Equal(t, resp.Turn.Outcome.Reason, TurnReasonProviderStopped)
	assert.Equal(t, resp.Turn.Outcome.Error, "some_new_reason")
	assert.Equal(t, resp.Turn.Outcome.Next, TurnNextContinue)
	assert.Equal(t, ran.Load(), int32(0))
	assertClosedBy(t, resp.Turn.Messages, TurnReasonProviderStopped)

	mock = &responseLLM{responses: []*llm.Response{textResponse("some_new_reason", "fine")}}
	agent, err = NewAgent(AgentOptions{Model: mock})
	assert.NoError(t, err)
	resp, err = agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.StopReason, "some_new_reason")
	assert.Len(t, resp.Turn.Messages, 2)
}

func pausedResponse(id string) *llm.Response {
	return callResponse("pause_turn",
		&llm.TextContent{Text: "searching"},
		&llm.ServerToolUseContent{ID: id, Name: "web_search", Input: map[string]any{"query": "q"}},
	)
}

// A paused server tool loop is sent again as it stands, with no message
// between, and completes.
func TestPauseTurnIsContinued(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		pausedResponse("srvtoolu_1"),
		pausedResponse("srvtoolu_2"),
		textResponse("end_turn", "found it"),
	}}
	agent, err := NewAgent(AgentOptions{Model: mock, ToolIterationLimit: 1})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("search"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, mock.calls(), 3)
	assert.Equal(t, resp.OutputText(), "found it")

	// Each resend ends in the paused assistant message.
	for i := 1; i < 3; i++ {
		req := mock.request(i)
		assert.Equal(t, req[len(req)-1].Role, llm.Assistant)
	}
}

// Past the pause limit the turn is incomplete with pause, and the trailing
// server tool call is dropped from the saved message.
func TestPauseTurnLimit(t *testing.T) {
	var responses []*llm.Response
	for i := range 11 {
		responses = append(responses, pausedResponse(fmt.Sprintf("srvtoolu_%d", i)))
	}
	mock := &responseLLM{responses: responses}
	sess := session.New("pause-limit")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("search"))
	assert.NoError(t, err)
	assert.Equal(t, mock.calls(), 11)
	assert.Equal(t, resp.Status, ResponseStatusIncomplete)
	assert.Equal(t, resp.Turn.Outcome.Reason, TurnReasonPause)

	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	last := saved[len(saved)-2]
	assert.Equal(t, last.Role, llm.Assistant)
	for _, c := range last.Content {
		_, isServerCall := c.(*llm.ServerToolUseContent)
		assert.False(t, isServerCall)
	}
	assertClosedBy(t, saved, TurnReasonPause)
}

// A model that keeps calling tools past the iteration limit: the last calls
// are not run, the turn is incomplete, and a continuation gives it more
// iterations.
func TestIterationLimitIsIncomplete(t *testing.T) {
	var ran atomic.Int32
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "work", `{}`)),
		callResponse("tool_use", toolUse("toolu_2", "work", `{}`)),
		callResponse("tool_use", toolUse("toolu_3", "work", `{}`)),
		textResponse("end_turn", "done"),
	}}
	sess := session.New("iteration-limit")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, ToolIterationLimit: 1, Tools: []Tool{countingTool("work", &ran)}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusIncomplete)
	assert.Equal(t, resp.Turn.Outcome.Reason, TurnReasonIterationLimit)
	assert.Equal(t, resp.Turn.Outcome.Next, TurnNextContinue)
	assert.Equal(t, ran.Load(), int32(1))
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{{ID: "toolu_2", Name: "work", State: ToolCallStateNotStarted}})

	resp, err = agent.CreateResponse(context.Background(), WithContinue())
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, ran.Load(), int32(2))
	assert.Equal(t, resp.OutputText(), "done")
}

// eventStreamLLM streams each scripted event list in turn, then ends the
// stream with the matching error, which may be nil.
type eventStreamLLM struct {
	mu      sync.Mutex
	streams [][]*llm.Event
	errs    []error
	calls   int
}

func (m *eventStreamLLM) Name() string { return "event-stream" }
func (m *eventStreamLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	return nil, errors.New("not used")
}
func (m *eventStreamLLM) Stream(ctx context.Context, opts ...llm.Option) (llm.StreamIterator, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.calls
	m.calls++
	if i >= len(m.streams) {
		return nil, fmt.Errorf("eventStreamLLM: unexpected call %d", i+1)
	}
	return &brokenStreamIterator{events: m.streams[i], err: m.errs[i]}, nil
}

func idx(i int) *int { return &i }

// partialStream is a stream that delivered a finished text block, a text
// block still streaming, a thinking block without its signature and a
// half-written tool call, and no usage.
func partialStream() []*llm.Event {
	return []*llm.Event{
		{Type: llm.EventTypeMessageStart, Message: &llm.Response{ID: "msg_1", Role: llm.Assistant}},
		{Type: llm.EventTypeContentBlockStart, Index: idx(0), ContentBlock: &llm.EventContentBlock{Type: llm.ContentTypeText}},
		{Type: llm.EventTypeContentBlockDelta, Index: idx(0), Delta: &llm.EventDelta{Type: llm.EventDeltaTypeText, Text: "Done: "}},
		{Type: llm.EventTypeContentBlockStop, Index: idx(0)},
		{Type: llm.EventTypeContentBlockStart, Index: idx(1), ContentBlock: &llm.EventContentBlock{Type: llm.ContentTypeThinking}},
		{Type: llm.EventTypeContentBlockDelta, Index: idx(1), Delta: &llm.EventDelta{Type: llm.EventDeltaTypeThinking, Thinking: "hmm"}},
		{Type: llm.EventTypeContentBlockStart, Index: idx(2), ContentBlock: &llm.EventContentBlock{Type: llm.ContentTypeText}},
		{Type: llm.EventTypeContentBlockDelta, Index: idx(2), Delta: &llm.EventDelta{Type: llm.EventDeltaTypeText, Text: "the result is"}},
		{Type: llm.EventTypeContentBlockStart, Index: idx(3), ContentBlock: &llm.EventContentBlock{Type: llm.ContentTypeToolUse, ID: "toolu_1", Name: "work"}},
		{Type: llm.EventTypeContentBlockDelta, Index: idx(3), Delta: &llm.EventDelta{Type: llm.EventDeltaTypeInputJSON, PartialJSON: `{"a":`}},
	}
}

// A stream cut off mid-response keeps the text the model was writing as
// the last assistant message; the unsigned thinking block and the
// half-written call are dropped, and the usage is marked unknown.
func TestInterruptedStreamKeepsPartialText(t *testing.T) {
	errNetwork := errors.New("connection reset")
	mock := &eventStreamLLM{streams: [][]*llm.Event{partialStream()}, errs: []error{errNetwork}}
	sess := session.New("partial-text")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	genErr := assertIncomplete(t, resp, err, TurnReasonStreamInterrupted, TurnNextContinue)
	assert.True(t, resp.Turn.Outcome.UsageUnknown)
	assert.Len(t, genErr.OutputMessages, 2)
	partial := genErr.OutputMessages[0]
	assert.Equal(t, partial.Role, llm.Assistant)
	assert.Len(t, partial.Content, 2)
	assert.Equal(t, partial.Content[0].(*llm.TextContent).Text, "Done: ")
	assert.Equal(t, partial.Content[1].(*llm.TextContent).Text, "the result is")

	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 3)
}

// DropPartialText leaves out the text still streaming; the finished block
// stays.
func TestDropPartialText(t *testing.T) {
	mock := &eventStreamLLM{streams: [][]*llm.Event{partialStream()}, errs: []error{errors.New("reset")}}
	agent, err := NewAgent(AgentOptions{Model: mock, IncompleteTurns: IncompleteTurnOptions{DropPartialText: true}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	genErr := assertIncomplete(t, resp, err, TurnReasonStreamInterrupted, TurnNextContinue)
	assert.Len(t, genErr.OutputMessages, 2)
	assert.Len(t, genErr.OutputMessages[0].Content, 1)
	assert.Equal(t, genErr.OutputMessages[0].Text(), "Done: ")
}

// A stream that ends without message_stop was interrupted, even though the
// iterator reported no error.
func TestStreamWithoutMessageStopIsInterrupted(t *testing.T) {
	mock := &eventStreamLLM{streams: [][]*llm.Event{partialStream()[:4]}, errs: []error{nil}}
	agent, err := NewAgent(AgentOptions{Model: mock})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	genErr := assertIncomplete(t, resp, err, TurnReasonStreamInterrupted, TurnNextContinue)
	assert.Equal(t, genErr.OutputMessages[0].Text(), "Done: ")
}

// Stop and PostGeneration aborts save the model's complete output.
func TestEndHookAbortsSaveTheOutput(t *testing.T) {
	for _, hookType := range []string{"Stop", "PostGeneration"} {
		t.Run(hookType, func(t *testing.T) {
			mock := &responseLLM{responses: []*llm.Response{textResponse("end_turn", "the answer")}}
			sess := session.New("end-hook-abort")
			hooks := Hooks{}
			if hookType == "Stop" {
				hooks.Stop = []StopHook{func(ctx context.Context, hctx *HookContext) (*StopDecision, error) {
					return nil, AbortGeneration("policy")
				}}
			} else {
				hooks.PostGeneration = []PostGenerationHook{func(ctx context.Context, hctx *HookContext) error {
					return AbortGeneration("policy")
				}}
			}
			agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Hooks: hooks})
			assert.NoError(t, err)

			resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
			assertIncomplete(t, resp, err, TurnReasonHookAbort, TurnNextInput)
			assert.Equal(t, resp.Turn.Outcome.Hook, hookType)
			saved, err := sess.Messages(context.Background())
			assert.NoError(t, err)
			assert.Len(t, saved, 3)
			assert.Equal(t, saved[1].Text(), "the answer")
			assertClosedBy(t, saved, TurnReasonHookAbort)
		})
	}
}

// A PreGeneration hook error saves the input and the outcome, nothing else.
func TestPreGenerationErrorSavesInputAndOutcome(t *testing.T) {
	mock := &responseLLM{}
	sess := session.New("pregen-error")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Hooks: Hooks{
		PreGeneration: []PreGenerationHook{func(ctx context.Context, hctx *HookContext) error {
			return errors.New("rejected input")
		}},
	}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("bad input"))
	assertIncomplete(t, resp, err, TurnReasonError, TurnNextInput)
	assert.Len(t, resp.OutputMessages, 1)
	assert.Nil(t, resp.Usage)
	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 2)
	assert.Equal(t, saved[0].Text(), "bad input")
	assertClosedBy(t, saved, TurnReasonError)
	assert.Equal(t, mock.calls(), 0)
}

// A provider error on the third model call: the first two iterations are
// saved with the outcome, and WithContinue calls the model once, running no
// tool again.
func TestProviderErrorThenContinue(t *testing.T) {
	var ran atomic.Int32
	errProvider := errors.New("overloaded")
	mock := &responseLLM{
		responses: []*llm.Response{
			callResponse("tool_use", toolUse("toolu_1", "work", `{}`)),
			callResponse("tool_use", toolUse("toolu_2", "work", `{}`)),
			nil,
			textResponse("end_turn", "all done"),
		},
		errs: []error{nil, nil, errProvider, nil},
	}
	sess := session.New("provider-error")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Tools: []Tool{countingTool("work", &ran)}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.True(t, errors.Is(err, errProvider))
	assertIncomplete(t, resp, err, TurnReasonProviderError, TurnNextContinue)
	assert.Equal(t, resp.Turn.Outcome.Error, "overloaded")
	assert.Equal(t, ran.Load(), int32(2))
	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 6)

	resp, err = agent.CreateResponse(context.Background(), WithContinue())
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "all done")
	assert.Equal(t, mock.calls(), 4)
	assert.Equal(t, ran.Load(), int32(2))

	// The failed turn's outcome is in the request the model saw, followed by
	// the turn-continue reminder.
	req := mock.request(3)
	outcome, ok := FindLatestTurnOutcome(req)
	assert.True(t, ok)
	assert.Equal(t, outcome.Reason, TurnReasonProviderError)
	assert.True(t, requestHasReminder(req, ReminderNameTurnContinue))
}

// A cancellation that arrives while the agent is suspending does not stop
// the suspension from being persisted: the OnSuspend hook sees an
// uncancelled context, the response is Suspended with no error, and the
// resume later succeeds.
func TestCancelWhileSuspendingPersistsSuspension(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "approve", `{}`)),
		textResponse("end_turn", "approved"),
	}}
	approve := &funcTool{name: "approve", call: func(ctx context.Context, input any) (*ToolResult, error) {
		// The request is dispatched, then the run is cancelled.
		cancel()
		return NewSuspendResult("waiting", nil), nil
	}}
	var hookCtxErr error
	sess := session.New("cancel-suspend")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Tools: []Tool{approve}, Hooks: Hooks{
		OnSuspend: []OnSuspendHook{func(ctx context.Context, hctx *HookContext) error {
			hookCtxErr = ctx.Err()
			return nil
		}},
	}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(ctx, WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
	assert.NoError(t, hookCtxErr)
	assert.True(t, sessIsSuspended(sess))

	resp, err = agent.CreateResponse(context.Background(), WithToolResults(map[string]*ToolResult{"toolu_1": NewToolResultText("yes")}))
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "approved")
}

// OnIncompleteTurn hooks see the closed turn on an uncancelled context, can
// rewrite what is saved and add a reminder, which lands before the outcome
// reminder; the saved event carries Metadata["outcome"].
func TestOnIncompleteTurnHook(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	sess, err := store.Open(context.Background(), "incomplete-hook")
	assert.NoError(t, err)

	errProvider := errors.New("provider failed: request req_123")
	mock := &responseLLM{responses: []*llm.Response{nil}, errs: []error{errProvider}}
	var hookCtxErr error
	var sawOutcome *TurnOutcome
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Hooks: Hooks{
		OnIncompleteTurn: []IncompleteTurnHook{func(ctx context.Context, hctx *HookContext) (*IncompleteTurnDecision, error) {
			hookCtxErr = ctx.Err()
			sawOutcome = hctx.Turn.Outcome
			hctx.Turn.Outcome.Error = "provider failed"
			hctx.OutputMessages = append(hctx.OutputMessages, llm.NewAssistantTextMessage("Sorry, that failed."))
			reminder, err := NewContextReminder("repair-note", "repaired")
			assert.NoError(t, err)
			assert.NoError(t, hctx.AppendReminder(reminder, Recorded))
			return nil, nil
		}},
	}})
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp, err := agent.CreateResponse(ctx, WithInput("go"))
	assert.Error(t, err)
	assert.NoError(t, hookCtxErr)
	assert.Equal(t, sawOutcome.Reason, TurnReasonProviderError)

	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 4)
	assert.Equal(t, saved[1].Text(), "Sorry, that failed.")
	_, ok := FindReminder(saved[2], "repair-note")
	assert.True(t, ok)
	outcome, ok := FindTurnOutcome(saved[3])
	assert.True(t, ok)
	assert.Equal(t, outcome.Error, "provider failed")
	assert.Equal(t, resp.Turn.Outcome.Error, "provider failed")

	// The event written to the store carries Metadata["outcome"].
	files, err := filepath.Glob(filepath.Join(dir, "*"))
	assert.NoError(t, err)
	assert.Len(t, files, 1)
	data, err := os.ReadFile(files[0])
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"outcome":"provider_error"`)
}

// A hook's Discard decision saves nothing.
func TestOnIncompleteTurnDiscard(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{nil}, errs: []error{errors.New("boom")}}
	sess := session.New("incomplete-discard")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Hooks: Hooks{
		OnIncompleteTurn: []IncompleteTurnHook{func(ctx context.Context, hctx *HookContext) (*IncompleteTurnDecision, error) {
			return &IncompleteTurnDecision{Discard: true}, nil
		}},
	}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assertIncomplete(t, resp, err, TurnReasonProviderError, TurnNextContinue)
	assert.Equal(t, resp.Turn.Persistence, PersistenceNone)
	assert.Equal(t, sess.EventCount(), 0)
}

// IncompleteTurns.Discard leaves an error exit unsaved, but a turn the model
// stopped short is still saved.
func TestDiscardStillSavesModelStops(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{textResponse("max_tokens", "cut")}}
	sess := session.New("discard-model-stop")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, IncompleteTurns: IncompleteTurnOptions{Discard: true}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Turn.Outcome.Reason, TurnReasonOutputLimit)
	assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
	assert.Equal(t, sess.EventCount(), 1)
}

// ctxCheckingSession refuses a write on a cancelled context, as a database
// session would, and records what it saved.
type ctxCheckingSession struct {
	mu    sync.Mutex
	saved [][]*llm.Message
	err   error
	block bool
}

func (s *ctxCheckingSession) ID() string { return "ctx-checking" }
func (s *ctxCheckingSession) Messages(context.Context) ([]*llm.Message, error) {
	return nil, nil
}
func (s *ctxCheckingSession) SaveTurn(ctx context.Context, msgs []*llm.Message, _ *llm.Usage) error {
	if s.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.err != nil {
		return s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, msgs)
	return nil
}

// The end-of-invocation write runs on a context without the cancellation,
// so a cancelled turn is still saved; the write's error sets Persistence.
func TestSalvageSave(t *testing.T) {
	t.Run("cancelled turn saved", func(t *testing.T) {
		sess := &ctxCheckingSession{}
		agent, err := NewAgent(AgentOptions{Model: blockingLLM{}, Session: sess})
		assert.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()
		resp, err := agent.CreateResponse(ctx, WithInput("hi"))
		assertIncomplete(t, resp, err, TurnReasonCanceled, TurnNextInput)
		assert.Equal(t, resp.Turn.Persistence, PersistenceSaved)
		assert.Len(t, sess.saved, 1)
	})
	t.Run("rejected write is failed", func(t *testing.T) {
		sess := &ctxCheckingSession{err: fmt.Errorf("quota: %w", ErrSaveRejected)}
		mock := &responseLLM{responses: []*llm.Response{nil}, errs: []error{errors.New("boom")}}
		agent, err := NewAgent(AgentOptions{Model: mock, Session: sess})
		assert.NoError(t, err)
		resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
		assert.True(t, errors.Is(err, ErrSaveRejected))
		assert.Equal(t, resp.Turn.Persistence, PersistenceFailed)
		assert.Equal(t, resp.Turn.Outcome.Reason, TurnReasonProviderError)
	})
	t.Run("timed out write is unknown", func(t *testing.T) {
		sess := &ctxCheckingSession{block: true}
		mock := &responseLLM{responses: []*llm.Response{nil}, errs: []error{errors.New("boom")}}
		agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, IncompleteTurns: IncompleteTurnOptions{SaveTimeout: 10 * time.Millisecond}})
		assert.NoError(t, err)
		resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
		assert.True(t, errors.Is(err, context.DeadlineExceeded))
		assert.Equal(t, resp.Turn.Persistence, PersistenceUnknown)
		assert.Equal(t, resp.Turn.Outcome.Reason, TurnReasonProviderError)
	})
	t.Run("completed turn rejected", func(t *testing.T) {
		sess := &ctxCheckingSession{err: fmt.Errorf("read only: %w", ErrSaveRejected)}
		mock := &responseLLM{responses: []*llm.Response{textResponse("end_turn", "hi")}}
		agent, err := NewAgent(AgentOptions{Model: mock, Session: sess})
		assert.NoError(t, err)
		resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
		assert.True(t, errors.Is(err, ErrSaveRejected))
		assert.Equal(t, resp.Status, ResponseStatusCompleted)
		assert.Equal(t, resp.Turn.Persistence, PersistenceFailed)
	})
}

// A write session.Session refuses before writing reports failed.
func TestSessionRefusalIsFailed(t *testing.T) {
	assert.True(t, errors.Is(session.ErrSuspendedSession, ErrSaveRejected))
	assert.True(t, errors.Is(session.ErrNotSuspended, ErrSaveRejected))
}

// Two continuations after a stopped turn: three events, nothing saved
// twice, TotalUsage the sum, and each Turn.Usage its own.
func TestContinuationsAreTheirOwnEvents(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		textResponse("max_tokens", "one"),
		textResponse("max_tokens", " two"),
		textResponse("end_turn", " three"),
	}}
	sess := session.New("continuations")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("count"))
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithContinue())
	assert.NoError(t, err)
	assert.Equal(t, resp.Turn.Usage.InputTokens, 3)
	resp, err = agent.CreateResponse(context.Background(), WithContinue())
	assert.NoError(t, err)
	assert.Equal(t, resp.Turn.Usage.InputTokens, 3)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)

	assert.Equal(t, sess.EventCount(), 3)
	assert.Equal(t, sess.TotalUsage().InputTokens, 9)
	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	// input, one, outcome, two, outcome, three
	assert.Len(t, saved, 6)
}

// A resumed turn's usage includes what the suspension had accumulated.
func TestResumedTurnUsageIncludesSuspension(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{
		callResponse("tool_use", toolUse("toolu_1", "approve", `{}`)),
		textResponse("end_turn", "approved"),
	}}
	approve := &funcTool{name: "approve", call: func(ctx context.Context, input any) (*ToolResult, error) {
		return NewSuspendResult("waiting", nil), nil
	}}
	sess := session.New("resume-usage")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Tools: []Tool{approve}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Suspension.Usage.InputTokens, 3)
	assert.Equal(t, sess.LoadSuspension().Usage.InputTokens, 3)

	resp, err = agent.CreateResponse(context.Background(), WithToolResults(map[string]*ToolResult{"toolu_1": NewToolResultText("yes")}))
	assert.NoError(t, err)
	assert.Equal(t, resp.Usage.InputTokens, 3)
	assert.Equal(t, resp.Turn.Usage.InputTokens, 6)
	assert.Equal(t, sess.TotalUsage().InputTokens, 6)
}

// A stateless caller appends Turn.Messages to the history it held and gets
// what a session would hold, for an incomplete turn as for a completed one.
func TestStatelessRecipeMatchesSession(t *testing.T) {
	script := func() *responseLLM {
		return &responseLLM{
			responses: []*llm.Response{
				callResponse("tool_use", toolUse("toolu_1", "work", `{}`)),
				nil,
				textResponse("end_turn", "done"),
			},
			errs: []error{nil, errors.New("boom"), nil},
		}
	}
	var ran atomic.Int32
	sess := session.New("stateless-recipe")
	sessAgent, err := NewAgent(AgentOptions{Model: script(), Session: sess, Tools: []Tool{countingTool("work", &ran)}})
	assert.NoError(t, err)
	_, err = sessAgent.CreateResponse(context.Background(), WithInput("go"))
	assert.Error(t, err)
	_, err = sessAgent.CreateResponse(context.Background(), WithContinue())
	assert.NoError(t, err)
	want, err := sess.Messages(context.Background())
	assert.NoError(t, err)

	agent, err := NewAgent(AgentOptions{Model: script(), Tools: []Tool{countingTool("work", &ran)}})
	assert.NoError(t, err)
	var history []*llm.Message
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.Error(t, err)
	history = append(history, resp.Turn.Messages...)
	resp, err = agent.CreateResponse(context.Background(), WithMessages(history...), WithContinue())
	assert.NoError(t, err)
	history = append(history, resp.Turn.Messages...)

	wantJSON, err := json.Marshal(want)
	assert.NoError(t, err)
	for i, msg := range history {
		history[i] = msg.Copy()
	}
	gotJSON, err := json.Marshal(history)
	assert.NoError(t, err)
	assert.Equal(t, string(gotJSON), string(wantJSON))
}

// WithContinue takes no input and needs a conversation.
func TestWithContinueErrors(t *testing.T) {
	mock := &responseLLM{}
	sess := session.New("continue-errors")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithContinue())
	assert.Error(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("hi"), WithContinue())
	assert.True(t, errors.Is(err, ErrContinueWithInput))

	stateless, err := NewAgent(AgentOptions{Model: mock})
	assert.NoError(t, err)
	_, err = stateless.CreateResponse(context.Background(), WithContinue())
	assert.Error(t, err)
	assert.Equal(t, mock.calls(), 0)
}

// A continuation of a history that ends in a finished exchange adds no
// reminder.
func TestWithContinueReminderOnlyWhenNeeded(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{textResponse("end_turn", "more")}}
	agent, err := NewAgent(AgentOptions{Model: mock})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(),
		WithMessages(llm.NewUserTextMessage("hi"), llm.NewAssistantTextMessage("hello"), llm.NewUserTextMessage("and?")),
		WithContinue())
	assert.NoError(t, err)
	assert.False(t, requestHasReminder(mock.request(0), ReminderNameTurnContinue))
}

// CloseTurn answers every open call, records the not-started ones, and ends
// the turn in the outcome reminder, without modifying its arguments.
func TestCloseTurn(t *testing.T) {
	assistant := &llm.Message{Role: llm.Assistant, Content: []llm.Content{
		toolUse("toolu_1", "a", `{}`),
		toolUse("toolu_2", "b", `{}`),
		toolUse("toolu_3", "c", `{}`),
	}}
	results := llm.NewToolResultMessage(&llm.ToolResultContent{ToolUseID: "toolu_1", Content: "ok"})
	turn := []*llm.Message{llm.NewUserTextMessage("go"), assistant, results}
	outcome := &TurnOutcome{
		Reason: TurnReasonCanceled,
		Error:  "context canceled",
		ToolCalls: []ToolCallRecord{
			{ID: "toolu_1", Name: "a", State: ToolCallStateCompleted},
			{ID: "toolu_2", Name: "b", State: ToolCallStateUnknown},
		},
		Next: TurnNextReconcile,
	}

	closed := CloseTurn(turn, outcome)
	assert.Len(t, closed, 4)
	assert.Len(t, results.Content, 1)
	assert.Len(t, outcome.ToolCalls, 2)
	answered := lastToolResults(t, closed)
	assert.Equal(t, answered["toolu_2"].Content, ToolCallUnknownText)
	assert.Equal(t, answered["toolu_3"].Content, ToolCallNotRunText)

	recorded, ok := FindTurnOutcome(closed[3])
	assert.True(t, ok)
	assert.Equal(t, recorded.Reason, TurnReasonCanceled)
	assert.Equal(t, recorded.Next, TurnNextReconcile)
	assert.Equal(t, recorded.ToolCalls[2], ToolCallRecord{ID: "toolu_3", Name: "c", State: ToolCallStateNotStarted})
	reminder, _ := FindReminder(closed[3], ReminderNameTurnIncomplete)
	assert.True(t, strings.HasPrefix(reminder.Content, "The previous turn was stopped by the user"))
	assert.Contains(t, reminder.Content, `"Unknown result:"`)
}

// Reminder details survive a copy and a FileStore round trip, and decode as
// a plain reminder where Details is ignored, as on an older version.
func TestReminderDetailsRoundTrip(t *testing.T) {
	outcome := &TurnOutcome{Reason: TurnReasonOutputLimit, Next: TurnNextContinue}
	msg := NewReminderMessage(NewTurnOutcomeReminder(outcome))

	copied, ok := FindTurnOutcome(msg.Copy())
	assert.True(t, ok)
	assert.Equal(t, copied, outcome)

	dir := t.TempDir()
	store, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	sess, err := store.Open(context.Background(), "details")
	assert.NoError(t, err)
	assert.NoError(t, sess.SaveTurn(context.Background(), []*llm.Message{llm.NewUserTextMessage("hi"), msg}, nil))
	reopened, err := session.NewFileStore(dir)
	assert.NoError(t, err)
	sess, err = reopened.Open(context.Background(), "details")
	assert.NoError(t, err)
	msgs, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	loaded, ok := FindLatestTurnOutcome(msgs)
	assert.True(t, ok)
	assert.Equal(t, loaded, outcome)

	data, err := json.Marshal(msg)
	assert.NoError(t, err)
	var old struct {
		Content []struct {
			Type    string `json:"type"`
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"content"`
	}
	assert.NoError(t, json.Unmarshal(data, &old))
	assert.Equal(t, old.Content[0].Name, ReminderNameTurnIncomplete)
	assert.NotEqual(t, old.Content[0].Content, "")
}

// A PostGeneration abort keeps the edits a Stop hook made to the output.
func TestPostGenerationAbortKeepsStopHookEdits(t *testing.T) {
	mock := &responseLLM{responses: []*llm.Response{textResponse("end_turn", "secret answer")}}
	sess := session.New("abort-keeps-edits")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess, Hooks: Hooks{
		Stop: []StopHook{func(ctx context.Context, hctx *HookContext) (*StopDecision, error) {
			hctx.Response.OutputMessages = []*llm.Message{llm.NewAssistantTextMessage("redacted")}
			return nil, nil
		}},
		PostGeneration: []PostGenerationHook{func(ctx context.Context, hctx *HookContext) error {
			return AbortGeneration("policy")
		}},
	}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assertIncomplete(t, resp, err, TurnReasonHookAbort, TurnNextInput)
	saved, err := sess.Messages(context.Background())
	assert.NoError(t, err)
	assert.Len(t, saved, 3)
	assert.Equal(t, saved[1].Text(), "redacted")
}
