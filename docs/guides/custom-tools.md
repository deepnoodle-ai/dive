# Custom Tools Guide

Create custom tools to extend your agent's capabilities.

## TypedTool Interface

Custom tools implement the `TypedTool[T]` generic interface:

```go
type TypedTool[T any] interface {
    Name() string
    Description() string
    Schema() *Schema
    Annotations() *ToolAnnotations
    Call(ctx context.Context, input T) (*ToolResult, error)
}
```

The type parameter `T` is the tool's input type. Dive automatically deserializes JSON from the LLM into `T`.

## Example: Calculator Tool

```go
package main

import (
    "context"
    "fmt"

    "github.com/deepnoodle-ai/dive"
)

type CalculatorTool struct{}

type CalculatorInput struct {
    Expression string `json:"expression" description:"Mathematical expression to evaluate"`
}

func (t *CalculatorTool) Name() string        { return "calculator" }
func (t *CalculatorTool) Description() string { return "Evaluate mathematical expressions" }

func (t *CalculatorTool) Schema() *dive.Schema {
    return &dive.Schema{
        Type:     "object",
        Required: []string{"expression"},
        Properties: map[string]*dive.SchemaProperty{
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

func (t *CalculatorTool) Call(ctx context.Context, input CalculatorInput) (*dive.ToolResult, error) {
    result, err := evaluateExpression(input.Expression)
    if err != nil {
        return nil, fmt.Errorf("calculation failed: %w", err)
    }
    return dive.NewToolResultText(fmt.Sprintf("Result: %f", result)), nil
}
```

## Registering with an Agent

Wrap with `dive.ToolAdapter` and pass to `AgentOptions.Tools`:

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:         "Math Assistant",
    SystemPrompt: "You are a math assistant.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        dive.ToolAdapter(&CalculatorTool{}),
    },
})
```

Note: Built-in tools in `toolkit/` already call `ToolAdapter` internally, so their constructors return a `dive.Tool` directly. You only need `ToolAdapter` for your own `TypedTool` implementations.

## Tool Annotations

Annotations help agents and permission systems understand tool behavior:

```go
func (t *MyTool) Annotations() *dive.ToolAnnotations {
    return &dive.ToolAnnotations{
        Title:           "My Tool",
        ReadOnlyHint:    false,          // Modifies data
        DestructiveHint: true,           // May overwrite/delete
        IdempotentHint:  false,          // Results may vary
        OpenWorldHint:   true,           // Accesses external resources
    }
}
```

## Error Handling

Return errors for unexpected failures. For expected "no result" cases, return a `ToolResult` with an error message:

```go
func (t *MyTool) Call(ctx context.Context, input MyInput) (*dive.ToolResult, error) {
    if input.Value == "" {
        return dive.NewToolResultError("value is required"), nil
    }
    // ...
    return dive.NewToolResultText("Success"), nil
}
```

## Tool Previews

Implement `TypedToolPreviewer[T]` to provide human-readable previews before execution:

```go
func (t *MyTool) PreviewCall(ctx context.Context, input MyInput) *dive.ToolCallPreview {
    return &dive.ToolCallPreview{
        Summary: fmt.Sprintf("Process %s", input.Value),
    }
}
```

## Best Practices

1. **Keep tools focused** - One tool, one purpose
2. **Validate input** - Check parameters before processing
3. **Write clear descriptions** - The LLM uses these to decide when to call your tool
4. **Set appropriate annotations** - Helps permission systems and agent reasoning
5. **Use `NewToolResultError`** for expected errors, `error` return for unexpected failures

For built-in tools, see the [Tools Guide](tools.md).
