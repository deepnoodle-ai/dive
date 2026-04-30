package otel

import (
	"context"

	"github.com/deepnoodle-ai/dive"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// runMarker is a context-key sentinel set by Run so the PreGeneration hook
// knows the invoke_agent span was already opened by the caller and should
// not be duplicated.
type runMarkerKey struct{}

func contextHasAgentSpan(ctx context.Context) bool {
	return ctx.Value(runMarkerKey{}) != nil
}

// Run wraps agent.CreateResponse, opening an invoke_agent span on the caller's
// context BEFORE the agent runs so chat and execute_tool spans nest correctly.
//
// Without this wrapper, the Extension's PreGeneration hook still emits an
// invoke_agent span — but Dive does not allow hooks to mutate the context
// downstream operations see, so chat / execute_tool spans wouldn't be
// children of that span. Use Run to get the full hierarchy.
//
// If you need finer control (custom span name, attributes, links, baggage),
// open the span yourself with WithSpan instead.
func Run(ctx context.Context, agent *dive.Agent, opts ...dive.CreateResponseOption) (*dive.Response, error) {
	tr := tracerOf(agent)
	name := agent.Name()
	if name == "" {
		name = "agent"
	}
	ctx, span := tr.Start(ctx, OperationInvokeAgent+" "+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String(AttrGenAIOperationName, OperationInvokeAgent),
			attribute.String(AttrGenAIAgentName, name),
		),
	)
	defer span.End()
	ctx = context.WithValue(ctx, runMarkerKey{}, span)

	resp, err := agent.CreateResponse(ctx, opts...)
	if err != nil {
		span.RecordError(err)
		return resp, err
	}
	if resp != nil && resp.Usage != nil {
		span.SetAttributes(
			attribute.Int(AttrGenAIUsageInputTokens, int(resp.Usage.InputTokens)),
			attribute.Int(AttrGenAIUsageOutputTokens, int(resp.Usage.OutputTokens)),
		)
	}
	return resp, nil
}

// tracerOf returns the package-default tracer. The agent argument is reserved
// for a future enhancement (per-agent tracer override via an option) but is
// unused for now.
func tracerOf(_ *dive.Agent) trace.Tracer {
	return defaultTracer()
}
