package dive

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestAgentCreateResponse demonstrates using the CreateResponse API
func TestAgentCreateResponse(t *testing.T) {
	// Setup a simple mock LLM
	mockLLM := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			return &llm.Response{
				ID:         "resp_123",
				Model:      "test-model",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "This is a test response"}},
				Type:       "message",
				StopReason: "stop",
				Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
			}, nil
		},
		nameFunc: func() string {
			return "test-model"
		},
	}

	// Create a simple agent with the mock LLM
	agent, err := NewAgent(AgentOptions{
		Name:  "TestAgent",
		Model: mockLLM,
	})
	assert.NoError(t, err)

	t.Run("CreateResponse with input string", func(t *testing.T) {
		resp, err := agent.CreateResponse(context.Background(), WithInput("Hello, agent!"))
		assert.NoError(t, err)

		// Check that items exist and contain the expected message
		assert.True(t, len(resp.Items) > 0, "expected response to have items")
		found := false
		for _, item := range resp.Items {
			if item.Type == ResponseItemTypeMessage && item.Message != nil {
				if item.Message.Text() == "This is a test response" {
					found = true
					break
				}
			}
		}
		assert.True(t, found, "expected to find 'This is a test response' in items")

		assert.NotNil(t, resp.Usage)
		assert.Equal(t, resp.Usage.InputTokens, 10)
		assert.Equal(t, resp.Usage.OutputTokens, 5)
	})

	t.Run("CreateResponse with messages", func(t *testing.T) {
		messages := []*llm.Message{
			llm.NewUserTextMessage("Here's a more complex message"),
		}
		resp, err := agent.CreateResponse(context.Background(), WithMessages(messages...))
		assert.NoError(t, err)

		assert.True(t, len(resp.Items) > 0, "expected response to have items")
		found := false
		for _, item := range resp.Items {
			if item.Type == ResponseItemTypeMessage && item.Message != nil {
				if item.Message.Text() == "This is a test response" {
					found = true
					break
				}
			}
		}
		assert.True(t, found, "expected to find 'This is a test response' in items")
	})

	t.Run("CreateResponse with callback for final message", func(t *testing.T) {
		var callbackItems []*ResponseItem
		eventCallback := func(ctx context.Context, item *ResponseItem) error {
			callbackItems = append(callbackItems, item)
			return nil
		}

		resp, err := agent.CreateResponse(
			context.Background(),
			WithInput("Hello, agent!"),
			WithEventCallback(eventCallback),
		)
		assert.NoError(t, err)
		assert.True(t, len(callbackItems) > 0, "expected callback to be called")

		// Find the message item in callback items
		foundMessage := false
		for _, item := range callbackItems {
			if item.Type == ResponseItemTypeMessage {
				foundMessage = true
				assert.NotNil(t, item.Message)
				assert.Equal(t, item.Message.Text(), "This is a test response")
				assert.NotNil(t, item.Usage)
				assert.Equal(t, item.Usage.InputTokens, 10)
				assert.Equal(t, item.Usage.OutputTokens, 5)
			}
		}
		assert.True(t, foundMessage, "expected to find a message callback item")
		assert.True(t, len(resp.Items) > 0, "expected response to have items")
	})
}

// TestMessageCopy tests the Message.Copy method
func TestMessageCopy(t *testing.T) {
	original := llm.NewUserTextMessage("Hello, world!")
	copied := original.Copy()

	assert.NotEqual(t, fmt.Sprintf("%p", copied), fmt.Sprintf("%p", original), "Copy should return a new pointer")
	assert.Equal(t, copied.Role, original.Role)
	assert.Equal(t, copied.Text(), original.Text())
}

// Mock types for testing

type mockLLM struct {
	generateFunc func(ctx context.Context, opts ...llm.Option) (*llm.Response, error)
	nameFunc     func() string
}

func (m *mockLLM) Name() string {
	if m.nameFunc != nil {
		return m.nameFunc()
	}
	return "mock-llm"
}

func (m *mockLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	return m.generateFunc(ctx, opts...)
}

// mockTool is a simple tool for testing tool call flows.
type mockTool struct {
	name        string
	callFunc    func(ctx context.Context, input any) (*ToolResult, error)
	annotations *ToolAnnotations
}

