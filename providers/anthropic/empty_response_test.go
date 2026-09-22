package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func emptyContentServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

// Opus 5.5 answers tool_choice "none" with no content blocks. Generate must
// return that response, as Stream does, rather than fail the call.
func TestGenerateReturnsEmptyContent(t *testing.T) {
	server := emptyContentServer(t, `{
		"id": "msg_1", "type": "message", "role": "assistant",
		"model": "claude-opus-5-5", "content": [], "stop_reason": "end_turn",
		"usage": {"input_tokens": 120, "output_tokens": 3}
	}`)
	provider := New(WithEndpoint(server.URL), WithAPIKey("test-key"))

	resp, err := provider.Generate(context.Background(),
		llm.WithModel(ModelClaudeOpus55),
		llm.WithMessages(llm.NewUserTextMessage("What's the weather in Paris?")),
	)

	assert.NoError(t, err)
	assert.Len(t, resp.Content, 0)
	assert.Equal(t, "end_turn", resp.StopReason)
	assert.Equal(t, 120, resp.Usage.InputTokens)
}

// A refusal can carry no content; the caller needs its stop details, not an
// error that hides them.
func TestGenerateReturnsEmptyRefusal(t *testing.T) {
	server := emptyContentServer(t, `{
		"id": "msg_2", "type": "message", "role": "assistant",
		"model": "claude-opus-5-5", "content": [], "stop_reason": "refusal",
		"stop_details": {"type": "reasoning_extraction"},
		"usage": {"input_tokens": 80, "output_tokens": 1}
	}`)
	provider := New(WithEndpoint(server.URL), WithAPIKey("test-key"))

	resp, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
	)

	assert.NoError(t, err)
	assert.Equal(t, "refusal", resp.StopReason)
	assert.NotNil(t, resp.StopDetails)
	assert.Equal(t, "reasoning_extraction", resp.StopDetails.Type)
}

// With nothing to prepend it to, a prefill is dropped instead of failing.
func TestGenerateEmptyContentWithPrefill(t *testing.T) {
	server := emptyContentServer(t, `{
		"id": "msg_3", "type": "message", "role": "assistant",
		"model": "claude-haiku-4-5", "content": [], "stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 0}
	}`)
	provider := New(WithEndpoint(server.URL), WithAPIKey("test-key"))

	resp, err := provider.Generate(context.Background(),
		llm.WithModel("claude-haiku-4-5"),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithPrefill("{", ""),
	)

	assert.NoError(t, err)
	assert.Len(t, resp.Content, 0)
}

// Stream applies a prefill only when a text block arrives; Generate matches it
// for a response that is only a tool call.
func TestGenerateToolUseOnlyWithPrefill(t *testing.T) {
	server := emptyContentServer(t, `{
		"id": "msg_4", "type": "message", "role": "assistant",
		"model": "claude-haiku-4-5", "stop_reason": "tool_use",
		"content": [{"type": "tool_use", "id": "toolu_1", "name": "get_weather", "input": {"city": "Paris"}}],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`)
	provider := New(WithEndpoint(server.URL), WithAPIKey("test-key"))

	resp, err := provider.Generate(context.Background(),
		llm.WithModel("claude-haiku-4-5"),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
		llm.WithPrefill("{", ""),
	)

	assert.NoError(t, err)
	assert.Len(t, resp.ToolCalls(), 1)
}
