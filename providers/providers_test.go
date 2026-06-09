package providers

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/retry"
)

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		statusCode int
		want       bool
	}{
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{520, true},
		{529, true}, // Anthropic overloaded_error
	}
	for _, tt := range tests {
		assert.Equal(t, shouldRetry(tt.statusCode), tt.want,
			"shouldRetry(%d)", tt.statusCode)
	}
}

func TestNewErrorMarksPermanent(t *testing.T) {
	// Non-retryable status codes are marked permanent so retry loops with
	// retry.SkipPermanent() stop immediately.
	assert.True(t, retry.IsPermanent(NewError(400, "bad request")))
	assert.False(t, retry.IsPermanent(NewError(529, "overloaded")))
}
