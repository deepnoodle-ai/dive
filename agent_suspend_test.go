package dive

import (
	"context"
	"encoding/json"
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

// pendingIDs extracts the ID field from a session's pending calls for easy
// equality assertions.
func pendingIDs(sess *session.Session) []string {
	calls := sess.PendingCalls()
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.ID
	}
	return out
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
	assert.Equal(t, pendingIDs(sess), []string{"toolu_a"})
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
	assert.Equal(t, pendingIDs(sess), []string{"toolu_b"})

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
	assert.Equal(t, pendingIDs(sess), []string{"toolu_a"})
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

func TestResumePostHooksFireInToolUseOrder(t *testing.T) {
	// Two tools suspend in parallel; resume supplies results for both.
	// PostToolUse hooks must fire in the order the tool_use blocks appear
	// in the assistant message, not in random map-iteration order.
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
				newScriptedToolUse("toolu_c", "tool_c", `{}`),
			),
			finalTextTurn("done"),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait b")}}}
	toolC := &scriptedTool{name: "tool_c", outcomes: []toolOutcome{{result: NewSuspendResult("wait c")}}}

	// Assert that we get the same ordering on repeated runs across different
	// map seeds. We do this by running the resume many times and verifying
	// the order stays stable.
	for iter := 0; iter < 20; iter++ {
		var order []string
		sess := session.New(fmt.Sprintf("order_%d", iter))
		agent, err := NewAgent(AgentOptions{
			Model:                 mock,
			Tools:                 []Tool{toolA, toolB, toolC},
			Session:               sess,
			ParallelToolExecution: true,
			Hooks: Hooks{
				PostToolUse: []PostToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						order = append(order, hctx.Call.ID)
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		// Fresh script per iteration
		mock.callIdx = 0
		mock.received = nil
		toolA.idx = 0
		toolB.idx = 0
		toolC.idx = 0

		_, err = agent.CreateResponse(context.Background(), WithInput("start"))
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(),
			WithToolResults(map[string]*ToolResult{
				"toolu_c": NewToolResultText("C"),
				"toolu_a": NewToolResultText("A"),
				"toolu_b": NewToolResultText("B"),
			}),
		)
		assert.NoError(t, err)
		assert.Equal(t, order, []string{"toolu_a", "toolu_b", "toolu_c"})
	}
}

func TestResumeNotStartedRunsInParallel(t *testing.T) {
	// A sequential agent lets tool A suspend and leaves B, C not-started.
	// On resume with a parallel-enabled agent, the not-started tools should
	// run through the parallel execution path.
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
				newScriptedToolUse("toolu_c", "tool_c", `{}`),
			),
			finalTextTurn("done"),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewToolResultText("B done")}}}
	toolC := &scriptedTool{name: "tool_c", outcomes: []toolOutcome{{result: NewToolResultText("C done")}}}

	sess := session.New("par_not_started")

	// Sequential agent suspends on A.
	seqAgent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{toolA, toolB, toolC},
		Session: sess,
	})
	assert.NoError(t, err)
	_, err = seqAgent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, toolB.CallCount(), 0, "B should not have run (sequential, A suspended)")
	assert.Equal(t, toolC.CallCount(), 0, "C should not have run (sequential, A suspended)")

	// Parallel agent resumes the same session. The not-started tails must
	// still execute via the parallel path (regression test for fix that
	// dispatches via executeToolCalls instead of hard-coding sequential).
	parAgent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{toolA, toolB, toolC},
		Session:               sess,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)
	resp, err := parAgent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, toolB.CallCount(), 1, "B should have run during resume")
	assert.Equal(t, toolC.CallCount(), 1, "C should have run during resume")
}

