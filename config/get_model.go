package config

import (
	"fmt"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/llm/providers/anthropic"
	"github.com/deepnoodle-ai/dive/llm/providers/google"
	"github.com/deepnoodle-ai/dive/llm/providers/grok"
	"github.com/deepnoodle-ai/dive/llm/providers/groq"
	"github.com/deepnoodle-ai/dive/llm/providers/ollama"
	"github.com/deepnoodle-ai/dive/llm/providers/openai"
	"github.com/deepnoodle-ai/dive/llm/providers/openaicompletions"
	"github.com/deepnoodle-ai/dive/llm/providers/openrouter"
)

var DefaultProvider = "anthropic"

func GetModel(providerName, modelName string) (llm.LLM, error) {

	if providerName == "" {
		providerName = DefaultProvider
	}

	switch providerName {
	case "anthropic":
		opts := []anthropic.Option{}
		if modelName != "" {
			opts = append(opts, anthropic.WithModel(modelName))
		}
		return anthropic.New(opts...), nil

	case "openai":
		opts := []openai.Option{}
		if modelName != "" {
			opts = append(opts, openai.WithModel(modelName))
		}
		return openai.New(opts...), nil

	case "openai-completions":
		// New Responses API
		opts := []openaicompletions.Option{}
		if modelName != "" {
			opts = append(opts, openaicompletions.WithModel(modelName))
		}
		return openaicompletions.New(opts...), nil

	case "groq":
		opts := []groq.Option{}
		if modelName != "" {
			opts = append(opts, groq.WithModel(modelName))
		}
		return groq.New(opts...), nil

	case "grok":
		opts := []grok.Option{}
		if modelName != "" {
			opts = append(opts, grok.WithModel(modelName))
		}
		return grok.New(opts...), nil

	case "ollama":
		opts := []ollama.Option{}
		if modelName != "" {
			opts = append(opts, ollama.WithModel(modelName))
		}
		return ollama.New(opts...), nil

	case "google":
		opts := []google.Option{}
		if modelName != "" {
			opts = append(opts, google.WithModel(modelName))
		}
		return google.New(opts...), nil

	case "openrouter":
		opts := []openrouter.Option{}
		if modelName != "" {
			opts = append(opts, openrouter.WithModel(modelName))
		}
		return openrouter.New(opts...), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %q", providerName)
	}
}
