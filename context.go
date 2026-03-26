package dive

import "context"

type contextKey string

const (
	toolCallIDKey   contextKey = "tool_call_id"
	toolStreamFnKey contextKey = "tool_stream_fn"
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
