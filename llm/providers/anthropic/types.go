package anthropic

import (
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
)

const (
	CacheControlTypeEphemeral  = "ephemeral"
	CacheControlTypePersistent = "persistent"
)

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
	Model       string           `json:"model"`
	Messages    []*llm.Message   `json:"messages"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	System      string           `json:"system,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
	Tools       []map[string]any `json:"tools,omitempty"`
	ToolChoice  *ToolChoice      `json:"tool_choice,omitempty"`
	Thinking    *Thinking        `json:"thinking,omitempty"`
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
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	InputSchema schema.Schema `json:"input_schema"`
}
