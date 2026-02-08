package dive

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestPreGenerationHooks(t *testing.T) {
	t.Run("hooks can modify system prompt", func(t *testing.T) {
		var capturedSystemPrompt string

		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				// Extract system prompt from options
				var config llm.Config
				config.Apply(opts...)
				capturedSystemPrompt = config.SystemPrompt

				return &llm.Response{
					ID:         "resp_123",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
			Hooks: Hooks{
				PreGeneration: []PreGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						hctx.SystemPrompt = "Modified system prompt"
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		assert.Contains(t, capturedSystemPrompt, "Modified system prompt")
	})

	t.Run("hooks can prepend messages", func(t *testing.T) {
		var capturedMessages []*llm.Message

		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				var config llm.Config
				config.Apply(opts...)
				capturedMessages = config.Messages

				return &llm.Response{
					ID:         "resp_123",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
			Hooks: Hooks{
				PreGeneration: []PreGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						// Prepend a context message
						contextMsg := llm.NewUserTextMessage("Context: This is important info")
						hctx.Messages = append([]*llm.Message{contextMsg}, hctx.Messages...)
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		assert.Equal(t, 2, len(capturedMessages))
		assert.Equal(t, "Context: This is important info", capturedMessages[0].Text())
		assert.Equal(t, "Hello", capturedMessages[1].Text())
	})

	t.Run("hook errors abort generation", func(t *testing.T) {
		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				t.Fatal("generate should not be called when hook returns error")
				return nil, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
			Hooks: Hooks{
				PreGeneration: []PreGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						return errors.New("hook error")
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "hook error")
	})

	t.Run("multiple hooks run in order", func(t *testing.T) {
		var order []string

		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				return &llm.Response{
					ID:         "resp_123",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
			Hooks: Hooks{
				PreGeneration: []PreGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						order = append(order, "first")
						return nil
					},
					func(ctx context.Context, hctx *HookContext) error {
						order = append(order, "second")
						return nil
					},
					func(ctx context.Context, hctx *HookContext) error {
						order = append(order, "third")
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		assert.Equal(t, []string{"first", "second", "third"}, order)
	})
}

func TestPostGenerationHooks(t *testing.T) {
	t.Run("hooks receive response data", func(t *testing.T) {
		var capturedHctx *HookContext

		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				return &llm.Response{
					ID:         "resp_123",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
			Hooks: Hooks{
				PostGeneration: []PostGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						capturedHctx = hctx
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(),
			WithInput("Hello"),
		)
		assert.NoError(t, err)

		assert.NotNil(t, capturedHctx.Response)
		assert.NotNil(t, resp)
		assert.NotNil(t, capturedHctx.Usage)
		assert.Equal(t, 10, capturedHctx.Usage.InputTokens)
		assert.Equal(t, 5, capturedHctx.Usage.OutputTokens)
		assert.Equal(t, 1, len(capturedHctx.OutputMessages))
	})

	t.Run("hook errors are logged but don't affect response", func(t *testing.T) {
		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				return &llm.Response{
					ID:         "resp_123",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
			Hooks: Hooks{
				PostGeneration: []PostGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						return errors.New("post-hook error")
					},
				},
			},
		})
		assert.NoError(t, err)

		// Should succeed despite the hook error
		resp, err := agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("hooks can communicate via Values", func(t *testing.T) {
		var receivedValue string

		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				return &llm.Response{
					ID:         "resp_123",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
			Hooks: Hooks{
				PreGeneration: []PreGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						hctx.Values["shared_key"] = "shared_value"
						return nil
					},
				},
				PostGeneration: []PostGenerationHook{
					func(ctx context.Context, hctx *HookContext) error {
						if v, ok := hctx.Values["shared_key"].(string); ok {
							receivedValue = v
						}
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		assert.Equal(t, "shared_value", receivedValue)
	})
}

func TestInjectContext(t *testing.T) {
	t.Run("prepends content as user message", func(t *testing.T) {
		var capturedMessages []*llm.Message

		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				var config llm.Config
				config.Apply(opts...)
				capturedMessages = config.Messages

				return &llm.Response{
					ID:         "resp_123",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
			Hooks: Hooks{
				PreGeneration: []PreGenerationHook{
					InjectContext(
						&llm.TextContent{Text: "Context item 1"},
						&llm.TextContent{Text: "Context item 2"},
					),
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		assert.Equal(t, 2, len(capturedMessages))
		// First message is the injected context
		assert.Equal(t, llm.User, capturedMessages[0].Role)
		assert.Contains(t, capturedMessages[0].Text(), "Context item 1")
		// Second message is the user input
		assert.Equal(t, "Hello", capturedMessages[1].Text())
	})

	t.Run("empty content does nothing", func(t *testing.T) {
		var capturedMessages []*llm.Message

		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				var config llm.Config
				config.Apply(opts...)
				capturedMessages = config.Messages

				return &llm.Response{
					ID:         "resp_123",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
			Hooks: Hooks{
				PreGeneration: []PreGenerationHook{
					InjectContext(), // No content
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		// Should only have the user input message
		assert.Equal(t, 1, len(capturedMessages))
	})
}

func TestHookContext(t *testing.T) {
	t.Run("NewHookContext initializes Values map", func(t *testing.T) {
		hctx := NewHookContext()
		assert.NotNil(t, hctx.Values)
	})

}

func TestCompactionHook(t *testing.T) {
	t.Run("compacts when message count exceeds threshold", func(t *testing.T) {
		summarized := false
		summarizer := func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
			summarized = true
			return []*llm.Message{llm.NewUserTextMessage("Summary of " + msgs[0].Text())}, nil
		}

		hook := CompactionHook(3, summarizer)

		// Create state with messages above threshold
		hctx := NewHookContext()
		hctx.Messages = []*llm.Message{
			llm.NewUserTextMessage("Message 1"),
			llm.NewAssistantTextMessage("Response 1"),
			llm.NewUserTextMessage("Message 2"),
			llm.NewAssistantTextMessage("Response 2"),
		}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.True(t, summarized)
		assert.Equal(t, 1, len(hctx.Messages))
		assert.Contains(t, hctx.Messages[0].Text(), "Summary of Message 1")
	})

	t.Run("does not compact when message count below threshold", func(t *testing.T) {
		summarized := false
		summarizer := func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
			summarized = true
			return msgs, nil
		}

		hook := CompactionHook(10, summarizer)

		hctx := NewHookContext()
		hctx.Messages = []*llm.Message{
			llm.NewUserTextMessage("Message 1"),
			llm.NewAssistantTextMessage("Response 1"),
		}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.False(t, summarized)
		assert.Equal(t, 2, len(hctx.Messages))
	})

	t.Run("propagates summarizer errors", func(t *testing.T) {
		summarizer := func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
			return nil, errors.New("summarization failed")
		}

		hook := CompactionHook(1, summarizer)

		hctx := NewHookContext()
		hctx.Messages = []*llm.Message{
			llm.NewUserTextMessage("Message 1"),
			llm.NewAssistantTextMessage("Response 1"),
		}

		err := hook(context.Background(), hctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "summarization failed")
	})
}

func TestUsageLogger(t *testing.T) {
	t.Run("logs usage data", func(t *testing.T) {
		var loggedUsage *llm.Usage

		hook := UsageLogger(func(usage *llm.Usage) {
			loggedUsage = usage
		})

		hctx := NewHookContext()
		hctx.Usage = &llm.Usage{
			InputTokens:  100,
			OutputTokens: 50,
		}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.Equal(t, 100, loggedUsage.InputTokens)
		assert.Equal(t, 50, loggedUsage.OutputTokens)
	})

	t.Run("handles nil usage gracefully", func(t *testing.T) {
		called := false
		hook := UsageLogger(func(usage *llm.Usage) {
			called = true
		})

		hctx := NewHookContext()
		hctx.Usage = nil

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.False(t, called)
	})

	t.Run("handles nil log func gracefully", func(t *testing.T) {
		hook := UsageLogger(nil)

		hctx := NewHookContext()
		hctx.Usage = &llm.Usage{InputTokens: 100}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
	})
}

func TestUsageLoggerWithSlog(t *testing.T) {
	t.Run("logs with slog logger", func(t *testing.T) {
		// Use NullLogger to verify it doesn't panic
		hook := UsageLoggerWithSlog(&llm.NullLogger{})

		hctx := NewHookContext()
		hctx.Usage = &llm.Usage{
			InputTokens:              100,
			OutputTokens:             50,
			CacheCreationInputTokens: 10,
			CacheReadInputTokens:     20,
		}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
	})

	t.Run("handles nil usage gracefully", func(t *testing.T) {
		hook := UsageLoggerWithSlog(&llm.NullLogger{})

		hctx := NewHookContext()
		hctx.Usage = nil

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
	})

	t.Run("handles nil logger gracefully", func(t *testing.T) {
		hook := UsageLoggerWithSlog(nil)

		hctx := NewHookContext()
		hctx.Usage = &llm.Usage{InputTokens: 100}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
	})
}

func TestMatchTool(t *testing.T) {
	t.Run("runs hook when tool name matches", func(t *testing.T) {
		called := false
		hook := MatchTool("Bash", func(ctx context.Context, hctx *HookContext) error {
			called = true
			return nil
		})

		hctx := &HookContext{
			Tool: &mockTool{name: "Bash"},
		}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("skips hook when tool name does not match", func(t *testing.T) {
		called := false
		hook := MatchTool("Bash", func(ctx context.Context, hctx *HookContext) error {
			called = true
			return nil
		})

		hctx := &HookContext{
			Tool: &mockTool{name: "Read"},
		}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.False(t, called)
	})

	t.Run("supports regex patterns", func(t *testing.T) {
		called := false
		hook := MatchTool("Bash|Edit|Write", func(ctx context.Context, hctx *HookContext) error {
			called = true
			return nil
		})

		hctx := &HookContext{
			Tool: &mockTool{name: "Edit"},
		}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("handles nil tool", func(t *testing.T) {
		called := false
		hook := MatchTool("Bash", func(ctx context.Context, hctx *HookContext) error {
			called = true
			return nil
		})

		hctx := &HookContext{}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.False(t, called)
	})
}

func TestMatchToolPost(t *testing.T) {
	t.Run("runs hook when tool name matches", func(t *testing.T) {
		called := false
		hook := MatchToolPost("Bash", func(ctx context.Context, hctx *HookContext) error {
			called = true
			return nil
		})

		hctx := &HookContext{
			Tool: &mockTool{name: "Bash"},
		}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("skips hook when tool name does not match", func(t *testing.T) {
		called := false
		hook := MatchToolPost("Bash", func(ctx context.Context, hctx *HookContext) error {
			called = true
			return nil
		})

		hctx := &HookContext{
			Tool: &mockTool{name: "Read"},
		}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.False(t, called)
	})
}

func TestMatchToolPostFailure(t *testing.T) {
	t.Run("runs hook when tool name matches", func(t *testing.T) {
		called := false
		hook := MatchToolPostFailure("Bash", func(ctx context.Context, hctx *HookContext) error {
			called = true
			return nil
		})

		hctx := &HookContext{
			Tool: &mockTool{name: "Bash"},
		}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("skips hook when tool name does not match", func(t *testing.T) {
		called := false
		hook := MatchToolPostFailure("Bash", func(ctx context.Context, hctx *HookContext) error {
			called = true
			return nil
		})

		hctx := &HookContext{
			Tool: &mockTool{name: "Read"},
		}

		err := hook(context.Background(), hctx)
		assert.NoError(t, err)
		assert.False(t, called)
	})
}

func TestStopDecision(t *testing.T) {
	t.Run("stop hook can continue generation", func(t *testing.T) {
		callCount := 0
		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				callCount++
				return &llm.Response{
					ID:         "resp_" + string(rune('0'+callCount)),
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response " + string(rune('0'+callCount))}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		stopCalls := 0
		agent, err := NewAgent(AgentOptions{
			Model: mockLLM,
			Hooks: Hooks{
				Stop: []StopHook{
					func(ctx context.Context, hctx *HookContext) (*StopDecision, error) {
						stopCalls++
						if stopCalls == 1 {
							return &StopDecision{
								Continue: true,
								Reason:   "Keep working",
							}, nil
						}
						return &StopDecision{Continue: false}, nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		assert.Equal(t, 2, callCount) // LLM called twice (initial + continuation)
		assert.Equal(t, 2, stopCalls)
	})

	t.Run("stop hook receives StopHookActive on continuation", func(t *testing.T) {
		callCount := 0
		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				callCount++
				return &llm.Response{
					ID:         "resp_1",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		var receivedActive []bool
		agent, err := NewAgent(AgentOptions{
			Model: mockLLM,
			Hooks: Hooks{
				Stop: []StopHook{
					func(ctx context.Context, hctx *HookContext) (*StopDecision, error) {
						receivedActive = append(receivedActive, hctx.StopHookActive)
						if !hctx.StopHookActive {
							return &StopDecision{Continue: true, Reason: "continue"}, nil
						}
						return &StopDecision{Continue: false}, nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		assert.Equal(t, []bool{false, true}, receivedActive)
	})
}

func TestPreIterationHook(t *testing.T) {
	t.Run("runs before each LLM call with iteration count", func(t *testing.T) {
		callCount := 0
		mockLLM := &mockLLM{
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

		var iterations []int
		agent, err := NewAgent(AgentOptions{
			Model: mockLLM,
			Tools: []Tool{tool},
			Hooks: Hooks{
				PreIteration: []PreIterationHook{
					func(ctx context.Context, hctx *HookContext) error {
						iterations = append(iterations, hctx.Iteration)
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)

		assert.Equal(t, []int{0, 1}, iterations)
	})

	t.Run("can modify system prompt per iteration", func(t *testing.T) {
		var capturedPrompts []string

		mockLLM := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				var config llm.Config
				config.Apply(opts...)
				capturedPrompts = append(capturedPrompts, config.SystemPrompt)

				return &llm.Response{
					ID:         "resp_1",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			SystemPrompt: "Original prompt",
			Model:        mockLLM,
			Hooks: Hooks{
				PreIteration: []PreIterationHook{
					func(ctx context.Context, hctx *HookContext) error {
						hctx.SystemPrompt = "Modified prompt"
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		assert.Equal(t, 1, len(capturedPrompts))
		assert.Equal(t, "Modified prompt", capturedPrompts[0])
	})
}

// newToolCallingMockLLM returns a mockLLM that issues one tool call on the
// first Generate call and a final text response on the second.
func newToolCallingMockLLM(toolName string) *mockLLM {
	callCount := 0
	return &mockLLM{
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
							Name:  toolName,
							Input: []byte(`{"key":"value"}`),
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
}

func TestPreToolUseHookIntegration(t *testing.T) {
	t.Run("hook denial prevents tool execution", func(t *testing.T) {
		toolCalled := false
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				toolCalled = true
				return NewToolResultText("tool output"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PreToolUse: []PreToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						return fmt.Errorf("tool %s not allowed", hctx.Tool.Name())
					},
				},
			},
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)
		assert.False(t, toolCalled, "tool should not have been called")
		assert.NotNil(t, resp)
	})

	t.Run("HookAbortError aborts generation", func(t *testing.T) {
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return NewToolResultText("tool output"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PreToolUse: []PreToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						return AbortGeneration("safety violation")
					},
				},
			},
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.Error(t, err)
		assert.Nil(t, resp)

		var abortErr *HookAbortError
		assert.ErrorAs(t, err, &abortErr)
		assert.Equal(t, "PreToolUse", abortErr.HookType)
		assert.Equal(t, "safety violation", abortErr.Reason)
	})

	t.Run("UpdatedInput rewrites tool arguments", func(t *testing.T) {
		var receivedInput any
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				receivedInput = input
				return NewToolResultText("tool output"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PreToolUse: []PreToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						hctx.UpdatedInput = []byte(`{"rewritten":"true"}`)
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)

		assert.NotNil(t, receivedInput)
		assert.Equal(t, `{"rewritten":"true"}`, string(receivedInput.([]byte)))
	})

	t.Run("AdditionalContext injected into tool result", func(t *testing.T) {
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return NewToolResultText("tool output"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PreToolUse: []PreToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						hctx.AdditionalContext = "pre-hook context"
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)

		// Find the tool call result in response items
		for _, item := range resp.Items {
			if item.Type == ResponseItemTypeToolCallResult {
				assert.Equal(t, "pre-hook context", item.ToolCallResult.AdditionalContext)
				return
			}
		}
		t.Fatal("expected to find a tool call result item")
	})
}

func TestPostToolUseHookIntegration(t *testing.T) {
	t.Run("hook runs after successful tool call", func(t *testing.T) {
		var capturedToolName string
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return NewToolResultText("tool output"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PostToolUse: []PostToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						capturedToolName = hctx.Tool.Name()
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)
		assert.Equal(t, "test_tool", capturedToolName)
	})

	t.Run("HookAbortError aborts generation", func(t *testing.T) {
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return NewToolResultText("tool output"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PostToolUse: []PostToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						return AbortGeneration("critical failure")
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.Error(t, err)

		var abortErr *HookAbortError
		assert.ErrorAs(t, err, &abortErr)
		assert.Equal(t, "PostToolUse", abortErr.HookType)
	})

	t.Run("AdditionalContext injected from post-hook", func(t *testing.T) {
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return NewToolResultText("tool output"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PostToolUse: []PostToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						hctx.AdditionalContext = "post-hook context"
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)

		for _, item := range resp.Items {
			if item.Type == ResponseItemTypeToolCallResult {
				assert.Equal(t, "post-hook context", item.ToolCallResult.AdditionalContext)
				return
			}
		}
		t.Fatal("expected to find a tool call result item")
	})
}

func TestPostToolUseFailureHookIntegration(t *testing.T) {
	t.Run("fires when tool returns error", func(t *testing.T) {
		var failureHookCalled bool
		var postHookCalled bool
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return nil, fmt.Errorf("tool crashed")
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PostToolUse: []PostToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						postHookCalled = true
						return nil
					},
				},
				PostToolUseFailure: []PostToolUseFailureHook{
					func(ctx context.Context, hctx *HookContext) error {
						failureHookCalled = true
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)
		assert.True(t, failureHookCalled, "PostToolUseFailure hook should have fired")
		assert.False(t, postHookCalled, "PostToolUse hook should NOT have fired")
	})

	t.Run("fires when tool result has IsError", func(t *testing.T) {
		var failureHookCalled bool
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return &ToolResult{
					Content: []*ToolResultContent{{
						Type: ToolResultContentTypeText,
						Text: "error occurred",
					}},
					IsError: true,
				}, nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PostToolUseFailure: []PostToolUseFailureHook{
					func(ctx context.Context, hctx *HookContext) error {
						failureHookCalled = true
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)
		assert.True(t, failureHookCalled, "PostToolUseFailure hook should fire for IsError results")
	})

	t.Run("does not fire on success", func(t *testing.T) {
		var failureHookCalled bool
		var postHookCalled bool
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return NewToolResultText("success"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PostToolUse: []PostToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						postHookCalled = true
						return nil
					},
				},
				PostToolUseFailure: []PostToolUseFailureHook{
					func(ctx context.Context, hctx *HookContext) error {
						failureHookCalled = true
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)
		assert.True(t, postHookCalled, "PostToolUse hook should fire on success")
		assert.False(t, failureHookCalled, "PostToolUseFailure hook should NOT fire on success")
	})

	t.Run("HookAbortError aborts generation", func(t *testing.T) {
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return nil, fmt.Errorf("tool crashed")
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PostToolUseFailure: []PostToolUseFailureHook{
					func(ctx context.Context, hctx *HookContext) error {
						return AbortGeneration("critical tool failure")
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.Error(t, err)

		var abortErr *HookAbortError
		assert.ErrorAs(t, err, &abortErr)
		assert.Equal(t, "PostToolUseFailure", abortErr.HookType)
	})
}

func TestAdditionalContextAccumulation(t *testing.T) {
	t.Run("pre and post context merged with newline", func(t *testing.T) {
		tool := &mockTool{
			name: "test_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return NewToolResultText("tool output"), nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: newToolCallingMockLLM("test_tool"),
			Tools: []Tool{tool},
			Hooks: Hooks{
				PreToolUse: []PreToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						hctx.AdditionalContext = "from pre-hook"
						return nil
					},
				},
				PostToolUse: []PostToolUseHook{
					func(ctx context.Context, hctx *HookContext) error {
						hctx.AdditionalContext = "from post-hook"
						return nil
					},
				},
			},
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)

		for _, item := range resp.Items {
			if item.Type == ResponseItemTypeToolCallResult {
				assert.Equal(t, "from pre-hook\nfrom post-hook", item.ToolCallResult.AdditionalContext)
				return
			}
		}
		t.Fatal("expected to find a tool call result item")
	})
}

func TestMatchToolInvalidRegex(t *testing.T) {
	t.Run("MatchTool returns error for invalid pattern", func(t *testing.T) {
		hook := MatchTool("[invalid", func(ctx context.Context, hctx *HookContext) error {
			t.Fatal("inner hook should not be called")
			return nil
		})

		hctx := &HookContext{Tool: &mockTool{name: "Bash"}}
		err := hook(context.Background(), hctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "error parsing regexp")
	})

	t.Run("MatchToolPost returns error for invalid pattern", func(t *testing.T) {
		hook := MatchToolPost("[invalid", func(ctx context.Context, hctx *HookContext) error {
			t.Fatal("inner hook should not be called")
			return nil
		})

		hctx := &HookContext{Tool: &mockTool{name: "Bash"}}
		err := hook(context.Background(), hctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "error parsing regexp")
	})

	t.Run("MatchToolPostFailure returns error for invalid pattern", func(t *testing.T) {
		hook := MatchToolPostFailure("[invalid", func(ctx context.Context, hctx *HookContext) error {
			t.Fatal("inner hook should not be called")
			return nil
		})

		hctx := &HookContext{Tool: &mockTool{name: "Bash"}}
		err := hook(context.Background(), hctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "error parsing regexp")
	})
}

func TestUserFeedbackError(t *testing.T) {
	t.Run("NewUserFeedback creates error with message", func(t *testing.T) {
		err := NewUserFeedback("please use a different approach")
		assert.Error(t, err)
		assert.Equal(t, "please use a different approach", err.Error())
	})

	t.Run("IsUserFeedback returns feedback for UserFeedbackError", func(t *testing.T) {
		err := NewUserFeedback("try again")
		feedback, ok := IsUserFeedback(err)
		assert.True(t, ok)
		assert.Equal(t, "try again", feedback)
	})

	t.Run("IsUserFeedback returns false for regular errors", func(t *testing.T) {
		err := errors.New("some error")
		feedback, ok := IsUserFeedback(err)
		assert.False(t, ok)
		assert.Equal(t, "", feedback)
	})

	t.Run("IsUserFeedback returns false for nil", func(t *testing.T) {
		feedback, ok := IsUserFeedback(nil)
		assert.False(t, ok)
		assert.Equal(t, "", feedback)
	})
}

func TestStopHookErrors(t *testing.T) {
	simpleMockLLM := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			return &llm.Response{
				ID:         "resp_1",
				Model:      "test-model",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
				Type:       "message",
				StopReason: "stop",
				Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
			}, nil
		},
		nameFunc: func() string { return "test-model" },
	}

	t.Run("HookAbortError aborts generation", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Model: simpleMockLLM,
			Hooks: Hooks{
				Stop: []StopHook{
					func(ctx context.Context, hctx *HookContext) (*StopDecision, error) {
						return nil, AbortGeneration("stop abort")
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.Error(t, err)

		var abortErr *HookAbortError
		assert.ErrorAs(t, err, &abortErr)
		assert.Equal(t, "Stop", abortErr.HookType)
	})

	t.Run("regular error is logged and generation completes", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Model: simpleMockLLM,
			Hooks: Hooks{
				Stop: []StopHook{
					func(ctx context.Context, hctx *HookContext) (*StopDecision, error) {
						return nil, errors.New("transient stop error")
					},
				},
			},
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "Response", resp.OutputText())
	})
}

func TestPreIterationHookErrors(t *testing.T) {
	t.Run("error aborts generation", func(t *testing.T) {
		llmCalled := false
		mockModel := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				llmCalled = true
				return &llm.Response{
					ID:         "resp_1",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			Model: mockModel,
			Hooks: Hooks{
				PreIteration: []PreIterationHook{
					func(ctx context.Context, hctx *HookContext) error {
						return errors.New("iteration blocked")
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.Error(t, err)
		assert.ErrorContains(t, err, "pre-iteration hook error: iteration blocked")
		assert.False(t, llmCalled, "LLM should not have been called")
	})

	t.Run("HookAbortError wraps through pre-iteration error", func(t *testing.T) {
		mockModel := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				return &llm.Response{
					ID:         "resp_1",
					Model:      "test-model",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Response"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		agent, err := NewAgent(AgentOptions{
			Model: mockModel,
			Hooks: Hooks{
				PreIteration: []PreIterationHook{
					func(ctx context.Context, hctx *HookContext) error {
						return AbortGeneration("abort from pre-iteration")
					},
				},
			},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.Error(t, err)

		// HookAbortError should be unwrappable through the wrapping
		var abortErr *HookAbortError
		assert.ErrorAs(t, err, &abortErr)
		assert.Equal(t, "abort from pre-iteration", abortErr.Reason)
	})
}
