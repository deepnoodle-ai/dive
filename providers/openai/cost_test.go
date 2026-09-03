package openai

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/dive/providers/openaicompletions"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go/v3/responses"
)

func TestOpenAIPricingRegistered(t *testing.T) {
	if len(TextModelPricing) == 0 {
		t.Skip("no openai pricing entries")
	}
	for model := range TextModelPricing {
		_, ok := providers.PricingFor(model, false)
		assert.True(t, ok, "openai pricing should be registered: "+model)
	}
}

func TestOpenAIPopulateCost(t *testing.T) {
	var model string
	for m, p := range TextModelPricing {
		if p.InputPrice > 0 {
			model = m
			break
		}
	}
	if model == "" {
		t.Skip("no priced openai model")
	}
	u := &llm.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}
	llm.PopulateCost(model, false, u)
	assert.NotNil(t, u.Cost, "cost should populate via the registry resolver")
	assert.True(t, u.Cost.Total > 0, "priced model should yield positive cost")
}

// GPT-6 Astra is the first OpenAI model in the catalog to carry a long-context
// tier. OpenAI publishes it as a rule rather than a table -- above 272K input
// tokens, input and cache rates double and output is billed at 1.5x -- so the
// arithmetic is pinned here alongside the rates.
func TestGPT6AstraLongContextPricing(t *testing.T) {
	p, ok := TextModelPricing[ModelGPT6Astra]
	assert.True(t, ok, "pricing should exist for "+ModelGPT6Astra)
	assert.Equal(t, p.InputPrice, 10.0)
	assert.Equal(t, p.CacheReadPrice, 1.0)
	assert.Equal(t, p.CacheWritePrice, 12.50)
	assert.Equal(t, p.OutputPrice, 50.0)

	assert.Equal(t, p.LongContextThreshold, 272_001)
	assert.Equal(t, p.LongContextInputPrice, p.InputPrice*2)
	assert.Equal(t, p.LongContextCacheReadPrice, p.CacheReadPrice*2)
	assert.Equal(t, p.LongContextOutputPrice, p.OutputPrice*1.5)

	// Output is billed at the tier the *input* size selects, so a short prompt
	// with a long completion stays on standard rates.
	short := p.CostOf(&llm.Usage{InputTokens: 200_000, OutputTokens: 100_000})
	assert.Equal(t, short.Input, 2.0)
	assert.Equal(t, short.Output, 5.0)

	long := p.CostOf(&llm.Usage{InputTokens: 300_000, OutputTokens: 100_000})
	assert.Equal(t, long.Input, 6.0)
	assert.Equal(t, long.Output, 7.5)

	// The published long-context cache-write rate is $25.00/1M. PricingInfo has
	// no field for it, so a cache write on a long request is understated here.
	longWrite := p.CostOf(&llm.Usage{InputTokens: 300_000, CacheCreationInputTokens: 1_000_000})
	assert.Equal(t, longWrite.CacheWrite, 12.50)
}

func TestOpenAICacheReadPricingCoverage(t *testing.T) {
	expected := map[string]float64{
		ModelGPT6Astra:  1.00,
		ModelGPT56:      0.50,
		ModelGPT56Sol:   0.50,
		ModelGPT56Terra: 0.25,
		ModelGPT56Luna:  0.10,
		ModelGPT55:      0.50,
		ModelGPT54:      0.25,
		ModelGPT54Mini:  0.075,
		ModelGPT54Nano:  0.02,
		ModelGPT52:      0.175,
		ModelGPT51:      0.125,
		ModelGPT5:       0.125,
		ModelGPT5Mini:   0.025,
		ModelGPT5Nano:   0.005,
		ModelGPT41:      0.50,
		ModelGPT4o:      1.25,
	}
	exclusions := map[string]string{
		ModelGPT52Pro: "the official GPT-5.2 Pro model page publishes input and output prices but no cached-input price",
	}

	for model := range TextModelPricing {
		_, covered := expected[model]
		_, excluded := exclusions[model]
		assert.True(t, covered || excluded, "OpenAI pricing model must be expected or explicitly excluded: "+model)
	}
	for model, reason := range exclusions {
		t.Run("excluded/"+model, func(t *testing.T) {
			assert.NotEmpty(t, reason)
			pricing, ok := TextModelPricing[model]
			assert.True(t, ok)
			assert.Equal(t, 0.0, pricing.CacheReadPrice)
		})
	}
	for model, wantPrice := range expected {
		t.Run(model, func(t *testing.T) {
			pricing, ok := TextModelPricing[model]
			assert.True(t, ok)
			assert.Equal(t, wantPrice, pricing.CacheReadPrice)
			cost := pricing.CostOf(&llm.Usage{CacheReadInputTokens: 1_000_000})
			assert.True(t, cost.CacheRead > 0)

			completionsPricing, ok := openaicompletions.TextModelPricing[model]
			assert.True(t, ok, "OpenAI Completions generated view must include "+model)
			assert.Equal(t, wantPrice, completionsPricing.CacheReadPrice)
		})
	}
}

func TestOpenAIPricingUsesDisjointCachedTokens(t *testing.T) {
	decoded, err := decodeAssistantResponse(&responses.Response{
		Usage: responses.ResponseUsage{
			InputTokens: 1_000_000,
			InputTokensDetails: responses.ResponseUsageInputTokensDetails{
				CachedTokens:     700_000,
				CacheWriteTokens: 200_000,
			},
		},
	})
	assert.NoError(t, err)
	pricing := TextModelPricing[ModelGPT56Sol]
	cost := pricing.CostOf(&decoded.Usage)
	assert.Equal(t, 0.5, cost.Input)
	assert.Equal(t, 0.35, cost.CacheRead)
	assert.Equal(t, 1.25, cost.CacheWrite)
	assert.Equal(t, 2.1, cost.Total)
}
