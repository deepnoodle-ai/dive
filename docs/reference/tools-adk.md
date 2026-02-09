# ADK-Go Tool System Reference

Reference documentation for the tool system in Google's [Agent Development Kit (ADK) for Go](https://github.com/google/adk-go). Captures the interfaces, schema handling, marshaling, and execution patterns worth considering for Dive.

## Architecture Overview

The tool system has four layers:

1. **Public interfaces** (`tool` package) - `Tool`, `Context`, `Toolset`, `Predicate`
2. **Internal interfaces** (`toolinternal` package) - `FunctionTool`, `RequestProcessor`
3. **Tool implementations** - `functiontool`, `agenttool`, `geminitool`, `mcptoolset`, etc.
4. **Execution engine** (`llminternal.Flow`) - Request processors, tool invocation loop, callbacks

The public `Tool` interface is intentionally minimal. The heavier `FunctionTool` and `RequestProcessor` interfaces are internal, keeping user-facing tool creation simple while giving the framework what it needs to wire tools into the LLM request/response loop.

## Public Interfaces

### Tool

```go
// tool/tool.go
type Tool interface {
    Name() string
    Description() string
    IsLongRunning() bool
}
```

Three methods, all read-only. `Name()` is what the LLM uses in function calls. `Description()` helps the LLM decide when to invoke the tool. `IsLongRunning()` signals that the tool returns an intermediate resource ID and finishes asynchronously.

This is just a marker interface. The actual callable behavior comes from the internal `FunctionTool` interface (see below).

### Context

```go
// tool/tool.go
type Context interface {
    agent.CallbackContext  // State(), Branch(), AgentName(), UserID(), etc.

    FunctionCallID() string
    Actions() *session.EventActions
    SearchMemory(context.Context, string) (*memory.SearchResponse, error)
    ToolConfirmation() *toolconfirmation.ToolConfirmation
    RequestConfirmation(hint string, payload any) error
}
```

Every tool invocation receives a `Context`. It provides:

- **Identity** - agent name, user ID, session ID, invocation ID, branch (via embedded `CallbackContext`)
- **State access** - read/write session state (via `State()`)
- **Side effects** - `Actions()` returns the `EventActions` for this event, letting tools set state deltas, artifact deltas, transfer signals, and escalation flags
- **Memory** - `SearchMemory()` for semantic search across past sessions
- **HITL confirmation** - `ToolConfirmation()` checks if a prior confirmation exists; `RequestConfirmation()` initiates a new approval request

The internal implementation (`toolinternal.toolContext`) wraps an `agent.InvocationContext` and injects a unique `FunctionCallID` (UUID) and a fresh `EventActions` per tool call.

### Toolset

```go
// tool/tool.go
type Toolset interface {
    Name() string
    Tools(ctx agent.ReadonlyContext) ([]Tool, error)
}
```

Toolsets provide dynamic tool resolution. `Tools()` is called at LLM request time, so the available tools can vary based on session state, user identity, or any other runtime context.

### Predicate & FilterToolset

```go
type Predicate func(ctx agent.ReadonlyContext, tool Tool) bool

func StringPredicate(allowedTools []string) Predicate  // whitelist by name
func FilterToolset(toolset Toolset, predicate Predicate) Toolset
```

Composable filtering for toolsets. `FilterToolset` wraps a toolset and evaluates the predicate against each tool at resolution time.

## Internal Interfaces

These are in `internal/toolinternal/` and not exported. Any tool that wants to be callable by the LLM must implement both.

### FunctionTool

```go
type FunctionTool interface {
    tool.Tool
    Declaration() *genai.FunctionDeclaration
    Run(ctx tool.Context, args any) (result map[string]any, err error)
}
```

- `Declaration()` returns the schema that gets sent to the LLM
- `Run()` receives raw `args` (always `map[string]any` from JSON) and must return `map[string]any`

The `Run` return type is always `map[string]any`. If a tool's natural return type isn't a map, the framework wraps it: `{"result": value}`.

