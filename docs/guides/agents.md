# Agent Guide

The `Agent` struct is the core building block of Dive applications. It manages LLM interactions, tool execution, and conversation flow.

## Creating an Agent

`NewAgent` returns an `*Agent` configured with the given options:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
)

func main() {
    agent, err := dive.NewAgent(dive.AgentOptions{
        Name:         "Assistant",
        SystemPrompt: "You are a helpful AI assistant.",
        Model:        anthropic.New(),
    })
    if err != nil {
        log.Fatal(err)
    }

    response, err := agent.CreateResponse(
        context.Background(),
        dive.WithInput("Hello!"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response.OutputText())
}
```

`OutputText` returns all the text of the turn's final assistant message. A
provider can split one answer into several text blocks, for example at
Anthropic citation boundaries or where Gemini attaches a thought signature.
`OutputText` joins adjacent fragments with no separator and skips empty
blocks, so you get the answer as the model wrote it. Separate passages, such
as text on either side of a reasoning block or a server tool call, are joined
with a blank line. Text from earlier messages in the turn, such as "Let me
check..." before a tool call, is not included. Read `response.OutputMessages`
for the full turn. `OutputText` is `llm.Message.AnswerText` applied to the
final message; call `AnswerText` directly on any model-written message (see
[Reading Message Text](llm-guide.md#reading-message-text)).

## AgentOptions

| Field                   | Type                    | Description                                      |
| ----------------------- | ----------------------- | ------------------------------------------------ |
| `Name`                  | `string`                | Agent identifier (for logging)                   |
| `SystemPrompt`          | `string`                | System prompt sent to the LLM                    |
| `Model`                 | `llm.LLM`               | LLM provider (required)                          |
| `Tools`                 | `[]Tool`                | Static tools available to the agent              |
| `Toolsets`              | `[]Toolset`             | Dynamic tool providers resolved per LLM request  |
| `Hooks`                 | `Hooks`                 | Hook functions grouped in a struct (see below)   |
| `Session`               | `Session`               | Persistent conversation state (see below)        |
| `ModelSettings`         | `*ModelSettings`        | Temperature, max tokens, reasoning, caching      |
| `ResponseTimeout`       | `time.Duration`         | Max time for a response (default: 30 min)        |
| `ToolIterationLimit`    | `int`                   | Max tool call iterations (default: 100)          |
| `ParallelToolExecution` | `bool`                  | Execute tool calls concurrently (default: false) |
| `IncompleteTurns`       | `IncompleteTurnOptions` | Turns that stop short (see below)                |
| `Durability`            | `DurabilityOptions`     | Step checkpoints and session claims (see below)  |

### Hooks Struct

The `Hooks` struct groups all hook slices:

| Field                | Type                       | Description                                       |
| -------------------- | -------------------------- | ------------------------------------------------- |
| `PreGeneration`      | `[]PreGenerationHook`      | Hooks called before LLM generation                |
| `PostGeneration`     | `[]PostGenerationHook`     | Hooks called after LLM generation                 |
| `PreToolUse`         | `[]PreToolUseHook`         | Hooks called before each tool execution           |
| `PostToolUse`        | `[]PostToolUseHook`        | Hooks called after each successful tool execution |
| `PostToolUseFailure` | `[]PostToolUseFailureHook` | Hooks called after each failed tool execution     |
| `Stop`               | `[]StopHook`               | Hooks called when agent is about to stop          |
| `PreIteration`       | `[]PreIterationHook`       | Hooks called before each LLM call in the loop     |
| `OnSuspend`          | `[]OnSuspendHook`          | Hooks called when a tool returns `SuspendResult`  |

## Generation Hooks

All hooks receive `*dive.HookContext`, which provides mutable access to:

- `Agent` - the agent running generation
- `Values` - arbitrary data shared between hooks (persists across all phases)
- `SystemPrompt` - modifiable system prompt (PreGeneration, PreIteration)
- `Messages` - modifiable message list (PreGeneration, PreIteration)
- `Response`, `OutputMessages`, `Usage` - generation results (PostGeneration, Stop)
- `Tool`, `Call` - tool details (PreToolUse, PostToolUse)
- `Result` - tool result (PostToolUse, PostToolUseFailure)

### PreGeneration

Runs before the LLM generation loop begins. Use it to load session history, inject context, or modify the system prompt:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        model,
    Hooks: dive.Hooks{
        PreGeneration: []dive.PreGenerationHook{
            func(ctx context.Context, hctx *dive.HookContext) error {
                hctx.SystemPrompt += "\nToday is Wednesday."
                return nil
            },
        },
    },
})
```

### PostGeneration

Runs after generation completes. Use it to save sessions, log results, or trigger side effects:

```go
Hooks: dive.Hooks{
    PostGeneration: []dive.PostGenerationHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            if hctx.Usage != nil {
                log.Printf("Tokens used: %d input, %d output", hctx.Usage.InputTokens, hctx.Usage.OutputTokens)
            }
            return nil
        },
    },
},
```

PostGeneration errors are logged but don't affect the returned `Response`, unless the hook returns a `*HookAbortError` (via `AbortGeneration()`), which aborts generation and returns an error.

### Stop Hook

Runs when the agent is about to stop responding. Can prevent stopping and continue generation:

```go
Hooks: dive.Hooks{
    Stop: []dive.StopHook{
        func(ctx context.Context, hctx *dive.HookContext) (*dive.StopDecision, error) {
            if hctx.StopHookActive {
                return nil, nil // prevent infinite loops
            }
            if !allTestsPassing() {
                return &dive.StopDecision{
                    Continue: true,
                    Reason:   "Tests are still failing. Please fix them.",
                }, nil
            }
            return nil, nil
        },
    },
},
```

When a Stop hook returns `Continue: true`, the `Reason` is injected as a user message and the generation loop re-enters. `hctx.StopHookActive` is true on subsequent stop checks.

### PreIteration Hook

Runs before each LLM call within the generation loop. Use it to modify the system prompt or messages between iterations:

```go
Hooks: dive.Hooks{
    PreIteration: []dive.PreIterationHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            // hctx.Iteration is the zero-based iteration number
            if hctx.Iteration > 5 {
                hctx.SystemPrompt += "\nPlease wrap up soon."
            }
            return nil
        },
    },
},
```

### Built-in Hook Helpers

- `dive.InjectContext(content...)` - Prepends content as a user message
- `dive.CompactionHook(threshold, summarizer)` - Triggers context compaction
- `dive.UsageLogger(logFunc)` - Logs token usage after generation
- `dive.UsageLoggerWithSlog(logger)` - Logs usage via slog
- `dive.MatchTool(pattern, hook)` - PreToolUse hook that only runs for matching tool names (regex)
- `dive.MatchToolPost(pattern, hook)` - PostToolUse hook that only runs for matching tool names (regex)
- `dive.MatchToolPostFailure(pattern, hook)` - PostToolUseFailure hook that only runs for matching tool names (regex)

## Tool Hooks

### PreToolUse

Runs before each tool execution. All hooks run in order. If any returns an error, the tool is denied. If all return nil, the tool is executed.

```go
Hooks: dive.Hooks{
    PreToolUse: []dive.PreToolUseHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            // Allow read-only tools
            if hctx.Tool.Annotations() != nil && hctx.Tool.Annotations().ReadOnlyHint {
                return nil
            }
            // Deny everything else
            return fmt.Errorf("tool %s requires approval", hctx.Tool.Name())
        },
    },
},
```

PreToolUse hooks can also:

- Set `hctx.UpdatedInput` to rewrite tool arguments before execution
- Set `hctx.AdditionalContext` to inject context into the tool result message

Use `dive.MatchTool(pattern, hook)` to run a hook only for specific tools:

```go
dive.MatchTool("Bash|Edit", func(ctx context.Context, hctx *dive.HookContext) error {
    // Only runs for Bash and Edit tools
    return nil
})
```

### PostToolUse

Runs after a tool call succeeds:

```go
Hooks: dive.Hooks{
    PostToolUse: []dive.PostToolUseHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            log.Printf("Tool %s succeeded", hctx.Tool.Name())
            return nil
        },
    },
},
```

### PostToolUseFailure

Runs after a tool call fails. This mirrors Claude Code's separate `PostToolUseFailure` event:

```go
Hooks: dive.Hooks{
    PostToolUseFailure: []dive.PostToolUseFailureHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            log.Printf("Tool %s failed: %v", hctx.Tool.Name(), hctx.Result.Error)
            return nil
        },
    },
},
```

## Event Callbacks

Use `WithEventCallback` to observe agent activity in real-time:

```go
response, err := agent.CreateResponse(ctx,
    dive.WithInput("Analyze this codebase"),
    dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
        switch item.Type {
        case dive.ResponseItemTypeMessage:
            // Complete assistant message
        case dive.ResponseItemTypeToolCall:
            fmt.Printf("Calling: %s\n", item.ToolCall.Name)
        case dive.ResponseItemTypeToolCallResult:
            // Tool result available
        case dive.ResponseItemTypeModelEvent:
            // Streaming event from LLM (for real-time UI)
            // Note: Event and Delta can be nil for non-delta events (e.g. ping, message_start)
            if item.Event != nil && item.Event.Delta != nil {
                fmt.Print(item.Event.Delta.Text)
            }
        }
        return nil
    }),
)
```

Every call that gets as far as the PreGeneration hooks ends with one
`ResponseItemTypeTurnEnded` item, whose `Turn` mirrors `Response.Turn`. It is
emitted after the session write, so a stream consumer can treat it as
end-of-stream. An error your callback returns for it is logged, not
returned.

## When a Turn Stops Short

Once a turn has begun (just before the PreGeneration hooks), `CreateResponse`
returns a `Response` even when it returns an error. A turn that stops before
it finishes has `Status == ResponseStatusIncomplete`, and
`Response.Turn.Outcome` says what stopped it and what can continue it:

```go
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
default:
    show(resp.OutputText())
}
```

`Outcome.Reason` says what happened. An error ended the turn for `canceled`,
`deadline`, `provider_error`, `stream_interrupted`, `hook_abort` (with
`Outcome.Hook`), `callback_error` and `error`: `err` is non-nil and wraps a
`*dive.GenerationError` whose `Response` is the same value. The model or its
provider stopped the turn short for `output_limit` (the response hit
`max_tokens`), `context_limit` (it filled the context window), `iteration_limit`
(the model still called tools at `ToolIterationLimit`), `provider_stopped`
(the provider ended the response early, or with a stop reason Dive does not
recognize while calling tools; `Outcome.Error` is the raw value) and `pause`
(a server tool loop paused more than ten times): `err` is nil.
`Outcome.Next` advises what the turn needs: `continue`, `reconcile` or
`input`.

The agent reads every response's stop reason before running its tools. A
response cut off at a limit, a refusal, and a provider stop run none of their
tool calls: a call whose input was cut off is dropped from the message, and
the others are answered "not run". A refusal is still a completed turn;
`Response.StopReason` and `Response.StopDetails` say it was one. A paused
server tool loop (`pause_turn`) is sent again, up to ten times. A stream that
ends before its end marker is interrupted, and `Outcome.UsageUnknown` is set
when no usage arrived.

**The turn is kept.** Before an incomplete turn is returned, it is closed:
every tool call is answered, the text the model was still writing is kept as
the last assistant message (a half-written tool call or thinking block is
dropped), and a `turn-incomplete` reminder recording the outcome is the last
message. Then `OnIncompleteTurn` hooks run, and the turn is saved to the
session like any other, so the next call sends it and the model knows how the
last turn ended. `Response.Turn.Messages` is the closed turn, and a stateless
caller appends it to its history for every status. Read an outcome back from
history with `dive.FindTurnOutcome` or `dive.FindLatestTurnOutcome`, and close
turns you keep yourself with `dive.CloseTurn`.

`Response.Turn.Persistence` is `saved` when the session acknowledged the
write, `none` when nothing was saved, `failed` when the session refused the
write before writing (its error wraps `dive.ErrSaveRejected`), and `unknown`
for any other error: reload the session before acting on it. A save error is
returned with the response, joined with the turn's own error. Every session
write at the end of a call runs on a context without the cancellation that may
have ended the turn, bounded by `IncompleteTurns.SaveTimeout` (30 seconds by
default).

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Model:   model,
    Session: sess,
    IncompleteTurns: dive.IncompleteTurnOptions{
        DropPartialText: true,             // leave out text still streaming
        SaveTimeout:     10 * time.Second, // bound the final session write
        // Discard: true,                  // the pre-v1.34 behaviour for error exits
    },
})
```

`Discard` restores the old behaviour for a turn that ends in an error: nothing
is saved, no `OnIncompleteTurn` hook runs, no not-run, unknown or `turn_ended`
item is emitted, and a failed resume leaves the session suspended. Use it
while your application still saves incomplete turns itself. A turn the model
stopped short is saved either way.

**Continuing.** `dive.WithContinue()` calls the model again on the history as
it stands, with no new input, so a stopped, failed or cut-off turn is picked
up without running any tool again. On a session that implements
`dive.TurnStore`, as `session.Session` does, the continuation folds into the
incomplete turn: the model sees the turn without its outcome reminder, and
the turn is saved again, same ID, with the new output, so no note is left in
the history. `Response.Turn.Messages` is then the whole turn. On any other
session the continuation is saved as its own turn, and the request carries a
model-only reminder saying the user asked to continue. A stateless caller
passes its history with `WithMessages` and appends `Response.Turn.Messages`.
When the history ends in an assistant message, as after an output limit, a
`turn-continue` reminder is recorded before the new output, so the history
keeps alternating roles.

```go
resp, err = agent.CreateResponse(ctx, dive.WithContinue())
```

A `context_limit` turn cannot be sent again as it stands: shorten the history
first (`session.Compact`, or a PreGeneration hook that trims it).

New input after an incomplete turn starts a new turn, and the model sees the
incomplete one with its outcome. Set `IncompleteTurns.RequireReconcile` to
refuse new input with `dive.ErrUnreconciledToolCalls` while the last turn has
a call whose result is unknown (`Outcome.Next == reconcile`), until it is
continued or removed. On a `TurnStore` the latest turn record decides, so
compacting the session does not clear it.

A turn that stops during a tool batch answers every call of the batch. A call
whose tool returned keeps its result. A call that never started is answered
with `dive.ToolCallNotRunText`: it had no effect. A parallel call that was still
running is answered with `dive.ToolCallUnknownText`: it may have taken effect.
The agent does not wait for it; its handle is on `Response.BackgroundTasks`,
and `dive.AwaitBackgroundTasks` and `dive.WithBackgroundResults` deliver its
result to the model later, as for a background tool. `Outcome.ToolCalls`
records each call's state, and `Outcome.Next` is `reconcile` when an unknown
call's tool is not annotated `ReadOnlyHint`. Every `tool_call` item gets a
`tool_call_result` item, with `Error` set to `dive.ErrToolCallNotRun` or
`dive.ErrToolCallUnknown` for these answers. A result that lands after the
turn stopped is kept without PostToolUse hooks.

### Stopping at a Step Boundary

`dive.WithSoftCancel` asks a run to stop at its next step boundary rather
than mid-call: before its next model call, and before each tool call it has
not started. Calls already running finish, so a soft cancel never leaves a
call unknown. The turn ends incomplete with reason `canceled`, and
`errors.Is(err, context.Canceled)` holds. A stop button can escalate:

```go
ctx, cancel := context.WithCancel(ctx)
ctx, softCancel := dive.WithSoftCancel(ctx)
// First press: softCancel(). Second press: cancel(), which stops at once.
```

The request travels with the context, so subagents stop at their own
boundaries, and a long-running tool can check `dive.SoftCanceled(ctx)`.

## CreateResponse Options

| Option                       | Description                                                 |
| ---------------------------- | ----------------------------------------------------------- |
| `WithInput(text)`            | Simple text input (creates a user message)                  |
| `WithMessages(msgs...)`      | Multiple messages                                           |
| `WithEventCallback(fn)`      | Receive events during generation                            |
| `WithSession(sess)`          | Per-call session override                                   |
| `WithPromptCacheKey(key)`    | Stable conversation key for provider prompt-cache routing   |
| `WithModelOnlyReminder(r)`   | Append a reminder for this response without recording it    |
| `WithValue(key, val)`        | Pass data to hooks via HookContext.Values                   |
| `WithToolResults(results)`   | Resume a session-backed suspended turn (see suspend-resume) |
| `WithResume(state, results)` | Resume statelessly with an explicit `SuspensionState`       |

## Runtime Context

Use typed reminders when the application needs to supply context the user did
not type. `WithModelOnlyReminder` appends contextual or operator state for one
`CreateResponse` call without recording it. Hooks call `hctx.AppendReminder`
with `Recorded` or `ModelOnly` to choose whether an appended reminder enters
conversation history.

Reminder tier and lifetime are separate choices. Operator reminders receive a
native operator role only where the target and message placement are known to
support it; otherwise Dive renders the same typed reminder as tagged user
content. See [Runtime Context and System Reminders](context-injection.md) for
the decision table, examples, provider behavior, and persistence rules.

## Model Settings

Fine-tune LLM behavior per agent:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a creative writer.",
    Model:        anthropic.New(),
    ModelSettings: &dive.ModelSettings{
        Temperature:     dive.Ptr(0.9),
        MaxTokens:       dive.Ptr(4000),
        ReasoningBudget: dive.Ptr(50000),
        Caching:         dive.Ptr(true),
    },
})
```

