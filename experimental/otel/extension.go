package otel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// InstrumentationName is the OTel instrumentation scope name set on every
// span emitted by this package.
const (
	InstrumentationName    = "github.com/deepnoodle-ai/dive/experimental/otel"
	InstrumentationVersion = "0.1.0"
)

// Options configures the Extension.
type Options struct {
	// Tracer to use. Defaults to otel.GetTracerProvider().Tracer(...).
	Tracer trace.Tracer

	// System sets gen_ai.system on every span. Suggested values: "anthropic",
	// "openai", "google.gemini". When empty, "dive" is used.
	System string

	// CaptureMessages, when true, attaches gen_ai.input.messages and
	// gen_ai.output.messages on chat spans. Off by default — payloads can
	// contain user input verbatim.
	CaptureMessages bool

	// CaptureToolIO, when true, attaches gen_ai.tool.call.arguments and
	// gen_ai.tool.call.result on execute_tool spans. Off by default.
	CaptureToolIO bool

	// Attributes are added to every span this Extension produces. Useful for
	// resource-style identifiers (mobius.run.id, mobius.step.id, …) that the
	// destination uses for correlation.
	Attributes []attribute.KeyValue
}

// Option mutates Options.
type Option func(*Options)

// WithTracer sets the OTel tracer.
func WithTracer(t trace.Tracer) Option { return func(o *Options) { o.Tracer = t } }

// WithSystem sets gen_ai.system.
func WithSystem(s string) Option { return func(o *Options) { o.System = s } }

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
// correlation keys like mobius.run.id / mobius.step.id.
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
	o := Options{System: "dive"}
	for _, opt := range opts {
		opt(&o)
	}
	if o.Tracer == nil {
		o.Tracer = otel.GetTracerProvider().Tracer(
			InstrumentationName,
			trace.WithInstrumentationVersion(InstrumentationVersion),
		)
	}
	return &Extension{opts: o}
}

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

// LLMHooks returns the provider-level hooks that emit chat spans. Wire these
// on AgentOptions.LLMHooks alongside the Extension on AgentOptions.Extensions.
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

// commonAttrs returns the attribute set applied to every span.
func (e *Extension) commonAttrs() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(e.opts.Attributes)+1)
	attrs = append(attrs, attribute.String(AttrGenAISystem, e.opts.System))
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
	if contextHasAgentSpan(ctx) {
		// Run owns lifecycle; just attach common attrs to the active span.
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(e.commonAttrs()...)
		return nil
	}
	attrs := append(e.commonAttrs(),
		attribute.String(AttrGenAIOperationName, OperationInvokeAgent),
		attribute.String(AttrGenAIAgentName, name),
	)
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
		span.SetAttributes(
			attribute.Int(AttrGenAIUsageInputTokens, int(hctx.Usage.InputTokens)),
			attribute.Int(AttrGenAIUsageOutputTokens, int(hctx.Usage.OutputTokens)),
		)
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
		attribute.String(AttrGenAIOperationName, OperationExecuteTool),
		attribute.String(AttrGenAIToolName, toolName),
		attribute.String(AttrGenAIToolCallID, hctx.Call.ID),
	)
	if e.opts.CaptureToolIO && len(hctx.Call.Input) > 0 {
		attrs = append(attrs, attribute.String(AttrGenAIToolCallArgs, string(hctx.Call.Input)))
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
			span.SetAttributes(attribute.String(AttrGenAIToolCallResult, string(data)))
		}
	}

	if failed {
		span.SetStatus(codes.Error, toolErrorMessage(hctx.Result))
		span.SetAttributes(attribute.String(AttrErrorType, "tool_error"))
		if hctx.Result != nil && hctx.Result.Error != nil {
			span.RecordError(hctx.Result.Error)
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
	attrs := append(e.commonAttrs(),
		attribute.String(AttrGenAIOperationName, OperationChat),
	)
	if cfg.Model != "" {
		attrs = append(attrs, attribute.String(AttrGenAIRequestModel, cfg.Model))
	}
	if cfg.MaxTokens != nil {
		attrs = append(attrs, attribute.Int(AttrGenAIRequestMaxTokens, *cfg.MaxTokens))
	}
	if cfg.Temperature != nil {
		attrs = append(attrs, attribute.Float64(AttrGenAIRequestTemperature, *cfg.Temperature))
	}
	if e.opts.CaptureMessages {
		if data, err := json.Marshal(cfg.Messages); err == nil {
			attrs = append(attrs, attribute.String(AttrGenAIInputMessages, string(data)))
		}
		if cfg.SystemPrompt != "" {
			attrs = append(attrs, attribute.String(AttrGenAISystemInstructions, cfg.SystemPrompt))
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

// afterGenerate ends the chat span and attaches response attributes.
func (e *Extension) afterGenerate(ctx context.Context, hctx *llm.HookContext) error {
	span := e.dequeueChatSpan(ctx)
	if span == nil {
		return nil
	}
	defer span.End()

	if hctx.Response == nil {
		return nil
	}
	if hctx.Response.Error != nil {
		span.SetStatus(codes.Error, hctx.Response.Error.Error())
		span.RecordError(hctx.Response.Error)
		return nil
	}
	resp := hctx.Response.Response
	if resp == nil {
		return nil
	}

	span.SetAttributes(
		attribute.String(AttrGenAIResponseModel, resp.Model),
		attribute.String(AttrGenAIResponseID, resp.ID),
		attribute.Int(AttrGenAIUsageInputTokens, int(resp.Usage.InputTokens)),
		attribute.Int(AttrGenAIUsageOutputTokens, int(resp.Usage.OutputTokens)),
	)
	// If the provider didn't echo a request.model on cfg, fall back to the
	// response model so the span carries something useful.
	if hctx.Request != nil && hctx.Request.Config != nil && hctx.Request.Config.Model == "" && resp.Model != "" {
		span.SetAttributes(attribute.String(AttrGenAIRequestModel, resp.Model))
	}
	if resp.StopReason != "" {
		span.SetAttributes(attribute.StringSlice(AttrGenAIResponseFinishReasons, []string{resp.StopReason}))
	}
	if e.opts.CaptureMessages {
		if msg := resp.Message(); msg != nil {
			if data, err := json.Marshal(msg); err == nil {
				span.SetAttributes(attribute.String(AttrGenAIOutputMessages, string(data)))
			}
		}
	}
	return nil
}

// onError ends an in-flight chat span as Error if AfterGenerate doesn't fire.
// Many providers fire AfterGenerate even on error, so this is best-effort.
func (e *Extension) onError(ctx context.Context, hctx *llm.HookContext) error {
	span := e.dequeueChatSpan(ctx)
	if span == nil {
		return nil
	}
	defer span.End()
	if hctx.Response != nil && hctx.Response.Error != nil {
		span.SetStatus(codes.Error, hctx.Response.Error.Error())
		span.RecordError(hctx.Response.Error)
	} else {
		span.SetStatus(codes.Error, "llm error")
	}
	return nil
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
