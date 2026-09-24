package openaicompletions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// unansweredHistory has a call answered only in part mid-history and an
// unanswered call at the tail.
func unansweredHistory() []*llm.Message {
	return []*llm.Message{
		llm.NewUserTextMessage("hi"),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_a", Name: "lookup", Input: []byte(`{}`)},
			&llm.ToolUseContent{ID: "call_b", Name: "lookup", Input: []byte(`{}`)},
		}},
		llm.NewToolResultMessage(&llm.ToolResultContent{ToolUseID: "call_a", Content: "found"}),
		llm.NewAssistantTextMessage("partly done"),
		llm.NewUserTextMessage("next"),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_c", Name: "lookup", Input: []byte(`{}`)},
		}},
	}
}

func TestCompletionsAnswersUnansweredToolCalls(t *testing.T) {
	var sent Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","model":"gpt-5.5","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(server.Close)
	provider := New(WithAPIKey("k"), WithEndpoint(server.URL), WithModel(ModelGPT55), WithMaxRetries(0))

	history := unansweredHistory()
	response, err := provider.Generate(context.Background(), llm.WithMessages(history...))
	assert.NoError(t, err)
	assert.Equal(t, response.StopReason, "stop")

	answered := map[string]string{}
	for _, m := range sent.Messages {
		if m.Role == "tool" {
			answered[m.ToolCallID] = m.Content
		}
	}
	assert.Equal(t, answered["call_a"], "found")
	assert.Contains(t, answered["call_b"], "Unknown result:")
	assert.Contains(t, answered["call_c"], "Unknown result:")
	assert.Equal(t, len(history), 6)
}

// TestCompletionsGenerateReportsToolUse verifies that the non-streaming path
// reports a stop reason, mapping tool_calls to tool_use as the stream does.
func TestCompletionsGenerateReportsToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","model":"gpt-5.5","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`))
	}))
	t.Cleanup(server.Close)
	provider := New(WithAPIKey("k"), WithEndpoint(server.URL), WithModel(ModelGPT55), WithMaxRetries(0))
	response, err := provider.Generate(context.Background(), llm.WithMessages(llm.NewUserTextMessage("hi")))
	assert.NoError(t, err)
	assert.Equal(t, response.StopReason, "tool_use")
}
