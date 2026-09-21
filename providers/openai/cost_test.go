package openai

import (
	"math"
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
	assert.Equal(t, p.LongContextCacheWritePrice, p.CacheWritePrice*2)
	assert.Equal(t, p.LongContextOutputPrice, p.OutputPrice*1.5)

	// Output is billed at the tier the *input* size selects, so a short prompt
	// with a long completion stays on standard rates.
	short := p.CostOf(&llm.Usage{InputTokens: 200_000, OutputTokens: 100_000})
	assert.Equal(t, short.Input, 2.0)
	assert.Equal(t, short.Output, 5.0)

	long := p.CostOf(&llm.Usage{InputTokens: 300_000, OutputTokens: 100_000})
	assert.Equal(t, long.Input, 6.0)
	assert.Equal(t, long.Output, 7.5)

	// Cache writes move to the long-context rate with everything else. The
	// full input size selects the tier -- cache-creation tokens included -- so
	// the same write costs half as much on a request that stays under it.
	longWrite := p.CostOf(&llm.Usage{InputTokens: 300_000, CacheCreationInputTokens: 100_000})
	assert.Equal(t, longWrite.CacheWrite, 2.50)

	shortWrite := p.CostOf(&llm.Usage{InputTokens: 100_000, CacheCreationInputTokens: 100_000})
	assert.Equal(t, shortWrite.CacheWrite, 1.25)
}

