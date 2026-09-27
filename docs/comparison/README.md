# Agent Frameworks Compared: Eino, ADK-Go, MAF-Go, pi, OpenHands and Nvoken

**Date:** 2026-09-26
**Purpose:** Inform the design of Dive v2 by studying how the three most serious Go agent frameworks solve the same problems. It also covers two very successful non-Go agent products whose libraries overlap with Dive (pi and OpenHands), and Nvoken, our own gateway built on Dive, as a production reference for durability and streaming.

**Source analyses** (from the code, with file and line citations):
- [eino.md](eino.md)
- [adk-go.md](adk-go.md)
- [agent-framework-go.md](agent-framework-go.md)
- [pi.md](pi.md)
- [openhands.md](openhands.md)
- [nvoken-cloud.md](nvoken-cloud.md), focused on durability, the stream API and the streaming protocol

**Issue-tracker analyses** (what users love, hate and ask for, with linked issues):
- [eino-issues.md](eino-issues.md)
- [adk-go-issues.md](adk-go-issues.md)
- [agent-framework-go-issues.md](agent-framework-go-issues.md)
- [pi-issues.md](pi-issues.md)
- [openhands-issues.md](openhands-issues.md)

This document summarizes and compares all of them, and doesn't repeat their citations.

| Framework | Repo @ commit | Go | Size | Lineage |
|---|---|---|---|---|
| **Eino** (ByteDance/CloudWeGo) | `cloudwego/eino` @ `ba04fde8` (2026-09-23) | 1.18 | ~341 files | Native Go. A graph engine came first, and the agent layer (`adk`) was added on top later. Providers and MCP live in the separate `eino-ext` repo. |
| **ADK-Go** (Google) | `google/adk-go` @ `12f7cabc` (2026-09-25) | 1.26 | ~660 files, ~77k LOC | Port of `adk-python`, with "Mirrors adk-python" comments throughout. The public API is built on Gemini `genai` types. |
| **MAF-Go** (Microsoft Agent Framework) | `microsoft/agent-framework-go` @ `425452c` (2026-09-25) | 1.26 | ~470 files | Port of .NET MAF, which grew out of Semantic Kernel and AutoGen. It ships a .NET-to-Go symbol map with a weekly parity audit. |
| **pi** (Earendil) | `earendil-works/pi` @ `2b0a123de` (2026-09-26) | TypeScript | ~170k LOC, 12 packages | Minimal terminal coding agent (~110k stars) built on its own `pi-ai` (LLM) and `pi-agent-core` (agent) libraries. Formerly `badlogic/pi-mono`. A durable "harness" and the `pi-durable` kernel are being built underneath it. |
| **OpenHands** | `OpenHands/software-agent-sdk` @ `d77ada7a` + `OpenHands/OpenHands` (Agent Canvas) @ `47a10808d` | Python / TS | SDK ~125k LOC | Open-source coding-agent platform (~89k stars). After a V0 to V1 rewrite, the core is a Python SDK plus Agent Server. The flagship repo is now Agent Canvas, a control plane that also hosts Claude Code, Codex and Gemini CLI over ACP. |
| **Nvoken** (Deep Noodle, private) | `deepnoodle-ai/nvoken-cloud` @ `deea133a` (2026-09-25) | — | ~897 files, ~75k LOC | A **service** built on Dive v1.34, not a library. It is a durable agent gateway on Postgres, with SSE streams for host applications. |

---

## 1. The one-paragraph verdict

None of the three makes the agent loop itself durable.
- **Eino** checkpoints only on interrupt or cancel.
- **ADK-Go** persists events but can't tell a tool that never ran from one whose result was lost.
- **MAF-Go** persists nothing mid-run, and its workflow checkpoints are at-least-once per superstep.

All three put real engineering into *pausing for a human*. None handles *surviving a crash in the middle of a tool loop*. Each also carries its lineage in its API:
- Eino has two message models and a graph engine underneath a `for` loop.
- ADK-Go is tied to Gemini types, has a 30-method god context, and runs two orchestration runtimes.
- MAF-Go fuses model and agent, and uses .NET idioms such as `*bool` defaults, reflection-typed options and constructor panics.

That leaves a clear opening for Dive: a Go-native, provider-neutral library whose **agent loop is the durable, hookable core**.

Two more pieces of evidence sharpen this.

**Nvoken shows the design works in production.** It survives a crash mid-tool-loop, knows which effects may have happened, and streams a replayable, cursor-addressed log to any number of readers. But it builds all of that *outside* Dive, around Dive's event callback, hooks and tool wrappers. That is the gap Dive v2 exists to close.

**The three issue trackers show where users actually bleed.** The top complaints are:
- **Provider breadth and dialect normalization.** ADK-Go's Gemini lock-in is its main adoption blocker. Eino fights OpenAI-compatible stream dialects. MAF-Go hits reasoning-replay 400s.
- **Streaming that breaks when tools appear.**
- **Partial turns lost on cancel or crash.** MAF declares this "by design".
- **Fragile approvals.** MAF has 114 approval issues, because it approves per batch, not per call.
- **Unbounded loops.** One MAF user burned 100M+ tokens.

Durability plus provider breadth plus per-call approvals would answer most of the highest-engagement threads across all three trackers.

**pi and OpenHands confirm the direction from outside Go.**
- **pi has the strongest `llm` layer in the survey.** It splits wire dialect, provider and model, with dialect quirks expressed as data on the model. About 40 providers share 10 wire implementations. It tracks which provider wrote each message, so it can hand a conversation from one provider to another. Its experimental harness implements intent-before-effect at the agent layer.
- **OpenHands is the most battle-tested record-and-projection design.** An append-only event log with an incremental prompt projection. "View properties" that keep compaction and repair provider-safe. Tool intent persisted before execution. A v2 WebSocket protocol that independently reached **almost the same design as Nvoken's**.

Their trackers repeat the same top pains as the Go trackers. Cross-provider transcript fidelity (pi has 82 thinking-replay issues). Sessions permanently broken by orphaned tool calls. Hard or missing loop limits. Approval code paths that conflate states. Five independent codebases now point at the same core: a validated step log, one projection, typed limits, and provider dialects as data.

---

## 2. Core packages and Go types

### Shape at a glance

| Concern | Eino | ADK-Go | MAF-Go |
|---|---|---|---|
| Model abstraction | `BaseModel[M]{Generate, Stream}`, generic over `*Message` \| `*AgenticMessage`. Tools are passed per request (`BindTools` was deprecated because it caused races) | `LLM{Name(); GenerateContent(ctx, *LLMRequest, stream bool) iter.Seq2[*LLMResponse, error]}`. Small, but every field is a `genai` type | **None.** A provider `RunFunc` returns `iter.Seq2[*ResponseUpdate, error]`, and each provider's `NewAgent` returns a finished `*agent.Agent` |
| Message model | Two of them: flat OpenAI-style `Message` (with three generations of multimodal fields) and content-block `AgenticMessage` | `genai.Content`/`genai.Part` | One `Message` holding a **sealed `Content` union** (~20 kinds, with a `RawContent` fallback for unknown kinds) and a `Source` provenance tag |
| Agent | `TypedAgent[M]{Name, Description, Run → *AsyncIterator[*TypedAgentEvent[M]]}` | `Agent` interface **sealed** by an unexported method, so custom agents are built with `agent.New(Config{Run: func...})` | Concrete `*agent.Agent` struct, with no interface |
| Entry point | `Runner` (owns checkpointing) | `Runner` (owns session I/O, plugins, persistence, compaction, HITL routing) | `Agent.Run`/`RunText`/`RunMessage` → `ResponseStream` |
| Stream type | `AsyncIterator` over an **unbounded channel** (Go 1.18 rules out `iter`) | `iter.Seq2[*session.Event, error]` everywhere | `iter.Seq2[*ResponseUpdate, error]` plus `Collect()` |
| Context passed to tools and hooks | `context.Context` plus `ProcessState[S]` lookup by type | `agent.Context`: about 30 methods, embeds `context.Context` | `context.Context` plus `AgentFromContext`, `WithFuncCallID` |
| State | Per-run `Session{Values, Events}` and graph-local `State` | Session event log plus `State` with `app:`/`user:`/`temp:` prefixes. Deltas travel on events | `Session`: a JSON state bag with lazy typed decode |
| Orchestration | `compose`: `Graph`/`Chain`/`Workflow` (Pregel or DAG) | `workflow` graph engine **and** the legacy agent tree (Sequential/Parallel/Loop agents, transfer) | `workflow`: a BSP superstep engine with an `Executor` struct of about 10 optional hooks |

