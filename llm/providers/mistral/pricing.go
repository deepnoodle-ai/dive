package mistral

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
	Price     float64 `json:"price_per_image"` // per image (USD)
	MaxSize   string  `json:"max_size"`        // e.g., "1024x1024"
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
	ModelMistralLarge2411: {
		Model:       ModelMistralLarge2411,
		InputPrice:  2.0,
		OutputPrice: 6.0,
		Currency:    "USD",
		UpdatedAt:   "2025-10-15",
	},
	ModelMistralLarge: {
		Model:       ModelMistralLarge,
		InputPrice:  2.0,
		OutputPrice: 6.0,
		Currency:    "USD",
		UpdatedAt:   "2025-10-15",
	},
	ModelMistralSmall: {
		Model:       ModelMistralSmall,
		InputPrice:  0.1,
		OutputPrice: 0.3,
		Currency:    "USD",
		UpdatedAt:   "2025-10-15",
	},
	ModelCodestralLatest: {
		Model:       ModelCodestralLatest,
		InputPrice:  0.3,
		OutputPrice: 0.9,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelCodestral2501: {
		Model:       ModelCodestral2501,
		InputPrice:  0.3,
		OutputPrice: 0.9,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelMistral7B: {
		Model:       ModelMistral7B,
		InputPrice:  0.25,
		OutputPrice: 0.25,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelMixtral8x7B: {
		Model:       ModelMixtral8x7B,
		InputPrice:  0.7,
		OutputPrice: 0.7,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelMixtral8x22B: {
		Model:       ModelMixtral8x22B,
		InputPrice:  2.0,
		OutputPrice: 6.0,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelCodestralMamba: {
		Model:       ModelCodestralMamba,
		InputPrice:  0.25,
		OutputPrice: 0.25,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}

// ImageModelPricing contains pricing for image generation models (currently none)
var ImageModelPricing = map[string]ImagePricingInfo{}

// EmbeddingModelPricing contains pricing for embedding models (currently none)
var EmbeddingModelPricing = map[string]EmbeddingPricingInfo{}
