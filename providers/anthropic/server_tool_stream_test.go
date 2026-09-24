package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// webSearchStream is a streamed web search answer, shaped like the example in
// Anthropic's web search tool docs: preamble text, the server tool call (its
// input streamed as input_json_delta), the search results, then the answer
// split into text blocks at a citation boundary.
const webSearchStream = `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"I'll search for when Claude Shannon was born."}}

data: {"type":"content_block_stop","index":0}

data: {"type":"content_block_start","index":1,"content_block":{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{}}}

data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"query\": "}}

data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"claude shannon birth date\"}"}}

data: {"type":"content_block_stop","index":1}

data: {"type":"content_block_start","index":2,"content_block":{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[{"type":"web_search_result","url":"https://en.wikipedia.org/wiki/Claude_Shannon","title":"Claude Shannon - Wikipedia","encrypted_content":"EqgfCioIARgB","page_age":"April 30, 2025"}]}}

data: {"type":"content_block_stop","index":2}

data: {"type":"content_block_start","index":3,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":3,"delta":{"type":"text_delta","text":"Based on the search results, "}}

data: {"type":"content_block_stop","index":3}

data: {"type":"content_block_start","index":4,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":4,"delta":{"type":"text_delta","text":"Claude Shannon was born on April 30, 1916"}}

data: {"type":"content_block_stop","index":4}

data: {"type":"content_block_start","index":5,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":5,"delta":{"type":"text_delta","text":"."}}

data: {"type":"content_block_stop","index":5}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":40}}

data: {"type":"message_stop"}

`

// webSearchMessage is the same answer as a non-streaming response.
const webSearchMessage = `{"id":"msg_1","type":"message","role":"assistant","model":"test-model","stop_reason":"end_turn",
"usage":{"input_tokens":10,"output_tokens":40},
"content":[
 {"type":"text","text":"I'll search for when Claude Shannon was born."},
 {"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"claude shannon birth date"}},
 {"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[{"type":"web_search_result","url":"https://en.wikipedia.org/wiki/Claude_Shannon","title":"Claude Shannon - Wikipedia","encrypted_content":"EqgfCioIARgB","page_age":"April 30, 2025"}]},
 {"type":"text","text":"Based on the search results, "},
 {"type":"text","text":"Claude Shannon was born on April 30, 1916"},
 {"type":"text","text":"."}
]}`

const plainTextStream = `data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","model":"test-model","content":[],"usage":{"input_tokens":10,"output_tokens":1}}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"You're welcome."}}

data: {"type":"content_block_stop","index":0}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}

data: {"type":"message_stop"}

`

func serveBody(contentType, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = io.WriteString(w, body)
	}))
}

// Streaming must keep server tool blocks as the same content Generate
// returns, rather than dropping them.
func TestStreamKeepsServerToolBlocksLikeGenerate(t *testing.T) {
	ctx := context.Background()

	streamServer := serveBody("text/event-stream", webSearchStream)
	defer streamServer.Close()
	iterator, err := New(WithAPIKey("k"), WithEndpoint(streamServer.URL)).
		Stream(ctx, llm.WithMessages(llm.NewUserTextMessage("When was Claude Shannon born?")))
	assert.NoError(t, err)
	defer iterator.Close()
	streamed := consumeAnthropicStream(t, iterator).Response()

	generateServer := serveBody("application/json", webSearchMessage)
	defer generateServer.Close()
	generated, err := New(WithAPIKey("k"), WithEndpoint(generateServer.URL)).
		Generate(ctx, llm.WithMessages(llm.NewUserTextMessage("When was Claude Shannon born?")))
	assert.NoError(t, err)

	streamedJSON, err := json.Marshal(streamed.Content)
	assert.NoError(t, err)
	generatedJSON, err := json.Marshal(generated.Content)
	assert.NoError(t, err)
	assert.Equal(t, string(generatedJSON), string(streamedJSON))

	serverToolUse, ok := streamed.Content[1].(*llm.ServerToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "claude shannon birth date", serverToolUse.Input["query"])
	_, ok = streamed.Content[2].(*llm.WebSearchToolResultContent)
	assert.True(t, ok)
}

// An agent answer that follows a streamed web search keeps the passage break
// before the answer, and the server tool blocks replay to Anthropic on the
// next turn.
func TestAgentWebSearchStreamOutputTextAndReplay(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		n := len(bodies)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 {
			_, _ = io.WriteString(w, webSearchStream)
		} else {
			_, _ = io.WriteString(w, plainTextStream)
		}
	}))
	defer server.Close()

	// A custom endpoint (a gateway or proxy to Anthropic) replays the blocks
	// too; only the ollama provider strips them.
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model:   New(WithAPIKey("k"), WithEndpoint(server.URL)),
		Session: session.New("web-search"),
	})
	assert.NoError(t, err)

	ctx := context.Background()
	resp, err := agent.CreateResponse(ctx, dive.WithInput("When was Claude Shannon born?"))
	assert.NoError(t, err)
	assert.Equal(t,
		"I'll search for when Claude Shannon was born.\n\nBased on the search results, Claude Shannon was born on April 30, 1916.",
		resp.OutputText())

	_, err = agent.CreateResponse(ctx, dive.WithInput("Thanks"))
	assert.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, len(bodies))
	var request struct {
		Messages []struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"messages"`
	}
	assert.NoError(t, json.Unmarshal([]byte(bodies[1]), &request))
	var types []string
	for _, message := range request.Messages {
		if message.Role != "assistant" {
			continue
		}
		for _, block := range message.Content {
			types = append(types, block["type"].(string))
			switch block["type"] {
			case "server_tool_use":
				assert.Equal(t, "srvtoolu_1", block["id"])
				assert.Equal(t, map[string]any{"query": "claude shannon birth date"}, block["input"])
			case "web_search_tool_result":
				assert.Equal(t, "srvtoolu_1", block["tool_use_id"])
				assert.True(t, strings.Contains(bodies[1], `"encrypted_content":"EqgfCioIARgB"`))
			}
		}
	}
	assert.Equal(t, []string{"text", "server_tool_use", "web_search_tool_result", "text", "text", "text"}, types)
}
