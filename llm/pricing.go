package llm

// PricingInfo represents pricing information for a specific service
type PricingInfo struct {
	Model       string  `json:"model"`
	InputPrice  float64 `json:"input_price_per_1m_tokens"`  // per 1M input tokens (USD)
	OutputPrice float64 `json:"output_price_per_1m_tokens"` // per 1M output tokens (USD)
	// CacheReadPrice is the price per 1M tokens read from the prompt cache (a
	// cache hit). Zero means the provider does not bill cache reads separately.
	CacheReadPrice float64 `json:"cache_read_price_per_1m_tokens,omitempty"`
	// CacheWritePrice is the price per 1M tokens written to the prompt cache (a
	// cache miss). For providers with multiple cache TTLs this is the default
	// (shortest) TTL rate. Zero means the provider does not surcharge writes.
	CacheWritePrice float64 `json:"cache_write_price_per_1m_tokens,omitempty"`
	Currency        string  `json:"currency"`
	UpdatedAt       string  `json:"updated_at"` // YYYY-MM-DD format
}

// Cost is an estimated monetary cost broken out by token category. It is an
// estimate computed from a PricingInfo snapshot (list prices as of its
// UpdatedAt date), not an authoritative billing figure.
type Cost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	Total      float64 `json:"total"`
	Currency   string  `json:"currency,omitempty"`
	Model      string  `json:"model,omitempty"`
}

// Add accumulates another Cost into this one. It is nil-safe on the argument.
// Each summand was computed at its own call's prices, so summing per-call costs
// stays correct even across model or speed changes within a session.
func (c *Cost) Add(other *Cost) {
	if c == nil || other == nil {
		return
	}
	c.Input += other.Input
	c.Output += other.Output
	c.CacheRead += other.CacheRead
	c.CacheWrite += other.CacheWrite
	c.Total += other.Total
	if c.Currency == "" {
		c.Currency = other.Currency
	}
}

// CostOf computes the estimated cost of the given usage at these prices.
func (p PricingInfo) CostOf(u *Usage) Cost {
	const perMillion = 1_000_000.0
	c := Cost{
		Input:      float64(u.InputTokens) * p.InputPrice / perMillion,
		Output:     float64(u.OutputTokens) * p.OutputPrice / perMillion,
		CacheRead:  float64(u.CacheReadInputTokens) * p.CacheReadPrice / perMillion,
		CacheWrite: float64(u.CacheCreationInputTokens) * p.CacheWritePrice / perMillion,
		Currency:   p.Currency,
		Model:      p.Model,
	}
	c.Total = c.Input + c.Output + c.CacheRead + c.CacheWrite
	return c
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
