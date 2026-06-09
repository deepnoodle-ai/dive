package providers

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

type stubLLM struct {
	name string
}

func (s *stubLLM) Name() string {
	return s.name
}

func (s *stubLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	return nil, nil
}

func TestCreateModel_ExplicitProvider(t *testing.T) {
	r := &Registry{}
	r.Register(ProviderEntry{
		Name:  "cloud",
		Match: PrefixesMatcher("mistral-"),
		Factory: func(model, endpoint string) llm.LLM {
			return &stubLLM{name: "cloud:" + model}
		},
	})
	r.Register(ProviderEntry{
		Name:  "ollama",
		Match: PrefixesMatcher("llama", "qwen"),
		Factory: func(model, endpoint string) llm.LLM {
			return &stubLLM{name: "ollama:" + model}
		},
	})

	// Explicit provider syntax bypasses matchers
	result := r.CreateModel("ollama/mistral:7b", "")
	assert.NotNil(t, result)
	assert.Equal(t, "ollama:mistral:7b", result.(*stubLLM).name)

	// Normal matching still works
	result = r.CreateModel("mistral-large-latest", "")
	assert.NotNil(t, result)
	assert.Equal(t, "cloud:mistral-large-latest", result.(*stubLLM).name)

	// Unknown provider returns nil
	result = r.CreateModel("unknown/model", "")
	assert.Nil(t, result)
}

func TestCreateModel_SlashRouting(t *testing.T) {
	r := &Registry{}
	r.Register(ProviderEntry{
		Name:  "openai",
		Match: PrefixesMatcher("gpt-"),
		Factory: func(model, endpoint string) llm.LLM {
			return &stubLLM{name: "openai:" + model}
		},
	})
	r.Register(ProviderEntry{
		Name:  "openrouter",
		Match: ContainsMatcher("/"),
		Factory: func(model, endpoint string) llm.LLM {
			return &stubLLM{name: "openrouter:" + model}
		},
	})

	// A "/"-containing ID whose prefix is NOT a registered provider name falls
	// through to the matcher loop (OpenRouter's ContainsMatcher claims it,
	// receiving the full model ID).
	result := r.CreateModel("meta-llama/llama-3-70b", "")
	assert.NotNil(t, result)
	assert.Equal(t, "openrouter:meta-llama/llama-3-70b", result.(*stubLLM).name)

	// A registered provider prefix wins over matchers: "openai/gpt-4" routes
	// to the native OpenAI provider with the prefix stripped, even though
	// OpenRouter's matcher would also match.
	result = r.CreateModel("openai/gpt-4", "")
	assert.NotNil(t, result)
	assert.Equal(t, "openai:gpt-4", result.(*stubLLM).name)

	// Plain model IDs without "/" are unchanged: matched by prefix matchers.
	result = r.CreateModel("gpt-4", "")
	assert.NotNil(t, result)
	assert.Equal(t, "openai:gpt-4", result.(*stubLLM).name)

	// Unknown model without "/" and no fallback returns nil.
	assert.Nil(t, r.CreateModel("totally-unknown-model", ""))
}

func TestCreateModel_SlashRoutingNoMatcher(t *testing.T) {
	// Without any "/"-matching entry, an unknown-prefix "/" model returns nil
	// (no fallback set).
	r := &Registry{}
	r.Register(ProviderEntry{
		Name:  "openai",
		Match: PrefixesMatcher("gpt-"),
		Factory: func(model, endpoint string) llm.LLM {
			return &stubLLM{name: "openai:" + model}
		},
	})
	assert.Nil(t, r.CreateModel("meta-llama/llama-3-70b", ""))

	// With a fallback, the fallback receives the full model ID.
	r.SetFallback(func(model, endpoint string) llm.LLM {
		return &stubLLM{name: "fallback:" + model}
	})
	result := r.CreateModel("meta-llama/llama-3-70b", "")
	assert.NotNil(t, result)
	assert.Equal(t, "fallback:meta-llama/llama-3-70b", result.(*stubLLM).name)
}
