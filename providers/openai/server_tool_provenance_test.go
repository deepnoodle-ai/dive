package openai

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go/v3/responses"
)

// Anthropic MCP connector blocks and web fetch results in the history are
// skipped: their mcptoolu_/srvtoolu_ IDs are not OpenAI item IDs.
func TestEncodeAssistantMessageSkipsForeignMCPBlocks(t *testing.T) {
	items, err := encodeAssistantMessage(&llm.Message{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.TextContent{Text: "Asking the server."},
			&llm.MCPToolUseContent{ID: "mcptoolu_1", Name: "echo", ServerName: "example", Input: []byte(`{}`)},
			&llm.MCPToolResultContent{ToolUseID: "mcptoolu_1", Content: []*llm.ContentChunk{{Type: "text", Text: "hi"}}},
			&llm.ServerToolUseContent{ID: "srvtoolu_2", Name: "web_fetch", Input: map[string]any{"url": "https://example.com"}},
			&llm.ServerToolResultContent{BlockType: llm.ContentTypeWebFetchToolResult, ToolUseID: "srvtoolu_2"},
			&llm.TextContent{Text: "Done."},
		},
	})
	assert.NoError(t, err)
	assert.Len(t, items, 2)
	assert.NotNil(t, items[0].OfOutputMessage)
	assert.NotNil(t, items[1].OfOutputMessage)
}

// OpenAI's own server tool items round-trip: a decoded mcp_call and
// web_search_call are sent back as the items they came from.
func TestEncodeAssistantMessageReplaysOwnServerToolItems(t *testing.T) {
	var mcpCall responses.ResponseOutputItemMcpCall
	assert.NoError(t, mcpCall.UnmarshalJSON([]byte(`{"type":"mcp_call","id":"mcp_abc","name":"ask","server_label":"deepwiki","arguments":"{}","output":"42"}`)))
	decoded, err := decodeMcpCallContent(mcpCall)
	assert.NoError(t, err)
	assert.Len(t, decoded, 2)

	content := append([]llm.Content{&llm.ServerToolUseContent{ID: "ws_1", Name: "web_search_call"}}, decoded...)
	items, err := encodeAssistantMessage(&llm.Message{Role: llm.Assistant, Content: content})
	assert.NoError(t, err)
	assert.Len(t, items, 2)
	assert.NotNil(t, items[0].OfWebSearchCall)
	assert.NotNil(t, items[1].OfMcpCall)
	assert.Equal(t, "mcp_abc", items[1].OfMcpCall.ID)
	assert.Equal(t, "42", items[1].OfMcpCall.Output.Value)
}
