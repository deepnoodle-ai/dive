package dive_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// These tests pin how a tool batch that stops early is recorded: every call
// is answered by what is known about it, in the messages, in the items and
// in TurnOutcome.ToolCalls.

// lastToolResults returns the tool_result blocks of the last tool_result
// message in messages, by tool-use ID.
func lastToolResults(t *testing.T, messages []*llm.Message) map[string]*llm.ToolResultContent {
	t.Helper()
	for i := len(messages) - 1; i >= 0; i-- {
		results := map[string]*llm.ToolResultContent{}
		for _, c := range messages[i].Content {
			if trc, ok := c.(*llm.ToolResultContent); ok {
				results[trc.ToolUseID] = trc
			}
		}
		if len(results) > 0 {
			return results
		}
	}
	t.Fatal("no tool_result message")
	return nil
}

// resultText returns the JSON of a tool_result block's content, for
// substring checks.
func resultText(t *testing.T, trc *llm.ToolResultContent) string {
	t.Helper()
	b, err := json.Marshal(trc.Content)
	assert.NoError(t, err)
	return string(b)
}

// assertEveryCallAnswered checks that every tool_use block in messages has a
// tool_result block.
func assertEveryCallAnswered(t *testing.T, messages []*llm.Message) {
	t.Helper()
	answered := map[string]bool{}
	for _, msg := range messages {
		for _, c := range msg.Content {
			if trc, ok := c.(*llm.ToolResultContent); ok {
				answered[trc.ToolUseID] = true
			}
		}
	}
	for _, msg := range messages {
		for _, c := range msg.Content {
			if tu, ok := c.(*llm.ToolUseContent); ok {
				assert.True(t, answered[tu.ID], "tool call %s has no result", tu.ID)
			}
		}
	}
}

// callItems returns, per tool-use ID, the types of the tool_call and
// tool_call_result items in items, in order.
func callItems(items []*ResponseItem) map[string][]ResponseItemType {
	out := map[string][]ResponseItemType{}
	for _, item := range items {
		switch item.Type {
		case ResponseItemTypeToolCall:
			out[item.ToolCall.ID] = append(out[item.ToolCall.ID], item.Type)
		case ResponseItemTypeToolCallResult:
			out[item.ToolCallResult.ID] = append(out[item.ToolCallResult.ID], item.Type)
		}
	}
	return out
}

// resultItem returns the last tool_call_result item for id.
func resultItem(t *testing.T, items []*ResponseItem, id string) *ToolCallResult {
	t.Helper()
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Type == ResponseItemTypeToolCallResult && items[i].ToolCallResult.ID == id {
			return items[i].ToolCallResult
		}
	}
	t.Fatalf("no tool_call_result item for %s", id)
	return nil
}

var announcedAndAnswered = []ResponseItemType{ResponseItemTypeToolCall, ResponseItemTypeToolCallResult}

