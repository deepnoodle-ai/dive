package toolkit

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShellManager_NewShellManager(t *testing.T) {
	sm := NewShellManager()
	require.NotNil(t, sm)
	require.NotNil(t, sm.shells)
	require.Empty(t, sm.shells)
}

func TestShellManager_StartBackground(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()

	t.Run("StartSimpleCommand", func(t *testing.T) {
		var cmd string
		var args []string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
			args = []string{"/c", "echo", "hello"}
		} else {
			cmd = "echo"
			args = []string{"hello"}
		}

		id, err := sm.StartBackground(ctx, cmd, args, "echo test", "")
		require.NoError(t, err)
		require.NotEmpty(t, id)
		require.Contains(t, id, "shell-")

		// Give it time to complete
		time.Sleep(100 * time.Millisecond)

		info, exists := sm.Get(id)
		require.True(t, exists)
		require.Equal(t, id, info.ID)
		require.Equal(t, cmd, info.Command)
		require.Equal(t, "echo test", info.Description)
	})

	t.Run("IncrementingIDs", func(t *testing.T) {
		var cmd string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
		} else {
			cmd = "true"
		}

		id1, err := sm.StartBackground(ctx, cmd, nil, "", "")
		require.NoError(t, err)

		id2, err := sm.StartBackground(ctx, cmd, nil, "", "")
		require.NoError(t, err)

		require.NotEqual(t, id1, id2)
	})

	t.Run("InvalidCommand", func(t *testing.T) {
		_, err := sm.StartBackground(ctx, "nonexistent-command-xyz", nil, "", "")
		require.Error(t, err)
	})
}

func TestShellManager_GetOutput(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()

	t.Run("GetOutputBlocking", func(t *testing.T) {
		var cmd string
		var args []string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
			args = []string{"/c", "echo", "test output"}
		} else {
			cmd = "echo"
			args = []string{"test output"}
		}

		id, err := sm.StartBackground(ctx, cmd, args, "", "")
		require.NoError(t, err)

		stdout, stderr, info, err := sm.GetOutput(id, true, 5*time.Second)
		require.NoError(t, err)
		require.Contains(t, stdout, "test output")
		require.Empty(t, stderr)
		require.Equal(t, ShellStatusCompleted, info.Status)
		require.NotNil(t, info.ExitCode)
		require.Equal(t, 0, *info.ExitCode)
	})

	t.Run("GetOutputNonBlocking", func(t *testing.T) {
		var cmd string
		var args []string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
			args = []string{"/c", "ping", "-n", "2", "127.0.0.1"}
		} else {
			cmd = "sleep"
			args = []string{"2"}
		}

		id, err := sm.StartBackground(ctx, cmd, args, "", "")
		require.NoError(t, err)

		// Non-blocking should return immediately
		_, _, info, err := sm.GetOutput(id, false, time.Second)
		require.NoError(t, err)
		require.Equal(t, ShellStatusRunning, info.Status)

		// Clean up
		sm.Kill(id)
	})

	t.Run("GetOutputNotFound", func(t *testing.T) {
		_, _, _, err := sm.GetOutput("nonexistent-shell", true, time.Second)
		require.Error(t, err)
		require.Contains(t, err.Error(), "shell not found")
	})
}

func TestShellManager_Kill(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()

	t.Run("KillRunningProcess", func(t *testing.T) {
		var cmd string
		var args []string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
			args = []string{"/c", "ping", "-n", "100", "127.0.0.1"}
		} else {
			cmd = "sleep"
			args = []string{"100"}
		}

		id, err := sm.StartBackground(ctx, cmd, args, "", "")
		require.NoError(t, err)

		// Verify it's running
		require.True(t, sm.IsRunning(id))

		// Kill it
		err = sm.Kill(id)
		require.NoError(t, err)

		// Give it time to terminate
		time.Sleep(200 * time.Millisecond)

		// Verify it's no longer running
		require.False(t, sm.IsRunning(id))

		info, exists := sm.Get(id)
		require.True(t, exists)
		require.Equal(t, ShellStatusKilled, info.Status)
	})

	t.Run("KillNotFound", func(t *testing.T) {
		err := sm.Kill("nonexistent-shell")
		require.Error(t, err)
		require.Contains(t, err.Error(), "shell not found")
	})
}

