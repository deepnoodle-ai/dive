package dive

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// scriptedLLM is a test double that returns pre-programmed responses in
// order. Each call to Generate consumes the next script entry.
type scriptedLLM struct {
	name     string
	script   []scriptedTurn
	callIdx  int
	received [][]*llm.Message
	mu       sync.Mutex
}

type scriptedTurn struct {
	text     string
	toolUses []*llm.ToolUseContent
	usage    llm.Usage
}

func (s *scriptedLLM) Name() string {
	if s.name == "" {
		return "scripted-llm"
	}
	return s.name
}

func (s *scriptedLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Record received messages for later assertions
	cfg := &llm.Config{}
	for _, opt := range opts {
		opt(cfg)
	}
	msgCopy := make([]*llm.Message, len(cfg.Messages))
	for i, msg := range cfg.Messages {
		msgCopy[i] = msg.Copy()
	}
	s.received = append(s.received, msgCopy)

	if s.callIdx >= len(s.script) {
		return nil, fmt.Errorf("scriptedLLM: unexpected call %d (only %d turns scripted)", s.callIdx+1, len(s.script))
	}
	turn := s.script[s.callIdx]
	s.callIdx++

	var content []llm.Content
	stopReason := "stop"
	if len(turn.toolUses) > 0 {
		for _, tu := range turn.toolUses {
			content = append(content, tu)
		}
		stopReason = "tool_use"
	} else {
		content = append(content, &llm.TextContent{Text: turn.text})
	}
	return &llm.Response{
		ID:         fmt.Sprintf("resp_%d", s.callIdx),
		Model:      s.Name(),
		Role:       llm.Assistant,
		Content:    content,
		Type:       "message",
		StopReason: stopReason,
		Usage:      turn.usage,
	}, nil
}

func (s *scriptedLLM) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callIdx
}

// scriptedTool returns pre-programmed outcomes in order. Each call consumes
// the next outcome; passing an empty outcomes slice causes the first call
// to error.
type scriptedTool struct {
	name     string
	outcomes []toolOutcome
	idx      int32
	calls    []string
	mu       sync.Mutex
}

type toolOutcome struct {
	result *ToolResult
	err    error
}

func (t *scriptedTool) Name() string                  { return t.name }
func (t *scriptedTool) Description() string           { return "scripted tool for testing" }
func (t *scriptedTool) Schema() *Schema               { return &Schema{Type: Object} }
func (t *scriptedTool) Annotations() *ToolAnnotations { return nil }

func (t *scriptedTool) Call(ctx context.Context, input any) (*ToolResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var inputStr string
	if b, ok := input.([]byte); ok {
		inputStr = string(b)
	}
	t.calls = append(t.calls, inputStr)
	i := int(atomic.AddInt32(&t.idx, 1)) - 1
	if i >= len(t.outcomes) {
		return nil, fmt.Errorf("scriptedTool %q: unexpected call %d", t.name, i+1)
	}
	o := t.outcomes[i]
	return o.result, o.err
}

func (t *scriptedTool) CallCount() int {
	return int(atomic.LoadInt32(&t.idx))
}

func toolUseAssistantTurn(uses ...*llm.ToolUseContent) scriptedTurn {
	return scriptedTurn{toolUses: uses, usage: llm.Usage{InputTokens: 1, OutputTokens: 1}}
}

func finalTextTurn(text string) scriptedTurn {
	return scriptedTurn{text: text, usage: llm.Usage{InputTokens: 1, OutputTokens: 1}}
}

// newScriptedToolUse is a small helper that builds a ToolUseContent with the
// given id, name, and json input string.
func newScriptedToolUse(id, name, input string) *llm.ToolUseContent {
	return &llm.ToolUseContent{ID: id, Name: name, Input: []byte(input)}
}

func TestSuspendSimple(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "approve", `{"req":"ok"}`)),
			// Should NOT be called — the session is suspended before a second LLM call.
		},
	}
	tool := &scriptedTool{
		name: "approve",
		outcomes: []toolOutcome{
			{result: NewSuspendResult("waiting on alice")},
		},
	}
	sess := session.New("s1")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{tool},
		Session: sess,
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Len(t, resp.PendingToolCalls, 1)
	assert.Equal(t, resp.PendingToolCalls[0].ID, "toolu_a")
	assert.Equal(t, resp.PendingToolCalls[0].Prompt, "waiting on alice")
	assert.Equal(t, mock.Calls(), 1, "LLM should only be called once before suspend")
	assert.True(t, sess.Suspended())
	assert.Equal(t, sess.PendingToolCallIDs(), []string{"toolu_a"})
}

