package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

const okMessage = `{"id":"msg_2","type":"message","role":"assistant","model":"test-model","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`

// requestBlockTypes sends messages through Generate and returns the block
// types of each message in the request body.
func requestBlockTypes(t *testing.T, messages ...*llm.Message) [][]string {
	t.Helper()
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, okMessage)
	}))
	defer server.Close()
	_, err := New(WithAPIKey("k"), WithEndpoint(server.URL)).
		Generate(context.Background(), llm.WithMessages(messages...))
	assert.NoError(t, err)

	var request struct {
		Messages []struct {
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		} `json:"messages"`
	}
	assert.NoError(t, json.Unmarshal(body, &request))
	var types [][]string
	for _, message := range request.Messages {
		var blockTypes []string
		for _, block := range message.Content {
			blockTypes = append(blockTypes, block.Type)
		}
		types = append(types, blockTypes)
	}
	return types
}

// A session that switches from OpenAI carries server tool items Anthropic
// never ran: a web_search_call and an mcp_call with its output. They are left
// out of the request, and a message holding nothing else is dropped.
func TestRequestDropsForeignServerToolContent(t *testing.T) {
	types := requestBlockTypes(t,
		llm.NewUserTextMessage("Search, then ask the MCP server"),
		&llm.Message{Role: llm.Assistant, Content: []llm.Content{
			&llm.ServerToolUseContent{ID: "ws_1", Name: "web_search_call", Input: map[string]any{"query": "q"}},
			&llm.TextContent{Text: "Found it."},
		}},
		&llm.Message{Role: llm.Assistant, Content: []llm.Content{
			&llm.MCPToolUseContent{ID: "mcp_1", Name: "lookup", ServerName: "docs", Input: json.RawMessage(`{}`)},
			&llm.MCPToolResultContent{ToolUseID: "mcp_1", Content: []*llm.ContentChunk{{Type: "text", Text: "result"}}},
			&llm.MCPListToolsContent{ServerLabel: "docs"},
		}},
		llm.NewUserTextMessage("Thanks"),
	)
	assert.Equal(t, [][]string{{"text"}, {"text"}, {"text"}}, types)
}

// Anthropic's own server tool calls and results are sent as they are,
// including a web fetch result and an MCP connector pair.
func TestRequestKeepsAnthropicServerToolContent(t *testing.T) {
	webFetch, err := llm.UnmarshalContent([]byte(`{"type":"web_fetch_tool_result","tool_use_id":"srvtoolu_2","content":{"type":"web_fetch_result","url":"https://example.com"}}`))
	assert.NoError(t, err)
	types := requestBlockTypes(t,
		llm.NewUserTextMessage("Summarize https://example.com"),
		&llm.Message{Role: llm.Assistant, Content: []llm.Content{
			&llm.ServerToolUseContent{ID: "srvtoolu_2", Name: "web_fetch", Input: map[string]any{"url": "https://example.com"}},
			webFetch,
			&llm.MCPToolUseContent{ID: "mcptoolu_1", Name: "lookup", ServerName: "docs", Input: json.RawMessage(`{}`)},
			&llm.MCPToolResultContent{ToolUseID: "mcptoolu_1", Content: []*llm.ContentChunk{{Type: "text", Text: "result"}}},
			&llm.TextContent{Text: "It is an example page."},
		}},
		llm.NewUserTextMessage("Thanks"),
	)
	assert.Equal(t, [][]string{
		{"text"},
		{"server_tool_use", "web_fetch_tool_result", "mcp_tool_use", "mcp_tool_result", "text"},
		{"text"},
	}, types)
}

// A paused turn (pause_turn) ends with a server tool call that has no result
// yet; sending it back as the last block resumes the turn, so it is kept.
func TestRequestKeepsPausedServerToolCall(t *testing.T) {
	types := requestBlockTypes(t,
		llm.NewUserTextMessage("Search for Claude Shannon"),
		&llm.Message{Role: llm.Assistant, Content: []llm.Content{
			&llm.TextContent{Text: "Searching."},
			&llm.ServerToolUseContent{ID: "srvtoolu_1", Name: "web_search", Input: map[string]any{"query": "claude shannon"}},
		}},
	)
	assert.Equal(t, [][]string{{"text"}, {"text", "server_tool_use"}}, types)
}
