//go:build integration

package openai

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/diveagents/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestIntegration_SimpleTextGeneration(t *testing.T) {
	provider := setupIntegrationProvider(t)

	response, err := provider.Generate(
		context.Background(),
		llm.WithUserTextMessage("Say 'Integration test successful' and nothing else"),
		llm.WithTemperature(0.0),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotEmpty(t, response.ID)
	require.Equal(t, llm.Assistant, response.Role)

	text := response.Message().Text()
	require.NotEmpty(t, text)
	require.Equal(t,
		"integration test successful",
		strings.TrimSpace(strings.ToLower(text)))
}

func TestIntegration_MultipleMessages(t *testing.T) {
	provider := setupIntegrationProvider(t)

	response, err := provider.Generate(
		context.Background(),
		llm.WithMessages(
			llm.NewUserTextMessage("Respond with a totally random word"),
			llm.NewAssistantTextMessage("The word is 'flabbergasted'"),
			llm.NewUserTextMessage("Now respond with that word only"),
		),
		llm.WithTemperature(0.0),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	text := strings.TrimSpace(strings.ToLower(response.Message().Text()))
	require.NotEmpty(t, text)
	require.Equal(t, "flabbergasted", text)
}

// TestIntegration_WebSearchTool tests the web search tool functionality
func TestIntegration_WebSearchTool(t *testing.T) {
	provider := setupIntegrationProvider(t)

	webSearchTool := NewWebSearchPreviewTool(
		WebSearchPreviewToolOptions{
			SearchContextSize: "medium",
			UserLocation: &UserLocation{
				Type:    "approximate",
				Country: "US",
			},
		})

	response, err := provider.Generate(
		context.Background(),
		llm.WithUserTextMessage("Search for the latest news on cryptocurrency"),
		llm.WithTools(webSearchTool),
		llm.WithTemperature(0.0),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotEmpty(t, response.Message().Text())

	// The response should contain some information about AI news
	text := strings.ToLower(response.Message().Text())
	require.True(t,
		strings.Contains(text, "bitcoin") || strings.Contains(text, "ethereum"),
		"Response should contain crypto content: %s", response.Message().Text())
}

// TestIntegration_ErrorScenarios tests various error conditions
func TestIntegration_ErrorScenarios(t *testing.T) {
	t.Run("invalid API key", func(t *testing.T) {
		provider := New(WithAPIKey("invalid-key"))

		ctx := context.Background()
		_, err := provider.Generate(ctx, llm.WithUserTextMessage("This should fail"))

		require.Error(t, err)
		require.Contains(t, strings.ToLower(err.Error()), "401")
	})

	t.Run("empty messages", func(t *testing.T) {
		provider := setupIntegrationProvider(t)

		_, err := provider.Generate(context.Background())

		require.Error(t, err)
		require.Contains(t, strings.ToLower(err.Error()), "no messages")
	})

	t.Run("context timeout", func(t *testing.T) {
		provider := setupIntegrationProvider(t)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		_, err := provider.Generate(ctx, llm.WithUserTextMessage("This should timeout"))
		require.Error(t, err)

		errMessage := strings.ToLower(err.Error())
		ok := strings.Contains(errMessage, "timeout") ||
			strings.Contains(errMessage, "context deadline exceeded")
		require.True(t, ok, "Error should indicate timeout: %s", err.Error())
	})
}

// TestIntegration_AdvancedFeatures tests advanced OpenAI-specific features
func TestIntegration_AdvancedFeatures(t *testing.T) {
	t.Run("reasoning effort with o-series model", func(t *testing.T) {
		provider := New(WithModel("o3"))

		response, err := provider.Generate(
			context.Background(),
			llm.WithUserTextMessage("Solve this simple math problem: 15 + 27"),
			llm.WithReasoningEffort("medium"),
		)

		require.NoError(t, err)
		require.NotNil(t, response)
		require.Contains(t, response.Message().Text(), "42")
	})

	t.Run("service tier configuration", func(t *testing.T) {
		provider := setupIntegrationProvider(t)

		response, err := provider.Generate(
			context.Background(),
			llm.WithUserTextMessage("Say 'Service tier test'"),
			llm.WithServiceTier("default"),
			llm.WithTemperature(0.0),
		)

		require.NoError(t, err)
		require.NotNil(t, response)
		require.NotEmpty(t, response.Message().Text())
	})
}

// setupIntegrationProvider creates a provider for integration testing
func setupIntegrationProvider(t *testing.T) *Provider {
	t.Helper()

	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check for API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	// Create provider with reasonable defaults for testing
	return New(
		WithAPIKey(apiKey),
		WithModel("gpt-4o"),
		WithMaxTokens(1000),
	)
}
