# Tracing Guide

See what your agent is doing — every LLM call, every tool call, with timing
and token usage — using OpenTelemetry. Add three lines to wire it up.

## What you get

One trace per `CreateResponse` call, shaped like this:

```text
invoke_agent My Agent
├── chat claude-opus-4-6      (timing, tokens, model)
├── execute_tool get_weather  (args, result, status)
├── execute_tool get_weather
└── chat claude-opus-4-6
```

Each span carries the standard OpenTelemetry GenAI attributes
(`gen_ai.usage.input_tokens`, `gen_ai.tool.name`, etc.), so any backend
that speaks OTel — Jaeger, Phoenix, Datadog, Honeycomb, your own
collector — will render them.

## Quick start

```bash
go get github.com/deepnoodle-ai/dive/otel
```

```go
import (
    "github.com/deepnoodle-ai/dive"
    otelext "github.com/deepnoodle-ai/dive/otel"
)

agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:   "My Agent",
    Model:  anthropic.New(),
    Tracer: otelext.NewTracer(otelext.WithProvider("anthropic")),
})

resp, err := agent.CreateResponse(ctx, dive.WithInput("hello"))
```

That's it. Spans go to whatever OpenTelemetry `TracerProvider` you've
configured globally (`otel.SetTracerProvider(...)`).

## See it locally

Run Phoenix in Docker — it has the nicest UI for agent traces:

```bash
docker run -p 6006:6006 -p 4318:4318 arizephoenix/phoenix:latest
```

Then point your app at it:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 go run .
```

Open http://localhost:6006 and re-run your agent. Your trace appears in
the UI within a few seconds.

A complete working example with the boilerplate to set up a
`TracerProvider` lives in
[`examples/otel_example`](https://github.com/deepnoodle-ai/dive/tree/main/examples/otel_example).

## Privacy: capturing message contents

By default, traces include timing, models, token counts, tool names, and
status — but **not** the raw text of messages or tool arguments. Opt in
when you want it (review your destination's data retention first):

```go
otelext.NewTracer(
    otelext.WithProvider("anthropic"),
    otelext.WithCaptureMessages(true), // chat input/output messages
    otelext.WithCaptureToolIO(true),   // tool args and results
)
```

## Adding your own labels

Pass any extra attributes you want on every span — useful for tying
traces back to a workflow run, customer ID, etc.:

```go
otelext.NewTracer(
    otelext.WithProvider("anthropic"),
    otelext.WithAttributes(
        attribute.String("workflow.run.id", runID),
        attribute.String("customer.id", customerID),
    ),
)
```

## Going further

- The [dive/otel reference](otel.md) documents every attribute and metric
  the adapter emits, plus how to nest your own HTTP spans under `chat`
  using `otelhttp`.
- Want to emit to something other than OpenTelemetry? Implement
  `dive.Tracer` directly — it's three small methods. See `tracer.go` in
  the dive package.
