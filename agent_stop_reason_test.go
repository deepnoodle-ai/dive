package dive

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestResponseCarriesLastStopReason pins that Response.StopReason and
// StopDetails are those of the last model response of the call, not the
// first: a tool-use iteration followed by a refusal reports the refusal.
func TestResponseCarriesLastStopReason(t *testing.T) {
	calls := 0
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			calls++
			if calls == 1 {
				return &llm.Response{
					ID:         "r1",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.ToolUseContent{ID: "t1", Name: "lookup", Input: []byte(`{}`)}},
					StopReason: "tool_use",
				}, nil
			}
			return &llm.Response{
				ID:          "r2",
				Role:        llm.Assistant,
				Content:     []llm.Content{&llm.TextContent{Text: "I can't help with that."}},
				StopReason:  "refusal",
				StopDetails: &llm.StopDetails{Type: "cyber"},
			}, nil
		},
	}
	tool := &mockTool{name: "lookup", callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
		return NewToolResultText("found"), nil
	}}
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.NoError(t, err)
	assert.Equal(t, resp.StopReason, "refusal")
	assert.Equal(t, resp.StopDetails.Type, "cyber")
	assert.Equal(t, llm.ClassifyStopReason(resp.StopReason), llm.StopKindRefusal)
}