### RequestProcessor

```go
type RequestProcessor interface {
    ProcessRequest(ctx tool.Context, req *model.LLMRequest) error
}
```

Called during request building. Each tool gets a chance to modify the `LLMRequest` before it's sent to the LLM. Most tools use this to call `toolutils.PackTool(req, self)` which adds their `Declaration()` to the request's function declarations list. Some tools inject additional system instructions (e.g., `loadMemoryTool` appends memory-related instructions).

## Schema Definition

Schemas flow through two separate paths depending on tool type:

### Path 1: JSON Schema (functiontool)

The `functiontool` package uses `github.com/google/jsonschema-go/jsonschema` for automatic schema inference from Go types:

```go
type Config struct {
    Name         string
    Description  string
    InputSchema  *jsonschema.Schema   // optional override
    OutputSchema *jsonschema.Schema   // optional override
    // ...
}

type Func[TArgs, TResults any] func(tool.Context, TArgs) (TResults, error)

func New[TArgs, TResults any](cfg Config, handler Func[TArgs, TResults]) (tool.Tool, error)
```

Schema inference:

1. If `InputSchema`/`OutputSchema` is provided in config, use it directly
2. Otherwise, call `jsonschema.For[T](nil)` to infer schema from the Go type
3. Call `.Resolve(nil)` to get a `*jsonschema.Resolved` for validation

The resolved schemas are stored on the `functionTool` struct and used for both LLM declaration and runtime validation.

**Constraints on TArgs:** Must be a struct, map, or pointer to those types. Primitives are rejected at construction time.

The declaration maps to:

```go
type genai.FunctionDeclaration struct {
    Name                 string
    Description          string
    ParametersJsonSchema any   // from inputSchema.Schema()
    ResponseJsonSchema   any   // from outputSchema.Schema()
}
```

The `ParametersJsonSchema` and `ResponseJsonSchema` fields accept `any` and receive the raw JSON Schema object from the resolved schema.

### Path 2: genai.Schema (agenttool, loadmemorytool, hand-built tools)

Some tools build `genai.FunctionDeclaration` directly using `genai.Schema`:

```go
// loadmemorytool example
decl := &genai.FunctionDeclaration{
    Name:        "load_memory",
    Description: "Loads the memory for the current user.",
    Parameters: &genai.Schema{
        Type: "OBJECT",
        Properties: map[string]*genai.Schema{
            "query": {
                Type:        "STRING",
                Description: "The query to search memory for.",
            },
        },
        Required: []string{"query"},
    },
}
```

This is the Gemini API's native schema type. It's simpler but less type-safe than JSON Schema inference.

### Path 3: MCP Schema (mcptoolset)

MCP tools carry their own JSON Schema from the MCP server. The `mcpTool` passes it through directly:

```go
if t.InputSchema != nil {
    mcp.funcDeclaration.ParametersJsonSchema = t.InputSchema
}
if t.OutputSchema != nil {
    mcp.funcDeclaration.ResponseJsonSchema = t.OutputSchema
}
```

## Input/Output Marshaling

### Input: LLM args -> Go types

The LLM returns function call arguments as `map[string]any` (parsed from JSON). The marshaling path in `functiontool.Run()`:

```text
LLM FunctionCall.Args (map[string]any)
    ↓ typeutil.ConvertToWithJSONSchema[map[string]any, TArgs](args, inputSchema)
        ↓ json.Marshal(args)        →  []byte
        ↓ json.Unmarshal → map       →  validate against schema
        ↓ json.Unmarshal → TArgs     →  typed Go value
    ↓ handler(ctx, typedArgs)
```

The `ConvertToWithJSONSchema` function (`internal/typeutil/convert.go`):

```go
func ConvertToWithJSONSchema[From, To any](v From, resolvedSchema *jsonschema.Resolved) (To, error) {
    rawArgs, _ := json.Marshal(v)
    if resolvedSchema != nil {
        var m map[string]any
        json.Unmarshal(rawArgs, &m)
        resolvedSchema.Validate(m)  // validate against JSON Schema
    }
    var typed To
    json.Unmarshal(rawArgs, &typed)
    return typed, nil
}
```

