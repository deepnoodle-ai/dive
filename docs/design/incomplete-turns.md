# Incomplete Turns

_Last updated: 2026-09-24_
_Status: proposal, no code written. Answers the Noodle team's request "Dive:
keep turns that don't finish" (24 September 2026, against v1.33.0). Revised
after the first review round on the pull request: a call that was running when
the turn ended is no longer recorded as "not run", the partial-resume failure
rule is explicit, the error contract covers every exit, and the session's
outcome metadata no longer depends on message order. Section references to
`agent.go` are against v1.33.1, the current release._

A turn that is stopped or fails partway is recorded the same way a turn that
finishes is: what happened is saved, the saved history is valid to send again,
and the record says how the turn ended, to the model and to the application.

The design has three parts:

1. **v1.34 (backwards compatible).** Save on every exit path, close the turn
   so it can be sent again, record the outcome, return the partial response,
   add a soft cancel, and repair unanswered tool calls in every encoder. The
   old behaviour stays one option away.
2. **Additive follow-ups.** Record a turn step by step so a crash loses
   nothing, close a suspended turn without a model call, and let a session
   hide cancelled turns from the model.
3. **A later breaking iteration.** Fold the outcome into the core contracts:
   `CreateResponse` reports how a turn ended through the response alone, and
   `Session` becomes turn-shaped.

## Summary of decisions

- A turn **begins** when the agent has loaded history, resolved any resume,
  and accepted the input, which is the moment PreGeneration hooks run. From
  then on every exit saves, with one exception: a partial resume, which still
  has external work outstanding, stays suspended and unchanged. Failures
  before the boundary return `(nil, err)` and save nothing, as today.
- Two new terminal statuses: `ResponseStatusCanceled` and
  `ResponseStatusFailed`. Cancelled means the context was cancelled or a
  soft cancel was requested; everything else that ends a turn early is a
  failure, including a deadline and a hook abort.
- `CreateResponse` returns **both** a non-nil `*Response` and a non-nil
  error for a cancelled or failed turn, on every exit after the turn begins.
  Completed and suspended turns return `err == nil`; cancelled and failed
  turns never do. The error always wraps a `*GenerationError`, which gains a
  `Response` field.
- The saved turn is **closed** before it is written: a tool call that never
  started is answered "not run, no effect", a call that was still running is
  answered "interrupted, result unknown" and its late result is handed back
  on `Response.BackgroundTasks`, a half-written tool call and an unfinished
  thinking block are dropped, and text the model was still writing is kept
  as an assistant message.
- The outcome is recorded as a **typed reminder with structured details**:
  `dive.Reminder` gains `Details`, rendered to the model as
  `<system-reminder name="turn-canceled">` or `turn-failed` with wording Dive
  owns. Older Dive versions decode it as a plain reminder, which matters for
  rollbacks. `dive.FindTurnOutcome` reads the details back as a
  `*TurnOutcome`.
- Session writes at the end of a turn use a **salvage context**: the values
  of the run's context without its cancellation, bounded by a timeout.
- A new **`OnIncompleteTurn` hook** can repair the messages, change the
  recorded error, notify, or discard the turn. PostGeneration hooks then fire
  with the status set, as they already do for suspended turns.
- **`dive.WithSoftCancel`** derives a context whose cancel asks the agent to
  end the turn at the next step boundary instead of immediately.
- Every provider encoder answers a tool call that has no result, in the
  request only, through one shared `llm` helper.
- **`AgentOptions.IncompleteTurns`** holds the escape hatches: `Discard`
  (v1.33 behaviour), `DropPartialText`, and `SaveTimeout`.

### What it looks like in an application

A chat application with a stop button, on a Dive session:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Model:   anthropic.New(),
    Tools:   tools,
    Session: sess,
})

ctx, cancel := context.WithCancel(ctx)       // second press: stop now
ctx, softCancel := dive.WithSoftCancel(ctx)  // first press: stop after this step

