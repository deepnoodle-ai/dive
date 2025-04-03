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

func (u *Usage) Copy() *Usage {
	return &Usage{
		InputTokens:              u.InputTokens,
		OutputTokens:             u.OutputTokens,
		CacheCreationInputTokens: u.CacheCreationInputTokens,
		CacheReadInputTokens:     u.CacheReadInputTokens,
	}
}

func (u *Usage) Add(other *Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheCreationInputTokens += other.CacheCreationInputTokens
	u.CacheReadInputTokens += other.CacheReadInputTokens
}

// Response is the generated response from an LLM. Matches the Anthropic
// response format documented here:
// https://docs.anthropic.com/en/api/messages#response-content
//
// In Dive, all LLM implementations must transform their responses into this
// format.
type Response struct {
	ID           string     `json:"id"`
	Model        string     `json:"model"`
	Role         Role       `json:"role"`
	Content      []*Content `json:"content"`
	StopReason   string     `json:"stop_reason"`
	StopSequence *string    `json:"stop_sequence,omitempty"`
	Type         string     `json:"type"`
	Usage        Usage      `json:"usage"`
}

// Message extracts and returns the message from the response.
func (r *Response) Message() *Message {
	return &Message{
		ID:      r.ID,
		Role:    r.Role,
		Content: r.Content,
	}
}

// ToolCalls extracts and returns all tool calls from the response.
func (r *Response) ToolCalls() []*ToolCall {
	var toolCalls []*ToolCall
	for _, content := range r.Content {
		if content.Type == ContentTypeToolUse {
			toolCalls = append(toolCalls, &ToolCall{
				ID:    content.ID,            // e.g. "toolu_01A09q90qw90lq917835lq9"
				Name:  content.Name,          // tool name
				Input: string(content.Input), // tool call input
			})
		}
	}
	return toolCalls
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

// ContentSourceType indicates the location of the media content.
type ContentSourceType string

const (
	ContentSourceTypeBase64 ContentSourceType = "base64"
	ContentSourceTypeURL    ContentSourceType = "url"
)

func (c ContentSourceType) String() string {
	return string(c)
}

// ContentSource conveys information about media content in a message.
type ContentSource struct {
	// Type is the type of the content source (base64 or url)
	Type ContentSourceType `json:"type"`

	// MediaType is the media type of the content. E.g. "image/jpeg"
	MediaType string `json:"media_type,omitempty"`

	// Data is base64 encoded data
	Data string `json:"data,omitempty"`

	// URL is the URL of the content
	URL string `json:"url,omitempty"`
}

// Content is a single block of content in a message. A message may contain
// multiple content blocks. The "type" and "text" fields are the most commonly
// used fields. The others are relevant when passing media and using tools.
type Content struct {
	// Type: text, image, tool_result, ...
	Type ContentType `json:"type"`

	// Text content
	Text string `json:"text,omitempty"`

	// Source conveys information about media content in a message.
	Source *ContentSource `json:"source,omitempty"`

	// ID of tool use (Tool use result only)
	ToolUseID string `json:"tool_use_id,omitempty"`

	// Content returned by a tool (Tool use result only)
	Content string `json:"content,omitempty"`

	// ID for the content (Tool use only)
	ID string `json:"id,omitempty"`

	// Name of the tool (Tool use only)
	Name string `json:"name,omitempty"`

	// Tool input (Tool use only)
	Input json.RawMessage `json:"input,omitempty"`

	// Thinking content (Assistant only)
	Thinking string `json:"thinking,omitempty"`

	// Signature for the content (Assistant only)
	Signature string `json:"signature,omitempty"`

	// Data in "redacted_thinking" blocks (Assistant only)
	Data string `json:"data,omitempty"`

	// Marks a cacheable content block
	CacheControl *CacheControl `json:"cache_control,omitempty"`

	// Hidden indicates that the content should not be shown to the user
	Hidden bool `json:"hidden,omitempty"`
}

// Message containing content passed to or from an LLM.
type Message struct {
	ID      string     `json:"id,omitempty"`
	Role    Role       `json:"role"`
	Content []*Content `json:"content"`
}

// LastText returns the last text content in the message.
func (m *Message) LastText() string {
	for i := len(m.Content) - 1; i >= 0; i-- {
		if m.Content[i].Type == ContentTypeText {
			return m.Content[i].Text
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
		if content.Type == ContentTypeText {
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
	m.Content = append(m.Content, &Content{Type: ContentTypeText, Text: text})
	return m
}

// WithImageData appends an image content block to the message.
func (m *Message) WithImageData(mediaType, base64Data string) *Message {
	m.Content = append(m.Content, &Content{
		Type: ContentTypeImage,
		Source: &ContentSource{
			Type:      ContentSourceTypeBase64,
			MediaType: mediaType,
			Data:      base64Data,
		},
	})
	return m
}

// Messages is shorthand for a slice of messages.
type Messages []*Message

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

// NewAssistantMessage creates a new assistant message with a single text content block.
func NewAssistantMessage(text string) *Message {
	return &Message{
		Role:    Assistant,
		Content: []*Content{{Type: ContentTypeText, Text: text}},
	}
}

// NewToolOutputMessage creates a new message with the user role and a list of
// tool outputs. Used to pass the results of tool calls back to an LLM.
func NewToolOutputMessage(outputs []*ToolResult) *Message {
	content := make([]*Content, len(outputs))
	for i, output := range outputs {
		content[i] = &Content{
			Type:      ContentTypeToolResult,
			ToolUseID: output.ID,
			Name:      output.Name,
			Content:   output.Output,
		}
	}
	return &Message{Role: User, Content: content}
}

// CacheControl is used to control caching of content blocks.
type CacheControl struct {
	Type CacheControlType `json:"type"`
}
