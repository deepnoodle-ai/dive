---
Title: Tracer interface for agent observability
Author: Curtis Myzie
Status: Draft
Last Updated: 2026-05-04
Supersedes: parts of plan-05-otel-improvements.md (the hook-based design)
---

# Tracer interface for agent observability

## Goal

Replace the hook-based OTel integration with a small, purpose-built `Tracer`
interface in dive core. The OpenTelemetry extension becomes one adapter
implementing that interface; other adapters (slog, Datadog, no-op for tests)
can implement it without touching dive core or pulling OTel into the core
module.

The motivating problems with the current hook-based design:

- Adding the OTel extension is not enough — users must remember to call
  `otelext.Run(ctx, agent, …)` instead of `agent.CreateResponse(ctx, …)`,
  otherwise the trace tree is broken (chat / execute_tool spans become
  roots instead of children of `invoke_agent`).
- Hooks observe ctx but cannot naturally influence what comes next, which
  forced an awkward `HookContext.UpdatedCtx` side-channel — added to
  `BeforeGenerate` and `PreToolUse` but not `PreGeneration`, leaving the
  contract inconsistent.
- A separate `LLMHookExtension` interface had to be introduced so an
  extension could contribute provider-level hooks alongside agent-level
  hooks. The split is incidental, not principled.
- The chat-span queue keyed by `context.Context` is brittle: any future
  provider that uses a derived ctx for `AfterGenerate` silently leaks
  spans (with only a debug log).
- Composition is hostile — two extensions that both want to install spans
  hit `UpdatedCtx` last-write-wins.

Hooks are still the right shape for *modification* concerns (rewriting
prompts, denying tool calls, injecting context, skills). They are the
wrong shape for *observation*. This plan separates the two.

## Approach

Two changes, one in dive core and one in `experimental/otel`:

1. **Add a `Tracer` interface to dive core.** Pure stdlib + dive types,
   no OTel dependency. The agent calls it inline at the lifecycle
   boundaries that matter for tracing (`CreateResponse` start/end, each
   chat iteration, each tool call). The agent owns its own ctx flow —
   each `Start*` method takes ctx and returns ctx, so spans nest
   naturally.

2. **Reimplement `experimental/otel` as a `Tracer` adapter.** Same OTel
   GenAI semconv coverage as today (spans, attributes, metrics), but
   delivered through the new interface instead of through hooks. The
   extension's hook plumbing, `Run` helper, ctx-keyed span queues, and
   `LLMHooks()` method all go away.

Hooks remain for everything that is currently hook-shaped and not
observation: `Extension`, skills, compaction, `InjectContext`, permission
gates, etc. Only the OTel extension stops using them.

## High-level API

### Core interface (in `dive` package)

```go
type Tracer interface {
    StartAgentRun(ctx context.Context, info AgentRunInfo) (context.Context, AgentRunSpan)
    StartChat(ctx context.Context, info ChatInfo) (context.Context, ChatSpan)
    StartToolCall(ctx context.Context, info ToolCallInfo) (context.Context, ToolCallSpan)
}

type AgentRunSpan interface {
    SetResponse(*Response)
    SetUsage(*llm.Usage)
    End(err error)
}

type ChatSpan interface {
    SetResponse(*llm.Response)
    SetTimeToFirstChunk(seconds float64)
    End(err error)
}

type ToolCallSpan interface {
    SetResult(*ToolCallResult)
    End(err error)
}
```

`AgentRunInfo`, `ChatInfo`, and `ToolCallInfo` are small, stable views of
what's about to happen — model name, message count, sampling params,
streaming flag, session, etc. — without exposing internal `llm.Config`
fields the tracer has no business with.

A package-level `NopTracer` implementation lets the agent skip nil
checks. The agent's default tracer is `NopTracer{}`; setting one is
opt-in via `AgentOptions.Tracer`.

### Wiring on the agent

```go
type AgentOptions struct {
    // ...existing fields...
    Tracer Tracer // optional; defaults to NopTracer
}

agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:   "Research Assistant",
    Model:  anthropic.New(),
    Tracer: otelext.NewTracer(),
})

resp, err := agent.CreateResponse(ctx, dive.WithInput("…")) // no Run wrapper
```

### Inside the agent

The agent calls the tracer at three points and threads the returned ctx
through:

```go
func (a *Agent) CreateResponse(ctx context.Context, opts ...) (*Response, error) {
    ctx, runSpan := a.tracer.StartAgentRun(ctx, AgentRunInfo{Agent: a, Session: sess})
    defer runSpan.End(retErr)

    for iteration := 0; iteration < limit; iteration++ {
        ctx, chatSpan := a.tracer.StartChat(ctx, ChatInfo{Agent: a, /* ... */})
        resp, err := a.model.Generate(ctx, ...)   // model HTTP nests under chat
        chatSpan.SetResponse(resp)
        chatSpan.End(err)

        for _, call := range toolCalls {
            toolCtx, toolSpan := a.tracer.StartToolCall(ctx, ToolCallInfo{Tool: tool, Call: call})
            result := tool.Call(toolCtx, ...)     // tool-internal spans nest under execute_tool
            toolSpan.SetResult(result)
            toolSpan.End(nil)
        }
    }
}
```

No `UpdatedCtx`. No hook queues. No "did you remember to call `Run`."

### Composition

`MultiTracer(t1, t2, ...)` fans Start/End calls out to N tracers. Each
returned ctx is the *last* tracer's ctx (since OTel only respects one
parent — multiple tracers writing into the same ctx is fine because each
adds its own span via `WithValue`-style key, and parent linkage is per
tracer-provider). Adapter detail; covered during implementation.

### OTel adapter (`experimental/otel`)

Public surface shrinks substantially:

