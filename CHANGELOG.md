# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

## [1.8.1] - 2026-06-09

### Fixed

- **Background task cancellation** — background goroutines spawned by tools
  were prematurely cancelled when their tool batch completed, because they
  inherited the batch-scoped `childCtx`. They now receive the outer
  `CreateResponse` context via a new `withBackgroundCtx`/`backgroundCtxFrom`
  helper, so background tasks live for the full agent turn.
- **CLI temperature zero-value** — the `--temperature` flag was always written
  to `ModelSettings.Temperature` (even when not set), forcing every request to
  `temperature=0`. The CLI now uses `ctx.IsSet("temperature")` and only sets
  the field when the flag is explicitly provided.
- **Claude 5 temperature rejection** — Fable 5 and Mythos 5 reject the
  temperature parameter at the API level. The Anthropic provider now skips
  setting temperature for these models, and logs a warning when a non-nil
  temperature is silently dropped.

## [1.8.0] - 2026-06-09

### Added

- **Claude Fable 5 / Mythos 5** — new `ModelClaudeFable5` and
  `ModelClaudeMythos5` constants with pricing (1M context / 128k output),
  adaptive-thinking and native-effort support (all five levels including
  `xhigh`/`max`), an OpenRouter `anthropic/claude-fable-5` ID, and a CLI
  model-picker entry for Fable 5. The Anthropic default stays
  `claude-opus-4-8`.
- **`SequentialOnlyHint` tool annotation** — a tool that mutates shared state
  can opt out of parallel execution; any batch containing such a tool runs
  sequentially even when `ParallelToolExecution` is enabled.
- **Scoped session permission grants** — `AllowToolForSession(tool, pattern)`
  grants (tool, specifier)-scoped session approvals. Dialog approvals now
  grant the exact approved command/path (or WebFetch domain) instead of a
  whole tool category; `AllowForSession`/`IsSessionAllowed` are deprecated but
  still honored.
- **Partial-work error reporting** — a mid-turn LLM failure now returns
  `*GenerationError` carrying the accumulated `Usage`, `OutputMessages`, and
  `Items` (recover via `errors.As`). New sentinels: `ErrReentrantSession` (a
  tool calling back into its own session fails fast instead of deadlocking)
  and `ErrSessionNotSuspended` (resume against a non-suspended session is
  detected before the LLM call, not after).
- **Demos** — `demos/colosseum` (agent tournament arena with A2A remote
  players) and `demos/noodleville` (agent-driven village simulation).

### Security

