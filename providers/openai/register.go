package openai

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

func init() {
	// OpenAI Responses API models
	providers.Register(providers.ProviderEntry{
		Name:    "openai",
		Match:   providers.PrefixesMatcher("gpt-", "o3", "o4", "codex"),
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
