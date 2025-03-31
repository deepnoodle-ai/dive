package anthropic

import (
	"encoding/json"

	"github.com/diveagents/dive/llm"
)

const (
	CacheControlTypeEphemeral  = "ephemeral"
	CacheControlTypePersistent = "persistent"
)

type Message struct {
	Role    string          `json:"role"`
	Content []*ContentBlock `json:"content"`
}

type CacheControl struct {
	Type string `json:"type"`
}

type ContentBlock struct {
	ID           string          `json:"id,omitempty"`
	Type         string          `json:"type"`
	Name         string          `json:"name,omitempty"`
	Text         string          `json:"text,omitempty"`
	Source       *ImageSource    `json:"source,omitempty"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	Content      string          `json:"content,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	CacheControl *CacheControl   `json:"cache_control,omitempty"`
	Thinking     string          `json:"thinking,omitempty"`  // "Let me analyze this step by step..."
	Signature    string          `json:"signature,omitempty"` // in "thinking" blocks
	Data         string          `json:"data,omitempty"`      // in "redacted_thinking" blocks
}

func (c *ContentBlock) SetCacheControl(cacheControlType string) {
	c.CacheControl = &CacheControl{Type: cacheControlType}
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

//	"thinking": {
//		"type": "enabled",
//		"budget_tokens": 16000
//	},
type Thinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type Request struct {
	Model       string         `json:"model"`
	Messages    []*llm.Message `json:"messages"`
	MaxTokens   *int           `json:"max_tokens,omitempty"`
	Temperature *float64       `json:"temperature,omitempty"`
	System      string         `json:"system,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
	Tools       []*Tool        `json:"tools,omitempty"`
	ToolChoice  *ToolChoice    `json:"tool_choice,omitempty"`
	Thinking    *Thinking      `json:"thinking,omitempty"`
}

type ToolChoiceType string

const (
	ToolChoiceTypeAuto ToolChoiceType = "auto"
	ToolChoiceTypeAny  ToolChoiceType = "any"
	ToolChoiceTypeTool ToolChoiceType = "tool"
)

type ToolChoice struct {
	Type               ToolChoiceType `json:"type"`
	Name               string         `json:"name,omitempty"`
	DisableParallelUse bool           `json:"disable_parallel_tool_use,omitempty"`
}

type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	InputSchema llm.Schema `json:"input_schema"`
}

type Response struct {
	ID           string          `json:"id"`
	Content      []*ContentBlock `json:"content"`
	Model        string          `json:"model"`
	Role         string          `json:"role"`
	StopReason   string          `json:"stop_reason"`
	StopSequence *string         `json:"stop_sequence"`
	Type         string          `json:"type"`
	Usage        Usage           `json:"usage"`
}

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// type StreamEvent struct {
// 	Type         string        `json:"type"`
// 	Index        int           `json:"index"`
// 	Message      StreamMessage `json:"message"`
// 	Delta        StreamDelta   `json:"delta"`
// 	ContentBlock ContentBlock  `json:"content_block"`
// 	Usage        Usage         `json:"usage"`
// }

// type StreamMessage struct {
// 	ID           string         `json:"id"`
// 	Role         string         `json:"role"`
// 	Type         string         `json:"type"`
// 	Model        string         `json:"model"`
// 	StopSequence *string        `json:"stop_sequence"`
// 	StopReason   *string        `json:"stop_reason"`
// 	Content      []ContentBlock `json:"content"`
// 	Usage        Usage          `json:"usage"`
// }

// type StreamDelta struct {
// 	Type         string  `json:"type"`
// 	Text         string  `json:"text"`
// 	StopReason   string  `json:"stop_reason,omitempty"`
// 	StopSequence *string `json:"stop_sequence,omitempty"`
// 	PartialJSON  string  `json:"partial_json,omitempty"`
// 	Thinking     string  `json:"thinking,omitempty"`
// 	Signature    string  `json:"signature,omitempty"`
// }
