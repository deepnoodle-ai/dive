package google

import "github.com/deepnoodle-ai/dive/llm"

// TextModelPricing contains pricing for all text generation models
var TextModelPricing = map[string]llm.PricingInfo{
	ModelGemini36Flash: {
		Model:       ModelGemini36Flash,
		InputPrice:  1.50,
		OutputPrice: 7.50,
		Currency:    "USD",
		UpdatedAt:   "2026-07-21",
	},
	ModelGemini35Flash: {
		Model:       ModelGemini35Flash,
		InputPrice:  1.50,
		OutputPrice: 9.00,
		Currency:    "USD",
		UpdatedAt:   "2026-05-28",
	},
	ModelGemini35FlashLite: {
		Model:       ModelGemini35FlashLite,
		InputPrice:  0.30,
		OutputPrice: 2.50,
		Currency:    "USD",
		UpdatedAt:   "2026-07-21",
	},
	ModelGemini31ProPreview: {
		Model:       ModelGemini31ProPreview,
		InputPrice:  2.00,  // Up to 200K tokens ($4.00 over 200K)
		OutputPrice: 12.00, // $18.00 over 200K tokens
		Currency:    "USD",
		UpdatedAt:   "2026-05-28",
	},
	ModelGemini31ProPreviewCustomTools: {
		Model:       ModelGemini31ProPreviewCustomTools,
		InputPrice:  2.00,  // Up to 200K tokens ($4.00 over 200K)
		OutputPrice: 12.00, // $18.00 over 200K tokens
		Currency:    "USD",
		UpdatedAt:   "2026-05-28",
	},
	ModelGemini31FlashLite: {
		Model:       ModelGemini31FlashLite,
		InputPrice:  0.25, // text/image/video ($0.50 audio)
		OutputPrice: 1.50,
		Currency:    "USD",
		UpdatedAt:   "2026-05-28",
	},
	ModelGemini31FlashLivePreview: {
		Model:       ModelGemini31FlashLivePreview,
		InputPrice:  0.75, // text input
		OutputPrice: 4.50, // text output
		Currency:    "USD",
		UpdatedAt:   "2026-05-28",
	},
	ModelGemini31FlashLitePreview: {
		Model:       ModelGemini31FlashLitePreview,
		InputPrice:  0.10,
		OutputPrice: 0.40,
		Currency:    "USD",
		UpdatedAt:   "2026-03-10",
	},
	ModelGemini31FlashImagePrev: {
		Model:       ModelGemini31FlashImagePrev,
		InputPrice:  0.50,
		OutputPrice: 3.00,
		Currency:    "USD",
		UpdatedAt:   "2026-03-10",
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
		UpdatedAt: "2026-06-30",
	},
	ModelGemini31FlashLiteImage: {
		Model:     ModelGemini31FlashLiteImage,
		Price:     0.0336, // $30 per 1M output tokens; 1120 tokens per 1K image
		MaxSize:   "1024x1024",
		Currency:  "USD",
		UpdatedAt: "2026-06-30",
	},
	ModelGemini31FlashImage: {
		Model:     ModelGemini31FlashImage,
		Price:     0.067, // $60 per 1M output tokens; ~$0.067 per 1K-resolution image
		MaxSize:   "4096x4096",
		Currency:  "USD",
		UpdatedAt: "2026-06-30",
	},
	ModelGemini3ProImage: {
		Model:     ModelGemini3ProImage,
		Price:     0.134, // $120 per 1M output tokens; ~$0.134 per 1K/2K image ($0.24 at 4K)
		MaxSize:   "4096x4096",
		Currency:  "USD",
		UpdatedAt: "2026-06-30",
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
