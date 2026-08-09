package grok

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestGrok45Pricing(t *testing.T) {
	p, ok := TextModelPricing[ModelGrok45]
	assert.True(t, ok, "pricing should exist for "+ModelGrok45)
	assert.Equal(t, 2.0, p.InputPrice)
	assert.Equal(t, 0.5, p.CacheReadPrice)
	assert.Equal(t, 6.0, p.OutputPrice)
}

func TestGrokPricingRegistered(t *testing.T) {
	for model := range TextModelPricing {
		_, ok := providers.PricingFor(model, false)
		assert.True(t, ok, "grok pricing should be registered: "+model)
	}
}

// xAI redirects these slugs and bills at the target's rates, so quoting the
// model's own retired price understates a Grok 4 Fast call more than fivefold.
func TestRetiredSlugsAreCostedAtTheirRedirectTarget(t *testing.T) {
	grok43 := TextModelPricing[ModelGrok43]
	for _, model := range []string{
		ModelGrok3,
		ModelGrok40709,
		ModelGrok41FastReasoning,
		ModelGrok41FastNonReasoning,
		ModelGrok4FastReasoning,
		ModelGrok4FastNonReasoning,
	} {
		p, ok := TextModelPricing[model]
		assert.True(t, ok, "pricing should exist for "+model)
		assert.Equal(t, grok43.InputPrice, p.InputPrice, model+" input")
		assert.Equal(t, grok43.OutputPrice, p.OutputPrice, model+" output")
	}

	build := TextModelPricing[ModelGrokBuild01]
	code := TextModelPricing[ModelGrokCodeFast1]
	assert.Equal(t, build.InputPrice, code.InputPrice)
	assert.Equal(t, build.OutputPrice, code.OutputPrice)
}

func TestGrok45PopulateCostIncludesCacheReads(t *testing.T) {
	u := &llm.Usage{
		InputTokens:          1_000_000,
		CacheReadInputTokens: 1_000_000,
		OutputTokens:         1_000_000,
	}
	llm.PopulateCost(ModelGrok45, false, u)
	assert.NotNil(t, u.Cost)
	assert.Equal(t, 2.0, u.Cost.Input)
	assert.Equal(t, 0.5, u.Cost.CacheRead)
	assert.Equal(t, 6.0, u.Cost.Output)
	assert.Equal(t, 8.5, u.Cost.Total)
}