This is a JSON round-trip conversion with optional schema validation in between. The validation runs against `map[string]any` (not the struct) because the schema library can't account for `omitempty` or custom marshaling on structs.

### Output: Go types -> LLM response

The return path in `functiontool.Run()`:

```text
handler returns TResults
    ↓ typeutil.ConvertToWithJSONSchema[TResults, map[string]any](output, outputSchema)
    ↓ if conversion fails and output validates against schema:
        ↓ wrap as {"result": output}
    ↓ returned as map[string]any for FunctionResponse
```

The fallback wrapping (`{"result": output}`) handles cases where the output is a primitive type that can't directly become a `map[string]any`. This mirrors the Python ADK behavior.

### Other tools

Non-functiontool implementations handle marshaling manuallylly:
- **agenttool**: Extracts text from sub-agent's last event, returns `{"result": text}`
- **mcptool**: Extracts text from MCP `TextContent`, returns `{"output": text}` or `{"output": structuredContent}`
- **loadmemorytool**: Manually pulls `"query"` from `args.(map[string]any)`, returns `{"memories": entries}`

## Tool Registration

Tools are configured on the agent:

```go
// agent/llmagent/llmagent.go
type Config struct {
    Tools    []tool.Tool     // static tool list
    Toolsets []tool.Toolset  // dynamic tool provision

    BeforeToolCallbacks  []BeforeToolCallback
    AfterToolCallbacks   []AfterToolCallback
    OnToolErrorCallbacks []OnToolErrorCallback
    // ...
}
```

At request time, the `toolProcessor` request processor:

1. Extracts `Tools` from the agent's config
2. Calls `toolset.Tools(ctx)` on each `Toolset` to resolve dynamic tools
3. Calls `ProcessRequest(ctx, req)` on each tool that implements `RequestProcessor`
4. Each tool's `ProcessRequest` calls `toolutils.PackTool(req, self)` to add its declaration

### Declaration Packing

`toolutils.PackTool` consolidates all function declarations into a single `genai.Tool`:

```go
func PackTool(req *model.LLMRequest, tool Tool) error {
    // Register tool by name in req.Tools map (detects duplicates)
    req.Tools[name] = tool

    // Find existing genai.Tool with FunctionDeclarations, or create one
    // Append this tool's Declaration() to FunctionDeclarations list
}
```

Multiple function tools share a single `genai.Tool.FunctionDeclarations` slice. Native Gemini tools (search, retrieval) are added as separate `genai.Tool` entries.

## Tool Execution Flow

When the LLM returns `FunctionCall` parts in its response:

```text
LLM response contains FunctionCall{ID, Name, Args}
    ↓
Look up tool by Name in req.Tools map
    ↓
Cast to toolinternal.FunctionTool
    ↓
Create toolContext with FunctionCallID, fresh EventActions, confirmation state
    ↓
Run BeforeToolCallbacks (can override args or skip tool)
    ↓
tool.Run(toolCtx, args)
    ↓
On error: run OnToolErrorCallbacks (can recover)
    ↓
Run AfterToolCallbacks (can modify result)
    ↓
Build FunctionResponse{ID, Name, Response: result}
    ↓
Merge all parallel tool responses into single Content
    ↓
Append to conversation, send back to LLM
    ↓
Loop until no more FunctionCalls in response
```

### Parallel Tool Calls

When the LLM returns multiple `FunctionCall` parts in a single response, they are all processed before sending any results back. All `FunctionResponse` parts are merged into a single `Content` with role `"user"`.

### Panic Recovery

`functiontool.Run()` wraps tool execution in a `defer/recover` block. Panics are caught and returned as errors with full stack traces:

```go
defer func() {
    if r := recover(); r != nil {
        err = fmt.Errorf("panic in tool %q: %v\nstack: %s", f.Name(), r, debug.Stack())
    }
}()
```