## Sessions

Sessions provide persistent conversation state across multiple `CreateResponse` calls. The agent automatically loads history before generation and saves new messages after.

### In-memory session

```go
sess := session.New("my-session")
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        anthropic.New(),
    Session:      sess,
})

// Turn 1 — history is empty
resp, _ := agent.CreateResponse(ctx, dive.WithInput("Hi, my name is Alice."))
fmt.Println(resp.OutputText())

// Turn 2 — history loaded automatically
resp, _ = agent.CreateResponse(ctx, dive.WithInput("What's my name?"))
fmt.Println(resp.OutputText()) // "Your name is Alice."
```

### Persistent session with a store

```go
home, _ := os.UserHomeDir()
store, _ := session.NewFileStore(filepath.Join(home, ".myapp", "sessions"))
sess, _ := store.Open(ctx, "my-session")

agent, _ := dive.NewAgent(dive.AgentOptions{
    Model:   anthropic.New(),
    Session: sess,
})
```

### Per-call session override

In server scenarios where one agent serves many users, override the session per call:

```go
resp, _ := agent.CreateResponse(ctx,
    dive.WithInput("Hello"),
    dive.WithSession(userSession),
)
```

### Turn records

`session.Session` implements `dive.TurnStore`: it stores each turn as a
record with an ID, a status, its outcome and the state of every tool call,
and advances a revision on every write. The agent loads from it and
checkpoints the turn at the end of every call.

