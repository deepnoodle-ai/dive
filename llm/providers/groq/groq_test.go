package groq

import (
	"context"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestHelloWorld(t *testing.T) {
	ctx := context.Background()
	provider := New()

	message := llm.NewUserTextMessage("respond with \"hello\"")
	response, err := provider.Generate(ctx, llm.WithMessages(message))
	require.NoError(t, err)

	text := strings.ToLower(response.Message().Text())
	require.Contains(t, text, "hello")
}
