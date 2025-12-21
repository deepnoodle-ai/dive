package llm

import (
	"github.com/deepnoodle-ai/wonton/schema"
)

// ToolResult contains the result of a tool call
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Output    string `json:"output,omitempty"`
	Error     error  `json:"error,omitempty"`
}

// ToolChoiceType is used to guide the LLM's choice of which tool to use.
type ToolChoiceType string

const (
	ToolChoiceTypeAuto ToolChoiceType = "auto"
	ToolChoiceTypeAny  ToolChoiceType = "any"
	ToolChoiceTypeTool ToolChoiceType = "tool"
	ToolChoiceTypeNone ToolChoiceType = "none"
)

// IsValid returns true if the ToolChoiceType is a known, valid value.
func (t ToolChoiceType) IsValid() bool {
	return t == ToolChoiceTypeAuto ||
		t == ToolChoiceTypeAny ||
		t == ToolChoiceTypeTool ||
		t == ToolChoiceTypeNone
}

// ToolChoiceAuto is a ToolChoice with type "auto".
var ToolChoiceAuto = &ToolChoice{Type: ToolChoiceTypeAuto}

// ToolChoiceAny is a ToolChoice with type "any".
var ToolChoiceAny = &ToolChoice{Type: ToolChoiceTypeAny}

// ToolChoiceNone is a ToolChoice with type "none".
var ToolChoiceNone = &ToolChoice{Type: ToolChoiceTypeNone}

// ToolChoice influences the behavior of the LLM when choosing which tool to use.
type ToolChoice struct {
	Type ToolChoiceType `json:"type"`
	Name string         `json:"name,omitempty"`
}

// Tool is an interface that defines a tool that can be called by an LLM.
type Tool interface {
	// Name of the tool.
	Name() string

	// Description of the tool.
	Description() string

	// Schema describes the parameters used to call the tool.
	Schema() *schema.Schema
}

// ToolConfiguration is an interface that may be implemented by a Tool to
// provide explicit JSON configuration to pass to the LLM provider.
type ToolConfiguration interface {

	// ToolConfiguration returns a map of configuration for the tool, when used
	// with the given provider.
	ToolConfiguration(providerName string) map[string]any
}

// NewToolDefinition creates a new ToolDefinition.
func NewToolDefinition() *ToolDefinition {
	return &ToolDefinition{}
}

// ToolDefinition is a concrete implementation of the Tool interface. Note this
// does not provide a mechanism for calling the tool, but only for describing
// what the tool does so the LLM can understand it. You might not use this
// implementation if you use a full dive.Tool implementation in your app.
type ToolDefinition struct {
	name        string
	description string
	schema      *schema.Schema
}

// Name returns the name of the tool, per the Tool interface.
func (t *ToolDefinition) Name() string {
	return t.name
}

// Description returns the description of the tool, per the Tool interface.
func (t *ToolDefinition) Description() string {
	return t.description
}

// Schema returns the schema of the tool, per the Tool interface.
func (t *ToolDefinition) Schema() *schema.Schema {
	return t.schema
}

// WithName sets the name of the tool.
func (t *ToolDefinition) WithName(name string) *ToolDefinition {
	t.name = name
	return t
}

// WithDescription sets the description of the tool.
func (t *ToolDefinition) WithDescription(description string) *ToolDefinition {
	t.description = description
	return t
}

// WithSchema sets the schema of the tool.
func (t *ToolDefinition) WithSchema(schema *schema.Schema) *ToolDefinition {
	t.schema = schema
	return t
}
