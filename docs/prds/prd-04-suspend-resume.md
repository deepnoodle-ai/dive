---
Title: Agent Suspend/Resume for Long-Running Tool Calls
Author: Curtis Myzie
Status: Implemented
Last Updated: 2026-04-11
Stakeholders: Dive maintainers
---

# Agent Suspend/Resume for Long-Running Tool Calls

## 1. Problem & Opportunity

Dive is increasingly being embedded in SaaS workflows where an agent, mid-turn, needs input from a human — an approval, a form submission, a clarifying answer, a file upload, an out-of-band review. In real product workflows, **that input may not arrive for hours or days**. A compliance reviewer may answer tomorrow morning. An approver may be on vacation. A customer may fill out a form next Monday.

Today, Dive has no way to express this cleanly. `CreateResponse` runs a synchronous loop: the goroutine owns the conversation until the LLM emits a final message. The current options for "wait for a human" are all bad:

- **Block inside the tool** — holds a goroutine and a request handler indefinitely, dies on process restart, doesn't scale.
- **Tool returns a placeholder** — the LLM sees a fake success and keeps going, which is wrong.
- **Tool returns an error** — the agent treats it as a failure, not a suspension, and the conversation state is lost.
- **Rebuild the loop externally** — every integrator ends up writing their own fragile suspend/resume on top of Dive's session.

Without a first-class mechanism, every SaaS embedding of Dive has to reinvent this, and none of the reinventions survive a process restart. As more users push Dive toward production workflows (Mobius Ops, Obelisk, Kard), this is the single biggest gap between "agent that answers questions" and "agent that participates in a multi-day business process."

**If we do nothing:** Dive stays suitable for synchronous chat and quick tool loops, but every team building a real workflow on top of it has to fork the agent loop or build a parallel orchestrator. We lose the embedding use case to frameworks that treat human-in-the-loop as a first-class concept.

## 2. Goals & Success Metrics

**Primary goal:** Any tool author can cause the agent to suspend mid-turn, persist its state, return control to the caller immediately, and resume cleanly later — including across process restarts.

**Success metrics:**
- **Primary:** An example SaaS integration can complete a full suspend → process restart → resume → final answer flow in under 50 lines of integration code, with no custom loop orchestration.
- **Secondary:**
  - Zero goroutines held during a suspended turn.
  - Suspended state survives process restart when backed by a persistent `session.Store`.
  - Existing (non-suspending) agent behavior is bit-for-bit unchanged — no regressions in tool latency, token usage, or message shape.
- **Guardrail:** The common case (tools that never suspend) must not pay any measurable cost — no extra allocations, no extra hook invocations, no API complexity that leaks into `FuncTool[T]` authors who don't care.

## 3. Target Users

**Primary persona: SaaS integrator.** A Go engineer embedding Dive in a product where agents run inside request handlers, background workers, or durable workflow engines. They care about: process restarts, serializable state, webhook-driven resume, clear APIs.

**Secondary persona: Tool author.** Someone writing a Dive tool — `AskUser`, `RequestApproval`, `WaitForWebhook`, an MCP tool that proxies to a slow external system. They need a clean way to say "this tool's answer isn't available yet; suspend the agent."

**Not optimized for:** the synchronous CLI user. Suspend/resume must not make their flow more complicated.

## 4. User Stories

### US-001: Tool author suspends an agent
**Description:** As a tool author, I want to return a typed `SuspendResult` from my tool's `Call` method so that the agent suspends its turn and the caller can fulfill the tool result later.

**Acceptance Criteria:**
- [x] A tool can return a typed `dive.SuspendResult` (or equivalent) from `Call` to signal suspension. Optional fields on the result allow the tool to attach a human-readable prompt or other metadata for the integrator.
- [x] When the agent observes this, the current `CreateResponse` unwinds without an error and without calling the LLM again.
- [x] The tool's `ToolUse` block is preserved in the conversation, but no `ToolResult` for it is written.
- [x] Documentation and runnable examples demonstrate the pattern (`docs/guides/suspend-resume.md` plus five examples under `examples/suspend/`: `human_approval`, `partial_resume`, `async_webhook`, `stateless`, and the shared `dialogspec`).
- [ ] At least one built-in toolkit tool demonstrates suspend (e.g. an async-mode `AskUser`). **Not done** — `toolkit/ask_user.go` uses a synchronous in-process `Dialog` by design. Adding a suspend-mode option is a separate enhancement; the pattern is demonstrated in `examples/suspend/` for now.

