# Suspend & Resume Guide

Suspend/resume lets an agent pause mid-turn while a tool is waiting on an
external input — a human approval, a webhook callback, a form submission,
a review that lands hours or days later — and resume cleanly without
holding a goroutine or losing state.

Suspending is a first-class result, not an error. The agent persists the
partial turn, `CreateResponse` returns `(*Response, nil)` with a suspended
status, and a later call (possibly from a different process) supplies the
missing tool result(s) to continue the turn.

## Mental model

A normal turn runs: user input → LLM → tool calls → tool results → LLM →
final assistant message. With suspend, a tool can short-circuit its call
and hand control back to the caller instead of returning a result:

```text
user input → LLM → tool A.Call → SuspendResult
                    ↓
                  partial turn persisted
                  Response{Status: Suspended, Suspension: {...}}
                    ↓
                  (hours/days pass, new process, webhook arrives)
                    ↓
  CreateResponse(WithToolResults{A.ID: "...answer..."})
                    ↓
                  LLM resumes with a complete tool_result → final message
```

A suspended response is not a failure. It is a deliberate pause with a
structured payload describing what the agent is waiting for. The LLM
never sees an unmatched `tool_use` — the agent only talks to the LLM
once every pending call has a real `tool_result`.

## Suspending from a tool

A tool author signals suspension by returning `NewSuspendResult`. Pass a
human-readable prompt plus optional metadata for the integrator's UI:

```go
func deployTool() dive.Tool {
    return dive.FuncTool("deploy",
        "Deploys the application. Requires human approval.",
        func(ctx context.Context, in *DeployInput) (*dive.ToolResult, error) {
            return dive.NewSuspendResult(
                fmt.Sprintf("Approve deploy of %s to %s?", in.Version, in.Environment),
                map[string]any{
                    "request_id":  newRequestID(),
                    "environment": in.Environment,
                    "version":     in.Version,
                },
            ), nil
        })
}
```

Contract:

- `SuspendResult.Prompt` is optional human-readable context, surfaced on
  `PendingToolCall.Prompt`.
- `SuspendResult.Metadata` is opaque to Dive and round-trips through the
  session. Values are JSON-marshalled; numbers come back as `float64`
  and custom structs become `map[string]any`, so stick to JSON-friendly
  values.
- `ToolResult` is a tagged union — set **either** the regular fields
  (`Content`, `Display`, `IsError`) **or** `Suspend`, never both. The
  agent treats a mixed result as an error routed through
  `PostToolUseFailure`. Use `NewSuspendResult` to stay on the safe side.
- `PreToolUse` fires normally for a suspending tool. `PostToolUse` and
  `PostToolUseFailure` do **not** fire — suspend is neither success nor
  failure.

## Observing a suspended response

The caller checks `Response.Status` and reads `Response.Suspension`:

```go
resp, err := agent.CreateResponse(ctx, dive.WithInput("Please deploy v1.4.2 to production."))
if err != nil {
    return err
}
if resp.Status != dive.ResponseStatusSuspended {
    fmt.Println(resp.OutputText())
    return nil
}

for _, pending := range resp.Suspension.PendingToolCalls {
    var args DeployInput
    if err := pending.UnmarshalInput(&args); err != nil {
        return err
    }
    // Route pending.ID + args + pending.Prompt/Metadata to wherever you
    // collect external input (email, Slack, DB row, webhook queue).
}
// Return from your handler — no goroutines held.
```

`Response.Suspension` is a `*SuspensionState` carrying:

| Field                | Purpose |
| -------------------- | ------- |
| `PendingToolCalls`   | Tool calls awaiting external results. Contains ID, name, input JSON, prompt, metadata. |
| `CompletedToolCalls` | Sibling tool calls that ran to completion in the same iteration as the suspender (parallel execution). Informational — their results are already merged into the persisted turn. |
| `TurnMessages`       | Snapshot of the in-progress turn (user input + any assistant/tool_result messages produced so far). Stateless callers pass this back via `WithResume` to reconstruct the conversation. |

Decode a pending call's input with either the method or the generic
helper:

```go
var args DeployInput
err := pending.UnmarshalInput(&args)

args, err := dive.DecodePendingInput[DeployInput](pending)
```

## Resuming with `WithToolResults`

