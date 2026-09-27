# Dive v2: conceptual model and core type foundation

**Date:** 2026-09-26  
**Status:** Independent assessment and proposed core model for discussion; not an approved specification.  
**Baseline:** v1.34.0 (`da15e91`), read together with the execution prototype in `experimental/v2` (`9e27a57`).  
**Companions:** [Integrator feedback](2026-09-26-dive-v2-integrator-feedback.md), [Architecture review](2026-09-26-dive-v2-architecture-review.md), [Top recommendations](2026-09-26-dive-v2-recommendations.md).

## Judgment

Dive's biggest deficiency is not a missing feature. It has the wrong primary
noun. The library is organized around `Agent.CreateResponse`, a request and
response call, and a full turn lifecycle was then attached to that call through
functional options, session type-switches, hooks, and synthetic messages. Each
attachment was reasonable on its own. Together they leave the engine with no
single owner for execution facts, and every serious consumer rebuilds those
facts outside the library.

The three companion documents diagnose the symptoms accurately and propose a
recording protocol, a session runner, typed policy decisions, and an honest
provider contract. This document agrees with that direction and pushes further
in three places:

1. **The turn is the aggregate and steps are the record.** The message list is a
   projection derived from the record, never the record itself. No stored record
   contains a synthetic message.
2. **Hooks dissolve into three narrower concepts:** typed policy decisions, an
   acknowledged recorder, and an isolated observer. Context delivery becomes one
   typed value with four independent dimensions.
3. **The agent definition is an immutable, versioned value** that the record
   pins, so accepted work can be recovered under the configuration it was
   accepted with.

The rest of the document is in three parts: an assessment of the current
concepts, the proposed core model, and how the two differ with an order of
transformation.

## Method and scope

This assessment reads the exported API of the root, `llm`, and `session`
packages at the baseline, the internal structure of `agent.go`, `turn.go`,
`hooks.go`, and `tool.go`, the three companion documents, and the
`experimental/v2` prototype. It does not rerun the architecture review's probes
and does not test live providers. Where it cites a probe result, the source is
the architecture review. Line references are to `da15e91`.

The unit of critique is the concept, not the implementation. A finding here says
a type or interface has the wrong responsibilities or the wrong relationships,
not that a function has a bug. Several implementation defects the review found
are consequences of the conceptual findings and are cited as evidence for them.

---

# Part I: Assessment

## The wrong primary noun

The natural way to read Dive today is: construct an `Agent`, call
`CreateResponse` with options, get a `Response`. That is a function-call model.
The facts that consumers need, and that the v1.33 and v1.34 work added, are
lifecycle facts: this turn started with this input, called the model three
times, ran two tools, suspended on a third, was resumed by another process, and
ended incomplete because the deadline passed. A function call does not have a
lifecycle. So the lifecycle was expressed through the only extension points a
function call has: its arguments, its return value, and callbacks.

The result is visible in the shape of the API:

| Concern | Today | Count |
| --- | --- | --- |
| Lifecycle transitions expressed as `CreateResponse` options | `WithInput`, `WithMessages`, `WithContinue`, `WithResume`, `WithToolResults`, `WithResumeRequest`, `WithBackgroundResults` | 7 |
| Hook phases sharing one mutable `HookContext` | SessionStart, PreGeneration, PreIteration, PreToolUse, PostToolUse, PostToolUseFailure, Stop, OnSuspend, OnIncompleteTurn, PostGeneration, PostBackgroundToolUse | 11 |
| Ways to put non-user text in front of the model | `SystemPrompt`, `Extension.Rules`, `InjectContext`, `SetSystemReminder`, typed reminders, `WithModelOnlyReminder`, `AppendReminder`, `SessionStartResult`, `StopDecision.Reason`, background-results message, turn-incomplete reminder, turn-continue reminder, `AdditionalContext` | 13 |
| Session interfaces the engine type-switches on | `Session`, `SuspendableSession`, `TurnStore`, `SessionClaimer` | 4 |
| Places the turn's messages appear on one `Response` | `Items`, `OutputMessages`, `Turn.Messages`, `Suspension.TurnMessages` | 4 |
| Status and reason vocabularies | `ResponseStatus`, `TurnReason`, `TurnNext`, `ToolCallState`, `SuspendReason`, `PersistenceState`, `TurnOriginKind`, `llm.StopKind` | 8 |
| Sentinel errors in the root package | `ErrResumeRequired`, `ErrNoSuspendedTurn`, `ErrContinueWithInput`, ... | 17 |
| Exported types, root and `llm` | | 96 + 105 |
| Lines in `agent.go`; in `CreateResponse` alone | | 3,793; 710 |

None of these numbers is damning alone. Together they show a library whose
concepts were added by accretion around a fixed center, rather than derived from
a model of what an agent turn is.

## Findings

### 1. The model's transcript is the record

A `[]*llm.Message` does three jobs: the durable account of what happened, the
projection the provider receives, and the unit of persistence. Because it is the
only durable structure, every execution fact and every projection choice ends up
inside it:

- The turn outcome is stored as the `Details` map of a reminder content block
  (`NewTurnOutcomeReminder`, read back with `FindTurnOutcome`).
- Closing an incomplete turn writes synthetic tool results into history
  (`CloseTurn`), with library wording as the result text.
- Continuation adds a `turn-continue` reminder so the request does not end in an
  assistant message.
- Background results arrive as a synthetic user message the caller never wrote.
- Model-only reminders exist because some context must be sent but must not be
  recorded, so the record and the request have already diverged.
- Compaction inserts a `SummaryContent` barrier into the same list.

Each is a fact or a policy wearing a message costume. This is the root cause of
DIVE-04, DIVE-08, DIVE-15, DIVE-24, and DIVE-26, and of Mobius's M-01 snapshot
problem: when the record is a list of messages, "replace the earlier catalog"
has no representation. The architecture review names the distinction between the
durable record and the projection, and the `experimental/v2` prototype already
inverts it: `replay(records)` derives state from steps. That instinct should
become the center of the design.

### 2. The turn is a by-product, not the aggregate