### Type definitions worth studying

- **MAF `stateValue`** (`agent/value.go`) holds either a live Go value or raw JSON. It decodes lazily into whatever type the caller asks for, and it **re-emits the original raw JSON for keys nobody read**. Plugins (approvals, todos, injection) can then each own a key without a schema registry. This is the best small type in the three codebases.
- **ADK `EventActions{StateDelta, ArtifactDelta, TransferToAgent, Escalate, RequestedToolConfirmations, Compaction}`** carries side effects as data on the event that caused them, so they commit in the same append. Tools can't forge the framework-owned `Compaction` field.
- **ADK `platform` seams** (`WithTimeProvider`, `WithUUIDProvider`, `WithTaskRunner`) are carried on the context. They make runs deterministic and are the whole integration surface an outside durable engine such as Temporal or Restate needs.
- **Eino `TypedRetryDecision`** lets retry logic reject a *successful* model output, rewrite the input (for example compress, then retry), and persist that rewrite.
- **MAF sealed `Content`** uses an unexported `kind()` method. Its discriminated JSON includes `TextReasoningContent.ProtectedData` for thinking signatures and encrypted reasoning, which cross-provider replay needs.

---

## 3. Key concepts and how they relate

**Eino.** Components (model, tool, retriever) are interfaces, and they compose into typed graphs through `Runnable[I,O]`, which has four stream modes. An `Agent` is anything that emits an `AsyncIterator` of events. `ChatModelAgent` is a ReAct graph of 7 to 9 nodes, and **it is rebuilt and compiled on every `Run`**. Middleware (`Handlers`) has two kinds of hooks: *state rewrites* (`BeforeModelRewriteState`, `AfterModelRewriteState`), which persist, and *call wrappers* (`WrapModel`, `Wrap*ToolCall`), which don't. Callbacks are a separate observability layer typed as `any`. Multi-agent transfer exists, but Eino's own docs mark it "NOT RECOMMENDED" in favour of agents-as-tools. `TurnLoop` is a long-lived loop that batches pushed items into agent turns. It is Eino's version of a session actor.

**ADK-Go.** The session **event log is the only source of truth**: every LLM step rebuilds its prompt by scanning `Session().Events()`. The hierarchy is invocation → agent calls → steps (one LLM call plus its tool calls). The agent tree supports transfer, and "stickiness" comes from the log: the next turn goes to whichever agent authored the last event. Branch and IsolationScope filter which parts of history each agent sees. Graph workflows (`workflow.Node`, `Edge`, `Route`) sit alongside the tree, and since v2 an LLM root agent is quietly wrapped in a one-node workflow. Callbacks are per agent and plugins are per runner, and both short-circuit on the first non-nil result. Compaction is non-destructive: a summary event records the range it covers.

**MAF-Go.** An agent is a **stack of stream-transforming middleware around a provider function**:

`Config.Middlewares → history/context providers → toolautocall (the loop) → MessageInjector → structuredOutput → provider RunFunc`

History providers and context providers (RAG, memory, compaction, skills, todo) run **once per `Run`, outside the tool loop**. Approvals and user-input requests are *message content*. The workflow engine is a separate world, bridged in both directions: agent ↔ executor, agent ↔ tool, and agent ↔ MCP/A2A/AG-UI server.

