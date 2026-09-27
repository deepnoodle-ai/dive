# Dive v2: consolidated integrator feedback

**Date:** 2026-09-26  
**Status:** Feedback register for design discussion; not an approved v2 specification.  
**Baseline:** Dive v1.34.0 (`da15e91`).

Dive's consumers repeatedly reconstruct facts the agent already knows: what
ran, which model answered, what was spent, why execution stopped, and which
messages belong in the next request. The common request is to make these facts
available through dependable contracts, while leaving application storage,
authorization, budgets, and presentation under the caller's control.

This document ports the critiques, proposed changes, reasoning, and important
counterarguments from the sources below. It preserves Nvoken's `DIVE-01` through
`DIVE-35` identifiers. Other identifiers are local to this consolidation.
Historical defects, proposed interfaces, and current guarantees are kept
separate. An item being included does not mean its proposed solution is accepted.

## Reading map

- [Sources and evidence](#sources-and-evidence)
- [What v1.34 already delivered](#what-v134-already-delivered)
- [Noodle's original requirements](#noodles-original-requirements)
- [Nvoken's 35 recommendations](#nvokens-35-recommendations)
- [Mobius feedback](#mobius-feedback)
- [Swarm feedback](#swarm-feedback)
- [Earlier Nvoken capability review](#earlier-nvoken-capability-review)
- [Application-owned findings](#application-owned-findings)
- [Earlier Dive proposals](#earlier-dive-proposals)
- [Decisions and sequencing](#decisions-and-sequencing)

## Sources and evidence

Sibling-source paths below are relative to the Dive repository root. They are
provenance for maintainers with those checkouts; this document is intended to be
usable without access to them. Private implementation paths, operational IDs,
and incident payloads have been omitted from the port.

| Key | Source                                                                                                                                                                                                       | Date and scope                                                                                                                                     |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| N   | `../noodle/docs/dive-partial-turns.md`, “Dive: keep turns that don't finish”                                                                                                                                 | September 24, against v1.33.0. Noodle's request, including a comparison of Noodle, Mobius, and Nvoken recovery.                                    |
| V   | `../nvoken-cloud/docs/reviews/2026-09-25-dive-simplification-review.md`                                                                                                                                      | September 25, against v1.34.0. All 35 Dive requests, 13 integration findings, and five application cleanups. Untracked in that checkout when read. |
| G   | `../nvoken-cloud/docs/reviews/2026-08-10-dive-capability-gap-review.md`                                                                                                                                      | August 10 capability register. Primarily features Nvoken did not expose, rather than features missing from Dive.                                   |
| S   | `../swarm/docs/nvoken-feedback.md`                                                                                                                                                                           | September 25 downstream experience through Nvoken. Swarm does not directly import Dive.                                                            |
| M1  | `../mobius-cloud/docs/research/technical/2026-07-15-dive-reminder-conflict-semantics-review.md`                                                                                                              | July 15 context semantics review. Later completion notes supersede its opening release-status text.                                                |
| M2  | `../mobius-cloud/docs/research/technical/2026-07-22-dive-media-input-gaps.md`                                                                                                                                | July 22 media review; its three concrete gaps are marked resolved in v1.18.0.                                                                      |
| D   | [Incomplete turns](incomplete-turns.md), especially [the later breaking iteration](incomplete-turns.md#a-later-breaking-iteration)                                                                           | Updated September 25; records v1.34 implementation and future candidates.                                                                          |
| T   | [Tool interface proposal](../reference/tool-interface-proposal.md)                                                                                                                                           | Earlier comparative design; several proposals have since shipped.                                                                                  |
| U   | [Usage buckets](../plans/plan-11-usage-bucket-conventions.md), [streaming usage defect](../bugs/streaming-usage-double-count.md)                                                                             | Historical accounting feedback from downstream users, with implemented fixes.                                                                      |
| X   | [Tool execution and filesystem search](tool-execution-and-search.md), [streaming retries](../plans/plan-09-streaming-generation-retries.md), [unknown tools](../plans/plan-10-unknown-tool-name-recovery.md) | Existing contracts and earlier production-driven requests to retain in v2.                                                                         |

Source V reports source inspection, a scripted probe suite, two multi-process
durability tests, and selected live provider runs. It explicitly says its
application defects were established by reading, not reproduced against a
provider. Its priorities reflect one stateless integrator and are not a survey
of every Dive user. Code-size figures below are that review's approximate
measurements, not measured savings from an implemented redesign.

This consolidation did not rerun those probes. Spot checks at `da15e91` confirmed
that step checkpoints require `TurnStore`, the item stream lacks the requested
complete model-call record, Google's tool configuration uses automatic mode,
and citation deltas are defined but absent from the response accumulator's
delta switch. Other unresolved claims remain attributed to their dated sources.
The older sources' provider counts, line numbers, and deployment claims should
not be read as current inventory.

## What v1.34 already delivered

Noodle's request is the origin of substantial work that has already landed.
The [incomplete-turns design](incomplete-turns.md) records preservation on early
exit, structured outcomes, stop-reason classification, incomplete-turn closure,
soft cancellation, continuation, recoverable turn records, optional per-step
checkpoints, and session claims. Nvoken's review explicitly says its probes
behaved as the design described and that the stateless path improved.

Keep those gains. The v2 critique concerns how callers access them and how many
overlapping contracts remain. Per-step durability currently requires a suitable
session and configuration; a stateless event callback is not automatically a
complete durable journal. A recorded step also cannot prove the outcome of an
external side effect that completed just before a crash but was never recorded.

Important corrections to the original request are already reflected in v1.34:

- A running call with no observed result is **unknown**, not “not run.” An
  in-process goroutine cannot be forcibly stopped merely by returning early.
- Status and reason are separate. The implementation uses `Incomplete` plus an
  outcome, rather than Noodle's proposed separate cancelled and failed statuses.
- Preservation and persistence are different facts. Save failure and uncertain
  save outcome must remain visible even if execution itself completed.
- The durable record and the projection sent to the model serve different
  purposes. Repairing a transcript must not rewrite what actually happened.

## Noodle's original requirements

Source N found three independent implementations of partial-turn recovery.
Noodle kept partial text and replayed stopped turns. Mobius kept partial text
for display but omitted cancelled turns from model history. Nvoken discarded
partial streamed text and checkpointed completed work. These differences were
application choices; reconstructing Dive's own messages was shared plumbing.

| ID   | Critique and requested behavior                                                                                                                                                    | Reasoning and v2 treatment                                                                                                                                                                                                                    |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| N-01 | Save on every exit and checkpoint each completed step, including input, model messages, tool results, and usage. Use a bounded save context independent of execution cancellation. | A stopped transcript must not disappear while its file edits or other side effects remain. v1.34 addresses this for configured sessions; expose equivalent step facts to other storage owners.                                                |
| N-02 | Keep replay history valid: answer open calls, discard partial tool JSON, handle unfinished provider state, and optionally retain draft text.                                       | Invalid history can poison every later request. Preserve the later distinction between never-started and unknown calls; cleanup rules for client and server tools differ.                                                                     |
| N-03 | Record why the turn ended as structured data and explain it to the model.                                                                                                          | Applications should not split error prose on separators, and the model should build on completed work instead of repeating it. v1.34 supplies an outcome and reminder; a dedicated outcome content type remains a breaking-version candidate. |
| N-04 | Return partial output and usage when execution ends early.                                                                                                                         | A useful partial answer or completed tool work should remain available to parents and UIs. v1.34 returns a response alongside many execution errors; DIVE-03 proposes simplifying that contract.                                              |
| N-05 | Repair unanswered tool calls in provider encoders as a final backstop.                                                                                                             | Old or externally written history should not permanently block execution. The client-call backstop shipped; DIVE-09 identifies a remaining server-call history case.                                                                          |
| N-06 | Allow stopping at a step boundary.                                                                                                                                                 | Budget exhaustion and “stop after this” should not require a sentinel callback error or interrupt an otherwise useful operation. Soft cancel shipped; typed application stop reasons remain requested.                                        |
| N-07 | Preserve incomplete work by default, with an explicit escape hatch.                                                                                                                | Data loss is the worse default. The compatibility escape hatch in v1.34 is itself confusing; DIVE-03 and DIVE-24 discuss its removal or clarification.                                                                                        |

### Edge cases and validation to retain

Noodle asked for tests of cancellation mid-batch and mid-stream, failure on a
later model call, continuation with valid provider history, and the opt-out
behavior. Its additional cases remain useful design requirements:

- Preserve input and outcome even if the first model call fails.
- If saved content caused the failure, retain evidence but offer an explicit
  repair hook or projection policy. Otherwise the same image/model mismatch
  can fail every later attempt. Pre-existing bad history is a separate problem.
- Close a cancelled suspension consistently without rerunning completed work.
- Keep full turn history when the working model context was compacted.
- Identify hook aborts and preserve results from a partially completed batch.
- Decide explicitly whether drafts and stopped turns are shown, replayed, or
  omitted. Storage preservation alone must not decide model visibility.
- Test crash recovery as well as orderly cancellation. Checkpointing completed
  steps does not guarantee recovery of uncheckpointed streamed bytes.

The intended application deletion was its partial-message assembler, sidecar
journal and recovery path, special save branch, and error-text separator. Its
transcript UI and presentation choices remain application responsibilities.

## Nvoken's 35 recommendations

These are open recommendations in source V, not an adopted plan. They are
grouped by concern while retaining every original identifier.

### Engine, persistence, and outcomes

**DIVE-01 — Let sessions consume the engine's step journal.** The agent currently
loads, locks, claims, recovers, and saves through several optional session
interfaces. A stateless runtime reconstructs roughly 270 lines of checkpoint
state from callbacks that lack some necessary facts. Proposed: an engine taking
explicit history/input and emitting typed steps through a synchronous sink. A
`session.Runner` would load and claim sessions, run the engine, and persist those
steps. The sink can stop progression before the next action if recording fails.
Turn identity and recovery helpers should be available without adopting Dive's
store. This moves session errors, revisions, claims, persistence fields, and
session-only options out of the engine-facing surface. The cost is migration
for session users, including the CLI, examples, demos, and A2A cancellation.

This differs from source D's proposal to fold the existing session interfaces
into one turn store while leaving it inside the agent. Both seek fewer
contracts; only the inversion makes the journal equally available to an
application-owned runtime. Neither architecture is selected here.

**DIVE-02 — Separate history, input, and pending results.** `WithMessages` and
`WithInput` share storage, but their meaning changes with sessions, resume, and
continue options. Some combinations are invalid and option order can overwrite
results. Proposed: `WithHistory` always means prior conversation; new input
always means new input; pending tool results can be applied to plain history
without requiring a Dive suspension object. This removes hand-built replay
messages and allows resumed results to receive normal hooks and events. The
matching and replay rules need to remain explicit and safe.

**DIVE-03 — One authoritative report of execution.** Returned errors,
`GenerationError`, response status, turn outcome, suspension copies, and terminal
items overlap. Proposed: errors mean execution did not begin; after it begins,
the response describes its outcome. Remove compatibility duplicates, `Discard`,
and the separate suspended terminal item. Keep merged turn messages in one
place, distinguish soft and hard cancellation, and define invocation versus
whole-turn usage. This replaces an integrator's long error-priority ladder with
one outcome interpretation. Persistence failures still need honest representation.

**DIVE-04 — Record every model call.** Message items omit iteration, requested
and served model, response ID, stop reason, and planned tool disposition. A call
whose content becomes empty may emit no message item at all. Proposed: always
emit a model-call record with those facts, usage, and message, including empty,
refused, truncated, and paused responses. Report served model separately from
wrapper/provider name. Nvoken built about 220 lines of evidence wrappers and
special settlement logic because missing records obscure spend and recovery.

**DIVE-05 — Typed external stops.** Applications currently use callback errors
or cancellation to represent budget limits, interrupts, deadlines, and credit
refusal. Proposed: a sink/callback decision carrying a caller-defined stop reason
and details; soft cancellation can carry the same reason. A deliberate policy
stop should not masquerade as callback failure, and callers should not have to
reconstruct its cause from sentinel errors and side state.

**DIVE-06 — Distinguish tool failure from turn-fatal infrastructure failure.**
A Go error from a tool normally becomes model-facing tool failure and the loop
continues. A lost lease or failed durable acceptance is different: a side effect
may already have happened, and another model call may repeat it. Proposed: a
typed fatal error/decision that stops the turn, an iteration accessor in tool
context, and tool middleware for shared durable start/accept behavior. Ordinary
recoverable tool errors must remain recoverable.

**DIVE-07 — Make loop policy explicit and turn-wide.** The loop forces a final
answer with injected text and `ToolChoiceNone`, resets its limit on Stop-hook
continuation, and fixes the provider-pause bound at ten. Proposed: a turn-wide
model-call limit, an incomplete outcome at the limit, optional final-answer
forcing, configurable pause limits, and per-iteration model settings. This lets
applications set remaining output budget or first-call-only tool choice without
wrapping the model or paying to circumvent built-in policy.

**DIVE-08 — Record input delivered during execution.** Hook-appended messages
can be model-only, while Stop continuation accepts a reason string rather than
structured input. Proposed: an `InputDelivered` step, structured continuation
messages, explicit recorded/model-only delivery, and turn-wide counts on Stop
context. A session user should not lose a nudge merely because it arrived between
iterations; durable applications should not have to invent their own record.

### Provider behavior and integration

**DIVE-09 — Repair stranded server calls on replay.** The client-call backstop
does not cover unanswered provider server calls in historical messages. A paused
response interrupted before resend can leave such a block mid-history, where a
provider may reject it. Proposed: encoder cleanup of unanswered server calls in
all but the final message, preserving valid continuation state at the tail.
Keep the stored record unchanged; document why this differs from client-call
repair. Nvoken currently performs this cleanup itself.

**DIVE-10 — Preserve citations in streaming.** Citation delta types exist but
are not accumulated, and the OpenAI Responses streaming path loses annotations
available in non-streaming decoding. Nvoken hides the streaming interface for
provider-tool turns to obtain a complete transcript. Proposed: accumulate
citations and annotation events, and offer an explicit streaming choice. A
caller should not trade live output for evidence or disguise a model's interface
just to select a mode.

**DIVE-11 — Consistent model-call middleware.** Hook coverage differs between
providers and streaming/non-streaming paths; encoded bodies are not uniformly
available; error hooks and per-call credentials are not consistently honored;
retries cannot be vetoed. Proposed: before/after/error phases everywhere,
encoded-request evidence or token estimates, typed call refusal, per-call
credentials/client options, absolute first-output/completion times, and retry
observation with veto. Nvoken's credential and budget wrappers cost about 250
lines and sometimes construct a provider client for each model call. Retry
attempts must remain visible to budgets and accounting.

**DIVE-12 — Typed provider errors.** Status/code/retry-after accessors do not
provide normalized failure categories or consistent body, type, message, and
parameter access. Nvoken parses error strings and HTTP bodies across roughly
430 lines to recognize context overflow and rejected media; some providers
cannot be classified through its current transport. Proposed: structured raw
evidence plus normalized categories, including context-window and media rejection
details when supplied. Missing provider details should remain unknown, not
invented. This moves provider dialect knowledge to the provider adapter.

**DIVE-13 — Provider construction parity.** Missing client injection in Google
and Grok forces custom construction. Nvoken's hand-built xAI provider omitted
prompt-cache-key support. Proposed: consistent client injection and an option to
disable environment fallbacks. Applications should not copy provider setup to
attach a transport, and explicit multi-tenant configuration should not be altered
by ambient endpoint or credential settings.

**DIVE-14 — Shared capabilities and strict validation.** Model metadata,
reasoning tables, and private adapter checks are fragmented. Nvoken duplicates
them in a roughly 1,327-line catalog plus admission checks and drift fixtures.
Proposed: exported per-model capabilities covering reasoning, temperature,
tool-choice interactions, modalities, maximum output, and server tools; strict
request validation rather than silent clamping; exported derived pricing with
a content version and documented model-alias fallback. Applications retain
offering/recommendation policy. The older assumption that existing generated
catalogs already contain all this information is too optimistic.

**DIVE-15 — Versioned message codec and visibility.** Stored message JSON lacks
a dialect version, while provider-private replay metadata shares content blocks
with public material. Nvoken maintains conversion helpers and a roughly 610-line
private-artifact sidecar layer. Proposed: a versioned codec, explicit public versus
provider-private classification, and split/merge helpers. Applications still own
storage and disclosure policy; the library should own how its own blocks round-trip.

**DIVE-16 — Observability parity.** The tracer omits cost and reasoning usage,
and fixed tracer attributes force per-request construction for tenant/turn data.
Requested and served models are confused when the request model is absent.
Proposed: complete usage attributes, per-call attributes, and separate requested
and served model fields. This eliminates supplemental instrumentation maintained
solely to fill missing library facts. See DIVE-34 for further findings.

### Compatibility, vocabulary, and testability

**DIVE-17 — Signal behavioral changes explicitly.** The review reports 28
“Changed” entries across v1.33/v1.34 and 24 broken Nvoken tests after stop-reason
classification changed. Unknown stops now withhold tool calls; truncated responses
can return an incomplete status with nil error; paused responses are resent.
Proposed: opt-in semantics until a major release, or unavoidable changes paired
with prominent migration instructions for stateless callers. Refresh stale session
design docs and make invalid fixtures fail clearly. Compiling unchanged does not
mean an integrator will settle outcomes correctly.

**DIVE-18 — Clear stop vocabulary and explicit refusal.** Provider stop strings,
model-call stop kinds, and turn reasons overlap without matching. “Incomplete”
describes both a particular provider ending and the general turn status. An empty
refusal can look like completed execution with no message. Proposed: distinguish
those vocabularies, expose classified stop kind alongside raw evidence, and make
refusal a typed fact with details. Always retain the model-call record. Applications
must distinguish refusal, valid tool-only work, and genuinely empty execution.

**DIVE-19 — Put execution state on tool results.** Consumers currently inspect
several sentinel/type errors to learn whether a call ran. A `tool_call` item may
describe a call that never starts, and suspension can first emit an empty apparent
success. Proposed: explicit call state, separate requested/planned/started events
or disposition, and waiting instead of a success-shaped result for suspension.
Reserve tool errors for actual tool failures; make event names match observable
execution so audit and settlement code need not reverse-engineer the loop.

**DIVE-20 — One tool-input type.** Tool inputs vary among `json.RawMessage`,
`map[string]any`, and `any`, forcing normalization and repeated marshaling.
Proposed: raw JSON at API boundaries and generic decoding inside typed tools.
This gives live and replayed calls the same representation. Any desired non-JSON
tool support needs an explicit separate design rather than accidental `any` behavior.

**DIVE-21 — Supported test models.** Hand-built responses without stop reasons
changed meaning in v1.34. Dive and its consumers maintain many incompatible fakes.
Proposed: `llm/llmtest` with scripted Generate/Stream behavior, request capture,
cancellation blocking, broken streams, and realistic response constructors.
Useful defaults can select tool-use versus end-turn; explicit malformed fixtures
must remain possible. Shared conformance fixtures reduce semantic upgrade surprises.

**DIVE-22 — One reasoning intent.** Effort, token budget, thinking mode, and
display interact differently across providers. Integrators set several fields to
express one choice and then qualify combinations themselves. Proposed: a reasoning
value such as off, effort, or budget, plus separate display preference; providers
apply it or reject it. Retain deliberate provider-specific escape hatches. Silent
clamping must not make recorded settings disagree with effective execution.

**DIVE-23 — Explicit limit units and zero semantics.** `ToolIterationLimit`
allows one more model call than its name suggests to Nvoken, and zero means a
default of 100 tool iterations. Proposed: document/name the unit, distinguish
unset from zero, and align with turn-wide accounting. A runtime must know whether
its limit bounds model calls, tool batches, or something else before it can use
that limit for spend control.

**DIVE-24 — Clarify discard and synthetic messages.** `Discard` does not remove
all incomplete-turn output: model-limit endings still close, and output messages
can include synthetic tool answers and user-role reminders. Proposed: replace
the option with an explicit contract or remove it under DIVE-03; mark library
message origin. Applications should deliberately render/filter generated context,
rather than mistake it for a user's message or assume an option suppresses it.

### Hooks, tools, defaults, and auxiliary packages

**DIVE-25 — Make hook authority visible in types.** One mutable `HookContext`
serves many phases whose writable fields and error behavior differ. Permission
infrastructure errors can become denials; rewritten tool input is not consistently
reflected in preview/events/tracing; later hooks can rewrite already-approved
arguments. Reminders can be queued where no delivery boundary remains, and
PostGeneration is not an every-exit hook. Proposed: phase-specific inputs and
typed allow/deny/suspend/abort decisions, consistent effective-input propagation,
defined rewrite/authorization ordering, errors for undeliverable reminders, and
one TurnEnd hook on every exit. Refresh hook documentation to match behavior.

**DIVE-26 — Preserve injected context across continuation.** Rebuilding history
for a Stop-hook continuation drops earlier model-only hook additions. Nudges
arrive as raw messages at one seam and fixed reminder text at another; continuation
input is not emitted consistently, and iteration numbering restarts. Proposed:
retain the working message set or deliver recorded structured input at both seams,
emit that input, and count across the entire turn. This prevents a model from
losing instructions it already acted on and keeps live/replayed history aligned.

**DIVE-27 — Explicit server defaults.** The review identifies six implicit
behaviors: unconditional reminder priming text, a random agent cache key, a
non-disableable default response timeout, total HTTP timeouts that cut long
streams, a null logger, and ambient environment fallbacks. Proposed: independent
options or a server preset for opt-in/needed-only priming, caller-controlled cache
keys, context-owned deadlines, stream-safe transport timeouts, an `slog` adapter
with content logging opt-in, and disabled environment fallbacks. These affect
prompt fidelity, cache reuse, tenant isolation, diagnosis, and long-running work.

**DIVE-28 — Lossless tool schemas.** The current schema representation cannot
decode nullable type arrays and drops constructs such as `anyOf`, `$defs`/`$ref`,
`const`, and `uniqueItems`; provider translation can lose more. MCP schemas can
therefore fail a whole turn or become less restrictive than the validator that
later rejects the model's arguments. Proposed: raw schema passthrough or a lossless
representation. Providers requiring a subset must reject or report translation
loss explicitly. Generated Go schemas and externally supplied schemas both need
support; convenience generation must not define the maximum expressiveness.

**DIVE-29 — Separate external, provider, and executable tools.** Host tools
currently implement dummy calls that suspend. Provider tools can carry nil schemas
and failing call methods while entering ordinary dispatch/suggestions. Unknown
calls skip hooks, complicating durable settlement. Proposed: declaration-only
external tools, a server-tool marker excluded from client dispatch, unknown-tool
handling, and validated names/collisions. The review also suggests running inline
siblings before returning external calls; ordering and side-effect safety must be
decided explicitly before adopting that policy.

**DIVE-30 — Separate execution policy from advisory hints.** Some annotations
are enforced, some only affect permissions/reconciliation, and others are unused.
Boolean hints collapse MCP's unset state into false. Proposed: explicit execution
policy for sequencing and batch halting, tri-state advisory hints with documented
defaults, and consideration of idempotence during reconciliation. Treating a hint
as permission to retry an uncertain side effect needs an explicit trust contract;
this consolidation does not adopt automatic retry from metadata alone.

**DIVE-31 — Honor, reject, or report every setting.** The review finds ignored
tool choice, penalties, reasoning/display controls, caching controls, headers,
and exported per-call configuration fields, with provider-specific differences.
Google hard-codes automatic tool choice. Proposed: strict unsupported-setting
errors, or an explicit lenient mode reporting adjustments/effective settings;
implement or remove unused configuration; support per-call model settings, tools,
and prompt overrides. Applications should not advertise controls that silently do
nothing or construct a new agent merely to vary one call.

**DIVE-32 — Canonical, safe tool results.** Missing image constructors/text
accessors, `any` content whose shape changes after JSON round-trip, missing JSON
tags, and errors serialized as `{}` all create normalization helpers downstream.
Raw infrastructure errors and panic stacks can also reach the model. Proposed:
canonical content, constructors/accessors, a serializable error representation,
and configurable safe model-facing error rendering with diagnostic detail logged
separately. Background work should preserve tool identity/context values while
detaching cancellation deliberately; parallel execution currently loses identity
in a reported background path.

**DIVE-33 — Consistent construction, names, and module releases.** Provider
option names and availability differ; one Google version option is reported
unused; `grok`, `xai`, and telemetry naming differ; capability gating can depend
on the spelling. Consumers also bump several tightly coupled modules together.
Proposed: consistent options, explicit aliases/canonical identities, and either
module skew checks or consolidated provider packaging. Consolidation trades a
simpler version matrix for a broader dependency unit; choose it deliberately.

**DIVE-34 — Complete execution observability.** Beyond DIVE-16, the review finds
missing requested model/settings, missing incomplete outcome/reason on run spans,
first-token timing represented only as an attribute, metrics losing trace context,
conversation identity tied to sessions, and undocumented MultiTracer chaining.
Proposed: populate model/effort/tool choice, outcome/reason, correctly linked timing
metrics, per-call conversation identity and attributes, provider-name mapping,
and a documented tracer composition contract.

**DIVE-35 — Make process-local assumptions explicit and replaceable.** Subagents
and background handles are in-memory; permission grants are process-local and
confirmation auto-allows without a dialog; provider registries cannot receive
full construction options and implicit fallback/match order depends on imports.
Proposed: a pluggable spawn interface for durable runtimes, explicit auto-approval
with denial when no confirmation mechanism exists, configurable registry factories,
and opt-in fallback. Dive need not become a distributed scheduler to expose these
seams. Local convenience should remain available and accurately documented.

## Mobius feedback

### M-01 — Events and snapshots need different context semantics

Source M1 shows why neither one global “latest wins” rule nor one global
“accumulate unless conflicting” rule is sufficient. Independent user nudges and
memory deltas must accumulate. A complete skill/repository/entitlement snapshot
must express removal by omission. Under accumulation, an omitted entry does not
contradict the earlier block, so stale entries can remain model-visible.

The proposed API is a per-reminder cumulative versus snapshot disposition,
declared when constructing context and explained in its rendering. Until then,
snapshot text must explicitly claim completeness and empty snapshots must state
that nothing remains. This applies to customer-supplied runtime context too,
not just Dive's built-in skill catalog. Recorded/model-only lifetime and
cumulative/snapshot semantics are separate dimensions.

### M-02 — Preserve the individual findings from that review

| Original finding | Critique, suggestion, and status in the source                                                                                                                                                                      |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| F1               | An empty skills reminder asserts nothing and cannot evict a stale catalog under accumulation. Emit an explicit “no skills available” statement. Marked implemented.                                                 |
| F2               | A smaller catalog still looks additive unless it claims completeness. Say unlisted skills are unavailable. Marked implemented.                                                                                      |
| F3               | Identical “more not shown” text used different denominators for a baseline and a delta. Label changed entries distinctly. A Mobius-side ambiguity, not proof of a model error; current resolution not audited here. |
| F4               | Hard-deleted memories cannot appear in a since-timestamp delta. Add explicit removal/tombstone information; compaction/rebaselining only bounds the stale period. Application-owned finding.                        |
| F5               | Repository/environment snapshots can omit old entries without contradicting them. Declare completeness or remove entries explicitly. Application content plus M-01's library semantics.                             |
| F6               | A test asserting substrings of the priming constant is a wording tripwire, not evidence that a model interprets updates correctly. Keep structural/replay tests and distinguish them from behavioral proof.         |
| F7               | Many registered reminder names had no emitters. Prune or implement unused vocabulary. Historical Mobius inventory, not a current Dive defect.                                                                       |
| F8               | Arbitrary customer `app-*` snapshots inherit the same ambiguity. Surface disposition through runtime-context APIs and document the interim completeness convention.                                                 |

### M-03 — Media must be carried or rejected explicitly

Source M2 found three provider gaps: Google silently discarded document blocks,
OpenAI Responses rejected URL documents, and the Chat Completions path rejected
images/documents, affecting compatible providers. All three are marked resolved
in v1.18.0. Do not reopen them as v2 features.

Retain the general contract: every content encoder must handle a block or report
that it cannot. A successful request that silently omits an attachment invites
confident answers about content the model never saw. Canonical content storage,
provider capability validation, and explicit translation loss belong together.

### M-04 — Usage normalization must cover streaming and aggregation

The [streaming usage report](../bugs/streaming-usage-double-count.md) attributes
an input double-counting defect to a Mobius report. Cumulative start/delta frames
were added as if they represented independent calls. The fix uses cumulative
absorption within a response and addition across calls. Its residual requests
were streaming invariants for every provider and a Chat Completions fixture where
usage and choices arrive together; their current test status needs a fresh audit.

The separate [input-bucket plan](../plans/plan-11-usage-bucket-conventions.md)
normalizes uncached input, cache reads, and cache creation into disjoint buckets.
Provider wire conventions must not leak into shared pricing arithmetic. Retain
tests for both dimensions: snapshot-versus-delta frames and subset-versus-disjoint
token buckets. Both affect budgets, billing estimates, and context gauges.

## Swarm feedback

Source S describes Nvoken behavior observed by a downstream application. Preserve
these needs as integration evidence; they do not establish that Dive caused the
reported incident or that Dive should implement a service console or ledger.

| ID   | Critique and suggestion                                                                                                                                                                                                                                                   | Owner and relevance to Dive                                                                                                                                          |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| S-01 | Successful tool work followed by empty final text was failed as a provider error. Permit tool-only completion; distinguish empty/no-work execution with its own reason.                                                                                                   | Nvoken settlement. Dive should expose enough outcome, refusal, usage, and tool evidence to make the distinction. The source did not fully trace the empty-text path. |
| S-02 | Hard spending caps depended on credits. Offer plain caps independently of credit policy.                                                                                                                                                                                  | Service budget product. Dive needs enforceable call boundaries and typed stops, not a credit system.                                                                 |
| S-03 | Short turns waited roughly six seconds queued. Investigate polling, worker, or lease delay.                                                                                                                                                                               | Service scheduling; no Dive cause established.                                                                                                                       |
| S-04 | The UI showed only uncached input, hiding most context tokens. Show total input with cached breakdown.                                                                                                                                                                    | Service UI; requires well-defined Dive usage buckets.                                                                                                                |
| S-05 | Failure classification was available through the API but hidden in the console. Display the actionable class beside the stop reason.                                                                                                                                      | Service UI; typed error/outcome evidence supports it.                                                                                                                |
| S-06 | Disabled logs looked like an empty log. Make disabled state explicit or enable development logging by default.                                                                                                                                                            | Service configuration/UI. Complements explicit logger behavior in Dive.                                                                                              |
| S-07 | SDK transport error text omitted the wrapped cause. Include the cause while preserving unwrapping.                                                                                                                                                                        | Nvoken SDK; general diagnostic lesson, not a Dive-specific finding.                                                                                                  |
| S-08 | “Agent: Inline behavior” was unclear. State that behavior was supplied per turn and no named agent was used.                                                                                                                                                              | Service copy.                                                                                                                                                        |
| S-09 | Estimated floating-point cost did not equal charged money; SDK precision and per-call rounding compounded the mismatch, uncertain calls could be missing, and conversation totals required paging. Expose exact charged amounts, conversation rollups, and credit events. | Service accounting. Dive should preserve cost provenance and complete call evidence; estimates and charges must remain distinct.                                     |
| S-10 | Turn results did not expose available reasoning content. Offer a deliberate reasoning surface and meaningful redaction metadata.                                                                                                                                          | Service disclosure contract, dependent on faithful provider content handling. It does not imply every provider exposes reasoning.                                    |
| S-11 | Input authorship lived in a forgeable text header. Carry structured sender identity into history and rendering.                                                                                                                                                           | Service/application identity, with a possible Dive message metadata seam. A model-facing name is not an authorization boundary.                                      |

## Earlier Nvoken capability review

Source G asks the reverse question from source V: which Dive capabilities did
Nvoken fail to expose? Its September 25 review index marks `USAGE-01` and `GEN-01`
closed and the rest not comprehensively rechecked. The table preserves every
register item without treating old provider counts or “missing” claims as current.

| ID         | Critique/suggestion and reasoning                                                                                                | Boundary for v2                                                                                                                                           |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| USAGE-01   | Inconsistent cache-token accounting overstates cost and prematurely consumes budgets.                                            | Closed in the later index; preserve disjoint bucket and stream invariants.                                                                                |
| CTX-01     | Expose typed recorded/model-only context so applications need not churn instructions and cache prefixes to supply runtime facts. | Dive already supplies context primitives; improve their continuation and snapshot contracts.                                                              |
| TOOL-01    | Expose more provider-run search/code/file tools instead of maintaining application equivalents.                                  | Primarily Nvoken product coverage; depends on provider-tool fidelity.                                                                                     |
| GEN-01     | Reasoning content was hidden from hosts regardless of preference.                                                                | Marked closed in the later index; Swarm's later report concerns its observed result surface and should be reconciled separately.                          |
| PROV-02    | Hand-maintained model catalogs duplicate provider facts.                                                                         | Shared capabilities are useful, but V establishes that today's generated catalog is incomplete for admission validation.                                  |
| POL-01     | Reuse a permission vocabulary for tool availability and result submission.                                                       | Caller/principal checks belong at admission, offer, and result-submission boundaries; a loop hook cannot authorize a future submitter in another process. |
| ORCH-01    | Durable child turns could provide subagents with owned identity, spend, and recovery.                                            | Runtime-owned orchestration; DIVE-35 proposes a pluggable seam. The old review explicitly called demand a bet.                                            |
| PROV-01    | Broader provider/compatible-endpoint coverage could reduce model restrictions.                                                   | Service market choice, not proof Dive needs new providers. Catalog maintenance was a prerequisite; demand was unvalidated in that review.                 |
| GEN-02     | Expose citations to hosts.                                                                                                       | Product surface plus DIVE-10's streaming fidelity.                                                                                                        |
| ORCH-02    | Support background tool work while the loop continues, not only a parked turn awaiting results.                                  | Durable runtime feature; process-local handles alone are insufficient.                                                                                    |
| POL-02     | More hook seams could support application policy and observability.                                                              | Expose useful decisions rather than reproducing every internal hook remotely. DIVE-25 clarifies those decisions first.                                    |
| INTEROP-02 | OAuth-authenticated MCP servers were not exposed.                                                                                | Service credential and refresh lifecycle; distinguish existing library support from integration coverage.                                                 |
| INTEROP-01 | A2A could make agents reachable through a standard interface.                                                                    | Adapter/product decision; Dive already has an A2A package.                                                                                                |
| GEN-03     | Pass through speed, parallel calls, caching, penalties, features, and headers where supported.                                   | DIVE-31 requires validating actual support before advertising controls.                                                                                   |
| TOOL-02    | Skills were absent.                                                                                                              | Product choice with positioning tension, not necessarily a library gap.                                                                                   |
| TOOL-03    | Static tool assembly omitted dynamic Toolset/Extension behavior.                                                                 | Dive already has these primitives; choose how the service exposes them.                                                                                   |
| MEDIA-01   | Image/video/speech generation was absent.                                                                                        | Experimental Dive capability and service scope, not an automatic v2 requirement.                                                                          |
| CTX-02     | Compaction only at turn boundaries may not serve long turns.                                                                     | Verify actual need and experimental behavior before committing; record versus model projection must stay separate.                                        |

## Application-owned findings

Source V also found integration defects that a cleaner Dive contract would
prevent or simplify. They are preserved here so moving feedback into Dive does
not accidentally transfer ownership or hide necessary application fixes.

| Source ID | Finding and suggested response                                                                                                                                                                                 |
| --------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| USAGE-02  | Custom xAI construction omitted prompt-cache-key support. Add the option locally; construction parity removes the copied setup.                                                                                |
| ORCH-03   | Durable tool infrastructure errors became recoverable model-facing failures. Until a fatal-tool contract exists, classify those errors and abort through the supported failure hook before another model call. |
| PROV-03   | Ambient OpenAI endpoint/organization/project/header settings could alter explicit server configuration, including compatible providers. Pin/validate configuration; add a library environment opt-out.         |
| PROV-04   | Parsing provider error JSON out of `Error()` loses OpenAI-family error types. Replace string parsing with structured errors.                                                                                   |
| USAGE-03  | First-output detection differed between Dive and Nvoken. Agree on a shared definition or expose the predicate/timing facts.                                                                                    |
| USAGE-04  | Every call could consume the whole turn output allowance, including after resume. Apply remaining allowance per call; post-call totals alone cannot enforce a hard ceiling.                                    |
| GEN-04    | Empty refusal was classified as invalid response and could lose call usage. Recognize refusal explicitly and record the call even without text.                                                                |
| TOOL-05   | Host/MCP/output schemas were narrowed or rejected by schema conversion. Validate compatibility at discovery until lossless transport exists.                                                                   |
| PROV-05   | Copied five-minute total HTTP timeouts also bounded streamed body reads. Use caller deadlines and deliberate transport timeouts. The report did not reproduce a long live call.                                |
| GEN-05    | Gemini advertised tool-choice modes that the adapter ignored. Narrow advertised support until encoding is implemented and qualified.                                                                           |
| CTX-03    | Nudges used inconsistent delivery seams and disappeared across continuation. Deliver them consistently and preserve the recorded history.                                                                      |
| PROV-06   | No logger was supplied, discarding warnings the integration assumed were visible. Adapt the application logger and filter content-bearing debug output.                                                        |
| GEN-06    | Implicit reminder priming could change tenant prompts and cache prefixes on upgrade. Pin model-facing behavior in qualification until explicit configuration exists.                                           |

The five application cleanups were: split the large generation function and its
closure state into testable responsibilities (`SIM-07`); centralize served-model
fallback (`SIM-08`); record pause/stop facts instead of inferring them twice from
stored content (`SIM-09`); consolidate fake models (`SIM-10`); and translate or
document the mismatch between application model-call limits and Dive tool-iteration
limits (`SIM-11`). These can proceed without a major Dive release.

## Earlier Dive proposals

Source T requests schema derivation from typed inputs, richer tool context,
dynamic tool resolution, function-based tools, and panic recovery. The latter
three exist at the baseline, and context accessors already expose turn/call IDs.
Automatic schema generation exists for `FuncTool`; that is not identical to
removing `Schema()` from every `TypedTool` implementation. Remaining questions
are the general typed-tool contract and whether a dedicated tool context adds
value beyond ordinary Go context plus accessors.

That proposal also argues for retaining typed multimodal results and previews,
avoiding confirmation represented as fake model tool calls, keeping shared-state
mutation out of a tool-specific action framework, and deferring non-JSON input
and output schemas without demonstrated need. Preserve these tradeoffs when
considering DIVE-20, DIVE-25, and DIVE-28; the old proposal's praise for generated
schemas does not establish lossless support for arbitrary external JSON Schema.

Source X provides three further contracts to carry forward:

- Retry transient stream creation failures only within a well-defined boundary
  before output is delivered. Retrying the whole turn can repeat side effects;
  retrying a partially consumed stream can duplicate or splice output. Preserve
  cancellation, backoff, and provider parity, and make attempts observable.
- Treat unknown tool names as recoverable, clearly reported non-execution with
  useful suggestions. Never silently dispatch a similar real tool. Earlier valid
  tool work must not disappear because a later name was invented.
- Filesystem search must distinguish complete no-match from incomplete/error,
  expose truncation to the model, bound memory, and honor advertised filters and
  options across backends. Cancellation must stop admitting late callbacks while
  accurately describing non-cooperative work that may still be running.

These are existing design contracts, not a claim that every old finding remains
open. A v2 cleanup should preserve their semantics and relevant regression tests.

## Decisions and sequencing

### The central ownership decision

Noodle asks Dive to eliminate reconstruction for ordinary session users. Nvoken
asks Dive to eliminate reconstruction for runtimes that already own persistence.
These goals agree about the needed facts, but do not prescribe the same storage
architecture. Evaluate both concrete alternatives:

| Direction                                          | Benefit                                                                               | Cost or unresolved question                                                                                          |
| -------------------------------------------------- | ------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Consolidate Session into a turn store inside Agent | Shorter migration for existing session users; one storage interface replaces several. | The engine still owns storage sequencing, and whole-turn checkpoints can require relational consumers to diff state. |
| Engine emits typed steps; session runner wraps it  | One execution contract serves local sessions and application-owned durable stores.    | Requires a rigorously ordered sink contract, session API migration, and a convenient default runner.                 |

Source V also considered adopting Dive sessions inside Nvoken. It would require
step-typed checkpoints and metadata, checkpoint decisions, explicit per-tool
recovery policy, outside writers under session-wide revisions, and store-assigned
call IDs. Its estimate was roughly 800–1,000 lines removed for comparable new
integration code, while moving lease and lock-order control into Dive. That is
why it prefers inversion. This estimate is one consumer's assessment, not a
general verdict against session-backed use.

### Decisions the sources do not settle

- What is recorded before a side effect, what is acknowledged after it, and what
  happens if the final journal write fails? Define step identity, ordering,
  idempotency, cancellation, and persistence uncertainty before calling the sink
  “durable.” Telemetry callbacks and control/persistence callbacks need different
  failure semantics.
- Should v2 use response-only execution outcomes? Make preflight failure,
  execution failure, and persistence failure independently understandable.
- Which behavior is engine policy versus runner/application policy: final-answer
  nudges, iteration limits, retries, external-tool scheduling, and recovery?
- How do input authorship, message origin, context authority, visibility, and
  snapshot disposition coexist without collapsing into one ambiguous metadata bag?
- How do strict capabilities coexist with fast-changing provider features and
  deliberate escape hatches? Unsupported must not silently become ignored.
- Which compatibility layers can disappear, and how will stored messages/turns,
  sessions, providers, A2A, and experimental clients migrate independently?

The caution about idempotence hints, external-tool ordering, and the sink's exact
durability contract is synthesis added here. The underlying requests are from
the sources; these questions must be resolved before adopting their sketches.

### Suggested work tracks

**Correctness and additive integration improvements:** schema preservation,
provider error structure/client injection, stranded-server-call repair, citation
accumulation, fatal-tool errors, explicit call-result state, complete model-call
evidence, test helpers, and observability. Several can ship before v2; changing
defaults or event meaning still requires compatibility review.

**Breaking contract work:** engine/session ownership, explicit history/input,
one outcome vocabulary, typed stop/hook decisions, turn-wide limits, canonical
tool inputs/results, message codec/visibility, and reasoning intent. Design the
step journal, model-call record, and stop decisions together because each relies
on the other two.

**Application work:** service budget products, scheduling, console behavior,
charged-money rollups, principal authorization, and integration fixes above.
Do not make those product features prerequisites for a Go library release.

### How to judge whether v2 is an improvement

Use representative session-backed and stateless integrations, not only API
examples. A successful redesign should delete duplicated message reconstruction,
stop inference, provider parsing, and fake-model machinery while preserving:

- Simple interactive/session use and externally owned persistence.
- Complete accounting for empty, refused, paused, failed, and successful calls.
- Honest never-started/running/unknown tool states and no accidental replay.
- Consistent live, saved, and replayed context under nudge/continue/resume.
- Equivalent content and usage across streaming and non-streaming paths.
- Explicit unsupported settings/schema behavior and inspectable effective settings.
- Clear migration instructions for behavioral changes, not only renamed symbols.

These are evaluation criteria distilled from the feedback, not a commitment to
implement every proposed API in one release.
