package anthropic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestComputerToolsetDeclaration(t *testing.T) {
	_, captured := generateCaptured(t, okResponse, nil,
		llm.WithModel(ModelClaudeOpus55),
		llm.WithTools(NewComputerToolset(ComputerToolsetOptions{Disabled: []string{"zoom"}})),
		llm.WithMessages(llm.NewUserTextMessage("Open the display settings.")),
	)
	assert.Equal(t, []any{map[string]any{
		"type":    "computer_toolset_20260801",
		"configs": map[string]any{"zoom": map[string]any{"enabled": false}},
	}}, captured.body["tools"])
	// The toolset needs no beta header, unlike computer_20251124.
	assert.Len(t, captured.betas, 0)
}

func TestComputerToolsetDefaultsSendTypeOnly(t *testing.T) {
	config := NewComputerToolset(ComputerToolsetOptions{}).ToolConfiguration(ProviderName)
	assert.Equal(t, map[string]any{"type": ComputerToolsetType}, config)
}

func TestComputerToolsetCallsDecodeToolsetName(t *testing.T) {
	response := `{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"content": [
			{"type": "tool_use", "id": "toolu_1", "name": "left_click", "toolset_name": "computer", "input": {"coordinate": [640, 60]}},
			{"type": "tool_use", "id": "toolu_2", "name": "screenshot", "toolset_name": "computer", "input": {}}
		],
		"stop_reason": "tool_use", "usage": {"input_tokens": 10, "output_tokens": 5}
	}`
	resp, _ := generateCaptured(t, response, nil,
		llm.WithModel(ModelClaudeOpus55),
		llm.WithTools(NewComputerToolset(ComputerToolsetOptions{})),
		llm.WithMessages(llm.NewUserTextMessage("Search for cats.")),
	)
	calls := resp.ToolCalls()
	assert.Len(t, calls, 2)
	assert.Equal(t, "left_click", calls[0].Name)
	assert.Equal(t, ComputerToolsetName, calls[0].ToolsetName)
	assert.Equal(t, ComputerToolsetName, resp.Content[1].(*llm.ToolUseContent).ToolsetName)
}

// Every result for a member call must echo toolset_name, or the API rejects
// the request. Results built without it get it from the matching call; a
// result for an ordinary tool gets none.
func TestComputerToolsetResultsGetToolsetName(t *testing.T) {
	_, captured := generateCaptured(t, okResponse, nil,
		llm.WithModel(ModelClaudeOpus55),
		llm.WithTools(NewComputerToolset(ComputerToolsetOptions{})),
		llm.WithMessages(
			llm.NewUserTextMessage("Search for cats."),
			llm.NewAssistantMessage(
				&llm.ToolUseContent{ID: "toolu_1", Name: "left_click", ToolsetName: "computer", Input: json.RawMessage(`{"coordinate":[640,60]}`)},
				&llm.ToolUseContent{ID: "toolu_2", Name: "lookup", Input: json.RawMessage(`{}`)},
			),
			llm.NewToolResultMessage(
				&llm.ToolResultContent{ToolUseID: "toolu_1", Content: "OK"},
				&llm.ToolResultContent{ToolUseID: "toolu_2", Content: "found"},
			),
		),
	)
	messages := captured.body["messages"].([]any)
	calls := messages[1].(map[string]any)["content"].([]any)
	assert.Equal(t, "computer", calls[0].(map[string]any)["toolset_name"])
	_, hasToolset := calls[1].(map[string]any)["toolset_name"]
	assert.False(t, hasToolset)

	results := messages[2].(map[string]any)["content"].([]any)
	assert.Equal(t, "computer", results[0].(map[string]any)["toolset_name"])
	_, hasToolset = results[1].(map[string]any)["toolset_name"]
	assert.False(t, hasToolset)
}

func TestComputerToolsetStreamedCallKeepsToolsetName(t *testing.T) {
	stream := `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"type","toolset_name":"computer","input":{}}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"text\":\"cats\"}"}}

data: {"type":"content_block_stop","index":0}

data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}

data: {"type":"message_stop"}

`
	server, _ := capturingServer(t, "text/event-stream", stream)
	provider := New(WithEndpoint(server.URL), WithAPIKey("test-key"))
	iterator, err := provider.Stream(context.Background(),
		llm.WithModel(ModelClaudeOpus55),
		llm.WithTools(NewComputerToolset(ComputerToolsetOptions{})),
		llm.WithMessages(llm.NewUserTextMessage("Search for cats.")),
	)
	assert.NoError(t, err)
	defer iterator.Close()

	calls := consumeAnthropicStream(t, iterator).Response().ToolCalls()
	assert.Len(t, calls, 1)
	assert.Equal(t, "type", calls[0].Name)
	assert.Equal(t, ComputerToolsetName, calls[0].ToolsetName)
	assert.Equal(t, `{"text":"cats"}`, string(calls[0].Input))
}
