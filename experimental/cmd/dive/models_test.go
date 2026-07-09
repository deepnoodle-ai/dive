package main

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

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
