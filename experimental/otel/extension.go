package otel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/semconv/v1.40.0/genaiconv"
	"go.opentelemetry.io/otel/trace"
)

// InstrumentationName is the OTel instrumentation scope name set on every
// span emitted by this package.
const InstrumentationName = "github.com/deepnoodle-ai/dive/experimental/otel"

// InstrumentationVersion is reported on every span and metric emitted by
// the extension. Resolved from the build's module version when the
// package is consumed by a tagged release; falls back to instrumentationFallbackVersion
// when the build doesn't expose one (e.g. local development with a
// replace directive).
var InstrumentationVersion = resolveInstrumentationVersion()

// instrumentationFallbackVersion is the value reported when
// runtime/debug.ReadBuildInfo can't resolve a tagged version for this
// module — most often during local development.
const instrumentationFallbackVersion = "0.1.0"

// Operation name constants. These mirror the canonical
// genaiconv.OperationName* values and are exported so callers can build span
// names without importing genaiconv themselves.
var (
	OperationChat        = string(genaiconv.OperationNameChat)
	OperationExecuteTool = string(genaiconv.OperationNameExecuteTool)
	OperationInvokeAgent = string(genaiconv.OperationNameInvokeAgent)
)

// Options configures the Extension.
type Options struct {
	// Tracer to use. Defaults to otel.GetTracerProvider().Tracer(...).
	Tracer trace.Tracer

	// Meter to use for the gen_ai.client.* histograms. Defaults to
	// otel.GetMeterProvider().Meter(...). Pass a noop meter to disable
	// metrics emission while keeping spans.
	Meter metric.Meter

	// Provider sets gen_ai.provider.name (and the legacy gen_ai.system) on
	// every span. Suggested values come from genaiconv.ProviderName* —
	// "anthropic", "openai", "gcp.gemini", etc. When empty, "dive" is used.
	Provider string

	// CaptureMessages, when true, attaches gen_ai.input.messages and
	// gen_ai.output.messages on chat spans. Off by default — payloads can
	// contain user input verbatim.
	CaptureMessages bool

	// CaptureToolIO, when true, attaches gen_ai.tool.call.arguments and
	// gen_ai.tool.call.result on execute_tool spans. Off by default.
	CaptureToolIO bool

	// Attributes are added to every span this Extension produces. Useful for
	// resource-style identifiers (for example AttrMobiusRunID and
	// AttrMobiusStepID) that the destination uses for correlation.
	Attributes []attribute.KeyValue
}

// Option mutates Options.
type Option func(*Options)

// WithTracer sets the OTel tracer.
func WithTracer(t trace.Tracer) Option { return func(o *Options) { o.Tracer = t } }

// WithMeter sets the OTel meter used for the gen_ai.client.* histograms.
// Defaults to otel.GetMeterProvider().Meter(InstrumentationName, ...).
func WithMeter(m metric.Meter) Option { return func(o *Options) { o.Meter = m } }

// WithProvider sets gen_ai.provider.name (and legacy gen_ai.system). Use
// values from genaiconv.ProviderName* where possible — "anthropic", "openai",
// "gcp.gemini", "aws.bedrock", etc.
func WithProvider(p string) Option { return func(o *Options) { o.Provider = p } }

// WithSystem is a deprecated alias for WithProvider, kept so existing call
// sites compile during the rename window.
//
// Deprecated: use WithProvider. The OTel GenAI spec migrated gen_ai.system to
// gen_ai.provider.name; the extension emits both keys during the migration.
func WithSystem(s string) Option { return WithProvider(s) }

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

// WithAttributes adds attributes to every emitted span. Useful for
// correlation keys like AttrMobiusRunID / AttrMobiusStepID.
func WithAttributes(a ...attribute.KeyValue) Option {
	return func(o *Options) { o.Attributes = append(o.Attributes, a...) }
}

