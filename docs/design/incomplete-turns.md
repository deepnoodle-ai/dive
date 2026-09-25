# Incomplete Turns

_Last updated: 2026-09-25_
_Status: accepted; v1.34 implemented (see "Order of work"), Phase 2 next. Answers the Noodle team's request "Dive:
keep turns that don't finish" (24 September 2026, against v1.33.0). Third
revision, after six reviews: the pull-request review (a running call must
not be recorded as "not run"; the partial-resume rule; the error contract;
outcome metadata), an independent review of how turns end today (the loop
never reads a stop reason; retrying a resume reruns tools; the background
results message is missing from `OutputMessages`; the cancel/suspend race), and
a colleague's design notes (the turn as the durable unit and `CreateResponse`
as one invocation of it; the record versus the projection sent to the model;
one status axis for what can happen next and a reason axis for what happened;
checkpoints). A fourth round of notes moved a refusal to a completed turn,
kept PostGeneration's existing scope, added an unknown persistence state and
the protocol-end rule for streams, and split Phase 2 into recoverable turns
and per-step durability. A fifth revision, after the pull-request reviews of
the third and fourth, made persistence honest about what a save error proves,
defined what an append-only session stores for a continuation and a resume,
fixed stop-reason precedence in the adapters, stopped a cancellation from
overriding a suspension whose work may already be dispatched, and specified
the late-result handle. A sixth revision, after the review of the fifth,
made a stop reason Dive does not recognize withhold the response's tool
calls, and aligned the suspension save row and section 9 with the
persistence contract. A seventh revision, after checking the design
against the code, corrected the server-tool cleanup rule, gave
`model_context_window_exceeded` its own reason, `context_limit`, since the
same history cannot be continued, required the parallel batch to hand a late
tool's real result to its handle, made `WithContinue` add its reminder
whenever the history ends in an assistant message, made `LockSession` return
the locked context, and replaced the order of work with six steps. Section
references to `agent.go` are against v1.33.1._

A turn that does not finish is recorded the way a turn that finishes is: what
happened is kept, the history is valid to send again, and the record says how
the turn ended and what can happen next, to the model and to the application.

The design has three parts:

1. **v1.34, backwards compatible.** Read the model's stop reason. Save on
   every exit. Close the turn so it can be sent again. Record the outcome.
   Return the turn with the error. Add a continuation, a soft cancel, and an
   encoder backstop. The old behaviour stays one option away.
2. **Phase 2, additive.** Checkpoint the turn as it runs, with a typed turn
   record, per-call execution state and revisions, so a crash loses nothing
   and a resume cannot rerun a tool.
3. **A later breaking iteration.** Fold the outcome into the core contracts.

## Two ideas the design rests on

**The turn is the unit of work; `CreateResponse` is one invocation of it.** A
turn starts with input and can span several model calls, tool batches,
suspensions and retries. An invocation can stop while the turn is unfinished.
Dive already treats a suspended turn this way; every other unfinished ending
is today either reported as completed or thrown away. In v1.34 every
invocation leaves the turn in one of three states, `completed`, `suspended`
or `incomplete`, and an incomplete turn says why it stopped and what can
continue it. Phase 2 gives the turn an identity and a revision so an
invocation can be retried against it.

**The record and the projection.** What the session keeps is the record of
what happened: the input, every finished message, every tool result, the
draft the model was writing, what is known about each call in flight, the
usage. What the model receives is a projection of that record: every call
answered, drafts kept or dropped, a note on how the turn ended. In v1.34 the
projection is computed once, when the turn is closed, because the `Session`
contract stores messages; the structured record rides along in the outcome
reminder's details so the projection can be rebuilt, and `CloseTurn` is the
one function that computes it. In Phase 2 the session stores the record and
computes the projection on load, with the same function. v1.34 is therefore
*conversation preservation* and Phase 2 is *turn recovery*; the two names are
used below so that neither promises what the other delivers.

## Summary of decisions

- **One new status, `ResponseStatusIncomplete`, with a reason.** The status
  says what can happen next; `TurnOutcome.Reason` says what happened:
  cancelled, deadline, provider error, stream interrupted, hook abort,
  callback error, output limit, iteration limit, pause. There is no
  `Canceled` or `Failed` status: a cancellation and a provider error are both
  turns that stopped short, and the application renders them by reason. A
  refusal is a completed turn: the model finished responding, and
  `Response.StopReason` says how.
- **`TurnOutcome.Next` says what the turn needs**: `continue` (another model
  call on the record as it stands), `reconcile` (a call with an unknown
  result must be checked first), or `input` (nothing to continue).
- **The agent reads the stop reason.** A response that stopped at the output
  limit or in a refusal runs none of its tool calls, and neither does one
  with a stop reason Dive does not recognize; a pause is continued a
  bounded number of times; the iteration limit ends the turn incomplete
  instead of running calls the model will never see; a stream that ends
  without its protocol's end marker is an interruption, not a finish.
- **`Response.Turn`** is the turn as this invocation left it: the messages a
  session saves (input and output, closed), cumulative usage, the outcome or
  the suspension, and whether it was persisted. It is set for every status.
  `Response.OutputMessages` and `Response.Usage` keep their invocation scope.
- **`(resp, err)` on every exit after the turn begins.** `err` is non-nil
  when the invocation hit an error; a turn the model itself stopped short
  returns `err == nil` with `Status == Incomplete`. `GenerationError` stays
  and gains a `Response` field.
- **Every tool call in the batch in flight is answered by what is known.**
  A call whose tool returned keeps its result. A call that never started is
  "not run, no effect". A call that was running is "unknown result, may have
  taken effect", and its late result comes back as a handle on
  `Response.BackgroundTasks`.
- **The outcome is a typed reminder with structured details**, name
  `turn-incomplete`, rendered with wording Dive owns per reason. Older Dive
  versions decode it as a plain reminder, which matters for rollbacks.
- **`WithContinue()`** runs another invocation with no new input, which is
  how a stopped or failed turn is picked up without rerunning any tool.
- **Session writes at the end of an invocation use a salvage context**, and
  a suspension is persisted even when the context has been cancelled, since
  the tool may already have dispatched the external work. `Persistence` says
  `failed` only when the store rejected the write before writing; any other
  error is `unknown`, and the session resyncs from its store.
- **`OnIncompleteTurn` hook**, **`WithSoftCancel`**, an **encoder backstop**
  for unanswered calls, and **`IncompleteTurns{Discard, DropPartialText,
  SaveTimeout}`** as before.
- **One terminal stream item, `turn_ended`**, for every invocation end.

### What it looks like in an application

A chat application with a stop button, on a Dive session:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{Model: anthropic.New(), Tools: tools, Session: sess})

ctx, cancel := context.WithCancel(ctx)       // second press: stop now
ctx, softCancel := dive.WithSoftCancel(ctx)  // first press: stop after this step

resp, err := agent.CreateResponse(ctx, dive.WithInput(text), dive.WithEventCallback(render))
switch {
case resp == nil:
    return err // the turn never started: bad input, session failed to load
case resp.Status == dive.ResponseStatusSuspended:
    askUser(resp.Turn.Suspension)
case resp.Status == dive.ResponseStatusIncomplete:
    o := resp.Turn.Outcome
    switch o.Reason {
    case dive.TurnReasonCanceled:
        notice("Stopped.")
    case dive.TurnReasonOutputLimit:
        notice("The answer was cut off.") // offer "continue"
    default:
        notice("The turn failed: " + o.Error)
    }
    if o.Next == dive.TurnNextReconcile {
        // resp.BackgroundTasks holds the late results of calls that were
        // still running: await them and deliver with WithBackgroundResults,
        // or check their effect yourself.
    }
default:
    show(resp.OutputText())
}

// "Continue" button, after a stopped or cut-off turn:
resp, err = agent.CreateResponse(ctx, dive.WithContinue())
```

Rebuilding the transcript from the session later, the marker is data:

```go
msgs, _ := sess.Messages(ctx)
for _, m := range msgs {
    if outcome, ok := dive.FindTurnOutcome(m); ok {
        renderMarker(outcome.Reason, outcome.Error)
        continue
    }
    renderMessage(m)
}
```

Nothing changes for an application that never cancels, never hits a limit,
and never sees a provider error.

## What happens today

Everything below was confirmed against v1.33.1, most of it by the independent
review's probes.

| What happened                                                          | `CreateResponse` returns          | The session keeps                                   |
| ---------------------------------------------------------------------- | --------------------------------- | --------------------------------------------------- |
| Normal finish                                                          | Completed                         | the turn                                            |
| A tool suspends                                                        | Suspended                         | the partial turn (SuspendableSession only)          |
| Cancel, provider error or callback error after tools ran               | `nil, *GenerationError`           | nothing: the input and the tool effects are forgotten |
| `max_tokens`, `refusal`, `pause_turn`, or a stream ending at EOF with no finish reason | Completed, no stop reason exposed | saved as if finished                     |
| Iteration limit reached while the model keeps calling tools            | Completed, `OutputText() == ""`   | a turn ending in a tool result, its calls executed  |
| `max_tokens` cuts off a `tool_use`                                     | the tool runs on truncated JSON, then `GenerationError` on the next call | nothing |
| `SaveTurn` fails, or a Stop, PostGeneration or OnSuspend hook aborts   | `nil, err`, the finished answer and its usage lost | nothing                          |
| A resume, then the model call fails                                    | `GenerationError`                 | still suspended; a retry reruns the tools and hooks |

Four details matter for the design.

**The partial work already exists in memory.** `generate` wraps every loop
failure in `*GenerationError` carrying the usage, output messages and items
so far, and `CreateResponse` folds in Stop-hook continuations and
resume-phase items (`agent.go:937-959`). Its doc comment says the partial
turn is deliberately not persisted because a half-turn can violate provider
role-alternation invariants. A half-turn is only invalid while a tool call
has no result, and the agent has what it needs to answer one.

**Both tool-batch paths discard finished results.** The sequential loop
returns `nil, ctx.Err()` though `batch.Outcomes` holds the results of every
completed call. The parallel drain loop does the same, and results that
finished but were not yet drained stay in the buffered channel.

**Streamed text is recoverable.** `ResponseAccumulator.Response` finalizes
whatever blocks it has. `generateStreaming` returns a nil response on error,
so the partial message never reaches the loop.

**The loop never reads `StopReason`.** It ends when the response has no
client tool calls (`agent.go`, `generate`). So a final answer cut at
`max_tokens` is completed; a `pause_turn` is completed with an unanswered
server tool call left in history, which Anthropic accepts only at the end of
a paused turn; a truncated `tool_use` is executed on its partial input, and
the next request fails because `ToolUseContent.MarshalJSON` rejects invalid
JSON; and the chat-completions stream iterator synthesizes a clean stop when
a stream ends at EOF with no finish reason
(`providers/openaicompletions/stream_iterator.go`, `endStream`). The API
guidance for a harness is the opposite: check `max_tokens` and `refusal`
before running tools, and resend on `pause_turn`.

Also confirmed: the message injected by `WithBackgroundResults` reaches the
session save but not `OutputMessages`, so a stateless caller following the
documented recipe loses it; the CLI compares the error with `!=
context.Canceled`, which never matches a wrapped error; and `finishSuspended`
never checks the context before `SaveSuspendedTurn`, while `CancelSuspension`
bypasses the per-session lock, so an A2A cancel can race a suspension write.

## v1.34 design

### 1. How a turn ends: status, reason, next

```go
const (
    ResponseStatusCompleted  ResponseStatus = "completed"
    ResponseStatusSuspended  ResponseStatus = "suspended"
    ResponseStatusIncomplete ResponseStatus = "incomplete" // new
)
```

The status is the coarse thing every caller checks, and it says what can
happen next: a completed turn needs nothing, a suspended turn needs external
results, an incomplete turn stopped short and carries an outcome. Why one
status rather than the proposal's `Cancelled` and `Failed`: a 429 after
retries, a user's stop and a cut-off answer all leave the turn in the same
position, stopped with a valid record, and the application already needs the
reason to render any of them. Two axes, status and reason, keep the API
small and the rendering honest.

```go
// TurnOutcome says how an incomplete turn stopped and what can continue it.
// It is set on Response.Turn.Outcome, given to hooks, and saved as the
// details of the turn-incomplete reminder (see FindTurnOutcome).
type TurnOutcome struct {
    // Reason says what stopped the turn.
    Reason TurnReason `json:"reason"`

    // Error is the error that ended the invocation, as text, for the error
    // reasons. Hooks may rewrite it before it is saved, for example to
    // remove a request ID from a provider error.
    Error string `json:"error,omitempty"`

    // Hook is the hook type ("PreToolUse", "Stop", ...) for TurnReasonHookAbort.
    Hook string `json:"hook,omitempty"`

    // UsageUnknown is set when the invocation's usage could not be observed,
    // as when a stream died before its usage frame, so a zero Usage is not
    // a measurement.
    UsageUnknown bool `json:"usage_unknown,omitempty"`

    // ToolCalls records every call of the batch in flight when the turn
    // stopped, in call order, with what is known about it. Empty when the
    // turn stopped between batches.
    ToolCalls []ToolCallRecord `json:"tool_calls,omitempty"`

    // Next says what the turn needs: another model call on the record as it
    // stands (WithContinue), reconciliation of a call with an unknown result
    // first, or new input because there is nothing to continue. Advisory:
    // the application decides.
    Next TurnNext `json:"next"`
}

