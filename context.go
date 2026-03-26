package dive

import "context"

type contextKey string

const toolCallIDKey contextKey = "tool_call_id"

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