The thing with identity, state, and lifecycle is the turn. Today `Turn` is
assembled at the end of `CreateResponse` and attached to the response. Its
transitions are inferred from option combinations whose meaning depends on the
session's capabilities. `WithMessages` is history for a stateless caller and
input for a session caller. `WithContinue` folds into an open turn on a
`TurnStore` and starts a new turn elsewhere. `WithResume` overrides the
session's state when a `SuspendableSession` is present and is required when it
is absent. Seventeen sentinel errors exist largely to reject invalid
combinations. Internally, `generate` keeps a working message slice separate from
the one `CreateResponse` holds, which is why a Stop continuation loses direct
hook additions and why the iteration limit resets.

A turn should be a state machine. Callers apply commands to it. The state
machine knows what is valid next.

### 3. Agent fuses definition, runtime, and state

`Agent` holds the system prompt, model, tools, hooks, limits, and a default
session, and exposes `SetModel` and `SetSystemPrompt`. In a service the
definition is versioned configuration, the runtime is per request, and turn
state is per turn. Fusing the three means a shared agent can change under a
running turn, and there is no value to pin in the record. The addendum to the
feedback register asks to recover accepted work from its recorded configuration
rather than today's. That is hard precisely because no `Definition` value exists
to record.

### 4. Hooks are an application's plugin vocabulary used as the library's core

Dive copied Claude Code's hook names. Claude Code is an application, and those
names are its extension points for users. A library needs the concepts
underneath them: a policy decision before a model call or a tool call, delivery
of context, recording of facts, and observation for display. Instead there are
four overlapping mechanisms: eleven agent hook slices, `llm.Hooks` at the
provider level, the `Tracer`, and the event callback.

One mutable `HookContext` serves all eleven phases with different writable
fields per phase. The architecture review's probes show the consequences: a
`PreToolUse` rewrite reaches the tool but not the preview, trace, or result
metadata; a later hook can rewrite arguments an earlier hook authorized; hook
iteration numbers are not populated for tool phases. These are not bugs in the
hooks. They are what a shared mutable bag does.

### 5. One callback is observer, controller, and journal

`EventCallback` receives pointers shared with the internal record, stops the
turn by returning an error, and is treated by stateless consumers as the
durability feed. Those are three contracts with three failure semantics. An
observer must not be able to alter the record. A controller needs typed
decisions. A journal needs acknowledgement, ordering, and a defined behavior
when a write fails. Terminal events are delivered on an uncancelled context and
their errors are logged, which is right for an observer and wrong for a journal.
`Response.Items` retains every streaming delta whether or not anyone asked for
them.

### 6. Tools conflate definition, execution, and result protocol

`dive.Tool` and `llm.Tool` are two interfaces for one idea, and `llm` needs only
the definition half. `Tool.Call(ctx, input any)` erases the JSON boundary that
every consumer then reconstructs (DIVE-20). `ToolResult` is a tagged union of
normal, suspend, and background, so a tool's return value chooses the engine's
execution mode. A late result reaches the engine through three different
mechanisms: a suspended call through `WithResume`, `WithToolResults`, or
`WithResumeRequest`; a background task through `WithBackgroundResults`; and a
parallel call still running at cancellation through a `BackgroundTaskHandle` and
then `WithBackgroundResults`. External tools that only declare themselves must
implement a `Call` that suspends. Provider-hosted tools carry nil schemas into
ordinary dispatch. The Go error from `Call` is "a tool failure the model can
see" rather than "the outcome is unknown", which is why a lost lease became
model feedback in the review's probe.

### 7. Session is four stacked interfaces and two unrelated concerns

The engine behaves differently depending on which of `Session`,
`SuspendableSession`, `TurnStore`, and `SessionClaimer` the value implements,
and the first half of `CreateResponse` is that dispatch. Checkpointing is only
available when the session is a `TurnStore`, and each checkpoint hands the store
a whole turn to diff. The `session` package also mixes a product concept, a
conversation with a title, metadata, forks, and a compaction history, with a
systems concept, a journal with revisions, claims, and torn-write recovery. The
second is excellent and should survive. It should sit under the engine as a
recorder, not beside it as a sibling the engine inspects.

### 8. The `llm` layer is Anthropic's wire shape, generalized, and admits loss

`llm.Response` says in its own documentation that it matches the Anthropic
response format. There are 21 content structs, several of them Anthropic
server-tool result shapes promoted to core types. `llm.Config` documents that
unsupported options are ignored, which is an admission that the layer is a
lowest common denominator rather than a contract. `dive.ModelSettings` mirrors
`llm.Config`, and `AgentOptions.LLMHooks` mirrors `llm.Hooks`, so the same idea
exists three times. There is no concept of a prepared request with effective
settings, no per-attempt identity, and no typed provider error. Cost resolution
is installed through a process global. Streaming and blocking paths are
accumulated differently, which is how citation deltas were lost.

### 9. The vocabulary is flat

Model-call facts (`StopKind`), tool-call facts (`ToolCallState`), turn facts
(`ResponseStatus`, `TurnReason`, `TurnNext`, `TurnOriginKind`), and persistence
facts (`PersistenceState`) all live at the same level in one namespace, and
`ResponseStatus` doubles as `Turn.Status`. "Incomplete" is both a provider stop
kind and a turn status. There are thirteen `TurnReason` values because model
limits, cancellation, hook aborts, callback errors, and process exit are all one
enumeration. A layered vocabulary, one per level with a defined mapping upward,
would be smaller and clearer.

### 10. Cohesion and coupling

The root package has low cohesion. `Dialog`, `TerminalDialog`, `DateTimeString`,
`Ptr`, `UsageLogger`, `CompactionHook`, `PromptToolGate`, and `PromptStopHook`
sit beside the engine. Static coupling between packages is acceptable: the root
does not import `session`, `toolkit` depends only on `dive.Tool`, and the
adapters depend inward. The damaging coupling is inside `agent.go`: temporal and
semantic coupling between option parsing, session capability detection, the
working message slice, the hook context, and the turn record. `CreateResponse`
at 710 lines and `generate` at 341 are the measurable form of that coupling.

