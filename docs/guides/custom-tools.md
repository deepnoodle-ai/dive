# Custom Tools Guide

Create custom tools to extend your agent's capabilities.

## TypedTool Interface

Custom tools implement the `TypedTool[T]` generic interface:

```go
// In package dive:
type TypedTool[T any] interface {
    Name() string
    Description() string
    Schema() *dive.Schema
    Annotations() *dive.ToolAnnotations
    Call(ctx context.Context, input T) (*dive.ToolResult, error)
}
```

The type parameter `T` is the tool's input type. Dive automatically deserializes JSON from the LLM into `T`.

## Example: Lookup Tool

Tool structs can hold fields — DB clients, API clients, config — and use them in `Call`. This is the primary way to integrate tools with external systems.

```go
package main

import (
    "context"
    "database/sql"
    "fmt"

    "github.com/deepnoodle-ai/dive"
)

type LookupTool struct {
    DB *sql.DB
}

type LookupInput struct {
    Name string `json:"name"`
}

func (t *LookupTool) Name() string        { return "lookup_employee" }
func (t *LookupTool) Description() string { return "Look up an employee by name" }

func (t *LookupTool) Schema() *dive.Schema {
    return &dive.Schema{
        Type:     "object",
        Required: []string{"name"},
        Properties: map[string]*dive.SchemaProperty{
            "name": {
                Type:        "string",
                Description: "Employee name to look up",
            },
        },
    }
}

func (t *LookupTool) Annotations() *dive.ToolAnnotations {
    return &dive.ToolAnnotations{
        Title:        "Employee Lookup",
        ReadOnlyHint: true,
    }
}

func (t *LookupTool) Call(ctx context.Context, input LookupInput) (*dive.ToolResult, error) {
    var role string
    err := t.DB.QueryRowContext(ctx,
        "SELECT role FROM employees WHERE name = ?", input.Name,
    ).Scan(&role)
    if err != nil {
        return dive.NewToolResultError(fmt.Sprintf("employee not found: %s", input.Name)), nil
    }
    return dive.NewToolResultText(fmt.Sprintf("%s is a %s", input.Name, role)), nil
}
```

## Registering with an Agent

Wrap with `dive.ToolAdapter` and pass to `AgentOptions.Tools`. Inject dependencies when constructing the tool:

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:         "HR Assistant",
    SystemPrompt: "You help answer questions about employees.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        dive.ToolAdapter(&LookupTool{DB: db}),
    },
})
```

Note: Built-in tools in `toolkit/` already call `ToolAdapter` internally, so their constructors return a `dive.Tool` directly. You only need `ToolAdapter` for your own `TypedTool` implementations.

This pattern works the same way for any dependency: HTTP clients, gRPC connections, caches, third-party SDKs, or configuration values. Store it as a field on the tool struct and use it in `Call`.

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
