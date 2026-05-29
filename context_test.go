package dive

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

// TestReportProgress_NoOpWithoutContextFunc verifies ReportProgress is safe to
// call when no progress function is configured — tools should not need to check.
func TestReportProgress_NoOpWithoutContextFunc(t *testing.T) {
	ctx := context.Background()
	// No panic, no error — just a no-op.
	ReportProgress(ctx, &ToolProgress{Display: "ignored"})
}

// TestReportProgress_NilProgressDropped verifies nil snapshots are silently dropped.
func TestReportProgress_NilProgressDropped(t *testing.T) {
	var called bool
	ctx := WithToolCallID(context.Background(), "call-1")
	ctx = WithToolProgressFunc(ctx, func(string, *ToolProgress) { called = true })
	ReportProgress(ctx, nil)
	assert.False(t, called, "nil progress should be dropped")
}

// TestReportProgress_DeliversSnapshot verifies that a snapshot flows through the
// configured function with the tool-call ID injected.
func TestReportProgress_DeliversSnapshot(t *testing.T) {
	var gotID string
	var gotProgress *ToolProgress

	ctx := WithToolCallID(context.Background(), "call-42")
	ctx = WithToolProgressFunc(ctx, func(id string, p *ToolProgress) {
		gotID = id
		gotProgress = p
	})

	ReportProgress(ctx, &ToolProgress{
		Display: "scanning… 12/100 files",
		Metadata: map[string]any{
			"files_scanned":   12,
			"files_total":     100,
			"bytes_processed": 4096,
		},
	})

	assert.Equal(t, "call-42", gotID)
	assert.NotNil(t, gotProgress)
	assert.Equal(t, "scanning… 12/100 files", gotProgress.Display)
	assert.Equal(t, 12, gotProgress.Metadata["files_scanned"])
	assert.Equal(t, 100, gotProgress.Metadata["files_total"])
	assert.Equal(t, 4096, gotProgress.Metadata["bytes_processed"])
}

// TestStreamOutputAndReportProgress_AreIndependent verifies the text-only and
// structured channels coexist: a tool can use either, both, or neither, and
// they don't interfere.
func TestStreamOutputAndReportProgress_AreIndependent(t *testing.T) {
	var streamCalls int
	var progressCalls int

	ctx := WithToolCallID(context.Background(), "call-99")
	ctx = WithToolStreamFunc(ctx, func(string, string) { streamCalls++ })
	ctx = WithToolProgressFunc(ctx, func(string, *ToolProgress) { progressCalls++ })

	StreamOutput(ctx, "stdout line 1\n")
	StreamOutput(ctx, "stdout line 2\n")
	ReportProgress(ctx, &ToolProgress{Metadata: map[string]any{"exit_code": nil}})

	assert.Equal(t, 2, streamCalls)
	assert.Equal(t, 1, progressCalls)
}