## What is right and must survive

- Disjoint usage buckets, `Absorb` versus `Add`, and cost provenance.
- Stop-reason classification into `StopKind`, with the raw value retained.
- The unknown versus not-started distinction for tool calls, and the rule that a
  running call with no result is unknown.
- The session store's revisions, claims, torn-write healing, forks, and
  non-destructive compaction.
- `AnswerUnansweredToolCalls` as an encoder backstop for foreign history.
- Permission rule matching for commands, paths, and domains.
- Typed function tools with generated schemas, previews, and multimodal results.
- Reminders as typed content with an authority tier and a name.
- Retry only at the stream-creation boundary, never of a partially consumed
  stream.
- Unknown tool names as recoverable non-execution with suggestions.

---

# Part II: The core model

## Principles

1. **One record, many projections.** The step log is the only durable account of
   a turn. The provider transcript, the UI transcript, the audit trail, and the
   billing record are all derived from it.
2. **The turn is the aggregate.** All state with identity lives on the turn.
   Commands transition it. The engine is stateless.
3. **Intent before effect, result before advance.** A model call or tool call is
   recorded as an intent before the network or the executor is touched, and its
   result is recorded before the loop advances. A recorded intent means the
   effect may have happened.
4. **Decisions are typed, inputs are immutable.** Policy sees a read-only view
   and returns a decision. Rewrites re-enter authorization.
5. **Observation cannot control.** An observer receives isolated values and has
   no return channel into execution.
6. **Unsupported is refused, not ignored.** A provider reports what it will do
   before it does it.
7. **Configuration is a value.** A definition is immutable and versioned, and
   the record names the version it ran under.
8. **One vocabulary per level.** A model call, a tool call, and a turn each have
   their own state words, with a defined mapping upward.

## Layers

Five layers. Dependencies point downward only. The engine never imports
`session`.

```text
session/      Runner, Store, Claims: one Recorder plus projection storage
dive/         Definition, Turn, Step, Command, Engine, Policy, Observer, Project
tool/         Def, Call, Executor, Outcome, Binding, Resolver
llm/          Provider, Request, Prepared, Result, Stop, Usage, Content, Error
adapters      permission, otel, a2a, skill, subagent, toolkit, providers/<vendor>
```

### Layer 0: `llm`, one model call, honestly reported

```go
type Request struct {
    Model    string
    System   string
    Messages []Message
    Tools    []tool.Def
    Settings Settings
}

// Settings is one struct. Reasoning is one value: Off, Effort(level), or
// Budget(tokens), plus a separate display preference.
type Settings struct { /* MaxTokens, Temperature, Reasoning, ToolChoice, ... */ }

type Provider interface {
    Name() string
    Capabilities(model string) (Capabilities, bool)
    Prepare(ctx context.Context, req Request) (Prepared, error)
    Call(ctx context.Context, p Prepared, sink Sink) (Result, error)
}

type Prepared struct {
    Request
    Effective   Settings       // what will actually be sent
    Adjustments []Adjustment   // clamps and drops, each with a reason; empty under strict
    Body        []byte         // encoded request, for evidence and token estimates
}

type Result struct {
    ID        string
    Message   Message
    Stop      Stop
    Usage     *Usage           // nil means unknown; zero means known zero
    Requested string
    Served    string
    Raw       json.RawMessage  // provider payload, never interpreted by core
}

type Stop struct {
    Raw     string
    Kind    StopKind           // Finished, ToolUse, OutputLimit, ContextLimit, Refusal, Pause, Incomplete, Other
    Details *StopDetails
}

type Sink func(Delta)          // text, reasoning, tool-arguments, citation deltas; nil for blocking

type Error struct {
    Provider   string
    Category   ErrorCategory   // RateLimited, Overloaded, ContextTooLong, MediaRejected, InvalidRequest, Auth, Transport, Unknown
    Status     int
    Code       string
    RetryAfter time.Duration
    Body       json.RawMessage
}
```

Content is a small canonical set plus one escape hatch:

```text
Text, Image, Document, ToolCall, ToolResult, Reasoning, Refusal, Opaque
```

`Opaque{Provider, Type, Raw}` carries any provider block the core does not
model, including server-tool results and replay signatures. It round-trips
byte-for-byte to the provider that produced it and is dropped, with a recorded
adjustment, by any other. This replaces the 21 structs and the
`ProviderMetadata` string map with one rule.

`Prepare` is strict by default: a setting the model does not support fails the
call before any network effect, and the error names the setting. A lenient
policy records each adjustment in `Prepared.Adjustments` instead. Unknown models
are reported as unknown and can use an explicit passthrough policy, so a static
catalog never blocks a new model. `Capabilities` carries provenance and a
qualification date. Pricing is a value on `Capabilities`, not a process global.

Both `Call` paths end in one `Result`. The streaming path feeds `Sink` and
accumulates; the blocking path passes a nil sink. Accumulation is one shared
implementation, so citations, annotations, and usage frames cannot diverge
between paths.

A scripted test provider, `llmtest.Provider`, with realistic result constructors
is part of this layer so Dive and its consumers stop maintaining incompatible
fakes.

### Layer 1: `tool`, definition, execution, and outcome are three things

```go
type Kind string // Local, External, ProviderHosted

type Def struct {
    Name        string
    Description string
    Schema      json.RawMessage   // raw JSON Schema, never re-modeled
    Annotations Annotations       // advisory hints; tri-state where MCP is tri-state
    Execution   Execution         // enforced policy: Parallel | Sequential | Chain
    Kind        Kind
}

type Call struct {
    ID        string
    Name      string
    Args      json.RawMessage     // effective, after transforms
    Requested json.RawMessage     // as the model wrote it, when different
}

type Executor interface {
    Execute(ctx context.Context, call Call) (Outcome, error) // non-nil error means Unknown
}

type State string // Succeeded, Failed, NotExecuted, Waiting, Detached

type Outcome struct {
    State  State
    Output *Output   // Succeeded or Failed
    Wait   *Wait     // Waiting: prompt, reason, metadata
    Handle *Handle   // Detached: delivers one Outcome later
}

type Output struct {
    Content []Block  // Text, Image, Audio, Resource; canonical, JSON-stable
    Display string
}

type Binding struct { Def; Executor }              // Executor nil for External and ProviderHosted
type Resolver func(ctx context.Context) ([]Binding, error)
```

