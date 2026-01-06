package openrouter

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

func init() {
	// Models with "/" are OpenRouter format (e.g., "openai/gpt-4", "google/gemini-pro")
	providers.Register(providers.ProviderEntry{
		Name:    "openrouter",
		Match:   providers.ContainsMatcher("/"),
		Factory: factory,
	})
}

func factory(model, endpoint string) llm.LLM {
	opts := []Option{WithModel(model)}
	if endpoint != "" {
		opts = append(opts, WithEndpoint(endpoint))
	}
	return New(opts...)
}