resp, err := agent.CreateResponse(ctx, dive.WithInput(text), dive.WithEventCallback(render))
switch {
case err == nil && resp.Status == dive.ResponseStatusSuspended:
    askUser(resp.Suspension)
case err == nil:
    show(resp.OutputText())
case resp == nil:
    return err // the turn never started: bad input, session failed to load
case resp.Status == dive.ResponseStatusCanceled:
    notice("Stopped.") // resp.OutputMessages is saved; the next turn continues from it
    // resp.BackgroundTasks: late results of calls that were still running,
    // to deliver on the next turn with WithBackgroundResults, or to drop.
default:
    notice("The turn failed: " + resp.Outcome.Error)
}
```

Rebuilding the transcript from the session later, the marker is data:

```go
msgs, _ := sess.Messages(ctx)
for _, m := range msgs {
    if outcome, ok := dive.FindTurnOutcome(m); ok {
        renderMarker(outcome.Status, outcome.Error) // "Stopped." or "The turn failed: ..."
        continue
    }
    renderMessage(m)
}
```

Nothing else changes for an application that never cancels and never sees a
provider error.

## What happens today

The proposal's description is right in outline. Three details matter for the
design.

**The partial work already exists in memory; it is thrown away at the
boundary.** `generate` wraps every loop failure in `*GenerationError`
carrying the usage, the output messages, and the items accumulated so far,
and `CreateResponse` folds in earlier Stop-hook continuations and resume-phase
items (`agent.go:937-959`). Its doc comment says the partial turn is
deliberately not persisted because a half-turn can violate provider
role-alternation invariants. That reasoning is the thing this design replaces:
a half-turn is only invalid while a tool call has no result, and the agent has
everything it needs to answer one.

**Both tool-batch paths discard finished results.** The sequential loop
returns `nil, ctx.Err()` even though `batch.Outcomes` holds the results of
every call that completed (`executeToolCallsSequential`). The parallel drain
loop does the same, and results that finished but were not yet drained stay in
the buffered channel (`executeToolCallsParallel`). Nothing is lost that cannot
be picked up at the point of return.

**Streamed text is recoverable.** `ResponseAccumulator.Response` finalizes
whatever blocks it has, before or after `message_stop`. `generateStreaming`
returns a nil response on error, so the partial message never reaches the
loop.

The exit paths after the turn begins, and what each one drops:

| Exit                                                   | Where                                  | Lost today                                       |
| ------------------------------------------------------ | -------------------------------------- | ------------------------------------------------ |
| PreGeneration hook error                               | `CreateResponse`                       | the input                                        |
| PreIteration hook error, tool resolution error         | `generate`                             | input, prior iterations                          |
| Model call error, including `context.Canceled`         | `generate`                             | input, prior iterations, partial text            |
| Event callback error                                   | `generate`, tool paths                 | input, prior iterations, in-flight batch results |
| Hook abort in PreToolUse / PostToolUse / PostToolUseFailure | tool paths                        | input, prior iterations, in-flight batch results |
| Context cancelled during a tool batch                  | tool paths                             | input, prior iterations, in-flight batch results |
| Stop or PostGeneration hook abort                      | `CreateResponse`                       | the whole completed turn                         |
| Session save error, including a cancelled context      | `CreateResponse`, `finishSuspended`    | the whole completed turn                         |
| Resume-phase errors                                    | `CreateResponse`                       | caller-supplied results, not-started results     |

The last two rows are worth noting: a turn the model finished is lost today if
the caller's context is cancelled during the save, because the save uses the
cancelled context.

## v1.34 design

### 1. Turn boundary and outcome

The turn begins immediately before PreGeneration hooks run. By then the
session history is loaded, the suspension (if any) is validated and prepared,
SessionStart seeds are saved, and the input is accepted. Errors before that
point (`ErrNoSuspendedTurn`, `ErrInputOnSuspendedSession`, a session load
error, a SessionStart hook error, the session lock lost to cancellation) are
the caller's problem and change nothing in the session, exactly as today.

One exit after the boundary is not an incomplete turn. A partial resume, where
the caller supplied some of the pending results and others are still
outstanding, never calls the model. If it fails in a post-tool hook, in the
event callback, in PostGeneration or in `SaveSuspendedTurn`, the session is
left exactly as it was, nothing is saved, and `(nil, err)` is returned; the
caller resubmits the same results. Section 6 has the rule.

From the turn's start, every exit is one of four outcomes:

```go
const (
    ResponseStatusCompleted ResponseStatus = "completed"
    ResponseStatusSuspended ResponseStatus = "suspended"
    ResponseStatusCanceled  ResponseStatus = "canceled" // new
    ResponseStatusFailed    ResponseStatus = "failed"   // new
)
```

Cancelled is chosen when `errors.Is(err, context.Canceled)` holds for the
error that ended the turn or for `ctx.Err()`, or when a soft cancel was
requested (section 7). Everything else is failed: a provider error, a
`context.DeadlineExceeded` from the caller or from `ResponseTimeout`, a hook
abort, a callback error, a persistence error. The rule is that cancelled is
the one outcome somebody chose; a deadline is a limit that was hit, and the
person and the model should see it as a failure with a reason.

The spelling follows Go's `context.Canceled` and Dive's existing
`DialogOutput.Canceled`; prose can say "cancelled".

The outcome is a small record:

```go
// TurnOutcome says how a turn that did not complete ended. It is returned on
// Response.Outcome, given to OnIncompleteTurn and PostGeneration hooks, and
// saved at the end of the turn as the details of a turn-canceled or
// turn-failed reminder (see FindTurnOutcome).
type TurnOutcome struct {
    // Status is ResponseStatusCanceled or ResponseStatusFailed.
    Status ResponseStatus `json:"status"`

    // Error is the error that ended the turn, as text. "context canceled"
    // for a cancellation. Hooks may rewrite it before it is saved, for
    // example to remove a request ID or a secret from a provider error.
    Error string `json:"error,omitempty"`

    // Hook is the hook type ("PreToolUse", "Stop", ...) when a
    // HookAbortError ended the turn.
    Hook string `json:"hook,omitempty"`

    // NotRun lists the IDs of the tool calls answered with
    // ToolCallNotRunText: the turn ended before they started, and they had
    // no effect.
    NotRun []string `json:"not_run,omitempty"`

    // Interrupted lists the IDs of the tool calls answered with
    // ToolCallInterruptedText: they were running when the turn ended and
    // their result is unknown. Their late results, if any, arrive on
    // Response.BackgroundTasks.
    Interrupted []string `json:"interrupted,omitempty"`
}
```

`Status` reuses `ResponseStatus` rather than introducing a second enum. Only
the two new values appear on an outcome in v1.34; a later status for a turn a
crashed process left open (Phase 2) joins the same enum.

### 2. Closing the turn

Before anything is saved, the agent turns what it has into a turn any provider
accepts. The rules, in order:

1. **The input messages are always kept**, including a synthetic
   background-results message and Stop-hook continuation reminders that the
   current code already folds into the saved turn.
2. **Every finished assistant message and every finished tool-result message
   is kept as it stood.** Mid-turn compaction rewrites only the model-facing
   working set; the saved turn comes from the output accumulator, exactly as
   for a completed turn.
3. **The batch in flight when the turn ended becomes a tool-result message,
   and every call in it is answered by what is actually known about it.**
   There are three kinds of call:

   - **A call whose tool returned a result keeps it**, even an error result
     the tool produced because its context was cancelled, since that is what
     happened and may carry partial output.
   - **A call that never started** is answered "not run": a later call in a
     sequential batch, every call in a batch whose PreToolUse phase aborted,
     a call in a partial streamed message, or one the agent refused to start
     because the context was already cancelled. It had no effect, and the
     text says so.
   - **A call that had started and had not reported when the turn ended** is
     answered "interrupted". Only a parallel batch can leave a call in this
     state: sequential execution waits for the running call, as it does
     today, so that call always records its own result. The agent does not
     wait for a running parallel tool (the proposal's requirement, and the
     1.32.0 decision that cancelling stops waiting). It does a non-blocking
     drain of results that have already landed in the batch channel, keeps
     those, and answers the rest as interrupted: the tool may have taken
     effect after the turn was saved, so the text says the result is unknown
     and never that the call had no effect.

   ```go
   // ToolCallNotRunText answers a tool call the turn ended before it
   // started. The call had no effect. Exported so applications can
   // recognize it in history.
   const ToolCallNotRunText = "Not run: the turn ended before this call started. It had no effect."

   // ToolCallInterruptedText answers a tool call that was running when the
   // turn ended and had not reported a result. Whether it took effect is
   // unknown.
   const ToolCallInterruptedText = "Interrupted: the turn ended while this call was running. Its result was not recorded; it may have taken effect, so check before repeating it."

   // ErrToolCallNotRun and ErrToolCallInterrupted are the
   // ToolCallResult.Error of such calls, as ErrBatchHalted is for a halted
   // call.
   var (
       ErrToolCallNotRun      = errors.New("dive: not run because the turn ended")
       ErrToolCallInterrupted = errors.New("dive: interrupted because the turn ended")
   )
   ```

   **An interrupted call's late result is not lost.** Its goroutine finishes
   on its own, as today, with its late stream events suppressed by the
   existing callback gate; but its final result is routed to a
   `BackgroundTaskHandle` on `Response.BackgroundTasks`, one per interrupted
   call, with `ToolUseID` set and a description naming the call. That is the
   mechanism Dive already has for a result that arrives after its turn:
   `AwaitBackgroundTasks` (with whatever timeout the application chooses)
   and `WithBackgroundResults` on the next turn deliver the real result to
   the model through the existing `background-tasks` reminder, and
   `PostBackgroundToolUse` hooks fire for it, using the PreToolUse hook
   context the handle carries. An application that wants to wait a moment
   for stragglers before its next turn does so with its own deadline; one
   that does not simply drops the handles. Either way the saved turn is
   honest about what was known when it was written.

   The two texts split by what the call did, not by why the turn ended; the
   outcome record that follows says why. Results appear in the original call
   order, with `AdditionalContext` text after them, the same shape as any
   batch. A soft cancel (section 7) never leaves a call interrupted.
4. **Text the model was still writing is kept** as the last assistant
   message, with `DropPartialText` to leave it out. `generateStreaming`
   returns the accumulator's partial response with the error, and
   `ResponseAccumulator` gains a way to report which blocks never received
   `content_block_stop`. In that partial message: a text block is kept, a
   `tool_use` block whose input is not complete JSON is dropped, a thinking
   block that was still streaming is dropped (its signature arrives last), a
   server tool call whose result never arrived is dropped by the existing
   `dropServerToolCalls` rule. A partial message with nothing left is not
   recorded. Non-streaming `Generate` has no partial content.
5. **The outcome reminder is appended last**, as a user-role message (section
   3). `Recorded` reminders that hooks queue while the turn ends are recorded
   before it, so the outcome reminder is always the final message of a closed
   turn.

The closing rules are one exported function so a session (Phase 2) or an
application with its own persistence can apply them to messages it holds:

```go
// CloseTurn returns turn with every tool call that has no result answered
// and the outcome recorded as a reminder at the end. Calls listed in
// outcome.Interrupted are answered with ToolCallInterruptedText; every
// other unanswered call with ToolCallNotRunText, and outcome.NotRun is
// filled with their IDs. It does not modify turn.
func CloseTurn(turn []*llm.Message, outcome *TurnOutcome) []*llm.Message
```

The cleanup of a partial streamed message (rule 4) is done by the agent where
the accumulator is, not by `CloseTurn`, which only sees messages.

### 3. Telling the model and the application

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

const (
    ReminderNameTurnCanceled = "turn-canceled"
    ReminderNameTurnFailed   = "turn-failed"
)

// NewTurnOutcomeReminder builds the reminder the agent records at the end of
// an incomplete turn. Applications rendering a transcript match its Name;
// the Content is Dive's wording and may change between releases.
func NewTurnOutcomeReminder(outcome *TurnOutcome) Reminder

// FindTurnOutcome returns the outcome recorded in a message, if any.
func FindTurnOutcome(message *llm.Message) (*TurnOutcome, bool)

// FindLatestTurnOutcome searches from newest to oldest.
func FindLatestTurnOutcome(messages []*llm.Message) (*TurnOutcome, bool)
```

