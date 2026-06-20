package llm

// Usage contains token usage information for an LLM response.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	// ReasoningTokens is the number of output tokens spent on reasoning, when
	// the provider reports it separately (e.g. OpenAI o-series and Grok
	// reasoning models). It is a subset of OutputTokens, not additive.
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
	// Speed indicates which inference speed served the request, either "fast"
	// or "standard". Populated by Anthropic when fast mode is requested.
	Speed string `json:"speed,omitempty"`
	// Cost is the estimated monetary cost of this usage, populated by the
	// provider (or the streaming accumulator) when model pricing is known. Nil
	// means cost is unknown — distinct from a known cost of zero (e.g. a local
	// model). It is an estimate at list prices, not a billing figure.
	Cost *Cost `json:"cost,omitempty"`
}

// Copy returns a deep copy of the usage data.
func (u *Usage) Copy() *Usage {
	cp := &Usage{
		InputTokens:              u.InputTokens,
		OutputTokens:             u.OutputTokens,
		CacheCreationInputTokens: u.CacheCreationInputTokens,
		CacheReadInputTokens:     u.CacheReadInputTokens,
		ReasoningTokens:          u.ReasoningTokens,
		Speed:                    u.Speed,
	}
	if u.Cost != nil {
		costCopy := *u.Cost
		cp.Cost = &costCopy
	}
	return cp
}

// Add incremental usage to this usage object.
func (u *Usage) Add(other *Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheCreationInputTokens += other.CacheCreationInputTokens
	u.CacheReadInputTokens += other.CacheReadInputTokens
	u.ReasoningTokens += other.ReasoningTokens
	if other.Cost != nil {
		if u.Cost == nil {
			u.Cost = &Cost{}
		}
		u.Cost.Add(other.Cost)
	}
}
