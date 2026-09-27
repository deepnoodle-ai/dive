# Nvoken (nvoken-cloud): source analysis for Dive

- **Repo:** `github.com/deepnoodle-ai/nvoken-cloud` (private), checked out at `~/git/deepnoodle/nvoken-cloud`
- **Commit analyzed:** `deea133a` ("Adopt Dive 1.34 and stop settling cut-off responses as finished")
- **Dive dependency:** `github.com/deepnoodle-ai/dive v1.34.0` plus the `otel`, `google`, `grok`, `meta` and `openai` provider modules (`go.mod:8-13`)
- **Size:** 897 Go files. Non-test code is about 75k lines. `internal/services` alone is 34.6k, `internal/adapters/postgres` 11.4k, `internal/adapters/httpapi` 7.7k, and `internal/adapters/divegen` (the Dive integration) 5.6k.

Every path below is relative to the nvoken-cloud root unless it says otherwise. Go types and SQL are quoted from source, with comments trimmed where marked.

**A different kind of subject.** Eino, ADK-Go and MAF-Go are libraries. Nvoken is a service built *on* Dive. It is the one codebase in this survey that has had to make an agent loop survive process death, serve many readers at once, and stream to browsers over flaky networks. Most of what it built sits outside Dive because Dive had no place for it. That makes Nvoken the best available evidence of what Dive v2's Recorder, Observer and event vocabulary need to carry, and of what should stay in a host.

---

## 1. Overview

Nvoken is "a durable, multi-provider agent runtime for host applications" (`README.md`). A host publishes an **Agent** (immutable **AgentRevisions**) and starts **Turns**. A Turn can optionally belong to a **Conversation** (transcript continuity) and a **MemorySpace**. Nvoken runs the model/tool loop in the background and exposes JSON reads, signed webhooks, and two SSE streams.

Five design commitments shape everything else.

1. **Postgres is the execution authority.** Turn state, claims, leases, fencing, checkpoints and settlement all live there (`CLAUDE.md`, "Execution reliability invariants"). Go loops "hold leases, never authority" (`docs/index/execution.md`). Redis and the in-process bus only reduce latency.
2. **The durable core stores four kinds of thing with four rules** (`docs/proposals/2026-07-29-durable-projections.md` §1):

   | Kind | Rule |
   |---|---|
   | Facts (`idempotency_key`, deadlines, snapshots) | Written once. Immutable. |
   | Ownership (`lease_owner`, `attempt`, `lease_expires_at`) | One writer at a time, fenced. |
   | Evidence (messages, checkpoints, model-call facts) | Append-only. The record of what happened. |
   | Projections (usage totals, derived flags) | Derived from the other three. Avoid storing them. |

   The proposal's argument, "every problem this proposal fixes is a projection that was made durable and then had to be defended", is the same argument Dive v2 makes with "one record, many projections".
3. **One private, ordered log per Conversation or standalone Turn.** The log (`execution_streams`) has two gap-free counters: message `sequence` and lifecycle `revision`. Every durable write that a client can see allocates a number on one of them. Streams, cursors and replay are all defined over that log.
4. **Durable and ephemeral are different frame classes on the wire.** Saved messages and lifecycle changes carry cursors and replay identically. Token previews carry identity but no cursor, are lossy by design, and are voided explicitly when lost.
5. **Dive is used statelessly.** Nvoken never uses Dive's `session` package. It calls `dive.Agent.CreateResponse` with `WithMessages`, uses `WithEventCallback` as its journal feed, and reconstructs the step record in Postgres (`internal/adapters/divegen/generator.go:1041-1184`).

### Repo layout (non-test Go files / lines)

| Dir | Files / LOC | Role |
|---|---|---|
| `internal/domain/` | 37 / 4.8k | Pure types: `Turn`, `TurnStatus`, `ToolCall`, `TurnCheckpoint`, `GenerationDelta` |
| `internal/ports/` | 16 / 2.5k | Interfaces, around 70 in `runtime.go` alone (`Clock`, `IDGenerator`, `ToolCallCoordinator`, `LiveEventBus`, ...) |
| `internal/services/` | 75 / 34.6k | Admission, execution, recovery, tool settlement, stream reads |
| `internal/adapters/postgres/` | 35 / 11.4k | pgx and sqlc repositories. One squashed migration baseline plus generated `schema.sql` |
| `internal/adapters/divegen/` | 11 / 5.6k | The Dive integration: agent wiring, event-callback journal, host/MCP/builtin tools, delta normalization, provider catalog |
| `internal/adapters/httpapi/` | 18 / 7.7k | REST plus the SSE stream handler |
| `internal/adapters/liveevents/` | 2 / 0.5k | Lossy fan-out: in-process and Redis Pub/Sub |
| `internal/engine/`, `internal/dispatch/`, `internal/delivery/` | 10 / 2.1k | Poll/claim loop, heartbeat, reaper, Cloud Tasks dispatch, callback delivery |

The dependency direction is hexagonal: `cmd -> daemon -> adapters -> services -> ports -> domain` (`docs/design/architecture.md`, "Code and contract boundaries").

---

## 2. Core packages and types (brief)

### Turn lifecycle

```go
// internal/domain/turn.go:31-69
type TurnStatus string
const (
    TurnQueued     TurnStatus = "queued"
    TurnRunning    TurnStatus = "running"
    TurnWaiting    TurnStatus = "waiting"      // parked on host/callback tools; owns no goroutine or lease
    TurnBudgetHold TurnStatus = "budget_hold"  // a consumption limit ran out; resumable
    TurnCompleted  TurnStatus = "completed"
    TurnIncomplete TurnStatus = "incomplete"
    TurnFailed     TurnStatus = "failed"
    TurnCancelled  TurnStatus = "cancelled"
)
type TurnStopReason string // end_turn, interrupted, max_iterations, deadline,
                           // max_output_tokens, max_estimated_cost, insufficient_credits
```

The state machine (`docs/index/execution.md`, "Turn state machine"): `queued -> running` by claim; `running -> waiting | budget_hold`; `waiting | budget_hold -> queued`; an expired lease sends `running -> queued`; and settlement, cancel or the deadline reaper sends any of them to a terminal status. Both `waiting` and `budget_hold` return through `queued` before another claim, so "resume" is always "claim again".

### Checkpoints and tool calls

```go
// internal/domain/toolcall.go:193-211
type TurnCheckpoint struct {
    ID, TurnID, ExecutionStreamID, TenantID string
    Sequence               int64              // per-Turn monotonic checkpoint counter
    Iteration              int                // model iteration this belongs to
    Kind                   TurnCheckpointKind // model | tool | nudge
    LeaseAttempt           int64              // fencing token of the writer
    ThroughMessageSequence int64              // transcript watermark
    MessageID              *string
    MessageSequence        *int64
    Usage, Provenance      json.RawMessage    // model checkpoints only
    EvidenceDigest         []byte             // sha over usage+provenance, for replay equality
    ModelCallFactID        string             // link to the billing intent row
    ToolCallID             *string            // tool checkpoints only
    CreatedAt              time.Time
}

// internal/domain/toolcall.go:225-238
type ModelCheckpointInput struct {
    Iteration       int
    Message         GenerationMessage
    Usage           ModelUsage
    Provenance      ModelProvenance
    ToolCalls       []ToolCallRequest // provider call ID, name, mode, input
    MessageID       string            // the ID already published on this iteration's previews
    ModelCallFactID string
    LastOutputAt    *time.Time
}
```

### The durable tool boundary

```go
// internal/ports/runtime.go:1314-1318, 1353-1356, 1362-1370
type ToolCallCoordinator interface {
    RecordModelCheckpoint(context.Context, domain.ExecutionClaim, domain.ModelCheckpointInput) (domain.ModelCheckpointResult, error)
    StartBuiltinToolCall(context.Context, domain.ExecutionClaim, int, string) (domain.ToolCallExecution, error)
    AcceptBuiltinToolResult(context.Context, domain.ExecutionClaim, domain.ToolCallExecution, json.RawMessage, bool) (domain.ToolCall, error)
}
type MCPToolCallCoordinator interface {
    StartMCPToolCall(context.Context, domain.ExecutionClaim, int, string, bool) (domain.MCPToolCallStart, error)
    AcceptMCPToolResult(context.Context, domain.ExecutionClaim, domain.ToolCallExecution, json.RawMessage, bool) (domain.ToolCall, error)
}
type DurableToolCoordinator interface {
    ToolCallCoordinator
    MCPToolCallCoordinator
    NudgeDrainer // required so the stop seam cannot strand nudges
}
```

This interface is, in effect, a hand-built Recorder. It is the part of Nvoken that Dive v2's `Recorder.Commit(ctx, Step)` is meant to replace.

### Live output

```go
// internal/ports/streaming.go:14-50
type GenerationDeltaEmitter func(domain.GenerationDelta) // must return quickly; never blocks execution

type LiveEvent struct {
    Type              string // message.delta | stream.resync | transcript.commit (internal wake)
    ExecutionStreamID string // fan-out key: the private log
    ConversationID    *string
    TurnID            *string
    ContentExpiresAt  *time.Time
    Payload           json.RawMessage
}
type LiveEventPublisher interface{ Publish(context.Context, LiveEvent) } // may drop
type LiveEventBus interface {
    LiveEventPublisher
    Subscribe(context.Context, string) LiveSubscription // delivery established before return
}
type LiveSubscription interface {
    Events() <-chan LiveEvent
    TakeGap() bool // true once if anything was dropped since the last call
    Close()
}
```

---

## 3. Durability

### 3.1 What is persisted

The execution-owned tables (from `internal/adapters/postgres/schema.sql`, which is generated from the single baseline migration `migrations/000001_runtime_schema.up.sql`):

