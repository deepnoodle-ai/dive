package ollama

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

func init() {
	// Register for locally hosted open-weight model families. Mistral is
	// deliberately absent — those models are served through the Mistral
	// provider, and claiming the prefix here would only make routing ambiguous.
	providers.Register(providers.ProviderEntry{
		Name: "ollama",
		Match: providers.PrefixesMatcher(
			"llama", "codellama",
			"mixtral",
			"gemma",
			"glm-",
			"gpt-oss",
			"qwen",
			"phi",
			"deepseek",
		),
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