// Cancelled mid-batch, sequential: the running call records its own result,
// without PostToolUse hooks, and the call after it is answered "not run".
func TestSequentialCancelKeepsFinishedCallAndSkipsTheRest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mock := &scriptedLLM{script: []scriptedTurn{toolUseAssistantTurn(
		newScriptedToolUse("toolu_1", "first", `{}`),
		newScriptedToolUse("toolu_2", "second", `{}`),
	)}}
	first := &funcTool{name: "first", call: func(ctx context.Context, input any) (*ToolResult, error) {
		cancel()
		return NewToolResultText("first done"), nil
	}}
	var secondRan atomic.Bool
	second := &funcTool{name: "second", call: func(ctx context.Context, input any) (*ToolResult, error) {
		secondRan.Store(true)
		return NewToolResultText("second done"), nil
	}}
	var postToolUse atomic.Int32
	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{first, second},
		Hooks: Hooks{PostToolUse: []PostToolUseHook{func(ctx context.Context, hctx *HookContext) error {
			postToolUse.Add(1)
			return nil
		}}},
	})
	assert.NoError(t, err)

	rec := &itemRecorder{}
	resp, err := agent.CreateResponse(ctx, WithInput("go"), WithEventCallback(rec.callback))
	assertIncomplete(t, resp, err, TurnReasonCanceled, TurnNextInput)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.False(t, secondRan.Load())
	assert.Equal(t, postToolUse.Load(), int32(0))
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{
		{ID: "toolu_1", Name: "first", State: ToolCallStateCompleted},
		{ID: "toolu_2", Name: "second", State: ToolCallStateNotStarted},
	})

	// The input, the assistant message, then every call answered.
	assert.Len(t, resp.OutputMessages, 2)
	assert.Len(t, resp.Turn.Messages, 3)
	assertEveryCallAnswered(t, resp.Turn.Messages)
	results := lastToolResults(t, resp.Turn.Messages)
	assert.False(t, results["toolu_1"].IsError)
	assert.Contains(t, resultText(t, results["toolu_1"]), "first done")
	assert.True(t, results["toolu_2"].IsError)
	assert.Contains(t, resultText(t, results["toolu_2"]), ToolCallNotRunText)

	// Every tool_call item has a result item, then turn_ended.
	calls := callItems(rec.items)
	assert.Equal(t, calls["toolu_1"], announcedAndAnswered)
	assert.Equal(t, calls["toolu_2"], announcedAndAnswered)
	assert.True(t, errors.Is(resultItem(t, rec.items, "toolu_2").Error, ErrToolCallNotRun))
	assert.Equal(t, rec.last().Type, ResponseItemTypeTurnEnded)
	assert.Equal(t, callItems(resp.Items), calls)
}

// A PreToolUse abort in a sequential batch: the calls before it keep their
// results, it and the calls after it are not started.
func TestSequentialPreToolUseAbortAnswersTheRest(t *testing.T) {
	mock := &scriptedLLM{script: []scriptedTurn{toolUseAssistantTurn(
		newScriptedToolUse("toolu_1", "ok", `{}`),
		newScriptedToolUse("toolu_2", "blocked", `{}`),
		newScriptedToolUse("toolu_3", "ok", `{}`),
	)}}
	ok := &funcTool{name: "ok", call: func(ctx context.Context, input any) (*ToolResult, error) {
		return NewToolResultText("fine"), nil
	}}
	blocked := &funcTool{name: "blocked", call: func(ctx context.Context, input any) (*ToolResult, error) {
		t.Error("blocked ran")
		return nil, nil
	}}
	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{ok, blocked},
		Hooks: Hooks{PreToolUse: []PreToolUseHook{func(ctx context.Context, hctx *HookContext) error {
			if hctx.Tool.Name() == "blocked" {
				return AbortGeneration("stop")
			}
			return nil
		}}},
	})
	assert.NoError(t, err)

	rec := &itemRecorder{}
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"), WithEventCallback(rec.callback))
	assertIncomplete(t, resp, err, TurnReasonHookAbort, TurnNextInput)
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{
		{ID: "toolu_1", Name: "ok", State: ToolCallStateCompleted},
		{ID: "toolu_2", Name: "blocked", State: ToolCallStateNotStarted},
		{ID: "toolu_3", Name: "ok", State: ToolCallStateNotStarted},
	})
	assertEveryCallAnswered(t, resp.Turn.Messages)
	calls := callItems(rec.items)
	for _, id := range []string{"toolu_1", "toolu_2", "toolu_3"} {
		assert.Equal(t, calls[id], announcedAndAnswered, id)
	}
}

