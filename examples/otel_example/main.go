// otel_example demonstrates emitting OpenTelemetry spans from a Dive agent.
//
// Spans follow the GenAI semantic conventions (gen_ai.*) and nest under a
// single invoke_agent root, so a destination like Mobius / Phoenix / Datadog
// can render the agent's LLM calls and tool calls as a tree.
//
// Two exporters are supported:
//
//   - stdout (default): prints spans to stderr in human-readable form. Good
//     for local development.
//   - OTLP/HTTP: set the standard env var OTEL_EXPORTER_OTLP_ENDPOINT
//     (e.g. http://localhost:4318) to push spans to a real collector.
//     Try `docker run -p 4318:4318 -p 16686:16686 -e COLLECTOR_OTLP_ENABLED=true
//     jaegertracing/all-in-one:latest` then open http://localhost:16686.
//
// Usage:
//
//	cd examples
//	ANTHROPIC_API_KEY=... go run ./otel_example
//	OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
//	  ANTHROPIC_API_KEY=... go run ./otel_example
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/deepnoodle-ai/dive"
	otelext "github.com/deepnoodle-ai/dive/experimental/otel"
	"github.com/deepnoodle-ai/dive/providers/anthropic"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

func main() {
	ctx := context.Background()

	tp, shutdown, err := setupTracerProvider(ctx)
	if err != nil {
		log.Fatalf("setup tracer: %v", err)
	}
	defer shutdown(ctx)
	otel.SetTracerProvider(tp)

	// A toy "weather" tool so the agent has something to call.
	type weatherIn struct {
		City string `json:"city" description:"city name"`
	}
	weather := dive.FuncTool("get_weather", "Look up the current weather in a city",
		func(ctx context.Context, in *weatherIn) (*dive.ToolResult, error) {
			return dive.NewToolResultText(fmt.Sprintf("It is 72°F and sunny in %s.", in.City)), nil
		},
	)

	ext := otelext.New(
		otelext.WithSystem("anthropic"),
		// Capture flags are off by default for privacy. Turn on for local
		// debugging — the spans will then carry verbatim message and tool
		// payloads.
		otelext.WithCaptureMessages(true),
		otelext.WithCaptureToolIO(true),
		// Resource-style identifiers any backend can correlate on. Mobius
		// uses these to attach spans to the right run/step.
		otelext.WithAttributes(
			attribute.String("mobius.run.id", "demo_run_1"),
			attribute.String("mobius.step.id", "demo_step_1"),
		),
	)

	// Wire otelhttp on the provider's HTTP client. Because Dive's
	// BeforeGenerate hook publishes the chat-span ctx via
	// HookContext.UpdatedCtx, the provider uses that ctx for the request,
	// and otelhttp's HTTP CLIENT span will nest under the chat span.
	httpClient := &http.Client{
		Timeout:   300 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Weather Agent",
		SystemPrompt: "You are a meteorologist. When asked about weather, use the get_weather tool.",
		Model:        anthropic.New(anthropic.WithClient(httpClient)),
		Tools:        []dive.Tool{weather},
		Extensions:   []dive.Extension{ext},
		LLMHooks:     ext.LLMHooks(),
	})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := otelext.Run(ctx, agent,
		dive.WithInput("What's the weather in San Francisco and Paris?"),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.OutputText())
}

func setupTracerProvider(ctx context.Context) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		semconv.ServiceName("dive-otel-example"),
		semconv.ServiceVersion("0.1.0"),
	))
	if err != nil {
		return nil, nil, err
	}

	var exporter sdktrace.SpanExporter
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		// OTLP/HTTP — endpoint defaults to https; force insecure for local
		// jaeger / signoz.
		opts := []otlptracehttp.Option{}
		if !isHTTPS(endpoint) {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		exporter, err = otlptracehttp.New(ctx, opts...)
	} else {
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
	if err != nil {
		return nil, nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	return tp, tp.Shutdown, nil
}

func isHTTPS(endpoint string) bool {
	return len(endpoint) >= 8 && endpoint[:8] == "https://"
}
