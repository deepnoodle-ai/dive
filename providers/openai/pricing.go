package openai

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
	ModelGPT52Pro: {
		Model:       ModelGPT52Pro,
		InputPrice:  21.00,
		OutputPrice: 168.00,
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

// ImageModelPricing contains pricing for image generation models
var ImageModelPricing = map[string]llm.ImagePricingInfo{
	"dall-e-3": {
		Model:     "dall-e-3",
		Price:     0.040,
		MaxSize:   "1024x1024",
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
	"dall-e-3-hd": {
		Model:     "dall-e-3-hd",
		Price:     0.080,
		MaxSize:   "1024x1024",
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
	"dall-e-2": {
		Model:     "dall-e-2",
		Price:     0.020,
		MaxSize:   "1024x1024",
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
	"gpt-4o-image": {
		Model:     "gpt-4o-image",
		Price:     0.035,
		MaxSize:   "1024x1024",
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
}

// EmbeddingModelPricing contains pricing for embedding models
var EmbeddingModelPricing = map[string]llm.EmbeddingPricingInfo{
	"text-embedding-3-small": {
		Model:     "text-embedding-3-small",
		Price:     0.020, // $0.00002 per 1K tokens = $0.020 per 1M tokens
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
	"text-embedding-3-large": {
		Model:     "text-embedding-3-large",
		Price:     0.130, // $0.00013 per 1K tokens = $0.130 per 1M tokens
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
	"text-embedding-ada-002": {
		Model:     "text-embedding-ada-002",
		Price:     0.100, // $0.0001 per 1K tokens = $0.100 per 1M tokens
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
}
