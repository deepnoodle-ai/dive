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
	PromptCacheKey      string          `json:"prompt_cache_key,omitempty"`
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
	// Reasoning is the OpenAI-compatible plaintext reasoning extension used by
	// gateways such as OpenRouter.
	Reasoning string `json:"reasoning,omitempty"`
	// ReasoningDetails preserves OpenRouter's structured reasoning blocks
	// verbatim for replay. It is intentionally opaque to this shared adapter.
	ReasoningDetails json.RawMessage `json:"reasoning_details,omitempty"`
}

func (m Message) MarshalJSON() ([]byte, error) {
	type alias Message
	if len(m.ContentParts) == 0 {
		return json.Marshal(alias(m))
	}
	return json.Marshal(struct {
		alias
		Content []ContentPart `json:"content"`
	}{
		alias:   alias(m),
		Content: m.ContentParts,
	})
}

// UnmarshalJSON accepts both content shapes: a plain string (the usual
// response form) or a content-part array.
func (m *Message) UnmarshalJSON(data []byte) error {
	var aux struct {
		Role             string          `json:"role"`
		Content          json.RawMessage `json:"content"`
		Name             string          `json:"name"`
		ToolCallID       string          `json:"tool_call_id"`
		ToolCalls        []ToolCall      `json:"tool_calls"`
		Reasoning        string          `json:"reasoning"`
		ReasoningDetails json.RawMessage `json:"reasoning_details"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m.Role = aux.Role
	m.Name = aux.Name
	m.ToolCallID = aux.ToolCallID
	m.ToolCalls = aux.ToolCalls
	m.Reasoning = aux.Reasoning
	reasoningDetails := bytes.TrimSpace(aux.ReasoningDetails)
	if len(reasoningDetails) == 0 || bytes.Equal(reasoningDetails, []byte("null")) {
		m.ReasoningDetails = nil
	} else {
		m.ReasoningDetails = append(m.ReasoningDetails[:0], reasoningDetails...)
	}
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
	Type string `json:"type"` // "text", "thinking", "image_url", or "file"
	Text string `json:"text,omitempty"`
	// Thinking is populated for Mistral ThinkChunk parts. Each nested part is
	// currently a text chunk, but the recursive shape matches the wire format.
	Thinking              []ContentPart          `json:"thinking,omitempty"`
	ImageURL              *ImageURLPart          `json:"image_url,omitempty"`
	File                  *FilePart              `json:"file,omitempty"`
	PromptCacheBreakpoint *PromptCacheBreakpoint `json:"prompt_cache_breakpoint,omitempty"`
}

type PromptCacheBreakpoint struct {
	Mode string `json:"mode"`
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
	Cost                    *float64                 `json:"cost,omitempty"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
	present                 bool
}

func (u *Usage) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*u = Usage{}
		return nil
	}
	type alias Usage
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = Usage(decoded)
	u.present = true
	return nil
}

// PromptTokensDetails breaks down the wire prompt token count. CachedTokens and
// CacheWriteTokens are subsets of PromptTokens on the wire; toLLMUsage resolves
// that relationship into Dive's disjoint input buckets.
type PromptTokensDetails struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
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
	prompt := max(0, u.PromptTokens)
	cached := 0
	written := 0
	if u.PromptTokensDetails != nil {
		cached = min(max(0, u.PromptTokensDetails.CachedTokens), prompt)
		written = min(max(0, u.PromptTokensDetails.CacheWriteTokens), prompt-cached)
	}
	usage := llm.Usage{
		InputTokens:              prompt - cached - written,
		OutputTokens:             u.CompletionTokens,
		CacheCreationInputTokens: written,
		CacheReadInputTokens:     cached,
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
	Role string `json:"role"`
	// Content holds the ordinary string form. ContentParts holds the typed
	// array form used by Mistral while it streams thinking chunks.
	Content          string          `json:"-"`
	ContentParts     []ContentPart   `json:"-"`
	Reasoning        string          `json:"reasoning,omitempty"`
	ReasoningDetails json.RawMessage `json:"reasoning_details,omitempty"`
	ToolCalls        []ToolCallDelta `json:"tool_calls,omitempty"`
}

// UnmarshalJSON accepts both Chat Completions delta.content shapes: the usual
// string and Mistral's array of ThinkChunk/TextChunk objects.
func (d *StreamDelta) UnmarshalJSON(data []byte) error {
	var aux struct {
		Role             string          `json:"role"`
		Content          json.RawMessage `json:"content"`
		Reasoning        string          `json:"reasoning"`
		ReasoningDetails json.RawMessage `json:"reasoning_details"`
		ToolCalls        []ToolCallDelta `json:"tool_calls"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	d.Role = aux.Role
	d.Reasoning = aux.Reasoning
	reasoningDetails := bytes.TrimSpace(aux.ReasoningDetails)
	if len(reasoningDetails) == 0 || bytes.Equal(reasoningDetails, []byte("null")) {
		d.ReasoningDetails = nil
	} else {
		d.ReasoningDetails = append(d.ReasoningDetails[:0], reasoningDetails...)
	}
	d.ToolCalls = aux.ToolCalls
	d.Content = ""
	d.ContentParts = nil
	content := bytes.TrimSpace(aux.Content)
	if len(content) == 0 || bytes.Equal(content, []byte("null")) {
		return nil
	}
	switch content[0] {
	case '"':
		return json.Unmarshal(content, &d.Content)
	case '[':
		return json.Unmarshal(content, &d.ContentParts)
	default:
		return fmt.Errorf("unexpected stream delta content shape: %s", content)
	}
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
