package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeatbeltBackend_Available(t *testing.T) {
	backend := &SeatbeltBackend{}
	if runtime.GOOS == "darwin" {
		// Just check it doesn't panic. Actual availability depends on sandbox-exec presence.
		// Most macOS systems have it.
		_ = backend.Available()
	} else {
		require.False(t, backend.Available())
	}
}

func TestDockerBackend_WrapCommand_Args(t *testing.T) {
	backend := NewDockerBackend()

	cfg := &Config{
		Enabled:      true,
		WorkDir:      "/host/work",
		AllowNetwork: false,
		Docker: DockerConfig{
			Image: "test-image",
		},
	}

	cmd := exec.Command("echo", "hello")
	wrapped, cleanup, err := backend.WrapCommand(context.Background(), cmd, cfg)
	require.NoError(t, err)
	defer cleanup()

	// Convert args to string for easier searching
	argsStr := strings.Join(wrapped.Args, " ")

	assert.Contains(t, argsStr, "run")
	assert.Contains(t, argsStr, "--read-only")
	assert.Contains(t, argsStr, "--network none")
	assert.Contains(t, argsStr, "test-image")
	assert.Contains(t, argsStr, "--cidfile")
}

func TestSandbox_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Determine backend to test
	var backend Backend
	sb := &SeatbeltBackend{}
	db := NewDockerBackend()

	if sb.Available() {
		backend = sb
	} else if db.Available() {
		backend = db
	} else {
		t.Skip("No sandbox backend available")
	}

	t.Logf("Testing with backend: %s", backend.Name())

	tempDir := t.TempDir()
	cfg := &Config{
		Enabled:      true,
		WorkDir:      tempDir,
		AllowNetwork: false,
		Docker: DockerConfig{
			Image: "alpine:latest",
		},
	}

	mgr := NewManager(cfg)

	// Test 1: Write allowed file
	cmd := exec.Command("sh", "-c", "echo 'allowed' > allowed.txt")
	cmd.Dir = tempDir
	wrapped, cleanup, err := mgr.Wrap(context.Background(), cmd)
	require.NoError(t, err)
	defer cleanup()

	output, err := wrapped.CombinedOutput()
	if err != nil {
		t.Logf("Command failed: %s", string(output))
	}
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(tempDir, "allowed.txt"))

	// Test 2: Write denied file (outside workspace)
	// We use HomeDir because TmpDir is allowed by default in the profile.
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	
	// Create a dummy file path in home dir (we won't actually write it hopefully)
	deniedPath := filepath.Join(homeDir, "dive-sandbox-test-denied.txt")
	
	// Ensure we don't overwrite anything existing (unlikely)
	if _, err := os.Stat(deniedPath); err == nil {
		t.Skipf("File %s exists, skipping negative test for safety", deniedPath)
	}

	cmd = exec.Command("sh", "-c", fmt.Sprintf("echo 'denied' > '%s'", deniedPath))
	cmd.Dir = tempDir
	wrapped, cleanup, err = mgr.Wrap(context.Background(), cmd)
	require.NoError(t, err)
	defer cleanup()

	output, err = wrapped.CombinedOutput()
	// Should fail
	if err == nil {
		// Clean up if it somehow succeeded
		os.Remove(deniedPath)
		t.Errorf("Expected error writing to %s, but succeeded. Output: %s", deniedPath, output)
	} else {
		t.Logf("Got expected error: %v", err)
	}
}


