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
	oteltrace "go.opentelemetry.io/otel/trace"
)

// fakeLLM is a minimal llm.LLM that returns scripted responses and (when so
// scripted) issues a tool call before producing a final assistant text. It
// mirrors real-provider semantics for HookContext.UpdatedCtx: if a
// BeforeGenerate hook publishes an updated ctx, the optional onRequest
// callback is invoked with that ctx (acting as the synthetic HTTP boundary),
// while AfterGenerate is fired on the original ctx.
type fakeLLM struct {
	name      string
	responses []*llm.Response
	idx       int

	// onRequest, if set, is called with the ctx the provider would use for
	// the underlying HTTP/SDK request (post-UpdatedCtx pickup). Tests use
	// this to open a synthetic HTTP span and verify it nests under the
	// chat span.
	onRequest func(ctx context.Context)
}

func (f *fakeLLM) Name() string { return f.name }

func (f *fakeLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.Config{}
	cfg.Apply(opts...)

	body := []byte("{}")
	beforeHook := &llm.HookContext{
		Type: llm.BeforeGenerate,
		Request: &llm.HookRequestContext{
			Messages: cfg.Messages,
			Config:   cfg,
			Body:     body,
		},
	}
	if err := cfg.FireHooks(ctx, beforeHook); err != nil {
		return nil, err
	}
	reqCtx := ctx
	if beforeHook.UpdatedCtx != nil {
		reqCtx = beforeHook.UpdatedCtx
	}
	if f.onRequest != nil {
		f.onRequest(reqCtx)
	}

	if f.idx >= len(f.responses) {
		return nil, errors.New("fakeLLM: no more scripted responses")
	}
	resp := f.responses[f.idx]
	f.idx++

	hctx := &llm.HookContext{
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

// streamingFakeLLM is the streaming counterpart of fakeLLM. It implements
// llm.StreamingLLM so the agent dispatches through generateStreaming, which
// is the path that exercises AfterGenerate firing from the agent (not from
// the provider). Same UpdatedCtx semantics as fakeLLM: BeforeGenerate fires
// inside Stream(); the post-UpdatedCtx ctx is handed to onRequest.
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
	cfg := &llm.Config{}
	cfg.Apply(opts...)

	beforeHook := &llm.HookContext{
		Type: llm.BeforeGenerate,
		Request: &llm.HookRequestContext{
			Messages: cfg.Messages,
			Config:   cfg,
			Body:     []byte("{}"),
		},
	}
	if err := cfg.FireHooks(ctx, beforeHook); err != nil {
		return nil, err
	}
	reqCtx := ctx
	if beforeHook.UpdatedCtx != nil {
		reqCtx = beforeHook.UpdatedCtx
	}
	if f.onRequest != nil {
		f.onRequest(reqCtx)
	}

	if f.idx >= len(f.responses) {
		return nil, errors.New("streamingFakeLLM: no more scripted responses")
	}
	resp := f.responses[f.idx]
	f.idx++

	// Synthesize the minimum event sequence that ResponseAccumulator
	// accepts: a MessageStart that carries the full response (content
	// already populated), then a MessageStop. finalizeContent skips
	// rebuilding content when the contentBlocks map is empty, so the
	// response we set on MessageStart survives intact.
	events := []*llm.Event{
		{Type: llm.EventTypeMessageStart, Message: resp},
		{Type: llm.EventTypeMessageStop},
	}
	return &sliceStreamIterator{events: events}, nil
}

// sliceStreamIterator emits a fixed slice of events. Sufficient for tests
// that don't care about delta semantics — only about the BeforeGenerate /
// AfterGenerate ctx contract.
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
	// The tool opens its own child span on the ctx it receives. Whether
	// that span ends up under execute_tool depends on whether the agent
	// passes the per-tool ctx in (HookContext.UpdatedCtx wiring).
	tracer := tp.Tracer("test")
	echoTool := dive.FuncTool("echo", "echoes input",
		func(ctx context.Context, in *echoIn) (*dive.ToolResult, error) {
			_, span := tracer.Start(ctx, "tool.work")
			span.End()
			return dive.NewToolResultText(in.Text), nil
		},
	)

	ext := otelext.New(
		otelext.WithSystem("fake"),
		otelext.WithCaptureToolIO(true),
		otelext.WithCaptureMessages(true),
		otelext.WithAttributes(attribute.String(otelext.AttrMobiusRunID, "run_test")),
	)

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Tester",
		Model:        model,
		SystemPrompt: "you are a test agent",
		Tools:        []dive.Tool{echoTool},
		Extensions:   []dive.Extension{ext},
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

	// Verify the tool's own child span (tool.work) nests under the
	// execute_tool span. This is the ctx-propagation guarantee — without
	// HookContext.UpdatedCtx wiring, tool.work would be a sibling of
	// execute_tool instead of a child.
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
		t.Errorf("tool.work span parent = %s, want execute_tool span %s — ctx propagation through PreToolUse is broken",
			toolWorkParentID, executeToolSpanID)
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
		if got[otelext.AttrMobiusRunID] != "run_test" {
			t.Errorf("execute_tool span missing %s attribute", otelext.AttrMobiusRunID)
		}
		if _, ok := got[otelext.AttrGenAIToolCallArgs]; !ok {
			t.Errorf("execute_tool span missing gen_ai.tool.call.arguments (CaptureToolIO=true)")
		}
	}
}

