package otel_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/dive/session"

	otelext "github.com/deepnoodle-ai/dive/otel"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// fakeLLM is a minimal llm.LLM that returns scripted responses. Tests use
// onRequest to verify spans opened from the per-call ctx (the one Generate
// receives, post-StartChat) parent under the chat span.
type fakeLLM struct {
	name      string
	responses []*llm.Response
	idx       int
	onRequest func(ctx context.Context)
}

func (f *fakeLLM) Name() string { return f.name }

func (f *fakeLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	if f.onRequest != nil {
		f.onRequest(ctx)
	}
	if f.idx >= len(f.responses) {
		return nil, errors.New("fakeLLM: no more scripted responses")
	}
	resp := f.responses[f.idx]
	f.idx++
	return resp, nil
}

// streamingFakeLLM mirrors fakeLLM but exposes Stream so the agent dispatches
// through generateStreaming.
type streamingFakeLLM struct {
	name      string
	responses []*llm.Response
	idx       int
	onRequest func(ctx context.Context)
}

func (f *streamingFakeLLM) Name() string { return f.name }

func (f *streamingFakeLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	return nil, errors.New("streamingFakeLLM: Generate not supported in this test")
}

func (f *streamingFakeLLM) Stream(ctx context.Context, opts ...llm.Option) (llm.StreamIterator, error) {
	if f.onRequest != nil {
		f.onRequest(ctx)
	}
	if f.idx >= len(f.responses) {
		return nil, errors.New("streamingFakeLLM: no more scripted responses")
	}
	resp := f.responses[f.idx]
	f.idx++
	events := []*llm.Event{
		{Type: llm.EventTypeMessageStart, Message: resp},
		{Type: llm.EventTypeMessageStop},
	}
	return &sliceStreamIterator{events: events}, nil
}

type sliceStreamIterator struct {
	events []*llm.Event
	i      int
	cur    *llm.Event
}

func (s *sliceStreamIterator) Next() bool {
	if s.i >= len(s.events) {
		return false
	}
	s.cur = s.events[s.i]
	s.i++
	return true
}

func (s *sliceStreamIterator) Event() *llm.Event { return s.cur }
func (s *sliceStreamIterator) Err() error        { return nil }
func (s *sliceStreamIterator) Close() error      { return nil }

// errFakeLLM always returns an error from Generate so the chat-error path can
// be exercised.
type errFakeLLM struct {
	name string
	err  error
}

func (f *errFakeLLM) Name() string { return f.name }
func (f *errFakeLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	return nil, f.err
}

func newRecordingProvider() (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	return tp, rec
}

// attrMap flattens a span's attributes into a key→value map for concise
// table-driven assertions.
func attrMap(s sdktrace.ReadOnlySpan) map[string]any {
	out := make(map[string]any, len(s.Attributes()))
	for _, kv := range s.Attributes() {
		out[string(kv.Key)] = kv.Value.AsInterface()
	}
	return out
}

func findChatSpan(spans []sdktrace.ReadOnlySpan) sdktrace.ReadOnlySpan {
	for _, s := range spans {
		if attrMap(s)["gen_ai.operation.name"] == "chat" {
			return s
		}
	}
	return nil
}

