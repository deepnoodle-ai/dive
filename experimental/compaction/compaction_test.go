package compaction

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestCompactMessagesClampsTinyInputBudget guards the budget clamp: a tiny
// WithMaxInputTokens drives the computed inputBudget negative, which without the
// clamp would make reduceToSummaryBudget a no-op and hand the summarizer the
// full, oversized transcript. The clamp must keep reduction engaged.
func TestCompactMessagesClampsTinyInputBudget(t *testing.T) {
	msgs := []*llm.Message{
		llm.NewUserTextMessage(strings.Repeat("x", 40_000)),
		llm.NewAssistantTextMessage(strings.Repeat("y", 40_000)),
		llm.NewUserTextMessage(strings.Repeat("z", 40_000)),
	}
	before := totalTokens(msgs)

	stub := &stubLLM{}
	_, _, err := CompactMessages(context.Background(), stub, msgs, "", "", 0, WithMaxInputTokens(100))
	assert.NoError(t, err)
	assert.True(t, stub.sawTokens < before,
		"tiny budget must still reduce the summarizer transcript (saw ~%d tokens, original ~%d)", stub.sawTokens, before)
}

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
				OutputTokens:             500, // Not included
				CacheCreationInputTokens: 200, // Not included (subset of input)
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

func totalTokens(msgs []*llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += estimateTokens(m)
	}
	return total
}

func TestEstimateTokensSizesMediaByCostNotBytes(t *testing.T) {
	// A 1.4 MB screenshot, the size a drag-and-drop attachment routinely is.
	screenshot := &llm.Message{Role: llm.User, Content: []llm.Content{
		&llm.TextContent{Text: "what does this image say?"},
		&llm.ImageContent{Source: &llm.ContentSource{
			Type:      llm.ContentSourceTypeBase64,
			MediaType: "image/png",
			Data:      strings.Repeat("A", 1_900_000),
		}},
	}}

	got := estimateTokens(screenshot)

	// Sizing the base64 at 4 bytes per token would call this ~475k — enough to
	// blow past any compaction threshold and summarize the conversation away
	// the moment an image is attached.
	assert.True(t, got < 2_500, "one image should not read as a full context window, got %d", got)
	assert.True(t, got >= base64ImageTokens, "the image still has to be counted, got %d", got)

	// A URL source carries no payload, so it stays on the serialized-size path.
	linked := &llm.Message{Role: llm.User, Content: []llm.Content{
		&llm.ImageContent{Source: &llm.ContentSource{
			Type: llm.ContentSourceTypeURL,
			URL:  "https://example.com/a.png",
		}},
	}}
	assert.True(t, estimateTokens(linked) < base64ImageTokens)

	// Text and tool payloads keep their serialized-size estimate.
	text := llm.NewUserTextMessage(strings.Repeat("x", 4_000))
	assert.True(t, estimateTokens(text) >= 1_000, "text is still ~4 bytes per token")
}

func TestEstimateTokensSizesDocumentsByDecodedLength(t *testing.T) {
	doc := func(base64Len int) *llm.Message {
		return &llm.Message{Role: llm.User, Content: []llm.Content{
			&llm.DocumentContent{Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeBase64,
				MediaType: "application/pdf",
				Data:      strings.Repeat("A", base64Len),
			}},
		}}
	}

	// A big PDF costs more than a small one, but nothing like its byte count.
	small := estimateTokens(doc(20_000))
	large := estimateTokens(doc(2_000_000))
	assert.True(t, large > small, "a longer document should cost more")
	assert.True(t, small >= base64ImageTokens, "a short document still costs at least a page")
	assert.True(t, large < 2_000_000/4, "a document must not be sized as if it were text")
}

