package toolkit

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/schema"
	"github.com/stretchr/testify/require"
)

func TestKillShellTool_Name(t *testing.T) {
	sm := NewShellManager()
	tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})
	require.Equal(t, "kill_shell", tool.Name())
}

func TestKillShellTool_Description(t *testing.T) {
	sm := NewShellManager()
	tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})
	desc := tool.Description()
	require.Contains(t, desc, "Terminate")
	require.Contains(t, desc, "shell_id")
}

func TestKillShellTool_Schema(t *testing.T) {
	sm := NewShellManager()
	tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})
	s := tool.Schema()

	require.Equal(t, schema.Object, s.Type)
	require.Contains(t, s.Required, "shell_id")
	require.Contains(t, s.Properties, "shell_id")
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
		require.NoError(t, err)

		// Verify it's running
		require.True(t, sm.IsRunning(id))

		// Kill it
		input := &KillShellInput{ShellID: id}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)
		require.Equal(t, "killed", response["status"])
		require.Equal(t, id, response["shell_id"])

		// Verify it's no longer running
		time.Sleep(200 * time.Millisecond)
		require.False(t, sm.IsRunning(id))
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
		require.NoError(t, err)

		// Wait for it to complete
		time.Sleep(100 * time.Millisecond)

		// Try to kill it
		input := &KillShellInput{ShellID: id}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)
		require.Equal(t, "shell is not running", response["message"])
	})

	t.Run("KillNonexistentShell", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})

		input := &KillShellInput{ShellID: "nonexistent"}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "shell not found")
	})

	t.Run("MissingShellID", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})

		input := &KillShellInput{ShellID: ""}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "shell_id is required")
	})

	t.Run("NoShellManager", func(t *testing.T) {
		tool := NewKillShellTool(KillShellToolOptions{ShellManager: nil})

		input := &KillShellInput{ShellID: "test"}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "shell manager not configured")
	})
}

func TestKillShellTool_PreviewCall(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()
	tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})

	input := &KillShellInput{ShellID: "shell-123"}
	preview := tool.PreviewCall(ctx, input)

	require.Contains(t, preview.Summary, "Kill shell")
	require.Contains(t, preview.Summary, "shell-123")
}

func TestKillShellTool_Annotations(t *testing.T) {
	sm := NewShellManager()
	tool := NewKillShellTool(KillShellToolOptions{ShellManager: sm})
	annotations := tool.Annotations()

	require.NotNil(t, annotations)
	require.Equal(t, "Kill Shell", annotations.Title)
	require.False(t, annotations.ReadOnlyHint)
	require.True(t, annotations.DestructiveHint)
}