| Table | Kind | Granularity | Key constraints |
|---|---|---|---|
| `execution_streams` | Ownership/serialization root | One per Conversation or standalone Turn | `next_message_sequence`, `next_lifecycle_revision` counters |
| `execution_stream_messages` | Evidence | One row per saved message (user input, assistant iteration, tool-result batch, nudge, reminder) | `UNIQUE (execution_stream_id, sequence)` (`schema.sql:1774`); `visibility` is `public` or `runtime`; `generation_eligible` |
| `turn_transitions` | Evidence | One row per lifecycle change | `PRIMARY KEY (execution_stream_id, revision)` (`:1927`); **append-only trigger** (`:2248`); deferred FK from `through_message_sequence` to a real message (`:2482`) |
| `turn_checkpoints` | Evidence | One per model iteration, per settled tool call, per nudge drain | `UNIQUE (turn_id, iteration) WHERE kind='model'` (`:2118`); `UNIQUE (tool_call_id) WHERE kind='tool'` (`:2122`) |
| `tool_calls` | Evidence plus a small state machine | One per requested call | `UNIQUE (turn_id, provider_call_id)` (`:1882`); `UNIQUE (turn_id, iteration, batch_ordinal)` (`:1873`); terminal rows immutable by trigger |
| `model_call_facts` | Evidence (billing) | One per provider attempt | `status IN (prepared, started, settled, not_started, uncertain)` |
| `provider_message_artifacts` | Evidence (private) | Provider-specific payloads (reasoning signatures, server-tool blocks) per assistant message | Never streamed |
| `turns` | Current snapshot plus ownership | One per Turn | `turns_one_nonterminal_per_stream` (`:2166`): one active Turn per log; terminal Turn immutable by trigger |
| `turn_admissions`, `turn_facts`, `turn_effective_behaviors` | Facts | Frozen at admission | `UNIQUE (tenant_id, idempotency_key)` (`:1903`, `:1936`) |
| `dispatches` | Outbox for Cloud Tasks | One active per Turn | `dispatches_one_active_turn` (`:2036`) |
| `turn_nudges` | Steering queue | One per nudge | `UNIQUE (turn_id, idempotency_key)` (`:2134`) |

Two notes on the model:

- **The Turn row, not the log, is authoritative for current state.** "Current Turn state is not reconstructed by replaying the event log; checkpoints are recovery evidence and the Turn row is the authoritative current snapshot" (`docs/design/architecture.md`, "ToolCalls and checkpoints"). This differs from Dive v2, where the Turn's state is derived by replaying steps.
- **Messages are the record, not a projection.** The transcript table holds user input, assistant iterations, tool results, drained nudges, private runtime reminders (`visibility = 'runtime'`), and synthetic tool results written when a Turn ends with calls open (`syntheticToolResultPayload`, `internal/services/toolcalls.go:1156-1180`). What the *model* sees is decided partly at read time. `nvoken_message_generation_eligible` (`schema.sql:190-198`) excludes a cancelled Turn's messages and a failed Turn's non-input messages, without rewriting them:

```sql
SELECT stored_generation_eligible AND CASE
    WHEN message_turn_id IS NULL THEN true
    WHEN turn_status = 'cancelled' THEN false
    ELSE message_role IN ('user', 'system') OR turn_status <> 'failed'
END
```

### 3.2 Commit groups: granularity and transaction boundaries

Every row in this table is one Postgres transaction. The "fence" column names the check that stops a stale executor.

| Event | What commits together | Fence | Source |
|---|---|---|---|
| **Admission** | Turn, `turn_facts`, effective behavior (with digest), input messages, initial `queued` transition, dispatch intent | Idempotency key plus request fingerprint | `docs/index/execution.md` "Invariants"; `queries/turn_admission.sql` |
| **Claim** | Revision reserved, `status='running'`, new lease, `attempt = attempt + 1`, `running` transition | `status='queued'` and deadlines | `queries/turn_execution.sql:322-365` (`CommitTurnClaim`) |
| **Model call intent** | `model_call_facts` row `prepared`, then `started` | Credit and budget decision | `divegen/generator.go:2193-2242` (`resolvingModel.reserve`); `services/credit_authority.go` |
| **Model iteration result** | Fenced checkpoint advance; assistant message; its **whole ToolCall batch as `pending` rows**; callback deliveries earned; provider artifact; model checkpoint | `AdvanceTurnCheckpoint`: owner, attempt, unexpired lease, monotonic sequence | `queries/checkpoints.sql:84-101`; `queries/execution_commit.sql:62-213` (`CommitModelCheckpoint`) |
| **Inline tool start** | `tool_calls.status='running'`, `attempt_lease_attempt`, `attempt_started_at` | Go-side `turnClaimOwns` under the stream and Turn locks | `services/toolcalls.go:236-290`, `:394-468` |
| **Inline tool result** | Sequence and revision reserved; tool message; ToolCall settled; tool checkpoint; retained `tool_call_facts`; a lifecycle transition with the Turn's status unchanged | Separate `AdvanceTurnCheckpoint` statement | `queries/execution_commit.sql:215-345` (`CommitToolResult`) |
| **Park for host tools** | `status='waiting'`, lease released, active time accrued, `waiting` transition | Owner, attempt, lease | `queries/turn_execution.sql:735-771` |
| **Host/callback result batch** | One tool message for the batch; each call settled; one tool checkpoint per call; waiting-checkpoint advance; if nothing is left open, a `queued` transition plus dispatch | `AdvanceWaitingTurnCheckpoint` fences on the exact checkpoint sequence observed | `services/external_settlement.go:68-260`; `queries/checkpoints.sql:104-117` |
| **Settle** | Final bookkeeping, open calls closed with a synthetic result, terminal transition, `stop_reason`, usage, provenance | Owner and attempt, or the seam variant for unowned work | `services/execution.go:358+`; `queries/turn_execution.sql` `SettleTurn`, `SettleTurnAtSeam` |
| **Lease recovery** | `running -> queued`, accrued time, `queued` transition | `lease_expires_at <= now()` and the same `attempt` | `queries/turn_execution.sql:922-951` (`RecoverTurnLease`) |

This is the step log Dive v2 describes, split across `messages`, `turn_checkpoints`, `tool_calls`, `model_call_facts` and `turn_transitions`. Nvoken already honors **intent before effect** for both kinds of effect:

- **Tools.** The `pending` row commits with the model's message, before any dispatch. The `running` row with an attempt number commits before the executor is touched. A result then commits before the loop advances.
- **Model calls.** `model_call_facts` moves `prepared -> started` before the provider is called and ends `settled`, `not_started` or `uncertain`.

Only the model-call intent lives in a billing table, not in the Turn's own evidence.

The cost is write amplification. A model iteration with one inline MCP call takes about five transactions: prepare, start, checkpoint, tool start, tool result. `docs/proposals/2026-07-29-execution-round-trip-reduction.md` records which round trips were collapsed into single CTE statements. `CommitModelCheckpoint` and `CommitToolResult` are each one statement with several write arms, and `ReserveExecutionStreamSequences` bumps both counters in one row update.

### 3.3 Ordering and sequence numbers

The log's counters live on the `execution_streams` row. Every writer reserves numbers with an `UPDATE ... RETURNING` on that row, which takes its row lock until commit (`queries/execution_commit.sql:1-60`):

```sql
UPDATE execution_streams AS stream
SET next_message_sequence = stream.next_message_sequence + sqlc.arg(message_count)::int,
    next_lifecycle_revision = stream.next_lifecycle_revision + 1, ...
RETURNING (stream.next_message_sequence - sqlc.arg(message_count)::int)::bigint AS message_sequence,
          (stream.next_lifecycle_revision - 1)::bigint AS lifecycle_revision;
```

Several properties follow from this one decision:

- **Gap-free, commit-ordered counters without `SERIALIZABLE`.** Writers to one log are serialized by the row lock, so sequence order equals commit order. In any MVCC snapshot, a head of `N` means every row below `N` is visible. The stream's page read relies on exactly that (§4.4). The comment is explicit that "nothing downstream may start assuming gaps are legal."
- **A global lock order.** Execution stream first, then Turn, everywhere: claim (`FindNextQueuedTurnForUpdate ... FOR UPDATE OF stream SKIP LOCKED`, `turn_execution.sql:178-192`), checkpoints, tool writes, cancel, reaper and budget resume (`docs/design/architecture.md`, "Durable execution flow").
- **Cross-Turn ordering in a Conversation for free.** Turns in one Conversation share the log, so `revision` orders changes across Turns as well as within one.
- **A foreign append cannot slip into a checkpointed range.** "Sequence reservation is an UPDATE on the ExecutionStream row, so every message writer serializes behind the drain's own stream lock" (`services/generation.go:1072-1076`).

Parallel tool results are recorded **in completion order** (each `CommitToolResult` reserves the next sequence) and **projected in call order**. `normalizeToolResultMessages` re-sorts tool results by the assistant message's `tool_use` order when building the provider request (`services/generation.go:1180`, `:1264`).

### 3.4 Leases, claims and fencing

- **Claim.** `CommitTurnClaim` increments `attempt` on every claim, including a resume from `waiting`, and sets `lease_owner` and `lease_expires_at`. The attempt is the fencing token. It is carried on the `ExecutionClaim` (`services/execution.go:1620-1638`) and checked by every later write.
- **Renewal.** `RenewTurnLease` (`turn_execution.sql:367-380`) runs from a heartbeat goroutine in `engine.ClaimExecutor` (`internal/engine/claim.go:95+`). Losing the lease cancels the executor context. A resurrected executor "may finish local computation but cannot commit through the old fence" (`architecture.md`).
- **Monotonic checkpoint fence.** `AdvanceTurnCheckpoint` requires `current_checkpoint_sequence < new` and `current_iteration <= new`, plus owner, attempt and an unexpired lease (`checkpoints.sql:84-101`). `CommitModelCheckpoint` is `:exec`, so the fence must be a separate preceding statement, and the SQL comments explain why `CommitToolResult` can fold the fence into its main statement (`execution_commit.sql:215-240`).
- **Delivery is never ownership.** In the `cloud_tasks` topology a Cloud Task names work, "but the executor still exact-claims the Turn in Postgres" (`architecture.md`, "Cloud Tasks"). Dispatch intent is an outbox row written in the admission or queue transaction.
- **Cancellation hint.** In the `cloud_tasks` topology, `LISTEN/NOTIFY` is used "only as a coalescable latency hint" to reach an in-flight executor (`internal/adapters/worksignal/postgres.go:14-26`). The durable cancel is the Turn row.
- **Deadlines.** `total`, `active_execution`, `execution_segment` and `waiting` deadlines are all durable columns. Waiting time does not count against active time, and credential and MCP binding expiries shift forward by the time spent waiting (`QueueWaitingTurn`, `turn_execution.sql:773-823`).