func TestResumeNotStartedReSuspends(t *testing.T) {
	// Sequential: A suspends, B and C are not-started. Resume with A's
	// result causes B to run and suspend (re-suspend). C is not-started
	// a second time and must run on the second resume.
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
				newScriptedToolUse("toolu_c", "tool_c", `{}`),
			),
			finalTextTurn("done"),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait b")}}}
	toolC := &scriptedTool{name: "tool_c", outcomes: []toolOutcome{{result: NewToolResultText("C done")}}}

	sess := session.New("notstart_resus")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{toolA, toolB, toolC}, Session: sess})
	assert.NoError(t, err)

	// Call 1: A suspends; B, C not-started.
	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, pendingIDs(sess), []string{"toolu_a"})

	// Call 2: resume with A. B runs, suspends. C is still not-started.
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, pendingIDs(sess), []string{"toolu_b"})
	assert.Equal(t, toolB.CallCount(), 1)
	assert.Equal(t, toolC.CallCount(), 0, "C should not have run yet")

	// Call 3: resume with B. C runs, final turn generates "done".
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_b": NewToolResultText("B done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, toolC.CallCount(), 1)
	assert.Equal(t, resp.OutputText(), "done")
	assert.False(t, sess.Suspended())
}

func TestSuspendedSessionInputReturnsError(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("inp")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)

	// Supplying new input on a suspended session must fail.
	_, err = agent.CreateResponse(context.Background(), WithInput("more"))
	assert.True(t, errors.Is(err, ErrSuspendedSessionInput))
	assert.True(t, sess.Suspended())
}

// nonSuspendableSession is a minimal Session that doesn't implement
// SuspendableSession, used to verify ErrSessionNotSuspendable.
type nonSuspendableSession struct {
	id   string
	msgs []*llm.Message
}

func (s *nonSuspendableSession) ID() string { return s.id }
func (s *nonSuspendableSession) Messages(ctx context.Context) ([]*llm.Message, error) {
	out := make([]*llm.Message, len(s.msgs))
	for i, m := range s.msgs {
		out[i] = m.Copy()
	}
	return out, nil
}
func (s *nonSuspendableSession) SaveTurn(ctx context.Context, msgs []*llm.Message, _ *llm.Usage) error {
	s.msgs = append(s.msgs, msgs...)
	return nil
}

func TestSuspendOnNonSuspendableSessionReturnsError(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := &nonSuspendableSession{id: "nonsus"}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.True(t, errors.Is(err, ErrSessionNotSuspendable))
}

func TestSuspendWithoutSessionReturnsError(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}})
	assert.NoError(t, err)

	// No session at all — suspension should fail with ErrSessionNotSuspendable.
	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.True(t, errors.Is(err, ErrSessionNotSuspendable))
}

func TestPartialResumeTwice(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
				newScriptedToolUse("toolu_c", "tool_c", `{}`),
			),
			finalTextTurn("done"),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait b")}}}
	toolC := &scriptedTool{name: "tool_c", outcomes: []toolOutcome{{result: NewSuspendResult("wait c")}}}
	sess := session.New("partial2")
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{toolA, toolB, toolC},
		Session:               sess,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)

	// First call: all three suspend.
	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Len(t, resp.PendingToolCalls, 3)

	// Partial resume: supply only A.
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Len(t, resp.PendingToolCalls, 2)
	assert.True(t, sess.Suspended())
	remaining := map[string]bool{}
	for _, p := range resp.PendingToolCalls {
		remaining[p.ID] = true
	}
	assert.True(t, remaining["toolu_b"])
	assert.True(t, remaining["toolu_c"])

	// Partial resume again: supply only B.
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_b": NewToolResultText("B done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Len(t, resp.PendingToolCalls, 1)
	assert.Equal(t, resp.PendingToolCalls[0].ID, "toolu_c")

	// Final resume: supply C → completion.
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_c": NewToolResultText("C done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, mock.Calls(), 2, "second LLM call runs after full resume")
	assert.False(t, sess.Suspended())

	// Session should have exactly one turn after resolution (replace-last
	// behavior means repeated suspended saves do not grow the event log).
	assert.Equal(t, sess.EventCount(), 1)
}

