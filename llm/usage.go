package llm

import "encoding/json"

// ModalityTokenUsage is a disjoint token breakdown for one provider modality.
// Its input buckets follow the same convention as Usage; OutputTokens includes
// any reasoning tokens when the provider attributes them to the modality.
type ModalityTokenUsage struct {
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// Usage contains token usage information for an LLM response.
//
// The input-side token buckets are mutually disjoint. InputTokens counts
// uncached input; CacheCreationInputTokens counts the subset reported as cache
// writes; and CacheReadInputTokens counts cache hits. When a provider does not
// expose cache writes, InputTokens contains all uncached input and
// CacheCreationInputTokensUnavailable is true. The full input size is their
// sum, exposed by TotalInputTokens.
// This differs deliberately from ReasoningTokens, which is a subset of
// OutputTokens rather than an additive bucket.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	// ToolUseInputTokens is the subset of InputTokens attributable to results
	// from provider-managed tool executions that were supplied back to the
	// model. It is not additive, just as ReasoningTokens is not additive.
	ToolUseInputTokens int `json:"tool_use_input_tokens,omitempty"`
	// ReasoningTokens is the number of output tokens spent on reasoning, when
	// the provider reports it separately (e.g. OpenAI o-series, Grok reasoning
	// models, and Anthropic extended thinking). It is a subset of OutputTokens,
	// not additive.
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
	// ModalityTokens preserves provider-reported modality detail using the same
	// disjoint buckets as the aggregate fields. Keys are lower-case provider
	// modality names such as "text", "audio", "image", and "video".
	ModalityTokens map[string]ModalityTokenUsage `json:"modality_tokens,omitempty"`
	// The incomplete flags are true when aggregate usage is exact but the
	// provider omitted detail needed to assign that category to modalities.
	InputModalityTokenDetailsIncomplete     bool `json:"input_modality_token_details_incomplete,omitempty"`
	OutputModalityTokenDetailsIncomplete    bool `json:"output_modality_token_details_incomplete,omitempty"`
	CacheReadModalityTokenDetailsIncomplete bool `json:"cache_read_modality_token_details_incomplete,omitempty"`
	// ServiceTier records the serving tier used for this request when known.
	// Aggregating requests served by different tiers produces "mixed".
	ServiceTier string `json:"service_tier,omitempty"`
	// CacheCreationInputTokensUnavailable distinguishes a measured zero from a
	// provider that exposes cache reads but no cache-creation token metric.
	CacheCreationInputTokensUnavailable bool `json:"cache_creation_input_tokens_unavailable,omitempty"`
	// CostEstimateUnavailable prevents a provider-specific unknown price from
	// being replaced by a generic model-only estimate. Cost remains nil.
	CostEstimateUnavailable bool `json:"cost_estimate_unavailable,omitempty"`
	// Speed indicates which inference speed served the request, either "fast"
	// or "standard". Populated by Anthropic when fast mode is requested.
	Speed string `json:"speed,omitempty"`
	// Cost is the monetary cost of this usage. Providers attach an authoritative
	// charge when they report one; otherwise Dive may estimate from cataloged
	// list prices. Nil means cost is unknown, distinct from a known zero.
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
		CachedTokens     int `json:"cached_tokens,omitempty"`
		CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
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
		normalize := func(prompt, cached, written int) {
			prompt = max(0, prompt)
			cached = min(max(0, cached), prompt)
			written = min(max(0, written), prompt-cached)
			input := prompt - cached - written
			if u.InputTokens != input || u.CacheReadInputTokens != cached || u.CacheCreationInputTokens != written {
				u.Cost = nil
			}
			u.InputTokens = input
			u.CacheReadInputTokens = cached
			u.CacheCreationInputTokens = written
		}

		_, hasInputTokens := fields["input_tokens"]
		_, hasInputDetails := fields["input_tokens_details"]
		_, hasPromptTokens := fields["prompt_tokens"]
		_, hasPromptDetails := fields["prompt_tokens_details"]
		switch {
		case hasInputTokens && hasInputDetails && raw.InputTokensDetails != nil:
			normalize(u.InputTokens, raw.InputTokensDetails.CachedTokens, raw.InputTokensDetails.CacheWriteTokens)
		case hasPromptTokens && hasPromptDetails && raw.PromptTokens != nil && raw.PromptTokensDetails != nil:
			normalize(*raw.PromptTokens, raw.PromptTokensDetails.CachedTokens, raw.PromptTokensDetails.CacheWriteTokens)
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
		InputTokens:                             u.InputTokens,
		OutputTokens:                            u.OutputTokens,
		CacheCreationInputTokens:                u.CacheCreationInputTokens,
		CacheReadInputTokens:                    u.CacheReadInputTokens,
		ToolUseInputTokens:                      u.ToolUseInputTokens,
		ReasoningTokens:                         u.ReasoningTokens,
		Speed:                                   u.Speed,
		ServiceTier:                             u.ServiceTier,
		InputModalityTokenDetailsIncomplete:     u.InputModalityTokenDetailsIncomplete,
		OutputModalityTokenDetailsIncomplete:    u.OutputModalityTokenDetailsIncomplete,
		CacheReadModalityTokenDetailsIncomplete: u.CacheReadModalityTokenDetailsIncomplete,
		CacheCreationInputTokensUnavailable:     u.CacheCreationInputTokensUnavailable,
		CostEstimateUnavailable:                 u.CostEstimateUnavailable,
	}
	if len(u.ModalityTokens) > 0 {
		cp.ModalityTokens = make(map[string]ModalityTokenUsage, len(u.ModalityTokens))
		for modality, usage := range u.ModalityTokens {
			cp.ModalityTokens[modality] = usage
		}
	}
	if u.Cost != nil {
		costCopy := *u.Cost
		cp.Cost = &costCopy
	}
	return cp
}

