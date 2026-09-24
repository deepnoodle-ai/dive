//go:build integration

package google

import (
	"context"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/internal/toolimagetest"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestIntegration_ModelSeesToolResultImage asks a live model what a tool's
// screenshot shows, in a plain result and in an error result. Gemini 3
// takes the image inside the function response; Gemini 2.5 rejects that, so
// the image is lifted into the user turn after the function responses.
//
// The conversation is written by hand rather than produced by the model, so
// its function call carries the signature Gemini documents for that case.
func TestIntegration_ModelSeesToolResultImage(t *testing.T) {
	if os.Getenv("GEMINI_API_KEY") == "" && os.Getenv("GOOGLE_API_KEY") == "" {
		t.Skip("GEMINI_API_KEY not set")
	}
	for _, model := range []string{DefaultModel, ModelGemini25Flash} {
		for _, isError := range []bool{false, true} {
			name := model
			if isError {
				name += "/error"
			}
			t.Run(name, func(t *testing.T) {
				scene := toolimagetest.NewScene(t)
				messages := scene.Messages(isError)
				messages[1].Content[0].(*llm.ToolUseContent).Metadata = llm.ProviderMetadata{
					googleThoughtSignatureMetadataKey: base64.StdEncoding.EncodeToString([]byte("skip_thought_signature_validator")),
				}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				response, err := New(WithModel(model)).Generate(ctx,
					llm.WithMessages(messages...),
					llm.WithTools(scene.Tool()),
				)
				assert.NoError(t, err)
				scene.Check(t, toolimagetest.Reply(response))
			})
		}
	}
}
