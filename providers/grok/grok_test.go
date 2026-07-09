package grok

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
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
	expected := "grok"
	assert.Equal(t, expected, name)
}

func TestProvider_DefaultModel(t *testing.T) {
	assert.Equal(t, ModelGrok45, DefaultModel)
}

func TestProvider_GetAPIKey(t *testing.T) {
	// Test with no env vars set
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROK_API_KEY", "")
	assert.Equal(t, "", getAPIKey())

	// Test with XAI_API_KEY
	t.Setenv("XAI_API_KEY", "xai-key")
	assert.Equal(t, "xai-key", getAPIKey())

	// Test with GROK_API_KEY as fallback
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROK_API_KEY", "grok-key")
	assert.Equal(t, "grok-key", getAPIKey())

	// Test XAI_API_KEY takes priority
	t.Setenv("XAI_API_KEY", "xai-key")
	t.Setenv("GROK_API_KEY", "grok-key")
	assert.Equal(t, "xai-key", getAPIKey())
}
