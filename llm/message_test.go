package llm

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestMessage_Text(t *testing.T) {
	t.Run("single text content", func(t *testing.T) {
		msg := NewAssistantTextMessage("hello world")
		assert.Equal(t, "hello world", msg.Text())
	})

	t.Run("multiple text contents separated by newlines", func(t *testing.T) {
		msg := &Message{Role: Assistant}
		msg.WithText("first", "second", "third")
		assert.Equal(t, "first\n\nsecond\n\nthird", msg.Text())
	})

	t.Run("empty message returns empty string", func(t *testing.T) {
		msg := &Message{Role: Assistant, Content: []Content{}}
		assert.Equal(t, "", msg.Text())
	})

	t.Run("skips non-text content", func(t *testing.T) {
		msg := &Message{
			Role: Assistant,
			Content: []Content{
				&ThinkingContent{Thinking: "hmm"},
				&TextContent{Text: "answer"},
			},
		}
		assert.Equal(t, "answer", msg.Text())
	})
}

func TestMessage_LastText(t *testing.T) {
	t.Run("returns last text block", func(t *testing.T) {
		msg := &Message{Role: Assistant}
		msg.WithText("first", "last")
		assert.Equal(t, "last", msg.LastText())
	})

	t.Run("returns empty string for no text", func(t *testing.T) {
		msg := &Message{Role: Assistant, Content: []Content{}}
		assert.Equal(t, "", msg.LastText())
	})
}

func TestMessage_WithText(t *testing.T) {
	msg := &Message{Role: User}
	result := msg.WithText("hello")
	// Returns the same message (builder pattern)
	assert.True(t, result == msg)
	assert.Equal(t, 1, len(msg.Content))
	assert.Equal(t, "hello", msg.Content[0].(*TextContent).Text)
}

func TestMessage_WithContent(t *testing.T) {
	msg := &Message{Role: User}
	img := &ImageContent{Source: &ContentSource{Type: ContentSourceTypeURL, URL: "https://example.com/img.png"}}
	result := msg.WithContent(img)
	assert.True(t, result == msg)
	assert.Equal(t, 1, len(msg.Content))
	assert.Equal(t, ContentTypeImage, msg.Content[0].Type())
}

func TestMessage_ImageContent(t *testing.T) {
	t.Run("returns image content when present", func(t *testing.T) {
		img := &ImageContent{Source: &ContentSource{Type: ContentSourceTypeURL, URL: "https://example.com/img.png"}}
		msg := &Message{Role: User, Content: []Content{
			&TextContent{Text: "look at this"},
			img,
		}}
		found, ok := msg.ImageContent()
		assert.True(t, ok)
		assert.Equal(t, "https://example.com/img.png", found.Source.URL)
	})

	t.Run("returns false when no image", func(t *testing.T) {
		msg := NewUserTextMessage("hello")
		_, ok := msg.ImageContent()
		assert.True(t, !ok)
	})
}

func TestMessage_ThinkingContent(t *testing.T) {
	t.Run("returns thinking content when present", func(t *testing.T) {
		msg := &Message{Role: Assistant, Content: []Content{
			&ThinkingContent{Thinking: "let me think..."},
			&TextContent{Text: "answer"},
		}}
		found, ok := msg.ThinkingContent()
		assert.True(t, ok)
		assert.Equal(t, "let me think...", found.Thinking)
	})

	t.Run("returns false when no thinking", func(t *testing.T) {
		msg := NewAssistantTextMessage("hello")
		_, ok := msg.ThinkingContent()
		assert.True(t, !ok)
	})
}

func TestMessage_DecodeInto(t *testing.T) {
	t.Run("decodes JSON text into struct", func(t *testing.T) {
		msg := NewAssistantTextMessage(`{"name":"test","value":42}`)
		var result struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}
		err := msg.DecodeInto(&result)
		assert.Nil(t, err)
		assert.Equal(t, "test", result.Name)
		assert.Equal(t, 42, result.Value)
	})

	t.Run("returns error for no text content", func(t *testing.T) {
		msg := &Message{Role: Assistant, Content: []Content{}}
		var result map[string]any
		err := msg.DecodeInto(&result)
		assert.NotNil(t, err)
	})
}

func TestMessage_Copy(t *testing.T) {
	t.Run("creates independent copy", func(t *testing.T) {
		original := NewAssistantTextMessage("hello")
		copied := original.Copy()

		// Same content
		assert.Equal(t, original.Role, copied.Role)
		assert.Equal(t, original.Text(), copied.Text())

		// Independent - modifying copy doesn't affect original
		copied.WithText("added")
		assert.Equal(t, 1, len(original.Content))
		assert.Equal(t, 2, len(copied.Content))
	})
}

func TestMessage_MarshalUnmarshalJSON(t *testing.T) {
	t.Run("round-trips text message", func(t *testing.T) {
		msg := NewUserTextMessage("hello world")
		data, err := json.Marshal(msg)
		assert.Nil(t, err)

		var decoded Message
		err = json.Unmarshal(data, &decoded)
		assert.Nil(t, err)
		assert.Equal(t, User, decoded.Role)
		assert.Equal(t, "hello world", decoded.Text())
	})

	t.Run("round-trips message with thinking content", func(t *testing.T) {
		msg := &Message{
			Role: Assistant,
			Content: []Content{
				&ThinkingContent{Thinking: "reasoning here", Signature: "sig123"},
				&TextContent{Text: "the answer"},
			},
		}
		data, err := json.Marshal(msg)
		assert.Nil(t, err)

		var decoded Message
		err = json.Unmarshal(data, &decoded)
		assert.Nil(t, err)
		assert.Equal(t, 2, len(decoded.Content))
		assert.Equal(t, ContentTypeThinking, decoded.Content[0].Type())
		assert.Equal(t, ContentTypeText, decoded.Content[1].Type())
	})
}
