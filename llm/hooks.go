package llm

import "context"

// Hook types for different LLM events
type HookType string

const (
	BeforeGenerate HookType = "before_generate"
	AfterGenerate  HookType = "after_generate"
	OnError        HookType = "on_error"
)

// HookContext contains information passed to hooks.
type HookContext struct {
	Type     HookType
	Request  *Request
	Response *Response
	Error    error
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
