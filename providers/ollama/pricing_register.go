package ollama

import "github.com/deepnoodle-ai/dive/providers"

// init publishes this provider's model pricing to the central registry so usage
// cost can be attached automatically. Ollama runs locally, so its registered
// prices are zero — yielding a known cost of $0 (distinct from unknown/nil).
func init() {
	for _, p := range TextModelPricing {
		providers.RegisterPricing(p, false)
	}
	// Image models their provider bills per token resolve through the same
	// registry as everything else, so llm.PopulateCost can price an image
	// request. Per-image models have no token rates and stay out of it.
	for _, p := range ImageModelPricing {
		if p.TokenPricing != nil {
			providers.RegisterPricing(*p.TokenPricing, false)
		}
	}
}