type TurnReason string

const (
    TurnReasonCanceled          TurnReason = "canceled"           // the context was cancelled, or a soft cancel was requested
    TurnReasonDeadline          TurnReason = "deadline"           // the context's deadline passed, including ResponseTimeout
    TurnReasonProviderError     TurnReason = "provider_error"     // a model call failed after the provider's own retries
    TurnReasonStreamInterrupted TurnReason = "stream_interrupted" // a stream ended without a terminal event
    TurnReasonHookAbort         TurnReason = "hook_abort"         // a hook returned HookAbortError
    TurnReasonCallbackError     TurnReason = "callback_error"     // the event callback returned an error
    TurnReasonError             TurnReason = "error"              // any other error; Error says which
    TurnReasonOutputLimit       TurnReason = "output_limit"       // the model stopped at max_tokens
    TurnReasonContextLimit      TurnReason = "context_limit"      // the response filled the model's context window
    TurnReasonIterationLimit    TurnReason = "iteration_limit"    // ToolIterationLimit reached with calls still requested
    TurnReasonProviderStopped   TurnReason = "provider_stopped"   // the provider stopped for a reason Dive does not act on; Error carries it
    TurnReasonPause             TurnReason = "pause"              // a server tool loop paused more than the agent continues
    TurnReasonProcessExit       TurnReason = "process_exit"       // Phase 2: the process ended with the turn open
)

type TurnNext string

const (
    TurnNextContinue  TurnNext = "continue"
    TurnNextReconcile TurnNext = "reconcile"
    TurnNextInput     TurnNext = "input"
)

// ToolCallRecord is what is known about one call of the batch in flight.
type ToolCallRecord struct {
    ID    string        `json:"id"`
    Name  string        `json:"name"`
    State ToolCallState `json:"state"`
}

type ToolCallState string

const (
    ToolCallStateCompleted ToolCallState = "completed" // a result is recorded, success or tool error
    ToolCallStateNotStarted ToolCallState = "not_started" // never started; it had no effect
    ToolCallStateUnknown   ToolCallState = "unknown"   // started, no result recorded; it may have taken effect
    ToolCallStateWaiting   ToolCallState = "waiting"   // Phase 2: suspended, awaiting an external result
)
```

The reason decides two things: whether the invocation returns an error, and
the default `Next`.

| Reason               | `err`   | Default `Next` | Notes                                                                                      |
| -------------------- | ------- | -------------- | ------------------------------------------------------------------------------------------ |
| `canceled`           | non-nil | `input`        | `errors.Is(err, context.Canceled)`; the person chose to stop, so the next move is theirs    |
| `deadline`           | non-nil | `continue`     | `errors.Is(err, context.DeadlineExceeded)`; a limit was hit, not a choice                  |
| `provider_error`     | non-nil | `continue`     | the provider's own retries were already spent; a later invocation retries the model step   |
| `stream_interrupted` | non-nil | `continue`     | partial output kept                                                                        |
| `hook_abort`         | non-nil | `input`        | the application stopped it; `Hook` names the hook                                          |
| `callback_error`     | non-nil | `continue`     |                                                                                            |
| `error`              | non-nil | `input`        |                                                                                            |
| `output_limit`       | nil     | `continue`     | the model's own stop; the answer is valid as far as it goes                                |
| `context_limit`      | nil     | `input`        | the same history cannot be sent again; the application shortens it first (see section 4)   |
| `iteration_limit`    | nil     | `continue`     | the requested calls were not run                                                           |
| `provider_stopped`   | nil     | `continue`     | the provider reported an early end it did not name as a limit or refusal, or a stop reason Dive does not recognize; `Error` carries the raw value; no call runs |
| `pause`              | nil     | `continue`     | only after the agent's own pause continuations were spent                                  |
| `process_exit`       | none    | `continue`     | Phase 2, set on load; no invocation returned it                                            |

Whatever the reason, `Next` is `reconcile` when any call in `ToolCalls` is
`unknown` and its tool is not annotated `ReadOnlyHint`: something may have
happened that the record cannot confirm, and the application should deliver
the late result or check the effect before the model repeats the call. The
annotation changes the advice, not the record: a started read-only call is
still `unknown`. The value is advisory; `WithContinue` works in every state.

### 2. The turn boundary

The turn begins immediately before PreGeneration hooks run. By then the
session history is loaded, the suspension, if any, is validated and prepared,
SessionStart seeds are saved, and the input is accepted. Errors before that
point (`ErrNoSuspendedTurn`, `ErrInputOnSuspendedSession`, a session load
error, a SessionStart hook error, the session lock lost to cancellation)
return `(nil, err)` and change nothing, exactly as today.

One exit after the boundary is not an incomplete turn. A partial resume,
where the caller supplied some pending results and others are still
outstanding, never calls the model. If it fails in a post-tool hook, in the
event callback, in PostGeneration or in `SaveSuspendedTurn`, the session is
left as it was in memory, resyncs from its store, and `(nil, err)` is
returned; the caller reloads and resubmits. Section 9 has the rule.

### 3. Closing the turn

Before anything is saved, the agent turns what it has into a record any
provider accepts. The rules, in order:

1. **The input messages are always kept**, including a synthetic
   background-results message and Stop-hook continuation reminders, which the
   current code already folds into the saved turn.
2. **Every finished assistant message and every finished tool-result message
   is kept as it stood.** Mid-turn compaction rewrites only the model-facing
   working set; the saved turn comes from the output accumulator, as for a
   completed turn.
3. **Every call of the batch in flight is answered by what is known about
   it**, and recorded in `TurnOutcome.ToolCalls`:

   - **A call whose tool returned a result keeps it**, even an error result
     the tool produced because its context was cancelled: that is what
     happened, and it may carry partial output. State `completed`.
   - **A call that never started** is answered "not run": a later call in a
     sequential batch, every call in a batch whose PreToolUse phase aborted,
     every call of a response that stopped at the output limit or in a
     refusal (a refusal's calls are answered this way although the turn is
     completed, section 4), every call requested past the iteration limit,
     or a call the agent refused to start because the context was already
     cancelled. It had no effect, and the text says so. State `not_started`.
   - **A call that had started and had not reported** is answered "unknown
     result". Only a parallel batch can leave a call in this state:
     sequential execution waits for the running call, as it does today, so
     that call always records its own result. The agent does not wait for a
     running parallel tool, which is the proposal's requirement and the
     1.32.0 decision that cancelling stops waiting. It drains, without
     blocking, the results that have already landed in the batch channel,
     keeps those, and answers the rest as unknown: the tool may take effect
     after the turn is saved, so the text says the result is unknown and
     never that the call had no effect. State `unknown`, for a read-only
     tool as for any other: `ReadOnlyHint` says repeating the call is
     harmless, not that it never ran, so it softens the advice (section 1)
     and leaves the record alone.

   ```go
   // ToolCallNotRunText answers a tool call the turn ended before it
   // started. The call had no effect.
   const ToolCallNotRunText = "Not run: the turn ended before this call started. It had no effect."

   // ToolCallUnknownText answers a tool call that was running when the turn
   // ended and had not reported a result. Whether it took effect is unknown.
   const ToolCallUnknownText = "Unknown result: the turn ended while this call was running. Its result was not recorded; it may have taken effect, so check before repeating it."

   // ErrToolCallNotRun and ErrToolCallUnknown are the ToolCallResult.Error of
   // such calls, as ErrBatchHalted is for a halted call.
   var (
       ErrToolCallNotRun  = errors.New("dive: not run because the turn ended")
       ErrToolCallUnknown = errors.New("dive: result unknown because the turn ended")
   )
   ```

   **An unknown call's late result is not lost.** The existing background
   path creates a handle only when a tool returns `BackgroundResult`, so an
   in-flight synchronous call needs its own registration. When the batch
   ends with calls still running, the agent registers a
   `BackgroundTaskHandle` for each before returning: a fresh task ID, the
   call's `ToolUseID`, a description naming the call, and the PreToolUse hook
   context. A forwarder that outlives the batch moves each goroutine's
   eventual completion from the batch channel to that handle's `Done`
   channel. Today a goroutine whose tool fails after the batch context is
   cancelled sends the context error in place of the tool's result
   (`executeToolCallsParallel`), and a sibling's context error cancels the
   whole batch; both change, so the goroutine always sends the tool's own
   result and the drain records rather than aborts; a panic is already an error result by the time it reaches the
   channel, as `executeTool` recovers it, and the late stream events stay
   suppressed by the existing callback gate. The handles are attached to
   `Response.BackgroundTasks` next to any from tools that returned
   `BackgroundResult`. From there the mechanism Dive already has for a result
   that arrives after its turn applies: `AwaitBackgroundTasks`, with whatever
   deadline the application chooses, and `WithBackgroundResults` on the next
   invocation deliver the result to the model through the existing
   `background-tasks` reminder, which gains the tool-use ID of the call each
   result settles so that two calls with the same description stay distinct,
   and `PostBackgroundToolUse` hooks fire for it. This is what
   `Next == reconcile` points at.

   The two texts split by what the call did, not by why the turn ended; the
   outcome reminder says why. Results appear in call order, with
   `AdditionalContext` text after them, the same shape as any batch. A soft
   cancel (section 10) never leaves a call unknown, because a running batch
   is allowed to finish.
4. **Text the model was still writing is kept** as the last assistant
   message, with `DropPartialText` to leave it out. `generateStreaming`
   returns the accumulator's partial response with the error, and
   `ResponseAccumulator` gains a way to report which blocks never received
   `content_block_stop`. In that partial message a text block is kept; a
   `tool_use` block whose input is not complete JSON is dropped, since
   `ToolUseContent.MarshalJSON` cannot send it; a thinking block that was
   still streaming is dropped, since its signature arrives last; a server
   tool call (`server_tool_use`, `mcp_tool_use`) whose result block is not in
   the partial message is dropped. That needs a new rule: the existing
   `dropServerToolCalls` only drops calls whose result block arrived and was
   skipped as undecodable. The new helper finds the server tool calls in a
   message that have no result block with the same ID; rule 5 uses it too,
   since a paused turn's trailing call is such a call. The same cleanup
   applies to a response the
   model stopped at the output limit: a truncated `tool_use` is dropped, a
   complete one is recorded and answered "not run". A partial message with
   nothing left is not recorded. Non-streaming `Generate` has no partial
   content.
5. **A pause the agent gave up on** (section 4) has its trailing server tool
   call dropped, since a request that follows it with a user message is
   rejected.
6. **The outcome reminder is appended last**, as a user-role message (section
   5). `Recorded` reminders that hooks queue while the turn ends are recorded
   before it, so the outcome reminder is always the final message of a closed
   turn.

The closing rules are one exported function so a session (Phase 2) or an
application with its own persistence can apply them to messages it holds:

```go
// CloseTurn returns turn with every tool call that has no result answered
// and the outcome recorded as a reminder at the end. Calls the outcome lists
// as unknown are answered with ToolCallUnknownText; every other unanswered
// call with ToolCallNotRunText, and the outcome's ToolCalls is completed
// with their records. It does not modify turn.
func CloseTurn(turn []*llm.Message, outcome *TurnOutcome) []*llm.Message
```

The cleanup of a partial streamed message (rule 4) is done by the agent where
the accumulator is, not by `CloseTurn`, which only sees messages.

### 4. Completion evidence at the model boundary

The loop stops treating "no client tool calls" as "finished". After each
model response it classifies the stop reason, then acts:

```go
// llm
type StopKind string

const (
    StopKindFinished    StopKind = "finished"     // end_turn, stop, stop_sequence
    StopKindToolUse     StopKind = "tool_use"
    StopKindOutputLimit StopKind = "output_limit" // max_tokens, length
    StopKindContextLimit StopKind = "context_limit" // model_context_window_exceeded, model_length
    StopKindRefusal     StopKind = "refusal"      // refusal, content_filter
    StopKindPause       StopKind = "pause"        // pause_turn
    StopKindIncomplete  StopKind = "incomplete"   // the provider ended early for a reason it did not name as a limit or refusal
    StopKindOther       StopKind = "other"        // a value Dive does not recognize; never runs a call
)

