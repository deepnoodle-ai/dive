package anthropic

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

func init() {
	// Register for claude-* models
	providers.Register(providers.ProviderEntry{
		Name:    "anthropic",
		Match:   providers.PrefixMatcher("claude-"),
		Factory: factory,
	})

	// Register as the fallback provider (for unknown models)
	providers.SetFallback(factory)

	registerPricing()
}

// registerPricing publishes Anthropic model pricing — with derived prompt-cache
// rates — to the central pricing registry so usage cost can be attached.
func registerPricing() {
	for _, p := range TextModelPricing {
		providers.RegisterPricing(withCachePricing(p), false)
	}
	for _, p := range FastModeTextPricing {
		providers.RegisterPricing(withCachePricing(p), true)
	}
}

// withCachePricing fills Anthropic's prompt-cache rates relative to the base
// input price: cache reads bill at 0.1x, 5-minute cache writes bill at 1.25x,
// and 1-hour cache writes bill at 2x. When a model has a long-context input
// tier, its long-context cache-write rates follow the same multipliers against
// LongContextInputPrice.
//
// Only zero fields are filled, so a model whose published rate departs from the
// multipliers states it in catalog.json instead: Fable 5.1 and Mythos 5.1 bill
// cache reads at 0.025x, and deriving 0.1x there would overcharge by 4x.
func withCachePricing(p llm.PricingInfo) llm.PricingInfo {
	if p.CacheReadPrice == 0 {
		p.CacheReadPrice = p.InputPrice * 0.10
	}
	if p.CacheWritePrice == 0 {
		p.CacheWritePrice = p.InputPrice * 1.25
	}
	if p.CacheWrite1hPrice == 0 {
		p.CacheWrite1hPrice = p.InputPrice * 2.0
	}
	if p.LongContextInputPrice > 0 {
		if p.LongContextCacheWritePrice == 0 {
			p.LongContextCacheWritePrice = p.LongContextInputPrice * 1.25
		}
		if p.LongContextCacheWrite1hPrice == 0 {
			p.LongContextCacheWrite1hPrice = p.LongContextInputPrice * 2.0
		}
	}
	return p
}

func factory(model, endpoint string) llm.LLM {
	opts := []Option{WithModel(model)}
	if endpoint != "" {
		opts = append(opts, WithEndpoint(endpoint))
	}
	return New(opts...)
}
