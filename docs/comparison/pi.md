# pi (Earendil) — Architecture Analysis for Dive

- **Repo:** `github.com/earendil-works/pi`, checked out at `~/git/lib/pi`
- **Commit analyzed:** `2b0a123de` ("feat(coding-agent): remove themes section from startup banner"), 2026-09-26. The history has about 6,550 commits since 2025-08-09.
- **Size:** about 170k lines of non-test TypeScript across 12 packages. The biggest are `coding-agent` (~76k), `agent` (~31k), `ai` (~25k) and `tui` (~19k).
- **Method:** I read the source directly. Paths are relative to the repo root. `H/` is shorthand for `packages/agent/src/harness/` and `CA/` for `packages/coding-agent/`.
- **Caveat:** pi is not a Go library, and it isn't a peer framework in the Eino/ADK/MAF sense. It is a coding agent, built on its own LLM and agent libraries. I compare it on those two libraries (`pi-ai`, `pi-agent-core`) and on the durable runtime the team is building beneath the product.

---

## 1. Overview

pi is Earendil's terminal coding agent (pi.dev). It is widely used and heavily extended, and it has become a reference point for the "minimal harness, maximal extensibility" school of agent design. The repo holds the product and the layers underneath it. Each layer is a separately published npm package.

### Philosophy

The stance is explicit, and the code follows it.

- **"Pi is a minimal, extensible AI agent for the terminal. Adapt Pi to your workflow, not the other way around."** (`CA/README.md:15`).
- **Deliberate omissions.** The pre-2026-09-22 README said so outright (`git show 25cc5c7bf^:packages/coding-agent/README.md:533-549`): *"No MCP… No sub-agents… No permission popups… No plan mode… No built-in to-dos. They confuse models… No background bash. Use tmux."* Each omission ships as an **example extension** instead: `permission-gate.ts`, `plan-mode/`, `subagent/`, `todo.ts`, `sandbox/`, `ssh.ts` in `CA/examples/extensions/`.
- **No permission system.** The root README says: *"Pi does not include a built-in permission system for restricting filesystem, process, network, or credential access"* (`README.md:42`). Containment is left to the environment: a micro-VM (Gondolin), Docker, or OpenShell.
- **A "least powerful mechanism" ladder** for customization: AGENTS.md → prompt template → skill → extension → TUI component → custom provider → package (`CA/docs/quickstart.md:96-106`).

Under that minimal product sits some of the most rigorous durability engineering in any agent codebase I've read. It is the new `pi-agent-core` **harness**, whose normative spec (`packages/agent/docs/harness.md`) runs to about 1,470 lines. Its successor, "Pico5", is specified in `packages/durable/docs/pico-v5.md`. The shipping CLI does not run on the harness yet: it runs on the older in-memory `Agent` plus a JSONL session tree. The harness powers only `CA/src/experimental/*`, the client/server worker mode. §4 separates what ships from what is specified.

### Repo layout

| Package | npm name | Responsibility |
|---|---|---|
| `ai` | `pi-ai` | Unified LLM API: `Models`/`Provider`/`Model`, one `AssistantMessageEvent` stream protocol, 10 wire "APIs" (Anthropic, OpenAI Completions/Responses/Codex, Azure, Google, Vertex, Bedrock, Mistral, `pi-messages`), ~40 provider factories, OAuth, a generated model catalog with costs and compat flags, retry and overflow classifiers, a faux provider |
| `agent` | `pi-agent-core` | (a) the **core loop** `agent-loop.ts` + stateful `Agent` class, in memory; (b) the **harness**: durable lanes, operations, session storage (memory/JSONL), compaction, hooks, events; (c) `pico3`, an experimental task-kernel redesign |
| `coding-agent` | `pi-coding-agent` | The CLI: `AgentSession` (a 4k-line wrapper over `Agent`), session tree manager, tools (read/bash/edit/write/grep/find/ls), extension runtime, TUI/print/JSON/RPC modes, SDK. `src/experimental/` holds the harness-based client/server worker |
| `durable` | `pi-durable` | The Pico5 kernel: atomic multi-record `Storage.commit`, conversations/entries/tasks/submissions/documents, memory/JSONL/SQLite backends. **Only the storage and transaction layers exist, and nothing imports it yet** |
| `session-backends/sqlite-node` | `pi-session-backend-sqlite-node` | SQLite implementation of the *harness* `Storage` interface |
| `chord` | `@earendil-works/chord` | App-composition runtime: services, a Go-style `Context`, single-writer replicated JSON state with sequenced delta ops, facets (plugins) |
| `protocol` / `server` / `client` | `pi-protocol` etc. | Experimental remote UI: length-prefixed CBOR envelopes carrying Chord service calls and replicated-state updates over a Unix socket or WebSocket relay |
| `telemetry` | `pi-telemetry` | Vendor-neutral, callback-scoped span contracts with typed schemas |
| `tui`, `evals` | | Differential-rendering terminal UI; Docker-isolated "documentation lift" evals |

---

## 2. Core packages and types

### 2.1 `pi-ai`: provider, model, message, stream

**Three nouns, cleanly separated** (`packages/ai/src/types.ts`, `models.ts`):

- **API** is a wire dialect such as `"anthropic-messages"` or `"openai-completions"`. `type Api = KnownApi | (string & {})` (`types.ts:17-29`) is open, but known values get typed options through `ApiOptionsMap` (`:257-276`).
- **Provider** is a runtime unit that owns auth, its model list and the operations (`models.ts:143-229`).
- **Model** is plain data that names its `api`, `provider`, `baseUrl`, costs, limits and **compat flags** (`types.ts:1062-1108`).

Dispatch goes `Models` → owning `Provider` → the API module for `model.api`.

```ts
export interface Provider<TApi extends Api = Api> {
  readonly id: string; readonly name: string; readonly auth: ProviderAuth;
  getModels(): readonly Model<TApi>[];                 // sync; dynamic providers refresh separately
  refreshModels?(context: RefreshModelsContext): Promise<void>;
  stream<T extends TApi>(model: Model<T>, context: TranscriptContext, options?: ApiStreamOptions<T>): AssistantMessageEventStream;
  streamSimple(model: Model<TApi>, context: TranscriptContext, options?: SimpleStreamOptions): AssistantMessageEventStream;
  fetchDeferred?(model, handle: DeferredHandle, options?): AssistantMessageEventStream;   // background/batch responses
  cancelDeferred?(model, handle, options?): Promise<void>;
  generateImages?(...): Promise<AssistantImages>;  classify?(...): Promise<ClassifierResult>;
}
```

There are two request tiers:
- `stream()` takes provider-specific options.
- `streamSimple()` takes the provider-neutral `SimpleStreamOptions` (`reasoning: ThinkingLevel`, `toolChoice`, `thinkingBudgets`, `deferred`) (`types.ts:343-351`).

`Model.thinkingLevelMap` maps pi's six levels (`minimal…max`) to provider values, and `null` marks a level as unsupported (`types.ts:85-87, 1088`).

**Messages.** Four roles, content blocks, plain JSON. The whole `Context` is `JSON.stringify`-serializable by design:

```ts
export interface SystemMessage { role: "system"; content: string | TextContent[];
  sections?: Record<string, string | null>;   // named prompt sections; later messages patch by name, null deletes
  toolsAdded?: Tool[]; toolsRemoved?: ToolReference[]; timestamp: number; }
export interface UserMessage { role: "user"; content: string | (TextContent | ImageContent)[]; timestamp: number; }
export interface AssistantMessage { role: "assistant"; content: (TextContent | ThinkingContent | ToolCall)[];
  api: Api; provider: ProviderId; model: string; responseModel?: string; responseId?: string;
  providerThinkingLevel?: string; diagnostics?: AssistantMessageDiagnostic[]; usage: Usage;
  stopReason: StopReason; deferred?: DeferredHandle; errorMessage?: string; rawStopReason?: string; timestamp: number; }
export type ToolResultMessage<TDetails> = { role: "toolResult"; toolCallId: string; toolName: string;
  content: (TextContent | ImageContent)[]; details?: TDetails; usage?: Usage; isError: boolean; timestamp: number; };
export type StopReason = "pending" | "stop" | "length" | "toolUse" | "error" | "aborted" | "deferred";
export interface ThinkingContent { type: "thinking"; thinking: string; thinkingSignature?: string; redacted?: boolean; }
export interface ToolCall { type: "toolCall"; id: string; name: string; arguments: JsonObject; thoughtSignature?: string; namespace?: string; }
```
(`types.ts:388-418, 443, 515-577`)

Four details stand out:

