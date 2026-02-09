package dive

import (
	"context"
	"fmt"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestToolset(t *testing.T) {
	t.Run("dynamic tools are available to LLM", func(t *testing.T) {
		callCount := 0
		var capturedTools []llm.Tool
		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				callCount++
				var config llm.Config
				config.Apply(opts...)
				capturedTools = config.Tools
				if callCount == 1 {
					return &llm.Response{
						ID:    "resp_1",
						Model: "test-model",
						Role:  llm.Assistant,
						Content: []llm.Content{
							&llm.ToolUseContent{ID: "t1", Name: "dynamic_tool", Input: []byte(`{}`)},
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

		dynamicTool := &mockTool{
			name: "dynamic_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				return NewToolResultText("dynamic result"), nil
			},
		}

		ts := &ToolsetFunc{
			ToolsetName: "test-toolset",
			Resolve: func(ctx context.Context) ([]Tool, error) {
				return []Tool{dynamicTool}, nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model:    mock,
			Toolsets: []Toolset{ts},
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)
		assert.Equal(t, resp.OutputText(), "Done")

		// Verify the dynamic tool was included in LLM tools
		assert.True(t, len(capturedTools) > 0, "expected dynamic tools in LLM request")
	})

	t.Run("static and dynamic tools merged", func(t *testing.T) {
		var capturedTools []llm.Tool
		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				var config llm.Config
				config.Apply(opts...)
				capturedTools = config.Tools
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

		staticTool := &mockTool{
			name:     "static_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
		}

		dynamicTool := &mockTool{
			name:     "dynamic_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
		}

		ts := &ToolsetFunc{
			ToolsetName: "test-toolset",
			Resolve: func(ctx context.Context) ([]Tool, error) {
				return []Tool{dynamicTool}, nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model:    mock,
			Tools:    []Tool{staticTool},
			Toolsets: []Toolset{ts},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.NoError(t, err)
		assert.Equal(t, len(capturedTools), 2)
	})

	t.Run("toolset error surfaces", func(t *testing.T) {
		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
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

		ts := &ToolsetFunc{
			ToolsetName: "failing-toolset",
			Resolve: func(ctx context.Context) ([]Tool, error) {
				return nil, fmt.Errorf("toolset connection failed")
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model:    mock,
			Toolsets: []Toolset{ts},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.Error(t, err)
		assert.ErrorContains(t, err, "failing-toolset")
		assert.ErrorContains(t, err, "toolset connection failed")
	})

	t.Run("duplicate tool names across toolsets detected", func(t *testing.T) {
		mock := &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				return &llm.Response{
					ID: "resp_1", Model: "test-model", Role: llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		staticTool := &mockTool{
			name:     "same_name",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
		}

		ts := &ToolsetFunc{
			ToolsetName: "test-toolset",
			Resolve: func(ctx context.Context) ([]Tool, error) {
				return []Tool{&mockTool{
					name:     "same_name",
					callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return nil, nil },
				}}, nil
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model:    mock,
			Tools:    []Tool{staticTool},
			Toolsets: []Toolset{ts},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("Hello"))
		assert.Error(t, err)
		assert.ErrorContains(t, err, `duplicate tool name: "same_name"`)
	})

	t.Run("HasTools returns true with only toolsets", func(t *testing.T) {
		mock := &mockLLM{nameFunc: func() string { return "test-model" }}
		ts := &ToolsetFunc{
			ToolsetName: "test",
			Resolve:     func(ctx context.Context) ([]Tool, error) { return nil, nil },
		}
		agent, err := NewAgent(AgentOptions{
			Model:    mock,
			Toolsets: []Toolset{ts},
		})
		assert.NoError(t, err)
		assert.True(t, agent.HasTools())
	})
}

func TestToolsetFunc(t *testing.T) {
	ts := &ToolsetFunc{
		ToolsetName: "my-toolset",
		Resolve: func(ctx context.Context) ([]Tool, error) {
			return nil, nil
		},
	}
	assert.Equal(t, ts.Name(), "my-toolset")

	tools, err := ts.Tools(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, tools)
}
