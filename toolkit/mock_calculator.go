package toolkit

import (
	"context"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/schema"
)

var _ dive.TypedTool[*MockCalculatorInput] = &MockCalculatorTool{}

type MockCalculatorInput struct {
	Expression string `json:"expression"`
}

type MockCalculatorTool struct {
	Result        string
	Error         error
	CapturedInput *MockCalculatorInput
}

func (t *MockCalculatorTool) Name() string {
	return "Calculator"
}

func (t *MockCalculatorTool) Description() string {
	return "Performs basic arithmetic calculations"
}

func (t *MockCalculatorTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"expression"},
		Properties: map[string]*schema.Property{
			"expression": {
				Type:        "string",
				Description: "The arithmetic expression to evaluate (e.g., '2 + 2')",
			},
		},
	}
}

func (t *MockCalculatorTool) Call(ctx context.Context, input *MockCalculatorInput) (*dive.ToolResult, error) {
	t.CapturedInput = input
	return dive.NewToolResultText(t.Result), t.Error
}

func (t *MockCalculatorTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Calculator",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}
