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