**Where they converge:**
1. `iter.Seq2` as the execution type (ADK-Go and MAF-Go; Eino can't use it on Go 1.18).
2. Agents-as-tools as the practical multi-agent primitive. Eino recommends it explicitly, and MAF-Go and ADK-Go (v2 task and single-turn modes) both steer toward it.
3. Human-in-the-loop as "end the run, resume later with an answer keyed by an ID".
4. A graph workflow engine offered *alongside* the agent loop.

**Where they diverge:** where the transcript lives.
- ADK-Go keeps it in the persisted log and re-reads it every step.
- Eino keeps it in graph-local state that is persisted only when the run is interrupted.
- MAF-Go keeps it in a local slice inside the loop, and history is stored only after a successful run.

---

## 4. Execution, durability, and guarantees

| | Eino | ADK-Go | MAF-Go |
|---|---|---|---|
| **When state is written** | **Only on interrupt or cancel.** Nothing is written per step | **Every non-partial event**, appended before the producer continues (an explicit `processed`/`ack` handshake) | **After a successful `Run`**, through the Session. Workflows checkpoint after every superstep |
| **Crash in the middle of a tool loop** | Everything since `Run` is lost | Completed events survive. A dangling `FunctionCall` means the tool "may have run", and nothing records which | All progress is lost |
| **Resume model** | Re-runs the interrupted graph node. Completed parallel sibling tools aren't re-run (`ExecutedTools`) | Rebuilds paused state by scanning the log. Children of dynamic nodes are **replayed from a cache keyed by stable run ID** (Temporal-style) | Agents: return and call `Run` again. Workflows: restore a checkpoint and **re-run the whole superstep** (at-least-once, and events have already been emitted) |
| **Serialization** | **gob**, with a `RegisterName` registry and byte-level patches to migrate old checkpoints | JSON on events. `any` values come back as `map[string]any` | JSON, with `PortableValue{TypeID}` resolved through a registry, falling back to `//go:linkname` into `reflect.typelinks` |
| **Idempotency** | None for tools | Duplicate HITL resumes are no-ops (`resolvedCount`). Nothing for tools | None |
| **Concurrency control** | None | Optimistic, by comparing timestamps (which depends on clocks). The in-memory session is changed before the DB commit | Workflow ownership CAS. The Session isn't synchronized |
| **Backpressure** | **None**: unbounded channel, and finalizers are needed to avoid stream leaks | Pull-based. The workflow queue is bounded at 16 | Pull-based. Only parallel tool calls use goroutines |
| **Streaming** | A side channel: the model stream is `Copy(2)`'d and one copy is fully concatenated before tools run | Partial chunks are yielded but never persisted. One aggregated non-partial response is yielded after them | Updates pass through until the first function call, then are **buffered**. If any tool needs approval, the rest of the stream is held back |
| **Ordering** | Tool results go into history in call order. Tool *events* arrive in completion order | Parallel tool results are merged into **one** event, in call order | Tool results go out as one update per iteration, after every call in that iteration finishes |

**Lessons:**
- **ADK-Go's commit-before-proceed handshake** is the right invariant. But it holds only because the Runner happens to be the consumer: `agent.Run` used standalone never sees its own tool results. Dive should put this guarantee at the Engine–Recorder boundary.
- **Nobody records intent before effect.** Dive v2's principle 3 ("a recorded intent means the effect may have happened") is exactly what all three lack. ADK-Go comes closest and still has the gap.
- **Avoid gob and runtime type discovery for durable data.** Eino's byte-patching migrations and MAF-Go's `linkname` hack are both warnings. Use JSON with explicit version fields and stable wire names.
- **Don't re-read the store to build the next request.** ADK-Go re-scans the whole history on every step, so each step costs O(n) in history length. Keep the working transcript in the Engine and use the store as a journal.

---

## 5. Tool systems and their Go interfaces

### Interfaces

```go
// Eino: four interfaces. The "Enhanced" variants reuse the method name
// InvokableRun with a different signature, so one type can't implement both.
InvokableRun(ctx, argumentsInJSON string, opts ...Option) (string, error)
StreamableRun(ctx, argumentsInJSON string, opts ...Option) (*StreamReader[string], error)
InvokableRun(ctx, *schema.ToolArgument, opts ...Option) (*schema.ToolResult, error)        // Enhanced
StreamableRun(ctx, *schema.ToolArgument, opts ...Option) (*StreamReader[*ToolResult], error) // Enhanced

// ADK-Go: the public interface doesn't match what the runtime needs.
type Tool interface { Name() string; Description() string; IsLongRunning() bool }
// The runtime actually requires these, from internal/toolinternal:
Declaration() *genai.FunctionDeclaration
Run(ctx agent.Context, args any) (map[string]any, error)
ProcessRequest(ctx agent.Context, req *model.LLMRequest) error  // missing = runtime failure

// MAF-Go: a ladder of capability interfaces.
type Tool interface { Name() string; Description() string }       // bare = hosted-tool marker
type SchemaTool interface { Tool; Schema() any; ReturnSchema() any } // declaration only → hands the call back to the caller
type FuncTool interface { SchemaTool; Call(ctx, args string) (any, error) }
type ApprovalRequiredTool interface { Tool; ApprovalRequired() bool }
```

### Typed function tools

All three use generics plus JSON-schema reflection:
- Eino: `InferTool[T, D]`
- ADK-Go: `functiontool.New[TArgs, TResults]`
- MAF-Go: `functool.New[In, Out]`

Two details worth copying:
- **MAF-Go validates arguments against the schema** before the handler runs.
- **Eino's `WithSchemaModifier`** lets you define custom struct tags.

Two to avoid:
- MAF-Go wraps a scalar input as `struct{ Arg0 T }`, so the model is shown a parameter literally named `Arg0`.
- ADK-Go rejects inputs that aren't structs or maps, and has a TODO admitting that tools with no arguments are awkward.

### Behavior comparison

| | Eino | ADK-Go | MAF-Go |
|---|---|---|---|
| **Tool error default** | **Aborts the whole run** (you have to wrap each tool to send errors back to the model) | Becomes `{"error": msg}` for the model (`errors.Is` identity is lost) | An opaque `"Error: Function failed."` goes to the model (details only with `IncludeDetailedErrors`). A **budget of 3 consecutive errors** stops the run |
| **Panics** | Recovered | Recovered in `functiontool` and workflow nodes; **not** in the parallel dispatch goroutines, so a custom tool can crash the process | Recovered |
| **Parallel calls** | On by default, no concurrency limit | On by default, unbounded goroutines, but an injectable `TaskRunner` | **Serial by default.** `AllowConcurrentInvocations` gives unbounded parallelism |
| **Result type** | `string`, or `ToolResult{Parts}` (multimodal) | **`map[string]any` only.** MCP images become placeholder text | `any`, or a passthrough `*FunctionResultContent` (multimodal) |
| **Model-mistake repair** | **`ToolAliases`** (tool name and argument key), `UnknownToolsHandler`, `ToolArgumentsHandler`, `patchtoolcalls` | Unknown tool → `OnToolError` → error result | A "not found" result goes to the model, or `TerminateOnUnknownCalls` |
| **Approval** | No API. Built from interrupts, and the tool **re-runs from the top** on resume. Tools that weren't the resume target must re-interrupt themselves | `ctx.RequestConfirmation` → a synthetic `adk_request_confirmation` call. The gating code is implemented three times | Built into the loop. **Approval binding**: responses are matched against requests recorded in the session, so a client can't forge an approval or swap arguments. Also standing "always approve" rules |
| **MCP** | Only in `eino-ext` | Client via the official go-sdk. Structured content is kept; everything else is flattened to text | **Both directions** (client and `AddTool` server). Names are normalized, and a collision is an error |
| **Dynamic tools** | Per run (`BeforeAgent`) or per model call (`State.ToolInfos`). Provider tool search | Toolsets resolved **once per run** | Per run, through context providers |

**Synthesis for Dive:**
- **One interface.** Use one tool interface with a rich result: content blocks plus an `IsError` flag, with the original Go `error` kept for Observers.
- **Errors go to the model.** Tool errors become error results the model sees, with details opaque by default (as MAF-Go does). Only a typed sentinel ends the run.
- **Recover panics once**, at the executor boundary.
- **Parallel calls:** run them with a concurrency limit, and only when a tool is marked concurrency-safe. Merge results in call order.
- **Tool annotations:** `ReadOnly`, `Destructive`, `Idempotent`, `ConcurrencySafe`, `NeedsApproval`.
- **Repair model mistakes** with aliases and an unknown-tool handler, as Eino does.
- **Resolve toolsets per step**, not once per run.
- **Approval belongs to the Engine, not the tool.** It is a Policy evaluated *before* dispatch, with MAF-Go-style binding to recorded requests. Tool authors should never have to write re-entry logic.

---

## 6. Execution loop implementation

| | Eino | ADK-Go | MAF-Go |
|---|---|---|---|
| **Where the loop lives** | A compiled ReAct **graph** (`adk/react.go`) | `llminternal.Flow.Run` → `runOneStep` in an internal 1.7k-line `base_flow.go` | `toolautocall` **middleware** installed by each provider's constructor (a port of .NET's `FunctionInvokingChatClient`) |
| **Loop shape** | Init → ChatModel → branch (tool calls?) → CancelCheck → ToolsNode → AfterToolCalls → CancelCheck → back to ChatModel | `for { range runOneStep; if lastEvent.IsFinalResponse() break }`. Each step: request processors (instructions, tools, **contents rebuilt from the log**, compaction, confirmations, transfer) → callbacks → LLM → callbacks → yield → parallel tools → yield the merged result → optional transfer, which **runs the target agent inline** | `for i := 0; ; i++ { model call; buffer or pass through; if no calls or approval needed break; run tools; yield tool message; append assistant and tool messages }` |
| **Iteration limit** | `MaxIterations` = 20, and hitting it is a **hard error** | **None.** `MaxLLMCalls` is declared but nothing reads it. The only cap is 10 consecutive thought-only turns | `MaximumIterationsPerRequest` = 40, and hitting it is **graceful**: one last call **with tools removed** forces a text answer |
| **Other ways the loop ends** | Return-directly tool, `Exit` action, interrupt, error, `ctx` | `SkipSummarization`, long-running tool IDs, `EndInvocation()`, ending on a partial event | Approval pending, unknown or non-invocable tool, error budget, `ctx`, the consumer breaking out |
| **Per-model-call hooks** | Yes: state-rewrite and wrap-model middleware | Yes: before/after model and on-model-error callback slices | **No.** Users can't add provider middleware, and compaction runs only once per `Run` |
| **Mid-run steering** | Safe-point cancel | None | `MessageInjector` drains queued user messages between model calls |
| **Stop reason** | Inferred from events | Inferred from `IsFinalResponse()` on the last event | `FinishReason` string |

**What to take:**
- **Write the loop as plain Go.** It should read as `for { call model; if no tool calls, break; run tools }`, as MAF-Go's and ADK-Go's inner loops do, not as a graph compiled on every run like Eino's.
- **Hook every model call.** There should be hooks at BeforeModelCall, AfterModelCall, BeforeToolCall and AfterToolCall. Compaction should be a per-step hook, which MAF-Go lacks.
- **Stop gracefully at the step limit.** Adopt MAF-Go's final tool-less call, and report the result as a typed `StopReason` (`completed | max_steps | suspended | cancelled | error | handoff`).
- **Make steering a core feature**, as MAF-Go's injector does.
- **Emit a `ToolCallStarted` event.** Eino lacks one, and MAF-Go delivers tool results only as a batch per iteration.
- **Don't buffer the stream** just because some tools might need approval.

---

## 7. Suspend/resume and error handling

### Suspend and resume

