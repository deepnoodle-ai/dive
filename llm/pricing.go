package llm

const (
	CostSourceListPriceEstimate = "list_price_estimate"
	CostSourceProviderReported  = "provider_reported"
	CostSourceMixed             = "mixed"
	CostModelMixed              = "mixed"
)

// PricingInfo represents pricing information for a specific service
type PricingInfo struct {
	Model       string  `json:"model"`
	InputPrice  float64 `json:"input_price_per_1m_tokens"`  // per 1M input tokens (USD)
	OutputPrice float64 `json:"output_price_per_1m_tokens"` // per 1M output tokens (USD)
	// Per-modality prices override the corresponding base price only for the
	// provider-reported tokens in that modality. Missing modalities retain the
	// base price. This is required for providers such as Gemini that price audio
	// input differently from text, image, and video input.
	InputPriceByModality     map[string]float64 `json:"input_price_per_1m_tokens_by_modality,omitempty"`
	OutputPriceByModality    map[string]float64 `json:"output_price_per_1m_tokens_by_modality,omitempty"`
	CacheReadPriceByModality map[string]float64 `json:"cache_read_price_per_1m_tokens_by_modality,omitempty"`
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
	// NonGlobalPriceMultiplier is applied by providers whose list price differs
	// between global and regional endpoints. CostOf itself remains location-free.
	NonGlobalPriceMultiplier float64 `json:"non_global_price_multiplier,omitempty"`
	Currency                 string  `json:"currency"`
	UpdatedAt                string  `json:"updated_at"` // YYYY-MM-DD format
}

// Cost is a monetary cost broken out by token category. Source distinguishes a
// provider-reported account charge from a list-price estimate. Some providers
// report only an authoritative Total, in which case BreakdownUnavailable is
// true and the category fields must not be treated as measured zeroes.
type Cost struct {
	Input                float64 `json:"input"`
	Output               float64 `json:"output"`
	CacheRead            float64 `json:"cache_read"`
	CacheWrite           float64 `json:"cache_write"`
	Total                float64 `json:"total"`
	Currency             string  `json:"currency,omitempty"`
	Model                string  `json:"model,omitempty"`
	Source               string  `json:"source,omitempty"`
	BreakdownUnavailable bool    `json:"breakdown_unavailable,omitempty"`
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
	if c.Model == "" {
		c.Model = other.Model
	} else if other.Model != "" && other.Model != c.Model {
		c.Model = CostModelMixed
	}
	if c.Source == "" {
		c.Source = other.Source
	} else if other.Source != "" && other.Source != c.Source {
		c.Source = CostSourceMixed
	}
	c.BreakdownUnavailable = c.BreakdownUnavailable || other.BreakdownUnavailable
}

// CostOf computes the estimated cost of the given usage at these prices.
// A nil usage yields a zero cost (still tagged with currency and model).
func (p PricingInfo) CostOf(u *Usage) Cost {
	if u == nil {
		return Cost{Currency: p.Currency, Model: p.Model, Source: CostSourceListPriceEstimate}
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
		Source:     CostSourceListPriceEstimate,
	}
	for modality, usage := range u.ModalityTokens {
		if price, ok := p.InputPriceByModality[modality]; ok {
			c.Input += float64(usage.InputTokens) * (price - inputPrice) / perMillion
		}
		if price, ok := p.OutputPriceByModality[modality]; ok {
			c.Output += float64(usage.OutputTokens) * (price - outputPrice) / perMillion
		}
		if price, ok := p.CacheReadPriceByModality[modality]; ok {
			c.CacheRead += float64(usage.CacheReadInputTokens) * (price - cacheReadPrice) / perMillion
		}
	}
	c.Total = c.Input + c.Output + c.CacheRead + c.CacheWrite
	return c
}

// Scaled returns a deep copy with every token price multiplied by factor.
// Thresholds and descriptive metadata are preserved. The regional multiplier
// is cleared because the returned prices already include the adjustment.
func (p PricingInfo) Scaled(factor float64) PricingInfo {
	p.InputPrice *= factor
	p.OutputPrice *= factor
	p.LongContextInputPrice *= factor
	p.LongContextCacheReadPrice *= factor
	p.LongContextOutputPrice *= factor
	p.CacheReadPrice *= factor
	p.CacheReadPriceAboveThreshold *= factor
	p.CacheWritePrice *= factor
	p.InputPriceByModality = scalePriceMap(p.InputPriceByModality, factor)
	p.OutputPriceByModality = scalePriceMap(p.OutputPriceByModality, factor)
	p.CacheReadPriceByModality = scalePriceMap(p.CacheReadPriceByModality, factor)
	p.NonGlobalPriceMultiplier = 0
	return p
}

func scalePriceMap(prices map[string]float64, factor float64) map[string]float64 {
	if len(prices) == 0 {
		return nil
	}
	scaled := make(map[string]float64, len(prices))
	for modality, price := range prices {
		scaled[modality] = price * factor
	}
	return scaled
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
