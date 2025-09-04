package grok

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

func TestProvider_Name(t *testing.T) {
	provider := New()
	name := provider.Name()
	expected := "grok-grok-4"
	require.Equal(t, expected, name)
}

func TestProvider_WithOptions(t *testing.T) {
	provider := New(
		WithModel("grok-3"),
		WithEndpoint("https://custom.x.ai/v1/chat/completions"),
		WithAPIKey("custom-key"),
		WithMaxTokens(8192),
	)

	require.Equal(t, "grok-3", provider.model)
	require.Equal(t, "https://custom.x.ai/v1/chat/completions", provider.endpoint)
	require.Equal(t, "custom-key", provider.apiKey)
	require.Equal(t, 8192, provider.maxTokens)
}

func TestProvider_GetAPIKey(t *testing.T) {
	// Test with no env vars set
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROK_API_KEY", "")
	require.Equal(t, "", getAPIKey())

	// Test with XAI_API_KEY
	t.Setenv("XAI_API_KEY", "xai-key")
	require.Equal(t, "xai-key", getAPIKey())

	// Test with GROK_API_KEY as fallback
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROK_API_KEY", "grok-key")
	require.Equal(t, "grok-key", getAPIKey())

	// Test XAI_API_KEY takes priority
	t.Setenv("XAI_API_KEY", "xai-key")
	t.Setenv("GROK_API_KEY", "grok-key")
	require.Equal(t, "xai-key", getAPIKey())
}