### 3.5 Crash recovery: how a Turn resumes after a process dies

1. **Detect.** The reaper lists expired leases, and `RecoverTurnLease` moves the Turn from `running` to `queued` with a new transition. Active time is accrued only up to the lease expiry.
2. **Reclaim.** Any worker claims it with `attempt + 1`.
3. **Rebuild from the durable prefix.** `GenerationExecutor.Execute` (`services/generation.go:205+`) re-reads the log (`ListExecutionStreamMessagesForGeneration`) and provider artifacts, then calls `loadGenerationRecovery` (`services/generation_recovery.go:31+`). That function validates every checkpoint, tool call and message against each other and fails the Turn as `internal` / `recovery_invalid` rather than executing on an inconsistent prefix. It returns:

   ```go
   // internal/services/generation_recovery.go:23-29
   type generationRecovery struct {
       Resume               *domain.GenerationResume // iteration, usage so far, open tool calls, structured output
       Latest               *domain.TurnCheckpoint
       Final                bool // last checkpoint was a final answer: settle without calling the model
       ExternalToolsPending bool // re-park, don't generate
       Provenance           domain.ModelProvenance
   }
   ```
4. **Decide at the seam, before any provider call** (`generation.go:434-512`):
   - An interrupt was requested and there is a durable prefix: stop now, with no extra model call.
   - External tools are still pending: re-park.
   - The last checkpoint was final: settle from it.
   - A budget is already exceeded: stop or hold.
   - Otherwise, re-enter Dive with the rebuilt transcript. `divegen` seeds its iteration counter and usage from `Resume` (`generator.go:742-743`).
5. **Reconcile tool calls that were in flight.** A call left `running` under an older attempt is handled by mode:
   - **Builtin tools** are restarted under the new attempt (`RestartToolCallAttempt`) and replayed before the model is called (`replayOpenToolCalls`, `generator.go:2034-2082`).
   - **MCP tools** are restarted only if the server's annotations say it is safe:
     ```go
     // internal/adapters/divegen/mcp_tool.go:239-244
     func safeMCPRetry(annotations domain.MCPToolAnnotations) bool {
         destructive := positiveMCPAnnotation(annotations.DestructiveHint)
         return !destructive &&
             (positiveMCPAnnotation(annotations.ReadOnlyHint) ||
                 positiveMCPAnnotation(annotations.IdempotentHint))
     }
     ```
     Otherwise the call is settled `failed` with a system-origin result telling the model: *"The remote tool may have completed before execution was interrupted. Its outcome is unknown, so nvoken did not retry it."* (`services/toolcalls.go:432-448`, `settleMCPUnknownOutcome` at `:545-597`).
   - **Host and callback tools** never run in-process, so there is nothing to reconcile.
6. **A model call that was in flight** is simply issued again. Its `model_call_facts` row is marked `uncertain` for billing (`services/credit_authority.go:132`, `:254-262`), and the usage report counts `uncertain_model_calls` separately (`services/usage.go:77-78`). Any previews from the dead attempt are voided on the wire by the higher `attempt` (§5.3).

Compare Dive v2's "Crash while a local tool runs" flow. There the engine commits `turn_stopped{Uncertain}` and the application reconciles through `Accept`. Nvoken instead resolves uncertainty automatically, using the MCP annotations as policy and telling the model when it cannot. Both are defensible. Section 9 argues that Dive should offer both as a typed policy.

### 3.6 Idempotency

| Operation | Key | Duplicate with same content | Duplicate with different content |
|---|---|---|---|
| Turn admission | `(tenant_id, idempotency_key)` plus request fingerprint, plus a materialization digest for fetched URL media | Returns the original Turn, without re-fetching media | Conflict |
| Model checkpoint (a worker re-recording an iteration) | `(turn_id, iteration)` plus the usage/provenance evidence digest | `replayModelCheckpoint` returns the stored result (`toolcalls.go:779+`) | `ErrToolCallConflict` |
| Inline tool result | ToolCall ID; terminal status | Equal stored result is acknowledged (`toolcalls.go:504-511`) | `ErrToolCallConflict` |
| Host tool results (`POST /v1/turns/{id}/tool-results`) | ToolCall ID | "An equal replay is acknowledged as deduplicated" (`deduplicated: true`) | "A changed replay conflicts" (409). First committed result wins |
| Nudge | `(turn_id, idempotency_key)` | Returns the existing nudge | Conflict |
| Callback delivery to the host | ToolCall ID and a signed envelope | The host must dedupe | — |

Nvoken does not promise exactly-once side effects. "Hosts must make their own side effects idempotent using stable Turn and ToolCall identities" (`architecture.md`, "Deliberate non-goals").

### 3.7 Invariants enforced in the database

Nvoken leans on Postgres, not Go validators, for its invariants. Triggers reject updates to `turn_transitions`, terminal Turns, terminal tool calls and attempts, admission evidence, and agent revisions (`schema.sql:366-505`, `:2198-2248`). CHECK constraints pin row shapes: a `running` Turn must hold a lease; terminal status ⇔ `ended_at`; a terminal tool call ⇔ it has a result message; a model checkpoint ⇔ it has usage, provenance and an evidence digest. Composite foreign keys enforce tenant containment. This is how the "facts, ownership, evidence" rules from §1 are enforced rather than merely documented.

### 3.8 Relation to Dive's session and turn store

Nvoken uses Dive only as a stateless loop inside one **segment** (one claim):

```go
// internal/adapters/divegen/generator.go:1041-1057 (comments trimmed)
agent, err := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: systemPrompt, Model: agentModel, Tools: tools,
    ModelSettings: settings, ToolIterationLimit: request.MaxIterations,
    Hooks: hooks, Tracer: tracer,
    // Postgres keeps the incomplete Turn: its checkpoints and ToolCall rows are the record...
    IncompleteTurns: dive.IncompleteTurnOptions{Discard: true},
})
options := []dive.CreateResponseOption{dive.WithMessages(messages...)}
```

- **The journal is the event callback.** On every `ResponseItemTypeMessage` that carries `Usage`, the callback converts the message, extracts tool requests, and calls `RecordModelCheckpoint` *synchronously inside the callback* before Dive may dispatch tools (`generator.go:1066-1158`). This is ADK's commit-before-proceed handshake, rebuilt from outside the library. It holds only because Dive happens to call the callback before running tools.
- **Tool effects are recorded by wrapping the tools.** Builtin and MCP tools call `Start*ToolCall` and `Accept*ToolResult` themselves. Host tools return `dive.NewSuspendResult(...)` (`generator.go:1774-1778`), and a suspended response becomes `ExternalToolsPending` (`:1253-1275`).
- **Stops from outside the loop are sentinel errors** returned from the callback: `errCheckpointInterrupt` and `errCheckpointBudget` (`generator.go:1136-1150`, mapped back at `:1223-1225`).
- **Nudges enter through hooks.** `PreIteration` appends drained nudge messages to `hctx.Messages` (`:956-990`), and a `Stop` hook returns `StopDecision{Continue: true, Reason: text}` when input arrived during the final response (`:996-1039`).
- **Dive's `session`, `TurnStore` and claims are unused.** `IncompleteTurns.Discard` exists specifically so that Dive does not invoke the callback a second time with placeholder results.

`docs/reviews/2026-09-25-dive-simplification-review.md` quantifies the cost. About 270 lines rebuild the step record from callbacks "and the data it needed wasn't all there". It also records the missing per-model-call record (`DIVE-04`), the lack of a typed external stop (`DIVE-05`), tools being unable to fail the Turn (`DIVE-06`), and 429 lines of HTTP-body sniffing to classify provider errors (`DIVE-12`). The Dive v2 conceptual model's acceptance criterion is exactly this integration: "Nvoken implements `Recorder` over its transaction, calls `Engine.Apply` directly, and deletes its callback checkpointing, evidence wrappers, and error-string parsing."

Mapping Nvoken's record onto Dive v2 step kinds:

| Dive v2 step | Nvoken equivalent today |
|---|---|
| `turn_started` (definition ref, settings, tool defs) | Admission transaction: `turn_effective_behaviors` with digest, `turn_admissions`, input messages, `queued` transition |
| `context_delivered` | Drained nudge messages plus a `nudge` checkpoint; runtime-visibility memory reminders |
| `model_requested` | `model_call_facts` `prepared -> started` (a billing table, not Turn evidence) |
| `model_completed` | `CommitModelCheckpoint`: assistant message, model checkpoint, **plus the pending ToolCall rows** |
| `model_failed` | `model_call_facts` `settled` with a failed outcome, or `uncertain`; there is no Turn-evidence row |
| `tool_started` | `tool_calls.status='running'` with `attempt_lease_attempt` |
| `tool_completed` | `CommitToolResult`, or `settleExternalToolResults` for host tools |
| `tool_waiting` | Pending host or callback call plus the `waiting` transition |
| `tool_uncertain` | Not recorded as such. The MCP call is settled `failed` with a system result, and a builtin is restarted |
| `turn_stopped` | Terminal transition plus `stop_reason`, `error` and `structured_output` on `turns` |

---

## 4. Stream API

The normative sources are the OpenAPI contract for shapes (`openapi/nvoken.yaml`, `StreamEvent` and friends) and `docs/design/streaming-protocol-specification.md` for everything the contract cannot express. The spec numbers its rules `R1` to `R55` for conformance tests. It was revised on 2026-09-01 to close `docs/reviews/2026-09-01-streaming-protocol-review.md` (`STR-01` to `STR-19`). The public SDK repository has four reducer implementations and a shared fixture, `sdk/conformance/fixtures/reducer.json` (spec Appendix B).

### 4.1 Routes and transport

```
GET /v1/turns/{turn_id}/stream                   Turn scope: one Turn's rows; closes after the terminal change
GET /v1/conversations/{conversation_id}/stream   Conversation scope: every Turn, current and future; never closes on settlement
```

