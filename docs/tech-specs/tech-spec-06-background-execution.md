# Background Tool Execution

**Status:** Draft
**Author:** Curtis Myzie
**Date:** 2026-05-04
**Workflow:** Spec → build in parallel. Design is settled; a few hook/OTel details below need a second look before those pieces land.
**Reviewers:** —
**PRD:** [prd-06-background-execution.md](../prds/prd-06-background-execution.md)

---

## Context

Dive's tool execution is fully synchronous: `executeOneToolCall` runs the tool, blocks until it returns, and the agent loop feeds the result straight to the LLM. This is fine for fast tools but makes Dive the wrong fit for anything that takes seconds or minutes — test runners, deployment watchers, subagent dispatches, log tailers.

The PRD is clear on scope: opt-in by returning a new `*ToolResult` variant; no magic timeouts; no changes to the synchronous path. The design here stays close to the existing Suspend/Resume mechanism, which already handles the "tool doesn't return a final result right now" case at the agent-loop level.

---

## Goals

- Tools can start async work and return immediately; the agent loop sends a readable "started" message to the LLM as the tool result.
- Completed background results are injectable into the next `CreateResponse` call via a single option.
- No goroutine or channel management is required from tool authors or callers.
- Panic in a background function is recovered and surfaced as an error result — not a silent crash.
- PostToolUse and a new PostBackgroundToolUse hook fire in the main agent goroutine (never from a background goroutine).
- OTel spans opened at tool-call time close correctly when the background result arrives.
- No behavior change for any tool that does not return `BackgroundResult`.

## Non-goals

- Auto-backgrounding based on elapsed time.
- Task registry on the `Agent` struct.
- Built-in polling or Monitor tool.
- Cross-turn or cross-process persistence of background handles.
- Partial delivery (delivering results for some tasks before others).
- Mixing Suspend and Background on a single `ToolResult`.

---

## Proposal

### New types

**`background.go`** — new file in the core `dive` package.

```go
// BackgroundResult is the internal state carried on ToolResult.Background.
// Tool authors do not construct this directly; they use NewBackgroundResult or
// NewBackgroundResultFull.
type BackgroundResult struct {
    id          string          // generated UUID
    description string
    done        chan *ToolResult // buffered, cap 1 — the goroutine sends exactly once
}

// BackgroundTaskHandle is the caller-facing handle returned on Response.BackgroundTasks.
type BackgroundTaskHandle struct {
    TaskID      string
    ToolUseID   string         // the llm tool_use block ID
    Description string
    Done        <-chan *ToolResult
    hookCtx     *HookContext   // kept for PostBackgroundToolUse; unexported
}
```

**`tool.go`** — add one field to `ToolResult`, parallel to the existing `Suspend` field:

```go
type ToolResult struct {
    Content    []*ToolResultContent
    Display    string
    IsError    bool
    Suspend    *SuspendResult
    Background *BackgroundResult  // new
}
```

`Background` and `Suspend` are mutually exclusive. `executeTool()` at line 2077 already validates the `Suspend/result` invariant (M3) — extend that check to Background.

**`response.go`** — add one field to `Response`:

```go
type Response struct {
    // ... existing fields unchanged ...
    BackgroundTasks []*BackgroundTaskHandle // nil when no background tasks
}
```

**`hooks.go`** — new hook type and slot on `Hooks`:

```go
type PostBackgroundToolUseHook func(ctx context.Context, hctx *HookContext) error

type Hooks struct {
    // ... existing fields unchanged ...
    PostBackgroundToolUse []PostBackgroundToolUseHook
}
```

### Constructor functions

```go
// NewBackgroundResult is the primary constructor. fn receives a context derived
// from the tool's execution context; cancellation propagates.
func NewBackgroundResult(
    ctx context.Context,
    description string,
    fn func(ctx context.Context) (string, error),
) *ToolResult

// NewBackgroundResultFull is for tools that need full control over the result
// (IsError, Display, structured content).
func NewBackgroundResultFull(
    ctx context.Context,
    description string,
    fn func(ctx context.Context) *ToolResult,
) *ToolResult
```

Both constructors:
1. Generate a UUID for the task ID.
2. Create a buffered channel of capacity 1.
3. Spawn a goroutine with `defer func() { if r := recover(); r != nil { ... } }()` — panic → `NewErrorResult("background task panicked: %v\n%s", r, debug.Stack())`.
4. The goroutine sends its result to the channel exactly once, then exits.
5. Return a `*ToolResult` with `Background` set and all other fields zero.

