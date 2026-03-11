package dive

import (
	"context"
	"fmt"
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
	name     string
	callFunc func(ctx context.Context, input any) (*ToolResult, error)
}

func (t *mockTool) Name() string                  { return t.name }
func (t *mockTool) Description() string           { return "mock tool" }
func (t *mockTool) Schema() *Schema               { return nil }
func (t *mockTool) Annotations() *ToolAnnotations { return nil }
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
		tool := &mockTool{
			name: "slow_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				cur := concurrent.Add(1)
				defer concurrent.Add(-1)
				// Track the max concurrency seen
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
			ParallelToolExecution: true,
		})
		assert.NoError(t, err)

		start := time.Now()
		resp, err := agent.CreateResponse(context.Background(), WithInput("Use tools"))
		elapsed := time.Since(start)
		assert.NoError(t, err)
		assert.Equal(t, resp.OutputText(), "Done")

		// All 3 tool results should be present
		results := resp.ToolCallResults()
		assert.Len(t, results, 3)

		// Verify actual concurrency: max concurrent should be > 1
		assert.True(t, maxConcurrent.Load() > 1,
			"expected concurrent execution, max concurrent was %d", maxConcurrent.Load())

		// Should complete in roughly 1 tool duration, not 3x
		assert.True(t, elapsed < 120*time.Millisecond,
			"expected parallel execution to be faster than sequential, took %s", elapsed)
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
