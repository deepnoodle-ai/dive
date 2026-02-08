# Hooks Guide

Hooks are Go functions that run at specific points in an agent's generation
loop. Use them to inject context, enforce permissions, log usage, control
stopping behavior, and react to tool outcomes.

All hooks receive `*dive.HookContext` and are registered via the `Hooks` struct
on `AgentOptions`.

## Quick Start

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        anthropic.New(),
    Hooks: dive.Hooks{
        PreGeneration: []dive.PreGenerationHook{
            func(ctx context.Context, hctx *dive.HookContext) error {
                hctx.SystemPrompt += "\nToday is Monday."
                return nil
            },
        },
        PostGeneration: []dive.PostGenerationHook{
            dive.UsageLogger(func(usage *llm.Usage) {
                log.Printf("Tokens: %d in, %d out", usage.InputTokens, usage.OutputTokens)
            }),
        },
    },
})
```

## Hook Flow

```
PreGeneration → [PreIteration → LLM → PreToolUse → Execute → PostToolUse / PostToolUseFailure]* → Stop → PostGeneration
```

1. **PreGeneration** runs once before the loop starts.
2. Inside the loop, **PreIteration** runs before each LLM call.
3. After the LLM responds with tool calls, **PreToolUse** runs before each tool, then
   **PostToolUse** (success) or **PostToolUseFailure** (failure) runs after.
4. When the loop exits, **Stop** hooks can force re-entry.
5. **PostGeneration** runs last.

## HookContext

All hooks receive `*HookContext`. Which fields are populated depends on the phase:

| Field            | PreGen | PostGen | PreToolUse | PostToolUse | PostToolUseFailure | Stop | PreIter |
| :--------------- | :----: | :-----: | :--------: | :---------: | :----------------: | :--: | :-----: |
| Agent            |   ✓    |    ✓    |     ✓      |      ✓      |         ✓          |  ✓   |    ✓    |
| Values           |   ✓    |    ✓    |     ✓      |      ✓      |         ✓          |  ✓   |    ✓    |
| SystemPrompt     |   ✓    |    ✓    |            |             |                    |      |    ✓    |
| Messages         |   ✓    |    ✓    |            |             |                    |      |    ✓    |
| Response         |        |    ✓    |            |             |                    |  ✓   |         |
| OutputMessages   |        |    ✓    |            |             |                    |  ✓   |         |
| Usage            |        |    ✓    |            |             |                    |  ✓   |         |
| Tool             |        |         |     ✓      |      ✓      |         ✓          |      |         |
| Call             |        |         |     ✓      |      ✓      |         ✓          |      |         |
| Result           |        |         |            |      ✓      |         ✓          |      |         |
| UpdatedInput     |        |         |     ✓      |             |                    |      |         |
| AdditionalContext|        |         |     ✓      |      ✓      |         ✓          |      |         |
| StopHookActive   |        |         |            |             |                    |  ✓   |         |
| Iteration        |        |         |            |             |                    |      |    ✓    |

The `Values` map persists across all phases within one `CreateResponse` call, so
hooks can pass data to each other.

## Generation Hooks

### PreGeneration

Runs once before the generation loop. Use it to load session history, inject
context, or modify the system prompt. Errors abort generation.

```go
Hooks: dive.Hooks{
    PreGeneration: []dive.PreGenerationHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            // Load session from database
            messages, _ := loadSession(ctx, sessionID)
            hctx.Messages = append(messages, hctx.Messages...)
            return nil
        },
    },
},
```

### PostGeneration

Runs after the generation loop completes (and after Stop hooks). Use it to
save sessions, log results, or trigger side effects. Errors are logged but
don't affect the returned `Response`.

```go
Hooks: dive.Hooks{
    PostGeneration: []dive.PostGenerationHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            return saveSession(ctx, sessionID, hctx.OutputMessages)
        },
    },
},
```

### PreIteration

Runs before each LLM call within the loop. Use it to update the system prompt
or messages between iterations. Errors abort generation.

```go
Hooks: dive.Hooks{
    PreIteration: []dive.PreIterationHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            if hctx.Iteration > 10 {
                hctx.SystemPrompt += "\nPlease wrap up your work soon."
            }
            return nil
        },
    },
},
```

## Tool Hooks

### PreToolUse

Runs before each tool call. All hooks run in order. If any returns an error,
the tool is denied and the error message is sent to the LLM.

```go
Hooks: dive.Hooks{
    PreToolUse: []dive.PreToolUseHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            if hctx.Tool.Annotations() != nil && hctx.Tool.Annotations().ReadOnlyHint {
                return nil
            }
            return fmt.Errorf("tool %s requires approval", hctx.Tool.Name())
        },
    },
},
```

PreToolUse hooks can also:

- **Rewrite input**: Set `hctx.UpdatedInput` to replace tool arguments before execution.
- **Inject context**: Set `hctx.AdditionalContext` to append text to the tool result message.

### PostToolUse

Runs after a tool call **succeeds**. Use it for logging, metrics, or result
transformation.

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

Runs after a tool call **fails** (tool returned an error, or the result has
`IsError` set). Use it for error diagnostics, failure alerts, or injecting
recovery guidance.

```go
Hooks: dive.Hooks{
    PostToolUseFailure: []dive.PostToolUseFailureHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            log.Printf("Tool %s failed: %v", hctx.Tool.Name(), hctx.Result.Error)
            hctx.AdditionalContext = "The tool failed. Try a different approach."
            return nil
        },
    },
},
```

This separation mirrors Claude Code's distinct `PostToolUse` and
`PostToolUseFailure` events.

## Stop Hook

Runs when the agent is about to stop responding. A hook can return
`Continue: true` to inject a reason as a user message and re-enter the
generation loop.

```go
Hooks: dive.Hooks{
    Stop: []dive.StopHook{
        func(ctx context.Context, hctx *dive.HookContext) (*dive.StopDecision, error) {
            if hctx.StopHookActive {
                return nil, nil // already continued once, let it stop
            }
            if !allTestsPassing() {
                return &dive.StopDecision{
                    Continue: true,
                    Reason:   "Tests are still failing. Keep working.",
                }, nil
            }
            return nil, nil
        },
    },
},
```

Check `hctx.StopHookActive` to prevent infinite loops — it's true when the
current stop check was triggered by a previous continuation.

## Hook Helpers

Built-in helpers reduce boilerplate:

### Generation helpers

```go
// Prepend content as a user message before generation
dive.InjectContext(
    llm.NewTextContent("Working directory: /home/user/project"),
)

