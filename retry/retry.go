package retry

import (
	"context"
	"math"
	"math/rand"
	"net/http"
	"time"
)

const (
	MaxRetries    = 3
	RetryBaseWait = 1 * time.Second
)

// RetryableFunc represents a function that can be retried
type RetryableFunc func() error

// WithRetry executes the given function with retry logic
func WithRetry(ctx context.Context, f RetryableFunc) error {
	var lastError error

	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			backoff := time.Duration(float64(RetryBaseWait) * math.Pow(2, float64(attempt-1)))
			jitter := time.Duration(rand.Float64() * float64(backoff) * 0.1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff + jitter):
			}
		}

		if err := f(); err != nil {
			lastError = err
			if apiErr, ok := err.(APIError); ok && !ShouldRetry(apiErr.StatusCode()) {
				return err
			}
			continue
		}
		return nil
	}
	return lastError
}

// ShouldRetry determines if the given status code should trigger a retry
func ShouldRetry(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || // 429
		statusCode == http.StatusServiceUnavailable || // 503
		statusCode == http.StatusGatewayTimeout // 504
}

// APIError interface for errors that contain HTTP status codes
type APIError interface {
	error
	StatusCode() int
}