// Add incremental usage to this usage object.
func (u *Usage) Add(other *Usage) {
	if other == nil {
		return
	}
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheCreationInputTokens += other.CacheCreationInputTokens
	u.CacheReadInputTokens += other.CacheReadInputTokens
	u.ToolUseInputTokens += other.ToolUseInputTokens
	u.ReasoningTokens += other.ReasoningTokens
	if len(other.ModalityTokens) > 0 {
		if u.ModalityTokens == nil {
			u.ModalityTokens = make(map[string]ModalityTokenUsage, len(other.ModalityTokens))
		}
		for modality, addition := range other.ModalityTokens {
			current := u.ModalityTokens[modality]
			current.InputTokens += addition.InputTokens
			current.OutputTokens += addition.OutputTokens
			current.CacheCreationInputTokens += addition.CacheCreationInputTokens
			current.CacheReadInputTokens += addition.CacheReadInputTokens
			u.ModalityTokens[modality] = current
		}
	}
	if u.ServiceTier == "" {
		u.ServiceTier = other.ServiceTier
	} else if other.ServiceTier != "" && other.ServiceTier != u.ServiceTier {
		u.ServiceTier = "mixed"
	}
	u.CacheCreationInputTokensUnavailable = u.CacheCreationInputTokensUnavailable || other.CacheCreationInputTokensUnavailable
	u.InputModalityTokenDetailsIncomplete = u.InputModalityTokenDetailsIncomplete || other.InputModalityTokenDetailsIncomplete
	u.OutputModalityTokenDetailsIncomplete = u.OutputModalityTokenDetailsIncomplete || other.OutputModalityTokenDetailsIncomplete
	u.CacheReadModalityTokenDetailsIncomplete = u.CacheReadModalityTokenDetailsIncomplete || other.CacheReadModalityTokenDetailsIncomplete
	u.CostEstimateUnavailable = u.CostEstimateUnavailable || other.CostEstimateUnavailable
	if u.CostEstimateUnavailable {
		u.Cost = nil
	} else if other.Cost != nil {
		if u.Cost == nil {
			costCopy := *other.Cost
			u.Cost = &costCopy
			return
		}
		u.Cost.Add(other.Cost)
	}
}