// Extension is a dive.Extension that emits OpenTelemetry spans for an
// agent's CreateResponse call.
type Extension struct {
	opts Options

	// chatSpans pairs BeforeGenerate / AfterGenerate hook firings.
	//
	// We can't key on *llm.Config because streaming providers fire Before
	// from their Stream method (with a provider-internal Config) while the
	// agent fires the matching After with a synthetic Config. We CAN key on
	// the calling ctx — the agent passes the same ctx into both, and
	// ctx values are unique per Generate call. Push on Before, dequeue
	// FIFO on After.
	chatSpans sync.Map // ctx → *chatQueue

	// chatStarts records the start time of each in-flight chat span so
	// afterGenerate can record gen_ai.client.operation.duration. Same
	// keying / FIFO semantics as chatSpans.
	chatStarts sync.Map // ctx → *startQueue

	// Pre-built metric instruments. Created in New so each Record call is
	// a cheap histogram write.
	durationMetric genaiconv.ClientOperationDuration
	tokenMetric    genaiconv.ClientTokenUsage
}

// startQueue mirrors chatQueue but stores wall-clock start times.
type startQueue struct {
	mu     sync.Mutex
	starts []time.Time
}

func (q *startQueue) push(t time.Time) {
	q.mu.Lock()
	q.starts = append(q.starts, t)
	q.mu.Unlock()
}

func (q *startQueue) pop() (time.Time, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.starts) == 0 {
		return time.Time{}, false
	}
	t := q.starts[0]
	q.starts = q.starts[1:]
	return t, true
}

// chatQueue holds the in-flight chat spans for a single Generate-invocation
// ctx. Mutex-guarded for safety even though the typical caller is
// single-goroutine.
type chatQueue struct {
	mu    sync.Mutex
	spans []trace.Span
}

func (q *chatQueue) push(s trace.Span) {
	q.mu.Lock()
	q.spans = append(q.spans, s)
	q.mu.Unlock()
}

func (q *chatQueue) pop() trace.Span {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.spans) == 0 {
		return nil
	}
	s := q.spans[0]
	q.spans = q.spans[1:]
	return s
}

// New constructs an Extension with the given options.
func New(opts ...Option) *Extension {
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
	// genaiconv.New* return noop instruments and a nil error when the
	// meter is nil; pass through any non-nil errors as a slog warning so
	// misconfiguration surfaces without crashing the agent.
	//
	// Bucket boundaries follow the OTel GenAI metric spec — wider than the
	// SDK defaults because LLM calls range from sub-second to multi-minute,
	// and token counts span from a handful to millions.
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
	return &Extension{opts: o, durationMetric: dur, tokenMetric: tok}
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

// Tools returns nil — this extension only adds hooks.
func (e *Extension) Tools() []dive.Tool { return nil }

// Rules returns "" — this extension does not augment the system prompt.
func (e *Extension) Rules() string { return "" }

// Hooks returns the agent-level hooks that emit invoke_agent and execute_tool
// spans.
func (e *Extension) Hooks() dive.Hooks {
	return dive.Hooks{
		PreGeneration:      []dive.PreGenerationHook{e.preGeneration},
		PostGeneration:     []dive.PostGenerationHook{e.postGeneration},
		PreToolUse:         []dive.PreToolUseHook{e.preToolUse},
		PostToolUse:        []dive.PostToolUseHook{e.postToolUse},
		PostToolUseFailure: []dive.PostToolUseFailureHook{e.postToolUseFailure},
		OnSuspend:          []dive.OnSuspendHook{e.onSuspend},
	}
}

// LLMHooks returns the provider-level hooks that emit chat spans. NewAgent
// wires these automatically when the extension is passed via AgentOptions.Extensions.
func (e *Extension) LLMHooks() llm.Hooks {
	return llm.Hooks{
		{Type: llm.BeforeGenerate, Func: e.beforeGenerate},
		{Type: llm.AfterGenerate, Func: e.afterGenerate},
		{Type: llm.OnError, Func: e.onError},
	}
}

// hctx.Values keys. Prefixed with a sentinel to avoid collisions with
// user/extension keys.
const (
	keyAgentSpan      = "_dive_otel.agent.span"
	keyToolSpanPrefix = "_dive_otel.tool.span."
)

// commonAttrs returns the attribute set applied to every span. Emits both
// gen_ai.provider.name (current spec) and gen_ai.system (legacy) so backends
// keyed on either name continue to work during the spec migration window.
func (e *Extension) commonAttrs() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(e.opts.Attributes)+2)
	attrs = append(attrs,
		semconv.GenAIProviderNameKey.String(e.opts.Provider),
		attribute.String(attrGenAISystem, e.opts.Provider),
	)
	attrs = append(attrs, e.opts.Attributes...)
	return attrs
}