// TestExtension_CtxPropagation_ParallelTools verifies that the parallel-tool
// execution path also wires HookContext.UpdatedCtx through to Tool.Call so
// each tool's internal spans nest under its own execute_tool span.
func TestExtension_CtxPropagation_ParallelTools(t *testing.T) {
	tp, rec := newRecordingProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	// Iter 1: assistant requests two tool calls in one message.
	// Iter 2: final text response.
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
			// Open a child span tagged with the input text so we can
			// verify each parallel call's ctx independently.
			_, span := tracer.Start(ctx, "tool.work."+in.Text)
			span.End()
			return dive.NewToolResultText(in.Text), nil
		},
	)

	ext := otelext.New(otelext.WithSystem("fake"))

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:                  "ParallelTester",
		Model:                 model,
		Tools:                 []dive.Tool{echoTool},
		Extensions:            []dive.Extension{ext},
		ParallelToolExecution: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := otelext.Run(context.Background(), agent, dive.WithInput("go")); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	// Build a name → SpanID and SpanID → ParentID lookup. For execute_tool
	// spans, also remember which tool_call_id they cover.
	executeToolByCallID := map[string]oteltrace.SpanID{} // call_id → execute_tool span id
	parents := map[oteltrace.SpanID]oteltrace.SpanID{}   // span id → parent id
	for _, s := range rec.Ended() {
		parents[s.SpanContext().SpanID()] = s.Parent().SpanID()
		if s.Name() == "execute_tool echo" {
			for _, kv := range s.Attributes() {
				if string(kv.Key) == otelext.AttrGenAIToolCallID {
					executeToolByCallID[kv.Value.AsString()] = s.SpanContext().SpanID()
				}
			}
		}
	}

	if len(executeToolByCallID) != 2 {
		t.Fatalf("expected 2 execute_tool spans (one per parallel call), got %d", len(executeToolByCallID))
	}

	// Each tool.work.X span should be a child of the execute_tool span for
	// its corresponding call ID. With UpdatedCtx wired correctly, each
	// goroutine sees its OWN tool ctx — not childCtx, and not a sibling's.
	for _, s := range rec.Ended() {
		switch s.Name() {
		case "tool.work.a":
			if got := s.Parent().SpanID(); got != executeToolByCallID["call_a"] {
				t.Errorf("tool.work.a parent = %s, want execute_tool[call_a] = %s",
					got, executeToolByCallID["call_a"])
			}
		case "tool.work.b":
			if got := s.Parent().SpanID(); got != executeToolByCallID["call_b"] {
				t.Errorf("tool.work.b parent = %s, want execute_tool[call_b] = %s",
					got, executeToolByCallID["call_b"])
			}
		}
	}

	// Reference parents to silence unused-variable on the lookup we built
	// for debug ergonomics — keeping it around makes future asserts easy.
	_ = parents
}