func TestResumePostHookAbortPropagates(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("abort")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{tool},
		Session: sess,
		Hooks: Hooks{
			PostToolUse: []PostToolUseHook{
				func(ctx context.Context, hctx *HookContext) error {
					return AbortGeneration("policy violation")
				},
			},
		},
	})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A")}),
	)
	var abortErr *HookAbortError
	assert.True(t, errors.As(err, &abortErr))
	assert.Equal(t, abortErr.HookType, "PostToolUse")
}

func TestSuspendAccumulatesUsageAcrossResume(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			{
				toolUses: []*llm.ToolUseContent{newScriptedToolUse("toolu_a", "tool_a", `{}`)},
				usage:    llm.Usage{InputTokens: 10, OutputTokens: 5},
			},
			{
				text:  "done",
				usage: llm.Usage{InputTokens: 20, OutputTokens: 7},
			},
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("usage")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	// The resume response's usage reflects only the second generation (20/7),
	// not the accumulated suspended-turn usage.
	assert.Equal(t, resp.Usage.InputTokens, 20)
	assert.Equal(t, resp.Usage.OutputTokens, 7)
}

func TestSuspendedResponseMessagesIncludeAssistant(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("outmsgs")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	// The suspended response's OutputMessages should contain at least the
	// assistant tool_use message so callers can render the turn.
	foundAssistant := false
	for _, msg := range resp.OutputMessages {
		if msg.Role == llm.Assistant {
			for _, c := range msg.Content {
				if _, ok := c.(*llm.ToolUseContent); ok {
					foundAssistant = true
				}
			}
		}
	}
	assert.True(t, foundAssistant, "suspended response should include assistant tool_use message")
}

func TestSuspendedResponseItemsOrderParallel(t *testing.T) {
	// Verify that items in the suspended response include the tool_call and
	// tool_call_result events for both completed and suspended siblings.
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewToolResultText("A")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait b")}}}
	sess := session.New("items")
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

	// Verify tool_call_result events exist for both tools (one completed,
	// one suspended).
	ids := map[string]bool{}
	for _, item := range resp.Items {
		if item.Type == ResponseItemTypeToolCallResult {
			ids[item.ToolCallResult.ID] = true
		}
	}
	assert.True(t, ids["toolu_a"])
	assert.True(t, ids["toolu_b"])
}

func TestResumeWithFileStoreCrossProcess(t *testing.T) {
	// End-to-end: a file-backed session is suspended in one "process" and
	// resumed in another. Verifies full integration through the agent.
	dir := t.TempDir()

	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
			finalTextTurn("done"),
		},
	}
	suspendTool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}

	// Process A
	{
		store, err := session.NewFileStore(dir)
		assert.NoError(t, err)
		sess, err := store.Open(context.Background(), "xproc")
		assert.NoError(t, err)
		agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{suspendTool}, Session: sess})
		assert.NoError(t, err)
		resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
		assert.NoError(t, err)
		assert.Equal(t, resp.Status, ResponseStatusSuspended)
	}

	// Process B: fresh store + fresh session instance, same agent factory.
	{
		store, err := session.NewFileStore(dir)
		assert.NoError(t, err)
		sess, err := store.Open(context.Background(), "xproc")
		assert.NoError(t, err)
		assert.True(t, sess.Suspended())
		assert.Equal(t, pendingIDs(sess), []string{"toolu_a"})

		// No tool needs to run on resume (A was suspended); supply its result.
		agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{suspendTool}, Session: sess})
		assert.NoError(t, err)
		resp, err := agent.CreateResponse(context.Background(),
			WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
		)
		assert.NoError(t, err)
		assert.Equal(t, resp.Status, ResponseStatusCompleted)
		assert.Equal(t, resp.OutputText(), "done")
		assert.False(t, sess.Suspended())
	}
}

func TestSuspendMetadataPreserved(t *testing.T) {
	tool := &scriptedTool{
		name: "tool_a",
		outcomes: []toolOutcome{
			{result: NewSuspendResultWithMetadata("wait", map[string]any{"owner": "alice", "urgency": "high"})},
		},
	}
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	sess := session.New("meta")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Len(t, resp.PendingToolCalls, 1)
	assert.Equal(t, resp.PendingToolCalls[0].Prompt, "wait")
	assert.NotNil(t, resp.PendingToolCalls[0].Metadata)
	assert.Equal(t, resp.PendingToolCalls[0].Metadata["owner"], "alice")
	assert.Equal(t, resp.PendingToolCalls[0].Metadata["urgency"], "high")
}

