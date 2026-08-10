package ollama

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestWithMaxRetriesControlsStreamingAttempts(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"rate limited"}}`)
	}))
	defer server.Close()

	provider := New(
		WithEndpoint(server.URL),
		WithMaxRetries(1),
		WithBaseWait(time.Millisecond),
	)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	iterator, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	defer iterator.Close()
	for iterator.Next() {
	}

	assert.Error(t, iterator.Err())
	assert.Equal(t, int64(2), requests.Load())
	assert.Equal(t, provider.Name(), provider.Provider.Name())
}

func TestGenerateInheritsDisjointAnthropicUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"msg_1",
			"type":"message",
			"role":"assistant",
			"model":"llama-test",
			"content":[{"type":"text","text":"ok"}],
			"usage":{"input_tokens":100,"output_tokens":10,"cache_creation_input_tokens":20,"cache_read_input_tokens":80}
		}`)
	}))
	defer server.Close()

	provider := New(
		WithEndpoint(server.URL),
		WithModel("llama-test"),
		WithMaxRetries(0),
	)
	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	assert.Equal(t, 100, response.Usage.InputTokens)
	assert.Equal(t, 20, response.Usage.CacheCreationInputTokens)
	assert.Equal(t, 80, response.Usage.CacheReadInputTokens)
	assert.Equal(t, 200, response.Usage.TotalInputTokens())
}
