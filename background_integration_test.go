package dive

// Integration tests for background task execution. These tests use the full
// agent loop with mock LLMs, so they exercise the complete path from tool
// execution through response assembly.

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// ---------------------------------------------------------------------------
// Agent loop: single background tool
// ---------------------------------------------------------------------------

// TestAgent_BackgroundTool_SingleTask verifies the full agent loop path:
// - Tool returns BackgroundResult → agent substitutes "started" message
// - Response.BackgroundTasks is populated
// - ContinueWithBackground re-enters and the LLM sees the completed result
func TestAgent_BackgroundTool_SingleTask(t *testing.T) {
	var callCount atomic.Int32

	bgTool := &mockTool{
		name: "run_tests",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewBackgroundResult(ctx, "running test suite", func(ctx context.Context) (string, error) {
				time.Sleep(10 * time.Millisecond)
				return "3 tests passed, 0 failed", nil
			}), nil
		},
	}

	var capturedToolResults []string
	var capturedUserMessages []string
	mock := &mockLLM{
		nameFunc: func() string { return "test-model" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			n := int(callCount.Add(1))
			cfg := &llm.Config{}
			cfg.Apply(opts...)

			// Capture user-role message text for assertions
			for _, msg := range cfg.Messages {
				if msg.Role == llm.User {
					capturedUserMessages = append(capturedUserMessages, msg.Text())
				}
			}

			switch n {
			case 1:
				return &llm.Response{
					ID:    "resp_1",
					Model: "test-model",
					Role:  llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{
							ID:    "tool_use_1",
							Name:  "run_tests",
							Input: []byte(`{}`),
						},
					},
					StopReason: "tool_use",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			case 2:
				return &llm.Response{
					ID:         "resp_2",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Tests started. I'll report back when done."}},
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 20, OutputTokens: 10},
				}, nil
			case 3:
				return &llm.Response{
					ID:         "resp_3",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "All 3 tests passed!"}},
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 30, OutputTokens: 5},
				}, nil
			}
			t.Errorf("unexpected call count %d", n)
			return nil, nil
		},
	}

	agent, err := NewAgent(AgentOptions{
		Name:  "TestAgent",
		Model: mock,
		Tools: []Tool{bgTool},
	})
	assert.NoError(t, err)

	ctx := context.Background()

	// Use event callback to capture the tool call result text (the "started" message)
	resp, err := agent.CreateResponse(ctx,
		WithInput("run all tests"),
		WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
			if item.Type == ResponseItemTypeToolCallResult && item.ToolCallResult != nil {
				capturedToolResults = append(capturedToolResults, toolResultText(item.ToolCallResult.Result))
			}
			return nil
		}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.True(t, len(resp.BackgroundTasks) == 1, "expected 1 background task")

	task := resp.BackgroundTasks[0]
	assert.Equal(t, task.Description, "running test suite")
	assert.Equal(t, task.ToolUseID, "tool_use_1")
	assert.NotEqual(t, task.TaskID, "")

	// The event callback should have captured the "started" message
	foundStarted := false
	for _, msg := range capturedToolResults {
		if strings.Contains(msg, "Background task started: running test suite") {
			foundStarted = true
		}
	}
	assert.True(t, foundStarted, "event callback should have seen the 'started' message")

	// Continue with background: pass the previous output as history for stateless case
	finalResp, err := ContinueWithBackground(ctx, agent, resp,
		WithMessages(resp.OutputMessages...),
	)
	assert.NoError(t, err)
	assert.Equal(t, finalResp.Status, ResponseStatusCompleted)
	assert.True(t, len(finalResp.BackgroundTasks) == 0, "no more background tasks")
	assert.Equal(t, finalResp.OutputText(), "All 3 tests passed!")

	// The completed result should have been injected as a user message
	foundCompleted := false
	for _, msg := range capturedUserMessages {
		if strings.Contains(msg, "Background task completed: running test suite") {
			foundCompleted = true
		}
		if strings.Contains(msg, "3 tests passed") {
			foundCompleted = true
		}
	}
	assert.True(t, foundCompleted, "LLM should have received the background completed message")

	assert.Equal(t, int(callCount.Load()), 3)
}

