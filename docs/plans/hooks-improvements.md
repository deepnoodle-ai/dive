# Hooks Improvements Plan

> **Status: Implemented** — Phases 1 and 2 have been implemented. The hook
> system now uses a unified `HookContext`, `Hooks` struct on `AgentOptions`,
> and includes all new hook types and helpers described below.

Dive hooks today cover the basics — PreGeneration, PostGeneration, PreToolUse,
PostToolUse — but fall short of what Claude Code hooks offer. This plan closes
those gaps while keeping Dive's library-first, typed-Go-function design.

The changes are organized into three phases: high-impact capabilities that
unblock new agent patterns, mid-priority ergonomic improvements, and lower-
priority additions.

## Context

Dive's hook system is defined in `hooks.go` and invoked from `agent.go`.
Hooks are Go functions registered via `AgentOptions` and run within the
`CreateResponse` → `generate` → `executeToolCalls` call chain.

The generation loop in `agent.go:286-358` iterates up to
`toolIterationLimit + 1` times. Each iteration calls the LLM, checks for
tool calls, executes them (with PreToolUse/PostToolUse hooks), and loops.
The loop exits when the LLM returns no tool calls or the iteration limit
is hit.

## Phase 1: High-Impact Capabilities

### 1.1 Stop Hook (Continue/Block Pattern)

**Problem:** Claude Code's `Stop` hook can prevent the agent from stopping and
make it continue working. Dive's `PostGenerationHook` runs after the loop is
done and cannot restart it. This blocks autonomous patterns like "keep working
until all tests pass."

**Design:**

Add a new `StopHook` type and `Stop` field to `AgentOptions`:

```go
// StopContext provides context when the agent is about to stop.
type StopContext struct {
    // Response is the response about to be returned.
    Response *Response

    // OutputMessages contains messages generated during this response.
    OutputMessages []*llm.Message

    // Usage contains token usage statistics.
    Usage *llm.Usage

    // StopHookActive is true when this stop check was triggered by a
    // previous stop hook continuation. Check this to prevent infinite loops.
    StopHookActive bool

    // Agent is the agent that is about to stop.
    Agent *Agent
}

// StopDecision tells the agent what to do after a stop hook runs.
type StopDecision struct {
    // Continue, when true, prevents the agent from stopping.
    // The Reason is injected as a user message so the LLM knows
    // why it should keep going.
    Continue bool

    // Reason is required when Continue is true. It's added to the
    // conversation as context for the next LLM iteration.
    Reason string
}

type StopHook func(ctx context.Context, hookCtx *StopContext) (*StopDecision, error)
```

**Integration in `agent.go`:** After the generation loop exits (line ~358),
before running PostGeneration hooks, run Stop hooks. If any returns
`Continue: true`, inject a user message with the reason and re-enter the
generation loop. Set `StopHookActive = true` on subsequent stop checks.

```
generate loop:
  ... (existing loop) ...

  // After loop exits:
  run Stop hooks
  if any says Continue:
    append reason as user message
    set stopHookActive = true
    goto top of generate loop
  else:
    proceed to PostGeneration hooks
```

Guard against infinite loops: if `StopHookActive` is already true and a hook
returns Continue again, allow it (the hook author is responsible for checking
`StopHookActive` to break cycles, mirroring Claude Code's `stop_hook_active`
field).

**AgentOptions addition:**

```go
// Stop hooks run when the agent is about to finish responding.
// A hook can prevent stopping by returning a StopDecision with Continue: true.
Stop []StopHook
```

### 1.2 Input Modification in PreToolUse

**Problem:** Claude Code's PreToolUse hooks can return `updatedInput` to
rewrite tool arguments before execution. Dive's PreToolUse hooks can only
allow or deny.

**Design:**

Add an `UpdatedInput` field to `PreToolUseContext`:

```go
type PreToolUseContext struct {
    Tool  Tool
    Call  *llm.ToolUseContent
    Agent *Agent

    // UpdatedInput, when set by a hook, replaces Call.Input before the
    // tool is executed. Only the last hook's UpdatedInput takes effect.
    // The hook is responsible for producing valid JSON for the tool's schema.
    UpdatedInput []byte
}
```

