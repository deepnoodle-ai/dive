package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetModel_GrokProvider(t *testing.T) {
	// Test default grok model
	provider, err := GetModel("grok", "")
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.Equal(t, "grok", provider.Name())

	// Test custom grok model
	provider, err = GetModel("grok", "grok-3")
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.Equal(t, "grok", provider.Name())

	// Test grok-code-fast-1 model
	provider, err = GetModel("grok", "grok-code-fast-1")
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.Equal(t, "grok", provider.Name())
}

func TestGetModel_AllProviders(t *testing.T) {
	providers := []string{"anthropic", "openai", "openai-completions", "groq", "grok", "ollama", "google"}

	for _, providerName := range providers {
		t.Run(providerName, func(t *testing.T) {
			provider, err := GetModel(providerName, "")
			require.NoError(t, err, "provider %s should be supported", providerName)
			require.NotNil(t, provider, "provider %s should return a valid instance", providerName)
			require.NotEmpty(t, provider.Name(), "provider %s should have a non-empty name", providerName)
		})
	}
}

func TestGetModel_OpenRouter(t *testing.T) {
	t.Run("openrouter provider", func(t *testing.T) {
		llm, err := GetModel("openrouter", "")
		require.NoError(t, err)
		require.NotNil(t, llm)
		require.Contains(t, llm.Name(), "openrouter")
	})

	t.Run("unsupported provider", func(t *testing.T) {
		_, err := GetModel("invalid-provider", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported provider")
	})
}