// ---------------------------------------------------------------------------
// Agent loop: ContinueWithBackground with no tasks is a no-op
// ---------------------------------------------------------------------------

func TestContinueWithBackground_NoTasks_ReturnsUnchanged(t *testing.T) {
	mock := &mockLLM{
		nameFunc: func() string { return "test-model" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			return &llm.Response{
				ID:         "resp_1",
				Model:      "test-model",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "hello"}},
				StopReason: "stop",
				Usage:      llm.Usage{},
			}, nil
		},
	}

	agent, err := NewAgent(AgentOptions{Name: "TestAgent", Model: mock})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.NoError(t, err)
	assert.True(t, len(resp.BackgroundTasks) == 0)

	// ContinueWithBackground should return the same response unchanged
	// without calling CreateResponse again (mock's generate is not called again)
	finalResp, err := ContinueWithBackground(context.Background(), agent, resp)
	assert.NoError(t, err)
	assert.Equal(t, finalResp, resp, "expected same response when no tasks")
}

// ---------------------------------------------------------------------------
// Agent loop: multiple concurrent background tools
// ---------------------------------------------------------------------------

func TestAgent_BackgroundTool_MultipleConcurrent(t *testing.T) {
	var callCount atomic.Int32

	// Two background tools run concurrently
	makeBgTool := func(name, result string, delay time.Duration) Tool {
		return &mockTool{
			name: name,
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return NewBackgroundResult(ctx, "task: "+name, func(ctx context.Context) (string, error) {
					time.Sleep(delay)
					return result, nil
				}), nil
			},
		}
	}

	mock := &mockLLM{
		nameFunc: func() string { return "test-model" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			n := int(callCount.Add(1))
			switch n {
			case 1:
				return &llm.Response{
					ID:   "resp_1",
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{ID: "t1", Name: "tool_a", Input: []byte(`{}`)},
						&llm.ToolUseContent{ID: "t2", Name: "tool_b", Input: []byte(`{}`)},
					},
					StopReason: "tool_use",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			case 2:
				return &llm.Response{
					ID:         "resp_2",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Both tasks started."}},
					StopReason: "stop",
					Usage:      llm.Usage{},
				}, nil
			case 3:
				return &llm.Response{
					ID:         "resp_3",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Both done."}},
					StopReason: "stop",
					Usage:      llm.Usage{},
				}, nil
			}
			t.Errorf("unexpected call %d", n)
			return nil, nil
		},
	}

	agent, err := NewAgent(AgentOptions{
		Name:  "TestAgent",
		Model: mock,
		Tools: []Tool{
			makeBgTool("tool_a", "result A", 20*time.Millisecond),
			makeBgTool("tool_b", "result B", 10*time.Millisecond),
		},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("run both"))
	assert.NoError(t, err)
	assert.Equal(t, len(resp.BackgroundTasks), 2, "expected 2 background tasks")

	finalResp, err := ContinueWithBackground(context.Background(), agent, resp,
		WithMessages(resp.OutputMessages...),
	)
	assert.NoError(t, err)
	assert.Equal(t, finalResp.OutputText(), "Both done.")
	assert.True(t, len(finalResp.BackgroundTasks) == 0)
}

// ---------------------------------------------------------------------------
// Agent loop: background tool + panic recovery in goroutine
// ---------------------------------------------------------------------------

