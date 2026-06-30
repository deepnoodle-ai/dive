package google

import (
	"testing"

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

	for _, model := range models {
		pricing, ok := ImageModelPricing[model]
		assert.True(t, ok, "missing Nano Banana image pricing for "+model)
		assert.Equal(t, model, pricing.Model)
		assert.True(t, pricing.Price > 0, "missing positive image price for "+model)
	}
}
