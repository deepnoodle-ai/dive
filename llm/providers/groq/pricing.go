package groq

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

// TextModelPricing contains pricing for all text generation models
var TextModelPricing = map[string]PricingInfo{
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

// Note: Groq does not currently offer image generation or embedding services
// These maps are kept empty but structured for potential future expansion

// ImageModelPricing contains pricing for image generation models (currently none)
var ImageModelPricing = map[string]ImagePricingInfo{}

// EmbeddingModelPricing contains pricing for embedding models (currently none)
var EmbeddingModelPricing = map[string]EmbeddingPricingInfo{}