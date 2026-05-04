# OpenTelemetry Tracing

> **Experimental**: This package lives in `experimental/otel/`. The API may change.

The `experimental/otel` package emits OpenTelemetry spans and metrics from a
Dive agent following the GenAI semantic conventions (`gen_ai.*`). Spans nest
under a single `invoke_agent` root so a destination like Jaeger, Phoenix,
Datadog, etc. can render the agent's LLM calls and tool calls as a tree.

The package implements the `dive.Tracer` interface — set it on
`dive.AgentOptions.Tracer` and the agent does the rest.

## Span shape

```
invoke_agent {agent.name}        # one per CreateResponse call
├── chat {request.model}          # one per LLM iteration
├── execute_tool {tool.name}      # one per tool call
└── execute_tool {tool.name}
```

Each span carries `gen_ai.*` attributes (`gen_ai.provider.name`,
`gen_ai.request.model`, `gen_ai.usage.input_tokens`, `gen_ai.tool.name`,
`gen_ai.tool.call.id`, etc.) so any backend that decodes the OTel GenAI
semconv reads them out of the box.

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

// 2. Build the Tracer. Resource-style attributes here are added to every
//    span so a destination can correlate spans to a run/step.
tracer := otelext.NewTracer(
    otelext.WithProvider("anthropic"),
    otelext.WithAttributes(
        attribute.String("workflow.run.id", runID),
        attribute.String("workflow.step.id", stepID),
    ),
)

// 3. Set the Tracer on the agent.
agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:   "Research Assistant",
    Model:  anthropic.New(),
    Tracer: tracer,
})

// 4. Use CreateResponse normally — no special wrapper required.
resp, err := agent.CreateResponse(ctx, dive.WithInput("…"))
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
| `invoke_agent` | `gen_ai.provider.name`, `gen_ai.operation.name=invoke_agent`, `gen_ai.agent.name`, `gen_ai.agent.id`, `gen_ai.agent.description`, `gen_ai.agent.version`, `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens` |
| `chat` | `gen_ai.provider.name`, `gen_ai.operation.name=chat`, `gen_ai.request.model`, `gen_ai.response.model`, `gen_ai.response.id`, `gen_ai.response.finish_reasons`, `gen_ai.usage.*`, optional `gen_ai.input.messages` / `gen_ai.output.messages` |
| `execute_tool` | `gen_ai.provider.name`, `gen_ai.operation.name=execute_tool`, `gen_ai.tool.name`, `gen_ai.tool.call.id`, `gen_ai.tool.type`, `gen_ai.tool.description`, optional `gen_ai.tool.call.arguments` / `gen_ai.tool.call.result` |

Resource-style attributes passed via `WithAttributes` are added to every span.

## Metrics

The tracer also records the spec-defined GenAI client metrics:

| Metric | Dimensions |
|---|---|
| `gen_ai.client.operation.duration` (histogram, seconds) | `gen_ai.operation.name`, `gen_ai.provider.name`, optional model + `error.type` |
| `gen_ai.client.token.usage` (histogram, tokens) | `gen_ai.operation.name`, `gen_ai.provider.name`, `gen_ai.token.type`, optional model |

Bucket boundaries follow the OTel GenAI metric spec — wider than SDK defaults
because LLM call durations and token counts span many orders of magnitude.

## Local development

The `examples/otel_example` program emits to stderr by default and to OTLP
when `OTEL_EXPORTER_OTLP_ENDPOINT` is set. Useful local backends:

- **Jaeger**: `docker run -p 4318:4318 -p 16686:16686 -e COLLECTOR_OTLP_ENABLED=true jaegertracing/all-in-one:latest` → http://localhost:16686
- **Phoenix** (best UI for agent traces): `docker run -p 6006:6006 -p 4318:4318 arizephoenix/phoenix:latest` → http://localhost:6006

## Nesting under the chat span

Spans your provider's HTTP client emits (e.g. `otelhttp` middleware on
`http.Client.Transport`) parent under the `chat` span automatically. The
agent passes the chat-span ctx directly into `LLM.Generate` / `LLM.Stream`,
so the provider's HTTP request inherits it without any hook involvement —
just install `otelhttp` on the client you pass to the provider:

```go
httpClient := &http.Client{
    Transport: otelhttp.NewTransport(http.DefaultTransport),
}
agent, _ := dive.NewAgent(dive.AgentOptions{
    Model:  anthropic.New(anthropic.WithClient(httpClient)),
    Tracer: otelext.NewTracer(otelext.WithProvider("anthropic")),
})
```

Tool-internal spans nest under their `execute_tool` span via the same
mechanism — the agent passes the tool-span ctx into `Tool.Call`.

## Implementing a different tracer

`dive.Tracer` is just three methods (`StartAgentRun`, `StartChat`,
`StartToolCall`), each returning a span object with a small surface (`End`,
plus a couple of setters). Implement the interface in your own package to
emit to a non-OTel system, or use `dive.MultiTracer(t1, t2, ...)` to fan
out to multiple tracers at once.