Why a reminder and not a new content type. A dedicated
`llm.TurnOutcomeContent` was the other candidate. It would be a cleaner type,
but `Message.UnmarshalJSON` fails on a content type it does not know, so a
session saved by 1.34 with one cancelled turn could not be opened by 1.33 at
all. Noodle uses Dive's sessions; a rollback after a deploy would break every
such conversation. A reminder with an extra field decodes on every existing
version (Go ignores unknown JSON fields), renders through the reminder path
every provider already has, and is covered by the reminder priming rule in
the system prompt. The typed API sits on top: `FindTurnOutcome` decodes
`Details` into a `TurnOutcome`, and `NewTurnOutcomeReminder` encodes one.
Details is generic on purpose; other reminders (skills, compaction) can use it
later.

The rendered block, contextual tier, user role. Dive owns the wording; the
implementation may tune it, and the names are the stable part:

```text
<system-reminder name="turn-canceled">
The previous turn was stopped before it finished. Everything above this note,
including every tool result, happened as shown. A tool result that begins
"Not run:" is a call that never started and had no effect. A tool result that
begins "Interrupted:" is a call that was running when the turn ended; whether
it took effect is unknown, so check before repeating it. Continue from the
completed work: do not repeat steps that completed, and do not assume steps
that did not complete have happened. If the user now asks for something else,
do that instead.
</system-reminder>
```

```text
<system-reminder name="turn-failed">
The previous turn failed before it finished. Error: <error>. Everything above
this note, including every tool result, happened as shown. A tool result that
begins "Not run:" is a call that never started and had no effect. A tool
result that begins "Interrupted:" is a call that was running when the turn
ended; whether it took effect is unknown, so check before repeating it.
Continue from the completed work: do not repeat steps that completed, and do
not assume steps that did not complete have happened.
</system-reminder>
```

For a hook abort `<error>` is `HookAbortError.Error()`, which already names
the hook type. For a deadline the wording says "the turn's time limit was
reached" while `TurnOutcome.Error` keeps the raw error string.

The reminder is a separate user message rather than a block inside the
tool-result message so that a turn ending in partial text (no tool-result
message) and a turn ending mid-batch have the same shape. Consecutive user
messages are a shape Dive already sends on every provider (SessionStart seeds
ahead of the input, `InjectContext`, a recorded reminder delivered after a
tool-result message), and Gemini's rule against a request ending in a model
turn is satisfied by it.

### 4. What `CreateResponse` returns

```go
// CreateResponse ...
//
// When the turn ends before it completes, CreateResponse returns a non-nil
// *Response together with a non-nil error: the error says why the turn
// ended (errors.Is(err, context.Canceled) for a cancellation), and the
// response says what happened. Response.Status is ResponseStatusCanceled or
// ResponseStatusFailed, Response.Outcome describes the end, and
// Response.OutputMessages holds the closed turn, ready to append to a
// history exactly like a completed turn's. Completed and suspended turns
// return a nil error; cancelled and failed turns never do.
func (a *Agent) CreateResponse(ctx context.Context, opts ...CreateResponseOption) (*Response, error)

type Response struct {
    // ...
    // Outcome is set when Status is ResponseStatusCanceled or
    // ResponseStatusFailed. Nil otherwise.
    Outcome *TurnOutcome `json:"outcome,omitempty"` // new
}

// GenerationError is kept for compatibility. The error CreateResponse
// returns for a turn that ended early still wraps one.
type GenerationError struct {
    Err            error
    Usage          *llm.Usage
    OutputMessages []*llm.Message // now the closed turn, same as Response.OutputMessages
    Items          []*ResponseItem
    Response       *Response      // new: the same response CreateResponse returns
}
```

Returning both values is unusual in Go but not unprecedented (`io.Reader`),
and no existing caller can break: code that checks `err` first ignores the
response, and code that only checks `err` keeps working. The alternative the
proposal offers, `errors.As` into a wrapper, is what `GenerationError`
already is, and it is the clumsier path; keeping it as a second route costs
nothing.

