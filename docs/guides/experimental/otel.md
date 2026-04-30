# OpenTelemetry Tracing

> **Experimental**: This package is in `experimental/otel/`. The API may change.

The `experimental/otel` package emits OpenTelemetry spans from a Dive agent
following the GenAI semantic conventions (`gen_ai.*`). Spans nest under a
single `invoke_agent` root so a destination like Jaeger, Phoenix, Datadog, or
Mobius can render the agent's LLM calls and tool calls as a tree.

## Span shape

```
invoke_agent {agent.name}        # one per CreateResponse call
├── chat {request.model}          # one per LLM iteration
├── execute_tool {tool.name}      # one per tool call
└── execute_tool {tool.name}
```

Each span carries `gen_ai.*` attributes (e.g. `gen_ai.system`,
`gen_ai.request.model`, `gen_ai.usage.input_tokens`,
`gen_ai.tool.name`, `gen_ai.tool.call.id`) so any backend that decodes the
OTel GenAI semconv reads them out of the box.

## Wiring

```go
import (
    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
    otelext "github.com/deepnoodle-ai/dive/experimental/otel"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// 1. Configure your TracerProvider (OTLP, Jaeger, stdout, …).
tp := sdktrace.NewTracerProvider(/* exporter */)
defer tp.Shutdown(ctx)
otel.SetTracerProvider(tp)

// 2. Build the Extension. Resource-style attributes here are added to every
//    span so a destination (e.g. Mobius) can correlate spans to a run/step.
ext := otelext.New(
    otelext.WithSystem("anthropic"),
    otelext.WithAttributes(
        attribute.String("mobius.run.id", runID),
        attribute.String("mobius.step.id", stepID),
    ),
)

// 3. Wire BOTH the Extension (agent + tool spans) and ext.LLMHooks() (chat
//    spans, fired at the provider layer).
agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:         "Research Assistant",
    Model:        anthropic.New(),
    Extensions:   []dive.Extension{ext},
    LLMHooks:     ext.LLMHooks(),
})

// 4. Use otelext.Run instead of agent.CreateResponse so chat / tool spans
//    nest under the invoke_agent root. (CreateResponse still works — the
//    Extension emits the agent span itself — but Dive does not let hooks
//    propagate ctx, so child spans wouldn't see the agent span as parent.)
resp, err := otelext.Run(ctx, agent, dive.WithInput("…"))
```

## Privacy

The opt-in flags below attach verbatim payloads — review your destination's
retention policy before turning them on.

| Option | Adds | Default |
|---|---|---|
| `WithCaptureMessages(true)` | `gen_ai.input.messages`, `gen_ai.output.messages`, `gen_ai.system_instructions` on chat spans | off |
| `WithCaptureToolIO(true)`   | `gen_ai.tool.call.arguments`, `gen_ai.tool.call.result` on execute_tool spans | off |

When off, the spans still carry timing, model, token usage, tool names, and
status — just not the raw text.

## Attributes emitted

| Span | Key attributes |
|---|---|
| `invoke_agent` | `gen_ai.system`, `gen_ai.operation.name=invoke_agent`, `gen_ai.agent.name`, `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens` |
| `chat` | `gen_ai.system`, `gen_ai.operation.name=chat`, `gen_ai.request.model`, `gen_ai.response.model`, `gen_ai.response.id`, `gen_ai.response.finish_reasons`, `gen_ai.usage.*`, optional `gen_ai.input.messages` / `gen_ai.output.messages` |
| `execute_tool` | `gen_ai.system`, `gen_ai.operation.name=execute_tool`, `gen_ai.tool.name`, `gen_ai.tool.call.id`, optional `gen_ai.tool.call.arguments` / `gen_ai.tool.call.result` |

Resource-style identifiers passed via `WithAttributes` (e.g. `mobius.run.id`)
are added to every span.

## Local development

The `examples/otel_example` program emits to stdout by default and to OTLP
when `OTEL_EXPORTER_OTLP_ENDPOINT` is set. Useful local backends:

- **Jaeger**: `docker run -p 4318:4318 -p 16686:16686 -e COLLECTOR_OTLP_ENABLED=true jaegertracing/all-in-one:latest` → http://localhost:16686
- **Phoenix** (best UI for agent traces): `docker run -p 6006:6006 -p 4318:4318 arizephoenix/phoenix:latest` → http://localhost:6006

## Nesting under the chat span

Spans your provider's HTTP client emits (e.g. `otelhttp` middleware on
`http.Client.Transport`) parent under the `chat` span, not the agent span.
This works because `BeforeGenerate` publishes the chat-span ctx via
`llm.HookContext.UpdatedCtx`, and each Dive provider picks it up for the
underlying HTTP/SDK request. Tool-internal spans nest under their
`execute_tool` span via the same mechanism on the agent-level
`HookContext.UpdatedCtx`. Custom providers wishing to support this should
honor `UpdatedCtx` after firing `BeforeGenerate`:

```go
beforeHook := &llm.HookContext{
    Type: llm.BeforeGenerate,
    Request: &llm.HookRequestContext{
        Messages: config.Messages,
        Config:   config,
        Body:     body,
    },
}
if err := config.FireHooks(ctx, beforeHook); err != nil {
    return nil, err
}
reqCtx := ctx
if beforeHook.UpdatedCtx != nil {
    reqCtx = beforeHook.UpdatedCtx
}
// use reqCtx for the HTTP/SDK call; keep using `ctx` for AfterGenerate
// so observers that pair Before/After by ctx identity still match.
```

## Limitations

- **Provider coverage:** `chat` spans are wired through `llm.Hooks`. All
  current Dive providers fire `BeforeGenerate` from both `Generate` and
  `Stream`; the agent itself fires `AfterGenerate` after streaming
  accumulation, so the contract is symmetric for both code paths. If a
  future provider doesn't fire `BeforeGenerate`, its calls will silently
  produce no chat spans.