The Go error from `Execute` means exactly one thing: the outcome is unknown. A
tool that knows it failed returns `Failed` with an error output. A tool that
knows it did nothing returns `NotExecuted`. This is the contract the prototype
arrived at and it removes the ambiguity that turned an infrastructure failure
into model feedback.

`Waiting` and `Detached` are states, not result variants. A tool that needs a
human returns `Waiting`. A tool that spawned work returns `Detached` with a
handle. An `External` definition has no executor and is always `Waiting`. A
`ProviderHosted` definition is sent to the provider and never dispatched
locally. The engine treats all three the same way: an effect is outstanding and
its result will arrive by command.

`Execution` is enforced by the engine. `Annotations` are advice for policy and
reconciliation and are never used to decide whether to retry an uncertain
effect. `FuncTool[T]` and `Typed[T]` remain as constructors that generate a
`Def` from a Go type and wrap the executor; they decode raw JSON at the boundary
and are the only place `any` appears.

### Layer 2: `dive`, the turn is the aggregate

#### Definition

```go
type Definition struct {
    ID       string
    Version  string
    System   string
    Model    llm.Provider
    Settings llm.Settings
    Tools    tool.Resolver
    Limits   Limits
    Context  []ContextItem      // standing operator context, such as rules
}

type Limits struct {
    ModelCalls      int           // per turn, across invocations; zero is invalid, use Default()
    PauseResends    int
    ToolConcurrency int           // 1 means sequential
    PerCallTimeout  time.Duration
}

func (d Definition) Hash() string  // stable over prompt, settings, tool defs, limits
```

A `Definition` is immutable. Changing the model means constructing a new
definition with a new version. The `turn_started` step records the definition
ID, version, and hash together with the effective settings and the tool
definitions in force, so a later invocation can tell whether it is running under
the configuration the work was accepted with.

#### Turn and Step

```go
type Turn struct {
    ID         string
    Steps      []Step             // the record
    // Derived, rebuilt by replay:
    Status     Status             // Open, Waiting, Stopped
    Calls      int                // model calls so far
    Usage      llm.Usage
    Pending    []Effect           // outstanding model or tool effects
    Working    []ContextItem      // ephemeral context for this invocation only
    Definition DefinitionRef
}

type Step struct {
    Version int
    TurnID  string
    Seq     int
    At      time.Time
    Kind    StepKind
    // Exactly one of the following is set, matching Kind:
    Started *Started
    Context *ContextItem
    Model   *ModelStep
    Tool    *ToolStep
    Stopped *Stop
}
```

Go has no sum types. A flat struct with a `Kind` and one populated payload is
JSON-stable, versionable, and easy to validate on replay, which is what the
prototype does. The step vocabulary:

| Kind | Payload | Meaning |
| --- | --- | --- |
| `turn_started` | input items, `DefinitionRef`, effective settings, tool defs, limits, origin | The turn exists. Nothing has been sent. |
| `context_delivered` | `ContextItem` with `Lifetime: Recorded` | Context the model will see from here on. |
| `model_requested` | attempt ID, model, effective settings, tool names, projection digest | Intent. A call may be in flight. |
| `model_completed` | attempt ID, `llm.Result` | The provider answered. Includes empty, refused, paused, and truncated answers. |
| `model_failed` | attempt ID, `llm.Error`, partial message, usage or nil | The provider did not answer, or the stream died. Usage nil means unknown. |
| `tool_started` | call ID, requested args, effective args, authorization decision | Intent. The executor may have started. |
| `tool_completed` | call ID, `Succeeded`, `Failed`, or `NotExecuted`, output | A definite result. A denial is `NotExecuted` with the decision, and has no `tool_started`. |
| `tool_waiting` | call ID, `Waiting` or `Detached`, wait info | An effect is outstanding; its result arrives by command. |
| `tool_uncertain` | call ID, error | The executor returned an error or the invocation ended while it ran. |
| `turn_stopped` | `Stop` | The invocation ended with this stop. A turn can stop and be continued. |

Step identity is `(TurnID, Seq)`. A repeated identity with identical content is
accepted as a duplicate. A repeated identity with different content is rejected.
Replay validates transitions, as the prototype does, so a corrupted or
hand-edited record fails to load rather than executing on a false state.

#### Commands

```go
type Command interface{ isCommand() }

type Start    struct { Input []ContextItem; Origin Origin }
type Deliver  struct { Items []ContextItem }
type Continue struct { Items []ContextItem; AcceptDefinitionChange bool }
type Accept   struct { CallID string; Outcome tool.Outcome; CommandID string }
type Cancel   struct { Reason string }
```

| Command | Valid when | Effect |
| --- | --- | --- |
| `Start` | turn has no steps | Records `turn_started` and runs the loop. |
| `Deliver` | turn is open or waiting | Records recorded items as steps, holds ephemeral items in the working set, runs nothing. For durable runtimes that queue nudges between iterations. |
| `Continue` | turn is stopped with a continuable stop, or waiting with all effects resolved | Delivers items, then runs the loop. Fails with `ErrDefinitionChanged` when the current definition hash differs from the recorded one, unless accepted. |
| `Accept` | a call is `Waiting`, `Detached`, or `Uncertain` | Records the outcome. A duplicate command ID with identical content acknowledges and does nothing else. A different content under the same ID, or a new ID for a settled call, is rejected. When no effect remains outstanding, runs the loop. |
| `Cancel` | turn is open or waiting | Records `turn_stopped{Canceled}` without a model call. Started calls become `tool_uncertain`; unstarted calls become `tool_completed{NotExecuted}`. |

`Accept` is the one path for every late result: a human answer to a suspended
tool, an external executor's result, a detached background task, and the
reconciled result of an uncertain call after a crash. Reconciliation is not a
separate mechanism. It is `Accept` with a verified outcome for a call recorded
as uncertain.

