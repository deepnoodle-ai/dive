package providers

import (
	"fmt"
	"net/http"
)

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

func (e *ProviderError) IsRecoverable() bool {
	return ShouldRetry(e.statusCode)
}

func NewError(statusCode int, body string) *ProviderError {
	return &ProviderError{statusCode: statusCode, body: body}
}

// ShouldRetry determines if the given status code should trigger a retry
func ShouldRetry(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || // 429
		statusCode == http.StatusServiceUnavailable || // 503
		statusCode == http.StatusGatewayTimeout // 504
}
