package providers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

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
		assert.Equal(t, shouldRetry(tt.statusCode, ""), tt.want,
			"shouldRetry(%d)", tt.statusCode)
	}
}

func TestNewErrorMarksPermanent(t *testing.T) {
	// Non-retryable status codes are marked permanent so retry loops with
	// retry.SkipPermanent() stop immediately.
	assert.True(t, retry.IsPermanent(NewError(400, "bad request")))
	assert.False(t, retry.IsPermanent(NewError(529, "overloaded")))
}

func TestShouldRetrySkipsTerminalCodes(t *testing.T) {
	// OpenAI returns an exhausted balance and an ordinary rate limit with the
	// same 429, so the code is what separates them.
	assert.False(t, shouldRetry(429, "insufficient_quota"))
	assert.False(t, shouldRetry(429, "credit_balance_exhausted"))
	assert.False(t, shouldRetry(429, "organization_spend_limit_exceeded"))
	assert.False(t, shouldRetry(429, "project_spend_limit_exceeded"))
	assert.False(t, shouldRetry(429, "organization_usage_limit_exceeded"))
	assert.True(t, shouldRetry(429, "slow_down"))
	assert.True(t, shouldRetry(503, "server_is_overloaded"))
}

func TestNewErrorReadsCodeFromBody(t *testing.T) {
	nested := NewError(429, `{"error":{"code":"insufficient_quota","message":"no credits"}}`)
	assert.True(t, retry.IsPermanent(nested))

	// xAI and other OpenAI-compatible providers put the code at the top level.
	flat := NewError(429, `{"code":"insufficient_quota","error":"no credits"}`)
	assert.True(t, retry.IsPermanent(flat))

	assert.False(t, retry.IsPermanent(NewError(429, `{"error":{"code":"slow_down"}}`)))
	assert.False(t, retry.IsPermanent(NewError(429, "not json at all")))
}

func TestNewErrorExplicitCodeWins(t *testing.T) {
	// The OpenAI SDK path passes a message rather than the JSON envelope, so
	// the code has to arrive separately.
	err := NewError(429, "You exceeded your current quota", WithErrorCode("insufficient_quota"))
	assert.True(t, retry.IsPermanent(err))

	var providerErr *ProviderError
	assert.True(t, errors.As(err, &providerErr))
	assert.Equal(t, providerErr.Code(), "insufficient_quota")
}

func TestErrorCarriesRetryAfter(t *testing.T) {
	err := NewError(429, "slow down", WithErrorHeader(http.Header{"Retry-After": []string{"7"}}))
	after, ok := RetryAfter(err)
	assert.True(t, ok)
	assert.Equal(t, after, 7*time.Second)

	// A permanent error still carries the hint through the Permanent wrapper.
	permanent := NewError(429, "no credits",
		WithErrorCode("insufficient_quota"),
		WithErrorHeader(http.Header{"Retry-After": []string{"7"}}))
	after, ok = RetryAfter(permanent)
	assert.True(t, ok)
	assert.Equal(t, after, 7*time.Second)

	_, ok = RetryAfter(NewError(429, "slow down"))
	assert.False(t, ok)
	_, ok = RetryAfter(errors.New("not a provider error"))
	assert.False(t, ok)
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value string
		want  time.Duration
		ok    bool
	}{
		{name: "seconds", value: "30", want: 30 * time.Second, ok: true},
		{name: "fractional seconds", value: "1.5", want: 1500 * time.Millisecond, ok: true},
		{name: "padded", value: " 30 ", want: 30 * time.Second, ok: true},
		{name: "http date", value: "Thu, 03 Sep 2026 12:00:20 GMT", want: 20 * time.Second, ok: true},
		{name: "http date in the past", value: "Thu, 03 Sep 2026 11:59:00 GMT"},
		{name: "zero", value: "0"},
		{name: "negative", value: "-5"},
		{name: "not a number", value: "soon"},
		{name: "infinity", value: "Inf"},
		{name: "empty", value: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			if tt.value != "" {
				header.Set("Retry-After", tt.value)
			}
			got, ok := parseRetryAfter(header, now)
			assert.Equal(t, ok, tt.ok)
			assert.Equal(t, got, tt.want)
		})
	}

	// A response with no headers at all must not panic.
	_, ok := parseRetryAfter(nil, now)
	assert.False(t, ok)
}

func TestRetryPolicyHonorsRetryAfter(t *testing.T) {
	header := http.Header{"Retry-After": []string{"0.05"}}
	var delays []time.Duration
	attempts := 0
	err := RetryPolicy{
		MaxAttempts: 2,
		BaseWait:    time.Hour, // would dominate if the header were ignored
		OnRetry: func(attempt int, err error, delay time.Duration) {
			delays = append(delays, delay)
		},
	}.Do(context.Background(), func() error {
		attempts++
		return NewError(429, "slow down", WithErrorHeader(header))
	})

	assert.Error(t, err)
	assert.Equal(t, attempts, 2)
	assert.Equal(t, delays, []time.Duration{50 * time.Millisecond})
}

func TestRetryPolicyCapsRetryAfter(t *testing.T) {
	// A provider asking for ten minutes must not hang the caller for ten
	// minutes; MaxWait is the ceiling.
	header := http.Header{"Retry-After": []string{"600"}}
	var delays []time.Duration
	err := RetryPolicy{
		MaxAttempts: 2,
		BaseWait:    time.Millisecond,
		MaxWait:     20 * time.Millisecond,
		OnRetry: func(attempt int, err error, delay time.Duration) {
			delays = append(delays, delay)
		},
	}.Do(context.Background(), func() error {
		return NewError(503, "overloaded", WithErrorHeader(header))
	})

	assert.Error(t, err)
	assert.Equal(t, delays, []time.Duration{20 * time.Millisecond})
}

func TestRetryPolicyFallsBackToBackoff(t *testing.T) {
	var delays []time.Duration
	err := RetryPolicy{
		MaxAttempts: 3,
		BaseWait:    10 * time.Millisecond,
		OnRetry: func(attempt int, err error, delay time.Duration) {
			delays = append(delays, delay)
		},
	}.Do(context.Background(), func() error {
		return NewError(503, "overloaded")
	})

	assert.Error(t, err)
	assert.Equal(t, len(delays), 2)
	// Exponential with 10% jitter: roughly 10ms then 20ms, and growing.
	assert.True(t, delays[0] > 0)
	assert.True(t, delays[1] > delays[0])
}

func TestRetryPolicyStopsOnPermanentError(t *testing.T) {
	attempts := 0
	err := RetryPolicy{MaxAttempts: 5, BaseWait: time.Millisecond}.
		Do(context.Background(), func() error {
			attempts++
			return NewError(429, "no credits", WithErrorCode("insufficient_quota"))
		})

	assert.Error(t, err)
	assert.Equal(t, attempts, 1)
}

func TestRetryPolicySucceeds(t *testing.T) {
	attempts := 0
	err := RetryPolicy{MaxAttempts: 3, BaseWait: time.Millisecond}.
		Do(context.Background(), func() error {
			attempts++
			if attempts < 2 {
				return NewError(503, "overloaded")
			}
			return nil
		})

	assert.NoError(t, err)
	assert.Equal(t, attempts, 2)
}
