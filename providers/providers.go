package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deepnoodle-ai/wonton/retry"
)

// defaultMaxRetryWait bounds the delay between attempts, including a delay a
// provider asked for with Retry-After.
const defaultMaxRetryWait = 5 * time.Minute

// terminalErrorCodes are provider error codes that never succeed on retry even
// though their status code normally would. OpenAI returns all of these with
// 429, the same status as an ordinary rate limit, so the status code alone
// cannot tell a request that is merely too fast from an account that is out of
// money. Retrying a spend limit burns the attempt budget and delays the error
// the caller needs to see.
var terminalErrorCodes = map[string]bool{
	"credit_balance_exhausted":          true,
	"insufficient_quota":                true,
	"organization_spend_limit_exceeded": true,
	"organization_usage_limit_exceeded": true,
	"project_spend_limit_exceeded":      true,
}

// ProviderError represents an error returned by an LLM provider API.
type ProviderError struct {
	statusCode int
	body       string
	code       string
	retryAfter time.Duration
	hasRetry   bool
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider api error (status %d): %s", e.statusCode, e.body)
}

func (e *ProviderError) StatusCode() int {
	return e.statusCode
}

// Code returns the provider's machine-readable error code, e.g. "slow_down" or
// "server_is_overloaded". It is empty when the provider did not supply one.
func (e *ProviderError) Code() string {
	return e.code
}

// RetryAfter returns the delay the provider asked for in its Retry-After
// header, and whether it sent one.
func (e *ProviderError) RetryAfter() (time.Duration, bool) {
	return e.retryAfter, e.hasRetry
}

// ErrorOption supplies additional detail about a failed provider response.
type ErrorOption func(*ProviderError)

// WithErrorCode sets the provider's machine-readable error code. Use it when
// the code has already been parsed — an SDK error value, say — since the body
// passed to NewError is then a message rather than the raw JSON envelope.
func WithErrorCode(code string) ErrorOption {
	return func(e *ProviderError) {
		e.code = code
	}
}

// WithErrorHeader supplies the failed response's headers, so a Retry-After
// hint is carried on the error. A nil header is ignored.
func WithErrorHeader(header http.Header) ErrorOption {
	return func(e *ProviderError) {
		if after, ok := parseRetryAfter(header, time.Now()); ok {
			e.retryAfter, e.hasRetry = after, true
		}
	}
}

// NewError creates a new ProviderError. Non-retryable responses are wrapped
// with retry.MarkPermanent.
func NewError(statusCode int, body string, opts ...ErrorOption) error {
	providerErr := &ProviderError{statusCode: statusCode, body: body}
	for _, opt := range opts {
		opt(providerErr)
	}
	if providerErr.code == "" {
		providerErr.code = errorCodeFromBody(body)
	}
	if !shouldRetry(statusCode, providerErr.code) {
		return retry.MarkPermanent(providerErr)
	}
	return providerErr
}

// RetryAfter reports the delay a provider asked for on err, if any.
func RetryAfter(err error) (time.Duration, bool) {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return 0, false
	}
	return providerErr.RetryAfter()
}

// shouldRetry determines whether a response should trigger a retry.
func shouldRetry(statusCode int, code string) bool {
	if terminalErrorCodes[code] {
		return false
	}
	return statusCode == http.StatusTooManyRequests || // 429
		statusCode == http.StatusInternalServerError || // 500
		statusCode == http.StatusBadGateway || // 502
		statusCode == http.StatusServiceUnavailable || // 503
		statusCode == http.StatusGatewayTimeout || // 504
		statusCode == 520 || // Cloudflare
		statusCode == 529 // Anthropic overloaded_error
}

// errorCodeFromBody pulls the error code out of a JSON error body. It accepts
// both the nested shape OpenAI and Anthropic use, {"error":{"code":...}}, and
// the flat {"code":...} some OpenAI-compatible providers return. A body that
// is a plain message rather than JSON yields an empty code.
func errorCodeFromBody(body string) string {
	// The members are decoded one at a time rather than into a single struct,
	// because a provider that sends "error" as a bare string would fail the
	// whole unmarshal and lose a top-level "code" sitting beside it.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return ""
	}
	var nested struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(envelope["error"], &nested); err == nil && nested.Code != "" {
		return nested.Code
	}
	var code string
	if err := json.Unmarshal(envelope["code"], &code); err == nil {
		return code
	}
	return ""
}

// parseRetryAfter interprets a Retry-After header in both forms RFC 9110
// allows: a delay in seconds, or the HTTP-date at which to retry. Fractional
// seconds are accepted because providers send them even though the grammar
// calls for an integer.
func parseRetryAfter(header http.Header, now time.Time) (time.Duration, bool) {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			return 0, false
		}
		return time.Duration(seconds * float64(time.Second)), true
	}
	if deadline, err := http.ParseTime(value); err == nil {
		if wait := deadline.Sub(now); wait > 0 {
			return wait, true
		}
		return 0, false
	}
	return 0, false
}

// RetryPolicy describes how a provider retries a failed request.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts, including the first.
	MaxAttempts int

	// BaseWait is the delay after the first failure. Later delays grow
	// exponentially from it.
	BaseWait time.Duration

	// MaxWait caps the delay between attempts. Defaults to five minutes.
	MaxWait time.Duration

	// RetryIf decides whether an error is worth another attempt. Defaults to
	// retrying everything not marked permanent.
	RetryIf func(error) bool

	// OnRetry is called before each retry, for logging.
	OnRetry func(attempt int, err error, delay time.Duration)
}

// Do runs fn under the policy. When the provider answered with a Retry-After
// header, that delay is used in place of the exponential backoff for the next
// attempt, capped at MaxWait. Providers send it on the errors where guessing
// is worst — a 429 for ramping traffic too quickly, a 503 for an overloaded
// model — and returning sooner than asked prolongs the condition.
//
// A RetryPolicy value drives one retry loop at a time: Do records the most
// recent error so the delay can read its Retry-After, and the delay function
// the retry package calls receives only an attempt number. The retry loop is
// sequential, so this needs no synchronization, but concurrent requests must
// not share a single Do call.
func (p RetryPolicy) Do(ctx context.Context, fn func() error) error {
	maxWait := p.MaxWait
	if maxWait <= 0 {
		maxWait = defaultMaxRetryWait
	}
	retryIf := p.RetryIf
	if retryIf == nil {
		retryIf = retry.SkipPermanent()
	}
	var lastErr error
	options := []retry.Option{
		retry.WithMaxAttempts(p.MaxAttempts),
		retry.WithBackoff(p.BaseWait, maxWait),
		retry.WithRetryIf(retryIf),
		retry.WithDelayFunc(func(attempt int, cfg *retry.Config) time.Duration {
			if after, ok := RetryAfter(lastErr); ok {
				return min(after, cfg.MaxBackoff)
			}
			return retry.ExponentialBackoff(attempt, cfg)
		}),
	}
	if p.OnRetry != nil {
		options = append(options, retry.WithOnRetry(p.OnRetry))
	}
	return retry.DoSimple(ctx, func() error {
		err := fn()
		lastErr = err
		return err
	}, options...)
}