func TestResumeRestoresSessionHistoryForSecondCall(t *testing.T) {
	// After a suspend/resume round trip, a subsequent CreateResponse call
	// sees the full conversation history (user input + assistant response)
	// from the resumed turn.
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
			finalTextTurn("first done"),
			finalTextTurn("second done"),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("history")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("first"))
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
	)
	assert.NoError(t, err)

	// Subsequent non-resume call with new input.
	resp, err := agent.CreateResponse(context.Background(), WithInput("second"))
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "second done")

	// The third LLM call should have seen the full history: first user
	// message, assistant tool_use, tool_result, assistant "first done",
	// second user message.
	thirdCallMsgs := mock.received[2]
	assert.True(t, len(thirdCallMsgs) >= 5)
	// First should be the original "first" user message
	assert.Equal(t, thirdCallMsgs[0].Role, llm.User)
	// Last should be the "second" user message
	last := thirdCallMsgs[len(thirdCallMsgs)-1]
	assert.Equal(t, last.Role, llm.User)
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

// ---------------------------------------------------------------------------
// Adversarial invariant tests (hardening pass)
// ---------------------------------------------------------------------------

// Invariant 1: after N rounds of partial resume, EventCount stays at 1.
func TestPartialResumeEventCountStable(t *testing.T) {
	ids := []string{"toolu_a", "toolu_b", "toolu_c", "toolu_d", "toolu_e"}
	var uses []*llm.ToolUseContent
	tools := make([]Tool, 0, len(ids))
	for _, id := range ids {
		name := "tool_" + id[len(id)-1:]
		uses = append(uses, newScriptedToolUse(id, name, `{}`))
		tools = append(tools, &scriptedTool{
			name:     name,
			outcomes: []toolOutcome{{result: NewSuspendResult("wait " + id)}},
		})
	}
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(uses...),
			finalTextTurn("all done"),
		},
	}
	sess := session.New("evtcount")
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 tools,
		Session:               sess,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, sess.EventCount(), 1)

	// Resolve one at a time: 4 partial-resume rounds, then a final one.
	for i, id := range ids {
		_, err = agent.CreateResponse(context.Background(),
			WithToolResults(map[string]*ToolResult{id: NewToolResultText("done " + id)}),
		)
		assert.NoError(t, err)
		assert.Equal(t, sess.EventCount(), 1,
			fmt.Sprintf("round %d: event count must remain 1 across partial resumes", i))
	}
	assert.False(t, sess.Suspended())
}