### US-002: Caller observes a suspended response
**Description:** As a SaaS integrator, I want `CreateResponse` to return a `Response` with a status field that clearly indicates the agent is suspended and which tool calls are awaiting external fulfillment.

**Acceptance Criteria:**
- [x] `Response.Status` distinguishes `ResponseStatusCompleted` from `ResponseStatusSuspended`. Suspended responses are returned as normal `(*Response, nil)`, NOT as errors.
- [x] A suspended `Response` exposes `PendingToolCalls` — a list of pending tool call IDs, their tool names, their input payloads, and any metadata supplied by the tool's `SuspendResult`. (Now nested under `Response.Suspension *SuspensionState`; `PendingToolCalls` is a field on the suspension.)
- [x] The caller can persist the response's session and return from their handler without holding any goroutines.

### US-003: Caller resumes a suspended agent with a tool result
**Description:** As a SaaS integrator, I want to resume a suspended agent by providing results for the pending tool calls so that the agent continues its turn as if the tool had returned normally.

**Acceptance Criteria:**
- [x] A resume API accepts a map of `tool_call_id → ToolResult` and a session (or session ID). Two entry points: `WithToolResults(map)` for session-backed callers, and `WithResume(state, map)` for stateless callers who manage history themselves.
- [x] On resume, the agent injects the provided results as a `ToolResult` message, skips the LLM generation for that step (the assistant message already exists), and re-enters the normal generation loop.
- [x] Resume works whether the same process or a different process (given a persistent `session.Store`) is driving it. Pinned by `TestResumeWithFileStoreCrossProcess`.
- [x] If the caller provides results for tool call IDs that are not in the pending set, resume errors clearly (`ErrUnknownPendingToolCall`) without mutating session state. Partial coverage of pending calls (where some pending IDs are satisfied and some are not) keeps the agent suspended — see US-005.

### US-004: Parallel tool calls where one suspends (Option A)
**Description:** As a tool author, I want non-suspending sibling tool calls to complete normally while a suspending sibling suspends the agent, so that work isn't wasted and the agent has partial results ready on resume.

**Acceptance Criteria:**
- [x] When parallel tool execution includes at least one suspending tool, the non-suspending siblings run to completion.
- [x] Their results are persisted with the session as part of the suspended turn.
- [x] On resume, the caller only needs to supply results for the tool calls that actually suspended — the completed siblings are already in place.
- [x] Hooks (`PostToolUse`, `PostToolUseFailure`) fire normally for the completed siblings; suspending tools do NOT fire `PostToolUse` (since there is no result yet).

### US-005: Multiple simultaneous suspensions
**Description:** As a tool author, I want two or more tool calls in the same iteration to be able to suspend concurrently, so that a single turn can wait on multiple external inputs at once.

**Acceptance Criteria:**
- [x] If multiple parallel tool calls return the suspend signal, the `Response.PendingToolCalls` includes all of them.
- [x] Resume accepts results for all pending calls at once, or for a subset — if a subset is supplied, the remaining ones stay pending and the agent stays suspended. Pinned by `TestPartialResumeTwice`.
- [x] Resuming with all pending results satisfied transitions the agent back to the generation loop in a single call.

### US-006: Suspend state survives process restart
**Description:** As a SaaS integrator using a persistent `session.Store`, I want to restart my process and still be able to resume a suspended agent so that long waits (hours, days) are not bounded by process lifetime.

**Acceptance Criteria:**
- [x] The suspended turn — assistant message with tool_use blocks, any completed sibling tool results — is persisted via the session before `CreateResponse` returns.
- [x] After a process restart, opening the same session shows the agent in a suspended state with the same pending tool calls.
- [x] Resume from the new process produces the same final output as resume from the original process would have. Pinned by `TestResumeWithFileStoreCrossProcess` and `TestCrossProcessSuspendMetadata`.

### US-007: Cancel a suspended agent
**Description:** As a SaaS integrator, I want to cancel a suspended agent (e.g. the user abandoned the workflow) and clean up without corrupting the session so that the conversation can be discarded or continued as a fresh turn.

