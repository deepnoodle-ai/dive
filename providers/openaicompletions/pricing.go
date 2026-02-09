package openaicompletions

import "github.com/deepnoodle-ai/dive/llm"

// TextModelPricing contains pricing for all text generation models
var TextModelPricing = map[string]llm.PricingInfo{
	ModelGPT52: {
		Model:       ModelGPT52,
		InputPrice:  1.75,
		OutputPrice: 14.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGPT51: {
		Model:       ModelGPT51,
		InputPrice:  1.25,
		OutputPrice: 10.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGPT5: {
		Model:       ModelGPT5,
		InputPrice:  1.25,
		OutputPrice: 10.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGPT5Mini: {
		Model:       ModelGPT5Mini,
		InputPrice:  0.25,
		OutputPrice: 2.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGPT5Nano: {
		Model:       ModelGPT5Nano,
		InputPrice:  0.05,
		OutputPrice: 0.40,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGPT41: {
		Model:       ModelGPT41,
		InputPrice:  2.00,
		OutputPrice: 8.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGPT4o: {
		Model:       ModelGPT4o,
		InputPrice:  5.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}
