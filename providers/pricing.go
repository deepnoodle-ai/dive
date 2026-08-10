package providers

import (
	"sync"
	"time"

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
		if p, ok := pricingForModel(fastPricing, model); ok {
			return p, true
		}
	}
	return pricingForModel(standardPricing, model)
}

func pricingForModel(table map[string]llm.PricingInfo, model string) (llm.PricingInfo, bool) {
	if p, ok := table[model]; ok {
		return p, true
	}
	base, ok := stripDatedModelVersion(model)
	if !ok {
		return llm.PricingInfo{}, false
	}
	p, ok := table[base]
	return p, ok
}

// Some providers return a dated deployment ID even when the request uses the
// corresponding stable catalog ID (for example gpt-5.4-mini-2026-03-17).
// Reuse the stable model's price only for the unambiguous YYYY-MM-DD suffix.
func stripDatedModelVersion(model string) (string, bool) {
	const suffixLength = len("-2006-01-02")
	if len(model) <= suffixLength {
		return "", false
	}
	suffix := model[len(model)-suffixLength:]
	if suffix[0] != '-' {
		return "", false
	}
	if _, err := time.Parse("2006-01-02", suffix[1:]); err != nil {
		return "", false
	}
	return model[:len(model)-suffixLength], true
}
