package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingOpenAIStreamBody struct {
	data   *strings.Reader
	err    error
	closed atomic.Bool
}

func (b *failingOpenAIStreamBody) Read(p []byte) (int, error) {
	if b.data.Len() == 0 {
		return 0, b.err
	}
	return b.data.Read(p)
}

func (b *failingOpenAIStreamBody) Close() error {
	b.closed.Store(true)
	return nil
}

func streamHTTPResponse(req *http.Request, status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func consumeStream(t *testing.T, provider *Provider, opts ...llm.Option) (llm.StreamIterator, []*llm.Event) {
	t.Helper()
	iterator, err := provider.Stream(context.Background(), opts...)
	assert.NoError(t, err)

	var events []*llm.Event
	for iterator.Next() {
		events = append(events, iterator.Event())
	}
	return iterator, events
}

func TestStreamRetryPersistent429(t *testing.T) {
	transport := &countingTransport{}
	provider := New(
		WithAPIKey("test-key"),
		WithClient(&http.Client{Transport: transport}),
		WithMaxRetries(2),
	)
	provider.retryBaseWait = time.Millisecond

	iterator, _ := consumeStream(t, provider,
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	defer iterator.Close()

	assert.Error(t, iterator.Err())
	assert.Equal(t, int64(3), transport.requests.Load())
	var statusErr interface{ StatusCode() int }
	assert.True(t, errors.As(iterator.Err(), &statusErr))
	assert.Equal(t, http.StatusTooManyRequests, statusErr.StatusCode())
}

func TestStreamRetry429ThenSuccess(t *testing.T) {
	fixture, err := os.ReadFile("fixtures/events-hello.txt")
	assert.NoError(t, err)

	var requests atomic.Int64
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			return streamHTTPResponse(req, http.StatusTooManyRequests, "application/json",
				`{"error":{"message":"temporarily at capacity","type":"rate_limit_error"}}`), nil
		}
		return streamHTTPResponse(req, http.StatusOK, "text/event-stream", string(fixture)), nil
	})
	provider := New(
		WithAPIKey("test-key"),
		WithClient(&http.Client{Transport: transport}),
		WithMaxRetries(2),
	)
	provider.retryBaseWait = time.Millisecond
	var hooks atomic.Int64

	iterator, events := consumeStream(t, provider,
		llm.WithMessages(llm.NewUserTextMessage("hello")),
		llm.WithHook(llm.BeforeGenerate, func(context.Context, *llm.HookContext) error {
			hooks.Add(1)
			return nil
		}),
	)
	defer iterator.Close()

	assert.NoError(t, iterator.Err())
	assert.Equal(t, int64(2), requests.Load())
	assert.Equal(t, int64(1), hooks.Load())

	accumulator := llm.NewResponseAccumulator()
	messageStarts := 0
	for _, event := range events {
		if event.Type == llm.EventTypeMessageStart {
			messageStarts++
		}
		assert.NoError(t, accumulator.AddEvent(event))
	}
	assert.Equal(t, 1, messageStarts)
	assert.Equal(t, "Hello! How can I assist you today?", accumulator.Response().Message().Text())
}

func TestStreamRetryTransportErrorThenSuccess(t *testing.T) {
	fixture, err := os.ReadFile("fixtures/events-hello.txt")
	assert.NoError(t, err)

	sentinel := errors.New("connection reset before response headers")
	var requests atomic.Int64
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			return nil, sentinel
		}
		return streamHTTPResponse(req, http.StatusOK, "text/event-stream", string(fixture)), nil
	})
	provider := New(
		WithAPIKey("test-key"),
		WithClient(&http.Client{Transport: transport}),
		WithMaxRetries(1),
	)
	provider.retryBaseWait = time.Millisecond

	iterator, events := consumeStream(t, provider,
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	defer iterator.Close()

	assert.NoError(t, iterator.Err())
	assert.Equal(t, int64(2), requests.Load())
	accumulator := llm.NewResponseAccumulator()
	for _, event := range events {
		assert.NoError(t, accumulator.AddEvent(event))
	}
	assert.True(t, accumulator.IsComplete())
	assert.Equal(t, "Hello! How can I assist you today?", accumulator.Response().Message().Text())
}

func TestStreamDoesNotRetryTransportErrorAfterFirstEvent(t *testing.T) {
	sentinel := errors.New("connection reset after first event")
	partialStream := `event: response.created
data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_partial","status":"in_progress","model":"test-model","output":[]}}

`
	body := &failingOpenAIStreamBody{data: strings.NewReader(partialStream), err: sentinel}
	var requests atomic.Int64
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     http.StatusText(http.StatusOK),
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       body,
			Request:    req,
		}, nil
	})
	provider := New(
		WithAPIKey("test-key"),
		WithClient(&http.Client{Transport: transport}),
		WithMaxRetries(2),
	)
	provider.retryBaseWait = time.Millisecond
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
	assert.True(t, body.closed.Load())
}

func TestStreamRetryPermanent400(t *testing.T) {
	var requests atomic.Int64
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		return streamHTTPResponse(req, http.StatusBadRequest, "application/json",
			`{"error":{"message":"invalid request","type":"invalid_request_error"}}`), nil
	})
	provider := New(
		WithAPIKey("test-key"),
		WithClient(&http.Client{Transport: transport}),
		WithMaxRetries(2),
	)
	provider.retryBaseWait = time.Millisecond

	iterator, _ := consumeStream(t, provider,
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	defer iterator.Close()

	assert.Error(t, iterator.Err())
	assert.Equal(t, int64(1), requests.Load())
}

type retrySignalLogger struct {
	retry chan struct{}
	once  sync.Once
}

func (l *retrySignalLogger) Debug(string, ...any)   {}
func (l *retrySignalLogger) Info(string, ...any)    {}
func (l *retrySignalLogger) Error(string, ...any)   {}
func (l *retrySignalLogger) With(...any) llm.Logger { return l }
func (l *retrySignalLogger) Warn(string, ...any) {
	l.once.Do(func() { close(l.retry) })
}

func TestStreamRetryCancellationDuringBackoff(t *testing.T) {
	transport := &countingTransport{}
	provider := New(
		WithAPIKey("test-key"),
		WithClient(&http.Client{Transport: transport}),
		WithMaxRetries(2),
	)
	provider.retryBaseWait = 10 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := &retrySignalLogger{retry: make(chan struct{})}
	iterator, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("hello")),
		llm.WithLogger(logger),
	)
	assert.NoError(t, err)
	defer iterator.Close()

	next := make(chan bool, 1)
	go func() { next <- iterator.Next() }()

	select {
	case <-logger.retry:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("retry did not enter backoff")
	}

	select {
	case gotEvent := <-next:
		assert.False(t, gotEvent)
	case <-time.After(time.Second):
		t.Fatal("stream did not stop promptly after cancellation")
	}
	assert.True(t, errors.Is(iterator.Err(), context.Canceled))
	assert.Equal(t, int64(1), transport.requests.Load())
}

func TestStreamRetryDefaultBudget(t *testing.T) {
	transport := &countingTransport{}
	provider := New(
		WithAPIKey("test-key"),
		WithClient(&http.Client{Transport: transport}),
	)
	provider.retryBaseWait = time.Millisecond

	iterator, _ := consumeStream(t, provider,
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	defer iterator.Close()

	assert.Error(t, iterator.Err())
	assert.Equal(t, int64(DefaultMaxRetries+1), transport.requests.Load())
}
