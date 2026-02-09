package grok

import "github.com/deepnoodle-ai/dive/llm"

// TextModelPricing contains pricing for all text generation models
var TextModelPricing = map[string]llm.PricingInfo{
	ModelGrok41FastReasoning: {
		Model:       ModelGrok41FastReasoning,
		InputPrice:  0.20,
		OutputPrice: 0.50,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGrok41FastNonReasoning: {
		Model:       ModelGrok41FastNonReasoning,
		InputPrice:  0.20,
		OutputPrice: 0.50,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGrok4FastReasoning: {
		Model:       ModelGrok4FastReasoning,
		InputPrice:  0.20,
		OutputPrice: 0.50,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGrok4FastNonReasoning: {
		Model:       ModelGrok4FastNonReasoning,
		InputPrice:  0.20,
		OutputPrice: 0.50,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGrok40709: {
		Model:       ModelGrok40709,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGrokCodeFast1: {
		Model:       ModelGrokCodeFast1,
		InputPrice:  0.20,
		OutputPrice: 1.50,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGrok3: {
		Model:       ModelGrok3,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGrok3Mini: {
		Model:       ModelGrok3Mini,
		InputPrice:  0.30,
		OutputPrice: 0.50,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGrok2Vision1212: {
		Model:       ModelGrok2Vision1212,
		InputPrice:  2.00,
		OutputPrice: 10.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
}
