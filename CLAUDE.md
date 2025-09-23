# CLAUDE.md

This file provides guidance to AI agents like Claude Code (claude.ai/code)
when working with code in this repository.

## Project Overview

Dive is an AI toolkit for Go that enables creating specialized teams of AI
agents and integrating with leading LLMs. It provides both a CLI and APIs
for building AI powered applications.

## Common Commands

### Building and Installation

- `go build ./cmd/dive` - Build the CLI binary
- `go test ./...` - Run all tests

### Testing

- Use `github.com/stretchr/testify/require` for all tests
- Prefer `require` functions over `assert` functions

## Architecture

### Core Interfaces

- **Agent** (`dive.go:21-27`): Main abstraction for AI entities that can execute tasks and respond to chat
- **LLM Interface** (`llm/llm.go`): Unified abstraction over different LLM providers

### Key Components

- **Agent** (`agent.go`): StandardAgent implementation
- **LLM Providers** (`llm/providers/`): Anthropic, OpenAI, Google, Grok, Ollama integrations
- **Tools** (`toolkit/`): Tool implementations
- **Configuration** (`config/`): YAML configuration of agents, tools, and MCP servers

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
