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
- **Tool** (`tool.go`): `Tool` and `TypedTool[T]` interfaces. `FuncTool[T]()` creates tools from functions with auto-generated schemas. `Toolset` interface provides dynamic tool resolution per LLM request. Tool panics are auto-recovered. All toolkit constructors return `*dive.TypedToolAdapter[T]` (satisfies `dive.Tool`). `ToolDeclarer` (`DeclaredTools`) marks a provider-defined toolset such as `anthropic.ComputerToolset`: calls route to the declared tools by name, which are left out of the request while the declarer is present. `ToolAnnotations.HaltsBatch` (automatic for calls with `ToolsetName`) runs a batch in order and answers later halting calls after a failure with an error, `ToolCallResult.Error == ErrBatchHalted`.
- **Hooks** (`hooks.go`): `Hooks` struct groups hook slices on `AgentOptions`. Hook types: `SessionStartHook`, `PreGenerationHook`, `PostGenerationHook`, `PreToolUseHook`, `PostToolUseHook`, `PostToolUseFailureHook`, `StopHook`, `PreIterationHook`, `OnSuspendHook`. All hooks receive `*HookContext`. PreToolUse hooks can set `HookContext.UpdatedInput` to rewrite the tool args. `SessionStartHook` fires once at the start of a fresh conversation (no prior messages, non-resume) and returns a `*SessionStartResult` to seed it (durable or ephemeral via `Persist`).
- **Tracer** (`tracer.go`): `Tracer` interface for observation (tracing, metrics, audit logging). Three methods (`StartAgentRun`, `StartChat`, `StartToolCall`) return `(ctx, span)`; the agent threads ctx through downstream calls so spans nest naturally. `NopTracer` (default) and `MultiTracer` live in core; the OpenTelemetry adapter is `otel.NewTracer` in the `dive/otel` module.
- **Turn outcome** (`outcome.go`, `turn.go`): once the turn begins (just before PreGeneration), every exit goes through `turn.end`, which reads one `turnRecord`. An error exit returns `(resp, err)` with `Status == ResponseStatusIncomplete`, `resp.Turn.Outcome` classifying the error, and `err` wrapping `*GenerationError{Response: resp}`; a failed partial resume returns `(nil, err)`. `Response.Turn` (messages saved, usage, outcome or suspension, `Persistence`) is set for every status, and `ResponseItemTypeTurnEnded` is the last item emitted. A tool batch that stops early (`toolbatch.go`) returns its partial batch; `closeToolBatch` answers each call it did not finish as not run or unknown, `Outcome.ToolCalls` records their states, and a parallel call still running gets a `BackgroundTaskHandle` for its late result. `WithSoftCancel` (`softcancel.go`) stops at the next step boundary. The loop reads each response's stop reason (`llm.ClassifyStopReason`): an output or context limit, a refusal, a provider stop and the iteration limit run no tool call, and a pause is resent. An incomplete turn is closed (`closeturn.go`: every call answered, a `turn-incomplete` reminder whose details hold the outcome), passed to `OnIncompleteTurn` hooks and saved; `WithContinue` picks it up; `IncompleteTurns.Discard` restores the old error behaviour. Design: `docs/design/incomplete-turns.md`.
- **Suspend/Resume** (`tool.go`, `response.go`, `dive.go`): A tool can pause the agent mid-turn by returning `NewSuspendResult(prompt, metadata)` or `NewSuspendResultWithReason(prompt, reason, metadata)` (sets `ToolResult.Suspend`). `SuspendReason` classifies why: `SuspendReasonInput` (default) or `SuspendReasonAuth`. `CreateResponse` returns `(*Response, nil)` with `Status == ResponseStatusSuspended` and `Response.Suspension *SuspensionState`. Resume via `WithToolResults` (session-backed) or `WithResume(state, results)` (stateless). `SuspendableSession` is an optional `Session` extension for auto-persistence with `CancelSuspension(ctx)` to abandon a suspended turn. `OnSuspend` hooks fire before persistence. See `docs/guides/suspend-resume.md`.

### Packages