func TestOpenAICacheReadPricingCoverage(t *testing.T) {
	expected := map[string]float64{
		ModelGPT6Astra:          1.00,
		ModelGPT56:              0.40,
		ModelGPT56Sol:           0.40,
		ModelGPT56Terra:         0.20,
		ModelGPT56Luna:          0.02,
		ModelGPT56Cyber:         1.25,
		ModelDaybreakBlueLatest: 0.40,
		ModelDaybreakRedLatest:  1.25,
		ModelGPT55:              0.50,
		ModelGPT54:              0.25,
		ModelGPT54Mini:          0.075,
		ModelGPT54Nano:          0.02,
		ModelGPT52:              0.175,
		ModelGPT51:              0.125,
		ModelGPT5:               0.125,
		ModelGPT5Mini:           0.025,
		ModelGPT5Nano:           0.005,
		ModelGPT41:              0.50,
		ModelGPT41Mini:          0.10,
		ModelGPT41Nano:          0.025,
		ModelGPT4o:              1.25,
	}
	exclusions := map[string]string{
		ModelGPT52Pro: "the official GPT-5.2 Pro model page publishes input and output prices but no cached-input price",
		ModelGPT54Pro: "OpenAI's pricing table leaves the cached-input column empty for the pro models",
		ModelGPT55Pro: "OpenAI's pricing table leaves the cached-input column empty for the pro models",
	}
	// Models the Chat Completions adapter deliberately does not carry, so the
	// generated view is expected to omit them.
	responsesOnly := map[string]string{
		ModelGPT6Astra:          "Chat Completions does not support function calling with GPT-6 Astra, so the model is Responses-only",
		ModelGPT56Cyber:         "the Daybreak cybersecurity models are documented on v1/responses only",
		ModelDaybreakBlueLatest: "the Daybreak cybersecurity models are documented on v1/responses only",
		ModelDaybreakRedLatest:  "the Daybreak cybersecurity models are documented on v1/responses only",
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
			if reason, only := responsesOnly[model]; only {
				assert.False(t, ok, "OpenAI Completions generated view must omit "+model+": "+reason)
				return
			}
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
	// InputTokens is the full 1M, so cached and cache-write tokens must not be
	// billed twice: only the 100K that were neither is charged at the input
	// rate. 1M also puts the request past the 272K long-context threshold, so
	// every rate here is the long-context one.
	assert.Equal(t, 0.8, cost.Input)
	assert.Equal(t, 0.56, cost.CacheRead)
	assert.Equal(t, 2.0, cost.CacheWrite)
	assert.Equal(t, cost.Input+cost.CacheRead+cost.CacheWrite, cost.Total)
}

// assertClose compares prices that only differ by float representation: 1.20 *
// 1.5 is not exactly 1.80 in binary floating point.
func assertClose(t *testing.T, got, want float64) {
	t.Helper()
	assert.True(t, math.Abs(got-want) < 1e-9, "want %v, got %v", want, got)
}

// The GPT-5.6 family carries the same long-context rule as GPT-6 Astra: above
// 272K input tokens, input and both cache rates double and output is billed at
// 1.5x. Sol is the default model, so pricing it on the standard tier for a
// long-input request undercharged by half.
func TestGPT56LongContextPricing(t *testing.T) {
	for _, model := range []string{ModelGPT56Sol, ModelGPT56Terra, ModelGPT56Luna} {
		t.Run(model, func(t *testing.T) {
			p, ok := TextModelPricing[model]
			assert.True(t, ok, "pricing should exist for "+model)

			assert.Equal(t, p.LongContextThreshold, 272_001)
			assert.Equal(t, p.LongContextInputPrice, p.InputPrice*2)
			assert.Equal(t, p.LongContextCacheReadPrice, p.CacheReadPrice*2)
			assert.Equal(t, p.LongContextCacheWritePrice, p.CacheWritePrice*2)
			assertClose(t, p.LongContextOutputPrice, p.OutputPrice*1.5)

			short := p.CostOf(&llm.Usage{InputTokens: 272_000, OutputTokens: 1_000_000})
			long := p.CostOf(&llm.Usage{InputTokens: 272_001, OutputTokens: 1_000_000})
			assert.True(t, long.Total > short.Total,
				"crossing the threshold must raise the bill for "+model)
		})
	}
}

// GPT-Image-2.5 is billed per token rather than per image, so it carries token
// rates in the image table rather than a per-image constant. The two modalities
// in one prompt differ: text input at $5/1M, image input at $8/1M, and cached
// image tokens at $2/1M against $1.25 for text.
func TestGPTImage25ModalityPricing(t *testing.T) {
	for _, model := range []string{ModelGPTImage25Sunburst, ModelGPTImage25Flare} {
		t.Run(model, func(t *testing.T) {
			pricing, ok := ImageModelPricing[model]
			assert.True(t, ok, "pricing should exist for "+model)
			assert.Equal(t, 0.0, pricing.Price, "OpenAI bills this model per token")

			cost, ok := pricing.CostOf(&llm.Usage{
				InputTokens:          2_000_000,
				CacheReadInputTokens: 1_000_000,
				OutputTokens:         1_000_000,
				ModalityTokens: map[string]llm.ModalityTokenUsage{
					"text":  {InputTokens: 1_000_000},
					"image": {InputTokens: 1_000_000, CacheReadInputTokens: 1_000_000},
				},
			})
			assert.True(t, ok, "token-billed model should price from usage")
			// 1M text at $5 plus 1M image at $8.
			assert.Equal(t, 13.0, cost.Input)
			assert.Equal(t, 2.0, cost.CacheRead)
			// The model emits image tokens only, so the base output rate is it.
			assert.Equal(t, 30.0, cost.Output)
		})
	}
}

// DALL-E really is billed per image, so the per-image side of the table has to
// keep working while the token side is added beside it.
func TestDallEPerImagePricing(t *testing.T) {
	pricing, ok := ImageModelPricing["dall-e-3"]
	assert.True(t, ok)
	assert.Nil(t, pricing.TokenPricing, "DALL-E 3 has a genuine per-image list price")

	cost, ok := pricing.CostOfImages(4)
	assert.True(t, ok)
	assert.Equal(t, 0.16, cost.Total)

	_, ok = pricing.CostOf(&llm.Usage{OutputTokens: 1_000_000})
	assert.False(t, ok, "per-image model cannot be priced from token usage")
}

// The image table used to be unreachable: nothing registered it, so
// llm.PopulateCost could never price an image request. Token-billed rows now
// resolve through the same registry as text models.
func TestTokenBilledImagePricingIsRegistered(t *testing.T) {
	for _, model := range []string{ModelGPTImage25Sunburst, ModelGPTImage25Flare} {
		p, ok := providers.PricingFor(model, false)
		assert.True(t, ok, "image pricing should be registered for "+model)
		assert.Equal(t, 5.00, p.InputPrice)
		assert.Equal(t, 8.00, p.InputPriceByModality["image"])
	}
	// The dated snapshot resolves through the same stable-id fallback the text
	// models use.
	p, ok := providers.PricingFor(ModelGPTImage25Sunburst20260908, false)
	assert.True(t, ok, "dated snapshot should resolve to the stable id's pricing")
	assert.Equal(t, 30.00, p.OutputPrice)

	// DALL-E has no token rates, so it stays out of the token registry.
	_, ok = providers.PricingFor("dall-e-3", false)
	assert.False(t, ok, "per-image models have no per-token cost to resolve")
}
