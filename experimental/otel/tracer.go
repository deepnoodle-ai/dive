package otel

import (
	"encoding/json"
	"log/slog"
	"runtime/debug"

	"github.com/deepnoodle-ai/dive"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/semconv/v1.40.0/genaiconv"
	"go.opentelemetry.io/otel/trace"
)

// InstrumentationName is the OTel instrumentation scope name set on every
// span and metric emitted by this package.
const InstrumentationName = "github.com/deepnoodle-ai/dive/experimental/otel"

// InstrumentationVersion is reported on every span and metric. Resolved from
// the build's module version when this package is consumed by a tagged
// release; falls back to instrumentationFallbackVersion for local builds.
var InstrumentationVersion = resolveInstrumentationVersion()

const instrumentationFallbackVersion = "0.0.0-dev"

// Options configures the Tracer.
type Options struct {
	// Tracer is the OTel tracer. Defaults to otel.GetTracerProvider().Tracer(...).
	Tracer trace.Tracer

	// Meter is the OTel meter for the gen_ai.client.* histograms. Defaults to
	// otel.GetMeterProvider().Meter(...). Pass a noop meter to disable
	// metrics emission while keeping spans.
	Meter metric.Meter

	// Provider sets gen_ai.provider.name on every span. Suggested values
	// come from genaiconv.ProviderName* — "anthropic", "openai",
	// "gcp.gemini", etc. When empty, "dive" is used.
	Provider string

	// CaptureMessages, when true, attaches gen_ai.input.messages and
	// gen_ai.output.messages on chat spans. Off by default — payloads can
	// contain user input verbatim.
	CaptureMessages bool

	// CaptureToolIO, when true, attaches gen_ai.tool.call.arguments and
	// gen_ai.tool.call.result on execute_tool spans. Off by default.
	CaptureToolIO bool

	// Attributes are added to every span this Tracer produces.
	Attributes []attribute.KeyValue
}

// Option mutates Options.
type Option func(*Options)

// WithTracer sets the OTel tracer.
func WithTracer(t trace.Tracer) Option { return func(o *Options) { o.Tracer = t } }

// WithMeter sets the OTel meter used for the gen_ai.client.* histograms.
func WithMeter(m metric.Meter) Option { return func(o *Options) { o.Meter = m } }

// WithProvider sets gen_ai.provider.name. Use values from
// genaiconv.ProviderName* where possible — "anthropic", "openai",
// "gcp.gemini", "aws.bedrock", etc.
func WithProvider(p string) Option { return func(o *Options) { o.Provider = p } }

// WithCaptureMessages enables capture of input/output messages on chat spans.
// OFF by default — payloads may contain user input.
func WithCaptureMessages(b bool) Option {
	return func(o *Options) { o.CaptureMessages = b }
}

// WithCaptureToolIO enables capture of tool arguments and results on
// execute_tool spans. OFF by default — payloads may contain user input.
func WithCaptureToolIO(b bool) Option {
	return func(o *Options) { o.CaptureToolIO = b }
}

// WithAttributes adds attributes to every emitted span.
func WithAttributes(a ...attribute.KeyValue) Option {
	return func(o *Options) { o.Attributes = append(o.Attributes, a...) }
}

// tracerImpl is a dive.Tracer that emits OpenTelemetry spans and metrics.
type tracerImpl struct {
	opts           Options
	durationMetric genaiconv.ClientOperationDuration
	tokenMetric    genaiconv.ClientTokenUsage
}

// NewTracer constructs a dive.Tracer that emits OpenTelemetry spans and
// metrics following the GenAI semantic conventions. Pass it via
// dive.AgentOptions.Tracer.
func NewTracer(opts ...Option) dive.Tracer {
	o := Options{Provider: "dive"}
	for _, opt := range opts {
		opt(&o)
	}
	if o.Tracer == nil {
		o.Tracer = otel.GetTracerProvider().Tracer(
			InstrumentationName,
			trace.WithInstrumentationVersion(InstrumentationVersion),
		)
	}
	if o.Meter == nil {
		o.Meter = otel.GetMeterProvider().Meter(
			InstrumentationName,
			metric.WithInstrumentationVersion(InstrumentationVersion),
		)
	}
	dur, err := genaiconv.NewClientOperationDuration(o.Meter,
		metric.WithExplicitBucketBoundaries(durationBucketsSeconds...),
	)
	if err != nil {
		slog.Default().Warn("otel: failed to create gen_ai.client.operation.duration histogram", "error", err)
	}
	tok, err := genaiconv.NewClientTokenUsage(o.Meter,
		metric.WithExplicitBucketBoundaries(tokenBuckets...),
	)
	if err != nil {
		slog.Default().Warn("otel: failed to create gen_ai.client.token.usage histogram", "error", err)
	}
	return &tracerImpl{opts: o, durationMetric: dur, tokenMetric: tok}
}

// Bucket boundaries from the OTel GenAI metric spec. Spec source:
// https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-metrics/
var (
	durationBucketsSeconds = []float64{
		0.01, 0.02, 0.04, 0.08, 0.16, 0.32, 0.64, 1.28,
		2.56, 5.12, 10.24, 20.48, 40.96, 81.92,
	}
	tokenBuckets = []float64{
		1, 4, 16, 64, 256, 1024, 4096, 16384,
		65536, 262144, 1048576, 4194304, 16777216, 67108864,
	}
)

// commonAttrs returns the attribute set applied to every span.
func (t *tracerImpl) commonAttrs() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(t.opts.Attributes)+1)
	attrs = append(attrs, semconv.GenAIProviderNameKey.String(t.opts.Provider))
	attrs = append(attrs, t.opts.Attributes...)
	return attrs
}

// agentIdentityAttrs returns gen_ai.agent.* for a Dive agent, skipping empty fields.
func agentIdentityAttrs(a *dive.Agent) []attribute.KeyValue {
	if a == nil {
		return nil
	}
	var attrs []attribute.KeyValue
	if name := a.Name(); name != "" {
		attrs = append(attrs, semconv.GenAIAgentName(name))
	}
	if id := a.ID(); id != "" {
		attrs = append(attrs, semconv.GenAIAgentID(id))
	}
	if desc := a.Description(); desc != "" {
		attrs = append(attrs, semconv.GenAIAgentDescription(desc))
	}
	if ver := a.Version(); ver != "" {
		attrs = append(attrs, semconv.GenAIAgentVersion(ver))
	}
	return attrs
}

// conversationAttrs returns gen_ai.conversation.id when a session is attached.
func conversationAttrs(sess dive.Session) []attribute.KeyValue {
	if sess == nil {
		return nil
	}
	id := sess.ID()
	if id == "" {
		return nil
	}
	return []attribute.KeyValue{semconv.GenAIConversationID(id)}
}

// resolveInstrumentationVersion returns this package's module version when
// available, falling back to instrumentationFallbackVersion for local builds.
func resolveInstrumentationVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return instrumentationFallbackVersion
	}
	for _, dep := range info.Deps {
		if dep == nil {
			continue
		}
		if dep.Path == InstrumentationName && dep.Version != "" && dep.Version != "(devel)" {
			return dep.Version
		}
	}
	if info.Main.Path == InstrumentationName && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return instrumentationFallbackVersion
}

// marshalJSON best-effort serialises v to a string. Returns "" on failure.
func marshalJSON(v any) string {
	if v == nil {
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}
