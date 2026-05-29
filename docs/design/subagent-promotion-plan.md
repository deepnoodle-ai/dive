# Promotion Plan: Subagents + Task Tooling → Stable API

Status: **DECISIONS LOCKED — ready to execute on approval.** Open decisions from the
first review pass are resolved in §7. This plan is the artifact requested before any
code moves; awaiting a go-ahead to start the first commit.

All work lands in the **existing PR #152** (branch `worktree-feat+explore-plan-subagents`)
as a sequence of commits — **not** separate PRs.

## 0. Goal

Move Dive's subagent + background-control tooling out of `experimental/` into a
stable, production-ready API, aligned with Claude Code's three-axis tool model but
shaped as composable **library primitives** (not harness policy). PR #152 (this
branch) did the prerequisite naming work; this plan covers the move itself.

## 1. The model we align to (three orthogonal axes)

| Axis | Meaning | Tool(s) | This effort |
|---|---|---|---|
| **EXECUTION** | Spawn a subagent (fresh context; final message returns to caller). `subagent_type` selects a persona. | `Agent` | **In scope** |
| **COORDINATION** | Track units of work (status/owner/deps). | `TodoWrite` | **Out of scope** — left in `experimental/` untouched (§7.2) |
| **BACKGROUND CONTROL** | Operate on anything running in the background by `task_id`. | `TaskStop`, `Monitor` | **In scope** (`TaskOutput` dropped, §7.3) |

Rule: **`Agent` = spawn; `Task*` = track + control.** Keep them separate.

## 2. Current state (facts established by exploration)

**Module facts.** Everything in scope lives in the **root module**
(`github.com/deepnoodle-ai/dive`). Only `experimental/mcp` and
`experimental/cmd/dive` have their own `go.mod`. So promotion is an **import-path
move within one module** — breaking for import paths, but no new module and no
special release choreography.

**What exists, and where:**

- `experimental/subagent/` — `Definition`, `Registry`, `FilterTools`, `Loader`
  (File/Composite/Map), built-ins `GeneralPurpose`/`Explore`/`Plan`, embedded
  prompts (`go:embed prompts/*.md`). Clean, self-contained. Core `dive` package
  has **no** subagent references anymore (the old `AgentOptions.Subagents` is gone).
- `experimental/toolkit/extended/` — a **grab-bag** package. In scope:
  - `task.go` — `AgentTool` (spawner) **+** `TaskRegistry`/`TaskRecord`/`TaskStatus`
    (the shared run registry) **+** `TaskOutputTool`.
  - `taskstop.go` — `TaskStopTool`.
  - `monitor.go` — `MonitorTool` (spawns a shell watcher; streams stdout as notifications).
  - **Out of scope, stay in `extended`:** `todo.go` (TodoWrite), `command.go`,
    `code_execution.go`, `get_shell_output.go`, `kill_shell.go`, `shell_manager.go`,
    `memory.go`, `util.go`.
- `experimental/todo/` — TodoWrite's types. **Out of scope** (left in place).

**Blast radius of the move (importers, excluding stale worktrees):**

- `experimental/cmd/dive/main.go` (separate module; `replace => ../../..`)
- `examples/subagent_example/main.go` (separate module; `replace => ..`)
- `experimental/toolkit/extended/task.go` (internal: imports `subagent`)

Both downstream modules use local `replace` directives, so they build against the
moved packages immediately — **no release tag required** to keep them green.
`a2a/` and `otel/` modules do not touch any of this.

**The shared substrate — and why it collapses (§7.7).** Today `TaskRegistry` holds
a rich `TaskRecord` (10 fields + a status machine + `snapshot`/`setResult`). But
once `resume` and `TaskOutput` are removed (§7.3, §7.4), **the only surviving reader
of a record is `TaskStop`**, which needs exactly a `description` + a `cancel` func.
Verified facts: there is no `/tasks` command (the registry is never enumerated);
`record.Agent`/`Suspension` were resume-only; `Output`/`Error`/`Status`/times/`done`
were `TaskOutput`-only; and the spawner never set `cancel` on agent records, so
`TaskStop` silently couldn't stop a background agent. So the substrate collapses to
a `map[id]→{cancel, description}`.

