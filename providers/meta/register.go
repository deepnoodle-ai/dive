package meta

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

func init() {
	providers.Register(providers.ProviderEntry{
		Name: "meta",
		// Every Model API family is named "muse-*"; matching the family rather
		// than the vendor keeps "meta-llama/..." OpenRouter ids falling through
		// to OpenRouter's matcher, which is where those are actually served.
		Match:   providers.PrefixMatcher("muse-"),
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
