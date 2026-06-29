package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

// minimalGenerateResponse is a valid (if empty) GenerateContent response body so
// the provider's response conversion succeeds and the call returns without error.
const minimalGenerateResponse = `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{}}`

// requestPayload mirrors the subset of the Gemini GenerateContent request body
// we need to inspect: the function-call parts and their sibling thought
// signatures.
type requestPayload struct {
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			FunctionCall *struct {
				Name string `json:"name"`
			} `json:"functionCall"`
			ThoughtSignature string `json:"thoughtSignature"`
		} `json:"parts"`
	} `json:"contents"`
}

// TestGoogleThoughtSignatureSentOnWire is the end-to-end confirmation of the
// fix: when a prior assistant tool call carries a Google thought signature, the
// actual HTTP request sent for the next turn must include that signature on the
// functionCall part. Without it Gemini 3 rejects the request with HTTP 400
// ("Function call is missing a thought_signature in functionCall parts"). The
// test drives a real genai client against an httptest server and inspects the
// captured request body, exercising the genai SDK's own request serialization
// rather than asserting on intermediate structs.
func TestGoogleThoughtSignatureSentOnWire(t *testing.T) {
	sigA := []byte("signature-for-tool-a")
	sigB := []byte("signature-for-tool-b")

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(minimalGenerateResponse))
	}))
	defer server.Close()

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:      "test-key",
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	assert.NoError(t, err)

	// Same-package test: inject the test-server-backed client so initClient
	// short-circuits and no real network call is made.
	p := New(WithModel("gemini-3.1-flash-lite"), WithMaxRetries(0))
	p.client = client

	// Two parallel function calls in the assistant turn, each with its own
	// signature — mirrors the reported failure ("...position 2") where a second
	// function call was missing its signature.
	messages := []*llm.Message{
		llm.NewUserTextMessage("status?"),
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ToolUseContent{
					ID:    "call_a",
					Name:  "tool_a",
					Input: []byte(`{"command":"status"}`),
					Metadata: llm.ProviderMetadata{
						googleThoughtSignatureMetadataKey: base64.StdEncoding.EncodeToString(sigA),
					},
				},
				&llm.ToolUseContent{
					ID:    "call_b",
					Name:  "tool_b",
					Input: []byte(`{"command":"version"}`),
					Metadata: llm.ProviderMetadata{
						googleThoughtSignatureMetadataKey: base64.StdEncoding.EncodeToString(sigB),
					},
				},
			},
		},
		llm.NewToolResultMessage(
			&llm.ToolResultContent{ToolUseID: "call_a", Content: "ok"},
			&llm.ToolResultContent{ToolUseID: "call_b", Content: "v1"},
		),
	}

	_, err = p.Generate(context.Background(), llm.WithMessages(messages...))
	assert.NoError(t, err)
	assert.True(t, len(capturedBody) > 0, "expected the request body to be captured")

	var payload requestPayload
	assert.NoError(t, json.Unmarshal(capturedBody, &payload))

	// Collect the signature actually serialized for each function call on the
	// wire, decoding back to the raw bytes Gemini originally issued.
	sigByName := map[string][]byte{}
	for _, content := range payload.Contents {
		for _, part := range content.Parts {
			if part.FunctionCall == nil {
				continue
			}
			assert.True(t, part.ThoughtSignature != "",
				"function call %q is missing its thought signature on the wire", part.FunctionCall.Name)
			decoded, derr := base64.StdEncoding.DecodeString(part.ThoughtSignature)
			assert.NoError(t, derr)
			sigByName[part.FunctionCall.Name] = decoded
		}
	}

	assert.Equal(t, sigA, sigByName["tool_a"])
	assert.Equal(t, sigB, sigByName["tool_b"])
}

// TestGoogleNoThoughtSignatureWhenAbsent guards the inverse: a tool call with no
// stored signature must not fabricate one. messagesToContents should succeed and
// leave the function-call part's ThoughtSignature empty.
func TestGoogleNoThoughtSignatureWhenAbsent(t *testing.T) {
	contents, err := messagesToContents([]*llm.Message{
		llm.NewUserTextMessage("hi"),
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ToolUseContent{
					ID:    "call_1",
					Name:  "tool_a",
					Input: []byte(`{}`),
				},
			},
		},
		llm.NewToolResultMessage(&llm.ToolResultContent{ToolUseID: "call_1", Content: "ok"}),
	})
	assert.NoError(t, err)
	assert.NotNil(t, contents[1].Parts[0].FunctionCall)
	assert.Equal(t, 0, len(contents[1].Parts[0].ThoughtSignature))
}
