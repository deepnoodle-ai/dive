package llm

import (
	"encoding/json"
	"fmt"
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

// WithText appends text content block(s) to the message.
func (m *Message) WithText(text ...string) *Message {
	for _, t := range text {
		m.Content = append(m.Content, &TextContent{Text: t})
	}
	return m
}

// WithContent appends content block(s) to the message.
func (m *Message) WithContent(content ...Content) *Message {
	m.Content = append(m.Content, content...)
	return m
}

// ImageContent returns the first image content in the message, if any.
func (m *Message) ImageContent() (*ImageContent, bool) {
	for _, content := range m.Content {
		if image, ok := content.(*ImageContent); ok {
			return image, true
		}
	}
	return nil, false
}

// ThinkingContent returns the first thinking content in the message, if any.
func (m *Message) ThinkingContent() (*ThinkingContent, bool) {
	for _, content := range m.Content {
		if thinking, ok := content.(*ThinkingContent); ok {
			return thinking, true
		}
	}
	return nil, false
}

// DecodeInto decodes the last text content in the message as JSON into a given
// Go object. This pairs with the WithResponseFormat request option.
func (m *Message) DecodeInto(v any) error {
	for i := len(m.Content) - 1; i >= 0; i-- {
		switch content := m.Content[i].(type) {
		case *TextContent:
			return json.Unmarshal([]byte(content.Text), v)
		}
	}
	return fmt.Errorf("no text content found")
}

// Copy creates a deep copy of the message.
//
// This method uses JSON marshaling/unmarshaling to create a fully independent
// copy of the message including all content blocks. The copied message can be
// modified without affecting the original.
//
// This is primarily used by ThreadRepository.ForkThread to ensure that forked
// conversation threads have independent message histories.
//
// If marshaling fails (which should be rare), falls back to a shallow copy
// of the content slice.
func (m *Message) Copy() *Message {
	data, err := json.Marshal(m)
	if err != nil {
		// Fall back to shallow copy if marshaling fails
		contentCopy := make([]Content, len(m.Content))
		copy(contentCopy, m.Content)
		return &Message{
			ID:      m.ID,
			Role:    m.Role,
			Content: contentCopy,
		}
	}
	var messageCopy Message
	if err := json.Unmarshal(data, &messageCopy); err != nil {
		// Fall back to shallow copy if unmarshaling fails
		contentCopy := make([]Content, len(m.Content))
		copy(contentCopy, m.Content)
		return &Message{
			ID:      m.ID,
			Role:    m.Role,
			Content: contentCopy,
		}
	}
	return &messageCopy
}

// MarshalJSON implements custom marshaling for Message to properly handle
// the polymorphic Content field.
func (m *Message) MarshalJSON() ([]byte, error) {
	type tempMessage struct {
		ID      string            `json:"id,omitempty"`
		Role    Role              `json:"role"`
		Content []json.RawMessage `json:"content"`
	}

	// Marshal each content item individually
	var contentRaw []json.RawMessage
	for _, content := range m.Content {
		contentBytes, err := json.Marshal(content)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal content: %v", err)
		}
		contentRaw = append(contentRaw, contentBytes)
	}

	tmp := tempMessage{
		ID:      m.ID,
		Role:    m.Role,
		Content: contentRaw,
	}

	return json.Marshal(tmp)
}

// UnmarshalJSON implements custom unmarshaling for Message to properly handle
// the polymorphic Content field.
func (m *Message) UnmarshalJSON(data []byte) error {
	type tempMessage struct {
		ID      string            `json:"id,omitempty"`
		Role    Role              `json:"role"`
		Content []json.RawMessage `json:"content"`
	}

	// Unmarshal JSON into the temporary struct
	var tmp tempMessage
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	// Copy all fields except Content
	m.ID = tmp.ID
	m.Role = tmp.Role

	// Process each content item
	m.Content = make([]Content, 0, len(tmp.Content))
	for _, rawContent := range tmp.Content {
		content, err := UnmarshalContent(rawContent)
		if err != nil {
			return fmt.Errorf("failed to unmarshal content: %v", err)
		}
		m.Content = append(m.Content, content)
	}
	return nil
}