func TestTracer_EmitsAgentChatToolSpans(t *testing.T) {
	tp, rec := newRecordingProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

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
	tracer := tp.Tracer("test")
	echoTool := dive.FuncTool("echo", "echoes input",
		func(ctx context.Context, in *echoIn) (*dive.ToolResult, error) {
			_, span := tracer.Start(ctx, "tool.work")
			span.End()
			return dive.NewToolResultText(in.Text), nil
		},
	)

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Tester",
		Model:        model,
		SystemPrompt: "you are a test agent",
		Tools:        []dive.Tool{echoTool},
		Tracer: otelext.NewTracer(
			otelext.WithProvider("fake"),
			otelext.WithCaptureToolIO(true),
			otelext.WithCaptureMessages(true),
		),
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := agent.CreateResponse(context.Background(), dive.WithInput("say hi"))
	if err != nil {
		t.Fatalf("CreateResponse failed: %v", err)
	}
	if resp.OutputText() != "echoed: hi" {
		t.Fatalf("unexpected output text: %q", resp.OutputText())
	}

	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	spans := rec.Ended()
	byOp := map[string]int{}
	for _, s := range spans {
		op, _ := attrMap(s)["gen_ai.operation.name"].(string)
		byOp[op]++
	}
	if byOp["invoke_agent"] != 1 {
		t.Errorf("expected 1 invoke_agent span, got byOp=%v", byOp)
	}
	if byOp["chat"] != 2 {
		t.Errorf("expected 2 chat spans (one per iteration), got %d (byOp=%v)", byOp["chat"], byOp)
	}
	if byOp["execute_tool"] != 1 {
		t.Errorf("expected 1 execute_tool span, got %d (byOp=%v)", byOp["execute_tool"], byOp)
	}

	// tool.work span must parent under execute_tool — that proves the
	// tracer's StartToolCall ctx flowed into Tool.Call.
	var executeToolSpanID, toolWorkParentID oteltrace.SpanID
	for _, s := range spans {
		switch s.Name() {
		case "execute_tool echo":
			executeToolSpanID = s.SpanContext().SpanID()
		case "tool.work":
			toolWorkParentID = s.Parent().SpanID()
		}
	}
	if !executeToolSpanID.IsValid() {
		t.Fatalf("did not find execute_tool span")
	}
	if toolWorkParentID != executeToolSpanID {
		t.Errorf("tool.work parent = %s, want execute_tool = %s — ctx propagation through StartToolCall is broken",
			toolWorkParentID, executeToolSpanID)
	}

	// chat / execute_tool spans must parent under the invoke_agent span.
	var agentSpanID oteltrace.SpanID
	for _, s := range spans {
		if attrMap(s)["gen_ai.operation.name"] == "invoke_agent" {
			agentSpanID = s.SpanContext().SpanID()
		}
	}
	if !agentSpanID.IsValid() {
		t.Fatal("did not find invoke_agent span")
	}
	for _, s := range spans {
		op := attrMap(s)["gen_ai.operation.name"]
		if op == "chat" || op == "execute_tool" {
			if got := s.Parent().SpanID(); got != agentSpanID {
				t.Errorf("%v span %s parent=%s, want %s", op, s.Name(), got, agentSpanID)
			}
		}
	}

	// Verify chat-span attributes.
	chat := findChatSpan(spans)
	if chat == nil {
		t.Fatal("no chat span")
	}
	got := attrMap(chat)
	if got["gen_ai.response.model"] != "fake-model-1" {
		t.Errorf("chat gen_ai.response.model: %v", got["gen_ai.response.model"])
	}
	if got["gen_ai.provider.name"] != "fake" {
		t.Errorf("chat gen_ai.provider.name: %v", got["gen_ai.provider.name"])
	}
	if _, ok := got["gen_ai.input.messages"]; !ok {
		t.Errorf("chat span missing gen_ai.input.messages (CaptureMessages=true)")
	}

	// Verify tool span attributes.
	for _, s := range spans {
		if attrMap(s)["gen_ai.operation.name"] != "execute_tool" {
			continue
		}
		got := attrMap(s)
		if got["gen_ai.tool.name"] != "echo" {
			t.Errorf("execute_tool tool name: %v", got["gen_ai.tool.name"])
		}
		if got["gen_ai.tool.call.id"] != "call_1" {
			t.Errorf("execute_tool call id: %v", got["gen_ai.tool.call.id"])
		}
		if _, ok := got["gen_ai.tool.call.arguments"]; !ok {
			t.Errorf("execute_tool span missing gen_ai.tool.call.arguments (CaptureToolIO=true)")
		}
	}
}

