package ollama

import (
	"testing"

	"github.com/diveagents/dive/llm"
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
	expected := "ollama-llama3.2"
	if name != expected {
		t.Errorf("Expected name %s, got %s", expected, name)
	}
}

func TestProvider_SupportsStreaming(t *testing.T) {
	provider := New()
	if !provider.SupportsStreaming() {
		t.Error("Expected provider to support streaming")
	}
}

func TestProvider_WithOptions(t *testing.T) {
	provider := New(
		WithModel("llama3.1"),
		WithEndpoint("http://custom:11434/v1/chat/completions"),
		WithAPIKey("custom-key"),
		WithMaxTokens(8192),
	)

	if provider.model != "llama3.1" {
		t.Errorf("Expected model llama3.1, got %s", provider.model)
	}

	if provider.endpoint != "http://custom:11434/v1/chat/completions" {
		t.Errorf("Expected custom endpoint, got %s", provider.endpoint)
	}

	if provider.apiKey != "custom-key" {
		t.Errorf("Expected custom API key, got %s", provider.apiKey)
	}

	if provider.maxTokens != 8192 {
		t.Errorf("Expected max tokens 8192, got %d", provider.maxTokens)
	}
}