func TestAgent_BackgroundTool_PanicInGoroutine(t *testing.T) {
	var callCount atomic.Int32

	panicTool := &mockTool{
		name: "panic_tool",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewBackgroundResult(ctx, "panicking work", func(ctx context.Context) (string, error) {
				panic("goroutine panic!")
			}), nil
		},
	}

	mock := &mockLLM{
		nameFunc: func() string { return "test-model" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			n := int(callCount.Add(1))
			switch n {
			case 1:
				return &llm.Response{
					ID:   "resp_1",
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{ID: "t1", Name: "panic_tool", Input: []byte(`{}`)},
					},
					StopReason: "tool_use",
					Usage:      llm.Usage{},
				}, nil
			case 2:
				return &llm.Response{
					ID:         "resp_2",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Task started."}},
					StopReason: "stop",
					Usage:      llm.Usage{},
				}, nil
			case 3:
				return &llm.Response{
					ID:         "resp_3",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Task errored."}},
					StopReason: "stop",
					Usage:      llm.Usage{},
				}, nil
			}
			return nil, nil
		},
	}

	agent, err := NewAgent(AgentOptions{
		Name:  "TestAgent",
		Model: mock,
		Tools: []Tool{panicTool},
	})
	assert.NoError(t, err)

	ctx := context.Background()
	resp, err := agent.CreateResponse(ctx, WithInput("do the panicky thing"))
	assert.NoError(t, err)
	assert.Equal(t, len(resp.BackgroundTasks), 1)

	results, err := AwaitBackgroundTasks(ctx, resp.BackgroundTasks)
	assert.NoError(t, err)

	panicResult := results[resp.BackgroundTasks[0].TaskID]
	assert.NotNil(t, panicResult)
	assert.True(t, panicResult.IsError, "panic should produce an error result")
	assert.True(t, strings.Contains(toolResultText(panicResult), "background task panicked"))

	// Re-enter with the panic result — agent should complete without crashing
	finalResp, err := agent.CreateResponse(ctx,
		WithBackgroundResults(resp.BackgroundTasks, results),
		WithMessages(resp.OutputMessages...),
	)
	assert.NoError(t, err)
	assert.Equal(t, finalResp.OutputText(), "Task errored.")
}

// ---------------------------------------------------------------------------
// PostBackgroundToolUse hook fires on result delivery
// ---------------------------------------------------------------------------

func TestAgent_PostBackgroundToolUseHook(t *testing.T) {
	var callCount atomic.Int32
	var hookFired atomic.Bool
	var hookResultText string

	bgTool := &mockTool{
		name: "bg_task",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewBackgroundResult(ctx, "hook test", func(ctx context.Context) (string, error) {
				return "hook result", nil
			}), nil
		},
	}

	mock := &mockLLM{
		nameFunc: func() string { return "test-model" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			n := int(callCount.Add(1))
			switch n {
			case 1:
				return &llm.Response{
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{ID: "t1", Name: "bg_task", Input: []byte(`{}`)},
					},
					StopReason: "tool_use",
					Usage:      llm.Usage{},
				}, nil
			default:
				return &llm.Response{
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "done"}},
					StopReason: "stop",
					Usage:      llm.Usage{},
				}, nil
			}
		},
	}

	agent, err := NewAgent(AgentOptions{
		Name:  "TestAgent",
		Model: mock,
		Tools: []Tool{bgTool},
		Hooks: Hooks{
			PostBackgroundToolUse: []PostBackgroundToolUseHook{
				func(ctx context.Context, hctx *HookContext) error {
					hookFired.Store(true)
					if hctx.Result != nil {
						hookResultText = toolResultText(hctx.Result.Result)
					}
					return nil
				},
			},
		},
	})
	assert.NoError(t, err)

	ctx := context.Background()
	resp, err := agent.CreateResponse(ctx, WithInput("run it"))
	assert.NoError(t, err)
	assert.False(t, hookFired.Load(), "hook should not fire until results delivered")

	_, err = ContinueWithBackground(ctx, agent, resp,
		WithMessages(resp.OutputMessages...),
	)
	assert.NoError(t, err)
	assert.True(t, hookFired.Load(), "hook should have fired when results were delivered")
	assert.Equal(t, hookResultText, "hook result")
}

// ---------------------------------------------------------------------------
// ToolResult invariant: Background + Content is rejected
// ---------------------------------------------------------------------------

