# Pi vs Dive: Compare & Contrast

Analysis of [earendil-works/pi](https://github.com/earendil-works/pi) — specifically `packages/agent` (pi-agent-core) and `packages/ai` (pi-ai) — versus Dive. Both solve the same problem at similar scope: a layered toolkit for building agents on top of multi-provider LLMs, with tools, hooks, and a CLI harness. The interesting differences are in shape, not size.

> This document was self-audited against dive's source after a first pass made several claims that turned out to be wrong about dive's existing capabilities. Where this doc says "dive doesn't X," it cites code; where pi has something dive lacks, the gap is described precisely.

## 1. Layering — both stack the same way

| Layer | Dive | Pi |
|---|---|---|
| Provider abstraction | `llm.LLM` / `llm.StreamingLLM` + `providers/` registry | `stream(model, context, options)` + `api-registry.ts` registry |
| Agent runtime | `*dive.Agent` (struct with internal mutex) | `Agent` (class) wrapping `agentLoop()` |
| Tools | `Tool` / `TypedTool[T]` / `FuncTool[T]()` | `AgentTool<TParameters>` |
| Session / state | `dive.Session` interface, `session/` package | No session — state lives on `Agent.state`, app supplies persistence |
| Extensions | `dive.Extension` (Tools, Hooks, Rules) | `ExtensionContext` callback (registerTool/registerCommand/on) |
| CLI harness | `experimental/cmd/dive/` | `packages/coding-agent` |

Same layering, different philosophy: **Dive's `CreateResponse(ctx, opts...)` is stateless from the caller's POV — conversation state lives in `Session`, and one call = one resolution (possibly suspending). Pi's `Agent` is a long-lived object you poke imperatively (`agent.prompt(...)`, `agent.steer(...)`, `agent.followUp(...)`), with reactive state on `agent.state`.** Both projects' agent objects are internally mutable; the difference is whether mutation is part of the public API.

## 2. Provider layer — small gaps, not large ones

Both use registries (`providers/registry.go` vs `api-registry.ts`); both are self-registering. The interesting divergences:

### Reasoning / thinking

Both have a unified reasoning abstraction. Dive's is in `llm/options.go:199-204`:

```go
type ReasoningEffort string
const (
    ReasoningEffortLow    ReasoningEffort = "low"
    ReasoningEffortMedium ReasoningEffort = "medium"
    ReasoningEffortHigh   ReasoningEffort = "high"
)
```

Plus a separate `ReasoningBudget *int` for Anthropic-style token budgets, both wired through `ModelSettings`.

Pi's equivalent (`packages/ai/src/types.ts`):

```ts
type ThinkingLevel = "minimal" | "low" | "medium" | "high" | "xhigh";

interface Model<TApi> {
  reasoning: boolean;
  thinkingLevelMap?: Partial<Record<ModelThinkingLevel, string | null>>;
  // null = unsupported on this model, string = provider-specific value
}
```

**Narrow gap, not large**:

- Pi has two extra levels (`minimal`, `xhigh`)
- Pi declares per-model capability via `thinkingLevelMap` so callers can pick "high" without first checking model compat; dive expects the caller to know what each model supports
- Pi's cross-provider handoff converts thinking blocks to `<thinking>` tagged text for compatibility — useful when a session bounces between Claude and Gemini

### Cost / usage

Pi puts cost on every assistant message. Dive doesn't:

```go
// llm/usage.go — just tokens
type Usage struct {
    InputTokens              int
    OutputTokens             int
    CacheCreationInputTokens int
    CacheReadInputTokens     int
}
```

`llm/pricing.go` defines `PricingInfo` (price per 1M tokens) but it's not wired into `Usage` — callers do the multiplication themselves. Pi bundles `cost: { input, output, cacheRead, cacheWrite, total }` into every `AssistantMessage`, computed at the provider boundary from the model's pricing table.

### Streaming events

Pi emits typed lifecycle triples (`text_start`/`text_delta`/`text_end`, same for `thinking_*` and `toolcall_*`), and each event carries a `partial: AssistantMessage` snapshot. Dive's `llm.Event` carries `Type`/`ContentBlock`/`Delta`, with `ResponseAccumulator` providing the snapshot pattern as a separate utility. Both work; pi's "every event ships the latest partial" pattern is slightly easier for UI consumers (no accumulator wiring).

## 3. Agent runtime — different control models

**Dive: turn = function call.** `CreateResponse(ctx, opts...)` runs one resolution. Suspension returns a `Response` with `Status == ResponseStatusSuspended` and a typed `*SuspensionState`; you resume by calling again with `WithResume(state, results)` or `WithToolResults(...)`. Session state is loaded/saved automatically when a `Session` is configured.

**Pi: turn = method on a long-lived object.** `agent.prompt(message)` kicks off work; the runtime runs to completion in the background; you `await agent.waitForIdle()` or subscribe to events. State mutates on `agent.state` (with copy-on-write accessor properties). The underlying `agentLoop()` is also exposed as a pure function returning an `EventStream` — both styles available.

### Hook surface comparison

Dive's hooks are typed callback slices on `AgentOptions.Hooks` (`agent.go:50-87`):

```go
type Hooks struct {
    PreGeneration         []PreGenerationHook
    PostGeneration        []PostGenerationHook
    PreToolUse            []PreToolUseHook
    PostToolUse           []PostToolUseHook
    PostToolUseFailure    []PostToolUseFailureHook
    Stop                  []StopHook
    PreIteration          []PreIterationHook
    OnSuspend             []OnSuspendHook
    PostBackgroundToolUse []PostBackgroundToolUseHook
}
```

All receive `*HookContext`, which carries `Agent`, `Session`, `Values` (cross-hook scratch map), and phase-specific fields. Mutation points:

- `hctx.SystemPrompt` / `hctx.Messages` (PreGeneration, PreIteration)
- `hctx.UpdatedInput []byte` (PreToolUse) — rewrites tool args
- `hctx.Result` (PostToolUse) — `hooks.go:176-177`: *"Hooks can modify hctx.Result to transform the tool output before it's sent to the LLM."*
- `hctx.AdditionalContext` — appends a text block to the tool-result message without mutating the result itself

Pi's hooks are config callbacks on `AgentLoopConfig`:

```ts
transformContext?:     (msgs) => Promise<AgentMessage[]>
convertToLlm:          (msgs) => Message[]
beforeToolCall?:       (ctx)  => Promise<{deny?, autoApprove?}>
afterToolCall?:        (ctx)  => Promise<{override?}>
shouldStopAfterTurn?:  (ctx)  => boolean
prepareNextTurn?:      (ctx)  => {systemPrompt?, model?, ...}
getSteeringMessages?:  ()     => Promise<AgentMessage[]>
getFollowUpMessages?:  ()     => Promise<AgentMessage[]>
```

The genuine gaps where pi does something dive doesn't:

1. **`prepareNextTurn` can swap model mid-loop.** Dive's `PreIterationHook` can rewrite `SystemPrompt` and `Messages` between iterations, but `HookContext` does **not** expose the model — so within a single `CreateResponse` call the model is fixed. Dive has `agent.SetModel(model)` (`agent.go:347`) for between-call swaps. Pi lets the agent itself escalate from a cheap model to an expensive one between turns within one stream.
2. **`getSteeringMessages` / `getFollowUpMessages`.** Dual-queue mechanism for injecting messages mid-execution. No dive equivalent (see §7).

What dive has that pi doesn't:

- **Typed `StopHook` with `StopDecision{Continue, Reason}`** — pi's `shouldStopAfterTurn` is a plain boolean
- **`PostToolUseFailureHook`** as a distinct event from successful PostToolUse
- **`OnSuspendHook`** — fires before persistence, can abort the suspend transition with a `HookAbortError` and the session stays in its previous state
- **`PostBackgroundToolUseHook`** for results returning from background goroutines
- **`HookAbortError`** for hard-abort semantics distinct from regular errors

## 4. Tool model — pi's is more streaming-aware, dive's is more state-aware

Dive's `Tool`:

```go
type Tool interface {
    Name() string
    Description() string
    Schema() *Schema
    Annotations() *ToolAnnotations
    Call(ctx context.Context, input any) (*ToolResult, error)
}

type ToolResult struct {
    Content    []*ToolResultContent  // what LLM sees
    Display    string                // human-readable markdown summary
    IsError    bool
    Suspend    *SuspendResult        // tagged union — exactly one of Content/Suspend/Background
    Background *backgroundResult
}
```

Pi's `AgentTool`:

```ts
interface AgentTool<TParameters> {
  name; description; label; parameters;
  prepareArguments?(args): Static<TParameters>;
  execute(toolCallId, params, signal, onUpdate?): Promise<AgentToolResult<TDetails>>;
  executionMode?: "sequential" | "parallel";
}
interface AgentToolResult<T> {
  content: (TextContent | ImageContent)[];
  details: T;                       // typed structured metadata for UI/logs
  terminate?: boolean;
}
```

### Streaming output

Both support mid-execution streaming, but with different shapes.

Dive (`context.go:30-43`):

```go
func WithToolStreamFunc(ctx context.Context, fn func(toolCallID string, text string)) context.Context
func StreamOutput(ctx context.Context, text string)  // tool calls this
```

The agent injects a stream function via context (`agent.go:2200-2208`); chunks flow out as `ResponseItemTypeToolStream` events through the `EventCallback`. **Text-only.**

Pi:

```ts
execute(toolCallId, params, signal, onUpdate?: (partialResult: AgentToolResult<TDetails>) => void)
```

`onUpdate` receives a **typed structured partial result**, not just text. The narrow gap: dive can stream `bash` output as text, but can't stream a partial `{exitCode, duration, stdout, stderr}` shape without serializing it.

### Tagged-union result types

Dive's `ToolResult` is a tagged union: a regular result, OR a `Suspend`, OR a `Background` dispatch. Validated at the agent boundary; malformed results route through `PostToolUseFailure`. Pi doesn't have first-class suspend/background result types — suspension is handled at the hook layer (`beforeToolCall` denial or external queue management), not as a tool's return value.

**Dive wins on result expressiveness.** First-class suspend-as-result and background-dispatch-as-result are real architectural advantages over pi's "stop and call us back" model.

### Parallel execution

Both support parallel tool execution within a turn. Dive (`agent.go:194,253,1673`):

```go
parallelToolExecution bool  // from AgentOptions.ParallelToolExecution
```

Two-phase: PreToolUse hooks run sequentially across all tools, then tool executions run in parallel, then PostToolUse hooks and result events run sequentially. **All-or-nothing per agent.** Pi has per-tool opt-out via `executionMode?: "sequential" | "parallel"` — useful for tools that aren't thread-safe (e.g., a state-mutating tool can opt out while bash/read/grep stay parallel).

### Other pi tool features

- **`prepareArguments`** — schema-compat shim to clean up sloppy JSON before validation. Dive expects schema-valid input.
- **`label`** — UI-facing display name distinct from `name`. Dive has none.
- **`terminate?: boolean`** result hint — tells the agent to stop after this batch. Dive uses `StopHook` for similar control (less ergonomic from inside a tool).

## 5. Suspend/resume — dive's typed state machine is more rigorous

Dive's suspend/resume is a first-class typed state machine:

```go
type SuspendReason string
const (
    SuspendReasonInput SuspendReason = "input_required"
    SuspendReasonAuth  SuspendReason = "auth_required"
)

type SuspendResult struct {
    Prompt   string
    Reason   SuspendReason
    Metadata map[string]any
}

type SuspensionState struct { /* PendingToolCalls, CompletedToolCalls, TurnMessages, ... */ }
```

Plus `SuspendableSession` extension for auto-persistence, `OnSuspendHook` firing before persistence (can abort), `CancelSuspension(ctx)` to abandon, and explicit mapping to A2A's `input-required` state.

Pi has nothing comparable in pi-mono itself — approval/HITL is handled via `beforeToolCall` returning `{ deny: ..., autoApprove: ... }`, and the runtime simply waits for the next `prompt()`. There's no typed "the agent is paused awaiting X" state. Pi's `pi-chat` package handles richer flows externally.

**Dive wins here, decisively.**

## 6. Messages — pi's extensibility is clever but Go-hostile

Pi lets apps add custom message types (`artifact`, `notification`, …) via TypeScript declaration merging on `CustomAgentMessages`, then filters them out via `convertToLlm` before LLM calls. The agent stream can carry arbitrary app-specific events alongside LLM messages with full type safety.

Dive has `ResponseItemType` as a closed const enum (`response.go:15`); extending it requires changing the core type. Not really portable to Go's type system. The *pattern* — a message interface with a convert-to-LLM filter step — could be borrowed if first-class artifact/notification/tool-progress messages ever become a need, but it's a significant API change.

## 7. Steering & follow-up — same mechanics dive already has, different API

```ts
agent.steer(message)    // inject while agent is still working through tool calls
agent.followUp(message) // queued; drained when agent would otherwise stop
```

### How steering actually works in pi

Reading `packages/agent/src/agent-loop.ts`, both queues drain as **plain user messages slotted into the conversation** — they do **not** attach to tool result messages. The mechanism is straightforward: two callbacks (`getSteeringMessages`, `getFollowUpMessages`) polled at two different sites in the loop.

The outer/inner loop structure (paraphrased):

```ts
while (true) {                                            // outer
    while (hasMoreToolCalls || pendingMessages.length > 0) {  // inner
        // inject pendingMessages as plain user messages
        // stream assistant turn
        // execute tool calls (results become toolResult messages)
        // emit turn_end; prepareNextTurn; shouldStopAfterTurn
        pendingMessages = getSteeringMessages() ?? [];     // ← STEER POLL
    }
    // inner loop exited → agent would stop here
    const followUps = getFollowUpMessages() ?? [];          // ← FOLLOW-UP POLL
    if (followUps.length > 0) { pendingMessages = followUps; continue; }
    break;
}
```

The key insight: **steering only drains while the inner loop is alive**, i.e., while `hasMoreToolCalls || pendingMessages.length > 0`. The inner loop stays alive in two ways:

1. The last assistant turn produced tool calls that didn't `terminate` — work is still ongoing.
2. A steer message was drained on the prior iteration — extends the inner loop by one more turn so the LLM can react to it.

Once the inner loop exits (no tool calls AND no pending steers), control falls to the outer loop, which polls follow-up instead.

| Loop position | What's polled | UX meaning |
|---|---|---|
| Inside inner loop (work in flight) | `getSteeringMessages` | "while you're still working, also consider this" |
| Inner loop exited (agent would stop) | `getFollowUpMessages` | "after you've stopped, here's the next task" |

The two queues are the same shape (plain user messages, injected the same way). They're drained from different polling sites that correspond to different UX intentions.

### What dive already has

Dive has both polling sites today:

- **Steer semantics** → `PreIterationHook` (`hooks.go:205`) fires before each LLM iteration inside the generation loop. A hook closure that drains an external queue and appends to `hctx.Messages` reproduces pi's steering exactly. (Dive's hook actually fires unconditionally per iteration, where pi's `getSteeringMessages` only fires while the inner loop is alive — slightly different but covers the same UX.)
- **Follow-up semantics** → `StopHook` returning `*StopDecision{Continue: true, Reason: msg}` (`hooks.go:243-253`) re-enters the generation loop with `Reason` injected as a user message. Same effect as pi's follow-up queue draining at the outer-loop poll site.

So the *capability* is there in dive today. What pi adds on top:

1. A blessed convenience API (`agent.Steer(msg)`, `agent.FollowUp(msg)`) so callers don't wire their own queues
2. The distinction between steer and follow-up is **surfaced in the API** — the caller picks which polling site their queue feeds into, rather than wiring it themselves with the right hook
3. Drain modes (`"all"` vs `"one-at-a-time"`) for how the queue empties per polling tick
4. Plugs into pi's long-lived `Agent` model, which fits the "user is typing while agent runs" UX naturally — dive's stateless `CreateResponse` makes the queue's lifecycle the caller's problem

**Revised assessment:** the gap is **ergonomic, not architectural**. A `Steer()` / `FollowUp()` convenience layer is sugar over `PreIterationHook` and `StopHook` respectively — useful sugar, but no new control-flow primitives required. Steering during a single `CreateResponse` works today; the API just isn't ergonomic and the steer-vs-follow-up *distinction* isn't surfaced.

## 8. Extensions — different scopes, both reasonable

Dive `Extension` (`agent.go:92-102`): declarative — `Tools() []Tool`, `Hooks() Hooks`, `Rules() string`. Merged at `NewAgent`. Pure, composable, no runtime side effects.

Pi `ExtensionContext`: imperative — `registerTool()`, `registerCommand()`, `on(event, ...)`, plus UI manipulation (`ui.setStatus()`, `ui.setWidget()`). Runs once on harness start.

These operate at different layers: dive's `Extension` is an agent-library concept; pi's `ExtensionContext` is a coding-agent-harness concept. Dive's `skill.Loader implements dive.Extension` is the closer analogy to pi's extension story, and the two are pretty similar in spirit.

## 9. TUI — pi has one; dive doesn't

`packages/tui` is fully decoupled from the agent (zero imports of agent-core or coding-agent) and is genuinely good: differential rendering, CSI 2026 sync, IME support. Dive's CLI is in `experimental/cmd/dive/` and is much simpler. If a TUI ever matters for dive, the *separation* is the lesson — keep the renderer agent-agnostic.

---

## Recommendations — what's actually borrowable

After auditing dive's source, the original recommendation list shrank considerably. Most "dive should add X" turned into "dive already has X." Here's what survives:

### Genuine, smaller gaps

1. **Materialize cost into `llm.Usage`.** Add `InputCost`/`OutputCost`/`CacheCost`/`TotalCost` fields (or a `Cost` sub-struct), populated at the provider boundary from `PricingInfo`. Today every caller does the multiplication.
2. **Per-tool parallel-execution opt-out.** `parallelToolExecution` is currently all-or-nothing per agent. Add `ParallelSafe() bool` to the `Tool` interface (or a flag on `ToolAnnotations`) so non-thread-safe tools can opt out while the rest stay parallel.
3. **Structured tool update channel.** `StreamOutput(ctx, text)` is text-only. Add a typed counterpart — e.g. `dive.UpdateTool(ctx, *ToolResultContent)` or `dive.UpdatePartialResult(ctx, any)` — for tools that want to stream structured progress.
4. **Expose `Model` on `HookContext` for `PreIteration`.** Lets a hook swap from a cheap model to an expensive one mid-loop based on what's happening, without needing a separate API surface.
5. **More reasoning levels + per-model capability map.** Add `ReasoningEffortMinimal` / `ReasoningEffortExtraHigh` if any model supports them; let `ModelSettings` or model metadata declare which levels are supported so callers can request "high" without first checking compat.

### Ergonomic sugar (capability already exists)

6. **`Steer()` / `FollowUp()` convenience methods on a long-lived runner.** Sugar over `PreIterationHook` (steer = inject while inner loop is alive) and `StopHook` returning `{Continue: true, Reason: msg}` (follow-up = restart loop after agent would stop). The distinction matters because it's how the caller picks *which polling site* their queue feeds into. Needs a Conversation/Runner wrapper to host the queues — not a fit for stateless `CreateResponse` directly, but no new control-flow primitives required.
7. **`label` on tools.** A UI-facing display name distinct from `Name()`. Trivial; nice for TUIs and audit logs.

### Things NOT to copy

- **Mutable `Agent.state` as primary API** — un-Go; dive's `CreateResponse` + `Session` boundary is cleaner.
- **Declaration-merging custom message types** — relies on TypeScript-specific semantics; the equivalent in Go would require a sealed-interface gymnastic that's worse than the closed `ResponseItemType` enum dive has today.
- **TS-style "registry of streaming functions"** — dive's interface-based provider model is already idiomatic.

### What dive already does better than pi

- **Typed suspend/resume** with `SuspendReason`, `SuspensionState`, `OnSuspendHook`, `SuspendableSession`, A2A `input-required` mapping
- **Tagged-union `ToolResult`** with first-class `Suspend` and `Background` variants
- **`PostToolUseFailureHook` as a distinct event** from PostToolUse
- **`HookAbortError`** for hard-abort semantics distinct from regular errors
- **`Extension` interface** (declarative Tools/Hooks/Rules) is cleaner than pi's imperative `ExtensionContext`
- **Background tool dispatch** with `PostBackgroundToolUseHook` for OTel span closure on result delivery