// preGeneration starts the invoke_agent span and stashes it on hctx.Values
// for postGeneration to retrieve. If Run already opened an invoke_agent
// span on ctx, this hook attributes the existing span and does not start
// a duplicate. In that case postGeneration is a no-op.
func (e *Extension) preGeneration(ctx context.Context, hctx *dive.HookContext) error {
	name := hctx.Agent.Name()
	if name == "" {
		name = "agent"
	}
	identity := agentIdentityAttrs(hctx.Agent)
	conv := conversationAttrs(hctx.Session)
	if contextHasAgentSpan(ctx) {
		// Run owns lifecycle; just attach common attrs to the active span.
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(e.commonAttrs()...)
		span.SetAttributes(identity...)
		span.SetAttributes(conv...)
		return nil
	}
	attrs := append(e.commonAttrs(), semconv.GenAIOperationNameInvokeAgent)
	attrs = append(attrs, identity...)
	attrs = append(attrs, conv...)
	_, span := e.opts.Tracer.Start(ctx, OperationInvokeAgent+" "+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
	hctx.Values[keyAgentSpan] = span
	return nil
}

// postGeneration ends the invoke_agent span, attaching usage and any error.
func (e *Extension) postGeneration(ctx context.Context, hctx *dive.HookContext) error {
	span := loadSpan(hctx.Values, keyAgentSpan)
	if span == nil {
		return nil
	}
	defer span.End()
	delete(hctx.Values, keyAgentSpan)

	if hctx.Usage != nil {
		span.SetAttributes(usageAttrs(hctx.Usage)...)
	}
	if hctx.Response != nil && hctx.Response.Status == dive.ResponseStatusSuspended {
		span.SetAttributes(attribute.String("dive.response.status", "suspended"))
	}
	return nil
}

// onSuspend annotates the agent span as suspended. PostGeneration still runs
// after this and ends the span.
func (e *Extension) onSuspend(ctx context.Context, hctx *dive.HookContext) error {
	span := loadSpan(hctx.Values, keyAgentSpan)
	if span == nil {
		return nil
	}
	span.AddEvent("agent.suspended")
	return nil
}

// preToolUse starts the execute_tool span and stashes it under a per-call
// key on hctx.Values. It also publishes the new ctx (which carries the
// span) via hctx.UpdatedCtx so the agent passes it into Tool.Call. Any
// spans the tool emits internally (HTTP, DB, retrieval, …) then nest under
// this execute_tool span.
func (e *Extension) preToolUse(ctx context.Context, hctx *dive.HookContext) error {
	if hctx.Call == nil {
		return nil
	}
	toolName := hctx.Call.Name
	attrs := append(e.commonAttrs(),
		semconv.GenAIOperationNameExecuteTool,
		semconv.GenAIToolName(toolName),
		semconv.GenAIToolCallID(hctx.Call.ID),
		// Spec values are "function", "extension", "datastore". Dive's
		// standard tools execute as in-process functions.
		semconv.GenAIToolType("function"),
	)
	if hctx.Tool != nil {
		if desc := hctx.Tool.Description(); desc != "" {
			attrs = append(attrs, semconv.GenAIToolDescription(desc))
		}
	}
	attrs = append(attrs, conversationAttrs(hctx.Session)...)
	if e.opts.CaptureToolIO && len(hctx.Call.Input) > 0 {
		attrs = append(attrs, semconv.GenAIToolCallArgumentsKey.String(string(hctx.Call.Input)))
	}
	toolCtx, span := e.opts.Tracer.Start(ctx, OperationExecuteTool+" "+toolName,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
	hctx.Values[keyToolSpanPrefix+hctx.Call.ID] = span
	hctx.UpdatedCtx = toolCtx
	return nil
}

// postToolUse ends a successful execute_tool span.
func (e *Extension) postToolUse(ctx context.Context, hctx *dive.HookContext) error {
	return e.endToolSpan(hctx, false)
}

// postToolUseFailure ends a failed execute_tool span and marks status=Error.
func (e *Extension) postToolUseFailure(ctx context.Context, hctx *dive.HookContext) error {
	return e.endToolSpan(hctx, true)
}

func (e *Extension) endToolSpan(hctx *dive.HookContext, failed bool) error {
	if hctx.Call == nil {
		return nil
	}
	key := keyToolSpanPrefix + hctx.Call.ID
	span := loadSpan(hctx.Values, key)
	if span == nil {
		return nil
	}
	defer span.End()
	delete(hctx.Values, key)

	if e.opts.CaptureToolIO && hctx.Result != nil && hctx.Result.Result != nil {
		// Marshal the protocol-level content. Best-effort; large payloads are
		// the user's responsibility to gate via WithCaptureToolIO.
		if data, err := json.Marshal(hctx.Result.Result.Content); err == nil {
			span.SetAttributes(semconv.GenAIToolCallResultKey.String(string(data)))
		}
	}

	if failed {
		errType := classifyToolError(hctx.Result)
		span.SetStatus(codes.Error, toolErrorMessage(hctx.Result))
		span.SetAttributes(semconv.ErrorTypeKey.String(errType))
		if hctx.Result != nil && hctx.Result.Error != nil {
			span.RecordError(hctx.Result.Error)
			span.AddEvent("gen_ai.client.operation.exception", trace.WithAttributes(
				semconv.ExceptionType(exceptionTypeOf(hctx.Result.Error)),
				semconv.ExceptionMessage(hctx.Result.Error.Error()),
			))
		} else {
			span.AddEvent("gen_ai.client.operation.exception", trace.WithAttributes(
				semconv.ExceptionMessage(toolErrorMessage(hctx.Result)),
			))
		}
	}
	return nil
}

// beforeGenerate starts a chat span and pairs it with the *llm.Config.
func (e *Extension) beforeGenerate(ctx context.Context, hctx *llm.HookContext) error {
	if hctx.Request == nil || hctx.Request.Config == nil {
		return nil
	}
	cfg := hctx.Request.Config
	attrs := append(e.commonAttrs(), semconv.GenAIOperationNameChat)
	if cfg.Model != "" {
		attrs = append(attrs, semconv.GenAIRequestModel(cfg.Model))
	}
	if cfg.MaxTokens != nil {
		attrs = append(attrs, semconv.GenAIRequestMaxTokens(*cfg.MaxTokens))
	}
	if cfg.Temperature != nil {
		attrs = append(attrs, semconv.GenAIRequestTemperature(*cfg.Temperature))
	}
	if cfg.FrequencyPenalty != nil {
		attrs = append(attrs, semconv.GenAIRequestFrequencyPenalty(*cfg.FrequencyPenalty))
	}
	if cfg.PresencePenalty != nil {
		attrs = append(attrs, semconv.GenAIRequestPresencePenalty(*cfg.PresencePenalty))
	}
	if hctx.Request.Streaming {
		attrs = append(attrs, attribute.Bool("gen_ai.request.stream", true))
	}
	attrs = append(attrs, serverAttrs(hctx.Request.Endpoint)...)
	attrs = append(attrs, conversationAttrs(dive.SessionFromContext(ctx))...)
	if e.opts.CaptureMessages {
		if data, err := json.Marshal(cfg.Messages); err == nil {
			attrs = append(attrs, semconv.GenAIInputMessagesKey.String(string(data)))
		}
		if cfg.SystemPrompt != "" {
			attrs = append(attrs, semconv.GenAISystemInstructionsKey.String(cfg.SystemPrompt))
		}
	}
	spanName := OperationChat
	if cfg.Model != "" {
		spanName = OperationChat + " " + cfg.Model
	}
	chatCtx, span := e.opts.Tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
	e.enqueueChatSpan(ctx, span)
	e.enqueueChatStart(ctx, time.Now())
	// Publish the chat-span ctx so the provider uses it for the underlying
	// HTTP/SDK request. HTTP-client middleware (e.g. otelhttp) will then
	// nest its spans under this chat span. The provider continues to use
	// the original ctx for AfterGenerate, so our queue lookup still
	// matches.
	hctx.UpdatedCtx = chatCtx
	return nil
}

// enqueueChatSpan pushes a span onto the queue for this ctx, creating the
// queue if needed.
func (e *Extension) enqueueChatSpan(ctx context.Context, span trace.Span) {
	v, _ := e.chatSpans.LoadOrStore(ctx, &chatQueue{})
	q := v.(*chatQueue)
	q.push(span)
}

// dequeueChatSpan pops the oldest span for this ctx, removing the queue
// entry once it goes empty.
func (e *Extension) dequeueChatSpan(ctx context.Context) trace.Span {
	v, ok := e.chatSpans.Load(ctx)
	if !ok {
		return nil
	}
	q := v.(*chatQueue)
	span := q.pop()
	q.mu.Lock()
	empty := len(q.spans) == 0
	q.mu.Unlock()
	if empty {
		e.chatSpans.Delete(ctx)
	}
	return span
}

// enqueueChatStart parallels enqueueChatSpan for wall-clock start times.
func (e *Extension) enqueueChatStart(ctx context.Context, t time.Time) {
	v, _ := e.chatStarts.LoadOrStore(ctx, &startQueue{})
	q := v.(*startQueue)
	q.push(t)
}

// dequeueChatStart pops the oldest start time for this ctx.
func (e *Extension) dequeueChatStart(ctx context.Context) (time.Time, bool) {
	v, ok := e.chatStarts.Load(ctx)
	if !ok {
		return time.Time{}, false
	}
	q := v.(*startQueue)
	t, ok := q.pop()
	q.mu.Lock()
	empty := len(q.starts) == 0
	q.mu.Unlock()
	if empty {
		e.chatStarts.Delete(ctx)
	}
	return t, ok
}

// afterGenerate ends the chat span and attaches response attributes.
func (e *Extension) afterGenerate(ctx context.Context, hctx *llm.HookContext) error {
	span := e.dequeueChatSpan(ctx)
	start, hasStart := e.dequeueChatStart(ctx)
	if span == nil {
		// AfterGenerate fired without a matching BeforeGenerate ctx in the
		// queue. The most likely cause is a provider that uses the
		// post-UpdatedCtx ctx for AfterGenerate instead of the original.
		// See HookContext.UpdatedCtx contract in llm/hooks.go.
		slog.Default().DebugContext(ctx, "otel: chat span queue miss on AfterGenerate — provider may be violating HookContext.UpdatedCtx contract")
		return nil
	}
	defer span.End()

	if hctx.Response == nil {
		if hasStart {
			e.recordDuration(ctx, start, hctx, nil, "")
		}
		return nil
	}
	if hctx.Response.Error != nil {
		errType := classifyChatError(hctx.Response.Error)
		recordChatError(span, hctx.Response.Error, errType)
		if hasStart {
			e.recordDuration(ctx, start, hctx, nil, errType)
		}
		return nil
	}
	resp := hctx.Response.Response
	if resp == nil {
		if hasStart {
			e.recordDuration(ctx, start, hctx, nil, "")
		}
		return nil
	}

	attrs := []attribute.KeyValue{
		semconv.GenAIResponseModel(resp.Model),
		semconv.GenAIResponseID(resp.ID),
	}
	attrs = append(attrs, usageAttrs(&resp.Usage)...)
	span.SetAttributes(attrs...)

	// If the provider didn't echo a request.model on cfg, fall back to the
	// response model so the span carries something useful.
	if hctx.Request != nil && hctx.Request.Config != nil && hctx.Request.Config.Model == "" && resp.Model != "" {
		span.SetAttributes(semconv.GenAIRequestModel(resp.Model))
	}
	if resp.StopReason != "" {
		span.SetAttributes(semconv.GenAIResponseFinishReasons(resp.StopReason))
	}
	if hctx.Response.TimeToFirstChunk > 0 {
		span.SetAttributes(attribute.Float64("gen_ai.response.time_to_first_chunk", hctx.Response.TimeToFirstChunk))
	}
	if e.opts.CaptureMessages {
		if msg := resp.Message(); msg != nil {
			if data, err := json.Marshal(msg); err == nil {
				span.SetAttributes(semconv.GenAIOutputMessagesKey.String(string(data)))
			}
		}
	}
	if hasStart {
		e.recordDuration(ctx, start, hctx, resp, "")
	}
	e.recordTokenUsage(ctx, hctx, resp)
	return nil
}

// onError ends an in-flight chat span as Error if AfterGenerate doesn't fire.
// Many providers fire AfterGenerate even on error, so this is best-effort.
func (e *Extension) onError(ctx context.Context, hctx *llm.HookContext) error {
	span := e.dequeueChatSpan(ctx)
	start, hasStart := e.dequeueChatStart(ctx)
	if span == nil {
		// Either the chat span was already ended by AfterGenerate, or the
		// provider used the post-UpdatedCtx ctx (contract violation).
		slog.Default().DebugContext(ctx, "otel: chat span queue miss on OnError")
		return nil
	}
	defer span.End()
	var errType string
	if hctx.Response != nil && hctx.Response.Error != nil {
		errType = classifyChatError(hctx.Response.Error)
		recordChatError(span, hctx.Response.Error, errType)
	} else {
		errType = errTypeOther
		span.SetStatus(codes.Error, "llm error")
		span.SetAttributes(semconv.ErrorTypeKey.String(errType))
		span.AddEvent("gen_ai.client.operation.exception", trace.WithAttributes(
			semconv.ExceptionMessage("llm error"),
		))
	}
	if hasStart {
		e.recordDuration(ctx, start, hctx, nil, errType)
	}
	return nil
}

// recordChatError attaches both the standard OTel exception event
// (interoperable with all backends) and the GenAI-specific
// gen_ai.client.operation.exception event the spec defines, plus a
// low-cardinality error.type span attribute. Both events carry the same
// exception attributes so a backend can decode whichever it understands.
func recordChatError(span trace.Span, err error, errType string) {
	if err == nil {
		return
	}
	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
	if errType == "" {
		errType = errTypeOther
	}
	span.SetAttributes(semconv.ErrorTypeKey.String(errType))
	// Per the GenAI exceptions spec, emit a sibling event named
	// gen_ai.client.operation.exception with the standard exception
	// attributes. severity is implicit on Span.AddEvent (WARN by spec).
	span.AddEvent("gen_ai.client.operation.exception", trace.WithAttributes(
		semconv.ExceptionType(exceptionTypeOf(err)),
		semconv.ExceptionMessage(err.Error()),
	))
}

// exceptionTypeOf returns a stable, low-cardinality type label for an
// error — typically the underlying type's qualified name.
func exceptionTypeOf(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}

// recordDuration emits gen_ai.client.operation.duration with the canonical
// dimensions (operation, provider) plus optional model + error.type.
func (e *Extension) recordDuration(ctx context.Context, start time.Time, hctx *llm.HookContext, resp *llm.Response, errType string) {
	dur := time.Since(start).Seconds()
	var attrs []attribute.KeyValue
	if hctx.Request != nil && hctx.Request.Config != nil && hctx.Request.Config.Model != "" {
		attrs = append(attrs, e.durationMetric.AttrRequestModel(hctx.Request.Config.Model))
	}
	if resp != nil && resp.Model != "" {
		attrs = append(attrs, e.durationMetric.AttrResponseModel(resp.Model))
	}
	if errType != "" {
		attrs = append(attrs, e.durationMetric.AttrErrorType(genaiconv.ErrorTypeAttr(errType)))
	}
	e.durationMetric.Record(ctx, dur,
		genaiconv.OperationNameChat,
		genaiconv.ProviderNameAttr(e.opts.Provider),
		attrs...,
	)
}

// recordTokenUsage emits gen_ai.client.token.usage for input and output
// token counts. The instrument requires partitioning by token type.
func (e *Extension) recordTokenUsage(ctx context.Context, hctx *llm.HookContext, resp *llm.Response) {
	if resp == nil {
		return
	}
	var attrs []attribute.KeyValue
	if hctx.Request != nil && hctx.Request.Config != nil && hctx.Request.Config.Model != "" {
		attrs = append(attrs, e.tokenMetric.AttrRequestModel(hctx.Request.Config.Model))
	}
	if resp.Model != "" {
		attrs = append(attrs, e.tokenMetric.AttrResponseModel(resp.Model))
	}
	provider := genaiconv.ProviderNameAttr(e.opts.Provider)
	if resp.Usage.InputTokens > 0 {
		e.tokenMetric.Record(ctx, int64(resp.Usage.InputTokens),
			genaiconv.OperationNameChat, provider, genaiconv.TokenTypeInput, attrs...)
	}
	if resp.Usage.OutputTokens > 0 {
		e.tokenMetric.Record(ctx, int64(resp.Usage.OutputTokens),
			genaiconv.OperationNameChat, provider, genaiconv.TokenTypeOutput, attrs...)
	}
}


// agentIdentityAttrs returns the gen_ai.agent.* attributes that describe a
// Dive agent (name, description, version, id), skipping fields that are
// empty so spans don't carry zero-valued strings.
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

// conversationAttrs returns gen_ai.conversation.id when a session is
// attached. Conversation ID is the primary correlation key in backends
// that group traces by chat thread.
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

// serverAttrs parses an endpoint URL into server.address and server.port
// attributes. Empty input returns nil (these are conditionally recommended
// — emitting them with a placeholder would be worse than omitting).
func serverAttrs(endpoint string) []attribute.KeyValue {
	if endpoint == "" {
		return nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return nil
	}
	attrs := []attribute.KeyValue{semconv.ServerAddress(u.Hostname())}
	if portStr := u.Port(); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			attrs = append(attrs, semconv.ServerPort(port))
		}
	}
	return attrs
}

