package groq

import "github.com/deepnoodle-ai/dive/llm"

// TextModelPricing contains pricing for all text generation models
var TextModelPricing = map[string]llm.PricingInfo{
	ModelLlama3370bVersatile: {
		Model:       ModelLlama3370bVersatile,
		InputPrice:  0.59,
		OutputPrice: 0.79,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelDeepSeekR1DistillLlama70b: {
		Model:       ModelDeepSeekR1DistillLlama70b,
		InputPrice:  0.50,
		OutputPrice: 0.80,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"llama-4-scout": {
		Model:       "llama-4-scout",
		InputPrice:  0.11,
		OutputPrice: 0.34,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"llama-4-maverick": {
		Model:       "llama-4-maverick",
		InputPrice:  0.50,
		OutputPrice: 0.77,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"mixtral-8x7b-32768": {
		Model:       "mixtral-8x7b-32768",
		InputPrice:  0.27,
		OutputPrice: 0.27,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"llama2-70b-4096": {
		Model:       "llama2-70b-4096",
		InputPrice:  0.70,
		OutputPrice: 0.80,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}