```go
// Construct the adapter — implements dive.Tracer.
func NewTracer(opts ...Option) dive.Tracer

type Option func(*Options)

func WithProvider(string) Option        // gen_ai.provider.name
func WithTracer(trace.Tracer) Option    // override OTel tracer
func WithMeter(metric.Meter) Option     // override OTel meter
func WithCaptureMessages(bool) Option   // privacy: include input/output
func WithCaptureToolIO(bool) Option     // privacy: include tool args/results
func WithAttributes(...attribute.KeyValue) Option
```

Removed from the current public surface:

- `Extension` type and `New() *Extension` (constructor returns `dive.Tracer` now)
- `Run`, `Extension.Run` (no longer needed)
- `LLMHooks()` method (no longer needed)
- `OperationChat` / `OperationExecuteTool` / `OperationInvokeAgent` constants (internal)
- Vendor-specific `Attr*` constants from `semconv.go` (vendor coupling —
  callers pass any custom resource attributes via `WithAttributes` like any
  other consumer)
- `WithSystem` (deprecated alias — no released callers to preserve)
- `gen_ai.system` legacy attribute (no migration window needed for a brand-new package)

Removed from dive core (with this plan applied):

- `LLMHookExtension` interface (agent.go)
- `HookContext.UpdatedCtx` (hooks.go)
- `llm.HookContext.UpdatedCtx` (llm/hooks.go)
- `HookRequestContext.Streaming` and `HookRequestContext.Endpoint`
  (the tracer gets these directly via `ChatRequest`)
- The agent's `fireLLMAfterGenerate` path can be simplified — providers
  no longer need to fire `BeforeGenerate`/`AfterGenerate` for tracing
  purposes. Whether to remove provider-side firing entirely or keep it
  for non-tracing hook use cases is a follow-up decision.

## Migration

This is a breaking change to `experimental/otel`. The package is
explicitly experimental, has no released consumers we need to preserve,
and the new shape is strictly cleaner — so a hard cut is appropriate.

For dive core:

- `AgentOptions.Tracer` is additive and optional. Existing agents
  without it get `NopTracer{}` and behave identically.
- Removing `LLMHookExtension` and `HookContext.UpdatedCtx` is
  technically breaking for any out-of-tree consumer that wired them up.
  Search shows no such consumer in this repo. Acceptable to drop.

## Implementation phases

### Phase 1 — Tracer interface in dive core

- Define `Tracer`, `ChatRequest`, `AgentRunSpan`, `ChatSpan`,
  `ToolCallSpan`, `NopTracer` in `tracer.go` (or `observability.go`).
- Add `AgentOptions.Tracer` and `Agent.tracer` field; default to
  `NopTracer{}`.
- Wire tracer calls into `CreateResponse`, the iteration loop in
  `generate`, and both serial and parallel tool execution paths.
- Remove `HookContext.UpdatedCtx`, `llm.HookContext.UpdatedCtx`,
  `LLMHookExtension`, `HookRequestContext.Streaming`, and
  `HookRequestContext.Endpoint`.
- Update agent_test.go and any hook tests.

### Phase 2 — OTel adapter rewrite

- Replace `Extension` with `Tracer` implementation.
- Each `Start*` method opens a span, returns a span object that closes
  it on `End`. No queue, no ctx-keyed maps.
- Metrics emission (`gen_ai.client.operation.duration`,
  `gen_ai.client.token.usage`) moves into `ChatSpan.End`.
- Delete `run.go`. Delete vendor-specific `Attr*` constants from `semconv.go`.
- Update `experimental/otel/extension_test.go` and
  `coverage_test.go` to drive the agent normally (no `Run`).
- Update `examples/otel_example/main.go` — drop `Run`, set `Tracer:`
  on `AgentOptions`.
- Update `docs/guides/experimental/otel.md`.

### Phase 3 — Provider cleanup (optional, follow-up)

With tracing handled by the agent's tracer, providers no longer need
to populate `Endpoint` / `Streaming` on `HookRequestContext`. They
can stop firing `BeforeGenerate` / `AfterGenerate` for tracing
purposes. Whether to keep these hook firings for other (non-tracing)
use cases is a separate decision. Out of scope for the initial
implementation.

## Open questions

- **`ChatRequest` shape.** What exactly does the tracer need to see?
  At minimum: model, streaming flag, endpoint, session, message count,
  sampling params (max tokens, temperature, etc.). Should it carry
  `*llm.Config` directly, or a tighter projection?
- **`MultiTracer` semantics.** Do we ship one in core, or leave it to
  adapters? Leaning core, since it's tiny and useful for tests
  ("install a recording tracer alongside OTel").
- **Suspend/resume.** The current `OnSuspend` hook is wired to mark
  the agent span. Equivalent in the tracer interface is
  `AgentRunSpan.SetSuspended()`. Suspended state is otherwise visible
  on the `Response`, so the agent calls `SetSuspended()` before
  `End(nil)`. Confirm this lines up with the semconv expectation.
- **Provider-level `BeforeGenerate` ctx propagation for `otelhttp`.**
  Today the chat-span ctx flows into the provider via
  `HookContext.UpdatedCtx`, which lets `otelhttp` middleware nest under
  chat. With the tracer interface, the agent can pass the chat-span ctx
  directly into `model.Generate(ctx, ...)`, and `otelhttp` works the
  same way without any hook involvement. No change needed to providers.

## Non-goals

- Replacing the hook system. Hooks stay; they remain the right answer
  for modification concerns.
- Changing the OTel semconv coverage. Same spans, same attributes,
  same metrics — just a different delivery mechanism.
- Designing a generic event bus. The `Tracer` interface is purpose-built
  for tracing; if other observation needs arise (audit logs, billing
  meters), they get their own purpose-built interfaces rather than a
  generic event system.