## Tool Callbacks

Six callback types provide interception points:

| Callback | Signature | When | Can Override |
| --- | --- | --- | --- |
| `BeforeModelCallback` | `(CallbackContext, *LLMRequest) → (*LLMResponse, error)` | Before LLM call | Return non-nil to skip LLM |
| `AfterModelCallback` | `(CallbackContext, *LLMResponse, error) → (*LLMResponse, error)` | After LLM call | Return non-nil to replace response |
| `OnModelErrorCallback` | `(CallbackContext, *LLMRequest, error) → (*LLMResponse, error)` | On LLM error | Return non-nil to recover |
| `BeforeToolCallback` | `(Context, Tool, args) → (result, error)` | Before tool.Run | Return non-nil to skip tool |
| `AfterToolCallback` | `(Context, Tool, args, result, error) → (result, error)` | After tool.Run | Return non-nil to replace result |
| `OnToolErrorCallback` | `(Context, Tool, args, error) → (result, error)` | On tool error | Return non-nil to recover |

Callbacks execute in order. The first one that returns a non-nil result short-circuits the rest.

For `BeforeToolCallback`, returning `(nil, nil)` means "continue normally" - you can modify `args` in place and let the tool run.

## Human-in-the-Loop Confirmation

The confirmation system (`tool/toolconfirmation/`) enables requiring user approval before tool execution.

### Configuration

Two modes on `functiontool.Config`:

```go
RequireConfirmation bool                  // always require approval
RequireConfirmationProvider any           // func(TArgs) bool - dynamic decision
```

### Confirmation Types

```go
const FunctionCallName = "adk_request_confirmation"

type ToolConfirmation struct {
    Hint      string  // message shown to user
    Confirmed bool    // user's decision
    Payload   any     // additional context
}
```

### Flow

1. Tool calls `ctx.RequestConfirmation(hint, payload)`
2. Framework stores `ToolConfirmation` in `eventActions.RequestedToolConfirmations[functionCallID]`
3. Event emitted with special `adk_request_confirmation` FunctionCall containing:
   - `originalFunctionCall`: the tool call that needs approval
   - `toolConfirmation`: hint and payload
4. Client shows prompt, user approves/rejects
5. Client sends `FunctionResponse` with `{"confirmed": true/false}` and matching ID
6. Next turn: `RequestConfirmationRequestProcessor` parses response
7. If confirmed: tool is re-invoked with `ctx.ToolConfirmation().Confirmed == true`
8. If rejected: tool returns error `"tool X call is rejected"`

### OriginalCallFrom Helper

```go
func OriginalCallFrom(functionCall *genai.FunctionCall) (*genai.FunctionCall, error)
```

Extracts the original tool call from a confirmation wrapper. Handles both typed `*genai.FunctionCall` and raw `map[string]any` formats.

## Built-in Tool Implementations

### functiontool (Generic Function Wrapper)

The primary user-facing tool builder. Wraps any Go function with automatic schema inference.

```go
type GetWeatherArgs struct {
    City string `json:"city" jsonschema:"description=The city name"`
}

weatherTool, _ := functiontool.New(functiontool.Config{
    Name:        "get_weather",
    Description: "Gets weather for a city",
}, func(ctx tool.Context, args GetWeatherArgs) (map[string]any, error) {
    return map[string]any{"temp": 72}, nil
})
```

For long-running tools, the framework appends a note to the description: *"NOTE: This is a long-running operation. Do not call this tool again if it has already returned some intermediate or pending status."*

### agenttool (Agent Composition)

Wraps an agent as a callable tool, enabling agent-to-agent delegation.

```go
agentTool := agenttool.New(subAgent, &agenttool.Config{
    SkipSummarization: true,
})
```

Execution creates a fresh in-memory session for the sub-agent, runs it via a new `Runner`, and returns the last text content as the tool result. The parent agent's state (minus `_adk` internal keys) is copied into the sub-agent's session.

