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

Dive is a Go library for building AI agents with a stable, extensible core API.

- 🚀 Embed AI agents in your Go applications
- 🛠️ Unified interface across 8+ LLM providers
- 🔌 Extensible via hooks - no core modifications needed
- ⚡ Stream responses in real-time

## Project Status

Dive has a clear separation between **stable core** and **experimental** features:

### 🟢 Core (Stable, Production-Ready)

- Agent interface and StandardAgent implementation
- Tool system with core file/shell operations
- LLM interface with 8+ provider integrations (Anthropic, OpenAI, Google, Grok, Groq, Mistral, Ollama, OpenRouter)
- Hook system for extensibility (PreGeneration, PostGeneration, PreToolUse, PostToolUse)
- Response streaming and events
- **Follows Go standard versioning** - Breaking changes require major version bump

### 🟡 Experimental (Active Development)

- Session persistence, permissions, context compaction
- Subagents, skills, sandbox isolation
- Model Context Protocol (MCP)
- CLI application
- **May change at any time** - Import from `experimental/*` path

---

## Quick Start

### Installation

```bash
go get github.com/deepnoodle-ai/dive
```

Set up your LLM API key:

```bash
export ANTHROPIC_API_KEY="your-key-here"
# Or: OPENAI_API_KEY, GEMINI_API_KEY, GROK_API_KEY, etc.
```

### Your First Agent

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
		SystemPrompt: "You are a helpful research assistant.",
		Model:        anthropic.New(),
	})
	if err != nil {
		log.Fatal(err)
	}

	response, err := agent.CreateResponse(
		context.Background(),
		dive.WithInput("What is the capital of France?"),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.OutputText())
}
```

### Direct LLM Usage

```go
import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
)

model := anthropic.New()
response, err := model.Generate(
	context.Background(),
	llm.WithMessages(llm.NewUserTextMessage("Hello!")),
	llm.WithMaxTokens(1024),
)
fmt.Println(response.Message.Text())
```

---

## Core Features

### Agent System

The `Agent` interface (`dive.go:14-21`) provides autonomous tool-using AI entities:

```go
type Agent interface {
	Name() string
	CreateResponse(ctx context.Context, opts ...CreateResponseOption) (*Response, error)
}
```

Create agents with `NewAgent` (`agent.go:133-179`):

```go
agent, err := dive.NewAgent(dive.AgentOptions{
	SystemPrompt: "Your instructions here",
	Model:        anthropic.New(),
	Tools:        []dive.Tool{/* tools */},

	// Optional: Hooks for extensibility
	PreGeneration:  []dive.PreGenerationHook{/* hooks */},
	PostGeneration: []dive.PostGenerationHook{/* hooks */},
	PreToolUse:     []dive.PreToolUseHook{/* hooks */},
	PostToolUse:    []dive.PostToolUseHook{/* hooks */},
})
```

**Options:**

```go
type AgentOptions struct {
	SystemPrompt string
	Model        llm.LLM
	Tools        []Tool

	PreGeneration  []PreGenerationHook
	PostGeneration []PostGenerationHook
	PreToolUse     []PreToolUseHook
	PostToolUse    []PostToolUseHook

	Confirmer          ConfirmToolFunc
	Logger             llm.Logger
	ModelSettings      *ModelSettings
	Hooks              llm.Hooks
	DateAwareness      *bool
	NoSystemPrompt     bool
	Context            []llm.Content
	ID                 string
	Name               string
	ResponseTimeout    time.Duration
	ToolIterationLimit int
}
```

### Tool System

Tools extend agent capabilities. Core tools (`toolkit/`):

**File Operations:**

- `Read` - Read file contents
- `Write` - Write files
- `Edit` - Exact string replacements (Claude Code aligned)
- `Glob` - Pattern-based file finding
- `Grep` - Regex content search (ripgrep-style)
- `ListDirectory` - Directory listings
- `TextEditor` - Multi-line editing

**Shell & Execution:**

- `Bash` - Persistent shell sessions (Claude Code aligned)

**Agent Features:**

- `AskUserQuestion` - Request user input

**Create custom tools:**

```go
type MyTool struct{}