- `session/` — Persistent conversation state: `Session` struct (implements `dive.Session`), `Store` interface, `MemoryStore`, `FileStore`, Fork, Compact.
- `providers/` — LLM providers (Anthropic, OpenAI, Google, Grok, Mistral, Ollama, OpenRouter). Registry-based (`providers/registry.go`), self-registering via `init()`. Shared encoder helpers in `providers/toolresult.go` (`ToolResultBlocks`, `ToolResultImageMediaType`, `LiftToolResultImages`/`LiftErrorToolResultImages`) rewrite only the request, never the caller's history. Every encoder applies `llm.AnswerUnansweredToolCalls` the same way; adapters report raw stop reasons that `llm.ClassifyStopReason` maps to an `llm.StopKind`.
- `toolkit/` — Built-in tools (Bash, ReadFile, WriteFile, Edit, Glob, Grep, ListDirectory, TextEditor, WebSearch, Fetch, AskUser).
- `toolkit/orchestration/` — Subagent spawning + background control, aligned with Claude Code's tool model: `Agent` spawns a subagent (EXECUTION); `TaskStop`/`Monitor` track and cancel background runs (CONTROL). `NewAgentTool` takes a `Subagents map[string]*subagent.Definition` plus either a `Model` (uses the built-in `DefaultAgentFactory`) or an `AgentFactory` (the seam for worktree/session/sandbox/hooks/model policy). Background spawns + monitors register in a shared `Runs` tracker that `TaskStop` cancels by `task_id`. Subagents are single-use; background results arrive automatically (no polling tool). See `docs/guides/subagents.md`.
- `subagent/` — Subagent catalog: `Definition` (prompt, allowed/disallowed tools, model), built-in read-only `Explore`/`Plan` and `GeneralPurpose`, `FilterTools`, and a `Loader` (markdown + YAML frontmatter). Catalogs are plain `map[string]*Definition`; `DescribeTypes()` renders the tool description.
- `permission/` — Rule-based tool permission management with modes, specifier patterns, and session allowlists.
- `skill/` — Unified skills and slash commands. `skill.Loader` implements `dive.Extension` — pass it to `AgentOptions.Extensions` to wire up the Skill tool, catalog hook, and content hook. Three-layer architecture: rules in system prompt, a typed contextual `<system-reminder name="skills">` prepended model-only as stable prompt-prefix context, and the Skill tool as a trigger with content via PostToolUseHook. Provider-based loading (filesystem, `.agents/skills/`), variable expansion, trigger matching. New integrations use `Reminder`, `WithModelOnlyReminder`, `NewReminderMessage`, and `HookContext.AppendReminder`; `SetSystemReminder` is the legacy plain-text compatibility path.
- `a2a/` — A2A (Agent-to-Agent) server and client adapter using the official `a2a-go/v2` SDK (separate Go module: `github.com/deepnoodle-ai/dive/a2a`). `Server` exposes a Dive agent as an A2A endpoint (JSON-RPC or REST). `RemoteAgent` calls remote A2A agents with zero SDK imports needed by callers (returns `*TaskResult`). `CardOptions` for static cards; `AgentCardProvider` for dynamic cards. Suspend/resume maps to `input-required` state. See `docs/guides/a2a.md`.
- `otel/` — OpenTelemetry tracer adapter (separate Go module: `github.com/deepnoodle-ai/dive/otel`).
- `experimental/` — Functional but unstable APIs: settings, sandbox, mcp, compaction, todo, toolkit.

### Design Philosophy

Dive aligns its tool interfaces and behaviors with Claude Code, leveraging
Anthropic's tuning of Claude for these tool patterns.

### Hook Flow

SessionLoad → SessionStart (first turn only) → PreGeneration → [PreIteration → LLM → PreToolUse → Execute → PostToolUse]* → Stop → PostGeneration → SessionSave

On suspend: OnSuspend → PostGeneration → SaveSuspendedTurn → return `Status=Suspended`.

On incomplete (error, cancellation, hook abort, or a model stop at a limit): close the turn → OnIncompleteTurn → SaveTurn / SaveResumedTurn → return `Status=Incomplete`. Session writes at the end of a call use a context without the run's cancellation.

A call halted by an earlier failure in its batch skips PreToolUse/PostToolUse/PostToolUseFailure but still emits tool_call/tool_call_result events.

Session load/save is automatic when `AgentOptions.Session` or `WithSession` is set. PreToolUse hooks return `nil` (allow) or `error` (deny). All hooks run; any error denies the tool. Stop hooks can return `Continue: true` to re-enter the loop. OnSuspend hooks run before persistence; an abort ends the turn incomplete with the suspending calls recorded as unknown.

## Documentation

Guides in `docs/guides/` (core) and `docs/guides/experimental/`.

## Changelog

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/). Keep
entries to one to three lines: bold summary, then the new value, breaking
change, or migration. How a bug was found belongs in the commit message.

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
