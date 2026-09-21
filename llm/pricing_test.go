package llm

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

// A caller dispatching on ok -- per-image first, token pricing as the fallback
// -- would route an empty request down the token path if a zero count looked
// the same as a model that has no per-image rate.
func TestCostOfImagesZeroCount(t *testing.T) {
	perImage := ImagePricingInfo{Model: "dall-e-3", Price: 0.04, Currency: "USD"}
	cost, ok := perImage.CostOfImages(0)
	assert.True(t, ok, "a per-image model is priced whether or not it ran")
	assert.Equal(t, 0.0, cost.Total)
	assert.Equal(t, "dall-e-3", cost.Model)

	negative, ok := perImage.CostOfImages(-3)
	assert.True(t, ok)
	assert.Equal(t, 0.0, negative.Total)

	tokenBilled := ImagePricingInfo{
		Model:        "gpt-image-2.5-sunburst",
		TokenPricing: &PricingInfo{OutputPrice: 30},
	}
	_, ok = tokenBilled.CostOfImages(4)
	assert.False(t, ok, "token-billed models have no per-image rate")
}