// A PreToolUse abort in a parallel batch: no call has started.
func TestParallelPreToolUseAbortStartsNoCall(t *testing.T) {
	mock := &scriptedLLM{script: []scriptedTurn{toolUseAssistantTurn(
		newScriptedToolUse("toolu_1", "ok", `{}`),
		newScriptedToolUse("toolu_2", "blocked", `{}`),
	)}}
	var ran atomic.Int32
	tool := func(name string) *funcTool {
		return &funcTool{name: name, call: func(ctx context.Context, input any) (*ToolResult, error) {
			ran.Add(1)
			return NewToolResultText("fine"), nil
		}}
	}
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{tool("ok"), tool("blocked")},
		ParallelToolExecution: true,
		Hooks: Hooks{PreToolUse: []PreToolUseHook{func(ctx context.Context, hctx *HookContext) error {
			if hctx.Tool.Name() == "blocked" {
				return AbortGeneration("stop")
			}
			return nil
		}}},
	})
	assert.NoError(t, err)

	rec := &itemRecorder{}
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"), WithEventCallback(rec.callback))
	assertIncomplete(t, resp, err, TurnReasonHookAbort, TurnNextInput)
	assert.Equal(t, ran.Load(), int32(0))
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{
		{ID: "toolu_1", Name: "ok", State: ToolCallStateNotStarted},
		{ID: "toolu_2", Name: "blocked", State: ToolCallStateNotStarted},
	})
	assertEveryCallAnswered(t, resp.Turn.Messages)
	calls := callItems(rec.items)
	assert.Equal(t, calls["toolu_1"], announcedAndAnswered)
	assert.Equal(t, calls["toolu_2"], announcedAndAnswered)
}

// Cancelled mid-batch, parallel: a finished call keeps its result, calls
// still running are unknown and not waited for, and their results arrive on
// handles that the next invocation delivers to the model.
func TestParallelCancelLeavesRunningCallsUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mock := &scriptedLLM{script: []scriptedTurn{
		toolUseAssistantTurn(
			newScriptedToolUse("toolu_fast", "fast", `{}`),
			newScriptedToolUse("toolu_s1", "slow", `{"panic":false}`),
			newScriptedToolUse("toolu_s2", "slow", `{"panic":true}`),
		),
		finalTextTurn("noted"),
	}}
	release := make(chan struct{})
	slowStarted := make(chan struct{}, 2)
	slow := &funcTool{name: "slow", call: func(ctx context.Context, input any) (*ToolResult, error) {
		slowStarted <- struct{}{}
		<-release // ignores cancellation
		if strings.Contains(string(input.([]byte)), "true") {
			panic("boom")
		}
		return NewToolResultText("committed"), nil
	}}
	fast := &funcTool{name: "fast", call: func(ctx context.Context, input any) (*ToolResult, error) {
		return NewToolResultText("fast done"), nil
	}}
	var mu sync.Mutex
	var backgroundHooks []string
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{fast, slow},
		ParallelToolExecution: true,
		Hooks: Hooks{
			PostToolUse: []PostToolUseHook{func(ctx context.Context, hctx *HookContext) error {
				if hctx.Tool.Name() == "fast" {
					<-slowStarted
					<-slowStarted
					cancel()
				}
				return nil
			}},
			PostBackgroundToolUse: []PostBackgroundToolUseHook{func(ctx context.Context, hctx *HookContext) error {
				mu.Lock()
				defer mu.Unlock()
				backgroundHooks = append(backgroundHooks, hctx.Call.ID)
				return nil
			}},
		},
	})
	assert.NoError(t, err)

	rec := &itemRecorder{}
	resp, err := agent.CreateResponse(ctx, WithInput("go"), WithEventCallback(rec.callback))
	assertIncomplete(t, resp, err, TurnReasonCanceled, TurnNextReconcile)
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{
		{ID: "toolu_fast", Name: "fast", State: ToolCallStateCompleted},
		{ID: "toolu_s1", Name: "slow", State: ToolCallStateUnknown},
		{ID: "toolu_s2", Name: "slow", State: ToolCallStateUnknown},
	})
	assertEveryCallAnswered(t, resp.Turn.Messages)
	results := lastToolResults(t, resp.Turn.Messages)
	assert.Contains(t, resultText(t, results["toolu_fast"]), "fast done")
	for _, id := range []string{"toolu_s1", "toolu_s2"} {
		assert.True(t, results[id].IsError)
		assert.Contains(t, resultText(t, results[id]), ToolCallUnknownText)
		item := resultItem(t, rec.items, id)
		assert.True(t, errors.Is(item.Error, ErrToolCallUnknown))
		assert.NotNil(t, item.BackgroundHandle)
	}
	calls := callItems(rec.items)
	for _, id := range []string{"toolu_fast", "toolu_s1", "toolu_s2"} {
		assert.Equal(t, calls[id], announcedAndAnswered, id)
	}

	// The running calls' handles are on the response, one per call.
	assert.Len(t, resp.BackgroundTasks, 2)
	byToolUse := map[string]*BackgroundTaskHandle{}
	for _, h := range resp.BackgroundTasks {
		byToolUse[h.ToolUseID] = h
	}
	assert.NotNil(t, byToolUse["toolu_s1"])
	assert.NotNil(t, byToolUse["toolu_s2"])
	assert.Equal(t, byToolUse["toolu_s1"].Description, byToolUse["toolu_s2"].Description)

	// The tools finish after the turn ended; their results arrive on the
	// handles, a panic as an error result.
	close(release)
	awaitCtx, awaitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer awaitCancel()
	late, err := AwaitBackgroundTasks(awaitCtx, resp.BackgroundTasks)
	assert.NoError(t, err)
	committed := late[byToolUse["toolu_s1"].TaskID]
	assert.False(t, committed.IsError)
	assert.Equal(t, committed.Content[0].Text, "committed")
	panicked := late[byToolUse["toolu_s2"].TaskID]
	assert.True(t, panicked.IsError)
	assert.Contains(t, panicked.Content[0].Text, "boom")

	// The next invocation shows the model both results, each named by its
	// tool-use ID, and fires PostBackgroundToolUse for each.
	history := append(resp.Turn.Messages, llm.NewUserTextMessage("what happened?"))
	_, err = agent.CreateResponse(context.Background(),
		WithMessages(history...),
		WithBackgroundResults(resp.BackgroundTasks, late),
	)
	assert.NoError(t, err)
	sent := mock.received[len(mock.received)-1]
	var reminder string
	for _, msg := range sent {
		b, err := json.Marshal(msg)
		assert.NoError(t, err)
		if text := string(b); strings.Contains(text, "Background task completed") {
			reminder = text
		}
	}
	assert.Contains(t, reminder, "Tool use ID: toolu_s1")
	assert.Contains(t, reminder, "Tool use ID: toolu_s2")
	assert.Contains(t, reminder, "committed")
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, len(backgroundHooks), 2)
}