| | Eino | ADK-Go | MAF-Go |
|---|---|---|---|
| **Primitive** | An address-based **interrupt signal tree** (`agent:A;node:ToolNode;tool:search:call_123`), carried as an error | **A function call whose ID is in `LongRunningToolIDs`**, answered later by a `FunctionResponse` with the same ID. Long-running tools, confirmations and input requests all reduce to this | **Content in the response**: `ToolApprovalRequestContent` (agents), or `RequestPort`/`ExternalRequest` (workflows) |
| **Resume API** | `ResumeWithParams(ctx, checkpointID, {Targets: map[interruptID]any})`, where interrupt IDs are UUIDs | Send the answer as the next user message. The Runner routes it to the paused invocation | Call `Run` again with the response content and the Session. For workflows, `SendResponse` or `ResumeStreaming(checkpoint)` |
| **On resume, work that already ran…** | …is re-entered: the interrupted node re-runs, and completed sibling tools are skipped | …is **replayed from a memoized cache** of child outputs keyed by run ID | …is kept by agents, since pending approvals come from the Session. Workflows re-run the superstep |
| **Graceful cancel** | **`CancelAfterChatModel` / `CancelAfterToolCalls`** give a *resumable* checkpoint (about 1,100 lines of CAS state machine) | Cooperative `EndInvocation()`. Plain `ctx` otherwise | Plain `ctx` |
| **Validating the resume** | Tools must implement the re-entry contract correctly | `ResponseSchema` validation, `ErrInvalidResumeResponse`, `ErrNothingToResume`, duplicate resumes ignored | Approval binding drops responses that don't match a recorded request |
| **Sharp edges** | Side effects before the interrupt point run twice. Rebuilding the graph must match exactly | At most one pending HITL per dynamic node. No graph fingerprint, so changing the graph between deploys silently corrupts resume | Workflow events are emitted before the checkpoint commits |

### Errors, retry, cancellation

| | Eino | ADK-Go | MAF-Go |
|---|---|---|---|
| **Error delivery** | **Inside events** (`AgentEvent.Err`), with no final error, so callers easily miss failures | In-band `yield(nil, err)`. Semantics differ by layer | The iterator's `error` for agents. **Events only** for workflow failures |
| **Error types** | Good: `ErrExceedMaxIterations`, `RetryExhaustedError`, `WillRetryError`, `CancelError` (resumable), `PanicErr` | Sentinels plus one structured `NodeRunError` | **Strings.** No sentinels, no classification of provider errors |
| **Model retry** | **Rich**: `ShouldRetry` sees successful outputs and can rewrite input; failover sticks to the last model that worked; retries are surfaced in the stream (`WillRetryError`) | None in the framework (left to the SDK). Workflow nodes have `RetryConfig` | **None anywhere** in the framework |
| **Tool retry** | Middleware | `retryandreflect` plugin (tells the model to reflect and try again) | Consecutive-error budget only |

**Synthesis for Dive:**
- **One interrupt primitive with typed payloads.** Borrow ADK-Go's `RequestInput{ID, Message, ResponseSchema, Payload}` and its sentinels.
- **Resume by ID.** Resume by call or interrupt ID, reuse the original turn, and make duplicate resumes no-ops.
- **Suspended runs return a typed outcome** (`Suspended{Pending []Interrupt}`) instead of requiring callers to inspect events.
- **Graceful, resumable stop**, backed by the step log rather than a separate state machine: "stop after this model turn" and "stop after these tools".
- **Typed provider errors** (`Kind: RateLimited | Overloaded | ContextLength | Auth`, plus `RetryAfter`).
- **Retry and failover in the `llm` layer**, only at stream creation, with Eino's richer `ShouldRetry(response, err)` contract and retries visible as events so a UI can discard partial output.
- **Return a terminal `(Outcome, error)`**, and treat events as progress.

---

## 8. Ideas to steal, ranked

| # | Idea | From | Why it matters |
|---|---|---|---|
| 1 | `iter.Seq2[Event, error]` from the provider up through the agent. Stopping the range cancels the work | ADK-Go, MAF-Go | Idiomatic, backpressured, no goroutine leaks, and one type for streaming and non-streaming |
| 2 | Commit-before-proceed handshake between producer and persistence | ADK-Go | The core invariant of a durable runtime |
| 3 | Side effects as data on the event, committed atomically | ADK-Go | "What happened" and "what changed" in one write, and parallel merges are deterministic |
| 4 | Approval binding to framework-recorded requests | MAF-Go | A real security property. Clients can't forge approvals or swap arguments |
| 5 | Session state bag with lazy typed decode that keeps raw JSON | MAF-Go | Plugins own keys without a registry, and forward-compatible fields aren't lost |
| 6 | Split between durable state rewrites and ephemeral call wrappers | Eino | Prevents middleware from silently breaking prompt caching or history |
| 7 | Graceful iteration cap (a final tool-less call) | MAF-Go | Users get an answer, not an error |
| 8 | Memoized replay of child steps keyed by stable run IDs | ADK-Go | Resume is deterministic while orchestration stays plain Go control flow (`DynamicNode` + `RunNode[OUT]`) |
| 9 | Clock, ID and task-runner seams on the context | ADK-Go | Deterministic tests, plus a clean hook for an outside durable engine |
| 10 | Retry predicate over `(response, err)` with input rewrite, plus sticky failover | Eino | Handles "succeeded but malformed" and "context too long, compress and retry" |
| 11 | Tool alias repair and unknown-tool handler | Eino | Cheap, and it measurably reduces failed turns |
| 12 | Opaque tool errors by default, consecutive-error budget, schema validation before the call | MAF-Go | Safer (no secrets leaked) and self-correcting |
| 13 | Message provenance (`Source`) and mid-run steering (`MessageInjector`) | MAF-Go | Stops injected context from being written back into history. Coding-agent interfaces need steering |
| 14 | Resumable safe-point cancel ("stop after this turn") | Eino | A real need in chat and coding interfaces |
| 15 | Non-destructive compaction events with coverage ranges, and a conformance test suite for storage backends | ADK-Go | Auditable compaction, and store implementations that verifiably behave the same |
| 16 | Message IDs shared between the stream and stored history | Eino | Lets a UI match streamed events to stored history |
| 17 | BSP supersteps with barrier-published state and write-conflict errors, *if* Dive adds graphs | MAF-Go | Deterministic concurrent state |
| 18 | Pending tool calls committed atomically with the model message; a fenced `running` record before dispatch | Nvoken | Tells "never started" apart from "may have run". None of the three libraries can |
| 19 | One fencing token (`attempt`) for both persistence and preview invalidation | Nvoken | Crash recovery and stale-stream cleanup become one mechanism |
| 20 | Preview identity equal to the future record's identity (reserved message ID, per-block offset, call ID on every argument fragment) | Nvoken | The switch from preview to saved message is an update, not a replace. Clients can detect loss themselves |
| 21 | Safe-retry policy from tool annotations (read-only or idempotent, and not destructive) | Nvoken | Recovers uncertain effects without a human |
| 22 | Recorded-fixture conformance suite for provider stream dialects | Issue trackers (all five) | The number-one source of user pain across trackers |
| 23 | Three-noun `llm` split: API dialect / Provider (auth, catalog) / Model as data with **compat flags** | pi | One `openaicompat` adapter serves dozens of backends without forks |
| 24 | Provenance on every assistant message, plus one `TransformForTarget` hand-off function (keep signatures only for the same model; drop or convert reasoning otherwise; remap tool-call IDs; skip error turns) | pi | Solves cross-provider reasoning-replay 400s and model switching mid-session |
| 25 | System messages that record **prompt sections and tools added or removed** | pi | Exact record of what the model was offered at step N; cache-friendly dynamic tools; loadout restored on resume |
| 26 | `outcome_ready` for parallel tools: durable in completion order, placed in source order | pi (harness) | No finished sibling result is lost when a batch is interrupted |
| 27 | Durable waits returned to the host (`Waiting{Retry\|Deferred\|Input, NotBefore}`) instead of sleeping | pi (harness) | A serverless or Cloud Run host schedules the next pass instead of holding an instance |
| 28 | **View properties**: provider invariants declared once, used both for compaction cut points and for cold-load repair | OpenHands | Makes compaction and replay provider-safe; the fix for the "permanently broken session" bug class |
| 29 | Cached incremental projection (extend on append, rebuild only on branch or recovery) | OpenHands | Record-as-truth without ADK-Go's O(history) rescan per step |
| 30 | Resource-keyed tool concurrency (per-call lock keys; undeclared means a per-tool mutex) | OpenHands | Strictly better than a boolean `ConcurrencySafe` flag |
| 31 | Stock stuck-detection policy with a one-time nudge before stopping | OpenHands | Loops are a top complaint everywhere, but naive detectors misfire, so it needs to be a configurable policy |
| 32 | Separate "run ended" from "session settled" | pi (tracker) | A year of host races over retries, compaction and queued follow-ups after `agent_end` |
| 33 | One pipeline for **every** model call (turns, compaction, summaries, sub-agents) | pi (tracker) | Compaction as a second request path was pi's recurring drift bug (wrong headers, hooks, caps) |