If the wrapped agent has an `InputSchema`, it's used directly. Otherwise, a default schema with a single `request` string parameter is generated.

### geminitool (Native Gemini Tools)

Thin wrapper around `genai.Tool` for Gemini-native capabilities (search, retrieval, code execution):

```go
searchTool := geminitool.New("google_search", &genai.Tool{
    GoogleSearch: &genai.GoogleSearch{},
})
```

These tools don't implement `FunctionTool` (no `Declaration()` or `Run()`). They're added directly to `req.Config.Tools` as `genai.Tool` entries. The LLM handles them natively.

### mcptoolset (MCP Integration)

Wraps Model Context Protocol servers as ADK toolsets. Each MCP tool becomes a `FunctionTool` with schema passed through from the MCP server.

```go
type ConfirmationProvider func(toolName string, args any) bool
```

MCP tool results are converted: structured content becomes `{"output": structuredContent}`, text content becomes `{"output": text}`.

### loadmemorytool (Explicit Memory Search)

An explicit tool the model can call to search memory:

```go
load_memory(query: string) → {memories: []memory.Entry}
```

Its `ProcessRequest` also injects system instructions: *"You have memory. You can use it to answer questions..."*

### preloadmemorytool (Implicit Memory Injection)

Not a callable tool. Implements only `RequestProcessor` to search memory using the user's query and inject results into system instructions automatically. The model never sees or calls this tool directly.

### exitlooptool (Control Flow)

Built on `functiontool` with empty args. Sets `Actions().Escalate = true` and `Actions().SkipSummarization = true` to break out of an agent loop.

```go
func exitLoop(ctx tool.Context, _ EmptyArgs) (map[string]string, error) {
    ctx.Actions().Escalate = true
    ctx.Actions().SkipSummarization = true
    return map[string]string{}, nil
}
```

### loadartifactstool (Artifact Injection)

Pre-fetches artifact content and appends it to the LLM request as additional `genai.Content`. Runs during request processing, not as a callable tool.

## LLM Request/Response Types

### LLMRequest

```go
type LLMRequest struct {
    Model    string
    Contents []*genai.Content
    Config   *genai.GenerateContentConfig
    Tools    map[string]any `json:"-"`  // internal: name → tool.Tool
}
```

`Tools` is a parallel map that tracks tool instances by name for lookup during response processing. It's excluded from JSON serialization. The actual LLM-facing tool declarations live in `Config.Tools` as `[]*genai.Tool`.

### LLMResponse

```go
type LLMResponse struct {
    Content           *genai.Content
    CitationMetadata  *genai.CitationMetadata
    GroundingMetadata *genai.GroundingMetadata
    UsageMetadata     *genai.GenerateContentResponseUsageMetadata
    CustomMetadata    map[string]any
    Partial           bool          // streaming: incomplete content
    TurnComplete      bool          // streaming: turn finished
    Interrupted       bool          // user interrupted during bidi streaming
    ErrorCode         string
    ErrorMessage      string
    FinishReason      genai.FinishReason
}
```

### Content & Parts

```go
type genai.Content struct {
    Parts []*genai.Part
    Role  genai.Role  // "model", "user", "system"
}

type genai.Part struct {
    Text                string
    FunctionCall        *genai.FunctionCall      // model wants to call a tool
    FunctionResponse    *genai.FunctionResponse   // tool execution result
    CodeExecutionResult *genai.CodeExecutionResult
    // ... other part types
}

type genai.FunctionCall struct {
    ID   string          // unique call ID
    Name string          // tool name
    Args map[string]any  // input arguments
}

type genai.FunctionResponse struct {
    ID       string          // matches FunctionCall.ID
    Name     string          // tool name
    Response map[string]any  // output result
}
```

## Patterns Worth Borrowing

### Minimal public interface, rich internal interface
The public `Tool` interface has just 3 read-only methods. The callable behavior (`Declaration()`, `Run()`) is internal. This keeps the user-facing API clean while giving the framework the power it needs. Users create tools via `functiontool.New()` and don't need to implement the internal interface.

