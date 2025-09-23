package ollama

import "github.com/deepnoodle-ai/dive/llm"

// Note: Ollama is a local inference platform - all models run locally and are free
// However, there are compute costs (electricity, hardware depreciation) that users bear
// For consistency with the pricing structure, we maintain the same format but with zero prices

// TextModelPricing contains pricing for all text generation models (free - local inference)
var TextModelPricing = map[string]llm.PricingInfo{
	"llama2": {
		Model:       "llama2",
		InputPrice:  0.00, // Free - runs locally
		OutputPrice: 0.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"llama2:13b": {
		Model:       "llama2:13b",
		InputPrice:  0.00,
		OutputPrice: 0.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"llama2:70b": {
		Model:       "llama2:70b",
		InputPrice:  0.00,
		OutputPrice: 0.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"codellama": {
		Model:       "codellama",
		InputPrice:  0.00,
		OutputPrice: 0.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"mistral": {
		Model:       "mistral",
		InputPrice:  0.00,
		OutputPrice: 0.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"neural-chat": {
		Model:       "neural-chat",
		InputPrice:  0.00,
		OutputPrice: 0.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"starling-lm": {
		Model:       "starling-lm",
		InputPrice:  0.00,
		OutputPrice: 0.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"orca-mini": {
		Model:       "orca-mini",
		InputPrice:  0.00,
		OutputPrice: 0.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"vicuna": {
		Model:       "vicuna",
		InputPrice:  0.00,
		OutputPrice: 0.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}

// ImageModelPricing contains pricing for image generation models (currently none)
var ImageModelPricing = map[string]llm.ImagePricingInfo{}

// EmbeddingModelPricing contains pricing for embedding models (free - local inference)
var EmbeddingModelPricing = map[string]llm.EmbeddingPricingInfo{
	"nomic-embed-text": {
		Model:     "nomic-embed-text",
		Price:     0.00, // Free - runs locally
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
	"all-minilm": {
		Model:     "all-minilm",
		Price:     0.00,
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
}