## 9. Anti-patterns to avoid

- **One vendor's wire types as the public API** (ADK-Go and `genai`). The OpenAI adapter has to reject about 20 settings and can't round-trip reasoning.
- **Two message models, with the whole API generic over both** (Eino).
- **Sealed or dishonest interfaces**: a sealed `Agent`, and a public `Tool` the runtime can't actually execute (ADK-Go).
- **God contexts** (ADK-Go's 30-method `agent.Context` that embeds `context.Context`).
- **Callback sprawl**: 12+ slices with first-non-nil-wins semantics, duplicated between plugins and agents (ADK-Go). Use one `func(next) next` middleware shape instead.
- **A graph engine underneath a simple loop**, compiled on every run (Eino).
- **Model and agent fused together**, with the loop as provider middleware (MAF-Go).
- **gob, `linkname`, or global type registries** that users must fill in, for durable data.
- **Unbounded channels and finalizer-based cleanup** (Eino).
- **Tool errors that abort runs** (Eino), and **string-only errors** (MAF-Go).
- **No loop bound** (ADK-Go), or a bound that fails hard (Eino).
- **Reflection-typed option bags, `*bool` defaults, and constructor panics** (MAF-Go).
- **Messages silently dropped** on a type mismatch at a workflow edge (MAF-Go).
- **Parity with another language as the design goal.** Both ports carry the cost. Dive's advantage is being Go-first.

---

## 10. How this maps onto Dive v2

The [v2 conceptual model](../design/2026-09-26-dive-v2-conceptual-model.md) already chooses the direction this survey supports. Here is how each v2 principle compares with the three frameworks:

| v2 principle | Evidence from the survey |
|---|---|
| **One record, many projections** (the step log is the only durable account) | ADK-Go is closest (the event log is the truth), but it rebuilds each request by re-scanning storage. Dive should keep the projection in memory and journal the steps. Eino and MAF-Go have no record at all during a run. |
| **The turn is the aggregate** | None of the three has one. ADK-Go's "invocation" is the nearest equivalent, but its state is inferred from the log. |
| **Intent before effect, result before advance** | This is **the gap in all three**. ADK-Go persists results but not intents. Eino and MAF-Go persist neither mid-run. Doing this well is Dive's clearest differentiator. **Nvoken already does it**, in about five fenced transactions per iteration, but outside Dive. Its commit groups are the natural template for Dive's step kinds (see §11). **pi's harness** does it at the library layer: it reserves IDs in the intent commit and uses `replay: never\|safe`, but has no cross-process fencing, and the shipping CLI doesn't use it yet. **OpenHands** persists tool intent before execution. It lacks a `started` marker and persists results per batch, and it re-runs unmatched calls on resume. |
| **Decisions are typed, inputs are immutable** | ADK-Go's callbacks and MAF-Go's middleware both mutate. Eino's rewrite hooks come closest to typed state transitions. Approval as a Policy decision before dispatch beats all three tool-level approval designs. |
| **Observation cannot control** | All three mix these. ADK-Go's `OnEvent` plugin can rewrite events before they are persisted, and Eino's callbacks share `any` pointers. |
| **Unsupported is refused, not ignored** | ADK-Go's `openaimodel` does this (it rejects unsupported config explicitly). MAF-Go and Eino silently pass options through. |
| **Configuration is a value** | Nobody does this. MAF-Go's `*Agent` is immutable-ish but not versioned. Eino and ADK-Go resume only if you rebuild an identical agent or graph. ADK-Go has a TODO asking for a graph fingerprint. |
| **One vocabulary per level** | ADK-Go's invocation → agent call → step hierarchy is well documented. Nobody has typed stop reasons per level. |

### Concrete API sketch implied by the survey

```go
// llm: one model call, provider-neutral, honest.
type Provider interface {
    Stream(ctx context.Context, req *Request) iter.Seq2[Event, error] // tools live in Request; clients are immutable
}

// tool: one interface, a rich outcome, annotations.
type Tool interface {
    Def() Def // name, description, JSON schema, annotations (ReadOnly, Idempotent, ConcurrencySafe, NeedsApproval)
    Call(ctx context.Context, call *Call) (*Result, error) // Result{Content []Content; IsError bool}; call.ID doubles as the idempotency key
}
func Func[In, Out any](name, desc string, fn func(context.Context, In) (Out, error), opts ...Option) Tool

// dive: a plain Go loop, a journaled Turn, typed outcome.
func (e *Engine) Run(ctx context.Context, t *Turn, cmd Command) iter.Seq2[Event, error]
// yield returns ⇒ the Recorder has acknowledged the step.
// The run ends with Outcome{Stop: Completed|MaxSteps|Suspended|Cancelled|Error, Pending []Interrupt}.
```

---

## 11. Nvoken: durability and streaming in production

Full analysis: [nvoken-cloud.md](nvoken-cloud.md). Nvoken is the only system surveyed that survives a crash mid-tool-loop and knows which effects may have happened. It is also the only one whose stream any number of readers can resume exactly from a cursor. That validates Dive v2's principles 1 and 3 in production code. Its limitation is that it does all of this *around* Dive, not *with* it.

### Durability

- **Commit groups.** These are the one-transaction units of work, each fenced on lease owner and `attempt`:
  - admission;
  - claim;
  - model-call intent (`model_call_facts`: prepared, then started);
  - the model iteration result: the assistant message **plus the whole tool-call batch as `pending` rows**, and a checkpoint;
  - tool start (`running`, stamped with the attempt);
  - tool result;
  - park for host tools;
  - host result batch;
  - settle.
- **The fence.** Every claim increments `attempt`, and every write checks it. A resurrected executor "may finish local computation but cannot commit".
- **Ordering.** One `UPDATE … RETURNING` on the per-Conversation `execution_streams` row reserves both gap-free counters: message `sequence` and lifecycle `revision`. Sequence order therefore equals commit order, without `SERIALIZABLE`.
- **Crash recovery** has four steps:
  1. The reaper returns expired leases to `queued`.
  2. A new claim runs as `attempt + 1`.
  3. The worker rebuilds from the durable prefix, checking it for consistency and failing the Turn as `internal` rather than running on bad evidence.
  4. It decides at the seam before any provider call. Builtin tools are restarted. MCP tools are restarted only if read-only or idempotent and not destructive. Otherwise the model is told *"its outcome is unknown, so nvoken did not retry it."*
- **Idempotency** covers admission, model checkpoints (checked with an evidence digest), tool results and host results (equal replays are deduplicated, changed replays get a 409), and nudges.

### Stream API

- **Routes.** Two SSE routes: per Turn (closes on settle) and per Conversation (never closes). There is no WebSocket, and every error happens before the 200.
- **Delivery.** Correctness comes from a **2 s poll of Postgres**. Latency comes from a payload-free commit **wake**. The stream subscribes before its first read. Postgres `LISTEN/NOTIFY` is not used for streams.
- **Cursors.** A cursor is an opaque, scope-bound position `(message_sequence, lifecycle_revision)`. Each page is read in one snapshot, **messages before changes**, so a client never sees a Turn settled before its last message. Replay "repeats nothing and skips nothing".
- **Fan-out.** Fan-out goes through bounded, non-blocking in-process or Redis buses. A dropped preview produces `stream.resync`. A write deadline produces `connection.closing{slow_consumer}`. Execution never waits for a reader.
- **Control.** Control is separate JSON commands: cancel (discard the work), interrupt (stop at a seam and keep the work), idempotent nudges (steering), host tool results, and budget resume.

### Streaming protocol

- **Four frames.**
  - `transcript.update` is durable and the only frame with an SSE `id`/cursor. It carries saved messages and Turn changes.
  - `message.delta` is an ephemeral preview.
  - `stream.resync` means drop your previews.
  - `connection.closing` gives a reason: `settled`, `rotate`, `idle` or `slow_consumer`.
- **Log fields versus current detail.** A Turn change always carries fields true *at that revision*. Current-state detail (tool calls, usage, stop reason) is attached only to the change still current at read time. Replayed history can't leak later outcomes, and one frame answers "why isn't this moving?". `terminal` is computed by the server, so clients never keep their own status lists.
- **Preview identity.** `message_id` is the ID the saved message *will* carry. `attempt` voids previews from a dead execution. A per-block byte `offset` lets clients detect loss themselves. `tool_call_id` and `name` are repeated on every argument fragment. Thinking previews are display-only.
- **Approvals.** There is no approval type. An approval or question is a **host tool**: the Turn parks in `waiting` with no lease, and the current change lists `arguments` and a deadline for `mode: host` calls. Results are bound to the recorded ToolCall, the same property as MAF-Go's approval binding.
- **The spec.** It has 55 numbered MUST rules, a server algorithm, a client fold, and cross-SDK reducer fixtures. The spec is more valuable than the code.
- **Where the docs and code disagree:**
  1. The preview `tool_call_id` is the *provider's* ID, while the saved `tool_use.id` is Nvoken's ToolCall UUID, so the spec's matching rule fails.
  2. The spec shows `attempt: 1` after a resume, but the code produces 2.
  3. Tool *start* writes no stream event, so R25 overstates what clients can see.

### What moves into Dive, and what stays in the gateway

| Move down into Dive v2 | Stay in the gateway |
|---|---|
| A Recorder contract shaped by Nvoken's commit groups: `model_completed` carries the requested tool calls; `tool_started` carries a fence; `model_requested` comes before every attempt | Postgres schema, triggers, lock order, `SKIP LOCKED` claims, leases, reaper |
| Preview identity in the Observer's `Delta`: a reserved message ID, attempt, offset, and **one canonical call ID minted at block start** (fixes discrepancy 1 at its source) | Log counters and cursors. Dive exposes a per-Turn `Seq`, and hosts map it to their own positions |
| Committed versus preview event classes, with documented guarantees (a delta never changes what's saved; the streaming and blocking paths save identical results) | SSE transport: opener, keepalives, rotation, write deadlines, Redis fan-out |
| A typed uncertainty policy: `StopUncertain`, `RetryIfSafe`, `ReportToModel`, rendered by projection and not stored as a synthetic message | Audience projection, tenancy, credits, budget holds, callbacks and webhooks |
| Seams as commands: nudges map to `Deliver`, interrupts to a soft stop at step boundaries, and a Stop hook that continues maps to a typed decision | The approval and question product surface (PRD 052), built on Dive's `Waiting` outcome |
| A stop vocabulary: `completed`, `incomplete` (a limit stopped the Turn cleanly), `failed` (it couldn't stop cleanly), an interrupt counted as *completed*, and a resumable budget hold | |
| A reference Go reducer over committed events and deltas, plus fixtures | |

---

## 12. What the issue trackers say

Full analyses: [eino-issues.md](eino-issues.md) (415 + 227 issues, about 60% in Chinese), [adk-go-issues.md](adk-go-issues.md) (334 issues, 1,272 PRs), and [agent-framework-go-issues.md](agent-framework-go-issues.md). The Go repo's issues are 88% porting bots, so that doc also mines 3,538 issues from the main .NET/Python repo. Reactions are thin in all three trackers. Most of the signal comes from clusters of duplicate issues, long threads, PR reactions, and maintainer stance statements.

### Themes across the trackers

| Theme | Eino | ADK-Go | MAF | Signal for Dive |
|---|---|---|---|---|
| **Provider breadth and dialects** | Every OpenAI-compatible backend streams tool calls differently (missing `index`, reused IDs, empty arguments, dropped `reasoning_content`). Users run forks | **The defining problem.** Other providers were requested 7+ times. The OpenAI PR (50 reactions) took ~8.5 months and shipped text-only on the Responses API. `genai` coupling is "intentional"; the Anthropic PR is still open | Reasoning-replay 400s (Responses reasoning items, DeepSeek), string versus concatenated tool args | **P0.** First-party multi-provider support with a recorded-fixture conformance suite is the biggest single opening |
| **Streaming with tools** | Loudest complaint: text before tool calls ends the loop; the documented fix buffers everything ("Fatal problem", 18 comments) | Gemini 3 metadata-only chunks broke 40–50% of streamed calls | Streaming and non-streaming paths diverged (the session was ignored when streaming) | Stream every delta; decide at end of stream; one execution path (`Collect()` over the stream) |
| **Partial turns / durability** | Checkpoints only at interrupts; per-node auto-checkpoint declined; gob "type not registered" failures | Dangling calls after a crash; timestamp-based optimistic concurrency kills turns | Session not saved on cancel, error or crash, **"by design"**. Checkpoints replay side effects (duplicate emails). "Persist partial turns" was reopened as a feature request | Dive's incomplete-turn and step-log design is exactly what users ask for. Make it the headline, with idempotency keys |
| **Approvals / HITL** | Tool re-entry contract; top-reacted issue asks for a tool-policy hook that binds arguments | Densest bug cluster: empty IDs (filed 3×), infinite loops, a tool ran before confirmation, streaming tools skip confirmation | 114 issues. **Approval applies to the whole batch**, not per call, "by design" (stateless tool loop). A position-dependent security bypass | Per-call, order-independent approval as a Policy before dispatch, bound to recorded requests, tested on every path |
| **Tool errors and results** | Tool errors abort the run, even for built-in and MCP tools ("developer decides") | Unknown tool → nil-pointer panic; errors serialized as `{}`; `map[string]any` results; no size bound | Most-commented issue: tools can't return text plus image. Tools can't see session or call context | `is_error` results by default; rich content results; tool context (session, turn, call IDs, request metadata) via `ctx`; result size limits |
| **Loop bounds and cost** | Hard error at `MaxIterations` | No loop bound ("token cost grows quadratically"); no 429/503 retry | A runaway loop used 100M+ tokens because an outer approval loop reset the inner iteration cap | One budget (steps, tokens, cost) across every layer that re-enters the loop; typed stop reasons; built-in retry that users can find |
| **Concurrency** | Shared agents and tool schemas mutated per run; `send on closed channel`; leaked streams | Map iteration races (filed twice), aliased caller slices | — | `-race` plus concurrent-reuse tests as a release gate; immutable shared definitions |
| **Openness and layering** | Two agent stacks; options that "only work through Runner" | Sealed `Agent`/`Tool`; wrappers silently bypassed | Too many layers per concern (compaction registrable in 3 places, telemetry enabled twice) | Open, honest interfaces; one obvious place per concern |
| **Most requested** | Session/memory (top missing feature for 18 months, now in v0.10 alpha), skills, dynamic tools, per-request params and structured output | Other providers, compaction (the top issue), skills, **evals (stubbed ~10 months)**, OTel, tool OAuth | Skills (33), Dev UI (31), AG-UI (20), realtime, **ACP (open, unclaimed)**, durable execution, per-call context | Ship: session log, compaction, skills, OTel by default, a Go-native eval harness, AG-UI/ACP adapters |

### Maintainer stance

Maintainers reveal as much as users do.
- **Both ports treat another language as the spec.** ADK-Go: *"adk-python is the source of truth for behaviour"*. This was used to reject a Go-first deadline wind-down PR. MAF-Go: *"We try to follow .NET MAF… Closing."* Community feature PRs die in parity review. Being **Go-first and innovating at the Go layer** is a positioning advantage Dive gets for free.
- **Eino is converging on what Dive v2 proposes.** It has moved from "mechanism, not policy" toward an opinionated Claude-Code-style harness. That includes an append-only session event log in v0.10, agents-as-tools over transfer, and an approval hook that binds arguments.
- **MAF is slowly conceding that "the loop needs state".** It added session-backed approval storage and retry-safe approval commits, after a year of declaring stateless behavior "by design".

### What the trackers add to the source analyses

- **Confirmed:** the source critiques. Eino's graph-on-a-loop, gob and abort-on-tool-error. ADK-Go's Gemini coupling, sealed interfaces, missing loop bound and dead `MaxLLMCalls`. MAF-Go's shallow durability, structured-output-plus-tools risk, and untyped tool results.
- **Nuanced:**
  - ADK-Go's HITL *design* is sound, but spreading it across flow, runner and three tool adapters re-breaks it on every new path.
  - MAF-Go treats idiomatic Go as a *secondary* goal that wins small choices, not as no goal at all.
  - Eino's "no cross-run memory" is out of date for the v0.10 alpha.
- **Underweighted by the source analyses:** provider-dialect normalization and cross-provider reasoning replay. These are where users actually lose time, and they belong in Dive's `llm` layer, backed by conformance tests.

---

## 13. pi: the provider layer and a durable harness

Full analyses: [pi.md](pi.md) and [pi-issues.md](pi-issues.md). pi is a TypeScript coding agent, not a peer framework. It is compared here on its two libraries (`pi-ai`, `pi-agent-core`) and the durable runtime being built beneath the product.

**Why it matters.** Users love pi for three things:
- A tiny core they extend themselves, often by asking pi to write the extension.
- Lean context. An independent benchmark measured 81% less input than OpenCode.
- Real multi-provider support with switching mid-session.

A team embedding `pi-agent-core` chose it over LangGraph.js because it is "deliberately *just the agent turn*".

**What to take:**
- **The `llm` layer.** It splits API dialect, Provider and Model, and dialect quirks live as compat flags on the model (for example `requiresReasoningContentOnAssistantMessages` and `thinkingFormat`), generated from a catalog. There is one documented stream event protocol. Errors arrive as messages in the stream. About 160 dialect regression tests back all of this.
- **Cross-provider hand-off.** Provenance is recorded on every message, and replay decisions are made per content block for the target model.
- **Prompt and tool changes recorded in the transcript** (`SystemMessage{sections, toolsAdded, toolsRemoved}`).
- **The harness state machine.** It maps closely onto Dive v2's Engine and Recorder:
  - every transition is one atomic commit;
  - response, usage and result IDs are reserved in the intent commit;
  - partial frames are persisted per delta;
  - `outcome_ready` handles out-of-order parallel completion;
  - `replay: never|safe` is captured at intent time and can only be downgraded on recovery;
  - tools get invocation memos;
  - retry waits are durable and handed back to the host.
- **Pico5's rule:** "no durable progress ⇒ fault".
- **Hook shape.** Boundary hooks return **drafts plus `continue`**, not side effects. `before_tool` fails closed. Tools carry prompt snippets and guidelines. An `operations` interface lets the same tool run locally, over SSH or in a VM.

**What to avoid or fix:**
- **Three runtimes at once.** The shipping CLI runs the in-memory loop plus JSONL, not the durable harness.
- **No limits.** There is no step, token or cost bound anywhere.
- **Stringly-typed errors.** Retryability and context overflow are detected by regex.
- **No cross-process leases or fencing.**
- **One storage commit per streamed delta.**
- **No HITL suspend primitive.** Approvals are blocking UI hooks.
- **A 4,000-line `AgentSession`** that monkey-patches hooks and reconciles steering by text match.

**What pi's tracker adds** (6,450 issues; most are auto-closed on arrival, so the signal is in long threads, reopened issues and title clusters):
- **Cross-provider transcript fidelity is the defining problem.** There are 82 thinking and reasoning replay issues:
  - signed thinking must be replayed byte-exact;
  - `reasoning_content` echo rules vary by lab and gateway;
  - Gemini `thoughtSignature` is lost behind OpenAI-compatible gateways;
  - Responses API call IDs are 450+ characters.

  The all-pairs hand-off test matrix couldn't keep up, so Dive should build it as recorded fixtures.
- **Transcript integrity.** 63 issues are about sessions bricked by orphaned tool calls after an abort, whitespace-only results, or injections mid-batch. **Enforce invariants when writing**, with replay-time repair only as a backstop. Persist each tool result as it completes.
- **Compaction is a second request path that drifts** (340 issues): wrong headers, skipped hooks, hard-coded output caps, and a trigger only at `agent_end`, so a long tool loop overflows the window. Send every model call through **one pipeline** and **one projection function**.
- **"Ended" is not "settled".** Hosts raced post-run retries, compaction and queued follow-ups for a year. Model these as loop states.
- **Hangs** (240 issues). Use a separate idle-stream watchdog, request deadline and retry budget.
- **Tool arguments.** Models add undeclared keys to about 20% of some edit calls. After 500-run experiments the maintainers chose to **accept and ignore extra keys** rather than enable strict decoding.
- **Many sessions per process.** The SDK hijacks the global fetch dispatcher, and extensions assume one session per process.
- **The maintainers refused things a library can't refuse.** Structured output, sampling parameters, model discovery for local models, native permissions and ACP were all pushed to extensions.

---

## 14. OpenHands: record, projection and protocol at scale

Full analyses: [openhands.md](openhands.md) and [openhands-issues.md](openhands-issues.md). The agent core now lives in `software-agent-sdk`, and `OpenHands/OpenHands` is the Agent Canvas frontend.

**The nouns** are Agent (frozen, serializable configuration), Conversation (the aggregate, holding an append-only event log that forks as a tree), Workspace (where tools act: local, Docker or remote) and Event. The same `Conversation` API runs in-process or against a remote Agent Server that runs *inside* the sandbox next to the tools.

**What to take:**
- **The event log is the record, and the prompt is a cached incremental projection** (`state.view`).
- **Tool intent (`ActionEvent`) is persisted before execution.** Subscribers are notified only after persistence.
- **View properties.** Tool pairing, batch atomicity and thinking-block rules are declared once. They define where compaction may cut, and they repair a corrupt history on load.
- **WebSocket v2** (`after_seq` cursor). Its frames are `Sync`, `Durable{seq}`, `Transient`, `ItemStarted{item_id, attempt}`, `Delta`, `ItemAborted` and `Error`. Every `ItemStarted` is retired by exactly one `Durable` or `ItemAborted`. Overflow disconnects the socket rather than dropping a frame. This is **structurally Nvoken's protocol**, arrived at independently. Canvas still uses v1, which replays by timestamp and dedupes on the client.
- **Lease fencing** (a file lease with a generation counter on every write).
- **Resource-keyed parallel tools.**
- **Stuck detection** that nudges once before stopping.
- **A USD budget across every LLM a run uses.**
- **A closed error-classification vocabulary.**
- **Secret masking inside stream chunks.**
- **Contract discipline:** OpenAPI generates the TS client, and golden fixtures plus schema-version migrations guard persisted formats.
- **ACP as a client.** Claude Code, Codex and Gemini CLI run as the agent inside OpenHands' conversation, event log and UI.

**What to avoid:**
- **Size.** Every code path exists twice, sync and async.
- **Hidden persistence.** State is saved when a field is assigned, and the snapshot and event log are not written atomically together.
- **Synthetic user messages in the record** for nudges and errors.
- **Hooks that block actions** from inside the event callback chain.
- **One "unmatched action" path** that means pending approval, crashed and interrupted all at once.
- **Batch-level approval.**
- **A hard 500-iteration cap that ends in `ERROR`.**
- **LiteLLM plus OpenAI-shaped messages as the core.**
- **Event kinds keyed by Python class name** through a process-global registry.

**What OpenHands' tracker adds** (4,888 + 1,755 issues; 1,290 closed by the stale bot; heavy traffic from the team's own agent):
- **Why V1 was rewritten, in the maintainers' words:**
  - five entry points, each with its own configuration system;
  - global state everywhere;
  - a 1,400-line controller;
  - a monorepo that "has grown like a weed";
  - a dual Action and ToolCall model;
  - Claude Code as the reference design.

  **Lesson:** configuration is a value passed in, and a library has one front door.
- **Tool-call and result pairing corrupts conversations permanently.** A duplicate observation on resume, a user message landing mid-tool, or recovery parented to a stale HEAD makes every later request fail. This is why view properties exist.
- **Loading.** One unknown event kind (a custom tool module that wasn't imported) makes the whole conversation fail to load. Use stable `Kind` strings with an opaque fallback.
- **The provider layer is a treadmill.** Examples: LiteLLM parameter dialects (temperature, `stop`, `thinking`); usage parsing that crashes on missing cache fields; thinking-signature errors; and in March 2026 a **LiteLLM credential-stealer release** that blocked CI. Owning the provider layer is also a supply-chain decision.
- **Security trusted the model.** A required `security_risk` argument broke tool calls across models. Only actions the model self-labels HIGH are gated. A user saying "Wait — do NOT run that" while an action awaits approval still lets it run on the next `run()`.
- **Hidden limits.** The iteration cap is invisible to the model and ends in a generic `ERROR`. The stuck detector misfires on reasoning models. "Prose without a tool call means finished" silently quits weaker models mid-task.
- **Injected harness text treated as instructions.** V0 injected "Trimming prompt to meet context window limitations", and the model began trimming the user's prompts.
- **The 2026 pivot.** Container-per-conversation, the largest failure cluster in the tracker, was abandoned. Canvas became an agent-neutral control plane, and the SDK is "an advanced step along the user journey, not the starting point".

### Updated cross-tracker themes

pi and OpenHands extend the §12 table. The rows below are the ones with new evidence:

| Theme | pi | OpenHands | Signal for Dive |
|---|---|---|---|
| **Provider dialects and replay** | 82 thinking-replay issues; 322 issues about Chinese-lab models; `compat` flags added one incident at a time | LiteLLM parameter and response drift, signature errors, supply-chain compromise | Now **all five** trackers put this at or near the top. Own the provider layer, express dialects as data, and back each provider with recorded conformance fixtures |
| **Transcript integrity** | Orphaned calls after abort; results lost per batch; injections mid-batch | Duplicate observations; messages landing mid-tool; stale-HEAD recovery | Invariants enforced when writing, per-call result commits, a queue for mid-tool input, and cold-load repair that is itself recorded |
| **Loop limits** | None exist | Hard cap at 500, reported as `ERROR`, invisible to the model | A typed `Limited` stop with a final no-tools answer, limits the model can see, and the ability to continue after a limit |
| **Approval states** | No suspend primitive; requests for a "waiting on user" state unmet | Pending, in-flight and interrupted conflated; implicit consent | Keep `Waiting`, `Uncertain` and `NotExecuted` as distinct step kinds. A new user message is never consent |
| **Compaction** | A drifting second request path; triggered only at turn end | Event-count triggers; skills lost after condensing; cache waste | Token-aware triggers at every safe point, through one pipeline and one projection, cache-preserving |
| **Most requested** | XDG config (refused), MCP (softening via "codemode"), **ACP (40 upvotes, refused)**, local model discovery, a separate compaction model | Git forges, MCP, **ACP / bring-your-own subscription** (the fastest-growing SDK cluster), no Docker, forking, budgets, structured output | ACP demand now shows up in MAF, pi and OpenHands, and only OpenHands has shipped it (as a client) |

---

## 15. ACP (Agent Client Protocol)

The ACP research lives in [../acpresearch/](../acpresearch/README.md). There is a first-pass survey, then three deeper dives: the agent adapter with a working spike, ACP v2 and streaming convergence, and client orchestration. There is also a draft comment for [ACP PR #2208](../acpresearch/pr-2208-draft-comment.md). Headlines that bear on this survey:

- **Demand is real across trackers.** MAF, pi (40 upvotes, refused) and OpenHands (the fastest-growing SDK cluster) all have ACP requests. Only OpenHands ships it, and only as a client.
- **Exposing Dive over ACP is feasible today.** The spike wrapped an unmodified Dive agent and drove it with a real client. Every gap it found is on Dive's side:
  - replay-unique message IDs;
  - a tool-start event;
  - per-call usage;
  - persisted diffs;
  - no synthetic messages in the record.

  These are the same gaps Dive v2's record model closes (§10).
- **One event vocabulary, many projections.** ACP v2, Nvoken SSE, OpenHands WS v2, acpx and the Dive v2 proposal agree on the essentials:
  - IDs minted by the producer and shared by previews and records;
  - lifecycle kept separate from content;
  - replay followed by live delivery;
  - waiting-on-human as visible state.

  Dive should own a three-class vocabulary (committed, preview, signal) that projects onto all of them.
- **Consuming ACP agents is a control-plane job,** not an `Agent.CreateResponse` feature. ACP permission prompts are not a security boundary; the sandbox is.

---

## 16. Open questions this survey raises for Dive

1. **Journal granularity and cost.** ADK-Go writes every non-partial event in its own transaction, and Nvoken needs about five fenced transactions per iteration with one inline tool. Should the Recorder contract allow hosts to fold steps (Nvoken's `CommitModelCheckpoint` writes message, tool calls and checkpoint in one statement)? Or should it allow async acknowledgement with a weaker, documented guarantee for lightweight uses?
2. **Recovering a tool that may have run.** When a step shows a recorded intent but no result, the tool may or may not have executed. Nvoken's evidence suggests offering the three typed policies from §11 (`StopUncertain`, `RetryIfSafe`, `ReportToModel`). Which should be the default for a library? `StopUncertain` is the safest; `RetryIfSafe` is the most usable.
3. **Orchestration scope.** All three libraries ship a graph engine, and Eino and ADK-Go now steer users toward agents-as-tools instead. Should Dive v2 ship only `RunNode`-style memoized sub-steps and skip a declarative graph entirely?
4. **Server-side conversation state** (OpenAI response IDs, provider caches). MAF-Go's dual ownership of history created a three-flag conflict policy and a cluster of multi-agent history bugs. The recommendation is that local history is always authoritative and provider-side state is an optimization stored on the turn. This should be confirmed against the providers Dive supports.
5. **How much of Nvoken's wire protocol belongs in Dive?** The event *vocabulary* and preview-identity rules clearly do. A Go reference reducer plus fixtures probably does, because Noodle and Mobius need the same fold. The transport and cursors should stay in hosts. Is an optional `dive/stream` SSE adapter worth shipping for smaller hosts, given that AG-UI and ACP adapters are also in demand?
6. **Approval primitive.** Nvoken deliberately models approvals as host tools. MAF-Go has a first-class approval content type, and users of all three trackers ask for per-call approval policy. Should Dive expose approval as a typed `Waiting` reason with its own record, or keep it a Policy decision that produces an ordinary waiting tool call?
7. **Provider conformance as a product.** The trackers say provider breadth decides adoption. Should Dive publish its provider conformance suite (recorded fixtures for stream assembly, reasoning replay, parallel calls, refusals) as a reusable package that third-party provider authors can run?
8. **How much of pi's `llm` design to adopt wholesale.** The API/Provider/Model split, compat flags as model data and `TransformForTarget` look strictly better than Dive v1's Anthropic-shaped `llm`. Should v2's Layer 0 be redesigned around them, including a generated model catalog with costs and flags? That also means committing to maintain the catalog.
9. **Durable partial streams.** pi persists every delta; Nvoken and OpenHands keep previews ephemeral with identity. Should the Recorder contract treat partial frames as an optional capability that is off by default?
10. **Strict versus lenient tool arguments.** pi's experiments favor accepting and ignoring extra keys over strict decoding. OpenHands broke tool calling by adding a required schema field. What should Dive's default validation policy be, and should it be per tool?
11. **ACP direction.** OpenHands shows the client side (hosting other agents). pi, MAF and OpenHands users all ask for the agent side. Should `adapters/acp` do both, with `request_permission` routed through Policy so an ACP turn can suspend?
