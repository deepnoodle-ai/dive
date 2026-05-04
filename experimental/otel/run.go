package otel

import (
	"context"

	"github.com/deepnoodle-ai/dive"

	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

// runMarker is a context-key sentinel set by Run so the PreGeneration hook
// knows the invoke_agent span was already opened by the caller and should
// not be duplicated.
type runMarkerKey struct{}

func contextHasAgentSpan(ctx context.Context) bool {
	return ctx.Value(runMarkerKey{}) != nil
}

// Run wraps agent.CreateResponse, opening an invoke_agent span on the
// caller's context BEFORE the agent runs so chat and execute_tool spans
// nest correctly. This package-level helper uses the global tracer
// provider; see Extension.Run for the variant that honors WithTracer /
// WithMeter on a configured Extension.
func Run(ctx context.Context, agent *dive.Agent, opts ...dive.CreateResponseOption) (*dive.Response, error) {
	return runWithTracer(ctx, defaultTracer(), agent, opts...)
}

// Run wraps agent.CreateResponse using this Extension's tracer/meter.
// Prefer Extension.Run over the package-level Run when you've configured
// WithTracer or WithMeter — those options would otherwise be ignored by
// the global helper.
func (e *Extension) Run(ctx context.Context, agent *dive.Agent, opts ...dive.CreateResponseOption) (*dive.Response, error) {
	return runWithTracer(ctx, e.opts.Tracer, agent, opts...)
}

// runWithTracer is the shared implementation for the package-level Run
// and Extension.Run. The tracer comes from the caller — callers without
// an Extension instance use the global provider; callers with one use
// e.opts.Tracer.
func runWithTracer(ctx context.Context, tr trace.Tracer, agent *dive.Agent, opts ...dive.CreateResponseOption) (*dive.Response, error) {
	name := agent.Name()
	if name == "" {
		name = "agent"
	}
	attrs := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			semconv.GenAIOperationNameInvokeAgent,
			semconv.GenAIAgentName(name),
		),
	}
	ctx, span := tr.Start(ctx, OperationInvokeAgent+" "+name, attrs...)
	defer span.End()
	ctx = context.WithValue(ctx, runMarkerKey{}, span)

	resp, err := agent.CreateResponse(ctx, opts...)
	if err != nil {
		span.RecordError(err)
		return resp, err
	}
	if resp != nil && resp.Usage != nil {
		span.SetAttributes(usageAttrs(resp.Usage)...)
	}
	return resp, nil
}