func TestAgent_BackgroundToolResult_MixedFieldsRejected(t *testing.T) {
	var callCount atomic.Int32

	malformedTool := &mockTool{
		name: "bad_tool",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			// Create a malformed result: Background + regular Content both set
			bg := NewBackgroundResult(ctx, "work", func(ctx context.Context) (string, error) {
				return "ok", nil
			})
			// Manually inject Content alongside Background (violates the union)
			bg.Content = []*ToolResultContent{{Type: ToolResultContentTypeText, Text: "extra"}}
			return bg, nil
		},
	}

	mock := &mockLLM{
		nameFunc: func() string { return "test-model" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			n := int(callCount.Add(1))
			switch n {
			case 1:
				return &llm.Response{
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{ID: "t1", Name: "bad_tool", Input: []byte(`{}`)},
					},
					StopReason: "tool_use",
					Usage:      llm.Usage{},
				}, nil
			default:
				return &llm.Response{
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "got an error"}},
					StopReason: "stop",
					Usage:      llm.Usage{},
				}, nil
			}
		},
	}

	agent, err := NewAgent(AgentOptions{
		Name:  "TestAgent",
		Model: mock,
		Tools: []Tool{malformedTool},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("test"))
	assert.NoError(t, err)
	// Malformed result → IsError result → no background task on response
	assert.True(t, len(resp.BackgroundTasks) == 0, "malformed result should not produce a background task")
	// The LLM got a final answer (not suspended, not background)
	assert.Equal(t, resp.OutputText(), "got an error")
}

// ---------------------------------------------------------------------------
// WithBackgroundResults: synthetic message format reaches LLM
// ---------------------------------------------------------------------------

func TestAgent_WithBackgroundResults_InjectsMessage(t *testing.T) {
	var capturedUserMessages []string
	var callCount atomic.Int32

	mock := &mockLLM{
		nameFunc: func() string { return "test-model" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			n := int(callCount.Add(1))
			cfg := &llm.Config{}
			cfg.Apply(opts...)

			// Capture user message text (using msg.Text() for combined text)
			for _, msg := range cfg.Messages {
				if msg.Role == llm.User {
					capturedUserMessages = append(capturedUserMessages, msg.Text())
				}
			}

			switch n {
			case 1:
				return &llm.Response{
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "done"}},
					StopReason: "stop",
					Usage:      llm.Usage{},
				}, nil
			}
			return nil, nil
		},
	}

	agent, err := NewAgent(AgentOptions{Name: "TestAgent", Model: mock})
	assert.NoError(t, err)

	// Simulate a completed background task
	handles := []*BackgroundTaskHandle{
		{TaskID: "task-99", ToolUseID: "t1", Description: "deploy to staging"},
	}
	results := map[string]*ToolResult{
		"task-99": NewToolResultText("deployment successful"),
	}

	_, err = agent.CreateResponse(context.Background(),
		WithBackgroundResults(handles, results),
		WithInput("check status"),
	)
	assert.NoError(t, err)

	// The synthetic completed message should appear in user messages
	found := false
	for _, msg := range capturedUserMessages {
		if strings.Contains(msg, "Background task completed: deploy to staging") &&
			strings.Contains(msg, "deployment successful") {
			found = true
		}
	}
	assert.True(t, found, "LLM should have received the background completed message")
}

// ---------------------------------------------------------------------------
// memSession: minimal in-package session for background integration tests
// ---------------------------------------------------------------------------

// memSession is a trivial in-memory Session implementation used by tests that
// need session-backed history without importing the session sub-package (which
// would create an import cycle from within package dive).
type memSession struct {
	mu       sync.Mutex
	id       string
	messages []*llm.Message
}

func newMemSession(id string) *memSession { return &memSession{id: id} }
func (s *memSession) ID() string         { return s.id }
func (s *memSession) Messages(_ context.Context) ([]*llm.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]*llm.Message, len(s.messages))
	copy(cp, s.messages)
	return cp, nil
}
func (s *memSession) SaveTurn(_ context.Context, msgs []*llm.Message, _ *llm.Usage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msgs...)
	return nil
}

