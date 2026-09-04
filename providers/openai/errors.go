package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/deepnoodle-ai/dive/providers"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func normalizeOpenAIError(err error) error {
	var apiErr *openaisdk.Error
	if errors.As(err, &apiErr) {
		// The SDK has already parsed the code out of the envelope, and the body
		// handed to NewError below is only the message, so pass the code
		// explicitly rather than leaving it to be re-parsed. It separates a 429
		// worth retrying ("slow_down") from one that never will be
		// ("insufficient_quota" and the spend limits).
		var header http.Header
		if apiErr.Response != nil {
			header = apiErr.Response.Header
		}
		return providers.NewError(apiErr.StatusCode, apiErrorMessage(apiErr),
			providers.WithErrorCode(apiErr.Code),
			providers.WithErrorHeader(header))
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
	var unquoted string
	if err := json.Unmarshal([]byte(raw), &unquoted); err == nil {
		return unquoted
	}
	return raw
}

// normalizeErrorBodyMiddleware repairs error responses whose "error" member is
// not a JSON object before the SDK tries to parse them.
//
// openai-go unwraps the "error" subtree of a failed response and unmarshals it
// into its apierror.Error struct. OpenAI-compatible providers do not all use
// the standard {"error":{"message":...}} shape — xAI/Grok returns
// {"code":"permission-denied","error":"<message>"} with a bare string. Through
// v3.50 the SDK's decoder tolerated that and left Message empty; from v3.51 it
// returns a *json.UnmarshalTypeError instead of an *openaisdk.Error, which
// costs Dive both the status code and the reason — a 403 for exhausted credits
// arrives as "json: cannot unmarshal string into Go value of type
// apierror.Error" and no longer classifies for retry.
//
// Rewriting the body into the shape the SDK expects keeps the status code, the
// message, and the retry classification intact, and leaves standard-shaped
// responses untouched.
func normalizeErrorBodyMiddleware(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	res, err := next(req)
	if err != nil || res == nil || res.StatusCode < 400 || res.Body == nil {
		return res, err
	}
	body, readErr := io.ReadAll(res.Body)
	// The bytes are already in hand and the body is replaced below, so a close
	// failure here is not actionable.
	_ = res.Body.Close()
	if readErr != nil {
		res.Body = io.NopCloser(bytes.NewReader(nil))
		return res, readErr
	}
	res.Body = io.NopCloser(bytes.NewReader(rewriteNonObjectErrorBody(body)))
	return res, nil
}

// rewriteNonObjectErrorBody returns body with a non-object "error" member
// replaced by {"message": <rendered>}, preserving the provider's other
// top-level fields. Bodies the SDK can already parse are returned unchanged.
func rewriteNonObjectErrorBody(body []byte) []byte {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return body
	}
	raw, ok := envelope["error"]
	if !ok {
		return body
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] == '{' {
		return body // already the shape the SDK expects
	}
	message := string(trimmed)
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		message = text
	}
	replacement, err := json.Marshal(map[string]string{"message": message})
	if err != nil {
		return body
	}
	envelope["error"] = replacement
	rewritten, err := json.Marshal(envelope)
	if err != nil {
		return body
	}
	return rewritten
}
