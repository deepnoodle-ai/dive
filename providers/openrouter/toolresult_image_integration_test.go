//go:build integration

package openrouter

import (
	"context"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/internal/toolimagetest"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestIntegration_ModelSeesToolResultImage asks a live model what a tool's
// screenshot shows, in a plain result and in an error result. OpenRouter
// speaks Chat Completions, so the image is lifted into the user message that
// follows the tool messages.
func TestIntegration_ModelSeesToolResultImage(t *testing.T) {
	if getAPIKey() == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}
	for _, model := range []string{ModelGPT54Mini, ModelClaudeSonnet5} {
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
