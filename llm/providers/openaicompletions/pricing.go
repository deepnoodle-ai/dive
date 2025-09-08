package openaicompletions

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

// Note: OpenAI Completions provider uses legacy OpenAI completion models
// These are typically older models with different pricing than the newer chat models

// TextModelPricing contains pricing for completion-based models
var TextModelPricing = map[string]PricingInfo{
	"text-davinci-003": {
		Model:       "text-davinci-003",
		InputPrice:  20.00,
		OutputPrice: 20.00, // Completions typically have same price for input/output
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"text-curie-001": {
		Model:       "text-curie-001",
		InputPrice:  2.00,
		OutputPrice: 2.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"text-babbage-001": {
		Model:       "text-babbage-001",
		InputPrice:  0.50,
		OutputPrice: 0.50,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"text-ada-001": {
		Model:       "text-ada-001",
		InputPrice:  0.40,
		OutputPrice: 0.40,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"gpt-3.5-turbo-instruct": {
		Model:       "gpt-3.5-turbo-instruct",
		InputPrice:  1.50,
		OutputPrice: 2.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}

// Note: OpenAI Completions does not offer separate image generation or embedding services
// These are handled by the main OpenAI provider
// These maps are kept empty but structured for consistency

// ImageModelPricing contains pricing for image generation models (handled by main openai provider)
var ImageModelPricing = map[string]ImagePricingInfo{}

// EmbeddingModelPricing contains pricing for embedding models (handled by main openai provider)
var EmbeddingModelPricing = map[string]EmbeddingPricingInfo{}