1. **Every `AssistantMessage` is stamped with `api`, `provider` and `model`.** That provenance is what makes cross-provider replay decidable (§2.1 handoff).
2. **Errors are values.** A failed call is still an `AssistantMessage`, with `stopReason: "error" | "aborted"` and `errorMessage`. It is persisted like any other message and filtered out when context is rebuilt.
3. **`ToolResultMessage.details`** is a typed, JSON-constrained side channel for UI and state reconstruction that never reaches the model. Only `content` does.
4. **System messages live in the transcript.** A later system message *patches* the prompt (`sections`) and the tool set (`toolsAdded`/`toolsRemoved`), and replaying the list yields the current prompt and tools (`utils/transcript.ts`). A branded `TranscriptContext` type (`types.ts:711-714`) ensures only `normalizeContext()` output reaches providers. Providers that support mid-conversation system messages (compat `supportsMidConvoSystemMessages`) get them in place. On Anthropic, tool changes go out as `defer_loading` additions, so **changing tools doesn't invalidate the prompt cache**. All other providers get the history collapsed back into a leading system message (README "System Messages").

**Stream protocol.** There is one event union for every provider:

```ts
export type AssistantMessageEvent =
  | { type: "start"; partial: AssistantMessage }
  | { type: "text_start" | "thinking_start" | "toolcall_start"; contentIndex: number; partial: AssistantMessage }
  | { type: "text_delta" | "thinking_delta" | "toolcall_delta"; contentIndex: number; delta: string; partial: AssistantMessage }
  | { type: "text_end"; contentIndex: number; content: string; partial: AssistantMessage }
  | { type: "toolcall_end"; contentIndex: number; toolCall: ToolCall; partial: AssistantMessage } // (+ thinking_end)
  | { type: "done"; reason: "stop" | "length" | "toolUse" | "deferred"; message: AssistantMessage }
  | { type: "error"; reason: "aborted" | "error"; error: AssistantMessage };
```
(`types.ts:732-748`)

The contract is documented (`:716-731`, `:353-363`):
- `start` precedes all updates.
- Block content grows only through deltas and is authoritative at `*_end`.
- Blocks may interleave by `contentIndex`.
- **Once a stream is returned, failures must be encoded in it**. Only missing auth may throw synchronously.

`AssistantMessageEventStream` is a push-based async iterable with a `result(): Promise<AssistantMessage>` (`utils/event-stream.ts:26-105`). Its queue is **unbounded**: producers never wait for consumers.

`AssistantMessageFrame` (`utils/assistant-message-frame.ts:8-35`) is a compact, **replayable** encoding of the same stream. It drops the cumulative `partial` and adds a `toolcall_checkpoint`. `reduceAssistantMessageFrames()` rebuilds the partial message. The harness persists these frames for crash recovery (§4), and the proxy and `pi-messages` wire protocols reuse the same delta-only shape (`packages/agent/src/proxy.ts:34-60`, `ai/src/api/pi-messages.ts:52+`).

**Dialect normalization lives in compat flags on the model, not in code forks.** `OpenAICompletionsCompat` has about 30 flags (`types.ts:754-832`):
- `supportsDeveloperRole`, `maxTokensField`, `requiresToolResultName`, `requiresAssistantAfterToolResult`;
- `requiresThinkingAsText`, `thinkingFormat`, `chatTemplateKwargs`;
- `supportsStrictMode`, `cacheControlFormat`, `sessionAffinityFormat`, `zaiToolStream`, …

Anthropic, Responses, Bedrock and Mistral have their own sets. When the catalog doesn't set flags, `detectCompat()` falls back to URL sniffing (`api/openai-completions.ts:1585-1690`). The generated catalog (`scripts/generate-models.ts` → `models.generated.ts`) sets them for verified models. About 160 test files cover dialect edge cases, for example `google-shared-gemini3-unsigned-tool-call`, `anthropic-empty-thinking-signature-compat`, `azure-openai-responses-reasoning-replay` and `cross-provider-handoff.test.ts`.

**Cross-provider handoff** is `transformMessages(messages, model, normalizeToolCallId?)` (`api/transform-messages.ts:64-235`), which every adapter calls. It works in two passes.

The first pass rewrites each assistant message according to whether its `provider/api/model` equals the target model:
- **Same model:** keep thinking blocks with signatures, including empty-text encrypted reasoning; keep redacted thinking.
- **Different model:** drop redacted thinking; convert thinking to **plain text**; strip `thoughtSignature`; remap tool call IDs through the adapter's `normalizeToolCallId` and carry the map over to the matching tool results.

It also downgrades images to text placeholders when the target lacks vision.

The second pass works on turn structure:
- It **skips** assistant messages with `stopReason` `error` or `aborted` entirely: "Replaying them can cause API errors… The model should retry from the last valid state".
- It **synthesizes `isError` "No result provided" results** for orphaned tool calls.
- It holds system messages that fall between a call and its results until after the results.

The README says cross-model thinking becomes `<thinking>`-tagged text. The code emits untagged text (`:113-116`), so the docs have drifted.

**Retry and overflow classification** are pure functions over `AssistantMessage`:
- `isRetryableAssistantError()` matches `errorMessage` against about 50 transient patterns and a non-retryable quota/billing list (`utils/retry.ts:1-247`).
- `isContextOverflow()` matches about 25 provider-specific overflow patterns, including *silent* overflow (usage.input ≥ contextWindow, or z.ai/Xiaomi `length` with zero output) (`utils/overflow.ts`).
- `retryAssistantCall(produce, policy, signal, callbacks)` gives exponential backoff with an abortable sleep, and normalizes an abort during backoff into an `aborted` message.

The classification is string-based. There are no typed error kinds.

**Other notable pieces:**
- A `faux` provider for deterministic tests (`providers/faux.ts`).
- `onPayload` (inspect or replace the raw request), `onResponse` and `onProviderStreamEvent` observers (`types.ts:132-199`).
- **Deferred responses** (`fetchDeferred`/`cancelDeferred` with a durable `DeferredHandle`) for provider background and batch jobs.
- Lazily imported API modules keep the core side-effect-free (`api/lazy.ts`, `index.ts:4-8`).

### 2.2 `pi-agent-core`, part 1: the core loop and `Agent`

`agent-loop.ts` (898 lines) is a pure function over a context snapshot. It does no persistence:

```ts
export function agentLoop(prompts: AgentMessage[], context: AgentContext, config: AgentLoopConfig,
  signal: AbortSignal | undefined, streamFn: StreamFn): EventStream<AgentEvent, AgentMessage[]>;
export function agentLoopContinue(context, config, signal, streamFn): EventStream<AgentEvent, AgentMessage[]>;
export async function runAgentLoop(prompts, context, config, emit: AgentEventSink, signal, streamFn): Promise<AgentMessage[]>;
export type AgentEventSink = (event: AgentEvent) => Promise<void> | void;   // awaited → backpressure
```
(`agent-loop.ts:31-150`)

`AgentLoopConfig extends SimpleStreamOptions` (`types.ts:189-338`) is a bag of optional callbacks:
- **Context:** `convertToLlm` (required; `AgentMessage[] → Message[]`), `transformContext`, `getApiKey` (called per request, for expiring OAuth tokens).
- **Turn control:** `prepareRequest` (replace context, model or thinking level before *every* request), `prepareNextTurn`, `finishTurn` (returns `{action:"continue"|"end"}`).
- **Queues:** `getSteeringMessages`, `getFollowUpMessages`.
- **Tools:** `toolExecution: "sequential" | "parallel"` (default parallel), `beforeToolCall`, `afterToolCall`.

Every callback's contract says *must not throw*.

**App-defined message types come in through declaration merging:**
```ts
export interface CustomAgentMessages { /* apps extend via `declare module` */ }
export type AgentMessage = Message | CustomAgentMessages[keyof CustomAgentMessages];
```
(`types.ts:361-370`)

The coding agent adds `bashExecution`, `custom` and `compactionSummary` roles, which `convertToLlm` maps to user messages or drops. Go has no declaration merging. The Go equivalent is an open interface (`type Message interface{ Role() string }`) plus a registry for JSON decoding, or a `Custom{Type string; Data json.RawMessage}` variant.

**Events** (`types.ts:485-500`):
```ts
export type AgentEvent =
  | { type: "agent_start" } | { type: "agent_end"; messages: AgentMessage[] }
  | { type: "turn_start" } | { type: "turn_end"; message: AgentMessage; toolResults: ToolResultMessage[] }
  | { type: "message_start" | "message_end"; message: AgentMessage }
  | { type: "message_update"; message: AgentMessage; assistantMessageEvent: AssistantMessageEvent }
  | { type: "tool_execution_start"; toolCallId; toolName; args }
  | { type: "tool_execution_update"; toolCallId; toolName; args; partialResult }
  | { type: "tool_execution_end"; toolCallId; toolName; result; isError: boolean };
```

`message_start`/`message_end` fire for *every* message: user, system, assistant, tool result. A consumer that folds `message_end` therefore rebuilds the transcript exactly. That is the whole state model of the `Agent` class (`agent.ts:563-612`).

`Agent` (`agent.ts`, 613 lines) wraps the loop with:
- mutable `AgentState` (`messages`, `tools`, `model`, `thinkingLevel`, `isStreaming`, `streamingMessage`, `pendingToolCalls`);
- steering and follow-up queues (`steer()`, `followUp()`, mode `"all" | "one-at-a-time"`);
- `abort()`, `waitForIdle()`, `prompt()`, `continue()`;
- `subscribe(listener)`, whose listeners are **awaited in order and are part of run settlement**.