- **Transport: Server-Sent Events only.** There is no WebSocket and no long-poll. "The protocol's semantics live in the frames. SSE contributes exactly three mechanics: the `id:` line, the `retry:` opener, and comment keepalives" (spec §3). The spec is written so that another framed transport could carry the same frames.
- **Inputs** (`internal/adapters/httpapi/target_streams.go:100-128`): `cursor` query, else `Last-Event-ID`, and `deltas=true|false` (default `true`). Unknown or repeated parameters, a blank cursor, or a repeated or blank `Last-Event-ID` are 400 (R3, R4).
- **Auth.** A bearer machine API key or a browser client token. `EventSource` cannot attach headers "and MUST NOT be used"; the spec requires a `fetch`-based reader (R55).
- **Every error happens before the 200** (R7, R8). Authorization, scope, cursor decoding and the first durable page read all run before `WriteHeader(200)` (`target_streams.go:239-252`). "There is no error frame." A failure after the stream opens is logged, the connection is closed, and the client reconciles through JSON reads.

### 4.2 Connection lifecycle

```go
// internal/adapters/httpapi/target_streams.go:231-265 (condensed)
connection.subscription = h.liveEvents.Subscribe(r.Context(), target.ExecutionStreamID) // 1. subscribe first
first, err := read(services.TargetStreamInput{Cursor: options.Cursor, Limit: services.MaxListPageSize}) // 2. first page
if err != nil { h.writeError(w, requestID, err); return }                                  //    still a JSON error
w.WriteHeader(http.StatusOK)                                                                // 3. commit
connection.drain(&first)                                                                    // 4. replay to head
if connection.settled || target.Terminal { connection.closeSettled(); return }              // 5. already over
writeSSEControl(w, h.stream.WriteTimeout, "retry: 1000\n\n")                                // 6. opener, only if staying open
```

The live loop (`:280-343`) selects over client disconnect, process shutdown (`rotate`), browser-visitor eviction (`retry: 10000` then `rotate`), jittered lifetime (`rotate`, or `idle` for a Conversation stream that wrote nothing), a keepalive tick (`: keepalive`), a poll tick (drain), and live events (`pump`). Defaults (`httpapi/server.go:42-50`, spec §6):

| Setting | Default |
|---|---|
| Durable poll (the correctness path) | 2 s |
| Keepalive comment | 15 s |
| Max connection lifetime | 5 min, jittered ±10% |
| Per-frame write timeout | 10 s, then a `slow_consumer` farewell |
| Page budget | 200 rows or 1 MiB of message content |
| Open streams per browser visitor per instance | 8, oldest evicted. Machine credentials are exempt |

The lifetime is short on purpose. Behind the Google Cloud load balancer, a browser that navigates away cancels its request, but the server-side connection stays open and keeps accepting keepalives. "The lifetime bounds how long a departed client holds a request slot" (spec §6).

### 4.3 How the stream is fed: a poll with a wake

There are three inputs, in decreasing order of authority:

1. **The durable poll.** Every 2 s the connection drains the log from its last delivered position. "Polling the log alone at the reconciliation interval discovers every committed message, every change, and every settlement ... without any live delivery at all" (R38).
2. **Commit wakes.** After any transaction that appends a message or records a change, the store publishes a payload-free `transcript.commit` event for that log (`internal/adapters/postgres/stream_commits.go:22-33`, registered with `afterCommit`). A connection that receives one drains immediately. Wakes are never written to clients. A dropped wake is not a gap: "the poll tick reconciles the log" (`liveevents/inprocess.go:49-53`).
3. **Preview events.** The executor's `GenerationDeltaEmitter` publishes `message.delta` payloads onto the bus (`services/generation.go:873-932`). They are never persisted.

The bus has two implementations. `liveevents.InProcess` is used when the embedded engine and the stream share a process. `liveevents.Redis` is used for the two-service Cloud Run topology (Memorystore). Both are **bounded and non-blocking**:

```go
// internal/adapters/liveevents/inprocess.go:40-55
func (s *subscription) deliver(event ports.LiveEvent) {
    ...
    select {
    case s.channel <- event:
    default:
        // A dropped wake loses nothing a client could see: the poll tick
        // reconciles the log. Only a dropped preview is a gap worth a resync.
        if event.Type != domain.LiveEventTranscriptCommit {
            s.gapped.Store(true)
        }
    }
}
```

Redis publishing goes through a bounded queue drained by one goroutine. A full queue or a Redis publish error records a pending `stream.resync` for that log and marks local subscribers gapped (`liveevents/redis.go:142-152`, `:216-243`). "Publication MUST never block or fail execution" (R39). Nvoken does not use Postgres `LISTEN/NOTIFY` for streams. It uses NOTIFY only for the cancellation hint.

### 4.4 Replay, cursors and the page algorithm

A **position** is `(message_sequence, lifecycle_revision)`. A **cursor** encodes a position plus the scope it was issued for:

```go
// internal/services/target_streams.go:69-80
type targetStreamPosition struct {
    MessageSequence   int64 `json:"message_sequence"`
    LifecycleRevision int64 `json:"lifecycle_revision"`
}
type targetStreamCursor struct {
    Version        int                  `json:"version"`         // 1
    Kind           string               `json:"kind"`            // "transcript"
    ConversationID string               `json:"conversation_id,omitempty"`
    TurnID         string               `json:"turn_id,omitempty"` // set when issued by a Turn connection
    Position       targetStreamPosition `json:"position"`
}
```

The cursor is unpadded base64url JSON, opaque to clients (R35, Appendix A). It never contains the private log ID. Scope rules (R36): a cursor from a Turn connection on a Conversation-bound Turn is accepted by the Conversation route, because it is a position in the same log. The reverse is rejected, and so are a standalone-Turn cursor on any Conversation route and a cursor "ahead of the committed transcript".

A page is read inside **one read-only snapshot** (`txm.WithReadSnapshot`, `services/target_streams.go:197`):

```text
head  := (next_message_sequence - 1, next_lifecycle_revision - 1)     // same snapshot as the rows
pos   := cursor, or the Turn's origin on the Turn route, or (0,0)
messages in (pos.m, head.m], ≤ 200 rows, then cut to 1 MiB (never cut the first row)
only if messages reached head.m: changes in (pos.r, head.r] with the remaining row budget
has_more := pos != head
```

"**Messages before changes at every cut**" is the key rule (spec §6.2). A change is read only after the message watermark has reached the head seen in the same snapshot, so a client can never see a Turn settled before its final message exists (R17). The cursor names rows already delivered, so a reconnect "repeats nothing and skips nothing." On the Turn route, rows from other Turns are filtered out but still advance the position. A cursorless Turn stream starts at that Turn's first row rather than scanning its Conversation (`targetStreamTurnStart`, `:324-355`). This fixed `STR-04`.

**Bootstrap** (R37, R54): `GET /v1/conversations/{id}/transcript` returns a `TranscriptSnapshot` with a `cursor` at the snapshot's position, so a client renders history from the JSON read and then opens the stream from that cursor. A bounded snapshot read with `limit` or `page_token` reports the cursor of the cut its walk started from, so paging older history never moves the resume position.

### 4.5 Fan-out, backpressure and slow consumers

- **Any number of connections per stream, and none affects execution** (R45). Each connection is an independent reader of Postgres with its own subscription. There is no shared per-Turn buffer to overflow.
- **Burst coalescing.** `pump` handles the arriving event and everything already buffered behind it, then drains *once* (`target_streams.go:399-455`): "Coalescing the drain is what keeps a burst of commits from costing a read per commit."
- **Gaps become one resync followed by a drain.** When `TakeGap()` fires, the connection writes `stream.resync`, drops buffered previews, and drains (R32, R39). The connection stays open.
- **Backpressure at the socket.** Every frame write sets `http.ResponseController.SetWriteDeadline(now + 10s)`. A deadline error sends a best-effort `connection.closing{slow_consumer}` that starts with a blank line "because the frame that timed out may have reached the socket in part" (`:565-575`).
- **Execution never waits for a reader.** Previews can be dropped at three layers (publisher queue, subscriber buffer, socket), and each drop surfaces as a resync, never as a stall.

### 4.6 Control from the client: cancellation and steering

The stream is read-only. "No connection end cancels, pauses, or otherwise affects a Turn" (R45). Control is ordinary JSON commands (`internal/domain/api_surface.go:121-136`):

| Command | Semantics |
|---|---|
| `POST /v1/turns/{id}/cancel` | Ends `cancelled` and discards the work: generation eligibility excludes the Turn's messages. Safe to repeat. |
| `POST /v1/turns/{id}/interrupt` | Ends `completed` with `stop_reason: interrupted` **at the next clean seam** and keeps the work. Between steps it stops before returning. Mid-step it returns `running` and stops "at worst one model call later." Open host calls are closed, so a late result returns 409. |
| `POST /v1/turns/{id}/nudges` (+ list, cancel) | Queue steering input with an idempotency key. It is drained at three seams: segment start, before each later iteration (`PreIteration` hook), and at stop (a `Stop` hook that continues the Turn) (`divegen/generator.go:936-1039`). Undrained nudges expire visibly at settlement. |
| `POST /v1/turns/{id}/tool-results` | Submit up to 32 host or callback results atomically. First write wins, equal replays dedupe, and closing the last open call queues the Turn. |
| `POST /v1/turns/{id}/resume` | Raise the limit that put a Turn on `budget_hold`, and continue. |

Browser client tokens may interrupt, nudge and submit host results for tools named in the AgentRevision's client interface (R24). Cancel and resume need app credentials.

---

## 5. Streaming protocol and message types

### 5.1 The frame union

Four frame types, discriminated on `type`, with the SSE `event:` name equal to `type` (R10):

| `type` | Class | SSE `id` | Purpose |
|---|---|---|---|
| `transcript.update` | durable | the cursor | Saved messages and Turn changes |
| `message.delta` | ephemeral | none | Live preview fragment of a message being written |
| `stream.resync` | ephemeral | none | Previews were lost; discard them |
| `connection.closing` | ephemeral | none | This connection is ending, and why |

**R1.** Only `transcript.update` carries an `id`, and cursors are issued nowhere else. **R2.** A client must treat `transcript.update` as the only source of transcript state and must not use preview text to decide how a Turn ended.

### 5.2 `transcript.update`: the durable frame

