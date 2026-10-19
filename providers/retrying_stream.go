package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/retry"
)

const maxStreamRetryBackoff = 5 * time.Minute

// Default stream timeouts. A provider whose stream opens with an event before
// the model starts work, such as Anthropic's message_start, can use a shorter
// first-event timeout.
const (
	DefaultStreamFirstEventTimeout = 5 * time.Minute
	DefaultStreamIdleTimeout       = 5 * time.Minute
)

// ErrStreamFirstEventTimeout is returned when a streaming attempt produces no
// event within StreamRetryConfig.FirstEventTimeout. The attempt is retried.
var ErrStreamFirstEventTimeout = errors.New("stream produced no event before the first-event timeout")

// ErrStreamIdleTimeout is returned when a stream that has started produces no
// event within StreamRetryConfig.IdleTimeout. It is not retried.
var ErrStreamIdleTimeout = errors.New("stream produced no event before the idle timeout")

// StreamFactory creates one transport-level streaming attempt for a logical
// provider generation. It must make its request with ctx, which is cancelled
// when the attempt times out. It must not fire logical-generation hooks or
// mutate caller-owned messages; retries may invoke it more than once.
type StreamFactory func(ctx context.Context) (llm.StreamIterator, error)

// StreamClient returns client with its Timeout removed. http.Client.Timeout
// covers reading the response body, so it would cut off a healthy stream that
// runs long; streams are bounded by StreamRetryConfig's timeouts instead.
func StreamClient(client *http.Client) *http.Client {
	if client == nil || client.Timeout == 0 {
		return client
	}
	copied := *client
	copied.Timeout = 0
	return &copied
}

// StreamRetryConfig configures retries before the first event is exposed to a
// stream consumer.
type StreamRetryConfig struct {
	Provider      string
	MaxRetries    int
	RetryBaseWait time.Duration
	Logger        llm.Logger

	// FirstEventTimeout bounds how long one attempt may take to produce its
	// first event, response headers included. An attempt that runs out is
	// abandoned and retried. Zero means no limit.
	FirstEventTimeout time.Duration

	// IdleTimeout bounds the wait for each event after the first. A stream
	// that goes quiet that long ends with ErrStreamIdleTimeout. Zero means no
	// limit.
	IdleTimeout time.Duration

	// NormalizeError maps SDK-specific errors onto Dive's retryable/permanent
	// error contract. A nil function leaves errors unchanged.
	NormalizeError func(error) error
}

// NewRetryingStreamIterator returns one logical stream backed by fresh
// transport attempts from factory. Factory errors and iterator errors share a
// single retry budget. The first exposed llm.Event commits the current attempt;
// after that point, errors are returned without retry to avoid duplicate output.
func NewRetryingStreamIterator(
	ctx context.Context,
	config StreamRetryConfig,
	factory StreamFactory,
) llm.StreamIterator {
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &retryingStreamIterator{
		ctx:     ctx,
		config:  config,
		factory: factory,
	}
}

type retryingStreamIterator struct {
	ctx     context.Context
	config  StreamRetryConfig
	factory StreamFactory

	current llm.StreamIterator
	// attemptCtx is the current attempt's context. The watchdog cancels it
	// with a timeout error as the cause.
	attemptCtx    context.Context
	cancelAttempt context.CancelCauseFunc
	watchdog      *time.Timer

	committed bool
	ended     bool
	err       error
	closeOnce sync.Once
}

