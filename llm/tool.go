package llm

import (
	"github.com/diveagents/dive/schema"
)

// ToolResult contains the result of a tool call
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Output    string `json:"output,omitempty"`
	Error     error  `json:"error,omitempty"`
}

// ToolChoice influences the behavior of the LLM when choosing which tool to use.
type ToolChoice string

// IsValid returns true if the ToolChoice is a known, valid value.
func (t ToolChoice) IsValid() bool {
	return t == ToolChoiceAuto ||
		t == ToolChoiceAny ||
		t == ToolChoiceNone ||
		t == ToolChoiceTool
}

const (
	ToolChoiceAuto ToolChoice = "auto"
	ToolChoiceAny  ToolChoice = "any"
	ToolChoiceNone ToolChoice = "none"
	ToolChoiceTool ToolChoice = "tool"
)

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