### Generic function wrapping with schema inference
`functiontool.New[TArgs, TResults]` uses Go generics to infer JSON Schema from type parameters. Users define a struct with `json` and `jsonschema` tags, and the framework handles everything else. Schema can be overridden when inference isn't sufficient.

### ProcessRequest as a tool-level middleware
Each tool gets to modify the `LLMRequest` before it's sent. Most just pack their declaration, but some inject system instructions (memory tool) or add native tool configs (Gemini tools). This is more flexible than a central tool registration system.

### JSON round-trip for type conversion
The `ConvertToWithJSONSchema` approach (marshal to JSON, optionally validate, unmarshal to target type) is simple and handles all the edge cases of converting `map[string]any` to typed structs. The schema validation step runs against the intermediate map, not the struct, to handle `omitempty` correctly.

### Uniform result type
All tools return `map[string]any`. Non-map results get wrapped as `{"result": value}`. This simplifies the framework's response handling - there's exactly one result format to deal with.

### Confirmation as a virtual function call
HITL approval uses the same `FunctionCall`/`FunctionResponse` mechanism as regular tool calls. The client doesn't need special handling beyond recognizing the `adk_request_confirmation` name. The framework re-invokes the original tool transparently after approval.

### Callback chains with short-circuit
Before/After/OnError callbacks for both model and tool calls follow a consistent pattern: execute in order, first non-nil result wins. This provides clean interception for caching, logging, access control, and error recovery.

## Package Structure

```text
tool/
    tool.go                    # Tool, Context, Toolset, Predicate interfaces
    functiontool/
        function.go            # Generic function tool builder
    agenttool/
        agent_tool.go          # Agent-as-tool wrapper
    geminitool/
        tool.go                # Native Gemini tool wrapper
    mcptoolset/
        tool.go                # MCP tool wrapper
        toolset.go             # MCP toolset (dynamic tool discovery)
    loadmemorytool/
        tool.go                # Explicit memory search tool
    preloadmemorytool/
        tool.go                # Implicit memory injection
    loadartifactstool/
        load_artifacts_tool.go # Artifact content injection
    exitlooptool/
        tool.go                # Loop exit control flow tool
    toolconfirmation/
        tool_confirmation.go   # HITL confirmation types

internal/
    toolinternal/
        tool.go                # FunctionTool, RequestProcessor interfaces
        context.go             # toolContext implementation
        toolutils/
            toolutils.go       # PackTool helper
    typeutil/
        convert.go             # ConvertToWithJSONSchema (JSON round-trip + validation)
    llminternal/
        base_flow.go           # Tool invocation loop, callback execution
        tools_processor.go     # Tool resolution request processor
        request_confirmation_processor.go  # HITL confirmation handling

model/
    llm.go                     # LLM, LLMRequest, LLMResponse types
```

## Things to Be Aware Of

- The `Tool` public interface is just a marker. All callable behavior is behind the internal `FunctionTool` interface. This means you can't create a callable tool by just implementing `Tool` - you must use `functiontool.New()` or implement the internal interface.
- `functiontool` requires `TArgs` to be a struct or map. Primitive input types (string, int) are not supported. Use a wrapper struct.
- Schema inference uses `jsonschema.For[T]()` which relies on struct tags. If your type has custom JSON marshaling, the inferred schema may not match. Use `InputSchema` override in that case.
- `agenttool.Run()` creates a fresh in-memory session and runner per invocation. This means sub-agent state doesn't persist across calls, and there's overhead for each invocation.
- `agenttool` duplicates the `PackTool` logic inline rather than calling `toolutils.PackTool`. There's a TODO noting this should be extracted.
- The `geminitool` description is hardcoded to "Performs a Google search..." regardless of what the tool actually does. This seems like a bug or placeholder.
- MCP tools pass JSON Schema through directly from the MCP server. There's no validation that the schema is compatible with the Gemini API's expectations.
