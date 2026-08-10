---
Title: Recover from unknown tool names instead of failing the generation
Author: Curtis Myzie
Status: Implemented
Last Updated: 2026-08-10
---

# Recover from unknown tool names instead of failing the generation

**Workflow:** Standard-tier spec, Dive team feedback, then implementation.

## Context

When a model emits a tool name that is not in the agent's declared toolset,
Dive returns a Go error from the tool-dispatch path. That error unwinds the
whole `CreateResponse` call. The turn ends, every tool call in the same
assistant message is discarded, and the caller receives a generation failure
with no transcript continuation.

This is the only tool-related failure in Dive that behaves this way. Every
other one becomes an error result the model can read and adapt to.

On 2026-08-09, two production sessions in a downstream deployment failed this
way within eighteen hours of each other, both on `claude-sonnet-5`. Each turn
died in roughly thirteen seconds. (Some identifiers in this document are
anonymized for publication; the shape and structure of the names are
preserved.)

Both recorded the same failure:

```text
model="claude-sonnet-5" route_mode=managed detected_provider=anthropic
llm=anthropic: tool call error: unknown tool "mobius_market_clock"
```

`mobius_market_clock` does not exist. The agent's toolkit grants
`core.market.clock`, which renders as the wire name `core_market_clock`. The
invented name is a blend: the `mobius_` prefix from one namespace with the
`market_clock` leaf from another.

The blend is easy to explain from the batch it appeared in. Both turns produced
the identical assistant message:

```text
tool_use  mobius_memory_recall     executed, result recorded
tool_use  mobius_memory_recall     executed, result recorded
tool_use  mobius_market_clock      does not exist -> turn failed
```

The model emitted `mobius_` twice successfully and then a third time with the
wrong leaf. It was priming on its own immediately preceding tokens.

The toolkit makes that likely. Of its 33 granted actions, 22 are `core.*`, 8
are `mobius.*` (`mobius.memory.*`, `mobius.artifacts.*`, `mobius.self.*`), and
3 are `firecrawl.*`. All are presented flat, so `mobius_` is not a foreign
prefix in that list. It is one the agent uses constantly.

The user-visible outcome was a blank response after thirteen seconds. The two
`mobius_memory_recall` calls had already executed and returned results; those
results were discarded along with the turn.

## Historical behavior and root cause

Before this implementation, two call sites returned a fatal error on an
unrecognized name:

- `agent.go:2497` in `executeOneToolCall` (sequential path)
- `agent.go:2248` in `executeToolCallsParallel`, Phase 1

Both used the same shape:

```go
tool, ok := toolsByName[toolCall.Name]
if !ok {
    return nil, fmt.Errorf("tool call error: unknown tool %q", toolCall.Name)
}
```

That error propagated through `executeToolCalls` to the generation loop, which
returned it from `CreateResponse`. No `tool_result` message was built, so the
batch produced nothing.

At the time, every other failure mode in the same file was recoverable:

| Failure                        | Handling                                |
| ------------------------------ | --------------------------------------- |
| `tool.Call` returns an error   | `IsError`, `"Tool execution error: %v"` |
| Tool panics                    | Recovered, converted to an error result |
| Tool returns a malformed union | `IsError` result so the agent converges |
| Tool name is unknown           | **Fatal, turn ends**                    |

The panic path stated the principle directly: results were converted "so the
LLM can see the failure and adapt, rather than crashing the process." A tool
that panicked was handled more gracefully than a name with a typo.

`batchHasSequentialOnlyTool` already treated an unknown name as a skippable,
non-fatal condition (`agent.go:2160`), so the file was not internally
consistent about this either.

### Not only a hallucination

The incident above was a model inventing a name, but fatal-on-unknown also
broke two supported configurations where the name was not invented at all:

- **Dynamic toolsets.** Tool resolution is per-iteration: the generation loop
  calls `resolveTools` (`agent.go:1912`) before every LLM request, and
  `Toolset` implementations are dynamic by design. A tool can legitimately
  disappear between iterations while its earlier calls remain in the
  transcript, and models re-call tools they have seen in context.
