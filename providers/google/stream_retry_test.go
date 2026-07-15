package google

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/retry"
	"google.golang.org/genai"
)

func newRetryTestProvider(t *testing.T, baseURL string, maxRetries int) *Provider {
	t.Helper()
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		HTTPOptions: genai.HTTPOptions{BaseURL: baseURL},
	})
	assert.NoError(t, err)
	provider := New(
		WithModel("test-model"),
		WithMaxRetries(maxRetries),
		WithRetryBaseWait(time.Millisecond),
	)
	provider.client = client
	return provider
}

func consumeGoogleProviderStream(t *testing.T, iterator llm.StreamIterator) *llm.ResponseAccumulator {
	t.Helper()
	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		assert.NoError(t, accumulator.AddEvent(iterator.Event()))
	}
	assert.NoError(t, iterator.Err())
	return accumulator
}

func TestStreamRetriesBeforeFirstEvent(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"code":429,"message":"temporarily overloaded","status":"RESOURCE_EXHAUSTED"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"responseId\":\"resp_1\",\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{}}\n\n")
	}))
	defer server.Close()

	provider := newRetryTestProvider(t, server.URL, 1)

	var hooks atomic.Int64
	iterator, err := provider.Stream(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
		llm.WithHook(llm.BeforeGenerate, func(context.Context, *llm.HookContext) error {
			hooks.Add(1)
			return nil
		}),
	)
	assert.NoError(t, err)
	defer iterator.Close()

	accumulator := consumeGoogleProviderStream(t, iterator)
	assert.True(t, accumulator.IsComplete())
	assert.Equal(t, "ok", accumulator.Response().Message().Text())
	assert.Equal(t, "stop", accumulator.Response().StopReason)
	assert.Equal(t, int64(2), requests.Load())
	assert.Equal(t, int64(1), hooks.Load())
}

func TestStreamDoesNotRetryPermanentLazyError(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":400,"message":"invalid request","status":"INVALID_ARGUMENT"}}`)
	}))
	defer server.Close()

	provider := newRetryTestProvider(t, server.URL, 2)
	iterator, err := provider.Stream(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	defer iterator.Close()

	assert.False(t, iterator.Next())
	assert.Error(t, iterator.Err())
	assert.Equal(t, int64(1), requests.Load())
}

func TestStreamHookFailureDoesNotStartRequest(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := newRetryTestProvider(t, server.URL, 2)
	sentinel := errors.New("hook rejected request")
	iterator, err := provider.Stream(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
		llm.WithHook(llm.BeforeGenerate, func(context.Context, *llm.HookContext) error {
			return sentinel
		}),
	)
	assert.True(t, errors.Is(err, sentinel))
	assert.Nil(t, iterator)
	assert.Equal(t, int64(0), requests.Load())
}

func TestWrapGoogleErrorHandlesValueAndPointerForms(t *testing.T) {
	valueErr := genai.APIError{Code: http.StatusBadRequest, Message: "invalid request"}
	pointerErr := &genai.APIError{Code: http.StatusTooManyRequests, Message: "overloaded"}

	assert.True(t, retry.IsPermanent(wrapGoogleError(valueErr)))
	assert.False(t, retry.IsPermanent(wrapGoogleError(pointerErr)))
}
