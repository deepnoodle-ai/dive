package anthropic

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
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

type Thinking struct {
	Type         string `json:"type"` // "enabled"
	BudgetTokens int    `json:"budget_tokens"`
}

type Request struct {
	Model       string                `json:"model"`
	Messages    []*llm.Message        `json:"messages"`
	MaxTokens   *int                  `json:"max_tokens,omitempty"`
	Temperature *float64              `json:"temperature,omitempty"`
	System      string                `json:"system,omitempty"`
	Stream      bool                  `json:"stream,omitempty"`
	Tools       []map[string]any      `json:"tools,omitempty"`
	ToolChoice  *ToolChoice           `json:"tool_choice,omitempty"`
	Thinking    *Thinking             `json:"thinking,omitempty"`
	MCPServers  []llm.MCPServerConfig `json:"mcp_servers,omitempty"`
}

type ToolChoiceType string

const (
	ToolChoiceTypeAuto ToolChoiceType = "auto"
	ToolChoiceTypeAny  ToolChoiceType = "any"
	ToolChoiceTypeTool ToolChoiceType = "tool"
	ToolChoiceTypeNone ToolChoiceType = "none"
)

type ToolChoice struct {
	Type                   ToolChoiceType `json:"type"`
	Name                   string         `json:"name,omitempty"`
	DisableParallelToolUse bool           `json:"disable_parallel_tool_use,omitempty"`
}

type Tool struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	InputSchema schema.Schema `json:"input_schema"`
}
