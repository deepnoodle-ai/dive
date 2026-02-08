package dive

import (
	"context"
	"errors"
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

	t.Run("NewGenerationState returns HookContext for compat", func(t *testing.T) {
		state := NewGenerationState()
		assert.NotNil(t, state.Values)
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
