package web

import "fmt"

// FetchError contains information about a fetch error.
type FetchError struct {
	StatusCode int
	Err        error
}

// NewFetchError creates a new FetchError with the given status code and error.
func NewFetchError(statusCode int, err error) *FetchError {
	return &FetchError{StatusCode: statusCode, Err: err}
}

func (e *FetchError) Error() string {
	return fmt.Sprintf("fetch failed with status code %d: %s", e.StatusCode, e.Err)
}

func (e *FetchError) Unwrap() error {
	return e.Err
}

func (e *FetchError) IsRecoverable() bool {
	return e.StatusCode == 429 || // Too Many Requests
		e.StatusCode == 500 || // Internal Server Error
		e.StatusCode == 502 || // Bad Gateway
		e.StatusCode == 503 || // Service Unavailable
		e.StatusCode == 504 // Gateway Timeout
}
