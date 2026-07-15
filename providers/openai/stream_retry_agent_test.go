package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dive "github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
)

const toolCallStream = `event: response.created
data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_tool","status":"in_progress","model":"test-model","output":[]}}

event: response.output_item.added
data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"fc_1","type":"function_call","status":"in_progress","arguments":"","call_id":"call_1","name":"record_effect"}}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","sequence_number":2,"output_index":0,"item_id":"fc_1","delta":"{}"}

event: response.function_call_arguments.done
data: {"type":"response.function_call_arguments.done","sequence_number":3,"output_index":0,"item_id":"fc_1","arguments":"{}"}

event: response.output_item.done
data: {"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":{"id":"fc_1","type":"function_call","status":"completed","arguments":"{}","call_id":"call_1","name":"record_effect"}}

event: response.completed
data: {"type":"response.completed","sequence_number":5,"response":{"id":"resp_tool","status":"completed","model":"test-model","output":[{"id":"fc_1","type":"function_call","status":"completed","arguments":"{}","call_id":"call_1","name":"record_effect"}],"usage":{"input_tokens":1,"output_tokens":1,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}

`

const finalAnswerStream = `event: response.created
data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_final","status":"in_progress","model":"test-model","output":[]}}

event: response.output_item.added
data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"msg_1","type":"message","status":"in_progress","content":[],"role":"assistant"}}

event: response.content_part.added
data: {"type":"response.content_part.added","sequence_number":2,"item_id":"msg_1","output_index":0,"content_index":0,"part":{"type":"output_text","annotations":[],"text":""}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","sequence_number":3,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"effect recorded once"}

event: response.output_text.done
data: {"type":"response.output_text.done","sequence_number":4,"item_id":"msg_1","output_index":0,"content_index":0,"text":"effect recorded once"}

event: response.output_item.done
data: {"type":"response.output_item.done","sequence_number":5,"output_index":0,"item":{"id":"msg_1","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"text":"effect recorded once"}],"role":"assistant"}}

event: response.completed
data: {"type":"response.completed","sequence_number":6,"response":{"id":"resp_final","status":"completed","model":"test-model","output":[{"id":"msg_1","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"text":"effect recorded once"}],"role":"assistant"}],"usage":{"input_tokens":2,"output_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}

`

func TestAgentRetriesLaterStreamingGenerationWithoutReexecutingTool(t *testing.T) {
	var requests atomic.Int64
	var bodiesMu sync.Mutex
	var requestBodies []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		bodiesMu.Lock()
		requestBodies = append(requestBodies, string(body))
		bodiesMu.Unlock()

		switch requests.Add(1) {
		case 1:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, toolCallStream)
		case 2:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"message":"temporarily at capacity","type":"rate_limit_error"}}`)
		case 3:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, finalAnswerStream)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	provider := New(
		WithAPIKey("test-key"),
		WithEndpoint(server.URL),
		WithMaxRetries(1),
	)
	provider.retryBaseWait = time.Millisecond

	var toolCalls atomic.Int64
	tool := dive.FuncTool("record_effect", "Record one external effect",
		func(context.Context, struct{}) (*dive.ToolResult, error) {
			toolCalls.Add(1)
			return dive.NewToolResultText("recorded"), nil
		},
	)
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model: provider,
		Tools: []dive.Tool{tool},
	})
	assert.NoError(t, err)

	response, err := agent.CreateResponse(context.Background(), dive.WithInput("Record the effect"))
	assert.NoError(t, err)
	assert.Equal(t, "effect recorded once", response.OutputText())
	assert.Equal(t, int64(1), toolCalls.Load())
	assert.Equal(t, int64(3), requests.Load())

	bodiesMu.Lock()
	bodies := append([]string(nil), requestBodies...)
	bodiesMu.Unlock()
	assert.Len(t, bodies, 3)
	assert.False(t, strings.Contains(bodies[0], "function_call_output"))
	assert.Contains(t, bodies[1], "function_call_output")
	assert.Equal(t, bodies[1], bodies[2])
}
