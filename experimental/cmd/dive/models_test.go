package main

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestLatestGeminiFlashModels(t *testing.T) {
	tests := []struct {
		model string
		label string
	}{
		{"gemini-3.8-flash", "Gemini 3.8 Flash"},
		{"gemini-3.7-flash", "Gemini 3.7 Flash"},
		{"gemini-3.5-flash-lite", "Gemini 3.5 Flash-Lite"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			info := lookupModel(tt.model)
			assert.NotNil(t, info)
			assert.Equal(t, tt.label, info.Label)
			assert.Equal(t, 1_000_000, info.ContextWindow)
		})
	}
}

func TestGoogleProviderCatalogIncludesLatestFlashModels(t *testing.T) {
	want := map[string]bool{
		"gemini-3.8-flash":      false,
		"gemini-3.5-flash-lite": false,
	}
	for _, provider := range providerCatalog {
		if provider.Name != "Google" {
			continue
		}
		for _, model := range provider.Models {
			if _, ok := want[model.ModelID]; ok {
				want[model.ModelID] = true
			}
		}
	}
	for model, found := range want {
		assert.True(t, found, "Google model picker is missing %s", model)
	}
}

func TestGrok45ContextWindow(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"grok-4.5", 500_000},
		{"grok-build-0.1", 256_000},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, contextWindowForModel(tt.model))
		})
	}
}

func TestOllamaContextWindow(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gpt-oss:20b", 131_072},
		{"gpt-oss", 131_072},
		{"qwen3.6:27b", 262_144},
		{"gemma4:12b", 262_144},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, contextWindowForModel(tt.model))
		})
	}
}

// The CLI reports no context window for a model the catalogs do not list, so
// the UI hides the context bar instead of showing a guessed size.
func TestUnknownModelHasNoContextWindow(t *testing.T) {
	assert.Equal(t, 0, contextWindowForModel("not-a-real-model-9000"))
}

func TestGPT56ContextWindow(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"gpt-5.6", 1_050_000},
		{"gpt-5.6-sol", 1_050_000},
		{"gpt-5.6-terra", 1_050_000},
		{"gpt-5.6-luna", 1_050_000},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, contextWindowForModel(tt.model))
		})
	}
}
