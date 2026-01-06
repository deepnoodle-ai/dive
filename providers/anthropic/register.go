package anthropic

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

func init() {
	// Register for claude-* models
	providers.Register(providers.ProviderEntry{
		Name:    "anthropic",
		Match:   providers.PrefixMatcher("claude-"),
		Factory: factory,
	})

	// Register as the fallback provider (for unknown models)
	providers.SetFallback(factory)
}

func factory(model, endpoint string) llm.LLM {
	opts := []Option{WithModel(model)}
	if endpoint != "" {
		opts = append(opts, WithEndpoint(endpoint))
	}
	return New(opts...)
}
