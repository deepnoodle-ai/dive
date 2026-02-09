# Tool Interface Evolution Proposal

Comparative analysis of Dive, ADK-Go, and Codex tool systems, with concrete
proposals for strengthening Dive's approach.

## Current State Assessment

### What Dive Does Well

**Clean separation of typed and untyped interfaces.** The `Tool` / `TypedTool[T]`
/ `TypedToolAdapter[T]` stack is well-designed. Users implement `TypedTool[T]` for
type safety; the framework wraps it into a `Tool` automatically. ADK achieves
something similar but through internal interfaces that users can't implement
directly. Dive's approach is more open.

**Rich tool results.** `ToolResult` supports multi-modal content (text, image,
audio), display hints for human-facing output, and error signaling. ADK forces
everything through `map[string]any`. Codex has `ToolOutput` with success flags but
no multi-modal content. Dive's model is the most expressive.

**ToolAnnotations with behavioral hints.** `ReadOnlyHint`, `DestructiveHint`,
`IdempotentHint`, `OpenWorldHint`, `EditHint` — these are MCP-aligned and more
granular than Codex's binary `is_mutating()`. ADK has no equivalent.

**ToolPreviewer for HITL.** The preview system (`ToolCallPreview` with summary and
details) gives permission systems something human-readable to display before
execution. Neither ADK nor Codex has an equivalent; ADK's confirmation uses opaque
payloads and Codex displays raw command strings.

**Comprehensive hook system.** Seven hook types covering the full generation
lifecycle. ADK has six callbacks but they're less composable (first non-nil wins,
no chaining). Codex has no hook system — approval is baked into the tool handler
trait.

**Schema generation from Go structs.** `schema.Generate()` in wonton already
supports the full range of JSON Schema constraints via struct tags. Neither ADK nor
Codex has this — ADK uses a third-party `jsonschema` library, Codex hand-writes
Rust enums.

### Where Dive Falls Short

**1. Schema and input struct are disconnected.** `TypedTool[T]` declares
`Schema()` independently of `T`. Every toolkit tool manually writes its schema AND
defines an input struct. These invariably drift. ADK solved this with
`functiontool.New[TArgs, TResults]` which infers the schema from the type
parameter. Dive has the infrastructure (`schema.Generate`) but doesn't use it.

**2. Tools receive bare `context.Context`.** Tools have no access to the agent,
session state, call ID, or shared hook values. ADK gives tools a rich `Context`
with state access, side effects, memory search, and HITL confirmation. Codex gives
tools `ToolInvocation` with session, turn context, and diff tracking. Dive tools
are isolated functions that can't participate in the broader agent lifecycle.

**3. No dynamic tool resolution.** `AgentOptions.Tools` is a static `[]Tool` set
at construction. ADK has `Toolset` with `Tools(ctx) ([]Tool, error)` called at
each LLM request, enabling context-dependent tool availability. Codex has
`DynamicToolSpec` for runtime tool injection from external clients.

**4. No panic recovery.** ADK wraps tool execution in `defer/recover` and returns
errors with full stack traces. Codex is Rust (no panics). Dive does nothing — a
panicking tool crashes the process.

## Proposals

### 1. Auto-derive Schema for TypedTool (breaking)

**Problem:** Every `TypedTool[T]` implementor writes `Schema()` by hand, repeating
what the input struct already declares. This is ~20 lines of boilerplate per tool
that can silently drift from the actual input type.

**Change:** Remove `Schema()` from the `TypedTool[T]` interface. The
`TypedToolAdapter[T]` will auto-generate the schema from `T` using
`schema.Generate()` at construction time.

```go
// Before
type TypedTool[T any] interface {
    Name() string
    Description() string
    Schema() *Schema          // removed
    Annotations() *ToolAnnotations
    Call(ctx context.Context, input T) (*ToolResult, error)
}

// After
type TypedTool[T any] interface {
    Name() string
    Description() string
    Annotations() *ToolAnnotations
    Call(ctx context.Context, input T) (*ToolResult, error)
}
```

