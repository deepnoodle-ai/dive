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
// input price: cache reads bill at 0.1x, and cache writes at the default
// 5-minute TTL bill at 1.25x. (The 1-hour TTL is 2x, but usage does not carry
// a per-TTL split, so the default rate is used.)
func withCachePricing(p llm.PricingInfo) llm.PricingInfo {
	if p.CacheReadPrice == 0 {
		p.CacheReadPrice = p.InputPrice * 0.10
	}
	if p.CacheWritePrice == 0 {
		p.CacheWritePrice = p.InputPrice * 1.25
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