// ---------------------------------------------------------------------------
// Session-backed continuation (no WithMessages needed)
// ---------------------------------------------------------------------------

// TestAgent_BackgroundTool_SessionBacked verifies that a session-backed agent
// can continue background results without passing WithMessages — the session
// carries the conversation history automatically.
func TestAgent_BackgroundTool_SessionBacked(t *testing.T) {
	var callCount atomic.Int32

	bgTool := &mockTool{
		name: "analyze",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewBackgroundResult(ctx, "analyzing data", func(ctx context.Context) (string, error) {
				return "analysis complete: 42 items found", nil
			}), nil
		},
	}

	mock := &mockLLM{
		nameFunc: func() string { return "test-model" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			n := int(callCount.Add(1))
			switch n {
			case 1:
				return &llm.Response{
					ID:   "resp_1",
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{ID: "t1", Name: "analyze", Input: []byte(`{}`)},
					},
					StopReason: "tool_use",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			case 2:
				return &llm.Response{
					ID:         "resp_2",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Analyzing in background."}},
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 20, OutputTokens: 10},
				}, nil
			case 3:
				return &llm.Response{
					ID:         "resp_3",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Found 42 items."}},
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 30, OutputTokens: 10},
				}, nil
			}
			t.Errorf("unexpected call count %d", n)
			return nil, nil
		},
	}

	sess := newMemSession("bg-session-test")
	agent, err := NewAgent(AgentOptions{
		Name:    "TestAgent",
		Model:   mock,
		Tools:   []Tool{bgTool},
		Session: sess,
	})
	assert.NoError(t, err)

	ctx := context.Background()
	resp, err := agent.CreateResponse(ctx, WithInput("analyze the data"))
	assert.NoError(t, err)
	assert.Equal(t, len(resp.BackgroundTasks), 1)

	// Session-backed: no WithMessages needed — session carries history
	finalResp, err := ContinueWithBackground(ctx, agent, resp)
	assert.NoError(t, err)
	assert.Equal(t, finalResp.OutputText(), "Found 42 items.")
	assert.Equal(t, int(callCount.Load()), 3)
}

// ---------------------------------------------------------------------------
// Background task with Display field
// ---------------------------------------------------------------------------

// TestAgent_BackgroundTool_WithDisplay verifies that NewBackgroundResultFull
// lets tools attach a Display field for richer UI output while keeping the
// Content text for the LLM.
func TestAgent_BackgroundTool_WithDisplay(t *testing.T) {
	var callCount atomic.Int32

	bgTool := &mockTool{
		name: "generate_report",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewBackgroundResultFull(ctx, "generating report", func(ctx context.Context) *ToolResult {
				return NewToolResultText("Report: 3 issues found").
					WithDisplay("# Report\n\n- Issue 1\n- Issue 2\n- Issue 3")
			}), nil
		},
	}

	mock := &mockLLM{
		nameFunc: func() string { return "test-model" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			n := int(callCount.Add(1))
			switch n {
			case 1:
				return &llm.Response{
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{ID: "t1", Name: "generate_report", Input: []byte(`{}`)},
					},
					StopReason: "tool_use",
					Usage:      llm.Usage{},
				}, nil
			default:
				return &llm.Response{
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Report generated."}},
					StopReason: "stop",
					Usage:      llm.Usage{},
				}, nil
			}
		},
	}

	agent, err := NewAgent(AgentOptions{
		Name:  "TestAgent",
		Model: mock,
		Tools: []Tool{bgTool},
	})
	assert.NoError(t, err)

	ctx := context.Background()
	resp, err := agent.CreateResponse(ctx, WithInput("generate report"))
	assert.NoError(t, err)
	assert.Equal(t, len(resp.BackgroundTasks), 1)

	results, err := AwaitBackgroundTasks(ctx, resp.BackgroundTasks)
	assert.NoError(t, err)

	result := results[resp.BackgroundTasks[0].TaskID]
	assert.NotNil(t, result)
	assert.Equal(t, result.Display, "# Report\n\n- Issue 1\n- Issue 2\n- Issue 3")
	assert.Equal(t, toolResultText(result), "Report: 3 issues found")

	// Continue: LLM receives the text (not Display), agent finishes
	finalResp, err := agent.CreateResponse(ctx,
		WithBackgroundResults(resp.BackgroundTasks, results),
		WithMessages(resp.OutputMessages...),
	)
	assert.NoError(t, err)
	assert.Equal(t, finalResp.OutputText(), "Report generated.")
}

