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

	provider := New(WithModel(ModelOpenAIGPT4o))

	// This would require an actual API key to work
	ctx := context.Background()
	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Say 'hello' and nothing else."),
	))
	require.NoError(t, err)
	require.NotNil(t, response)

	require.Equal(t, llm.Assistant, response.Role)
	require.Equal(t, "hello", strings.ToLower(response.Message().Text()))
}
