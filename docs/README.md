# Dive Documentation

Dive is a Go library for building AI agents and integrating with leading LLMs.

- [Dive as an Agent Harness](harness-features.md) - Catalog of every harness feature Dive provides, organized by concern

## Core Guides

- [Installation](guides/installation.md) - Setting up Dive
- [Quick Start](guides/quick-start.md) - Build your first agent
- [Agents](guides/agents.md) - Agent creation, hooks, and event handling
- [Tools](guides/tools.md) - Built-in tools
- [Custom Tools](guides/custom-tools.md) - Creating your own tools
- [LLM Guide](guides/llm-guide.md) - Working with different LLM providers
- [Model Controls](guides/model-controls.md) - What a model takes, what Dive would send, and what it actually sent
- [Runtime Context and System Reminders](guides/context-injection.md) - Authority tiers, delivery lifetime, persistence, provider fallback, and CLI demos
- [Permissions](guides/permissions.md) - Tool execution permissions
- [Skills](guides/skills.md) - Modular agent capabilities and slash commands
- [Sub-Agents](guides/subagents.md) - Spawning specialized agents (Agent tool) and background control (TaskStop, Monitor)
- [Tracing](guides/tracing.md) - OpenTelemetry tracing and metrics for agent runs (full reference: [otel.md](guides/otel.md))

## Experimental Guides

These cover packages in `experimental/` with unstable APIs:

- [Compaction](guides/experimental/compaction.md) - Context compaction for long conversations
- [MCP Integration](guides/experimental/mcp-integration.md) - Model Context Protocol support
- [Sandboxing](guides/experimental/sandboxing.md) - Secure command execution isolation
- [Todo Lists](guides/experimental/todo-lists.md) - Task progress tracking

## Design Documents

- [Sandboxing](design/sandboxing.md) - Sandboxing design
- [Runtime Context Injection](design/context-injection.md) - Implemented context authority, delivery, ordering, persistence, and provider-rendering contract
- [Model Controls Registry](design/model-controls-registry.md) - Published per-model controls, network-free previews, and effective controls on the response

## Project

- [Releasing](releasing.md) - Cutting the changelog, tagging modules, and publishing the GitHub release
- [Release notes](releases/) - Published release bodies, one file per version
