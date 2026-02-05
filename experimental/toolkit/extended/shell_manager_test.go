package extended

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestShellManager_NewShellManager(t *testing.T) {
	sm := NewShellManager()
	assert.NotNil(t, sm)
	assert.NotNil(t, sm.shells)
	assert.Empty(t, sm.shells)
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
		assert.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.Contains(t, id, "shell-")

		// Give it time to complete
		time.Sleep(100 * time.Millisecond)

		info, exists := sm.Get(id)
		assert.True(t, exists)
		assert.Equal(t, id, info.ID)
		assert.Equal(t, cmd, info.Command)
		assert.Equal(t, "echo test", info.Description)
	})

	t.Run("IncrementingIDs", func(t *testing.T) {
		var cmd string
		if runtime.GOOS == "windows" {
			cmd = "cmd"
		} else {
			cmd = "true"
		}

		id1, err := sm.StartBackground(ctx, cmd, nil, "", "")
		assert.NoError(t, err)

		id2, err := sm.StartBackground(ctx, cmd, nil, "", "")
		assert.NoError(t, err)

		assert.NotEqual(t, id1, id2)
	})

	t.Run("InvalidCommand", func(t *testing.T) {
		_, err := sm.StartBackground(ctx, "nonexistent-command-xyz", nil, "", "")
		assert.Error(t, err)
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
		assert.NoError(t, err)

		stdout, stderr, info, err := sm.GetOutput(id, true, 5*time.Second)
		assert.NoError(t, err)
		assert.Contains(t, stdout, "test output")
		assert.Empty(t, stderr)
		assert.Equal(t, ShellStatusCompleted, info.Status)
		assert.NotNil(t, info.ExitCode)
		assert.Equal(t, 0, *info.ExitCode)
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
		assert.NoError(t, err)

		// Non-blocking should return immediately
		_, _, info, err := sm.GetOutput(id, false, time.Second)
		assert.NoError(t, err)
		assert.Equal(t, ShellStatusRunning, info.Status)

		// Clean up
		sm.Kill(id)
	})

	t.Run("GetOutputNotFound", func(t *testing.T) {
		_, _, _, err := sm.GetOutput("nonexistent-shell", true, time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "shell not found")
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
		assert.NoError(t, err)

		// Verify it's running
		assert.True(t, sm.IsRunning(id))

		// Kill it
		err = sm.Kill(id)
		assert.NoError(t, err)

		// Give it time to terminate
		time.Sleep(200 * time.Millisecond)

		// Verify it's no longer running
		assert.False(t, sm.IsRunning(id))

		info, exists := sm.Get(id)
		assert.True(t, exists)
		assert.Equal(t, ShellStatusKilled, info.Status)
	})

	t.Run("KillNotFound", func(t *testing.T) {
		err := sm.Kill("nonexistent-shell")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "shell not found")
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
	assert.Len(t, shells, 2)

	ids := []string{shells[0].ID, shells[1].ID}
	assert.Contains(t, ids, id1)
	assert.Contains(t, ids, id2)
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
	assert.Len(t, running, 1)
	assert.Equal(t, slowID, running[0].ID)

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
		assert.True(t, sm.IsRunning(id))

		sm.Kill(id)
		time.Sleep(200 * time.Millisecond)
		assert.False(t, sm.IsRunning(id))
	})

	t.Run("NotFound", func(t *testing.T) {
		assert.False(t, sm.IsRunning("nonexistent"))
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
	assert.Len(t, sm.List(), 2)

	// Cleanup with 0 duration (remove all completed)
	removed := sm.Cleanup(0)
	assert.Equal(t, 2, removed)

	// Verify they're gone
	assert.Empty(t, sm.List())
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
	assert.Len(t, ids, 20)

	// All IDs should be unique
	idSet := make(map[string]bool)
	for _, id := range ids {
		assert.False(t, idSet[id], "duplicate ID found")
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
	assert.NoError(t, err)

	// Wait for completion
	stdout, stderr, info, err := sm.GetOutput(id, true, 5*time.Second)
	assert.NoError(t, err)
	assert.Empty(t, stdout)
	_ = stderr // May or may not have content

	assert.Equal(t, ShellStatusFailed, info.Status)
	assert.NotNil(t, info.ExitCode)
	assert.NotEqual(t, 0, *info.ExitCode)
}

func TestShellManager_MaxShellLimit(t *testing.T) {
	ctx := context.Background()

	// Create a shell manager with a small max shells limit
	sm := NewShellManager(ShellManagerOptions{
		MaxShells: 3,
	})

	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "ping", "-n", "10", "127.0.0.1"}
	} else {
		cmd = "sleep"
		args = []string{"10"}
	}

	// Start max shells number of long-running commands
	var ids []string
	for i := 0; i < 3; i++ {
		id, err := sm.StartBackground(ctx, cmd, args, "", "")
		assert.NoError(t, err, "Should be able to start shell %d", i+1)
		ids = append(ids, id)
	}

	// The 4th shell should fail due to max shells limit
	_, err := sm.StartBackground(ctx, cmd, args, "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum number of background shells")

	// Kill one shell
	err = sm.Kill(ids[0])
	assert.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// Now we should be able to start another shell
	newID, err := sm.StartBackground(ctx, cmd, args, "", "")
	assert.NoError(t, err, "Should be able to start a new shell after killing one")

	// Clean up
	for _, id := range ids[1:] {
		sm.Kill(id)
	}
	sm.Kill(newID)
}

