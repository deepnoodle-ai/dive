package openairesponses

import (
	"testing"

	"github.com/diveagents/dive/llm"
)

func TestNew(t *testing.T) {
	provider := New()

	if provider == nil {
		t.Fatal("Expected provider to be created")
	}

	if provider.model != DefaultModel {
		t.Errorf("Expected default model %s, got %s", DefaultModel, provider.model)
	}

	if provider.endpoint != DefaultEndpoint {
		t.Errorf("Expected default endpoint %s, got %s", DefaultEndpoint, provider.endpoint)
	}
}

func TestWithOptions(t *testing.T) {
	customModel := "gpt-4o"
	customEndpoint := "https://custom.endpoint.com"

	provider := New(
		WithModel(customModel),
		WithEndpoint(customEndpoint),
		WithStore(true),
		WithBackground(false),
	)

	if provider.model != customModel {
		t.Errorf("Expected model %s, got %s", customModel, provider.model)
	}

	if provider.endpoint != customEndpoint {
		t.Errorf("Expected endpoint %s, got %s", customEndpoint, provider.endpoint)
	}

	if provider.store == nil || !*provider.store {
		t.Error("Expected store to be true")
	}

	if provider.background == nil || *provider.background {
		t.Error("Expected background to be false")
	}
}

func TestWebSearchOptions(t *testing.T) {
	provider := New(
		WithWebSearchOptions(WebSearchOptions{
			SearchContextSize: "large",
			Domains:           []string{"example.com"},
		}),
	)

	if len(provider.enabledTools) != 1 || provider.enabledTools[0] != "web_search_preview" {
		t.Error("Expected web_search_preview to be enabled")
	}

	if provider.webSearchOptions == nil {
		t.Fatal("Expected web search options to be set")
	}

	if provider.webSearchOptions.SearchContextSize != "large" {
		t.Errorf("Expected search context size 'large', got %s", provider.webSearchOptions.SearchContextSize)
	}
}

func TestImageGenerationOptions(t *testing.T) {
	provider := New(
		WithImageGenerationOptions(ImageGenerationOptions{
			Size:    "1024x1024",
			Quality: "high",
		}),
	)

	if len(provider.enabledTools) != 1 || provider.enabledTools[0] != "image_generation" {
		t.Error("Expected image_generation to be enabled")
	}

	if provider.imageGenerationOptions == nil {
		t.Fatal("Expected image generation options to be set")
	}

	if provider.imageGenerationOptions.Size != "1024x1024" {
		t.Errorf("Expected size '1024x1024', got %s", provider.imageGenerationOptions.Size)
	}
}

func TestMCPServerOptions(t *testing.T) {
	provider := New(
		WithMCPServer("test", "https://test.com", map[string]string{"key": "value"}),
	)

	if provider.mcpServers == nil {
		t.Fatal("Expected MCP servers to be initialized")
	}

	config, exists := provider.mcpServers["test"]
	if !exists {
		t.Fatal("Expected MCP server 'test' to be configured")
	}

	if config.ServerURL != "https://test.com" {
		t.Errorf("Expected server URL 'https://test.com', got %s", config.ServerURL)
	}

	if config.Headers["key"] != "value" {
		t.Error("Expected header 'key' to have value 'value'")
	}
}

func TestImplementsInterfaces(t *testing.T) {
	provider := New()

	// Test that provider implements LLM interface
	var _ llm.LLM = provider

	// Test that provider implements StreamingLLM interface
	var _ llm.StreamingLLM = provider
}

func TestSupportsStreaming(t *testing.T) {
	provider := New()

	if !provider.SupportsStreaming() {
		t.Error("Expected provider to support streaming")
	}
}

func TestName(t *testing.T) {
	provider := New(WithModel("gpt-4o"))

	expected := "openai-responses-gpt-4o"
	if provider.Name() != expected {
		t.Errorf("Expected name %s, got %s", expected, provider.Name())
	}
}