**Acceptance Criteria:**
- [x] There is a documented way to finalize a suspended session — supplying an `IsError: true` `ToolResult` for each pending tool call. The error results flow through the normal `PostToolUseFailure` path; a dedicated `CancelTurn` API was deferred to v2 (see §6 Non-Goals). Pinned by `TestResumeErrorResultCancelsTurn`.
- [x] After cancellation, starting a new `CreateResponse` on the session does not leave dangling tool_use blocks without matching tool_results.

## 5. Functional Requirements

- **FR-1:** The agent loop must detect a dedicated suspend signal returned by a tool. The signal is a typed `SuspendResult` (not an error sentinel), allowing tools to attach optional metadata such as a human-readable prompt.
- **FR-2:** When a suspend signal is observed, the agent must NOT construct a fake tool result, must NOT call the LLM again in the same turn, and must exit `generate` without returning an error.
- **FR-3:** `Response` must gain a `Status` field with at least two values: `ResponseStatusCompleted` and `ResponseStatusSuspended`. Existing responses continue to return `Completed`. Suspended responses are returned as `(*Response, nil)` — suspension is NOT modeled as an error.
- **FR-4:** A suspended `Response` must expose two separate fields:
  - `PendingToolCalls`: list of `{ID, Name, Input, Prompt}` for tools that suspended and are awaiting external results. `Prompt` carries optional metadata from the tool's `SuspendResult`.
  - `CompletedToolCalls`: list of tool calls (and their results) that ran to completion in the same iteration alongside the suspending sibling(s). This is informational — the results are already persisted in the session — but surfaces them to the caller so a SaaS UI can show "here's what we finished before pausing for your input."
- **FR-5:** On suspend, the session turn saved must include the assistant message (with tool_use blocks) and a tool_result message containing results for any completed sibling tools, with pending tool calls represented in a way the agent can detect on resume (see FR-7).
- **FR-6:** There must be a resume API callable via the existing `CreateResponse` surface — preferred — accepting something like `WithToolResults(map[string]ToolResult)`. A separate `ResumeResponse` method is acceptable if it keeps `CreateResponse` simple.
- **FR-7:** On resume, the agent must inspect the session, identify the pending tool calls, verify that the caller-provided results cover them (fully or partially), merge them into the turn's tool_result message, and resume the generation loop without re-running the LLM call that produced the tool_use.
- **FR-8:** Parallel tool execution must treat suspending tools as non-fatal: other tools in the same batch continue and their results are collected. Only after the batch completes is the suspend surfaced.
- **FR-9:** Hooks behave as follows during a suspend:
  - `PreToolUse` fires for the suspending tool (it runs normally up to the point of suspend).
  - `PostToolUse` does NOT fire for the suspending tool.
  - `PostToolUseFailure` does NOT fire for the suspending tool (suspend is not a failure).
  - `PostGeneration` DOES fire, with `Response.Status == ResponseStatusSuspended`, so existing hook authors get a single consistent end-of-turn signal.
  - A new **`OnSuspend` hook** fires specifically when the agent transitions into a suspended state. It receives the suspended `Response` (including `PendingToolCalls`) and is the preferred integration point for SaaS flows that need to trigger external notifications (emails, task creation, webhook dispatch) when input is needed.