**The contract holds for every exit after the turn begins**, not only for
failures inside `generate`: a PreGeneration hook error, a PreIteration hook
error, a tool resolution error, a model call error, an event callback error,
a hook abort in any tool hook, in Stop or in PostGeneration, a full-resume
failure before the model call, and a failed salvage save all return the same
shape, with the same `*Response` reachable both directly and through
`GenerationError.Response`. Today the `Response` is created after
PreGeneration hooks run and most of these sites `return nil, err` on their
own; the implementation creates it before the hooks and routes every exit
through one function that closes the turn, saves, and builds the error. The
one exception is the partial-resume failure of section 1: no turn ended, so
it returns `(nil, err)`; that error keeps wrapping a `*GenerationError`
carrying the items emitted so far, as it does today, with `Response == nil`.
The test plan asserts `errors.As` and the status on each class.

Other fields on an incomplete response: `Usage` is the usage so far; `Items`
is everything emitted, including the synthesized not-run and interrupted
results and the terminal outcome item (section 10); `BackgroundTasks` carries
the handles of background tasks started before the end and one handle per
interrupted call (section 2), so the caller can await, deliver or drop them;
`Suspension` is set on a cancelled or failed full resume with the merged turn
and `PendingToolCalls == nil`, so stateless callers flush it the way they
flush a completed resume.

If the salvage save itself fails, the returned error is
`errors.Join(err, saveErr)` and the response is still returned; the caller
has the closed turn in hand even though the session does not.

### 5. Hooks

```go
type Hooks struct {
    // ...
    // OnIncompleteTurn hooks run when a turn ends cancelled or failed, after
    // the turn is closed and before it is saved. They can repair the messages
    // that will be saved, change the recorded error, notify an external
    // system, or discard the turn. They do not run when
    // IncompleteTurns.Discard is set.
    OnIncompleteTurn []IncompleteTurnHook
}

// IncompleteTurnHook receives hctx.Outcome (mutable), hctx.OutputMessages (the
// closed turn without the outcome reminder, mutable), hctx.Usage,
// hctx.Response, and hctx.Messages. Regular errors are logged. The context is
// the salvage context: not cancelled, so the hook can do I/O.
type IncompleteTurnHook func(ctx context.Context, hctx *HookContext) (*IncompleteTurnDecision, error)

type IncompleteTurnDecision struct {
    // Discard drops this turn: nothing is saved and PostGeneration does not
    // run, as if IncompleteTurns.Discard were set for this call only.
    Discard bool
}

type HookContext struct {
    // ...
    // Outcome is set for OnIncompleteTurn hooks and for PostGeneration hooks
    // on a cancelled or failed turn.
    Outcome *TurnOutcome
}
```

The decision return mirrors `StopHook`. The proposal's
`OnIncompleteTurn func([]*llm.Message, error) []*llm.Message` covers repair
only; a hook in `Hooks` also covers notification (the reason `OnSuspend`
exists) and the per-turn discard, and it composes through `Extension`.

Two uses the hook is for:

- **Repairing a turn whose own content caused the failure.** A tool returned
  an image to a text-only model and the next model call got a 400. The hook
  finds the result in `hctx.OutputMessages` and replaces the image with a
  note, so the saved history does not fail every later request the same way.
- **Dropping a turn a policy hook aborted.** A compliance hook aborted in
  PostGeneration because the output violates policy. The hook reads
  `hctx.Outcome.Hook == "PostGeneration"` and the marker it left in
  `hctx.Values`, and returns `Discard: true`.

**PostGeneration** fires on a kept incomplete turn after `OnIncompleteTurn`
and before the save, with `hctx.Response.Status` set and `hctx.Outcome`
populated. This matches what `finishSuspended` does for suspended turns so
metrics and usage loggers see every turn end once. A `HookAbortError` from
PostGeneration on an already-failing turn is logged, not honoured. For a
turn the model finished, a PostGeneration abort makes the turn failed:
`OnIncompleteTurn` then runs and the turn is saved with the abort as its
outcome; PostGeneration is not run a second time. Stop hooks never run on an
incomplete turn.

The hook flow becomes:

```text
SessionLoad → SessionStart → PreGeneration → [PreIteration → LLM → PreToolUse → Execute → PostToolUse]* → Stop → PostGeneration → SessionSave
   on suspend:         OnSuspend → PostGeneration → SaveSuspendedTurn
   on cancel/failure:  OnIncompleteTurn → PostGeneration → SaveTurn / SaveResumedTurn
```

### 6. Saving

**The salvage context.** Every session write at the end of a turn, including
the write of a completed turn, uses
`context.WithoutCancel(ctx)` bounded by `IncompleteTurns.SaveTimeout`
(default 30 seconds). The derived context keeps the run's values (tracing
span, the session-lock marker) and drops its cancellation. `FileStore` and
`MemoryStore` ignore the context; a database-backed `Session` would otherwise
refuse the write with the very cancellation that ended the turn, which is the
"salvage context" trick all three applications had to invent. Making the
completed-turn save use it too closes the save-error row in the table above.

**Plain sessions.** An incomplete turn is saved with `SaveTurn`, the same
call as a completed turn; the messages carry the outcome reminder, so any
`Session` implementation works unchanged and can read the outcome back with
`FindTurnOutcome`. `session.Session.SaveTurn` additionally records
`Metadata["outcome"]` on the event when the turn carries an outcome reminder
anywhere in its messages (`FindLatestTurnOutcome` over the turn), next to the
existing `"suspended"` metadata. The agent keeps the outcome reminder as the
last message of a closed turn, but the session does not depend on that: a
custom arrangement, or a reminder recorded after it, still yields the metadata
that the Phase 2 hidden-turn view reads. Phase 2's `TurnRecorder.EndTurn`
receives the outcome as a value and needs no scan.

**Resumed turns.** Two cases, split by whether external work is still
outstanding.

A *full* resume, where every pending call has a result (caller-supplied, or
produced by re-running the calls that were not started before the
suspension), that is cancelled or fails at any point after the boundary,
before or after the model call, is closed and written with `SaveResumedTurn`
(`rs.TurnMessages` + output + closing), which replaces the suspended event and
clears the suspension. The caller-supplied results are kept, not-started calls
the agent had not reached are answered "not run", and re-run calls still
executing in a parallel batch are answered "interrupted". This is a behaviour
change: today a failed resume leaves the session suspended so the resume can
be retried, but the retry requires the caller to supply the external results
again, and the results just supplied are exactly the part worth keeping. With
the outcome recorded, "retry" becomes "send the next turn". `Discard` restores
the old behaviour with the rest.