### 2.3 `pi-agent-core`, part 2: the durable harness

The harness (`H/`) is a separate runtime. It shares only message and tool types with the core loop and imports neither `agent.ts` nor `agent-loop.ts`.

**Vocabulary** (`H/runtime/types.ts`, `H/session/types.ts`):
- **Session:** the durable store. It holds an immutable entry tree (`id`/`parentId`), typed mutable **values** and append-only **lists** at namespaced addresses, and a usage ledger.
- **Lane:** a Branch tip, its configuration, an inbox of queued inputs, and at most one **operation**. Several lanes can share one Session tree.
- **Operation:** one accepted unit of work, `run | compaction | navigation`. It is stored as write-once `OperationMeta` plus an `OperationState` that is **replaced in full after every transition**.
- **Drive:** a process-local pass that advances one operation until it settles or reaches a durable wait (a retry `notBefore` or a deferred handle).

```ts
export type OperationState = StartingOperation | CheckpointOperation | AssistantReadyOperation
  | AssistantEffectPendingOperation | AssistantRetryWaitOperation | ToolsOperation
  | DeferredSuspendedOperation | DeferredEffectPendingOperation | SummaryDecidingOperation
  | SummaryReadyOperation | SummaryEffectPendingOperation | SummaryRetryWaitOperation
  | NavigationReadyToCommitOperation;                                         // 13 flat leaves
export type ToolCall = { sourceIndex: number; resultEntryId: string } & (
  | { status: "planned" } | { status: "effect_pending"; replay: "never" | "safe" }
  | { status: "outcome_ready"; terminate: boolean } | { status: "completed"; terminate: boolean });
export type Control = { status: "running" } | { status: "cancel_requested"; requestedAt: number };
export type Continuation = { kind: "need_assistant"; overflowRecoveryUsed: boolean }
                         | { kind: "may_finish"; includeFinalAssistant: boolean };
```
(`H/session/types.ts:250-329`)

**Storage** is one interface with memory, JSONL and SQLite implementations, plus an exported conformance suite:
```ts
export type Write = EntryWrite | UsageWrite | ValueWrite | ListWrite;
export interface Storage {
  commit(writes: Write[], context: Context): Promise<CommitResult>;     // atomic; one seq per write
  getEntries(ids, ctx); getValue<T>(address: Value<T>, ctx); scanValues<T>(prefix, ctx);
  readList<T>(address: ValueList<T>, options, ctx); scanBranch(query, ctx); scanEntries(query, ctx);
  scanUsage(query, ctx); getStats(ctx); close(ctx);
}
export type EntryType = "message" | "compaction" | "branch_summary" | "custom";
```
(`H/session/types.ts:16-64, 388-471`)

Addresses are phantom-typed: `value<T>(namespace, key)` and `list<T>(…)` (`H/session/values.ts:22-30, 98-106`). The durable key layout is `pi.op.state/<op>`, `pi.op.tool_args/<op>:<step>:<i>`, `pi.op.tool_memo/<op>:<inv>:<name>`, `pi.pending.entry/<id>`, `pi.pending.assistant_frame/<op>:<resp>`, `pi.lane.state/<lane>`, `pi.result/<op>` (`values.ts:158-195`). In Go, generic methods aren't allowed, so this becomes `type Value[T any] struct{ NS, Key string }` with free functions `Get[T]` and `Set[T]`.

`Context` here is **Chord's `Context`**: `{ abortSignal; value<T>(key) }` (`packages/chord/src/types.ts:15-19`). It is a deliberate copy of Go's `context.Context`, so it maps one-to-one.

### 2.4 `pi-durable` (Pico5): the target kernel

Pico5 generalizes the harness into **tasks with checkpointed phases**. Generation, tools and compaction are all task kinds. A single `Storage.commit(writes)` atomically commits entries, whole task records, submissions and Chord-delta documents under **one `Seq` per commit** (`packages/durable/src/types.ts:513-529, 665-749`).

```ts
export type TaskState<S, R> = { status: "pending"; checkpoint: S } | { status: "running"; checkpoint: S }
                            | { status: "terminal"; outcome: TaskOutcome<R> };   // completed|failed|aborted|orphaned|faulted
export type EntryRecord = { id: EntryId; conversationId; kind: string;
  model?: readonly Message[]; data?: JsonValue;       // model-visible vs app payload, separated
  head?: EntryId; edits?: readonly ContextEdit[];     // compaction/redaction as appends, never deletes
  byTaskId?: TaskId };
type SubmissionRecordBase = { id; conversationId; /** Host-provided deduplication key */ requestId?: string };
```
(`durable/src/types.ts:185-200, 209-281, 335-387`)

Implementation status:
- **Built:** the record types, the storage kernel, the three backends, poisoning on uncertain storage failure, `ReadAfterWrite` enforcement, and a 1,520-line conformance suite.
- **Specified only:** the scheduler, phases, tools and the harness replacement (`docs/pico-v5-handoff.md:14`; `pico-v5.md:269` says "Pico5 is not implemented yet").

### 2.5 `pi-coding-agent`

`AgentSession` (`CA/src/core/agent-session.ts`, 4,023 lines) wraps `Agent` and adds persistence, extensions, compaction, retry, queues and the prompt and tool loadout.

It works by **assigning and chaining the Agent's hook fields** in its constructor (`:435-441`): `beforeToolCall`/`afterToolCall` → extension `tool_call`/`tool_result`, `prepareRequest` → the session projection, `prepareNextTurnWithContext` → threshold compaction, `finishTurn` → the extension boundary.

**The session file is authoritative.** Every request re-derives its messages from `sessionManager.buildSessionProjection()` (`:611-636`).

**Session entries** form an append-only JSONL tree (`CA/src/core/session-manager.ts:183-194`):
```ts
export type SessionEntry = SessionMessageEntry | ThinkingLevelChangeEntry | ModelChangeEntry | UsageEntry
  | CompactionEntry | BranchSummaryEntry | CustomEntry | CustomMessageEntry | ContextEditEntry | LabelEntry | SessionInfoEntry;
```
- `CustomEntry` holds extension state and is not sent to the LLM.
- `CustomMessageEntry` holds extension content that is sent to the LLM.
- `ContextEditEntry` (`replacement: {content} | null`) omits or replaces an earlier entry's contribution **without mutating history**.

**`ToolDefinition`** (`CA/src/core/extensions/types.ts:461-519`) adds the following to the core `AgentTool`:
- `label`;
- `promptSnippet` (one line in the generated `<tools>` prompt section) and `promptGuidelines` (bullets appended to `<rules>` while the tool is active);
- `renderCall`/`renderResult`;
- an `ExtensionContext` passed to `execute`.

The system prompt is built as named sections (`preamble, tools, rules, docs, project_context, skills, cwd, …`), **diffed** before each request, and appended as a patch `SystemMessage` only when something changed (`agent-session.ts:1410-1423`, `system-prompt.ts:198-210`).

### 2.6 `protocol`, `server`, `client`, `chord`

Covered in §8. In short: 8 envelope types in CBOR carrying Chord service calls. UI state is a **replicated JSON document** updated by sequenced delta ops, not an event log.

---

## 3. Key concepts and how they relate

```text
                         pi-ai
  Model (data: api, provider, compat, cost) ──► Provider ──► API module (wire dialect)
  Context{messages[System|User|Assistant|ToolResult]} ─normalizeContext─► TranscriptContext
        │                                 transformMessages (cross-provider replay)
        ▼
  AssistantMessageEventStream  (start / *_start / *_delta / *_end / done|error)
        │                         └─► AssistantMessageFrame (replayable, persisted by harness)
        ▼
 ┌──────────────────────── pi-agent-core ─────────────────────────────────────────┐
 │ (A) agent-loop (in-memory)                 (B) harness (durable)               │
 │   runLoop: steer → prepareRequest →          Lane ─ Operation(meta + state)     │
 │   stream → tools → finishTurn → followUp     13-leaf state machine; Drive pass  │
 │   AgentEvent sink (awaited)                  intent commit → effect → settle    │
 │   Agent class: state reducer + queues        Session: entry tree + values +     │
 │                                              lists + usage (memory/JSONL/SQLite)│
 │                                              Hooks (before_tool …), EventBus    │
 └────────────────────────────────────────────────────────────────────────────────┘
        ▲ ships today                               ▲ experimental worker mode
 pi-coding-agent: AgentSession (wraps Agent)     session-worker → Chord services
   SessionManager JSONL tree, ContextEdit,         → pi-protocol/server/client
   extensions (ExtensionAPI.on/registerTool),      replicated LaneSnapshot doc
   compaction, auto-retry, TUI/print/json/rpc
        ▼ future
 pi-durable (Pico5): tasks w/ checkpointed phases, one Seq per atomic commit
```

