package google

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

func init() {
	providers.Register(providers.ProviderEntry{
		Name:    "google",
		Match:   providers.PrefixMatcher("gemini-"),
		Factory: factory,
	})
}

func factory(model, endpoint string) llm.LLM {
	opts := []Option{WithModel(model)}
	// Note: Google provider doesn't support custom endpoints in the same way
	// It uses WithAPIKey, WithProjectID, WithLocation instead
	return New(opts...)
}