- **Resume.** `executeToolCalls` is also invoked when resuming a suspended
  turn (`agent.go:827`) against a freshly resolved toolset. If the toolset
  changed across suspend/resume, a pending call's name can be unknown by the
  time it runs, which caused the resume to fail.

Both were reasons this was a correctness fix, not just hallucination tolerance.

### Batch loss differed by execution mode

The two paths discarded work differently, and neither was good:

- **Parallel:** the name check ran in Phase 1 before any tool executed, so every
  valid call in the batch was discarded unrun. Pure wasted latency.
- **Sequential:** calls preceding the bad one executed to completion. Their side
  effects landed; their results were thrown away with the batch. The caller had
  no durable record that they ran.

Dive defaults to sequential (`ParallelToolExecution` is opt-in), so the
side-effect case was the live pre-change behavior. In the two incidents above
the completed calls were read-only, but that was luck, not design.

## Goals

1. An unknown tool name becomes a recoverable, in-turn error rather than a
   terminal generation failure.
2. The model receives enough information to correct itself on the next
   iteration, rather than merely being told it failed.
3. Valid tool calls in the same batch are unaffected.
4. Sequential and parallel paths behave identically.
5. The event stays countable. Recovery must not make a real defect invisible.

## Non-goals

- **Constraining tool-name generation.** Raised by the reporting customer by
  analogy to structured output. It is not offered by the hosted provider APIs,
  and it would be the wrong fix regardless: masking an invalid name does not
  make the model want the right one, it makes it emit the next most probable
  _valid_ one. That converts a loud failure into a silent call against the
  wrong tool. The error is the useful signal here.
- **Fuzzy name correction at dispatch.** Dive stays an exact-name dispatcher.
  Suggestions are returned to the model; they are never silently substituted.
  Remapping a name the model did not emit risks dispatching to the wrong tool,
  which is strictly worse than an extra round trip.
- **Retrying the generation.** Recovery happens through the normal tool-result
  loop, not a retry wrapper.
- **Fixing downstream transcript handling.** Mobius has a separate defect where
  a failed turn's dangling tool calls are not closed when the assistant message
  was already committed mid-turn. That is tracked on the Mobius side and is not
  specific to unknown names; timeouts and provider errors hit it too.

## Proposal

Return an error `ToolCallResult` instead of a Go error, mirroring the existing
`tool.Call` failure shape so the result flows through the normal path. This is
also what Claude Code itself does with an unknown tool name — an error
`tool_result` the model reads and corrects on the next iteration — and Dive's
stated design philosophy is to align its tool behavior with Claude Code. That
makes this a parity fix, not a judgment call.

In the sequential path (`executeOneToolCall`) the lookup becomes:

```go
tool, ok := toolsByName[toolCall.Name]
if !ok {
    return unknownToolResult(toolCall, toolsByName), nil
}
```

(The parallel path has a different return shape and routes the same result
through its prep slots; see the implementation section, which also covers the
hook and event details both paths must get right.)

```go
func unknownToolResult(
    call *llm.ToolUseContent,
    toolsByName map[string]Tool,
) *ToolCallResult {
    suggestions := suggestToolNames(call.Name, toolsByName)
    return &ToolCallResult{
        ID:    call.ID,
        Name:  call.Name,
        Input: call.Input,
        Result: &ToolResult{
            Content: []*ToolResultContent{{
                Type: ToolResultContentTypeText,
                Text: unknownToolMessage(call.Name, suggestions),
            }},
            IsError: true,
        },
        Error: &UnknownToolError{Name: call.Name, Suggestions: suggestions},
    }
}
```

The error is a typed `UnknownToolError` rather than a bare `fmt.Errorf`, so
callers can detect and count the event with `errors.As` — no parsing of
result text, and no new `ResponseItemType` needed (see Observability):

```go
// UnknownToolError reports a tool call whose name matched nothing in the
// toolset declared for the request. No tool was dispatched.
type UnknownToolError struct {
    Name        string
    Suggestions []string
}

func (e *UnknownToolError) Error() string {
    return fmt.Sprintf("unknown tool %q", e.Name)
}
```