Hard cancellation remains the context's job. Soft cancellation remains a context
value the loop checks at step boundaries. Neither is a command, because both can
arrive while the engine holds the turn.

#### Engine, Recorder, Policy, Observer

```go
type Engine struct{}

func (Engine) Apply(ctx context.Context, t *Turn, def Definition, cmd Command, deps Deps) (Outcome, error)

type Deps struct {
    Recorder Recorder
    Policy   Policy
    Observer Observer    // optional
    Spawner  Spawner     // optional; process-local default
}

type Recorder interface {
    Commit(ctx context.Context, step Step) (Receipt, error)
}

type CommitError struct {
    Disposition Disposition   // Rejected: nothing was written. Unknown: the write may have landed.
    Cause       error
}

type Policy interface {
    BeforeModel(ctx context.Context, plan ModelPlan) (ModelDecision, error)
    AuthorizeTool(ctx context.Context, call tool.Call, view TurnView) (ToolDecision, error)
    OnStop(ctx context.Context, view TurnView) (StopDecision, error)
}

type ModelDecision struct { Deliver []ContextItem; Settings *llm.Settings; Abort *Abort }
type ToolDecision  struct { Allow bool; Feedback string; Rewrite json.RawMessage; Abort *Abort }
type StopDecision  struct { Continue bool; Items []ContextItem }

type Observer func(Event)   // Event: ModelDelta, ToolOutput, ToolProgress, StepCommitted; isolated copies
```

The engine loop, in words:

1. Replay the turn's steps to derive state. Refuse an invalid record.
2. Apply the command's own step or steps through `Recorder.Commit`. A commit
   error ends the invocation with `Unacknowledged` set to the step that did not
   land and `WriteError` saying whether the write was rejected or is unknown.
3. While the turn is open and nothing is outstanding:
   - If the last model result requested tools, prepare each call: normalize
     and validate the arguments against the schema, run `AuthorizeTool`, apply
     a rewrite and re-authorize, then commit `tool_started` with both the
     requested and the effective arguments. Denials commit
     `tool_completed{NotExecuted}` and never start. Execute with the
     definition's concurrency bound; commit each completion in observed order.
     `Waiting` and `Detached` outcomes commit `tool_waiting` and, once the
     batch is drained, end the invocation with `Stop{Waiting}`.
   - Otherwise, if the model-call limit is reached, commit
     `turn_stopped{Limited}` and return.
   - Otherwise, run `BeforeModel` with the plan. Deliver any items. Build the
     projection. Call `Provider.Prepare`. Commit `model_requested`. Call the
     provider, feeding the observer. Commit `model_completed` or
     `model_failed`. Classify the stop kind: a refusal, a limit, or an
     unrecognized stop with tool calls commits `turn_stopped` and returns; a
     pause is resent up to the limit; a finished answer runs `OnStop` and
     either continues or commits `turn_stopped{Completed}`.
4. Return the `Outcome`.

A `Go error` from `Apply` means the invocation never began: an invalid command,
a corrupt record, or a nil dependency. Everything after the first commit is
described by the `Outcome`. A policy infrastructure error after the turn began
is `Stop{Failed}` with the cause; a policy denial is a decision and is recorded
as one.

```go
type Outcome struct {
    Turn           *Turn
    Stop           Stop              // this invocation's stop
    Persistence    Persistence       // Acknowledged, Rejected, Unknown
    Unacknowledged *Step
    WriteError     *CommitError
}

type Stop struct {
    Kind   StopKind   // Completed, Waiting, Canceled, Limited, Refused, Failed, Uncertain
    Limit  string     // for Limited: model_calls, output, context, pause
    Cause  error      // for Failed; also serialized as text
    Detail *llm.StopDetails
}

func (s Stop) Next() Next   // Input, Continue, Accept, Reconcile; advisory
```

This is the turn vocabulary. It maps upward from `llm.StopKind` and `tool.State`
by fixed rules and replaces `ResponseStatus`, `TurnReason`, `TurnNext`, and
`SuspendReason` in the root namespace. Persistence is a separate field, never
folded into the stop.

#### Projection

```go
func Project(history []llm.Message, t *Turn, p ProjectionPolicy) ([]llm.Message, error)

type ProjectionPolicy struct {
    DropPartialText   bool
    IncludeReasoning  bool
    RenderAuthority   llm.ReminderAuthorityResolver
    Compaction        *Compaction        // summary to substitute for earlier history
}
```

`Project` builds the provider transcript from the record:

- `turn_started` input items become the user message.
- Recorded context items become reminder blocks. Items with `Disposition:
  Snapshot` and the same name render only in their latest version, which is the
  M-01 fix, and an empty snapshot renders an explicit "nothing remains".
  Cumulative items all render.
- `model_completed` becomes the assistant message, with reasoning and opaque
  blocks carried as the provider needs them.
- Tool outcomes become tool result blocks in call order: `Succeeded` and
  `Failed` as their output, `NotExecuted` and `Uncertain` as library wording
  that says so. A `Waiting` call has no result and the turn cannot be projected
  for sending; the engine never asks.
- A `turn_stopped` that a `Continue` follows renders one contextual item
  explaining the stop, built from the recorded `Stop`. The wording is a
  projection choice and can change between releases; the record does not.
- Working ephemeral items append at the tail.
- Compaction substitutes a summary for earlier history and leaves the record
  alone.

Nothing in `Project` writes. The same function serves the engine, a UI that
wants the model's view, and a test that checks what would be sent.

#### ContextItem

```go
type ContextItem struct {
    Name        string
    Origin      Origin        // User, Operator, Library, Tool
    Authority   Authority     // Operator, Contextual
    Lifetime    Lifetime      // Recorded, Ephemeral
    Disposition Disposition   // Cumulative, Snapshot
    Content     []llm.Content // usually text; may carry images or documents
    Details     map[string]any
}
```

