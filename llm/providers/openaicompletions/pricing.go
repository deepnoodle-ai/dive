package openaicompletions

import "github.com/deepnoodle-ai/dive/llm"

// TextModelPricing contains pricing for all text generation models
var TextModelPricing = map[string]llm.PricingInfo{
	ModelGPT5: {
		Model:       ModelGPT5,
		InputPrice:  1.25,
		OutputPrice: 10.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGPT5Mini: {
		Model:       ModelGPT5Mini,
		InputPrice:  0.25,
		OutputPrice: 2.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGPT5Nano: {
		Model:       ModelGPT5Nano,
		InputPrice:  0.05,
		OutputPrice: 0.40,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGPT4o: {
		Model:       ModelGPT4o,
		InputPrice:  5.0,
		OutputPrice: 15.0,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}
