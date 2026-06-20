package providers_test

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/providers/mistral"
	"github.com/deepnoodle-ai/dive/providers/ollama"
	"github.com/deepnoodle-ai/dive/providers/openaicompletions"
	"github.com/deepnoodle-ai/dive/providers/openrouter"
	"github.com/deepnoodle-ai/wonton/assert"
)

// assertRegistered verifies that importing a provider package registered its
// pricing table with the central registry, by resolving one of its own models.
// (openai/google/grok live in separate modules and are covered by their own
// tests; these are the providers that share the root module.)
func assertRegistered(t *testing.T, name string, table map[string]llm.PricingInfo) {
	t.Helper()
	if len(table) == 0 {
		t.Skipf("%s has no pricing entries", name)
		return
	}
	for model := range table {
		_, ok := providers.PricingFor(model, false)
		assert.True(t, ok, name+" pricing should be registered for "+model)
	}
}

func TestProvidersRegisterPricing(t *testing.T) {
	assertRegistered(t, "anthropic", anthropic.TextModelPricing)
	assertRegistered(t, "openaicompletions", openaicompletions.TextModelPricing)
	assertRegistered(t, "mistral", mistral.TextModelPricing)
	assertRegistered(t, "openrouter", openrouter.TextModelPricing)
	// Ollama runs locally; entries (if any) are free.
	assertRegistered(t, "ollama", ollama.TextModelPricing)
}

func TestPopulateCostForMistralModel(t *testing.T) {
	var model string
	for m, p := range mistral.TextModelPricing {
		if p.InputPrice > 0 {
			model = m
			break
		}
	}
	if model == "" {
		t.Skip("no priced mistral model")
	}
	u := &llm.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}
	llm.PopulateCost(model, false, u)
	assert.NotNil(t, u.Cost, "cost should populate via the registry resolver")
	assert.True(t, u.Cost.Total > 0, "priced model should yield a positive cost")
}
