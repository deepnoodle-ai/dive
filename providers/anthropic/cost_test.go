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
	assert.Equal(t, 0.5, p.CacheReadPrice)     // 0.1x input
	assert.Equal(t, 6.25, p.CacheWritePrice)   // 1.25x input (5m)
	assert.Equal(t, 10.0, p.CacheWrite1hPrice) // 2.0x input (1h)
}

func TestRegisteredFastPricing(t *testing.T) {
	p, ok := providers.PricingFor(ModelClaudeOpus48, true)
	assert.True(t, ok, "Opus 4.8 fast-mode pricing should be registered")
	assert.Equal(t, 10.0, p.InputPrice) // fast premium
	assert.Equal(t, 50.0, p.OutputPrice)
	assert.Equal(t, 12.5, p.CacheWritePrice)
	assert.Equal(t, 20.0, p.CacheWrite1hPrice)
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

func TestFinalizeUsagePrices5mAnd1hCacheWritesSeparately(t *testing.T) {
	usage := &llm.Usage{
		InputTokens:                1_000_000,
		OutputTokens:               1_000_000,
		CacheReadInputTokens:       1_000_000,
		CacheCreationInputTokens:   2_000_000,
		CacheCreation5mInputTokens: 1_000_000,
		CacheCreation1hInputTokens: 1_000_000,
	}
	finalizeUsage(&llm.Config{}, ModelClaudeOpus48, usage)
	assert.NotNil(t, usage.Cost)
	// 6.25 (5m write) + 10.00 (1h write) = 16.25
	assert.Equal(t, 16.25, usage.Cost.CacheWrite)
	// 5 (in) + 25 (out) + 0.5 (read) + 16.25 (write)
	assert.Equal(t, 46.75, usage.Cost.Total)
}

func TestHaiku55LongContextCacheWritePricing(t *testing.T) {
	p, ok := providers.PricingFor(ModelClaudeHaiku55, false)
	assert.True(t, ok, "Haiku 5.5 pricing should be registered")
	assert.Equal(t, 0.125, p.CacheWritePrice)
	assert.Equal(t, 0.20, p.CacheWrite1hPrice)
	assert.Equal(t, 0.625, p.LongContextCacheWritePrice)
	assert.Equal(t, 1.00, p.LongContextCacheWrite1hPrice)

	// Under 100k tokens: 5m writes at $0.125/M, 1h writes at $0.20/M.
	short := p.CostOf(&llm.Usage{
		InputTokens:                10_000,
		CacheCreationInputTokens:   40_000,
		CacheCreation5mInputTokens: 10_000,
		CacheCreation1hInputTokens: 30_000,
	})
	assert.InDelta(t, (10_000*0.125+30_000*0.20)/1_000_000, short.CacheWrite, 1e-12)

	// Over 100k tokens: 5m writes at $0.625/M, 1h writes at $1.00/M.
	long := p.CostOf(&llm.Usage{
		InputTokens:                70_000,
		CacheCreationInputTokens:   40_000,
		CacheCreation5mInputTokens: 10_000,
		CacheCreation1hInputTokens: 30_000,
	})
	assert.InDelta(t, (10_000*0.625+30_000*1.00)/1_000_000, long.CacheWrite, 1e-12)
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
	out := withCachePricing(llm.PricingInfo{
		Model:                 "x",
		InputPrice:            4,
		LongContextInputPrice: 8,
	})
	assert.Equal(t, 0.4, out.CacheReadPrice)
	assert.Equal(t, 5.0, out.CacheWritePrice)
	assert.Equal(t, 8.0, out.CacheWrite1hPrice)
	assert.Equal(t, 10.0, out.LongContextCacheWritePrice)
	assert.Equal(t, 16.0, out.LongContextCacheWrite1hPrice)
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
			assert.Equal(t, 0.25, p.CacheReadPrice)     // 0.025x input, not 1.0
			assert.Equal(t, 12.50, p.CacheWritePrice)   // 1.25x input, as everywhere
			assert.Equal(t, 20.00, p.CacheWrite1hPrice) // 2.0x input, as everywhere
		})
	}
}

// Opus 5.5 bills cache hits at 0.05x base input, and the caching multipliers
// stack on the fast-mode rate, so both cache-read rates are stated, not derived.
func TestOpus55Pricing(t *testing.T) {
	p, ok := providers.PricingFor(ModelClaudeOpus55, false)
	assert.True(t, ok, "Opus 5.5 standard pricing should be registered")
	assert.Equal(t, 4.0, p.InputPrice)
	assert.Equal(t, 20.0, p.OutputPrice)
	assert.Equal(t, 0.2, p.CacheReadPrice)    // 0.05x input, not 0.4
	assert.Equal(t, 5.0, p.CacheWritePrice)   // 1.25x input, as everywhere
	assert.Equal(t, 8.0, p.CacheWrite1hPrice) // 2.0x input, as everywhere

	fast, ok := providers.PricingFor(ModelClaudeOpus55, true)
	assert.True(t, ok, "Opus 5.5 fast-mode pricing should be registered")
	assert.Equal(t, 8.0, fast.InputPrice)
	assert.Equal(t, 40.0, fast.OutputPrice)
	assert.Equal(t, 0.4, fast.CacheReadPrice)
	assert.Equal(t, 10.0, fast.CacheWritePrice)
	assert.Equal(t, 16.0, fast.CacheWrite1hPrice)
}

func TestSonnet5StandardPricing(t *testing.T) {
	p, ok := providers.PricingFor(ModelClaudeSonnet5, false)
	assert.True(t, ok, "Sonnet 5 pricing should be registered")
	assert.Equal(t, 2.0, p.InputPrice)
	assert.Equal(t, 10.0, p.OutputPrice)
	assert.Equal(t, 0.2, p.CacheReadPrice)
}
