package toolkit

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

func TestKillShellTool_Name(t *testing.T) {
	sm := NewShellManager()
	tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})
	assert.Equal(t, "KillShell", tool.Name())
}

func TestKillShellTool_Description(t *testing.T) {
	sm := NewShellManager()
	tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})
	desc := tool.Description()
	assert.Contains(t, desc, "Terminate")
	assert.Contains(t, desc, "shell_id")
}

func TestKillShellTool_Schema(t *testing.T) {
	sm := NewShellManager()
	tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})
	s := tool.Schema()

	assert.Equal(t, schema.Object, s.Type)
	assert.Contains(t, s.Required, "shell_id")
	assert.Contains(t, s.Properties, "shell_id")
}

func TestKillShellTool_Call(t *testing.T) {
	ctx := context.Background()

	t.Run("KillRunningShell", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})

		// Start a long-running command
		var cmd string
		var args []string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
			args = []string{"/c", "ping", "-n", "100", "127.0.0.1"}
		} else {
			cmd = "sleep"
			args = []string{"100"}
		}

		id, err := sm.StartBackground(ctx, cmd, args, "long running", "")
		assert.NoError(t, err)

		// Verify it's running
		assert.True(t, sm.IsRunning(id))

		// Kill it
		input := &KillShellInput{ShellID: id}
		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		assert.NoError(t, err)
		assert.Equal(t, "killed", response["status"])
		assert.Equal(t, id, response["shell_id"])

		// Verify it's no longer running
		time.Sleep(200 * time.Millisecond)
		assert.False(t, sm.IsRunning(id))
	})

	t.Run("KillNotRunningShell", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})

		// Start a quick command
		var cmd string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
		} else {
			cmd = "true"
		}

		id, err := sm.StartBackground(ctx, cmd, nil, "", "")
		assert.NoError(t, err)

		// Wait for it to complete
		time.Sleep(100 * time.Millisecond)

		// Try to kill it
		input := &KillShellInput{ShellID: id}
		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		assert.NoError(t, err)
		assert.Equal(t, "shell is not running", response["message"])
	})

	t.Run("KillNonexistentShell", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})

		input := &KillShellInput{ShellID: "nonexistent"}
		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "shell not found")
	})

	t.Run("MissingShellID", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})

		input := &KillShellInput{ShellID: ""}
		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "shell_id is required")
	})

	t.Run("NoShellManager", func(t *testing.T) {
		tool := NewKillShellTool(KillShellToolOptions{ShellManager: nil})

		input := &KillShellInput{ShellID: "test"}
		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "shell manager not configured")
	})
}

func TestKillShellTool_PreviewCall(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()
	tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})

	input := &KillShellInput{ShellID: "shell-123"}
	preview := tool.PreviewCall(ctx, input)

	assert.Contains(t, preview.Summary, "Kill shell")
	assert.Contains(t, preview.Summary, "shell-123")
}

func TestKillShellTool_Annotations(t *testing.T) {
	sm := NewShellManager()
	tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})
	annotations := tool.Annotations()

	assert.NotNil(t, annotations)
	assert.Equal(t, "KillShell", annotations.Title)
	assert.False(t, annotations.ReadOnlyHint)
	assert.True(t, annotations.DestructiveHint)
}
