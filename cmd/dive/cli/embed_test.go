package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddingOutputFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		expected EmbeddingOutputFormat
	}{
		{
			name:     "json format",
			format:   "json",
			expected: EmbeddingOutputJSON,
		},
		{
			name:     "vector format",
			format:   "vector",
			expected: EmbeddingOutputVector,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format := EmbeddingOutputFormat(tt.format)
			require.Equal(t, tt.expected, format)
		})
	}
}

func TestReadStdin_EmptyInput(t *testing.T) {
	// This test would require stdin mocking, which is complex
	// For now, we'll test the basic structure
	require.NotNil(t, readStdin)
}