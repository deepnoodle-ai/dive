# Quick Start Guide

Get up and running with Dive in a few minutes.

## Prerequisites

- **Go 1.25** or later
- An API key from at least one LLM provider:
  - [Anthropic](https://console.anthropic.com/) (`ANTHROPIC_API_KEY`)
  - [OpenAI](https://platform.openai.com/api-keys) (`OPENAI_API_KEY`)
  - Or [Ollama](https://ollama.ai/) running locally (no key needed)

## Your First Agent

### 1. Initialize Your Project

```bash
mkdir my-dive-project && cd my-dive-project
go mod init my-dive-project
go get github.com/deepnoodle-ai/dive
```

### 2. Create `main.go`

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
        Name:         "My Assistant",
        SystemPrompt: "You are a helpful AI assistant who provides clear, concise answers.",
        Model:        anthropic.New(),
    })
    if err != nil {
        log.Fatal(err)
    }

    response, err := agent.CreateResponse(
        context.Background(),
        dive.WithInput("What is artificial intelligence?"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response.OutputText())
}
```

### 3. Run It

```bash
export ANTHROPIC_API_KEY="your-key-here"
go run main.go
```

## Adding Tools

Give your agent the ability to read files and search the web:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
    "github.com/deepnoodle-ai/dive/toolkit"
)

func main() {
    agent, err := dive.NewAgent(dive.AgentOptions{
        Name:         "Research Assistant",
        SystemPrompt: "You help research topics and save findings to files.",
        Model:        anthropic.New(),
        Tools: []dive.Tool{
            toolkit.NewReadFileTool(),
            toolkit.NewWriteFileTool(),
            toolkit.NewGlobTool(),
            toolkit.NewGrepTool(),
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    response, err := agent.CreateResponse(
        context.Background(),
        dive.WithInput("List all Go files in the current directory"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response.OutputText())
}
```

## Streaming with Event Callbacks

Use `WithEventCallback` to receive events as the agent works:

```go
response, err := agent.CreateResponse(ctx,
    dive.WithInput("Analyze the project structure"),
    dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
        switch item.Type {
        case dive.ResponseItemTypeToolCall:
            fmt.Printf("Calling tool: %s\n", item.ToolCall.Name)
        case dive.ResponseItemTypeModelEvent:
            // Handle streaming text deltas for real-time UI
        }
        return nil
    }),
)
```

## Next Steps

- [Agents Guide](agents.md) - Hooks, event handling, and advanced configuration
- [Tools Guide](tools.md) - All built-in tools
- [Custom Tools](custom-tools.md) - Build your own tools
- [LLM Guide](llm-guide.md) - Work with different providers