func TestResumeSimple(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "approve", `{}`)),
			finalTextTurn("done"),
		},
	}
	tool := &scriptedTool{
		name: "approve",
		outcomes: []toolOutcome{
			{result: NewSuspendResult("waiting")},
		},
	}
	sess := session.New("s1")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.NoError(t, err)
	assert.True(t, sess.Suspended())

	// Resume with a real result for toolu_a
	resp, err := agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{
			"toolu_a": NewToolResultText("approved"),
		}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.OutputText(), "done")
	assert.False(t, sess.Suspended())
	assert.Equal(t, mock.Calls(), 2)
	// The second LLM call should see a complete tool_result message.
	lastCallMsgs := mock.received[1]
	// Expect: user, assistant(tool_use), tool_result(complete)
	assert.True(t, len(lastCallMsgs) >= 3, "expected at least 3 messages in resumed LLM call")
	lastMsg := lastCallMsgs[len(lastCallMsgs)-1]
	assert.Equal(t, lastMsg.Role, llm.User)
	foundResult := false
	for _, c := range lastMsg.Content {
		if trc, ok := c.(*llm.ToolResultContent); ok && trc.ToolUseID == "toolu_a" {
			foundResult = true
		}
	}
	assert.True(t, foundResult, "resumed tool_result should reference toolu_a")
}

func TestSuspendResumeSuspendAgain(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
			toolUseAssistantTurn(newScriptedToolUse("toolu_b", "tool_b", `{}`)),
			finalTextTurn("all done"),
		},
	}
	toolA := &scriptedTool{
		name:     "tool_a",
		outcomes: []toolOutcome{{result: NewSuspendResult("wait A")}},
	}
	toolB := &scriptedTool{
		name:     "tool_b",
		outcomes: []toolOutcome{{result: NewSuspendResult("wait B")}},
	}
	sess := session.New("multi")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{toolA, toolB}, Session: sess})
	assert.NoError(t, err)

	// Call 1: suspend on A
	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, resp.PendingToolCalls[0].ID, "toolu_a")

	// Call 2: resume with A's result; LLM emits tool B, which suspends
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A result")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, resp.PendingToolCalls[0].ID, "toolu_b")
	assert.True(t, sess.Suspended())
	assert.Equal(t, sess.PendingToolCallIDs(), []string{"toolu_b"})

	// Call 3: resume with B's result → final completion
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_b": NewToolResultText("B result")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.OutputText(), "all done")
	assert.Equal(t, mock.Calls(), 3)
	assert.False(t, sess.Suspended())
}

func TestParallelOneSuspends(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
			finalTextTurn("done"),
		},
	}
	toolA := &scriptedTool{
		name:     "tool_a",
		outcomes: []toolOutcome{{result: NewToolResultText("A done")}},
	}
	toolB := &scriptedTool{
		name:     "tool_b",
		outcomes: []toolOutcome{{result: NewSuspendResult("wait B")}},
	}
	sess := session.New("par1")
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
	assert.Len(t, resp.PendingToolCalls, 1)
	assert.Equal(t, resp.PendingToolCalls[0].ID, "toolu_b")
	assert.Len(t, resp.CompletedToolCalls, 1)
	assert.Equal(t, resp.CompletedToolCalls[0].ID, "toolu_a")
	assert.Equal(t, toolA.CallCount(), 1)
	assert.Equal(t, toolB.CallCount(), 1)

	// Resume
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_b": NewToolResultText("B done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.OutputText(), "done")
}

func TestParallelMultipleSuspend(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
				newScriptedToolUse("toolu_c", "tool_c", `{}`),
			),
			finalTextTurn("all done"),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewToolResultText("A")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait B")}}}
	toolC := &scriptedTool{name: "tool_c", outcomes: []toolOutcome{{result: NewSuspendResult("wait C")}}}
	sess := session.New("parmulti")
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{toolA, toolB, toolC},
		Session:               sess,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Len(t, resp.PendingToolCalls, 2)
	assert.Len(t, resp.CompletedToolCalls, 1)
	// Should have both B and C pending
	pendingIDs := map[string]bool{}
	for _, p := range resp.PendingToolCalls {
		pendingIDs[p.ID] = true
	}
	assert.True(t, pendingIDs["toolu_b"])
	assert.True(t, pendingIDs["toolu_c"])

	// Partial resume: only B
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_b": NewToolResultText("B done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Len(t, resp.PendingToolCalls, 1)
	assert.Equal(t, resp.PendingToolCalls[0].ID, "toolu_c")

	// Final resume: C
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_c": NewToolResultText("C done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
}

func TestSequentialSuspendSkipsTail(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
				newScriptedToolUse("toolu_c", "tool_c", `{}`),
			),
			finalTextTurn("all done"),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewToolResultText("A")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait B")}}}
	toolC := &scriptedTool{name: "tool_c", outcomes: []toolOutcome{{result: NewToolResultText("C")}}}
	sess := session.New("seqsus")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{toolA, toolB, toolC},
		Session: sess,
		// Sequential is the default
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, toolA.CallCount(), 1, "A should have run")
	assert.Equal(t, toolB.CallCount(), 1, "B should have run (and suspended)")
	assert.Equal(t, toolC.CallCount(), 0, "C should NOT have run (skipped)")
	// Snapshot shows 1 pending and 1 completed (not including not-started C)
	assert.Len(t, resp.PendingToolCalls, 1)
	assert.Equal(t, resp.PendingToolCalls[0].ID, "toolu_b")
	assert.Len(t, resp.CompletedToolCalls, 1)
	assert.Equal(t, resp.CompletedToolCalls[0].ID, "toolu_a")

	// Resume with B's result → C should now run
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_b": NewToolResultText("B done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, toolC.CallCount(), 1, "C should have run on resume")
}