func (t *MyTool) Name() string { return "my_tool" }
func (t *MyTool) Description() string { return "Does something useful" }
func (t *MyTool) Schema() schema.Schema { /* parameters */ }
func (t *MyTool) Annotations() dive.ToolAnnotations {
	return dive.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: false,
		OpenWorldHint:   false,
	}
}
func (t *MyTool) Call(ctx context.Context, input []byte) (*dive.ToolResult, error) {
	// Implementation
	return &dive.ToolResult{
		Content: []*dive.ToolResultContent{{
			Type: dive.ToolResultContentTypeText,
			Text: "Result",
		}},
	}, nil
}
```

See `tool.go:48-57` for the Tool interface. Use `ToolAdapter` for type-safe tools.

### Hook System

Hooks enable extending agent behavior without modifying core code:

**Generation Hooks (`hooks.go`):**

- `PreGenerationHook` - Runs before LLM generation (load session, inject context, modify system prompt)
- `PostGenerationHook` - Runs after generation (save session, log results, trigger side effects)

**Tool Hooks:**

- `PreToolUseHook` - Runs before tool execution (permissions, validation, input modification)
- `PostToolUseHook` - Runs after tool execution (logging, metrics, result processing)

```go
func myPreGenHook(ctx context.Context, state *dive.GenerationState) error {
	// Modify state.SystemPrompt, state.Messages, etc.
	state.SystemPrompt += "\n\nAdditional context: ..."
	return nil
}

agent, _ := dive.NewAgent(dive.AgentOptions{
	PreGeneration: []dive.PreGenerationHook{myPreGenHook},
})
```

**All experimental features are implemented as hooks.** This allows you to compose functionality without core dependencies.

### LLM Interface

Unified interface (`llm/llm.go:7-13`) across multiple providers:

**Supported Providers:**

| Provider      | Models                                     | Tools | Features                  |
| ------------- | ------------------------------------------ | ----- | ------------------------- |
| **Anthropic** | Claude Sonnet, Opus, Haiku                 | Yes   | Computer Use, Web Search  |
| **OpenAI**    | GPT-5, GPT-4, o1, o3, Codex                | Yes   | Vision, structured output |
| **Google**    | Gemini 2.5, Gemini 3 Preview               | Yes   | Vision, tool calling      |
| **Grok**      | Grok 4, Grok Code Fast                     | Yes   | Reasoning                 |
| **OpenRouter**| 200+ models from multiple providers        | Yes   | Unified API               |
| **Groq**      | Fast inference                             | Yes   | Low latency               |
| **Mistral**   | Mistral models                             | Yes   | European hosting          |
| **Ollama**    | Local models (llama3.2, deepseek-r1, etc.) | Yes   | Privacy, no API key       |

```go
// Each provider implements llm.LLM interface
provider := anthropic.New(anthropic.WithModel("claude-sonnet-4-5"))
provider := openai.New(openai.WithModel("gpt-5"))
provider := google.New(google.WithModel("gemini-2.5-pro"))
provider := ollama.New(ollama.WithModel("llama3.2:3b"))
```

**LLM Interface:**

```go
type LLM interface {
	Name() string
	Generate(ctx context.Context, opts ...Option) (*Response, error)
}