Session-backed callers supply the results map and nothing else — the
agent reads the stored `SuspensionState` from the session:

```go
results := map[string]*dive.ToolResult{
    pendingID: dive.NewToolResultText("approved by alice"),
}
final, err := agent.CreateResponse(ctx, dive.WithToolResults(results))
```

The agent:

1. Loads the suspended turn from the session.
2. Validates every key in `results` against the pending set
   (`ErrUnknownPendingToolCall` if any ID is not pending).
3. Splices the caller's results into the partial `tool_result` message.
4. Runs `PostToolUseFailure` for any `IsError: true` result.
5. Continues the generation loop from the next LLM call.

Pass an `IsError` result to signal that the external answer was a
rejection, failure, or abandonment:

```go
results[pendingID] = dive.NewToolResultError("approval denied")
```

The LLM sees the error result through the normal failure path and
decides how to react. This is also how you cancel a suspended turn:
supply an `IsError` result for every pending call.

## Resuming without a session (stateless)

Callers who manage history themselves use `WithResume` to hand back the
`SuspensionState` alongside the results:

```go
resp, err := agent.CreateResponse(ctx,
    dive.WithMessages(preHistory...),  // everything before this turn
    dive.WithResume(saved, results),   // saved is the prior resp.Suspension
)
```

`WithResume` overrides any session-stored state, which is also what you
want when doing a cross-process handoff where the resumer holds a newer
snapshot than what is on disk.

On completion, the agent populates `resp.Suspension` a second time —
this time with `PendingToolCalls == nil` and `TurnMessages` holding the
**final merged turn**. Stateless callers flush in one append:

```go
if resp.Suspension != nil && len(resp.Suspension.PendingToolCalls) == 0 {
    preHistory = append(preHistory, resp.Suspension.TurnMessages...)
    saved = nil
}
```

No reconciliation of a stale partial `tool_result` from the caller's
saved state — the agent hands back the merged view.

## Partial resume

A suspended turn may hold multiple pending calls (parallel tool execution
where several tools suspend at once). The caller can satisfy them one at
a time: pass results for a subset, and the agent returns a new suspended
`Response` listing only the still-pending calls.

```go
resp, _ := agent.CreateResponse(ctx,
    dive.WithToolResults(map[string]*dive.ToolResult{
        pendingA: dive.NewToolResultText("ack from alpha team"),
        // B and C intentionally omitted — still pending.
    }))

// resp.Status is still ResponseStatusSuspended.
// resp.Suspension.PendingToolCalls has B and C.
```

`OnSuspend` hooks do **not** re-fire on a partial resume — they only
announce new suspensions, not continuations.

## Parallel and sequential tool execution

With `AgentOptions.ParallelToolExecution = true`, sibling tools keep
running when one suspends. Their results are collected into the
persisted turn and exposed on `Suspension.CompletedToolCalls` for
informational use by a UI. `PostToolUse` / `PostToolUseFailure` fire
normally for the siblings.

With sequential execution (the default), a suspending tool stops the
batch: earlier tools keep their results, later tools in the batch are
**not** started. On resume, once the suspended call is satisfied, the
agent runs the remaining sequential tools before the next LLM call.

## The `OnSuspend` hook

`OnSuspend` fires when the agent transitions into a suspended state,
**before** `PostGeneration` and **before** the session is persisted. Use
it to dispatch webhooks, send emails, create review tasks:

```go
func webhookNotifier(ctx context.Context, hctx *dive.HookContext) error {
    if hctx.Response == nil || hctx.Response.Suspension == nil {
        return nil
    }
    for _, p := range hctx.Response.Suspension.PendingToolCalls {
        payload := map[string]any{
            "webhook_url": "https://example.com/webhooks/tool-result",
            "tool_call":   p,
        }
        if err := postJSON(ctx, payload); err != nil {
            return err // aborts persistence — caller sees the error
        }
    }
    return nil
}

agent, _ := dive.NewAgent(dive.AgentOptions{
    Model: anthropic.New(),
    Tools: []dive.Tool{deployTool()},
    Hooks: dive.Hooks{OnSuspend: []dive.OnSuspendHook{webhookNotifier}},
})
```

