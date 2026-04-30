package otel_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"

	otelext "github.com/deepnoodle-ai/dive/experimental/otel"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// fakeLLM is a minimal llm.LLM that returns scripted responses and (when so
// scripted) issues a tool call before producing a final assistant text.
type fakeLLM struct {
	name      string
	responses []*llm.Response
	idx       int
}

func (f *fakeLLM) Name() string { return f.name }

func (f *fakeLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.Config{}
	cfg.Apply(opts...)

	body := []byte("{}")
	hctx := &llm.HookContext{
		Type: llm.BeforeGenerate,
		Request: &llm.HookRequestContext{
			Messages: cfg.Messages,
			Config:   cfg,
			Body:     body,
		},
	}
	if err := cfg.FireHooks(ctx, hctx); err != nil {
		return nil, err
	}

	if f.idx >= len(f.responses) {
		return nil, errors.New("fakeLLM: no more scripted responses")
	}
	resp := f.responses[f.idx]
	f.idx++

	hctx = &llm.HookContext{
		Type: llm.AfterGenerate,
		Request: &llm.HookRequestContext{
			Messages: cfg.Messages,
			Config:   cfg,
			Body:     body,
		},
		Response: &llm.HookResponseContext{Response: resp},
	}
	_ = cfg.FireHooks(ctx, hctx)
	return resp, nil
}

func newRecordingProvider() (*trace.TracerProvider, *tracetest.SpanRecorder) {
	rec := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(rec))
	return tp, rec
}

