// Package otel emits OpenTelemetry spans from a Dive agent following the
// GenAI semantic conventions (`gen_ai.*`).
//
// Status: experimental. The API may change before promotion to a stable
// package. Vendored OTel deps are kept in a separate Go module so callers
// who do not use this package don't pay for them.
//
// # Span shape
//
//	invoke_agent {agent.name}        // wraps an entire CreateResponse call
//	├── chat {request.model}         // each LLM iteration
//	├── execute_tool {tool.name}     // each tool call
//	└── execute_tool {tool.name}
//
// # Wiring
//
//	tp := /* your TracerProvider, e.g. otlptracehttp + sdktrace.NewTracerProvider */
//	defer tp.Shutdown(ctx)
//	otel.SetTracerProvider(tp)
//
//	ext := otelext.New(otelext.WithSystem("anthropic"))
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Name:         "Research Assistant",
//	    Model:        anthropic.New(),
//	    SystemPrompt: "You are an enthusiastic researcher.",
//	    Extensions:   []dive.Extension{ext},
//	})
//
//	// Use Run to open the invoke_agent span so chat/tool spans nest under it.
//	resp, err := otelext.Run(ctx, agent, dive.WithInput("hello"))
//
// # Privacy
//
// Verbatim conversation content is dropped by default. Opt in with
// WithCaptureMessages (chat input/output) and WithCaptureToolIO (tool
// arguments and results). Both produce attributes that may contain user
// data — review your destination's retention policy before enabling.
package otel
