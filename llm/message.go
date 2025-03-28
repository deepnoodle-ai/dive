package llm

import (
	"encoding/json"
	"strings"
)

// Role indicates the role of a message in a conversation. Either "user",
// "assistant", or "system".
type Role string

const (
	User      Role = "user"
	Assistant Role = "assistant"
	System    Role = "system"
)

func (r Role) String() string {
	return string(r)
}

// Usage contains token usage information for an LLM response.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// Request conveys information about a request to an LLM. This is used primarily
// for hooks and isn't used to interact with the actual LLM providers.
type Request struct {
	Messages []*Message `json:"messages"`
	Config   *Config    `json:"config,omitempty"`
	Body     []byte     `json:"-"`
}

// Response from an LLM
type Response struct {
	ID         string      `json:"id"`
	Model      string      `json:"model"`
	StopReason string      `json:"stop_reason"`
	Role       Role        `json:"role"`
	Message    Message     `json:"message"`
	Usage      Usage       `json:"usage"`
	ToolCalls  []*ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall is a call made by an LLM
type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"`
}

// ContentType indicates the type of a content block in a message
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeImage      ContentType = "image"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
	ContentTypeThinking   ContentType = "thinking"
)

// Content is a single block of content in a message. A message may contain
// multiple content blocks.
type Content struct {
	// Type: text, image, tool_result, ...
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

	// ToolUseID is used when passing a tool result back to the LLM
	ToolUseID string `json:"tool_use_id,omitempty"`

	// Thinking is the thinking of the content
	Thinking string `json:"thinking,omitempty"`

	// Signature is the signature of the content
	Signature string `json:"signature,omitempty"`
}

// Message containing content passed to or from an LLM.
type Message struct {
	ID      string     `json:"id,omitempty"`
	Role    Role       `json:"role"`
	Content []*Content `json:"content"`
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
// were multiple text contents, they are separated by two newlines.
func (m *Message) CompleteText() string {
	var sb strings.Builder
	for i, content := range m.Content {
		if content.Type == ContentTypeText {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(content.Text)
		}
	}
	return sb.String()
}

// WithText appends a text content block to the message.
func (m *Message) WithText(text string) *Message {
	m.Content = append(m.Content, &Content{Type: ContentTypeText, Text: text})
	return m
}

// WithImage appends an image content block to the message.
func (m *Message) WithImage(data string) *Message {
	m.Content = append(m.Content, &Content{Type: ContentTypeImage, Data: data})
	return m
}

// NewMessage creates a new message with the given role and content blocks.
func NewMessage(role Role, content []*Content) *Message {
	return &Message{Role: role, Content: content}
}

// NewUserMessage creates a new user message with a single text content block.
func NewUserMessage(text string) *Message {
	return &Message{
		Role:    User,
		Content: []*Content{{Type: ContentTypeText, Text: text}},
	}
}

// NewSingleUserMessage creates a new user message with a single text content block
// and returns a slice of one message.
func NewSingleUserMessage(text string) []*Message {
	return []*Message{
		{
			Role:    User,
			Content: []*Content{{Type: ContentTypeText, Text: text}},
		},
	}
}

// NewAssistantMessage creates a new assistant message with a single text content block.
func NewAssistantMessage(text string) *Message {
	return &Message{
		Role:    Assistant,
		Content: []*Content{{Type: ContentTypeText, Text: text}},
	}
}

// NewToolOutputMessage creates a new message with the user role and a list of
// tool outputs. Used to pass the results of tool calls back to an LLM.
func NewToolOutputMessage(outputs []*ToolOutput) *Message {
	content := make([]*Content, len(outputs))
	for i, output := range outputs {
		content[i] = &Content{
			Type:      ContentTypeToolResult,
			ToolUseID: output.ID,
			Name:      output.Name,
			Text:      output.Output,
		}
	}
	return &Message{Role: User, Content: content}
}
