package toolkit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
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
	assert.Contains(t, desc, "do not persist between calls")
	assert.Contains(t, desc, "Filesystem and other external changes")
	if runtime.GOOS == "windows" {
		assert.Contains(t, desc, "cmd /C")
	} else {
		assert.Contains(t, desc, "/bin/bash -c")
		assert.NotContains(t, desc, "zsh")
	}
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
	assert.Equal(t, "hello\n", result.Content[0].Text)
	assert.Empty(t, result.Display)
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
	assert.Equal(t, "", result.Content[0].Text)

	// Test failing command
	result, err = tool.Call(ctx, &BashInput{Command: "false"})
	assert.NoError(t, err)
	assert.True(t, result.IsError) // Non-zero exit code is an error
	assert.Equal(t, "<error>Exit code 1\n</error>", result.Content[0].Text)
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

func TestBashTool_Call_ShellStateDoesNotPersist(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	baseline, err := os.Getwd()
	assert.NoError(t, err)
	temporaryDir := t.TempDir()
	quotedTemporaryDir := "'" + strings.ReplaceAll(temporaryDir, "'", "'\"'\"'") + "'"
	tool := NewBashTool(BashToolOptions{WorkspaceDir: string(filepath.Separator)})
	ctx := context.Background()

	result, err := tool.Call(ctx, &BashInput{
		Command: "cd " + quotedTemporaryDir + " && pwd && export DIVE_BASH_STATE_TEST=value && false",
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, temporaryDir)

	result, err = tool.Call(ctx, &BashInput{
		Command: `printf 'status=%s\n' "$?"; pwd; printf 'env=%s\n' "${DIVE_BASH_STATE_TEST-unset}"`,
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, "status=0\n"+baseline+"\nenv=unset\n", result.Content[0].Text)
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
	assert.Equal(t, "error\n", result.Content[0].Text)
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
	assert.Equal(t, "hello\n", result.Content[0].Text)
	assert.Empty(t, result.Display)
}

func TestBashTool_Call_InterleavesStdoutAndStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool()
	result, err := tool.Call(context.Background(), &BashInput{
		Command: "printf 'out 1\\n'; printf 'err 1\\n' >&2; printf 'out 2\\n'; printf 'err 2\\n' >&2",
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, "out 1\nerr 1\nout 2\nerr 2\n", result.Content[0].Text)
}

func TestBashTool_Call_FailureWrapsMergedOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool()
	result, err := tool.Call(context.Background(), &BashInput{
		Command: "printf 'hello stdout\\n'; printf 'hello stderr' >&2; exit 3",
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Equal(t,
		"<error>Exit code 3\nhello stdout\nhello stderr</error>",
		result.Content[0].Text,
	)
}

func TestBashTool_Call_StreamsMergedOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	var streamed strings.Builder
	var progressCalls int
	ctx := dive.WithToolStreamFunc(context.Background(), func(_ string, text string) {
		streamed.WriteString(text)
	})
	ctx = dive.WithToolProgressFunc(ctx, func(_ string, _ *dive.ToolProgress) {
		progressCalls++
	})
	tool := NewBashTool()
	result, err := tool.Call(ctx, &BashInput{
		Command: "printf 'out\\n'; printf 'err\\n' >&2",
	})
	assert.NoError(t, err)
	assert.Equal(t, result.Content[0].Text, streamed.String())
	assert.Equal(t, "out\nerr\n", streamed.String())
	assert.Equal(t, 0, progressCalls, "Bash should expose only the text stream, not structured progress metadata")
}

func TestBashTool_Call_TruncatesCombinedOutputOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool(BashToolOptions{MaxOutputLength: 8})
	result, err := tool.Call(context.Background(), &BashInput{
		Command: "printf '12345'; printf '67890' >&2",
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, "12345678\n... (output truncated)", result.Content[0].Text)
}

func TestBashTool_Call_DrainsOutputAfterCaptureLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool(BashToolOptions{MaxOutputLength: 64})
	result, err := tool.Call(context.Background(), &BashInput{
		Command: "for ((i=0; i<20000; i++)); do printf x; done",
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, 64+len(outputTruncationMarker), len(result.Content[0].Text))
	assert.Equal(t, 1, strings.Count(result.Content[0].Text, outputTruncationMarker))
}

func TestBashTool_Call_TimeoutTerminatesDescendantsHoldingOutputPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process-group behavior")
	}

	tool := NewBashTool()
	started := time.Now()
	result, err := tool.Call(context.Background(), &BashInput{
		Command: "sleep 30 &",
		Timeout: 100,
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Less(t, time.Since(started), 2*time.Second,
		"an inherited output pipe must not delay timeout handling")
}

func TestBoundedOutputRetainsOnlyConfiguredLimit(t *testing.T) {
	output := boundedOutput{limit: 8}

	n, err := output.Write([]byte("12345"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	n, err = output.Write([]byte("67890"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n, "discarded bytes are still consumed from the pipe")
	assert.Equal(t, 8, output.buf.Len())
	assert.Equal(t, "12345678"+outputTruncationMarker, output.String())
	assert.Equal(t, 1, strings.Count(output.String(), outputTruncationMarker))
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
	// Timeout kills the command, whose numeric signal exit is -1.
	assert.Contains(t, result.Content[0].Text, "<error>Exit code -1\n")
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

func TestBashTool_Call_LongLineIsMarkedAsTruncated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - bash not available")
	}

	tool := NewBashTool()
	ctx := context.Background()

	// Chunked reads preserve non-newline output and apply the normal marked
	// truncation policy; there is no scanner line limit.
	result, err := tool.Call(ctx, &BashInput{
		Command: "head -c 2000000 /dev/zero | tr '\\0' 'a'",
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "... (output truncated)")
	assert.LessOrEqual(t, len(result.Content[0].Text), DefaultMaxOutputLength+40)
}
