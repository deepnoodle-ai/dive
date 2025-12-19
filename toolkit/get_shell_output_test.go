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

func TestGetShellOutputTool_Name(t *testing.T) {
	sm := NewShellManager()
	tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})
	require.Equal(t, "get_shell_output", tool.Name())
}

func TestGetShellOutputTool_Description(t *testing.T) {
	sm := NewShellManager()
	tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})
	desc := tool.Description()
	require.Contains(t, desc, "output")
	require.Contains(t, desc, "shell_id")
	require.Contains(t, desc, "block")
	require.Contains(t, desc, "timeout")
}

func TestGetShellOutputTool_Schema(t *testing.T) {
	sm := NewShellManager()
	tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})
	s := tool.Schema()

	require.Equal(t, schema.Object, s.Type)
	require.Contains(t, s.Required, "shell_id")
	require.Contains(t, s.Properties, "shell_id")
	require.Contains(t, s.Properties, "block")
	require.Contains(t, s.Properties, "timeout")
}

func TestGetShellOutputTool_Call(t *testing.T) {
	ctx := context.Background()

	t.Run("GetOutputBlocking", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})

		var cmd string
		var args []string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
			args = []string{"/c", "echo", "hello world"}
		} else {
			cmd = "echo"
			args = []string{"hello world"}
		}

		id, err := sm.StartBackground(ctx, cmd, args, "echo test", "")
		require.NoError(t, err)

		blockTrue := true
		input := &GetShellOutputInput{
			ShellID: id,
			Block:   &blockTrue,
			Timeout: 5000,
		}

		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Equal(t, id, response["shell_id"])
		require.Equal(t, "completed", response["status"])
		require.Contains(t, response["stdout"], "hello world")
		require.Equal(t, float64(0), response["exit_code"])
	})

	t.Run("GetOutputNonBlocking", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})

		var cmd string
		var args []string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
			args = []string{"/c", "ping", "-n", "5", "127.0.0.1"}
		} else {
			cmd = "sleep"
			args = []string{"5"}
		}

		id, err := sm.StartBackground(ctx, cmd, args, "", "")
		require.NoError(t, err)

		blockFalse := false
		input := &GetShellOutputInput{
			ShellID: id,
			Block:   &blockFalse,
		}

		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Equal(t, "running", response["status"])

		// Clean up
		sm.Kill(id)
	})

	t.Run("GetOutputDefaultBlocking", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})

		var cmd string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
		} else {
			cmd = "true"
		}

		id, err := sm.StartBackground(ctx, cmd, nil, "", "")
		require.NoError(t, err)

		// Don't set Block - should default to true
		input := &GetShellOutputInput{
			ShellID: id,
			Timeout: 5000,
		}

		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Equal(t, "completed", response["status"])
	})

	t.Run("GetOutputNonexistent", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})

		input := &GetShellOutputInput{ShellID: "nonexistent"}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "error getting shell output")
	})

	t.Run("MissingShellID", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})

		input := &GetShellOutputInput{ShellID: ""}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "shell_id is required")
	})

	t.Run("NoShellManager", func(t *testing.T) {
		tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: nil})

		input := &GetShellOutputInput{ShellID: "test"}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "shell manager not configured")
	})

	t.Run("FailedCommand", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})

		var cmd string
		var args []string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
			args = []string{"/c", "exit", "1"}
		} else {
			cmd = "false"
			args = nil
		}

		id, err := sm.StartBackground(ctx, cmd, args, "", "")
		require.NoError(t, err)

		input := &GetShellOutputInput{
			ShellID: id,
			Timeout: 5000,
		}

		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Equal(t, "failed", response["status"])
		require.NotEqual(t, float64(0), response["exit_code"])
	})
}

func TestGetShellOutputTool_PreviewCall(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()
	tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})

	t.Run("Blocking", func(t *testing.T) {
		blockTrue := true
		input := &GetShellOutputInput{ShellID: "shell-123", Block: &blockTrue}
		preview := tool.PreviewCall(ctx, input)
		require.Contains(t, preview.Summary, "shell-123")
		require.Contains(t, preview.Summary, "blocking")
	})

	t.Run("NonBlocking", func(t *testing.T) {
		blockFalse := false
		input := &GetShellOutputInput{ShellID: "shell-456", Block: &blockFalse}
		preview := tool.PreviewCall(ctx, input)
		require.Contains(t, preview.Summary, "shell-456")
		require.Contains(t, preview.Summary, "non-blocking")
	})
}

