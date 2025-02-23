package llm

import "context"

// Hook types for different LLM events
type HookType string

const (
	BeforeGenerate HookType = "before_generate"
	AfterGenerate  HookType = "after_generate"
	OnError        HookType = "on_error"
	BeforeStream   HookType = "before_stream"
	OnStreamChunk  HookType = "on_stream_chunk"
	AfterStream    HookType = "after_stream"
)

// HookContext contains information passed to hooks
type HookContext struct {
	Type     HookType
	Messages []*Message
	Config   *GenerateConfig
	Response *Response // Only set for AfterGenerate and OnStreamChunk
	Error    error     // Only set for OnError
	Stream   Stream    // Only set for stream-related hooks
}

// Hook is a function that gets called during LLM operations
type Hook func(ctx context.Context, hookCtx *HookContext)

type Hooks map[HookType]Hook