One type with four independent dimensions replaces the thirteen mechanisms in
the inventory. Origin says who asserted the item and is authorship, not
authorization. Authority says how strongly the provider should render it.
Lifetime says whether it is a step or a working-set entry. Disposition says
whether later items with the same name accumulate or replace. User input is a
`ContextItem` with `Origin: User` so that a turn's input and a mid-turn nudge
are the same kind of thing. Standing rules on the `Definition` are items with
`Origin: Operator`. Skills publish a snapshot item. A tool that wants to inform
the model after it returns delivers a `Tool` item.

### Layer 3: `session`, a Recorder plus a convenient runner

```go
type Runner struct {
    Store  Store          // append steps, list turns, load projected history
    Claims Claimer        // optional lease; required for several processes on one session
    Policy Policy
}

func (r Runner) Run(ctx context.Context, sessionID string, def Definition, cmd Command, obs Observer) (Outcome, error)
```

`Run` claims the session, loads the projected history and the open turn,
constructs a `Recorder` over the store, applies the command, releases the claim,
and returns. The store keeps the tested revision, claim, torn-write, and fork
behavior it has today. Its unit of storage becomes the step, appended as one
line, instead of a whole turn to diff. Compaction records a summary and a range
of turns it replaces; `Project` consumes it. Titles, metadata, and listing
remain as conveniences on the store.

The engine never sees the runner. An application that owns its database
implements `Recorder` over its transaction, calls `Engine.Apply` directly, and
uses `Project` with its own history. Both callers get the same execution
contract.

### Layer 4: adapters

| Package | Today | In the model |
| --- | --- | --- |
| `permission` | A `PreToolUseHook` that auto-allows without a dialog | A `Policy` whose `AuthorizeTool` requires an explicit decision for a missing approver. `Dialog` moves here. |
| `otel` | Implements `Tracer`; misses cost, reasoning, incomplete state | Consumes `StepCommitted` events and per-attempt model steps. A small `Instrumentation` seam for span context propagation into provider calls remains. |
| `a2a` | Maps `Status` and `SuspendReason` | Maps `Stop.Kind`. `Waiting` is input-required; `Uncertain` and `Failed` are distinct. |
| `skill` | `Extension` with hooks and a legacy reminder path | Publishes a snapshot `ContextItem` and tool bindings. Skill content after invocation is a `Tool`-origin item. |
| `subagent`, `orchestration` | In-memory spawn and `Runs` tracker | A `Spawner` seam. The default is process-local and returns `Detached` handles. A host can supply durable child turns. |
| `toolkit` | Depends on `dive.Tool` | Depends on `tool.Binding` only. |
| `providers/<vendor>` | Encode from `llm.Config`, ignore what they cannot send | Implement `Prepare` and `Call`. Dialect knowledge and typed errors live here. |
| CLI | Agent plus session | `Runner` plus an `Observer`. |

A Claude Code compatible hook facade can be built over `Policy` and
`ContextItem` for consumers who want those names. It is a convenience, not the
core.

## Consequences of the shape

**One mechanism for an outstanding effect.** Suspend, background, and a parallel
call still running at cancellation are today three mechanisms with five resume
options. Here they are one outcome state, `Waiting` or `Detached`, and one
`Accept` command with an idempotent command ID. External tools follow the same
path. The prototype's recovery experiment, where a second worker accepts a
reconciled result and a redelivery is harmless, is this contract.

**The record never contains a synthetic message.** Closed tool calls, the
explanation of an incomplete turn, continuation prompts, and background results
are all projection artifacts. `IncompleteTurnOptions.Discard` has no reason to
exist: the record is always kept and the projection decides what the model sees.
`Turn.Messages`, `OutputMessages`, `Suspension.TurnMessages`, and
`Response.Items` collapse into `Project` plus the observer.

**Context has one type and four dimensions.** Delivered items are steps, so
continuation cannot lose a nudge and iteration numbering does not restart.
Snapshot disposition gives Mobius's catalog problem a representation. Trusted
operator context survives start, continue, and resume because it is in the
record.

**Policy replaces hooks.** Each decision has an immutable input and a typed
result. A rewrite re-enters authorization, so the call that is authorized is the
call that runs, is previewed, is traced, and is recorded. An infrastructure
error inside a policy fails the invocation. A denial is a recorded decision. An
absent approver is a configured decision, never an implicit allow.

**Turn-wide counters by construction.** The turn owns model-call count, usage,
pending effects, and working context across suspension and continuation because
it is the only state.

**Configuration is pinned.** `turn_started` records the definition reference,
effective settings, and tool definitions. Recovery reads them back. A changed
definition is visible and must be accepted explicitly.

**Providers report what they did.** `Prepare` runs before any admission or spend
reservation, and its `Effective` and `Adjustments` are recorded on
`model_requested`. Requested and served models are separate fields. Both paths
end in one `Result`, so streaming cannot lose evidence the blocking path keeps.

**Every model attempt has a record.** An empty answer, a refusal, a pause, a
truncated answer, and a failed stream each commit a step with usage or an
explicit unknown. Aggregates are sums over steps. Nothing is inferred from blank
text.

## Worked flows

**Simple call, no session.**

```go
turn := dive.NewTurn()
out, err := engine.Apply(ctx, turn, def, dive.Start{Input: dive.UserText("Hello")},
    dive.Deps{Recorder: dive.MemoryRecorder(), Policy: dive.AllowAll()})
fmt.Println(out.Turn.AnswerText())
```

The facade `Agent.CreateResponse` does exactly this. The simple case stays one
call.

**External tool through a session.** `Runner.Run(Start)` records the input, the
model requests `book_flight`, the definition marks it `External`, the engine
commits `tool_started` and `tool_waiting`, and returns `Stop{Waiting}`. Hours
later another process calls `Runner.Run(Accept{CallID, Outcome, CommandID})`.
The runner loads the turn, the engine commits `tool_completed`, and the loop
continues to a final answer. A network fault that loses the acknowledgement
leads the caller to reload and resend the same `Accept`, which returns a
duplicate and does nothing else.

**Crash while a local tool runs.** The record ends at `tool_started`. On the
next load the runner sees an open turn with a pending effect and a claim that
has expired. It commits `turn_stopped{Uncertain}` under `process_exit`, and
`Stop.Next()` says reconcile. The application checks the downstream service by
the call's stable ID, then `Accept`s the verified outcome. The engine never
re-executes a recorded intent.

