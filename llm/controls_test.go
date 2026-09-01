package llm

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func testControls(budget int, temperature float64) *EffectiveControls {
	return &EffectiveControls{
		ReasoningEffort: ReasoningEffortHigh,
		ReasoningBudget: &budget,
		Thinking:        ThinkingTypeEnabled,
		Temperature:     &temperature,
	}
}

func TestEffectiveControlsCloneIsDeep(t *testing.T) {
	original := testControls(4096, 0.4)
	clone := original.Clone()
	*clone.ReasoningBudget = 1
	*clone.Temperature = 1

	assert.Equal(t, 4096, *original.ReasoningBudget)
	assert.Equal(t, 0.4, *original.Temperature)
}

func TestEffectiveControlsEqualDistinguishesUnsetFromZero(t *testing.T) {
	zeroBudget := 0
	unset := EffectiveControls{}
	sentZero := EffectiveControls{ReasoningBudget: &zeroBudget}

	assert.False(t, unset.Equal(sentZero), "a budget of 0 is not the same as sending none")
	assert.True(t, unset.Equal(EffectiveControls{}))
	assert.True(t, sentZero.Equal(EffectiveControls{ReasoningBudget: &zeroBudget}))
	assert.False(t, testControls(4096, 0.4).Equal(*testControls(4096, 0.5)))
}

func TestUsageCopyIsolatesControls(t *testing.T) {
	usage := &Usage{InputTokens: 10, Controls: testControls(4096, 0.4)}
	copied := usage.Copy()
	*copied.Controls.ReasoningBudget = 1

	assert.Equal(t, 4096, *usage.Controls.ReasoningBudget)
}

func TestUsageAbsorbSupersedesControls(t *testing.T) {
	// Streaming frames are cumulative, so a later frame replaces the earlier
	// report rather than being merged into it.
	usage := &Usage{Controls: testControls(4096, 0.4)}
	usage.Absorb(&Usage{Controls: testControls(8192, 0.4)})
	assert.Equal(t, 8192, *usage.Controls.ReasoningBudget)

	// A frame that carries no controls leaves the earlier report standing.
	usage.Absorb(&Usage{OutputTokens: 3})
	assert.Equal(t, 8192, *usage.Controls.ReasoningBudget)
}

func TestUsageAddKeepsAgreeingControlsAndClearsDisagreeing(t *testing.T) {
	agreed := &Usage{InputTokens: 1, Controls: testControls(4096, 0.4)}
	agreed.Add(&Usage{InputTokens: 1, Controls: testControls(4096, 0.4)})
	assert.NotNil(t, agreed.Controls)
	assert.Equal(t, 4096, *agreed.Controls.ReasoningBudget)

	// Two turns served with different controls cannot be summarized by either,
	// and a third agreeing turn must not resurrect one of them.
	mixed := &Usage{InputTokens: 1, Controls: testControls(4096, 0.4)}
	mixed.Add(&Usage{InputTokens: 1, Controls: testControls(8192, 0.4)})
	assert.Nil(t, mixed.Controls)
	assert.True(t, mixed.ControlsMixed)

	mixed.Add(&Usage{InputTokens: 1, Controls: testControls(8192, 0.4)})
	assert.Nil(t, mixed.Controls)
	assert.True(t, mixed.ControlsMixed)

	// The mixed state propagates into a larger aggregate.
	total := &Usage{Controls: testControls(8192, 0.4)}
	total.Add(mixed)
	assert.Nil(t, total.Controls)
	assert.True(t, total.ControlsMixed)
}

func TestUsageCopyCarriesTheMixedControlsFlag(t *testing.T) {
	usage := &Usage{ControlsMixed: true}
	assert.True(t, usage.Copy().ControlsMixed)
}

func TestUsageAbsorbClearsTheMixedControlsFlag(t *testing.T) {
	// Absorb merges cumulative frames of one request, so a frame that reports
	// controls is authoritative rather than a second opinion.
	usage := &Usage{ControlsMixed: true}
	usage.Absorb(&Usage{Controls: testControls(4096, 0.4)})
	assert.False(t, usage.ControlsMixed)
	assert.NotNil(t, usage.Controls)
}

func TestUsageAddIsolatesControlsFromTheSource(t *testing.T) {
	source := &Usage{Controls: testControls(4096, 0.4)}
	total := &Usage{}
	total.Add(source)
	*total.Controls.ReasoningBudget = 1

	assert.Equal(t, 4096, *source.Controls.ReasoningBudget)
}

func TestUsageControlsRoundTripThroughJSON(t *testing.T) {
	data, err := json.Marshal(&Usage{InputTokens: 1, Controls: testControls(4096, 0.4)})
	assert.NoError(t, err)

	var decoded Usage
	assert.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, ReasoningEffortHigh, decoded.Controls.ReasoningEffort)
	assert.Equal(t, 4096, *decoded.Controls.ReasoningBudget)
	assert.Equal(t, ThinkingTypeEnabled, decoded.Controls.Thinking)
	assert.Equal(t, 0.4, *decoded.Controls.Temperature)

	// A usage frame with no controls stays absent rather than empty.
	data, err = json.Marshal(&Usage{InputTokens: 1})
	assert.NoError(t, err)
	var fields map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(data, &fields))
	_, present := fields["controls"]
	assert.False(t, present)
}
