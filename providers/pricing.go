package providers

import (
	"sync"

	"github.com/deepnoodle-ai/dive/llm"
)

// Pricing registry. Providers register their per-model pricing here from
// init(), mirroring how they register model factories. A central registry lets
// any consumer resolve cost by model name without importing every provider
// package, and lets the llm streaming accumulator attach cost automatically via
// the resolver wired up in init below.
var (
	pricingMu       sync.RWMutex
	standardPricing = map[string]llm.PricingInfo{}
	fastPricing     = map[string]llm.PricingInfo{}
)

func init() {
	// Wire the llm package's cost resolver to this registry so that
	// llm.PopulateCost (and therefore streamed responses) can price usage
	// without an import cycle.
	llm.SetCostResolver(PricingFor)
}

// RegisterPricing records pricing for a model. fast indicates the entry applies
// to fast-mode requests (e.g. Anthropic's premium fast inference); register the
// standard entry with fast=false. Typically called from a provider's init().
func RegisterPricing(info llm.PricingInfo, fast bool) {
	pricingMu.Lock()
	defer pricingMu.Unlock()
	if fast {
		fastPricing[info.Model] = info
	} else {
		standardPricing[info.Model] = info
	}
}

// PricingFor returns the registered pricing for a model at the given speed.
// When fast is requested but no fast-specific entry exists, it falls back to
// the standard entry. ok is false when no pricing is registered for the model.
func PricingFor(model string, fast bool) (llm.PricingInfo, bool) {
	pricingMu.RLock()
	defer pricingMu.RUnlock()
	if fast {
		if p, ok := fastPricing[model]; ok {
			return p, true
		}
	}
	p, ok := standardPricing[model]
	return p, ok
}