**Integration in `agent.go:executeToolCalls`:** After all PreToolUse hooks
run without error, check if `hookCtx.UpdatedInput` is non-nil. If so, use it
instead of `toolCall.Input` when calling `executeTool`.

```go
if !denied {
    input := toolCall.Input
    if hookCtx.UpdatedInput != nil {
        input = hookCtx.UpdatedInput
    }
    result = a.executeTool(ctx, tool, toolCall, input, preview)
}
```

This is backwards-compatible: existing hooks that don't set `UpdatedInput`
behave identically.

### 1.3 Context Injection from Tool Hooks

**Problem:** Claude Code hooks can inject `additionalContext` that gets added
to the LLM's context. Dive hooks can't add messages mid-loop.

**Design:**

Add an `AdditionalContext` field to both `PreToolUseContext` and
`PostToolUseContext`:

```go
// AdditionalContext, when set by a hook, is appended as a text content
// block to the tool result message sent to the LLM. This lets hooks
// provide guidance without modifying the tool result itself.
AdditionalContext string
```

**Integration:** After executing the tool (or after deny), if
`AdditionalContext` is non-empty on either the pre or post context, append
it as a `TextContent` block to the tool result message. This rides the
existing message flow without creating new messages.

## Phase 2: Ergonomic Improvements

### 2.1 Tool Name Matcher

**Problem:** Hooks that only care about specific tools must self-filter with
boilerplate like `if hookCtx.Tool.Name() != "Bash" { return nil }`.

**Design:**

Add matcher helper constructors that wrap existing hook functions:

```go
// MatchTool returns a PreToolUseHook that only runs when the tool name
// matches the given pattern. The pattern is a Go regexp.
func MatchTool(pattern string, hook PreToolUseHook) PreToolUseHook {
    re := regexp.MustCompile(pattern)
    return func(ctx context.Context, hookCtx *PreToolUseContext) error {
        if !re.MatchString(hookCtx.Tool.Name()) {
            return nil
        }
        return hook(ctx, hookCtx)
    }
}

// MatchToolPost is the PostToolUse equivalent.
func MatchToolPost(pattern string, hook PostToolUseHook) PostToolUseHook {
    re := regexp.MustCompile(pattern)
    return func(ctx context.Context, hookCtx *PostToolUseContext) error {
        if !re.MatchString(hookCtx.Tool.Name()) {
            return nil
        }
        return hook(ctx, hookCtx)
    }
}
```

Usage:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    PreToolUse: []dive.PreToolUseHook{
        dive.MatchTool("Bash|Edit|Write", func(ctx context.Context, hookCtx *dive.PreToolUseContext) error {
            // Only runs for Bash, Edit, or Write tools
            return nil
        }),
    },
})
```

This is purely additive — it doesn't change the hook type signatures. The
`pattern` argument is compiled once at construction time, not per invocation.

### 2.2 Pre-Iteration Hook

**Problem:** Dive's PreGeneration runs once before the loop starts. There's no
hook that fires before each individual LLM call within the iterative tool-use
loop. This matters for use cases like dynamic system prompt updates, token
budget checks, or injecting context that depends on tool results from the
previous iteration.

**Design:**

Add a `PreIterationHook` type:

```go
// PreIterationContext provides context before each LLM call in the
// generation loop.
type PreIterationContext struct {
    // Iteration is the zero-based iteration number within the loop.
    Iteration int

    // SystemPrompt can be modified to change the prompt for this iteration.
    SystemPrompt *string

    // Messages is the current message list. Hooks can append messages
    // but should not remove or reorder existing ones.
    Messages []*llm.Message

    // Agent is the agent running the loop.
    Agent *Agent
}

