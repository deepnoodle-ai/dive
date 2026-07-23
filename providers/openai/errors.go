package openai

import (
	"errors"
	"strconv"
	"strings"

	"github.com/deepnoodle-ai/dive/providers"
	openaisdk "github.com/openai/openai-go/v3"
)

func normalizeOpenAIError(err error) error {
	var apiErr *openaisdk.Error
	if errors.As(err, &apiErr) {
		return providers.NewError(apiErr.StatusCode, apiErrorMessage(apiErr))
	}
	return err
}

// apiErrorMessage extracts a human-readable message from an SDK API error. The
// SDK parses Message out of the standard OpenAI shape {"error":{"message":...}},
// but OpenAI-compatible providers (e.g. xAI/Grok) return other shapes — such as
// {"code":"permission-denied","error":"<message>"} where "error" is a bare
// string. In those cases Message is empty, so fall back to the raw error payload
// the SDK captured, unquoting it when it is a JSON string, so the underlying
// reason (auth, credits, rate limits) is still surfaced instead of a blank body.
func apiErrorMessage(apiErr *openaisdk.Error) string {
	if apiErr.Message != "" {
		return apiErr.Message
	}
	raw := strings.TrimSpace(apiErr.RawJSON())
	if unquoted, err := strconv.Unquote(raw); err == nil {
		raw = unquoted
	}
	return raw
}
