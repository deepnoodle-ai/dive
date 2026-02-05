package interactor

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestAutoApprove(t *testing.T) {
	interactor := &AutoApprove{}

	t.Run("Confirm always returns true", func(t *testing.T) {
		result, err := interactor.Confirm(context.Background(), &ConfirmRequest{
			Title:   "Delete file?",
			Message: "This will permanently delete the file.",
		})
		assert.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("Select returns default option", func(t *testing.T) {
		resp, err := interactor.Select(context.Background(), &SelectRequest{
			Title: "Choose an option",
			Options: []SelectOption{
				{Value: "a", Label: "Option A"},
				{Value: "b", Label: "Option B", Default: true},
				{Value: "c", Label: "Option C"},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, "b", resp.Value)
		assert.False(t, resp.Canceled)
	})

	t.Run("Select returns first option if no default", func(t *testing.T) {
		resp, err := interactor.Select(context.Background(), &SelectRequest{
			Title: "Choose an option",
			Options: []SelectOption{
				{Value: "a", Label: "Option A"},
				{Value: "b", Label: "Option B"},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, "a", resp.Value)
	})

	t.Run("Select returns canceled if no options", func(t *testing.T) {
		resp, err := interactor.Select(context.Background(), &SelectRequest{
			Title:   "Choose an option",
			Options: []SelectOption{},
		})
		assert.NoError(t, err)
		assert.True(t, resp.Canceled)
	})

	t.Run("MultiSelect returns default options", func(t *testing.T) {
		resp, err := interactor.MultiSelect(context.Background(), &MultiSelectRequest{
			Title: "Select features",
			Options: []SelectOption{
				{Value: "a", Label: "Feature A", Default: true},
				{Value: "b", Label: "Feature B"},
				{Value: "c", Label: "Feature C", Default: true},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, []string{"a", "c"}, resp.Values)
		assert.False(t, resp.Canceled)
	})

	t.Run("MultiSelect respects MinSelect", func(t *testing.T) {
		resp, err := interactor.MultiSelect(context.Background(), &MultiSelectRequest{
			Title:     "Select features",
			MinSelect: 2,
			Options: []SelectOption{
				{Value: "a", Label: "Feature A", Default: true},
				{Value: "b", Label: "Feature B"},
				{Value: "c", Label: "Feature C"},
			},
		})
		assert.NoError(t, err)
		assert.True(t, len(resp.Values) >= 2)
	})

	t.Run("MultiSelect returns empty if MinSelect is 0", func(t *testing.T) {
		resp, err := interactor.MultiSelect(context.Background(), &MultiSelectRequest{
			Title:     "Select features",
			MinSelect: 0,
			Options: []SelectOption{
				{Value: "a", Label: "Feature A"},
				{Value: "b", Label: "Feature B"},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, 0, len(resp.Values))
	})

	t.Run("Input returns default value", func(t *testing.T) {
		resp, err := interactor.Input(context.Background(), &InputRequest{
			Title:   "Enter name",
			Default: "John",
		})
		assert.NoError(t, err)
		assert.Equal(t, "John", resp.Value)
		assert.False(t, resp.Canceled)
	})
}

func TestDenyAll(t *testing.T) {
	interactor := &DenyAll{}

	t.Run("Confirm always returns false", func(t *testing.T) {
		result, err := interactor.Confirm(context.Background(), &ConfirmRequest{
			Title:   "Delete file?",
			Message: "This will permanently delete the file.",
		})
		assert.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("Select always returns canceled", func(t *testing.T) {
		resp, err := interactor.Select(context.Background(), &SelectRequest{
			Title: "Choose an option",
			Options: []SelectOption{
				{Value: "a", Label: "Option A"},
			},
		})
		assert.NoError(t, err)
		assert.True(t, resp.Canceled)
	})

	t.Run("MultiSelect always returns canceled", func(t *testing.T) {
		resp, err := interactor.MultiSelect(context.Background(), &MultiSelectRequest{
			Title: "Select features",
			Options: []SelectOption{
				{Value: "a", Label: "Feature A"},
			},
		})
		assert.NoError(t, err)
		assert.True(t, resp.Canceled)
	})

	t.Run("Input always returns canceled", func(t *testing.T) {
		resp, err := interactor.Input(context.Background(), &InputRequest{
			Title:   "Enter name",
			Default: "John",
		})
		assert.NoError(t, err)
		assert.True(t, resp.Canceled)
	})
}

func TestTerminal(t *testing.T) {
	t.Run("NewTerminal with default mode", func(t *testing.T) {
		terminal := NewTerminal(TerminalOptions{})
		assert.Equal(t, ModeIfNotReadOnly, terminal.Mode)
	})

	t.Run("NewTerminal with custom mode", func(t *testing.T) {
		terminal := NewTerminal(TerminalOptions{Mode: ModeAlways})
		assert.Equal(t, ModeAlways, terminal.Mode)
	})

	t.Run("ShouldInteract ModeNever", func(t *testing.T) {
		terminal := &Terminal{Mode: ModeNever}
		assert.False(t, terminal.ShouldInteract(nil))
	})

	t.Run("ShouldInteract ModeAlways", func(t *testing.T) {
		terminal := &Terminal{Mode: ModeAlways}
		assert.True(t, terminal.ShouldInteract(nil))
	})

	t.Run("ShouldInteract ModeIfDestructive with destructive tool", func(t *testing.T) {
		terminal := &Terminal{Mode: ModeIfDestructive}
		tool := &mockTool{annotations: &dive.ToolAnnotations{DestructiveHint: true}}
		assert.True(t, terminal.ShouldInteract(tool))
	})

	t.Run("ShouldInteract ModeIfDestructive with non-destructive tool", func(t *testing.T) {
		terminal := &Terminal{Mode: ModeIfDestructive}
		tool := &mockTool{annotations: &dive.ToolAnnotations{DestructiveHint: false}}
		assert.False(t, terminal.ShouldInteract(tool))
	})

	t.Run("ShouldInteract ModeIfNotReadOnly with read-only tool", func(t *testing.T) {
		terminal := &Terminal{Mode: ModeIfNotReadOnly}
		tool := &mockTool{annotations: &dive.ToolAnnotations{ReadOnlyHint: true}}
		assert.False(t, terminal.ShouldInteract(tool))
	})

	t.Run("ShouldInteract ModeIfNotReadOnly with non-read-only tool", func(t *testing.T) {
		terminal := &Terminal{Mode: ModeIfNotReadOnly}
		tool := &mockTool{annotations: &dive.ToolAnnotations{ReadOnlyHint: false}}
		assert.True(t, terminal.ShouldInteract(tool))
	})

	t.Run("ShouldInteract with nil tool", func(t *testing.T) {
		terminal := &Terminal{Mode: ModeIfDestructive}
		assert.True(t, terminal.ShouldInteract(nil))
	})

	t.Run("ShouldInteract with nil annotations", func(t *testing.T) {
		terminal := &Terminal{Mode: ModeIfDestructive}
		tool := &mockTool{annotations: nil}
		assert.True(t, terminal.ShouldInteract(tool))
	})
}


// mockTool implements dive.Tool for testing Terminal.ShouldInteract
type mockTool struct {
	annotations *dive.ToolAnnotations
}

func (m *mockTool) Name() string                                   { return "mock" }
func (m *mockTool) Description() string                            { return "Mock tool" }
func (m *mockTool) Schema() *dive.Schema                           { return nil }
func (m *mockTool) Annotations() *dive.ToolAnnotations             { return m.annotations }
func (m *mockTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, nil
}
