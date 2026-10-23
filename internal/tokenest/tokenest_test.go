package tokenest

import (
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestMessageSizesMediaByCostNotBytes(t *testing.T) {
	// A 1.4 MB screenshot, the size a drag-and-drop attachment routinely is.
	screenshot := &llm.Message{Role: llm.User, Content: []llm.Content{
		&llm.TextContent{Text: "what does this image say?"},
		&llm.ImageContent{Source: &llm.ContentSource{
			Type:      llm.ContentSourceTypeBase64,
			MediaType: "image/png",
			Data:      strings.Repeat("A", 1_900_000),
		}},
	}}

	got := Message(screenshot)

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
	assert.True(t, Message(linked) < base64ImageTokens)

	// Text and tool payloads keep their serialized-size estimate.
	text := llm.NewUserTextMessage(strings.Repeat("x", 4_000))
	assert.True(t, Message(text) >= 1_000, "text is still ~4 bytes per token")
}

func TestMessageSizesDocumentsByDecodedLength(t *testing.T) {
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
	small := Message(doc(20_000))
	large := Message(doc(2_000_000))
	assert.True(t, large > small, "a longer document should cost more")
	assert.True(t, small >= base64ImageTokens, "a short document still costs at least a page")
	assert.True(t, large < 2_000_000/4, "a document must not be sized as if it were text")
}