type PreIterationHook func(ctx context.Context, hookCtx *PreIterationContext) error
```

**Integration:** At the top of each loop iteration in `agent.go:generate`,
before calling the LLM, run PreIteration hooks. Errors abort generation
(same as PreGeneration).

**AgentOptions addition:**

```go
// PreIteration hooks run before each LLM call within the generation loop.
// Use these to modify the system prompt or messages between iterations.
PreIteration []PreIterationHook
```

### 2.3 PostToolUse Failure Distinction

**Problem:** Claude Code separates `PostToolUse` (success) from
`PostToolUseFailure` (failure). Dive fires `PostToolUseHook` for both.

**Design:**

Rather than adding a separate hook type, add a convenience field to
`PostToolUseContext`:

```go
type PostToolUseContext struct {
    Tool   Tool
    Call   *llm.ToolUseContent
    Result *ToolCallResult
    Agent  *Agent

    // Failed is true when the tool execution returned an error.
    // Use this to distinguish success from failure without inspecting
    // Result.Error directly.
    Failed bool
}
```

And add a matcher helper:

```go
// OnToolFailure returns a PostToolUseHook that only runs when the tool
// call failed.
func OnToolFailure(hook PostToolUseHook) PostToolUseHook {
    return func(ctx context.Context, hookCtx *PostToolUseContext) error {
        if !hookCtx.Failed {
            return nil
        }
        return hook(ctx, hookCtx)
    }
}

// OnToolSuccess returns a PostToolUseHook that only runs when the tool
// call succeeded.
func OnToolSuccess(hook PostToolUseHook) PostToolUseHook {
    return func(ctx context.Context, hookCtx *PostToolUseContext) error {
        if hookCtx.Failed {
            return nil
        }
        return hook(ctx, hookCtx)
    }
}
```

This avoids a new hook type while providing the filtering Claude Code gets
from separate events. Set `Failed = true` in `executeToolCalls` when the tool
returns an error or when `result.Result.IsError` is true.

## Phase 3: Lower Priority

### 3.1 Prompt-Based Hook Helper

**Problem:** Claude Code's `type: "prompt"` hooks delegate decisions to an
LLM. Dive users can build this themselves, but a built-in helper reduces
boilerplate.

**Design:**

```go
// PromptBasedPreToolUse returns a PreToolUseHook that consults an LLM
// to decide whether a tool call should proceed.
//
// The promptTemplate receives the tool name and input as context.
// The LLM must respond with JSON: {"ok": true} or {"ok": false, "reason": "..."}.
//
// Example:
//
//     dive.PromptBasedPreToolUse(model, "Should this {{.ToolName}} call proceed? Input: {{.Input}}")
//
func PromptBasedPreToolUse(model llm.LLM, promptTemplate string) PreToolUseHook
```

Similarly for Stop hooks:

```go
func PromptBasedStop(model llm.LLM, promptTemplate string) StopHook
```

These are convenience constructors built on top of the core hook types. They
parse the LLM response and return the appropriate error/decision. The `model`
parameter lets callers choose a fast/cheap model for evaluation.

This is a lower priority because the building blocks are straightforward for
users to assemble. It becomes more valuable once Stop hooks exist (Phase 1.1).

### 3.2 SessionEnd / Cleanup Hook

**Problem:** No hook fires when the agent is done with its lifecycle. For
library users, this is less critical than for CLI users (callers control
the lifecycle), but it's useful for resource cleanup patterns.

**Design:**

This is a documentation/pattern recommendation rather than a new hook type.
Since Dive is a library, callers already control what happens after
`CreateResponse` returns. A `defer` block or `PostGeneration` hook serves
this purpose. Document this pattern in the hooks guide.

If demand warrants it later, a `CleanupHook` could be added to `AgentOptions`
that runs in a `defer` inside `CreateResponse`, guaranteed to execute even on
errors.

## Implementation Order

```
Phase 1 (DONE — unblocks new agent patterns):
  1.1 Stop Hook ............... hooks.go, agent.go        ✓
  1.2 PreToolUse Input Mod .... hooks.go, agent.go        ✓
  1.3 Context Injection ....... hooks.go, agent.go        ✓

Phase 2 (DONE — ergonomic wins):
  2.1 Tool Name Matcher ....... hooks.go                  ✓
  2.2 Pre-Iteration Hook ...... hooks.go, agent.go        ✓
  2.3 PostToolUse Failure ..... hooks.go, agent.go        ✓

  Additional (unified design):
  - Unified HookContext ........ hooks.go                  ✓
  - Hooks struct on AgentOptions agent.go                  ✓

