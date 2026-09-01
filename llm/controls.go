package llm

// EffectiveControls is the provider-neutral set of reasoning and sampling
// controls Dive resolved for one request. It is not a serialized provider
// request: it names what Dive decided in Dive's own vocabulary, so a caller can
// compare what it asked for against what was sent without parsing a provider
// payload.
//
// Dive clamps, translates, and drops controls a model cannot take, and the
// request succeeds. This type is how that adjustment becomes observable: it is
// produced by the same planner that builds the wire request, both before the
// call (modelcaps.Preview) and on the response (Usage.Controls).
//
// A zero-valued field means Dive sent nothing for that control.
type EffectiveControls struct {
	noUnkeyedLiterals struct{}

	// ReasoningEffort is the effort level sent as a native provider parameter.
	// It is empty when no effort parameter was sent, including when an effort
	// was emulated as a reasoning budget instead.
	ReasoningEffort ReasoningEffort `json:"reasoning_effort,omitempty"`

	// ReasoningBudget is the manual thinking-token budget sent, if any.
	ReasoningBudget *int `json:"reasoning_budget,omitempty"`

	// Thinking is the thinking mode the request resolved to, including a mode
	// the model applies by default when the caller requested none.
	Thinking ThinkingType `json:"thinking,omitempty"`

	// Temperature is the sampling temperature sent, if any.
	Temperature *float64 `json:"temperature,omitempty"`
}

// Clone returns a deep copy so callers cannot mutate shared pointer fields.
func (c EffectiveControls) Clone() EffectiveControls {
	if c.ReasoningBudget != nil {
		budget := *c.ReasoningBudget
		c.ReasoningBudget = &budget
	}
	if c.Temperature != nil {
		temperature := *c.Temperature
		c.Temperature = &temperature
	}
	return c
}

// Equal compares by value, treating nil and a pointer to an equal value as
// different: nil means the control was not sent at all.
func (c EffectiveControls) Equal(other EffectiveControls) bool {
	return c.ReasoningEffort == other.ReasoningEffort &&
		c.Thinking == other.Thinking &&
		equalIntPtr(c.ReasoningBudget, other.ReasoningBudget) &&
		equalFloatPtr(c.Temperature, other.Temperature)
}

func equalIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func equalFloatPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
