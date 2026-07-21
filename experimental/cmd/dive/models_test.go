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
		{"gemini-3.6-flash", "Gemini 3.6 Flash"},
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
		"gemini-3.6-flash":      false,
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
		{"grok-4.5-latest", 500_000},
		{"grok-build-latest", 500_000},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, contextWindowForModel(tt.model))
		})
	}
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