```go
// internal/adapters/httpapi/target_streams.go:40-70
type transcriptUpdateEventResponse struct {
    Type        string                        `json:"type"`         // "transcript.update"
    Messages    []conversationMessageResponse `json:"messages"`     // ascending sequence; may be empty
    TurnChanges []turnChangeResponse          `json:"turn_changes"` // ascending revision; may be empty
    HasMore     bool                          `json:"has_more"`     // page was cut by budget; more follows immediately
    Cursor      string                        `json:"cursor"`       // equals the SSE id
}

type turnChangeResponse struct {
    // Log fields: always present, describe the step itself.
    TurnID                 string            `json:"turn_id"`
    ConversationID         *string           `json:"conversation_id"`
    ContentExpiresAt       *time.Time        `json:"content_expires_at"`
    Revision               int64             `json:"revision"`
    Status                 domain.TurnStatus `json:"status"`
    Terminal               bool              `json:"terminal"` // status ∈ {completed, incomplete, failed, cancelled}
    Current                bool              `json:"current"`  // still the Turn's latest revision as of this read
    ThroughMessageSequence *int64            `json:"through_message_sequence"`
    Error                  json.RawMessage   `json:"error"`             // non-null only on the current terminal change
    StructuredOutput       json.RawMessage   `json:"structured_output"` // same
    OccurredAt             time.Time         `json:"occurred_at"`
    // Detail fields: describe the Turn *now*; only on the change with Current set.
    StopReason                 *domain.TurnStopReason     `json:"stop_reason,omitempty"`
    CreditBlock                *domain.CreditBlock        `json:"credit_block,omitempty"`
    ToolCalls                  *[]toolCallSummaryResponse `json:"tool_calls,omitempty"` // [] means "none"
    Usage                      json.RawMessage            `json:"usage,omitempty"`
    Provenance                 json.RawMessage            `json:"provenance,omitempty"`
    StructuredOutputProvenance json.RawMessage            `json:"structured_output_provenance,omitempty"`
    FinalAnswerMessageID       *string                    `json:"final_answer_message_id,omitempty"`
}
```

A frame is never empty (R15). Within a frame, clients apply messages before changes (R17). The **log/detail split** is the protocol's most distinctive idea. A replayed older change carries only what was true at that revision, and "current Turn detail" attaches only to the change that is still current *at read time* (R46). Replaying a `running` change therefore can't leak what happened later, and the latest change alone answers "why isn't this Turn moving?" without a second request. `current` lets a client tell an omitted detail field from an empty one. This fixed `STR-07`.

`ToolCallSummary` (OpenAPI `ToolCallSummary`, spec §4.1):

| Field | Presence | Meaning |
|---|---|---|
| `id` | always | ToolCall ID, equal to the saved `tool_use` block's `id` |
| `name` | always | |
| `mode` | always | `builtin`, `host`, `callback` or `mcp` |
| `status` | always | `pending`, `running`, `completed`, `failed` or `cancelled`. For a host call, `running` means "waiting on you" |
| `arguments` | only while settleable | The model's input object, present on a current `waiting` change for a non-terminal `host`/`callback` call. Browser tokens see only client-interface host tools |
| `deadline_at` | same rule | The Turn's waiting deadline, or `null` when unbounded (the default) |
| `updated_at` | always | |

"A call you must run yourself is one carrying `arguments` with `mode: host`" (R24). That rule is the protocol's whole representation of a pending action.

`ConversationMessage` carries `id`, `conversation_id`, `content_expires_at`, optional `agent_id`, `turn_id`, optional `user_key`, `sequence`, `role` (`system | user | assistant | tool`), `content`, optional `copied_from_message_id`, optional `phase` (`commentary | final_answer`), and `created_at`. Content blocks are Nvoken's own shapes, not a provider's: `text` (with citations), `image` and `document` (descriptors with media type, size and `sha256:` digest, never bytes), `tool_use` (id = ToolCall ID), `server_tool_use` (the provider's ID, for provider-run tools such as web search; never settleable), `tool_result` (`tool_use_id`, the host's JSON verbatim, `is_error`), `reminder` (`app-*` name, `contextual | operator` tier), and `redacted` (an assistant message whose every block is provider-private) (OpenAPI `ConversationContentBlock`; R19).

`phase` is computed at read time. On the stream a message is always `commentary` when it is written, and nothing corrects it later. The terminal change instead names the answer in `final_answer_message_id` (R20). This is a small, clear instance of projection over record: the JSON reads compute `phase: final_answer` for the same message by the same rule.

### 5.3 `message.delta`: preview identity

```go
// internal/domain/streaming.go:65-78 (the wire mirror is httpapi/target_streams.go:72-84)
type GenerationDeltaEvent struct {
    Type           string    `json:"type"`           // "message.delta"
    ConversationID *string   `json:"conversation_id"` // scope check only; not written to the wire
    TurnID         string    `json:"turn_id"`
    Attempt        int64     `json:"attempt"`        // execution attempt (lease fencing token)
    MessageID      string    `json:"message_id"`     // ID the saved assistant message WILL carry
    ContentIndex   int       `json:"content_index"`  // provider block index while writing
    Offset         int64     `json:"offset"`         // UTF-8 bytes of earlier fragments of this block
    Kind           string    `json:"kind"`           // text | thinking | tool_arguments
    Delta          string    `json:"delta"`          // non-empty
    ToolCallID     string    `json:"tool_call_id,omitempty"` // tool_arguments only
    Name           string    `json:"name,omitempty"`         // tool_arguments only
    EmittedAt      time.Time `json:"emitted_at"`
}
```

The design choices, and the review findings behind them:

- **Preview identity is the future record's identity.** `divegen` reserves a message ID at the first delta of an iteration (`generationCheckpointState.reserveMessageID`, `generator.go:1485-1500`), and `RecordModelCheckpoint` *uses that ID* for the saved message (`services/toolcalls.go:99-109`). The handoff from preview to saved message is therefore an update to a row the client already has, not one row disappearing and another appearing (OpenAPI `PreviewMessageID`).
- **`attempt` scopes previews to an execution attempt.** It is the lease fencing token, so a recovered execution automatically voids the dead attempt's previews: "a higher `attempt` discards every preview of that Turn" (R49.1). A retried attempt allocates a new `message_id`, which is harmless.
- **`offset` lets the client detect loss itself.** A client whose accumulated length disagrees with `offset` discards the block and waits for the saved message (R28). The review argued the client should not have to depend on the server noticing (`STR-05`).
- **Tool identity is denormalized onto every `tool_arguments` fragment** "so that a lost frame cannot orphan a later fragment" (R27). The provider names the call only once, at `content_block_start`, so `divegen` keeps a per-iteration `index -> (id, name)` map (`generator.go:1513-1541`, `:2703-2741`).
- **Thinking previews are display-only.** "No content block stores reasoning, no read returns it", and a preview carries no signature (R29). A Turn run with explicit `reasoning` controls emits no `thinking` previews at all (`services/generation.go:885-888`). Provider reasoning signatures live in the private `provider_message_artifacts` for replay to the same provider.
- **The server validates every preview** before forwarding it and drops invalid ones silently (R30, `validTargetGenerationDeltaEvent`, `target_streams.go:663-682`).
- **Concatenation is scoped** to one `(turn_id, attempt, message_id, content_index)`. Tool arguments must be compared structurally, because the saved `input` is re-serialized (R28).

### 5.4 `stream.resync` and `connection.closing`

```go
// internal/adapters/httpapi/target_streams.go:86-95
type streamResyncResponse struct {
    Type   string  `json:"type"`              // "stream.resync"
    TurnID *string `json:"turn_id,omitempty"` // absent = every preview in scope is void
    Reason string  `json:"reason"`            // "live_delivery_gap"
}
type connectionClosingResponse struct {
    Type   string `json:"type"`   // "connection.closing"
    Reason string `json:"reason"` // settled | rotate | idle | slow_consumer
}
```

- A resync with `turn_id` voids that Turn's previews. Without it, all previews in scope are void; "absence is the scope signal; the field is never null" (R31). The client discards and waits, and does not reconnect. The server drains immediately after the resync (R32).
- `connection.closing` "speaks about the connection" and carries no cursor, "because a client must already hold its last durable cursor to survive a silent drop" (R33). `settled` is the one reason that also speaks about the stream. It was added (`STR-01`) because a generic SSE client honoring `retry: 1000` used to reconnect to a settled Turn once a second forever. The actions: `settled` = do not reconnect; `rotate` = reconnect now; `idle` = reconnect lazily; `slow_consumer` = reconnect after widening buffers.

### 5.5 Lifecycle ordering guarantees

The spec lists five consumption guarantees (§8). Items 1 to 4 below restate its first four. Its fifth, that thinking previews are display-only, is covered in §5.3. Items 5 to 7 are structural properties that follow from the other rules.

1. Durable frames carry the cursor. A preview never advances the position (R1, R50).
2. Delta concatenation is scoped to `(turn_id, attempt, message_id, content_index)` (R28, R49).
3. `stream.resync` invalidates every affected preview (R31, R32).
4. Only a `terminal: true` change and the JSON Turn reads say how a Turn ended. No connection end cancels anything (R22, R45, R53).
5. Messages are folded before changes, across frames as well as within one. A Turn is never seen settled before its last message (R17, §6.2).
6. A message is delivered at most once per position. Clients still fold by identity, `sequence` for messages and `(turn_id, revision)` for changes, because reconnecting from an older cursor replays (R18, R48).
7. `revision` is shared by every Turn in a Conversation, so it orders changes across Turns (R23).

### 5.6 Delta versus snapshot semantics

The protocol has three granularities, each with a different contract:

- **Messages** are *append-only snapshots*. Each saved message is complete and immutable. There is no message-level delta on the durable path.
- **Changes** are an *append-only log with a read-time projection*. Log fields describe the step, and detail fields are a snapshot of "now" attached to the current change only.
- **Previews** are *deltas*. They are append-only fragments with an explicit offset, voided by attempt, resync, saved message or terminal change (R49).

The client fold (spec §7) holds five pieces of state: messages by `sequence`, changes by `(turn_id, revision)`, previews by `(message_id, content_index)` with the highest `attempt` per Turn, settled Turns, and the cursor. Building a rendered transcript needs three more folds that the stream deliberately does not perform (§7.3). A saved message merges into its preview row. A `tool_result` "reaches backwards" to mutate the earlier `tool_use` row. Compactions are never streamed and must be read separately.

### 5.7 How each concern is represented

