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
