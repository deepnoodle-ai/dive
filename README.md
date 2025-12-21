<div align="center">

<h1>Dive</h1>

<a href="https://www.anthropic.com"><img alt="Claude" src="https://img.shields.io/badge/Claude-6B48FF.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://www.openai.com"><img alt="GPT-4" src="https://img.shields.io/badge/GPT--4o%20|%20o1%20|%20o3-10A37F.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://cloud.google.com/vertex-ai"><img alt="Gemini" src="https://img.shields.io/badge/Gemini-4285F4.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://x.ai"><img alt="Grok" src="https://img.shields.io/badge/Grok-1DA1F2.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://ollama.ai"><img alt="Ollama" src="https://img.shields.io/badge/Ollama-1E2952.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://openrouter.ai"><img alt="OpenRouter" src="https://img.shields.io/badge/OpenRouter-7C3AED.svg?style=for-the-badge&labelColor=000000"></a>
<a href="https://deepnoodle.ai"><img alt="Made by Deep Noodle" src="https://img.shields.io/badge/MADE%20BY%20Deep%20Noodle-000000.svg?style=for-the-badge&labelColor=000000"></a>

</div>

Dive is a Go library for building AI agents and integrating with leading LLMs.

- 🚀 Embed AI agents in your Go applications
- 🤖 Create specialized agents with custom tools
- 🛠️ Unified interface across LLM providers
- ⚡ Stream responses in real-time

Dive is designed as a library-first toolkit, providing a clean API for building
AI-powered applications. It comes batteries-included with sensible defaults,
but offers the modularity you need for extensive customization.

## Project Status

Dive is shaping up nicely, but is still a young project.

- **Feedback is highly valued** on concepts, APIs, and usability
- **Some breaking changes will happen** as the API matures
- **Not yet recommended for production use**

You can also use [GitHub Discussions](https://github.com/deepnoodle-ai/dive/discussions) for questions, suggestions, or feedback.

We welcome your input! 🙌

Please leave a GitHub star if you're interested in the project!

## Features

### Library Features

- **Agents**: Create specialized agents with configurable reasoning and tools
- **Supervisor Patterns**: Build hierarchical agent systems with work delegation
- **Multiple LLMs**: Unified interface for Anthropic, OpenAI, Google, OpenRouter, Grok, and Ollama
- **Extended Reasoning**: Configure reasoning effort and budget for deep thinking
- **Model Context Protocol (MCP)**: Connect to MCP servers for external tool access
- **Tools**: Give agents rich capabilities to interact with the world
- **Tool Annotations**: Semantic hints about tool behavior (read-only, destructive, etc.)
- **Streaming**: Stream agent events for real-time UI updates
- **Thread Management**: Persistent conversation threads with memory
- **Confirmation System**: Built-in confirmation system for destructive operations
- **Advanced Model Settings**: Fine-tune temperature, penalties, caching, and tool behavior

### Additional Features

- **CLI**: Basic command-line interface for testing and experimentation

## Quick Start

### Installation

Install Dive using go get:

```
go get github.com/deepnoodle-ai/dive
```

Set up your LLM provider API key:

```bash
export ANTHROPIC_API_KEY="your-key-here"
# Or use other providers: OPENAI_API_KEY, GEMINI_API_KEY, GROK_API_KEY, OPENROUTER_API_KEY
```

### Creating an Agent

Here's a quick example of creating a chat agent:

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:         "Research Assistant",
    Instructions: "You are an enthusiastic and deeply curious researcher.",
    Model:        anthropic.New(),
})

