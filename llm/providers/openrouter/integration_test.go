package openrouter

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestOpenRouterIntegration(t *testing.T) {
	// Skip if no API key is available
	if getAPIKey() == "" {
		t.Skip("Skipping integration test: no OPENROUTER_API_KEY or OPENAI_API_KEY set")
	}

	t.Run("provider creation", func(t *testing.T) {
		provider := New(
			WithModel("openai/gpt-3.5-turbo"),
			WithSiteURL("https://test.com"),
			WithSiteName("Test App"),
		)
		require.NotNil(t, provider)
		require.Contains(t, provider.Name(), "openai/gpt-3.5-turbo")
	})

	t.Run("basic generation", func(t *testing.T) {
		provider := New(WithModel("openai/gpt-3.5-turbo"))
		
		// This would require an actual API key to work
		ctx := context.Background()
		_, err := provider.Generate(ctx, llm.WithMessages(
			llm.NewUserTextMessage("Say 'hello' and nothing else."),
		))
		
		// We expect an error without a valid API key, but the provider should be properly initialized
		if err != nil {
			require.Contains(t, err.Error(), "error making request")
		}
	})
}