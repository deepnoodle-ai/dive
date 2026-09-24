//go:build integration

package grok

import (
	"context"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/internal/toolimagetest"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestIntegration_ModelSeesToolResultImage asks a live model what a tool's
// screenshot shows, in a plain result and in an error result. Grok
// takes the image inside the function call output, as OpenAI does.
func TestIntegration_ModelSeesToolResultImage(t *testing.T) {
	if getAPIKey() == "" {
		t.Skip("XAI_API_KEY not set")
	}
	for _, model := range []string{DefaultModel} {
		for _, isError := range []bool{false, true} {
			name := model
			if isError {
				name += "/error"
			}
			t.Run(name, func(t *testing.T) {
				scene := toolimagetest.NewScene(t)
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				response, err := New(WithModel(model)).Generate(ctx,
					llm.WithMessages(scene.Messages(isError)...),
					llm.WithTools(scene.Tool()),
				)
				assert.NoError(t, err)
				scene.Check(t, toolimagetest.Reply(response))
			})
		}
	}
}