The adapter generates and caches the schema:

```go
func ToolAdapter[T any](tool TypedTool[T]) *TypedToolAdapter[T] {
    var zero T
    s, err := schema.Generate(zero)
    if err != nil {
        // Fall back: allow manual schema via SchemaOverrider interface
        panic(fmt.Sprintf("cannot generate schema for %T: %v", zero, err))
    }
    return &TypedToolAdapter[T]{tool: tool, schema: s}
}
```

For tools that need schema customization beyond what struct tags provide, add an
opt-in override interface:

```go
// SchemaProvider is an optional interface for TypedTools that need custom schemas
// beyond what struct tag generation provides. When implemented, this schema is
// used instead of auto-generation.
type SchemaProvider interface {
    Schema() *Schema
}
```

The adapter checks for this at construction:

```go
func ToolAdapter[T any](tool TypedTool[T]) *TypedToolAdapter[T] {
    a := &TypedToolAdapter[T]{tool: tool}
    if sp, ok := tool.(SchemaProvider); ok {
        a.schema = sp.Schema()
    } else {
        var zero T
        s, _ := schema.Generate(zero)
        a.schema = s
    }
    return a
}
```

**Impact on toolkit:** All 11 toolkit tools delete their `Schema()` methods and
add struct tags to their input types. For example:

```go
// Before
type ReadFileInput struct {
    FilePath string `json:"file_path"`
    Offset   int    `json:"offset,omitempty"`
    Limit    int    `json:"limit,omitempty"`
}

func (t *ReadFileTool) Schema() *schema.Schema {
    return &schema.Schema{
        Type:     "object",
        Required: []string{"file_path"},
        Properties: map[string]*schema.Property{
            "file_path": {Type: "string", Description: "The absolute path..."},
            "offset":    {Type: "integer", Description: "The line number..."},
            "limit":     {Type: "integer", Description: "The number of lines..."},
        },
    }
}

// After
type ReadFileInput struct {
    FilePath string `json:"file_path" description:"The absolute path to the file to read"`
    Offset   int    `json:"offset,omitempty" description:"The line number to start reading from (1-based)"`
    Limit    int    `json:"limit,omitempty" description:"The number of lines to read"`
}

// No Schema() method needed.
```

**Migration:** This is a breaking change to the `TypedTool[T]` interface. Users
implementing it directly must remove their `Schema()` method and add struct tags.
The untyped `Tool` interface retains `Schema()` — it has no type parameter to
infer from.

### 2. Introduce ToolContext (breaking)

**Problem:** Tools receive a bare `context.Context` and can't access the agent,
session, call identity, or communicate with hooks.

**Change:** Add a `ToolContext` type and update both `Tool` and `TypedTool[T]`:

```go
// ToolContext is passed to every tool invocation, providing access to
// agent state and inter-hook communication.
type ToolContext struct {
    context.Context

    // CallID is the unique identifier for this tool invocation.
    CallID string

    // Agent is the agent executing this tool. Access the agent's name,
    // tools, and other metadata. Nil in testing contexts.
    Agent *Agent

    // Values is the shared hook values map for the current generation.
    // Tools can read values set by PreToolUse hooks and write values
    // for PostToolUse hooks to consume.
    Values map[string]any
}
```

Update the interfaces:

```go
type Tool interface {
    Name() string
    Description() string
    Schema() *Schema
    Annotations() *ToolAnnotations
    Call(ctx *ToolContext, input any) (*ToolResult, error)
}

type TypedTool[T any] interface {
    Name() string
    Description() string
    Annotations() *ToolAnnotations
    Call(ctx *ToolContext, input T) (*ToolResult, error)
}
```

Also update the previewer interfaces:

```go
type ToolPreviewer interface {
    PreviewCall(ctx *ToolContext, input any) *ToolCallPreview
}

type TypedToolPreviewer[T any] interface {
    PreviewCall(ctx *ToolContext, input T) *ToolCallPreview
}
```

**Design notes:**

- `ToolContext` embeds `context.Context` so existing `ctx.Done()`,
  `ctx.Value()` etc. still work.
