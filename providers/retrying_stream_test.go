package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

type testStreamIterator struct {
	events     []*llm.Event
	err        error
	pos        int
	closed     bool
	closeCount int
	closeErr   error
}

func (s *testStreamIterator) Next() bool {
	if s.pos >= len(s.events) {
		return false
	}
	s.pos++
	return true
}

func (s *testStreamIterator) Event() *llm.Event { return s.events[s.pos-1] }
func (s *testStreamIterator) Err() error        { return s.err }
func (s *testStreamIterator) Close() error {
	s.closed = true
	s.closeCount++
	return s.closeErr
}

func TestRetryingStreamFirstAttemptSuccess(t *testing.T) {
	stream := &testStreamIterator{events: []*llm.Event{
		{Type: llm.EventTypeMessageStart},
		{Type: llm.EventTypeMessageStop},
	}}
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    2,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		attempts.Add(1)
		return stream, nil
	})

	assert.True(t, iterator.Next())
	assert.Equal(t, llm.EventTypeMessageStart, iterator.Event().Type)
	assert.True(t, iterator.Next())
	assert.Equal(t, llm.EventTypeMessageStop, iterator.Event().Type)
	assert.False(t, iterator.Next())
	assert.NoError(t, iterator.Err())
	assert.Equal(t, int64(1), attempts.Load())
	assert.NoError(t, iterator.Close())
	assert.Equal(t, 1, stream.closeCount)
}

func TestRetryingStreamSharesBudgetAcrossFactoryAndIteratorErrors(t *testing.T) {
	preEventFailure := &testStreamIterator{err: errors.New("connection reset")}
	success := &testStreamIterator{events: []*llm.Event{{Type: llm.EventTypeMessageStart}}}
	var attempts atomic.Int64

	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    2,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		switch attempts.Add(1) {
		case 1:
			return nil, NewError(429, "rate limited")
		case 2:
			return preEventFailure, nil
		default:
			return success, nil
		}
	})
	defer iterator.Close()

	assert.True(t, iterator.Next())
	assert.Equal(t, llm.EventTypeMessageStart, iterator.Event().Type)
	assert.Equal(t, int64(3), attempts.Load())
	assert.True(t, preEventFailure.closed)
}

func TestRetryingStreamClosesIteratorReturnedWithFactoryError(t *testing.T) {
	failed := &testStreamIterator{}
	success := &testStreamIterator{events: []*llm.Event{{Type: llm.EventTypeMessageStart}}}
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    1,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		if attempts.Add(1) == 1 {
			return failed, NewError(http.StatusTooManyRequests, "rate limited")
		}
		return success, nil
	})
	defer iterator.Close()

	assert.True(t, iterator.Next())
	assert.Equal(t, int64(2), attempts.Load())
	assert.Equal(t, 1, failed.closeCount)
}

func TestRetryingStreamRetriesCloseErrorBeforeFirstEvent(t *testing.T) {
	sentinel := errors.New("failed to close empty response body")
	failed := &testStreamIterator{closeErr: sentinel}
	success := &testStreamIterator{events: []*llm.Event{{Type: llm.EventTypeMessageStart}}}
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    1,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		if attempts.Add(1) == 1 {
			return failed, nil
		}
		return success, nil
	})
	defer iterator.Close()

	assert.True(t, iterator.Next())
	assert.Equal(t, int64(2), attempts.Load())
	assert.Equal(t, 1, failed.closeCount)
}

func TestRetryingStreamPreservesStreamAndCloseErrors(t *testing.T) {
	streamErr := errors.New("stream read failed")
	closeErr := errors.New("stream close failed")
	stream := &testStreamIterator{err: streamErr, closeErr: closeErr}
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    0,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		return stream, nil
	})
	defer iterator.Close()

	assert.False(t, iterator.Next())
	assert.True(t, errors.Is(iterator.Err(), streamErr))
	assert.True(t, errors.Is(iterator.Err(), closeErr))
	assert.Equal(t, 1, stream.closeCount)
}

func TestRetryingStreamStopsOnPermanentFactoryError(t *testing.T) {
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    2,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		attempts.Add(1)
		return nil, NewError(400, "bad request")
	})
	defer iterator.Close()

	assert.False(t, iterator.Next())
	assert.Error(t, iterator.Err())
	assert.Equal(t, int64(1), attempts.Load())
}

