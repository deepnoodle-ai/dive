package mistral

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

func init() {
	providers.Register(providers.ProviderEntry{
		Name:    "mistral",
		Match:   providers.PrefixesMatcher("mistral-", "ministral-", "codestral-", "devstral-"),
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
