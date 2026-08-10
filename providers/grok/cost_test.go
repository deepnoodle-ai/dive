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
	assert.Equal(t, 0.3, p.CacheReadPrice)
	assert.Equal(t, 6.0, p.OutputPrice)
	assert.Equal(t, 200_000, p.LongContextThreshold)
	assert.Equal(t, 4.0, p.LongContextInputPrice)
	assert.Equal(t, 0.6, p.LongContextCacheReadPrice)
	assert.Equal(t, 12.0, p.LongContextOutputPrice)
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
		assert.Equal(t, grok43.CacheReadPrice, p.CacheReadPrice, model+" cache read")
		assert.Equal(t, grok43.OutputPrice, p.OutputPrice, model+" output")
	}

	build := TextModelPricing[ModelGrokBuild01]
	code := TextModelPricing[ModelGrokCodeFast1]
	assert.Equal(t, build.InputPrice, code.InputPrice)
	assert.Equal(t, build.CacheReadPrice, code.CacheReadPrice)
	assert.Equal(t, build.OutputPrice, code.OutputPrice)
}

func TestPublishedGrokCacheReadPricingCoverage(t *testing.T) {
	want := map[string]float64{
		ModelGrok45:                 0.30,
		ModelGrok43:                 0.20,
		ModelGrok420Reasoning:       0.20,
		ModelGrok420NonReasoning:    0.20,
		ModelGrok420MultiAgent:      0.20,
		ModelGrokBuild01:            0.20,
		ModelGrok41FastReasoning:    0.20,
		ModelGrok41FastNonReasoning: 0.20,
		ModelGrok4FastReasoning:     0.20,
		ModelGrok4FastNonReasoning:  0.20,
		ModelGrok40709:              0.20,
		ModelGrokCodeFast1:          0.20,
		ModelGrok3:                  0.20,
	}

	for model, pricing := range TextModelPricing {
		if model == ModelGrok3Mini {
			// xAI no longer lists this model or publishes a redirect target, so
			// its historical pricing cannot be given a verified cache-read rate.
			assert.Equal(t, 0.0, pricing.CacheReadPrice)
			continue
		}
		wantPrice, ok := want[model]
		assert.True(t, ok, "missing cache-read coverage expectation for "+model)
		assert.Equal(t, wantPrice, pricing.CacheReadPrice, model)
	}
}

func TestGrokLongContextPricingBoundary(t *testing.T) {
	models := []string{
		ModelGrok45,
		ModelGrok43,
		ModelGrok420Reasoning,
		ModelGrok420NonReasoning,
		ModelGrok420MultiAgent,
		ModelGrokBuild01,
	}
	for _, model := range models {
		p := TextModelPricing[model]
		assert.Equal(t, 200_000, p.LongContextThreshold, model)

		standard := p.CostOf(&llm.Usage{
			InputTokens:          100_000,
			CacheReadInputTokens: 99_999,
			OutputTokens:         1_000_000,
		})
		assert.Equal(t, float64(100_000)*p.InputPrice/1_000_000, standard.Input, model+" standard input")
		assert.Equal(t, float64(99_999)*p.CacheReadPrice/1_000_000, standard.CacheRead, model+" standard cache read")
		assert.Equal(t, p.OutputPrice, standard.Output, model+" standard output")

		long := p.CostOf(&llm.Usage{
			InputTokens:          100_000,
			CacheReadInputTokens: 100_000,
			OutputTokens:         1_000_000,
		})
		assert.Equal(t, float64(100_000)*p.LongContextInputPrice/1_000_000, long.Input, model+" long input")
		assert.Equal(t, float64(100_000)*p.LongContextCacheReadPrice/1_000_000, long.CacheRead, model+" long cache read")
		assert.Equal(t, p.LongContextOutputPrice, long.Output, model+" long output")
	}
}

func TestGrok45PopulateCostAppliesLongContextTier(t *testing.T) {
	u := &llm.Usage{
		InputTokens:          100_000,
		CacheReadInputTokens: 100_000,
		OutputTokens:         1_000_000,
	}
	llm.PopulateCost(ModelGrok45, false, u)
	assert.NotNil(t, u.Cost)
	assert.Equal(t, 0.4, u.Cost.Input)
	assert.Equal(t, 0.06, u.Cost.CacheRead)
	assert.Equal(t, 12.0, u.Cost.Output)
	assert.Equal(t, 12.46, u.Cost.Total)
}
