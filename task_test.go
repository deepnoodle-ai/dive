package dive

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseStructuredResponseText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected StructuredResponse
	}{
		{
			name:  "only thinking tag",
			input: `<think>Ok, let me see</think>`,
			expected: StructuredResponse{
				thinking:          "Ok, let me see",
				content:           "",
				statusDescription: "",
				status:            TaskStatusUnknown,
				rawText:           `<think>Ok, let me see</think>`,
			},
		},
		{
			name:  "thinking and response text",
			input: `<think>Processing request</think>Here is the answer`,
			expected: StructuredResponse{
				thinking:          "Processing request",
				content:           "Here is the answer",
				statusDescription: "",
				status:            TaskStatusUnknown,
				rawText:           `<think>Processing request</think>Here is the answer`,
			},
		},
		{
			name:  "all tags with content",
			input: `<think>Analyzing</think><status>Working</status>Processing complete`,
			expected: StructuredResponse{
				thinking:          "Analyzing",
				content:           "Processing complete",
				statusDescription: "Working",
				status:            TaskStatusUnknown,
				rawText:           `<think>Analyzing</think><status>Working</status>Processing complete`,
			},
		},
		{
			name:  "empty input",
			input: "",
			expected: StructuredResponse{
				thinking:          "",
				content:           "",
				statusDescription: "",
				status:            TaskStatusUnknown,
				rawText:           "",
			},
		},
		{
			name:  "only status tag",
			input: `<status>In Progress</status>`,
			expected: StructuredResponse{
				thinking:          "",
				content:           "",
				statusDescription: "In Progress",
				status:            TaskStatusUnknown,
				rawText:           `<status>In Progress</status>`,
			},
		},
		{
			name:  "bad status tag",
			input: `<status>Hmm`,
			expected: StructuredResponse{
				thinking:          "",
				content:           "<status>Hmm",
				statusDescription: "",
				status:            TaskStatusUnknown,
				rawText:           `<status>Hmm`,
			},
		},
		{
			name:  "bad think tag",
			input: `<think>Hmm`,
			expected: StructuredResponse{
				thinking:          "",
				content:           "<think>Hmm",
				statusDescription: "",
				status:            TaskStatusUnknown,
				rawText:           `<think>Hmm`,
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