// A read-only tool still running when the turn ends is unknown too; only
// the advice changes.
func TestParallelCancelReadOnlyUnknownCallAdvisesInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mock := &scriptedLLM{script: []scriptedTurn{toolUseAssistantTurn(
		newScriptedToolUse("toolu_fast", "fast", `{}`),
		newScriptedToolUse("toolu_read", "read", `{}`),
	)}}
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{})
	read := &funcTool{
		name: "read",
		ann:  &ToolAnnotations{ReadOnlyHint: true},
		call: func(ctx context.Context, input any) (*ToolResult, error) {
			close(started)
			<-release
			return NewToolResultText("read"), nil
		},
	}
	fast := &funcTool{name: "fast", call: func(ctx context.Context, input any) (*ToolResult, error) {
		return NewToolResultText("fast done"), nil
	}}
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{fast, read},
		ParallelToolExecution: true,
		Hooks: Hooks{PostToolUse: []PostToolUseHook{func(ctx context.Context, hctx *HookContext) error {
			<-started
			cancel()
			return nil
		}}},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(ctx, WithInput("go"))
	assertIncomplete(t, resp, err, TurnReasonCanceled, TurnNextInput)
	assert.Equal(t, resp.Turn.Outcome.ToolCalls[1], ToolCallRecord{ID: "toolu_read", Name: "read", State: ToolCallStateUnknown})
	assertEveryCallAnswered(t, resp.Turn.Messages)
}