**Stop-policy continuation with a nudge.** `OnStop` returns `Continue` with a
recorded `ContextItem`. The engine commits `context_delivered`, increments no
special counter, and calls the model again under the same turn-wide limit. If
the process dies and the turn is continued elsewhere, the nudge is in the record
and the projection renders it.

**Compaction.** The store records a summary and the turn range it replaces.
`Project` substitutes the summary for that range. The steps remain and are
recoverable. A compacted session with an uncertain last turn still says so,
because the turn record is what decides, not the transcript.

---

# Part III: How it differs and how to get there

## Comparison

| Today | Model |
| --- | --- |
| `Agent` is definition, runtime, and state | `Definition` value, stateless `Engine`, stateful `Turn` |
| `CreateResponse(opts...)` with the mode inferred from options and session type | `Apply(turn, def, Command)` with five explicit commands |
| Messages are the record; facts ride in reminder details | Steps are the record; messages are `Project` output |
| Eleven hook slices sharing one mutable context | `Policy` with three typed decisions, `Recorder`, `Observer`, `ContextItem` |
| `EventCallback` observes, controls, and journals | `Observer` for display, `Recorder` for durability, commands for control |
| Four session interfaces the engine switches on | Engine sees only `Recorder`; `session.Runner` wraps it |
| `ToolResult` chooses suspend or background | `tool.Outcome` state; late results arrive by `Accept` |
| `dive.Tool` and `llm.Tool`, `Call(ctx, any)` | `tool.Def` for `llm`, `tool.Executor` with raw JSON for `dive` |
| `llm.Config` ignores unsupported options | `Provider.Prepare` fails or records adjustments |
| 21 content structs shaped by one provider | Seven canonical blocks plus `Opaque` |
| Eight overlapping status vocabularies | One per level: `llm.Stop`, `tool.State`, `dive.Stop` |
| Whole-turn checkpoints a store must diff | Steps appended one at a time |
| Process-global cost resolver and provider registry | Pricing on `Capabilities`; explicit construction |

## Transformations, in dependency order

The first two determine everything else. Each item names what it lets a consumer
delete, because that is the acceptance test.

1. **Invert record and projection.** Define `Step`, `Turn`, and `Project`. Port
   `CloseTurn`, the outcome reminder, the continue reminder, and the
   background-results message into `Project`. Deletes: Noodle's partial recorder
   and sidecar journal; Nvoken's step reconstruction from callbacks and its
   private-artifact sidecar; Mobius's replay assembly.
2. **Make the turn the aggregate and commands the API.** Replace the option
   combinations with `Start`, `Deliver`, `Continue`, `Accept`, and `Cancel`.
   Keep `Agent.CreateResponse` as a facade for the simple case. Deletes: the
   error-priority ladders in every consumer, and most of the seventeen sentinel
   errors; the persistence ones and the result-conflict one remain.
3. **Split the callback into `Recorder` and `Observer`, and hooks into
   `Policy`.** Ship the Claude Code compatible hook facade at the same time.
   Deletes: Nvoken's evidence wrappers; the stateless checkpoint code; every
   defensive copy consumers make of callback payloads.
4. **Split tools into `Def`, `Executor`, and `Outcome`.** Raw JSON at the
   boundary. External and provider-hosted tools become kinds of definition.
   Deletes: dummy suspending `Call` methods for host tools; tool-input
   normalization helpers; the three late-result paths.
5. **Add `Provider.Prepare` and shared attempt middleware.** Strict by default.
   Move dialect knowledge and typed errors into adapters. Deletes: Nvoken's
   credential and budget wrappers, its error-string parsing, and most of its
   capability catalog.
6. **Collapse context mechanisms into `ContextItem`.** Deletes: Mobius's
   completeness conventions and continuation workarounds.
7. **Make `Definition` immutable and pinned in the record.** Deletes: the
   application's own configuration snapshotting for recovery.

Correctness repairs the review already lists, citation payloads, schema
round-tripping, effective-input reporting, ignored tool choice, and explicit
approval behavior, should ship on v1 where they can, with compatibility review.
They do not wait for the model.

## Mapping to the feedback register

| Feedback | Where the model answers it |
| --- | --- |
| DIVE-01, N-01 | `Recorder` under the engine; `session.Runner` as one owner among others |
| DIVE-02 | Commands; history is `Project` input, input is a `Start` item |
| DIVE-03, N-04, N-07 | `Outcome` with `Stop` and separate `Persistence`; Go error only before the first commit; no `Discard` |
| DIVE-04, DIVE-18, S-01, GEN-04 | `model_requested` and `model_completed` for every attempt; `Refused` is a stop kind |
| DIVE-05, N-06 | `StopDecision`, `Cancel{Reason}`, `ModelDecision.Abort` with a caller reason |
| DIVE-06, ORCH-03 | `Execute` error means `Uncertain`; `Abort` from policy is turn-fatal |
| DIVE-07, DIVE-23, SIM-11 | `Limits` on the `Definition`, counted on the `Turn` across invocations |
| DIVE-08, DIVE-26, CTX-03 | `context_delivered` steps; the working set survives continuation |
| DIVE-09, N-05 | `Project` drops unanswered provider-hosted calls in all but the tail message |
| DIVE-10, GEN-02 | One accumulator behind both `Call` paths; citations in the canonical `Text` block |
| DIVE-11 | `Prepare` then `Call`; middleware around each attempt |
| DIVE-12, PROV-04 | `llm.Error` with category and raw body |
| DIVE-14, PROV-02, GEN-05 | `Capabilities` with provenance; strict `Prepare` |
| DIVE-15 | `Step.Version`; `Opaque` for provider-private blocks |
| DIVE-16, DIVE-34 | `otel` reads steps, including per-attempt model and cost |
| DIVE-19 | `tool.State` on every outcome; `tool_started` is the only "about to run" event |
| DIVE-20 | Raw JSON on `tool.Call`; typed decoding inside `Typed[T]` |
| DIVE-21 | `llmtest.Provider` |
| DIVE-22 | `Settings.Reasoning` as one value plus display |
| DIVE-24 | No synthetic messages in the record; `Origin` on every item |
| DIVE-25 | `Policy` with immutable views and typed decisions |
| DIVE-27, GEN-06, PROV-06 | Explicit defaults on `Definition`; no unconditional priming |
| DIVE-28, TOOL-05 | `Def.Schema` is raw JSON Schema |
| DIVE-29 | `Def.Kind` |
| DIVE-30 | `Def.Execution` enforced; `Annotations` advisory and tri-state |
| DIVE-31 | `Prepare` fails or records adjustments |
| DIVE-32 | `tool.Output` canonical blocks; error rendering in `Project` |
| DIVE-35, ORCH-01, ORCH-02 | `Spawner` seam; `permission` requires an explicit missing-approver decision |
| M-01, F1, F2, F5, F8 | `ContextItem.Disposition: Snapshot` |
| M-03 | `Prepare` refuses or records a dropped block |
| M-04, USAGE-01 | `Usage` unchanged; one accumulator |
| Addendum §1 | `Runner.Run` exposes the whole engine contract |
| Addendum §2 | `DefinitionRef` in `turn_started`; `Continue.AcceptDefinitionChange` |
| Addendum §3 | `Waiting` versus `Detached`; `Wait.Reason` for deadline policy |
| Addendum §4 | `Prepare` before `model_requested`; usage on every attempt step |