func (t *mockTool) Name() string                  { return t.name }
func (t *mockTool) Description() string           { return "mock tool" }
func (t *mockTool) Schema() *Schema               { return nil }
func (t *mockTool) Annotations() *ToolAnnotations { return t.annotations }
func (t *mockTool) Call(ctx context.Context, input any) (*ToolResult, error) {
	return t.callFunc(ctx, input)
}

func TestResponseItemsContainToolCalls(t *testing.T) {
	callCount := 0
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			callCount++
			if callCount == 1 {
				return &llm.Response{
					ID:    "resp_1",
					Model: "test-model",
					Role:  llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{
							ID:    "tool_1",
							Name:  "test_tool",
							Input: []byte(`{}`),
						},
					},
					Type:       "message",
					StopReason: "tool_use",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			}
			return &llm.Response{
				ID:         "resp_2",
				Model:      "test-model",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				Type:       "message",
				StopReason: "stop",
				Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
			}, nil
		},
		nameFunc: func() string { return "test-model" },
	}

	tool := &mockTool{
		name: "test_tool",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewToolResultText("tool output"), nil
		},
	}

	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{tool},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
	assert.NoError(t, err)

	// Verify Response.Items contains all item types in order
	var types []ResponseItemType
	for _, item := range resp.Items {
		types = append(types, item.Type)
	}

	expected := []ResponseItemType{
		ResponseItemTypeMessage,
		ResponseItemTypeToolCall,
		ResponseItemTypeToolCallResult,
		ResponseItemTypeMessage,
	}
	assert.Equal(t, types, expected)

	// Verify ToolCallResults() returns the tool result
	results := resp.ToolCallResults()
	assert.Len(t, results, 1)
	assert.Equal(t, results[0].Name, "test_tool")

	// Verify OutputText() returns the final message text
	assert.Equal(t, resp.OutputText(), "Done")
}

func TestAgentRecoversCanonicalDottedToolAlias(t *testing.T) {
	callCount := 0
	toolCalled := false
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			callCount++
			if callCount == 1 {
				return &llm.Response{
					ID:   "resp_1",
					Role: llm.Assistant,
					Content: []llm.Content{&llm.ToolUseContent{
						ID: "tool_1", Name: "naming.concept.walk", Input: []byte(`{"seed":"compass"}`),
					}},
					StopReason: "tool_use",
				}, nil
			}
			return &llm.Response{
				ID: "resp_2", Role: llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				StopReason: "stop",
			}, nil
		},
	}
	tool := &mockTool{
		name: "naming_concept_walk",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			toolCalled = true
			return NewToolResultText("associations"), nil
		},
	}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("Explore compass"))
	assert.NoError(t, err)
	assert.True(t, toolCalled)
	assert.Equal(t, resp.ToolCallResults()[0].Name, "naming_concept_walk")

	toolUse, ok := resp.OutputMessages[0].Content[0].(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, toolUse.Name, "naming_concept_walk")
}

func TestProviderSafeToolAlias(t *testing.T) {
	assert.Equal(t, providerSafeToolAlias(" naming.concept.walk "), "naming_concept_walk")
	assert.Equal(t, providerSafeToolAlias("already_safe"), "already_safe")
	assert.Equal(t, len(providerSafeToolAlias("very.long.tool.name.with.sections.that.exceed.the.provider.name.limit.by.quite.a.bit")), 64)
}

// TestPreIterationCanRewriteMessages verifies that a PreIteration hook can
// replace the working message set mid-turn (the seam mid-turn compaction relies
// on): the next LLM iteration sees the rewritten messages, while the response's
// OutputMessages keep full fidelity (the rewrite is model-facing only, so the
// saved turn is unaffected).
func TestPreIterationCanRewriteMessages(t *testing.T) {
	const sentinel = "compacted-context-sentinel"

	var secondCallMessages []*llm.Message
	callCount := 0
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			callCount++
			if callCount == 1 {
				// First iteration: emit a tool call so the loop continues.
				return &llm.Response{
					ID:         "resp_1",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.ToolUseContent{ID: "t1", Name: "noop", Input: []byte(`{}`)}},
					Type:       "message",
					StopReason: "tool_use",
				}, nil
			}
			// Second iteration: record what the model actually received.
			cfg := &llm.Config{}
			cfg.Apply(opts...)
			secondCallMessages = cfg.Messages
			return &llm.Response{
				ID:         "resp_2",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				Type:       "message",
				StopReason: "stop",
			}, nil
		},
	}

	tool := &mockTool{
		name:     "noop",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return NewToolResultText("ok"), nil },
	}

	// On the second iteration (i==1), replace the whole working set with a
	// single sentinel message — standing in for a compaction summary.
	rewrite := func(ctx context.Context, hctx *HookContext) error {
		if hctx.Iteration == 1 {
			hctx.Messages = []*llm.Message{llm.NewUserTextMessage(sentinel)}
		}
		return nil
	}

	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{tool},
		Hooks: Hooks{PreIteration: []PreIterationHook{rewrite}},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
	assert.NoError(t, err)
	assert.Equal(t, "Done", resp.OutputText())

	// The second LLM call saw exactly the rewritten message set.
	assert.Len(t, secondCallMessages, 1)
	assert.Equal(t, sentinel, secondCallMessages[0].Text())

	// ...but the turn's OutputMessages keep full fidelity (assistant tool_use,
	// tool_result, final assistant) — the model-facing rewrite never touched
	// what gets persisted.
	assert.Equal(t, 3, len(resp.OutputMessages))
	for _, m := range resp.OutputMessages {
		assert.NotEqual(t, sentinel, m.Text(), "persisted turn must not contain the model-facing compaction sentinel")
	}
}

