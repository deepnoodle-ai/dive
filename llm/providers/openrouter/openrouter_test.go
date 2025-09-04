package openrouter

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestProvider_ImplementsInterfaces(t *testing.T) {
	provider := New()

	// Test that it implements LLM interface
	var _ llm.LLM = provider

	// Test that it implements StreamingLLM interface
	var _ llm.StreamingLLM = provider
}

func TestNew(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		provider := New()
		require.NotNil(t, provider)
		require.Equal(t, DefaultModel, provider.model)
		require.Equal(t, DefaultEndpoint, provider.endpoint)
		require.Equal(t, DefaultMaxTokens, provider.maxTokens)
		require.NotNil(t, provider.Provider)
	})

	t.Run("with options", func(t *testing.T) {
		provider := New(
			WithAPIKey("test-key"),
			WithModel("openai/gpt-3.5-turbo"),
			WithEndpoint("https://custom.endpoint.com"),
			WithMaxTokens(2048),
			WithSiteURL("https://myapp.com"),
			WithSiteName("My App"),
		)
		require.NotNil(t, provider)
		require.Equal(t, "test-key", provider.apiKey)
		require.Equal(t, "openai/gpt-3.5-turbo", provider.model)
		require.Equal(t, "https://custom.endpoint.com", provider.endpoint)
		require.Equal(t, 2048, provider.maxTokens)
		require.Equal(t, "https://myapp.com", provider.siteURL)
		require.Equal(t, "My App", provider.siteName)
	})
}

func TestName(t *testing.T) {
	provider := New(WithModel("openai/gpt-4"))
	require.Equal(t, "openrouter-openai/gpt-4", provider.Name())
}

func TestGetAPIKey(t *testing.T) {
	t.Run("OPENROUTER_API_KEY", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "openrouter-key")
		t.Setenv("OPENAI_API_KEY", "openai-key")
		
		key := getAPIKey()
		require.Equal(t, "openrouter-key", key)
	})

	t.Run("fallback to OPENAI_API_KEY", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "openai-key")
		
		key := getAPIKey()
		require.Equal(t, "openai-key", key)
	})

	t.Run("no API key", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "")
		
		key := getAPIKey()
		require.Equal(t, "", key)
	})
}