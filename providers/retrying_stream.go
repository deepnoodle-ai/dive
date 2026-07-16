package providers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/retry"
)

const maxStreamRetryBackoff = 5 * time.Minute

// StreamFactory creates one transport-level streaming attempt for a logical
// provider generation. It must not fire logical-generation hooks or mutate
// caller-owned messages; retries may invoke it more than once.
type StreamFactory func() (llm.StreamIterator, error)

// StreamRetryConfig configures retries before the first event is exposed to a
// stream consumer.
type StreamRetryConfig struct {
	Provider      string
	MaxRetries    int
	RetryBaseWait time.Duration
	Logger        llm.Logger

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

	current   llm.StreamIterator
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
		if s.current.Next() {
			return true
		}
		s.err = s.normalizeError(s.current.Err())
		s.ended = true
		return false
	}

	var hasEvent bool
	err := retry.DoSimple(s.ctx, func() error {
		if s.factory == nil {
			return retry.MarkPermanent(fmt.Errorf("providers: stream factory is nil"))
		}

		stream, err := s.factory()
		if err != nil {
			if stream != nil {
				_ = stream.Close()
			}
			return s.normalizeError(err)
		}
		if stream == nil {
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

		normalizedStreamErr := s.normalizeError(streamErr)
		if normalizedStreamErr == nil {
			if closeErr != nil {
				return s.normalizeError(closeErr)
			}
			s.ended = true
			return nil
		}
		if closeErr != nil {
			return errors.Join(normalizedStreamErr, closeErr)
		}
		return normalizedStreamErr
	},
		retry.WithMaxAttempts(s.config.MaxRetries+1),
		retry.WithBackoff(s.config.RetryBaseWait, maxStreamRetryBackoff),
		retry.WithRetryIf(func(err error) bool {
			if s.ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return !retry.IsPermanent(err)
		}),
		retry.WithOnRetry(func(attempt int, err error, delay time.Duration) {
			s.logRetry(attempt+1, s.config.MaxRetries+1, err, delay)
		}),
	)
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
	})
	return closeErr
}

func (s *retryingStreamIterator) normalizeError(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	if s.ctx != nil && s.ctx.Err() != nil {
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