func TestResponseOutputMessages(t *testing.T) {
	callCount := 0
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			callCount++
			if callCount == 1 {
				return &llm.Response{
					ID:    "resp_1",
					Model: "test-model",
					Role:  llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{
							ID:    "tool_1",
							Name:  "test_tool",
							Input: []byte(`{}`),
						},
					},
					Type:       "message",
					StopReason: "tool_use",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			}
			return &llm.Response{
				ID:         "resp_2",
				Model:      "test-model",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				Type:       "message",
				StopReason: "stop",
				Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
			}, nil
		},
		nameFunc: func() string { return "test-model" },
	}

	tool := &mockTool{
		name: "test_tool",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewToolResultText("tool output"), nil
		},
	}

	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{tool},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
	assert.NoError(t, err)

	// OutputMessages should contain: assistant msg, tool result msg, final assistant msg
	assert.Len(t, resp.OutputMessages, 3)
	assert.Equal(t, resp.OutputMessages[0].Role, llm.Assistant)
	assert.Equal(t, resp.OutputMessages[1].Role, llm.User) // tool results are user messages
	assert.Equal(t, resp.OutputMessages[2].Role, llm.Assistant)
	assert.Equal(t, resp.OutputMessages[2].Text(), "Done")
}

func TestNilToolOutput(t *testing.T) {
	callCount := 0
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			callCount++
			if callCount == 1 {
				return &llm.Response{
					ID:    "resp_1",
					Model: "test-model",
					Role:  llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{
							ID:    "tool_1",
							Name:  "nil_tool",
							Input: []byte(`{}`),
						},
					},
					Type:       "message",
					StopReason: "tool_use",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			}
			return &llm.Response{
				ID:         "resp_2",
				Model:      "test-model",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				Type:       "message",
				StopReason: "stop",
				Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
			}, nil
		},
		nameFunc: func() string { return "test-model" },
	}

	tool := &mockTool{
		name: "nil_tool",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return nil, nil
		},
	}

	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{tool},
	})
	assert.NoError(t, err)

	// This should not panic
	resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "Done")
}

func TestDuplicateToolNames(t *testing.T) {
	mock := &mockLLM{nameFunc: func() string { return "test-model" }}

	tool1 := &mockTool{
		name:     "same_name",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
	}
	tool2 := &mockTool{
		name:     "same_name",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
	}

	_, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{tool1, tool2},
	})
	assert.Error(t, err)
	assert.ErrorContains(t, err, `duplicate tool name: "same_name"`)
}

func TestToolsReturnsCopy(t *testing.T) {
	mock := &mockLLM{nameFunc: func() string { return "test-model" }}
	tool := &mockTool{
		name:     "test_tool",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
	}

	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{tool},
	})
	assert.NoError(t, err)

	tools := agent.Tools()
	assert.Len(t, tools, 1)

	// Modifying returned slice should not affect agent's tools
	tools[0] = nil
	agentTools := agent.Tools()
	assert.NotNil(t, agentTools[0], "modifying Tools() return value should not affect agent's internal tools")
}