// ClassifyStopReason maps a provider's stop reason, as llm.Response carries
// it, onto the kinds the agent acts on. Unknown values are StopKindOther,
// which the agent treats as finished only when the response carries no
// client tool call.
func ClassifyStopReason(reason string) StopKind
```

The classifier's table covers every provider Dive ships, not only the four
adapters named below: Anthropic's `model_context_window_exceeded` (the
context window, not `max_tokens`, cut the response) is its own kind, as is
Mistral's `model_length`, which means the same, and
Ollama, Mistral, Grok and OpenRouter spellings are listed from their
adapters. Providers keep their raw values (Anthropic `end_turn`/`max_tokens`/`refusal`/
`pause_turn`, Gemini `stop`/`max_tokens`/`other`, chat completions
`stop`/`length`/`content_filter`, the Responses API's mapped set), and
`Response.StopReason` exposes the raw value of the last model response.

**Precedence in the adapters.** A stop reason reports why the response
ended, not whether it contains tool calls. An adapter reports `tool_use`
only when the response ended because of its tool calls; an output-limit,
refusal or other early termination takes precedence over the calls the
response also carries, which the agent then answers "not run". Today the
Responses adapter reports `tool_use` as soon as it sees a call item, before
it checks `status == incomplete` and `max_output_tokens`, so a cut-off
response that carries a call would run it; that order is reversed, and its
`incomplete` without a named reason, `cancelled`, `timeout` and `error`
values classify as `StopKindIncomplete`. The Google adapter maps every finish
reason other than stop and max-tokens to `other`, which loses the safety
reasons and the tool-call reasons; it maps `SAFETY`, `RECITATION`,
`BLOCKLIST`, `PROHIBITED_CONTENT` and `SPII` to their lowercase names, which
classify as the refusal kind, and `MALFORMED_FUNCTION_CALL`,
`UNEXPECTED_TOOL_CALL` and `TOO_MANY_TOOL_CALLS` to their lowercase names,
which classify as `incomplete`. The raw value is always kept on
`Response.StopReason`.

**A value Dive does not know never runs a call.** Unknown values classify as
`other`. A response of that kind with no client tool call is treated as
finished: nothing acts on the guess, the raw value is on
`Response.StopReason` for the application, and marking every compatible
server's odd spelling of a normal end as incomplete would only churn. One
that carries a client tool call runs none of them: the calls are answered
"not run" and the turn is incomplete with `provider_stopped`, exactly as for
`StopKindIncomplete`. The adapter checklist above is Dive's best knowledge
of each provider, not a guarantee: the Google SDK Dive pins already defines
`UNEXPECTED_TOOL_CALL`, "the tool call generated by the model is invalid",
and `TOO_MANY_TOOL_CALLS`, neither of which the adapter names today, and a
provider adds a truncation or refusal spelling before Dive learns it. A call
run on such a response acts on output the provider said did not complete,
and nothing reports it; a call withheld is reported in the outcome with the
raw value, and is fixed by adding the spelling to the adapter. Dive fails
closed where a wrong guess would act and open where it would only report.

The agent's rules:

- **Output limit.** None of the response's tool calls run. A truncated
  `tool_use` is dropped from the message; a complete one is answered "not
  run". The turn is incomplete with `output_limit`, `err == nil`, and
  `Next == continue`: the recorded note tells the model to continue exactly
  where it stopped. Automatic continuation is left to the application, which
  knows whether more output is wanted; `WithContinue` is one call away.
- **Context limit.** Handled like the output limit, with one difference:
  the input and the partial answer already fill the context window, so
  sending the same history again, with or without `WithContinue`, is
  rejected as too long. The turn is incomplete with `context_limit`,
  `err == nil`, and `Next == input`. The partial answer is recorded, and the
  application shortens the history before it continues: `session.Compact`,
  or a PreGeneration hook that trims the working set, then `WithContinue`.
  Dive does not compact on its own here; a compaction policy is the
  application's (the `experimental/compaction` hooks already provide one).
- **Refusal.** None of the response's tool calls run; any it made are
  answered "not run". The turn is completed, not incomplete: the model
  finished responding and there is nothing to continue. `Response.StopReason`
  is the provider's refusal value and `Response.StopDetails` carries the
  category when the provider reports one, which is what an application
  routes on. No outcome reminder is recorded.
- **Pause.** The agent resends the conversation, with the assistant message
  and its trailing server tool call and no new user message, up to
  `pauseTurnLimit` (10) times per invocation, as the API documents. Beyond
  the limit the turn is incomplete with `pause`; the trailing server tool
  call is dropped from the saved message so the history stays valid, and the
  next invocation starts the server loop again. The limit is a constant, not
  an option, until someone needs to tune it.
- **Provider stopped.** A response of `StopKindIncomplete`, or of
  `StopKindOther` that carries a client tool call, runs none of its tool
  calls; they are answered "not run". Incomplete with `provider_stopped`,
  `err == nil`, `Next == continue`, and `Error` carrying the raw stop
  reason. A `StopKindOther` response with no client tool call is completed.
- **Iteration limit.** Today the last allowed iteration sends `tool_choice:
  none` and a nudge; if the model still requests tools, the calls run and
  the loop ends with their results unseen. They no longer run: the turn is
  incomplete with `iteration_limit`, the calls are answered "not run", and a
  continuation gives the model more iterations. `Response.OutputText()`
  being empty is no longer the only signal.
- **Stream interrupted.** A provider adapter distinguishes its protocol's
  end from the transport's. The chat-completions iterator treats `[DONE]` as
  a valid end even when no `finish_reason` arrived, since some compatible
  servers omit it, and synthesizes the closing events as it does today; a
  bare EOF with neither is an error, `io.ErrUnexpectedEOF`, in place of the
  fabricated `message_stop`. The retry wrapper's rule stands, no retry after
  the first event. The agent keeps the partial response and whatever usage
  was observed, records `stream_interrupted` with `err` non-nil and
  `Next == continue`, and sets `UsageUnknown` when no usage frame arrived, so
  a zero `Usage` is not mistaken for a measurement. Tool-call fragments in
  the partial message stay visible in the model events on `Response.Items`
  and are never run.

### 5. Telling the model and the application

The outcome is recorded as a typed reminder whose `Details` carry the
`TurnOutcome`:

```go
// llm
type ReminderContent struct {
    Name    string         `json:"name"`
    Tier    ReminderTier   `json:"tier"`
    Content string         `json:"content"`
    // Details is structured data about the reminder for applications. It is
    // not rendered to the model. Values must be JSON-friendly; after a
    // round trip numbers are float64 and structs are map[string]any.
    Details map[string]any `json:"details,omitempty"` // new
}

// dive
type Reminder struct {
    Name    string
    Tier    ReminderTier
    Content string
    Details map[string]any // new
}

func (r Reminder) WithDetails(details map[string]any) Reminder

const ReminderNameTurnIncomplete = "turn-incomplete"

// NewTurnOutcomeReminder builds the reminder the agent records at the end of
// an incomplete turn. Applications match its Name and read its Details; the
// Content is Dive's wording per reason and may change between releases.
func NewTurnOutcomeReminder(outcome *TurnOutcome) Reminder

// FindTurnOutcome returns the outcome recorded in a message, if any.
func FindTurnOutcome(message *llm.Message) (*TurnOutcome, bool)

// FindLatestTurnOutcome searches from newest to oldest.
func FindLatestTurnOutcome(messages []*llm.Message) (*TurnOutcome, bool)
```

Why a reminder and not a new content type. A dedicated content type would be
cleaner, but `Message.UnmarshalJSON` fails on a content type it does not
know, so a session saved by 1.34 with one incomplete turn could not be opened
by 1.33 at all. Noodle runs on Dive's sessions; a rollback after a deploy
would break every such conversation. A reminder with an extra field decodes
on every existing version, since Go ignores unknown JSON fields, renders
through the reminder path every provider already has, and is covered by the
reminder priming rule in the system prompt. The typed API sits on top:
`FindTurnOutcome` decodes `Details` into a `TurnOutcome`, and
`NewTurnOutcomeReminder` encodes one. One name for every reason keeps
`FindLatestReminder` callers simple; the reason is in the details.

The wording per reason, contextual tier, user role. Dive owns it and may tune
it; the name and the details are the contract. Every variant ends with the
same two sentences about tool results, included only when `ToolCalls` has a
call that is not completed:

> A tool result that begins "Not run:" is a call that never started and had
> no effect. A tool result that begins "Unknown result:" is a call that was
> running when the turn ended; whether it took effect is unknown, so check
> before repeating it.

| Reason                                                          | Wording                                                                                                                                                                                           |
| --------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `canceled`                                                      | The previous turn was stopped by the user before it finished. Everything above this note, including every tool result, happened as shown. Do not repeat completed steps or assume unfinished ones happened. Wait for the user's next instruction rather than resuming the stopped work on your own. |
| `deadline`, `provider_error`, `stream_interrupted`, `callback_error`, `error` | The previous turn failed before it finished. Error: `<error>`. Everything above this note, including every tool result, happened as shown. Continue from the completed work: do not repeat steps that completed, and do not assume steps that did not complete have happened. |
| `hook_abort`                                                    | The previous turn was stopped by the application before it finished: `<error>`. Everything above this note happened as shown. Do not retry the stopped step unless the user asks.                  |
| `output_limit`                                                  | The previous response was cut off at the output limit before it finished. Continue from exactly where it stopped, without repeating what was already written.                                        |
| `context_limit`                                                 | The previous response stopped because the conversation reached the model's context window. Everything above this note happened as shown. Continue from where it stopped, without repeating what was already written. |
| `iteration_limit`                                               | The previous turn reached its limit of tool calls before finishing. Finish with the information already gathered, or ask the user before continuing.                                                 |
| `provider_stopped`                                              | The previous turn ended because the provider stopped the response before it finished (reason: `<reason>`). Any tool call it made was not run. Everything above this note happened as shown. Continue from the completed work; if the same stop repeats, tell the user rather than retrying. |
| `pause`                                                         | The previous turn's server tool loop was paused before it finished. Its trailing call was not completed; start it again if the user still wants it.                                                  |
| `process_exit` (Phase 2)                                        | The previous turn was interrupted: the process ended before it finished. Everything above this note happened as shown.                                                                             |

For a deadline the `<error>` is worded as "the turn's time limit was
reached" while `TurnOutcome.Error` keeps the raw error string.

The reminder is a separate user message rather than a block inside the
tool-result message so that a turn ending in partial text and a turn ending
mid-batch have the same shape. Consecutive user messages are a shape Dive
already sends on every provider (SessionStart seeds ahead of the input,
`InjectContext`, a recorded reminder delivered after a tool-result message),
and Gemini's rule against a request ending in a model turn is satisfied by it.

The cancelled wording tells the model to wait for the user. When the user
does ask to continue, `WithContinue` (section 7) adds a model-only reminder
saying so, so the two never conflict.

### 6. What `CreateResponse` returns

```go
type Response struct {
    // ... Items, OutputMessages, Usage, CreatedAt, FinishedAt, Status, Suspension, BackgroundTasks

    // StopReason is the raw stop reason of the last model response in this
    // invocation ("end_turn", "max_tokens", ...), or "" when there was none.
    // StopDetails carries the provider's structured detail when it reports
    // one, such as a refusal category.
    StopReason  string           `json:"stop_reason,omitempty"`  // new
    StopDetails *llm.StopDetails `json:"stop_details,omitempty"` // new

    // Turn is the turn as this invocation left it. It is set for every
    // status once the turn has begun, including Completed.
    Turn *Turn `json:"turn,omitempty"` // new
}

// Turn is the record of one turn as the last invocation left it: the unit a
// session saves and a stateless caller appends to its history.
type Turn struct {
    // Messages is what a session saves for this invocation, closed so it
    // can be sent again. For a fresh turn it is the input and the output.
    // For a resume it is the whole suspended turn merged with this
    // invocation's output, since the session replaces the suspended event.
    // For a continuation it is this invocation's output alone, since in
    // v1.34 a continuation is saved as its own event; Phase 2 folds it into
    // the turn it continues. It includes the synthetic background-results
    // message, which OutputMessages has never carried.
    Messages []*llm.Message `json:"messages"`

    // Usage is the turn's usage as far as the agent can know it: this
    // invocation's, plus, on a resume, what the suspended turn had already
    // accumulated, which SuspensionState.Usage now carries (session.Session
    // fills it from the suspended event; a session that does not track it
    // leaves it nil, and Usage is then this invocation's). The session is
    // always handed the invocation's own usage and sums on SaveResumedTurn,
    // as today, so TotalUsage never double-counts. Response.Usage is this
    // invocation's alone.
    Usage *llm.Usage `json:"usage,omitempty"`

    // Outcome is set when Status is ResponseStatusIncomplete.
    Outcome *TurnOutcome `json:"outcome,omitempty"`

    // Suspension is set when Status is ResponseStatusSuspended.
    // Response.Suspension mirrors it, and keeps its documented extra role of
    // carrying the merged turn on a completed resume until the next major
    // version; Messages is the same information for every status.
    Suspension *SuspensionState `json:"suspension,omitempty"`

    // Persistence says whether a session recorded this state.
    Persistence PersistenceState `json:"persistence"`
}

type PersistenceState string

const (
    PersistenceNone    PersistenceState = "none"    // no session
    PersistenceSaved   PersistenceState = "saved"   // the store acknowledged the write
    PersistenceFailed  PersistenceState = "failed"  // the store rejected the write before writing anything
    PersistenceUnknown PersistenceState = "unknown" // any other error, or a timeout; the write may have landed
)

