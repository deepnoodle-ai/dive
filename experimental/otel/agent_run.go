package otel

import (
	"context"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

// StartAgentRun opens an `invoke_agent` span. The returned ctx carries the
// span so chat and tool spans nest under it.
func (t *tracerImpl) StartAgentRun(ctx context.Context, info dive.AgentRunInfo) (context.Context, dive.AgentRunSpan) {
	name := "agent"
	if info.Agent != nil && info.Agent.Name() != "" {
		name = info.Agent.Name()
	}
	attrs := append(t.commonAttrs(), semconv.GenAIOperationNameInvokeAgent)
	attrs = append(attrs, agentIdentityAttrs(info.Agent)...)
	attrs = append(attrs, conversationAttrs(info.Session)...)

	ctx, span := t.opts.Tracer.Start(ctx, "invoke_agent "+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
	return ctx, &agentRunSpan{span: span}
}

type agentRunSpan struct {
	span trace.Span
}

func (s *agentRunSpan) SetResponse(resp *dive.Response) {
	if resp == nil {
		return
	}
	if resp.Status == dive.ResponseStatusSuspended {
		s.span.SetAttributes(attribute.String("dive.response.status", "suspended"))
	}
}

func (s *agentRunSpan) SetUsage(u *llm.Usage) {
	if u == nil {
		return
	}
	s.span.SetAttributes(usageAttrs(u)...)
}

func (s *agentRunSpan) End(err error) {
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
	s.span.End()
}

// usageAttrs returns gen_ai.usage.* attributes including cache token counts
// when non-zero.
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