// Start chatting with the agent
response, err := agent.CreateResponse(ctx, dive.WithInput("Hello there!"))
```

Or use the Dive LLM interface directly:

```go
model := anthropic.New()
response, err := model.Generate(
  context.Background(),
  llm.WithMessages(llm.NewUserTextMessage("Hello there!")),
  llm.WithMaxTokens(2048),
  llm.WithTemperature(0.7),
)
if err != nil {
  log.Fatal(err)
}
fmt.Println(response.Message.Text())
```

## Examples

The `examples/programs/` directory contains runnable examples demonstrating various features:

| Example                    | Description                                  |
| -------------------------- | -------------------------------------------- |
| `llm_example`              | Basic LLM text generation                    |
| `google_example`           | Using Google Gemini with streaming           |
| `ollama_example`           | Local models with Ollama, including tool use |
| `openai_responses_example` | OpenAI Responses API                         |
| `openrouter_example`       | Using OpenRouter for model access            |
| `image_example`            | Image analysis with vision models            |
| `pdf_example`              | PDF document processing                      |
| `mcp_servers_example`      | Connecting to MCP servers                    |
| `todo_tracking_example`    | Real-time todo list tracking with agents     |
| `code_execution_example`   | Code execution capabilities                  |

Run an example with:

```bash
go run ./examples/programs/llm_example
```

## CLI

Dive includes a basic CLI for testing and experimentation. Build it with:

```bash
git clone git@github.com:deepnoodle-ai/dive.git
cd dive/cmd/dive && go install .
```

Run the interactive assistant:

```bash
dive                                    # Start interactive chat
dive --model claude-sonnet-4-20250514   # Use a specific model
dive --workspace /path/to/project       # Set workspace directory
```

## LLM Providers

Dive provides a unified interface for working with different LLM providers:

- **Anthropic** (Claude Sonnet, Haiku, Opus)
- **OpenAI** (GPT-5, GPT-4, o1, o3)
- **OpenRouter** (Access to 200+ models from multiple providers with unified API)
- **Google** (Gemini models)
- **Grok** (Grok 4, Grok Code Fast)
- **Ollama** (Local model serving)

Each provider implementation handles API communication, token counting,
tool calling, and other details.

```go
provider := anthropic.New(anthropic.WithModel("claude-sonnet-4-20250514"))

provider := openai.New(openai.WithModel("gpt-5"))

provider := openrouter.New(openrouter.WithModel("openai/gpt-4o"))

provider := google.New(google.WithModel("gemini-2.5-flash"))

provider := ollama.New(ollama.WithModel("llama3.2:3b"))
```

## Model Context Protocol (MCP)

Dive supports the Model Context Protocol (MCP) for connecting to external tools and services:

```go
response, err := anthropic.New().Generate(
    context.Background(),
    llm.WithMessages(llm.NewUserTextMessage("What are the open tickets?")),
    llm.WithMCPServers(
        llm.MCPServerConfig{
            Type:               "url",
            Name:               "linear",
            URL:                "https://mcp.linear.app/sse",
            AuthorizationToken: "your-token-here",
        },
    ),
)
```

MCP servers can also be configured in YAML configurations for declarative setup.

### Verified Models

Latest models verified to work with Dive:

| Provider  | Model                    | Tools |
| --------- | ------------------------ | ----- |
| Anthropic | `claude-opus-4-5`        | Yes   |
| Anthropic | `claude-sonnet-4-5`      | Yes   |
| Anthropic | `claude-haiku-4-5`       | Yes   |
| OpenAI    | `gpt-5.2`                | Yes   |
| OpenAI    | `o3`                     | Yes   |
| OpenAI    | `codex`                  | Yes   |
| Google    | `gemini-3-pro-preview`   | Yes   |
| Google    | `gemini-2.5-pro`         | Yes   |
| Grok      | `grok-4-1-fast-reasoning`| Yes   |

**Local Models via Ollama**: Dive supports local model inference through Ollama.
Models with tool calling support (like `llama3.2`) work best for agentic tasks.

## Tool Use

Tools extend agent capabilities. Dive intentionally aligns its tool interfaces
and behaviors with Claude Code in many cases. This alignment leverages
Anthropic's tuning of Claude for these specific tool patterns, making Dive
agents highly productive out of the box.

Dive includes these built-in tools:

**File Operations**
- **read_file**: Read content from files
- **write_file**: Write content to files
- **edit**: Exact string replacements in files (Claude Code aligned)
- **glob**: Find files using glob patterns
- **grep**: Search file contents with regex (ripgrep-style)
- **list_directory**: List directory contents

**Shell & Execution**
- **bash**: Persistent bash sessions with state (Claude Code aligned)
- **task**: Spawn subagents for complex subtasks

**Web & Search**
- **web_search**: Search the web using Google or Kagi
- **fetch**: Extract content from webpages via Firecrawl

**Agent Features**
- **todo_write**: Track task progress with todo lists
- **skill**: Activate specialized skills
- **memory**: Persistent memory files across sessions
- **ask_user**: Request user input or clarification

### Tool Annotations

Dive's tool system includes rich annotations that provide hints about tool behavior:

```go
type ToolAnnotations struct {
    Title           string      // Human-readable title
    ReadOnlyHint    bool        // Tool only reads, doesn't modify
    DestructiveHint bool        // Tool may make destructive changes
    IdempotentHint  bool        // Tool is safe to call multiple times
    OpenWorldHint   bool        // Tool accesses external resources
}
```

### Custom Tools

Creating custom tools is straightforward using the `TypedTool` interface:

```go
type SearchTool struct{}

