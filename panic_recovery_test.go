package dive

import (
	"context"
	"fmt"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestPanicRecovery(t *testing.T) {
	t.Run("tool panic is recovered and returned as error result", func(t *testing.T) {
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
							&llm.ToolUseContent{ID: "t1", Name: "panicking_tool", Input: []byte(`{}`)},
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
					Content:    []llm.Content{&llm.TextContent{Text: "Recovered"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		tool := &mockTool{
			name: "panicking_tool",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				panic("something went terribly wrong")
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: mock,
			Tools: []Tool{tool},
		})
		assert.NoError(t, err)

		// Should NOT panic — the panic should be recovered
		resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)
		assert.Equal(t, resp.OutputText(), "Recovered")

		// Verify the tool result was an error
		results := resp.ToolCallResults()
		assert.Len(t, results, 1)
		assert.NotNil(t, results[0].Error)
		assert.True(t, results[0].Result.IsError)
		assert.Contains(t, results[0].Result.Content[0].Text, "panicked")
		assert.Contains(t, results[0].Result.Content[0].Text, "something went terribly wrong")
	})

	t.Run("error panic is recovered", func(t *testing.T) {
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
							&llm.ToolUseContent{ID: "t1", Name: "error_panicker", Input: []byte(`{}`)},
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
					Content:    []llm.Content{&llm.TextContent{Text: "OK"}},
					Type:       "message",
					StopReason: "stop",
					Usage:      llm.Usage{InputTokens: 15, OutputTokens: 3},
				}, nil
			},
			nameFunc: func() string { return "test-model" },
		}

		tool := &mockTool{
			name: "error_panicker",
			callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
				panic(fmt.Errorf("unexpected error"))
			},
		}

		agent, err := NewAgent(AgentOptions{
			Model: mock,
			Tools: []Tool{tool},
		})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("Use the tool"))
		assert.NoError(t, err)
		assert.Equal(t, resp.OutputText(), "OK")
	})
}
