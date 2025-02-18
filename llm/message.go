package llm

import (
	"encoding/json"
	"strings"
)

type ContentType string

const (
	ContentTypeText    ContentType = "text"
	ContentTypeImage   ContentType = "image"
	ContentTypeToolUse ContentType = "tool_use"
)

// Content is a single piece of content in a message.
type Content struct {
	// Type: text, image, ...
	Type ContentType `json:"type"`

	// Text content
	Text string `json:"text,omitempty"`

	// Data is base64 encoded data
	Data string `json:"data,omitempty"`

	// MediaType is the media type of the content
	MediaType string `json:"media_type,omitempty"`

	// ID is the ID of the content
	ID string `json:"id,omitempty"`

	// Name is the name of the content
	Name string `json:"name,omitempty"`

	// Input is the input of the content
	Input json.RawMessage `json:"input,omitempty"`
}

// Message passed to an LLM for generation.
type Message struct {
	Role    Role      `json:"role"`
	Content []Content `json:"content"`
}

// Text returns the message text content. Specifically, it returns the last text
// content in the message. To retrieve a concatenated text from all message
// content, use CompleteText instead.
func (m *Message) Text() string {
	for i := len(m.Content) - 1; i >= 0; i-- {
		if m.Content[i].Type == ContentTypeText {
			return m.Content[i].Text
		}
	}
	return ""
}

// CompleteText returns a concatenated text from all message content. If there
// were multiple text contents, they are separated by a newline.
func (m *Message) CompleteText() string {
	var sb strings.Builder
	for i, content := range m.Content {
		if content.Type == ContentTypeText {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(content.Text)
		}
	}
	return sb.String()
}

func (m *Message) WithText(text string) *Message {
	m.Content = append(m.Content, Content{Type: ContentTypeText, Text: text})
	return m
}

func (m *Message) WithImage(data string) *Message {
	m.Content = append(m.Content, Content{Type: ContentTypeImage, Data: data})
	return m
}

func NewMessage(role Role, content []Content) *Message {
	return &Message{Role: role, Content: content}
}

func NewUserMessage(text string) *Message {
	return &Message{
		Role:    User,
		Content: []Content{{Type: ContentTypeText, Text: text}},
	}
}

func NewAssistantMessage(text string) *Message {
	return &Message{
		Role:    Assistant,
		Content: []Content{{Type: ContentTypeText, Text: text}},
	}
}

// type RawMessage struct {
// 	Role    Role            `json:"role"`
// 	Content json.RawMessage `json:"content"`
// }

// func (m *RawMessage) Parse() (*Message, error) {
// 	message := &Message{Role: m.Role}
// 	// First try to unmarshal as string
// 	var stringContent string
// 	if err := json.Unmarshal(m.Content, &stringContent); err == nil {
// 		message.Text = stringContent
// 		return message, nil
// 	}
// 	// If that fails, try to unmarshal as array of Content
// 	var contentArray []Content
// 	if err := json.Unmarshal(m.Content, &contentArray); err != nil {
// 		return nil, fmt.Errorf("content must be either string or array: %w", err)
// 	}
// 	message.Content = contentArray
// 	return message, nil
// }