- **FR-10:** Sequential tool execution supports suspend as follows: if a sequential batch `[A, B, C]` is executing and B suspends, A's result is kept, B becomes pending, and C is skipped. On resume, once B's external result is supplied, the agent injects B's result and then runs C before proceeding to the next LLM call. C is NOT treated as pending while B is suspended — it simply hasn't started yet.
- **FR-11:** If the caller provides tool results for tool calls that don't exist in the pending set, the resume API must return a clear error and make no state changes.
- **FR-12:** If the caller provides results for only some pending tool calls, the agent must remain suspended, updating the session to reflect the newly-supplied results, and return a new suspended `Response` listing the still-pending calls.
- **FR-13:** Suspend/resume must work with both in-memory (`session.MemoryStore`) and persistent (`session.FileStore`, and any future store) sessions.
- **FR-14:** Streaming (`WithEventCallback`) must receive events up to the suspend point normally. A new `ResponseItemTypeSuspended` terminal item must be emitted carrying the same `PendingToolCalls` and `CompletedToolCalls` as the final `Response`, so stream consumers get a typed signal without having to reconcile against `Response.Status` after the stream ends.
- **FR-15:** Tools that do NOT suspend must pay zero API cost. Existing `FuncTool[T]` authors should not need to touch their code, and their tools should behave identically to today.
- **FR-16:** `WithToolResults` accepts `map[string]*dive.ToolResult` — reusing the existing `dive.ToolResult` type. Callers can supply a success result (`NewToolResultText("...")`), a content-block result, or a failure result (`IsError: true`) indicating the user declined or the external fulfillment failed.
- **FR-17:** A resumed tool call whose supplied `ToolResult` has `IsError: true` flows through the normal failure path: `PostToolUseFailure` hooks fire as if the tool itself had failed. The LLM sees an error tool_result and decides how to react.
- **FR-18:** Multi-iteration suspend is supported. An agent may suspend, be resumed, advance one or more iterations, and then suspend again within the same logical turn. The session persistence layer must handle this — each suspend/resume cycle writes a consistent snapshot, and the `CreateResponse` call that finally returns a `Completed` response sees a single coherent turn from the caller's perspective (even though under the hood it was assembled across multiple process invocations).
- **FR-19:** Suspended session detection must use BOTH an explicit flag persisted by the session AND the structural invariant (assistant message with unmatched tool_use blocks). The flag is authoritative and queryable via `session.Store.List()`; the structural invariant is a correctness check. The two must always agree; a mismatch is a bug.
- **FR-20:** The `OnSuspend` hook fires BEFORE `PostGeneration` for suspended responses. `PostGeneration` still runs so existing hook authors (metrics, logging, skill/compaction packages) see a single consistent end-of-turn signal regardless of status.
- **FR-21:** `Agent.responseTimeout` applies only to active execution time. When `CreateResponse` returns a suspended response, the timer is effectively reset — each subsequent resume call starts with a fresh timeout budget. A multi-day external wait does not consume the timeout.
- **FR-22:** On resume, the agent's current `model` and `systemPrompt` are read via the existing snapshot-under-lock pattern. If `SetModel`/`SetSystemPrompt` was called between suspend and resume, the new values are honored. This matches existing per-call snapshot behavior and must be documented clearly.
- **FR-23:** Context cancellation during resume behaves exactly as it does in a normal `CreateResponse` call today — the loop unwinds at the next cancellation point and may leave the session with partial writes. Callers who care about all-or-nothing resume semantics must not cancel `ctx` mid-resume. This must be documented in the resume API reference.

## 6. Non-Goals (Out of Scope)