Setting `Error` preserves the Go-level signal for anything inspecting the
result, while `Result.IsError` is what reaches the model.

The message carries the correction:

```text
Tool "mobius_market_clock" does not exist and was not called. Did you mean:
core_market_clock. Call one of the tools declared for this turn.
```

### Suggestion algorithm

Suggestions are drawn only from `toolsByName`, which is the agent's declared
set for that turn. Split names into non-empty segments on every `_`, `-`, or
`.` delimiter. Evaluate every candidate, keep only the first tier with a
match, then order that tier by its match score and finally by the complete wire
name in ascending lexical order. This is the total ordering; map iteration
order must not leak into messages or tests.

1. **Segment-suffix match.** Split both names on `_`, `-`, and `.`; rank by
   the longest common suffix of segments, then by the suffix's total character
   count, both descending. A match requires at least two shared trailing
   segments, or one shared trailing segment of at least six characters —
   `mobius_market_clock` matches `core_market_clock` on the two-segment suffix
   `market_clock`, while a lone shared `clock` is below the floor and cannot
   tie `core_market_clock` with `core_portfolio_clock`. This is the dominant
   real-world shape: right leaf, wrong namespace. Note that the incident name
   is _not_ reachable by edit distance — `mobius_` → `core_` exceeds the
   tier-2 bound — so this tier is load-bearing and gets its own test.
2. **Bounded edit distance.** Levenshtein within `min(3, len/4)` on the full
   name, for ordinary typos; lower distance ranks first.
3. **Prefix match.** The namespace is the complete sequence of segments before
   the terminal leaf. A candidate matches when that sequence equals the
   query's sequence, regardless of which supported delimiter appeared in the
   original names; longer namespaces rank first. A flat, single-segment name
   has no namespace and therefore cannot match this tier. Return matching
   namespace members so the model can pick.

Cap at three suggestions. The full toolset is already in the model's context
via the tools parameter, so suggestions are a nudge, not documentation. Emit
none rather than a poor one; a bare "does not exist" is still recoverable, and
a confident wrong suggestion is worse than silence.

### Iteration limit

Unknown-name results use the same agent-wide `ToolIterationLimit` as every
other recoverable tool failure. There is no unknown-specific counter or retry
budget. If the model keeps calling a missing tool, the existing final-iteration
mechanism appends the "respond with a final answer now" instruction and forces
`ToolChoiceNone`.

This keeps recovery consistent with tool execution errors, panics, malformed
results, and hook denials. Deployments that want a tighter overall bound can
already lower `ToolIterationLimit`; unknown names do not justify a second state
machine with different reset semantics.

### Observability

Recovery must not make this invisible. Three surfaces:

- `a.logger.Warn` with the invented name, the suggestions offered, and the
  agent name.
- `ToolCallResult.Error` set to `*UnknownToolError`, so callers count the
  event with `errors.As` — no parsing of result text.
- The unknown call emits the standard `ResponseItemTypeToolCall` /
  `ResponseItemTypeToolCallResult` pair, exactly like an executed or denied
  call. The `tool_use` exists in the assistant message and the error result
  occupies its transcript slot, so stream consumers that pair the two events
  keep working unmodified.

A distinct `ResponseItemType` for the event was considered and rejected:
existing stream consumers silently drop item types they do not know,
exhaustive switches break on a new one, and it raises a double-counting
question against the `tool_call_result` item the slot already emits. The
typed error on the existing item carries the same information.

This matters for a class the recovery does not cover. If a real defect starts
producing wrong names — a wire-name transform regression, a broken alias map,
a toolkit migration that renames actions under running agents — the
pre-change behavior surfaces it within the hour. Post-change the agent
quietly self-heals, or quietly burns tokens failing to. The warning log and
the countable typed error are what keep that discoverable, and monitoring
must watch them rather than error rates (see Tradeoffs).

### No configuration

This should ship unconditionally, with no opt-out.

