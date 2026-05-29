package dive

import "context"

type contextKey string

const (
	toolCallIDKey     contextKey = "tool_call_id"
	toolStreamFnKey   contextKey = "tool_stream_fn"
	toolProgressFnKey contextKey = "tool_progress_fn"
)

// WithToolCallID returns a context with the given tool call ID.
// This is set by the agent before calling a tool, so tools can
// associate state with their specific invocation.
func WithToolCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, toolCallIDKey, id)
}

// ToolCallID returns the tool call ID from the context, or empty string.
func ToolCallID(ctx context.Context) string {
	if id, ok := ctx.Value(toolCallIDKey).(string); ok {
		return id
	}
	return ""
}

// WithToolStreamFunc returns a context with a streaming output function.
// This is set by the agent before calling a tool, enabling tools to
// stream incremental output to the UI during execution.
func WithToolStreamFunc(ctx context.Context, fn func(toolCallID string, text string)) context.Context {
	return context.WithValue(ctx, toolStreamFnKey, fn)
}

// StreamOutput sends a chunk of streaming output from a tool to the UI.
// Tools call this during execution to provide real-time feedback.
// Safe to call even if no stream function is configured (it's a no-op).
func StreamOutput(ctx context.Context, text string) {
	fn, ok := ctx.Value(toolStreamFnKey).(func(string, string))
	if !ok || fn == nil {
		return
	}
	fn(ToolCallID(ctx), text)
}

// WithToolProgressFunc returns a context with a structured progress function.
// This is set by the agent before calling a tool, enabling tools to emit typed
// progress snapshots (exit code, byte count, parsed progress) in addition to
// or instead of text-only StreamOutput.
//
// Tools call ReportProgress to deliver a snapshot; the agent re-emits it as a
// ResponseItem of type ResponseItemTypeToolProgress carrying the ToolProgress.
func WithToolProgressFunc(ctx context.Context, fn func(toolCallID string, progress *ToolProgress)) context.Context {
	return context.WithValue(ctx, toolProgressFnKey, fn)
}

// ReportProgress delivers a structured progress snapshot from a running tool.
// Tools call this whenever they have a typed progress update worth surfacing to
// UIs or evaluators — e.g. on each stdout chunk, after each scanned file, or on
// a periodic percent-complete tick. Each call replaces the previous snapshot;
// it is not a delta.
//
// Safe to call even if no progress function is configured (it's a no-op) and
// safe to call any number of times during a single tool execution. Nil
// snapshots are dropped.
//
// ReportProgress and StreamOutput are independent channels: tools may use one,
// the other, or both. StreamOutput carries text deltas (append); ReportProgress
// carries structured ToolProgress snapshots (replace).
func ReportProgress(ctx context.Context, progress *ToolProgress) {
	if progress == nil {
		return
	}
	fn, ok := ctx.Value(toolProgressFnKey).(func(string, *ToolProgress))
	if !ok || fn == nil {
		return
	}
	fn(ToolCallID(ctx), progress)
}
