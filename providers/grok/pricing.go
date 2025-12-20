package grok

import "github.com/deepnoodle-ai/dive/llm"

// TextModelPricing contains pricing for all text generation models
var TextModelPricing = map[string]llm.PricingInfo{
	ModelGrok40709: {
		Model:       ModelGrok40709,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGrokCodeFast1: {
		Model:       ModelGrokCodeFast1,
		InputPrice:  0.20,
		OutputPrice: 1.50,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGrok3: {
		Model:       ModelGrok3,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGrok3Mini: {
		Model:       ModelGrok3Mini,
		InputPrice:  0.30,
		OutputPrice: 0.50,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}