func TestShellManager_OutputSizeLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - different shell behavior")
	}

	ctx := context.Background()

	// Create a shell manager with a small output limit (1KB)
	maxSize := int64(1024)
	sm := NewShellManager(ShellManagerOptions{
		MaxOutputSize: maxSize,
	})

	// Generate output larger than the limit (2KB of 'x' characters)
	// Using yes | head to generate predictable output
	cmd := "sh"
	args := []string{"-c", "yes x | head -c 2048"}

	id, err := sm.StartBackground(ctx, cmd, args, "", "")
	assert.NoError(t, err)

	// Wait for completion
	stdout, _, _, err := sm.GetOutput(id, true, 5*time.Second)
	assert.NoError(t, err)

	// Output should be truncated
	assert.LessOrEqual(t, int64(len(stdout)), maxSize+100, "Output should be approximately limited to max size")
	assert.Contains(t, stdout, "truncated", "Output should contain truncation message")
}

func TestShellManager_DefaultLimits(t *testing.T) {
	// Test that default limits are set correctly
	sm := NewShellManager()

	assert.Equal(t, int64(DefaultMaxOutputSize), sm.maxOutputSize)
	assert.Equal(t, DefaultMaxShells, sm.maxShells)
}

func TestShellManager_CustomLimits(t *testing.T) {
	// Test that custom limits are applied
	sm := NewShellManager(ShellManagerOptions{
		MaxOutputSize: 5 * 1024 * 1024, // 5MB
		MaxShells:     100,
	})

	assert.Equal(t, int64(5*1024*1024), sm.maxOutputSize)
	assert.Equal(t, 100, sm.maxShells)
}

func TestLimitedWriter(t *testing.T) {
	t.Run("WritesWithinLimit", func(t *testing.T) {
		lw := newLimitedWriter(100)

		n, err := lw.Write([]byte("Hello, World!"))
		assert.NoError(t, err)
		assert.Equal(t, 13, n)

		result := lw.String()
		assert.Equal(t, "Hello, World!", result)
		assert.False(t, lw.truncated)
	})

	t.Run("TruncatesWhenExceedsLimit", func(t *testing.T) {
		lw := newLimitedWriter(10)

		// Write more than the limit
		testData := []byte("This is a very long string that exceeds the limit")
		n, err := lw.Write(testData)
		assert.NoError(t, err)
		assert.Equal(t, len(testData), n) // Returns full length to not break the writer

		result := lw.String()
		assert.True(t, lw.truncated)
		assert.Contains(t, result, "truncated")
		// The actual content before truncation should be limited
		assert.LessOrEqual(t, len(result), 100) // Allow some room for truncation message
	})

	t.Run("MultipleWrites", func(t *testing.T) {
		lw := newLimitedWriter(20)

		lw.Write([]byte("12345"))    // 5 bytes
		lw.Write([]byte("67890"))    // 5 bytes = 10 total
		lw.Write([]byte("ABCDE"))    // 5 bytes = 15 total
		lw.Write([]byte("FGHIJ"))    // 5 bytes = 20 total
		lw.Write([]byte("OVERFLOW")) // Should trigger truncation

		result := lw.String()
		assert.True(t, lw.truncated)
		assert.Contains(t, result, "truncated")
	})

	t.Run("DiscardAfterTruncation", func(t *testing.T) {
		lw := newLimitedWriter(5)

		lw.Write([]byte("12345"))    // Fills the buffer
		lw.Write([]byte("ABCDE"))    // Triggers truncation
		lw.Write([]byte("OVERFLOW")) // Should be discarded silently

		result := lw.String()
		assert.True(t, lw.truncated)
		// Should not contain OVERFLOW
		assert.NotContains(t, result, "OVERFLOW")
	})
}
