package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func toolResultFromMessages(t *testing.T, messages []*llm.Message) *llm.ToolResultContent {
	t.Helper()
	converted, err := convertMessages(messages)
	assert.NoError(t, err)
	for _, msg := range converted {
		for _, c := range msg.Content {
			if trc, ok := c.(*llm.ToolResultContent); ok {
				return trc
			}
		}
	}
	t.Fatal("no tool result found in converted messages")
	return nil
}

// TestConvertToolResultImageBlocks verifies typed image blocks (e.g. from an
// MCP tool returning a screenshot) are converted to Anthropic's native image
// block shape, which requires a source object rather than flat data fields.
func TestConvertToolResultImageBlocks(t *testing.T) {
	trc := toolResultFromMessages(t, []*llm.Message{
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: "toolu_1",
			Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "captured screenshot"},
				{Type: dive.ToolResultContentTypeImage, Data: "aW1nZGF0YQ==", MimeType: "image/png"},
			},
		}),
	})

	content, ok := trc.Content.([]llm.Content)
	assert.True(t, ok)
	assert.Len(t, content, 2)

	text, ok := content[0].(*llm.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "captured screenshot", text.Text)

	image, ok := content[1].(*llm.ImageContent)
	assert.True(t, ok)
	assert.Equal(t, llm.ContentSourceTypeBase64, image.Source.Type)
	assert.Equal(t, "image/png", image.Source.MediaType)
	assert.Equal(t, "aW1nZGF0YQ==", image.Source.Data)

	// The converted result must marshal to Anthropic's wire shape.
	wire, err := json.Marshal(trc)
	assert.NoError(t, err)
	assert.Contains(t, string(wire), `"source":{"type":"base64","media_type":"image/png","data":"aW1nZGF0YQ=="}`)
}

// TestConvertToolResultTextBlocksWire verifies text-only typed blocks keep
// the same wire shape they had when marshaled directly.
func TestConvertToolResultTextBlocksWire(t *testing.T) {
	trc := toolResultFromMessages(t, []*llm.Message{
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: "toolu_1",
			Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "line one"},
			},
		}),
	})
	wire, err := json.Marshal(trc)
	assert.NoError(t, err)
	assert.Contains(t, string(wire), `"content":[{"type":"text","text":"line one"}]`)
}

// TestConvertToolResultBlocksSurviveJSONRoundTrip verifies blocks that
// round-tripped through session persistence (arriving as []any) are still
// converted to native shapes.
func TestConvertToolResultBlocksSurviveJSONRoundTrip(t *testing.T) {
	original := llm.NewToolResultMessage(&llm.ToolResultContent{
		ToolUseID: "toolu_1",
		Content: []*dive.ToolResultContent{
			{Type: dive.ToolResultContentTypeImage, Data: "aW1nZGF0YQ==", MimeType: "image/jpeg"},
		},
	})
	body, err := json.Marshal(original)
	assert.NoError(t, err)
	var replayed llm.Message
	assert.NoError(t, json.Unmarshal(body, &replayed))

	trc := toolResultFromMessages(t, []*llm.Message{&replayed})
	content, ok := trc.Content.([]llm.Content)
	assert.True(t, ok)
	assert.Len(t, content, 1)
	image, ok := content[0].(*llm.ImageContent)
	assert.True(t, ok)
	assert.Equal(t, "image/jpeg", image.Source.MediaType)
}

// TestConvertToolResultStringContentUnchanged verifies plain string tool
// results pass through untouched.
func TestConvertToolResultStringContentUnchanged(t *testing.T) {
	trc := toolResultFromMessages(t, []*llm.Message{
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: "toolu_1",
			Content:   "plain output",
		}),
	})
	assert.Equal(t, "plain output", trc.Content)
}

// TestConvertToolResultAudioBlockPlaceholder verifies block types Anthropic
// cannot accept are replaced with a text placeholder instead of producing an
// invalid request.
func TestConvertToolResultAudioBlockPlaceholder(t *testing.T) {
	trc := toolResultFromMessages(t, []*llm.Message{
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: "toolu_1",
			Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeAudio, Data: "YXVkaW8=", MimeType: "audio/wav"},
			},
		}),
	})
	content, ok := trc.Content.([]llm.Content)
	assert.True(t, ok)
	text, ok := content[0].(*llm.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "[audio content omitted]", text.Text)
}
