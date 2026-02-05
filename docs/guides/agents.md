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
| `PreGeneration`      | `[]PreGenerationHook`  | Hooks called before LLM generation                |
| `PostGeneration`     | `[]PostGenerationHook` | Hooks called after LLM generation                 |
| `PreToolUse`         | `[]PreToolUseHook`     | Hooks called before each tool execution           |
| `PostToolUse`        | `[]PostToolUseHook`    | Hooks called after each tool execution            |
| `Confirmer`          | `ConfirmToolFunc`      | Called when a PreToolUse hook returns `AskResult` |
| `ModelSettings`      | `*ModelSettings`       | Temperature, max tokens, reasoning, caching       |
| `ResponseTimeout`    | `time.Duration`        | Max time for a response (default: 30 min)         |
| `ToolIterationLimit` | `int`                  | Max tool call iterations (default: 100)           |

## Generation Hooks

### PreGeneration

Runs before the LLM is called. Use it to load session history, inject context, or modify the system prompt:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        model,
    PreGeneration: []dive.PreGenerationHook{
        func(ctx context.Context, state *dive.GenerationState) error {
            // Load session history
            if history, ok := loadHistory(state.SessionID); ok {
                state.Messages = append(history, state.Messages...)
            }
            return nil
        },
    },
})
```

The `GenerationState` provides mutable access to:

- `SessionID`, `UserID` - identifiers
- `SystemPrompt` - modifiable system prompt
- `Messages` - modifiable message list
- `Values` - arbitrary data shared between hooks

### PostGeneration

Runs after generation completes. Use it to save sessions, log results, or trigger side effects:

```go
PostGeneration: []dive.PostGenerationHook{
    func(ctx context.Context, state *dive.GenerationState) error {
        // Save session
        return saveHistory(state.SessionID, state.Messages, state.OutputMessages)
    },
},
```

PostGeneration errors are logged but don't affect the returned `Response`.

### Built-in Hook Helpers

- `dive.InjectContext(content...)` - Prepends content as a user message
- `dive.CompactionHook(threshold, summarizer)` - Triggers context compaction
- `dive.UsageLogger(logFunc)` - Logs token usage after generation
- `dive.UsageLoggerWithSlog(logger)` - Logs usage via slog

## Tool Hooks

### PreToolUse

Runs before each tool execution. Returns one of:

- `AllowResult()` - execute the tool
- `DenyResult(msg)` - reject with a message
- `AskResult(msg)` - prompt the user for confirmation
- `ContinueResult()` - defer to the next hook

```go
PreToolUse: []dive.PreToolUseHook{
    func(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
        // Allow read-only tools automatically
        if hookCtx.Tool.Annotations() != nil && hookCtx.Tool.Annotations().ReadOnlyHint {
            return dive.AllowResult(), nil
        }
        // Ask for confirmation on other tools
        return dive.AskResult("Execute this tool?"), nil
    },
},
```

### PostToolUse

Runs after tool execution. Can modify the result before it's sent to the LLM:

```go
PostToolUse: []dive.PostToolUseHook{
    func(ctx context.Context, hookCtx *dive.PostToolUseContext) error {
        log.Printf("Tool %s completed", hookCtx.Tool.Name())
        return nil
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
        case dive.ResponseItemTypeInit:
            fmt.Printf("Session: %s\n", item.Init.SessionID)
        case dive.ResponseItemTypeMessage:
            // Complete assistant message
        case dive.ResponseItemTypeToolCall:
            fmt.Printf("Calling: %s\n", item.ToolCall.Name)
        case dive.ResponseItemTypeToolCallResult:
            // Tool result available
        case dive.ResponseItemTypeModelEvent:
            // Streaming event from LLM (for real-time UI)
        case dive.ResponseItemTypeTodo:
            // Todo list updated
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

## Subagents

Subagent support is available in `experimental/subagent/`. See the experimental packages for details.

## Next Steps

- [Tools Guide](tools.md) - Built-in tools
- [Custom Tools](custom-tools.md) - Create your own tools
- [LLM Guide](llm-guide.md) - Provider configuration