// TestHookAbortError tests the HookAbortError functionality across all hook types
func TestHookAbortError(t *testing.T) {
	mockLLM := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			return &llm.Response{
				ID:         "resp_123",
				Model:      "test-model",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Test response"}},
				Type:       "message",
				StopReason: "stop",
				Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
			}, nil
		},
	}

	t.Run("PostGeneration with regular error logs and continues", func(t *testing.T) {
		agent, _ := NewAgent(AgentOptions{
			Model: mockLLM,
			Hooks: Hooks{
				PostGeneration: []PostGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						return fmt.Errorf("regular error")
					},
				},
			},
		})

		resp, err := agent.CreateResponse(context.Background(), WithInput("test"))
		assert.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("PostGeneration with HookAbortError aborts", func(t *testing.T) {
		agent, _ := NewAgent(AgentOptions{
			Model: mockLLM,
			Hooks: Hooks{
				PostGeneration: []PostGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						return AbortGeneration("safety violation detected")
					},
				},
			},
		})

		resp, err := agent.CreateResponse(context.Background(), WithInput("test"))
		assert.Error(t, err)
		assert.Nil(t, resp)

		var abortErr *HookAbortError
		assert.ErrorAs(t, err, &abortErr)
		assert.Equal(t, abortErr.Reason, "safety violation detected")
		assert.Equal(t, abortErr.HookType, "PostGeneration")
	})

	t.Run("PreGeneration with any error aborts", func(t *testing.T) {
		agent, _ := NewAgent(AgentOptions{
			Model: mockLLM,
			Hooks: Hooks{
				PreGeneration: []PreGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						return fmt.Errorf("setup failed")
					},
				},
			},
		})

		resp, err := agent.CreateResponse(context.Background(), WithInput("test"))
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorContains(t, err, "pre-generation hook error: setup failed")
	})

	t.Run("HookAbortError with cause", func(t *testing.T) {
		causeErr := fmt.Errorf("underlying cause")
		agent, _ := NewAgent(AgentOptions{
			Model: mockLLM,
			Hooks: Hooks{
				PostGeneration: []PostGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						return AbortGenerationWithCause("wrapped error", causeErr)
					},
				},
			},
		})

		_, err := agent.CreateResponse(context.Background(), WithInput("test"))
		assert.Error(t, err)

		var abortErr *HookAbortError
		assert.ErrorAs(t, err, &abortErr)
		assert.Equal(t, abortErr.Cause, causeErr)
		assert.ErrorIs(t, err, causeErr)
	})
}