A *partial* resume, where the caller supplied some results and others are
still pending, never calls the model. It can still fail, at four points: a
PostToolUse or PostToolUseFailure hook aborting for a supplied result
(`fireResumePostHooks`), the event callback rejecting the `tool_call_result`
item that announces it, a PostGeneration hook aborting inside
`finishSuspended`, or `SaveSuspendedTurn` failing. None of those closes the
turn: external work is outstanding, and a closed turn could never accept it.
The session is left exactly as it was before the call, with the earlier
suspension and the earlier pending set, nothing is saved, and `(nil, err)` is
returned, as today; a new test pins the invariant at each of the four points.
The caller still holds
the results it just supplied and resubmits them; a stream consumer sees their
`tool_call_result` items again on the retry, which is the existing behaviour
of a failed resume. The alternative, saving the supplied results alongside the
still-pending calls, would need a session write that half-advances a
suspension and a resume path that tolerates one, for results the caller
already has in hand; not worth it.

**Stateless callers** get the closed turn on `OutputMessages` and append it
to their own history like any other turn.

**Under the session lock.** The whole sequence runs inside the existing
per-session lock, so a concurrent `CreateResponse` on the same session waits
for the salvage write.

### 7. Soft cancel

A stop button and an out-of-budget check want the agent to stop at a clean
point rather than mid-call. nvoken gets this today by returning a sentinel
error from the event callback, which fires from the streaming loop and from
parallel tool goroutines and aborts whatever is in flight.

```go
// WithSoftCancel returns a copy of parent that carries a cancel request.
// Calling cancel asks an agent run using ctx to end at its next step
// boundary: before its next model call, and before each tool call it has not
// started. Tool calls already running finish; calls not started are answered
// with ToolCallNotRunText. The turn ends with ResponseStatusCanceled and an
// error for which errors.Is(err, context.Canceled) holds, exactly as a hard
// cancellation does. A second, ordinary context cancellation still stops the
// run at once.
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
model's last message requested tool calls has those calls answered as not run,
which is the outcome the person pressing stop wants: nothing more happens.
Because a running batch is allowed to finish, a soft cancel never produces an
interrupted call; every recorded result is the tool's own.

### 8. Encoder backstop

Each provider encoder answers any tool call that has no result, in the
request only, so a session saved by an older version, by an application with a
bug, or by a process that died mid-turn (before Phase 2) cannot block every
request after it:

```go
// llm
// ToolCallNotRunText and ToolCallInterruptedText are the texts of the error
// results recorded for a tool call that was never answered. The dive
// constants of the same names are these values.
const (
    ToolCallNotRunText      = "Not run: the turn ended before this call started. It had no effect."
    ToolCallInterruptedText = "Interrupted: the turn ended while this call was running. Its result was not recorded; it may have taken effect, so check before repeating it."
)

// AnswerUnansweredToolCalls returns messages in which every tool_use block
// is followed by a tool_result for its ID. A missing result is inserted as
// an error result with ToolCallInterruptedText, into the next user message
// when there is one and otherwise as a new tool-result message after the
// assistant message. The interrupted text is used because history alone
// cannot say whether the call ran, and "unknown" is the safe claim. Server
// tool calls are left alone. Copy-on-write: messages is returned as is
// when nothing is missing.
func AnswerUnansweredToolCalls(messages []*Message) []*Message
```

It lives in `llm` so `dive.CloseTurn` and the encoders share one
implementation (`providers` imports `dive`, so the reverse dependency is not
available). The Anthropic, OpenAI Responses, Chat Completions and Google
encoders call it where they already normalize history, next to
`RenderReminders` and the image lifting. Gemini's opposite check, a tool result
whose call is missing, is out of scope but is the same helper's natural next
rule.

### 9. Options

```go
type AgentOptions struct {
    // ...
    // IncompleteTurns configures what the agent does when a turn is
    // cancelled or fails before it completes.
    IncompleteTurns IncompleteTurnOptions
}

type IncompleteTurnOptions struct {
    // Discard restores the behaviour before v1.34: an incomplete turn is not
    // closed, not saved, and no OnIncompleteTurn or PostGeneration hook runs
    // for it; no not-run or interrupted results and no outcome item are
    // emitted; a failed resume leaves the session suspended. CreateResponse still returns the
    // partial response alongside the error, with the raw partial messages
    // GenerationError carries today. For applications that keep incomplete
    // turns themselves and are not ready to remove that code.
    Discard bool

    // DropPartialText leaves out text the model was still writing when the
    // turn ended.
    DropPartialText bool

    // SaveTimeout bounds the session write at the end of a turn. The write
    // uses a context without the cancellation that ended the turn. Default
    // 30 seconds.
    SaveTimeout time.Duration
}
```

`Discard` is the whole old behaviour, not just the save, because an
application that rebuilds turns from the event stream (Noodle today) would
otherwise see Dive's synthesized not-run results next to its own and record
each call twice.

### 10. Streaming items

Every tool call the agent answers as not run or interrupted emits a
`tool_call_result` item with `Error == ErrToolCallNotRun` or
`ErrToolCallInterrupted`, as `haltToolCall` does for a halted call, so every
`tool_call` item has a result item. After a successful save (or the
decision not to save) the agent emits one terminal item:

```go
const (
    // ResponseItemTypeTurnOutcome is a terminal item emitted when a turn ends
    // cancelled or failed. The Outcome field mirrors Response.Outcome. Stream
    // consumers treat it as end-of-stream, as they do ResponseItemTypeSuspended.
    ResponseItemTypeTurnOutcome ResponseItemType = "turn_outcome"
)

type ResponseItem struct {
    // ...
    Outcome *TurnOutcome `json:"outcome,omitempty"`
}
```

Both are emitted with the salvage context, since the run's context is
cancelled. Neither is emitted under `Discard`.

### Interface summary

Everything new or changed in v1.34, in one place:

```go
// dive
const ResponseStatusCanceled, ResponseStatusFailed ResponseStatus
type  TurnOutcome struct{ Status ResponseStatus; Error, Hook string; NotRun, Interrupted []string }
type  Response struct{ ...; Outcome *TurnOutcome }       // new field
type  GenerationError struct{ ...; Response *Response }  // new field
const ToolCallNotRunText, ToolCallInterruptedText = llm.ToolCallNotRunText, llm.ToolCallInterruptedText
var   ErrToolCallNotRun, ErrToolCallInterrupted error
// Response.BackgroundTasks also carries one handle per interrupted call
func  CloseTurn(turn []*llm.Message, outcome *TurnOutcome) []*llm.Message
const ReminderNameTurnCanceled, ReminderNameTurnFailed string
func  NewTurnOutcomeReminder(outcome *TurnOutcome) Reminder
func  FindTurnOutcome(message *llm.Message) (*TurnOutcome, bool)
func  FindLatestTurnOutcome(messages []*llm.Message) (*TurnOutcome, bool)
type  Reminder struct{ ...; Details map[string]any }; func (Reminder) WithDetails(map[string]any) Reminder
type  Hooks struct{ ...; OnIncompleteTurn []IncompleteTurnHook }
type  IncompleteTurnHook func(context.Context, *HookContext) (*IncompleteTurnDecision, error)
type  IncompleteTurnDecision struct{ Discard bool }
type  HookContext struct{ ...; Outcome *TurnOutcome }
type  AgentOptions struct{ ...; IncompleteTurns IncompleteTurnOptions }
type  IncompleteTurnOptions struct{ Discard, DropPartialText bool; SaveTimeout time.Duration }
func  WithSoftCancel(parent context.Context) (context.Context, func())
func  SoftCanceled(ctx context.Context) bool
const ResponseItemTypeTurnOutcome ResponseItemType; ResponseItem.Outcome *TurnOutcome

