# Custom Tools Guide

Create custom tools to extend your agent's capabilities. Dive offers two approaches: `FuncTool` for quick tool creation from a function, and `TypedTool[T]` for tools that need struct-based state.

## FuncTool (Recommended for Simple Tools)

`FuncTool` creates a tool from a function. The schema is auto-generated from the input type's struct tags — no manual `Schema()` method needed:

```go
type WeatherInput struct {
    City  string `json:"city" description:"City name"`
    Units string `json:"units,omitempty" description:"Temperature units" enum:"celsius,fahrenheit"`
}

weatherTool := dive.FuncTool("get_weather", "Get current weather for a city",
    func(ctx context.Context, input *WeatherInput) (*dive.ToolResult, error) {
        temp := fetchWeather(input.City, input.Units)
        return dive.NewToolResultText(fmt.Sprintf("%.1f°%s in %s", temp, input.Units, input.City)), nil
    },
)
```

The schema is derived from `WeatherInput` struct tags:
- `json:"city"` — required field (no `omitempty`)
- `json:"units,omitempty"` — optional field
- `description:"..."` — field description for the LLM
- `enum:"celsius,fahrenheit"` — constrained values

Other supported tags: `default`, `example`, `minimum`, `maximum`, `minLength`, `maxLength`, `pattern`, `format`, `nullable`, `required`.

### FuncTool Options

```go
// Set annotations
dive.FuncTool("tool", "desc", fn,
    dive.WithFuncToolAnnotations(&dive.ToolAnnotations{
        ReadOnlyHint:  true,
        OpenWorldHint: true,
    }),
)

// Override the auto-generated schema
dive.FuncTool("tool", "desc", fn,
    dive.WithFuncToolSchema(customSchema),
)
```

## TypedTool Interface

For tools that need struct-based state (DB connections, API clients, config), implement `TypedTool[T]`:

```go
type TypedTool[T any] interface {
    Name() string
    Description() string
    Schema() *dive.Schema
    Annotations() *dive.ToolAnnotations
    Call(ctx context.Context, input T) (*dive.ToolResult, error)
}
```

The type parameter `T` is the tool's input type. Dive automatically deserializes JSON from the LLM into `T`.

### Example: Lookup Tool

Tool structs can hold fields — DB clients, API clients, config — and use them in `Call`:

```go
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

### Registering with an Agent

Wrap with `dive.ToolAdapter` and pass to `AgentOptions.Tools`:

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

Note: Built-in tools in `toolkit/` already call `ToolAdapter` internally. `FuncTool` also wraps internally. You only need `ToolAdapter` for your own `TypedTool` implementations.

## Dynamic Tools with Toolsets

Use `Toolset` to provide tools that vary at runtime — MCP servers, permission-filtered tools, or context-dependent availability:

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Model: anthropic.New(),
    Toolsets: []dive.Toolset{
        &dive.ToolsetFunc{
            ToolsetName: "mcp-tools",
            Resolve: func(ctx context.Context) ([]dive.Tool, error) {
                return discoverMCPTools(ctx)
            },
        },
    },
})
```

`Toolset.Tools(ctx context.Context)` is called before each LLM request, returning `([]Tool, error)`, so the tool set can change between iterations. Tools from toolsets are merged with static `Tools`.

## Tool Annotations

Annotations help agents and permission systems understand tool behavior:

```go
func (t *MyTool) Annotations() *dive.ToolAnnotations {
    return &dive.ToolAnnotations{
        Title:              "My Tool",
        ReadOnlyHint:       false, // Modifies data
        DestructiveHint:    true,  // May overwrite/delete
        IdempotentHint:     false, // Results may vary
        OpenWorldHint:      true,  // Accesses external resources
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
    return dive.NewToolResultText("Success"), nil
}
```

Tool panics are automatically recovered and converted to error results — the LLM sees the error and can adapt. The stack trace is logged but not sent to the model.

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

1. **Use `FuncTool` for simple tools** — Less boilerplate, auto-generated schema
2. **Use `TypedTool[T]` when you need state** — DB connections, API clients, config
3. **Keep tools focused** — One tool, one purpose
4. **Write clear descriptions** — The LLM uses these to decide when to call your tool
5. **Set appropriate annotations** — Helps permission systems and agent reasoning
6. **Use `NewToolResultError`** for expected errors, `error` return for unexpected failures

For built-in tools, see the [Tools Guide](tools.md).