func TestGetShellOutputTool_Annotations(t *testing.T) {
	sm := NewShellManager()
	tool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})
	annotations := tool.Annotations()

	require.NotNil(t, annotations)
	require.Equal(t, "Get Shell Output", annotations.Title)
	require.True(t, annotations.ReadOnlyHint)
	require.False(t, annotations.DestructiveHint)
}

// ListShellsTool tests

func TestListShellsTool_Name(t *testing.T) {
	sm := NewShellManager()
	tool := NewListShellsTool(ListShellsToolOptions{ShellManager: sm})
	require.Equal(t, "list_shells", tool.Name())
}

func TestListShellsTool_Description(t *testing.T) {
	sm := NewShellManager()
	tool := NewListShellsTool(ListShellsToolOptions{ShellManager: sm})
	desc := tool.Description()
	require.Contains(t, desc, "List")
	require.Contains(t, desc, "shell")
}

func TestListShellsTool_Schema(t *testing.T) {
	sm := NewShellManager()
	tool := NewListShellsTool(ListShellsToolOptions{ShellManager: sm})
	s := tool.Schema()

	require.Equal(t, schema.Object, s.Type)
	require.Contains(t, s.Properties, "only_running")
}

func TestListShellsTool_Call(t *testing.T) {
	ctx := context.Background()

	t.Run("ListAllShells", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewListShellsTool(ListShellsToolOptions{ShellManager: sm})

		var cmd string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
		} else {
			cmd = "true"
		}

		// Start a few commands
		sm.StartBackground(ctx, cmd, nil, "cmd1", "")
		sm.StartBackground(ctx, cmd, nil, "cmd2", "")

		// Wait for completion
		time.Sleep(100 * time.Millisecond)

		input := &ListShellsInput{OnlyRunning: false}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Equal(t, float64(2), response["count"])
	})

	t.Run("ListOnlyRunning", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewListShellsTool(ListShellsToolOptions{ShellManager: sm})

		var quickCmd, slowCmd string
		var slowArgs []string
		if runtime.GOOS == "windows" {
			quickCmd = "cmd"
			slowCmd = "cmd"
			slowArgs = []string{"/c", "ping", "-n", "10", "127.0.0.1"}
		} else {
			quickCmd = "true"
			slowCmd = "sleep"
			slowArgs = []string{"10"}
		}

		// Start a quick and slow command
		sm.StartBackground(ctx, quickCmd, nil, "quick", "")
		slowID, _ := sm.StartBackground(ctx, slowCmd, slowArgs, "slow", "")

		// Wait for quick to complete
		time.Sleep(100 * time.Millisecond)

		input := &ListShellsInput{OnlyRunning: true}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Equal(t, float64(1), response["count"])

		// Clean up
		sm.Kill(slowID)
	})

	t.Run("EmptyList", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewListShellsTool(ListShellsToolOptions{ShellManager: sm})

		input := &ListShellsInput{}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Equal(t, float64(0), response["count"])
	})

	t.Run("NoShellManager", func(t *testing.T) {
		tool := NewListShellsTool(ListShellsToolOptions{ShellManager: nil})

		input := &ListShellsInput{}
		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "shell manager not configured")
	})
}

func TestListShellsTool_PreviewCall(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()
	tool := NewListShellsTool(ListShellsToolOptions{ShellManager: sm})

	t.Run("ListAll", func(t *testing.T) {
		input := &ListShellsInput{OnlyRunning: false}
		preview := tool.PreviewCall(ctx, input)
		require.Contains(t, preview.Summary, "all shells")
	})

	t.Run("ListRunning", func(t *testing.T) {
		input := &ListShellsInput{OnlyRunning: true}
		preview := tool.PreviewCall(ctx, input)
		require.Contains(t, preview.Summary, "running")
	})
}

func TestListShellsTool_Annotations(t *testing.T) {
	sm := NewShellManager()
	tool := NewListShellsTool(ListShellsToolOptions{ShellManager: sm})
	annotations := tool.Annotations()

	require.NotNil(t, annotations)
	require.Equal(t, "List Shells", annotations.Title)
	require.True(t, annotations.ReadOnlyHint)
	require.False(t, annotations.DestructiveHint)
}