The behavior it replaces is not one a caller would knowingly choose. A flag
would exist only to preserve "a misspelled name should end the turn," which no
deployment wants on purpose.

The security argument that might justify strictness does not apply. An
unknown name dispatches nothing: the lookup fails, no tool runs, no boundary
is crossed. Halting on it is theater.

There is deliberately no hook escape hatch either. No `PreToolUse` or
`PostToolUse` hook fires for a name that resolves to nothing (safety
invariant 3), so no hook could see the event to veto it — an earlier draft
claimed a deployment could "express halt-on-unknown as a hook," which is
inconsistent with that invariant. A deployment that wants to react can watch
for `*UnknownToolError` on the emitted `tool_call_result` items via the event
callback, or inspect results after the turn.

## Safety invariants

1. No undeclared tool is ever dispatched. The lookup still fails; only the
   reporting changes.
2. Suggestions come only from `toolsByName`, so the message cannot reveal a
   tool the agent does not have.
3. Hooks do not fire for a tool that does not exist. Both call sites already
   perform the lookup before hook construction, but preserving the invariant
   takes more than that in the parallel path: the drain loop must also skip
   `PostToolUse`/`PostToolUseFailure` for the unknown slot. Routing the result
   through `deniedResults` alone is **not** sufficient — denied slots are
   skipped for execution but still flow through the drain loop, which fires
   `PostToolUseFailure` with `HookContext.Tool` taken from the prep
   (`agent.go:2406`), and for an unknown name that is nil. See implementation
   step 3.
4. Exactly one `tool_result` per `tool_use`. Providers reject an assistant
   tool-call batch that is not fully answered, so the error result must occupy
   the same slot the real result would have.
5. Valid calls in the batch execute and report normally.
6. An unknown name never produces a `Suspend` or `Background` outcome.
7. An unknown call still emits the standard `tool_call` and
   `tool_call_result` response items, in both execution modes.

## Alternatives considered

### Keep it fatal, but preserve the batch

Return the error while still building the `tool_result` message from completed
calls. Fixes the discarded-work half but not the dead turn, and still gives the
model no path to correct itself. Strictly less useful for the same effort.

### Remap near-miss names at dispatch

Resolve `mobius_market_clock` to `core_market_clock` automatically when the
match is unambiguous. Tempting, and it would have silently fixed both
incidents. Rejected: it makes dispatch inexact, so a wrong-but-confident match
calls a tool the model did not ask for. Mobius already made this call in its
own alias layer, which accepts exact canonical-to-wire mappings and explicitly
refuses lookalikes. Dive should hold the same line.

### Configuration flag

Covered above. The knob has no defensible "on" setting.

### Constrained decoding of tool names

Covered in non-goals. Not available on hosted providers, and it trades a
detectable failure for an undetectable one.

## Tradeoffs and consequences

- **Loud becomes quiet.** The main cost, mitigated by the warning log, the
  countable `*UnknownToolError`, and the existing agent-wide iteration limit. Worth stating
  plainly in review: we are choosing to handle something automatically that
  currently pages a human.
- **The worst case is now an apology, not an error.** With the graceful
  final iteration, a systemic defect that makes every tool name unknown
  manifests as degraded answers plus warn-log volume, never as failed turns.
  Monitoring must watch the log line and the typed-error count, because error
  rates will no longer move.
- **Cost per incident rises.** Today the turn dies at ~13 seconds. After, the
  model spends at least one extra generation, sometimes more. That is the right
  trade for a recovered turn. `ToolIterationLimit` remains the caller's common
  cost bound for all recoverable tool failures.
- **Public behavior change.** Callers relying on the hard error stop receiving
  it. Minor version bump and a changelog entry, not a patch release.
- **Sequential side effects still precede the failure.** This change means the
  batch is no longer discarded, so completed results are now recorded rather
  than lost. That is an improvement, but callers with non-idempotent tools
  should still not assume a failed turn ran nothing.

## Implementation

1. Add `UnknownToolError`, `unknownToolResult`, `unknownToolMessage`, and
   `suggestToolNames` with the suggestion ranking, in a new
   `agent_unknown_tool.go` alongside their tests.
