// otel_example demonstrates emitting OpenTelemetry spans AND metrics from a
// Dive agent.
//
// Spans follow the GenAI semantic conventions (gen_ai.*) and nest under a
// single invoke_agent root, so a destination like Mobius / Phoenix / Datadog
// can render the agent's LLM calls and tool calls as a tree. Metrics
// (gen_ai.client.operation.duration, gen_ai.client.token.usage) follow the
// matching metric spec, with the spec-recommended bucket boundaries.
//
// Two exporters are supported:
//
//   - stdout (default): prints spans and metrics to stderr in human-readable
//     form. Good for local development.
//   - OTLP/HTTP: set OTEL_EXPORTER_OTLP_ENDPOINT (e.g. http://localhost:4318)
//     to push both signals to a real collector. The repo includes a
//     docker-compose at examples/otel_example/docker-compose.yaml that
//     runs the OTel collector with the `debug` exporter — use it to see
//     the raw OTLP payload Dive produces.
//
// Usage:
//
//	cd examples
//	ANTHROPIC_API_KEY=... go run ./otel_example
//	# or, with the bundled collector:
//	(cd otel_example && docker compose up -d)
//	OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
//	  ANTHROPIC_API_KEY=... go run ./otel_example
//	docker compose -f otel_example/docker-compose.yaml logs -f otel-collector
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
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

func main() {
	ctx := context.Background()

	tp, mp, shutdown, err := setupProviders(ctx)
	if err != nil {
		log.Fatalf("setup providers: %v", err)
	}
	defer shutdown(ctx)
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)

	// A toy "weather" tool so the agent has something to call.
	type weatherIn struct {
		City string `json:"city" description:"city name"`
	}
	weather := dive.FuncTool("get_weather", "Look up the current weather in a city",
		func(ctx context.Context, in *weatherIn) (*dive.ToolResult, error) {
			return dive.NewToolResultText(fmt.Sprintf("It is 72°F and sunny in %s.", in.City)), nil
		},
	)

	tracer := otelext.NewTracer(
		otelext.WithProvider("anthropic"),
		// Capture flags are off by default for privacy. Turn on for local
		// debugging — the spans will then carry verbatim message and tool
		// payloads.
		otelext.WithCaptureMessages(true),
		otelext.WithCaptureToolIO(true),
		// Arbitrary resource-style attributes any backend can correlate on.
		otelext.WithAttributes(
			attribute.String("demo.run.id", "demo_run_1"),
			attribute.String("demo.step.id", "demo_step_1"),
		),
	)

	// Wire otelhttp on the provider's HTTP client. Because the agent passes
	// the chat-span ctx directly into the model's Generate/Stream call, the
	// provider's HTTP request inherits that ctx, and otelhttp's CLIENT span
	// nests under the chat span automatically — no hook plumbing needed.
	httpClient := &http.Client{
		Timeout:   300 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Weather Agent",
		Description:  "Reports current weather conditions for a given city",
		Version:      "0.1.0",
		SystemPrompt: "You are a meteorologist. When asked about weather, use the get_weather tool.",
		Model:        anthropic.New(anthropic.WithClient(httpClient)),
		Tools:        []dive.Tool{weather},
		Tracer:       tracer,
	})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := agent.CreateResponse(ctx,
		dive.WithInput("What's the weather in San Francisco and Paris?"),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.OutputText())
}

// setupProviders wires both a TracerProvider and a MeterProvider, returning
// a single shutdown that drains both. When OTEL_EXPORTER_OTLP_ENDPOINT is
// set, both signals go to the OTLP/HTTP collector at that endpoint;
// otherwise they print to stderr.
func setupProviders(ctx context.Context) (*sdktrace.TracerProvider, *metric.MeterProvider, func(context.Context) error, error) {
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		semconv.ServiceName("dive-otel-example"),
		semconv.ServiceVersion("0.1.0"),
	))
	if err != nil {
		return nil, nil, nil, err
	}

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	insecure := endpoint != "" && !isHTTPS(endpoint)

	var traceExporter sdktrace.SpanExporter
	var metricExporter metric.Exporter
	if endpoint != "" {
		traceOpts := []otlptracehttp.Option{}
		metricOpts := []otlpmetrichttp.Option{}
		if insecure {
			traceOpts = append(traceOpts, otlptracehttp.WithInsecure())
			metricOpts = append(metricOpts, otlpmetrichttp.WithInsecure())
		}
		traceExporter, err = otlptracehttp.New(ctx, traceOpts...)
		if err != nil {
			return nil, nil, nil, err
		}
		metricExporter, err = otlpmetrichttp.New(ctx, metricOpts...)
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		traceExporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, nil, nil, err
		}
		metricExporter, err = stdoutmetric.New()
		if err != nil {
			return nil, nil, nil, err
		}
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	// PeriodicReader with a short interval so a single short-lived demo
	// flushes its histograms before the process exits.
	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			metric.WithInterval(2*time.Second),
		)),
	)

	shutdown := func(ctx context.Context) error {
		// Force one final metric flush so the demo's single chat call
		// surfaces in the output, then shut both providers down.
		if err := mp.ForceFlush(ctx); err != nil {
			return err
		}
		if err := tp.Shutdown(ctx); err != nil {
			return err
		}
		return mp.Shutdown(ctx)
	}
	return tp, mp, shutdown, nil
}

func isHTTPS(endpoint string) bool {
	return len(endpoint) >= 8 && endpoint[:8] == "https://"
}
