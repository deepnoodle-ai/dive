package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSummarizationPrompts(t *testing.T) {
	tests := []struct {
		name     string
		length   string
		expected string
	}{
		{
			name:     "short summary",
			length:   "short",
			expected: "You are a concise summarization assistant. Create a brief, focused summary that captures only the most essential points. Aim for 1-3 sentences that distill the core message or findings. Be direct and eliminate any redundancy.",
		},
		{
			name:     "medium summary",
			length:   "medium",
			expected: "You are a balanced summarization assistant. Create a well-structured summary that covers the main points while maintaining important context and details. Aim for a paragraph or two that provides a comprehensive overview without being verbose.",
		},
		{
			name:     "long summary",
			length:   "long",
			expected: "You are a detailed summarization assistant. Create a thorough summary that preserves important details, context, and nuances while organizing the information clearly. Include key supporting points and maintain the structure of the original content.",
		},
		{
			name:     "default/unknown length",
			length:   "unknown",
			expected: "You are a summarization assistant. Create a clear, well-organized summary of the provided text that captures the main points and key details.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := summarizationPrompts(tt.length)
			require.Equal(t, tt.expected, result)
		})
	}
}
