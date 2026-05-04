package otel

import (
	"context"
	"fmt"

	"github.com/deepnoodle-ai/dive"

	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

// StartToolCall opens an `execute_tool` span. The returned ctx carries the
// span so any spans the tool emits internally nest under it.
func (t *tracerImpl) StartToolCall(ctx context.Context, info dive.ToolCallInfo) (context.Context, dive.ToolCallSpan) {
	name := "tool"
	if info.Call != nil && info.Call.Name != "" {
		name = info.Call.Name
	}

	attrs := append(t.commonAttrs(), semconv.GenAIOperationNameExecuteTool)
	if info.Call != nil {
		attrs = append(attrs,
			semconv.GenAIToolName(info.Call.Name),
			semconv.GenAIToolCallID(info.Call.ID),
			// Spec values: "function", "extension", "datastore". Dive's
			// standard tools execute as in-process functions.
			semconv.GenAIToolType("function"),
		)
	}
	if info.Tool != nil {
		if desc := info.Tool.Description(); desc != "" {
			attrs = append(attrs, semconv.GenAIToolDescription(desc))
		}
	}
	attrs = append(attrs, conversationAttrs(info.Session)...)
	if t.opts.CaptureToolIO && info.Call != nil && len(info.Call.Input) > 0 {
		attrs = append(attrs, semconv.GenAIToolCallArgumentsKey.String(string(info.Call.Input)))
	}

	ctx, span := t.opts.Tracer.Start(ctx, "execute_tool "+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	)
	return ctx, &toolCallSpan{tracer: t, span: span}
}

type toolCallSpan struct {
	tracer *tracerImpl
	span   trace.Span
	result *dive.ToolCallResult
}

func (s *toolCallSpan) SetResult(r *dive.ToolCallResult) {
	if r == nil {
		return
	}
	s.result = r
	if s.tracer.opts.CaptureToolIO && r.Result != nil && r.Result.Content != nil {
		if data := marshalJSON(r.Result.Content); data != "" {
			s.span.SetAttributes(semconv.GenAIToolCallResultKey.String(data))
		}
	}
}

func (s *toolCallSpan) End(err error) {
	defer s.span.End()

	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
		s.span.SetAttributes(semconv.ErrorTypeKey.String(exceptionTypeOf(err)))
		return
	}
	failed := s.result != nil && (s.result.Error != nil || (s.result.Result != nil && s.result.Result.IsError))
	if failed {
		errType := classifyToolError(s.result)
		s.span.SetStatus(codes.Error, toolErrorMessage(s.result))
		s.span.SetAttributes(semconv.ErrorTypeKey.String(errType))
		if s.result.Error != nil {
			s.span.RecordError(s.result.Error)
			s.span.AddEvent("gen_ai.client.operation.exception", trace.WithAttributes(
				semconv.ExceptionType(exceptionTypeOf(s.result.Error)),
				semconv.ExceptionMessage(s.result.Error.Error()),
			))
		} else {
			s.span.AddEvent("gen_ai.client.operation.exception", trace.WithAttributes(
				semconv.ExceptionMessage(toolErrorMessage(s.result)),
			))
		}
	}
}

// exceptionTypeOf returns a low-cardinality type label for an error.
func exceptionTypeOf(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}

// toolErrorMessage extracts a short human-readable error message.
func toolErrorMessage(r *dive.ToolCallResult) string {
	if r == nil {
		return "tool error"
	}
	if r.Error != nil {
		return r.Error.Error()
	}
	if r.Result != nil && r.Result.IsError && len(r.Result.Content) > 0 {
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
