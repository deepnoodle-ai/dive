package retry

import (
	"context"
	"math"
	"math/rand"
	"time"
)

const (
	MaxRetries = 3
	BaseWait   = 2 * time.Second
)

type retryConfig struct {
	MaxRetries int
	BaseWait   time.Duration
}

type Option func(*retryConfig)

func WithMaxRetries(maxRetries int) Option {
	return func(c *retryConfig) {
		c.MaxRetries = maxRetries
	}
}

func WithBaseWait(baseWait time.Duration) Option {
	return func(c *retryConfig) {
		c.BaseWait = baseWait
	}
}

// RetryableFunc represents a function that can be retried
type RetryableFunc func() error

// Do executes the given function with retry logic
func Do(ctx context.Context, f RetryableFunc, opts ...Option) error {
	var lastError error

	config := &retryConfig{
		MaxRetries: MaxRetries,
		BaseWait:   BaseWait,
	}
	for _, opt := range opts {
		opt(config)
	}

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			backoff := time.Duration(float64(config.BaseWait) * math.Pow(2, float64(attempt-1)))
			jitter := time.Duration(rand.Float64() * float64(backoff) * 0.1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff + jitter):
			}
		}

		if err := f(); err != nil {
			lastError = err
			if IsRecoverable(err) {
				continue
			}
			return err
		}
		return nil
	}
	return lastError
}
