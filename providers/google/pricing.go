package google

import "github.com/deepnoodle-ai/dive/llm"

// TextModelPricing contains pricing for all text generation models
var TextModelPricing = map[string]llm.PricingInfo{
	ModelGemini3ProPreview: {
		Model:       ModelGemini3ProPreview,
		InputPrice:  2.00, // Up to 200K tokens
		OutputPrice: 12.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGemini3FlashPreview: {
		Model:       ModelGemini3FlashPreview,
		InputPrice:  0.50,
		OutputPrice: 3.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGemini25Flash: {
		Model:       ModelGemini25Flash,
		InputPrice:  0.30,
		OutputPrice: 2.50,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGemini25FlashLite: {
		Model:       ModelGemini25FlashLite,
		InputPrice:  0.10,
		OutputPrice: 0.40,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGemini25Pro: {
		Model:       ModelGemini25Pro,
		InputPrice:  1.25, // Up to 200K tokens
		OutputPrice: 10.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGemini25ProLong: {
		Model:       ModelGemini25ProLong,
		InputPrice:  2.50, // Over 200K tokens
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2026-02-09",
	},
	ModelGemini20Flash: {
		Model:       ModelGemini20Flash,
		InputPrice:  0.10,
		OutputPrice: 0.40,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGemini15Pro: {
		Model:       ModelGemini15Pro,
		InputPrice:  1.25,
		OutputPrice: 5.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGemini15Flash: {
		Model:       ModelGemini15Flash,
		InputPrice:  0.15,
		OutputPrice: 0.60,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}

// ImageModelPricing contains pricing for image generation models
var ImageModelPricing = map[string]llm.ImagePricingInfo{
	ModelGemini25FlashImage: {
		Model:     ModelGemini25FlashImage,
		Price:     0.039, // $30 per 1M tokens, 1290 tokens per 1024x1024 image
		MaxSize:   "1024x1024",
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
}

// EmbeddingModelPricing contains pricing for embedding models
var EmbeddingModelPricing = map[string]llm.EmbeddingPricingInfo{
	"text-embedding-004": {
		Model:     "text-embedding-004",
		Price:     0.0625, // $0.0000625 per 1K tokens = $0.0625 per 1M tokens
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
	"text-multilingual-embedding-002": {
		Model:     "text-multilingual-embedding-002",
		Price:     0.0625, // Same as text-embedding-004
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
}
