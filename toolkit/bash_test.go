package toolkit

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestBashTool_Name(t *testing.T) {
	tool := NewBashTool()
	assert.Equal(t, "Bash", tool.Name())
}

func TestBashTool_Description(t *testing.T) {
	tool := NewBashTool()
	desc := tool.Description()
	assert.Contains(t, desc, "persistent bash session")
	assert.Contains(t, desc, "bash_20250124")
	assert.Contains(t, desc, runtime.GOOS)
}

func TestBashTool_Schema(t *testing.T) {
	tool := NewBashTool()
	schema := tool.Schema()

	assert.NotNil(t, schema)
	assert.Equal(t, "object", string(schema.Type))

	// Check properties exist
	assert.Contains(t, schema.Properties, "command")
	assert.Contains(t, schema.Properties, "restart")
	assert.Contains(t, schema.Properties, "timeout")
	assert.Contains(t, schema.Properties, "description")
	assert.Contains(t, schema.Properties, "working_directory")
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

	// Test restart preview
	t.Run("restart", func(t *testing.T) {
		preview := tool.Unwrap().(*BashTool).PreviewCall(ctx, &BashInput{Restart: true})
		assert.Equal(t, "Restart bash session", preview.Summary)
	})

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

func TestBashTool_Call_Restart(t *testing.T) {
	tool := NewBashTool()
	ctx := context.Background()

	result, err := tool.Call(ctx, &BashInput{Restart: true})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "restarted")
	assert.Equal(t, "Bash session restarted", result.Display)
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

	// Cleanup
	tool.Unwrap().(*BashTool).Close()
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

	// Cleanup
	tool.Unwrap().(*BashTool).Close()
}

func TestBashTool_Call_PersistentSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool()
	ctx := context.Background()

	// Set environment variable
	result, err := tool.Call(ctx, &BashInput{Command: "export MY_VAR=test123"})
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	// Verify it persists
	result, err = tool.Call(ctx, &BashInput{Command: "echo $MY_VAR"})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "test123")

	// Cleanup
	tool.Unwrap().(*BashTool).Close()
}

func TestBashTool_Call_WorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool()
	ctx := context.Background()

	// Change directory
	result, err := tool.Call(ctx, &BashInput{Command: "cd /tmp && pwd"})
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	// Verify working directory persists
	result, err = tool.Call(ctx, &BashInput{Command: "pwd"})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "/tmp")

	// Cleanup
	tool.Unwrap().(*BashTool).Close()
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

	// Cleanup
	tool.Unwrap().(*BashTool).Close()
}

func TestBashTool_Call_RestartClearsState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool()
	ctx := context.Background()

	// Set variable
	_, err := tool.Call(ctx, &BashInput{Command: "export MY_VAR=before"})
	assert.NoError(t, err)

	// Restart session
	result, err := tool.Call(ctx, &BashInput{Restart: true})
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	// Verify variable is gone (after restart, need new command to start session)
	result, err = tool.Call(ctx, &BashInput{Command: "echo ${MY_VAR:-unset}"})
	assert.NoError(t, err)
	assert.Contains(t, result.Content[0].Text, "unset")

	// Cleanup
	tool.Unwrap().(*BashTool).Close()
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

	// Cleanup
	tool.Unwrap().(*BashTool).Close()
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
	assert.Contains(t, result.Content[0].Text, "timed out")

	// Allow time for cleanup
	time.Sleep(200 * time.Millisecond)

	// Cleanup (create new tool since session may be in bad state)
}

func TestBashSession_Execute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	session, err := NewBashSession()
	assert.NoError(t, err)
	defer session.Close()

	ctx := context.Background()

	// Test simple command
	stdout, stderr, exitCode, err := session.Execute(ctx, "echo hello", 5*time.Second)
	assert.NoError(t, err)
	assert.Contains(t, stdout, "hello")
	assert.Empty(t, stderr)
	assert.Equal(t, 0, exitCode)
}

func TestBashSession_Restart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	session, err := NewBashSession()
	assert.NoError(t, err)
	defer session.Close()

	ctx := context.Background()

	// Set variable
	_, _, _, err = session.Execute(ctx, "export TEST_VAR=hello", 5*time.Second)
	assert.NoError(t, err)

	// Restart
	err = session.Restart()
	assert.NoError(t, err)

	// Check variable is gone
	stdout, _, _, err := session.Execute(ctx, "echo ${TEST_VAR:-unset}", 5*time.Second)
	assert.NoError(t, err)
	assert.Contains(t, stdout, "unset")
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
