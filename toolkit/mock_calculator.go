package toolkit

import (
	"context"

	"github.com/getstingrai/dive/llm"
)

type MockCalculatorTool struct {
	Result string
	Error  error
	Input  string
}

func (t *MockCalculatorTool) Definition() *llm.ToolDefinition {
	return &llm.ToolDefinition{
		Name:        "Calculator",
		Description: "Performs basic arithmetic calculations",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"expression"},
			Properties: map[string]*llm.SchemaProperty{
				"expression": {
					Type:        "string",
					Description: "The arithmetic expression to evaluate (e.g., '2 + 2')",
				},
			},
		},
	}
}

func (t *MockCalculatorTool) Call(ctx context.Context, input string) (string, error) {
	t.Input = input
	return t.Result, t.Error
}

func (t *MockCalculatorTool) ShouldReturnResult() bool {
	return true
}