DIVE-13 and DIVE-33 are construction and packaging concerns that the model does
not change; they should be fixed in the provider adapters regardless.

## Where this diverges from the companion documents

- **The agent as entry point.** The architecture review keeps `Agent` as the
  familiar entry and adapts hooks onto the new contract. This document treats
  `Agent` as a facade over `Definition` and `Runner`, and hooks as an optional
  compatibility layer over `Policy`. The reason is the third finding: as long as
  `Agent` is the center, definition, runtime, and state stay fused.
- **`Turn.Messages`.** The review still speaks of the turn's messages as a
  field. Here messages are only ever `Project` output. This is what retires the
  outcome-in-reminder-details representation and makes M-01 solvable.
- **Event sourcing.** The review warns against a public event-sourced workflow
  framework. Agreed on the framework, the scheduler, and the actor model. But
  the record is an event log, the prototype already replays one, and `Step`
  should be the public contract rather than an internal detail. Embrace the log;
  add no machinery around it.
- **Definition versioning.** The recommendations do not mention it, yet the
  addendum's second item depends on it. It belongs in the first prototype that
  records `turn_started`.
- **Prototype step shape.** The prototype's `Step` is close to right. The
  additions here are `tool_waiting` as a distinct kind, requested versus
  effective arguments on `tool_started`, `context_delivered`, and the definition
  reference on `turn_started`.

## Risks and open questions

- **Command ergonomics in Go.** Five command structs are less idiomatic than
  functional options. The facade must make the one-line case one line, and the
  examples must make ignoring `Outcome.Stop` awkward.
- **Projection cost.** `Project` becomes a hot path. The runner should cache the
  projected history per completed turn in the store, and a benchmark with long
  tool outputs should precede any copying decision.
- **Content canonicalization.** Reducing 21 blocks to seven plus `Opaque` risks
  fidelity if `Opaque` does not round-trip faithfully, or if a block one
  provider treats as opaque is one another provider needs to read. The rule
  "byte-for-byte to the producer, dropped with a recorded adjustment elsewhere"
  needs a conformance test per provider.
- **Ephemeral context.** This document keeps `Lifetime: Ephemeral` items out of
  the record for privacy and size. The alternative, recording everything and
  letting `Project` respect lifetime, gives a complete audit trail at the cost
  of persisting content the caller asked not to persist. Decide before freezing
  `Step`.
- **Denials as steps.** Here a denial commits `tool_completed{NotExecuted}` with
  the decision and no `tool_started`. An alternative records `tool_authorized`
  as its own kind for every call. The first is fewer steps; the second is a
  cleaner audit of the authorization phase. Either is consistent with the model.
- **Write amplification.** Intent steps must be acknowledged before effects, so
  they are synchronous writes. A turn with many small tool calls commits several
  times per call. The file store already appends a line per step; a
  transactional host may want to batch results with its own accounting write,
  which the recorder interface allows.
- **Migration size.** This is a major version with a v1 record importer that
  turns stored message lists into steps, preserving unknown states and provider
  artifacts. The importer must be exercised on completed, incomplete, suspended,
  running, compacted, and forked v1 sessions.

## Acceptance criteria

The review's criteria stand. Add these, which are specific to this model:

- No stored record anywhere contains a message the user or the model did not
  produce.
- Suspend, background, external, and post-crash reconciliation share one
  executor state and one `Accept` command, exercised by one test suite.
- Noodle runs on `session.Runner` and deletes its partial recorder.
- Nvoken implements `Recorder` over its transaction, calls `Engine.Apply`
  directly, and deletes its callback checkpointing, evidence wrappers, and
  error-string parsing.
- Mobius calls `Project` with its own history and deletes its completeness
  conventions.
- A definition change between admission and recovery is detected by a test, not
  by a user.
- The same conformance fixtures run against every provider adapter's `Prepare`
  and `Call`, streaming and blocking, and produce identical `Result` values.

---

## Appendix: baseline inventory

Counts at `da15e91`, from `go doc -all` on the root, `llm`, and `session`
packages and from `wc -l` on non-test sources.

| Measure | Value |
| --- | --- |
| Exported types, root package | 96 |
| Exported functions, root package | 117 |
| Exported types, `llm` | 105 |
| Exported functions, `llm` | 197 |
| Interfaces, root package | 15 |
| Hook function types | 11 |
| `CreateResponse` options | 12 |
| Content block structs, `llm` | 21 |
| Non-test lines, root package | 10,058 |
| Non-test lines, `llm` | 4,814 |
| Non-test lines, `session` | 2,800 |
| Lines, `agent.go` | 3,793 |
| Lines, `CreateResponse` | 710 |
| Lines, `generate` | 341 |
