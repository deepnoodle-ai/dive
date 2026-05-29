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
- **Extension** (`agent.go`): `Extension` interface (`Tools`, `Hooks`, `Rules`) for composable agent capabilities. Set on `AgentOptions.Extensions`. Extensions provide tools, hooks, and system prompt rules that are merged during `NewAgent`.
- **Session** (`dive.go`): `Session` interface (`ID`, `Messages`, `SaveTurn`). Set on `AgentOptions.Session` or per-call via `WithSession`. The `session` package provides `New()` (in-memory) and store-backed implementations.
- **LLM** (`llm/llm.go`): `LLM` and `StreamingLLM` interfaces abstract over providers.
- **Tool** (`tool.go`): `Tool` and `TypedTool[T]` interfaces. `FuncTool[T]()` creates tools from functions with auto-generated schemas. `Toolset` interface provides dynamic tool resolution per LLM request. Tool panics are auto-recovered. All toolkit constructors return `*dive.TypedToolAdapter[T]` (satisfies `dive.Tool`).
- **Hooks** (`hooks.go`): `Hooks` struct groups hook slices on `AgentOptions`. Hook types: `SessionStartHook`, `PreGenerationHook`, `PostGenerationHook`, `PreToolUseHook`, `PostToolUseHook`, `PostToolUseFailureHook`, `StopHook`, `PreIterationHook`, `OnSuspendHook`. All hooks receive `*HookContext`. PreToolUse hooks can set `HookContext.UpdatedInput` to rewrite the tool args. `SessionStartHook` fires once at the start of a fresh conversation (no prior messages, non-resume) and returns a `*SessionStartResult` to seed it (durable or ephemeral via `Persist`).
- **Tracer** (`tracer.go`): `Tracer` interface for observation (tracing, metrics, audit logging). Three methods (`StartAgentRun`, `StartChat`, `StartToolCall`) return `(ctx, span)`; the agent threads ctx through downstream calls so spans nest naturally. `NopTracer` (default) and `MultiTracer` live in core; the OpenTelemetry adapter is `otel.NewTracer` in the `dive/otel` module.
- **Suspend/Resume** (`tool.go`, `response.go`, `dive.go`): A tool can pause the agent mid-turn by returning `NewSuspendResult(prompt, metadata)` or `NewSuspendResultWithReason(prompt, reason, metadata)` (sets `ToolResult.Suspend`). `SuspendReason` classifies why: `SuspendReasonInput` (default) or `SuspendReasonAuth`. `CreateResponse` returns `(*Response, nil)` with `Status == ResponseStatusSuspended` and `Response.Suspension *SuspensionState`. Resume via `WithToolResults` (session-backed) or `WithResume(state, results)` (stateless). `SuspendableSession` is an optional `Session` extension for auto-persistence with `CancelSuspension(ctx)` to abandon a suspended turn. `OnSuspend` hooks fire before persistence. See `docs/guides/suspend-resume.md`.

### Packages

- `session/` — Persistent conversation state: `Session` struct (implements `dive.Session`), `Store` interface, `MemoryStore`, `FileStore`, Fork, Compact.
- `providers/` — LLM providers (Anthropic, OpenAI, Google, Grok, Mistral, Ollama, OpenRouter). Registry-based (`providers/registry.go`), self-registering via `init()`.
- `toolkit/` — Built-in tools (Bash, ReadFile, WriteFile, Edit, Glob, Grep, ListDirectory, TextEditor, WebSearch, Fetch, AskUser).
- `toolkit/orchestration/` — Subagent spawning + background control, aligned with Claude Code's tool model: `Agent` spawns a subagent (EXECUTION); `TaskStop`/`Monitor` track and cancel background runs (CONTROL). `NewAgentTool` takes a `Subagents map[string]*subagent.Definition` plus either a `Model` (uses the built-in `DefaultAgentFactory`) or an `AgentFactory` (the seam for worktree/session/sandbox/hooks/model policy). Background spawns + monitors register in a shared `Runs` tracker that `TaskStop` cancels by `task_id`. Subagents are single-use; background results arrive automatically (no polling tool). See `docs/guides/subagents.md`.
- `subagent/` — Subagent catalog: `Definition` (prompt, allowed/disallowed tools, model), built-in read-only `Explore`/`Plan` and `GeneralPurpose`, `FilterTools`, and a `Loader` (markdown + YAML frontmatter). Catalogs are plain `map[string]*Definition`; `DescribeTypes()` renders the tool description.
- `permission/` — Rule-based tool permission management with modes, specifier patterns, and session allowlists.
- `skill/` — Unified skills and slash commands. `skill.Loader` implements `dive.Extension` — pass it to `AgentOptions.Extensions` to wire up the Skill tool, catalog hook, and content hook. Three-layer architecture: rules in system prompt, catalog as `<system-reminder name="skills">` in first user message, tool as trigger with content via PostToolUseHook. Provider-based loading (filesystem, `.agents/skills/`), variable expansion, trigger matching. `dive.SetSystemReminder` manages named blocks in conversation context.
- `a2a/` — A2A (Agent-to-Agent) server and client adapter using the official `a2a-go/v2` SDK (separate Go module: `github.com/deepnoodle-ai/dive/a2a`). `Server` exposes a Dive agent as an A2A endpoint (JSON-RPC or REST). `RemoteAgent` calls remote A2A agents with zero SDK imports needed by callers (returns `*TaskResult`). `CardOptions` for static cards; `AgentCardProvider` for dynamic cards. Suspend/resume maps to `input-required` state. See `docs/guides/a2a.md`.
- `otel/` — OpenTelemetry tracer adapter (separate Go module: `github.com/deepnoodle-ai/dive/otel`).
- `experimental/` — Functional but unstable APIs: settings, sandbox, mcp, compaction, todo, toolkit.

### Design Philosophy

Dive aligns its tool interfaces and behaviors with Claude Code, leveraging
Anthropic's tuning of Claude for these tool patterns.

### Hook Flow

SessionLoad → SessionStart (first turn only) → PreGeneration → [PreIteration → LLM → PreToolUse → Execute → PostToolUse]* → Stop → PostGeneration → SessionSave

On suspend: OnSuspend → PostGeneration → SaveSuspendedTurn → return `Status=Suspended`.

Session load/save is automatic when `AgentOptions.Session` or `WithSession` is set. PreToolUse hooks return `nil` (allow) or `error` (deny). All hooks run; any error denies the tool. Stop hooks can return `Continue: true` to re-enter the loop. OnSuspend hooks run before persistence, so returning an error aborts the suspend transition without a compensating rollback.

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
