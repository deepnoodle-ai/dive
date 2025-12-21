package openrouter

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

func TestNew(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		provider := New()
		assert.NotNil(t, provider)
		assert.Equal(t, DefaultModel, provider.model)
		assert.Equal(t, DefaultEndpoint, provider.endpoint)
		assert.Equal(t, DefaultMaxTokens, provider.maxTokens)
		assert.NotNil(t, provider.Provider)
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
		assert.NotNil(t, provider)
		assert.Equal(t, "test-key", provider.apiKey)
		assert.Equal(t, "openai/gpt-3.5-turbo", provider.model)
		assert.Equal(t, "https://custom.endpoint.com", provider.endpoint)
		assert.Equal(t, 2048, provider.maxTokens)
		assert.Equal(t, "https://myapp.com", provider.siteURL)
		assert.Equal(t, "My App", provider.siteName)
	})
}

func TestName(t *testing.T) {
	provider := New(WithModel("openai/gpt-4"))
	assert.Equal(t, "openrouter", provider.Name())
}

func TestGetAPIKey(t *testing.T) {
	t.Run("OPENROUTER_API_KEY", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "openrouter-key")
		t.Setenv("OPENAI_API_KEY", "openai-key")

		key := getAPIKey()
		assert.Equal(t, "openrouter-key", key)
	})

	t.Run("fallback to OPENAI_API_KEY", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "openai-key") // ignored

		key := getAPIKey()
		assert.Equal(t, "", key)
	})

	t.Run("no API key", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "")

		key := getAPIKey()
		assert.Equal(t, "", key)
	})
}