// A result that landed after the batch stopped is kept, without
// PostToolUse hooks.
func TestParallelCancelKeepsLandedResultsWithoutHooks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mock := &scriptedLLM{script: []scriptedTurn{toolUseAssistantTurn(
		newScriptedToolUse("toolu_a", "a", `{}`),
		newScriptedToolUse("toolu_b", "b", `{}`),
	)}}
	bReturned := make(chan struct{})
	a := &funcTool{name: "a", call: func(ctx context.Context, input any) (*ToolResult, error) {
		return NewToolResultText("a done"), nil
	}}
	b := &funcTool{name: "b", call: func(ctx context.Context, input any) (*ToolResult, error) {
		defer close(bReturned)
		return NewToolResultText("b done"), nil
	}}
	var mu sync.Mutex
	var hooked []string
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{a, b},
		ParallelToolExecution: true,
		Hooks: Hooks{PostToolUse: []PostToolUseHook{func(ctx context.Context, hctx *HookContext) error {
			mu.Lock()
			hooked = append(hooked, hctx.Call.ID)
			mu.Unlock()
			// The first result's hook waits for the other tool to return,
			// then cancels, so the other result is in the channel.
			<-bReturned
			time.Sleep(50 * time.Millisecond)
			cancel()
			return nil
		}}},
	})
	assert.NoError(t, err)

	rec := &itemRecorder{}
	resp, err := agent.CreateResponse(ctx, WithInput("go"), WithEventCallback(rec.callback))
	assertIncomplete(t, resp, err, TurnReasonCanceled, TurnNextInput)
	for _, record := range resp.Turn.Outcome.ToolCalls {
		assert.Equal(t, record.State, ToolCallStateCompleted, record.ID)
	}
	mu.Lock()
	assert.Len(t, hooked, 1)
	mu.Unlock()
	results := lastToolResults(t, resp.Turn.Messages)
	assert.Contains(t, resultText(t, results["toolu_a"]), "a done")
	assert.Contains(t, resultText(t, results["toolu_b"]), "b done")
	calls := callItems(rec.items)
	assert.Equal(t, calls["toolu_a"], announcedAndAnswered)
	assert.Equal(t, calls["toolu_b"], announcedAndAnswered)
}

// A soft cancel during a sequential batch: the running call finishes with
// its hooks, the calls after it are not started, and no model call follows.
func TestSoftCancelDuringSequentialBatch(t *testing.T) {
	ctx, softCancel := WithSoftCancel(context.Background())
	mock := &scriptedLLM{script: []scriptedTurn{
		toolUseAssistantTurn(
			newScriptedToolUse("toolu_1", "first", `{}`),
			newScriptedToolUse("toolu_2", "second", `{}`),
		),
		finalTextTurn("unreachable"),
	}}
	first := &funcTool{name: "first", call: func(ctx context.Context, input any) (*ToolResult, error) {
		softCancel()
		assert.True(t, SoftCanceled(ctx))
		assert.NoError(t, ctx.Err())
		return NewToolResultText("first done"), nil
	}}
	second := &funcTool{name: "second", call: func(ctx context.Context, input any) (*ToolResult, error) {
		t.Error("second ran")
		return nil, nil
	}}
	var postToolUse atomic.Int32
	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{first, second},
		Hooks: Hooks{PostToolUse: []PostToolUseHook{func(ctx context.Context, hctx *HookContext) error {
			postToolUse.Add(1)
			return nil
		}}},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(ctx, WithInput("go"))
	assertIncomplete(t, resp, err, TurnReasonCanceled, TurnNextInput)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Equal(t, mock.Calls(), 1)
	assert.Equal(t, postToolUse.Load(), int32(1))
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{
		{ID: "toolu_1", Name: "first", State: ToolCallStateCompleted},
		{ID: "toolu_2", Name: "second", State: ToolCallStateNotStarted},
	})
	assertEveryCallAnswered(t, resp.Turn.Messages)
}