func TestRetryingStreamExhaustsTransientIteratorErrors(t *testing.T) {
	sentinel := errors.New("connection reset before first event")
	var attempts atomic.Int64
	var streams []*testStreamIterator
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    2,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		attempts.Add(1)
		stream := &testStreamIterator{err: sentinel}
		streams = append(streams, stream)
		return stream, nil
	})
	defer iterator.Close()

	assert.False(t, iterator.Next())
	assert.True(t, errors.Is(iterator.Err(), sentinel))
	assert.Equal(t, int64(3), attempts.Load())
	for _, stream := range streams {
		assert.Equal(t, 1, stream.closeCount)
	}
	assert.False(t, iterator.Next())
	assert.Equal(t, int64(3), attempts.Load())
}

func TestRetryingStreamStopsOnNormalizedPermanentIteratorError(t *testing.T) {
	rawErr := errors.New("sdk authentication error")
	var attempts atomic.Int64
	var normalizations atomic.Int64
	stream := &testStreamIterator{err: rawErr}
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    2,
		RetryBaseWait: time.Millisecond,
		NormalizeError: func(err error) error {
			normalizations.Add(1)
			assert.True(t, errors.Is(err, rawErr))
			return NewError(http.StatusUnauthorized, "unauthorized")
		},
	}, func() (llm.StreamIterator, error) {
		attempts.Add(1)
		return stream, nil
	})
	defer iterator.Close()

	assert.False(t, iterator.Next())
	assert.Error(t, iterator.Err())
	assert.Equal(t, int64(1), attempts.Load())
	assert.Equal(t, int64(1), normalizations.Load())
	assert.Equal(t, 1, stream.closeCount)
}

func TestRetryingStreamEmptySuccessDoesNotRetry(t *testing.T) {
	var attempts atomic.Int64
	stream := &testStreamIterator{}
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    2,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		attempts.Add(1)
		return stream, nil
	})
	defer iterator.Close()

	assert.False(t, iterator.Next())
	assert.NoError(t, iterator.Err())
	assert.Equal(t, int64(1), attempts.Load())
	assert.Equal(t, 1, stream.closeCount)
}

func TestRetryingStreamRejectsNilFactoryAndNilStream(t *testing.T) {
	t.Run("nil factory", func(t *testing.T) {
		iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
			MaxRetries:    2,
			RetryBaseWait: time.Millisecond,
		}, nil)
		defer iterator.Close()
		assert.False(t, iterator.Next())
		assert.Error(t, iterator.Err())
	})

	t.Run("nil stream", func(t *testing.T) {
		var attempts atomic.Int64
		iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
			MaxRetries:    2,
			RetryBaseWait: time.Millisecond,
		}, func() (llm.StreamIterator, error) {
			attempts.Add(1)
			return nil, nil
		})
		defer iterator.Close()
		assert.False(t, iterator.Next())
		assert.Error(t, iterator.Err())
		assert.Equal(t, int64(1), attempts.Load())
	})
}

func TestRetryingStreamClampsNegativeRetryBudget(t *testing.T) {
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		MaxRetries:    -1,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		attempts.Add(1)
		return nil, NewError(http.StatusTooManyRequests, "rate limited")
	})
	defer iterator.Close()

	assert.Nil(t, iterator.Event())
	assert.False(t, iterator.Next())
	assert.Error(t, iterator.Err())
	assert.Equal(t, int64(1), attempts.Load())
}

func TestRetryingStreamDoesNotRetryAfterFirstEvent(t *testing.T) {
	committed := &testStreamIterator{
		events: []*llm.Event{{Type: llm.EventTypeMessageStart}},
		err:    errors.New("connection reset after first event"),
	}
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    2,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		attempts.Add(1)
		return committed, nil
	})
	defer iterator.Close()

	assert.True(t, iterator.Next())
	assert.False(t, iterator.Next())
	assert.Error(t, iterator.Err())
	assert.Equal(t, int64(1), attempts.Load())
}

