package ollama

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// A session that used Anthropic web search and then switches to Ollama must
// not send Anthropic's server tool blocks: Ollama never ran those tools.
func TestHistoryFromAnthropicWebSearchOmitsServerToolBlocks(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_2","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()

	_, err := New(WithEndpoint(server.URL)).Generate(context.Background(), llm.WithMessages(
		llm.NewUserTextMessage("When was Claude Shannon born?"),
		&llm.Message{Role: llm.Assistant, Content: []llm.Content{
			&llm.TextContent{Text: "Searching."},
			&llm.ServerToolUseContent{ID: "srvtoolu_1", Name: "web_search", Input: map[string]any{"query": "q"}},
			&llm.WebSearchToolResultContent{ToolUseID: "srvtoolu_1"},
			&llm.TextContent{Text: "April 30, 1916."},
		}},
		llm.NewUserTextMessage("Thanks"),
	))
	assert.NoError(t, err)
	assert.False(t, strings.Contains(body, "server_tool_use"))
	assert.False(t, strings.Contains(body, "web_search_tool_result"))
	assert.True(t, strings.Contains(body, "April 30, 1916."))
}