Because the hook runs before persistence, returning
`dive.AbortGeneration("...")` (or any error on the critical path) aborts
the transition: the caller sees an error and the session stays in its
previous state. No compensating rollback needed.

`PostGeneration` still runs on suspended responses with
`Status == ResponseStatusSuspended`, so existing hook authors (metrics,
logging) see one consistent end-of-turn signal.

## Session flag and `List` filter

`SuspendableSession` is an optional extension implemented by the core
`session.Session`. It adds `LoadSuspension`, `SaveSuspendedTurn`, and
`SaveResumedTurn`. A plain `Session` — or no session at all — still
supports suspend/resume; the caller just manages the state directly.

Stores report the flag on `SessionInfo.Suspended` and accept a filter
so you can sweep for stale suspended sessions:

```go
infos, _ := store.List(ctx, &session.ListOptions{
    Suspended: dive.Ptr(true),
})
for _, info := range infos {
    // ... cancel or reap a suspended session that has been idle too long
}
```

To cancel a stale suspension, open the session, load its state, and
resume with `IsError` results for every pending call.

## Streaming

`WithEventCallback` receives events up to the suspend point normally.
After the suspending iteration, the agent emits a terminal
`ResponseItemTypeSuspended` item whose `Suspended` field carries the
same `PendingToolCalls` and `CompletedToolCalls` as
`Response.Suspension`:

```go
dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
    if item.Type == dive.ResponseItemTypeSuspended {
        for _, p := range item.Suspended.PendingToolCalls {
            // ... route pending call to a waiting queue
        }
    }
    return nil
})
```

Stream consumers should treat this item as end-of-stream and then
observe `Response.Status` and `Response.Suspension` on the final return.

## Errors

| Error                           | Cause |
| ------------------------------- | ----- |
| `ErrResumeRequired`             | `CreateResponse` was called on a suspended session without `WithResume`, `WithToolResults`, or new input. Resume is explicit — no silent no-op polling. |
| `ErrInputOnSuspendedSession`    | New user input was supplied while the session is suspended. Resume the current turn first. |
| `ErrNoSuspendedTurn`            | `WithResume` or `WithToolResults` was supplied but there is no suspended turn to resume. |
| `ErrUnknownPendingToolCall`     | A key in `WithToolResults` is not in the pending set. No state changes. |

## Concurrency

`CreateResponse` serializes calls that share a `Session.ID()` using an
in-process per-ID lock, so two goroutines or two agents hitting the same
session run one after the other rather than interleaving. This is an
in-process guarantee only. For multi-process deployments that share a
session, use a backend with its own serialization — `FileStore` is
documented as single-writer-per-session and does not take an OS-level
lock.

Stateless callers (no session) skip the lock entirely; coordination is
their responsibility.

## Timeouts

`AgentOptions.ResponseTimeout` applies only to active execution. When
`CreateResponse` returns a suspended response, the timer is released.
Each subsequent resume call starts with a fresh budget — a multi-day
external wait does not consume the timeout.

## Model and system prompt updates between suspend and resume

`SetModel` and `SetSystemPrompt` between suspend and resume are honored.
`CreateResponse` takes a fresh snapshot of the agent's model and system
prompt at the top of every call, including on resume.

## Subagents

In the current version, tools running inside `experimental/subagent`
cannot suspend — a subagent call that tries to return `SuspendResult`
surfaces as an error. Cross-boundary nested suspend is deferred until a
concrete use case lands.

## Worked examples

Runnable examples live in `examples/suspend/`:

- `human_approval/` — synchronous human approval dialog, single session.
- `async_webhook/` — cross-process suspend/resume using `FileStore` and
  the `OnSuspend` webhook dispatch pattern.
- `partial_resume/` — parallel tool execution with multiple pending
  calls resumed one at a time.
- `stateless/` — suspend/resume with no session at all; history is
  stored on the caller's side and handed back via `WithResume`.

The `dialogspec/` directory next to them is a shared helper package the
examples use to serialize a "what the caller should ask the human"
payload through `SuspendResult.Metadata`.

## Next steps

- [Agents Guide](agents.md) — Full agent configuration reference.
- [Custom Tools](custom-tools.md) — How to build tools that can suspend.
- [Hooks Guide](hooks.md) — Hook ordering and the `OnSuspend` hook.
