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

## AgentOptions

| Field                | Type                   | Description                                       |
| -------------------- | ---------------------- | ------------------------------------------------- |
| `Name`               | `string`               | Agent identifier (for logging)                    |
| `SystemPrompt`       | `string`               | System prompt sent to the LLM                     |
| `Model`              | `llm.LLM`              | LLM provider (required)                           |
| `Tools`              | `[]Tool`               | Tools available to the agent                      |
| `Hooks`              | `Hooks`                | Hook functions grouped in a struct (see below)    |
| `ModelSettings`      | `*ModelSettings`       | Temperature, max tokens, reasoning, caching       |
| `ResponseTimeout`    | `time.Duration`        | Max time for a response (default: 30 min)         |
| `ToolIterationLimit` | `int`                  | Max tool call iterations (default: 100)           |

### Hooks Struct

The `Hooks` struct groups all hook slices:

| Field            | Type                   | Description                                       |
| ---------------- | ---------------------- | ------------------------------------------------- |
| `PreGeneration`  | `[]PreGenerationHook`  | Hooks called before LLM generation                |
| `PostGeneration` | `[]PostGenerationHook` | Hooks called after LLM generation                 |
| `PreToolUse`     | `[]PreToolUseHook`     | Hooks called before each tool execution           |
| `PostToolUse`        | `[]PostToolUseHook`        | Hooks called after each successful tool execution |
| `PostToolUseFailure` | `[]PostToolUseFailureHook` | Hooks called after each failed tool execution     |
| `Stop`               | `[]StopHook`               | Hooks called when agent is about to stop          |
| `PreIteration`   | `[]PreIterationHook`   | Hooks called before each LLM call in the loop     |

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

## CreateResponse Options

| Option                  | Description                                |
| ----------------------- | ------------------------------------------ |
| `WithInput(text)`       | Simple text input (creates a user message) |
| `WithMessages(msgs...)` | Multiple messages                          |
| `WithEventCallback(fn)` | Receive events during generation           |

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

## Multi-Turn Conversations

Agents are stateless. To maintain a conversation across calls, accumulate messages using `response.OutputMessages`:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        anthropic.New(),
})

var messages []*llm.Message

// First turn
resp, _ := agent.CreateResponse(ctx, dive.WithInput("Hi, my name is Alice."))
fmt.Println(resp.OutputText())

// Build history: input + output
messages = append(messages, llm.NewUserTextMessage("Hi, my name is Alice."))
messages = append(messages, resp.OutputMessages...)

// Second turn
messages = append(messages, llm.NewUserTextMessage("What's my name?"))
resp, _ = agent.CreateResponse(ctx, dive.WithMessages(messages...))
fmt.Println(resp.OutputText())
```

`OutputMessages` includes both assistant messages and tool result messages in the correct order. This is important when the agent uses tools — using `response.Items` alone will miss the tool result messages that the LLM needs to see.

For persistent sessions across process restarts, see the experimental `session` package which provides hook-based session save/load.

## Subagents

Subagent support is available in `experimental/subagent/`. See the experimental packages for details.

## Next Steps

- [Tools Guide](tools.md) - Built-in tools
- [Custom Tools](custom-tools.md) - Create your own tools
- [LLM Guide](llm-guide.md) - Provider configuration
