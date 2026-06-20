package anthropic

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
)

const (
	CacheControlTypeEphemeral = "ephemeral"
)

// SystemBlock is a single system-prompt content block. The Anthropic API
// accepts the system prompt either as a plain string or as an array of text
// blocks; the array form is required to attach a cache_control breakpoint to
// the system prompt so the tools+system prefix caches independently of the
// moving message tail.
type SystemBlock struct {
	Type         string            `json:"type"`
	Text         string            `json:"text"`
	CacheControl *llm.CacheControl `json:"cache_control,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type Thinking struct {
	// Type is "adaptive" (recommended for Opus 4.6+ and Sonnet 4.6, and the
	// only supported mode on Opus 4.7/4.8), "enabled" (manual budget mode), or
	// "disabled".
	Type string `json:"type"`
	// BudgetTokens is only used with Type "enabled".
	BudgetTokens int `json:"budget_tokens,omitempty"`
	// Display controls how thinking content is returned: "summarized" or
	// "omitted". Defaults vary by model (omitted on Opus 4.7/4.8).
	Display string `json:"display,omitempty"`
}

// OutputConfig carries the effort parameter, which controls how eagerly the
// model spends tokens (thinking, tool calls, and text). Supported on Opus 4.5+
// and Sonnet 4.6 with no beta header required.
type OutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type Request struct {
	Model       string         `json:"model"`
	Messages    []*llm.Message `json:"messages"`
	MaxTokens   *int           `json:"max_tokens,omitempty"`
	Temperature *float64       `json:"temperature,omitempty"`
	System      []*SystemBlock `json:"system,omitempty"`
	// CacheControl, when set, enables Anthropic automatic prompt caching: the
	// API places (and advances) a cache breakpoint on the moving conversation
	// tail, consuming one of the 4 available breakpoint slots. Not supported on
	// Bedrock or Vertex; the provider falls back to an explicit tail breakpoint
	// there.
	CacheControl      *llm.CacheControl            `json:"cache_control,omitempty"`
	Stream            bool                         `json:"stream,omitempty"`
	Speed             string                       `json:"speed,omitempty"`
	Tools             []map[string]any             `json:"tools,omitempty"`
	ToolChoice        *ToolChoice                  `json:"tool_choice,omitempty"`
	Thinking          *Thinking                    `json:"thinking,omitempty"`
	OutputConfig      *OutputConfig                `json:"output_config,omitempty"`
	MCPServers        []llm.MCPServerConfig        `json:"mcp_servers,omitempty"`
	ContextManagement *llm.ContextManagementConfig `json:"context_management,omitempty"`
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