How the concepts relate:

1. **The message list is the model's view, never the record.** Both the coding agent and the harness store an entry tree and *project* messages from it. The coding agent does this with `buildSessionProjection`, and `context_edit` entries hide retried or failed attempts. The harness does it with `H/session/context.ts`, which filters `error`/`aborted`/`deferred` assistant entries and applies compaction. Pico5 splits `model` from `data` at the entry level. This is Dive v2's principle 1, "one record, many projections", arrived at independently.
2. **The turn is not the aggregate. The operation is.** A harness *operation* is exactly Dive's Turn: one accepted input, durable identity, a lifecycle, a typed result (`pi.result/<op>`, `run_end` with `completed|aborted|failed`). A pi "turn" (`turn_start`/`turn_end`) is one assistant response plus its tool batch, which is Dive's *step*.
3. **Configuration is captured into the state.** Run settings, lane configuration, stream options and retry policy are copied into `OperationState` at acceptance, so a mid-run config change can't affect an in-flight step (`H/session/types.ts:316-342`). This is half of Dive's "configuration is a value": pinned, but not versioned. The tool registry itself is process-local. On restart it must match the captured `activeToolNames`, or the run fails with `configured_tools_unavailable` (`H/runtime/drive/generation.ts:80-89`).
4. **Extensibility is layered by power.** At the `Agent` level: callbacks in the loop config. At the harness level: named hook pipelines with defined aggregation rules. At the product level: `ExtensionAPI.on(event)` with about 40 events, some of which return decisions.

---

## 4. Execution, durability, and guarantees

pi has **three durability tiers**, and they should be judged separately.

### 4.1 Tier 1 (ships today): core loop + coding-agent JSONL session

- **Execution.** `runAgentLoop` runs on the Node event loop. Tools in a batch run concurrently via `Promise.all` (§5).
- **Streaming.** `streamAssistantResponse` (`agent-loop.ts:380-466`) pushes the provider's live `partial` into `context.messages`, replacing it on each event, and emits `message_update` with both the cumulative message and the raw event. `emit` is **awaited**, so a slow subscriber slows the loop. In RPC mode, `waitForRawStdoutBackpressure` is subscribed so a blocked stdout pipe pauses the agent (`CA/src/modes/rpc/rpc-mode.ts:361-363`).
- **Persistence.** `SessionManager` appends one JSONL line per finalized message (`message_end`) and per structural entry. Partial assistant output is never persisted. A crash loses the in-flight response and **any tool results of the current batch that were not yet written**. There is no intent record, so a restart can't tell "never ran" from "ran but result lost". This tier sits at the same level as MAF-Go's agent path, with finer granularity (per message rather than per run).
- **Ordering.** Tool-result messages are emitted in **assistant source order** after the whole batch finishes, while `tool_execution_end` events fire in completion order (`agent-loop.ts:643-651`). This is the same split Dive's comparison README recommends.

### 4.2 Tier 2 (built, used by the experimental worker): the harness

The normative trace is `packages/agent/docs/harness.md:61-85`. Every transition is **one atomic `Storage.commit`**, and all effects happen outside the Session's `MutationLine`:

| Step | Commit contents | Ref |
|---|---|---|
| Accept | prompt entries + tip move + `op.meta` + `op.state=starting`. No hooks, no effects | `H/runtime/lane.ts:497-649` |
| Before provider call | `assistant.effect_pending` with **reserved `responseEntryId` and `usageId`** | `H/runtime/drive/generation.ts:132-172` |
| During stream | one list-append per `AssistantMessageFrame` to `pi.pending.assistant_frame`, **enqueued without awaiting** (provider never waits on storage) | `H/runtime/progress.ts:35-67`; `docs/assistant-durability.md` |
| Settle response | response entry + usage row + tip + delete frame list + next state, in one TX | `H/runtime/drive/response.ts:322-481` |
| Before each tool | `pi.op.tool_args` (validated, post-`before_tool` args) + `call=effect_pending(replay)` | `H/runtime/drive/tools.ts:187-227` |
| Tool progress (opt-in) | `onUpdate(partial, {checkpoint:true})` replaces `pi.pending.tool_output` | `H/runtime/progress.ts:90-117` |
| Tool done | finalized result → `pi.pending.entry`, `call=outcome_ready` (completion order) | `tools.ts:229-293` |
| Placement | contiguous ready prefix → immutable result entries **in source order**, `completed` | `H/runtime/drive/tool-placement.ts:137-246` |
| Terminal | `pi.result/<op>` + delete every op-owned address | `H/runtime/drive/terminal.ts:26-60` |

**Crash and resume.** Reopening a harness starts no work. It restores lane projections and returns the open operations, and the host decides when to drive them (`H/runtime/harness.ts:375-408`). Because the state is total, recovery doesn't replay a journal: it reads one value and dispatches on its leaf. Leftover work is handled like this:

- **Orphaned `assistant.effect_pending`.** Recovery reduces the committed frames into a partial message, stamps it `stopReason: "error"` with an "interrupted… outcome is unknown" diagnostic, settles it, and **treats it as retryable** (`H/runtime/drive/recovery.ts:22-84`, `response.ts:277-289`). The provider call may be billed twice. That is accepted and documented as "no exactly-once" (`harness.md:118`).
- **Orphaned tool `effect_pending`.** It re-executes with the persisted args only if the tool declares `replay: "safe"` **both** in the stored intent **and** in its current definition. Otherwise it writes a synthetic `isError` result made of the last checkpoint content plus an interruption marker (`tools.ts:44-45, 158-168, 515-540`). None of the built-in tools declares `safe`, not even `read`.
- **`outcome_ready`.** Never re-executed. It is only placed. This is the fix for the parallel-tools problem that `docs/tool-durability.md` opens with: "B and C exist only in process memory because A prevented source-ordered placement."
- **Idempotency helpers.** The invocation ID is the reserved `resultEntryId`, so it is stable across replay, unlike the provider's batch-local `toolCallId`. Tools get durable **invocation-scoped memos** (`getMemo`/`setMemo`) and "Flue-style `step.do`" memoization. Every memo write is fenced on the call still being `effect_pending` (`tools.ts:82-131`).
- **Faults.** Any storage or invariant error calls `Harness.fault()`, which seals all lanes and poisons the instance (`H/runtime/harness.ts:309-320`). The dispatcher faults if a procedure makes no durable progress (`H/runtime/drive.ts:101-104`).
- **Concurrency.** A single in-process `MutationLine` covers the whole Session, and there are **no leases or fences across processes**. The SQLite backend's README says: "The host lifecycle, not this backend, guarantees one writable owner per Session… implements no cross-process lease, lock, fence, heartbeat, or takeover."
- **Durability of the write itself.** JSONL appends per commit with **no fsync** (`H/session/jsonl/storage.ts:128-151`). SQLite uses WAL with `synchronous=NORMAL`.
- **Granularity cost.** One commit per streamed delta, where Nvoken does one per checkpoint. JSONL physical growth is never reclaimed (`harness.md:145`).

**Events.** Events are emitted **after commit**. Recipients are snapshotted while the line is held, so event order equals commit order. Every listener receives a `structuredClone`, and delivery is **awaited** (`H/events.ts:35-46`). Events are not durable. The reconnect story is `watch()`: a snapshot plus a buffered stream, and a pure `reduceLaneSnapshot` reducer for remote mirrors.

**Retry is durable.** `assistant.retry_wait{notBefore}` is a real state. `drive({waitForRetry:false})` returns `{waiting, reason:"retry", notBefore}`, so a serverless host can schedule the next pass instead of sleeping (`H/runtime/drive/generation.ts:234-282`). Deferred provider jobs do the same with a stored handle and poll time. This is **the most serverless-friendly drive contract of any system surveyed**, Nvoken included.

### 4.3 Tier 3 (specified): Pico5 / `pi-durable`

Pico5 states the rule in almost the same words as Dive v2:

> "commit intent phase → perform external effect → commit outcome or next phase. Reopening in an intent phase means the effect may have happened." (`packages/durable/docs/pico-v5.md:1352-1363`)

It also adds the following, all specified but not yet built:
- **"All visible progress is durable. There is no volatile publication path."** (`:51-65`). The UI sees only committed partials, throttled into document commits, so "A crash may lose only the uncommitted throttle window".
- **"checkpoint unchanged → `faulted` because no durable progress was made"** (`:1324-1336`). A phase handler that returns without committing is a bug, and the runtime detects it mechanically.
- **Submissions with a host `requestId`**, deduplicated per conversation before any write (`:1486-1487`). This is idempotent admission, which the harness lacks.
- **The replay policy can only be downgraded.** A tool reruns only if the stored intent says `safe` AND the current declaration says `safe` (`:1711-1714`).
- **Unknown task kinds after a deploy** become `orphaned`, or, in the live-registries spec, `pending` with a `blocked{missing_kind|kind_too_old|migration_failed}` reason (`docs/pico-v5-live-registries.md:362-407`).