- `Values` is the same map from `HookContext.Values`, enabling tool-hook
  communication. A PreToolUse hook could set `Values["workspace_dir"]` and the
  tool could read it.
- `Agent` gives tools access to `agent.Name()`, `agent.Tools()`, etc. A sub-agent
  tool could use this to spawn the parent's agent.
- Keeping this a concrete struct (not interface) avoids the complexity of ADK's
  `tool.Context` interface with 9 methods.
- Unlike ADK, tools do NOT get direct state mutation (`State().Set()`). State
  changes flow through hooks and the session system. This is deliberate — tools
  should be pure functions of their input, with side effects managed externally.

**Migration:** Every `Tool` and `TypedTool[T]` implementation must update its
`Call` signature from `context.Context` to `*ToolContext`. The fix is mechanical:
rename the parameter and it's done, since `*ToolContext` embeds
`context.Context`.

### 3. Add Toolset Interface

**Problem:** Tools are static. Real agents need tools that appear/disappear based
on context — user permissions, loaded MCP servers, conversation state.

**Change:** Add a `Toolset` interface and support it on `AgentOptions`:

```go
// Toolset provides dynamic tool resolution. Tools() is called before each
// LLM request, allowing the available tools to vary based on runtime context.
type Toolset interface {
    // Name identifies this toolset for logging and debugging.
    Name() string

    // Tools returns the tools available in the current context.
    // Called before each LLM request. Implementations should be fast
    // (cache tool instances, don't re-create them on every call).
    Tools(ctx context.Context) ([]Tool, error)
}

type AgentOptions struct {
    // ...existing fields...
    Tools    []Tool
    Toolsets []Toolset  // NEW
}
```

At each LLM request, the agent resolves tools:

```go
func (a *Agent) resolveTools(ctx context.Context) ([]Tool, error) {
    tools := slices.Clone(a.tools) // static tools
    for _, ts := range a.toolsets {
        dynamic, err := ts.Tools(ctx)
        if err != nil {
            return nil, fmt.Errorf("toolset %s: %w", ts.Name(), err)
        }
        tools = append(tools, dynamic...)
    }
    return tools, nil
}
```

**Convenience: `ToolsetFunc`**

```go
// ToolsetFunc adapts a function into a Toolset.
type ToolsetFunc struct {
    ToolsetName string
    Resolve     func(ctx context.Context) ([]Tool, error)
}

func (f *ToolsetFunc) Name() string { return f.ToolsetName }
func (f *ToolsetFunc) Tools(ctx context.Context) ([]Tool, error) {
    return f.Resolve(ctx)
}
```

**Use cases:**
- MCP servers: tools discovered at runtime
- Permission-filtered tools: different tools for different users
- Context-dependent tools: "upload" tool only available after "login"

### 4. Add FuncTool Builder

**Problem:** Creating a typed tool requires implementing a full struct with 4-5
methods. For simple tools — especially user-defined ones — this is too much
ceremony.

**Change:** Add a functional tool builder:

```go
// FuncTool creates a Tool from a function. The schema is auto-generated from
// the input type T. Use struct tags on T to control descriptions, constraints,
// and required fields.
func FuncTool[T any](name, description string, fn func(ctx *ToolContext, input T) (*ToolResult, error), opts ...FuncToolOption) Tool {
    ft := &funcTool[T]{
        name:        name,
        description: description,
        fn:          fn,
    }
    for _, opt := range opts {
        opt.apply(ft)
    }
    return ToolAdapter(ft)
}

// FuncToolOption configures a FuncTool.
type FuncToolOption interface {
    apply(ft any)
}

// WithAnnotations sets annotations on a FuncTool.
func WithAnnotations(a *ToolAnnotations) FuncToolOption { ... }
```

Usage:

