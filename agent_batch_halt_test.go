package dive

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// batchLLM answers each Generate with the next batch of tool calls, then with
// "Done". It records the tools declared on each request.
type batchLLM struct {
	mu      sync.Mutex
	batches [][]*llm.ToolUseContent
	calls   int
	tools   [][]string
}

func (m *batchLLM) Name() string { return "batch-llm" }

func (m *batchLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var cfg llm.Config
	cfg.Apply(opts...)
	var names []string
	for _, t := range cfg.Tools {
		names = append(names, t.Name())
	}
	m.tools = append(m.tools, names)
	i := m.calls
	m.calls++
	if i < len(m.batches) {
		content := make([]llm.Content, len(m.batches[i]))
		for j, call := range m.batches[i] {
			content[j] = call
		}
		return &llm.Response{Role: llm.Assistant, Content: content, StopReason: "tool_use"}, nil
	}
	return &llm.Response{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "Done"}}, StopReason: "stop"}, nil
}

func call(id, name string) *llm.ToolUseContent {
	return &llm.ToolUseContent{ID: id, Name: name, Input: []byte(`{}`)}
}

// countingTool records how many times it ran and returns result, err.
type countingTool struct {
	name   string
	ann    *ToolAnnotations
	result *ToolResult
	err    error
	runs   atomic.Int32
}

func (t *countingTool) Name() string                  { return t.name }
func (t *countingTool) Description() string           { return t.name }
func (t *countingTool) Schema() *Schema               { return nil }
func (t *countingTool) Annotations() *ToolAnnotations { return t.ann }
func (t *countingTool) Call(ctx context.Context, input any) (*ToolResult, error) {
	t.runs.Add(1)
	if t.result == nil && t.err == nil {
		return NewToolResultText(t.name + " ok"), nil
	}
	return t.result, t.err
}

var halts = &ToolAnnotations{HaltsBatch: true}

// hookLog records which calls the tool hooks saw.
type hookLog struct {
	mu                     sync.Mutex
	pre, post, postFailure []string
}

func (h *hookLog) hooks(deny string) Hooks {
	record := func(list *[]string) func(context.Context, *HookContext) error {
		return func(_ context.Context, hctx *HookContext) error {
			h.mu.Lock()
			*list = append(*list, hctx.Call.ID)
			h.mu.Unlock()
			return nil
		}
	}
	return Hooks{
		PreToolUse: []PreToolUseHook{func(ctx context.Context, hctx *HookContext) error {
			_ = record(&h.pre)(ctx, hctx)
			if hctx.Call.Name == deny {
				return errors.New("denied by the person")
			}
			return nil
		}},
		PostToolUse:        []PostToolUseHook{record(&h.post)},
		PostToolUseFailure: []PostToolUseFailureHook{record(&h.postFailure)},
	}
}

func resultsByID(resp *Response) map[string]*ToolCallResult {
	out := map[string]*ToolCallResult{}
	for _, r := range resp.ToolCallResults() {
		out[r.ID] = r
	}
	return out
}

func resultText(r *ToolCallResult) string {
	if r == nil || r.Result == nil || len(r.Result.Content) == 0 {
		return ""
	}
	return r.Result.Content[0].Text
}

const plainHaltText = "Not executed: an earlier tool call in this response failed."