### 4.4 Guarantee summary

| | Tier 1 (ships) | Tier 2 harness | Nvoken | Dive v2 target |
|---|---|---|---|---|
| Intent before effect | no | **yes** (model + tool) | yes | yes |
| Partial stream survives crash | no | yes (per-delta frames) | preview only | Recorder choice |
| Parallel out-of-order outcomes durable | no | **yes** (`outcome_ready`) | per-call rows | should |
| Uncertain tool policy | n/a | `replay: never\|safe` | read-only/idempotent/destructive annotations | typed policy |
| Idempotent admission | no | no (Pico5: `requestId`) | yes | yes |
| Cross-process fencing | no | **no** | lease + `attempt` fence | host-provided |
| Durable retry wait / host-scheduled resume | no | **yes** | reaper + lease | should |

---

## 5. Tool systems and their interfaces

### 5.1 Definition and schema

Tools are **TypeBox** schemas, which are JSON Schema objects with static TS types (`Static<T>`), validated at runtime. They are not zod.

```ts
export interface Tool<TParameters extends TSchema = TSchema> {           // pi-ai: the model-facing declaration
  name: string; description: string; parameters: TParameters;
  constrainedSampling?: false | { type: "json_schema"; strict: "prefer" | "require" }
                              | { type: "grammar"; variants: Partial<Record<"openai_lark"|"openai_regex", string>> };
}
export interface AgentTool<TParameters extends TSchema = TSchema, TDetails = any> extends Tool<TParameters> {
  label: string;
  prepareArguments?: (args: unknown) => Static<TParameters>;  // compat shim BEFORE validation
  /** Execute the tool call. Throw on failure instead of encoding errors in `content`. */
  execute: (toolCallId: string, params: Static<TParameters>, signal?: AbortSignal,
            onUpdate?: AgentToolUpdateCallback<TDetails>) => Promise<AgentToolResult<TDetails>>;
  /** Recovery policy for an effect whose durable intent exists but whose outcome is unknown. */
  replay?: "never" | "safe";
  executionMode?: "sequential" | "parallel";
}
export interface AgentToolResult<T> { content: (TextContent | ImageContent)[]; details: T; usage?: Usage; terminate?: boolean; }
```
(`packages/ai/src/types.ts:670-685`; `packages/agent/src/types.ts:419-468`)

Five design choices are worth naming:

1. **The declaration and the implementation are one value.** `AgentTool` *is* the `Tool` sent to the model, and `toToolDeclaration` strips the execution fields. That makes one interface, not a capability ladder (compare with MAF-Go's four).
2. **`content` goes to the model; `details` goes to the UI and state reconstruction.** `details` is typed and JSON-constrained. Tool-owned state such as todos lives in `details` and is rebuilt by folding the branch on `session_start` (`CA/examples/extensions/todo.ts:114-133`). Branching and forking therefore "just work" for tool state.
3. **`prepareArguments`** is a per-tool repair hook for model quirks. `edit` uses it to accept `edits` sent as a JSON string, or legacy top-level `oldText`/`newText` (`CA/src/core/tools/edit.ts:103-131`). This is the tool-local cousin of Eino's `ToolAliases`.
4. **`constrainedSampling`** declares strict JSON-schema or grammar sampling *per tool*, and providers map it or refuse it.
5. **Validation** (`ai/src/utils/validation.ts:317-350`) clones the args, normalizes optional nulls, runs `Value.Convert` coercion, then checks. On failure the model gets a formatted error listing JSON paths *and the received arguments*.

**Go translation.** TypeBox's defining property is "the schema value *is* the type". Go gets there from the other direction: derive the schema from a struct with reflection, as in `functool.New[In, Out]`. `prepareArguments` becomes `func(json.RawMessage) (json.RawMessage, error)`. `details` becomes a `json.RawMessage` or a generic `D` on a typed tool wrapper.

### 5.2 Invocation pipeline (core loop, `agent-loop.ts:505-861`)

1. **Truncated output.** If the assistant `stopReason === "length"`, **no call runs**. Every call gets "…arguments may be truncated. Re-issue the tool call" (`:475-500`). This is a cheap, correct guard that none of Eino, ADK-Go or MAF-Go has.
2. **Mode selection.** The batch is sequential if `toolExecution === "sequential"` or **any** called tool is `executionMode: "sequential"`. Otherwise it's parallel.
3. **Prepare, serially in source order.** Emit `tool_execution_start`. Look up the tool; an unknown tool becomes an error result. Run `prepareArguments`, then validation, then `beforeToolCall` (block or terminate). Check the abort.
4. **Execute.** In parallel mode the effects run concurrently via `Promise.all`, with **no concurrency limit**. `onUpdate` partials become `tool_execution_update` events and are ignored after settlement.
5. **Finalize.** `afterToolCall` may replace `content`, `details`, `isError`, `usage` or `terminate` field by field. `tool_execution_end` fires in completion order. Tool-result `message_start/end` fire in **source order** once the batch is done.
6. **Terminate.** The run stops after the batch only if **every** result sets `terminate: true`.

The harness follows the same pipeline, plus the durable states described in §4.2. Two harness differences matter:
- per-tool `executionMode` is ignored there, and only the run-level mode applies;
- `before_tool` may *replace* arguments, which are then re-validated.

### 5.3 Errors

- **Tools throw on failure.** A throw becomes an `isError: true` result whose content is the error message. That includes built-ins: `bash` throws on a non-zero exit with the output plus "Command exited with code N" (`CA/src/core/tools/bash.ts:367-372`).
- **Errors never end the run.** There is no consecutive-error budget and no opaque-error mode. The raw message goes to the model, compared with MAF-Go's `"Error: Function failed."` default.
- **Hooks fail closed.** A throwing `tool_call` extension handler blocks the tool: "A `tool_call` handler failure blocks the tool as a fail-safe" (`CA/docs/extensions.md:207`). The harness `before_tool` behaves the same way (`H/hooks.ts:156-186`).

### 5.4 Approvals and permissions

There are none in core, by philosophy. The only mechanism is the pre-dispatch hook: `beforeToolCall` → `{block, reason, terminate}`. `CA/examples/extensions/permission-gate.ts` is 34 lines: it `ctx.ui.select`s and returns `{block:true}`.

Key properties:
- **The hook sees validated arguments and runs before intent is recorded.** In the harness, the effective post-hook args are what get persisted, and recovery does **not** rerun `before_tool`.
- **Per call, not per batch.** That avoids MAF's top complaint.
- **No suspension.** An approval is a blocking UI prompt *inside* the hook, with a timeout in RPC mode (`rpc-mode.ts:91-131`). There is no "park the turn, resume days later with an answer" primitive. For a CLI that is the right trade. For a service it isn't, and Dive needs both.

### 5.5 MCP

MCP is **deliberately absent**. A grep of `CA/src` finds only one comment. The intended route is CLI tools documented in skills, or an extension that calls `registerTool` at runtime (`CA/examples/extensions/dynamic-tools.ts`). Mid-session tool additions are cache-friendly because of `SystemMessage.toolsAdded` (§2.1).

### 5.6 Result content

- **Content types.** Results carry text and image blocks, with `usage` for tool-internal LLM spend. Images to non-vision models become placeholders (`transform-messages.ts:12-57`).
- **Truncation.** Built-ins truncate to 2,000 lines or 50 KB (`H/utils/truncate.ts:11-13`) with **actionable notices**. `read` appends "Use offset=N to continue". `bash` keeps the tail, spills the full output to a temp file, and names its path.
- **File edits.** `write` and `edit` serialize per realpath through `withFileMutationQueue`, so parallel calls can't race on one file.
- **Operations interfaces.** Every built-in tool takes an `operations` object (`BashOperations.exec`, `ReadOperations.readFile/access`, …), so the same tool runs over SSH, in a VM or in a sandbox (`CA/src/core/tools/bash.ts:59-78`; `CA/examples/extensions/ssh.ts`). The harness generalizes this into an injected `ExecutionEnv = FileSystem & Shell`, whose methods return Results and never throw (`H/types.ts:275-407`).

---

## 6. Execution loop implementation

The core loop `runLoop` is at `packages/agent/src/agent-loop.ts:162-320`:

```text
pending = getSteeringMessages()                       // user may have typed while idle
outer: while true
  inner: while hasMoreToolCalls || pending.length
    if lastCompletedTurn: prepareNextTurn(...)  → replace context/model/thinking, append messages
                          (long; e.g. compaction) → re-poll steering if nothing pending; emit turn_start
    declareToolChanges(context, prepared+pending)     // diff executable tools vs transcript → system msg
    emit message_start/end for each; push to context
    prepareRequest(...)                               // replace context/model/thinking for THIS request
    msg = streamAssistantResponse(...)                // transformContext → convertToLlm → normalizeContext
                                                      // → streamFn; partial replaced in-place per event
    if msg.stopReason ∈ {error, aborted}: finishTurn; turn_end; agent_end; return   // hard exit
    toolCalls = msg.content.filter(toolCall)
    results = stopReason=="length" ? failAll(toolCalls) : executeToolCalls(...)
    hasMoreToolCalls = toolCalls.length > 0 && !batch.terminate
    decision = finishTurn(...)  ; emit turn_end
    if decision == end: agent_end; return
    pending = getSteeringMessages()
  followUps = getFollowUpMessages(); if any: pending = followUps; continue outer
  if explicitContinuation (finishTurn said continue but nothing else scheduled): continue outer
  break
emit agent_end
```

Points to note:

- **No iteration limit anywhere.** There is no `maxTurns`, `maxSteps` or `maxIterations` in `packages/agent/src` or `CA/src`. The loop is bounded only by the model stopping, `finishTurn → end`, all-`terminate` batches, or abort. That is the same gap as ADK-Go, and the one that cost a MAF user 100M tokens. For an interactive CLI with a human at Esc it's defensible. For a library it isn't.
- **Termination is data-driven and typed only at the message level.** The run's outcome is the last assistant message's `stopReason`. The harness adds a real outcome record: `run_end{status: completed|aborted|failed, error: OperationError}`.
- **Steering vs follow-up is a first-class distinction:**
  - `steer()` messages are injected after the current tool batch, before the next model call. Tool calls already in flight are **not** skipped.
  - `followUp()` messages wait until the agent would otherwise stop.
  - The harness adds `nextRun` (held for the next accepted run) and `write` (appended at the next boundary if busy).
  - All four are durable inbox items in the harness (`H/runtime/lane.ts:1434-1516`), and `cancelQueued` reports `cancelled | already_consumed | not_found`.
  - Mid-stream extension messages are deferred to `turn_end` "so it never lands between a tool call and its result" (`CA/src/core/agent-session.ts:1960-1966`).
- **Tool loadout diffing.** `declareToolChanges` compares `context.tools` (what can execute) with the tools declared in the transcript (what the model has seen), and inserts `toolsAdded`/`toolsRemoved` system messages (`agent-loop.ts:322-374`). Dynamic toolsets therefore change *per step* while staying cache-stable.
- **Streaming is threaded through one path.** The provider stream is consumed exactly once. Each event updates the in-context partial and is emitted, and `done`/`error` swaps in `response.result()`. There's no separate blocking path: `complete()` is `stream().result()` (`ai/src/models.ts:880-886`), so streaming and non-streaming can't diverge. The harness adds, per event: encode a frame → enqueue the durable append (not awaited) → emit and await `message_update` → pull the next provider event.
- **Two hooks per request, cleanly separated.**
  - `prepareRequest` runs before *every* model request and may replace context, model or thinking level. The replacement persists for the rest of the run.
  - `transformContext` rewrites what is sent **without persisting**.

  This is Eino's "state rewrite vs call wrapper" split. The harness keeps it as `transform_context` (per request, not persisted) versus entries committed at boundaries.
- **Boundary hooks return drafts, not side effects.** At the product level, `turn_end` and `agent_before_settle` extension handlers return `{entries?: SessionBoundaryDraft[], continue?: boolean}`. The session validates and commits the drafts, and `continue` forces exactly one more model request (`CA/src/core/extensions/types.ts:820`).

---

## 7. Suspend/resume and error handling

### 7.1 Abort and cancellation

- **Core.** `Agent.abort()` aborts the run's `AbortController`, which is passed to the provider stream, hooks and tools. A provider abort produces an `aborted` assistant message. Parallel tools not yet started get "Operation aborted".
- **Coding agent.** Abort additionally cancels retry sleeps, compaction and branch summaries, and **returns queued messages to the editor** (`agent-session.ts:2075-2085`).
- **Harness.** Abort is **durable**: `requestAbort` commits `control = cancel_requested` and returns the drained steer and follow-up messages. The reconciliation procedure then settles everything, in this order:
  1. staged outcomes are placed;
  2. `planned` calls get "cancelled before completion";
  3. running tools get the interruption result;
  4. orphaned provider calls are settled from frames;
  5. deferred jobs get a best-effort remote `cancelDeferred`;
  6. `pi.result` records `aborted`.

  (`H/runtime/drive/reconcile.ts`)
- **Caller cancellation is not abort.** An aborted caller `Context` ends only that caller's wait, and `close()` is "a controlled crash" that leaves operations resumable (`harness.md:29`). This separation between *stop waiting*, *stop the work* and *stop the process* is exactly right, and Dive should copy it.

### 7.2 Suspension

pi has **no human-in-the-loop suspend primitive**. The durable waits are all machine waits: `retry_wait{notBefore}` and `deferred.suspended{handle, pollAfter}`, the latter for provider background jobs. Everything interactive is a blocking UI call inside a hook or tool. Pico5 would model long waits as task phases with memos, and subagents as owned sub-conversations (`pico-v5.md:1675-1705`).

**Contrast:**
- Eino, ADK-Go and MAF-Go all invest mainly in *pausing for a human*.
- pi invests mainly in *surviving a crash*.
- Nvoken does both: parks for host tools and holds a lease.

Dive needs both. pi's machinery covers the half the three Go frameworks lack.

### 7.3 Branching, forking, navigation

- **The session is a tree.** Every entry has `parentId`, and a branch is just a tip pointer.
- **`/tree` (navigate)** moves the tip anywhere. It can first write a `branch_summary` entry, parented at the target, that summarizes the abandoned path back to the common ancestor (`agent-session.ts:3581-3778`; `H/runtime/lane.ts:733-919`). The agent can therefore "remember" what was tried on the abandoned branch.
- **`/fork` and `/clone`** create a new session file with `parentSession` (`agent-session-runtime.ts:262-357`).
- **Harness fork policy is explicit per namespace** (`H/session/fork-policy.ts:40-67`):
  - operation, pending and result state is dropped;
  - lane state resets to idle;
  - unknown `pi.*` namespaces throw;
  - app namespaces are copied only on tree forks.
- **Tool state follows branches for free**, because tools keep it in `details` on result entries.

### 7.4 Compaction

- **Triggers:**
  - manual;
  - threshold: `tokens > contextWindow − reserveTokens` (16,384), checked **before each next request mid-run**, not only between runs;
  - overflow recovery.
- **Cut point:** walk back until `keepRecentTokens` (20,000). Never cut at a tool result. A split turn gets its own prefix summary (`H/compaction/compaction.ts:314-406`).
- **Summary:** a fixed Markdown structure (Goal / Constraints / Progress / Key Decisions / Next Steps / Critical Context), **updated iteratively** from the previous summary, with read and modified file lists carried in `details`.
- **Nothing is deleted.** A `CompactionEntry{summary, retainedTail, tokensBefore, details, fromHook}` is appended, and context reads stop scanning at the newest compaction (`H/runtime/transcript.ts:50-67`). Pico5 generalizes this to `head` markers plus `edits`.
- **Durable inputs.** The `CompactionPreparation` is persisted before summarization, so hooks and the summary request see identical inputs after a crash (`H/runtime/drive/structural.ts:75-100`).
- **Replaceable.** Extensions can decline or supply their own compaction (`session_before_compact`, `before_compaction`).
- **Cache-aware.** Summary calls use `cacheRetention: "none"` and a fresh session ID. Main calls use a stable `${session.id}:${lane}` session ID for provider cache affinity.

### 7.5 Retry and provider errors

- **Layers.** Provider SDK retries (`maxRetries`, `maxRetryDelayMs`: if a server asks for a longer wait, fail and let the outer layer decide) sit under agent-level retries (`RetryPolicy{maxRetries:3, baseDelayMs}`, exponential, capped at 60 s).
- **Events.** `auto_retry_start/end` let a UI show and cancel the backoff.
- **History stays clean.** A failed assistant message **stays in the raw log but is omitted from the projection** by a `context_edit` entry (coding-agent). In the harness, `error` entries are filtered out of provider context.
- **Overflow recovery comes before retry.** A context overflow, or a recoverable `length` stop, from the same model triggers **one** compact-and-retry, with the failed attempt omitted by `context_edit`. A second overflow fails with "Context overflow recovery failed after one compact-and-retry attempt" (`agent-session.ts:2599-2745`; `H/runtime/drive/response.ts:189-248`).
- **Weakness.** All classification is regex over `errorMessage`. There's no `Kind` enum and no `RetryAfter` field. That is pragmatic and battle-tested (every pattern cites an issue number), but it's stringly typed.

---

## 8. Protocol / client-server

pi exposes the agent to UIs in two unrelated ways.

### 8.1 Shipping: JSONL over stdio (`--mode json`, `--mode rpc`)

- **Events.** `AgentSessionEvent` = `AgentEvent` + `agent_settled`, `queue_update`, `compaction_start/end`, `auto_retry_start/end`, `entry_appended`, `session_info_changed`, `thinking_level_changed`, `bash_execution_update` (`CA/src/core/agent-session.ts:164-206`).
- **Wire projection.** On the wire, `toJsonEvent` **strips the cumulative `partial`/`message` from `message_update`**, leaving deltas. `message_start` carries the initial message and `message_end` the authoritative one (`CA/src/modes/json-event.ts:40-61`).
- **RPC commands.** 32 JSON commands on stdin: `prompt, steer, follow_up, abort, compact, fork, clone, get_tree, get_entries{since}, get_messages, set_model, cycle_thinking_level, bash, export_html, …` (`CA/src/modes/rpc/rpc-types.ts:20-74`).
- **`prompt` answers at preflight, not completion.** The response carries `disposition: handled | started | queued`, and completion is the separate `agent_settled` event (`rpc-mode.ts:394-413`). The split between `agent_end` (the loop emitted its last event) and `agent_settled` (retries, compaction and listeners are done) is worth copying.
- **Extension UI sub-protocol.** `extension_ui_request{id, method: select|confirm|input|editor|notify|…}` / `extension_ui_response`, with agent-side timeouts. This is how blocking approvals reach a remote UI.
- **What's missing.** No sequence numbers, message IDs or run IDs on `message_update`. Loss can't be detected, and resync is by pull (`get_messages`, `get_entries{since: entryId}`). Framing splits on LF only, deliberately avoiding Node `readline`'s U+2028 splitting (`rpc/jsonl.ts:4-20`).

### 8.2 Experimental: CBOR + Chord replicated state

- **Protocol v8** (`packages/protocol/src/protocol.ts`): 4-byte big-endian length + one CBOR item, which must be strict JSON (no binary).
  - Client → server: `hello`, `request{id, target, call}`, `cancel{id, target}`.
  - Server → client: `hello`, `hello_error`, `response{id, ok, result|error}`, `service_update{subscriptionId, update}`, `attachment`.
  - Version negotiation is exact-match only, and every object is `additionalProperties: false`.
- **Server** (`packages/server`): a router that never decodes business payloads.
  - One child process per Session (`CA/src/experimental/session-worker-manager.ts`).
  - Unix socket at mode 0600, or a WebSocket relay with a bearer token.
  - Every session request must carry `{serverId, sessionId, attachmentId}`. The `attachmentId` is delivered out-of-band, which **fences stale frames after a re-attach** without sequence bookkeeping (`server/src/session-router.ts:224-232`).
  - A slow consumer is **disconnected** at 64 MiB queued. In stdio mode, by contrast, the agent is stalled. The two behave in opposite ways.
- **UI state is a replicated document, not an event stream.** `LaneSnapshot` (`H/agent-harness.ts:228-249`) holds `transcript: Entry[]`, `tipId`, `configuration`, `stats`, `queues`, and `operation{id, kind, status, retry, deferred, streamingMessage, runningTools}`.
  - The worker applies harness events with the pure `reduceLaneSnapshot`.
  - Chord diffs each revision into ops: `["r"|"s"|"d"|"a"|"t"|"p"|"m", path, …]`. Text growth becomes `a` (string-append) ops with interned paths, so **token streaming costs about the same as raw deltas while the model stays state-based** (`chord/src/delta/index.ts:32-66`).
  - Each member has a `sequence`, and a gap throws and clears the replica (`chord/src/services/state.ts:305-311`).
  - Snapshot/update races are closed on *both* sides: the server buffers until the subscribe response is sent, and the client buffers until `start()`.
- **Reconnect** is manual: "It never reconnects or replays requests automatically… explicitly repeat only operations known to be safe" (`packages/client/README.md:34`). It re-subscribes and gets a **full snapshot, including the whole transcript**.

**Contrast with Nvoken's SSE design:**
- **Nvoken:** cursor-addressed durable `transcript.update` frames, ephemeral previews with reserved message IDs and byte offsets, and `stream.resync`.
- **pi:** folds preview and committed state into one document. The UI is trivially consistent and gets "current operation, running tools, queued inputs" for free. The costs are an O(transcript) reconnect, no history paging, and no preview identity (the streaming message has no ID of its own; it is `operation.streamingMessage`).

A hybrid is the natural design for Dive hosts: cursor frames for the committed transcript, plus a small replicated "live operation" document (preview, running tools, queues) that is snapshotted on connect.

---

## 9. Strengths, weaknesses, and lessons for Dive

### 9.1 What pi does exceptionally well

1. **Provider abstraction and dialect normalization. This is the best in the survey.**
   - The split is clean: the API is a wire dialect, the Provider owns auth and models, and the Model is data carrying compat flags.
   - About 40 providers run on 10 wire implementations. Differences are expressed as **data on the model**, generated from a catalog, not as adapter forks.
   - One event protocol with a documented contract, and errors-as-messages.
   - About 160 dialect regression tests.

   This addresses the #1 pain point in all three Go frameworks' issue trackers (see [README.md §12](README.md)). Eino users run forks to handle OpenAI-compatible stream quirks. pi has `requiresReasoningContentOnAssistantMessages`, `zaiToolStream` and `thinkingFormat`.
2. **Cross-provider message handoff.**
   - Every assistant message carries `api/provider/model` provenance.
   - Replay decisions are made per block against the target model: keep signatures for the same model; drop redacted thinking and convert the rest to text otherwise; normalize tool-call IDs with a mapping carried over to results.
   - Error and aborted turns are skipped, and orphaned calls get synthetic results.

   This is the concrete answer to MAF-Go's reasoning-replay 400s and ADK-Go's lock-in.
3. **The transcript carries prompt and tool changes.** `SystemMessage{sections, toolsAdded, toolsRemoved}` makes the record *exact*: it records what the model was offered at each point, replays deterministically, keeps prompt caches warm through dynamic tool changes, and restores the tool loadout on resume. No other surveyed framework can say what tools the model had at step N.
4. **Intent-before-effect implemented at the agent layer (harness).** Reserved response, usage and result IDs in the intent commit. Per-delta frames. `outcome_ready` for out-of-order parallel completion. Per-tool `replay: never|safe`. Invocation memos. Durable retry waits and a host-schedulable `drive()`. **This is Dive v2's principle 3 in shipping library code**, which neither Eino, ADK-Go nor MAF-Go has. It is also the only system besides Nvoken that knows which effects may have happened.
5. **An append-only record with derived projections.** `context_edit`, compaction as an appended entry with a retained tail, branch summaries, custom entries (not LLM-visible) versus custom messages (LLM-visible), and Pico5's `model` versus `data` split. It matches Dive v2 "one record, many projections" point for point, and pi arrived there independently under production pressure.
6. **Event model.** `message_start`/`message_end` for *every* message makes folding events equal rebuilding the transcript. There are also awaited sinks for real backpressure, source-order results with completion-order tool events, and the `agent_end`/`agent_settled` split.
7. **Extensibility by power level.** Boundary hooks return **drafts plus `continue`** instead of performing side effects. `tool_call` fails closed. Tools contribute their own prompt snippets and guidelines. Per-tool `operations` interfaces let one tool implementation run locally, over SSH or in a VM. The product itself proves the extension surface is sufficient, since plan mode, subagents, permissions and todos are all examples.
8. **Small correctness details:** refusing to execute tool calls from a `length`-truncated message; deferring injected messages so they never split a call from its result; `prompt()` acknowledging at preflight; the per-file mutation queue.
9. **Specification culture.** `harness.md`, `tool-durability.md`, `assistant-durability.md` and `pico-v5.md` are normative, with state tables, traces and non-goals. The storage conformance suites are exported as data (`ConformanceCase[]`), so out-of-package backends reuse them.

### 9.2 What is awkward

1. **Three runtimes at once.** The core `Agent` (ships), the harness (experimental), plus pico3 and Pico5 (future). They have two storage interfaces with different sequence semantics: one seq per write versus one per commit. The shipping CLI still lacks the durability the harness provides. The storage format is "WIP… shapes may change in place" (`harness.md:160`).
2. **No loop bound and no budget.** Nothing caps steps, tokens or cost in core, harness or CLI.
3. **Stringly-typed errors.** Retryability and overflow are both regex over `errorMessage`, with no error kinds.
4. **No cross-process ownership.** No leases, fences or `attempt` counters. That is fine for a CLI and disqualifying for Cloud Run without host work (compare Nvoken).
5. **Write amplification.** One storage commit per streamed delta, and JSONL is never compacted. The team's own later docs propose ephemeral pending output instead.
6. **Unbounded push streams.** `EventStream` never applies backpressure to producers. Parallel tools have no concurrency limit.
7. **Mutation-as-API in the product layer.** `AgentSession` monkey-patches and chains the `Agent`'s hook fields. `tool_call` rewrites args by mutating `event.input`. `before_agent_start` mutates `systemPromptOptions` in place. Steering queues are reconciled by *text match* (`agent-session.ts:903-914`). And there's a 4,000-line god class holding about 20 boolean or abort-controller fields. The harness fixes most of this with named, aggregated hook pipelines.
8. **Spec/code drift in places.**
   - A throwing `before_drive` hook *faults* the harness, while the spec says it only rejects the pass. A test asserts the fault (`packages/agent/test/harness/runtime/drive-public.test.ts:604-610`).
   - The harness ignores per-tool `executionMode`.
   - Only one telemetry span is actually started.
   - The README's `<thinking>` tagging for handoff isn't implemented.
9. **No HITL suspend primitive.** Approvals are blocking UI prompts. This is a product decision, but a library can't inherit it.
10. **Remote protocol.** Exact-match versioning, `additionalProperties: false` everywhere (no forward compatibility), full-transcript resync, and no preview identity.

### 9.3 Concrete recommendations for Dive

**Provider layer (`llm`):**
1. **Adopt pi's three-noun split.** `Model` is a value, `{ID, Provider, API, Limits, Cost, Compat}`; `Provider` owns auth and catalog; the API adapter is a dialect. Put **dialect flags on the model as data** (`Compat.RequiresReasoningContentOnAssistant`, `MaxTokensField`, `SupportsMidConvoSystem`, …), generated from a catalog and overridable by users, with URL-sniffing only as a fallback. This lets one `openaicompat` adapter serve dozens of backends without forks.
2. **Stamp provenance on every assistant message** (`API`, `Provider`, `Model`, `ResponseModel`, `ResponseID`) and implement a single `llm.TransformForTarget(msgs, target)` that every adapter calls. Keep signatures only for the same model. Drop redacted or encrypted reasoning cross-model, and convert visible reasoning to text. Remap tool-call IDs through an adapter-supplied normalizer and carry the map over to results. Skip error and aborted turns. Synthesize error results for orphaned calls.
3. **Errors are values in the stream.** Once `Stream` returns, failures arrive as a final event whose message has `StopReason: Error|Aborted`. Do better than pi on classification: attach a typed `ProviderError{Kind: RateLimited|Overloaded|ContextOverflow|Auth|Quota|Transient, RetryAfter}`, keep pi's regex tables as the *fallback* classifier inside each adapter, and include silent-overflow detection.
4. **Port the stream contract verbatim**, including the `*_end` authority rule and interleaving by `ContentIndex`. Add a compact, replayable `Frame` encoding with a reducer. Recorders can then persist partials cheaply, and the same shape serves as the wire delta.
5. **Refuse "length"-truncated tool calls** in the loop. It's a two-line guard.

**Transcript and record:**

6. **Add a system-message variant with `Sections map[string]*string`, `ToolsAdded` and `ToolsRemoved`**, and have the Engine diff the executable toolset against the transcript before each step (pi's `declareToolChanges`). It gives exact replay of what the model was offered, cache-friendly dynamic tools, and loadout restoration on resume. It also dissolves several of Dive's 13 "ways to put non-user text in front of the model" (v2 conceptual model, Part I) into one typed, recorded mechanism.
7. **Borrow `ContextEdit{Target, Replacement *Content}` and the `model` versus `data` split** for the projection layer: failed attempts, redactions and compaction become appended facts, never rewrites. Keep pi's "custom entry (not model-visible) vs custom message (model-visible)" distinction for extensions.

**Engine and Recorder (durability):**

8. **Use the harness state machine as the reference for Dive's step kinds.** Reserve the response, usage and result IDs *in the intent step*, and use the reserved result ID as the invocation identity, not the provider's call ID. Add `outcome_ready` so parallel completions become durable immediately while placement stays in source order. Make the settle step one atomic write: response, usage, tip, delete partials, next state.
9. **Tool `Replay` policy captured at intent time, downgrade-only on recovery** (stored `safe` AND current `safe`). Combine it with Nvoken's annotation-derived policy (`ReadOnly || Idempotent) && !Destructive`) so built-ins like `read` are safe by default, which pi misses. Offer invocation memos (`tx.Memo(name, fn)`) and opt-in bounded progress checkpoints.
10. **Make durable waits return to the host.** `Engine.Drive(ctx, turn) (Outcome, error)` should return `Waiting{Reason: Retry|Deferred|Input, NotBefore}` instead of sleeping, and support provider deferred/batch handles as a first-class wait. On Cloud Run this is the difference between holding an instance and scheduling a task.
11. **Adopt Pico5's "no durable progress ⇒ fault" rule** as an Engine assertion, plus poison-on-uncertain-storage-failure with a typed `ErrRejected` for guaranteed no-op failures. Keep Nvoken's fencing (`attempt`), which pi lacks. It belongs in the Recorder contract, not in every host.
12. **Separate the three cancellations:** caller stops waiting (`ctx`), user aborts work (a durable `Control` flip plus reconciliation), and process closes (a controlled crash that stays resumable).

**Loop, hooks, events:**

13. **Keep the loop plain**, as pi's `runLoop` is (compare Eino's graph), but **add what pi lacks**: a step and token budget with MAF-Go's graceful final tool-less call, and typed `StopReason`s.
14. **Keep both steering and follow-up**, plus `nextRun`, as durable inbox items with IDs rather than text matching, and a `CancelQueued` that returns `Cancelled|AlreadyConsumed|NotFound`. Defer injected messages so they never land between a call and its result.
15. **Use pi's hook split as the Policy/Observer shape.**
    - `PrepareRequest` (persisting) versus `TransformContext` (per request, not persisted).
    - `BeforeTool` returns `{Args, Block{Reason, Terminate}}`, fails closed and runs before intent.
    - `AfterTool` returns field patches.
    - Boundary policies return `{Drafts []Entry, Continue bool}`.

    Use named pipelines with documented aggregation rules (chained, first-block-wins, last-wins), not monkey-patched fields.
16. **Emit `MessageStart`/`MessageEnd` for every message**, including user, system and tool results, so folding events equals the transcript. Keep result *messages* in source order and tool *events* in completion order. Add the IDs pi's wire lacks: run ID, reserved message ID, sequence number.
17. **Tool ergonomics to copy:** `content` versus `details` on results (branch-safe tool state); `PromptSnippet` and `PromptGuidelines` on tool definitions; `PrepareArguments` repair hooks; actionable truncation notices; per-path mutation locks; and an `Env`/`Operations` interface on built-ins so the same tool runs locally, in a sandbox, or remotely.

**Testing and specs:**

18. **Ship the storage conformance suite as data** (`[]ConformanceCase`) the way pi does, so Postgres, SQLite and in-memory Recorders prove identical semantics. Also ship a **provider dialect regression corpus**; pi's ~160 test files are the model. And ship a `faux` provider in the library itself.

### 9.4 Contrasts with Eino, ADK-Go, MAF-Go and Nvoken

| Concern | pi | Eino | ADK-Go | MAF-Go | Nvoken |
|---|---|---|---|---|---|
| Provider model | API dialect / Provider / Model-as-data with compat flags; ~40 providers | `BaseModel[M]`; providers in `eino-ext`; forks for dialects | `genai` types as public API | none (provider constructs the agent) | Dive v1 providers |
| Cross-provider replay | per-block, provenance-driven, tested | partial | n/a (Gemini-first) | reasoning-replay 400s | inherits Dive |
| Prompt/tool changes in record | **yes** (`SystemMessage` patches) | no | no | no | no |
| Loop | plain `while` with callbacks | compiled graph | `runOneStep` flow | middleware | Dive v1 loop |
| Loop bound | **none** | hard error at 20 | none | graceful at 40 | budget holds |
| Intent before effect | harness: model + tool (experimental); CLI: no | no | no | no | yes (5 fenced TXs/iter) |
| Partial stream durable | per-delta frames | no | no | no | previews ephemeral |
| Parallel outcome durability | `outcome_ready` then source-order placement | no | merged event | batch | per-call rows |
| Uncertain tool policy | `replay: never\|safe` | re-run node | none | none | annotations → safe-retry |
| Fencing / leases | none | none | timestamp OCC | workflow CAS | `attempt` fence + lease |
| HITL suspend | none (blocking UI hook) | interrupt tree | long-running call IDs | approval content + binding | host tools, `waiting` |
| Approval | fail-closed `before_tool`, per call | via interrupts | `RequestConfirmation` | per batch | host tool |
| Tool errors | throw → `isError`, raw message, never ends run | abort run | `{"error"}` | opaque + budget of 3 | error result |
| MCP | deliberately none | ext | client | client + server | yes |
| Compaction | threshold (mid-run), overflow (one retry), iterative structured summary, append-only | middleware | non-destructive event | once per run | — |
| UI protocol | JSONL events (no IDs) / replicated doc (CBOR) | callbacks | events | updates | SSE, cursor + previews |

**The one-line verdict.** pi is the strongest reference in the survey for the **`llm` layer** (dialects, handoff, stream contract) and for **how an agent loop records intent before effect**, and its harness is the closest existing code to Dive v2's Engine/Recorder. It is the weakest on **bounds, typed errors, multi-process ownership and HITL**. Nvoken and MAF-Go already show how to cover exactly those gaps. Dive should take pi's provider layer and harness state machine, and add Nvoken's fencing and MAF-Go's graceful bound and approval binding.
