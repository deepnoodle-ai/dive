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
