package google

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
	ModelGemini25Flash: {
		Model:       ModelGemini25Flash,
		InputPrice:  0.15,
		OutputPrice: 0.60,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGemini25FlashLite: {
		Model:       ModelGemini25FlashLite,
		InputPrice:  0.075, // Estimated as typically half of Flash
		OutputPrice: 0.30,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	ModelGemini25FlashPro: {
		Model:       ModelGemini25FlashPro,
		InputPrice:  1.25,
		OutputPrice: 10.00, // Up to 200K tokens
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"gemini-2.5-pro-long": {
		Model:       "gemini-2.5-pro-long",
		InputPrice:  2.50, // Over 200K tokens
		OutputPrice: 15.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"gemini-2.0-flash": {
		Model:       "gemini-2.0-flash",
		InputPrice:  0.10,
		OutputPrice: 0.40,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"gemini-1.5-pro": {
		Model:       "gemini-1.5-pro",
		InputPrice:  1.25,
		OutputPrice: 5.00,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
	"gemini-1.5-flash": {
		Model:       "gemini-1.5-flash",
		InputPrice:  0.15,
		OutputPrice: 0.60,
		Currency:    "USD",
		UpdatedAt:   "2025-01-15",
	},
}

// ImageModelPricing contains pricing for image generation models
var ImageModelPricing = map[string]ImagePricingInfo{
	"gemini-2.5-flash-image": {
		Model:     "gemini-2.5-flash-image",
		Price:     0.039, // $30 per 1M tokens, 1290 tokens per 1024x1024 image
		MaxSize:   "1024x1024",
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
}

// EmbeddingModelPricing contains pricing for embedding models
var EmbeddingModelPricing = map[string]EmbeddingPricingInfo{
	"text-embedding-004": {
		Model:     "text-embedding-004",
		Price:     0.0625, // $0.0000625 per 1K tokens = $0.0625 per 1M tokens
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
	"text-multilingual-embedding-002": {
		Model:     "text-multilingual-embedding-002",
		Price:     0.0625, // Same as text-embedding-004
		Currency:  "USD",
		UpdatedAt: "2025-01-15",
	},
}