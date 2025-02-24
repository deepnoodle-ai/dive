package dive

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseResponseText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected StructuredResponse
	}{
		{
			name:  "only thinking tag",
			input: `<think>Ok, let me see</think>`,
			expected: StructuredResponse{
				Thinking: "Ok, let me see",
				Text:     "",
				Status:   "",
			},
		},
		{
			name:  "thinking and response text",
			input: `<think>Processing request</think>Here is the answer`,
			expected: StructuredResponse{
				Thinking: "Processing request",
				Text:     "Here is the answer",
				Status:   "",
			},
		},
		{
			name:  "all tags with content",
			input: `<think>Analyzing</think><status>Working</status>Processing complete`,
			expected: StructuredResponse{
				Thinking: "Analyzing",
				Text:     "Processing complete",
				Status:   "Working",
			},
		},
		{
			name:  "empty input",
			input: "",
			expected: StructuredResponse{
				Thinking: "",
				Text:     "",
				Status:   "",
			},
		},
		{
			name:  "only status tag",
			input: `<status>In Progress</status>`,
			expected: StructuredResponse{
				Thinking: "",
				Text:     "",
				Status:   "In Progress",
			},
		},
		{
			name: "multiple lines with whitespace",
			input: `<think>Thinking deeply</think>
					<status>Processing</status>Here is the
multiline
response`,
			expected: StructuredResponse{
				Thinking: "Thinking deeply",
				Text:     "Here is the\nmultiline\nresponse",
				Status:   "Processing",
			},
		},
		{
			name:  "bad status tag",
			input: `<status>Hmm`,
			expected: StructuredResponse{
				Thinking: "",
				Text:     "<status>Hmm",
				Status:   "",
			},
		},
		{
			name:  "bad think tag",
			input: `<think>Hmm`,
			expected: StructuredResponse{
				Thinking: "",
				Text:     "<think>Hmm",
				Status:   "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := ParseStructuredResponse(tt.input)
			require.Equal(t, tt.expected, response)
		})
	}
}
