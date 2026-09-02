package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/wonton/assert"
	openaisdk "github.com/openai/openai-go/v3"
)

// newAPIError builds an SDK error the way the SDK does: it extracts the "error"
// subtree from the response body and unmarshals that into the Error type. The
// subtree is passed through normalizeErrorBodyMiddleware's rewrite first, since
// that is what the SDK sees on the wire once the middleware is installed.
func newAPIError(t *testing.T, statusCode int, errSubtree string) *openaisdk.Error {
	t.Helper()
	rewritten := rewriteNonObjectErrorBody([]byte(`{"error":` + errSubtree + `}`))
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	assert.NoError(t, json.Unmarshal(rewritten, &envelope))
	var e openaisdk.Error
	assert.NoError(t, e.UnmarshalJSON(envelope.Error))
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
	// The middleware lifts the bare string into message, so the SDK populates
	// Message rather than leaving it empty for the RawJSON fallback to rescue.
	assert.Equal(t, msg, apiErr.Message)

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
	assert.Equal(t, "see https://example.com/docs for details", apiErr.Message)

	got := normalizeOpenAIError(apiErr)

	var provErr *providers.ProviderError
	assert.True(t, errors.As(got, &provErr))
	assert.Equal(t, "provider api error (status 403): see https://example.com/docs for details", provErr.Error())
}

func TestNormalizeOpenAIError_NonAPIErrorPassthrough(t *testing.T) {
	orig := errors.New("connection refused")
	assert.Equal(t, orig, normalizeOpenAIError(orig))
}

// TestGrokShapedErrorBodySurvivesTheSDK is the regression guard for the
// openai-go v3.51 decoder change. With a bare-string "error" member the SDK
// stopped producing an *openaisdk.Error at all and returned a raw
// *json.UnmarshalTypeError, which cost the caller both the status code and the
// reason. This drives a real client against a Grok-shaped 403 body.
func TestGrokShapedErrorBodySurvivesTheSDK(t *testing.T) {
	const message = "Your team has either used all available credits or reached its monthly spending limit."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"permission-denied","error":"` + message + `"}`))
	}))
	defer server.Close()

	provider := New(WithAPIKey("test"), WithEndpoint(server.URL), WithMaxRetries(0))
	_, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")))
	assert.Error(t, err)

	var provErr *providers.ProviderError
	assert.True(t, errors.As(err, &provErr), "want a ProviderError, got %T: %v", err, err)
	assert.Equal(t, 403, provErr.StatusCode())
	assert.Contains(t, provErr.Error(), message)
}

// TestRewriteNonObjectErrorBodyLeavesStandardShapesAlone keeps the middleware
// from touching bodies the SDK already parses correctly.
func TestRewriteNonObjectErrorBodyLeavesStandardShapesAlone(t *testing.T) {
	for _, body := range []string{
		`{"error":{"message":"invalid model","type":"invalid_request_error"}}`,
		`{"detail":"no error member here"}`,
		`not json at all`,
	} {
		assert.Equal(t, body, string(rewriteNonObjectErrorBody([]byte(body))))
	}
}
