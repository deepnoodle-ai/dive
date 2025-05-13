package toolkit

import (
	"context"

	"github.com/diveagents/dive/llm"
)

var _ llm.ToolWithMetadata = &MockCalculatorTool{}

type MockCalculatorTool struct {
	Result string
	Error  error
	Input  string
}

func (t *MockCalculatorTool) Name() string {
	return "Calculator"
}

func (t *MockCalculatorTool) Description() string {
	return "Performs basic arithmetic calculations"
}

func (t *MockCalculatorTool) Schema() llm.Schema {
	return llm.Schema{
		Type:     "object",
		Required: []string{"expression"},
		Properties: map[string]*llm.SchemaProperty{
			"expression": {
				Type:        "string",
				Description: "The arithmetic expression to evaluate (e.g., '2 + 2')",
			},
		},
	}
}

func (t *MockCalculatorTool) Call(ctx context.Context, input *llm.ToolCallInput) (*llm.ToolCallOutput, error) {
	t.Input = input.Input
	return llm.NewToolCallOutput(t.Result), t.Error
}

func (t *MockCalculatorTool) Metadata() llm.ToolMetadata {
	return llm.ToolMetadata{
		Version:    "0.0.1",
		Capability: llm.ToolCapabilityReadOnly,
	}
}
