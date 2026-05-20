package dive

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

// TestUpdateTool_NoOpWithoutContextFunc verifies UpdateTool is safe to call
// when no update function is configured — tools should not need to check.
func TestUpdateTool_NoOpWithoutContextFunc(t *testing.T) {
	ctx := context.Background()
	// No panic, no error — just a no-op.
	UpdateTool(ctx, &ToolUpdate{Display: "ignored"})
}

// TestUpdateTool_NilUpdateDropped verifies nil updates are silently dropped.
func TestUpdateTool_NilUpdateDropped(t *testing.T) {
	var called bool
	ctx := WithToolCallID(context.Background(), "call-1")
	ctx = WithToolUpdateFunc(ctx, func(string, *ToolUpdate) { called = true })
	UpdateTool(ctx, nil)
	assert.False(t, called, "nil update should be dropped")
}

// TestUpdateTool_DeliversStructuredSnapshot verifies that an update flows
// through the configured function with the tool-call ID injected.
func TestUpdateTool_DeliversStructuredSnapshot(t *testing.T) {
	var gotID string
	var gotUpdate *ToolUpdate

	ctx := WithToolCallID(context.Background(), "call-42")
	ctx = WithToolUpdateFunc(ctx, func(id string, u *ToolUpdate) {
		gotID = id
		gotUpdate = u
	})

	UpdateTool(ctx, &ToolUpdate{
		Display: "scanning… 12/100 files",
		Metadata: map[string]any{
			"files_scanned":   12,
			"files_total":     100,
			"bytes_processed": 4096,
		},
	})

	assert.Equal(t, "call-42", gotID)
	assert.NotNil(t, gotUpdate)
	assert.Equal(t, "scanning… 12/100 files", gotUpdate.Display)
	assert.Equal(t, 12, gotUpdate.Metadata["files_scanned"])
	assert.Equal(t, 100, gotUpdate.Metadata["files_total"])
	assert.Equal(t, 4096, gotUpdate.Metadata["bytes_processed"])
}

// TestStreamOutputAndUpdateTool_AreIndependent verifies the text-only and
// structured channels coexist: a tool can use either, both, or neither, and
// they don't interfere.
func TestStreamOutputAndUpdateTool_AreIndependent(t *testing.T) {
	var streamCalls int
	var updateCalls int

	ctx := WithToolCallID(context.Background(), "call-99")
	ctx = WithToolStreamFunc(ctx, func(string, string) { streamCalls++ })
	ctx = WithToolUpdateFunc(ctx, func(string, *ToolUpdate) { updateCalls++ })

	StreamOutput(ctx, "stdout line 1\n")
	StreamOutput(ctx, "stdout line 2\n")
	UpdateTool(ctx, &ToolUpdate{Metadata: map[string]any{"exit_code": nil}})

	assert.Equal(t, 2, streamCalls)
	assert.Equal(t, 1, updateCalls)
}
