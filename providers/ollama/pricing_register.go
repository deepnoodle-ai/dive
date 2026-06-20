package ollama

import "github.com/deepnoodle-ai/dive/providers"

// init publishes this provider's model pricing to the central registry so usage
// cost can be attached automatically. Ollama runs locally, so its registered
// prices are zero — yielding a known cost of $0 (distinct from unknown/nil).
func init() {
	for _, p := range TextModelPricing {
		providers.RegisterPricing(p, false)
	}
}