// ErrSaveRejected is wrapped by a Session's write when it is refused before
// anything is written, so the agent can report PersistenceFailed. Any error
// that does not wrap it, or one of session.Session's own pre-write
// sentinels (ErrSuspendedSession, ErrNotSuspended), is PersistenceUnknown.
var ErrSaveRejected = errors.New("dive: session rejected the write before writing")
```

`Response.Usage` and `FinishedAt` keep their invocation scope: `FinishedAt`
is when the invocation ended, even when the turn is suspended or incomplete.
`Response.OutputMessages` is the new output of this invocation, which for an
incomplete invocation includes the closing messages. A stateless caller has
one recipe for every status: append `resp.Turn.Messages` to the history it
held before the call, where on a resume "before the call" means before the
suspended turn, since the caller replaces that turn exactly as the session
does, and keep `resp.Suspension` while a turn is suspended for `WithResume`.
That replaces the three recipes in use today, and the background-results
message is no longer lost.

**A save error proves less than it seems.** `Persistence` is what the store
said, not what the store holds. `session.Session` on a `FileStore` can
return an error after `os.Rename` has replaced the file (the parent
directory's sync or close), and an append can fail on `Sync` after its bytes
are written; today `withRollback` then restores the in-memory state, so the
cached session and a fresh `Open` disagree. Hence the rule above: `failed`
only for a write the store rejected before writing, `unknown` for everything
else. After `unknown`, `session.Session` re-reads its state from the store
before it serves another call, instead of trusting its rollback, and a
custom session does the same or documents that it does not. A caller treats
`resp.Turn` as what the agent produced and `Persistence` as whether the
session holds it, and reloads before acting on the session when it is not
`saved`.

**The contract by exit class.** The `Response` is created before
PreGeneration hooks run, and every exit after the boundary goes through one
function that closes the turn, saves, emits the terminal item, and builds the
return. That function reads one accumulator for the turn, fed alike by the
resume phase, the generation loop and Stop-hook continuations, into which
every model response and every tool result is recorded before the callbacks
and hooks that could fail run. Step 3 put the accumulator and the function
in place (`turnRecord` and `turn.end`); tool results of a batch that fails
part way reach the record in step 5.

| Exit                                                                  | Returns                                   | `Status`     | `Persistence`      |
| --------------------------------------------------------------------- | ----------------------------------------- | ------------ | ------------------ |
| Before the turn begins                                                | `(nil, err)`                              |              |                    |
| Completed                                                             | `(resp, nil)`                             | `completed`  | `saved` or `none`  |
| Completed, save failed                                                | `(resp, err)`                             | `completed`  | `failed` or `unknown` |
| Suspended                                                             | `(resp, nil)`                             | `suspended`  | `saved` or `none`  |
| A new suspension whose `SaveSuspendedTurn` failed                     | `(resp, err)` (today `(nil, err)`); the caller reloads, and persists `resp.Suspension` itself only when the store does not hold it | `suspended`  | `failed` or `unknown` |
| Incomplete, error reason                                              | `(resp, err)`, `err` wraps `*GenerationError{Response: resp}` | `incomplete` | `saved`, `none`, or `failed`/`unknown` with `errors.Join` |
| Incomplete, model-stop reason (`output_limit`, `iteration_limit`, `provider_stopped`, `pause`) | `(resp, nil)`    | `incomplete` | `saved` or `none`  |
| Partial resume failed                                                 | `(nil, err)`, `err` wraps `*GenerationError` with the items so far and `Response == nil` | | unchanged |

The error reasons cover every exit that used to `return nil, err` on its own:
a PreGeneration hook error, a PreIteration hook error, a tool resolution
error, a model call error, an event callback error, a hook abort in any tool
hook, in Stop, in PostGeneration or in OnSuspend, and a full-resume failure
before the model call. A callback error raised by the terminal `turn_ended`
item, after the state is persisted, is logged and does not change the
return: the state was committed, and `Persistence` says so.

Returning both values is unusual in Go but not unprecedented (`io.Reader`),
and no existing caller can break: code that checks `err` first ignores the
response, and code that only checks `err` keeps working. `GenerationError`
stays as the compatibility route with its existing fields, which become
views of the same response.

Other fields on an incomplete response: `Items` is everything emitted,
including the synthesized not-run and unknown results, but not the terminal
item, which mirrors the response itself;
`BackgroundTasks` carries the handles of background tasks started before the
end and one handle per unknown call.

### 7. Continuing: `WithContinue`

```go
// WithContinue asks for another invocation of the conversation as recorded,
// with no new input: the model is called on the history as it stands. It is
// how a stopped, failed or cut-off turn is picked up without rerunning any
// tool, since every tool result is already in the record. When the last
// turn is incomplete, or the history ends in an assistant message, the
// agent adds a model-only reminder, "turn-continue", saying the user asked
// to continue from where the turn stopped, so the request never ends in an
// assistant turn, which some models reject as a prefill. Returns ErrResumeRequired on a suspended session, which must be
// resumed first, and an error when there is no history at all.
func WithContinue() CreateResponseOption
```

Session-backed callers pass it alone. Stateless callers already have the
means, `WithMessages(history...)` with a history that ends in the closed
turn, and may add `WithContinue` for the reminder. An input-less call on a
session-backed agent runs today by accident; `WithContinue` makes it a
contract. In v1.34 the continuation is saved as its own event with no input:
`SaveTurn` appends its output and its usage, nothing already saved is
written again, `resp.Turn.Messages` is that output, and the stateless recipe
appends it. Phase 2 folds the continuation into the turn it continues.

This is also the answer to the retried resume that reruns tools: after a
full resume fails, the caller-supplied results and everything the resume ran
are in the closed turn, and `WithContinue` retries the model step alone.

Closing the turn and continuing it leaves a `turn-incomplete` note in
history for every stop, including a transient provider error retried a
minute later. That is accepted for v1.34: the provider's own retries absorb
most transient failures below the loop, the note is small and truthful, and
the `Session` contract has no way to edit a saved turn. Phase 2's open turns
are the refinement for callers who want a continuation to leave no trace.

### 8. Hooks

```go
type Hooks struct {
    // ...
    // OnIncompleteTurn hooks run when a turn ends incomplete, for any
    // reason, after the turn is closed and before it is saved. They can
    // repair the messages that will be saved, change the recorded error,
    // notify an external system, or discard the turn. They do not run for
    // an error exit when IncompleteTurns.Discard is set.
    OnIncompleteTurn []IncompleteTurnHook
}

// IncompleteTurnHook receives hctx.Turn (Outcome mutable), hctx.OutputMessages
// (the closed output without the outcome reminder, mutable), hctx.Usage,
// hctx.Response and hctx.Messages. Regular errors are logged. The context is
// the salvage context: not cancelled, so the hook can do I/O.
type IncompleteTurnHook func(ctx context.Context, hctx *HookContext) (*IncompleteTurnDecision, error)

type IncompleteTurnDecision struct {
    // Discard drops this turn: nothing is saved, as if
    // IncompleteTurns.Discard were set for this call only.
    Discard bool
}

type HookContext struct {
    // ...
    // Turn is the turn as it will be returned and saved. Set for
    // OnIncompleteTurn and for PostGeneration hooks.
    Turn *Turn
}
```

The decision return mirrors `StopHook`. Two uses the hook is for: repairing
a turn whose own content caused the failure (a tool returned an image to a
text-only model; the hook replaces the image with a note so later requests
do not fail the same way), and dropping a turn a policy hook aborted (the
hook reads `hctx.Turn.Outcome.Hook == "PostGeneration"` and the marker it left
in `hctx.Values`, and returns `Discard: true`).

**PostGeneration keeps its scope.** It fires on completed and suspended
turns, as today, and not on incomplete ones; `OnIncompleteTurn` is the end
hook for those. Every invocation therefore ends in exactly one of the two,
and a metrics or usage hook that wants every end registers both. This is the
conservative choice over broadening PostGeneration: existing hooks keep their
meaning, and the incomplete path needs no "an abort here is only logged"
special case. For a turn the model finished, a PostGeneration abort makes
the turn incomplete with `hook_abort`, the model's complete output saved,
and `OnIncompleteTurn` runs. An **OnSuspend** abort is the same kind of
exit: the turn is incomplete with `hook_abort`, the completed siblings keep
their results, and the suspending calls are recorded as `unknown`, answered
with `ToolCallUnknownText`, with `Next == reconcile`. "Never started" cannot
be claimed for them: the tool may have dispatched its request before it
suspended (the suspend-resume guide's own example does), and an earlier
OnSuspend hook may have dispatched before a later one aborted. OnSuspend
hooks run with the salvage context, since a hook that dispatches work must
not fail on a cancellation that is ending the run. Stop hooks never run on
an incomplete turn.

```text
SessionLoad → SessionStart → PreGeneration → [PreIteration → LLM → PreToolUse → Execute → PostToolUse]* → Stop → PostGeneration → SessionSave
   on suspend:      OnSuspend → PostGeneration → SaveSuspendedTurn
   on incomplete:   OnIncompleteTurn → SaveTurn / SaveResumedTurn
```

### 9. Saving

**The salvage context.** Every session write at the end of an invocation,
including the write of a completed or suspended turn, uses
`context.WithoutCancel(ctx)` bounded by `IncompleteTurns.SaveTimeout`
(default 30 seconds). The derived context keeps the run's values (tracing
span, the session-lock marker) and drops its cancellation. `FileStore` and
`MemoryStore` ignore the context; a database-backed `Session` would otherwise
refuse the write with the very cancellation that ended the turn, which is the
trick all three applications had to invent. Making the completed-turn save
use it too closes the save-error row in the table above. The error's
classification is the one in section 6: `failed` only when the session
refused the write before writing, by wrapping `ErrSaveRejected` or with one
of `session.Session`'s pre-write sentinels; any other error, and a write
that hit `SaveTimeout`, is `unknown`, since it may have landed. This holds
for `SaveSuspendedTurn` as much as for `SaveTurn`: the caller reloads
before it retries or persists the returned snapshot, and an error never
implies that nothing was saved.

**Cancellation versus suspension.** A suspension is persisted even when the
context has been cancelled by the time the agent reaches it. A tool that
returns `SuspendResult` has usually dispatched its request already, and an
OnSuspend hook may dispatch before the cancellation is observed; closing the
turn as cancelled with those calls "not run" would be false, and would leave
the external result with no suspended session to land in. So
`finishSuspended` runs its hooks and `SaveSuspendedTurn` with the salvage
context and the invocation returns `Suspended` with a nil error: the
cancellation changed nothing about the outcome. What the application does
with a suspension it no longer wants is its own explicit act, taken under
the lock: `CancelSuspension` today, `CancelSuspendedTurn` in Phase 2, which
records the outcome and answers the pending calls as `unknown`. This
replaces the earlier rule that a suspension is never persisted after a
cancellation, and it settles the confirmed race with A2A's `Cancel` in the
honest direction: the suspension is written, and `Cancel`, taking the lock
after the run's write, removes it. The only remaining need is that lock:

```go
// LockSession takes the per-session lock CreateResponse uses, so a caller
// that changes a session outside the agent (CancelSuspension, a rewind, an
// import) is serialized with any run on it. The returned context marks the
// lock as held, so a CreateResponse made with it fails with
// ErrReentrantSession instead of deadlocking. It returns
// ErrReentrantSession when ctx already holds the lock and ctx.Err() when
// cancelled while waiting.
func LockSession(ctx context.Context, id string) (lockedCtx context.Context, unlock func(), err error)
```

With it, A2A's `Cancel` cancels the run, takes the lock (which waits for the
run's write), then cancels the suspension, in that order.

**Plain sessions.** An incomplete turn is saved with `SaveTurn`, the same call
as a completed turn; the messages carry the outcome reminder, so any
`Session` implementation works unchanged and can read the outcome back with
`FindTurnOutcome`. `session.Session` additionally records
`Metadata["outcome"]` with the reason on the event it writes, whether
`SaveTurn` appends it or `SaveResumedTurn` replaces the suspended event with
a closed full resume, when the turn carries an outcome reminder anywhere in
its messages, next to the existing `"suspended"` metadata. The agent keeps
the outcome reminder last, but the session does not depend on that. The
metadata's consumers are the Phase 2 hidden-turn view and store listings;
Phase 2's checkpoint receives the outcome as a value and needs no scan.

**Resumed turns.** Two cases, split by whether external work is still
outstanding.

A *full* resume, where every pending call has a result, caller-supplied or
produced by rerunning the calls that were not started before the suspension,
that stops at any point after the boundary, before or after the model call,
is closed and written with `SaveResumedTurn` (`rs.TurnMessages` + output +
closing), which replaces the suspended event and clears the suspension. The
caller-supplied results are kept, not-started calls the agent had not reached
are "not run", and rerun calls still executing in a parallel batch are
"unknown". This is a behaviour change: today a failed resume leaves the
session suspended so the resume can be retried, but the retry reruns the
tools and hooks, and the results just supplied are exactly the part worth
keeping. The retry is now `WithContinue`. `Discard` restores the old
behaviour with the rest.

A *partial* resume, where the caller supplied some results and others are
still pending, never calls the model. It can still fail, at four points: a
PostToolUse or PostToolUseFailure hook aborting for a supplied result
(`fireResumePostHooks`), the event callback rejecting the `tool_call_result`
item that announces it, a PostGeneration hook aborting inside
`finishSuspended`, or `SaveSuspendedTurn` failing. None of those closes the
turn: external work is outstanding, and a closed turn could never accept it.
In memory the session is left as it was before the call, with the earlier
suspension and the earlier pending set, and `(nil, err)` is returned, as
today; a new test pins the invariant at each of the four points. The store
may still have taken the write when the failure was `SaveSuspendedTurn`
itself, since a rename can land before a later error, so the session
resyncs from its store and the caller reloads before resubmitting: if the
resync shows the results were accepted, the resubmission returns
`ErrUnknownPendingToolCall` for them, which is the existing signal to move
on. Otherwise the caller still holds the results it just supplied and
resubmits them; a stream consumer sees their `tool_call_result` items again
on the retry, which is the existing behaviour of a failed resume.

One thing a failed attempt does not undo: the PostToolUse and
PostToolUseFailure hooks for the supplied results have already run, and the
retry runs them again. Hook delivery on a partial resume is at-least-once. A
hook with an external side effect deduplicates on the tool-call ID, which is
stable across attempts, and `IdempotentHint` says nothing about hooks.
Saving the supplied results alongside the still-pending calls would need a
session write that half-advances a suspension; Phase 2's revisions provide
exactly that, and narrow the hook window to the gap between a hook's effect
and the checkpoint that follows it.

**Stateless callers** get the closed turn on `Turn.Messages` and append it to
their own history like any other turn.

**Under the session lock.** The whole sequence runs inside the existing
per-session lock, so a concurrent `CreateResponse` on the same session waits
for the salvage write.

### 10. Soft cancel

A stop button and an out-of-budget check want the agent to stop at a clean
point rather than mid-call. nvoken gets this today by returning a sentinel
error from the event callback, which fires from the streaming loop and from
parallel tool goroutines and aborts whatever is in flight.

```go
// WithSoftCancel returns a copy of parent that carries a cancel request.
// Calling cancel asks an agent run using ctx to end at its next step
// boundary: before its next model call, and before each tool call it has not
// started. Tool calls already running finish; calls not started are answered
// with ToolCallNotRunText. The turn ends incomplete with TurnReasonCanceled
// and an error for which errors.Is(err, context.Canceled) holds, exactly as
// a hard cancellation does. A second, ordinary context cancellation still
// stops the run at once.
//
// The request travels with the context, so a subagent started by a tool sees
// it and stops at its own boundary.
func WithSoftCancel(parent context.Context) (ctx context.Context, cancel func())