func TestShellManager_List(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
	} else {
		cmd = "true"
	}

	// Start a few commands
	id1, _ := sm.StartBackground(ctx, cmd, nil, "cmd1", "")
	id2, _ := sm.StartBackground(ctx, cmd, nil, "cmd2", "")

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	shells := sm.List()
	require.Len(t, shells, 2)

	ids := []string{shells[0].ID, shells[1].ID}
	require.Contains(t, ids, id1)
	require.Contains(t, ids, id2)
}

func TestShellManager_ListRunning(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()

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

	// Start a quick command and a slow command
	_, _ = sm.StartBackground(ctx, quickCmd, nil, "quick", "")
	slowID, _ := sm.StartBackground(ctx, slowCmd, slowArgs, "slow", "")

	// Wait for quick to complete
	time.Sleep(100 * time.Millisecond)

	running := sm.ListRunning()
	require.Len(t, running, 1)
	require.Equal(t, slowID, running[0].ID)

	// Clean up
	sm.Kill(slowID)
}

func TestShellManager_IsRunning(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()

	t.Run("RunningProcess", func(t *testing.T) {
		var cmd string
		var args []string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
			args = []string{"/c", "ping", "-n", "10", "127.0.0.1"}
		} else {
			cmd = "sleep"
			args = []string{"10"}
		}

		id, _ := sm.StartBackground(ctx, cmd, args, "", "")
		require.True(t, sm.IsRunning(id))

		sm.Kill(id)
		time.Sleep(200 * time.Millisecond)
		require.False(t, sm.IsRunning(id))
	})

	t.Run("NotFound", func(t *testing.T) {
		require.False(t, sm.IsRunning("nonexistent"))
	})
}

func TestShellManager_Cleanup(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
	} else {
		cmd = "true"
	}

	// Start and complete some commands
	_, _ = sm.StartBackground(ctx, cmd, nil, "", "")
	_, _ = sm.StartBackground(ctx, cmd, nil, "", "")

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	// Verify we have shells
	require.Len(t, sm.List(), 2)

	// Cleanup with 0 duration (remove all completed)
	removed := sm.Cleanup(0)
	require.Equal(t, 2, removed)

	// Verify they're gone
	require.Empty(t, sm.List())
}

func TestShellManager_Concurrency(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()

	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
	} else {
		cmd = "true"
	}

	// Start many commands concurrently
	done := make(chan string, 20)
	for i := 0; i < 20; i++ {
		go func() {
			id, err := sm.StartBackground(ctx, cmd, nil, "", "")
			if err == nil {
				done <- id
			} else {
				done <- ""
			}
		}()
	}

	// Collect results
	var ids []string
	for i := 0; i < 20; i++ {
		id := <-done
		if id != "" {
			ids = append(ids, id)
		}
	}

	// All should have succeeded
	require.Len(t, ids, 20)

	// All IDs should be unique
	idSet := make(map[string]bool)
	for _, id := range ids {
		require.False(t, idSet[id], "duplicate ID found")
		idSet[id] = true
	}
}

func TestShellManager_FailingCommand(t *testing.T) {
	ctx := context.Background()
	sm := NewShellManager()

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

	// Wait for completion
	stdout, stderr, info, err := sm.GetOutput(id, true, 5*time.Second)
	require.NoError(t, err)
	require.Empty(t, stdout)
	_ = stderr // May or may not have content

	require.Equal(t, ShellStatusFailed, info.Status)
	require.NotNil(t, info.ExitCode)
	require.NotEqual(t, 0, *info.ExitCode)
}
