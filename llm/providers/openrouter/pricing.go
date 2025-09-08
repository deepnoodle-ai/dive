package openrouter

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

// Note: OpenRouter aggregates multiple providers and models
// Pricing varies significantly based on the underlying provider and model
// These are representative prices - actual pricing should be fetched from OpenRouter API

// TextModelPricing contains representative pricing for popular models via OpenRouter
var TextModelPricing = map[string]PricingInfo{
	"anthropic/claude-3.5-sonnet": {
		Model:       "anthropic/claude-3.5-sonnet",
		InputPrice:  3.00,
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"openai/gpt-4o": {
		Model:       "openai/gpt-4o",
		InputPrice:  2.50,
		OutputPrice: 10.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"openai/gpt-4o-mini": {
		Model:       "openai/gpt-4o-mini",
		InputPrice:  0.15,
		OutputPrice: 0.60,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"google/gemini-pro": {
		Model:       "google/gemini-pro",
		InputPrice:  1.25,
		OutputPrice: 5.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"meta-llama/llama-3.1-70b-instruct": {
		Model:       "meta-llama/llama-3.1-70b-instruct",
		InputPrice:  0.90,
		OutputPrice: 0.90,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"meta-llama/llama-3.1-8b-instruct": {
		Model:       "meta-llama/llama-3.1-8b-instruct",
		InputPrice:  0.18,
		OutputPrice: 0.18,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"mistralai/mistral-7b-instruct": {
		Model:       "mistralai/mistral-7b-instruct",
		InputPrice:  0.00, // Often free on OpenRouter
		OutputPrice: 0.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"deepseek/deepseek-r1": {
		Model:       "deepseek/deepseek-r1",
		InputPrice:  0.55,
		OutputPrice: 2.19,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}

// ImageModelPricing contains representative pricing for image generation models
var ImageModelPricing = map[string]ImagePricingInfo{
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
var EmbeddingModelPricing = map[string]EmbeddingPricingInfo{
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