| Concern | While it happens (ephemeral) | Durably |
|---|---|---|
| Assistant text | `message.delta{kind: text}` | `assistant` message, `text` blocks |
| Reasoning | `message.delta{kind: thinking}` (display-only) | Never public. A private provider artifact; `redacted` if the whole message is private |
| Tool call requested | `message.delta{kind: tool_arguments, tool_call_id, name}` | `tool_use` block in the assistant message. A `ToolCallSummary` appears on the *next* current change |
| Tool running (inline MCP/builtin) | Nothing. The start writes no transition and no wake | Only via `tool_calls[].status` on whatever change is next read |
| Tool result | — | `tool` message with `tool_result` blocks. An inline result also records a change with the Turn's status unchanged (`CommitToolResult`) |
| Approval, question, or client-side action | — | A **host tool**. The Turn parks `waiting`, and the current change's `tool_calls` carry `arguments` and `deadline_at` for `mode: host`. There is no approval type (see below) |
| Budget pause | — | `status: budget_hold` plus `stop_reason` plus `credit_block` |
| Steering accepted | — | A drained nudge lands as a saved message covered by a `nudge` checkpoint |
| Tool error | — | `tool_result.is_error: true`, and `ToolCallSummary.status: failed` |
| Turn failure | — | Current terminal change: `status: failed`, `error: TurnFailure{code, message, details}` |
| Usage and cost | — | `usage` (disjoint input buckets, `reasoning_tokens` as a subset, `model_calls`) and `provenance` on the current *terminal* change only, machine credentials only. Per-call detail is available from `GET /v1/turns/{id}/timeline` |
| Completion | — | `terminal: true`, `stop_reason`, `final_answer_message_id`, `structured_output` |
| Server-run tools (web search) | — | `server_tool_use` block with the provider's ID |
| Compaction | — | Never streamed. Read from `GET /v1/conversations/{id}/compactions` |

**Approvals.** Nvoken deliberately has no approval primitive. `docs/guides/asking-the-user.md` explains: "A structured question to the user is a host tool. The park/webhook/resume machinery already *is* 'block until someone answers'." `docs/proposals/2026-08-08-tool-calling-records-and-contract.md` §3 states that "the allowlist is the approval" for MCP tools, and that a future per-tool approval flag "can reuse the waiting machinery, so an approval resolves exactly like a host tool result. No new lifecycle is needed." `docs/prds/052-prd-interaction-records.md` (Status: **Proposed**) would add durable `request_approval` and question records with quorum rules, ported from Mobius Cloud. It is not implemented.

A host-tool approval still has MAF-Go's *binding* property. A result is accepted only for a ToolCall ID the Turn recorded, only while it is pending, and only with the Turn's tenant scope. The arguments the approver sees come from the recorded `tool_use`, not from the client.

**Failures** use a closed code set (OpenAPI `TurnFailure`): `deadline_exceeded` (with `details.scope`), `budget_exceeded`, `cost_estimate_unavailable`, `provider_key_unavailable`, `context_window_exceeded`, `input_media_rejected`, `provider_error` (with `details.provider_failure_class`: `upstream_rejected`, `upstream_unavailable`, `throttled`, `configuration`, `canceled`, `timeout_or_transport`, `invalid_response` or `unknown`), `mcp_discovery_failed`, `structured_output_unsatisfied`, and `internal`. "Provider error text is never passed through here."

**Status semantics are written for clients.** `incomplete` means a limit stopped the Turn *cleanly*, and the reply carries forward. `failed` means it could not stop cleanly. An interrupt is `completed`, "because you asked the turn to end there." A limit shows up as `failed{budget_exceeded}` only when the Turn "could not stop cleanly" (OpenAPI `TurnStatus`, `TurnStopReason`).

### 5.8 IDs and correlation

| ID | Minted | Correlates |
|---|---|---|
| `turn_id` | Admission | Every row, preview and resync |
| `message.id` | Reserved at the first preview of an iteration; minted at checkpoint otherwise | `message.delta.message_id` equals the saved `ConversationMessage.id` |
| `sequence` / `revision` | Log counters under the stream row lock | Cursor coordinates, fold keys, and `through_message_sequence` (which messages precede a change) |
| ToolCall ID | `prepareToolCalls` at model checkpoint | `tool_use.id` = `ToolCallSummary.id` = `tool_result.tool_use_id` = `submitHostToolResults.tool_call_id` |
| Provider call ID | Provider | `message.delta.tool_call_id` (see discrepancy 1 below) |
| `attempt` | Every claim | Voids stale previews |

### 5.9 Versioning and forward compatibility

There is no protocol version on the wire. The cursor has `version: 1` internally. Compatibility is by rule (R14): clients must ignore unknown frame types and fields; must not render an unknown delta `kind`; must treat an unknown resync reason as `live_delivery_gap` and an unknown closing reason as `rotate`; and "MUST NOT infer that a Turn ended from a status it does not know". `terminal` exists so that no client keeps its own terminal set (R21). Every enum in the OpenAPI carries "Expect new values here over time." `api_surface.go` pins the operation table, and `make openapi-check` rejects drift between the split sources and the joined contract.

### 5.10 Worked example: a tool call, then an approval pause

The scenario is a Conversation-bound Turn. The AgentRevision declares `lookup_order` (an MCP tool annotated read-only) and `request_approval` (a host tool, the asking-the-user pattern). The client opens `GET /v1/turns/…t1/stream` without a cursor, right after admission. IDs and cursors are abbreviated, JSON is wrapped for reading, and every `data:` line is compact on the wire. The frame boundaries are one plausible cut: rows committed in separate transactions may arrive in separate frames or be coalesced by one drain.

```
id: c1
event: transcript.update
data: {"type":"transcript.update",
  "messages":[{"id":"…m1","conversation_id":"…c0","content_expires_at":null,"turn_id":"…t1",
    "sequence":7,"role":"user","content":[{"type":"text","text":"Refund order A-17"}],"created_at":"…"}],
  "turn_changes":[
    {"turn_id":"…t1","revision":12,"status":"queued","terminal":false,"current":false,
     "through_message_sequence":7,"error":null,"structured_output":null,"occurred_at":"…", …},
    {"turn_id":"…t1","revision":13,"status":"running","terminal":false,"current":true,
     "through_message_sequence":7,"error":null,"structured_output":null,"occurred_at":"…",
     "tool_calls":[]}],
  "has_more":false,"cursor":"c1"}

retry: 1000

event: message.delta          ← iteration 1 streams (attempt 1, message …m2 reserved)
data: {"type":"message.delta","turn_id":"…t1","attempt":1,"message_id":"…m2","content_index":0,
  "offset":0,"kind":"text","delta":"Let me look that up.","emitted_at":"…"}

event: message.delta
data: {"type":"message.delta","turn_id":"…t1","attempt":1,"message_id":"…m2","content_index":1,
  "offset":0,"kind":"tool_arguments","delta":"{\"order_id\":\"A-1",
  "tool_call_id":"toolu_01AB…","name":"lookup_order","emitted_at":"…"}

event: message.delta
data: {…"message_id":"…m2","content_index":1,"offset":16,"kind":"tool_arguments","delta":"7\"}",
  "tool_call_id":"toolu_01AB…","name":"lookup_order",…}

id: c2                         ← CommitModelCheckpoint: message + pending ToolCall; no change row
event: transcript.update
data: {"type":"transcript.update",
  "messages":[{"id":"…m2","turn_id":"…t1","sequence":8,"role":"assistant","phase":"commentary",
    "content":[{"type":"text","text":"Let me look that up."},
      {"type":"tool_use","id":"…tc1","name":"lookup_order","input":{"order_id":"A-17"}}], …}],
  "turn_changes":[],"has_more":false,"cursor":"c2"}

                               ← MCP start commits tool_calls.status=running: nothing on the wire
id: c3                         ← CommitToolResult: tool message + a change with status unchanged
event: transcript.update
data: {"type":"transcript.update",
  "messages":[{"id":"…m3","turn_id":"…t1","sequence":9,"role":"tool",
    "content":[{"type":"tool_result","tool_use_id":"…tc1","content":"{\"total\":42.0,\"eligible\":true}"}], …}],
  "turn_changes":[{"turn_id":"…t1","revision":14,"status":"running","terminal":false,"current":true,
    "through_message_sequence":9,"error":null,"structured_output":null,"occurred_at":"…",
    "tool_calls":[{"id":"…tc1","name":"lookup_order","mode":"mcp","status":"completed","updated_at":"…"}]}],
  "has_more":false,"cursor":"c3"}

event: message.delta          ← iteration 2 (message …m4)
data: {…"message_id":"…m4","content_index":0,"offset":0,"kind":"tool_arguments",
  "delta":"{\"summary\":\"Refund $42 for A-17\"}","tool_call_id":"toolu_01CD…","name":"request_approval",…}

id: c4                         ← model checkpoint for iteration 2, then ParkTurnForHostTools
event: transcript.update
data: {"type":"transcript.update",
  "messages":[{"id":"…m4","turn_id":"…t1","sequence":10,"role":"assistant","phase":"commentary",
    "content":[{"type":"tool_use","id":"…tc2","name":"request_approval",
      "input":{"summary":"Refund $42 for A-17"}}], …}],
  "turn_changes":[{"turn_id":"…t1","revision":15,"status":"waiting","terminal":false,"current":true,
    "through_message_sequence":10,"error":null,"structured_output":null,"occurred_at":"…",
    "tool_calls":[
      {"id":"…tc1","name":"lookup_order","mode":"mcp","status":"completed","updated_at":"…"},
      {"id":"…tc2","name":"request_approval","mode":"host","status":"running",
       "arguments":{"summary":"Refund $42 for A-17"},"deadline_at":null,"updated_at":"…"}]}],
  "has_more":false,"cursor":"c4"}

: keepalive
```

The Turn now holds no goroutine and no lease. The client sees a `mode: host` call with `arguments` and renders the approval. The connection may rotate while the approver decides. The client reconnects with `cursor=c4` and receives nothing new, only the opener and keepalives. The approver's UI (or the host backend) then calls:

```http
POST /v1/turns/…t1/tool-results
{"results":[{"tool_call_id":"…tc2","content":{"approved":true,"by":"u-9"}}]}
→ 202 {"results":[{"tool_call_id":"…tc2","status":"completed","deduplicated":false}], …}
```