- **Permission deny rules are now absolute** — deny rules evaluate before the
  session allowlist, and specifier-bearing deny rules fail closed when no
  specifier can be extracted. Bash patterns match per command segment,
  quote-aware (`Bash(go test *)` no longer authorizes
  `go test ./...; rm -rf /`; command substitution never matches an allow
  rule). File path specifiers are cleaned and segment-aware (`*` no longer
  crosses `/`; `..` traversal can't escape an allowed prefix), and WebFetch
  patterns match the real URL host so lookalike domains don't pass. Matching
  dispatches per tool through `DefaultSpecifierMatchers`, overridable via
  `Config.SpecifierMatchers`.
- **Skill hardening** — `!{...}` shell expansion runs against the raw template
  before argument substitution, so model-controlled arguments can't inject
  commands (`$1`–`$9`/`$ARGUMENTS` are passed to the shell as data); the Skill
  tool can no longer invoke user-invocable-only commands hidden from the
  catalog; `allowed-tools` is documented as informational only.
- **Toolkit fail-closed** — the file tools now return their workspace
  validator construction error instead of silently granting unrestricted
  filesystem access; Fetch's SSRF protection validates the IP actually dialed
  after DNS resolution (closing the DNS-rebinding window); Glob/Grep default
  excludes now match top-level directories like `node_modules/`.

### Changed

- **A2A final-answer extraction** — the server emits a single final artifact
  built from the last renderable assistant message (previously one artifact
  per message), and `RemoteAgent` extracts the latest artifact, so
  `TaskResult.Text` is the final answer rather than a tool-use preamble.
  Intermediate messages still stream as `working` status updates.
- **FileStore session aliasing** — `FileStore` caches the live `*Session` per
  ID, so every `Open` of the same ID returns the same shared, synchronized
  instance (fixes double-Open divergence that could silently delete turns on
  disk).
- **`settings.local.json` deep-merges** with `settings.json` instead of
  shadowing it: objects merge per key (local wins), arrays replace wholesale,
  and scalar keys present in the local file win.
- **OpenAI `WithMaxRetries`** is now the single retry knob: it configures
  Dive's retry loop and SDK-internal retries are disabled, eliminating
  double-retry amplification (up to 9 requests per persistent error). Also
  applies to Grok.

### Fixed

- **Agent loop** — data race on the response-item accumulator under parallel
  tool execution; extension `PostBackgroundToolUse` hooks were silently
  dropped; a PostToolUse hook setting `Result = nil` no longer panics or
  orphans the tool_use block; injected background-results messages are now
  persisted to the session; hook `Messages`/`Iteration` refresh every loop
  iteration; SessionStart hook `Values` are visible to later hooks; the
  per-session lock is context-aware.
- **Suspend/resume** — `WithResume` on a session-backed agent no longer drops
  prior history; suspend-phase usage now accumulates into `TotalUsage()`;
  sessions deep-copy messages on ingestion so later caller mutations can't
  rewrite stored history; resume tool-result merges are deterministic and
  survive message-replacing PreGeneration hooks.
- **Session durability** — a torn final JSONL line no longer makes a FileStore
  session permanently unreadable (healed on open); removed the 1 MB read cap
  that broke sessions containing large events; fixed `Put`/`List` races; an
  unrecognized header line no longer triggers a destructive overwrite.
- **Providers** — openaicompletions streaming no longer reports zero token
  usage (Mistral, OpenRouter); OpenAI stream content-block indices are no
  longer off by one; Anthropic web-search error results decode instead of
  failing the whole response; the Google stream iterator now emits usage, stop
  reason, sequential indices, and parallel tool calls, with unique synthetic
  tool-call IDs and no stdout debug logging; 502/529 are retryable while
  permanent errors are no longer retried in openaicompletions; cached and
  reasoning token details are parsed across providers; Anthropic no longer
  mutates caller-owned messages; registry routing of `/`-containing model IDs
  reaches the OpenRouter matcher.
- **Toolkit** — Grep `offset` and `-n` are honored (working pagination and
  line numbers on both search paths); Bash scanner failures return an error
  instead of silently truncating output; ReadFile offset/limit reads handle
  long lines; TextEditor's unbounded, racy file history was removed.
- **MCP (experimental)** — `Client.Close` actually closes the underlying
  client (no more subprocess leaks); the HTTP transport sends configured auth
  headers; a second OAuth flow no longer panics and the configured token store
  is honored; `RefreshTools` cleans up server-side-removed tools.

## [1.7.0] - 2026-05-29

### Added

- **Text-to-speech and transcription** — new `media.TextToSpeech` and
  `media.Transcribe` functions backed by `TextToSpeechProvider` /
  `TranscriptionProvider` interfaces, an `AudioFormat` type
  (mp3/opus/aac/flac/wav/pcm), and options for voice, voice instructions,
  speech speed, audio format, language, and transcription prompt. Supported on
  OpenAI (`gpt-4o-mini-tts`, `gpt-4o-transcribe`, `gpt-4o-mini-transcribe`,
  `gpt-4o-transcribe-diarize`, `whisper-1`) and Google
  (`gemini-2.5-flash-preview-tts`, `gemini-2.5-pro-preview-tts`,
  `gemini-3.1-flash-tts-preview`), with new text-to-speech and transcription
  examples.
- **Latest OpenAI models** — added `gpt-5.5`, `gpt-5.4-mini`, `gpt-5.4-nano`,
  and `gpt-image-2`; OpenAI defaults now use `gpt-5.5` for text and
  `gpt-image-2` for image generation. Reasoning effort is now normalized for
  known OpenAI, Grok, and Anthropic model families without tightening the
  public `llm.ReasoningEffort` string type.

## [1.6.0] - 2026-05-29

### Added

- **Claude Opus 4.8 / 4.7** model constants and pricing; Anthropic and
  OpenRouter now default to Opus 4.8. Added a `FastModeTextPricing` table.
- **Native Anthropic effort** — `WithReasoningEffort` maps to
  `output_config.effort` on Opus 4.5+/Sonnet 4.6 (older models keep the legacy
  effort→budget mapping). New levels `ReasoningEffortMinimal` (matches OpenAI
  gpt-5), `ReasoningEffortXHigh`, and `ReasoningEffortMax`.
- **Adaptive thinking** — `WithAdaptiveThinking()`, `WithThinking(...)`, and
  `WithThinkingDisplay(...)`; on adaptive-only models (Opus 4.7/4.8) a manual
  `WithReasoningBudget` falls back to adaptive thinking. `dive.ModelSettings`
  gains `Thinking`, `ThinkingDisplay`, and `Speed`.
- **Fast mode** — `WithSpeed(llm.SpeedFast)` sets the `fast-mode-2026-02-01`
  beta header; `Usage.Speed` reports the speed used.
- **Grok server-side tools** — `CodeExecutionTool` (sandboxed Python) and
  `CollectionsSearchTool` (`file_search`); `WebSearchTool` gains
  `EnableImageSearch`. New `docs/guides/grok.md` and examples. Adds `grok-4.3`
  (new default) plus new Gemini/Grok models and pricing.
- **Reasoning token usage** — `llm.Usage.ReasoningTokens` reports reasoning
  output tokens (OpenAI Responses, Grok); the streaming path now fills
  `CacheReadInputTokens`. `ResponsesIncludeProvider` lets a server-side tool
  request response `include` data.
- **Structured tool progress** — tools emit typed snapshots via
  `dive.ReportProgress(ctx, *dive.ToolProgress)`, delivered as
  `ResponseItemTypeToolProgress` events (latest-wins, distinct from the
  text-delta `StreamOutput`). Bash now reports line/byte/elapsed progress.
- **`SessionStartHook`** — fires once at the start of a fresh conversation to
  seed it from external state, returning a `*SessionStartResult` (durable or
  ephemeral via `Persist`).
- **Model-judgment hook helpers** — `PromptStopHook` and `PromptToolGate` let a
  model make a hook decision via a forced structured verdict.
- **Refusal `stop_details`** on `llm.Response`.

### Changed

- **Subagents promoted to stable** — the subagent layer moves out of
  experimental to core `subagent/` and `toolkit/orchestration/`, aligned with
  Claude Code's tool model. Adds built-in read-only `Explore`/`Plan` personas;
  `DisallowedTools` is now enforced in `FilterTools` and parseable from the
  `disallowed-tools` key in `.dive/agents/*.md` frontmatter. The
  subagent-spawner tool is `Agent` (renamed from the experimental `Task`), with
  the `Task*` prefix reserved for background control (`TaskStop`/`Monitor`).
- **Non-destructive compaction** — `Compact()` inserts a checkpoint instead of
  collapsing the log. `Messages()` returns the active window (latest summary +
  everything after); new `AllMessages()` returns the full transcript, and
  `CompactionHistory()` returns one `CompactionRecord` per checkpoint.

### Fixed

- Effort/thinking requests no longer 400 on Opus 4.7/4.8 (which reject manual
  thinking budgets); `ReasoningEffort` with `Thinking: disabled` on a legacy
  model now errors instead of silently re-enabling thinking.
- Corrected Grok 4.20 pricing ($1.25/$2.50 per 1M tokens) and the X-search
  handle limit (now 20, was capped at 10).
- `file_search` / collections-search responses now decode instead of returning
  an "unsupported" error.

## [1.5.0] - 2026-05-15

### Added

- **`Extension` interface** for composable agent capabilities. Extensions bundle
  tools, hooks, and system prompt rules and are merged during `NewAgent` via
  `AgentOptions.Extensions`.
- **Agent suspend/resume** for out-of-process tool results. A tool returns
  `NewSuspendResult` to pause the agent; the response comes back with
  `Status == ResponseStatusSuspended` and a `Suspension` state for later
  resumption via `WithToolResults` or `WithResume`. `SuspendableSession` adds
  auto-persistence; the `OnSuspend` hook fires before persistence.
- **`Tracer` interface** for agent observability (tracing, metrics, audit
  logging) with `StartAgentRun` / `StartChat` / `StartToolCall`. `NopTracer`
  and `MultiTracer` ship in core; the OpenTelemetry adapter lives in the
  promoted `dive/otel` module.
- **A2A (Agent-to-Agent) support** as a stable submodule (`a2a/`), built on the
  official `a2a-go/v2` SDK. `Server` exposes a Dive agent as JSON-RPC or REST;
  `RemoteAgent` calls remote A2A endpoints. Suspend/resume maps to the A2A
  `input-required` state. Static and dynamic agent cards supported.
- **Background tool execution** — tools can opt into running in the background
  while the agent continues, with results returned later.
- **Skill system** as a stable package (`skill/`) — unified skills and slash
  commands implementing the `Extension` interface, with provider-based loading
  (filesystem, `.agents/skills/`), variable expansion, and trigger matching.
  Supports agentskills.io standard frontmatter fields in `SkillConfig`.
- **Media generation tools** for images and videos with path traversal
  protection, duration schema, and aspect ratio controls.
- **CLI enhancements**: `models` command, interactive model switcher, status
  line in the input area, hanging indent for assistant messages, and broad UI
  polish.

### Changed

- **Subagent reliability** improvements with auto-retrieval of nested agent
  results.
- **Ollama provider** switched to the Anthropic Messages API; adds
  `provider/model` syntax for unambiguous routing.
- **Skip retrying permanent errors** in provider retry loops (auth failures,
  4xx client errors).
- **Promoted out of experimental**: `dive/otel`, `a2a` (renamed from `a2alib`),
  and `toolkit/firecrawl` are now stable submodules.
- **Upgraded dependencies**: OpenTelemetry 1.40→1.41, wonton 0.0.29→0.0.34.

## [1.4.0] - 2026-03-25

### Added

- **Grok provider** as a standalone Go submodule (`providers/grok/`). Built on the
  OpenAI Responses API with support for Grok 4.20 models (reasoning, non-reasoning,
  multi-agent).
- **Server-side tools for Grok**: `WebSearchTool` (web search with domain filters and
  image understanding) and `XSearchTool` (X/Twitter search with handle filters, date
  ranges, and media understanding).
- **Prompt caching for Grok** via `WithPromptCacheKey(key)` option for cache reuse
  across requests.
- **OpenAI provider extensions**: `ResponsesToolProvider` interface for custom tool
  types and `WithExtraRequestOptions` for per-request SDK options.

### Changed

- **Upgraded dependencies**: grpc v1.79.3, jsonparser v1.1.2 (DoS fix),
  openai-go v3.29.0, genai v1.51.0.

## [1.3.0] - 2026-03-12

### Changed

- **Stream parallel tool results as they complete.** `ToolCallResult` events and
  `PostToolUse` hooks now fire as soon as each tool finishes, rather than waiting
  for all parallel tools to complete. Callbacks remain single-threaded via a channel
  consumer. Result events now arrive in completion order, not declaration order.

## [1.2.0] - 2026-03-11

### Added

- **Parallel tool execution** via `AgentOptions.ParallelToolExecution` (defaults to `false`).
  When enabled and the LLM returns multiple tool calls in one message, they execute
  concurrently via goroutines. Hooks and event callbacks remain serialized for safety.
  Three-phase design: pre-hooks run sequentially, tools execute in parallel, post-hooks
  run sequentially.

## [1.1.0] - 2026-03-10

### Changed

- **Upgrade OpenAI Go SDK from v1 to v3** (`openai-go` v1.12.0 → v3.26.0). All SDK
  migration handled internally in `providers/openai`; Dive's public API is unchanged.
  Streaming reasoning deduplicated and per-summary-part tracking added.
- **Update provider models and features for March 2026.** Anthropic: claude-sonnet-4-6,
  new beta features. OpenAI: gpt-5.4 (new default), gpt-5.3, gpt-5.1-mini, o3-mini.
  Google: gemini-3.1-pro/flash variants. Grok: removed deprecated grok-2 models.
- **Upgrade all dependencies to latest versions.** Key bumps: mcp-go v0.43→v0.45,
  golang.org/x/net v0.50→v0.51, googleapis/gax-go v2.17→v2.18, opentelemetry
  v1.40→v1.42, grpc v1.78→v1.79, genai SDK v1.46→v1.49.

## [1.0.1] - 2025-02-09

### Changed

- **Decoupled root module** from provider and experimental sub-modules. The root
  `go.mod` no longer depends on `providers/google`, `providers/openai`, or
  `experimental/mcp`, significantly reducing transitive dependencies for consumers
  who only need the core library.
- **Added `examples/` module** with its own `go.mod` to hold example code separately
  from the core module.
- **Added `tag-modules` Makefile target** for tagging all sub-modules in one step
  (`make tag-modules VERSION=v1.0.0`).
- **Added `examples` to `tidy-all`** module list in Makefile.

## [1.0.0] - 2025-02-09

First official stable release. Major architectural overhaul from v0.0.x with a simpler
agent API, a new hook system, and clearly separated core vs experimental packages.

### Added

- **Hook system** with 7 hook types (`PreGeneration`, `PostGeneration`,
  `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PreIteration`, `Stop`)
  and shared `*HookContext`. Built-in hooks for context injection, compaction,
  and usage logging.
- **Session package** (`session/`) with `FileStore` and `MemoryStore` backends,
  plus fork and compact operations.
- **Permission package** (`permission/`) promoted to core. Rule-based tool
  permissions with modes, specifier patterns, and session allowlists.
- **`FuncTool[T]`** for creating tools from functions with auto-generated schemas.
- **`Toolset` interface** for dynamic per-request tool resolution.
- **Provider registry** with self-registering providers via `init()`.
- **Gemini 3 models** (`gemini-3-pro-preview`, `gemini-3-flash-preview`).
- **Tool panic recovery**, `OutputMessages` on Response, `llms.txt`.

### Changed

- **Agent is a concrete struct**, not an interface. `SystemPrompt` replaces
  `Instructions`. `AgentOptions` streamlined with `Hooks`, `Toolsets`, `Session`.
- **Toolkit constructors** return `*dive.TypedToolAdapter[T]` with variadic options.
- **CLI moved** to `experimental/cmd/dive/`.
- **Provider defaults updated**: Anthropic `claude-opus-4-6`, OpenAI `gpt-5.2`,
  Google `gemini-2.5-pro`. Pricing updated across all providers.
- **Experimental boundary**: MCP, sandbox, skills, slash commands, subagents,
  compaction, todo, settings, and extended toolkit moved under `experimental/`.

### Removed

- **Groq provider**.
- **Thread system** replaced by `dive.Session` interface.
- **Interactor and Confirmer** replaced by hooks and the permission package.
- **Subagent loader and compaction** from core (available in `experimental/`).
