package dive

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// instructingTool is a tool that implements SystemInstructor.
type instructingTool struct {
	mockTool
	instructions string
}

func (t *instructingTool) SystemInstructions() string {
	return t.instructions
}

func TestSystemInstructor(t *testing.T) {
	t.Run("tool instructions appended to system prompt", func(t *testing.T) {
		var capturedSystemPrompt string
		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				var config llm.Config
				config.Apply(opts...)
				capturedSystemPrompt = config.SystemPrompt
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

		tool := &instructingTool{
			mockTool: mockTool{
				name: "memory_tool",
				callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
					return NewToolResultText("ok"), nil
				},
			},
			instructions: "You have access to memory. Use the memory_tool to search past conversations.",
		}

		agent, err := NewAgent(AgentOptions{
			SystemPrompt: "You are a helpful assistant.",
			Model:        mock,
			Tools:        []Tool{tool},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		assert.Contains(t, capturedSystemPrompt, "You are a helpful assistant.")
		assert.Contains(t, capturedSystemPrompt, "You have access to memory")
	})

	t.Run("multiple tool instructions are all included", func(t *testing.T) {
		var capturedSystemPrompt string
		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				var config llm.Config
				config.Apply(opts...)
				capturedSystemPrompt = config.SystemPrompt
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

		tool1 := &instructingTool{
			mockTool: mockTool{
				name:     "tool_a",
				callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
			},
			instructions: "Instructions for tool A.",
		}
		tool2 := &instructingTool{
			mockTool: mockTool{
				name:     "tool_b",
				callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
			},
			instructions: "Instructions for tool B.",
		}

		agent, err := NewAgent(AgentOptions{
			Model: mock,
			Tools: []Tool{tool1, tool2},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		assert.Contains(t, capturedSystemPrompt, "Instructions for tool A.")
		assert.Contains(t, capturedSystemPrompt, "Instructions for tool B.")
	})

	t.Run("empty instructions are skipped", func(t *testing.T) {
		var capturedSystemPrompt string
		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				var config llm.Config
				config.Apply(opts...)
				capturedSystemPrompt = config.SystemPrompt
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

		tool := &instructingTool{
			mockTool: mockTool{
				name:     "quiet_tool",
				callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
			},
			instructions: "", // empty
		}

		agent, err := NewAgent(AgentOptions{
			SystemPrompt: "Base prompt",
			Model:        mock,
			Tools:        []Tool{tool},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		// System prompt should just be the base, no extra newlines
		assert.Equal(t, capturedSystemPrompt, "Base prompt")
	})

	t.Run("instructions from toolset tools are included", func(t *testing.T) {
		var capturedSystemPrompt string
		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				var config llm.Config
				config.Apply(opts...)
				capturedSystemPrompt = config.SystemPrompt
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

		dynamicTool := &instructingTool{
			mockTool: mockTool{
				name:     "dynamic_instructor",
				callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
			},
			instructions: "Dynamic tool instructions here.",
		}

		ts := &ToolsetFunc{
			ToolsetName: "test-toolset",
			Resolve: func(ctx context.Context) ([]Tool, error) {
				return []Tool{dynamicTool}, nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			SystemPrompt: "Base",
			Model:        mock,
			Toolsets:     []Toolset{ts},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)

		assert.Contains(t, capturedSystemPrompt, "Dynamic tool instructions here.")
	})
}