## 3. Package layout (DECIDED — Option 3: single package)

```
subagent/                 (promote experimental/subagent → subagent)
                          Definition, FilterTools, Loader, builtins, prompts
                          (Registry type → plain map, §7.8)

toolkit/orchestration/    spawner + control + a trivial run tracker, split by axis:
    agent.go              Agent spawner tool + AgentFactory + AgentToolOptions  [EXECUTION]
    runs.go               Runs (map[id]→{cancel, description}) — shared tracker
    taskstop.go           TaskStopTool                                          [CONTROL]
    monitor.go            MonitorTool                                           [CONTROL]
```

One cohesive package users import once; files split along the three axes so the
code still reads by axis (CLEANUP #4 satisfied). The spawner and control tools share
the in-package `Runs` tracker directly. `subagent` stays a separate top-level
catalog package (matches `session`/`permission`/`skill`).

Tool *names* (LLM-facing strings) stay Claude-Code-aligned: `Agent`, `TaskStop`,
`Monitor`. The shared state is the trivial `Runs` tracker (§7.7), not a status
machine. The package name `orchestration` is the one easily-changed knob here —
alternatives if preferred: `toolkit/agents`, `toolkit/run`.

## 4. Public API surface after promotion

### `subagent/`

```go
type Definition struct { Description, Prompt string; Tools, DisallowedTools []string; Model string }
var GeneralPurpose, Explore, Plan *Definition
func FilterTools(def *Definition, allTools []dive.Tool) []dive.Tool

// Registry type retired (§7.8). A catalog is now a plain map:
func DescribeTypes(m map[string]*Definition) string  // was Registry.GenerateToolDescription
// (A catalog is a plain map literal, e.g. {"GeneralPurpose": GeneralPurpose}.)

// Loader kept; Load already returns the map you pass to the Agent tool.
type Loader interface{ Load(ctx) (map[string]*Definition, error) }
// FileLoader, CompositeLoader, NewFileLoader, LoadFromDirectory.
// (MapLoader + LoadIntoRegistry dropped — a map literal / map merge does the same.)
```

### `toolkit/orchestration/` (spawner + control + trivial tracker)

```go
// --- agent.go (EXECUTION) ---
type AgentFactory func(ctx, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error)

type AgentToolOptions struct {
    Subagents      map[string]*subagent.Definition  // plain map (§7.8)
    AgentFactory   AgentFactory
    ParentTools    []dive.Tool
    Runs           *Runs          // optional; enables TaskStop on background spawns
    DefaultTimeout time.Duration  // synchronous spawns only
}
// Returns the adapter directly (today it returns *AgentTool and the caller
// hand-wraps with dive.ToolAdapter — that wart goes away).
func NewAgentTool(opts AgentToolOptions) *dive.TypedToolAdapter[*AgentToolInput]
// Tool input: prompt, description, subagent_type, model, run_in_background.
// (resume removed, §7.4.)

// --- runs.go (the shared substrate) ---
type Runs struct{ ... }   // ~35 lines: mu + map[id]{cancel, description}
func NewRuns() *Runs
// add/stop/remove are unexported — the app just holds *Runs and wires it to tools.

// --- taskstop.go (CONTROL) ---
func NewTaskStopTool(TaskStopToolOptions) *dive.TypedToolAdapter[*TaskStopToolInput]
// TaskOutputTool: REMOVED (§7.3).

// --- monitor.go (CONTROL) ---
func NewMonitorTool(MonitorToolOptions) *dive.TypedToolAdapter[*MonitorToolInput]
```

Behavior: **synchronous** spawns run with `DefaultTimeout` and return inline (not
tracked — there's no id to stop a turn-blocking call). **Background** spawns get a
`context.WithCancel(context.Background())`, register their cancel in `Runs`, dispatch
via core's `NewBackgroundResultFull` (which delivers the result automatically), and
deregister on completion — so `TaskStop` works uniformly on background agents and
monitors. Only the background path emits an id, labeled **"Task ID"** (§7.5).

**Convention cleanup applied during the move:** every promoted constructor returns
`*dive.TypedToolAdapter[T]` (matches the rest of `toolkit/`), so the CLI/example
stop hand-wrapping with `dive.ToolAdapter(...)`.

## 5. Execution as commits (single PR #152)

A commit sequence, all on `worktree-feat+explore-plan-subagents`:

1. **Commit 1 — Promote `subagent`.** `git mv experimental/subagent → subagent`
   (incl. `prompts/`); fix package path in the 3 importers. Pure move + import rewrite.
2. **Commit 2 — Create `toolkit/orchestration`.** Move `task.go`/`taskstop.go`/
   `monitor.go` out of `extended`; split into `agent.go` (spawner), `runs.go`
   (the `Runs` tracker), `taskstop.go` (TaskStop), and `monitor.go`;
   constructors return adapters. Apply the decisions: **replace
   `TaskRegistry`/`TaskRecord`/`TaskStatus` with the trivial `Runs` tracker** (§7.7),
   **drop `TaskOutput`** (§7.3), **drop `resume` + the suspend-handling branches**
   (§7.4), **make background spawns stoppable** (register their cancel in `Runs`),
   **standardize the handle label to "Task ID"** (§7.5). Update CLI `main.go`
   (`NewRuns()`; remove `taskOutputTool` wiring; drop `dive.ToolAdapter(...)`
   wrappers; pass `Subagents` map) and the example. Reword Monitor's description line
   that referenced `TaskOutput`. (If §7.8 is taken, this commit also retires
   `subagent.Registry` for a map — or split that into its own commit.)
3. **Commit 3 — Docs.** Update `CLAUDE.md` (`subagent` + `toolkit/orchestration`
   now core; dropped from the experimental list). Promote the stale
   `docs/guides/experimental/subagents.md` to `docs/guides/subagents.md`, rewritten
   for the new API (Agent/TaskStop/Monitor, the `Subagents` map, no resume/TaskOutput)
   and adding it to the docs index; document `AgentFactory` as the
   worktree/session/sandbox seam (§7.6). `docs/guides/experimental/todo-lists.md`
   needs no change — `TodoWrite` stays in `experimental/toolkit/extended`. The
   example's header comment was refreshed in Commit 2.

Each commit: `gofmt`, tests with `github.com/deepnoodle-ai/wonton/assert`, and a build
across all three modules (root `go test ./...`; `experimental/cmd/dive` build+vet;
`examples` build).

## 6. This is a move, not a deprecation

These packages are under `experimental/`, and the repo is explicit (README §
"Experimental Features"): *"Packages under `experimental/*` have no stability
guarantees. APIs may change at any time."* So there is **no deprecation, no shims,
no alias forwarders** — the old paths simply stop existing. Move + rewrite the 3
internal importers in the same commit (they're `replace`-pinned, so they stay green
without a release).

A "what moved where" table goes in the PR description / release notes purely as a
convenience for anyone who had wired up the experimental paths — framed as
informational, **not** a compatibility commitment:

| Old (experimental) | New |
|---|---|
| `experimental/subagent` | `subagent` |
| `extended.NewAgentTool` / `AgentTool` | `orchestration.NewAgentTool` (`toolkit/orchestration`) |
| `extended.TaskRegistry` / `TaskRecord` / `TaskStatus` | `orchestration.Runs` + `NewRuns()` |
| `extended.NewTaskStopTool` | `orchestration.NewTaskStopTool` |
| `extended.NewMonitorTool` | `orchestration.NewMonitorTool` |
| `extended.NewTaskOutputTool` | gone — background results arrive automatically |
| `AgentToolInput.Resume` / `resume` param | gone — subagents are single-use |
| `subagent.Registry` / `NewRegistry` | plain `map[string]*subagent.Definition` + `DescribeTypes` (§7.8) |

`experimental/toolkit/extended` **survives** for its unrelated tools (TodoWrite,
shell, memory, code-exec); only `task.go`/`taskstop.go`/`monitor.go` leave.

## 7. Decisions (resolved in review)

### 7.1 Package layout → **Option 3 (single `toolkit/orchestration` package).**
Registry + Agent spawner + TaskStop + Monitor in one package, split across
`agent.go`/`task.go`/`monitor.go`. `subagent` stays a separate catalog package.

### 7.2 Task board (#3) → **Out of scope.**
Do not touch any task-board / coordination tooling in this work. `TodoWrite`,
`experimental/todo`, and `extended/todo.go` are **left in `experimental/` unchanged.**

### 7.3 `TaskOutput` → **Drop it.**
Removed from the promoted control family. Rationale: it was vestigial — its own
description *and* Monitor's both told the model not to call it (background results
already arrive automatically via Dive's `BackgroundResult` →
`Response.BackgroundTasks` → `ContinueWithBackground`; monitors notify
automatically; synchronous spawns return inline). Control family = `TaskStop` +
`Monitor`. The CLI's `taskOutputTool` wiring is removed; Monitor's description line
referencing `TaskOutput` is reworded.

### 7.4 `resume` / suspend → **Removed. Subagents are single-use.**
Drop the `resume` parameter from the `Agent` tool input + schema, the resume branch
in `Call`, and the suspend-handling branches in `executeTask`
(`TaskStatusSuspended`, `record.Suspension`, the "waiting for input" result path).
This also retires the latent bug where `resume` reused a session-less `*dive.Agent`
and silently lost prior-turn context. If a subagent's tool ever suspends, the
spawner surfaces it as a normal terminal result rather than carrying resume state.

### 7.5 Display-string + ID reconciliation (CLEANUP #4) → **Standardize on "Task ID".**
The handle is `task_…` and `TaskStop`/`Monitor` key on `task_id`, so the `Agent`
tool's output uses **"Task ID:"** consistently (today it mixes "Agent ID:" and
"Task ID:"). Folded into PR B.