// Invariant 2: cross-process — suspended Prompt + Metadata survive a
// FileStore round trip, AND survive a partial resume round trip so that
// the re-suspended response still carries the original payload.
func TestCrossProcessSuspendMetadata(t *testing.T) {
	dir := t.TempDir()
	toolA := &scriptedTool{
		name: "tool_a",
		outcomes: []toolOutcome{{
			result: NewSuspendResultWithMetadata("wait on alice", map[string]any{"owner": "alice", "severity": "high"}),
		}},
	}
	toolB := &scriptedTool{
		name: "tool_b",
		outcomes: []toolOutcome{{
			result: NewSuspendResultWithMetadata("wait on bob", map[string]any{"owner": "bob", "severity": "low"}),
		}},
	}
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
			finalTextTurn("done"),
		},
	}

	// Process A: create and suspend on both tools in parallel.
	{
		store, err := session.NewFileStore(dir)
		assert.NoError(t, err)
		sess, err := store.Open(context.Background(), "meta")
		assert.NoError(t, err)
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
		assert.Len(t, resp.PendingToolCalls, 2)
	}

	// Process B: fresh store → fresh session. Metadata must be readable.
	var freshAgent *Agent
	var freshSess *session.Session
	{
		store, err := session.NewFileStore(dir)
		assert.NoError(t, err)
		sess, err := store.Open(context.Background(), "meta")
		assert.NoError(t, err)
		freshSess = sess
		assert.True(t, sess.Suspended())

		pending := sess.PendingCalls()
		assert.Equal(t, len(pending), 2)
		byID := map[string]session.PendingCall{}
		for _, p := range pending {
			byID[p.ID] = p
		}
		assert.Equal(t, byID["toolu_a"].Prompt, "wait on alice")
		assert.Equal(t, byID["toolu_a"].Metadata["owner"], "alice")
		assert.Equal(t, byID["toolu_a"].Metadata["severity"], "high")
		assert.Equal(t, byID["toolu_b"].Prompt, "wait on bob")
		assert.Equal(t, byID["toolu_b"].Metadata["owner"], "bob")

		agent, err := NewAgent(AgentOptions{
			Model:                 mock,
			Tools:                 []Tool{toolA, toolB},
			Session:               sess,
			ParallelToolExecution: true,
		})
		assert.NoError(t, err)
		freshAgent = agent
	}

	// Process B: partial resume — only supply A's result. The remaining
	// suspended response must still carry B's original Prompt and Metadata,
	// reconstructed from the persisted session rather than from a live tool
	// call (tool_b is never called again on this path).
	resp, err := freshAgent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Len(t, resp.PendingToolCalls, 1)
	assert.Equal(t, resp.PendingToolCalls[0].ID, "toolu_b")
	assert.Equal(t, resp.PendingToolCalls[0].Prompt, "wait on bob")
	assert.NotNil(t, resp.PendingToolCalls[0].Metadata)
	assert.Equal(t, resp.PendingToolCalls[0].Metadata["owner"], "bob")
	assert.Equal(t, resp.PendingToolCalls[0].Metadata["severity"], "low")
	assert.True(t, freshSess.Suspended())
}

// Invariant 3: sequential ↔ parallel agents can be mixed on the same session
// across a suspend/resume cycle.
func TestMixSequentialParallelAgentsOnSameSession(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
			finalTextTurn("done"),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewToolResultText("B ok")}}}

	sess := session.New("mixed")

	// Sequential agent suspends on A; B never runs.
	seqAgent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{toolA, toolB}, Session: sess})
	assert.NoError(t, err)
	resp, err := seqAgent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, toolB.CallCount(), 0, "sequential: B should not have run")

	// Parallel agent resumes, executes B via the parallel path, completes.
	parAgent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{toolA, toolB},
		Session:               sess,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)
	resp, err = parAgent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, toolB.CallCount(), 1)
	assert.False(t, sess.Suspended())
}

// panickyTool panics on its first call. Used to verify the resume not-started
// path survives a tool panic (it should recover into an error result).
type panickyTool struct {
	name  string
	fired int32
}

func (p *panickyTool) Name() string                  { return p.name }
func (p *panickyTool) Description() string           { return "panics on first call" }
func (p *panickyTool) Schema() *Schema               { return &Schema{Type: Object} }
func (p *panickyTool) Annotations() *ToolAnnotations { return nil }
func (p *panickyTool) Call(ctx context.Context, input any) (*ToolResult, error) {
	if atomic.AddInt32(&p.fired, 1) == 1 {
		panic("boom")
	}
	return NewToolResultText("ok"), nil
}

// Invariant 4: tool panic during not-started resume execution is recovered
// into an error result; session ends in a coherent state (either resumed
// normally or still suspended on other pending work).
func TestResumeNotStartedToolPanic(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
			finalTextTurn("recovered"),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a")}}}
	toolB := &panickyTool{name: "tool_b"}

	sess := session.New("panic_resume")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{toolA, toolB},
		Session: sess,
	})
	assert.NoError(t, err)

	// First turn: A suspends; sequential skips B.
	_, err = agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.True(t, sess.Suspended())

	// Resume: supplies A; B is not-started and panics when executed. The
	// panic must be recovered into an error result, and generation must
	// continue to final completion.
	resp, err := agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.OutputText(), "recovered")
	assert.False(t, sess.Suspended())
}

