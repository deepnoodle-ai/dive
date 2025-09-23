# Custom Tools Guide

Create custom tools to extend your agents' capabilities beyond the built-in toolkit.

## Core Concepts

Tools are functions that agents can call to perform specific actions:

- **Deterministic**: Predictable and repeatable results
- **Focused**: Single, well-defined purpose
- **Robust**: Handle errors gracefully
- **Documented**: Clear descriptions for LLM usage

## TypedTool Interface

All custom tools implement the `TypedTool` interface:

```go
type TypedTool[T any] interface {
    Name() string
    Description() string
    Schema() *schema.Schema
    Annotations() *ToolAnnotations
    Call(ctx context.Context, input *T) (*ToolResult, error)
}
```

## Simple Tool Example

```go
import (
    "context"
    "fmt"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/schema"
)

type CalculatorTool struct{}

type CalculatorInput struct {
    Expression string `json:"expression" description:"Mathematical expression to evaluate"`
}

func (t *CalculatorTool) Name() string {
    return "calculator"
}

func (t *CalculatorTool) Description() string {
    return "Evaluate mathematical expressions"
}

func (t *CalculatorTool) Schema() *schema.Schema {
    return &schema.Schema{
        Type:     "object",
        Required: []string{"expression"},
        Properties: map[string]*schema.Property{
            "expression": {
                Type:        "string",
                Description: "Mathematical expression to evaluate",
            },
        },
    }
}

func (t *CalculatorTool) Annotations() *dive.ToolAnnotations {
    return &dive.ToolAnnotations{
        Title:          "Calculator",
        ReadOnlyHint:   true,
        IdempotentHint: true,
    }
}

func (t *CalculatorTool) Call(ctx context.Context, input *CalculatorInput) (*dive.ToolResult, error) {
    result, err := evaluateExpression(input.Expression)
    if err != nil {
        return nil, fmt.Errorf("calculation failed: %w", err)
    }

    return dive.NewToolResultText(fmt.Sprintf("Result: %f", result)), nil
}
```

## Using Custom Tools

```go
import (
    "github.com/deepnoodle-ai/dive"
)

// Create and register the tool
calc := &CalculatorTool{}
agent, err := dive.NewAgent(dive.AgentOptions{
    Name: "Math Assistant",
    Tools: []dive.Tool{
        dive.ToolAdapter(calc),
    },
})
```

## Tool Annotations

Provide hints about tool behavior:

```go
type ToolAnnotations struct {
    Title           string         // Human-readable name
    ReadOnlyHint    bool           // Only reads, doesn't modify
    DestructiveHint bool           // May delete or overwrite data
    IdempotentHint  bool           // Safe to call multiple times
    OpenWorldHint   bool           // Accesses external resources
    Extra           map[string]any // Additional custom annotations
}
```

## Error Handling

Always provide clear error messages:

```go
func (t *MyTool) Call(ctx context.Context, input *MyInput) (*dive.ToolResult, error) {
    if input.Value == "" {
        return nil, fmt.Errorf("value is required")
    }

    // Tool logic here

    return dive.NewToolResultText("Success"), nil
}
```

## Best Practices

1. **Keep tools focused** - One tool, one purpose
2. **Validate input** - Check parameters before processing
3. **Handle errors gracefully** - Provide helpful error messages
4. **Use appropriate annotations** - Help agents understand tool behavior
5. **Test thoroughly** - Ensure tools work reliably
6. **Document clearly** - Write descriptions that help LLMs use tools effectively

For built-in tools, see the [Built-in Tools Guide](tools.md).
