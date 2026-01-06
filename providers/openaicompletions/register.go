package openaicompletions

import (
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

func init() {
	// Explicit OpenAI Completions API (prefix: openai-completions:)
	providers.Register(providers.ProviderEntry{
		Name:    "openai-completions",
		Match:   providers.PrefixMatcher("openai-completions:"),
		Factory: factory,
	})
}

func factory(model, endpoint string) llm.LLM {
	// Strip the "openai-completions:" prefix
	actualModel := strings.TrimPrefix(model, "openai-completions:")
	opts := []Option{WithModel(actualModel)}
	if endpoint != "" {
		opts = append(opts, WithEndpoint(endpoint))
	}
	return New(opts...)
}