func TestReduceToSummaryBudget(t *testing.T) {
	bigText := strings.Repeat("x", 200_000) // ~50k tokens

	t.Run("no-op when under budget", func(t *testing.T) {
		msgs := []*llm.Message{llm.NewUserTextMessage("hello")}
		out := reduceToSummaryBudget(msgs, 1_000_000)
		assert.Equal(t, 1, len(out))
		assert.Equal(t, "hello", out[0].Text())
	})

	t.Run("clips the largest, preserves small, leaves originals untouched", func(t *testing.T) {
		small := llm.NewUserTextMessage("hello")
		big := llm.NewAssistantTextMessage(bigText)
		msgs := []*llm.Message{small, big}
		budget := 5_000

		out := reduceToSummaryBudget(msgs, budget)

		assert.True(t, totalTokens(out) <= budget)
		assert.Equal(t, "hello", out[0].Text())           // small preserved verbatim
		assert.True(t, len(out[1].Text()) > 0)            // big still has content
		assert.True(t, len(out[1].Text()) < len(bigText)) // ...but truncated
		assert.True(t, strings.Contains(out[1].Text(), "truncated"))
		assert.Equal(t, len(bigText), len(big.Text())) // original not mutated
	})

	t.Run("preserves tool_use/tool_result pairing (never drops messages)", func(t *testing.T) {
		toolUse := &llm.Message{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "t1", Name: "read", Input: json.RawMessage(`{"path":"big.txt"}`)},
		}}
		bigResult := &llm.Message{Role: llm.User, Content: []llm.Content{
			&llm.ToolResultContent{ToolUseID: "t1", Content: strings.Repeat("y", 300_000)},
		}}
		msgs := []*llm.Message{toolUse, bigResult}
		budget := 4_000

		out := reduceToSummaryBudget(msgs, budget)

		assert.Equal(t, 2, len(out)) // nothing dropped → pairing intact
		_, isToolUse := out[0].Content[0].(*llm.ToolUseContent)
		assert.True(t, isToolUse)
		tr, isToolResult := out[1].Content[0].(*llm.ToolResultContent)
		assert.True(t, isToolResult)
		truncated, _ := tr.Content.(string)
		assert.True(t, len(truncated) < 300_000)
		assert.True(t, totalTokens(out) <= budget)
	})
}

func TestReduceToSummaryBudgetCullsUntruncatableContent(t *testing.T) {
	t.Run("culls an image that does not fit the budget", func(t *testing.T) {
		image := &llm.Message{Role: llm.User, Content: []llm.Content{
			&llm.ImageContent{Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeBase64,
				MediaType: "image/png",
				Data:      strings.Repeat("A", 400_000),
			}},
		}}
		// An image costs base64ImageTokens however long its encoding is, so the
		// budget has to be under that for culling to be the right call.
		out := reduceToSummaryBudget([]*llm.Message{image}, base64ImageTokens/2)

		assert.Equal(t, 1, len(out)) // not dropped
		txt, isText := out[0].Content[0].(*llm.TextContent)
		assert.True(t, isText) // image culled to a text placeholder
		assert.True(t, strings.Contains(txt.Text, "image content omitted"))
		assert.True(t, totalTokens(out) <= base64ImageTokens/2)
	})

	t.Run("keeps an image that fits the budget", func(t *testing.T) {
		// The same megabytes of base64, but a budget that accommodates what the
		// image actually costs: it must survive intact. Sizing media by encoded
		// bytes instead would read as ~100k tokens and cull it every time.
		image := &llm.Message{Role: llm.User, Content: []llm.Content{
			&llm.ImageContent{Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeBase64,
				MediaType: "image/png",
				Data:      strings.Repeat("A", 400_000),
			}},
		}}
		out := reduceToSummaryBudget([]*llm.Message{image}, 10_000)

		assert.Equal(t, 1, len(out))
		_, isImage := out[0].Content[0].(*llm.ImageContent)
		assert.True(t, isImage, "an image within budget should not be culled")
	})

	t.Run("culls an oversized tool_use input but keeps the block paired", func(t *testing.T) {
		toolUse := &llm.Message{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "t1", Name: "write", Input: json.RawMessage(`{"data":"` + strings.Repeat("z", 300_000) + `"}`)},
		}}
		result := &llm.Message{Role: llm.User, Content: []llm.Content{
			&llm.ToolResultContent{ToolUseID: "t1", Content: "ok"},
		}}
		out := reduceToSummaryBudget([]*llm.Message{toolUse, result}, 2_000)

		assert.Equal(t, 2, len(out))
		tu, isToolUse := out[0].Content[0].(*llm.ToolUseContent) // block kept → pairing intact
		assert.True(t, isToolUse)
		assert.Equal(t, "t1", tu.ID)
		assert.True(t, len(tu.Input) < 1000) // input culled
		assert.True(t, totalTokens(out) <= 2_000)
	})
}

func TestTruncateText(t *testing.T) {
	assert.Equal(t, "short", truncateText("short", 200)) // under limit unchanged

	out := truncateText(strings.Repeat("a", 1000), 200)
	assert.True(t, len(out) <= 200)
	assert.True(t, strings.Contains(out, "truncated"))
}