2. Replace the fatal return at `agent.go:2497` (`executeOneToolCall`). On
   lookup failure: emit the `ResponseItemTypeToolCall` event (the early
   return otherwise skips the emission at `agent.go:2512`), emit the
   `ResponseItemTypeToolCallResult` event, log the warning, and return
   `unknownToolResult(...)` — before preview generation and before any hook
   context is built.
3. Replace the fatal return at `agent.go:2248` (parallel Phase 1): set
   `deniedResults[i]` to the unknown result **and** add an `unknown bool`
   flag to `toolCallPrep` that the drain loop checks to bypass
   `PostToolUse`/`PostToolUseFailure`. The existing `denied` flag alone is
   not sufficient — denied slots skip execution but still fire failure hooks
   with `prep.tool` (nil here) as `HookContext.Tool` (`agent.go:2406`),
   violating safety invariant 3 and handing existing hooks a nil interface.
   Still emit the `tool_call` event in Phase 1 as for any other call.
4. Add the logger warning to the parallel path (the sequential path gets it
   in step 2).
5. Changelog entry; ship the behavior change in the next minor release.

## Tests

- Unknown name in a single-call batch returns an `IsError` result and the
  generation continues to the next iteration.
- Unknown name alongside valid calls: valid calls execute, all calls receive
  results, and the batch is not discarded. Both execution modes.
- `ToolCallResult.Error` unwraps to `*UnknownToolError` via `errors.As`,
  carrying the invented name and the suggestions offered.
- The unknown call emits the standard `tool_call` and `tool_call_result`
  response items, in both execution modes.
- The suggestion for `mobius_market_clock` against a toolset containing
  `core_market_clock` and `mobius_memory_recall` is `core_market_clock`, and
  it comes from the segment-suffix tier — the name is beyond the edit
  distance bound, so this pins that tier 1 is load-bearing.
- A single shared trailing segment below the length floor produces no
  suggestion (`core_market_clock` vs a query sharing only `clock`).
- Equal-score candidates are ordered by complete wire name, and suggestion
  output is deterministic across runs and map insertion orders.
- Prefix-tier tests cover `_`, `-`, and `.` delimiters, equal namespace scores,
  and flat names that must not qualify as namespace matches.
- No suggestion is emitted when nothing scores within threshold.
- Suggestions never include a name absent from `toolsByName`.
- Repeated unknown-only iterations use the standard `ToolIterationLimit`
  final-answer path; there is no unknown-specific counter.
- No `PreToolUse`, `PostToolUse`, or `PostToolUseFailure` hook fires for an
  unknown name — in the sequential path and in the parallel drain loop.
- Every `tool_use` in a batch containing an unknown name has a matching
  `tool_result`.
- Resuming a suspended turn whose pending call names a tool no longer in the
  re-resolved toolset produces an error result instead of failing the resume.

## Rollout and verification

Ship in the next minor release. Mobius picks it up by bumping its pin in
`go.mod` alongside the provider modules, which move together.

Verification is available directly: the reporting deployment is collecting a
count of unknown-name errors over a week. After the bump, that count should
persist as a recovery counter while `generation_failed` turns attributable to
this cause go to zero.

## Resolved questions

1. **Distinct `ResponseItemType`, or a field on the existing item?** Neither
   a new type nor an extra field: a typed `*UnknownToolError` on
   `ToolCallResult.Error`, delivered on the standard `tool_call_result` item.
   Counting works through `errors.As`, stream consumers need no new type, and
   nothing double-counts. Rationale in Observability.
2. **Should unknown names have a separate retry threshold?** No. Every other
   recoverable tool failure uses `ToolIterationLimit`, and unknown names should
   follow the same policy. A dedicated counter adds state and ambiguous reset
   semantics without establishing a broader failure-budget abstraction.
3. **Suggestion list size?** Cap at three and never fall back to dumping the
   toolset. The full tool list is already in the model's context via the
   tools parameter; a longer list is documentation, not a nudge.