That call commits the tool message, the settlement, the tool checkpoint, the `queued` transition and the dispatch intent in one transaction. A worker then claims the Turn with **`attempt` 2**:

```
id: c5
event: transcript.update
data: {"type":"transcript.update",
  "messages":[{"id":"…m5","turn_id":"…t1","sequence":11,"role":"tool",
    "content":[{"type":"tool_result","tool_use_id":"…tc2","content":{"approved":true,"by":"u-9"}}], …}],
  "turn_changes":[
    {"turn_id":"…t1","revision":16,"status":"queued","terminal":false,"current":false,
     "through_message_sequence":11, …},
    {"turn_id":"…t1","revision":17,"status":"running","terminal":false,"current":true,
     "through_message_sequence":11, …,
     "tool_calls":[{"id":"…tc1",…,"status":"completed"},{"id":"…tc2",…,"mode":"host","status":"completed"}]}],
  "has_more":false,"cursor":"c5"}

event: message.delta          ← attempt 2: the client drops any attempt-1 previews it still holds
data: {"type":"message.delta","turn_id":"…t1","attempt":2,"message_id":"…m6","content_index":0,
  "offset":0,"kind":"text","delta":"Approved. The $42 refund for A-17 is on its way.",…}

id: c6                         ← final model checkpoint, then SettleTurn (possibly one drain)
event: transcript.update
data: {"type":"transcript.update",
  "messages":[{"id":"…m6","turn_id":"…t1","sequence":12,"role":"assistant","phase":"commentary",
    "content":[{"type":"text","text":"Approved. The $42 refund for A-17 is on its way."}], …}],
  "turn_changes":[{"turn_id":"…t1","revision":18,"status":"completed","terminal":true,"current":true,
    "through_message_sequence":12,"error":null,"structured_output":null,"occurred_at":"…",
    "stop_reason":"end_turn","tool_calls":[…both completed…],
    "usage":{"input_tokens":2210,"output_tokens":96,"model_calls":3},
    "provenance":{"provider":"anthropic","requested_model":"…","served_model":"…","provider_key_source":"platform"},
    "final_answer_message_id":"…m6"}],
  "has_more":false,"cursor":"c6"}

event: connection.closing
data: {"type":"connection.closing","reason":"settled"}
```

Notes on the example:

- **Correlating the tool preview to the saved block.** Match by `message_id` and block order, or by `name`. The preview's `tool_call_id` (`toolu_01AB…`) is the provider's ID, and the saved `tool_use.id` (`…tc1`) is Nvoken's ToolCall UUID. See discrepancy 1.
- **Reconnecting from `c1` after settlement.** This replays sequences 8 to 12 and revisions 14 to 18, writes no opener, and ends with `settled`. Replayed revisions 14, 15 and 17 now carry `current: false` and no `tool_calls`.
- **A process crash between the MCP start and its result.** The reaper records a `queued` change. The reclaim under `attempt` 2 records `running`. `lookup_order` is read-only, so it is re-run. Had it been destructive, a `tool_result` with `is_error: true` and the "outcome is unknown" text would appear instead. On the wire the client sees one extra queued/running pair and an attempt bump that voids stale previews.

### 5.11 Where docs and code disagree

1. **Preview `tool_call_id` is not the ToolCall ID.** `normalizedDelta` takes `event.ContentBlock.ID`, which is the provider's ID (`divegen/generator.go:2703-2708`). The test asserts `delta.ToolCallID == "call_1"` (`generator_test.go:554`). The saved block's `id` is rewritten to a fresh ToolCall UUID (`services/toolcalls.go:712-768`: `block["id"] = encodedID`), and the OpenAPI types `ToolUseBlock.id` as `ToolCallID`. Spec §4.2 says "Match a `tool_arguments` preview to its saved block by `tool_call_id`", and the §9 worked example shows the same `…tc1` in both places. Neither holds at HEAD. The `MessageDeltaEvent.tool_call_id` OpenAPI description ("The tool call these arguments belong to") does not say which ID space it uses. The fix is cheap: mint the ToolCall ID at `content_block_start`, or rewrite the delta's ID through the same map.
2. **`attempt` after a resume.** The spec's §9 example shows `"attempt":1` on previews after a `waiting -> queued -> running` cycle. `CommitTurnClaim` increments `attempt` on every claim (`turn_execution.sql:336`), so those previews carry `attempt: 2`. The client behavior is the same either way, because a higher attempt only voids stale previews.
3. **R25 overstates tool-status visibility.** R25 says "a tool call changing status reserves a lifecycle revision of its own, so a failed or long-running call is visible on the stream before the message carrying its result." In the code:
   - Only `CommitToolResult` (inline settlement) and terminal closing reserve a revision.
   - `StartToolCallAttempt` writes no transition and no wake (`adapters/postgres/toolcalls.go:117-127`), so a two-minute MCP call shows nothing between `tool_use` and `tool_result`.
   - The inline result message and its revision commit in one transaction and are delivered messages-first, so the change arrives *with* the result, not before it.
   - Host and callback results in a partial batch reserve no revision (`external_settlement.go:68-189`).

   The start and settlement preview frames proposed in `2026-08-08-tool-calling-records-and-contract.md` §4 were not built.
4. **Where they agree.** The architecture document's durability claims check out against the SQL. "Model output and requested ToolCalls commit together" (`CommitModelCheckpoint`), "tool results append their own checkpoints" (`CommitToolResult`, `settleExternalToolResults`), and "every settlement write is fenced on lease owner plus attempt" all hold.

---

## 6. Execution loop (brief)

```text
engine.Runner poll (embedded) | Cloud Task -> executorhttp (cloud_tasks)
  -> TurnExecutionService.ClaimNext/ClaimExact  (CommitTurnClaim, attempt+1)
  -> engine.ClaimExecutor.runClaim               (heartbeat/renew; deadline minus settlement reserve)
     -> GenerationExecutor.Execute               (one segment)
        drain nudges; prepare memory; load log + artifacts; compaction; recovery prefix
        seam decisions (interrupt / re-park / final replay / budget)
        -> divegen.Generator.GenerateStream      (dive.Agent.CreateResponse, WithMessages)
             hooks: PreIteration (nudges), Stop (nudges)
             EventCallback: deltas -> LiveEventPublisher; message+usage -> RecordModelCheckpoint
             tools: builtin/MCP record start+result; host -> SuspendResult
        <- GenerationResponse{Usage, MessagesCheckpointed, ExternalToolsPending, BudgetExceeded, Interrupted, ...}
     -> TurnExecutionService.settle | parkForExternalTools | parkForBudget   (fenced transaction)
```

- **One segment per claim.** A segment runs Dive's in-memory loop across inline tool iterations. A host tool or a budget hold ends the segment, and the next segment re-reads the log from Postgres. Nvoken does **not** re-read storage per step within a segment, which avoids ADK-Go's O(history) per-step scan.
- **The iteration cap** is Dive's `ToolIterationLimit`, set from the Turn's `max_iterations` clamped to an installation ceiling (`LimitPolicy.IterationCeiling`). Reaching it is `incomplete{max_iterations}`, or `budget_hold` if the Turn opted to hold.
- **Budgets are checked at checkpoint seams** (`checkpointBudgetExceeded`, `generator.go:1600`). Cost is estimated before each call (`MaxCostAtRiskMicroUSD`), and credits can refuse a call before it is made (`CreditCallRefusedError`).
- **Dive 1.34's "stopped short" responses** (`ResponseStatusIncomplete`) are mapped explicitly (`stoppedShort`, `generator.go:1396`). The HEAD commit message, "stop settling cut-off responses as finished", is the Dive v1.34 behavior change the simplification review flags as `DIVE-17`.

## 7. Tool system (brief)

| Mode | Who runs it | Durability | Wire |
|---|---|---|---|
| `builtin` | Nvoken, in-process. A small registry: `fetch`, and `memory_remember`/`recall`/`forget` when a MemorySpace is attached (`divegen/builtin_tool.go`) | start/accept records; restarted after a crash | `tool_use`/`tool_result`; no `arguments` in summaries |
| `mcp` | Nvoken, through guarded egress, to a remote MCP server admitted with the Turn | start/accept records; retried after a crash only if read-only or idempotent and not destructive | same |
| `host` | The host application (backend or browser) | `pending` ToolCall; the Turn parks `waiting` with no lease; results by `POST /tool-results` | `arguments` plus `deadline_at` while pending |
| `callback` | Nvoken POSTs a signed request to a host URL through the durable `deliveries` worker. The result comes back by submission (ack, then settle: PRD 062) | a delivery row is written with the model checkpoint | like host, answerable by machine credentials |

Tool definitions come from the frozen effective behavior: name, description, raw JSON Schema `input_schema`, mode, and callback URL and timeout. Builtin descriptors project into `dive.Tool` from one registry, "a builtin implementation can supply behavior but cannot restate its own contract" (`builtin_tool.go:14-18`). Unknown tool names requested by the model are logged (`EventUnknownToolRequested`). Structured output is a reserved tool (`structuredoutput.ReservedToolName`) whose validation failures go back to the model as bounded feedback. Provider-hosted tools (web search) become `server_tool_use` blocks. Stranded `pause_turn` server calls are removed from all but the last message (`dropStrandedServerToolCalls`, `generator.go:1367`).

## 8. Suspend/resume and error handling (brief)

- **There is one suspension primitive: the external ToolCall.** `waiting` covers host tools, callbacks, questions and approvals alike. Resume means submitting results, which queues the Turn for a normal claim. There is no in-memory continuation and no Dive `WithResume`: the next segment rebuilds from the log. Waiting is unbounded by default (`DefaultWaitingTimeout: 0`, `services/controls.go:69`). A waiting deadline, when set, is reaped to `failed{deadline_exceeded, scope: waiting}`.
- **A second suspension: `budget_hold`.** It applies to iteration, output-token, cost and credit ceilings when the Turn was admitted with `on_budget_exhausted: hold`. Deadlines pause while a Turn is held. `resume` raises the exact limit that ran out, and a credit grant resumes it automatically (`services/credit_resume_worker.go`).
- **Stops**:
  - An interrupt is soft and seam-based, and is recorded as `completed{interrupted}`.
  - A cancel is hard, and generation eligibility then excludes the Turn's work.
  - Limits end `incomplete` when the Turn stopped cleanly, or `failed` when a deadline lands mid-request.
  - Lease loss is not a Turn outcome. It leads to recovery.