type StreamingLLM interface {
	LLM
	Stream(ctx context.Context, opts ...Option) (ResponseIterator, error)
}
```

**Features:**

- Streaming via `llm.StreamingLLM`
- Tool calling with JSON schema
- Vision support (images, PDFs)
- Prompt caching
- Token counting & pricing
- Structured output
- Citations

### Streaming & Events

Real-time streaming with event callbacks:

```go
agent.CreateResponse(ctx,
	dive.WithInput("Generate a report"),
	dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
		switch item.Type {
		case dive.ResponseItemTypeInit:
			fmt.Printf("Session: %s\n", item.Init.SessionID)
		case dive.ResponseItemTypeMessage:
			fmt.Printf("Message: %s\n", item.Message.Text())
		case dive.ResponseItemTypeModelEvent:
			// Streaming text deltas
			fmt.Print(item.Event.Delta.Text)
		case dive.ResponseItemTypeToolCall:
			fmt.Printf("Tool: %s\n", item.ToolCall.Name)
		case dive.ResponseItemTypeToolCallResult:
			fmt.Printf("Result: %v\n", item.ToolCallResult.Result)
		}
		return nil
	}),
)
```

**Event Types (`response.go`):**

- `Init` - Session initialization with SessionID
- `Message` - Complete LLM messages
- `ModelEvent` - Streaming text deltas, reasoning tokens, citations
- `ToolCall` - Tool invocation with input
- `ToolCallResult` - Tool execution result
- `Todo` - Task progress updates (when using TodoWrite tool)

---

## Experimental Features

> ⚠️ **Warning:** Experimental features may change or be removed without notice.
> Import from `github.com/deepnoodle-ai/dive/experimental/*`

### Session Management

Persistent conversation storage via hooks.

```go
import "github.com/deepnoodle-ai/dive/experimental/session"

repo := session.NewFileRepository(".dive/sessions")
hook := session.NewSessionHook(repo)

agent, _ := dive.NewAgent(dive.AgentOptions{
	PreGeneration:  []dive.PreGenerationHook{hook.PreGeneration},
	PostGeneration: []dive.PostGenerationHook{hook.PostGeneration},
})

response, _ := agent.CreateResponse(ctx,
	dive.WithSessionID("conversation-123"),
	dive.WithInput("Continue where we left off"),
)
```

### Permission System

Rule-based tool execution control.

```go
import "github.com/deepnoodle-ai/dive/experimental/permission"

perm := permission.New(permission.Config{
	Mode: permission.ModeDefault,
	Rules: []permission.Rule{
		permission.DenyCommandRule("bash", "rm -rf *", "Blocked"),
		permission.AllowRule("read_*"),
	},
})

agent, _ := dive.NewAgent(dive.AgentOptions{
	PreToolUse: []dive.PreToolUseHook{perm.PreToolUse},
})
```

### Context Compaction

Auto-summarize long conversations when approaching token limits.

```go
import "github.com/deepnoodle-ai/dive/experimental/compaction"

hook := compaction.NewHook(compaction.Config{
	Enabled:   true,
	Threshold: 100000,
	Model:     anthropic.New(),
})

agent, _ := dive.NewAgent(dive.AgentOptions{
	PostGeneration: []dive.PostGenerationHook{hook.PostGeneration},
})
```

### Subagents

Spawn specialized child agents for focused subtasks.

```go
import "github.com/deepnoodle-ai/dive/experimental/subagent"

taskTool := subagent.NewTaskTool(map[string]*subagent.Definition{
	"code-reviewer": {
		Description: "Expert code reviewer",
		Prompt:      "You review code for security and quality.",
		Tools:       []string{"Read", "Grep"},
	},
})

agent, _ := dive.NewAgent(dive.AgentOptions{
	Tools: []dive.Tool{taskTool},
})
```

### CLI Application

Interactive command-line interface.

```bash
cd experimental/cmd/dive && go install .
dive chat                                  # Interactive mode
dive --model claude-sonnet-4-5             # Specify model
dive --workspace /path/to/project          # Set workspace
```

### Other Experimental Features

- **Settings** (`experimental/settings`) - Load configuration from `.dive/settings.json`
- **Todo Tracking** (`experimental/todo`) - TodoTracker helper for task progress
- **Sandbox** (`experimental/sandbox`) - Docker/Seatbelt isolation for tool execution
- **MCP** (`experimental/mcp`) - Model Context Protocol client for external tools
- **Skills** (`experimental/skill`) - Markdown-based skill loading system
- **Slash Commands** (`experimental/slashcmd`) - User-defined CLI commands
- **External Tools** (`experimental/toolkit/*`) - Google Search, Kagi Search, Firecrawl web scraping

---

## Examples

Core examples in `examples/programs/`:

**Core API:**

- `llm_example` - Basic LLM text generation
- `google_example` - Google Gemini with streaming
- `ollama_example` - Local models with tool calling
- `openai_responses_example` - OpenAI Responses API
- `openrouter_example` - OpenRouter multi-provider access
- `image_example` - Vision capabilities with images
- `pdf_example` - PDF document processing

**Experimental:**

- `mcp_servers_example` - MCP server integration
- `todo_tracking_example` - Real-time todo list tracking
- `code_execution_example` - Code execution capabilities

Run with:

```bash
go run ./examples/programs/llm_example
```

---

## Documentation

### Core Guides

- [Quick Start](./docs/guides/quick-start.md)
- [Agents Guide](./docs/guides/agents.md)
- [Custom Tools](./docs/guides/custom-tools.md)
- [LLM Guide](./docs/guides/llm-guide.md)
- [Tools Overview](./docs/guides/tools.md)

### API Reference

- [Agent API](./docs/api/agent.md)
- [LLM API](./docs/api/llm.md)
- [Core Types](./docs/api/core.md)

### Experimental Guides

- [MCP Integration](./docs/guides/mcp-integration.md)
- [Permissions](./docs/guides/permissions.md)
- [Sandboxing](./docs/guides/sandboxing.md)
- [Skills](./docs/guides/skills.md)
- [Slash Commands](./docs/guides/slash-commands.md)
- [Compaction](./docs/guides/compaction.md)
- [Todo Lists](./docs/guides/todo-lists.md)

---

## Stability Policy

### Core Package

Core packages (`dive`, `llm`, `providers`, `toolkit`) follow **Go standard versioning practices**.

Breaking changes require a major version increment.

### Experimental Packages

All packages under `experimental/*` have **no stability guarantees**.

APIs may change or be removed at any time. Use experimental features when you want cutting-edge functionality and can tolerate API changes.

---

## Contributing

We welcome contributions!

**Core contributions require:**

- Comprehensive tests
- Documentation for all public APIs
- Backward compatibility (or discussion for major version changes)

**Experimental contributions:**

- Tests encouraged
- Documentation encouraged
- Breaking changes acceptable

See [GitHub Discussions](https://github.com/deepnoodle-ai/dive/discussions) for questions and feedback.

Please leave a GitHub star if you're interested in the project!
