package grok

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
	ModelGrok40709: {
		Model:       ModelGrok40709,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGrokCodeFast1: {
		Model:       ModelGrokCodeFast1,
		InputPrice:  0.20,
		OutputPrice: 1.50,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGrok3: {
		Model:       ModelGrok3,
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGrok3Mini: {
		Model:       ModelGrok3Mini,
		InputPrice:  1.00,
		OutputPrice: 5.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}

// Note: xAI Grok does not currently offer image generation or embedding services
// These maps are kept empty but structured for potential future expansion

// ImageModelPricing contains pricing for image generation models (currently none)
var ImageModelPricing = map[string]ImagePricingInfo{}

// EmbeddingModelPricing contains pricing for embedding models (currently none)  
var EmbeddingModelPricing = map[string]EmbeddingPricingInfo{}