- **Scheduling / reminders.** This mechanism does not schedule follow-ups, send reminders to users, or time out suspended agents. Those belong in a higher layer (the SaaS integrator's workflow engine, `experimental/cron`, etc.).
- **Built-in timeout on suspended turns.** The caller decides when a suspension has waited too long and cancels. We won't build an auto-expiry in Dive core.
- **Multi-agent coordination.** Suspending agent A to wait on agent B is a use case, but it should be expressible using the same primitive without any agent-to-agent plumbing in core.
- **Cross-conversation resume.** Resume is scoped to the same session. Moving a suspended turn between sessions is out of scope.
- **Changing the wire format of LLM messages.** Any "pending" marker is internal to Dive's session representation, not sent to the LLM.
- **UI for pending state.** Surfacing the suspended state to humans is the integrator's job.
- **Subagent-internal suspension.** In v1, tools running inside a subagent (`experimental/subagent`) MUST NOT return `SuspendResult`. Subagents are opaque tool-call wrappers from the parent's perspective; nested suspend-resume across the parent/subagent boundary is deferred. If a subagent tool tries to suspend, the subagent call returns a clear error. Revisit when a concrete use case shows up.
- **Dedicated cancel API.** No `CancelSuspendedResponse` method in v1. Callers abandon a suspended turn by invoking `WithToolResults` with `IsError: true` entries for every pending call. If this pattern proves common enough to deserve sugar, add a method in v2.

**Future considerations (deferred but worth designing for):**
- A convenience helper for building webhook-driven resume flows.
- MCP tool bridge: allowing an MCP tool to signal suspend via its protocol.

## 7. Dependencies & Risks

| Risk / Dependency | Impact | Mitigation |
|---|---|---|
| Session stores must round-trip partial turns (assistant msg + partial tool_result). | Resume silently loses data if a store drops unknown fields. | Audit `MemoryStore` and `FileStore` for round-trip fidelity; add regression tests for the suspended shape. |
| Anthropic/OpenAI strictness about unmatched `tool_use` blocks in message history. | On resume, if we reconstruct messages wrong, the LLM provider rejects the request. | Never send a suspended state to the LLM. The resume path must synthesize the matching `tool_result` block before any generate call. Add provider-level tests. |
| Parallel execution ordering. | Non-suspending siblings may have side effects the user later wants to undo if they cancel the suspension. | Document clearly: side effects in siblings are committed; cancellation does not roll them back. Tool authors can opt into sequential execution if this matters. |
| Streaming consumers assume a completed final message. | Existing streaming UIs break when they receive a suspended response. | Introduce a clearly-typed suspended terminator event; document the migration. |
| `ToolCall ID` uniqueness across a session over long time spans. | Resume lookups might collide. | IDs already come from the LLM and are turn-scoped; we only need to resolve them within the suspended turn, not globally. |
| Concurrent resume attempts (two callers both try to resume the same session). | Race conditions corrupt session state. | Session stores must serialize writes; document that resume is not safe to call concurrently on the same session. |

## 8. Assumptions & Constraints

**Assumptions:**
- Integrators using suspend/resume are using a persistent `session.Store`, or knowingly accept that an in-memory store loses state on restart.
- LLM providers continue to accept conversations where a prior assistant turn contained tool_use blocks that were later satisfied.
- Tool call IDs within a single turn are unique (currently true for all supported providers).

**Constraints:**
- Must not break the existing `Agent`, `Tool`, `FuncTool[T]`, `Response`, or `Session` public API. Additive only.
- Must respect the project's library-first philosophy: no CLI coupling, no global state, explicit options.
- Go 1.25, existing provider set.

## 9. Design Considerations

**Agent loop integration points** (for reference, not a spec):
- `executeToolCallsParallel` / `executeToolCallsSequential` need to recognize the suspend sentinel and propagate it upward alongside completed results, not as an early error.
- `generate` needs a new terminal condition: "batch contained at least one suspend" → return partial state.
- `CreateResponse` needs to persist the partial turn and return a suspended `Response` instead of running PostGeneration / SaveTurn as if completed (or: run them with explicit suspended status so hook authors can react).
- Resume detection lives in `prepareMessages`/`CreateResponse`: if the session ends in an assistant message with unsatisfied tool_uses, enter resume mode.

**API shape sketch** (for alignment, not final):
```go
// Tool author:
return &dive.SuspendResult{
    Prompt: "Waiting for approval from @alice",
}, nil

// Caller observing a suspension:
resp, err := agent.CreateResponse(ctx, dive.WithInput("..."), dive.WithSession(sess))
if err != nil {
    return err
}
if resp.Status == dive.ResponseStatusSuspended {
    for _, pending := range resp.PendingToolCalls {
        // persist pending.ID, pending.Name, pending.Input somewhere the user can answer
    }
    return // handler exits, goroutine releases
}

// Caller resuming later (possibly in a new process):
resp, err := agent.CreateResponse(ctx,
    dive.WithSession(sess),
    dive.WithToolResults(map[string]dive.ToolResult{
        "toolu_abc": {Content: "User said yes"},
    }),
)
```

## 10. Technical Considerations

- **Provider compatibility matrix.** Need to verify that Anthropic, OpenAI, Google, and Ollama all accept a resumed conversation where the `tool_result` was assembled after the fact. Anthropic is strictest about this and is the primary test bed.
- **Hook interaction.** `PostGeneration` currently runs after every `CreateResponse`. Deciding whether it runs for suspended responses — and whether a new `OnSuspend` hook is introduced — affects the `skill`, `compaction`, and `subagent` packages.
- **Subagent interaction.** `experimental/subagent` launches nested agents. If a subagent suspends, the parent must also suspend cleanly. The primitive should compose without any subagent-specific branching.
- **Test fixtures.** Add a mock LLM and a mock tool that can be scripted to suspend, resume, and suspend again, for deterministic tests.

## 11. Open Questions

All prior open questions have been resolved and folded into the requirements above. Remaining uncertainty is implementation-level and belongs in a design/technical doc rather than this PRD:

- **Exact Go type signatures** for `SuspendResult`, `WithToolResults`, `PendingToolCall`, `CompletedToolCall`, and the `OnSuspend` hook.
- **Provider-level verification** that Anthropic, OpenAI, Google, and Ollama all accept a resumed conversation where tool_results are synthesized after the fact.
- **Test strategy** for the "suspend, resume, suspend again" multi-iteration case — likely a scriptable mock LLM plus a scriptable tool.
