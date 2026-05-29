package grok

import "github.com/deepnoodle-ai/dive/llm"

// TextModelPricing contains pricing for all text generation models
var TextModelPricing = map[string]llm.PricingInfo{
	ModelGrok43: {
		Model:       ModelGrok43,
		InputPrice:  1.25,
		OutputPrice: 2.50,
		Currency:    "USD",
		UpdatedAt:   "2026-05-28",
	},
	ModelGrok420Reasoning: {
		Model:       ModelGrok420Reasoning,
		InputPrice:  1.25,
		OutputPrice: 2.50,
		Currency:    "USD",
		UpdatedAt:   "2026-05-28",
	},
	ModelGrok420NonReasoning: {
		Model:       ModelGrok420NonReasoning,
		InputPrice:  1.25,
		OutputPrice: 2.50,
		Currency:    "USD",
		UpdatedAt:   "2026-05-28",
	},
	ModelGrok420MultiAgent: {
		Model:       ModelGrok420MultiAgent,
		InputPrice:  1.25,
		OutputPrice: 2.50,
		Currency:    "USD",
		UpdatedAt:   "2026-05-28",
	},
	ModelGrokBuild01: {
		Model:       ModelGrokBuild01,
		InputPrice:  1.00,
		OutputPrice: 2.00,
		Currency:    "USD",
		UpdatedAt:   "2026-05-28",
	},
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
}

// ImageModelPricing contains pricing for Grok Imagine image generation models.
var ImageModelPricing = map[string]llm.ImagePricingInfo{
	ModelImagineImage: {
		Model:     ModelImagineImage,
		Price:     0.02,
		Currency:  "USD",
		UpdatedAt: "2026-05-28",
	},
	ModelImagineImageQuality: {
		Model:     ModelImagineImageQuality,
		Price:     0.05,
		Currency:  "USD",
		UpdatedAt: "2026-05-28",
	},
}
