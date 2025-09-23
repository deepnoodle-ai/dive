package openrouter

import "github.com/deepnoodle-ai/dive/llm"

// TextModelPricing contains representative pricing for popular models via OpenRouter
var TextModelPricing = map[string]llm.PricingInfo{
	ModelClaudeSonnet4: {
		Model:       ModelClaudeSonnet4,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelClaudeOpus41: {
		Model:       ModelClaudeOpus41,
		InputPrice:  15.00,
		OutputPrice: 75.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGPT5: {
		Model:       ModelGPT5,
		InputPrice:  0.625,
		OutputPrice: 5.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGPT4o: {
		Model:       ModelGPT4o,
		InputPrice:  2.50,
		OutputPrice: 10.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGemini25Flash: {
		Model:       ModelGemini25Flash,
		InputPrice:  0.30,
		OutputPrice: 2.50,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGemini25Pro: {
		Model:       ModelGemini25Pro,
		InputPrice:  1.25,
		OutputPrice: 10.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelDeepSeekR1: {
		Model:       ModelDeepSeekR1,
		InputPrice:  0.50,
		OutputPrice: 2.18,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}

// ImageModelPricing contains representative pricing for image generation models
var ImageModelPricing = map[string]llm.ImagePricingInfo{
	"openai/dall-e-3": {
		Model:     "openai/dall-e-3",
		Price:     0.040,
		MaxSize:   "1024x1024",
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
	"stability-ai/stable-diffusion-xl": {
		Model:     "stability-ai/stable-diffusion-xl",
		Price:     0.035,
		MaxSize:   "1024x1024",
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
}

// EmbeddingModelPricing contains representative pricing for embedding models
var EmbeddingModelPricing = map[string]llm.EmbeddingPricingInfo{
	"openai/text-embedding-3-small": {
		Model:     "openai/text-embedding-3-small",
		Price:     0.020,
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
	"openai/text-embedding-3-large": {
		Model:     "openai/text-embedding-3-large",
		Price:     0.130,
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
}
