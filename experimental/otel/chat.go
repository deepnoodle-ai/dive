package otel

import (
	"context"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/semconv/v1.40.0/genaiconv"
	"go.opentelemetry.io/otel/trace"
)

// StartChat opens a `chat` CLIENT span for one LLM iteration.
func (t *tracerImpl) StartChat(ctx context.Context, info dive.ChatInfo) (context.Context, dive.ChatSpan) {
	attrs := append(t.commonAttrs(), semconv.GenAIOperationNameChat)
	attrs = append(attrs, agentIdentityAttrs(info.Agent)...)
	attrs = append(attrs, conversationAttrs(info.Session)...)
	if info.Model != "" {
		attrs = append(attrs, semconv.GenAIRequestModel(info.Model))
	}
	if info.MaxTokens != nil {
		attrs = append(attrs, semconv.GenAIRequestMaxTokens(*info.MaxTokens))
	}
	if info.Temperature != nil {
		attrs = append(attrs, semconv.GenAIRequestTemperature(*info.Temperature))
	}
	if info.FrequencyPenalty != nil {
		attrs = append(attrs, semconv.GenAIRequestFrequencyPenalty(*info.FrequencyPenalty))
	}
	if info.PresencePenalty != nil {
		attrs = append(attrs, semconv.GenAIRequestPresencePenalty(*info.PresencePenalty))
	}
	if t.opts.CaptureMessages {
		if data := marshalJSON(info.Messages); data != "" {
			attrs = append(attrs, semconv.GenAIInputMessagesKey.String(data))
		}
		if info.SystemPrompt != "" {
			attrs = append(attrs, semconv.GenAISystemInstructionsKey.String(info.SystemPrompt))
		}
	}

	name := "chat"
	if info.Model != "" {
		name = "chat " + info.Model
	}

	ctx, span := t.opts.Tracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
	return ctx, &chatSpan{
		tracer:       t,
		span:         span,
		start:        time.Now(),
		requestModel: info.Model,
	}
}

type chatSpan struct {
	tracer       *tracerImpl
	span         trace.Span
	start        time.Time
	requestModel string
	resp         *llm.Response
	ttfc         float64
}

func (s *chatSpan) SetResponse(resp *llm.Response) {
	if resp == nil {
		return
	}
	s.resp = resp
	attrs := []attribute.KeyValue{
		semconv.GenAIResponseModel(resp.Model),
		semconv.GenAIResponseID(resp.ID),
	}
	attrs = append(attrs, usageAttrs(&resp.Usage)...)
	if s.requestModel == "" && resp.Model != "" {
		attrs = append(attrs, semconv.GenAIRequestModel(resp.Model))
	}
	if resp.StopReason != "" {
		attrs = append(attrs, semconv.GenAIResponseFinishReasons(resp.StopReason))
	}
	s.span.SetAttributes(attrs...)
	if s.tracer.opts.CaptureMessages {
		if msg := resp.Message(); msg != nil {
			if data := marshalJSON(msg); data != "" {
				s.span.SetAttributes(semconv.GenAIOutputMessagesKey.String(data))
			}
		}
	}
}

func (s *chatSpan) SetTimeToFirstChunk(seconds float64) {
	s.ttfc = seconds
	if seconds > 0 {
		s.span.SetAttributes(attribute.Float64("gen_ai.server.time_to_first_token", seconds))
	}
}

func (s *chatSpan) End(err error) {
	defer s.span.End()

	dur := time.Since(s.start).Seconds()
	errType := ""
	if err != nil {
		errType = classifyChatError(err)
		recordChatError(s.span, err, errType)
	}
	s.recordDuration(dur, errType)
	if err == nil && s.resp != nil {
		s.recordTokenUsage(s.resp)
	}
}

// recordDuration emits gen_ai.client.operation.duration.
func (s *chatSpan) recordDuration(dur float64, errType string) {
	var attrs []attribute.KeyValue
	if s.requestModel != "" {
		attrs = append(attrs, s.tracer.durationMetric.AttrRequestModel(s.requestModel))
	}
	if s.resp != nil && s.resp.Model != "" {
		attrs = append(attrs, s.tracer.durationMetric.AttrResponseModel(s.resp.Model))
	}
	if errType != "" {
		attrs = append(attrs, s.tracer.durationMetric.AttrErrorType(genaiconv.ErrorTypeAttr(errType)))
	}
	s.tracer.durationMetric.Record(context.Background(), dur,
		genaiconv.OperationNameChat,
		genaiconv.ProviderNameAttr(s.tracer.opts.Provider),
		attrs...,
	)
}

// recordTokenUsage emits gen_ai.client.token.usage for input and output tokens.
func (s *chatSpan) recordTokenUsage(resp *llm.Response) {
	var attrs []attribute.KeyValue
	if s.requestModel != "" {
		attrs = append(attrs, s.tracer.tokenMetric.AttrRequestModel(s.requestModel))
	}
	if resp.Model != "" {
		attrs = append(attrs, s.tracer.tokenMetric.AttrResponseModel(resp.Model))
	}
	provider := genaiconv.ProviderNameAttr(s.tracer.opts.Provider)
	if resp.Usage.InputTokens > 0 {
		s.tracer.tokenMetric.Record(context.Background(), int64(resp.Usage.InputTokens),
			genaiconv.OperationNameChat, provider, genaiconv.TokenTypeInput, attrs...)
	}
	if resp.Usage.OutputTokens > 0 {
		s.tracer.tokenMetric.Record(context.Background(), int64(resp.Usage.OutputTokens),
			genaiconv.OperationNameChat, provider, genaiconv.TokenTypeOutput, attrs...)
	}
}

// recordChatError attaches the standard OTel exception event plus the GenAI
// gen_ai.client.operation.exception event the spec defines, plus a low-
// cardinality error.type attribute.
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
	span.AddEvent("gen_ai.client.operation.exception", trace.WithAttributes(
		semconv.ExceptionType(exceptionTypeOf(err)),
		semconv.ExceptionMessage(err.Error()),
	))
}