// SoftCanceled reports whether ctx carries a soft cancel request. A
// long-running tool may check it to wind down early.
func SoftCanceled(ctx context.Context) bool
```

The natural escalation for a UI is one context of each: press stop once for
`softCancel()`, again for `cancel()`. A soft-cancelled turn in which the
model's last message requested tool calls has those calls answered "not
run", which is the outcome the person pressing stop wants: nothing more
happens. Because a running batch is allowed to finish, a soft cancel never
produces an unknown call.

### 11. Encoder backstop

Each provider encoder answers any tool call that has no result, in the
request only, so a session saved by an older version, by an application with a
bug, or by a process that died mid-turn (before Phase 2) cannot block every
request after it:

```go
// llm
// ToolCallNotRunText and ToolCallUnknownText are the texts of the error
// results recorded for a tool call that was never answered. The dive
// constants of the same names are these values.
const (
    ToolCallNotRunText  = "Not run: the turn ended before this call started. It had no effect."
    ToolCallUnknownText = "Unknown result: the turn ended while this call was running. Its result was not recorded; it may have taken effect, so check before repeating it."
)

// AnswerUnansweredToolCalls returns messages in which every tool_use block
// is followed by a tool_result for its ID. A missing result is inserted as
// an error result with ToolCallUnknownText, into the next message when it is
// a tool-result message and otherwise as a new tool-result message after the
// assistant message, so a user message holding no results (a reminder an
// encoder may render in a system or developer role) is never mixed with them. The unknown text is used because history alone cannot
// say whether the call ran, and "unknown" is the safe claim. Server tool
// calls are left alone. Copy-on-write: messages is returned as is when
// nothing is missing.
func AnswerUnansweredToolCalls(messages []*Message) []*Message
```

It lives in `llm` so `dive.CloseTurn` and the encoders share one
implementation (`providers` imports `dive`, so the reverse dependency is not
available). The Anthropic, OpenAI Responses, Chat Completions and Google
encoders call it where they already normalize history, next to
`RenderReminders` and the image lifting. Gemini's opposite check, a tool
result whose call is missing, is out of scope but is the same helper's
natural next rule.

### 12. Options

```go
type AgentOptions struct {
    // ...
    // IncompleteTurns configures what the agent does when a turn stops
    // before it completes.
    IncompleteTurns IncompleteTurnOptions
}

type IncompleteTurnOptions struct {
    // Discard restores the behaviour before v1.34 for the error reasons: an
    // invocation that ends in an error saves nothing, runs no
    // OnIncompleteTurn hook, emits no not-run, unknown or turn_ended items,
    // and leaves a failed resume suspended. CreateResponse still returns the
    // response alongside the error, with the raw partial messages
    // GenerationError carries today and Turn.Persistence none. Turns the
    // model itself stopped short (output limit, iteration limit, provider
    // stop, pause) were saved before v1.34 and still are. For
    // applications that keep incomplete turns themselves and are not ready
    // to remove that code.
    Discard bool

    // DropPartialText leaves out text the model was still writing when the
    // turn stopped.
    DropPartialText bool

    // SaveTimeout bounds the session write at the end of an invocation. The
    // write uses a context without the cancellation that ended the turn.
    // Default 30 seconds.
    SaveTimeout time.Duration
}
```

`Discard` is the whole old behaviour for error exits, not just the save,
because an application that rebuilds turns from the event stream (Noodle
today) would otherwise see Dive's synthesized results next to its own and
record each call twice.

### 13. Streaming items

Every tool call the agent answers as not run or unknown emits a
`tool_call_result` item with `Error == ErrToolCallNotRun` or
`ErrToolCallUnknown`, as `haltToolCall` does for a halted call, so every
`tool_call` item has a result item. Every invocation then ends with one
terminal item:

```go
const (
    // ResponseItemTypeTurnEnded is the terminal item of every invocation:
    // completed, suspended (including a partial resume) or incomplete. The
    // Turn field mirrors Response.Turn, including its persistence state.
    // Stream consumers treat it as end-of-stream.
    ResponseItemTypeTurnEnded ResponseItemType = "turn_ended"
)

type ResponseItem struct {
    // ...
    Turn *Turn `json:"turn,omitempty"`
}
```

`ResponseItemTypeSuspended` is still emitted before `turn_ended` on a
suspension, for existing consumers, and is deprecated. Terminal items are
emitted after persistence, with the salvage context, since the run's context
may be cancelled; a callback error at that point is logged, because the state
is already committed. Under `Discard`, an error exit emits none of these, as
before v1.34.

### 14. Fixes that stand on their own

Each of these is independent of the rest and can ship first:

- **Snapshot copies.** `cloneSuspensionState` copies `TurnMessages` by
  pointer and `cloneCompletedToolCall` shares `Result`; both are documented
  as "treated as immutable" but nothing enforces it. Deep-copy with
  `Message.Copy` and a result clone, as `copyMessages` already does for
  events.
- **Stop reason plumbing.** `Response.StopReason` and
  `llm.ClassifyStopReason`, the precedence fix in the Responses adapter and
  the finish-reason mapping in the Google adapter, its tool-call reasons
  included, and the chat-completions iterator reporting a bare EOF, without
  `[DONE]` or a finish reason, as an error (section 4).
- **Session resync after a write error.** `session.Session` re-reads its
  state from the store after any failed write instead of restoring its
  in-memory copy, since the write may have landed (section 6).
- **`LockSession`** (section 9).
- **The CLI** compares the error with `!=`; it should use `errors.Is`, and
  render the outcome reminder as a transcript marker on resume. It gets
  stop-persistence for free.
- **A2A** maps `incomplete` with `canceled` to `TaskStateCanceled` and other
  incomplete turns to failed with the partial output as an artifact, and its
  `Cancel` takes `LockSession` before `CancelSuspension`.
- **The subagent tool** puts the partial answer and the reason in its result
  instead of "Subagent failed" alone.

### Interface summary

Everything new or changed in v1.34, in one place:

```go
// dive
const ResponseStatusIncomplete ResponseStatus
type  TurnReason string; const TurnReasonCanceled, TurnReasonDeadline, TurnReasonProviderError, TurnReasonStreamInterrupted,
      TurnReasonHookAbort, TurnReasonCallbackError, TurnReasonError, TurnReasonOutputLimit, TurnReasonContextLimit, TurnReasonIterationLimit,
      TurnReasonProviderStopped, TurnReasonPause, TurnReasonProcessExit TurnReason
type  TurnNext string; const TurnNextContinue, TurnNextReconcile, TurnNextInput TurnNext
type  ToolCallState string; const ToolCallStateCompleted, ToolCallStateNotStarted, ToolCallStateUnknown, ToolCallStateWaiting ToolCallState
type  ToolCallRecord struct{ ID, Name string; State ToolCallState }
type  TurnOutcome struct{ Reason TurnReason; Error, Hook string; UsageUnknown bool; ToolCalls []ToolCallRecord; Next TurnNext }
type  Turn struct{ Messages []*llm.Message; Usage *llm.Usage; Outcome *TurnOutcome; Suspension *SuspensionState; Persistence PersistenceState }
type  PersistenceState string; const PersistenceNone, PersistenceSaved, PersistenceFailed, PersistenceUnknown PersistenceState
var   ErrSaveRejected error                                                 // a Session wraps it for a pre-write refusal
type  SuspensionState struct{ ...; Usage *llm.Usage }                       // new field, filled by session.Session
type  Response struct{ ...; StopReason string; StopDetails *llm.StopDetails; Turn *Turn } // new fields
type  GenerationError struct{ ...; Response *Response }                    // new field
const ToolCallNotRunText, ToolCallUnknownText = llm.ToolCallNotRunText, llm.ToolCallUnknownText
var   ErrToolCallNotRun, ErrToolCallUnknown error
func  CloseTurn(turn []*llm.Message, outcome *TurnOutcome) []*llm.Message
const ReminderNameTurnIncomplete = "turn-incomplete"
func  NewTurnOutcomeReminder(outcome *TurnOutcome) Reminder
func  FindTurnOutcome(message *llm.Message) (*TurnOutcome, bool)
func  FindLatestTurnOutcome(messages []*llm.Message) (*TurnOutcome, bool)
type  Reminder struct{ ...; Details map[string]any }; func (Reminder) WithDetails(map[string]any) Reminder
func  WithContinue() CreateResponseOption                                   // model-only reminder "turn-continue"
type  Hooks struct{ ...; OnIncompleteTurn []IncompleteTurnHook }
type  IncompleteTurnHook func(context.Context, *HookContext) (*IncompleteTurnDecision, error)
type  IncompleteTurnDecision struct{ Discard bool }
type  HookContext struct{ ...; Turn *Turn }
type  AgentOptions struct{ ...; IncompleteTurns IncompleteTurnOptions }
type  IncompleteTurnOptions struct{ Discard, DropPartialText bool; SaveTimeout time.Duration }
func  WithSoftCancel(parent context.Context) (context.Context, func())
func  SoftCanceled(ctx context.Context) bool
func  LockSession(ctx context.Context, id string) (context.Context, func(), error)
const ResponseItemTypeTurnEnded ResponseItemType; ResponseItem.Turn *Turn   // ResponseItemTypeSuspended deprecated
// Response.BackgroundTasks also carries one handle per unknown call

// llm
type  ReminderContent struct{ ...; Details map[string]any }
type  StopKind string; func ClassifyStopReason(reason string) StopKind   // finished, tool_use, output_limit, context_limit, refusal, pause, incomplete, other
const ToolCallNotRunText, ToolCallUnknownText string
func  AnswerUnansweredToolCalls(messages []*Message) []*Message
// ResponseAccumulator: a way to get the partial response with per-block completeness

