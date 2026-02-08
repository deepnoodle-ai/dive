# Hooks Design Document

This document describes the design of Dive's hook system. The implementation
lives in `hooks.go` and `agent.go`.

## Overview

Hooks are Go functions that run at specific points in the agent's generation
loop. They provide deterministic control over agent behavior: inject context,
enforce permissions, log usage, prevent stopping, and react to tool outcomes.

Dive's hook names and semantics align with Claude Code's hook events where a
direct mapping exists. The generation-level hooks (`PreGeneration`,
`PostGeneration`, `PreIteration`) are Dive-specific because Claude Code's
lifecycle is CLI-centric while Dive's is API-centric.

### Goals

1. **Claude Code alignment** — Hook names match Claude Code events where applicable
2. **Unified context** — All hooks receive `*HookContext` with a shared `Values` map
3. **Composability** — Hooks are plain functions; helpers like `MatchTool` wrap them
4. **Library-first** — No config files, no shell commands; hooks are Go code

### Non-Goals

1. Shell-based hooks (Claude Code's primary hook mechanism)
2. Async/background hooks
3. Prompt-based or agent-based hooks (can be built on top)

## Hook Types

| Hook Type                | When it fires                       | Claude Code equivalent   |
| :----------------------- | :---------------------------------- | :----------------------- |
| `PreGenerationHook`      | Before the LLM generation loop      | `SessionStart` (loosely) |
| `PostGenerationHook`     | After the generation loop completes | —                        |
| `PreToolUseHook`         | Before a tool call executes         | `PreToolUse`             |
| `PostToolUseHook`        | After a tool call succeeds          | `PostToolUse`            |
| `PostToolUseFailureHook` | After a tool call fails             | `PostToolUseFailure`     |
| `StopHook`               | When the agent is about to stop     | `Stop`                   |
| `PreIterationHook`       | Before each LLM call in the loop    | —                        |

All hook types are `func(ctx context.Context, hctx *HookContext) error` except
`StopHook` which returns `(*StopDecision, error)`.

## Hook Flow

```text
CreateResponse
│
├─ PreGeneration hooks
│
├─ generate loop:
│  ├─ PreIteration hooks
│  ├─ LLM call
│  └─ for each tool call:
│     ├─ PreToolUse hooks
│     ├─ execute tool
│     └─ PostToolUse hooks (success) OR PostToolUseFailure hooks (failure)
│
├─ Stop hooks
│  └─ if Continue: true → inject reason as user message, re-enter loop
│
└─ PostGeneration hooks
```

## HookContext

All hooks receive `*HookContext`. Fields are populated based on the phase:

```go
type HookContext struct {
    Agent        *Agent
    Values       map[string]any       // shared across all phases

    SystemPrompt string               // PreGeneration, PreIteration
    Messages     []*llm.Message       // PreGeneration, PreIteration

    Response       *Response           // PostGeneration, Stop
    OutputMessages []*llm.Message      // PostGeneration, Stop
    Usage          *llm.Usage          // PostGeneration, Stop

    Tool   Tool                        // PreToolUse, PostToolUse, PostToolUseFailure
    Call   *llm.ToolUseContent         // PreToolUse, PostToolUse, PostToolUseFailure
    Result *ToolCallResult             // PostToolUse, PostToolUseFailure

    UpdatedInput      []byte           // PreToolUse (set to rewrite input)
    AdditionalContext string           // PreToolUse, PostToolUse, PostToolUseFailure

    StopHookActive bool                // Stop
    Iteration      int                 // PreIteration
}
```

The `Values` map persists across all phases within one `CreateResponse` call,
enabling hooks to pass data to each other.

## PostToolUse vs PostToolUseFailure

Claude Code fires separate events for tool success and failure. Dive mirrors
this with two hook types:

- **`PostToolUseHook`** fires when the tool call succeeds (no error, `IsError`
  not set). Use for logging, metrics, result transformation.
- **`PostToolUseFailureHook`** fires when the tool call fails (error returned
  or `Result.IsError` is true). Use for error diagnostics, retry logic,
  failure alerts.

The agent dispatches to the correct list based on the tool call outcome.
Both receive the same `*HookContext` fields (`Tool`, `Call`, `Result`).

## Stop Hook

Stop hooks run after the generation loop exits and before PostGeneration.
A hook can return `&StopDecision{Continue: true, Reason: "..."}` to inject
the reason as a user message and re-enter the generation loop.

`hctx.StopHookActive` is true on subsequent stop checks, so hooks can detect
re-entry and avoid infinite loops.

## PreToolUse Capabilities

PreToolUse hooks can do more than allow/deny:

- **Input modification**: Set `hctx.UpdatedInput` to rewrite tool arguments
  before execution. Only the last hook's value takes effect.
- **Context injection**: Set `hctx.AdditionalContext` to append text to the
  tool result message sent to the LLM.

## Error Handling

| Hook type          | Regular error              | `*HookAbortError`     |
| :----------------- | :------------------------- | :-------------------- |
| PreGeneration      | Aborts generation          | Aborts generation     |
| PostGeneration     | Logged, response preserved | Aborts, returns error |
| PreToolUse         | Denies tool call           | Aborts generation     |
| PostToolUse        | Logged, result preserved   | Aborts generation     |
| PostToolUseFailure | Logged, result preserved   | Aborts generation     |
| Stop               | Logged, continues          | Aborts generation     |
| PreIteration       | Aborts generation          | Aborts generation     |

## Hook Helpers

| Helper                 | Type                     | Description                           |
| :--------------------- | :----------------------- | :------------------------------------ |
| `InjectContext`        | `PreGenerationHook`      | Prepends content as a user message    |
| `CompactionHook`       | `PreGenerationHook`      | Triggers compaction above a threshold |
| `UsageLogger`          | `PostGenerationHook`     | Logs token usage via callback         |
| `UsageLoggerWithSlog`  | `PostGenerationHook`     | Logs token usage via slog             |
| `MatchTool`            | `PreToolUseHook`         | Runs only for matching tool names     |
| `MatchToolPost`        | `PostToolUseHook`        | Runs only for matching tool names     |
| `MatchToolPostFailure` | `PostToolUseFailureHook` | Runs only for matching tool names     |

All `Match*` helpers accept a Go regexp pattern compiled once at construction.

## Hooks Struct

Hooks are grouped on `AgentOptions` in the `Hooks` struct:

```go
type Hooks struct {
    PreGeneration      []PreGenerationHook
    PostGeneration     []PostGenerationHook
    PreToolUse         []PreToolUseHook
    PostToolUse        []PostToolUseHook
    PostToolUseFailure []PostToolUseFailureHook
    Stop               []StopHook
    PreIteration       []PreIterationHook
}
```

## Files

| File       | Contents                                                                                       |
| :--------- | :--------------------------------------------------------------------------------------------- |
| `hooks.go` | `HookContext`, all hook types, helpers, error types                                            |
| `agent.go` | `Hooks` struct, `AgentOptions`, dispatch in `CreateResponse` / `generate` / `executeToolCalls` |
| `state.go` | `StateKey*` constants for the `Values` map                                                     |
