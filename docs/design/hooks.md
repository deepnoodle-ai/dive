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

| Hook Type                | When it fires                                         | Claude Code equivalent |
| :----------------------- | :---------------------------------------------------- | :--------------------- |
| `SessionStartHook`       | Start of a fresh conversation, before the first LLM call | `SessionStart`      |
| `PreGenerationHook`      | Before the LLM generation loop                        | —                      |
| `PostGenerationHook`     | After the generation loop completes                   | —                      |
| `PreToolUseHook`         | Before a tool call executes                           | `PreToolUse`           |
| `PostToolUseHook`        | After a tool call succeeds                            | `PostToolUse`          |
| `PostToolUseFailureHook` | After a tool call fails                               | `PostToolUseFailure`   |
| `StopHook`               | When the agent is about to stop                       | `Stop`                 |
| `PreIterationHook`       | Before each LLM call in the loop                      | —                      |

Most hook types are `func(ctx context.Context, hctx *HookContext) error`. The
exceptions return richer values: `StopHook` returns `(*StopDecision, error)` and
`SessionStartHook` returns `(*SessionStartResult, error)`.

## Hook Flow

```text
CreateResponse
│
├─ load session history
├─ SessionStart hooks   (only when no prior messages AND not a resume)
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

    SessionStartSource SessionStartSource // SessionStart (why it fired)

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

## SessionStart Hook

`SessionStartHook` seeds a conversation from external state — project config,
user preferences, memory — at the moment it begins, without forcing callers to
write a `PreGenerationHook` that manually inspects the history for emptiness.

```go
type SessionStartHook func(ctx context.Context, hctx *HookContext) (*SessionStartResult, error)

type SessionStartResult struct {
    Messages []*llm.Message // prepended to the conversation, ahead of user input
    Persist  bool           // save to the session (durable) vs first-turn-only (ephemeral)
}
```

### Trigger

The hook fires when **both** conditions hold:

1. The loaded session has **no prior messages**, and
2. The turn is **not** resuming a suspended one (`hasResumeIntent == false`).

This placement is deliberate. The block runs *after* the suspend/resume guards
in `CreateResponse`, so a resume — including a stateless `WithResume` call,
whose synthetic empty history would otherwise look like a fresh start — never
re-fires it. Resuming a turn is not starting a session; firing seed logic there
would re-run side effects (config loads, external lookups) and then discard the
result, since the resume path rebuilds history from the suspended turn rather
than `sessionMsgs`.

Because the trigger is "no prior messages," a **stateless** agent (no `Session`)
fires the hook on every `CreateResponse` — there is never any prior history.
That is intended and documented on the type; the trigger is not "first ever call
for this session object."

### Source

`HookContext.SessionStartSource` tells the hook *why* it fired, mirroring Claude
Code's `SessionStart` source matcher. Today the agent emits only
`SessionStartStartup`. Carrying the source on the context (rather than gating
behavior with a bare boolean) keeps the API open to future sources — e.g.
re-seeding after compaction, or `resume`/`clear`-style starts — without a
signature change.

### Persist vs ephemeral

`SessionStartResult.Persist` chooses how long the seed lives:

- **`Persist: true`** — durable context. The seed messages are written to the
  session as their own *pre-turn* via a `SaveTurn` call before generation, so
  they remain in `Messages()` on every later turn and survive suspend/resume.
  Saving them as a separate pre-turn (rather than splicing them into the first
  turn's input→output delta) keeps the persistence path simple: the normal turn
  delta and the suspend path are untouched, and resume reads the seeds back
  naturally from session history.
- **`Persist: false`** — ephemeral priming. The messages are appended to the
  in-memory history for this call only and influence the first generation, but
  are never saved; later turns do not see them.

`Persist` requires a `Session`. Stateless calls have nowhere to save to, so
seeds are always ephemeral there regardless of the flag.

This maps onto Claude Code's two SessionStart outputs: `Persist: true` is the
analogue of `additionalContext` (context that persists for the session), and
`Persist: false` is closer to `initialUserMessage` (priming for the first turn).

### Errors

A hook error aborts `CreateResponse` immediately (it is logged and returned
wrapped as `session start hook error`). The session has not been written at that
point — for a persistent seed the pre-turn `SaveTurn` happens only after all
hooks succeed — so an aborted start leaves the session untouched.

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
| SessionStart       | Aborts generation          | Aborts generation     |
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
    SessionStart       []SessionStartHook
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
