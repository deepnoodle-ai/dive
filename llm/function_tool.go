package llm

import "context"

// ToolFunc is the function signature for a tool call.
type ToolFunc func(ctx context.Context, input *ToolCallInput) (*ToolCallOutput, error)

// FunctionTool is a tool that is defined by a function.
type FunctionTool struct {
	fn          ToolFunc
	name        string
	description string
	schema      Schema
}

// NewFunctionTool creates a new FunctionTool.
func NewFunctionTool(fn ToolFunc) *FunctionTool {
	return &FunctionTool{fn: fn}
}

func (t *FunctionTool) WithName(name string) *FunctionTool {
	t.name = name
	return t
}

func (t *FunctionTool) WithDescription(description string) *FunctionTool {
	t.description = description
	return t
}

func (t *FunctionTool) WithSchema(schema Schema) *FunctionTool {
	t.schema = schema
	return t
}

func (t *FunctionTool) Name() string {
	return t.name
}

func (t *FunctionTool) Description() string {
	return t.description
}

func (t *FunctionTool) Schema() Schema {
	return t.schema
}

func (t *FunctionTool) Call(ctx context.Context, input *ToolCallInput) (*ToolCallOutput, error) {
	return t.fn(ctx, input)
}

// NewToolCallOutput creates a new ToolCallOutput with the given output.
func NewToolCallOutput(output string) *ToolCallOutput {
	return &ToolCallOutput{Output: output}
}