// Trigger compaction when message count exceeds threshold
dive.CompactionHook(50, summarizer)

// Log token usage after generation
dive.UsageLogger(func(usage *llm.Usage) { ... })

// Log token usage via slog
dive.UsageLoggerWithSlog(logger)
```

### Tool name matchers

Filter hooks to specific tools using a Go regexp pattern:

```go
// Only run for Bash and Edit tools
dive.MatchTool("Bash|Edit", preToolUseHook)

// Only log successful Bash calls
dive.MatchToolPost("Bash", postToolUseHook)

// Only alert on Bash failures
dive.MatchToolPostFailure("Bash", postToolUseFailureHook)
```

The pattern is compiled once when the helper is called, not on every
invocation.

## Error Handling

How errors are handled depends on the hook type:

| Hook type            | Regular error              | `*HookAbortError`       |
| :------------------- | :------------------------- | :---------------------- |
| PreGeneration        | Aborts generation          | Aborts generation       |
| PostGeneration       | Logged, response preserved | Aborts, returns error   |
| PreToolUse           | Denies tool call           | Aborts generation       |
| PostToolUse          | Logged, result preserved   | Aborts generation       |
| PostToolUseFailure   | Logged, result preserved   | Aborts generation       |
| Stop                 | Logged, continues          | Aborts generation       |
| PreIteration         | Aborts generation          | Aborts generation       |

Use `dive.AbortGeneration("reason")` to create a `*HookAbortError` when a
critical failure should stop generation entirely:

```go
func(ctx context.Context, hctx *dive.HookContext) error {
    if isSafetyViolation(hctx) {
        return dive.AbortGeneration("safety policy violation")
    }
    return nil
}
```

## Combining Hooks

Hooks compose naturally. Register multiple hooks of the same type and they run
in order:

```go
Hooks: dive.Hooks{
    PreToolUse: []dive.PreToolUseHook{
        permission.Hook(config, dialog),               // check permissions
        dive.MatchTool("Bash", validateBashCommand),    // validate Bash specifically
    },
    PostToolUse: []dive.PostToolUseHook{
        logToolCall,                                    // log all successes
        dive.MatchToolPost("Edit|Write", formatCode),   // format after edits
    },
    PostToolUseFailure: []dive.PostToolUseFailureHook{
        alertOnFailure,                                 // alert on any failure
    },
},
```

## Next Steps

- [Agents Guide](agents.md) — Full agent configuration reference
- [Custom Tools](custom-tools.md) — Build tools that hooks can intercept
- [Permissions Guide](experimental/permissions.md) — Hook-based permission system
- [Compaction Guide](experimental/compaction.md) — Hook-based context compaction