func TestRetryingStreamCloseBeforeNextDoesNotStartAttempt(t *testing.T) {
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    2,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		attempts.Add(1)
		return &testStreamIterator{}, nil
	})

	assert.NoError(t, iterator.Close())
	assert.NoError(t, iterator.Close())
	assert.False(t, iterator.Next())
	assert.NoError(t, iterator.Err())
	assert.Equal(t, int64(0), attempts.Load())
}

func TestRetryingStreamCanceledBeforeNextDoesNotStartAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(ctx, StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    2,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		attempts.Add(1)
		return &testStreamIterator{}, nil
	})
	defer iterator.Close()

	assert.False(t, iterator.Next())
	assert.True(t, errors.Is(iterator.Err(), context.Canceled))
	assert.Equal(t, int64(0), attempts.Load())
}

func TestRetryingStreamCancellationFromAttemptStopsRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(ctx, StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    2,
		RetryBaseWait: time.Millisecond,
	}, func() (llm.StreamIterator, error) {
		attempts.Add(1)
		cancel()
		return nil, errors.New("transport stopped")
	})
	defer iterator.Close()

	assert.False(t, iterator.Next())
	assert.True(t, errors.Is(iterator.Err(), context.Canceled))
	assert.Equal(t, int64(1), attempts.Load())
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

func TestRetryingStreamCancellationStopsBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := &retrySignalLogger{retry: make(chan struct{})}
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(ctx, StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    2,
		RetryBaseWait: 10 * time.Second,
		Logger:        logger,
	}, func() (llm.StreamIterator, error) {
		attempts.Add(1)
		return nil, NewError(429, "rate limited")
	})
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
	assert.Equal(t, int64(1), attempts.Load())
}

type captureRetryLogger struct {
	message string
	args    []any
}

func (l *captureRetryLogger) Debug(string, ...any)   {}
func (l *captureRetryLogger) Info(string, ...any)    {}
func (l *captureRetryLogger) Error(string, ...any)   {}
func (l *captureRetryLogger) With(...any) llm.Logger { return l }
func (l *captureRetryLogger) Warn(message string, args ...any) {
	l.message = message
	l.args = append([]any(nil), args...)
}

func TestRetryingStreamLogsAttemptWithoutProviderBody(t *testing.T) {
	logger := &captureRetryLogger{}
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "grok",
		MaxRetries:    1,
		RetryBaseWait: time.Millisecond,
		Logger:        logger,
	}, func() (llm.StreamIterator, error) {
		if attempts.Add(1) == 1 {
			return nil, NewError(429, "sensitive provider response")
		}
		return &testStreamIterator{events: []*llm.Event{{Type: llm.EventTypeMessageStart}}}, nil
	})
	defer iterator.Close()

	assert.True(t, iterator.Next())
	assert.Equal(t, "retrying streaming generation", logger.message)

	fields := make(map[string]any, len(logger.args)/2)
	for i := 0; i+1 < len(logger.args); i += 2 {
		key, ok := logger.args[i].(string)
		if ok {
			fields[key] = logger.args[i+1]
		}
	}
	assert.Equal(t, "grok", fields["provider"])
	assert.Equal(t, 2, fields["attempt"])
	assert.Equal(t, 2, fields["max_attempts"])
	assert.Equal(t, true, fields["before_first_event"])
	assert.Equal(t, http.StatusTooManyRequests, fields["status"])
	assert.False(t, strings.Contains(fmt.Sprint(logger.args...), "sensitive provider response"))
}

func TestRetryingStreamLogsTransportErrorWithoutStatus(t *testing.T) {
	logger := &captureRetryLogger{}
	var attempts atomic.Int64
	iterator := NewRetryingStreamIterator(context.Background(), StreamRetryConfig{
		Provider:      "test",
		MaxRetries:    1,
		RetryBaseWait: time.Millisecond,
		Logger:        logger,
	}, func() (llm.StreamIterator, error) {
		if attempts.Add(1) == 1 {
			return nil, errors.New("connection reset")
		}
		return &testStreamIterator{events: []*llm.Event{{Type: llm.EventTypeMessageStart}}}, nil
	})
	defer iterator.Close()

	assert.True(t, iterator.Next())
	for i := 0; i+1 < len(logger.args); i += 2 {
		assert.False(t, logger.args[i] == "status")
	}
}
