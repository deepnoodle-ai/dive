package ollama

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

func init() {
	// Register for locally hosted open-weight model families. "mistral-" is
	// deliberately absent: the Mistral provider claims it, so Mistral models on
	// Ollama need the provider selected explicitly.
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
