package mistral

import "github.com/deepnoodle-ai/dive/providers"

// init publishes this provider's model pricing to the central registry so usage
// cost can be attached automatically (see providers.PricingFor / llm.PopulateCost).
func init() {
	for _, p := range TextModelPricing {
		providers.RegisterPricing(p, false)
	}
}
