package dive

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestCalculateContextTokens(t *testing.T) {
	tests := []struct {
		name     string
		usage    *llm.Usage
		expected int
	}{
		{
			name:     "nil usage",
			usage:    nil,
			expected: 0,
		},
		{
			name: "all zeros",
			usage: &llm.Usage{
				InputTokens:              0,
				OutputTokens:             0,
				CacheCreationInputTokens: 0,
				CacheReadInputTokens:     0,
			},
			expected: 0,
		},
		{
			name: "input only",
			usage: &llm.Usage{
				InputTokens:  1000,
				OutputTokens: 500, // Not included in context
			},
			expected: 1000, // Only input tokens
		},
		{
			name: "with cache read tokens",
			usage: &llm.Usage{
				InputTokens:              1000,
				OutputTokens:             500,  // Not included
				CacheCreationInputTokens: 200,  // Not included (subset of input)
				CacheReadInputTokens:     300,
			},
			expected: 1300, // InputTokens + CacheReadInputTokens
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateContextTokens(tt.usage)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldCompact(t *testing.T) {
	tests := []struct {
		name         string
		usage        *llm.Usage
		messageCount int
		threshold    int
		expected     bool
	}{
		{
			name: "below default threshold",
			usage: &llm.Usage{
				InputTokens:  50000,
				OutputTokens: 1000,
			},
			messageCount: 10,
			threshold:    0, // Use default
			expected:     false,
		},
		{
			name: "above default threshold",
			usage: &llm.Usage{
				InputTokens:  100001, // Context tokens exceed threshold
				OutputTokens: 2000,   // Output tokens not counted for threshold
			},
			messageCount: 10,
			threshold:    0, // Use default
			expected:     true,
		},
		{
			name: "above custom threshold",
			usage: &llm.Usage{
				InputTokens:  50000,
				OutputTokens: 1000,
			},
			messageCount: 10,
			threshold:    50000,
			expected:     true,
		},
		{
			name: "below custom threshold",
			usage: &llm.Usage{
				InputTokens:  40000,
				OutputTokens: 1000,
			},
			messageCount: 10,
			threshold:    50000,
			expected:     false,
		},
		{
			name: "too few messages",
			usage: &llm.Usage{
				InputTokens:  150000,
				OutputTokens: 1000,
			},
			messageCount: 1,
			threshold:    0,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldCompact(tt.usage, tt.messageCount, tt.threshold)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractSummary(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "valid summary",
			text:     "Some preamble <summary>This is the summary content</summary> some trailing text",
			expected: "This is the summary content",
		},
		{
			name:     "summary with whitespace",
			text:     "<summary>  \n  Summary with whitespace  \n  </summary>",
			expected: "Summary with whitespace",
		},
		{
			name:     "no summary tags",
			text:     "Just some text without summary tags",
			expected: "",
		},
		{
			name:     "missing end tag",
			text:     "<summary>This has no end tag",
			expected: "",
		},
		{
			name:     "missing start tag",
			text:     "This has no start tag</summary>",
			expected: "",
		},
		{
			name:     "empty summary",
			text:     "<summary></summary>",
			expected: "",
		},
		{
			name:     "multiline summary",
			text:     "<summary>\n# Task Overview\nDoing something\n\n# Next Steps\n1. Step one\n</summary>",
			expected: "# Task Overview\nDoing something\n\n# Next Steps\n1. Step one",
		},
		{
			name:     "uppercase tags",
			text:     "<SUMMARY>Uppercase content</SUMMARY>",
			expected: "Uppercase content",
		},
		{
			name:     "mixed case tags",
			text:     "<Summary>Mixed Case</Summary>",
			expected: "Mixed Case",
		},
		{
			name:     "preserves content case",
			text:     "<SUMMARY>Content With MIXED Case</SUMMARY>",
			expected: "Content With MIXED Case",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSummary(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterPendingToolUse(t *testing.T) {
	tests := []struct {
		name     string
		messages []*llm.Message
		expected int // expected number of messages after filtering
	}{
		{
			name:     "empty messages",
			messages: []*llm.Message{},
			expected: 0,
		},
		{
			name: "no tool use in last message",
			messages: []*llm.Message{
				{Role: llm.User, Content: []llm.Content{&llm.TextContent{Text: "Hello"}}},
				{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "Hi there"}}},
			},
			expected: 2,
		},
		{
			name: "last message is user",
			messages: []*llm.Message{
				{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "Hi"}}},
				{Role: llm.User, Content: []llm.Content{&llm.TextContent{Text: "Hello"}}},
			},
			expected: 2,
		},
		{
			name: "only tool use in last message - remove entire message",
			messages: []*llm.Message{
				{Role: llm.User, Content: []llm.Content{&llm.TextContent{Text: "Hello"}}},
				{
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{ID: "tool1", Name: "test"},
					},
				},
			},
			expected: 1,
		},
		{
			name: "mixed content - filter tool use",
			messages: []*llm.Message{
				{Role: llm.User, Content: []llm.Content{&llm.TextContent{Text: "Hello"}}},
				{
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.TextContent{Text: "Let me help"},
						&llm.ToolUseContent{ID: "tool1", Name: "test"},
					},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterPendingToolUse(tt.messages)
			assert.Len(t, result, tt.expected)

			// Verify that the last message (if exists) has no tool use
			if len(result) > 0 {
				lastMsg := result[len(result)-1]
				if lastMsg.Role == llm.Assistant {
					for _, content := range lastMsg.Content {
						_, isToolUse := content.(*llm.ToolUseContent)
						assert.False(t, isToolUse, "filtered message should not contain tool use")
					}
				}
			}
		})
	}
}

func TestCompactionConfigDefaults(t *testing.T) {
	assert.Equal(t, 100000, DefaultContextTokenThreshold)
	assert.NotEmpty(t, DefaultCompactionSummaryPrompt)
	assert.Contains(t, DefaultCompactionSummaryPrompt, "<summary>")
	assert.Contains(t, DefaultCompactionSummaryPrompt, "</summary>")
}

func TestCompactionEventStructure(t *testing.T) {
	// Test that CompactionEvent can be properly constructed
	event := &CompactionEvent{
		TokensBefore:      150000,
		TokensAfter:       5000,
		Summary:           "Test summary",
		MessagesCompacted: 50,
	}

	assert.Equal(t, 150000, event.TokensBefore)
	assert.Equal(t, 5000, event.TokensAfter)
	assert.Equal(t, "Test summary", event.Summary)
	assert.Equal(t, 50, event.MessagesCompacted)
}