// Invariant 5: suspend during the last tool-producing iteration is persisted
// correctly and can be resumed to completion.
func TestSuspendOnLastIterationBoundary(t *testing.T) {
	// ToolIterationLimit=2 → generationLimit=3. i=0 runs tools normally;
	// i=1 runs tools and is still allowed to do so (lastIteration flag is
	// set at the END of i=1, for iteration i=2 which will have tool_choice
	// set to none). So iteration 1 is the last tool-producing iteration.
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
			toolUseAssistantTurn(newScriptedToolUse("toolu_b", "tool_b", `{}`)),
			finalTextTurn("finished"),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewToolResultText("A")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait at boundary")}}}

	sess := session.New("boundary")
	agent, err := NewAgent(AgentOptions{
		Model:              mock,
		Tools:              []Tool{toolA, toolB},
		Session:            sess,
		ToolIterationLimit: 2,
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.True(t, sess.Suspended())
	assert.Equal(t, sess.EventCount(), 1)

	// Resume: the final LLM call has tool_choice=none (lastIteration path).
	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_b": NewToolResultText("B done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.OutputText(), "finished")
	assert.False(t, sess.Suspended())
}

// blockingTool blocks until ctx is cancelled, then returns a ctx error. Used
// to verify resume-time context cancellation leaves the session untouched.
type blockingTool struct {
	name    string
	started chan struct{}
}

func (b *blockingTool) Name() string                  { return b.name }
func (b *blockingTool) Description() string           { return "blocks until ctx cancel" }
func (b *blockingTool) Schema() *Schema               { return &Schema{Type: Object} }
func (b *blockingTool) Annotations() *ToolAnnotations { return nil }
func (b *blockingTool) Call(ctx context.Context, input any) (*ToolResult, error) {
	close(b.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

// Invariant 6: resume with a context that cancels mid-execution — the
// session must remain in its pre-resume state (fully unchanged). We verify
// by snapshotting Messages() as JSON before and after.
//
// We drive the cancel through the parallel-exec path so that a tool worker
// returning ctx.Err propagates as a Go error out of executeToolCalls, causing
// CreateResponse to return without saving.
func TestResumeContextCancelMidExecution(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
				newScriptedToolUse("toolu_c", "tool_c", `{}`),
			),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a")}}}
	toolB := &blockingTool{name: "tool_b", started: make(chan struct{})}
	toolC := &blockingTool{name: "tool_c", started: make(chan struct{})}

	sess := session.New("ctxcancel")

	// First call: sequential agent → A suspends, B and C not-started.
	seqAgent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{toolA, toolB, toolC},
		Session: sess,
	})
	assert.NoError(t, err)
	_, err = seqAgent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)
	assert.True(t, sess.Suspended())

	// Snapshot pre-resume state.
	preMsgs, _ := sess.Messages(context.Background())
	preJSON, _ := json.Marshal(preMsgs)
	prePending := pendingIDs(sess)

	// Resume with a parallel agent so not-started B, C run concurrently
	// and ctx cancel propagates as a Go error from executeToolCallsParallel.
	parAgent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{toolA, toolB, toolC},
		Session:               sess,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, resumeErr := parAgent.CreateResponse(ctx,
			WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
		)
		done <- resumeErr
	}()

	// Wait for both blocking tools to start, then cancel.
	<-toolB.started
	<-toolC.started
	cancel()

	err = <-done
	assert.True(t, err != nil, "resume must return an error when ctx cancels mid-execution")

	// Session state must be unchanged.
	postMsgs, _ := sess.Messages(context.Background())
	postJSON, _ := json.Marshal(postMsgs)
	assert.Equal(t, string(preJSON), string(postJSON))
	assert.True(t, sess.Suspended())
	assert.Equal(t, pendingIDs(sess), prePending)
}

