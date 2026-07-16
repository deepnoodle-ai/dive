package grok

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestProvider_ImplementsInterfaces(t *testing.T) {
	provider := New()

	// Test that it implements LLM interface
	var _ llm.LLM = provider

	// Test that it implements StreamingLLM interface
	var _ llm.StreamingLLM = provider
}

func TestProvider_Name(t *testing.T) {
	provider := New()
	name := provider.Name()
	expected := "grok"
	assert.Equal(t, expected, name)
}

func TestProvider_DefaultModel(t *testing.T) {
	assert.Equal(t, ModelGrok45, DefaultModel)
}

func TestProvider_GetAPIKey(t *testing.T) {
	// Test with no env vars set
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROK_API_KEY", "")
	assert.Equal(t, "", getAPIKey())

	// Test with XAI_API_KEY
	t.Setenv("XAI_API_KEY", "xai-key")
	assert.Equal(t, "xai-key", getAPIKey())

	// Test with GROK_API_KEY as fallback
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROK_API_KEY", "grok-key")
	assert.Equal(t, "grok-key", getAPIKey())

	// Test XAI_API_KEY takes priority
	t.Setenv("XAI_API_KEY", "xai-key")
	t.Setenv("GROK_API_KEY", "grok-key")
	assert.Equal(t, "xai-key", getAPIKey())
}

func TestWithMaxRetriesControlsStreamingAttempts(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"temporarily at capacity","type":"rate_limit_error"}}`))
	}))
	defer server.Close()

	provider := New(
		WithAPIKey("test-key"),
		WithEndpoint(server.URL),
		WithMaxRetries(1),
		WithRetryBaseWait(time.Millisecond),
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
}
