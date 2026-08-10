package llm

// PricingInfo represents pricing information for a specific service
type PricingInfo struct {
	Model       string  `json:"model"`
	InputPrice  float64 `json:"input_price_per_1m_tokens"`  // per 1M input tokens (USD)
	OutputPrice float64 `json:"output_price_per_1m_tokens"` // per 1M output tokens (USD)
	// LongContextThreshold selects the long-context input, cache-read, and
	// output prices when the request's full input size is at least this many
	// tokens. Zero means the model has no unified long-context pricing tier.
	LongContextThreshold      int     `json:"long_context_threshold_tokens,omitempty"`
	LongContextInputPrice     float64 `json:"long_context_input_price_per_1m_tokens,omitempty"`
	LongContextCacheReadPrice float64 `json:"long_context_cache_read_price_per_1m_tokens,omitempty"`
	LongContextOutputPrice    float64 `json:"long_context_output_price_per_1m_tokens,omitempty"`
	// CacheReadPrice is the price per 1M tokens read from the prompt cache (a
	// cache hit). Zero means the provider does not bill cache reads separately.
	CacheReadPrice float64 `json:"cache_read_price_per_1m_tokens,omitempty"`
	// CacheReadPriceAboveThreshold replaces CacheReadPrice when the request's
	// full input size exceeds CacheReadPriceThreshold tokens. Both fields are
	// zero for models without tiered cache-read pricing.
	CacheReadPriceAboveThreshold float64 `json:"cache_read_price_above_threshold_per_1m_tokens,omitempty"`
	CacheReadPriceThreshold      int     `json:"cache_read_price_threshold_tokens,omitempty"`
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
// A nil usage yields a zero cost (still tagged with currency and model).
func (p PricingInfo) CostOf(u *Usage) Cost {
	if u == nil {
		return Cost{Currency: p.Currency, Model: p.Model}
	}
	const perMillion = 1_000_000.0
	inputPrice := p.InputPrice
	outputPrice := p.OutputPrice
	cacheReadPrice := p.CacheReadPrice
	totalInput := u.TotalInputTokens()
	if p.LongContextThreshold > 0 && totalInput >= p.LongContextThreshold {
		inputPrice = p.LongContextInputPrice
		outputPrice = p.LongContextOutputPrice
		cacheReadPrice = p.LongContextCacheReadPrice
	} else if p.CacheReadPriceThreshold > 0 && totalInput > p.CacheReadPriceThreshold {
		cacheReadPrice = p.CacheReadPriceAboveThreshold
	}
	c := Cost{
		Input:      float64(u.InputTokens) * inputPrice / perMillion,
		Output:     float64(u.OutputTokens) * outputPrice / perMillion,
		CacheRead:  float64(u.CacheReadInputTokens) * cacheReadPrice / perMillion,
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
