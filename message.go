package agent

import (
	"encoding/json"
	"fmt"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

func (r Role) String() string {
	return string(r)
}

type MessageContentSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
}

type MessageContentType string

const (
	MessageContentTypeText       MessageContentType = "text"
	MessageContentTypeImage      MessageContentType = "image"
	MessageContentTypeToolUse    MessageContentType = "tool_use"
	MessageContentTypeToolResult MessageContentType = "tool_result"
)

type MessageContent struct {

	// Type: text, image, tool_use, tool_result
	Type MessageContentType `json:"type"`

	// Text content
	Text string `json:"text,omitempty"`

	// ID is the tool use ID
	ID string `json:"id,omitempty"`

	// Name is the tool name
	Name string `json:"name,omitempty"`

	// Source is used to provide image or other file content
	Source *MessageContentSource `json:"source,omitempty"`

	// Input should be set on tool use content only
	Input json.RawMessage `json:"input,omitempty"`

	// ToolUseID should be set on tool result content only
	ToolUseID string `json:"tool_use_id,omitempty"`
}

type Message struct {
	Role       Role             `json:"role"`
	Content    []MessageContent `json:"content,omitempty"`
	Text       string           `json:"text,omitempty"`
	UserID     string           `json:"user_id,omitempty"`
	ExternalID string           `json:"external_id,omitempty"`
}

type RawMessage struct {
	Role    Role            `json:"role"`
	Content json.RawMessage `json:"content"`
}

func (m *RawMessage) Parse() (*Message, error) {
	message := &Message{Role: m.Role}
	// First try to unmarshal as string
	var stringContent string
	if err := json.Unmarshal(m.Content, &stringContent); err == nil {
		message.Text = stringContent
		return message, nil
	}
	// If that fails, try to unmarshal as array of MessageContent
	var contentArray []MessageContent
	if err := json.Unmarshal(m.Content, &contentArray); err != nil {
		return nil, fmt.Errorf("content must be either string or array: %w", err)
	}
	message.Content = contentArray
	return message, nil
}

func (m *Message) WithUserID(userID string) *Message {
	m.UserID = userID
	return m
}

func (m *Message) WithExternalID(externalID string) *Message {
	m.ExternalID = externalID
	return m
}

func (m *Message) GetText() string {
	if m.Text != "" {
		return m.Text
	}
	// Return last text content
	for i := len(m.Content) - 1; i >= 0; i-- {
		if m.Content[i].Type == MessageContentTypeText {
			return m.Content[i].Text
		}
	}
	return ""
}

func NewTextMessage(role Role, content string) *Message {
	return &Message{Role: role, Text: content}
}

func NewUserTextMessage(content string) *Message {
	return &Message{Role: RoleUser, Text: content}
}

func NewAssistantTextMessage(content string) *Message {
	return &Message{Role: RoleAssistant, Text: content}
}

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

type Response struct {
	ID      string   `json:"id,omitempty"`
	Model   string   `json:"model,omitempty"`
	Object  any      `json:"object,omitempty"`
	Usage   Usage    `json:"usage,omitempty"`
	Message *Message `json:"message,omitempty"`
}

type MessageOption func(config *MessageConfig)

type MessageConfig struct {
	ToolFunction ToolFunction      `json:"-"`
	Model        string            `json:"model"`
	MaxTokens    *int              `json:"max_tokens,omitempty"`
	Temperature  *float64          `json:"temperature,omitempty"`
	SystemPrompt string            `json:"system_prompt,omitempty"`
	Tools        []*ToolDefinition `json:"tools,omitempty"`
	ToolChoice   string            `json:"tool_choice,omitempty"`
	CacheControl string            `json:"cache_control,omitempty"`
}

// WithModel sets the model for the message.
func WithModel(model string) MessageOption {
	return func(config *MessageConfig) {
		config.Model = model
	}
}

// WithToolFunction sets the tool function for the message.
func WithToolFunction(toolFunc ToolFunction) MessageOption {
	return func(config *MessageConfig) {
		config.ToolFunction = toolFunc
	}
}

// WithMaxTokens sets the max tokens for the message.
func WithMaxTokens(maxTokens int) MessageOption {
	return func(config *MessageConfig) {
		config.MaxTokens = &maxTokens
	}
}

// WithTemperature sets the temperature for the message.
func WithTemperature(temperature float64) MessageOption {
	return func(config *MessageConfig) {
		config.Temperature = &temperature
	}
}

// WithSystemPrompt sets the system prompt for the message.
func WithSystemPrompt(systemPrompt string) MessageOption {
	return func(config *MessageConfig) {
		config.SystemPrompt = systemPrompt
	}
}

// WithTools sets the tools for the message.
func WithTools(tools []*ToolDefinition) MessageOption {
	return func(config *MessageConfig) {
		config.Tools = tools
	}
}

// WithToolChoice sets the tool choice for the message.
func WithToolChoice(toolChoice string) MessageOption {
	return func(config *MessageConfig) {
		config.ToolChoice = toolChoice
	}
}

// WithCacheControl sets the cache control for the message.
func WithCacheControl(cacheControl string) MessageOption {
	return func(config *MessageConfig) {
		config.CacheControl = cacheControl
	}
}
