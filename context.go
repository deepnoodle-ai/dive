package dive

import "context"

type contextKey string

const (
	toolCallIDKey   contextKey = "tool_call_id"
	toolStreamFnKey contextKey = "tool_stream_fn"
	toolUpdateFnKey contextKey = "tool_update_fn"
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

// WithToolUpdateFunc returns a context with a structured tool-update function.
// This is set by the agent before calling a tool, enabling tools to emit
// typed partial results (exit code, byte count, parsed progress) in addition
// to or instead of text-only StreamOutput.
//
// Tools call UpdateTool to deliver an update; the agent re-emits it as a
// ResponseItem of type ResponseItemTypeToolUpdate carrying the ToolUpdate.
func WithToolUpdateFunc(ctx context.Context, fn func(toolCallID string, update *ToolUpdate)) context.Context {
	return context.WithValue(ctx, toolUpdateFnKey, fn)
}

// UpdateTool delivers a structured partial result snapshot from a running
// tool. Tools call this whenever they have a typed progress update worth
// surfacing to UIs or evaluators — e.g. on each stdout chunk, after each
// scanned file, or on a periodic percent-complete tick.
//
// Safe to call even if no update function is configured (it's a no-op) and
// safe to call any number of times during a single tool execution. Nil
// updates are dropped.
//
// UpdateTool and StreamOutput are independent channels: tools may use one,
// the other, or both. StreamOutput carries text-only chunks; UpdateTool
// carries structured ToolUpdate snapshots.
func UpdateTool(ctx context.Context, update *ToolUpdate) {
	if update == nil {
		return
	}
	fn, ok := ctx.Value(toolUpdateFnKey).(func(string, *ToolUpdate))
	if !ok || fn == nil {
		return
	}
	fn(ToolCallID(ctx), update)
}
