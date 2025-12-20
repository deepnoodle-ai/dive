package dive

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestCalculateTotalTokens(t *testing.T) {
	agent := &StandardAgent{}

	tests := []struct {
		name     string
		usage    *llm.Usage
		expected int
	}{
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
			name: "input and output only",
			usage: &llm.Usage{
				InputTokens:  1000,
				OutputTokens: 500,
			},
			expected: 1500,
		},
		{
			name: "with cache tokens",
			usage: &llm.Usage{
				InputTokens:              1000,
				OutputTokens:             500,
				CacheCreationInputTokens: 200,
				CacheReadInputTokens:     300,
			},
			expected: 2000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.calculateTotalTokens(tt.usage)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldCompact(t *testing.T) {
	tests := []struct {
		name         string
		compaction   *CompactionConfig
		usage        *llm.Usage
		messageCount int
		expected     bool
	}{
		{
			name:       "nil compaction config",
			compaction: nil,
			usage: &llm.Usage{
				InputTokens:  150000,
				OutputTokens: 1000,
			},
			messageCount: 10,
			expected:     false,
		},
		{
			name: "compaction disabled",
			compaction: &CompactionConfig{
				Enabled: false,
			},
			usage: &llm.Usage{
				InputTokens:  150000,
				OutputTokens: 1000,
			},
			messageCount: 10,
			expected:     false,
		},
		{
			name: "below default threshold",
			compaction: &CompactionConfig{
				Enabled: true,
			},
			usage: &llm.Usage{
				InputTokens:  50000,
				OutputTokens: 1000,
			},
			messageCount: 10,
			expected:     false,
		},
		{
			name: "above default threshold",
			compaction: &CompactionConfig{
				Enabled: true,
			},
			usage: &llm.Usage{
				InputTokens:  99000,
				OutputTokens: 2000,
			},
			messageCount: 10,
			expected:     true,
		},
		{
			name: "above custom threshold",
			compaction: &CompactionConfig{
				Enabled:               true,
				ContextTokenThreshold: 50000,
			},
			usage: &llm.Usage{
				InputTokens:  50000,
				OutputTokens: 1000,
			},
			messageCount: 10,
			expected:     true,
		},
		{
			name: "below custom threshold",
			compaction: &CompactionConfig{
				Enabled:               true,
				ContextTokenThreshold: 50000,
			},
			usage: &llm.Usage{
				InputTokens:  40000,
				OutputTokens: 1000,
			},
			messageCount: 10,
			expected:     false,
		},
		{
			name: "too few messages",
			compaction: &CompactionConfig{
				Enabled: true,
			},
			usage: &llm.Usage{
				InputTokens:  150000,
				OutputTokens: 1000,
			},
			messageCount: 1,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &StandardAgent{
				compaction: tt.compaction,
			}
			result := agent.shouldCompact(tt.usage, tt.messageCount)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractSummary(t *testing.T) {
	agent := &StandardAgent{}

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.extractSummary(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterPendingToolUse(t *testing.T) {
	agent := &StandardAgent{}

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
			result := agent.filterPendingToolUse(tt.messages)
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