func TestBatchHalt(t *testing.T) {
	t.Run("a failed call halts the later halting calls", func(t *testing.T) {
		click := &countingTool{name: "click", ann: halts, result: NewToolResultError("no such window")}
		typ := &countingTool{name: "type", ann: halts}
		key := &countingTool{name: "key", ann: halts}
		model := &batchLLM{batches: [][]*llm.ToolUseContent{{
			call("c1", "click"), call("c2", "type"), call("c3", "key"),
		}}}
		var log hookLog
		var events []string
		agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{click, typ, key}, Hooks: log.hooks("")})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("go"),
			WithEventCallback(func(_ context.Context, item *ResponseItem) error {
				switch item.Type {
				case ResponseItemTypeToolCall:
					events = append(events, "call:"+item.ToolCall.ID)
				case ResponseItemTypeToolCallResult:
					events = append(events, "result:"+item.ToolCallResult.ID)
				}
				return nil
			}))
		assert.NoError(t, err)
		assert.Equal(t, resp.OutputText(), "Done")

		assert.Equal(t, click.runs.Load(), int32(1))
		assert.Equal(t, typ.runs.Load(), int32(0))
		assert.Equal(t, key.runs.Load(), int32(0))

		results := resultsByID(resp)
		assert.Equal(t, resultText(results["c1"]), "no such window")
		for _, id := range []string{"c2", "c3"} {
			assert.True(t, results[id].Result.IsError)
			assert.Equal(t, resultText(results[id]), plainHaltText)
			assert.True(t, errors.Is(results[id].Error, ErrBatchHalted))
		}

		// Halted calls run no hooks, but every call gets both events.
		assert.Equal(t, log.pre, []string{"c1"})
		assert.Equal(t, log.postFailure, []string{"c1"})
		assert.Len(t, log.post, 0)
		assert.Equal(t, events, []string{"call:c1", "result:c1", "call:c2", "result:c2", "call:c3", "result:c3"})

		// The model sees every call answered, as errors after the failure.
		toolResults := resp.OutputMessages[1].Content
		assert.Len(t, toolResults, 3)
		for _, c := range toolResults {
			assert.True(t, c.(*llm.ToolResultContent).IsError)
		}
	})

	t.Run("a denied call halts the batch without asking about the rest", func(t *testing.T) {
		click := &countingTool{name: "click", ann: halts}
		typ := &countingTool{name: "type", ann: halts}
		model := &batchLLM{batches: [][]*llm.ToolUseContent{{call("c1", "click"), call("c2", "type")}}}
		var log hookLog
		agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{click, typ}, Hooks: log.hooks("click")})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, click.runs.Load(), int32(0))
		assert.Equal(t, typ.runs.Load(), int32(0))
		assert.Equal(t, log.pre, []string{"c1"})
		results := resultsByID(resp)
		assert.Equal(t, resultText(results["c1"]), "denied by the person")
		assert.Equal(t, resultText(results["c2"]), plainHaltText)
	})

	t.Run("a Go error halts the batch", func(t *testing.T) {
		click := &countingTool{name: "click", ann: halts, err: errors.New("display asleep")}
		typ := &countingTool{name: "type", ann: halts}
		model := &batchLLM{batches: [][]*llm.ToolUseContent{{call("c1", "click"), call("c2", "type")}}}
		agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{click, typ}})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, typ.runs.Load(), int32(0))
		assert.Equal(t, resultText(resultsByID(resp)["c2"]), plainHaltText)
	})

	t.Run("calls without the annotation neither halt nor are halted", func(t *testing.T) {
		failingPlain := &countingTool{name: "lookup", result: NewToolResultError("not found")}
		click := &countingTool{name: "click", ann: halts, result: NewToolResultError("missed")}
		typ := &countingTool{name: "type", ann: halts}
		notes := &countingTool{name: "notes"}
		model := &batchLLM{batches: [][]*llm.ToolUseContent{{
			call("c1", "lookup"), // fails, but does not halt
			call("c2", "click"),  // runs, fails, halts
			call("c3", "notes"),  // not annotated: still runs
			call("c4", "type"),   // halted
		}}}
		agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{failingPlain, click, typ, notes}})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, click.runs.Load(), int32(1))
		assert.Equal(t, notes.runs.Load(), int32(1))
		assert.Equal(t, typ.runs.Load(), int32(0))
		results := resultsByID(resp)
		assert.Equal(t, resultText(results["c3"]), "notes ok")
		assert.Equal(t, resultText(results["c4"]), plainHaltText)
	})

	t.Run("an unknown plain tool does not halt the batch", func(t *testing.T) {
		typ := &countingTool{name: "type", ann: halts}
		model := &batchLLM{batches: [][]*llm.ToolUseContent{{call("c1", "no_such_tool"), call("c2", "type")}}}
		agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{typ}})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, typ.runs.Load(), int32(1))
	})

	t.Run("toolset member calls halt without the annotation", func(t *testing.T) {
		click := &countingTool{name: "left_click", result: NewToolResultError("missed")}
		typ := &countingTool{name: "type"}
		clickCall := call("c1", "left_click")
		clickCall.ToolsetName = "computer"
		typeCall := call("c2", "type")
		typeCall.ToolsetName = "computer"
		model := &batchLLM{batches: [][]*llm.ToolUseContent{{clickCall, typeCall}}}
		agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{click, typ}})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, typ.runs.Load(), int32(0))
		assert.Equal(t, resultText(resultsByID(resp)["c2"]),
			"Not executed: an earlier computer action in this turn failed.")
	})

	t.Run("an unknown toolset member halts the batch", func(t *testing.T) {
		typ := &countingTool{name: "type"}
		zoom := call("c1", "zoom")
		zoom.ToolsetName = "computer"
		typeCall := call("c2", "type")
		typeCall.ToolsetName = "computer"
		model := &batchLLM{batches: [][]*llm.ToolUseContent{{zoom, typeCall}}}
		agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{typ}})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, typ.runs.Load(), int32(0))
	})

	t.Run("halting does not carry into the next response's batch", func(t *testing.T) {
		click := &countingTool{name: "click", ann: halts, result: NewToolResultError("missed")}
		typ := &countingTool{name: "type", ann: halts}
		model := &batchLLM{batches: [][]*llm.ToolUseContent{
			{call("c1", "click"), call("c2", "type")},
			{call("c3", "type")},
		}}
		agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{click, typ}})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, typ.runs.Load(), int32(1))
		assert.Equal(t, resultText(resultsByID(resp)["c3"]), "type ok")
	})

	t.Run("parallel execution falls back to sequential", func(t *testing.T) {
		var running, maxRunning atomic.Int32
		track := func(ctx context.Context, input any) (*ToolResult, error) {
			n := running.Add(1)
			defer running.Add(-1)
			for {
				old := maxRunning.Load()
				if n <= old || maxRunning.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			return NewToolResultText("ok"), nil
		}
		plain := &mockTool{name: "plain", callFunc: track}
		click := &mockTool{name: "click", callFunc: track, annotations: halts}
		failing := &countingTool{name: "fail", ann: halts, result: NewToolResultError("missed")}
		typ := &countingTool{name: "type", ann: halts}
		model := &batchLLM{batches: [][]*llm.ToolUseContent{{
			call("c1", "plain"), call("c2", "click"), call("c3", "plain"), call("c4", "fail"), call("c5", "type"),
		}}}
		agent, err := NewAgent(AgentOptions{
			Model:                 model,
			Tools:                 []Tool{plain, click, failing, typ},
			ParallelToolExecution: true,
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, maxRunning.Load(), int32(1))
		assert.Equal(t, typ.runs.Load(), int32(0))
		assert.Equal(t, resultText(resultsByID(resp)["c5"]), plainHaltText)
	})
}

func TestHaltsBatchAnnotationJSON(t *testing.T) {
	data, err := (&ToolAnnotations{HaltsBatch: true}).MarshalJSON()
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"haltsBatch":true`)

	var ann ToolAnnotations
	assert.NoError(t, ann.UnmarshalJSON(data))
	assert.True(t, ann.HaltsBatch)
	assert.Len(t, ann.Extra, 0)
}