func TestParallelToolExecution(t *testing.T) {
	t.Run("tools execute concurrently", func(t *testing.T) {
		callCount := 0
		var concurrent atomic.Int32

		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				callCount++
				if callCount == 1 {
					return &llm.Response{
						ID:    "resp_1",
						Model: "test-model",
						Role:  llm.Assistant,
						Content: []llm.Content{
							&llm.ToolUseContent{ID: "t1", Name: "slow_tool", Input: []byte(`{"id":"1"}`)},
							&llm.ToolUseContent{ID: "t2", Name: "slow_tool", Input: []byte(`{"id":"2"}`)},
							&llm.ToolUseContent{ID: "t3", Name: "slow_tool", Input: []byte(`{"id":"3"}`)},
						},
						Type:       "message",
						StopReason: "tool_use",
						Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
					}, nil
				}
				return &llm.Response{
					ID:         "resp_2",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		var maxConcurrent atomic.Int32
		var startTimes sync.Map // tool index -> time.Time
		toolDuration := 50 * time.Millisecond
		tool := &mockTool{
			name: "slow_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				cur := concurrent.Add(1)
				defer concurrent.Add(-1)
				startTimes.Store(cur, time.Now())
				for {
					old := maxConcurrent.Load()
					if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
						break
					}
				}
				time.Sleep(toolDuration)
				return NewToolResultText("done"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model:                 mock,
			Tools:                 []Tool{tool},
			ParallelToolExecution: true,
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use tools"))
		assert.NoError(t, err)
		assert.Equal(t, resp.OutputText(), "Done")

		// All 3 tool results should be present
		results := resp.ToolCallResults()
		assert.Len(t, results, 3)

		// Verify actual concurrency: max concurrent should be > 1
		assert.True(t, maxConcurrent.Load() > 1,
			"expected concurrent execution, max concurrent was %d", maxConcurrent.Load())

		// Verify overlap deterministically: collect start timestamps and
		// assert that at least two tools started within one tool duration.
		var starts []time.Time
		startTimes.Range(func(_, v any) bool {
			starts = append(starts, v.(time.Time))
			return true
		})
		assert.True(t, len(starts) >= 2, "expected at least 2 recorded start times")
		earliest, latest := starts[0], starts[0]
		for _, ts := range starts[1:] {
			if ts.Before(earliest) {
				earliest = ts
			}
			if ts.After(latest) {
				latest = ts
			}
		}
		assert.True(t, latest.Sub(earliest) < toolDuration,
			"expected overlapping starts, spread was %s (tool duration %s)", latest.Sub(earliest), toolDuration)
	})

	t.Run("callbacks fire as each tool completes", func(t *testing.T) {
		// Verify that ToolCallResult events are emitted as soon as each tool
		// finishes, rather than waiting for all tools to complete. The fast
		// tool's result callback should fire while the slow tool is still running.
		callCount := 0

		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				callCount++
				if callCount == 1 {
					return &llm.Response{
						ID:    "resp_1",
						Model: "test-model",
						Role:  llm.Assistant,
						Content: []llm.Content{
							&llm.ToolUseContent{ID: "t1", Name: "fast_tool", Input: []byte(`{}`)},
							&llm.ToolUseContent{ID: "t2", Name: "slow_tool", Input: []byte(`{}`)},
						},
						Type:       "message",
						StopReason: "tool_use",
						Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
					}, nil
				}
				return &llm.Response{
					ID:         "resp_2",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		slowDuration := 200 * time.Millisecond
		var slowRunning atomic.Bool
		slowStarted := make(chan struct{})

		fastTool := &mockTool{
			name: "fast_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				// Wait until the slow tool has entered its body before returning,
				// so we know slowRunning is true when the fast callback fires.
				<-slowStarted
				return NewToolResultText("fast"), nil
			},
		}
		slowTool := &mockTool{
			name: "slow_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				slowRunning.Store(true)
				close(slowStarted)
				defer slowRunning.Store(false)
				time.Sleep(slowDuration)
				return NewToolResultText("slow"), nil
			},
		}

		// Track when each ToolCallResult callback fires and whether the
		// slow tool is still running at that moment.
		type callbackRecord struct {
			toolName         string
			slowStillRunning bool
		}
		var records []callbackRecord

		agent, err := NewAgent(AgentOptions{
			Model:                 mock,
			Tools:                 []Tool{fastTool, slowTool},
			ParallelToolExecution: true,
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(),
			WithInput("Use tools"),
			WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
				if item.Type == ResponseItemTypeToolCallResult {
					records = append(records, callbackRecord{
						toolName:         item.ToolCallResult.Name,
						slowStillRunning: slowRunning.Load(),
					})
				}
				return nil
			}),
		)
		assert.NoError(t, err)
		assert.Equal(t, resp.OutputText(), "Done")
		assert.Len(t, records, 2)

		// The fast tool's callback should have fired while the slow tool
		// was still running. Find the fast tool's record and verify.
		var fastRecord *callbackRecord
		for i := range records {
			if records[i].toolName == "fast_tool" {
				fastRecord = &records[i]
				break
			}
		}
		assert.NotNil(t, fastRecord, "expected fast_tool callback record")
		assert.True(t, fastRecord.slowStillRunning,
			"fast_tool callback should fire while slow_tool is still running")
	})

	t.Run("sequential when parallel disabled", func(t *testing.T) {
		callCount := 0
		var concurrent atomic.Int32

		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				callCount++
				if callCount == 1 {
					return &llm.Response{
						ID:    "resp_1",
						Model: "test-model",
						Role:  llm.Assistant,
						Content: []llm.Content{
							&llm.ToolUseContent{ID: "t1", Name: "slow_tool", Input: []byte(`{"id":"1"}`)},
							&llm.ToolUseContent{ID: "t2", Name: "slow_tool", Input: []byte(`{"id":"2"}`)},
						},
						Type:       "message",
						StopReason: "tool_use",
						Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
					}, nil
				}
				return &llm.Response{
					ID:         "resp_2",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		var maxConcurrent atomic.Int32
		tool := &mockTool{
			name: "slow_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				cur := concurrent.Add(1)
				defer concurrent.Add(-1)
				for {
					old := maxConcurrent.Load()
					if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
				return NewToolResultText("done"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model:                 mock,
			Tools:                 []Tool{tool},
			ParallelToolExecution: false, // default
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use tools"))
		assert.NoError(t, err)
		assert.Equal(t, resp.OutputText(), "Done")

		// Max concurrent should be exactly 1 (sequential)
		assert.Equal(t, maxConcurrent.Load(), int32(1))
	})

	t.Run("hooks work with parallel execution", func(t *testing.T) {
		callCount := 0
		var hookCalls atomic.Int32

		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				callCount++
				if callCount == 1 {
					return &llm.Response{
						ID:    "resp_1",
						Model: "test-model",
						Role:  llm.Assistant,
						Content: []llm.Content{
							&llm.ToolUseContent{ID: "t1", Name: "tool_a", Input: []byte(`{}`)},
							&llm.ToolUseContent{ID: "t2", Name: "tool_b", Input: []byte(`{}`)},
						},
						Type:       "message",
						StopReason: "tool_use",
						Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
					}, nil
				}
				return &llm.Response{
					ID:         "resp_2",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		toolA := &mockTool{
			name: "tool_a",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return NewToolResultText("a"), nil
			},
		}
		toolB := &mockTool{
			name: "tool_b",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return NewToolResultText("b"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model:                 mock,
			Tools:                 []Tool{toolA, toolB},
			ParallelToolExecution: true,
			Hooks: Hooks{
				PostToolUse: []PostToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						hookCalls.Add(1)
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use tools"))
		assert.NoError(t, err)
		assert.Equal(t, resp.OutputText(), "Done")

		// Both tools should have triggered the PostToolUse hook
		assert.Equal(t, hookCalls.Load(), int32(2))
	})
}

// TestSequentialOnlyHint verifies that when a batch contains any tool with
// SequentialOnlyHint set, the agent falls back to sequential execution even
// when ParallelToolExecution is enabled.
func TestSequentialOnlyHint(t *testing.T) {
	t.Run("hint forces sequential fallback", func(t *testing.T) {
		callCount := 0
		var concurrent atomic.Int32
		var maxConcurrent atomic.Int32

		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				callCount++
				if callCount == 1 {
					return &llm.Response{
						ID:    "resp_1",
						Model: "test-model",
						Role:  llm.Assistant,
						Content: []llm.Content{
							&llm.ToolUseContent{ID: "t1", Name: "parallel_tool", Input: []byte(`{}`)},
							&llm.ToolUseContent{ID: "t2", Name: "serial_tool", Input: []byte(`{}`)},
							&llm.ToolUseContent{ID: "t3", Name: "parallel_tool", Input: []byte(`{}`)},
						},
						Type:       "message",
						StopReason: "tool_use",
						Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
					}, nil
				}
				return &llm.Response{
					ID:         "resp_2",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		trackConcurrency := func(ctx context.Context, input any) (*ToolResult, error) {
			cur := concurrent.Add(1)
			defer concurrent.Add(-1)
			for {
				old := maxConcurrent.Load()
				if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			return NewToolResultText("done"), nil
		}

		parallelTool := &mockTool{name: "parallel_tool", callFunc: trackConcurrency}
		serialTool := &mockTool{
			name:        "serial_tool",
			callFunc:    trackConcurrency,
			annotations: &ToolAnnotations{SequentialOnlyHint: true},
		}

		agent, err := NewAgent(AgentOptions{
			Model:                 mock,
			Tools:                 []Tool{parallelTool, serialTool},
			ParallelToolExecution: true,
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use tools"))
		assert.NoError(t, err)
		assert.Equal(t, resp.OutputText(), "Done")
		assert.Len(t, resp.ToolCallResults(), 3)

		assert.Equal(t, int32(1), maxConcurrent.Load(),
			"sequential-only tool in batch should force sequential execution, max concurrent was %d", maxConcurrent.Load())
	})

	t.Run("no hint preserves parallel execution", func(t *testing.T) {
		callCount := 0
		var concurrent atomic.Int32
		var maxConcurrent atomic.Int32

		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				callCount++
				if callCount == 1 {
					return &llm.Response{
						ID:    "resp_1",
						Model: "test-model",
						Role:  llm.Assistant,
						Content: []llm.Content{
							&llm.ToolUseContent{ID: "t1", Name: "tool_a", Input: []byte(`{}`)},
							&llm.ToolUseContent{ID: "t2", Name: "tool_a", Input: []byte(`{}`)},
							&llm.ToolUseContent{ID: "t3", Name: "tool_a", Input: []byte(`{}`)},
						},
						Type:       "message",
						StopReason: "tool_use",
						Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
					}, nil
				}
				return &llm.Response{
					ID:         "resp_2",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		tool := &mockTool{
			name: "tool_a",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				cur := concurrent.Add(1)
				defer concurrent.Add(-1)
				for {
					old := maxConcurrent.Load()
					if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				return NewToolResultText("done"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model:                 mock,
			Tools:                 []Tool{tool},
			ParallelToolExecution: true,
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use tools"))
		assert.NoError(t, err)
		assert.Equal(t, resp.OutputText(), "Done")
		assert.True(t, maxConcurrent.Load() > 1,
			"expected concurrent execution without the hint, max concurrent was %d", maxConcurrent.Load())
	})
}

// mockExtension implements Extension for testing.
type mockExtension struct {
	tools []Tool
	hooks Hooks
	rules string
}

func (e *mockExtension) Tools() []Tool { return e.tools }
func (e *mockExtension) Hooks() Hooks  { return e.hooks }
func (e *mockExtension) Rules() string { return e.rules }

func TestExtensionMerge(t *testing.T) {
	mock := &mockLLM{nameFunc: func() string { return "test-model" }}

	t.Run("merges tools from extensions", func(t *testing.T) {
		directTool := &mockTool{
			name:     "direct_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
		}
		extTool := &mockTool{
			name:     "ext_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
		}

		agent, err := NewAgent(AgentOptions{
			Model: mock,
			Tools: []Tool{directTool},
			Extensions: []Extension{
				&mockExtension{tools: []Tool{extTool}},
			},
		})
		assert.NoError(t, err)
		assert.Len(t, agent.Tools(), 2)
		assert.Equal(t, "direct_tool", agent.Tools()[0].Name())
		assert.Equal(t, "ext_tool", agent.Tools()[1].Name())
	})

	t.Run("merges hooks from extensions", func(t *testing.T) {
		var order []string
		agent, err := NewAgent(AgentOptions{
			Model: mock,
			Hooks: Hooks{
				PreGeneration: []PreGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						order = append(order, "direct")
						return nil
					},
				},
			},
			Extensions: []Extension{
				&mockExtension{hooks: Hooks{
					PreGeneration: []PreGenerationHook{
						func(ctx context.Context, hctx *HookContext) error {
							order = append(order, "ext")
							return nil
						},
					},
				}},
			},
		})
		assert.NoError(t, err)

		// Run PreGeneration hooks to verify merge order
		for _, hook := range agent.hooks.PreGeneration {
			hook(context.Background(), &HookContext{})
		}
		assert.Equal(t, []string{"direct", "ext"}, order)
	})

	t.Run("merges rules into system prompt", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Model:        mock,
			SystemPrompt: "Base prompt.",
			Extensions: []Extension{
				&mockExtension{rules: "Extension rules."},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, "Base prompt.\n\nExtension rules.\n\n"+reminderPrimingRule, agent.SystemPrompt())
	})

	t.Run("empty rules not appended", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Model:        mock,
			SystemPrompt: "Base prompt.",
			Extensions: []Extension{
				&mockExtension{rules: ""},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, "Base prompt.\n\n"+reminderPrimingRule, agent.SystemPrompt())
	})

	t.Run("nil extensions skipped", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Model:      mock,
			Extensions: []Extension{nil},
		})
		assert.NoError(t, err)
		assert.Len(t, agent.Tools(), 0)
	})

	t.Run("multiple extensions merged in order", func(t *testing.T) {
		ext1Tool := &mockTool{
			name:     "tool_1",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
		}
		ext2Tool := &mockTool{
			name:     "tool_2",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
		}

		agent, err := NewAgent(AgentOptions{
			Model: mock,
			Extensions: []Extension{
				&mockExtension{tools: []Tool{ext1Tool}, rules: "Rules 1."},
				&mockExtension{tools: []Tool{ext2Tool}, rules: "Rules 2."},
			},
		})
		assert.NoError(t, err)
		assert.Len(t, agent.Tools(), 2)
		assert.Equal(t, "tool_1", agent.Tools()[0].Name())
		assert.Equal(t, "tool_2", agent.Tools()[1].Name())
		assert.Contains(t, agent.SystemPrompt(), "Rules 1.\n\nRules 2.")
	})

	t.Run("duplicate tool from extension rejected", func(t *testing.T) {
		tool := &mockTool{
			name:     "same_name",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
		}

		_, err := NewAgent(AgentOptions{
			Model: mock,
			Tools: []Tool{tool},
			Extensions: []Extension{
				&mockExtension{tools: []Tool{tool}},
			},
		})
		assert.Error(t, err)
		assert.ErrorContains(t, err, `duplicate tool name: "same_name"`)
	})

	t.Run("merges every hook type from extensions", func(t *testing.T) {
		// One hook of each type — if a new hook slice is added to Hooks
		// without a matching line in the NewAgent extension merge, this
		// test catches the omission.
		extHooks := Hooks{
			SessionStart: []SessionStartHook{
				func(ctx context.Context, hctx *HookContext) (*SessionStartResult, error) { return nil, nil },
			},
			PreGeneration: []PreGenerationHook{
				func(ctx context.Context, hctx *HookContext) error { return nil },
			},
			PostGeneration: []PostGenerationHook{
				func(ctx context.Context, hctx *HookContext) error { return nil },
			},
			PreToolUse: []PreToolUseHook{
				func(ctx context.Context, hctx *HookContext) error { return nil },
			},
			PostToolUse: []PostToolUseHook{
				func(ctx context.Context, hctx *HookContext) error { return nil },
			},
			PostToolUseFailure: []PostToolUseFailureHook{
				func(ctx context.Context, hctx *HookContext) error { return nil },
			},
			Stop: []StopHook{
				func(ctx context.Context, hctx *HookContext) (*StopDecision, error) { return nil, nil },
			},
			PreIteration: []PreIterationHook{
				func(ctx context.Context, hctx *HookContext) error { return nil },
			},
			OnSuspend: []OnSuspendHook{
				func(ctx context.Context, hctx *HookContext) error { return nil },
			},
			PostBackgroundToolUse: []PostBackgroundToolUseHook{
				func(ctx context.Context, hctx *HookContext) error { return nil },
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model:      mock,
			Extensions: []Extension{&mockExtension{hooks: extHooks}},
		})
		assert.NoError(t, err)
		assert.Len(t, agent.hooks.SessionStart, 1)
		assert.Len(t, agent.hooks.PreGeneration, 1)
		assert.Len(t, agent.hooks.PostGeneration, 1)
		assert.Len(t, agent.hooks.PreToolUse, 1)
		assert.Len(t, agent.hooks.PostToolUse, 1)
		assert.Len(t, agent.hooks.PostToolUseFailure, 1)
		assert.Len(t, agent.hooks.Stop, 1)
		assert.Len(t, agent.hooks.PreIteration, 1)
		assert.Len(t, agent.hooks.OnSuspend, 1)
		assert.Len(t, agent.hooks.PostBackgroundToolUse, 1)
	})
}

// TestParallelToolStreaming_NoDataRace verifies that the response item
// accumulator is synchronized when parallel tools stream output from their
// own goroutines while the drain loop processes completions on the main
// goroutine. Run with -race to catch regressions.
func TestParallelToolStreaming_NoDataRace(t *testing.T) {
	const chunksPerTool = 50

	// Both tools rendezvous before streaming so their StreamOutput calls
	// are guaranteed to overlap.
	var ready sync.WaitGroup
	ready.Add(2)
	makeStreamTool := func(name string) Tool {
		return &mockTool{
			name: name,
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				ready.Done()
				ready.Wait()
				for i := 0; i < chunksPerTool; i++ {
					StreamOutput(ctx, "chunk")
				}
				return NewToolResultText(name + " done"), nil
			},
		}
	}

	callCount := 0
	mock := &mockLLM{
		nameFunc: func() string { return "test-model" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			callCount++
			if callCount == 1 {
				return &llm.Response{
					ID:    "resp_1",
					Model: "test-model",
					Role:  llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{ID: "t1", Name: "stream_a", Input: []byte(`{}`)},
						&llm.ToolUseContent{ID: "t2", Name: "stream_b", Input: []byte(`{}`)},
					},
					Type:       "message",
					StopReason: "tool_use",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			}
			return &llm.Response{
				ID:         "resp_2",
				Model:      "test-model",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				Type:       "message",
				StopReason: "stop",
				Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
			}, nil
		},
	}

	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{makeStreamTool("stream_a"), makeStreamTool("stream_b")},
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("stream both"))
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "Done")

	// All stream events from both tool goroutines must be collected.
	streamItems := 0
	for _, item := range resp.Items {
		if item.Type == ResponseItemTypeToolStream {
			streamItems++
		}
	}
	assert.Equal(t, streamItems, 2*chunksPerTool)
}