// TestExtension_ChatSpanCtxPropagation verifies that a span opened from the
// ctx the provider receives after BeforeGenerate (i.e. HookContext.UpdatedCtx
// pickup) parents under the chat span. This is the contract that lets
// otelhttp middleware on the provider's HTTP client nest under chat.
func TestExtension_ChatSpanCtxPropagation(t *testing.T) {
	tp, rec := newRecordingProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	tracer := tp.Tracer("test")

	// fakeLLM opens a "fake.http" span on the ctx it would use for the
	// underlying HTTP call — i.e. the post-UpdatedCtx ctx from the
	// BeforeGenerate hook.
	var httpSpanIDs []oteltrace.SpanID
	model := &fakeLLM{
		name: "fake-model-ctx",
		responses: []*llm.Response{
			{
				ID:    "msg_1",
				Model: "fake-model-ctx",
				Role:  llm.Assistant,
				Content: []llm.Content{
					&llm.TextContent{Text: "hello"},
				},
				Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
				StopReason: "end_turn",
			},
		},
		onRequest: func(ctx context.Context) {
			_, span := tracer.Start(ctx, "fake.http")
			httpSpanIDs = append(httpSpanIDs, span.SpanContext().SpanID())
			span.End()
		},
	}

	ext := otelext.New(otelext.WithSystem("fake"))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:       "CtxTester",
		Model:      model,
		Extensions: []dive.Extension{ext},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := otelext.Run(context.Background(), agent, dive.WithInput("hi")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	if len(httpSpanIDs) != 1 {
		t.Fatalf("expected 1 fake.http span, got %d", len(httpSpanIDs))
	}

	// Find the chat span ID and verify fake.http parents under it.
	var chatSpanID oteltrace.SpanID
	for _, s := range rec.Ended() {
		for _, kv := range s.Attributes() {
			if string(kv.Key) == otelext.AttrGenAIOperationName && kv.Value.AsString() == "chat" {
				chatSpanID = s.SpanContext().SpanID()
			}
		}
	}
	if !chatSpanID.IsValid() {
		t.Fatalf("did not find chat span")
	}
	for _, s := range rec.Ended() {
		if s.Name() != "fake.http" {
			continue
		}
		if got := s.Parent().SpanID(); got != chatSpanID {
			t.Errorf("fake.http parent = %s, want chat span %s — chat-ctx propagation through BeforeGenerate is broken",
				got, chatSpanID)
		}
	}
}

// TestExtension_ChatSpanCtxPropagation_Streaming is the streaming-path twin
// of TestExtension_ChatSpanCtxPropagation. The streaming path is structurally
// different: providers fire BeforeGenerate from Stream() but the agent (not
// the provider) fires AfterGenerate after iterator accumulation. This test
// locks in the contract that both paths produce identically-parented
// fake.http spans.
func TestExtension_ChatSpanCtxPropagation_Streaming(t *testing.T) {
	tp, rec := newRecordingProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	tracer := tp.Tracer("test")

	var httpSpanIDs []oteltrace.SpanID
	model := &streamingFakeLLM{
		name: "fake-streaming-ctx",
		responses: []*llm.Response{
			{
				ID:    "msg_1",
				Model: "fake-streaming-ctx",
				Role:  llm.Assistant,
				Content: []llm.Content{
					&llm.TextContent{Text: "hello"},
				},
				Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
				StopReason: "end_turn",
			},
		},
		onRequest: func(ctx context.Context) {
			_, span := tracer.Start(ctx, "fake.http")
			httpSpanIDs = append(httpSpanIDs, span.SpanContext().SpanID())
			span.End()
		},
	}

	ext := otelext.New(otelext.WithSystem("fake"))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:       "CtxTesterStreaming",
		Model:      model,
		Extensions: []dive.Extension{ext},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := otelext.Run(context.Background(), agent, dive.WithInput("hi")); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	if len(httpSpanIDs) != 1 {
		t.Fatalf("expected 1 fake.http span, got %d", len(httpSpanIDs))
	}

	var chatSpanID oteltrace.SpanID
	chatSpansEnded := 0
	for _, s := range rec.Ended() {
		for _, kv := range s.Attributes() {
			if string(kv.Key) == otelext.AttrGenAIOperationName && kv.Value.AsString() == "chat" {
				chatSpanID = s.SpanContext().SpanID()
				chatSpansEnded++
			}
		}
	}
	if !chatSpanID.IsValid() {
		t.Fatalf("did not find chat span")
	}
	// The agent's fireLLMAfterGenerate must end the chat span — if streaming
	// providers stopped firing AfterGenerate themselves and the agent's
	// closing call regressed, the chat span would never end and wouldn't
	// appear in rec.Ended(). Asserting >=1 catches that regression.
	if chatSpansEnded < 1 {
		t.Fatalf("expected the streaming chat span to be ended (via agent.fireLLMAfterGenerate), saw %d ended chat spans", chatSpansEnded)
	}
	for _, s := range rec.Ended() {
		if s.Name() != "fake.http" {
			continue
		}
		if got := s.Parent().SpanID(); got != chatSpanID {
			t.Errorf("fake.http parent = %s, want chat span %s — chat-ctx propagation in the streaming path is broken",
				got, chatSpanID)
		}
	}
}