Because the channel is buffered (cap 1), the goroutine can always send without blocking, even if no caller ever reads. No leak.

### Agent loop changes (`agent.go`)

The changes are localized to `executeOneToolCall` and the `generate` loop's result-collection phase.

**In `executeOneToolCall`:**

After `executeTool()` returns and the result has `Background != nil`:

1. Fire PostToolUse hooks with a synthesized `*ToolResult` whose text content is the "started" message (FR-5). This is what gets added to the conversation as the tool_result block.
2. Build a `BackgroundTaskHandle` from `result.Background`, attaching the current `ToolUseID` and a copy of the `HookContext` (so PostBackgroundToolUse can be fired later with proper context).
3. Return the synthesized "started" result (not the empty `Background` carrier) as the tool_result content going to the LLM.

The "started" message text (stable, may appear in system prompts):
```
Background task started: <description>
Task ID: <id>
The result will be delivered in a follow-up message.
```

**In `generate` / `executeToolCalls`:**

After a full tool batch completes, collect all `BackgroundTaskHandle`s from the batch. Attach them to the `generateResult` (or accumulate them into a slice that `CreateResponse` picks up). `CreateResponse` sets `resp.BackgroundTasks` from this slice at the end.

### PostBackgroundToolUse hook

The hook fires when background results are _delivered_ to the agent — not when the goroutine finishes. This guarantees it always runs on the main agent goroutine.

The delivery path:

1. Caller awaits results via `AwaitBackgroundTasks`.
2. Caller passes results into the next `CreateResponse` via `WithBackgroundResults`.
3. At the start of `generate`, the agent iterates over the injected results, fires `PostBackgroundToolUse` for each using the `hookCtx` stored on `BackgroundTaskHandle`, then discards the hook context.

The `HookContext` stored on the handle carries the original tool call's values (tool name, input, OTel span context). The OTel tracer's span is already threaded through `ctx` at `StartToolCall` time, so the hook closes it by calling `span.End()` on the context retrieved from `hookCtx`.

### Caller-facing helpers (`background.go`)

```go
// AwaitBackgroundTasks blocks until all tasks deliver results or ctx is cancelled.
// Returns partial results plus ctx.Err() on cancellation.
// Background goroutines continue running after cancellation; results remain on Done.
func AwaitBackgroundTasks(
    ctx context.Context,
    tasks []*BackgroundTaskHandle,
) (map[string]*ToolResult, error)

// WithBackgroundResults injects completed background results as a synthetic
// user message at the start of the next CreateResponse call.
func WithBackgroundResults(results map[string]*ToolResult) CreateResponseOption

// ContinueWithBackground is the convenience wrapper for the common interactive loop.
// Returns resp unchanged (without calling CreateResponse again) if BackgroundTasks is empty.
func ContinueWithBackground(
    ctx context.Context,
    agent *Agent,
    resp *Response,
    opts ...CreateResponseOption,
) (*Response, error)
```

`WithBackgroundResults` builds a synthetic user message injected before the LLM's next turn. Format for multiple tasks:

```
The following background tasks have completed:

Background task completed: <description>
Task ID: <id>
Result:
<result text>

Background task completed: <description>
Task ID: <id>
Error:
<error text>
```

Single task omits the introductory line and the blank-line framing.

### Synthetic message injection

`WithBackgroundResults` sets a `backgroundResults` field on the internal `callOpts` struct (following the same pattern as `WithResume`). In `generate`, if `callOpts.backgroundResults` is non-nil, a user-role message with the formatted text is prepended to the input messages before the first LLM call. The `PostBackgroundToolUse` hooks fire immediately after this message is constructed, before `generate` proceeds to the LLM call.

### File layout

| File | Change |
|------|--------|
| `background.go` | New file. `BackgroundResult`, `BackgroundTaskHandle`, constructors, `AwaitBackgroundTasks`, `ContinueWithBackground`, `WithBackgroundResults`, message formatting. |
| `tool.go` | Add `Background *BackgroundResult` to `ToolResult`. Extend M3 invariant check. |
| `response.go` | Add `BackgroundTasks []*BackgroundTaskHandle` to `Response`. |
| `hooks.go` | Add `PostBackgroundToolUseHook` type and `PostBackgroundToolUse []PostBackgroundToolUseHook` to `Hooks`. |
| `agent.go` | `executeOneToolCall`: detect Background, substitute "started" message, build handle. `generate`: accumulate handles, pass to `CreateResponse`. `callOpts`: add `backgroundResults` field. Pre-generate: inject synthetic message and fire PostBackgroundToolUse hooks. |
| `experimental/cmd/dive/app.go` | Post-`CreateResponse`: iterate `resp.BackgroundTasks`, spawn goroutine per handle reading `Done`, call `scheduleBackgroundFlush()` on completion. |