```go
type WeatherInput struct {
    City    string `json:"city" description:"City name"`
    Units   string `json:"units,omitempty" description:"Temperature units" enum:"celsius,fahrenheit" default:"celsius"`
}

weatherTool := dive.FuncTool("get_weather", "Get current weather for a city",
    func(ctx *dive.ToolContext, input *WeatherInput) (*dive.ToolResult, error) {
        temp := fetchWeather(input.City, input.Units)
        return dive.NewToolResultText(fmt.Sprintf("Temperature: %d%s", temp, input.Units)), nil
    },
    dive.WithAnnotations(&dive.ToolAnnotations{ReadOnlyHint: true}),
)

agent, _ := dive.NewAgent(dive.AgentOptions{
    Tools: []dive.Tool{weatherTool},
    // ...
})
```

This is the "one-liner tool" story that ADK has with `functiontool.New` and that
Dive currently lacks. The schema, type conversion, and interface implementation are
all handled automatically.

### 5. Add Panic Recovery

**Problem:** A panicking tool crashes the entire process. In production, tools run
user-provided code, invoke external processes, and do I/O — panics are inevitable.

**Change:** Wrap tool execution in the agent's loop:

```go
func (a *Agent) callTool(ctx *ToolContext, tool Tool, input any) (result *ToolResult, err error) {
    defer func() {
        if r := recover(); r != nil {
            result = NewToolResultError(fmt.Sprintf("tool %s panicked: %v", tool.Name(), r))
            err = nil // don't propagate — let the LLM see the error and adapt
            if a.logger != nil {
                a.logger.Error("tool panic",
                    "tool", tool.Name(),
                    "panic", r,
                    "stack", string(debug.Stack()),
                )
            }
        }
    }()
    return tool.Call(ctx, input)
}
```

Panics become tool errors that the LLM can see and respond to, rather than
process-terminating crashes. The stack trace is logged but not sent to the LLM.

## What We Deliberately Skip

**Uniform result type (ADK: `map[string]any`).** Dive's `ToolResult` with typed
content blocks is strictly better. Don't regress.

**Virtual confirmation function calls (ADK).** Dive's `ToolPreviewer` +
`PreToolUseHook` + permission package is cleaner. The confirmation doesn't pollute
the conversation history.

**FreeformTool (Codex).** Non-JSON tool input (diff syntax, etc.) is niche.
Support it if needed via the untyped `Tool` interface, which already accepts `any`.

**Tool-level state mutation (ADK: `ctx.Actions().StateDelta`).** Keep tools as
pure functions. Side effects flow through hooks and sessions. This is a
deliberate philosophical choice that keeps tool implementations testable and
composable.

**Output schema (ADK).** Low value — LLMs don't use tool output schemas for
validation, and tool output varies too much to schema-ify usefully.

## Migration Path

**Phase 1 — Additive (non-breaking):**
- Add `ToolContext` as a new type
- Add `Toolset` interface and `AgentOptions.Toolsets`
- Add `FuncTool` builder
- Add panic recovery in the agent loop
- Add `SchemaProvider` optional interface

**Phase 2 — Breaking changes (major version):**
- Remove `Schema()` from `TypedTool[T]`
- Change `Call(context.Context, ...)` to `Call(*ToolContext, ...)` on both
  `Tool` and `TypedTool[T]`
- Update `ToolPreviewer` / `TypedToolPreviewer[T]` signatures
- Migrate all toolkit tools to struct-tag-based schemas

**Phase 3 — Toolkit migration:**
- Convert all 11 toolkit tools to use struct tags instead of manual schemas

## Summary

| Change | Breaking | Impact | Complexity |
|--------|----------|--------|------------|
| Auto-derive schema | Yes | High — eliminates boilerplate, prevents drift | Medium |
| ToolContext | Yes | High — enables tool-agent communication | Low |
| Toolset | No | High — enables dynamic tool resolution | Low |
| FuncTool builder | No | High — dramatically simplifies tool creation | Low |
| Panic recovery | No | Medium — production safety | Trivial |

The two breaking changes (auto-derive schema, ToolContext) are the most impactful
and should ship together in a major version bump. The mechanical migration is
straightforward: delete `Schema()` methods, add struct tags, change `context.Context`
to `*ToolContext` in call signatures.
