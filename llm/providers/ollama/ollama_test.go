package ollama

import (
	"testing"

	"github.com/diveagents/dive/llm"
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
	expected := "ollama-llama3.2:3b"
	require.Equal(t, expected, name)
}

func TestProvider_SupportsStreaming(t *testing.T) {
	provider := New()
	require.True(t, provider.SupportsStreaming())
}

func TestProvider_WithOptions(t *testing.T) {
	provider := New(
		WithModel("llama3.1"),
		WithEndpoint("http://custom:11434/v1/chat/completions"),
		WithAPIKey("custom-key"),
		WithMaxTokens(8192),
	)

	require.Equal(t, "llama3.1", provider.model)
	require.Equal(t, "http://custom:11434/v1/chat/completions", provider.endpoint)
	require.Equal(t, "custom-key", provider.apiKey)
	require.Equal(t, 8192, provider.maxTokens)
}
