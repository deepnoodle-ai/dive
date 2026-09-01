package anthropic

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestRegisteredPricingHasDerivedCacheRates(t *testing.T) {
	p, ok := providers.PricingFor(ModelClaudeOpus48, false)
	assert.True(t, ok, "Opus 4.8 standard pricing should be registered")
	assert.Equal(t, 5.0, p.InputPrice)
	assert.Equal(t, 25.0, p.OutputPrice)
	assert.Equal(t, 0.5, p.CacheReadPrice)   // 0.1x input
	assert.Equal(t, 6.25, p.CacheWritePrice) // 1.25x input
}

func TestRegisteredFastPricing(t *testing.T) {
	p, ok := providers.PricingFor(ModelClaudeOpus48, true)
	assert.True(t, ok, "Opus 4.8 fast-mode pricing should be registered")
	assert.Equal(t, 10.0, p.InputPrice) // fast premium
	assert.Equal(t, 50.0, p.OutputPrice)
}

func TestFinalizeUsageAttachesCost(t *testing.T) {
	usage := &llm.Usage{
		InputTokens:              1_000_000,
		OutputTokens:             1_000_000,
		CacheReadInputTokens:     1_000_000,
		CacheCreationInputTokens: 1_000_000,
	}
	finalizeUsage(&llm.Config{}, ModelClaudeOpus48, usage)
	assert.NotNil(t, usage.Cost, "finalizeUsage should attach cost for a known model")
	// 5 (in) + 25 (out) + 0.5 (read) + 6.25 (write)
	assert.Equal(t, 36.75, usage.Cost.Total)
	assert.Equal(t, ModelClaudeOpus48, usage.Cost.Model)
}

func TestFinalizeUsageUsesFastPricingWhenServedFast(t *testing.T) {
	usage := &llm.Usage{InputTokens: 1_000_000, Speed: string(llm.SpeedFast)}
	finalizeUsage(&llm.Config{}, ModelClaudeOpus48, usage)
	assert.NotNil(t, usage.Cost)
	assert.Equal(t, 10.0, usage.Cost.Total, "fast speed should bill at fast-mode input price")
}

func TestFinalizeUsageUnknownModelLeavesCostNil(t *testing.T) {
	usage := &llm.Usage{InputTokens: 1_000_000}
	finalizeUsage(&llm.Config{}, "totally-unknown-model", usage)
	assert.Nil(t, usage.Cost, "unknown model should leave cost unknown (nil)")
}

func TestWithCachePricing(t *testing.T) {
	out := withCachePricing(llm.PricingInfo{Model: "x", InputPrice: 4})
	assert.Equal(t, 0.4, out.CacheReadPrice)
	assert.Equal(t, 5.0, out.CacheWritePrice)
}

// Fable 5.1 and Mythos 5.1 bill cache hits at 0.025x base input, not the 0.1x
// every other Claude model uses. The catalog states that rate outright; this
// pins that withCachePricing leaves it alone instead of deriving over it.
func TestFable51CacheReadPricingIsNotDerived(t *testing.T) {
	for _, model := range []string{ModelClaudeFable51, ModelClaudeMythos51} {
		t.Run(model, func(t *testing.T) {
			p, ok := providers.PricingFor(model, false)
			assert.True(t, ok, "pricing should be registered")
			assert.Equal(t, 10.0, p.InputPrice)
			assert.Equal(t, 50.0, p.OutputPrice)
			assert.Equal(t, 0.25, p.CacheReadPrice)   // 0.025x input, not 1.0
			assert.Equal(t, 12.50, p.CacheWritePrice) // 1.25x input, as everywhere
		})
	}
}

func TestSonnet5StandardPricing(t *testing.T) {
	p, ok := providers.PricingFor(ModelClaudeSonnet5, false)
	assert.True(t, ok, "Sonnet 5 pricing should be registered")
	assert.Equal(t, 2.0, p.InputPrice)
	assert.Equal(t, 10.0, p.OutputPrice)
	assert.Equal(t, 0.2, p.CacheReadPrice)
}