func (s *retryingStreamIterator) Next() bool {
	if s.ended || s.err != nil {
		return false
	}

	if s.committed {
		s.watch(s.config.IdleTimeout, ErrStreamIdleTimeout)
		hasNext := s.current.Next()
		s.stopWatch()
		if hasNext {
			return true
		}
		s.err = s.normalizeError(s.current.Err())
		s.ended = true
		s.endAttempt()
		return false
	}

	var hasEvent bool
	openStream := func() error {
		if s.factory == nil {
			return retry.MarkPermanent(fmt.Errorf("providers: stream factory is nil"))
		}

		s.startAttempt()
		s.watch(s.config.FirstEventTimeout, ErrStreamFirstEventTimeout)
		defer s.stopWatch()

		stream, err := s.factory(s.attemptCtx)
		if err != nil {
			if stream != nil {
				_ = stream.Close()
			}
			err = s.normalizeError(err)
			s.endAttempt()
			return err
		}
		if stream == nil {
			s.endAttempt()
			return retry.MarkPermanent(fmt.Errorf("providers: stream factory returned nil"))
		}
		s.current = stream

		if s.current.Next() {
			// The first exposed event is the commit point. A committed stream is
			// never replaced, even if no content-bearing delta has arrived yet.
			s.committed = true
			hasEvent = true
			return nil
		}

		streamErr := s.current.Err()
		closeErr := s.current.Close()
		s.current = nil // Close every failed attempt before creating a replacement.
		defer s.endAttempt()

		normalizedStreamErr := s.normalizeError(streamErr)
		if normalizedStreamErr == nil {
			if closeErr != nil {
				return s.normalizeError(closeErr)
			}
			s.ended = true
			return nil
		}
		if closeErr != nil {
			return errors.Join(normalizedStreamErr, s.normalizeError(closeErr))
		}
		return normalizedStreamErr
	}

	err := RetryPolicy{
		MaxAttempts: s.config.MaxRetries + 1,
		BaseWait:    s.config.RetryBaseWait,
		MaxWait:     maxStreamRetryBackoff,
		RetryIf: func(err error) bool {
			// The caller's own deadline shows up as s.ctx.Err(). Matching
			// context.DeadlineExceeded on err would also refuse http.Client
			// timeouts, which report as that error while the caller is live.
			// An attempt the watchdog cancelled reports its timeout error
			// instead of context.Canceled (see normalizeError).
			if s.ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return false
			}
			return !retry.IsPermanent(err)
		},
		OnRetry: func(attempt int, err error, delay time.Duration) {
			s.logRetry(attempt+1, s.config.MaxRetries+1, err, delay)
		},
	}.Do(s.ctx, openStream)
	if err != nil {
		s.err = err
		s.ended = true
		return false
	}
	return hasEvent
}

func (s *retryingStreamIterator) Event() *llm.Event {
	if s.current == nil {
		return nil
	}
	return s.current.Event()
}

func (s *retryingStreamIterator) Err() error {
	return s.err
}

func (s *retryingStreamIterator) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		s.ended = true
		if s.current != nil {
			closeErr = s.current.Close()
		}
		s.endAttempt()
	})
	return closeErr
}

// startAttempt gives the next attempt its own context, so the watchdog can
// abandon it without cancelling the caller's.
func (s *retryingStreamIterator) startAttempt() {
	s.attemptCtx, s.cancelAttempt = context.WithCancelCause(s.ctx)
}

// endAttempt releases the current attempt's context.
func (s *retryingStreamIterator) endAttempt() {
	if s.cancelAttempt != nil {
		s.cancelAttempt(nil)
	}
}

// watch cancels the current attempt with cause if it is still waiting after
// timeout. A zero timeout sets no watchdog.
func (s *retryingStreamIterator) watch(timeout time.Duration, cause error) {
	if timeout <= 0 || s.cancelAttempt == nil {
		return
	}
	s.stopWatch()
	cancel := s.cancelAttempt
	s.watchdog = time.AfterFunc(timeout, func() {
		cancel(fmt.Errorf("%w (%s)", cause, timeout))
	})
}

func (s *retryingStreamIterator) stopWatch() {
	if s.watchdog != nil {
		s.watchdog.Stop()
	}
}

func (s *retryingStreamIterator) normalizeError(err error) error {
	// The transport reports a watchdog cancellation as context.Canceled, and
	// an iterator may report it as a clean end. Report the timeout that
	// caused it either way: it is retried before the first event, and after
	// it the stream is interrupted, not complete.
	if s.ctx.Err() == nil && s.attemptCtx != nil && s.attemptCtx.Err() != nil {
		if cause := context.Cause(s.attemptCtx); errors.Is(cause, ErrStreamFirstEventTimeout) || errors.Is(cause, ErrStreamIdleTimeout) {
			return cause
		}
	}
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	if s.config.NormalizeError != nil {
		return s.config.NormalizeError(err)
	}
	return err
}

func (s *retryingStreamIterator) logRetry(attempt, maxAttempts int, err error, delay time.Duration) {
	if s.config.Logger == nil {
		return
	}
	args := []any{
		"provider", s.config.Provider,
		"attempt", attempt,
		"max_attempts", maxAttempts,
		"before_first_event", true,
		"backoff", delay,
	}
	if status := providerStatusCode(err); status != 0 {
		args = append(args, "status", status)
	}
	s.config.Logger.Warn("retrying streaming generation", args...)
}

func providerStatusCode(err error) int {
	var statusErr interface{ StatusCode() int }
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode()
	}
	return 0
}
