package openai

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestOpenAIPricingRegistered(t *testing.T) {
	for model := range TextModelPricing {
		_, ok := providers.PricingFor(model, false)
		assert.True(t, ok, "openai pricing should be registered: "+model)
		return
	}
	t.Skip("no openai pricing entries")
}

func TestOpenAIPopulateCost(t *testing.T) {
	var model string
	for m, p := range TextModelPricing {
		if p.InputPrice > 0 {
			model = m
			break
		}
	}
	if model == "" {
		t.Skip("no priced openai model")
	}
	u := &llm.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}
	llm.PopulateCost(model, false, u)
	assert.NotNil(t, u.Cost, "cost should populate via the registry resolver")
	assert.True(t, u.Cost.Total > 0, "priced model should yield positive cost")
}