```go
snap, _ := sess.Load(ctx)      // History, OpenTurn (suspended, incomplete or running), Revision
turns, _ := sess.Turns(ctx)    // every turn's record; superseded incomplete turns marked
err := sess.RemoveLastTurn(ctx) // delete the last turn, whatever its state
```

`CheckpointTurn(ctx, expectedRevision, turn)` imports a turn you hold, such
as a `Response.Turn` from another process; it fails with
`dive.ErrRevisionConflict` when the session changed since you read it. A
wrapper that embeds `*session.Session` to intercept `SaveTurn` or
`SaveSuspendedTurn` must intercept `CheckpointTurn` too, which the agent
calls instead.

### Durable turns

By default a turn is recorded once, when the call ends, so a process that
exits mid-turn loses it. `DurabilityOptions.CheckpointSteps` records the turn
as it runs, with status `running`: before the first model call, after each
model response whose tool calls will run, as each call starts
(`ToolCallStateRunning`) and as each result arrives. A step checkpoint that
fails stops the turn before that step, so no tool runs unrecorded.
`FileStore` appends one line per step.

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Model:      model,
    Session:    sess, // a dive.TurnStore, such as a session.Session
    Durability: dive.DurabilityOptions{CheckpointSteps: true},
})
```

The next call on a session whose last turn is `running` closes that turn
first: it becomes incomplete with `TurnReasonProcessExit`, its running calls
`unknown`, and `Next` is `reconcile` unless every one of them is read-only.
Continue it with `WithContinue`, or check the unknown calls first. A tool
that ran between its `running` checkpoint and its result's is the window
that remains: `dive.TurnID(ctx)` and `dive.ToolCallID(ctx)` give it a stable
key for services that accept idempotency keys.

Several processes can share one session when each claims it.
`DurabilityOptions.Claim` claims the session (`dive.SessionClaimer`) before a
call loads it, renews the claim every third of `ClaimTTL` while the call
runs, and releases it at the end. A call on a session another owner holds
fails with `dive.ErrSessionClaimed` before doing anything; a call whose claim
cannot be renewed is cancelled, and its error wraps `ErrSessionClaimed`. A
`FileStore` keeps the claim in a file next to the session's, so processes
sharing its directory see it, and a newly claimed session reads back what
other processes wrote. A claim left by a process that exited expires after
`ClaimTTL`, so another process can then take the session and recover its
running turn.

```go
Durability: dive.DurabilityOptions{
    CheckpointSteps: true,
    Claim:           true,
    ClaimTTL:        30 * time.Second, // the default
},
```

### Fork and compact

```go
// Fork a conversation up to its last completed turn
forked := sess.Fork("new-branch")
store.Put(ctx, forked)

