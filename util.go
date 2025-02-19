package agents

import "errors"

func isRetryableError(err error, retryableErrors []error) bool {
	if len(retryableErrors) == 0 {
		return true // If no specific errors are specified, retry all errors
	}
	for _, retryableErr := range retryableErrors {
		if errors.Is(err, retryableErr) {
			return true
		}
	}
	return false
}
