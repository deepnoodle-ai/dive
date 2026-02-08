# Dive

[![CI](https://github.com/deepnoodle-ai/dive/actions/workflows/go-test.yml/badge.svg)](https://github.com/deepnoodle-ai/dive/actions/workflows/go-test.yml)
[![Go.Dev reference](https://img.shields.io/badge/go.dev-reference-blue?logo=go&logoColor=white)](https://pkg.go.dev/github.com/deepnoodle-ai/dive)
[![Apache-2.0 license](https://img.shields.io/badge/License-Apache%202.0-brightgreen.svg)](https://opensource.org/licenses/Apache-2.0)

Dive is a foundational Go library for building AI agents and LLM-powered applications.

Dive gives you three main things: consistent access to 8+ LLM providers, a
tool-calling system, and a robust agent loop with hooks. Images, documents, local
tools, MCP tools, and structured output all work across providers. Most other libraries
have gaps on this front. The agent runs the generate-call-repeat loop for you,
with hooks to intercept before and after each step. Tools and hooks are the
primary extension points.

The built-in toolkit includes Read, Write, Edit, Glob, Grep, Bash, and more.
Use all of them, some of them, or bring your own. The built-in tools align with
Claude Code's patterns, so you benefit from any model tuning that Anthropic has
done for these tool shapes.

Dive is unopinionated. You provide the system prompt and decide which tools and
hooks to install. Your agents do what you tell them. There are no hidden prompts
or library-imposed behaviors.

Use the LLM layer when you want direct access to model capabilities. Use the
agent layer when you want the tool-calling loop handled for you. Use Dive to
build CLIs, add AI to back-end SaaS services, or run agents within a workflow
orchestrator.

Everything _outside_ the `experimental/` directory is stable, while everything
_inside_ `experimental/` may change. The experimental packages add more tools,
permissions, sessions, and a CLI similar to Claude Code. Use experimental code
as inspiration, copy and modify it, or use it directly.

Dive is developed by [Deep Noodle](https://deepnoodle.ai) and is used in
multiple production AI deployments.

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a senior software engineer.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        toolkit.NewReadFileTool(),
        toolkit.NewTextEditorTool(),
        toolkit.NewListDirectoryTool(),
    },
})

response, err := agent.CreateResponse(ctx, dive.WithInput("Please fix the failing test"))
fmt.Println(response.OutputText())
```

## Installation

```bash
go get github.com/deepnoodle-ai/dive
```

Set your LLM API key:

```bash
export ANTHROPIC_API_KEY="your-key" # and/or OPENAI_API_KEY, GEMINI_API_KEY, etc.
```

## Usage

### Agent

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:         "engineer",
    SystemPrompt: "You are a senior software engineer.",
    Model:        anthropic.New(anthropic.WithModel("claude-opus-4-5")),
    Tools: []dive.Tool{
        toolkit.NewReadFileTool(),
        toolkit.NewTextEditorTool(),
        toolkit.NewListDirectoryTool(),
    },
    // Hooks for extensibility
    Hooks: dive.Hooks{
        PreGeneration:  []dive.PreGenerationHook{loadSession},
        PostGeneration: []dive.PostGenerationHook{saveSession},
        PreToolUse:     []dive.PreToolUseHook{checkPermissions},
        PostToolUse:    []dive.PostToolUseHook{logToolCall},
    },
    // Model settings
    ModelSettings: &dive.ModelSettings{
        MaxTokens:   dive.Ptr(16000),
        Temperature: dive.Ptr(0.7),
    },
    // Limits
    ToolIterationLimit: 50,
    ResponseTimeout:    5 * time.Minute,
})

// CreateResponse runs the agent loop until the task completes.
// Use WithEventCallback for streaming progress updates.
response, err := agent.CreateResponse(ctx,
    dive.WithInput("Fix the failing test"),
    dive.WithEventCallback(func(ctx context.Context, event *dive.ResponseItem) error {
        fmt.Print(event.Event.Delta.Text) // stream text as it arrives
        return nil
    }),
)
fmt.Println(response.OutputText())
```

### LLM

Use the LLM interface for direct model access without the agent loop:

```go
model := google.New(google.WithModel("gemini-3-flash-preview"))
response, err := model.Generate(ctx,
    llm.WithMessages(llm.NewUserMessage(
        llm.NewTextContent("What is in this image?"),
        llm.NewImageContent(llm.ContentURL("https://example.com/photo.jpg")),
    )),
    llm.WithMaxTokens(1024),
)
fmt.Println(response.Message.Text())
```

### Providers

Anthropic, OpenAI, Google, Grok, OpenRouter, Groq, Mistral, Ollama. All support
tool calling.

Some providers are separate Go modules to isolate dependencies. For example, to
use Google:

```bash
go get github.com/deepnoodle-ai/dive/providers/google
```

### Tools

Core tools in `toolkit/`: Read, Write, Edit, Glob, Grep, ListDirectory,
TextEditor, Bash, WebFetch, WebSearch, AskUserQuestion.

Create simple tools with `FuncTool` — schema auto-generated from struct tags:

```go
type OrderInput struct {
    OrderID string `json:"order_id" description:"Order ID to look up"`
}

orderTool := dive.FuncTool("get_order", "Look up an order by ID",
    func(ctx context.Context, input *OrderInput) (*dive.ToolResult, error) {
        status := lookupOrder(input.OrderID)
        return dive.NewToolResultText(status), nil
    },
)
```

For tools with struct state (DB connections, API clients), implement
`TypedTool[T]` and wrap with `dive.ToolAdapter()`. Use `Toolset` for dynamic
tools resolved at runtime (MCP servers, permission-filtered tools):

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Model: anthropic.New(),
    Tools: []dive.Tool{orderTool},
    Toolsets: []dive.Toolset{mcpToolset},
})
```

See the [Custom Tools Guide](./docs/guides/custom-tools.md) for the full interface and more examples.

### Hooks

Extend agent behavior without modifying core code. All hooks receive `*HookContext`:

- `PreGenerationHook` — Load session, inject context, modify system prompt
- `PostGenerationHook` — Save session, log results, trigger side effects
- `PreToolUseHook` — Permissions, validation, input modification
- `PostToolUseHook` — Logging, metrics, result processing (success)
- `PostToolUseFailureHook` — Error handling, retry logic, failure logging
- `StopHook` — Prevent the agent from stopping and continue generation
- `PreIterationHook` — Modify system prompt or messages between loop iterations

Hooks are grouped in a `Hooks` struct on `AgentOptions`. Hook flow:

```text
PreGeneration → [PreIteration → LLM → PreToolUse → Execute → PostToolUse]* → Stop → PostGeneration
```

### Dialog

The `Dialog` interface handles user-facing prompts during agent execution.
It's used by the permission system to confirm tool calls, and by the
`AskUser` tool to collect input from the user. A single `Show` method covers
confirmations, single/multi-select, and free-form text input. The mode is
determined by which fields are set on `DialogInput`.

Dive ships two built-in implementations: `AutoApproveDialog` (says yes to
everything) and `DenyAllDialog` (denies/cancels everything). Provide your
own `Dialog` to wire prompts into a TUI, web UI, or Slack bot.

### Content Types

Messages sent to and received from LLMs contain typed content blocks
(`llm.Content`). The main types are:

| Type                | Description                                         |
| ------------------- | --------------------------------------------------- |
| `TextContent`       | Plain text — the most common content type           |
| `ImageContent`      | An image, either inline bytes or a URL              |
| `DocumentContent`   | A document (e.g. PDF), inline bytes or URL          |
| `ToolUseContent`    | A tool call requested by the model                  |
| `ToolResultContent` | The result returned to the model after a tool call  |
| `ThinkingContent`   | Extended thinking / chain-of-thought from the model |
| `RefusalContent`    | The model declined to respond                       |

All content types implement `llm.Content` and are used in `llm.Message.Content`.

### Streaming

Real-time streaming with event callbacks:

```go
agent.CreateResponse(ctx,
    dive.WithInput("Generate a report"),
    dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
        switch item.Type {
        case dive.ResponseItemTypeMessage:
            fmt.Println(item.Message.Text())
        case dive.ResponseItemTypeModelEvent:
            fmt.Print(item.Event.Delta.Text) // streaming deltas
        case dive.ResponseItemTypeToolCall:
            fmt.Printf("Tool: %s\n", item.ToolCall.Name)
        }
        return nil
    }),
)
```

## Experimental Features

Packages under `experimental/*` have no stability guarantees. APIs may change at
any time.

- **Session** — Persistent conversation storage via hooks
- **Permission** — Rule-based tool execution control
- **Compaction** — Auto-summarize conversations approaching token limits
- **Subagent** — Spawn specialized child agents for subtasks
- **Sandbox** — Docker/Seatbelt isolation for tool execution
- **MCP** — Model Context Protocol client for external tools
- **Skills** — Markdown-based skill loading system
- **Slash Commands** — User-defined CLI commands
- **Settings** — Load configuration from `.dive/settings.json`
- **Todo** — Real-time todo list tracking during agent execution
- **Toolkit** — Additional tool packages (extended, firecrawl, google, kagi)
- **CLI** — Interactive command-line interface (`experimental/cmd/dive`)

## Examples

Run examples from the `examples/` directory:

```bash
cd examples

# Claude runs Python to compute 53^4 (Anthropic)
go run ./code_execution_example

# Agent with web search (Anthropic)
go run ./server_tools_example

# Vision: describe an image from a URL (Anthropic)
go run ./image_example

# Document analysis with source citations (Anthropic)
go run ./citations_example

# Web search, reasoning, structured output, and MCP (OpenAI)
go run ./openai_responses_example
```

## Documentation

- [Quick Start](./docs/guides/quick-start.md) — Get up and running in minutes
- [Agents Guide](./docs/guides/agents.md) — Agent loop, hooks, and configuration
- [Custom Tools](./docs/guides/custom-tools.md) — Build and register your own tools
- [LLM Guide](./docs/guides/llm-guide.md) — Direct model access without the agent loop
- [Tools Overview](./docs/guides/tools.md) — Built-in toolkit reference
- [llms.txt](./llms.txt) — AI-optimized reference for agents developing with Dive

## See Also

[Wonton](https://github.com/deepnoodle-ai/wonton) is a companion Go library
for building CLI applications. It provides a TUI framework, HTML-to-Markdown
conversion, HTTP utilities, and other common building blocks. Dive's
experimental CLI is built with Wonton, and the two libraries pair well for
building agent-powered command-line tools.

[Workflow](https://github.com/deepnoodle-ai/workflow) is a lightweight Go
library for composing multi-step workflows. Use it to orchestrate Dive agents
into pipelines, fan-out/fan-in patterns, and other structured execution flows.

## Contributing

Questions and ideas: [GitHub Discussions](https://github.com/deepnoodle-ai/dive/discussions)

Bugs and PRs: [GitHub Issues](https://github.com/deepnoodle-ai/dive/issues)

## License

[Apache License 2.0](./LICENSE)
