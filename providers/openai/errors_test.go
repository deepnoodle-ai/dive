package openai

import (
	"errors"
	"testing"

	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/wonton/assert"
	openaisdk "github.com/openai/openai-go/v3"
)

// newAPIError builds an SDK error the way the SDK does: it extracts the "error"
// subtree from the response body and unmarshals that into the Error type.
func newAPIError(t *testing.T, statusCode int, errSubtree string) *openaisdk.Error {
	t.Helper()
	var e openaisdk.Error
	err := e.UnmarshalJSON([]byte(errSubtree))
	assert.NoError(t, err)
	e.StatusCode = statusCode
	return &e
}

func TestNormalizeOpenAIError_StandardMessage(t *testing.T) {
	// Standard OpenAI shape: {"error":{"message":"...","type":"..."}}.
	apiErr := newAPIError(t, 400, `{"message":"invalid model","type":"invalid_request_error"}`)

	got := normalizeOpenAIError(apiErr)

	var provErr *providers.ProviderError
	assert.True(t, errors.As(got, &provErr))
	assert.Equal(t, 400, provErr.StatusCode())
	assert.Contains(t, provErr.Error(), "invalid model")
}

func TestNormalizeOpenAIError_StringErrorFallback(t *testing.T) {
	// xAI/Grok shape: {"code":"permission-denied","error":"<string>"}. The SDK
	// hands UnmarshalJSON the bare "error" string, leaving Message empty. The
	// message must still surface via the RawJSON fallback, unquoted — not as
	// the raw quoted JSON literal.
	msg := "Your team has either used all available credits or reached its monthly spending limit."
	apiErr := newAPIError(t, 403, `"`+msg+`"`)
	assert.Equal(t, "", apiErr.Message) // precondition: Message really is empty

	got := normalizeOpenAIError(apiErr)

	var provErr *providers.ProviderError
	assert.True(t, errors.As(got, &provErr))
	assert.Equal(t, 403, provErr.StatusCode())
	assert.Equal(t, `provider api error (status 403): `+msg, provErr.Error())
}

func TestNormalizeOpenAIError_StringErrorFallback_JSONEscapes(t *testing.T) {
	// The raw payload is JSON, not a Go string literal — escapes like \/ are
	// valid JSON but not valid Go escape sequences, so the fallback must use
	// a JSON-aware unquote rather than strconv.Unquote.
	apiErr := newAPIError(t, 403, `"see https:\/\/example.com\/docs for details"`)
	assert.Equal(t, "", apiErr.Message)

	got := normalizeOpenAIError(apiErr)

	var provErr *providers.ProviderError
	assert.True(t, errors.As(got, &provErr))
	assert.Equal(t, "provider api error (status 403): see https://example.com/docs for details", provErr.Error())
}

func TestNormalizeOpenAIError_NonAPIErrorPassthrough(t *testing.T) {
	orig := errors.New("connection refused")
	assert.Equal(t, orig, normalizeOpenAIError(orig))
}