Phase 3 (do later — nice to have):
  3.1 Prompt-Based Helper ..... hooks.go (new file)
  3.2 Cleanup Documentation ... docs/guides/
```

## Files Changed

| File | Changes |
|:--|:--|
| `hooks.go` | Unified `HookContext` replacing `GenerationState`, `PreToolUseContext`, `PostToolUseContext`. New types: `StopHook`, `StopDecision`, `PreIterationHook`, `PostToolUseFailureHook`. New fields: `UpdatedInput`, `AdditionalContext`, `StopHookActive`, `Iteration`. New helpers: `MatchTool`, `MatchToolPost`, `MatchToolPostFailure`. Type aliases for backwards compat. |
| `agent.go` | `Hooks` struct on `AgentOptions` replacing 4 separate hook fields. Agent stores `hooks Hooks`. `CreateResponse` implements stop hook loop. `generate` runs PreIteration hooks. `executeToolCalls` handles UpdatedInput, Failed, AdditionalContext. |
| `tool.go` | `AdditionalContext` field on `ToolCallResult`. |
| `hooks_test.go` | Tests for all new hook types and helpers. |
| `agent_test.go` | Updated to use `Hooks` struct. |
| `experimental/session/hooks.go` | Updated to use `*dive.HookContext`. |
| `experimental/compaction/hooks.go` | Updated to use `*dive.HookContext`. |
| `experimental/permission/hooks.go` | Updated to use `*dive.HookContext`. |
| `experimental/cmd/dive/main.go` | Updated to use `Hooks` struct. |

## Backwards Compatibility

All changes are additive:

- Type aliases (`GenerationState = HookContext`, `PreToolUseContext = HookContext`,
  `PostToolUseContext = HookContext`) preserve compatibility for code referencing old types.
- `NewGenerationState()` is preserved as a deprecated wrapper around `NewHookContext()`.
- New fields on `HookContext` have zero values that preserve current behavior
  (`UpdatedInput nil` = no modification, `AdditionalContext ""` = no injection,
  `Failed false` = existing behavior for hooks that don't check it).
- New hook slices on `Hooks` struct default to nil (no hooks).
- New helper functions (`MatchTool`, `OnToolFailure`, etc.) are optional
  wrappers — they don't change hook type signatures.

## Design Decisions

**Why a unified `HookContext` instead of separate context types?**
The original design had separate `GenerationState`, `PreToolUseContext`, and
`PostToolUseContext` types. The unified `HookContext` shares a single `Values`
map across all phases, allows hooks to access any field they need, and
simplifies the API. Type aliases (`GenerationState = HookContext`, etc.)
maintain backwards compatibility.

**Why a `Hooks` struct instead of individual fields on `AgentOptions`?**
Grouping all hook slices into a single `Hooks` struct keeps `AgentOptions`
clean as hook types grow (now 6 types). It also makes it easy to compose
hook sets from different packages.

**Why a separate `StopHook` type instead of reusing `PostGenerationHook`?**
PostGeneration hooks signal completion and run cleanup. Stop hooks control
flow. Mixing these concerns in one type would force all PostGeneration hooks
to return a decision they don't care about. Separate types keep the
interfaces clean.

**Why `UpdatedInput []byte` on the context instead of a return value?**
PreToolUseHook's signature returns `error`. Changing it to return
`([]byte, error)` would break all existing hooks. Mutating a field on the
context struct is the established pattern (see `HookContext.SystemPrompt`).

**Why separate `PostToolUseHook` and `PostToolUseFailureHook`?**
This matches Claude Code's separate `PostToolUse` and `PostToolUseFailure`
events. Separate types make intent clear: success hooks handle logging and
result processing, failure hooks handle error recovery and diagnostics.
The agent dispatches to the correct hook list based on the tool call outcome.

**Why not run PreToolUse hooks in parallel?**
Sequential execution lets later hooks see the effects of earlier hooks (e.g.,
one hook sets `UpdatedInput`, the next validates it). Parallel execution would
require a merge strategy. Sequential is simpler and matches the current
design.
