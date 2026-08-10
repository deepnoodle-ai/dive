package llm

import "encoding/json"

// Usage contains token usage information for an LLM response.
//
// The input-side token buckets are mutually disjoint. InputTokens counts only
// uncached input tokens that were not written to a prompt cache;
// CacheCreationInputTokens counts cache writes; and CacheReadInputTokens counts
// cache hits. The full input size is their sum, exposed by TotalInputTokens.
// This differs deliberately from ReasoningTokens, which is a subset of
// OutputTokens rather than an additive bucket.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	// ReasoningTokens is the number of output tokens spent on reasoning, when
	// the provider reports it separately (e.g. OpenAI o-series, Grok reasoning
	// models, and Anthropic extended thinking). It is a subset of OutputTokens,
	// not additive.
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

// TotalInputTokens returns the full input size across the disjoint input-side
// token buckets.
func (u Usage) TotalInputTokens() int {
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

// UnmarshalJSON accepts provider-native usage shapes and maps their cache and
// reasoning breakdowns onto Dive's canonical token buckets.
func (u *Usage) UnmarshalJSON(data []byte) error {
	type usageAlias Usage
	type inputTokensDetails struct {
		CachedTokens int `json:"cached_tokens,omitempty"`
	}
	var raw struct {
		usageAlias
		InputTokensDetails  *inputTokensDetails `json:"input_tokens_details,omitempty"`
		PromptTokens        *int                `json:"prompt_tokens,omitempty"`
		PromptTokensDetails *inputTokensDetails `json:"prompt_tokens_details,omitempty"`
		OutputTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens,omitempty"`
			ThinkingTokens  int `json:"thinking_tokens,omitempty"`
		} `json:"output_tokens_details,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*u = Usage(raw.usageAlias)

	// An explicit canonical cache bucket, including zero, declares that the
	// input fields are already disjoint. Provider-native detail objects must not
	// reinterpret that data.
	_, hasCanonicalCacheRead := fields["cache_read_input_tokens"]
	_, hasCanonicalCacheCreation := fields["cache_creation_input_tokens"]
	if !hasCanonicalCacheRead && !hasCanonicalCacheCreation {
		normalize := func(prompt, cached int) {
			prompt = max(0, prompt)
			cached = min(max(0, cached), prompt)
			input := prompt - cached
			if u.InputTokens != input || u.CacheReadInputTokens != cached {
				u.Cost = nil
			}
			u.InputTokens = input
			u.CacheReadInputTokens = cached
		}

		_, hasInputTokens := fields["input_tokens"]
		_, hasInputDetails := fields["input_tokens_details"]
		_, hasPromptTokens := fields["prompt_tokens"]
		_, hasPromptDetails := fields["prompt_tokens_details"]
		switch {
		case hasInputTokens && hasInputDetails && raw.InputTokensDetails != nil:
			normalize(u.InputTokens, raw.InputTokensDetails.CachedTokens)
		case hasPromptTokens && hasPromptDetails && raw.PromptTokens != nil && raw.PromptTokensDetails != nil:
			normalize(*raw.PromptTokens, raw.PromptTokensDetails.CachedTokens)
		}
	}

	if u.ReasoningTokens == 0 && raw.OutputTokensDetails != nil {
		switch {
		case raw.OutputTokensDetails.ReasoningTokens != 0:
			u.ReasoningTokens = raw.OutputTokensDetails.ReasoningTokens
		case raw.OutputTokensDetails.ThinkingTokens != 0:
			u.ReasoningTokens = raw.OutputTokensDetails.ThinkingTokens
		}
	}
	return nil
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
