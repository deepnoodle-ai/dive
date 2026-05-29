package llm

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestReasoningEffort_IsValid(t *testing.T) {
	cases := []struct {
		effort ReasoningEffort
		valid  bool
	}{
		{ReasoningEffortMinimal, true},
		{ReasoningEffortLow, true},
		{ReasoningEffortMedium, true},
		{ReasoningEffortHigh, true},
		{ReasoningEffortXHigh, true},
		{ReasoningEffortMax, true},
		{ReasoningEffort(""), false},
		{ReasoningEffort("none"), false},
		{ReasoningEffort("nope"), false},
	}
	for _, c := range cases {
		assert.Equal(t, c.valid, c.effort.IsValid(),
			"IsValid(%q) should be %v", string(c.effort), c.valid)
	}
}

func TestWithReasoningEffort_Minimal(t *testing.T) {
	cfg := &Config{}
	cfg.Apply(WithReasoningEffort(ReasoningEffortMinimal))
	assert.Equal(t, ReasoningEffortMinimal, cfg.ReasoningEffort)
}
