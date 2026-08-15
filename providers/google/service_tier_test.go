package google

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

// TestBuildGenAIGenerateConfigServiceTier pins the field-level contract: an
// unset tier must leave GenerateContentConfig.ServiceTier at its zero value so
// the genai SDK's omitempty drops it from the request body, while an explicit
// tier is carried through. This is the assertion that would have caught the
// v1.21.0 regression where the "unspecified" sentinel reached the wire and
// Vertex AI rejected every generation call with a 400.
func TestBuildGenAIGenerateConfigServiceTier(t *testing.T) {
	genConfig, err := buildGenAIGenerateConfig(&Request{Model: "gemini-3.7-flash"})
	assert.NoError(t, err)
	assert.Equal(t, genai.ServiceTier(""), genConfig.ServiceTier)

	genConfig, err = buildGenAIGenerateConfig(&Request{
		Model:       "gemini-3.7-flash",
		ServiceTier: genai.ServiceTierStandard,
	})
	assert.NoError(t, err)
	assert.Equal(t, genai.ServiceTierStandard, genConfig.ServiceTier)
}

// TestGoogleServiceTierOmittedOnWire is the end-to-end confirmation across the
// whole request chain: when the caller asks for no service tier, the marshalled
// request body must not mention serviceTier at all, on either backend. Omission
// is the only setting both backends accept — the genai SDK's lowercase enum
// spellings are rejected by Vertex AI, and Vertex's SERVICE_TIER_* proto names
// are rejected by the Developer API. Same-package tests inject a
// test-server-backed genai client so no real network call or credential is
// needed; supplying HTTPClient explicitly keeps the Vertex path off ADC.
func TestGoogleServiceTierOmittedOnWire(t *testing.T) {
	newServer := func(captured *[]byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			*captured = body
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(minimalGenerateResponse))
		}))
	}

	t.Run("vertex backend", func(t *testing.T) {
		var capturedBody []byte
		server := newServer(&capturedBody)
		defer server.Close()

		client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
			Backend:     genai.BackendVertexAI,
			Project:     "test-project",
			Location:    "global",
			HTTPClient:  &http.Client{},
			HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
		})
		assert.NoError(t, err)

		p := New(WithModel("gemini-3.7-flash"), WithVertexAI("global"),
			WithProjectID("test-project"), WithMaxRetries(0))
		p.client = client

		_, err = p.Generate(context.Background(),
			llm.WithMessages(llm.NewUserTextMessage("hi")))
		assert.NoError(t, err)
		assert.True(t, len(capturedBody) > 0, "expected the request body to be captured")
		assert.False(t, strings.Contains(string(capturedBody), `"serviceTier"`),
			"serviceTier must be omitted from the Vertex request body, got: %s", capturedBody)
	})

	t.Run("developer api backend", func(t *testing.T) {
		var capturedBody []byte
		server := newServer(&capturedBody)
		defer server.Close()

		client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
			APIKey:      "test-key",
			HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
		})
		assert.NoError(t, err)

		p := New(WithModel("gemini-3.7-flash"), WithMaxRetries(0))
		p.client = client

		_, err = p.Generate(context.Background(),
			llm.WithMessages(llm.NewUserTextMessage("hi")))
		assert.NoError(t, err)
		assert.True(t, len(capturedBody) > 0, "expected the request body to be captured")
		assert.False(t, strings.Contains(string(capturedBody), `"serviceTier"`),
			"serviceTier must be omitted from the Developer API request body, got: %s", capturedBody)
	})

	// Guard against overcorrecting: an explicitly requested tier must still be
	// serialized (the Developer API accepts the SDK's lowercase spellings).
	t.Run("explicit standard is sent", func(t *testing.T) {
		var capturedBody []byte
		server := newServer(&capturedBody)
		defer server.Close()

		client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
			APIKey:      "test-key",
			HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
		})
		assert.NoError(t, err)

		p := New(WithModel("gemini-3.7-flash"), WithMaxRetries(0))
		p.client = client

		_, err = p.Generate(context.Background(),
			llm.WithMessages(llm.NewUserTextMessage("hi")),
			llm.WithServiceTier("standard"))
		assert.NoError(t, err)
		assert.True(t, strings.Contains(string(capturedBody), `"serviceTier":"standard"`),
			"an explicitly requested tier must reach the wire, got: %s", capturedBody)
	})
}
