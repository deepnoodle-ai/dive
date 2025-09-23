package llm

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
