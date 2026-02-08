# Codex Sessions Reference

This document describes the session architecture in
[OpenAI Codex](https://github.com/openai/codex), an open-source coding agent.
Codex is implemented in Rust and its session model provides useful patterns for
building interactive, resumable AI agent conversations.

## Overview

A **session** is the core runtime abstraction for an interactive conversation
between a user and an AI model. It manages conversation turns, model
configuration, tool execution policies, and persistence. Sessions can be
**resumed** across process restarts by replaying a persisted rollout file.

Each session is identified by a `ThreadId` (UUID v7) and may have an optional
human-readable name.

## Key Types

### `ThreadId`

A newtype wrapper around a UUID v7, used as the unique session identifier.
UUID v7 is time-ordered, which means session IDs sort chronologically.

```rust
// protocol/src/thread_id.rs
pub struct ThreadId {
    uuid: Uuid,
}
```

### `Session`

The top-level runtime handle. Shared via `Arc<Session>` and holds the mutable
state, event channels, and service connections.

```rust
// core/src/codex.rs
pub(crate) struct Session {
    pub(crate) conversation_id: ThreadId,
    tx_event: Sender<Event>,
    agent_status: watch::Sender<AgentStatus>,
    state: Mutex<SessionState>,
    features: Features,
    pending_mcp_server_refresh_config: Mutex<Option<McpServerRefreshConfig>>,
    pub(crate) active_turn: Mutex<Option<ActiveTurn>>,
    pub(crate) services: SessionServices,
    next_internal_sub_id: AtomicU64,
}
```

| Field                               | Purpose                                                            |
| ----------------------------------- | ------------------------------------------------------------------ |
| `conversation_id`                   | Unique `ThreadId` for this session                                 |
| `tx_event`                          | Channel for pushing events to clients (TUI, VSCode, etc.)          |
| `agent_status`                      | Watch channel for broadcasting agent status updates                |
| `state`                             | Mutex-protected mutable session state                              |
| `features`                          | Feature flags, invariant for the session lifetime                  |
| `pending_mcp_server_refresh_config` | Pending MCP server refresh configuration                           |
| `active_turn`                       | Guards against concurrent turn execution (at most one active turn) |
| `services`                          | MCP connections, auth, model providers                             |
| `next_internal_sub_id`              | Atomic counter for generating internal submission IDs              |

### `SessionState`

The mutable interior of a session. Protected by a `Mutex` and updated on every
turn or configuration change.

```rust
// core/src/state/session.rs
pub(crate) struct SessionState {
    pub(crate) session_configuration: SessionConfiguration,
    pub(crate) history: ContextManager,
    pub(crate) latest_rate_limits: Option<RateLimitSnapshot>,
    pub(crate) server_reasoning_included: bool,
    pub(crate) dependency_env: HashMap<String, String>,
    pub(crate) mcp_dependency_prompted: HashSet<String>,
    pub(crate) initial_context_seeded: bool,
    pub(crate) pending_resume_previous_model: Option<String>,
}
```

| Field                           | Purpose                                                                |
| ------------------------------- | ---------------------------------------------------------------------- |
| `session_configuration`         | All static and mutable configuration for the session                   |
| `history`                       | `ContextManager` containing ordered conversation items and token usage |
| `latest_rate_limits`            | Most recent rate limit snapshot from the API                           |
| `server_reasoning_included`     | Whether the server included reasoning tokens                           |
| `dependency_env`                | Environment variables for dependency resolution                        |
| `mcp_dependency_prompted`       | MCP dependencies already prompted to the user                          |
| `initial_context_seeded`        | Whether initial context has been loaded into history                   |
| `pending_resume_previous_model` | Previous model name (for warning on model switch after resume)         |

### `SessionConfiguration`

Contains all configuration for a session, including model settings, policies,
instructions, and working directory.

```rust
// core/src/codex.rs
pub(crate) struct SessionConfiguration {
    provider: ModelProviderInfo,
    collaboration_mode: CollaborationMode,
    model_reasoning_summary: ReasoningSummaryConfig,
    developer_instructions: Option<String>,
    user_instructions: Option<String>,
    personality: Option<Personality>,
    base_instructions: String,
    compact_prompt: Option<String>,
    approval_policy: Constrained<AskForApproval>,
    sandbox_policy: Constrained<SandboxPolicy>,
    windows_sandbox_level: WindowsSandboxLevel,
    cwd: PathBuf,
    codex_home: PathBuf,
    thread_name: Option<String>,
    original_config_do_not_use: Arc<Config>,
    session_source: SessionSource,
    dynamic_tools: Vec<DynamicToolSpec>,
}
```

| Field                     | Purpose                                                    |
| ------------------------- | ---------------------------------------------------------- |
| `provider`                | Model provider identifier (e.g. "openai", "openrouter")    |
| `collaboration_mode`      | Model slug and reasoning effort settings                   |
| `model_reasoning_summary` | Configuration for reasoning token summaries                |
| `developer_instructions`  | Developer instructions that supplement base instructions   |
| `user_instructions`       | Model instructions appended to the base instructions       |
| `personality`             | Personality preference for the model                       |
| `base_instructions`       | Core system prompt                                         |
| `compact_prompt`          | Override prompt used during context compaction             |
| `approval_policy`         | When to escalate tool calls for user approval              |
| `sandbox_policy`          | How to sandbox executed commands                           |
| `windows_sandbox_level`   | Windows-specific sandbox level                             |
| `cwd`                     | Working directory; all relative paths resolve against this |
| `codex_home`              | Directory containing all Codex state for this session      |
| `thread_name`             | Optional user-facing name for the session                  |
| `session_source`          | Origin of the session (cli, vscode, exec, mcp, subagent)   |
| `dynamic_tools`           | User-registered dynamic tool specifications                |

### `ContextManager`

Manages conversation history as an ordered list of response items, oldest first.

```rust
// core/src/context_manager/history.rs
pub(crate) struct ContextManager {
    items: Vec<ResponseItem>,
    token_info: Option<TokenUsageInfo>,
}
```

The `ContextManager` supports:

- **Appending** new `ResponseItem`s (messages, function calls, function call outputs)
- **Compaction** to trim old items when the context window fills
- **Rollback** to remove the last N user turns

### `InitialHistory`

Determines how a session's history is initialized at creation time.

```rust
// protocol/src/protocol.rs
pub enum InitialHistory {
    New,
    Resumed(ResumedHistory),
    Forked(Vec<RolloutItem>),
}
```

| Variant   | Behavior                                                      |
| --------- | ------------------------------------------------------------- |
| `New`     | Fresh session with no prior history                           |
| `Resumed` | Loaded from a rollout file, retaining the original `ThreadId` |
| `Forked`  | New `ThreadId` but seeded with items from an existing session |

### `SessionConfiguredEvent`

Emitted to clients when a session is fully initialized. Contains all metadata
needed for the UI to render the session.

```rust
// protocol/src/protocol.rs
pub struct SessionConfiguredEvent {
    pub session_id: ThreadId,
    pub forked_from_id: Option<ThreadId>,
    pub thread_name: Option<String>,
    pub model: String,
    pub model_provider_id: String,
    pub approval_policy: AskForApproval,
    pub sandbox_policy: SandboxPolicy,
    pub cwd: PathBuf,
    pub reasoning_effort: Option<ReasoningEffortConfig>,
    pub history_log_id: u64,
    pub history_entry_count: usize,
    pub initial_messages: Option<Vec<EventMsg>>,
    pub rollout_path: Option<PathBuf>,
}
```

## Session Lifecycle

### Creation

Entry point: `Codex::spawn()` -> `Session::new()`

1. Performs parallel async setup (rollout recorder, shell discovery, history
   metadata)
2. Generates a new `ThreadId` (UUID v7)
3. Builds `SessionConfiguration` from the provided `Config`
4. Initializes `ContextManager` for conversation history
5. Sets up `SessionServices` (MCP, auth, model providers)
6. Emits a `SessionConfigured` event with full metadata
7. Starts background tasks (file watcher, WebSocket pre-connect, MCP servers)

### Turn Execution

State flows through an async submission queue:

```
User -> submit(Op) -> submission_loop() -> lock state -> mutate -> persist -> emit Event
```

The `Op` enum defines all operations a client can submit. The most important
variants for session state are:

| Op Variant                       | Effect                                                                 |
| -------------------------------- | ---------------------------------------------------------------------- |
| `UserInput`                      | Sends user message items to the model and records the response         |
| `UserTurn`                       | Like `UserInput` but includes full turn context (cwd, policies, model) |
| `OverrideTurnContext`            | Updates session configuration without sending a message                |
| `ExecApproval` / `PatchApproval` | Resolves pending tool approval requests                                |
| `Compact`                        | Triggers context compaction (summarization)                            |
| `SetThreadName`                  | Updates the user-facing session name                                   |
| `Undo`                           | Reverts the most recent turn                                           |
| `ThreadRollback`                 | Drops the last N user turns from in-memory context                     |
| `Interrupt`                      | Aborts the current turn                                                |
| `Shutdown`                       | Gracefully shuts down the session                                      |

### Configuration Updates

`OverrideTurnContext` allows incremental updates to session configuration. All
fields are optional; omitted fields preserve their current values. Updates are
validated through `SessionConfiguration::apply()`.

Updatable fields: `cwd`, `approval_policy`, `sandbox_policy`, `model`,
`effort`, `summary`, `collaboration_mode`, `personality`,
`windows_sandbox_level`.

### Concurrency Model

- **Single active turn**: `Mutex<Option<ActiveTurn>>` prevents concurrent model
  invocations. Only one turn executes at a time.
- **Session state**: Protected by `Mutex<SessionState>`.
- **Services**: Use `RwLock` for read-heavy MCP connections.
- **Event broadcasting**: Async unbounded channel decouples state mutation from
  UI rendering.

## Persistence

### Rollout Files

The primary persistence mechanism. Each session writes to an append-only JSONL
file:

```
~/.codex/sessions/rollout-<timestamp>-<thread-id>.jsonl
```

Each line is a `RolloutItem` which can be session metadata, an event message,
or a response item. The `RolloutRecorder` manages writes and flushing:

```rust
// core/src/rollout/recorder.rs
pub struct RolloutRecorder {
    tx: Sender<RolloutCmd>,
    pub(crate) rollout_path: PathBuf,
    state_db: Option<StateDbHandle>,
}
```

Rollout files can be inspected with standard JSON tooling (e.g. `jq`, `fx`).

### Session Index

An append-only index file maps thread IDs to human-readable names:

```
~/.codex/session_index.jsonl
```

Each entry contains `{id, thread_name, updated_at}`. Lookups read from the
end of the file so the newest entry for a given ID wins.

### Resumption

When resuming a session:

1. The rollout file is loaded by path
2. Conversation history is reconstructed from the `RolloutItem`s
3. Metadata is restored (base instructions, dynamic tools, session source)
4. A warning is emitted if the model has changed since the original session
5. Execution continues with the same `ThreadId`

## Event Flow

Sessions communicate with clients (TUI, VSCode extension, MCP server) through
an event channel. Key events in the session lifecycle:

| Event                | When                                             |
| -------------------- | ------------------------------------------------ |
| `SessionConfigured`  | Session is fully initialized                     |
| `TurnStarted`        | A new turn begins processing                     |
| `AgentMessage`       | Model produces a text response                   |
| `ExecCommandRequest` | Model requests tool execution, awaiting approval |
| `ExecCommandOutput`  | Tool execution result                            |
| `TurnComplete`       | Turn finishes (success, error, or interruption)  |
| `ThreadNameUpdated`  | Session name changes                             |

Every state change emits events and is persisted to the rollout file, ensuring
durability and auditability.
