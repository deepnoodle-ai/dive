package llm

import "context"

// Hook types for different LLM events
type HookType string

const (
	BeforeGenerate HookType = "before_generate"
	AfterGenerate  HookType = "after_generate"
	OnError        HookType = "on_error"
)

// HookRequestContext contains information about a request to an LLM.
type HookRequestContext struct {
	Messages []*Message `json:"messages"`
	Config   *Config    `json:"config,omitempty"`
	Body     []byte     `json:"-"`

	// Streaming is true when the provider received the request via Stream
	// rather than Generate. Set by the provider before firing
	// BeforeGenerate so observability hooks can label the span
	// (gen_ai.request.stream).
	Streaming bool `json:"streaming,omitempty"`

	// Endpoint is the absolute URL the provider will POST to (e.g.
	// "https://api.anthropic.com/v1/messages"). Set by the provider so
	// observability hooks can populate server.address and server.port on
	// CLIENT spans without each provider implementing the parsing.
	Endpoint string `json:"endpoint,omitempty"`
}

// HookResponseContext contains information about a response from an LLM.
type HookResponseContext struct {
	Response *Response `json:"response,omitempty"`
	Error    error     `json:"error,omitempty"`

	// TimeToFirstChunk is the elapsed seconds from request issue to the
	// first content chunk being received. Set by the agent for streaming
	// responses; zero for non-streaming. Surfaced as
	// gen_ai.response.time_to_first_chunk.
	TimeToFirstChunk float64 `json:"time_to_first_chunk,omitempty"`
}

// HookContext contains information passed to hooks.
type HookContext struct {
	Type     HookType             `json:"type"`
	Request  *HookRequestContext  `json:"request,omitempty"`
	Response *HookResponseContext `json:"response,omitempty"`

	// UpdatedCtx, if set by a BeforeGenerate hook, replaces the context the
	// provider uses for the underlying HTTP/SDK request. Mirrors
	// dive.HookContext.UpdatedCtx. Useful for OTel: the otel extension sets
	// this to the chat-span ctx so HTTP-client middleware (e.g. otelhttp)
	// nests provider HTTP spans under the chat span.
	//
	// Providers MUST honor this for the request that follows BeforeGenerate
	// but MUST keep using the original ctx for AfterGenerate so observers
	// that pair Before/After by ctx identity continue to work.
	UpdatedCtx context.Context `json:"-"`
}

// Hook is a function that gets called during LLM operations
type HookFunc func(ctx context.Context, hookCtx *HookContext) error

// Hook is used to register callbacks for different LLM events.
type Hook struct {
	Type HookType
	Func HookFunc
}

// Hooks is a list of hooks.
type Hooks []Hook
