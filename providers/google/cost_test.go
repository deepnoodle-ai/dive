package google

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestGoogleCacheReadPricingCoverage(t *testing.T) {
	expected := map[string]float64{
		ModelGemini36Flash:                 0.15,
		ModelGemini35Flash:                 0.15,
		ModelGemini35FlashLite:             0.03,
		ModelGemini31ProPreview:            0.20,
		ModelGemini31ProPreviewCustomTools: 0.20,
		ModelGemini31FlashLite:             0.025,
		ModelGemini3FlashPreview:           0.05,
		ModelGemini25Flash:                 0.03,
		ModelGemini25FlashLite:             0.01,
		ModelGemini25Pro:                   0.125,
		ModelGemini20Flash:                 0.025,
	}
	exclusions := map[string]string{
		ModelGemini31FlashLivePreview: "the official pricing row publishes no context-caching price",
		ModelGemini31FlashLitePreview: "the current official pricing page publishes no row for this retired preview id",
		ModelGemini31FlashImagePrev:   "the official image-model pricing row publishes no context-caching price",
		ModelGemini15Pro:              "the current official pricing page no longer publishes a Gemini 1.5 Pro row",
		ModelGemini15Flash:            "the current official pricing page no longer publishes a Gemini 1.5 Flash row",
	}

	for model := range TextModelPricing {
		_, covered := expected[model]
		_, excluded := exclusions[model]
		assert.True(t, covered || excluded, "Google pricing model must be expected or explicitly excluded: "+model)
	}
	for model, reason := range exclusions {
		t.Run("excluded/"+model, func(t *testing.T) {
			assert.NotEmpty(t, reason)
			pricing, ok := TextModelPricing[model]
			assert.True(t, ok)
			assert.Equal(t, 0.0, pricing.CacheReadPrice)
		})
	}
	for model, wantPrice := range expected {
		t.Run(model, func(t *testing.T) {
			pricing, ok := TextModelPricing[model]
			assert.True(t, ok)
			assert.Equal(t, wantPrice, pricing.CacheReadPrice)
			cost := pricing.CostOf(&llm.Usage{CacheReadInputTokens: 1_000_000})
			assert.True(t, cost.CacheRead > 0)
		})
	}
}
