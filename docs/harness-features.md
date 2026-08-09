# Dive as an Agent Harness

An agent harness equips an AI agent with tools, connects it to its
environment, and manages the loop that lets it do real work. This page
catalogs the harness features Dive provides, organized by concern, with
pointers to the relevant guides and packages.

Everything listed here is stable unless marked _(experimental)_. Experimental
features live under `experimental/` and may change.

## The Agentic Loop

The core of the harness: `Agent.CreateResponse` runs the generate → tool-call
→ repeat loop until the model produces a final answer. See the
[Agents Guide](guides/agents.md).

- **Iteration and time limits** — `ToolIterationLimit` (default 100) and
  `ResponseTimeout` (default 30 minutes) bound every run.
- **Forced final answer** — on the last allowed iteration the agent sets
  `ToolChoiceNone` and instructs the model to answer, so runs never end
  mid-tool-call.
- **Parallel tool execution** — `ParallelToolExecution` runs a batch of tool
  calls concurrently, with automatic sequential fallback when any tool in the
  batch is annotated `SequentialOnlyHint`.
- **Tool panic recovery** — a panic inside a tool becomes an `IsError` tool
  result the model can react to, rather than crashing the process.
- **Partial-work recovery** — a failed turn returns `GenerationError`
  carrying usage, output messages, and response items, so callers can still
  bill, log, and salvage.
- **Streaming** — agents use `StreamingLLM` when available and emit typed
  `ResponseItem` events (message, tool_call, tool_call_result, model_event,
  tool_stream, tool_progress, suspended) through `WithEventCallback`.

## Tool System

The abstractions that equip an agent with capabilities. See the
[Tools Guide](guides/tools.md) and [Custom Tools Guide](guides/custom-tools.md).

- **Tool interfaces** — `Tool`, typed `TypedTool[T]` with automatic JSON
  unmarshaling, and `FuncTool[T]` which generates a schema from a Go struct.
- **Dynamic toolsets** — the `Toolset` interface resolves the tool list per
  LLM request, enabling MCP-backed, permission-filtered, or context-dependent
  tool availability.
- **Tool annotations** — ReadOnly, Destructive, Idempotent, OpenWorld, and
  SequentialOnly hints drive permission categorization and execution behavior.
- **Tool previews** — `ToolPreviewer` produces a human-readable summary of
  what a call will do before it runs, for approval UIs.
- **Live output channels** — running tools can stream text with
  `StreamOutput` and publish structured progress snapshots with
  `ReportProgress`.
- **Rich results** — tool results carry text, image, or audio content, an
  optional display variant, and error status.

## Built-in Toolkit

Ready-made tools in `toolkit/`, aligned with Claude Code's tool patterns so
agents benefit from Anthropic's model tuning for these tool shapes.

- **File and system** — Read, Write, Edit, Glob, Grep, ListDirectory,
  TextEditor, and Bash (with timeout and output caps).
- **Web** — WebFetch and WebSearch with pluggable backends (Firecrawl,
  Google Custom Search, Kagi).
- **Interaction and media** — AskUserQuestion, ImageGeneration,
  VideoGeneration.
- **Extended tools** _(experimental)_ — Command with background shell
  support, KillShell / GetShellOutput / ListShells backed by a
  `ShellManager`, TodoWrite with a todo tracker, the Anthropic Memory tool
  with pluggable storage, and Anthropic server-side Code Execution. See
  [Todo Lists](guides/experimental/todo-lists.md) and
  [Media Generation](guides/experimental/media-generation.md).

## Multi-Agent Orchestration

Spawning and controlling subagents, aligned with Claude Code's model. See the
[Sub-Agents Guide](guides/subagents.md).

- **Agent tool** — spawns single-use subagents, synchronously or in the
  background.
- **Subagent catalog** — `subagent.Definition` holds a prompt, tool
  allow/disallow lists, and model routing. Built-ins: read-only `Explore` and
  `Plan`, plus `GeneralPurpose`. A markdown + YAML frontmatter loader reads
  definitions from disk.
- **Background run control** — `Monitor` and `TaskStop` tools operate on a
  shared `Runs` tracker; background results are delivered to the parent agent
  automatically on a later turn, with no polling tool.
- **AgentFactory seam** — customize how subagents are constructed, e.g. to
  add worktrees, sessions, sandboxes, or per-subagent model policy.

## Hooks

Ten lifecycle interception points, grouped in the `Hooks` struct. See the
[Hooks Guide](guides/hooks.md).

