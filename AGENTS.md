# CLAUDE.md

This file provides guidance to AI agents like Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Dive is an AI toolkit for Go that enables creating specialized teams of AI agents and integrating with leading LLMs. It provides both a CLI and APIs for building AI-native applications.

## Common Commands

### Building and Installation
- `go install ./cmd/dive` - Install the Dive CLI
- `go build ./cmd/dive` - Build the CLI binary
- `go test ./...` - Run all tests
- `go test -short ./...` - Run tests excluding long-running ones
- `make cover` - Generate test coverage report

### Testing
- Use `github.com/stretchr/testify/require` for all tests
- Prefer `require` functions over `assert` functions
- Run specific test: `go test ./path/to/package -run TestName`

### CLI Usage
- `dive chat --provider anthropic --model claude-sonnet-4-20250514` - Chat with an agent
- `dive classify --text "input" --labels "label1,label2"` - Text classification
- `dive config check /path/to/config.yaml` - Validate configuration
- `dive diff old.txt new.txt --explain-changes` - AI-powered semantic diff
- `dive compare --provider1 anthropic --provider2 openai --input "prompt"` - Compare LLM providers

## Architecture

### Core Interfaces
- **Agent** (`dive.go:30-46`): Main abstraction for AI entities that can execute tasks and respond to chat
- **Environment** (`dive.go:50-72`): Container for agents with document/thread repositories
- **LLM Interface** (`llm/llm.go`): Unified abstraction over different LLM providers

### Key Components
- **Agent System** (`agent/`): Agent implementations, templates, and prompt logic
- **LLM Providers** (`llm/providers/`): Anthropic, OpenAI, Google, Grok, Ollama integrations
- **Tools** (`llm/tool.go`): Pluggable capabilities (web search, file operations, etc.)
- **Configuration** (`config/`): YAML parsing and builders for declarative setup
- **Environment** (`environment/`): Runtime orchestration and action handlers
- **Event Streaming** (`event.go`): Real-time events for UI updates and pipeline chaining

### Provider Support
- **Anthropic**: Claude models with tool calling
- **OpenAI**: GPT models including o1, o3 series
- **Google**: Gemini models via Vertex AI
- **Grok**: X.AI models
- **Ollama**: Local model serving
- **OpenRouter**: Access to 200+ models

### Tool System
Built-in tools include:
- `web_search`: Google Custom Search or Kagi Search
- `fetch`: Web content retrieval via Firecrawl
- `list_directory`, `read_file`, `write_file`: File operations
- `text_editor`: Advanced file editing
- `command`: Execute external commands
- `generate_image`: Image generation

Custom tools implement the `llm.Tool` interface with type safety via `dive.ToolAdapter`.

### Configuration-First Approach
- Agents and MCP servers defined in YAML for portability
- Declarative configuration types in `config/types.go`
- Support for Model Context Protocol (MCP) server integration
- Environment variables for API keys and tool configuration

### Event-Driven Architecture
- Streaming events: `response.created`, `tool_call`, `tool_result`, etc.
- `ResponseStream` interface for real-time updates
- Event callbacks for custom handling during response generation

### Testing Strategy
- Comprehensive test coverage across all packages
- Test files follow `*_test.go` naming convention
- Use `require` package for assertions
- Short flag support for excluding long-running tests
