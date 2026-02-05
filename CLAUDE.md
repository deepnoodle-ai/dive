# CLAUDE.md

## Project Overview

Dive is a Go library for building AI agents and integrating with leading LLMs.
Library-first approach — the CLI in `experimental/cmd/dive/` is secondary.

## Commands

- `go test ./...` — Run all tests
- `cd experimental/cmd/dive && go build` — Build the CLI (optional)
- Use `github.com/deepnoodle-ai/wonton/assert` for all tests

## Architecture

### Core Types

- **Agent** (`agent.go`): Created via `NewAgent(AgentOptions)`, returns `*Agent`. Manages tool execution and conversation.
- **LLM** (`llm/llm.go`): `LLM` and `StreamingLLM` interfaces abstract over providers.
- **Tool** (`tool.go`): `Tool` and `TypedTool[T]` interfaces. All toolkit constructors return `*dive.TypedToolAdapter[T]` (satisfies `dive.Tool`).
- **Hooks** (`hooks.go`): `PreGenerationHook`, `PostGenerationHook`, `PreToolUseHook`, `PostToolUseHook`.

### Packages

- `providers/` — LLM providers (Anthropic, OpenAI, Google, Grok, Groq, Mistral, Ollama, OpenRouter). Registry-based (`providers/registry.go`), self-registering via `init()`.
- `toolkit/` — Built-in tools (Bash, ReadFile, WriteFile, Edit, Glob, Grep, ListDirectory, TextEditor, WebSearch, Fetch, AskUser).
- `experimental/` — Functional but unstable APIs: permission, session, settings, sandbox, mcp, skill, slashcmd, subagent, compaction, todo, toolkit.

### Design Philosophy

Dive aligns its tool interfaces and behaviors with Claude Code, leveraging
Anthropic's tuning of Claude for these tool patterns.

### Hook Flow

PreGeneration → [LLM → PreToolUse → Execute → PostToolUse]* → PostGeneration

PreToolUse hooks return `nil` (allow) or `error` (deny). All hooks run; any error denies the tool.

## Documentation

Guides in `docs/guides/` (core) and `docs/guides/experimental/`.

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
