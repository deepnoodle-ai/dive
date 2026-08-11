package google

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

func TestGoogleCacheReadPricingCoverage(t *testing.T) {
	expected := map[string]float64{
		ModelGemini36Flash:                 0.15,
		ModelGemini35Flash:                 0.15,
		ModelGemini35FlashLite:             0.03,
		ModelGemini31ProPreview:            0.20,
		ModelGemini31ProPreviewCustomTools: 0.20,
		ModelGemini31FlashLite:             0.025,
		ModelGemini3FlashPreview:           0.05,
		ModelGemini25Flash:                 0.03,
		ModelGemini25FlashLite:             0.01,
		ModelGemini25Pro:                   0.125,
		ModelGemini20Flash:                 0.025,
	}
	exclusions := map[string]string{
		ModelGemini31FlashLivePreview: "the official pricing row publishes no context-caching price",
		ModelGemini31FlashLitePreview: "the current official pricing page publishes no row for this retired preview id",
		ModelGemini31FlashImagePrev:   "the official image-model pricing row publishes no context-caching price",
		ModelGemini15Pro:              "the current official pricing page no longer publishes a Gemini 1.5 Pro row",
		ModelGemini15Flash:            "the current official pricing page no longer publishes a Gemini 1.5 Flash row",
	}

	for model := range TextModelPricing {
		_, covered := expected[model]
		_, excluded := exclusions[model]
		assert.True(t, covered || excluded, "Google pricing model must be expected or explicitly excluded: "+model)
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
		})
	}
}

func TestGoogleProLongContextPricingBoundary(t *testing.T) {
	tests := []struct {
		model          string
		standardInput  float64
		standardCache  float64
		standardOutput float64
		longInput      float64
		longCache      float64
		longOutput     float64
	}{
		{ModelGemini31ProPreview, 2.00, 0.20, 12.00, 4.00, 0.40, 18.00},
		{ModelGemini31ProPreviewCustomTools, 2.00, 0.20, 12.00, 4.00, 0.40, 18.00},
		{ModelGemini25Pro, 1.25, 0.125, 10.00, 2.50, 0.25, 15.00},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			pricing := TextModelPricing[tt.model]
			assert.Equal(t, 200_001, pricing.LongContextThreshold)
			assert.Equal(t, tt.longInput, pricing.LongContextInputPrice)
			assert.Equal(t, tt.longCache, pricing.LongContextCacheReadPrice)
			assert.Equal(t, tt.longOutput, pricing.LongContextOutputPrice)

			for _, boundary := range []struct {
				totalInput int
				wantInput  float64
				wantCache  float64
				wantOutput float64
			}{
				{200_000, tt.standardInput, tt.standardCache, tt.standardOutput},
				{200_001, tt.longInput, tt.longCache, tt.longOutput},
			} {
				usage := llm.Usage{
					InputTokens:          boundary.totalInput - 100_000,
					CacheReadInputTokens: 100_000,
					OutputTokens:         1_000_000,
				}
				assert.Equal(t, boundary.totalInput, usage.TotalInputTokens())
				cost := pricing.CostOf(&usage)
				assert.Equal(t, float64(usage.InputTokens)*boundary.wantInput/1_000_000, cost.Input)
				assert.InDelta(t, boundary.wantCache/10, cost.CacheRead, 1e-12)
				assert.Equal(t, boundary.wantOutput, cost.Output)
			}
		})
	}
}

func TestGoogleModalityPricing(t *testing.T) {
	pricing := TextModelPricing[ModelGemini25Flash]
	usage := &llm.Usage{
		InputTokens:          2_000_000,
		OutputTokens:         1_000_000,
		CacheReadInputTokens: 2_000_000,
		ModalityTokens: map[string]llm.ModalityTokenUsage{
			"audio": {InputTokens: 1_000_000, CacheReadInputTokens: 1_000_000},
		},
	}

	cost := pricing.CostOf(usage)
	assert.InDelta(t, 1.30, cost.Input, 1e-12)
	assert.InDelta(t, 0.13, cost.CacheRead, 1e-12)
	assert.InDelta(t, 2.50, cost.Output, 1e-12)
	assert.InDelta(t, 3.93, cost.Total, 1e-12)
}

func TestPopulateGoogleCostFailsClosed(t *testing.T) {
	t.Run("standard list price", func(t *testing.T) {
		usage := &llm.Usage{InputTokens: 100_000, OutputTokens: 1_000_000, CacheReadInputTokens: 100_000}
		populateGoogleCost(ModelGemini25Pro, nil, usage, googlePricingContext{})
		assert.NotNil(t, usage.Cost)
		assert.Equal(t, 0.125, usage.Cost.Input)
		assert.Equal(t, 0.0125, usage.Cost.CacheRead)
		assert.Equal(t, 10.00, usage.Cost.Output)
		assert.Equal(t, "", usage.ServiceTier)
	})

	t.Run("Vertex regional multiplier", func(t *testing.T) {
		usage := &llm.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadInputTokens: 1_000_000}
		populateGoogleCost(ModelGemini35Flash, nil, usage, googlePricingContext{
			vertexAI: true, location: "us-central1",
		})
		assert.NotNil(t, usage.Cost)
		assert.InDelta(t, 1.65, usage.Cost.Input, 1e-12)
		assert.InDelta(t, 0.165, usage.Cost.CacheRead, 1e-12)
		assert.InDelta(t, 9.90, usage.Cost.Output, 1e-12)
	})

	t.Run("Vertex on-demand tier", func(t *testing.T) {
		usage := &llm.Usage{InputTokens: 1}
		populateGoogleCost(ModelGemini35Flash, &genai.GenerateContentResponseUsageMetadata{
			TrafficType: genai.TrafficTypeOnDemand,
		}, usage, googlePricingContext{vertexAI: true, location: "global"})
		assert.NotNil(t, usage.Cost)
		assert.Equal(t, "standard", usage.ServiceTier)
	})

	tests := []struct {
		name     string
		model    string
		metadata *genai.GenerateContentResponseUsageMetadata
		usage    *llm.Usage
		context  googlePricingContext
		wantTier string
	}{
		{
			name: "uncataloged model", model: "gemini-future-model",
			usage: &llm.Usage{InputTokens: 1},
		},
		{
			name: "missing modality detail", model: ModelGemini25Flash,
			usage: &llm.Usage{InputTokens: 1, InputModalityTokenDetailsIncomplete: true},
		},
		{
			name: "Vertex priority traffic", model: ModelGemini35Flash,
			metadata: &genai.GenerateContentResponseUsageMetadata{
				TrafficType: genai.TrafficTypeOnDemandPriority,
			},
			usage:    &llm.Usage{InputTokens: 1},
			context:  googlePricingContext{vertexAI: true, location: "global"},
			wantTier: "priority",
		},
		{
			name: "unsupported request tier", model: ModelGemini35Flash,
			usage:   &llm.Usage{InputTokens: 1},
			context: googlePricingContext{serviceTier: genai.ServiceTierPriority},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			populateGoogleCost(tt.model, tt.metadata, tt.usage, tt.context)
			assert.Nil(t, tt.usage.Cost)
			assert.True(t, tt.usage.CostEstimateUnavailable)
			if tt.wantTier != "" {
				assert.Equal(t, tt.wantTier, tt.usage.ServiceTier)
			}
		})
	}
}
