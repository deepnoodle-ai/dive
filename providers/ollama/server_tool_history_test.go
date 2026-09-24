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

	answer := &llm.Message{Role: llm.Assistant, Content: []llm.Content{
		&llm.TextContent{Text: "Searching."},
		&llm.ServerToolUseContent{ID: "srvtoolu_1", Name: "web_search", Input: map[string]any{"query": "q"}},
		&llm.WebSearchToolResultContent{ToolUseID: "srvtoolu_1"},
		&llm.TextContent{Text: "April 30, 1916."},
	}}
	_, err := New(WithEndpoint(server.URL)).Generate(context.Background(), llm.WithMessages(
		llm.NewUserTextMessage("When was Claude Shannon born?"),
		answer,
		llm.NewUserTextMessage("Thanks"),
	))
	assert.NoError(t, err)
	assert.False(t, strings.Contains(body, "server_tool_use"))
	assert.False(t, strings.Contains(body, "web_search_tool_result"))
	assert.True(t, strings.Contains(body, "Searching."))
	assert.True(t, strings.Contains(body, "April 30, 1916."))

	// The caller's history is not modified.
	assert.Len(t, answer.Content, 4)
}

// A message holding only server tool blocks is dropped entirely.
func TestWithoutServerToolsDropsEmptiedMessages(t *testing.T) {
	opts := withoutServerTools([]llm.Option{llm.WithMessages(
		llm.NewUserTextMessage("hi"),
		&llm.Message{Role: llm.Assistant, Content: []llm.Content{
			&llm.ServerToolUseContent{ID: "srvtoolu_1", Name: "web_search"},
		}},
	)})
	config := &llm.Config{}
	config.Apply(opts...)
	assert.Len(t, config.Messages, 1)
	assert.Equal(t, llm.User, config.Messages[0].Role)
}
