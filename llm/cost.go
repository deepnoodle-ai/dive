package llm

import "sync/atomic"

// CostResolver returns the pricing for a model at a given speed (fast vs.
// standard). ok is false when no pricing is known for the model.
type CostResolver func(model string, fast bool) (PricingInfo, bool)

// costResolver is set by the providers package (via SetCostResolver) so the
// llm package can resolve pricing without importing providers, which would be
// an import cycle. It is consulted from PopulateCost.
var costResolver atomic.Pointer[CostResolver]

// SetCostResolver installs the global pricing resolver. The providers package
// wires this up in init() so that PopulateCost — and therefore the streaming
// accumulator — can attach cost to usage automatically. Passing nil clears it.
func SetCostResolver(r CostResolver) {
	if r == nil {
		costResolver.Store(nil)
		return
	}
	costResolver.Store(&r)
}

// PopulateCost sets u.Cost from the resolved pricing for the model, when a
// resolver is installed and pricing is known. It is a no-op (leaving u.Cost
// nil — i.e. "unknown") when there is no resolver, no pricing, or no usage.
// fast selects fast-mode pricing where a provider distinguishes it.
func PopulateCost(model string, fast bool, u *Usage) {
	if u == nil {
		return
	}
	rp := costResolver.Load()
	if rp == nil {
		return
	}
	pricing, ok := (*rp)(model, fast)
	if !ok {
		return
	}
	cost := pricing.CostOf(u)
	u.Cost = &cost
}
