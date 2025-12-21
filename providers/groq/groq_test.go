package groq

import (
	"context"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestHelloWorld(t *testing.T) {
	ctx := context.Background()
	provider := New()

	message := llm.NewUserTextMessage("respond with \"hello\"")
	response, err := provider.Generate(ctx, llm.WithMessages(message))
	assert.NoError(t, err)

	text := strings.ToLower(response.Message().Text())
	assert.Contains(t, text, "hello")
}