// A soft cancel during a parallel batch lets every running call finish and
// stops before the next model call: no call is unknown.
func TestSoftCancelDuringParallelBatch(t *testing.T) {
	ctx, softCancel := WithSoftCancel(context.Background())
	mock := &scriptedLLM{script: []scriptedTurn{
		toolUseAssistantTurn(
			newScriptedToolUse("toolu_a", "a", `{}`),
			newScriptedToolUse("toolu_b", "b", `{}`),
		),
		finalTextTurn("unreachable"),
	}}
	aStarted := make(chan struct{})
	a := &funcTool{name: "a", call: func(ctx context.Context, input any) (*ToolResult, error) {
		close(aStarted)
		time.Sleep(50 * time.Millisecond)
		return NewToolResultText("a done"), nil
	}}
	b := &funcTool{name: "b", call: func(ctx context.Context, input any) (*ToolResult, error) {
		<-aStarted
		softCancel()
		return NewToolResultText("b done"), nil
	}}
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{a, b},
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(ctx, WithInput("go"))
	assertIncomplete(t, resp, err, TurnReasonCanceled, TurnNextInput)
	assert.Equal(t, mock.Calls(), 1)
	assert.Len(t, resp.Turn.Outcome.ToolCalls, 0)
	assert.Len(t, resp.BackgroundTasks, 0)
	results := lastToolResults(t, resp.Turn.Messages)
	assert.Contains(t, resultText(t, results["toolu_a"]), "a done")
	assert.Contains(t, resultText(t, results["toolu_b"]), "b done")
}

// A soft cancel during a model call: the calls the response requests are
// answered "not run" and none runs.
func TestSoftCancelDuringModelCall(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		ctx, softCancel := WithSoftCancel(context.Background())
		model := &softCancellingLLM{
			scriptedLLM: scriptedLLM{script: []scriptedTurn{toolUseAssistantTurn(
				newScriptedToolUse("toolu_1", "tool", `{}`),
				newScriptedToolUse("toolu_2", "tool", `{}`),
			)}},
			cancel: softCancel,
		}
		tool := &funcTool{name: "tool", call: func(ctx context.Context, input any) (*ToolResult, error) {
			t.Error("tool ran")
			return nil, nil
		}}
		agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{tool}, ParallelToolExecution: parallel})
		assert.NoError(t, err)

		rec := &itemRecorder{}
		resp, err := agent.CreateResponse(ctx, WithInput("go"), WithEventCallback(rec.callback))
		assertIncomplete(t, resp, err, TurnReasonCanceled, TurnNextInput)
		assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{
			{ID: "toolu_1", Name: "tool", State: ToolCallStateNotStarted},
			{ID: "toolu_2", Name: "tool", State: ToolCallStateNotStarted},
		})
		assertEveryCallAnswered(t, resp.Turn.Messages)
		calls := callItems(rec.items)
		assert.Equal(t, calls["toolu_1"], announcedAndAnswered)
		assert.Equal(t, calls["toolu_2"], announcedAndAnswered)
	}
}

// A soft cancel requested while a PreIteration hook runs stops the turn
// before the model is called.
func TestSoftCancelDuringPreIterationHook(t *testing.T) {
	ctx, softCancel := WithSoftCancel(context.Background())
	mock := &scriptedLLM{script: []scriptedTurn{finalTextTurn("unreachable")}}
	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Hooks: Hooks{PreIteration: []PreIterationHook{func(ctx context.Context, hctx *HookContext) error {
			softCancel()
			return nil
		}}},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(ctx, WithInput("go"))
	assertIncomplete(t, resp, err, TurnReasonCanceled, TurnNextInput)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Equal(t, mock.Calls(), 0)
}

// softCancellingLLM requests a soft cancel while it generates.
type softCancellingLLM struct {
	scriptedLLM
	cancel func()
}

func (m *softCancellingLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	m.cancel()
	return m.scriptedLLM.Generate(ctx, opts...)
}

// A soft cancel request travels with the context into nested soft-cancel
// scopes, and the cancel function is idempotent.
func TestSoftCanceledReachesNestedContexts(t *testing.T) {
	assert.False(t, SoftCanceled(context.Background()))
	outer, cancelOuter := WithSoftCancel(context.Background())
	inner, cancelInner := WithSoftCancel(context.WithValue(outer, struct{}{}, 1))
	assert.False(t, SoftCanceled(outer))
	assert.False(t, SoftCanceled(inner))

	cancelInner()
	assert.True(t, SoftCanceled(inner))
	assert.False(t, SoftCanceled(outer))

	cancelOuter()
	cancelOuter()
	assert.True(t, SoftCanceled(outer))
	assert.NoError(t, outer.Err())
}

