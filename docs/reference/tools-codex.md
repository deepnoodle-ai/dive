# Codex Tools Reference

This document describes the tools architecture in
[OpenAI Codex](https://github.com/openai/codex), an open-source coding agent.
Codex tools follow a registry-based pattern where each tool has a spec
(schema), a handler (execution logic), and a marshaling layer for
inputs/outputs.

## Overview

A **tool** in Codex is anything the model can invoke to interact with the
outside world: running shell commands, reading files, calling MCP servers, or
delegating to user-defined handlers. Tools are defined by three things:

1. A **spec** (`ToolSpec`) describing the tool's name, description, and JSON
   Schema parameters — sent to the model so it knows what it can call
2. A **handler** (`ToolHandler` trait) that executes the tool and returns output
3. A **registry** (`ToolRegistry`) that maps tool names to handlers at runtime

## Core Trait: `ToolHandler`

Every tool handler implements this async trait.

```rust
// core/src/tools/registry.rs
#[async_trait]
pub trait ToolHandler: Send + Sync {
    /// Whether this handler services Function-style or MCP-style payloads.
    fn kind(&self) -> ToolKind;

    /// Returns true if the payload shape matches this handler's kind.
    fn matches_kind(&self, payload: &ToolPayload) -> bool;

    /// Returns true if the invocation might mutate the user's environment
    /// (filesystem, OS state, etc.). Defaults to false. When true, the
    /// invocation waits on the tool-call gate before proceeding.
    async fn is_mutating(&self, invocation: &ToolInvocation) -> bool;

    /// Execute the tool and return output for the model.
    async fn handle(&self, invocation: ToolInvocation) -> Result<ToolOutput, FunctionCallError>;
}
```

```rust
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum ToolKind {
    Function,
    Mcp,
}
```

The `is_mutating` method is central to the approval flow. When it returns
`true`, the runtime gates execution behind the session's approval policy
before calling `handle`.

## Tool Specs and Schema

### `ToolSpec`

The serialized tool definition sent to the model. There are four variants
matching different tool wire formats.

```rust
// core/src/client_common.rs
#[derive(Debug, Clone, Serialize, PartialEq)]
#[serde(tag = "type")]
pub(crate) enum ToolSpec {
    #[serde(rename = "function")]
    Function(ResponsesApiTool),

    #[serde(rename = "local_shell")]
    LocalShell {},

    #[serde(rename = "web_search")]
    WebSearch {
        external_web_access: Option<bool>,
    },

    #[serde(rename = "custom")]
    Freeform(FreeformTool),
}
```

| Variant      | Use Case                                               |
| ------------ | ------------------------------------------------------ |
| `Function`   | Standard JSON function tools with typed parameters     |
| `LocalShell` | Built-in shell execution (Responses API native tool)   |
| `WebSearch`  | Built-in web search (Responses API native tool)        |
| `Freeform`   | Custom tools with non-JSON formats (e.g. `apply_patch` uses a diff syntax) |

### `ResponsesApiTool`

The standard function tool definition, compatible with the OpenAI Responses
API.

```rust
// core/src/client_common.rs
pub struct ResponsesApiTool {
    pub name: String,
    pub description: String,
    pub strict: bool,
    pub parameters: JsonSchema,
}
```

When `strict` is `true`, the model is constrained to produce output that
validates against the schema. All properties must appear in `required` and
`additionalProperties` must be `false`.

### `FreeformTool`

For tools that accept non-JSON input (e.g. a unified diff format).

```rust
// core/src/client_common.rs
pub struct FreeformTool {
    pub name: String,
    pub description: String,
    pub format: FreeformToolFormat,
}

pub struct FreeformToolFormat {
    pub r#type: String,   // e.g. "text"
    pub syntax: String,   // e.g. "diff"
    pub definition: String,
}
```

### `JsonSchema`

Codex uses its own JSON Schema enum rather than an arbitrary `serde_json::Value`.
This constrains schemas to a supported subset and enables strict validation.

```rust
// core/src/tools/spec.rs
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum JsonSchema {
    Boolean {
        description: Option<String>,
    },
    String {
        description: Option<String>,
    },
    Number {
        description: Option<String>,
    },
    Array {
        items: Box<JsonSchema>,
        description: Option<String>,
    },
    Object {
        properties: BTreeMap<String, JsonSchema>,
        required: Option<Vec<String>>,
        additional_properties: Option<AdditionalProperties>,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(untagged)]
pub enum AdditionalProperties {
    Boolean(bool),
    Schema(Box<JsonSchema>),
}
```

Schemas from external sources (MCP servers, dynamic tools) pass through a
`sanitize_json_schema()` function that normalizes missing `type` fields,
infers types from other keywords (`properties` implies `object`, `items`
implies `array`), and ensures required structural fields are present.

### `ConfiguredToolSpec`

Wraps a `ToolSpec` with runtime configuration.

```rust
// core/src/tools/registry.rs
pub struct ConfiguredToolSpec {
    pub spec: ToolSpec,
    pub supports_parallel_tool_calls: bool,
}
```

## Tool Registration

Tools are registered via a builder pattern before the session starts.

```rust
// core/src/tools/registry.rs
pub struct ToolRegistryBuilder {
    handlers: HashMap<String, Arc<dyn ToolHandler>>,
    specs: Vec<ConfiguredToolSpec>,
}

impl ToolRegistryBuilder {
    pub fn new() -> Self;
    pub fn push_spec(&mut self, spec: ToolSpec);
    pub fn push_spec_with_parallel_support(&mut self, spec: ToolSpec, supports_parallel: bool);
    pub fn register_handler(&mut self, name: impl Into<String>, handler: Arc<dyn ToolHandler>);
    pub fn build(self) -> (Vec<ConfiguredToolSpec>, ToolRegistry);
}
```

The `build()` method returns both the list of specs (sent to the model) and
the registry (used at dispatch time).

```rust
pub struct ToolRegistry {
    handlers: HashMap<String, Arc<dyn ToolHandler>>,
}

impl ToolRegistry {
    pub fn handler(&self, name: &str) -> Option<Arc<dyn ToolHandler>>;
}
```

## Tool Invocation

### `ToolInvocation`

Created by the turn execution loop when the model emits a tool call. Carries
all context needed by a handler.

```rust
// core/src/tools/context.rs
pub struct ToolInvocation {
    pub session: Arc<Session>,
    pub turn: Arc<TurnContext>,
    pub tracker: SharedTurnDiffTracker,
    pub call_id: String,
    pub tool_name: String,
    pub payload: ToolPayload,
}
```

### `ToolPayload`

The payload variant determines how the tool was invoked and how its arguments
are shaped.

```rust
// core/src/tools/context.rs
pub enum ToolPayload {
    Function {
        arguments: String,        // Raw JSON string from the model
    },
    Custom {
        input: String,            // Raw text input (for freeform tools)
    },
    LocalShell {
        params: ShellToolCallParams,
    },
    Mcp {
        server: String,           // MCP server name
        tool: String,             // Tool name within that server
        raw_arguments: String,    // Raw JSON string
    },
}
```

### Dispatch Flow

The dispatch sequence:

1. Look up the handler by tool name in the `ToolRegistry`
2. Verify `matches_kind()` — the payload shape must match the handler kind
3. If `is_mutating()` returns `true`, wait on the tool-call gate (approval)
4. Call `handler.handle(invocation)`
5. Convert the `ToolOutput` into a `ResponseInputItem` for the model

```rust
// core/src/tools/context.rs (simplified)
pub async fn dispatch(
    &self,
    invocation: ToolInvocation,
) -> Result<ResponseInputItem, FunctionCallError> {
    let handler = self.handler(&invocation.tool_name)?;

    if handler.is_mutating(&invocation).await {
        invocation.turn.tool_call_gate.wait_ready().await;
    }

    let output = handler.handle(invocation).await?;
    Ok(output.into_response(&call_id, &payload))
}
```

## Input/Output Marshaling

### Input: JSON to Rust

Function tool arguments arrive as a raw JSON string from the model. Handlers
deserialize into typed Rust structs using serde:

```rust
// core/src/tools/handlers/mod.rs
fn parse_arguments<T>(arguments: &str) -> Result<T, FunctionCallError>
where
    T: for<'de> Deserialize<'de>,
{
    serde_json::from_str(arguments).map_err(|err| {
        FunctionCallError::RespondToModel(
            format!("failed to parse function arguments: {err}")
        )
    })
}
```

Parsing errors produce `FunctionCallError::RespondToModel`, which sends the
error message back to the model so it can retry with corrected arguments.

### Output: Rust to Response Items

```rust
// core/src/tools/context.rs
pub enum ToolOutput {
    Function {
        body: FunctionCallOutputBody,
        success: Option<bool>,
    },
    Mcp {
        result: Result<CallToolResult, String>,
    },
}
```

`ToolOutput` is converted to the appropriate wire format via `into_response()`:

| Payload Type | Output Wire Format           |
| ------------ | ---------------------------- |
| `Function`   | `ResponseInputItem::FunctionCallOutput` with structured body |
| `Custom`     | `ResponseInputItem::CustomToolCallOutput` with plain string  |
| `Mcp`        | `ResponseInputItem::McpToolCallOutput` with MCP result       |

The `success` field allows tools to report failure without raising an error,
letting the model see the output and decide how to proceed.

## Built-in Tools

Codex registers built-in tools via the `build_specs()` function. Key tools:

### Shell Execution

| Tool              | Description                                    | Key Parameters                                   |
| ----------------- | ---------------------------------------------- | ------------------------------------------------ |
| `exec_command`    | PTY-based command execution                    | `cmd`, `workdir`, `shell`, `timeout`, `tty`      |
| `write_stdin`     | Write to an existing exec session              | `session_id`, `chars`                            |
| `shell`           | Array-based command execution                  | `command` (array), `workdir`, `timeout_ms`       |
| `shell_command`   | String-based command execution                 | `command` (string), `workdir`, `timeout_ms`      |

### File Operations

| Tool          | Description                              |
| ------------- | ---------------------------------------- |
| `apply_patch` | Apply a unified diff patch to files      |
| `read_file`   | Read file contents (experimental)        |
| `list_dir`    | List directory contents (experimental)   |
| `grep_files`  | Search file contents (experimental)      |

### Agent Collaboration

| Tool            | Description                              |
| --------------- | ---------------------------------------- |
| `spawn_agent`   | Launch a sub-agent                       |
| `send_input`    | Send input to a running sub-agent        |
| `resume_agent`  | Resume a paused sub-agent                |
| `wait`          | Wait for a sub-agent to complete         |
| `close_agent`   | Terminate a sub-agent                    |

### Other

| Tool                 | Description                           |
| -------------------- | ------------------------------------- |
| `request_user_input` | Ask the user a question with options  |
| `view_image`         | View a local image file               |
| `get_memory`         | Load a stored memory payload          |
| `update_plan`        | Update the agent's plan (freeform)    |

### MCP Resource Tools

| Tool                          | Description                    |
| ----------------------------- | ------------------------------ |
| `list_mcp_resources`          | List resources from MCP server |
| `list_mcp_resource_templates` | List resource templates        |
| `read_mcp_resource`           | Read a specific MCP resource   |

## Dynamic Tools

Dynamic tools are user-registered tools defined at runtime via
`DynamicToolSpec`. They allow external clients (e.g. the VSCode extension) to
inject tools into the session without modifying Codex itself.

### Definition

```rust
// protocol/src/dynamic_tools.rs
pub struct DynamicToolSpec {
    pub name: String,
    pub description: String,
    pub input_schema: JsonValue,    // Arbitrary JSON Schema
}
```

### Invocation Request and Response

When the model calls a dynamic tool, the session emits a request event to the
client, which must respond with the result.

```rust
// protocol/src/dynamic_tools.rs
pub struct DynamicToolCallRequest {
    pub call_id: String,
    pub turn_id: String,
    pub tool: String,
    pub arguments: JsonValue,
}

pub struct DynamicToolResponse {
    pub content_items: Vec<DynamicToolCallOutputContentItem>,
    pub success: bool,
}

pub enum DynamicToolCallOutputContentItem {
    InputText { text: String },
    InputImage { image_url: String },
}
```

### Integration Flow

1. `DynamicToolSpec` is converted to `ResponsesApiTool` via
   `dynamic_tool_to_openai_tool()`, with schema sanitization
2. A shared `DynamicToolHandler` is registered for all dynamic tool names
3. On invocation, the handler emits a `DynamicToolCallRequest` event to the
   client and awaits the `DynamicToolResponse` via a oneshot channel
4. The response is converted to `ToolOutput::Function` with content items

Dynamic tools are always treated as mutating (`is_mutating` returns `true`).

## MCP Tools

MCP (Model Context Protocol) tools integrate external tool servers into the
Codex tool system.

### Schema Conversion

MCP tool definitions are converted to `ResponsesApiTool` via
`mcp_tool_to_openai_tool()`:

```rust
// core/src/tools/spec.rs
pub(crate) fn mcp_tool_to_openai_tool(
    fully_qualified_name: String,
    tool: rmcp::model::Tool,
) -> Result<ResponsesApiTool, serde_json::Error>;
```

The conversion:
- Extracts `description` and `input_schema` from the MCP tool definition
- Ensures `properties` exists (some MCP servers omit it)
- Runs `sanitize_json_schema()` to normalize type fields and structure
- Produces a `ResponsesApiTool` with `strict: false`

### Handler

A shared `McpHandler` routes calls to the appropriate MCP server connection:

```rust
// core/src/tools/handlers/mcp.rs
pub struct McpHandler;

impl ToolHandler for McpHandler {
    fn kind(&self) -> ToolKind { ToolKind::Mcp }

    async fn handle(&self, invocation: ToolInvocation) -> Result<ToolOutput, FunctionCallError> {
        // Extracts server name, tool name, and raw arguments from ToolPayload::Mcp
        // Delegates to handle_mcp_tool_call() which routes to the MCP server
        // Returns ToolOutput::Mcp with the server's CallToolResult
    }
}
```

## Tool Approval Flow

Tools that may mutate the environment go through an approval gate. The flow is
driven by the session's `AskForApproval` and `SandboxPolicy` configuration.

### Approval Request

```rust
// core/src/exec_policy.rs
pub(crate) struct ExecApprovalRequest<'a> {
    pub command: &'a [String],
    pub approval_policy: AskForApproval,
    pub sandbox_policy: &'a SandboxPolicy,
    pub sandbox_permissions: SandboxPermissions,
    pub prefix_rule: Option<Vec<String>>,
}
```

### Approval Outcomes

```rust
// core/src/exec_policy.rs
pub enum ExecApprovalRequirement {
    Forbidden { reason: String },
    NeedsApproval {
        reason: String,
        proposed_execpolicy_amendment: Option<ExecPolicyAmendment>,
    },
    Skip {
        bypass_sandbox: bool,
        proposed_execpolicy_amendment: Option<ExecPolicyAmendment>,
    },
}
```

| Outcome         | Behavior                                                |
| --------------- | ------------------------------------------------------- |
| `Forbidden`     | Command is blocked; error returned to the model         |
| `NeedsApproval` | Execution pauses; user is prompted via `ExecApproval` event |
| `Skip`          | Execution proceeds immediately (may bypass sandbox)     |

Shell tools include `sandbox_permissions` and `justification` parameters in
their schema, allowing the model to request specific permissions and explain
why they are needed. The policy manager evaluates commands against configured
exec rules to produce the approval decision.
