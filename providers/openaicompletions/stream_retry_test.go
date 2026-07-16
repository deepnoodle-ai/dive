package openaicompletions

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

const successfulChatCompletionsStream = `data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"}}]}

data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"test-model","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}

data: [DONE]

`

type completionsRoundTripFunc func(*http.Request) (*http.Response, error)

func (f completionsRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingCompletionsBody struct {
	data   *strings.Reader
	err    error
	closed atomic.Bool
}

func (b *failingCompletionsBody) Read(p []byte) (int, error) {
	if b.data.Len() == 0 {
		return 0, b.err
	}
	return b.data.Read(p)
}

func (b *failingCompletionsBody) Close() error {
	b.closed.Store(true)
	return nil
}

func completionsResponse(req *http.Request, body io.ReadCloser) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     http.StatusText(http.StatusOK),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       body,
		Request:    req,
	}
}

func consumeCompletionsStream(t *testing.T, iterator llm.StreamIterator) *llm.ResponseAccumulator {
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
			_, _ = io.WriteString(w, `{"error":{"message":"temporarily overloaded"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, successfulChatCompletionsStream)
	}))
	defer server.Close()

	provider := New(
		WithAPIKey("test-key"),
		WithEndpoint(server.URL),
		WithMaxRetries(1),
		WithBaseWait(time.Millisecond),
	)
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

	accumulator := consumeCompletionsStream(t, iterator)
	assert.True(t, accumulator.IsComplete())
	assert.Equal(t, "ok", accumulator.Response().Message().Text())
	assert.Equal(t, int64(2), requests.Load())
	assert.Equal(t, int64(1), hooks.Load())
}

func TestStreamRetriesBodyReadFailureBeforeFirstEvent(t *testing.T) {
	sentinel := errors.New("connection reset before first event")
	failedBody := &failingCompletionsBody{data: strings.NewReader(""), err: sentinel}
	var requests atomic.Int64
	client := &http.Client{Transport: completionsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			return completionsResponse(req, failedBody), nil
		}
		return completionsResponse(req, io.NopCloser(strings.NewReader(successfulChatCompletionsStream))), nil
	})}
	provider := New(
		WithAPIKey("test-key"),
		WithClient(client),
		WithMaxRetries(1),
		WithBaseWait(time.Millisecond),
	)
	iterator, err := provider.Stream(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	defer iterator.Close()

	accumulator := consumeCompletionsStream(t, iterator)
	assert.True(t, accumulator.IsComplete())
	assert.Equal(t, "ok", accumulator.Response().Message().Text())
	assert.Equal(t, int64(2), requests.Load())
	assert.True(t, failedBody.closed.Load())
}

func TestStreamDoesNotRetryBodyFailureAfterFirstEvent(t *testing.T) {
	sentinel := errors.New("connection reset after first event")
	partialBody := &failingCompletionsBody{
		data: strings.NewReader(`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"partial"}}]}` + "\n"),
		err:  sentinel,
	}
	var requests atomic.Int64
	client := &http.Client{Transport: completionsRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		return completionsResponse(req, partialBody), nil
	})}
	provider := New(
		WithAPIKey("test-key"),
		WithClient(client),
		WithMaxRetries(2),
		WithBaseWait(time.Millisecond),
	)
	iterator, err := provider.Stream(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	defer iterator.Close()

	var events int
	for iterator.Next() {
		events++
	}
	assert.True(t, events > 0)
	assert.True(t, errors.Is(iterator.Err(), sentinel))
	assert.Equal(t, int64(1), requests.Load())
	assert.True(t, partialBody.closed.Load())
}