---

## Alternatives Considered

**Reuse the Suspend/Resume mechanism.** Background results feel similar to suspend — the tool doesn't return a final result immediately. The difference: Suspend is driven by the _agent_ (the next call must supply results); Background is driven by a _goroutine_ (results arrive asynchronously). Reusing Suspend would require adding background-goroutine lifecycle management inside `SuspensionState`, complicating a feature already complex enough to have its own guide. Keeping them separate keeps both simpler.

**Return a channel directly from the tool.** Tools return `*ToolResult`; adding a channel there means every tool result reader has to check for nil channel. Wrapping the channel inside `BackgroundResult` (itself inside `ToolResult.Background`) makes the nil-check cheap and keeps the zero value safe for the 99% of tools that never use it.

**Auto-fire PostBackgroundToolUse from the background goroutine.** Simpler plumbing, but hooks must not run concurrently with the agent loop and the requirement to close OTel spans from the same goroutine that opened them makes this fragile. Deferring to delivery (in the next `CreateResponse`) costs one extra turn but keeps the hook execution model uniform.

---

## Tradeoffs and Consequences

**Caller must loop.** `ContinueWithBackground` returns a `*Response` that may itself have `BackgroundTasks`. The caller must loop until the response has no pending tasks. This is correct behavior (background tasks can start more background tasks) but adds a small obligation for callers who don't use `ContinueWithBackground`.

**HookContext must be kept alive.** `BackgroundTaskHandle` holds a reference to the original `HookContext`, which in turn holds a reference to the tool call's context (including OTel span). This keeps those objects alive until the handle is GC'd or the background result is delivered. For typical use (results arrive within minutes), this is negligible. For handles that are created and then dropped without reading `Done`, the objects stay live until the goroutine completes and the channel is GC'd — still bounded, not a leak, but worth documenting.

**LLM must cooperate.** The "started" message tells the LLM a result is coming. A model that ignores this and keeps generating tool calls to check status would produce incorrect behavior. System prompt guidance helps; the PRD notes this risk. The format text is stable so it can appear in system prompts.

**No partial delivery.** If a caller has 10 background tasks and 9 finish quickly and 1 stalls, `AwaitBackgroundTasks` blocks on the slow one. Partial delivery is explicitly a non-goal. Callers who need partial delivery can read `handle.Done` channels directly.

---

## Rollout

Additive. No existing behavior changes. New types and functions are addable without breaking existing callers. The `Background` field on `ToolResult` is nil by default — the agent loop touches it only when non-nil.

No migration. No feature flag. Ship when tests pass.

CLI integration (`app.go`) is the first consumer and serves as the integration test harness.

---

## Open Questions

1. **Background + Suspend in the same turn.** The PRD makes them mutually exclusive per `ToolResult`, but a single turn could have one tool suspend and a different tool background. What happens? The agent loop currently returns when it sees `batch.Suspended = true` — does it still collect background handles before returning? Proposal: if any tool in the batch suspends, the entire turn suspends and any background tasks that started in the same batch are tracked on `SuspensionState` (or returned alongside suspension state). Needs a decision before building the batch-processing change.

2. **PostBackgroundToolUse hook error handling.** PostToolUse hook errors today affect `AdditionalContext` but don't abort the turn. What should a PostBackgroundToolUse error do? It fires in the next `CreateResponse` call, so aborting that call is one option; logging only is another. Proposal: log-only, consistent with PostGeneration hooks, since by the time this fires the background work is already done.

3. **OTel span lifetime in dropped handles.** If a caller drops a `BackgroundTaskHandle` without reading `Done`, the span stays open until the goroutine finishes and the channel is GC'd. This may show up as unclosed spans in OTel exporters. Consider whether `NewBackgroundResult` should close the span on goroutine completion (from the background goroutine) rather than deferring to a hook — or document the tradeoff explicitly for the otel package.
