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
}

// HookResponseContext contains information about a response from an LLM.
type HookResponseContext struct {
	Response *Response `json:"response,omitempty"`
	Error    error     `json:"error,omitempty"`
}

// HookContext contains information passed to hooks.
type HookContext struct {
	Type     HookType             `json:"type"`
	Request  *HookRequestContext  `json:"request,omitempty"`
	Response *HookResponseContext `json:"response,omitempty"`
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
