package llm

import (
	"strings"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestEstimateTokensSizesMediaByCostNotBytes(t *testing.T) {
	// A 1.4 MB screenshot, the size a drag-and-drop attachment routinely is.
	screenshot := &Message{Role: User, Content: []Content{
		&TextContent{Text: "what does this image say?"},
		&ImageContent{Source: &ContentSource{
			Type:      ContentSourceTypeBase64,
			MediaType: "image/png",
			Data:      strings.Repeat("A", 1_900_000),
		}},
	}}

	got := EstimateTokens(screenshot)

	// Sizing the base64 at 4 bytes per token would call this ~475k — enough to
	// blow past any compaction threshold and summarize the conversation away
	// the moment an image is attached.
	assert.True(t, got < 2_500, "one image should not read as a full context window, got %d", got)
	assert.True(t, got >= base64ImageTokens, "the image still has to be counted, got %d", got)

	// A URL source carries no payload, so it stays on the serialized-size path.
	linked := &Message{Role: User, Content: []Content{
		&ImageContent{Source: &ContentSource{
			Type: ContentSourceTypeURL,
			URL:  "https://example.com/a.png",
		}},
	}}
	assert.True(t, EstimateTokens(linked) < base64ImageTokens)

	// Text and tool payloads keep their serialized-size estimate.
	text := NewUserTextMessage(strings.Repeat("x", 4_000))
	assert.True(t, EstimateTokens(text) >= 1_000, "text is still ~4 bytes per token")
}

func TestEstimateTokensSizesDocumentsByDecodedLength(t *testing.T) {
	doc := func(base64Len int) *Message {
		return &Message{Role: User, Content: []Content{
			&DocumentContent{Source: &ContentSource{
				Type:      ContentSourceTypeBase64,
				MediaType: "application/pdf",
				Data:      strings.Repeat("A", base64Len),
			}},
		}}
	}

	// A big PDF costs more than a small one, but nothing like its byte count.
	small := EstimateTokens(doc(20_000))
	large := EstimateTokens(doc(2_000_000))
	assert.True(t, large > small, "a longer document should cost more")
	assert.True(t, small >= base64ImageTokens, "a short document still costs at least a page")
	assert.True(t, large < 2_000_000/4, "a document must not be sized as if it were text")
}