// usageAttrs returns the gen_ai.usage.* attributes for the given Usage,
// including cache token counts when non-zero.
func usageAttrs(u *llm.Usage) []attribute.KeyValue {
	if u == nil {
		return nil
	}
	attrs := []attribute.KeyValue{
		semconv.GenAIUsageInputTokens(u.InputTokens),
		semconv.GenAIUsageOutputTokens(u.OutputTokens),
	}
	if u.CacheCreationInputTokens != 0 {
		attrs = append(attrs, semconv.GenAIUsageCacheCreationInputTokens(u.CacheCreationInputTokens))
	}
	if u.CacheReadInputTokens != 0 {
		attrs = append(attrs, semconv.GenAIUsageCacheReadInputTokens(u.CacheReadInputTokens))
	}
	return attrs
}

// loadSpan retrieves a span from a values map. Returns nil if absent or if
// the value is not a span (defensive — Values is `map[string]any`).
func loadSpan(values map[string]any, key string) trace.Span {
	v, ok := values[key]
	if !ok {
		return nil
	}
	span, _ := v.(trace.Span)
	return span
}

// toolErrorMessage extracts a short human-readable error message from a
// tool result for SetStatus.
func toolErrorMessage(r *dive.ToolCallResult) string {
	if r == nil {
		return "tool error"
	}
	if r.Error != nil {
		return r.Error.Error()
	}
	if r.Result != nil && r.Result.IsError && len(r.Result.Content) > 0 {
		// First text content as the message.
		for _, c := range r.Result.Content {
			if c.Type == dive.ToolResultContentTypeText && c.Text != "" {
				return truncate(c.Text, 200)
			}
		}
	}
	return "tool error"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return fmt.Sprintf("%s…", s[:n])
}

// resolveInstrumentationVersion returns the module version of this
// package's containing module when available. The build records this
// when callers depend on a tagged release; local builds with a replace
// directive get instrumentationFallbackVersion instead.
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
