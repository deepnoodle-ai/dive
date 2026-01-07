# CLAUDE.md

This file provides guidance to AI agents like Claude Code (claude.ai/code)
when working with code in this repository.

## Project Overview

Dive is a Go library for building AI agents and integrating with leading LLMs.
It takes a library-first approach, providing a clean API for embedding AI
capabilities into Go applications. A basic CLI is included for testing and
experimentation, but the library is the primary interface.

## Common Commands

- `go test ./...` - Run all tests
- `cd cmd/dive && go build` - Build the CLI binary (optional)

### Testing Guidelines

- Use `github.com/deepnoodle-ai/wonton/assert` for all tests

## Architecture

### Core Interfaces

- **Agent** (`dive.go:14-21`): Main abstraction for AI entities that can execute tasks and respond to chat
- **LLM Interface** (`llm/llm.go:7-13`): Unified abstraction over different LLM providers
- **Tool Interface** (`llm/tool.go:48-57`): Tools with schema, annotations, and permissions
- **Session Repository** (`session.go`): Persistent conversation storage with file and memory backends

### Key Components

- **Agent** (`agent.go`): StandardAgent implementation with tool execution and conversation management
- **LLM Providers** (`providers/`): See Provider Support section below
- **Tools** (`toolkit/`): Built-in tool implementations (see Tools section)
- **CLI** (`cmd/dive/`): Basic command-line interface (secondary to library)
- **Permissions** (`permission.go`, `permission_rules.go`, `permission_config.go`): Comprehensive permission system with hooks and modes
- **Settings** (`settings.go`): Load settings from `.dive/settings.json` (Claude Code compatible)
- **Sandbox** (`sandbox/`): Docker/Seatbelt sandboxing with network isolation
- **MCP** (`mcp/`): Model Context Protocol client for external tools and resources
- **Skills** (`skill/`): Modular agent capabilities loaded from markdown files
- **Subagents** (`subagent.go`, `subagent_loader.go`): Specialized child agents for focused tasks
- **Compaction** (`compaction.go`): Automatic context summarization for long conversations
- **Todo Tracking** (`todo_tracker.go`): Progress tracking for multi-step tasks

### Design Philosophy

Dive intentionally aligns its tool interfaces and behaviors with Claude Code.
This leverages Anthropic's tuning of Claude for these tool patterns, making
tool use highly productive.

### Provider Support

Providers use a registry-based architecture (`providers/registry.go`) where each
provider self-registers via `init()` functions with pattern matching:

- **Anthropic** - Claude models (prefix: "claude-", also fallback)
  - Includes special tools: Computer Use, Code Execution, Web Search
- **Google** - Gemini models
- **Grok** - X.AI's Grok models
- **Groq** - Groq inference engine
- **Mistral** - Mistral models
- **Ollama** - Local model serving
- **OpenAI** - Standard Responses API
- **OpenAI Completions** - Legacy Completions API
- **OpenRouter** - Multi-provider access

### Tools

Built-in tools in `toolkit/` are organized by category:

**File Operations**: Read, Write, Edit (exact string replacement), Glob (pattern matching),
Grep (ripgrep-style search), ListDirectory, TextEditor

**Shell & Execution**: Bash (persistent sessions), ShellManager, KillShell, GetShellOutput,
CodeExecution, Command, Task (spawn subagents)

**Web & Search**: WebSearch (Google/Kagi), Fetch (webpage extraction), Google Search,
Kagi Search, Firecrawl (web scraping)

**Agent Features**: TodoWrite (task tracking), Skill (activate skills), Memory (persistent files),
AskUserQuestion (user input), Extract (structured data)

Tools support rich annotations (`tool.go:11-19`) including hints for read-only, destructive,
idempotent, open-world, and edit operations.

### Permission System

The permission system (`permission.go`) provides fine-grained control:

**Permission Modes**:
- `Default` - Standard rule-based checks
- `Plan` - Read-only operations only
- `AcceptEdits` - Auto-accept edit operations
- `BypassPermissions` - Allow all (dangerous)

**Permission Flow**: PreToolUse Hook → Deny Rules → Allow Rules → Ask Rules → Mode Check → Execute → PostToolUse Hook

Settings can be loaded from `.dive/settings.json` with allow/deny patterns:
```json
{
  "permissions": {
    "allow": ["WebSearch", "Bash(go build:*)", "Read(/path/**)"],
    "deny": ["Bash(rm -rf *:*)"]
  }
}
```

### MCP (Model Context Protocol)

Full MCP support (`mcp/`) enables integration with external tools and resources:
- HTTP (SSE) and stdio server support
- OAuth 2.0 with PKCE for authentication
- Dynamic tool and resource discovery
- Multi-server management

Configure MCP servers in `.dive/settings.json`:
```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path"]
    }
  }
}
```

### Sandboxing

Production-ready sandboxing (`sandbox/`) provides isolation:
- Docker/Podman support (Linux/Windows)
- macOS Seatbelt support
- Filesystem restrictions with path allowlists
- Network isolation with domain allowlists
- Built-in HTTP proxy for filtered web access
- Environment variable and credential pass-through

See `docs/sandboxing.md` for configuration details.

### Advanced Features

**Context Compaction**: Automatically summarize conversations when approaching token limits (default: 100k tokens)

**Subagents**: Spawn specialized child agents with isolated contexts and tool access via the Task tool

**Skills**: Load modular capabilities from `.dive/skills/*.md` with YAML frontmatter metadata

**Todo Tracking**: Track multi-step task progress with TodoWrite tool and TodoTracker helper

**Enhanced LLM Features**: Citations, prompt caching, structured output, token pricing, usage tracking, server-sent events, hooks

## Documentation

Comprehensive guides are available in `docs/`:
- `agents.md` - Agent creation and configuration
- `compaction.md` - Context compaction guide
- `custom-tools.md` - Creating custom tools
- `llm-guide.md` - LLM usage guide
- `mcp-integration.md` - MCP setup and usage
- `permissions.md` - Permission system guide
- `sandboxing.md` - Sandboxing setup
- `skills.md` - Skill system guide
- `todo-lists.md` - Todo tracking guide
- `tools.md` - Tool system overview

## Example

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
		Name:         "Research Assistant",
		Instructions: "You are an enthusiastic and deeply curious researcher.",
		Model:        anthropic.New(),
	})
	if err != nil {
		log.Fatal(err)
	}
	response, err := agent.CreateResponse(context.Background(), dive.WithInput("What is the capital of France?"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.OutputText())
}
```