- **Errors are classified in the service layer, not by Dive.** `classifiedProviderCallError` (`generator.go:405`) and `media_rejection.go` and `context_window.go` parse provider errors into `ProviderCallError{Class}` values: context window, media rejected, throttled, and so on (`DIVE-12`). Transient failures (`ports.TransientFailure`) and lease loss propagate as Go errors, so the claim is abandoned and later recovered. Everything else becomes a `TurnExecutionResult` with a `TurnFailure`. Recovery-evidence inconsistencies fail the Turn `internal` instead of executing.

---

## 9. Strengths, weaknesses, and lessons for Dive

### What is excellent and worth borrowing

1. **Intent before effect, implemented and tested at production grain.** A pending ToolCall row commits with the model message, a `running` row with an attempt number commits before the executor starts, and a result commits before the loop advances. `model_call_facts` does the same for provider calls. None of Eino, ADK-Go or MAF-Go records a tool that "may have run". Nvoken can tell "never started" (`pending`) from "started, outcome lost" (`running` under a superseded attempt), and acts on the difference.
2. **Fencing tokens everywhere.** `attempt` is incremented by every claim and checked by every write. A stale executor cannot commit. The same number scopes previews on the wire, so crash recovery and preview invalidation share one mechanism.
3. **Gap-free per-log counters under a single row lock.** One `UPDATE ... RETURNING` gives commit-ordered, snapshot-consistent sequences. These make exact cursors, messages-before-changes pages, and cross-Turn ordering fall out almost for free. It is the simplest correct way to get a replayable log out of Postgres.
4. **A durable/ephemeral split with shared identity.** Previews carry the saved message's future ID, an attempt, and a byte offset. Durable frames alone carry cursors. Loss is surfaced explicitly (`stream.resync`), and the durable path never depends on the live one: poll for correctness, wake for latency.
5. **Log fields versus current detail on changes.** Replayed history is truthful to its moment, and the current change answers "why is it stuck" in one read. `terminal` is computed by the server so that clients never keep a status list.
6. **Record versus projection in practice.** `phase` and generation eligibility are computed at read time, and provider-private blocks become `redacted`. The facts / ownership / evidence / projection taxonomy (§1) is a clean statement of the Dive v2 principle, with the scar tissue that motivated it.
7. **Safe-retry policy from tool annotations.** "Read-only or idempotent, and not destructive" is a pragmatic default for recovering an uncertain effect without a human.
8. **A specification written for third-party implementers.** Numbered MUST rules, a server algorithm, a client fold, conformance checklists and cross-SDK reducer fixtures. This is rarer, and more valuable, than the code.

### What is awkward

1. **The step record is reconstructed from outside Dive.** The event callback is the journal, sentinel errors are the stop signals, hooks inject nudges, and tool wrappers write intents. Correctness depends on callback ordering that Dive does not promise (`DIVE-01`, `DIVE-05`).
2. **The record is spread over five tables, and one of them is billing.** Model-call intent lives in `model_call_facts`, not in the Turn's evidence, so "every model attempt has a record" holds only for billing. A model call that returns nothing has no Turn-evidence row (`DIVE-04`).
3. **Messages are still the record.** Synthetic `tool_result` rows ("Tool execution stopped because the Turn ended.") and the unknown-outcome text are written into the transcript. This is the same "fact wearing a message costume" that Dive v2 finding 1 describes.
4. **Uncertainty is resolved by telling the model.** For a destructive MCP call with an unknown outcome, the model is told and the loop continues. The host gets no reconcile hook, and there is no `Uncertain` stop.
5. **Tool execution is invisible between `tool_use` and `tool_result`.** There is no start frame, and R25 overstates this (§5.11).
6. **ID spaces diverge on the wire.** Previews carry provider call IDs, while records carry ToolCall UUIDs (§5.11).
7. **High write amplification.** About five transactions per model iteration with one inline tool. This is acceptable for a service, but it would be a poor default for a library.
8. **There is no approval primitive and no typed question record.** PRD 052 is only proposed. Approval as a host tool works, but every client re-derives "is this an approval?" from the tool name.

### What should move down into Dive

1. **The Recorder contract, shaped by Nvoken's commit groups.** Dive v2's `Recorder.Commit(ctx, Step)` should make each of Nvoken's commit groups one step. Three requirements follow:
   - `model_completed` must carry the requested tool calls, so the host can write pending calls atomically with the message, exactly as `CommitModelCheckpoint` does.
   - `tool_started` must carry an attempt or fence number that the Recorder can check.
   - `model_requested` must precede every provider attempt, including ones that return nothing, so a host needs no side table.

   Dive should also pass a fencing token through `Deps`, such as a `Fence` that the Recorder validates, so hosts do not have to thread `attempt` through closures.
2. **Preview identity in the Observer's delta type.** Nvoken's `generationCheckpointState` exists to add three things Dive's model events lack, and all of them belong in Dive's `Delta`:
   - a message ID reserved before the first delta and reused by `model_completed`;
   - the call ID and name on every tool-argument fragment;
   - an attempt number and a per-block offset.

   Dive should also **mint one canonical tool-call ID at block start**, keep the provider's ID as a secondary field, and use the canonical ID in both the delta and the record. That removes discrepancy 1 at its source.
3. **An event vocabulary split into "committed" and "preview".** Nvoken's wire classes map directly onto Dive v2's Observer: `StepCommitted` (durable, ordered, identical on replay) and `Delta` (lossy, identity-bearing, voidable). Dive should document the same guarantees Nvoken's spec states: a delta never changes what is saved; the streaming and blocking paths persist identical results (R42); and a higher attempt voids earlier deltas.
4. **Typed tool-uncertainty policy.** Offer `Reconcile` as a Policy decision with three built-ins: `StopUncertain` (the Dive v2 default), `RetryIfSafe` (Nvoken's annotation rule), and `ReportToModel` (Nvoken's text, rendered by `Project` rather than stored as a message). Nvoken would then delete `settleMCPUnknownOutcome`, and Dive keeps "no synthetic messages in the record".
5. **Seams as commands.** Nudges map to `Deliver`, and interrupts at checkpoints map to soft cancel at step boundaries. `OnStop` continuing on undrained input maps to `StopDecision{Continue, Items}`. That replaces three hook insertions and two sentinel errors in `divegen`.
6. **Stop vocabulary aligned with Nvoken's client-facing semantics.** `completed` versus `incomplete` (clean limit) versus `failed` (unclean), an interrupt as a *completed* stop, and `budget_hold` as a resumable stop. These are well argued in the OpenAPI prose and map onto Dive v2's `Stop.Kind` plus `Next()`.
7. **A reference fold and conformance fixtures, but not a transport.** Nvoken's SDKs ship four reducers and a shared fixture. Dive could ship a small `dive/project` or `dive/stream` package: a Go reducer over `StepCommitted` and `Delta` that yields a render model (preview merge, `tool_result` reaching back to its `tool_use`, the final answer), plus fixtures. Noodle and Mobius need the same fold.

### What should stay in the gateway

- Postgres schema, triggers, the lock order, `SKIP LOCKED` claiming, leases, heartbeats and the reaper.
- The **log counters and cursors**. A cursor is a position in a *host's* log, which may interleave many Turns (Nvoken's Conversation log does). Dive should expose per-Turn `Seq` and let hosts map it onto their own positions.
- SSE transport: the opener, keepalives, jittered rotation, visitor eviction, write deadlines and slow-consumer farewells, and Redis fan-out with gap flags.
- Audience projection (R47: browser tokens lose usage and provenance), tenancy, credentials, credits, budget holds, callback delivery and webhooks.
- The approval and question product surface (PRD 052), built on Dive's `Waiting` outcome.

### Contrasts with Eino, ADK-Go and MAF-Go

| | Eino | ADK-Go | MAF-Go | Nvoken (on Dive 1.34) |
|---|---|---|---|---|
| Mid-loop durability | Checkpoint only on interrupt or cancel | Every non-partial event; tool intent not recorded | None mid-run | **Every model iteration and tool boundary**, fenced |
| Tool "may have run" | Unknown | Unknown (a dangling `FunctionCall`) | Unknown | **Known**: `running` under a superseded attempt |
| Crash recovery | Re-run the interrupted node | Rebuild from the log; memoized children | Re-run the superstep | Reclaim, validate the prefix, retry safe tools, report the rest to the model |
| Concurrency control | None | Timestamp optimism | Workflow CAS | Row locks plus a monotonic `attempt` fence |
| Idempotency | None | Duplicate HITL resumes ignored | None | Admission, checkpoint, tool result, host result, nudge |
| Streaming persisted? | Side-channel copy | Partials never persisted | Buffered once tools appear | Previews never persisted, but **keyed to the record they become** |
| Resumable stream | In-process iterator | In-process `iter.Seq2`; REST/SSE adapter | In-process; AG-UI/A2A adapters | **Cursor over committed state**, exact replay, multi-reader |
| Approval | Interrupt plus re-run | `adk_request_confirmation` | Built-in, **bound to recorded requests** | Host tool; bound to the recorded ToolCall; no dedicated type |
| Mid-run steering | Safe-point cancel | None | `MessageInjector` | Nudges drained at three seams, idempotent |
| Stop reasons | Inferred | Inferred | `FinishReason` string | Typed status, stop reason and failure code, with `terminal` |

**Where Nvoken is ahead:** the durability and streaming columns. It is the only system surveyed that survives a crash in the middle of a tool loop and knows which effects may have happened. It is also the only one whose stream can be resumed exactly from a cursor by any number of readers. Both properties validate Dive v2's principles 1 and 3 in production code.

**Where it is behind:**
- It has no graph or orchestration layer. `docs/proposals/2026-09-24-nvoken-backed-agent-swarm.md` is still a proposal, although tool-triggered child Turns exist through `parent_turn_id`.
- It has no approval type, where MAF-Go's is first-class.
- Its middleware is Dive's v1 hook set.
- The step record lives outside the agent library. That is not a weakness of Nvoken's design so much as the gap Dive v2 exists to close. Once Dive owns the record, most of `divegen` should become a `Recorder` over Nvoken's transaction.
