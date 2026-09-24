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

// A stored session can hold a server tool call whose result an older Dive
// dropped while decoding a stream. The call is left out of the request, which
// Anthropic would otherwise reject on every later turn.
func TestRequestDropsOrphanedServerToolCall(t *testing.T) {
	types := requestBlockTypes(t,
		llm.NewUserTextMessage("Summarize https://example.com"),
		&llm.Message{Role: llm.Assistant, Content: []llm.Content{
			&llm.TextContent{Text: "Fetching."},
			&llm.ServerToolUseContent{ID: "srvtoolu_orphan", Name: "web_fetch", Input: map[string]any{"url": "https://example.com"}},
			&llm.TextContent{Text: "It is an example page."},
		}},
		llm.NewUserTextMessage("Thanks"),
	)
	assert.Equal(t, [][]string{{"text"}, {"text", "text"}, {"text"}}, types)
}

// Paired calls and results are sent as they are, including a web fetch
// result, and a result without its call is left out.
func TestRequestKeepsPairedServerToolBlocks(t *testing.T) {
	webFetch, err := llm.UnmarshalContent([]byte(`{"type":"web_fetch_tool_result","tool_use_id":"srvtoolu_2","content":{"type":"web_fetch_result","url":"https://example.com"}}`))
	assert.NoError(t, err)
	types := requestBlockTypes(t,
		llm.NewUserTextMessage("Summarize https://example.com"),
		&llm.Message{Role: llm.Assistant, Content: []llm.Content{
			&llm.ServerToolUseContent{ID: "srvtoolu_2", Name: "web_fetch", Input: map[string]any{"url": "https://example.com"}},
			webFetch,
			&llm.WebSearchToolResultContent{ToolUseID: "srvtoolu_missing"},
			&llm.TextContent{Text: "It is an example page."},
		}},
		llm.NewUserTextMessage("Thanks"),
	)
	assert.Equal(t, [][]string{{"text"}, {"server_tool_use", "web_fetch_tool_result", "text"}, {"text"}}, types)
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
