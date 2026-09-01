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
	// Controls records the reasoning and sampling controls Dive actually sent,
	// so a caller can see that a request was clamped, translated, or dropped
	// rather than served exactly as asked. Nil means the provider reports no
	// effective controls for this request.
	Controls *EffectiveControls `json:"controls,omitempty"`
	// ControlsMixed distinguishes an aggregate whose requests were served with
	// different controls from one that has no controls to report. Controls is
	// nil in both cases; this flag says which, the way ServiceTier says "mixed".
	ControlsMixed bool `json:"controls_mixed,omitempty"`
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
	cp.ControlsMixed = u.ControlsMixed
	if u.Controls != nil {
		controlsCopy := u.Controls.Clone()
		cp.Controls = &controlsCopy
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

// Absorb merges a cumulative usage frame into this usage object. Streaming
// providers report running totals for the whole message rather than
// increments (Anthropic documents message_delta usage as cumulative), so a
// later frame supersedes an earlier one field by field. A zero token count
// means the frame omitted that field, so token buckets merge by max rather
// than wholesale replacement — message_delta frames that carry only
// output_tokens must not erase the input buckets seeded by message_start.
// Use Add for aggregating usage across separate requests.
func (u *Usage) Absorb(other *Usage) {
	if other == nil {
		return
	}
	u.InputTokens = max(u.InputTokens, other.InputTokens)
	u.OutputTokens = max(u.OutputTokens, other.OutputTokens)
	u.CacheCreationInputTokens = max(u.CacheCreationInputTokens, other.CacheCreationInputTokens)
	u.CacheReadInputTokens = max(u.CacheReadInputTokens, other.CacheReadInputTokens)
	u.ToolUseInputTokens = max(u.ToolUseInputTokens, other.ToolUseInputTokens)
	u.ReasoningTokens = max(u.ReasoningTokens, other.ReasoningTokens)
	if len(other.ModalityTokens) > 0 {
		if u.ModalityTokens == nil {
			u.ModalityTokens = make(map[string]ModalityTokenUsage, len(other.ModalityTokens))
		}
		for modality, frame := range other.ModalityTokens {
			current := u.ModalityTokens[modality]
			current.InputTokens = max(current.InputTokens, frame.InputTokens)
			current.OutputTokens = max(current.OutputTokens, frame.OutputTokens)
			current.CacheCreationInputTokens = max(current.CacheCreationInputTokens, frame.CacheCreationInputTokens)
			current.CacheReadInputTokens = max(current.CacheReadInputTokens, frame.CacheReadInputTokens)
			u.ModalityTokens[modality] = current
		}
	}
	if other.ServiceTier != "" {
		u.ServiceTier = other.ServiceTier
	}
	if other.Speed != "" {
		u.Speed = other.Speed
	}
	if other.Controls != nil {
		controlsCopy := other.Controls.Clone()
		u.Controls = &controlsCopy
		u.ControlsMixed = false
	}
	u.CacheCreationInputTokensUnavailable = u.CacheCreationInputTokensUnavailable || other.CacheCreationInputTokensUnavailable
	u.InputModalityTokenDetailsIncomplete = u.InputModalityTokenDetailsIncomplete || other.InputModalityTokenDetailsIncomplete
	u.OutputModalityTokenDetailsIncomplete = u.OutputModalityTokenDetailsIncomplete || other.OutputModalityTokenDetailsIncomplete
	u.CacheReadModalityTokenDetailsIncomplete = u.CacheReadModalityTokenDetailsIncomplete || other.CacheReadModalityTokenDetailsIncomplete
	u.CostEstimateUnavailable = u.CostEstimateUnavailable || other.CostEstimateUnavailable
	if u.CostEstimateUnavailable {
		u.Cost = nil
	} else if other.Cost != nil {
		// Cumulative frames supersede: the later provider-reported cost
		// replaces the earlier one rather than summing with it.
		costCopy := *other.Cost
		u.Cost = &costCopy
	}
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
	// Effective controls describe a single request, so an aggregate reports
	// them only while the requests that reported any agreed. A disagreement is
	// sticky: it clears the field and latches ControlsMixed rather than letting
	// a later request speak for the sum, the same way ServiceTier stays "mixed".
	u.ControlsMixed = u.ControlsMixed || other.ControlsMixed
	switch {
	case u.ControlsMixed:
		u.Controls = nil
	case u.Controls == nil:
		if other.Controls != nil {
			controlsCopy := other.Controls.Clone()
			u.Controls = &controlsCopy
		}
	case other.Controls != nil && !u.Controls.Equal(*other.Controls):
		u.Controls = nil
		u.ControlsMixed = true
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
		} else {
			u.Cost.Add(other.Cost)
		}
	}
}
