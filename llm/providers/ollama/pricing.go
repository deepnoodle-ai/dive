package ollama

// PricingInfo represents pricing information for a specific service
type PricingInfo struct {
	Model       string  `json:"model"`
	InputPrice  float64 `json:"input_price_per_1m_tokens"`  // per 1M input tokens (USD)
	OutputPrice float64 `json:"output_price_per_1m_tokens"` // per 1M output tokens (USD)
	Currency    string  `json:"currency"`
	UpdatedAt   string  `json:"updated_at"` // YYYY-MM-DD format
}

// ImagePricingInfo represents pricing for image generation services
type ImagePricingInfo struct {
	Model     string  `json:"model"`
	Price     float64 `json:"price_per_image"`    // per image (USD)
	MaxSize   string  `json:"max_size"`           // e.g., "1024x1024"
	Currency  string  `json:"currency"`
	UpdatedAt string  `json:"updated_at"`
}

// EmbeddingPricingInfo represents pricing for embedding services
type EmbeddingPricingInfo struct {
	Model     string  `json:"model"`
	Price     float64 `json:"price_per_1m_tokens"` // per 1M tokens (USD)
	Currency  string  `json:"currency"`
	UpdatedAt string  `json:"updated_at"`
}

// Note: Ollama is a local inference platform - all models run locally and are free
// However, there are compute costs (electricity, hardware depreciation) that users bear
// For consistency with the pricing structure, we maintain the same format but with zero prices

// TextModelPricing contains pricing for all text generation models (free - local inference)
var TextModelPricing = map[string]PricingInfo{
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
var ImageModelPricing = map[string]ImagePricingInfo{}

// EmbeddingModelPricing contains pricing for embedding models (free - local inference)
var EmbeddingModelPricing = map[string]EmbeddingPricingInfo{
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