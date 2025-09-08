package openai

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
	"gpt-5-2025-08-07": {
		Model:       "gpt-5-2025-08-07",
		InputPrice:  1.25,
		OutputPrice: 10.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"gpt-4o": {
		Model:       "gpt-4o",
		InputPrice:  2.50,
		OutputPrice: 10.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"gpt-4o-mini": {
		Model:       "gpt-4o-mini",
		InputPrice:  0.15,
		OutputPrice: 0.60,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"gpt-4-turbo": {
		Model:       "gpt-4-turbo",
		InputPrice:  10.00,
		OutputPrice: 30.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"gpt-3.5-turbo": {
		Model:       "gpt-3.5-turbo",
		InputPrice:  0.50,
		OutputPrice: 1.50,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}

// ImageModelPricing contains pricing for image generation models
var ImageModelPricing = map[string]ImagePricingInfo{
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
var EmbeddingModelPricing = map[string]EmbeddingPricingInfo{
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