// providers: each encoder calls llm.AnswerUnansweredToolCalls; the
// chat-completions iterator reports a bare EOF, without [DONE] or a finish reason, as an error
// session: SaveTurn and SaveResumedTurn record Metadata["outcome"] when the turn carries an outcome
// reminder anywhere; LoadSuspension fills SuspensionState.Usage; the session resyncs from its store after
// a failed write; cloneSuspensionState and cloneCompletedToolCall deep-copy
```

### The cases

From the proposal and the reviews:

- **A failure caused by the turn's own content.** Kept. `OnIncompleteTurn`
  repairs it (section 8). If the application does nothing, the next turn's
  failure is itself recorded, so the person sees the same error twice rather
  than a silent loop.
- **A failure caused by earlier history.** Saving this turn changes nothing
  about the earlier history. The encoder backstop removes one class of it,
  unanswered calls; Anthropic's empty text blocks were fixed in 1.33.1.
- **Parallel tool execution.** Results drained before the end are kept with
  their hooks applied. Results that finished but were still in the channel
  are picked up with a non-blocking drain and kept, without PostToolUse
  hooks, which would otherwise run with a cancelled context; documented.
  Calls still running are answered "unknown", never "not run", read-only
  tools included; their late results come back as handles for the
  application to deliver or drop. Sequential execution waits for the running
  call, as today, so it records that call's own result, and only the calls
  after it are "not run".
- **A tool that ignores cancellation and commits after the save.** Its call
  is recorded as unknown, the reminder tells the model to check before
  repeating it, and its handle delivers the commit's real result for the next
  invocation.
- **`max_tokens` inside a tool call.** The tool is never run. The truncated
  block is dropped from the message, a complete block is answered "not run",
  and the turn is incomplete with `output_limit`.
- **`pause_turn`.** Continued up to ten times inside the invocation; beyond
  that, incomplete with `pause` and the trailing server tool call dropped.
- **The iteration limit with a model that ignores `tool_choice: none`.** The
  requested calls are not run; incomplete with `iteration_limit`.
- **A stream that ends at bare EOF.** Incomplete with `stream_interrupted`,
  partial text kept, `err` non-nil, `UsageUnknown` when no usage frame came.
  A `[DONE]` without a finish reason is a normal end.
- **A refusal.** Completed, with `Response.StopReason` and `StopDetails` for
  the application to route on; any tool call it made is answered "not run"
  and no reminder is recorded.
- **Suspension.** A suspended turn is not incomplete; it is paused with
  external work outstanding, and stays as it is. A full resume that stops is
  closed (section 9). A cancellation that arrives while the agent is
  suspending does not stop the suspension from being persisted, since the
  work may be dispatched. Closing a suspended turn without a model call is
  Phase 2.
- **Mid-turn compaction.** The saved turn comes from the output accumulator,
  never from the compacted working set, as for a completed turn.
- **Hook aborts.** `TurnOutcome.Hook` names the hook type and `Error` carries
  `HookAbortError.Error()`, which includes the reason and the cause. Stop,
  PostGeneration and OnSuspend aborts save the output that existed.
- **A turn that stops before the first model call returns.** The input and
  the outcome are saved; `OutputMessages` is just the outcome reminder. A
  PreGeneration hook that rejects input therefore records the rejected input
  with `hook_abort`, which is the right record; an application that would
  rather not have it can validate before calling `CreateResponse` or discard
  from `OnIncompleteTurn`.
- **Cancellation during the final save of a completed turn.** The salvage
  context makes the save succeed and the turn is completed, not lost.
- **Background tasks.** Their "started" result is already in history; the
  handles come back on the response, and the synthetic results message of
  the next invocation is in `Turn.Messages` for stateless callers.

### Compatibility

What changes for an application that upgrades and changes nothing:

1. Turns that stop with an error are saved to its session, closed and with
   an outcome reminder, and the next invocation sends them. If the
   application also saves them itself, it now has duplicates: set
   `IncompleteTurns.Discard` until that code is removed. The changelog entry
   says this in its first line.
2. `CreateResponse` returns a non-nil response with the error. Code that
   checks `err` first is unaffected.
3. A response the model cut at the output limit or the context window, an
   exhausted pause, and the iteration limit now return `Status == Incomplete` with `err == nil`, and
   their tool calls are not run. A refusal stays completed, and its tool
   calls are not run either; so are the calls of a response whose stop
   reason Dive does not recognize, which is incomplete with
   `provider_stopped`. Code that treated every nil error as a finished
   answer sees the same text it saw before, plus a status it can check.
4. `GenerationError.OutputMessages` is the closed output, not the raw partial
   messages. Code that answered open calls itself from it would now answer
   them twice; `Discard` restores the raw messages.
5. A failed full resume no longer leaves the session suspended.
6. Session writes at the end of an invocation use an uncancelled context
   bounded by a timeout. A custom session that relied on the cancellation to
   skip the write will now write.
7. Event callbacks receive not-run and unknown result items and a
   `turn_ended` item for every invocation, with an uncancelled context.
8. A chat-completions stream that ends at bare EOF, without `[DONE]` or a
   finish reason, is an error where it was a clean stop.
9. A suspension whose save failed returns the response with the error where
   it returned `(nil, err)`.

PostGeneration hooks are not on this list: they keep firing on completed and
suspended turns only. Items 2, 3, 7's `turn_ended` on completed turns, 8 and
9 are what `Discard` does not undo; none of them can break a caller that
checks `err` first or switches on known statuses.

What each of the three applications does after upgrading:

- **Noodle** deletes `partial.go`, the save branch in `runturn.go`, and the
  error separator; reads `resp.Turn.Outcome` for the notice, and
  `dive.FindTurnOutcome` per message when rebuilding a transcript from the
  session; wires the stop button to `WithSoftCancel` then `WithCancel`, and a
  continue button to `WithContinue`.
- **mobius-cloud** has no Dive session and keeps its own tables. It reads
  `resp.Turn` instead of settling the turn from the event stream, or does the
  same from `OnIncompleteTurn` with `Discard: true` as a no-op. Whether a
  cancelled turn is sent on the next request is its own history's choice, as
  before; the cancelled wording, which tells the model to wait for the user,
  is meant to make sending it safe.
- **nvoken-cloud** is stateless. It replaces its callback sentinel with
  `WithSoftCancel`, sets `DropPartialText`, and appends `resp.Turn.Messages`
  to its history the same way for every status.

### Tests

- Cancelled mid-batch, one call finished and one not. Sequential: the saved
  messages are the input, the assistant message, a tool-result message with
  the finished result and a not-run error for the call that was never
  started, then the `turn-incomplete` reminder whose details decode to a
  `TurnOutcome` with `canceled`, that call `not_started`, and `Next == input`.
  Parallel, with the second tool still running: the same shape with an
  unknown-result error, the call `unknown`, `Next == reconcile`, and a
  handle for it on `Response.BackgroundTasks`; a read-only tool in the same
  position is `unknown` too, with `Next` left at `input`. Both histories
  encode without error on the
  Anthropic, OpenAI Responses, Gemini and Chat Completions encoders.
- A parallel tool that ignores cancellation and commits after the turn is
  saved: recorded as unknown, never as not run; the reminder does not say it
  had no effect; its handle delivers the commit's result once the tool
  returns; `WithBackgroundResults` on the next invocation shows a scripted
  model the real result and fires `PostBackgroundToolUse`. Two unknown calls
  with the same description: the completion reminder names each by its
  tool-use ID, and a panic in one arrives as an error result.
- Cancelled while text is streaming: the partial text is the last assistant
  message; with `DropPartialText` it is absent; a half-written `tool_use`
  block and an unsigned thinking block are dropped.
- Third model call fails with a provider error: the first two iterations'
  messages and results are saved, then the reminder with `provider_error`
  and the error text; `(resp, err)` with `Status == Incomplete`, `Next ==
  continue`; `errors.As` yields a `*GenerationError` whose `Response` is the
  same; `WithContinue` then calls the model once and runs no tool.
- A final answer cut at `max_tokens`: `Incomplete`, `output_limit`, `err ==
  nil`, `Response.StopReason == "max_tokens"`, the reminder recorded;
  `WithContinue` continues with the `turn-continue` model-only reminder
  present in the request and absent from the session.
- A response stopped with `model_context_window_exceeded`: `Incomplete`,
  `context_limit`, `err == nil`, `Next == input`, the partial answer and the
  reminder recorded, no tool run; after the test compacts the session,
  `WithContinue` sends the shortened history.
- `max_tokens` inside a `tool_use`: the tool never runs; the truncated block
  is absent from the saved message; a complete sibling is answered "not
  run".
- A refusal with a tool call present: no tool runs, the call is answered
  "not run", `Status == Completed`, `Response.StopReason` is the refusal
  value, `StopDetails` is carried, and no reminder is recorded.
- Adapter precedence: a Responses API response carrying a call with
  `incomplete`/`max_output_tokens` is reported as `max_tokens` and the call
  is not run; the same for a Gemini `SAFETY` finish with a function call
  (completed, call not run) and a chat-completions `length` with
  `tool_calls`; a Responses `incomplete` without a named reason gives
  `provider_stopped` with no call run; a Gemini `UNEXPECTED_TOOL_CALL` with
  a function call is `provider_stopped` with the call not run.
- An unrecognized stop reason: a scripted model returning `some_new_reason`
  with a client tool call ends `Incomplete` with `provider_stopped`,
  `Error == "some_new_reason"`, the call answered "not run" and the tool
  never invoked; the same value with text only is `Completed` with
  `Response.StopReason == "some_new_reason"` and no reminder recorded.
- `pause_turn`: a scripted model that pauses twice then finishes is called
  three times with no user message between; one that pauses eleven times
  ends `Incomplete` with `pause` and no trailing server tool call in the
  saved message.
- Iteration limit with a model that keeps requesting tools: the calls are not
  run, `iteration_limit`, and a continuation gives the model more
  iterations.
- Chat-completions stream ending at bare EOF: `stream_interrupted`, partial
  text kept, `err` non-nil, `UsageUnknown` set when no usage frame arrived;
  `[DONE]` without a finish reason is a normal end.
- Hook aborts in PreToolUse, Stop, PostGeneration and OnSuspend:
  `Outcome.Hook` names the hook; Stop and PostGeneration save the model's
  complete output; OnSuspend saves the completed siblings, records the
  suspending call as `unknown`, and the session is not suspended.
- A cancellation arriving while the agent is suspending, after the tool
  recorded that it dispatched its request: the suspension is persisted, the
  response is `Suspended` with a nil error, the OnSuspend hooks saw an
  uncancelled context, and `WithToolResults` later succeeds.
- PreGeneration hook error: the input and the outcome are saved, nothing
  else.
- Soft cancel requested during a tool batch and during a model call: the
  running call finishes, later calls are not started, no further model call
  is made, `canceled`, `errors.Is(err, context.Canceled)`, no unknown calls.
- Full resume cancelled after all pending calls were supplied: the session
  is no longer suspended, the caller-supplied results are in the saved turn,
  and `resp.Turn.Messages` is the closed turn. The existing
  `TestResumeContextCancelMidExecution` is this case and its expectation
  flips; its old expectation moves under `IncompleteTurns.Discard`.
- A partial resume failing at each of its four points: the cached session's
  suspension and pending set are unchanged, `resp == nil`, and resubmitting
  the same results succeeds. With a failure injected after the rename in
  `SaveSuspendedTurn`: a fresh `Open` shows the advanced pending set, the
  cached session resyncs to match, and the resubmission returns
  `ErrUnknownPendingToolCall`. A side-effecting PostToolUse hook records
  its effect on the failed attempt and again on the retry: at-least-once, as
  documented.
- Two continuations after a stopped turn: the session holds three events
  with no message saved twice, `TotalUsage` is the sum of the three
  invocations, and each continuation's `resp.Turn.Usage` is its own. A full
  resume: `resp.Turn.Usage` equals the suspended event's usage plus the
  resume's, and `TotalUsage` equals the same.
- `(resp, err)`, `errors.As`, `genErr.Response == resp` and the status on
  every exit class: PreGeneration error, PreIteration error, tool resolution
  error, model error, event callback error, hook aborts in PreToolUse,
  PostToolUse, Stop, PostGeneration and OnSuspend, a full-resume failure
  before the model call, and a salvage save failure with `Persistence`
  `failed` or `unknown` by the error.
- A completed turn whose save fails: `(resp, err)`, `Status == Completed`,
  `Persistence == failed` for a pre-write rejection and `unknown` for any
  other error; the same for a suspension whose save fails. For each of
  `SaveTurn`, `SaveSuspendedTurn` and `SaveResumedTurn`, a failure injected
  before the write leaves a fresh `Open` unchanged, and one injected after
  the rename or the append shows the new state on a fresh `Open` and on the
  resynced cached session; the closed full resume's replacement event
  carries `Metadata["outcome"]`.
- `IncompleteTurns.Discard`: nothing saved, no not-run, unknown or
  `turn_ended` items on an error exit, the session stays suspended on a
  failed resume; a `max_tokens` answer is still saved. The existing
  `TestGenerationErrorExposesPartialWork` becomes this test with the option
  set, and a new version without it asserts the save.
- `OnIncompleteTurn`: rewriting `hctx.OutputMessages` changes what is saved;
  `Discard: true` saves nothing; the hook sees an uncancelled context; a
  reminder it records lands before the outcome reminder, which stays the
  last message, and the event still carries `Metadata["outcome"]`.
- `turn_ended` is the last item of every invocation, including a partial
  resume, and a callback error on it does not change the return.
- Resumed session after each of the above: the next `CreateResponse` sends
  the saved turn, and a scripted model receives the reminder.
- Stateless: `preTurn + resp.Turn.Messages` is the history a session would
  hold for every status, including one with background results.
- Encoder backstop: a history with an unanswered call in the middle and one
  at the tail encodes on all four encoders with the inserted unknown-result
  block, and the caller's messages are unchanged.
- Salvage context: a session whose `SaveTurn` returns `ctx.Err()` still
  saves; a session that wraps `ErrSaveRejected` gives `Persistence ==
  failed`; one that returns any other error, or blocks until `SaveTimeout`,
  gives `unknown` with the error joined.
- Snapshot isolation: mutating a returned `SuspensionState` or its messages
  does not change the session.
- Reminder details round-trip through `Message.Copy` and `FileStore`, and a
  reminder with details decodes as a plain reminder when `Details` is
  ignored, the downgrade case.

### Documentation and changelog

`docs/guides/agents.md` gains an "Incomplete turns" section: statuses and
reasons, `Response.Turn`, `WithContinue`, `IncompleteTurns`, soft cancel,
reading outcomes from history. `docs/guides/hooks.md` adds `OnIncompleteTurn`
and the new flow. `docs/guides/suspend-resume.md` documents the
cancelled-resume rule and the cancel-versus-suspend ordering.
`docs/harness-features.md` replaces the "Partial-work recovery" bullet and
adds stop-reason handling. `GenerationError`'s doc comment loses its
"intentionally NOT persisted" paragraph. `CLAUDE.md`'s hook flow gains the
incomplete line.

Changelog, under Changed:

> **Turns that stop before they finish are saved and reported.** A cancelled
> or failed turn is recorded, closed so it can be sent again, with a
> `turn-incomplete` reminder saying why; `CreateResponse` returns the
> `Response` with the error, and `Response.Turn` is the turn as saved. A
> response cut at `max_tokens` or the iteration limit now returns
> `ResponseStatusIncomplete`, and neither it, a refusal nor an unrecognized
> stop reason runs a tool. Set `AgentOptions.IncompleteTurns.Discard` to keep
> the old error behaviour, or if your application saves incomplete turns
> itself.

Under Added: `Response.Turn`, `Response.StopReason` and `StopDetails`, `SuspensionState.Usage`, `ErrSaveRejected`, `WithContinue`,
`WithSoftCancel`, `OnIncompleteTurn`, `TurnOutcome` and `FindTurnOutcome`,
`Reminder.Details`, `LockSession`, `turn_ended` items,
`llm.AnswerUnansweredToolCalls` in every encoder, `llm.ClassifyStopReason`.

### Order of work

Six steps, each a pull request that leaves `main` consistent. The exit-path
refactor is its own step because `CreateResponse` ends in a dozen places and
keeps its state in five slices; changing that and the behaviour at once
would make neither reviewable.

1. **Standalone fixes** (section 14). Done: deep-copied suspension
   snapshots, `LockSession` and its use in A2A's `Cancel`, `session.Session`
   resyncing from its store after a failed write (healing a torn append on
   the way), and the CLI's `errors.Is`. `ErrSaveRejected` moves to step 6,
   where `Persistence` first reads it.
2. **Model boundary plumbing** (sections 4 and 11). Done:
   `llm.StopKind` and `ClassifyStopReason` with the spelling table of every
   adapter family (Anthropic and Ollama, Responses and Grok and Meta, Chat
   Completions and Mistral, OpenRouter and DeepInfra, Gemini), the
   Responses precedence fix, Gemini finish reasons kept distinct,
   `Response.StopReason` and `StopDetails`,
   `ResponseAccumulator.UnfinishedContent`, and
   `llm.AnswerUnansweredToolCalls` with the two texts in all four encoders.
   Chat Completions reports a bare EOF as `io.ErrUnexpectedEOF`; a stream
   that ends at `[DONE]` with no `finish_reason` reports `tool_use` or
   `stop` by whether it made calls, and non-streaming `Generate`, which set
   no stop reason at all, reports one. Left for step 6: the Anthropic and
   Responses iterators end without an error when the transport closes
   before their terminal event, so the agent treats a stream whose
   accumulator never saw `message_stop` as interrupted. The Gemini iterator
   keeps closing a stream that ends without a finish reason cleanly, as #271
   chose (decided on 25 September): its stop reason is
   `other`, so the text is kept and no tool call in it runs.
3. **Exit-path refactor.** Done: one turn record (`turnRecord` in
   `turn.go`) fed by the resume phase, the generation loop and Stop-hook
   continuations, into which each model response, its usage and stop reason
   are recorded before the callback that could fail; the `Response` created
   before PreGeneration; every exit after the boundary through `turn.end`,
   which builds the `*GenerationError` from the record, and finishes a
   completed or suspended turn. The existing tests pass unmodified, and new
   tests pinning each exit's return pass on the old code and the new. The
   one visible difference is that `Response.CreatedAt` now precedes the
   PreGeneration hooks. Found on the way: tool calls the resume phase runs
   did not report their background task handles, so
   `Response.BackgroundTasks` missed a background task a resumed call
   started; step 4 records each handle in the turn record as its task
   starts, which fixes that and keeps handles on an error exit.
4. **The envelope** (sections 1, 6 and 13). Done:
   `ResponseStatusIncomplete`, `Turn`, `TurnOutcome`, `TurnReason`,
   `TurnNext`, `ToolCallRecord`, `PersistenceState`, `Response.Turn`,
   `GenerationError.Response` and `turn_ended` (`outcome.go`, `turn.go`).
   Every error exit after the boundary returns `(resp, err)` with the error
   wrapped in a `*GenerationError`, classified from the error chain and from
   the errors the turn record saw the event callback and the model call
   return: `canceled`, `deadline`, `hook_abort`, `callback_error`,
   `provider_error`, `stream_interrupted` (a stream that had delivered an
   event, or `io.ErrUnexpectedEOF`) and `error`. Nothing new is saved, so
   an incomplete turn reports `Persistence == none` and its messages are
   not closed yet. Decided while building it:
   - A completed or suspended turn whose save fails returns `(resp, err)`
     with `Persistence == unknown`; `failed` and `ErrSaveRejected` come with
     step 6.
   - `turn_ended` is not added to `Response.Items`, like the suspended item
     before it: the response already is what the item mirrors. A callback
     error on either terminal item is logged.
   - A failed partial resume returns `(nil, err)` at all of its failure
     points, with `err` wrapping a `*GenerationError` whose `Response` is nil,
     and emits no `turn_ended`, since the turn has not ended.
   - An `OnSuspend` or PostGeneration abort on a new suspension is
     incomplete with `hook_abort`; the response carries no suspension, since
     nothing recorded it.
   - Reasons from model stops (`output_limit` and the rest), `ToolCalls`,
     `reconcile` and `UsageUnknown` arrive with steps 5 and 6, and their
     constants with them.
5. **Tool batch outcomes** (section 3, rule 3, and section 10). Done: both
   batch paths return the partial batch with a state per call
   (`toolbatch.go`), the non-blocking drain, the unknown-call handles and
   their forwarder, the not-run and unknown items, `TurnOutcome.ToolCalls`
   with `reconcile`, and `WithSoftCancel` (`softcancel.go`). A parallel
   goroutine always sends its tool's own result, including an error the tool
   returned because the batch context was cancelled, and the drain records
   it rather than aborting the batch. The background completion reminder
   names each result's tool-use ID. Decided while building it:
   - The answers go into the output now, as the stopped batch's
     `tool_result` message (appended to the merged message on a resume), so
     `Turn.Messages` is a history a provider accepts before `CloseTurn`
     exists; step 6 adds the outcome reminder and saves it.
   - The items a stopped batch still owes (a `tool_call` never announced, a
     `tool_call_result` not delivered) are emitted by the exit on a context
     that is not cancelled, and a callback error on them is logged, so it
     cannot replace the error that ended the turn.
   - A parallel call is started by the goroutine that wins its state from
     pending; a batch that stops first abandons it, so a call launched but
     not yet running is recorded as not started, never unknown.
   - The drain emits through a serialized `emit` that reports whether the
     item reached the callback: the gate that suppresses late stream events
     from tool goroutines also dropped result items once the context ended.
   - A soft cancel is checked at the top of every generation iteration and
     again just before its model call (PreIteration hooks can wait on a
     person), before each sequential call and again after its PreToolUse hooks (which
     can wait on a person), and before each parallel call is prepared or
     launched. Open question 1 is taken as recommended.
   - No grace period (open question 3): a parallel tool that honours its
     context is recorded as unknown or with its own cancellation result,
     depending on whether its result landed before the drain stopped.
   - An `OnSuspend` abort's suspending calls stay out of `ToolCalls` until
     step 6, which answers them in the partial `tool_result` message.
6. **Keeping the turn** (sections 3, 5, 7 to 9 and 12). Done, in one pull
   request: `CloseTurn`, `Reminder.Details` and the outcome reminder
   (`closeturn.go`), saving on every exit with the salvage context,
   `ErrSaveRejected` and `PersistenceFailed`, `OnIncompleteTurn`,
   `IncompleteTurns`, closing a failed full resume, persisting a suspension
   after a cancellation, the output-limit, context-limit, refusal,
   provider-stop, pause and iteration-limit rules, partial streamed text and
   the stream that ends without `message_stop`, `WithContinue`,
   `Metadata["outcome"]`, `SuspensionState.Usage`, the A2A mapping and the
   subagent tool's partial answer from section 14, and the guides. Decided
   while building it:
   - `CloseTurn` modifies neither argument. The outcome it records, with a
     `not_started` record for each call it answered, is read back with
     `FindTurnOutcome`. The agent closes the output and, on a resume, the
     suspended turn; a fresh turn's input messages are left alone, since a
     stateless caller's input can carry its whole history.
   - A call the close answers gets a `tool_call_result` item, preceded by a
     `tool_call` item when it was never announced.
   - The calls of a response that will not run them (a limit, a refusal, a
     provider stop, the iteration limit) are answered in the loop, with
     their items, as a stopped batch's are. A response left with nothing
     once its truncated calls are dropped is not recorded.
   - A pause is resent without spending a tool iteration;
     `HookContext.Iteration` still counts every model call. A paused
     response that carries a client tool call is `provider_stopped`, since
     answering the call would break the pause protocol.
   - In a partial streamed message, a tool call still streaming is dropped
     whatever its input, and `DropPartialText` drops only the text blocks
     still streaming. `UsageUnknown` is set only when a stream started and
     reported no usage.
   - Under `Discard` the output is left as the error found it: a stopped
     batch's answers and the partial text are not added either.
   - `WithContinue` rejects input on a session and any resume with
     `ErrContinueWithInput`, treats a stateless caller's messages as
     history, and skips SessionStart hooks. The `turn-continue` name is
     exported as `ReminderNameTurnContinue`.
   - A suspension also beats a cancellation inside a sequential batch: the
     suspending call's result is reported before the context is checked.
     A PostGeneration abort on a new suspension is handled like an
     OnSuspend abort.
   - `SaveTimeout` bounds the write through its context; a session that
     ignores its context is not interrupted.
   - `TurnReasonProcessExit` and `ToolCallStateWaiting` are left to Phase 2,
     which sets them. The CLI's transcript marker (section 14) is not done.

Phase 2a and 2b below are the seventh and eighth stages.

## Phase 2: recoverable turns, then per-step durability

v1.34 is conversation preservation: the record is stored as messages, once
per invocation, and the projection is fixed at save time. Two limits follow.
A process that dies mid-invocation loses the invocation, and a continuation
leaves a note in history because the record and the projection are the same
thing. Phase 2 removes both in two steps that keep the default storage cost
where it is: first a typed, versioned turn record persisted at the same
invocation boundaries (turn recovery), then per-step checkpoints and
execution ownership as opt-in capabilities (durability). The existing
`Session` and `SuspendableSession` remain as compatibility adapters, and the
legacy path is documented as conversation preservation, never as turn
recovery.

### 2a. Recoverable turns

**The turn record.** `Turn` grows in place, additively:

```go
type Turn struct {
    // v1.34 fields: Messages, Usage, Outcome, Suspension, Persistence

    // Schema versions the stored record.
    Schema int `json:"schema,omitempty"`

    // ID identifies the logical turn across invocations. Revision is the
    // session revision at which the record was last checkpointed.
    ID       string `json:"id,omitempty"`
    Revision uint64 `json:"revision,omitempty"`

    // Origin says what started the turn: new input, a continuation, or
    // delivered background results, with a link to the turn they came from.
    Origin *TurnOrigin `json:"origin,omitempty"`

    // Status of the turn: running (an invocation is advancing it; never
    // returned by CreateResponse), suspended, incomplete, or completed.
    Status ResponseStatus `json:"status,omitempty"`

    // ToolCalls is the execution state of every call in the turn, not only
    // the batch in flight: not_started → running → completed | waiting | unknown.
    ToolCalls []ToolCallRecord `json:"tool_calls,omitempty"`

    // Superseded is set when new input arrived while the turn was
    // incomplete: the turn was abandoned, its record intact.
    Superseded bool `json:"superseded,omitempty"`
}
```

`ResponseStatus` gains `ResponseStatusRunning` for the record only.
`ToolCallState` gains `running`; `not_started`, `unknown` and `waiting` keep
their v1.34 meaning. `dive.TurnIDFromContext(ctx)` exposes the turn ID to a
tool next to the existing tool-call ID, so a tool or service that supports
idempotency keys has a stable execution key; Dive's part ends at exposing
it, and no annotation turns into a retry guarantee on its own.

**The session extension.**

```go
// TurnStore is an optional Session extension that stores turn records. Load
// takes a context and returns an error, unlike LoadSuspension, so a remote
// store can implement it.
type TurnStore interface {
    Session

    // Load returns the history needed to build context, the open turn if
    // any (suspended or incomplete), and the session revision a later
    // checkpoint must carry.
    Load(ctx context.Context) (*SessionSnapshot, error)

    // CheckpointTurn records the turn's state and returns the new session
    // revision. It fails with ErrRevisionConflict when the stored revision
    // is not expectedRevision, and the caller reloads. The revision is the
    // session's, advanced by every write (a checkpoint, a compaction, a
    // rewind), so a checkpoint fails when anything changed since the load.
    CheckpointTurn(ctx context.Context, expectedRevision uint64, turn *Turn) (uint64, error)
}

