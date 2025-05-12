package llm

import (
	"context"
	"time"
)

// ToolResult contains the input and output of a tool call
type ToolResult struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Input       string     `json:"input,omitempty"`
	Output      string     `json:"output,omitempty"`
	Error       error      `json:"error,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// Schema describes the structure of the parameters used to call a tool.
type Schema struct {
	Type       string                     `json:"type"`
	Properties map[string]*SchemaProperty `json:"properties"`
	Required   []string                   `json:"required,omitempty"`
}

// SchemaProperty describes a property of a schema.
type SchemaProperty struct {
	Type        string                     `json:"type"`
	Description string                     `json:"description"`
	Enum        []string                   `json:"enum,omitempty"`
	Items       *SchemaProperty            `json:"items,omitempty"`
	Required    []string                   `json:"required,omitempty"`
	Properties  map[string]*SchemaProperty `json:"properties,omitempty"`
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

// ToolCallInput is the input for a tool call.
type ToolCallInput struct {
	Input     string
	Confirmer Confirmer
}

// ToolCallOutput is the output from a tool call.
type ToolCallOutput struct {
	Output  string
	Summary string
}

// WithSummary sets the summary of the ToolCallOutput.
func (o *ToolCallOutput) WithSummary(summary string) *ToolCallOutput {
	o.Summary = summary
	return o
}

// Tool is an interface for a tool that can be called by an LLM.
type Tool interface {
	// Name of the tool.
	Name() string

	// Description of the tool.
	Description() string

	// Schema describes the parameters used to call the tool.
	Schema() Schema

	// Call is the function that is called to use the tool.
	Call(ctx context.Context, input *ToolCallInput) (*ToolCallOutput, error)
}

// ToolCapability indicates whether a tool is read-only or not.
type ToolCapability string

const (
	ToolCapabilityReadOnly  ToolCapability = "read-only"
	ToolCapabilityReadWrite ToolCapability = "read-write"
)

// ToolMetadata is a struct that contains metadata about a tool.
type ToolMetadata struct {
	Version    string
	Capability ToolCapability
}

func (m *ToolMetadata) IsReadOnly() bool {
	return m.Capability == ToolCapabilityReadOnly
}

// ToolWithMetadata is an interface that extends the Tool interface with metadata.
type ToolWithMetadata interface {
	Tool

	// Metadata returns metadata about the tool.
	Metadata() ToolMetadata
}
