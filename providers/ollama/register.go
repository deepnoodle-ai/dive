package ollama

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

func init() {
	// Register for llama/mixtral/gemma models
	providers.Register(providers.ProviderEntry{
		Name: "ollama",
		Match: providers.PrefixesMatcher(
			"llama", "codellama",
			"mixtral",
			"gemma",
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