### 7.6 Worktree isolation (CLEANUP #4) → **Stays out of the library.**
Already out: the `AgentFactory` callback is the seam where an app adds worktree
isolation, sandboxing, per-subagent sessions, or model routing before returning the
agent. Doc-only — document `AgentFactory` as that extension point in Commit 3.

### 7.7 The run registry → **Collapse `TaskRegistry`/`TaskRecord` to a trivial `Runs` tracker.**
Added during the "is this actually the best design?" pass. Rationale (verified):
after §7.3 + §7.4, the only surviving reader of a record is `TaskStop`, which needs
just a `description` + a `cancel`. There is no `/tasks` command (the registry is
never enumerated). So the 10-field `TaskRecord` + status machine +
`snapshot`/`setResult` collapses to `map[id]→{cancel, description}` (~35 lines).
Bonus fix: the spawner currently never sets `cancel` on agent records, so `TaskStop`
silently can't stop a background agent — registering the cancel in `Runs` makes
`TaskStop` work uniformly across agents and monitors.

### 7.8 `subagent.Registry` → **Decided: replace with a plain map.**
The `Registry` wrapper (mutex + `Register`/`RegisterAll`/`Get`/`List`/`Len`/
`GenerateToolDescription`) is built once and never mutated or enumerated at runtime
in either caller, and `Loader.Load` already returns `map[string]*Definition`. Drop
the type; pass the map to the `Agent` tool; provide a `DescribeTypes(map)` free
function. (A `DefaultTypes()` convenience was considered and dropped — a one-entry
map literal is clearer and the `"GeneralPurpose"` key isn't really hidden.) The
subagent_type key is PascalCase (`GeneralPurpose`, matching `Explore`/`Plan`).

Verified nothing obvious is lost: the only capability unique to the type is the
mutex for concurrent runtime registration, and (a) neither caller registers at
runtime, (b) there is no `/agents`-style live listing, (c) no test exercises
concurrency, (d) the catalog is read-only after construction (concurrent map reads
are safe). `List` sorting and the tool-description format are preserved via
`DescribeTypes`. To remove the one residual footgun — an app mutating a map it
already handed off — **`NewAgentTool` takes a defensive copy of the map at
construction**, freezing its view without needing a mutex.

## 8. Verification plan

- Root: `go test ./...` (covers `subagent`, `toolkit/orchestration`).
- CLI: `cd experimental/cmd/dive && go build ./... && go vet ./...`.
- Examples: `cd examples && go build ./...` (rebuild `subagent_example`).
- `a2a/` and `otel/` modules: unaffected (verified — no imports of moved packages).
