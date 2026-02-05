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

### Core Types

- **Agent** (`agent.go`): Concrete struct for AI entities with tool execution and conversation management. Created via `NewAgent(AgentOptions)`, returns `*Agent`.
- **LLM Interface** (`llm/llm.go`): Unified abstraction over different LLM providers (`LLM` and `StreamingLLM` interfaces).
- **Tool Interface** (`tool.go`): `Tool` and `TypedTool[T]` interfaces with schema, annotations, and `ToolAdapter` for type conversion.
- **Hooks** (`hooks.go`): `PreGenerationHook`, `PostGenerationHook`, `PreToolUseHook`, `PostToolUseHook` for customizing agent behavior.

### Key Packages

- **LLM Providers** (`providers/`): See Provider Support section below
- **Tools** (`toolkit/`): Built-in tool implementations (see Tools section)
- **CLI** (`cmd/dive/`): Basic command-line interface (secondary to library)

### Experimental Packages (`experimental/`)

These packages are functional but have unstable APIs:

- **Permission** (`experimental/permission/`): Permission system with modes, rules, and hooks
- **Settings** (`experimental/settings/`): Load settings from `.dive/settings.json`
- **Session** (`experimental/session/`): Persistent conversation storage (file and memory backends)
- **Sandbox** (`experimental/sandbox/`): Docker/Seatbelt sandboxing with network isolation
- **MCP** (`experimental/mcp/`): Model Context Protocol client for external tools
- **Skills** (`experimental/skill/`): Modular agent capabilities from markdown files
- **Slash Commands** (`experimental/slashcmd/`): User-invocable CLI commands from markdown files
- **Subagents** (`experimental/subagent/`): Specialized child agents for focused tasks
- **Compaction** (`experimental/compaction/`): Context summarization for long conversations
- **Todo** (`experimental/todo/`): Progress tracking for multi-step tasks
- **Experimental Toolkit** (`experimental/toolkit/`): Additional tools beyond core toolkit

### Design Philosophy

Dive intentionally aligns its tool interfaces and behaviors with Claude Code.
This leverages Anthropic's tuning of Claude for these tool patterns, making
tool use highly productive.

### Provider Support

Providers use a registry-based architecture (`providers/registry.go`) where each
provider self-registers via `init()` functions with pattern matching:

- **Anthropic** - Claude models (prefix: "claude-", also fallback)
- **Google** - Gemini models
- **Grok** - X.AI's Grok models
- **Groq** - Groq inference engine
- **Mistral** - Mistral models
- **Ollama** - Local model serving
- **OpenAI** - Standard Responses API
- **OpenAI Completions** - Legacy Completions API
- **OpenRouter** - Multi-provider access

### Tools

Built-in tools in `toolkit/` (core):

**File Operations**: ReadFile, WriteFile, Edit, Glob, Grep, ListDirectory, TextEditor

**Shell**: Bash

**Web**: WebSearch, Fetch

**User Interaction**: AskUser

All tool constructors return `*dive.TypedToolAdapter[T]` which satisfies `dive.Tool`.
Example: `toolkit.NewBashTool(toolkit.BashToolOptions{...})`.

### Hook System

The hook system (`hooks.go`) provides customization points:

**Generation Hooks**: `PreGenerationHook` and `PostGenerationHook` run before/after the LLM generation loop. Access `GenerationState` to modify system prompt, messages, or read results.

**Tool Hooks**: `PreToolUseHook` and `PostToolUseHook` run around individual tool executions. PreToolUse returns `AllowResult()`, `DenyResult(msg)`, `AskResult(msg)`, or `ContinueResult()`.

**Hook Flow**: PreGeneration → [LLM → PreToolUse → Execute → PostToolUse]\* → PostGeneration

## Documentation

Core guides are in `docs/guides/`:

- `quick-start.md` - Build your first agent
- `agents.md` - Agent creation and configuration
- `tools.md` - Built-in tools overview
- `custom-tools.md` - Creating custom tools
- `llm-guide.md` - LLM providers and configuration

Experimental guides are in `docs/guides/experimental/`:

- `permissions.md` - Permission system
- `compaction.md` - Context compaction
- `mcp-integration.md` - MCP setup and usage
- `sandboxing.md` - Sandboxing setup
- `skills.md` - Skill system
- `slash-commands.md` - Slash commands
- `todo-lists.md` - Todo tracking

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
		SystemPrompt: "You are an enthusiastic and deeply curious researcher.",
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
