package otel

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// defaultTracer returns the global OTel tracer scoped to this package.
func defaultTracer() trace.Tracer {
	return otel.GetTracerProvider().Tracer(
		InstrumentationName,
		trace.WithInstrumentationVersion(InstrumentationVersion),
	)
}
