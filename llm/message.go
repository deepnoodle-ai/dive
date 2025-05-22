package llm

import (
	"strings"
)

// Role indicates the role of a message in a conversation.
// Either "user", "assistant", or "system".
type Role string

const (
	User      Role = "user"
	Assistant Role = "assistant"
	System    Role = "system"
)

func (r Role) String() string {
	return string(r)
}

// Messages is shorthand for a slice of messages.
type Messages []*Message

// Message containing content passed to or from an LLM.
type Message struct {
	ID      string    `json:"id,omitempty"`
	Role    Role      `json:"role"`
	Content []Content `json:"content"`
}

// LastText returns the last text content in the message.
func (m *Message) LastText() string {
	for i := len(m.Content) - 1; i >= 0; i-- {
		switch content := m.Content[i].(type) {
		case *TextContent:
			return content.Text
		}
	}
	return ""
}

// Text returns a concatenated text from all message content. If there
// were multiple text contents, they are separated by two newlines.
func (m *Message) Text() string {
	var textCount int
	var sb strings.Builder
	for _, content := range m.Content {
		switch content := content.(type) {
		case *TextContent:
			if textCount > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(content.Text)
			textCount++
		}
	}
	return sb.String()
}

// WithText appends a text content block to the message.
func (m *Message) WithText(text string) *Message {
	m.Content = append(m.Content, &TextContent{Text: text})
	return m
}

// WithImageData appends an image content block to the message.
func (m *Message) WithImageData(mediaType, base64Data string) *Message {
	m.Content = append(m.Content, &ImageContent{
		Source: &ContentSource{
			Type:      ContentSourceTypeBase64,
			MediaType: mediaType,
			Data:      base64Data,
		},
	})
	return m
}

// Messages implements the Messages interface, allowing a single message to
// be provided to Agent generation methods.
func (m *Message) Messages() []*Message {
	return []*Message{m}
}
