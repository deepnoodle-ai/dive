package google

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestLatestGeminiFlashModels(t *testing.T) {
	tests := []struct {
		model       string
		wantID      string
		inputPrice  float64
		outputPrice float64
	}{
		{ModelGemini36Flash, "gemini-3.6-flash", 1.50, 7.50},
		{ModelGemini35FlashLite, "gemini-3.5-flash-lite", 0.30, 2.50},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.wantID, tt.model)
			pricing, ok := TextModelPricing[tt.model]
			assert.True(t, ok)
			assert.Equal(t, tt.inputPrice, pricing.InputPrice)
			assert.Equal(t, tt.outputPrice, pricing.OutputPrice)
		})
	}
}

func TestLatestGeminiFlashModelsOmitTemperature(t *testing.T) {
	temperature := 0.8
	provider := New()

	for _, model := range []string{ModelGemini36Flash, ModelGemini35FlashLite} {
		t.Run(model, func(t *testing.T) {
			var request Request
			err := provider.applyRequestConfig(&request, &llm.Config{
				Model:       model,
				Temperature: &temperature,
			})
			assert.NoError(t, err)
			assert.Nil(t, request.Temperature)

			generateConfig, err := buildGenAIGenerateConfig(&request)
			assert.NoError(t, err)
			assert.Nil(t, generateConfig.Temperature)
		})
	}
}

func TestGemini35FlashKeepsTemperature(t *testing.T) {
	temperature := 0.8
	provider := New()
	var request Request
	err := provider.applyRequestConfig(&request, &llm.Config{
		Model:       ModelGemini35Flash,
		Temperature: &temperature,
	})
	assert.NoError(t, err)
	assert.NotNil(t, request.Temperature)
	assert.Equal(t, temperature, *request.Temperature)
}