// Include an open turn: an incomplete one as it is, a suspended or running one closed
forked = sess.Fork("with-open-turn", session.ForkWithOpenTurn())

// Compact history with a summarizer
sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
    // Use an LLM to summarize, or implement custom logic
    return summarize(ctx, msgs)
})
```

## Multi-Turn Without Sessions

If you prefer manual message management, agents are stateless by default. Accumulate messages using `response.OutputMessages`:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        anthropic.New(),
})

var messages []*llm.Message

// First turn
resp, _ := agent.CreateResponse(ctx, dive.WithInput("Hi, my name is Alice."))
messages = append(messages, llm.NewUserTextMessage("Hi, my name is Alice."))
messages = append(messages, resp.OutputMessages...)

// Second turn
messages = append(messages, llm.NewUserTextMessage("What's my name?"))
resp, _ = agent.CreateResponse(ctx, dive.WithMessages(messages...))
```

`OutputMessages` includes both assistant messages and tool result messages in the correct order.

## Suspend and Resume

Agents can pause mid-turn while a tool waits on an external input —
human approval, webhook callback, form submission — and resume cleanly
later, including across process restarts. A tool signals suspension by
returning `dive.NewSuspendResult`, and the caller reads
`resp.Status == dive.ResponseStatusSuspended` plus `resp.Suspension` to
learn what's pending:

```go
resp, _ := agent.CreateResponse(ctx, dive.WithInput("Please deploy v1.4.2"))
if resp.Status == dive.ResponseStatusSuspended {
    for _, p := range resp.Suspension.PendingToolCalls {
        // route p.ID + p.Input + p.Prompt to a review queue and return
    }
    return
}

// Later, possibly in another process:
final, _ := agent.CreateResponse(ctx, dive.WithToolResults(map[string]*dive.ToolResult{
    pendingID: dive.NewToolResultText("approved"),
}))
```

See the [Suspend & Resume Guide](suspend-resume.md) for the full flow,
including `OnSuspend` hooks, stateless resume with `WithResume`, partial
resumes, and the streaming `ResponseItemTypeSuspended` terminator.

## Subagents

Subagent support is available in `experimental/subagent/`. See the experimental packages for details.

## Next Steps

- [Tools Guide](tools.md) - Built-in tools
- [Custom Tools](custom-tools.md) - Create your own tools
- [Runtime Context](context-injection.md) - Inject typed system reminders
- [Suspend & Resume](suspend-resume.md) - Pause mid-turn for human input or async callbacks
- [LLM Guide](llm-guide.md) - Provider configuration
