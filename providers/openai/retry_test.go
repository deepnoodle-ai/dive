package openai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// countingTransport counts every HTTP request and always returns a 429.
type countingTransport struct {
	requests atomic.Int64
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.requests.Add(1)
	body := `{"error": {"message": "rate limited", "type": "rate_limit_error"}}`
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Too Many Requests",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// TestGenerateRetryCount verifies that retries are owned solely by Dive's
// retry loop: with WithMaxRetries(2), a persistent 429 produces exactly 3
// HTTP requests (initial + 2 retries), not multiplied by SDK-internal
// retries.
func TestGenerateRetryCount(t *testing.T) {
	transport := &countingTransport{}
	provider := New(
		WithAPIKey("test-key"),
		WithClient(&http.Client{Transport: transport}),
		WithMaxRetries(2),
	)
	provider.retryBaseWait = time.Millisecond

	_, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.Error(t, err)
	assert.Equal(t, int64(3), transport.requests.Load())
}

// TestGenerateRetryCountDefault verifies the default retry budget (2 retries,
// 3 total attempts) with no SDK-layer amplification.
func TestGenerateRetryCountDefault(t *testing.T) {
	transport := &countingTransport{}
	provider := New(
		WithAPIKey("test-key"),
		WithClient(&http.Client{Transport: transport}),
	)
	provider.retryBaseWait = time.Millisecond

	_, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.Error(t, err)
	assert.Equal(t, int64(DefaultMaxRetries+1), transport.requests.Load())
}
