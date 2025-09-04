package openrouter

import (
	"context"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestOpenRouterIntegration(t *testing.T) {
	// Skip if no API key is available
	if getAPIKey() == "" {
		t.Skip("Skipping integration test: no OPENROUTER_API_KEY set")
	}

	provider := New(WithModel("openai/gpt-3.5-turbo"))

	// This would require an actual API key to work
	ctx := context.Background()
	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Say 'hello' and nothing else."),
	))
	require.NotNil(t, response)
	require.NoError(t, err)

	require.Equal(t, llm.Assistant, response.Role)
	require.Equal(t, "hello", strings.ToLower(response.Message().Text()))
}