// TestTracer_CtxPropagation_ParallelTools verifies the parallel-tool path
// also threads StartToolCall ctx into Tool.Call so each tool's spans nest
// under its own execute_tool span.
func TestTracer_CtxPropagation_ParallelTools(t *testing.T) {
	tp, rec := newRecordingProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

	llmResponses := []*llm.Response{
		{
			ID:    "msg_1",
			Model: "fake-model-2",
			Role:  llm.Assistant,
			Content: []llm.Content{
				&llm.ToolUseContent{ID: "call_a", Name: "echo", Input: json.RawMessage(`{"text":"a"}`)},
				&llm.ToolUseContent{ID: "call_b", Name: "echo", Input: json.RawMessage(`{"text":"b"}`)},
			},
			Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
			StopReason: "tool_use",
		},
		{
			ID:    "msg_2",
			Model: "fake-model-2",
			Role:  llm.Assistant,
			Content: []llm.Content{
				&llm.TextContent{Text: "done"},
			},
			Usage:      llm.Usage{InputTokens: 12, OutputTokens: 4},
			StopReason: "end_turn",
		},
	}
	model := &fakeLLM{name: "fake-model-2", responses: llmResponses}

	type echoIn struct {
		Text string `json:"text"`
	}
	tracer := tp.Tracer("test")
	echoTool := dive.FuncTool("echo", "echoes input",
		func(ctx context.Context, in *echoIn) (*dive.ToolResult, error) {
			_, span := tracer.Start(ctx, "tool.work."+in.Text)
			span.End()
			return dive.NewToolResultText(in.Text), nil
		},
	)

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:                  "ParallelTester",
		Model:                 model,
		Tools:                 []dive.Tool{echoTool},
		Tracer:                otelext.NewTracer(otelext.WithProvider("fake")),
		ParallelToolExecution: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := agent.CreateResponse(context.Background(), dive.WithInput("go")); err != nil {
		t.Fatalf("CreateResponse: %v", err)
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	executeToolByCallID := map[string]oteltrace.SpanID{}
	for _, s := range rec.Ended() {
		if s.Name() == "execute_tool echo" {
			for _, kv := range s.Attributes() {
				if string(kv.Key) == "gen_ai.tool.call.id" {
					executeToolByCallID[kv.Value.AsString()] = s.SpanContext().SpanID()
				}
			}
		}
	}
	if len(executeToolByCallID) != 2 {
		t.Fatalf("expected 2 execute_tool spans, got %d", len(executeToolByCallID))
	}
	for _, s := range rec.Ended() {
		switch s.Name() {
		case "tool.work.a":
			if got := s.Parent().SpanID(); got != executeToolByCallID["call_a"] {
				t.Errorf("tool.work.a parent = %s, want %s", got, executeToolByCallID["call_a"])
			}
		case "tool.work.b":
			if got := s.Parent().SpanID(); got != executeToolByCallID["call_b"] {
				t.Errorf("tool.work.b parent = %s, want %s", got, executeToolByCallID["call_b"])
			}
		}
	}
}

// TestTracer_ChatSpanCtxPropagation verifies that a span opened from the ctx
// the model receives in Generate parents under the chat span. This is the
// contract that lets otelhttp middleware on the provider's HTTP client nest
// under chat — no hook involvement needed.
func TestTracer_ChatSpanCtxPropagation(t *testing.T) {
	tp, rec := newRecordingProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

	tracer := tp.Tracer("test")

	var httpSpanIDs []oteltrace.SpanID
	model := &fakeLLM{
		name: "fake-model-ctx",
		responses: []*llm.Response{{
			ID: "msg_1", Model: "fake-model-ctx", Role: llm.Assistant,
			Content:    []llm.Content{&llm.TextContent{Text: "hello"}},
			Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
			StopReason: "end_turn",
		}},
		onRequest: func(ctx context.Context) {
			_, span := tracer.Start(ctx, "fake.http")
			httpSpanIDs = append(httpSpanIDs, span.SpanContext().SpanID())
			span.End()
		},
	}

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:   "CtxTester",
		Model:  model,
		Tracer: otelext.NewTracer(otelext.WithProvider("fake")),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := agent.CreateResponse(context.Background(), dive.WithInput("hi")); err != nil {
		t.Fatalf("CreateResponse: %v", err)
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	if len(httpSpanIDs) != 1 {
		t.Fatalf("expected 1 fake.http span, got %d", len(httpSpanIDs))
	}
	var chatSpanID oteltrace.SpanID
	for _, s := range rec.Ended() {
		if attrMap(s)["gen_ai.operation.name"] == "chat" {
			chatSpanID = s.SpanContext().SpanID()
		}
	}
	if !chatSpanID.IsValid() {
		t.Fatal("did not find chat span")
	}
	for _, s := range rec.Ended() {
		if s.Name() != "fake.http" {
			continue
		}
		if got := s.Parent().SpanID(); got != chatSpanID {
			t.Errorf("fake.http parent = %s, want chat = %s — chat-ctx propagation is broken",
				got, chatSpanID)
		}
	}
}

// TestTracer_ChatSpanCtxPropagation_Streaming is the streaming-path twin of
// TestTracer_ChatSpanCtxPropagation. In the streaming path the agent uses
// generateStreaming, which feeds the chat-span ctx into Stream() and ends
// the chat span after iterator accumulation completes.
func TestTracer_ChatSpanCtxPropagation_Streaming(t *testing.T) {
	tp, rec := newRecordingProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

	tracer := tp.Tracer("test")

	var httpSpanIDs []oteltrace.SpanID
	model := &streamingFakeLLM{
		name: "fake-streaming-ctx",
		responses: []*llm.Response{{
			ID: "msg_1", Model: "fake-streaming-ctx", Role: llm.Assistant,
			Content:    []llm.Content{&llm.TextContent{Text: "hello"}},
			Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
			StopReason: "end_turn",
		}},
		onRequest: func(ctx context.Context) {
			_, span := tracer.Start(ctx, "fake.http")
			httpSpanIDs = append(httpSpanIDs, span.SpanContext().SpanID())
			span.End()
		},
	}

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:   "CtxTesterStreaming",
		Model:  model,
		Tracer: otelext.NewTracer(otelext.WithProvider("fake")),
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := agent.CreateResponse(context.Background(), dive.WithInput("hi")); err != nil {
		t.Fatalf("CreateResponse: %v", err)
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	if len(httpSpanIDs) != 1 {
		t.Fatalf("expected 1 fake.http span, got %d", len(httpSpanIDs))
	}

	var chatSpanID oteltrace.SpanID
	chatEnded := 0
	for _, s := range rec.Ended() {
		if attrMap(s)["gen_ai.operation.name"] == "chat" {
			chatSpanID = s.SpanContext().SpanID()
			chatEnded++
		}
	}
	if chatEnded != 1 {
		t.Fatalf("expected 1 ended chat span, saw %d", chatEnded)
	}
	for _, s := range rec.Ended() {
		if s.Name() != "fake.http" {
			continue
		}
		if got := s.Parent().SpanID(); got != chatSpanID {
			t.Errorf("fake.http parent = %s, want chat = %s — streaming chat-ctx propagation broken",
				got, chatSpanID)
		}
	}
}

// TestTracer_CacheTokenAttrs verifies cache token counts on llm.Usage are
// emitted as gen_ai.usage.cache_*.input_tokens attributes when non-zero.
func TestTracer_CacheTokenAttrs(t *testing.T) {
	tp, rec := newRecordingProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

	model := &fakeLLM{
		name: "fake",
		responses: []*llm.Response{{
			ID: "msg_1", Model: "fake", Role: llm.Assistant,
			Content: []llm.Content{&llm.TextContent{Text: "hi"}},
			Usage: llm.Usage{
				InputTokens:              100,
				OutputTokens:             20,
				CacheCreationInputTokens: 50,
				CacheReadInputTokens:     200,
			},
			StopReason: "end_turn",
		}},
	}
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "Cache", Model: model,
		Tracer: otelext.NewTracer(otelext.WithProvider("fake")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agent.CreateResponse(context.Background(), dive.WithInput("hi")); err != nil {
		t.Fatalf("CreateResponse: %v", err)
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	chat := findChatSpan(rec.Ended())
	if chat == nil {
		t.Fatal("no chat span found")
	}
	got := attrMap(chat)
	if got["gen_ai.usage.input_tokens"] != int64(100) {
		t.Errorf("input_tokens: got %v, want 100", got["gen_ai.usage.input_tokens"])
	}
	if got["gen_ai.usage.cache_creation.input_tokens"] != int64(50) {
		t.Errorf("cache_creation: got %v, want 50", got["gen_ai.usage.cache_creation.input_tokens"])
	}
	if got["gen_ai.usage.cache_read.input_tokens"] != int64(200) {
		t.Errorf("cache_read: got %v, want 200", got["gen_ai.usage.cache_read.input_tokens"])
	}
}

// TestTracer_ConversationID verifies gen_ai.conversation.id appears on chat,
// execute_tool, and invoke_agent spans when the agent has a session.
func TestTracer_ConversationID(t *testing.T) {
	tp, rec := newRecordingProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

	model := &fakeLLM{
		name: "fake",
		responses: []*llm.Response{{
			ID: "msg_1", Model: "fake", Role: llm.Assistant,
			Content:    []llm.Content{&llm.TextContent{Text: "ok"}},
			Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
			StopReason: "end_turn",
		}},
	}
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "ConvAgent", Model: model,
		Tracer:  otelext.NewTracer(otelext.WithProvider("fake")),
		Session: session.New("convo-42"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agent.CreateResponse(context.Background(), dive.WithInput("hi")); err != nil {
		t.Fatalf("CreateResponse: %v", err)
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	for _, s := range rec.Ended() {
		op := attrMap(s)["gen_ai.operation.name"]
		if op != "chat" && op != "invoke_agent" {
			continue
		}
		if got := attrMap(s)["gen_ai.conversation.id"]; got != "convo-42" {
			t.Errorf("%v span conversation id: got %v", op, got)
		}
	}
}

// TestTracer_AgentIdentityAttrs verifies gen_ai.agent.* fields appear on the
// invoke_agent span.
func TestTracer_AgentIdentityAttrs(t *testing.T) {
	tp, rec := newRecordingProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

	model := &fakeLLM{
		name: "fake",
		responses: []*llm.Response{{
			ID: "msg_1", Model: "fake", Role: llm.Assistant,
			Content:    []llm.Content{&llm.TextContent{Text: "ok"}},
			Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
			StopReason: "end_turn",
		}},
	}
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:        "Researcher",
		ID:          "agent-001",
		Description: "Curious researcher",
		Version:     "2025-05-04",
		Model:       model,
		Tracer:      otelext.NewTracer(otelext.WithProvider("fake")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agent.CreateResponse(context.Background(), dive.WithInput("hi")); err != nil {
		t.Fatalf("CreateResponse: %v", err)
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	for _, s := range rec.Ended() {
		got := attrMap(s)
		if got["gen_ai.operation.name"] != "invoke_agent" {
			continue
		}
		if got["gen_ai.agent.name"] != "Researcher" {
			t.Errorf("agent.name: got %v", got["gen_ai.agent.name"])
		}
		if got["gen_ai.agent.id"] != "agent-001" {
			t.Errorf("agent.id: got %v", got["gen_ai.agent.id"])
		}
		if got["gen_ai.agent.description"] != "Curious researcher" {
			t.Errorf("agent.description: got %v", got["gen_ai.agent.description"])
		}
		if got["gen_ai.agent.version"] != "2025-05-04" {
			t.Errorf("agent.version: got %v", got["gen_ai.agent.version"])
		}
		return
	}
	t.Fatal("no invoke_agent span found")
}

// TestTracer_Metrics verifies gen_ai.client.operation.duration and
// gen_ai.client.token.usage are recorded with the spec-required dimensions.
func TestTracer_Metrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prevMP := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	defer otel.SetMeterProvider(prevMP)

	tp, _ := newRecordingProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

	model := &fakeLLM{
		name: "fake",
		responses: []*llm.Response{{
			ID: "msg_1", Model: "fake", Role: llm.Assistant,
			Content:    []llm.Content{&llm.TextContent{Text: "ok"}},
			Usage:      llm.Usage{InputTokens: 12, OutputTokens: 7},
			StopReason: "end_turn",
		}},
	}
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "Metric", Model: model,
		Tracer: otelext.NewTracer(otelext.WithProvider("anthropic")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agent.CreateResponse(context.Background(), dive.WithInput("hi")); err != nil {
		t.Fatalf("CreateResponse: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	sawDuration := false
	sawInputTokens := false
	sawOutputTokens := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "gen_ai.client.operation.duration":
				sawDuration = true
				assertHasDims(t, m, []string{"gen_ai.operation.name", "gen_ai.provider.name"})
			case "gen_ai.client.token.usage":
				assertHasDims(t, m, []string{"gen_ai.operation.name", "gen_ai.provider.name", "gen_ai.token.type"})
				if hist, ok := m.Data.(metricdata.Histogram[int64]); ok {
					for _, dp := range hist.DataPoints {
						for _, kv := range dp.Attributes.ToSlice() {
							if string(kv.Key) == "gen_ai.token.type" {
								switch kv.Value.AsString() {
								case "input":
									sawInputTokens = true
								case "output":
									sawOutputTokens = true
								}
							}
						}
					}
				}
			}
		}
	}
	if !sawDuration {
		t.Error("did not see gen_ai.client.operation.duration metric")
	}
	if !sawInputTokens {
		t.Error("did not see input token usage")
	}
	if !sawOutputTokens {
		t.Error("did not see output token usage")
	}
}

// TestTracer_ChatErrorClassifies verifies providers.NewError(429) produces
// error.type=rate_limit on the chat span and the GenAI exception event.
func TestTracer_ChatErrorClassifies(t *testing.T) {
	tp, rec := newRecordingProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

	model := &errFakeLLM{err: providers.NewError(429, "slow down")}

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "ErrAgent", Model: model,
		Tracer: otelext.NewTracer(otelext.WithProvider("fake")),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = agent.CreateResponse(context.Background(), dive.WithInput("hi"))
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	chat := findChatSpan(rec.Ended())
	if chat == nil {
		t.Fatal("no chat span — error path may have skipped span emission")
	}
	if got := attrMap(chat)["error.type"]; got != "rate_limit" {
		t.Errorf("error.type: got %v, want rate_limit", got)
	}
	sawExceptionEvent := false
	for _, ev := range chat.Events() {
		if ev.Name == "gen_ai.client.operation.exception" {
			sawExceptionEvent = true
		}
	}
	if !sawExceptionEvent {
		t.Error("chat span missing gen_ai.client.operation.exception event")
	}
}

// TestTracer_ToolDescriptionAndType verifies gen_ai.tool.description and
// gen_ai.tool.type appear on execute_tool spans.
func TestTracer_ToolDescriptionAndType(t *testing.T) {
	tp, rec := newRecordingProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevTP)

	type echoIn struct {
		Text string `json:"text"`
	}
	echoTool := dive.FuncTool("echo", "echoes input back to caller",
		func(ctx context.Context, in *echoIn) (*dive.ToolResult, error) {
			return dive.NewToolResultText(in.Text), nil
		},
	)

	model := &fakeLLM{
		name: "fake",
		responses: []*llm.Response{
			{
				ID: "msg_1", Model: "fake", Role: llm.Assistant,
				Content: []llm.Content{
					&llm.ToolUseContent{ID: "c1", Name: "echo", Input: []byte(`{"text":"hi"}`)},
				},
				Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
				StopReason: "tool_use",
			},
			{
				ID: "msg_2", Model: "fake", Role: llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "done"}},
				Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
				StopReason: "end_turn",
			},
		},
	}
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "ToolAgent", Model: model,
		Tools:  []dive.Tool{echoTool},
		Tracer: otelext.NewTracer(otelext.WithProvider("fake")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agent.CreateResponse(context.Background(), dive.WithInput("hi")); err != nil {
		t.Fatalf("CreateResponse: %v", err)
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	for _, s := range rec.Ended() {
		got := attrMap(s)
		if got["gen_ai.operation.name"] != "execute_tool" {
			continue
		}
		if got["gen_ai.tool.description"] != "echoes input back to caller" {
			t.Errorf("tool.description: got %v", got["gen_ai.tool.description"])
		}
		if got["gen_ai.tool.type"] != "function" {
			t.Errorf("tool.type: got %v", got["gen_ai.tool.type"])
		}
		return
	}
	t.Fatal("no execute_tool span")
}

// assertHasDims verifies every data point of a histogram metric carries the
// listed required dimensions.
func assertHasDims(t *testing.T, m metricdata.Metrics, required []string) {
	t.Helper()
	check := func(set []string) {
		want := make(map[string]bool, len(required))
		for _, r := range required {
			want[r] = false
		}
		for _, k := range set {
			if _, ok := want[k]; ok {
				want[k] = true
			}
		}
		for k, present := range want {
			if !present {
				t.Errorf("metric %s data point missing required dim %q", m.Name, k)
			}
		}
	}
	switch h := m.Data.(type) {
	case metricdata.Histogram[float64]:
		for _, dp := range h.DataPoints {
			var keys []string
			for _, kv := range dp.Attributes.ToSlice() {
				keys = append(keys, string(kv.Key))
			}
			check(keys)
		}
	case metricdata.Histogram[int64]:
		for _, dp := range h.DataPoints {
			var keys []string
			for _, kv := range dp.Attributes.ToSlice() {
				keys = append(keys, string(kv.Key))
			}
			check(keys)
		}
	default:
		t.Errorf("metric %s: unexpected data shape %T", m.Name, m.Data)
	}
}

// keep sdktrace import
var _ = sdktrace.NewTracerProvider
