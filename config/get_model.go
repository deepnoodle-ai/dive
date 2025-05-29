package config

import (
	"fmt"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
	"github.com/diveagents/dive/llm/providers/groq"
	"github.com/diveagents/dive/llm/providers/ollama"
	"github.com/diveagents/dive/llm/providers/openai"
	"github.com/diveagents/dive/llm/providers/openaicompletions"
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

	case "ollama":
		opts := []ollama.Option{}
		if modelName != "" {
			opts = append(opts, ollama.WithModel(modelName))
		}
		return ollama.New(opts...), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %q", providerName)
	}
}
