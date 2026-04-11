# Suspend/Resume Review — Combined Findings

Two independent reviews of the suspend/resume PR (#137, branch
`agent-suspend-resume`). One focused on correctness, durability, and
production safety; the other focused on API shape, idiomatic Go, and
caller ergonomics. They are largely complementary — one found mostly
runtime concerns, the other mostly surface-area concerns.

This document captures both verbatim-in-spirit so the next pass can be
planned from a single source.

## Combined verdict

**The internal algorithm is sound. The API needs one focused redesign
pass before merge: suspend/resume must work without a `Session`.**

The internal algorithm is sound:
- Sequential vs parallel split is coherent.
- Persisted pending payloads (full `Prompt`/`Metadata`) are the right
  call for cross-process resume.
- Reusing `CreateResponse` for resume keeps the mental model small.
- Adversarial test coverage is strong (28 invariant tests + 5 store
  round-trips, all green under `-race`).

What needs work falls into three buckets: **(1) make sessions
optional**, **(2) public API simplification**, **(3) caller
ergonomics and production hardening**.

## Core design constraint (non-negotiable)

**Sessions must be a convenience for users who want auto-persistence,
not a requirement for the feature to function.**

Some of Dive's most important users do not use the `Session`
interface at all — they manage history themselves and pass it via
`WithMessages` on each call. Today, suspend/resume hard-requires a
`SuspendableSession` (`agent.go:589-595` returns
`ErrSessionNotSuspendable` if one isn't present), which means the
feature is unavailable to those users.

The next pass must remove that requirement. The information needed
to resume already lives on `Response` (`PendingToolCalls`,
`CompletedToolCalls`, `OutputMessages`) — what's missing is a way to
feed it back in on the next call. Once that loop closes, sessions
become a *convenience* for users who want the agent to auto-persist
on their behalf, not a *gate* on whether suspend/resume works at all.

This single redesign collapses the original H2, H3, and M4 findings
into one coherent change and unlocks the feature for stateless users.

---

## Findings by severity

### High

#### H1. Cross-process file locking — RESOLVED BY DOCUMENTATION
**Source:** correctness review.
**Where:** `session/file_store.go`.
**Status:** descoped — `FileStore` is now documented as
single-writer-per-session by design.

Original concern: `FileStore` has an in-process `sync.RWMutex` but no
`flock()` or equivalent, so two processes writing the same session
concurrently could race (last-writer-wins on rename, append vs rewrite
collision).

**Resolution:** the intended deployment model is single-writer per
session. Sequential cross-process handoff (suspend in process A → exit
→ resume in process B) is fully supported and crash-consistent under
the existing tmp+rename+fsync code. The race only exists when two
processes try to be active on the same session simultaneously, which
is an explicit non-goal.

`FileStore` godoc now states this contract explicitly and points
multi-instance deployments at a database-backed `Session` backend
instead. **Optional belt-and-suspenders** for a future PR: add an
advisory `syscall.Flock(LOCK_EX|LOCK_NB)` on `Open()` held for the
`*Session` lifetime so accidental double-open fails fast (~30 lines,
Linux/macOS; Windows needs `LockFileEx`).

#### H2. Suspend/resume hard-requires a `Session` (and the interface is too storage-shaped)
**Source:** API review + reframe based on user-base reality.
**Where:** `agent.go:589-595` (the hard requirement); `dive.go:32-65`
(the interface shape); leaks `session.PendingCall` across the package
boundary.

**Two problems in one finding:**

**(a) The hard requirement.** `finishSuspended` returns
`ErrSessionNotSuspendable` if no `SuspendableSession` is present.
Some of Dive's most important users do not use sessions at all —
suspend/resume is currently unavailable to them. **This is the core
design constraint; see callout near the top of this document.**

**(b) The interface shape.** Even for users who do use sessions, the
interface forces custom implementers to understand agent internals,
import both `dive` and `session`, and live with two parallel
pending-call types (`session.PendingCall` and `dive.PendingToolCall`).
Six storage-level methods exposed: `Suspended`, `PendingCalls`,
`LastEventMessageCount`, `SaveSuspendedTurn`, `SaveResumedTurn`,
`AbandonSuspension`.

**Fix path (combined):**
1. Add `WithPendingToolCalls([]*PendingToolCall)` (or equivalent
   `WithSuspendedState`) so callers can pass pending state in
   directly.
2. Make the agent read authoritative pending state from options
   first, falling back to session only if present.
3. Drop the hard `SuspendableSession` requirement — the agent only
   needs it when the caller wants auto-persistence.
4. Move `PendingToolCall` (and any related types) to the root `dive`
   package so there is one canonical type and `session` no longer
   leaks across the boundary.
5. Shrink `SuspendableSession` to the minimum the agent actually
   needs for auto-persistence — likely just two methods: "save this
   suspended turn" and "save this resumed turn." Everything the agent
   currently *reads* from the session (`Suspended`, `PendingCalls`,
   `LastEventMessageCount`) becomes derivable from the messages
   passed to `CreateResponse` plus the new `WithPendingToolCalls`
   option.

A stateless suspend/resume flow then looks like:

```go
// Suspend
resp, _ := agent.CreateResponse(ctx, dive.WithMessages(history...))
if resp.Status == dive.ResponseStatusSuspended {
    saveWherever(history, resp.OutputMessages, resp.PendingToolCalls)
    return
}

// Resume — no session involved
resp, _ := agent.CreateResponse(ctx,
    dive.WithMessages(append(history, savedOutput...)...),
    dive.WithPendingToolCalls(savedPending),
    dive.WithToolResults(results),
)
```

Users who *do* want auto-persistence still get it by setting
`AgentOptions.Session` to a `SuspendableSession`. The behavior is
identical from their perspective; the agent just calls the save
methods on their behalf instead of returning state on the Response.

#### H3. Resume is too implicit
**Source:** API review.
**Where:** `agent.go:327-336`.
**Linked to H2:** falls out naturally once H2 is fixed.

A suspended session changes `CreateResponse` behavior even when the
caller passes neither input nor `WithToolResults`. In practice this
means "call `CreateResponse` on a suspended session with no input"
silently goes through the partial-resume path and returns a
no-op-rewrite suspended response. It works, but it is not what most
callers will expect.

**The H2 fix solves this for free** for stateless users: presence of
`WithPendingToolCalls` (or `WithToolResults`) becomes the explicit
opt-in. There is no implicit "the session is suspended so I'll
silently resume" path because there is no session being read.

For users who DO use sessions, the same option-based opt-in still
applies: the agent should require `WithToolResults` (or a new
`WithResume()` marker) to enter the resume path on a suspended
session. Calling `CreateResponse` with neither input nor results on a
suspended session should error out, not silently no-op.

---

### Medium

#### M1. `appendEvent` does not fsync (durability gap on the hot path)
**Source:** correctness review.
**Where:** `session/file_store.go:205`.

Normal `SaveTurn` writes a JSONL line and never calls `f.Sync()`
before close. Power loss between commit and the OS flushing pagecache
can lose the most recent completed turn. Suspend writes are safe
(they use `putSession`, which now fsyncs both the file and the parent
directory after the recent fix). This is a typical perf trade-off but
should be a configurable knob (`Sync bool` on `FileStore`) and
documented.

#### M2. Concurrent `CreateResponse` on the same session is unsafe (pre-existing)
**Source:** correctness review.
**Where:** `agent.go:CreateResponse`.

Two concurrent `CreateResponse` calls on the same session can
interleave their reads of `Messages()` and writes via `SaveTurn`,
producing tangled events. The PR doc warns that *resume* isn't safe
to run concurrently — the issue is broader: any two `CreateResponse`
calls on one session are unsafe. Suspend/resume makes this riskier
because cross-process resume implies callers may not coordinate.

**Fix paths:**
- Per-session lock at the agent layer.
- Or document prominently and provide a recipe for caller-side
  coordination.

#### M3. `ToolResult.Suspend` is a tagged union without enforcement
**Source:** API review.
**Where:** `tool.go:99-126`.

The shape is pragmatic and additive but not Go-crisp. Callers can set
`Content`, `Display`, `IsError`, *and* `Suspend` simultaneously and
the contract is "some of those are silently ignored." That's
survivable but not idiomatic.

**Fix paths:**
- Validate at construction time and reject mixed states.
- Or split into two distinct return types (e.g. `Tool.Call` returns
  `(any, error)` where `any` is `*ToolResult` xor `*SuspendResult`).
  This is a bigger surgery and would touch every tool implementation —
  probably too invasive for v1.

#### M4. Public suspension surface is too spread out
**Source:** API review.
**Where:** `response.go:38, 149, 154, 159`, plus authoritative state
on the session via `dive.go:39`.
**Linked to H2:** the H2 fix makes Response the canonical home, which
naturally collapses this surface.

Today there are five places to look: `Response.Status`,
`Response.PendingToolCalls`, `Response.CompletedToolCalls`, the
terminal `ResponseItemTypeSuspended` stream item, and
`session.PendingCalls()`. It takes a while to internalize which is
informational and which is authoritative.

**Fix path (combined with H2):**
Once `Response` becomes the authoritative carrier (because stateless
users have nowhere else to read from), collapse the related fields
into one nested `Response.Suspension *SuspensionState` payload
containing pending calls, completed calls, and any other info the
caller needs to round-trip back via `WithPendingToolCalls`. The
`SuspensionState` value should be exactly what the caller stores and
passes back — symmetric in/out, no impedance mismatch.

#### M5. Caller ergonomics — input decoding is manual
**Source:** API review.
**Where:** examples in `examples/suspend/human_approval/main.go:66`,
`examples/suspend/partial_resume/main.go:52`.

Callers have to manually `json.Unmarshal` `PendingToolCall.Input` to
get at the original tool input. Low-level and flexible, not turnkey.

**Fix paths:**
- Add `(p *PendingToolCall) UnmarshalInput(into any) error` helper.
- Add a generic `DecodePendingInput[T](*PendingToolCall) (T, error)`
  helper.
- The toolkit could provide typed pending-call accessors keyed by
  tool name.

---

### Low

#### L1. Stop hook + suspend interaction has a latent bug (pre-existing)
**Source:** correctness review.
**Where:** `agent.go:501-509`.

When a `Stop` hook returns `Continue: true`, generate is re-entered
via `goto generateLoop`. Each `generate()` call produces a fresh
`outputMessages` slice (`agent.go:1097`); the previous iteration's
outputs are appended to the LLM `messages` but **not** carried into
`response.OutputMessages`. If the re-entered generate then suspends,
`finishSuspended` saves `inputMessages + response.OutputMessages` and
loses the first iteration's assistant turn plus the Stop hook's reason
message. Pre-existing bug; suspend/resume makes the consequence worse.

#### L2. JSON metadata type fidelity loss
**Source:** correctness review.
**Where:** `session/session.go:108-138` (`deepCopyJSONValue`).

`SuspendResult.Metadata` is `map[string]any`. After the first
`clonePendingCalls` call, anything that isn't a recognized scalar gets
round-tripped through `json.Marshal`/`Unmarshal`. So `int` becomes
`float64`, custom struct types become generic maps, **even in-process
before any disk write**. This will surprise tool authors.

**Fix:** doc-only — explicitly state in `SuspendResult.Metadata`
godoc that values must be JSON-friendly and that numeric values come
back as `float64`.

#### L3. Polling-with-empty-input has undefined semantics
**Source:** correctness review.
**Where:** `agent.go:327-336, 415-424`.

Calling `CreateResponse` on a suspended session with no input and no
`ToolResults` silently re-saves the suspended turn and returns it.
It's a no-op rewrite of the JSONL file every poll. Mostly subsumed by
**H3** above.

#### L4. Hook contract doc/code mismatch
**Source:** API review.
**Where:** `hooks.go:206`, `agent.go:601-615`, PR description.

`OnSuspend` runs **before** persistence in the current code, and the
abort path triggers compensation. The PR body still describes an
earlier "abort then compensate" model. Not a code bug, but the docs
and PR description need to say exactly one thing — webhook/outbox
designs depend on this ordering.

#### L5. No backpressure / max suspended sessions
**Source:** correctness review.

A misbehaving caller can suspend forever and accumulate JSONL files
indefinitely. Not the library's responsibility to enforce, but
`FileStore.List` should expose a `suspended-only` filter and the docs
should mention an "abandon stale suspended sessions" pattern.

---

## Cleanups (not blocking)

- `dive.go:182` — godoc says "Resume is not safe to call concurrently
  on the same session." The actual rule is broader. Fix the comment
  or add a per-session lock.
- `session/session.go:316-420` — extract a `withRollback` helper. The
  rollback pattern is now repeated three times across
  `SaveSuspendedTurn`, `SaveResumedTurn`, `AbandonSuspension`.
- `session/memory_store.go:62-73` — `List` takes `sess.mu.RLock()`
  while holding `s.mu.RLock()`. Lock order is consistent (store →
  session) but worth a comment so it stays that way.
- `examples/suspend/async_webhook/main.go` — explicitly warn about the
  missing file lock for multi-process production use.
- Eight public nouns for one concept: `SuspendResult`,
  `PendingToolCall`, `CompletedToolCall`, `WithToolResults`,
  `SuspendableSession`, `OnSuspend`, `ResponseStatusSuspended`,
  `ResponseItemTypeSuspended`. The next pass should consolidate.

---

## Themes the next pass should address

1. **Production safety floor.** File locking, optional fsync on the
   hot path, and a clear story for multi-instance deployments. Until
   these land, the docs should bound the supported deployment shape.

2. **Public API simplification.** The implementation is correct; the
   surface is overpacked. Two reviewers independently flagged the
   spread-out shape. Worth one focused refactor pass before downstream
   code starts depending on it.

3. **Resume is opt-in and explicit.** Either a dedicated entry point
   or a hard error when the contract is ambiguous. No silent no-op
   pollers.

4. **Turnkey ergonomics for the common case.** Input decoding helpers,
   one canonical place to read suspension state, fewer types to
   internalize.

5. **Documentation alignment.** Hook ordering, metadata caveats,
   concurrency rules, deployment constraints. Several callouts in this
   review are doc-only.

6. **Pre-existing bugs surfaced by review.** Stop hook + suspend
   interaction (L1) and concurrent CreateResponse (M2) predate this
   PR but should be tracked so they don't get lost.

---

## Suggested priority order for the follow-up

**Must-do before merge (or before any user starts depending on the
v1 surface):**

1. **(H2 + H3 + M4) The combined "sessions are optional" redesign.**
   This is the single largest item and shapes everything else:
   - Add `WithPendingToolCalls` (or `WithSuspendedState`) option.
   - Make agent read pending state from options first, session
     second.
   - Drop the hard `SuspendableSession` requirement.
   - Move `PendingToolCall` to root `dive` package.
   - Shrink `SuspendableSession` to the minimum needed for
     auto-persistence.
   - Collapse `Response`'s suspension fields into one
     `Response.Suspension *SuspensionState`.
   - Make resume opt-in via the presence of suspension options.
2. **(M5) Add input-decoding helpers** —
   `(*PendingToolCall).UnmarshalInput(into any) error` and a generic
   `DecodePendingInput[T]`. Stateless callers do more marshalling, so
   this matters more without sessions.
3. **(M3) Validate `ToolResult.Suspend` mutual-exclusion** at
   construction. Cheap, prevents surprising silent-ignore behavior.

**Follow-up PRs (no API rewrite needed):**

4. (M1) Add an `Sync` knob to `FileStore` for the hot-path durability
   trade-off.
5. (L1) Fix the pre-existing Stop hook + suspend message loss in
   `agent.go:501-509`.
6. (M2) Per-session locking or a documented coordination recipe for
   concurrent `CreateResponse` on the same session.
7. (L2) Document JSON metadata type-fidelity caveat in
   `SuspendResult.Metadata` godoc.
8. (L4) Reconcile hook contract docs with current `OnSuspend`
   ordering (runs before persistence; abort path triggers
   compensation).
9. Cleanups: `withRollback` helper extraction in `session/session.go`,
   lock-order comment in `memory_store.go`, example warning in
   `async_webhook/main.go`.

(H1 was descoped — single-writer-per-session is now the documented
contract. Optional flock guardrail can be added later if useful.)
(L3 and L5 are subsumed by item 1.)
