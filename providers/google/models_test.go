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
			logger := &recordingWarningLogger{}
			var request Request
			err := provider.applyRequestConfig(&request, &llm.Config{
				Model:       model,
				Temperature: &temperature,
				Logger:      logger,
			})
			assert.NoError(t, err)
			assert.Nil(t, request.Temperature)
			assert.Len(t, logger.warnings, 1)

			generateConfig, err := buildGenAIGenerateConfig(&request)
			assert.NoError(t, err)
			assert.Nil(t, generateConfig.Temperature)
		})
	}
}

func TestShouldOmitTemperatureCoversFutureGeminiModels(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{ModelGemini35FlashLite, true},
		{"gemini-3.5-flash-lite-001", true},
		{ModelGemini36Flash, true},
		{"gemini-3.7-pro", true},
		{"gemini-4-pro", true},
		{"models/gemini-4.1-flash", true},
		{ModelGemini35Flash, false},
		{ModelGemini31ProPreview, false},
		{"not-a-gemini-model", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldOmitTemperature(tt.model))
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

type recordingWarningLogger struct {
	warnings []string
}

func (l *recordingWarningLogger) Debug(string, ...any) {}
func (l *recordingWarningLogger) Info(string, ...any)  {}
func (l *recordingWarningLogger) Warn(message string, _ ...any) {
	l.warnings = append(l.warnings, message)
}
func (l *recordingWarningLogger) Error(string, ...any)   {}
func (l *recordingWarningLogger) With(...any) llm.Logger { return l }
