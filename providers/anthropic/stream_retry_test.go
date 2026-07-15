package anthropic

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

const successfulAnthropicStream = `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[]}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}

data: {"type":"content_block_stop","index":0}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}

data: {"type":"message_stop"}

`

type anthropicRoundTripFunc func(*http.Request) (*http.Response, error)

func (f anthropicRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingAnthropicBody struct {
	data   *strings.Reader
	err    error
	closed atomic.Bool
}

func (b *failingAnthropicBody) Read(p []byte) (int, error) {
	if b.data.Len() == 0 {
		return 0, b.err
	}
	return b.data.Read(p)
}

func (b *failingAnthropicBody) Close() error {
	b.closed.Store(true)
	return nil
}

func anthropicResponse(req *http.Request, body io.ReadCloser) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     http.StatusText(http.StatusOK),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       body,
		Request:    req,
	}
}

func consumeAnthropicStream(t *testing.T, iterator llm.StreamIterator) *llm.ResponseAccumulator {
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
		_, _ = io.WriteString(w, successfulAnthropicStream)
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

	accumulator := consumeAnthropicStream(t, iterator)
	assert.True(t, accumulator.IsComplete())
	assert.Equal(t, "ok", accumulator.Response().Message().Text())
	assert.Equal(t, int64(2), requests.Load())
	assert.Equal(t, int64(1), hooks.Load())
}

func TestStreamRetriesBodyReadFailureBeforeFirstEvent(t *testing.T) {
	sentinel := errors.New("connection reset before first event")
	failedBody := &failingAnthropicBody{data: strings.NewReader(""), err: sentinel}
	var requests atomic.Int64
	client := &http.Client{Transport: anthropicRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			return anthropicResponse(req, failedBody), nil
		}
		return anthropicResponse(req, io.NopCloser(strings.NewReader(successfulAnthropicStream))), nil
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

	accumulator := consumeAnthropicStream(t, iterator)
	assert.True(t, accumulator.IsComplete())
	assert.Equal(t, "ok", accumulator.Response().Message().Text())
	assert.Equal(t, int64(2), requests.Load())
	assert.True(t, failedBody.closed.Load())
}

func TestStreamDoesNotRetryBodyFailureAfterFirstEvent(t *testing.T) {
	sentinel := errors.New("connection reset after first event")
	partialBody := &failingAnthropicBody{
		data: strings.NewReader(`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"test-model","content":[]}}` + "\n"),
		err:  sentinel,
	}
	var requests atomic.Int64
	client := &http.Client{Transport: anthropicRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		return anthropicResponse(req, partialBody), nil
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

	assert.True(t, iterator.Next())
	assert.Equal(t, llm.EventTypeMessageStart, iterator.Event().Type)
	assert.False(t, iterator.Next())
	assert.True(t, errors.Is(iterator.Err(), sentinel))
	assert.Equal(t, int64(1), requests.Load())
	assert.True(t, partialBody.closed.Load())
}