func (t *SearchTool) Name() string { return "search" }
func (t *SearchTool) Description() string { return "Search for information" }
func (t *SearchTool) Schema() schema.Schema { /* define parameters */ }
func (t *SearchTool) Annotations() dive.ToolAnnotations { /* tool hints */ }
func (t *SearchTool) Call(ctx context.Context, input *SearchInput) (*dive.ToolResult, error) {
    // Tool implementation
}

// Use with ToolAdapter for type safety
tool := dive.ToolAdapter(searchTool)
```

Go interfaces are in-place to support swapping in different tool implementations.

## Agent Features

Dive provides advanced capabilities for building sophisticated AI agents.
See the [docs/guides](./docs/guides) directory for detailed documentation.

### Subagents

Spawn specialized child agents for focused subtasks with isolated contexts
and restricted tool access:

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:  "Main Agent",
    Model: anthropic.New(),
    Tools: allTools,
    Subagents: map[string]*dive.SubagentDefinition{
        "code-reviewer": {
            Description: "Expert code reviewer for security and quality reviews.",
            Prompt:      "You are a code review specialist.",
            Tools:       []string{"Read", "Grep", "Glob"},
            Model:       "haiku",
        },
    },
})
```

Subagents can also be loaded from markdown files with YAML frontmatter.

### Skills

Modular capabilities that extend agent functionality through specialized
instructions with optional tool restrictions:

```markdown
<!-- .dive/skills/code-reviewer.md -->
---
name: code-reviewer
description: Review code for best practices and potential issues.
allowed-tools:
  - Read
  - Grep
  - Glob
---

You are a code reviewer focused on identifying issues and suggesting improvements.
```

### Permissions

Fine-grained control over tool execution with declarative rules and hooks:

```go
Permission: &dive.PermissionConfig{
    Mode: dive.PermissionModeDefault,
    Rules: dive.PermissionRules{
        dive.DenyCommandRule("bash", "rm -rf *", "Recursive deletion blocked"),
        dive.AllowRule("read_*"),
        dive.AllowRule("glob"),
        dive.AskRule("write_*", "Confirm file write"),
    },
}
```

### Context Compaction

Automatically manage conversation context as it grows, summarizing history
when token usage exceeds a threshold:

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:  "Assistant",
    Model: anthropic.New(),
    Compaction: &dive.CompactionConfig{
        Enabled:               true,
        ContextTokenThreshold: 100000,
    },
})
```

### Todo Lists

Track task progress with the TodoWrite tool and TodoTracker helper:

```go
tracker := dive.NewTodoTracker()

resp, err := agent.CreateResponse(ctx,
    dive.WithInput("Set up a new Go project with testing"),
    dive.WithEventCallback(tracker.HandleEvent),
)

completed, _, total := tracker.Progress()
fmt.Printf("Completed %d/%d tasks\n", completed, total)
```

### Thread Management

Persistent conversation threads with resumption and forking:

```go
agent, err := dive.NewAgent(dive.AgentOptions{
    Name:             "Assistant",
    Model:            anthropic.New(),
    ThreadRepository: dive.NewMemoryThreadRepository(),
})

// First interaction
resp, err := agent.CreateResponse(ctx,
    dive.WithThreadID("user-123"),
    dive.WithInput("My name is Alice"),
)

// Later - agent remembers context
resp2, err := agent.CreateResponse(ctx,
    dive.WithThreadID("user-123"),
    dive.WithInput("What's my name?"),
)
```

## Contributors

We're looking for contributors! Whether you're fixing bugs, adding features,
improving documentation, or spreading the word, your help is appreciated.
