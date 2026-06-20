package llm

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestPricingInfoCostOf(t *testing.T) {
	p := PricingInfo{
		Model:           "test-model",
		InputPrice:      5.00,  // $5 / 1M
		OutputPrice:     25.00, // $25 / 1M
		CacheReadPrice:  0.50,  // 0.1x input
		CacheWritePrice: 6.25,  // 1.25x input
		Currency:        "USD",
	}
	u := &Usage{
		InputTokens:              1_000_000,
		OutputTokens:             1_000_000,
		CacheReadInputTokens:     1_000_000,
		CacheCreationInputTokens: 1_000_000,
	}
	c := p.CostOf(u)
	assert.Equal(t, 5.00, c.Input)
	assert.Equal(t, 25.00, c.Output)
	assert.Equal(t, 0.50, c.CacheRead)
	assert.Equal(t, 6.25, c.CacheWrite)
	assert.Equal(t, 36.75, c.Total)
	assert.Equal(t, "USD", c.Currency)
	assert.Equal(t, "test-model", c.Model)
}

func TestPricingInfoCostOf_ZeroCachePrices(t *testing.T) {
	// A provider that does not bill cache separately: cache tokens contribute 0.
	p := PricingInfo{InputPrice: 3.00, OutputPrice: 15.00, Currency: "USD"}
	u := &Usage{InputTokens: 2_000_000, OutputTokens: 100_000, CacheReadInputTokens: 9_000_000}
	c := p.CostOf(u)
	assert.Equal(t, 6.00, c.Input)
	assert.Equal(t, 1.50, c.Output)
	assert.Equal(t, 0.0, c.CacheRead)
	assert.Equal(t, 7.50, c.Total)
}

func TestUsageCostAddAndCopy(t *testing.T) {
	a := &Usage{InputTokens: 100, Cost: &Cost{Input: 1.0, Total: 1.0, Currency: "USD"}}
	b := &Usage{InputTokens: 50, Cost: &Cost{Input: 0.5, Output: 0.25, Total: 0.75}}

	a.Add(b)
	assert.Equal(t, 150, a.InputTokens)
	assert.NotNil(t, a.Cost)
	assert.Equal(t, 1.5, a.Cost.Input)
	assert.Equal(t, 0.25, a.Cost.Output)
	assert.Equal(t, 1.75, a.Cost.Total)
	assert.Equal(t, "USD", a.Cost.Currency)

	// Copy is deep: mutating the copy must not affect the original.
	cp := a.Copy()
	cp.Cost.Total = 999
	assert.Equal(t, 1.75, a.Cost.Total, "original cost must be unchanged after mutating copy")
}

func TestUsageAdd_NilCostSummand(t *testing.T) {
	a := &Usage{InputTokens: 10}
	a.Add(&Usage{InputTokens: 5}) // neither has cost
	assert.Nil(t, a.Cost, "no cost should remain nil")

	a.Add(&Usage{InputTokens: 5, Cost: &Cost{Total: 2.0}})
	assert.NotNil(t, a.Cost)
	assert.Equal(t, 2.0, a.Cost.Total)
}

func TestPopulateCost_ResolverRoundTrip(t *testing.T) {
	t.Cleanup(func() { SetCostResolver(nil) })

	SetCostResolver(func(model string, fast bool) (PricingInfo, bool) {
		if model != "known" {
			return PricingInfo{}, false
		}
		in := 5.0
		if fast {
			in = 10.0
		}
		return PricingInfo{Model: model, InputPrice: in, Currency: "USD"}, true
	})

	// Unknown model -> cost stays nil.
	u := &Usage{InputTokens: 1_000_000}
	PopulateCost("unknown", false, u)
	assert.Nil(t, u.Cost)

	// Known model, standard speed.
	PopulateCost("known", false, u)
	assert.NotNil(t, u.Cost)
	assert.Equal(t, 5.0, u.Cost.Total)

	// Known model, fast speed uses fast pricing.
	u2 := &Usage{InputTokens: 1_000_000}
	PopulateCost("known", true, u2)
	assert.Equal(t, 10.0, u2.Cost.Total)
}

func TestPopulateCost_NoResolver(t *testing.T) {
	SetCostResolver(nil)
	u := &Usage{InputTokens: 1_000_000}
	PopulateCost("anything", false, u)
	assert.Nil(t, u.Cost, "without a resolver, cost must remain unknown (nil)")
}

func TestResponseAccumulatorPopulatesCost(t *testing.T) {
	t.Cleanup(func() { SetCostResolver(nil) })
	SetCostResolver(func(model string, fast bool) (PricingInfo, bool) {
		if model != "priced-model" {
			return PricingInfo{}, false
		}
		return PricingInfo{Model: model, InputPrice: 3.0, OutputPrice: 15.0, Currency: "USD"}, true
	})

	acc := NewResponseAccumulator()
	assert.NoError(t, acc.AddEvent(&Event{
		Type:    EventTypeMessageStart,
		Message: &Response{ID: "msg_1", Role: Assistant, Model: "priced-model"},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeMessageDelta,
		Delta: &EventDelta{StopReason: "end_turn"},
		Usage: &Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
	}))
	assert.NoError(t, acc.AddEvent(&Event{Type: EventTypeMessageStop}))

	cost := acc.Response().Usage.Cost
	assert.NotNil(t, cost, "streaming accumulator should attach cost at completion")
	assert.Equal(t, 18.0, cost.Total) // 3 (input) + 15 (output)
	assert.Equal(t, "priced-model", cost.Model)
}
