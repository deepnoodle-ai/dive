package providers

import (
	"fmt"
	"net/http"

	"github.com/deepnoodle-ai/wonton/retry"
)

// ProviderError represents an error returned by an LLM provider API.
type ProviderError struct {
	statusCode int
	body       string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider api error (status %d): %s", e.statusCode, e.body)
}

func (e *ProviderError) StatusCode() int {
	return e.statusCode
}

// NewError creates a new ProviderError. Non-retryable status codes are wrapped
// with retry.MarkPermanent.
func NewError(statusCode int, body string) error {
	err := &ProviderError{statusCode: statusCode, body: body}
	if !shouldRetry(statusCode) {
		return retry.MarkPermanent(err)
	}
	return err
}

// shouldRetry determines if the given status code should trigger a retry
func shouldRetry(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || // 429
		statusCode == http.StatusInternalServerError || // 500
		statusCode == http.StatusServiceUnavailable || // 503
		statusCode == http.StatusGatewayTimeout || // 504
		statusCode == 520 // Cloudflare
}
