package anthropic

import (
	"encoding/json"

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
	// Display controls how thinking content is returned: "summarized",
	// "omitted", or "updates" (beta). Defaults vary by model (omitted on Opus
	// 4.7 and later).
	Display string `json:"display,omitempty"`
	// BlockBinding sets what the API does with a replayed thinking block that
	// no longer matches its conversation (beta).
	BlockBinding *BlockBinding `json:"block_binding,omitempty"`
}

// BlockBinding is the thinking.block_binding object of the
// thinking-binding-controls beta.
type BlockBinding struct {
	PrefixMismatchBehavior PrefixMismatchBehavior `json:"prefix_mismatch_behavior"`
}

// PrefixMismatchBehavior sets what the API does with a replayed thinking block
// when something before it (the system prompt, tools, or an earlier message)
// has changed since the block was created.
type PrefixMismatchBehavior string

const (
	// PrefixMismatchError rejects the request with a 400 naming the first
	// failing block. It is the API default for accounts created on or after
	// 2026-08-31; setting it opts an older account into the same check.
	PrefixMismatchError PrefixMismatchBehavior = "error"
	// PrefixMismatchDropBlock drops each failing block and every thinking
	// block after it, and the request succeeds. Dropped blocks are listed in
	// llm.Response.InputTransformations.
	PrefixMismatchDropBlock PrefixMismatchBehavior = "drop_block"
)

// OutputConfig carries the effort parameter, which controls how eagerly the
// model spends tokens (thinking, tool calls, and text). Supported on Opus 4.5+
// and Sonnet 4.6 with no beta header required.
type OutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

// requestMessage is a message in wire form. It adds output_config, which only
// an effort message (llm.NewEffortMessage) carries.
type requestMessage struct {
	Role         llm.Role      `json:"role"`
	Content      []llm.Content `json:"content"`
	OutputConfig *OutputConfig `json:"output_config,omitempty"`
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

// MarshalJSON writes an effort message's level as output_config.effort, the
// per-message effort beta's wire form. Every other message encodes as usual.
func (r Request) MarshalJSON() ([]byte, error) {
	type request Request
	messages := make([]any, len(r.Messages))
	for i, message := range r.Messages {
		if message.Effort == "" {
			messages[i] = message
			continue
		}
		content := message.Content
		if content == nil {
			content = []llm.Content{}
		}
		messages[i] = requestMessage{
			Role:         message.Role,
			Content:      content,
			OutputConfig: &OutputConfig{Effort: string(message.Effort)},
		}
	}
	return json.Marshal(struct {
		request
		Messages []any `json:"messages"`
	}{request(r), messages})
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