- **Lifecycle coverage** — SessionStart, PreGeneration, PreIteration,
  PreToolUse, PostToolUse, PostToolUseFailure, Stop, PostGeneration,
  OnSuspend, and PostBackgroundToolUse.
- **Gate, rewrite, and steer** — hooks can deny tools, rewrite tool
  arguments via `UpdatedInput`, inject additional context, force the loop to
  continue (`StopDecision{Continue: true}`), or abort the run
  (`HookAbortError`).
- **LLM-as-judge hooks** — `PromptStopHook` asks a model "is the task
  actually done?" before allowing the agent to stop; `PromptToolGate` asks
  "is this tool call safe?" before allowing execution.
- **Ready-made hooks** — `InjectContext`, `CompactionHook`, `UsageLogger`,
  and regex-scoped `MatchTool` wrappers.
- **Extensions** — the `Extension` interface bundles tools, hooks, and
  system-prompt rules into one pluggable unit; the skill loader is an
  Extension.

## Context Engineering

Managing what the model sees. See
[Runtime Context and System Reminders](guides/context-injection.md).

- **Typed system reminders** — first-class `Reminder` blocks with authority
  tiers (contextual vs. operator) and recording modes (recorded vs.
  model-only). A fixed priming rule appended to every system prompt tells
  models how much authority reminders carry.
- **Context injection** — `InjectContext` adds documents or content before
  generation; `HookContext.AppendReminder` injects mid-loop.
- **Compaction** _(experimental)_ — token-threshold summarization of
  conversation history, including mid-turn compaction: a PreIteration hook
  that shrinks the in-flight working set without touching persisted history.
  See [Compaction](guides/experimental/compaction.md).
- **Server-side context management** — Anthropic context-editing strategies
  (clear tool uses, clear thinking) configured per request via
  `WithContextManagement`.
- **Prompt caching** — automatic cache-breakpoint placement with TTL
  control, on by default for providers that support it.

## Human-in-the-Loop

Pausing for people. See the [Suspend and Resume Guide](guides/suspend-resume.md).

- **Suspend/resume** — any tool can pause the entire turn by returning
  `NewSuspendResult` (for approval, authentication, or external input).
  Resume statelessly with `WithResume` or session-backed with
  `WithToolResults`. Partial resumes are supported: supplying a subset of
  pending results keeps the turn suspended.
- **Dialog abstraction** — the `Dialog` interface models confirm, select,
  and text prompts, with terminal, auto-approve, and deny-all
  implementations. The core loop never calls it directly; permission hooks
  and tools wire it in.
- **AskUser tool** — lets the model itself ask the user questions mid-task.

## Background Work

- **Background tool results** — a tool returns `NewBackgroundResult` to
  dispatch long-running work to a managed goroutine. The agent receives a
  "started" message immediately; results are injected on a later turn and
  fire `PostBackgroundToolUse` hooks. Helpers: `AwaitBackgroundTasks`,
  `ContinueWithBackground`. Background goroutines are panic-safe and
  leak-free.

## Memory and Persistence

Conversation state in the `session/` package.

- **Sessions** — set `AgentOptions.Session` or `WithSession` and the agent
  loads history before each turn and saves after. State is an append-only
  event log: `Messages` returns the active window, `AllMessages` the full
  transcript.
- **Stores** — `MemoryStore` and `FileStore` (atomic writes, torn-write
  detection, optional fsync) behind a pluggable `Store` interface with
  listing, metadata, and usage rollups.
- **Forking and checkpoints** — `Fork` branches a conversation;
  compaction appends a non-destructive summary checkpoint while originals
  remain recoverable.
- **Suspension persistence** — suspended turns persist and survive process
  restarts; `CancelSuspension` abandons one.
- **Concurrency safety** — per-session locking serializes concurrent
  `CreateResponse` calls, with reentrancy detection (`ErrReentrantSession`).

## Safety and Governance

Controlling what the agent may do. See the
[Permissions Guide](guides/permissions.md).

- **Permission engine** — allow/ask/deny rules with Claude Code-style
  patterns (`Bash(go test *)`), modes (Default, Plan, AcceptEdits,
  BypassPermissions, DontAsk), and scoped session grants. Deny rules are
  evaluated first and are absolute — they win even in bypass mode.
- **Command, path, and domain matching** — shell-aware command splitting
  (catches `;`, `&&`, `|`, and command substitution), segment-safe path
  globs, and domain matching for web tools.
