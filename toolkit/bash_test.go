package toolkit

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestBashTool_Name(t *testing.T) {
	tool := NewBashTool()
	assert.Equal(t, "Bash", tool.Name())
}

func TestBashTool_Description(t *testing.T) {
	tool := NewBashTool()
	desc := tool.Description()
	assert.Contains(t, desc, "Execute shell commands")
	assert.Contains(t, desc, runtime.GOOS)
}

func TestBashTool_Schema(t *testing.T) {
	tool := NewBashTool()
	schema := tool.Schema()

	assert.NotNil(t, schema)
	assert.Equal(t, "object", string(schema.Type))

	// Check properties exist
	assert.Contains(t, schema.Properties, "command")
	assert.Contains(t, schema.Properties, "timeout")
	assert.Contains(t, schema.Properties, "description")
	assert.Contains(t, schema.Properties, "working_directory")

	// Command is required
	assert.Contains(t, schema.Required, "command")
}

func TestBashTool_Annotations(t *testing.T) {
	tool := NewBashTool()
	annotations := tool.Annotations()

	assert.Equal(t, "Bash", annotations.Title)
	assert.False(t, annotations.ReadOnlyHint)
	assert.False(t, annotations.IdempotentHint)
	assert.True(t, annotations.DestructiveHint)
	assert.True(t, annotations.OpenWorldHint)
}

func TestBashTool_PreviewCall(t *testing.T) {
	tool := NewBashTool()
	ctx := context.Background()

	// Test command preview
	t.Run("command", func(t *testing.T) {
		preview := tool.Unwrap().(*BashTool).PreviewCall(ctx, &BashInput{Command: "echo hello"})
		assert.Contains(t, preview.Summary, "echo hello")
	})

	// Test description override
	t.Run("description", func(t *testing.T) {
		preview := tool.Unwrap().(*BashTool).PreviewCall(ctx, &BashInput{
			Command:     "echo hello",
			Description: "Print greeting",
		})
		assert.Equal(t, "Print greeting", preview.Summary)
	})

	// Test long command truncation
	t.Run("long command", func(t *testing.T) {
		longCmd := strings.Repeat("x", 100)
		preview := tool.Unwrap().(*BashTool).PreviewCall(ctx, &BashInput{Command: longCmd})
		assert.Less(t, len(preview.Summary), 70)
		assert.Contains(t, preview.Summary, "...")
	})
}

func TestBashTool_Call_EmptyCommand(t *testing.T) {
	tool := NewBashTool()
	ctx := context.Background()

	result, err := tool.Call(ctx, &BashInput{})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "'command' is required")
}

func TestBashTool_Call_SimpleCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool()
	ctx := context.Background()

	result, err := tool.Call(ctx, &BashInput{Command: "echo hello"})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "hello")
	assert.Contains(t, result.Content[0].Text, "return_code")
}

func TestBashTool_Call_CommandWithExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool()
	ctx := context.Background()

	// Test successful command
	result, err := tool.Call(ctx, &BashInput{Command: "true"})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, `"return_code":0`)

	// Test failing command
	result, err = tool.Call(ctx, &BashInput{Command: "false"})
	assert.NoError(t, err)
	assert.True(t, result.IsError) // Non-zero exit code is an error
	assert.Contains(t, result.Content[0].Text, `"return_code":1`)
}

func TestBashTool_Call_WorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	// Create a temp dir within the workspace
	tempDir := t.TempDir()
	tool := NewBashTool(BashToolOptions{
		WorkspaceDir: tempDir,
	})
	ctx := context.Background()

	result, err := tool.Call(ctx, &BashInput{
		Command:          "pwd",
		WorkingDirectory: tempDir,
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, tempDir)
}

func TestBashTool_Call_Stderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool()
	ctx := context.Background()

	// Command that writes to stderr
	result, err := tool.Call(ctx, &BashInput{Command: "echo error >&2"})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "stderr")
	assert.Contains(t, result.Content[0].Text, "error")
}

func TestBashTool_Call_WithDescription(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool()
	ctx := context.Background()

	result, err := tool.Call(ctx, &BashInput{
		Command:     "echo hello",
		Description: "Print greeting",
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Display, "Print greeting")
	assert.Contains(t, result.Display, "exit 0")
}

func TestBashTool_Call_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool()
	ctx := context.Background()

	// Command that takes too long with very short timeout
	result, err := tool.Call(ctx, &BashInput{
		Command: "sleep 10",
		Timeout: 100, // 100ms timeout
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	// Timeout results in exit code -1 due to signal
	assert.Contains(t, result.Content[0].Text, `"return_code":-1`)
}

func TestTruncateCommand(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"hello\nworld", 20, "hello world"},
		{"", 10, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateCommand(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateOutput(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello\n... (output truncated)"},
		{"", 10, ""},
		{"abc", 0, "abc"}, // 0 means no limit
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateOutput(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBashTool_Call_ReturnsConfigError(t *testing.T) {
	tool := &BashTool{
		configErr: errors.New("validator init failed"),
	}

	result, err := tool.Call(context.Background(), &BashInput{Command: "echo hello"})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "validator init failed")
}

func TestBashTool_Call_ReturnsWorkspaceConfigErrorWhenValidatorMissing(t *testing.T) {
	tool := &BashTool{workspaceDir: "/bad/workspace"}

	result, err := tool.Call(context.Background(), &BashInput{Command: "echo hello"})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "WorkspaceDir \"/bad/workspace\"")
	assert.Contains(t, result.Content[0].Text, "path validator is not initialized")
}
