package otel

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/providers"
)

// Low-cardinality error.type values for chat operations. The set is kept
// small on purpose — these become metric dimension values, so explosive
// cardinality (e.g. raw error messages) would balloon storage and break
// dashboards. Anything we can't bucket falls back to "_OTHER", matching
// genaiconv.ErrorTypeOther.
const (
	errTypeRateLimit     = "rate_limit"
	errTypeTimeout       = "timeout"
	errTypeContextLength = "context_length"
	errTypeAuth          = "auth"
	errTypeNetwork       = "network"
	errTypeOther         = "_OTHER"
)

// classifyChatError reduces a provider error to a low-cardinality bucket
// suitable for the error.type metric dimension and span attribute. Unwraps
// retry / fmt.Errorf envelopes before inspecting.
func classifyChatError(err error) string {
	if err == nil {
		return ""
	}

	// HTTP status code from the provider — most authoritative signal.
	var perr *providers.ProviderError
	if errors.As(err, &perr) {
		switch perr.StatusCode() {
		case 401, 403:
			return errTypeAuth
		case 408, 504:
			return errTypeTimeout
		case 413:
			return errTypeContextLength
		case 429:
			return errTypeRateLimit
		}
	}

	// Native context cancellation / timeout from the request ctx.
	if errors.Is(err, context.DeadlineExceeded) {
		return errTypeTimeout
	}
	if errors.Is(err, context.Canceled) {
		return errTypeOther
	}

	// Network-layer errors from net/http (DNS, TCP, TLS).
	var nerr net.Error
	if errors.As(err, &nerr) {
		if nerr.Timeout() {
			return errTypeTimeout
		}
		return errTypeNetwork
	}

	// Last-ditch substring sniffing for providers that return free-form
	// strings without a typed error. Cheap and bounded.
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context length") || strings.Contains(msg, "maximum context"):
		return errTypeContextLength
	case strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests"):
		return errTypeRateLimit
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out"):
		return errTypeTimeout
	case strings.Contains(msg, "unauthorized") || strings.Contains(msg, "invalid api key"):
		return errTypeAuth
	}
	return errTypeOther
}

// Tool error type buckets, paralleling the chat-side classifier.
const (
	errTypeToolPanic        = "panic"
	errTypeToolNotFound     = "tool_not_found"
	errTypeToolArgsInvalid  = "args_invalid"
	errTypeToolReturnedErr  = "tool_returned_error"
	errTypeToolDefault      = "tool_error"
)

// classifyToolError reduces a tool execution failure to a low-cardinality
// bucket. The agent's executeTool path differentiates panics, missing
// tools, and arg-decoding failures via distinct error wrappers; this
// function maps them onto the spec-suggested error.type values.
func classifyToolError(r *dive.ToolCallResult) string {
	if r == nil {
		return errTypeToolDefault
	}
	if r.Error != nil {
		msg := strings.ToLower(r.Error.Error())
		switch {
		case strings.Contains(msg, "panic"):
			return errTypeToolPanic
		case strings.Contains(msg, "not found") || strings.Contains(msg, "unknown tool"):
			return errTypeToolNotFound
		case strings.Contains(msg, "invalid") && (strings.Contains(msg, "input") || strings.Contains(msg, "argument") || strings.Contains(msg, "json")):
			return errTypeToolArgsInvalid
		}
		return errTypeToolReturnedErr
	}
	if r.Result != nil && r.Result.IsError {
		return errTypeToolReturnedErr
	}
	return errTypeToolDefault
}