// Invariant 7: supplying a result for an ID that exists in the assistant
// tool_use message but is not in the pending set (e.g. already completed)
// returns ErrUnknownPendingToolCall without mutating the session.
func TestResumeCompletedIDReturnsError(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewToolResultText("A done")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait b")}}}
	sess := session.New("completed_id")
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{toolA, toolB},
		Session:               sess,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.True(t, sess.Suspended())
	assert.Equal(t, pendingIDs(sess), []string{"toolu_b"})

	preMsgs, _ := sess.Messages(context.Background())
	preJSON, _ := json.Marshal(preMsgs)

	// Caller supplies a result for toolu_a — which was already completed
	// in the suspended turn and is NOT in the pending set.
	_, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("double?")}),
	)
	assert.True(t, errors.Is(err, ErrUnknownPendingToolCall))

	// Session untouched.
	postMsgs, _ := sess.Messages(context.Background())
	postJSON, _ := json.Marshal(postMsgs)
	assert.Equal(t, string(preJSON), string(postJSON))
	assert.True(t, sess.Suspended())
	assert.Equal(t, pendingIDs(sess), []string{"toolu_b"})
}

// Invariant 8: forking a suspended session produces a non-suspended fork.
func TestForkSuspendedSessionClearsSuspension(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("fork_src")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.True(t, sess.Suspended())

	forked := sess.Fork("fork_dst")
	assert.False(t, forked.Suspended(), "forked session must not be suspended")
	assert.Equal(t, len(forked.PendingCalls()), 0)
	// Original unchanged.
	assert.True(t, sess.Suspended())
}

// Invariant 9: Compact refuses to run on a suspended session.
func TestCompactSuspendedSessionRefuses(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("cmp")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.True(t, sess.Suspended())

	err = sess.Compact(context.Background(), func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
		return []*llm.Message{llm.NewAssistantTextMessage("summary")}, nil
	})
	assert.True(t, errors.Is(err, session.ErrSuspendedSession))

	// Session still suspended, unchanged event count.
	assert.True(t, sess.Suspended())
	assert.Equal(t, sess.EventCount(), 1)
}

// Invariant 10: SaveResumedTurn called when the session is NOT suspended
// returns ErrNotSuspended and does not overwrite the last event.
func TestSaveResumedTurnNotSuspendedGuard(t *testing.T) {
	ctx := context.Background()
	sess := session.New("guard")
	err := sess.SaveTurn(ctx, []*llm.Message{
		llm.NewUserTextMessage("hi"),
		llm.NewAssistantTextMessage("hello"),
	}, nil)
	assert.NoError(t, err)
	assert.Equal(t, sess.EventCount(), 1)

	err = sess.SaveResumedTurn(ctx, []*llm.Message{
		llm.NewUserTextMessage("wrong"),
	}, nil)
	assert.True(t, errors.Is(err, session.ErrNotSuspended))
	assert.Equal(t, sess.EventCount(), 1)

	// Last event must still contain the original messages unchanged.
	msgs, _ := sess.Messages(ctx)
	assert.Equal(t, len(msgs), 2)
	assert.Equal(t, msgs[0].Text(), "hi")
	assert.Equal(t, msgs[1].Text(), "hello")
}

// failingSuspendableSession wraps a real session and fails SaveSuspendedTurn.
// Used to verify the agent propagates persistence errors from suspend.
type failingSuspendableSession struct {
	inner   *session.Session
	saveErr error
}

func (f *failingSuspendableSession) ID() string { return f.inner.ID() }
func (f *failingSuspendableSession) Messages(ctx context.Context) ([]*llm.Message, error) {
	return f.inner.Messages(ctx)
}
func (f *failingSuspendableSession) SaveTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error {
	return f.inner.SaveTurn(ctx, messages, usage)
}
func (f *failingSuspendableSession) Suspended() bool                 { return f.inner.Suspended() }
func (f *failingSuspendableSession) PendingCalls() []session.PendingCall {
	return f.inner.PendingCalls()
}
func (f *failingSuspendableSession) LastEventMessageCount() int { return f.inner.LastEventMessageCount() }
func (f *failingSuspendableSession) SaveSuspendedTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage, pending []session.PendingCall) error {
	return f.saveErr
}
func (f *failingSuspendableSession) SaveResumedTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error {
	return f.inner.SaveResumedTurn(ctx, messages, usage)
}
func (f *failingSuspendableSession) AbandonSuspension(ctx context.Context) error {
	return f.inner.AbandonSuspension(ctx)
}

