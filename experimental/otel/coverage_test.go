package otel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/dive/session"

	otelext "github.com/deepnoodle-ai/dive/experimental/otel"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestExtension_CacheTokenAttrs verifies that cache token counts on
// llm.Usage are emitted as gen_ai.usage.cache_*.input_tokens attributes
// when non-zero. The Anthropic provider populates these on prompt-cache
// hits; the GenAI spec promotes them to first-class attributes.
func TestExtension_CacheTokenAttrs(t *testing.T) {
	tp, rec := newRecordingProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	model := &fakeLLM{
		name: "fake",
		responses: []*llm.Response{{
			ID:    "msg_1",
			Model: "fake",
			Role:  llm.Assistant,
			Content: []llm.Content{
				&llm.TextContent{Text: "hi"},
			},
			Usage: llm.Usage{
				InputTokens:              100,
				OutputTokens:             20,
				CacheCreationInputTokens: 50,
				CacheReadInputTokens:     200,
			},
			StopReason: "end_turn",
		}},
	}

	ext := otelext.New(otelext.WithProvider("fake"))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "Cache", Model: model, Extensions: []dive.Extension{ext},
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

// TestExtension_ConversationID verifies gen_ai.conversation.id is emitted
// on chat, execute_tool, and invoke_agent spans when the agent has a
// session — both via the dive.HookContext.Session field and via the
// ctx-attached session that LLM hooks read.
func TestExtension_ConversationID(t *testing.T) {
	tp, rec := newRecordingProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	model := &fakeLLM{
		name: "fake",
		responses: []*llm.Response{{
			ID: "msg_1", Model: "fake", Role: llm.Assistant,
			Content:    []llm.Content{&llm.TextContent{Text: "ok"}},
			Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
			StopReason: "end_turn",
		}},
	}

	ext := otelext.New(otelext.WithProvider("fake"))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "ConvAgent", Model: model, Extensions: []dive.Extension{ext},
		Session: session.New("convo-42"),
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

	for _, s := range rec.Ended() {
		op := attrMap(s)["gen_ai.operation.name"]
		if op != "chat" && op != "invoke_agent" {
			continue
		}
		if got := attrMap(s)["gen_ai.conversation.id"]; got != "convo-42" {
			t.Errorf("%v span missing conversation id: got %v", op, got)
		}
	}
}

// TestExtension_AgentIdentityAttrs verifies the gen_ai.agent.* attributes
// (name, id, description, version) appear on the invoke_agent span.
func TestExtension_AgentIdentityAttrs(t *testing.T) {
	tp, rec := newRecordingProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	model := &fakeLLM{
		name: "fake",
		responses: []*llm.Response{{
			ID: "msg_1", Model: "fake", Role: llm.Assistant,
			Content:    []llm.Content{&llm.TextContent{Text: "ok"}},
			Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
			StopReason: "end_turn",
		}},
	}

	ext := otelext.New(otelext.WithProvider("fake"))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:        "Researcher",
		ID:          "agent-001",
		Description: "Curious researcher",
		Version:     "2025-05-04",
		Model:       model,
		Extensions:  []dive.Extension{ext},
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

// TestExtension_ToolDescriptionAndType verifies gen_ai.tool.description
// and gen_ai.tool.type are populated on execute_tool spans.
func TestExtension_ToolDescriptionAndType(t *testing.T) {
	tp, rec := newRecordingProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(otel.GetTracerProvider())

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

	ext := otelext.New(otelext.WithProvider("fake"))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "ToolAgent", Model: model,
		Tools:      []dive.Tool{echoTool},
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

// TestExtension_Metrics verifies that gen_ai.client.operation.duration
// and gen_ai.client.token.usage are recorded with the spec-required
// dimensions.
func TestExtension_Metrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	defer otel.SetMeterProvider(otel.GetMeterProvider())

	tp, _ := newRecordingProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	model := &fakeLLM{
		name: "fake",
		responses: []*llm.Response{{
			ID: "msg_1", Model: "fake", Role: llm.Assistant,
			Content:    []llm.Content{&llm.TextContent{Text: "ok"}},
			Usage:      llm.Usage{InputTokens: 12, OutputTokens: 7},
			StopReason: "end_turn",
		}},
	}

	ext := otelext.New(otelext.WithProvider("anthropic"))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "Metric", Model: model, Extensions: []dive.Extension{ext},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := otelext.Run(context.Background(), agent, dive.WithInput("hi")); err != nil {
		t.Fatalf("Run: %v", err)
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

// TestExtension_ServerAddressFromEndpoint verifies server.address /
// server.port are populated from the provider's Endpoint field.
func TestExtension_ServerAddressFromEndpoint(t *testing.T) {
	tp, rec := newRecordingProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	model := &endpointFakeLLM{
		fakeLLM: fakeLLM{
			name: "fake",
			responses: []*llm.Response{{
				ID: "msg_1", Model: "fake", Role: llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "ok"}},
				Usage:      llm.Usage{InputTokens: 1, OutputTokens: 1},
				StopReason: "end_turn",
			}},
		},
		endpoint: "https://api.example.com:8443/v1/messages",
	}

	ext := otelext.New(otelext.WithProvider("fake"))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "EndpointAgent", Model: model, Extensions: []dive.Extension{ext},
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

	chat := findChatSpan(rec.Ended())
	if chat == nil {
		t.Fatal("no chat span")
	}
	got := attrMap(chat)
	if got["server.address"] != "api.example.com" {
		t.Errorf("server.address: got %v", got["server.address"])
	}
	if got["server.port"] != int64(8443) {
		t.Errorf("server.port: got %v", got["server.port"])
	}
}

// TestExtension_ChatErrorClassifies verifies that providers.NewError(429)
// surfaces as error.type=rate_limit on the chat span and the GenAI
// exception event fires.
func TestExtension_ChatErrorClassifies(t *testing.T) {
	tp, rec := newRecordingProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(otel.GetTracerProvider())

	model := &errFakeLLM{err: providers.NewError(429, "slow down")}

	ext := otelext.New(otelext.WithProvider("fake"))
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "ErrAgent", Model: model, Extensions: []dive.Extension{ext},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = otelext.Run(context.Background(), agent, dive.WithInput("hi"))
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

// errFakeLLM always returns an error from Generate so the OnError /
// AfterGenerate-with-error path can be exercised. It still fires the
// Before/After hook contract so the Extension's chat span lifecycle
// runs to completion.
type errFakeLLM struct {
	name string
	err  error
}

func (f *errFakeLLM) Name() string {
	if f.name != "" {
		return f.name
	}
	return "fake"
}

func (f *errFakeLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.Config{}
	cfg.Apply(opts...)
	if err := cfg.FireHooks(ctx, &llm.HookContext{
		Type:    llm.BeforeGenerate,
		Request: &llm.HookRequestContext{Messages: cfg.Messages, Config: cfg},
	}); err != nil {
		return nil, err
	}
	_ = cfg.FireHooks(ctx, &llm.HookContext{
		Type:     llm.AfterGenerate,
		Request:  &llm.HookRequestContext{Messages: cfg.Messages, Config: cfg},
		Response: &llm.HookResponseContext{Error: f.err},
	})
	return nil, f.err
}

// endpointFakeLLM extends fakeLLM by populating HookRequestContext.Endpoint
// so server.address parsing can be verified end-to-end.
type endpointFakeLLM struct {
	fakeLLM
	endpoint string
}

func (f *endpointFakeLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.Config{}
	cfg.Apply(opts...)
	beforeHook := &llm.HookContext{
		Type:    llm.BeforeGenerate,
		Request: &llm.HookRequestContext{Messages: cfg.Messages, Config: cfg, Endpoint: f.endpoint},
	}
	if err := cfg.FireHooks(ctx, beforeHook); err != nil {
		return nil, err
	}
	if f.fakeLLM.idx >= len(f.fakeLLM.responses) {
		return nil, errors.New("no responses")
	}
	resp := f.fakeLLM.responses[f.fakeLLM.idx]
	f.fakeLLM.idx++
	_ = cfg.FireHooks(ctx, &llm.HookContext{
		Type:     llm.AfterGenerate,
		Request:  &llm.HookRequestContext{Messages: cfg.Messages, Config: cfg, Endpoint: f.endpoint},
		Response: &llm.HookResponseContext{Response: resp},
	})
	return resp, nil
}

// findChatSpan returns the first ended span whose gen_ai.operation.name
// is "chat", or nil. Used by tests that don't care about the iteration
// count.
func findChatSpan(spans []sdktrace.ReadOnlySpan) sdktrace.ReadOnlySpan {
	for _, s := range spans {
		if attrMap(s)["gen_ai.operation.name"] == "chat" {
			return s
		}
	}
	return nil
}

// attrMap flattens a span's attributes into a key→AsInterface map for
// concise table-driven assertions.
func attrMap(s sdktrace.ReadOnlySpan) map[string]any {
	out := make(map[string]any, len(s.Attributes()))
	for _, kv := range s.Attributes() {
		out[string(kv.Key)] = kv.Value.AsInterface()
	}
	return out
}

// assertHasDims verifies every data point of a histogram metric carries
// the listed required dimensions.
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

// _silenceTracetestImport keeps the alias used so editors don't drop it.
var _ = tracetest.NewSpanRecorder