// llm
type  ReminderContent struct{ ...; Details map[string]any }
const ToolCallNotRunText, ToolCallInterruptedText string
func  AnswerUnansweredToolCalls(messages []*Message) []*Message
// ResponseAccumulator: a way to get the partial response with per-block completeness

// providers: each encoder calls llm.AnswerUnansweredToolCalls
// session: SaveTurn records Metadata["outcome"] when the turn carries an outcome reminder anywhere
```

### The cases from the proposal

- **A failure caused by the turn's own content.** Kept. `OnIncompleteTurn`
  repairs it (section 5). The next turn's failure, if the app does nothing,
  is itself recorded, so the person sees the same error twice rather than a
  silent loop.
- **A failure caused by earlier history.** Saving this turn changes nothing
  about the earlier history. The encoder backstop removes one class of it
  (unanswered calls); Anthropic's empty text blocks were fixed in 1.33.1.
- **Parallel tool execution.** Results drained before the end are kept with
  their hooks applied. Results that finished but were still in the channel
  are picked up with a non-blocking drain and kept, without PostToolUse
  hooks, which would otherwise run with a cancelled context; documented.
  Calls still running are answered as interrupted, never as not run, and
  are not waited for; their late results come back as handles on
  `Response.BackgroundTasks` for the application to deliver on the next turn
  or drop. Sequential execution waits for the running call, as today, so it
  records that call's own result, and only the calls after it are not run.
- **Suspension.** A suspended turn is not incomplete; it is paused with
  external work outstanding, and stays as it is. A resume with everything
  supplied that then fails is closed (section 6). Closing a suspended turn
  without a model call is Phase 2.
- **Mid-turn compaction.** The saved turn comes from the output accumulator,
  never from the compacted working set, as for a completed turn.
- **Hook aborts.** `TurnOutcome.Hook` names the hook type and `Error` carries
  `HookAbortError.Error()`, which includes the reason and the cause.
- **A turn that fails before the first model call returns.** The input and
  the outcome are saved; `OutputMessages` is just the outcome reminder. A
  PreGeneration hook that rejects input therefore records the rejected input
  with a failed outcome, which is the right record; an application that would
  rather not have it can validate before calling `CreateResponse` or discard
  from `OnIncompleteTurn`.
- **Cancellation during the final save of a completed turn.** The salvage
  context makes the save succeed and the turn is completed, not lost.
- **Background tasks.** Their "started" result is already in history; the
  handles come back on the response. The results, when they arrive, can be
  delivered with `WithBackgroundResults` on the next turn as today.

### Compatibility

What changes for an application that upgrades and changes nothing:

1. Cancelled and failed turns are saved to its session, closed and with an
   outcome reminder, and the next turn sends them. If the application also
   saves them itself, it now has duplicates: set `IncompleteTurns.Discard`
   until that code is removed. The changelog entry says this in its first
   line.
2. `CreateResponse` returns a non-nil response with the error. Code that
   checks `err` first is unaffected.
3. `GenerationError.OutputMessages` is the closed turn, not the raw partial
   messages. Code that answered open calls itself from it would now answer
   them twice; `Discard` restores the raw messages.
4. PostGeneration hooks fire on cancelled and failed turns, with the status
   set. Hooks that count completed turns should check `hctx.Response.Status`
   (they should already, for suspended turns).
5. A failed resume no longer leaves the session suspended.
6. Session writes at the end of a turn use an uncancelled context bounded by
   a timeout. A custom session that relied on the cancellation to skip the
   write will now write.
7. Event callbacks receive not-run and interrupted result items and a
   `turn_outcome` item after cancellation, with an uncancelled context.

Item 2 is the only one `Discard` does not undo, and it cannot break a caller.

What each of the three applications does after upgrading:

- **Noodle** deletes `partial.go`, the save branch in `runturn.go`, and the
  error separator; reads `resp.Outcome` for the notice, and
  `dive.FindTurnOutcome` per message when rebuilding a transcript from the
  session; wires the stop button to `WithSoftCancel` then `WithCancel`.
- **mobius-cloud** has no Dive session and keeps its own tables. It reads
  `resp.OutputMessages` and `resp.Outcome` instead of settling the turn from
  the event stream, or does the same from `OnIncompleteTurn` with
  `Discard: true` as a no-op. Whether a cancelled turn is sent on the next
  request is its own history's choice, as before; the reminder's wording
  ("If the user now asks for something else, do that instead") is meant to
  make sending it safe.
- **nvoken-cloud** is stateless. It replaces its callback sentinel with
  `WithSoftCancel`, sets `DropPartialText`, and appends `resp.OutputMessages`
  to its history the same way for every status.

The CLI in `experimental/cmd/dive` gets stop-persistence for free; two
follow-ups there are to test cancellation with `errors.Is` (it compares with
`!=` today, which never matches a wrapped error) and to render the outcome
reminder as a transcript marker on resume. The A2A executor can map
`ResponseStatusCanceled` to `TaskStateCanceled` instead of failed, and the
subagent tool can put the partial answer text in its error result; both are
small and separate.

### Tests

The proposal's list, plus the cases the design added:

- Cancelled mid-batch, one call finished and one not. Sequential: the saved
  messages are the input, the assistant message, a tool-result message with
  the finished result and a not-run error for the call that was never
  started, then the `turn-canceled` reminder whose details decode to a
  `TurnOutcome` listing that ID under `NotRun`. Parallel, with the second
  tool still running: the same shape with an interrupted error instead, the
  ID under `Interrupted`, and a handle for it on `Response.BackgroundTasks`.
  Both histories encode without error on the Anthropic, OpenAI Responses,
  Gemini and Chat Completions encoders.
- A parallel tool that ignores cancellation and commits after the turn is
  saved: the saved turn answers it "interrupted", never "not run"; the
  reminder does not say it had no effect; its handle delivers the commit's
  result once the tool returns; `WithBackgroundResults` on the next turn
  shows a scripted model the real result and fires `PostBackgroundToolUse`.
- A partial resume failing at each of its four points (a PostToolUse abort
  for a supplied result, an event callback error, a PostGeneration abort, a
  `SaveSuspendedTurn` error): the session's suspension and pending set are
  unchanged, nothing is saved, `resp == nil`, and resubmitting the same
  results succeeds.
- `errors.As(err, &genErr)` holds, `genErr.Response == resp`, and the status
  is as expected on every exit class: PreGeneration error, PreIteration
  error, tool resolution error, model error, event callback error, hook
  aborts in PreToolUse, PostToolUse, Stop and PostGeneration, a full-resume
  failure before the model call, and a salvage save failure.
- Cancelled while text is streaming: the partial text is the last assistant
  message; with `DropPartialText` it is absent; a half-written `tool_use`
  block and an unsigned thinking block are dropped.
- Third model call fails with a provider error: the first two iterations'
  messages and results are saved, then `turn-failed` with the error;
  `CreateResponse` returns `ResponseStatusFailed` with the error, and
  `errors.As` still yields a `*GenerationError` whose `Response` is the same.
- Hook aborts in PreToolUse, Stop and PostGeneration: `Outcome.Hook` names
  the hook; the Stop and PostGeneration cases save the model's complete
  output.
- PreGeneration hook error: the input and the outcome are saved, nothing
  else.
- Soft cancel requested during a tool batch and during a model call: the
  running call finishes, later calls are not started, no further model call
  is made, status is cancelled, `errors.Is(err, context.Canceled)`.
- Resume cancelled after all pending calls were supplied: the session is no
  longer suspended, the caller-supplied results are in the saved turn, and
  `resp.Suspension.TurnMessages` is the closed turn. The existing
  `TestResumeContextCancelMidExecution` is this case (A supplied, B and C
  re-run in parallel and cancelled mid-execution) and its expectation flips:
  A's result saved, B and C interrupted with handles, the session no longer
  suspended; its old expectation moves under `IncompleteTurns.Discard`.
- `IncompleteTurns.Discard`: nothing saved, no not-run, interrupted or
  outcome items, the session stays suspended on a failed resume; the existing
  `TestGenerationErrorExposesPartialWork` becomes this test with the option
  set, and a new version of it without the option asserts the save.
- `OnIncompleteTurn`: rewriting `hctx.OutputMessages` changes what is saved;
  `Discard: true` saves nothing; the hook sees an uncancelled context; a
  reminder it records lands before the outcome reminder, which stays the last
  message, and the event still carries `Metadata["outcome"]`.
- Resumed session after each of the above: the next `CreateResponse` sends
  the saved turn, and a scripted model receives the reminder.
- Encoder backstop: a history with an unanswered call in the middle and one
  at the tail encodes on all four encoders with the inserted result, and the
  caller's messages are unchanged.
- Salvage context: a session whose `SaveTurn` returns `ctx.Err()` still
  saves; a session that blocks trips `SaveTimeout` and the error is joined.
- Cancellation during the final save of a completed turn saves the turn and
  returns completed.
- Reminder details round-trip through `Message.Copy` and `FileStore`, and a
  reminder with details decodes as a plain reminder when `Details` is
  ignored (the downgrade case).

### Documentation and changelog

`docs/guides/agents.md` gains an "Incomplete turns" section (statuses, the
returned response, `IncompleteTurns`, soft cancel, reading outcomes from
history). `docs/guides/hooks.md` adds `OnIncompleteTurn` and the new flow.
`docs/guides/suspend-resume.md` documents the cancelled-resume rule.
`docs/harness-features.md` replaces the "Partial-work recovery" bullet.
`GenerationError`'s doc comment loses its "intentionally NOT persisted"
paragraph. `CLAUDE.md`'s hook flow gains the cancel/failure line.

Changelog, under Changed:

> **Turns that are cancelled or fail are saved.** The session records what
> happened, closed so it can be sent again, with a `turn-canceled` or
> `turn-failed` reminder saying how it ended; `CreateResponse` returns the
> partial `Response` with the error. Set `AgentOptions.IncompleteTurns.Discard`
> to keep the old behaviour, or if your application saves incomplete turns
> itself.

Under Added: `WithSoftCancel`, `OnIncompleteTurn`, `TurnOutcome` and
`FindTurnOutcome`, `Reminder.Details`, `llm.AnswerUnansweredToolCalls` in
every encoder.

## Phase 2: additive follow-ups

None of these change v1.34's contracts. They can ship one at a time.

### 2a. Record the turn step by step

v1.34 saves at the end of the turn, on every exit. A process that dies
mid-turn still loses the turn. The proposal's "better still" is to write each
step as it finishes, which is what mobius-cloud and nvoken-cloud do in their
own tables.

`Session` is the stable core interface and cannot grow methods, so this is an
optional extension in the style of `SuspendableSession`:

```go
// TurnRecorder is an optional Session extension for recording a turn as it
// runs, so a process that ends mid-turn loses nothing. The agent uses it
// when the session implements it and falls back to SaveTurn otherwise.
type TurnRecorder interface {
    Session

    // BeginTurn opens a turn with the caller's input. From now on Messages
    // includes the open turn.
    BeginTurn(ctx context.Context, input []*llm.Message) error

    // RecordStep appends messages the open turn produced: an assistant
    // message after each model call, a tool-result message after each batch.
    // Usage is the step's own usage.
    RecordStep(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error

    // EndTurn closes the open turn. A nil outcome means it completed. The
    // messages are the closing messages the agent produced after the last
    // recorded step (an in-flight batch's results, the outcome reminder).
    EndTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage, outcome *TurnOutcome) error
}
```

Decisions this carries:

- **An open turn is closed on load.** A session opened with a turn that was
  never ended (the process died) closes it with `CloseTurn` and a new status,
  `ResponseStatusInterrupted` ("the process ended before the turn
  finished"), before returning from `Open`. Every unanswered call in the
  last recorded batch is listed as interrupted, never as not run, because the
  process may have run it. The agent applies the same check
  at the start of `CreateResponse` for a session it constructed in memory.
  The reminder name is `turn-interrupted`. This is why `TurnOutcome.Status`
  reuses `ResponseStatus`: the third value joins the same enum even though
  `CreateResponse` never returns it.
- **`FileStore` appends.** Steps are new JSONL line types (`turn_begin`,
  `step`, `turn_end`) folded into one event on read, so the hot path stays a
  single appended line, as `SaveTurn` is today. `putSession` rewrites are
  unaffected.
- **Suspension on an open turn** replaces the open turn with the suspended
  event, as `SaveSuspendedTurn` replaces the last event today; `EndTurn` is
  not called. `Compact` refuses while a turn is open, as it refuses while
  suspended.
- **Usage** is summed over the steps and the end, so `TotalUsage` holds.
- **Mid-turn compaction** does not affect what is recorded: steps come from
  the accumulator.

The interaction that needs care is the resume path, where the "open turn" is
the suspended event being replaced; the implementation plan should pin it
with the same invariants tests `agent_suspend_test.go` already has.

### 2b. Close a suspended turn without a model call

`CancelSuspension` removes the suspended turn as if it never happened, and
A2A's `Cancel` relies on that. What the proposal asks for is different: close
the turn, answer its pending calls as not run, record a cancelled outcome, and
keep everything that ran. That is an agent operation, because it goes through
the resume machinery and the session lock:

```go
// CancelSuspendedTurn closes the session's suspended turn without calling
// the model: every pending call is answered with ToolCallNotRunText, the
// completed siblings keep their results, and a turn-canceled outcome is
// recorded. The session is no longer suspended afterwards. Stateless callers
// pass WithMessages and WithResume(state, nil) and read the closed turn from
// Response.Suspension.TurnMessages. Returns ErrNoSuspendedTurn when there is
// nothing to close.
func (a *Agent) CancelSuspendedTurn(ctx context.Context, opts ...CreateResponseOption) (*Response, error)
```

It returns `ResponseStatusCanceled` with a nil error, the one place a
cancelled status comes without one, because nothing failed: the caller asked
for exactly this. That exception should be stated on `CreateResponse`'s
contract when this ships. `CancelSuspension` stays as the "forget it" path.

### 2c. Hide cancelled turns from the model

mobius-cloud leaves a cancelled turn out of the next request so the model
does not answer a request the person moved away from. In v1.34 the reminder's
last sentence covers that, and the two applications that want the turn sent
get it. If a Dive-session application wants hiding, it is a view policy on
`session.Session`, the same layer that decides the active window after
compaction: `Messages` skips events whose `Metadata["outcome"]` is
`"canceled"` when the policy is on, `AllMessages` keeps them. The agent does
not need to know. Deferred until an application on Dive's own sessions asks
for it.

## A later breaking iteration

When a major version is on the table, the outcome can move from the edges of
the contracts into them. Candidates, each independent:

- **`CreateResponse` reports how the turn ended through the response
  alone.** A non-nil error means the call did not run: validation, a session
  that could not load, a resume precondition. Everything after the turn
  begins is `Response.Status` with `Response.Outcome`, including a provider
  error, the way `http.Client.Do` returns a response for a 500. Callers stop
  writing `if err != nil` for outcomes they want to handle by status, and
  `GenerationError` goes away. `IncompleteTurns.Discard` goes with it; the
  hook covers the remaining cases.
- **`Session` becomes turn-shaped.** One write method instead of `SaveTurn`,
  `SaveSuspendedTurn`, `SaveResumedTurn` and the Phase 2 recorder:

  ```go
  type Turn struct {
      Input      []*llm.Message
      Output     []*llm.Message
      Usage      *llm.Usage
      Outcome    *TurnOutcome     // nil when completed
      Suspension *SuspensionState // non-nil when suspended
  }

  type Session interface {
      ID() string
      Messages(ctx context.Context) ([]*llm.Message, error)
      BeginTurn(ctx context.Context, input []*llm.Message) error
      RecordStep(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error
      EndTurn(ctx context.Context, turn *Turn) error
  }
  ```

  Sessions then know turn boundaries and outcomes as data rather than by
  reading the last message, and `SuspendableSession` folds into `EndTurn`
  with a `Suspension`.
- **One terminal stream item.** `ResponseItemTypeSuspended` and
  `ResponseItemTypeTurnOutcome` become `ResponseItemTypeTurnEnded`, emitted
  for every turn end including completion, carrying `Status`, `Suspension`
  and `Outcome`. Stream consumers get one end-of-stream signal.
- **The outcome as its own content type.** Once every deployed version can
  decode it, `llm.TurnOutcomeContent` replaces the reminder-with-details
  encoding, and `Message.UnmarshalJSON` learns to skip unknown block types the
  way `Response.UnmarshalJSON` already does, so the next new type is not a
  rollback hazard either.

## Decisions taken against the proposal

Where this design departs from the request, and why:

| Proposal                                          | Here                                                   | Reason                                                                                 |
| ------------------------------------------------- | ------------------------------------------------------ | -------------------------------------------------------------------------------------- |
| `turn-stopped` / `turn-failed`                    | `turn-canceled` / `turn-failed`                        | one word for the status, the reminder and the error (`context.Canceled`)               |
| Two not-run texts, by why the turn ended          | two texts, by what the call did (not run / interrupted) | the outcome reminder says why the turn ended; whether a call ran is the fact that matters |
| A call still running answered as not run          | answered as interrupted, result unknown; late result on `Response.BackgroundTasks` | a tool that ignores cancellation can commit after the save, so "no effect" would be false (review finding) |
| `TurnOutcomeContent` or reminder details          | reminder details                                       | older Dive versions must still open the session (rollbacks)                            |
| `OnIncompleteTurn func(msgs, err) msgs`           | a hook in `Hooks` with a decision                      | also covers notification and per-turn discard; composes through `Extension`            |
| `DiscardIncompleteTurns` on `AgentOptions`        | `IncompleteTurns.Discard` plus two more knobs           | the partial-text choice the applications differ on needs a home                        |
| Save step by step in the same release             | end-of-turn save now, recorder as Phase 2               | needs a new session extension and store format; the close logic is shared either way   |
| A stopped turn hidden from the next request       | Phase 2c, session-level                                 | a view policy, not an agent one; the reminder wording addresses the motivating case    |
| Stop at a step boundary                           | `WithSoftCancel` on the context                         | reaches subagents and tools; no sentinel through the callback                          |

## Open questions

1. **Should a soft cancel also stop between the calls of a sequential
   batch?** This design says yes (before each call not yet started), which
   ends sooner and leaves nothing half-done because an unstarted call has no
   effects. The alternative, finishing the batch, is what "step" suggests to
   some readers. Recommendation: before each call.
2. **PostToolUse hooks for results drained after cancellation.** Skipped
   here, since they would run with a cancelled context and their effects
   (`AdditionalContext`, reminders) are advisory. An application that needs
   them can watch `tool_call_result` items instead.
5. **A grace period after cancellation.** A parallel tool that honours its
   context returns within milliseconds of a cancel with its own "cancelled"
   result, but the drain loop returns at once, so that call is recorded as
   interrupted and its honest result only arrives through the handle. A
   short fixed wait, tens of milliseconds, after the cancel would record the
   tool's own result in most cases at the cost of that much stop latency.
   Not in this design, which keeps the 1.32.0 rule that cancelling stops
   waiting and leaves any wait to the application's `AwaitBackgroundTasks`
   deadline; worth measuring during implementation.
3. **Should `ResponseTimeout` be cancelled rather than failed?** Failed, in
   this design, with wording that says the time limit was reached. If an
   application uses a deadline as its stop button, it can treat
   `errors.Is(err, context.DeadlineExceeded)` however it likes; the recorded
   status is the model-facing one.
4. **Wording.** The two reminder texts above are a first draft; the names
   are the contract. Worth a pass against a few models before release,
   particularly whether "do that instead" makes a model too quick to abandon
   completed work.