func TestExtension_EmitsAgentChatToolSpans(t *testing.T) {
	tp, rec := newRecordingProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	// Script: iter 1 → assistant calls echo tool. iter 2 → final text.
	llmResponses := []*llm.Response{
		{
			ID:    "msg_1",
			Model: "fake-model-1",
			Role:  llm.Assistant,
			Content: []llm.Content{
				&llm.ToolUseContent{ID: "call_1", Name: "echo", Input: json.RawMessage(`{"text":"hi"}`)},
			},
			Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
			StopReason: "tool_use",
		},
		{
			ID:    "msg_2",
			Model: "fake-model-1",
			Role:  llm.Assistant,
			Content: []llm.Content{
				&llm.TextContent{Text: "echoed: hi"},
			},
			Usage:      llm.Usage{InputTokens: 12, OutputTokens: 4},
			StopReason: "end_turn",
		},
	}
	model := &fakeLLM{name: "fake-model-1", responses: llmResponses}

	type echoIn struct {
		Text string `json:"text"`
	}
	echoTool := dive.FuncTool("echo", "echoes input",
		func(ctx context.Context, in *echoIn) (*dive.ToolResult, error) {
			return dive.NewToolResultText(in.Text), nil
		},
	)

	ext := otelext.New(
		otelext.WithSystem("fake"),
		otelext.WithCaptureToolIO(true),
		otelext.WithCaptureMessages(true),
		otelext.WithAttributes(attribute.String("mobius.run.id", "run_test")),
	)

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Tester",
		Model:        model,
		SystemPrompt: "you are a test agent",
		Tools:        []dive.Tool{echoTool},
		Extensions:   []dive.Extension{ext},
		LLMHooks:     ext.LLMHooks(),
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := otelext.Run(context.Background(), agent, dive.WithInput("say hi"))
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if resp.OutputText() != "echoed: hi" {
		t.Fatalf("unexpected output text: %q", resp.OutputText())
	}

	// Force flush.
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	spans := rec.Ended()
	byOp := map[string]int{}
	for _, s := range spans {
		op := ""
		for _, kv := range s.Attributes() {
			if string(kv.Key) == otelext.AttrGenAIOperationName {
				op = kv.Value.AsString()
			}
		}
		byOp[op]++
	}
	if byOp["invoke_agent"] < 1 {
		t.Errorf("expected at least 1 invoke_agent span, got byOp=%v (spans=%d)", byOp, len(spans))
	}
	if byOp["chat"] != 2 {
		t.Errorf("expected 2 chat spans (one per iteration), got %d (byOp=%v)", byOp["chat"], byOp)
	}
	if byOp["execute_tool"] != 1 {
		t.Errorf("expected 1 execute_tool span, got %d (byOp=%v)", byOp["execute_tool"], byOp)
	}

	// Verify hierarchy: chat / execute_tool spans should have invoke_agent
	// span as their parent (because Run opened the parent on ctx).
	var agentSpanID string
	for _, s := range spans {
		for _, kv := range s.Attributes() {
			if string(kv.Key) == otelext.AttrGenAIOperationName && kv.Value.AsString() == "invoke_agent" {
				agentSpanID = s.SpanContext().SpanID().String()
			}
		}
	}
	if agentSpanID == "" {
		t.Fatalf("did not find invoke_agent span")
	}
	for _, s := range spans {
		op := ""
		for _, kv := range s.Attributes() {
			if string(kv.Key) == otelext.AttrGenAIOperationName {
				op = kv.Value.AsString()
			}
		}
		if op == "chat" || op == "execute_tool" {
			parent := s.Parent().SpanID().String()
			if parent != agentSpanID {
				t.Errorf("%s span %s parent=%s, want %s", op, s.Name(), parent, agentSpanID)
			}
		}
	}

	// Verify gen_ai.* attributes on a chat span.
	var sawChatAttrs bool
	for _, s := range spans {
		op := ""
		for _, kv := range s.Attributes() {
			if string(kv.Key) == otelext.AttrGenAIOperationName {
				op = kv.Value.AsString()
			}
		}
		if op != "chat" {
			continue
		}
		got := map[string]any{}
		for _, kv := range s.Attributes() {
			got[string(kv.Key)] = kv.Value.AsInterface()
		}
		// Either gen_ai.request.model or gen_ai.response.model should be set
		// to the model name. Providers that don't echo cfg.Model leave the
		// request attr falling back to resp.Model.
		if got[otelext.AttrGenAIResponseModel] != "fake-model-1" {
			t.Errorf("chat span missing/wrong gen_ai.response.model: %v", got[otelext.AttrGenAIResponseModel])
		}
		if got[otelext.AttrGenAISystem] != "fake" {
			t.Errorf("chat span missing gen_ai.system: %v", got[otelext.AttrGenAISystem])
		}
		if _, ok := got[otelext.AttrGenAIInputMessages]; !ok {
			t.Errorf("chat span missing gen_ai.input.messages (CaptureMessages=true)")
		}
		sawChatAttrs = true
		break
	}
	if !sawChatAttrs {
		t.Error("did not verify any chat span attributes")
	}

	// Verify tool span carries gen_ai.tool.* and the run-id attribute.
	for _, s := range spans {
		op := ""
		for _, kv := range s.Attributes() {
			if string(kv.Key) == otelext.AttrGenAIOperationName {
				op = kv.Value.AsString()
			}
		}
		if op != "execute_tool" {
			continue
		}
		got := map[string]any{}
		for _, kv := range s.Attributes() {
			got[string(kv.Key)] = kv.Value.AsInterface()
		}
		if got[otelext.AttrGenAIToolName] != "echo" {
			t.Errorf("execute_tool span tool name: %v", got[otelext.AttrGenAIToolName])
		}
		if got[otelext.AttrGenAIToolCallID] != "call_1" {
			t.Errorf("execute_tool span call id: %v", got[otelext.AttrGenAIToolCallID])
		}
		if got["mobius.run.id"] != "run_test" {
			t.Errorf("execute_tool span missing mobius.run.id attribute")
		}
		if _, ok := got[otelext.AttrGenAIToolCallArgs]; !ok {
			t.Errorf("execute_tool span missing gen_ai.tool.call.arguments (CaptureToolIO=true)")
		}
	}
}
