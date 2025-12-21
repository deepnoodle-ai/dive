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
- `go build ./cmd/dive` - Build the CLI binary (optional)

### Testing Guidelines

- Use `github.com/stretchr/testify/require` for all tests
- Prefer `require` functions over `assert` functions

## Architecture

### Core Interfaces

- **Agent** (`dive.go:21-27`): Main abstraction for AI entities that can execute tasks and respond to chat
- **LLM Interface** (`llm/llm.go`): Unified abstraction over different LLM providers

### Key Components

- **Agent** (`agent.go`): StandardAgent implementation
- **LLM Providers** (`llm/providers/`): Anthropic, OpenAI, Google, Grok, Ollama integrations
- **Tools** (`toolkit/`): Built-in tool implementations
- **Configuration** (`config/`): YAML configuration of agents, tools, and MCP servers
- **CLI** (`cmd/dive/`): Basic command-line interface (secondary to library)

### Design Philosophy

Dive intentionally aligns its tool interfaces and behaviors with Claude Code.
This leverages Anthropic's tuning of Claude for these tool patterns, making
tool use highly productive.

### Provider Support

Anthropic, Google, Grok, Groq, Ollama, OpenAI (Responses API), OpenAI (Completions API), OpenRouter.

## Example

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm/providers/anthropic"
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