// A full resume that stops while running the suspended batch's not-started
// calls records the whole batch and answers every call in the merged
// tool_result message.
func TestResumeStoppedBatchRecordsWholeBatch(t *testing.T) {
	mock := &scriptedLLM{script: []scriptedTurn{toolUseAssistantTurn(
		newScriptedToolUse("toolu_a", "tool_a", `{}`),
		newScriptedToolUse("toolu_b", "tool_b", `{}`),
		newScriptedToolUse("toolu_c", "tool_c", `{}`),
	)}}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a", nil)}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	toolB := &funcTool{name: "tool_b", call: func(ctx context.Context, input any) (*ToolResult, error) {
		cancel()
		return NewToolResultText("b done"), nil
	}}
	toolC := &funcTool{name: "tool_c", call: func(ctx context.Context, input any) (*ToolResult, error) {
		t.Error("tool_c ran")
		return nil, nil
	}}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{toolA, toolB, toolC}})
	assert.NoError(t, err)

	suspended, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, suspended.Status, ResponseStatusSuspended)

	resp, err := agent.CreateResponse(ctx,
		WithResume(suspended.Suspension, map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
	)
	assertIncomplete(t, resp, err, TurnReasonCanceled, TurnNextInput)
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{
		{ID: "toolu_a", Name: "tool_a", State: ToolCallStateCompleted},
		{ID: "toolu_b", Name: "tool_b", State: ToolCallStateCompleted},
		{ID: "toolu_c", Name: "tool_c", State: ToolCallStateNotStarted},
	})
	assertEveryCallAnswered(t, resp.Turn.Messages)
	results := lastToolResults(t, resp.Turn.Messages)
	assert.Contains(t, resultText(t, results["toolu_a"]), "A done")
	assert.Contains(t, resultText(t, results["toolu_b"]), "b done")
	assert.Contains(t, resultText(t, results["toolu_c"]), ToolCallNotRunText)
}

// A callback error on a tool_call_result item in a parallel batch keeps the
// finished result and answers the running call as unknown.
func TestParallelCallbackErrorAnswersRunningCall(t *testing.T) {
	mock := &scriptedLLM{script: []scriptedTurn{toolUseAssistantTurn(
		newScriptedToolUse("toolu_fast", "fast", `{}`),
		newScriptedToolUse("toolu_slow", "slow", `{}`),
	)}}
	release := make(chan struct{})
	defer close(release)
	slowStarted := make(chan struct{})
	slow := &funcTool{name: "slow", call: func(ctx context.Context, input any) (*ToolResult, error) {
		close(slowStarted)
		<-release
		return NewToolResultText("slow done"), nil
	}}
	fast := &funcTool{name: "fast", call: func(ctx context.Context, input any) (*ToolResult, error) {
		<-slowStarted
		return NewToolResultText("fast done"), nil
	}}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{fast, slow}, ParallelToolExecution: true})
	assert.NoError(t, err)

	rejected := errors.New("client went away")
	var calls atomic.Int32
	resp, err := agent.CreateResponse(context.Background(), WithInput("go"),
		WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
			if item.Type == ResponseItemTypeToolCallResult && item.ToolCallResult.ID == "toolu_fast" && calls.Add(1) == 1 {
				return rejected
			}
			return nil
		}))
	assertIncomplete(t, resp, err, TurnReasonCallbackError, TurnNextReconcile)
	assert.True(t, errors.Is(err, rejected))
	assert.Equal(t, resp.Turn.Outcome.ToolCalls, []ToolCallRecord{
		{ID: "toolu_fast", Name: "fast", State: ToolCallStateCompleted},
		{ID: "toolu_slow", Name: "slow", State: ToolCallStateUnknown},
	})
	assertEveryCallAnswered(t, resp.Turn.Messages)
	assert.Len(t, resp.BackgroundTasks, 1)
}