- **Workspace confinement** — `PathValidator` prevents path traversal
  outside the workspace for file tools.
- **Sandboxing** _(experimental)_ — Docker and macOS Seatbelt backends wrap
  command execution; a domain-allowlisting HTTP proxy enforces network
  policy with audit logging. See [Sandboxing](guides/experimental/sandboxing.md).
- **Settings files** _(experimental)_ — `.dive/settings.json` plus
  `.dive/settings.local.json` with Claude Code merge semantics for
  permissions and sandbox configuration.

## Skills and Slash Commands

Modular, file-based capabilities. See the [Skills Guide](guides/skills.md).

- **Unified skills system** — markdown skills with YAML frontmatter,
  discovered from `.claude/`, `.dive/`, and `.agents/` directories. Slash
  commands are skills without a description.
- **Three-layer integration** — rules in the system prompt, a typed catalog
  reminder appended model-only at the request tail, and a `Skill` tool that
  injects skill content on invocation.
- **Triggers and expansion** — keyword and pattern triggers, `$ARGUMENTS`
  and positional variables, and `!{command}` shell substitution for local
  skills.

## Model Layer

Cross-provider LLM access in `llm/` and `providers/`. See the
[LLM Guide](guides/llm-guide.md).

- **Eight providers, one interface** — Anthropic, OpenAI (Responses and
  Chat Completions), Google, Grok, Mistral, Ollama, and OpenRouter implement
  `LLM`/`StreamingLLM`. Every provider normalizes to one Anthropic-shaped
  response and content model, which is why sessions, compaction, A2A, and
  tracing all operate on a single representation.
- **Model registry** — model-name strings route to providers by
  prefix/pattern with a fallback, so `providers.CreateModel("gemini-2.5-pro")`
  just works.
- **Multimodal content** — images, documents, and files via base64 or URL,
  across providers.
- **Reasoning controls** — effort levels (low through max), budgets,
  adaptive thinking, thinking display, and fast mode, with per-model
  normalization (models that reject manual budgets fall back to adaptive
  thinking automatically).
- **Structured output** — JSON object and JSON schema response formats,
  plus tool-choice control (auto, any, none, or a specific tool).
- **Provider server-side tools** — Anthropic web search, code execution,
  and computer use; Grok web, X, and collections search; OpenAI web search
  preview, code interpreter, and file search.
- **Retry and resilience** — transient failures (429, 5xx) are classified
  and retried with exponential backoff; the retrying stream wrapper retries
  only before the first event is committed, so consumers never see duplicate
  events.
- **Cost and usage tracking** — every response carries token usage
  (including cache and reasoning tokens) automatically priced by a per-model
  pricing registry.
- **LLM-level hooks** — BeforeGenerate, AfterGenerate, and OnError
  callbacks around raw model calls, separate from agent hooks.

## Observability

See the [Tracing Guide](guides/tracing.md) and [OTel reference](guides/otel.md).

- **Tracer abstraction** — the `Tracer` interface produces nested
  AgentRun → Chat → ToolCall spans threaded through context. `NopTracer`
  (default) and `MultiTracer` live in core, so the core module carries no
  telemetry dependencies.
- **OpenTelemetry adapter** — the separate `dive/otel` module emits GenAI
  semantic-convention spans and metrics (operation duration, token usage,
  time-to-first-token), with opt-in capture of messages and tool I/O.

## Interoperability

- **MCP client** _(experimental)_ — connect to MCP servers over HTTP or
  stdio with full OAuth 2.0/PKCE support and pluggable token stores. Remote
  tools adapt into `dive.Tool` with filtering and approval configuration.
  See [MCP Integration](guides/experimental/mcp-integration.md).
- **Remote MCP connectors** — configure server-side MCP (the Anthropic and
  OpenAI connector APIs) directly on `ModelSettings.MCPServers`.
- **A2A** — expose any Dive agent as an Agent-to-Agent endpoint (JSON-RPC
  or REST, agent cards, streaming) and call remote A2A agents through
  `RemoteAgent`. Suspend/resume maps to the A2A `input-required` state for
  cross-agent human-in-the-loop round trips. See the [A2A Guide](guides/a2a.md).

## What Dive Deliberately Leaves Out

Dive is unopinionated about persona and policy. It does not install a system
prompt, a task policy, or an evaluation loop — you provide the prompt and
decide which tools and hooks to install. The one fixed behavior is the
reminder priming rule, which tells models how to interpret typed runtime
reminders; it explains authority but does not enforce behavior.
