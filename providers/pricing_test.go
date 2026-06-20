package providers

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestPricingRegistryLookup(t *testing.T) {
	RegisterPricing(llm.PricingInfo{Model: "reg-test-model", InputPrice: 3, OutputPrice: 15, Currency: "USD"}, false)
	RegisterPricing(llm.PricingInfo{Model: "reg-test-model", InputPrice: 6, OutputPrice: 30, Currency: "USD"}, true)

	p, ok := PricingFor("reg-test-model", false)
	assert.True(t, ok)
	assert.Equal(t, 3.0, p.InputPrice)

	// Fast lookup gets the fast entry.
	p, ok = PricingFor("reg-test-model", true)
	assert.True(t, ok)
	assert.Equal(t, 6.0, p.InputPrice)

	// Unknown model.
	_, ok = PricingFor("does-not-exist", false)
	assert.False(t, ok)
}

func TestPricingFor_FastFallsBackToStandard(t *testing.T) {
	RegisterPricing(llm.PricingInfo{Model: "reg-standard-only", InputPrice: 2, Currency: "USD"}, false)
	// No fast entry registered; fast lookup should fall back to standard.
	p, ok := PricingFor("reg-standard-only", true)
	assert.True(t, ok)
	assert.Equal(t, 2.0, p.InputPrice)
}

func TestCostResolverIsWired(t *testing.T) {
	// init() should have installed PricingFor as the llm cost resolver.
	RegisterPricing(llm.PricingInfo{Model: "reg-wired-model", InputPrice: 5, Currency: "USD"}, false)
	u := &llm.Usage{InputTokens: 1_000_000}
	llm.PopulateCost("reg-wired-model", false, u)
	assert.NotNil(t, u.Cost, "providers init should wire llm.SetCostResolver to PricingFor")
	assert.Equal(t, 5.0, u.Cost.Total)
}
