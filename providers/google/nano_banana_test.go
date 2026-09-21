package google

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestNanoBananaModelCatalog(t *testing.T) {
	models := []string{
		ModelGemini31FlashLiteImage,
		ModelGemini31FlashImage,
		ModelGemini3ProImage,
		ModelGemini25FlashImage,
	}

	assert.Equal(t, "gemini-3.1-flash-lite-image", ModelGemini31FlashLiteImage)
	assert.Equal(t, "gemini-3.1-flash-image", ModelGemini31FlashImage)
	assert.Equal(t, "gemini-3-pro-image", ModelGemini3ProImage)
	assert.Equal(t, "gemini-2.5-flash-image", ModelGemini25FlashImage)
	assert.Equal(t, ModelGemini3ProImage, ModelGemini31ProImage)

	// Google bills every Nano Banana model per token, so the rates live in
	// TokenPricing and Price stays zero. Recording them as a per-image constant
	// meant picking one output resolution: gemini-3-pro-image is $0.134 at 1K
	// and $0.24 at 4K off the same $120/1M rate.
	for _, model := range models {
		pricing, ok := ImageModelPricing[model]
		assert.True(t, ok, "missing Nano Banana image pricing for "+model)
		assert.Equal(t, model, pricing.Model)
		assert.Equal(t, 0.0, pricing.Price, "Nano Banana is token-billed, not per-image: "+model)
		assert.NotNil(t, pricing.TokenPricing, "missing token pricing for "+model)
		assert.True(t, pricing.TokenPricing.InputPrice > 0, "missing input rate for "+model)
		assert.True(t, pricing.TokenPricing.OutputPriceByModality["image"] > 0,
			"missing image output rate for "+model)
	}
}

// A generated image is billed as output tokens at the image rate, not at the
// model's text output rate. gemini-3-pro-image charges $120/1M for image tokens
// against $12/1M for text, so pricing image output as text is a 10x undercount.
func TestNanoBananaImageOutputPricing(t *testing.T) {
	pricing, ok := ImageModelPricing[ModelGemini3ProImage]
	assert.True(t, ok)

	cost, ok := pricing.CostOf(&llm.Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		ModalityTokens: map[string]llm.ModalityTokenUsage{
			"image": {OutputTokens: 1_000_000},
		},
	})
	assert.True(t, ok, "token-billed model should price from usage")
	assert.Equal(t, 2.00, cost.Input)
	assert.Equal(t, 120.00, cost.Output)

	// A per-image model answers the other way around.
	_, ok = pricing.CostOfImages(4)
	assert.False(t, ok, "token-billed model has no per-image price")
}