// ---------------------------------------------------------------------------
// Chained background tasks
// ---------------------------------------------------------------------------

// TestAgent_ChainedBackgroundTasks verifies that a background continuation
// can itself spawn new background tasks, allowing multi-step async chains.
func TestAgent_ChainedBackgroundTasks(t *testing.T) {
	var callCount atomic.Int32

	bgTool1 := &mockTool{
		name: "step_one",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewBackgroundResult(ctx, "step one", func(ctx context.Context) (string, error) {
				return "step one done", nil
			}), nil
		},
	}
	bgTool2 := &mockTool{
		name: "step_two",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewBackgroundResult(ctx, "step two", func(ctx context.Context) (string, error) {
				return "step two done", nil
			}), nil
		},
	}

	mock := &mockLLM{
		nameFunc: func() string { return "test-model" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			n := int(callCount.Add(1))
			switch n {
			case 1: // initial: request step_one
				return &llm.Response{
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{ID: "t1", Name: "step_one", Input: []byte(`{}`)},
					},
					StopReason: "tool_use",
					Usage:      llm.Usage{},
				}, nil
			case 2: // step_one started: ack
				return &llm.Response{
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Step one started."}},
					StopReason: "stop",
					Usage:      llm.Usage{},
				}, nil
			case 3: // step_one results delivered: request step_two
				return &llm.Response{
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{ID: "t2", Name: "step_two", Input: []byte(`{}`)},
					},
					StopReason: "tool_use",
					Usage:      llm.Usage{},
				}, nil
			case 4: // step_two started: ack
				return &llm.Response{
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Step two started."}},
					StopReason: "stop",
					Usage:      llm.Usage{},
				}, nil
			case 5: // step_two results delivered: final answer
				return &llm.Response{
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "All done!"}},
					StopReason: "stop",
					Usage:      llm.Usage{},
				}, nil
			}
			t.Errorf("unexpected call count %d", n)
			return nil, nil
		},
	}

	agent, err := NewAgent(AgentOptions{
		Name:  "TestAgent",
		Model: mock,
		Tools: []Tool{bgTool1, bgTool2},
	})
	assert.NoError(t, err)

	ctx := context.Background()

	// Turn 1: spawns step_one
	resp, err := agent.CreateResponse(ctx, WithInput("run all steps"))
	assert.NoError(t, err)
	assert.Equal(t, len(resp.BackgroundTasks), 1, "first turn should have 1 bg task")
	assert.Equal(t, resp.BackgroundTasks[0].Description, "step one")

	// Turn 2: step_one result delivered → spawns step_two
	resp2, err := ContinueWithBackground(ctx, agent, resp, WithMessages(resp.OutputMessages...))
	assert.NoError(t, err)
	assert.Equal(t, len(resp2.BackgroundTasks), 1, "second turn should spawn step two")
	assert.Equal(t, resp2.BackgroundTasks[0].Description, "step two")

	// Turn 3: step_two result delivered → final answer
	finalResp, err := ContinueWithBackground(ctx, agent, resp2, WithMessages(resp2.OutputMessages...))
	assert.NoError(t, err)
	assert.Equal(t, len(finalResp.BackgroundTasks), 0)
	assert.Equal(t, finalResp.OutputText(), "All done!")
	assert.Equal(t, int(callCount.Load()), 5)
}
