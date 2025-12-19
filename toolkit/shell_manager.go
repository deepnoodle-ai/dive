package toolkit

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// ShellStatus represents the status of a background shell
type ShellStatus string

const (
	ShellStatusRunning   ShellStatus = "running"
	ShellStatusCompleted ShellStatus = "completed"
	ShellStatusFailed    ShellStatus = "failed"
	ShellStatusKilled    ShellStatus = "killed"
)

// ShellInfo contains information about a background shell process
type ShellInfo struct {
	ID          string      `json:"id"`
	Command     string      `json:"command"`
	Args        []string    `json:"args,omitempty"`
	Description string      `json:"description,omitempty"`
	Status      ShellStatus `json:"status"`
	StartTime   time.Time   `json:"start_time"`
	EndTime     *time.Time  `json:"end_time,omitempty"`
	ExitCode    *int        `json:"exit_code,omitempty"`
	Error       string      `json:"error,omitempty"`
}

// backgroundShell tracks a running background shell
type backgroundShell struct {
	info       ShellInfo
	cmd        *exec.Cmd
	stdout     *bytes.Buffer
	stderr     *bytes.Buffer
	done       chan struct{}
	cancelFunc context.CancelFunc
}

// ShellManager manages background shell processes
type ShellManager struct {
	mu      sync.RWMutex
	shells  map[string]*backgroundShell
	counter int
}

// NewShellManager creates a new ShellManager
func NewShellManager() *ShellManager {
	return &ShellManager{
		shells: make(map[string]*backgroundShell),
	}
}

// StartBackground starts a command in the background and returns its ID
func (m *ShellManager) StartBackground(ctx context.Context, name string, args []string, description string, workingDir string) (string, error) {
	m.mu.Lock()
	m.counter++
	id := fmt.Sprintf("shell-%d", m.counter)
	m.mu.Unlock()

	// Create a cancellable context for this shell
	shellCtx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(shellCtx, name, args...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	shell := &backgroundShell{
		info: ShellInfo{
			ID:          id,
			Command:     name,
			Args:        args,
			Description: description,
			Status:      ShellStatusRunning,
			StartTime:   time.Now(),
		},
		cmd:        cmd,
		stdout:     stdout,
		stderr:     stderr,
		done:       make(chan struct{}),
		cancelFunc: cancel,
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		cancel()
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	// Store the shell
	m.mu.Lock()
	m.shells[id] = shell
	m.mu.Unlock()

	// Wait for completion in background
	go func() {
		defer close(shell.done)
		err := cmd.Wait()

		m.mu.Lock()
		defer m.mu.Unlock()

		now := time.Now()
		shell.info.EndTime = &now

		if err != nil {
			if shellCtx.Err() == context.Canceled {
				shell.info.Status = ShellStatusKilled
			} else {
				shell.info.Status = ShellStatusFailed
				shell.info.Error = err.Error()
			}
		} else {
			shell.info.Status = ShellStatusCompleted
		}

		if cmd.ProcessState != nil {
			exitCode := cmd.ProcessState.ExitCode()
			shell.info.ExitCode = &exitCode
		}
	}()

	return id, nil
}

// GetOutput retrieves output from a background shell
// If block is true, waits for completion (up to timeout)
// Returns stdout, stderr, info, and any error
func (m *ShellManager) GetOutput(id string, block bool, timeout time.Duration) (string, string, *ShellInfo, error) {
	m.mu.RLock()
	shell, exists := m.shells[id]
	m.mu.RUnlock()

	if !exists {
		return "", "", nil, fmt.Errorf("shell not found: %s", id)
	}

	if block {
		// Wait for completion with timeout
		select {
		case <-shell.done:
			// Completed
		case <-time.After(timeout):
			// Timeout - return current output anyway
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	infoCopy := shell.info
	return shell.stdout.String(), shell.stderr.String(), &infoCopy, nil
}

// Kill terminates a background shell
func (m *ShellManager) Kill(id string) error {
	m.mu.RLock()
	shell, exists := m.shells[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("shell not found: %s", id)
	}

	// Cancel the context to kill the process
	shell.cancelFunc()

	// Wait briefly for it to terminate
	select {
	case <-shell.done:
	case <-time.After(5 * time.Second):
		// Force kill if still running
		if shell.cmd.Process != nil {
			shell.cmd.Process.Kill()
		}
	}

	return nil
}

// List returns information about all shells
func (m *ShellManager) List() []ShellInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]ShellInfo, 0, len(m.shells))
	for _, shell := range m.shells {
		result = append(result, shell.info)
	}
	return result
}

// ListRunning returns information about running shells only
func (m *ShellManager) ListRunning() []ShellInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []ShellInfo
	for _, shell := range m.shells {
		if shell.info.Status == ShellStatusRunning {
			result = append(result, shell.info)
		}
	}
	return result
}

// Get returns information about a specific shell
func (m *ShellManager) Get(id string) (*ShellInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	shell, exists := m.shells[id]
	if !exists {
		return nil, false
	}
	infoCopy := shell.info
	return &infoCopy, true
}

// IsRunning checks if a shell is still running
func (m *ShellManager) IsRunning(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	shell, exists := m.shells[id]
	if !exists {
		return false
	}
	return shell.info.Status == ShellStatusRunning
}

// Cleanup removes completed shells older than the given duration
func (m *ShellManager) Cleanup(olderThan time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	removed := 0

	for id, shell := range m.shells {
		if shell.info.Status != ShellStatusRunning && shell.info.EndTime != nil && shell.info.EndTime.Before(cutoff) {
			delete(m.shells, id)
			removed++
		}
	}

	return removed
}