// Invariant 11: SaveSuspendedTurn failure is propagated from CreateResponse,
// not swallowed. The caller must see the failure instead of a stale pending
// session state.
func TestSaveSuspendedTurnErrorPropagates(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	failing := &failingSuspendableSession{
		inner:   session.New("prop"),
		saveErr: errors.New("disk full"),
	}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: failing})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.True(t, err != nil, "CreateResponse must return the save error")
	assert.True(t, errors.Is(err, failing.saveErr))
	// Inner session never transitioned to suspended (the save was a no-op
	// that failed before any state change).
	assert.False(t, failing.inner.Suspended())
}

// Invariant 12: an OnSuspend hook that aborts with HookAbortError leaves the
// session not suspended — the agent compensates by calling AbandonSuspension.
func TestOnSuspendAbortRollsBack(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("rollback")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{tool},
		Session: sess,
		Hooks: Hooks{
			OnSuspend: []OnSuspendHook{
				func(ctx context.Context, hctx *HookContext) error {
					return AbortGeneration("policy: no external waits")
				},
			},
		},
	})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	var abortErr *HookAbortError
	assert.True(t, errors.As(err, &abortErr))
	assert.Equal(t, abortErr.HookType, "OnSuspend")

	// Rollback: session must no longer report suspended.
	assert.False(t, sess.Suspended())
	assert.Equal(t, len(sess.PendingCalls()), 0)
}

// Invariant 13: turn messages from a session snapshot are not mutated by
// resume-time writes that touch the merged tool_result message. We take a
// JSON-snapshot of the loaded messages before resuming and re-check the
// same snapshot object after resume to ensure the session's view is stable.
func TestResumeTurnMessagesNoAliasing(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
			finalTextTurn("done"),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewToolResultText("B")}}}
	sess := session.New("nonalias")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Tools:   []Tool{toolA, toolB},
		Session: sess,
	})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)

	// Grab the session's messages — these are what Messages() returns to
	// arbitrary readers. Their content should not mutate even while a
	// concurrent/overlapping resume runs.
	preSnap, _ := sess.Messages(context.Background())
	preJSON, _ := json.Marshal(preSnap)

	// Resume — this runs B as not-started and appends its result to the
	// tool_result message within the agent's copy.
	resp, err := agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)

	// The original snapshot object must not have been mutated through a
	// shared pointer. Re-serialize and compare.
	postJSON, _ := json.Marshal(preSnap)
	assert.Equal(t, string(preJSON), string(postJSON),
		"the pre-resume message snapshot must remain byte-identical after resume")
}

// Invariant 14: generate-driven suspends carry a non-empty OutputMessages
// (so stream consumers can render the turn), while partial-resume-only
// suspends (no generate call) carry an empty OutputMessages. Callers that
// want the full turn should read from the session.
func TestSuspendedOutputMessagesInvariant(t *testing.T) {
	// Part A: generate-driven suspend → OutputMessages non-empty.
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "tool_a", `{}`)),
		},
	}
	tool := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait")}}}
	sess := session.New("outmsgs_a")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)
	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.True(t, len(resp.OutputMessages) > 0, "generate-driven suspend must include OutputMessages")

	// Part B: partial-resume-only suspend → OutputMessages is empty.
	mock2 := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a")}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait b")}}}
	sess2 := session.New("outmsgs_b")
	agent2, err := NewAgent(AgentOptions{
		Model:                 mock2,
		Tools:                 []Tool{toolA, toolB},
		Session:               sess2,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)
	_, err = agent2.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)

	// Partial resume: provide A, B still pending → suspended without
	// re-entering generate.
	resp, err = agent2.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Equal(t, len(resp.OutputMessages), 0,
		"partial-resume-only suspend must not carry OutputMessages (caller should read from session)")
}
