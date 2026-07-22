package openaicompletions

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
)

type ReasoningEffort string

const (
	ReasoningEffortNone    ReasoningEffort = "none"
	ReasoningEffortMinimal ReasoningEffort = "minimal"
	ReasoningEffortLow     ReasoningEffort = "low"
	ReasoningEffortMedium  ReasoningEffort = "medium"
	ReasoningEffortHigh    ReasoningEffort = "high"
	ReasoningEffortXHigh   ReasoningEffort = "xhigh"
	ReasoningEffortMax     ReasoningEffort = "max"
)

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type Request struct {
	Model               string          `json:"model"`
	Messages            []Message       `json:"messages"`
	MaxTokens           *int            `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int            `json:"max_completion_tokens,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	StreamOptions       *StreamOptions  `json:"stream_options,omitempty"`
	Tools               []Tool          `json:"tools,omitempty"`
	ToolChoice          any             `json:"tool_choice,omitempty"`
	PresencePenalty     *float64        `json:"presence_penalty,omitempty"`  // -2 to 2, default 0
	FrequencyPenalty    *float64        `json:"frequency_penalty,omitempty"` // -2 to 2, default 0
	ReasoningEffort     ReasoningEffort `json:"reasoning_effort,omitempty"`  // supported reasoning models only
	ReasoningFormat     string          `json:"reasoning_format,omitempty"`  // groq only?
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ContentParts, when non-empty, replaces Content in the marshaled JSON
	// with a content-part array (multimodal messages). When building a
	// request, set at most one of Content and ContentParts. When decoding a
	// response that used the array shape, both are populated: ContentParts
	// holds the parts and Content holds their joined text, so callers that
	// only read Content never silently see an empty message.
	ContentParts []ContentPart `json:"-"`
	Name         string        `json:"name,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
}

func (m Message) MarshalJSON() ([]byte, error) {
	if len(m.ContentParts) == 0 {
		type alias Message
		return json.Marshal(alias(m))
	}
	return json.Marshal(struct {
		Role       string        `json:"role"`
		Content    []ContentPart `json:"content"`
		Name       string        `json:"name,omitempty"`
		ToolCallID string        `json:"tool_call_id,omitempty"`
		ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	}{m.Role, m.ContentParts, m.Name, m.ToolCallID, m.ToolCalls})
}

// UnmarshalJSON accepts both content shapes: a plain string (the usual
// response form) or a content-part array.
func (m *Message) UnmarshalJSON(data []byte) error {
	var aux struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		Name       string          `json:"name"`
		ToolCallID string          `json:"tool_call_id"`
		ToolCalls  []ToolCall      `json:"tool_calls"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m.Role = aux.Role
	m.Name = aux.Name
	m.ToolCallID = aux.ToolCallID
	m.ToolCalls = aux.ToolCalls
	m.Content = ""
	m.ContentParts = nil
	content := bytes.TrimSpace(aux.Content)
	if len(content) == 0 || bytes.Equal(content, []byte("null")) {
		return nil
	}
	switch content[0] {
	case '"':
		return json.Unmarshal(content, &m.Content)
	case '[':
		if err := json.Unmarshal(content, &m.ContentParts); err != nil {
			return err
		}
		// Mirror the text into Content so consumers reading only the string
		// field don't silently observe an empty message.
		m.Content = joinTextParts(m.ContentParts)
		return nil
	default:
		return fmt.Errorf("unexpected message content shape: %s", content)
	}
}

// ContentPart is one element of a multimodal content array in a Chat
// Completions message.
type ContentPart struct {
	Type     string        `json:"type"` // "text", "image_url", or "file"
	Text     string        `json:"text,omitempty"`
	ImageURL *ImageURLPart `json:"image_url,omitempty"`
	File     *FilePart     `json:"file,omitempty"`
}

// ImageURLPart carries an image in a content-part array, referenced by
// public URL or data URL.
type ImageURLPart struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// FilePart carries a file (typically a PDF) in a content-part array, either
// inline as a data URL or by Files API ID.
type FilePart struct {
	Filename string `json:"filename,omitempty"`
	FileData string `json:"file_data,omitempty"`
	FileID   string `json:"file_id,omitempty"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  *schema.Schema `json:"parameters"`
	Strict      bool           `json:"strict,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type Response struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

// PromptTokensDetails breaks down the prompt token count. CachedTokens is the
// portion of prompt_tokens served from the prompt cache (a subset of
// PromptTokens, not additive).
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// CompletionTokensDetails breaks down the completion token count.
// ReasoningTokens is the portion of completion_tokens spent on reasoning (a
// subset of CompletionTokens, not additive).
type CompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// toLLMUsage converts wire usage to llm.Usage, carrying cache and reasoning
// token detail when the server reports it.
func (u Usage) toLLMUsage() llm.Usage {
	usage := llm.Usage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}
	if u.PromptTokensDetails != nil {
		usage.CacheReadInputTokens = u.PromptTokensDetails.CachedTokens
	}
	if u.CompletionTokensDetails != nil {
		usage.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	return usage
}

// {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1694268190,"model":"gpt-4o-mini", "system_fingerprint": "fp_44709d6fcb", "choices":[{"index":0,"delta":{"role":"assistant","content":""},"logprobs":null,"finish_reason":null}]}

// {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1694268190,"model":"gpt-4o-mini", "system_fingerprint": "fp_44709d6fcb", "choices":[{"index":0,"delta":{"content":"Hello"},"logprobs":null,"finish_reason":null}]}

// ....

// {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1694268190,"model":"gpt-4o-mini", "system_fingerprint": "fp_44709d6fcb", "choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}

type StreamResponse struct {
	ID                string         `json:"id"`                 // chatcmpl-B6ffy5hheub7qvA7LWuXEqDXR3TQ5
	Object            string         `json:"object"`             // chat.completion.chunk
	Created           int64          `json:"created"`            // 1740929870
	Model             string         `json:"model"`              // gpt-4o-2024-08-06
	ServiceTier       string         `json:"service_tier"`       // default
	SystemFingerprint string         `json:"system_fingerprint"` // fp_eb9dce56a8
	Choices           []StreamChoice `json:"choices"`
	Usage             Usage          `json:"usage,omitempty"`
}

type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        StreamDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type StreamDelta struct {
	Role      string          `json:"role"`
	Content   string          `json:"content,omitempty"`
	Reasoning string          `json:"reasoning,omitempty"`
	ToolCalls []ToolCallDelta `json:"tool_calls,omitempty"`
}

// ToolCallDelta represents a partial tool call in a streaming response
type ToolCallDelta struct {
	Index    int               `json:"index"`
	ID       string            `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"`
	Function ToolFunctionDelta `json:"function,omitempty"`
}

// ToolFunctionDelta represents a partial function call in a streaming response
type ToolFunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}