func TestResumeUnknownID(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("unknown")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_bogus": NewToolResultText("nope")}),
	)
	assert.True(t, errors.Is(err, ErrUnknownPendingToolCall))
	// Session unchanged
	assert.True(t, sess.Suspended())
	assert.Equal(t, sess.PendingToolCallIDs(), []string{"toolu_a"})
}

func TestResumeNoSuspendedState(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			finalTextTurn("hello"),
		},
	}
	sess := session.New("fresh")
	agent, err := NewAgent(AgentOptions{Model: mock, Session: sess})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_x": NewToolResultText("x")}),
	)
	assert.True(t, errors.Is(err, ErrNoSuspendedState))
}

func TestResumeErrorResultCancelsTurn(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "approve", `{}`)),
			finalTextTurn("ok, stopping"),
		},
	}
	tool := &scriptedTool{
		name: "approve",
		outcomes: []toolOutcome{
			{result: NewSuspendResult("wait")},
		},
	}

	var postFailureCount int
	sess := session.New("canc")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{tool},
		Session: sess,
		Hooks: Hooks{
			PostToolUseFailure: []PostToolUseFailureHook{
				func(ctx context.Context, hctx *HookContext) error {
					postFailureCount++
					return nil
				},
			},
		},
	})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultError("denied")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, postFailureCount, 1, "PostToolUseFailure should fire for caller-supplied error result")
}

func TestOnSuspendHookOrder(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}

	var order []string
	sess := session.New("hook_order")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{tool},
		Session: sess,
		Hooks: Hooks{
			OnSuspend: []OnSuspendHook{
				func(ctx context.Context, hctx *HookContext) error {
					assert.Equal(t, hctx.Response.Status, ResponseStatusSuspended)
					order = append(order, "onSuspend")
					return nil
				},
			},
			PostGeneration: []PostGenerationHook{
				func(ctx context.Context, hctx *HookContext) error {
					assert.Equal(t, hctx.Response.Status, ResponseStatusSuspended)
					order = append(order, "postGen")
					return nil
				},
			},
		},
	})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, order, []string{"onSuspend", "postGen"})
}

func TestOnSuspendHookSeesPending(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait on alice")}}}

	var hookResponse *Response
	sess := session.New("hook_sees")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{tool},
		Session: sess,
		Hooks: Hooks{
			OnSuspend: []OnSuspendHook{
				func(ctx context.Context, hctx *HookContext) error {
					hookResponse = hctx.Response
					return nil
				},
			},
		},
	})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.NotNil(t, hookResponse)
	assert.Len(t, hookResponse.PendingToolCalls, 1)
	assert.Equal(t, hookResponse.PendingToolCalls[0].ID, "toolu_a")
	assert.Equal(t, hookResponse.PendingToolCalls[0].Prompt, "wait on alice")
}

func TestStreamingSuspendedItem(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("stream")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	var items []*ResponseItem
	_, err = agent.CreateResponse(context.Background(),
		WithInput("start"),
		WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
			items = append(items, item)
			return nil
		}),
	)
	assert.NoError(t, err)
	// Last item emitted should be the suspended terminal
	assert.True(t, len(items) > 0)
	last := items[len(items)-1]
	assert.Equal(t, last.Type, ResponseItemTypeSuspended)
	assert.NotNil(t, last.Suspended)
	assert.Len(t, last.Suspended.PendingToolCalls, 1)
}

func TestSuspendNoRegressionForNonSuspendingTools(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
			finalTextTurn("done"),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewToolResultText("A")}}}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	// Status is empty (treated as Completed) or explicitly set to Completed
	assert.True(t, resp.Status == "" || resp.Status == ResponseStatusCompleted)
	assert.Equal(t, resp.OutputText(), "done")
	assert.Nil(t, resp.PendingToolCalls)
}

func TestSetModelBetweenSuspendResume(t *testing.T) {
	modelA := &scriptedLLM{
		name: "model-A",
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	modelB := &scriptedLLM{
		name: "model-B",
		script: []scriptedTurn{
			finalTextTurn("via B"),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("setmodel")
	agent, err := NewAgent(AgentOptions{Model: modelA, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)

	// Swap the model
	agent.SetModel(modelB)

	resp, err := agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.OutputText(), "via B")
	assert.Equal(t, modelA.Calls(), 1)
	assert.Equal(t, modelB.Calls(), 1)
}