type SessionSnapshot struct {
    History  []*llm.Message // completed turns, projected
    OpenTurn *Turn          // nil when the last turn completed
    Revision uint64
}
```

By default the agent checkpoints at the same boundaries as v1.34: once, when
the invocation completes, suspends or stops. What changes is what is
stored. `Messages()` on such a session is the projection: completed turns as
saved, plus the open turn passed through `CloseTurn` with its current state.
This is where the record and the projection separate: an incomplete draft
stays available for display, completed results stay durable, what the model
sees of an unfinished turn is computed on load by an explicit policy rather
than fixed at save time, and the placeholder results for unanswered calls
exist only in the projection. A continuation of an open turn can therefore
leave no note in history: the outcome lives on the record, and the
projection for an invocation that continues the turn omits it.

**Resume against a revision.** A session-backed resume names the turn and
the revision it expects, plus the new results; a stale revision is a
conflict and the caller reloads:

```go
type ResumeRequest struct {
    TurnID           string
    ExpectedRevision uint64
    ToolResults      map[string]*ToolResult
}
```

A supplied result is accepted, its post-tool hooks run, and it is
checkpointed, in that order, so a partial resume that fails after the
checkpoint keeps the result, which v1.34 cannot do, and resubmitting an
already-accepted result is idempotent: the agent sees it checkpointed, runs
no hook twice and emits no item twice. The window between a hook's effect
and its checkpoint remains: a failure inside it means the hook runs again on
the retry, the same at-least-once contract as v1.34, narrowed to that gap.
A conflicting second result for the same call fails clearly. Importing a caller-held `Turn` into a session, the
stateless cross-process handoff, becomes an explicit operation with the
same conflict check, instead of `WithResume` silently replacing the stored
state. A stale revision also covers the single-process case, where two
callers hold snapshots of one session.

**Turn boundaries.** With turn identity, the rules become part of the
contract:

| Trigger                                | Turn                                                                                   |
| -------------------------------------- | -------------------------------------------------------------------------------------- |
| New input                              | starts a new turn; an open incomplete turn is marked superseded, its record intact     |
| Supplied results for a suspended turn  | continues the turn                                                                     |
| `WithContinue`                         | continues the turn                                                                     |
| A Stop-hook continuation               | continues the turn, inside the invocation                                              |
| Delivered background results           | starts a new turn whose origin links to the turn that started the task                 |

New input never silently supersedes a suspended turn
(`ErrInputOnSuspendedSession` stands), and with the opt-in strict mode never
supersedes a turn with unknown calls (`ErrUnreconciledToolCalls`); by default
the unknown calls are projected with their placeholder results and the
reminder, as in v1.34.

**Closing, deleting, forking, hiding.**

- **`Agent.CancelSuspendedTurn(ctx, opts...)`** closes a suspended turn
  without a model call: the pending calls are recorded as `unknown`, since
  their requests may be out in the world, the completed siblings keep their
  results, a `canceled` outcome is recorded with `Next == reconcile`, and
  the session is no longer suspended. It returns `Incomplete` with a nil
  error, the one place a cancelled turn comes without one, because nothing
  failed. Stateless callers pass `WithMessages` and `WithResume(state, nil)`
  and read the closed turn from `Response.Turn.Messages`.
- **`session.Session.RemoveLastTurn(ctx)`** is the deletion, separately
  named. `CancelSuspension` is deprecated as its alias. Removing a turn from
  storage and cancelling it are different operations, and the Fork doc's
  promise of a rollback becomes true.
- **`Fork`** copies up to the last completed turn by default; copying an
  open turn requires an explicit option that says what to do with waiting
  and unknown calls.
- **Hidden turns.** mobius-cloud leaves a cancelled turn out of the next
  request so the model does not answer a request the person moved away
  from. In v1.34 the cancelled wording covers that. On a `TurnStore`,
  hiding is a projection policy: `Messages` skips superseded turns when the
  policy is on, and the record keeps them. Deferred until an application on
  Dive's own sessions asks for it.

### 2b. Per-step durability and execution ownership

Opt-in capabilities for applications that need recovery from a process
crash, or that run several processes against one session:

- **Per-step checkpoints.** The agent checkpoints inside the invocation: the
  input and identity before the first model call; a completed model
  response before its tools run; each tool call marked `running` before the
  tool is called; each result as it arrives, parallel results independently;
  and the final status. On load, a call recorded `running` is potentially
  executed: it becomes `unknown`, read-only tools included, the turn is
  closed with `process_exit`, and `Next` is `reconcile` unless every unknown
  call is read-only. The window
  between a tool's external effect and the checkpoint of its result remains;
  it stays honest through the unknown state and the turn ID as an
  idempotency key. A store that supports this appends step records rather
  than rewriting the session: `FileStore` adds JSONL line types for them, so
  the hot path stays a single appended line, and rewriting the whole file per
  step is never the default.
- **Execution ownership.** A revision check rejects a stale commit; it does
  not stop two processes from running tools before one of them loses the
  write race. Multi-process execution needs an ownership claim or lease on
  the session before work begins, exposed as a capability of the store and
  documented apart from the in-process lock's guarantee.

## A later breaking iteration

When a major version is on the table, the outcome can move from the edges of
the contracts into them. Candidates, each independent:

- **`CreateResponse` reports how the turn ended through the response
  alone.** A non-nil error means the call did not run: validation, a session
  that could not load, a resume precondition. Everything after the turn
  begins is `Response.Status` with `Response.Turn`, including a provider
  error, the way `http.Client.Do` returns a response for a 500.
  `GenerationError` and `IncompleteTurns.Discard` go away; the hook covers
  the remaining cases.
- **`Session` is the turn store.** `Load` and `CheckpointTurn` replace
  `Messages`, `SaveTurn`, `SaveSuspendedTurn`, `SaveResumedTurn` and
  `LoadSuspension`; `SuspendableSession` folds in.
- **The options say what they mean.** `WithMessages` is always new input.
  A resume reads the pre-turn history from the session or from an explicit
  `WithHistory`, and never changes the meaning of `WithMessages`.
- **One terminal item.** `ResponseItemTypeSuspended` is removed;
  `turn_ended` is the only end-of-stream signal. `Response.Suspension` loses
  its completed-resume role; `Turn.Messages` is the one place for the merged
  turn.
- **The outcome as its own content type.** Once every deployed version can
  decode it, `llm.TurnOutcomeContent` replaces the reminder-with-details
  encoding, and `Message.UnmarshalJSON` learns to skip unknown block types the
  way `Response.UnmarshalJSON` already does, so the next new type is not a
  rollback hazard either.

## Decisions taken across the proposal and the reviews

| Suggested                                                  | Here                                                                 | Reason                                                                                              |
| ---------------------------------------------------------- | -------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `ResponseStatusCancelled` and `Failed` (proposal, review 1) | one `Incomplete` status with `TurnOutcome.Reason`                    | one axis for what can happen next, one for what happened; every renderer needs the reason anyway     |
| Six statuses incl. `running`, `waiting`, `failed`, `cancelled` (colleague) | three returned statuses; `running` is a turn-record state in Phase 2 and `waiting` is `suspended` | `CreateResponse` never returns a running turn; failed versus incomplete is the reason plus `Next` |
| `turn-stopped` / `turn-failed` reminder names               | one name, `turn-incomplete`, reason in the details                   | one thing to match; wording varies by reason                                                        |
| A running call answered "not run" (proposal)               | "unknown result", late result on `Response.BackgroundTasks`          | a tool that ignores cancellation can commit after the save; "no effect" would be false (review)     |
| Two not-run texts by why the turn ended                    | two texts by what the call did                                       | the reminder says why the turn ended                                                                |
| A new content type for the outcome                         | reminder details                                                     | older Dive versions must still open the session                                                     |
| `OnIncompleteTurn func(msgs, err) msgs`                    | a hook in `Hooks` with a decision                                    | also notification and per-turn discard; composes through `Extension`                                |
| `DiscardIncompleteTurns` on `AgentOptions`                 | `IncompleteTurns` with three knobs                                   | the partial-text choice needs a home                                                                |
| Saving interrupted turns behind an opt-in (review 2)       | default on, `Discard` to opt out                                     | the requesting application's judgement: losing a turn is worse than keeping one                     |
| `Response.TurnMessages` (review 2)                         | `Response.Turn` with messages, usage, outcome, suspension, persistence | the same idea with room for Phase 2's identity and revision                                       |
| Expose stop reasons first, automate later (colleague)       | output limit exposed; pause continued up to a bound                  | a pause is mechanical and documented as "resend"; more output is a choice                            |
| Checkpoint every step in the same release (proposal, colleague) | end-of-invocation save now, checkpoints in Phase 2               | needs a new session extension and store format; the close function is shared either way             |
| A stopped turn hidden from the next request                 | Phase 2, a projection policy                                         | the cancelled wording addresses the motivating case                                                 |
| Stop at a step boundary via the callback                   | `WithSoftCancel` on the context                                      | reaches subagents and tools; no sentinel through the callback                                       |
| A fourth status, `cancelled`, for an abandoned turn (round 4) | a reason on `Incomplete`; Phase 2 marks a superseded turn on the record | the status stays the next-action axis; the disposition is record metadata                        |
| A refusal is completed with its reason preserved (round 4)  | adopted                                                              | the model finished responding; there is nothing to continue                                         |
| A finalization hook instead of broadening PostGeneration (round 4) | adopted: PostGeneration keeps its scope; `OnIncompleteTurn` is the end hook for incomplete turns | one end hook per outcome, no semantic change to existing hooks             |
| `interrupted` as the reason for a stop (round 4)            | `canceled`                                                           | matches `context.Canceled`                                                                          |
| `awaiting_result`, `not_started` state names (round 4)      | `waiting` kept; `not_started` adopted for both phases                | one vocabulary across v1.34 and Phase 2                                                             |
| `[DONE]` without a finish reason is a valid end (round 4)   | adopted                                                              | protocol end versus transport end                                                                   |
| A fourth persistence state for an uncertain commit (round 4) | adopted, `unknown`                                                  | an error must not imply that nothing was saved                                                      |
| Stages: defects, envelope, recoverable turns, durability (round 4) | adopted as the ordering of Phase 2                            | keeps the default storage cost where it is                                                          |
| A started read-only call answered "not run" (rev. 3)          | `unknown`; the annotation softens `Next` only                        | repeatability is a replay policy, not evidence (two reviews)                                        |
| A suspension is never persisted after a cancellation (rev. 3) | the suspension is persisted; cancelling it is the application's explicit act | the tool or a hook may already have dispatched the work (review)                              |
| A store error means the write did not land (rev. 3, 4)         | `failed` only for a pre-write rejection, `unknown` otherwise; the session resyncs | a rename can land before a later error (review)                                           |
| An unknown stop reason is finished and its calls run (rev. 5)  | `other` never runs a call; text-only stays completed                 | the pinned Google SDK already has `UNEXPECTED_TOOL_CALL`, which the adapter does not name (review) |
| `Turn.Messages` cumulative for every invocation (rev. 3)       | cumulative on a resume, this invocation's on a continuation           | v1.34 sessions append; only a resume replaces (review)                                              |
| `model_context_window_exceeded` as an output limit (rev. 7 draft) | its own reason, `context_limit`, with `Next == input`             | continuing on the same history is rejected as too long; the application compacts first (review)     |
| `LockSession` returns only `unlock` (rev. 6)                   | it also returns the locked context                                   | a caller that locks and then runs the agent must get `ErrReentrantSession`, not a deadlock          |
| Three increments of work (rev. 6)                              | six steps, the exit-path refactor on its own                         | the refactor and the behaviour change are each reviewable alone                                     |

## Open questions

1. **Should a soft cancel also stop between the calls of a sequential
   batch?** This design says yes, before each call not yet started, which
   ends sooner and leaves nothing half-done. Recommendation: before each
   call.
2. **PostToolUse hooks for results drained after cancellation.** Skipped
   here, since they would run with a cancelled context and their effects are
   advisory. An application that needs them can watch `tool_call_result`
   items instead.
3. **A grace period after cancellation.** A parallel tool that honours its
   context returns within milliseconds with its own "cancelled" result, but
   the drain loop returns at once, so that call is recorded as unknown and
   its honest result only arrives through the handle. A short fixed wait,
   tens of milliseconds, would record the tool's own result in most cases at
   the cost of that much stop latency. Not in this design; worth measuring
   during implementation.
4. **The pause bound.** Ten continuations per invocation, as a constant. An
   option if anyone needs it.
5. **`Next` as advice or as a check.** Advisory here: `WithContinue` works
   in every state. The strict mode that refuses new input over unknown calls
   is a Phase 2 option, not a default.
6. **The scope of `Discard`.** It restores the old behaviour for error exits
   only; model-stop turns were always saved and now carry a reminder. An
   application that wants those without the reminder can strip it in
   `OnIncompleteTurn`.
7. **Wording.** The table in section 5 is a first draft; the name and the
   details are the contract. Worth a pass against a few models before
   release, in particular whether the cancelled wording makes a model too
   passive on the next real request.
