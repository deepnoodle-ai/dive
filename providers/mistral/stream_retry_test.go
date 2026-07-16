package mistral

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
		WithAPIKey("test-key"),
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
