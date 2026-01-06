// Package toolkit provides tools for AI agents.
//
// The BashTool in this file implements a bash tool that aligns with Anthropic's
// bash_20250124 tool specification. It provides a persistent bash session that
// maintains state (environment variables, working directory) between commands.
//
// Reference: https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/bash-tool
package toolkit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/sandbox"
	"github.com/deepnoodle-ai/wonton/schema"
)

var (
	_ dive.TypedTool[*BashInput]          = &BashTool{}
	_ dive.TypedToolPreviewer[*BashInput] = &BashTool{}
)

const (
	// DefaultBashTimeout is the default timeout for bash commands (2 minutes)
	DefaultBashTimeout = 2 * time.Minute
	// MaxBashTimeout is the maximum allowed timeout (10 minutes)
	MaxBashTimeout = 10 * time.Minute
	// DefaultMaxOutputLength is the default maximum output length in characters
	DefaultMaxOutputLength = 30000
)

// BashInput represents the input for the bash tool.
// This aligns with Anthropic's bash_20250124 tool specification.
type BashInput struct {
	// Command is the bash command to execute. Required unless Restart is true.
	Command string `json:"command,omitempty"`
	// Restart set to true will restart the bash session, clearing all state.
	Restart bool `json:"restart,omitempty"`
	// Timeout in milliseconds (max 600000ms / 10 minutes, default 120000ms / 2 minutes)
	Timeout int `json:"timeout,omitempty"`
	// Description is a brief description of what the command does (5-10 words)
	Description string `json:"description,omitempty"`
	// WorkingDirectory sets the initial working directory for the session
	WorkingDirectory string `json:"working_directory,omitempty"`
}

// BashSession manages a persistent bash process.
type BashSession struct {
	mu             sync.Mutex
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	stdout         io.ReadCloser
	stderr         io.ReadCloser
	workingDir     string
	maxOutputLen   int
	pathValidator  *PathValidator
	sandboxManager *sandbox.Manager
	cleanup        func()
}

// BashSessionOptions configures a BashSession
type BashSessionOptions struct {
	// WorkingDirectory sets the initial working directory
	WorkingDirectory string
	// MaxOutputLength limits the output size (default: 30000 characters)
	MaxOutputLength int
	// PathValidator for workspace validation (optional)
	PathValidator *PathValidator
	// SandboxManager for sandboxed execution (optional)
	SandboxManager *sandbox.Manager
}

// NewBashSession creates a new persistent bash session.
func NewBashSession(opts ...BashSessionOptions) (*BashSession, error) {
	var resolvedOpts BashSessionOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}
	if resolvedOpts.MaxOutputLength <= 0 {
		resolvedOpts.MaxOutputLength = DefaultMaxOutputLength
	}

	session := &BashSession{
		workingDir:     resolvedOpts.WorkingDirectory,
		maxOutputLen:   resolvedOpts.MaxOutputLength,
		pathValidator:  resolvedOpts.PathValidator,
		sandboxManager: resolvedOpts.SandboxManager,
	}

	if err := session.start(); err != nil {
		return nil, err
	}
	return session, nil
}

// start initializes the bash process
func (s *BashSession) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Determine shell based on OS
	shell := "/bin/bash"
	var shellArgs []string
	if runtime.GOOS == "windows" {
		shell = "cmd"
		shellArgs = []string{"/Q"} // Quiet mode
	}

	s.cmd = exec.Command(shell, shellArgs...)
	if s.workingDir != "" {
		s.cmd.Dir = s.workingDir
	}

	// Apply sandboxing if configured
	if s.sandboxManager != nil {
		wrapped, cleanup, err := s.sandboxManager.Wrap(context.Background(), s.cmd)
		if err != nil {
			return fmt.Errorf("sandbox wrap failed: %w", err)
		}
		s.cmd = wrapped
		s.cleanup = cleanup
	}

	var err error
	s.stdin, err = s.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	s.stdout, err = s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	s.stderr, err = s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start bash session: %w", err)
	}

	return nil
}

// Close terminates the bash session
func (s *BashSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cleanup != nil {
		defer s.cleanup()
	}

	if s.stdin != nil {
		s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
	return nil
}

// Restart terminates and restarts the bash session
func (s *BashSession) Restart() error {
	s.Close()
	return s.start()
}

// Execute runs a command in the persistent bash session
func (s *BashSession) Execute(ctx context.Context, command string, timeout time.Duration) (stdout, stderr string, exitCode int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd == nil || s.cmd.Process == nil {
		return "", "", -1, fmt.Errorf("bash session not started")
	}

	// Generate unique markers for output boundaries
	marker := fmt.Sprintf("__DIVE_CMD_END_%d__", time.Now().UnixNano())
	exitCodeMarker := fmt.Sprintf("__DIVE_EXIT_%d__", time.Now().UnixNano())

	// Construct command with markers
	// We echo markers to both stdout and stderr to capture both streams
	fullCmd := fmt.Sprintf("%s; echo \"%s$?\"; echo \"%s\" >&2\n", command, exitCodeMarker, marker)

	// Write command to stdin
	if _, err := s.stdin.Write([]byte(fullCmd)); err != nil {
		return "", "", -1, fmt.Errorf("failed to write command: %w", err)
	}

	// Read output with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdoutBuf, stderrBuf bytes.Buffer
	exitCode = 0

	// Read stdout in goroutine
	stdoutDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(s.stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, exitCodeMarker) {
				codeStr := strings.TrimPrefix(line, exitCodeMarker)
				if code, err := strconv.Atoi(codeStr); err == nil {
					exitCode = code
				}
				stdoutDone <- nil
				return
			}
			if stdoutBuf.Len() < s.maxOutputLen {
				stdoutBuf.WriteString(line)
				stdoutBuf.WriteString("\n")
			}
		}
		stdoutDone <- scanner.Err()
	}()

	// Read stderr in goroutine
	stderrDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(s.stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if line == marker {
				stderrDone <- nil
				return
			}
			if stderrBuf.Len() < s.maxOutputLen {
				stderrBuf.WriteString(line)
				stderrBuf.WriteString("\n")
			}
		}
		stderrDone <- scanner.Err()
	}()

	// Wait for completion or timeout
	select {
	case <-ctx.Done():
		return stdoutBuf.String(), stderrBuf.String(), -1, fmt.Errorf("command timed out after %s", timeout)
	case err := <-stdoutDone:
		if err != nil {
			return stdoutBuf.String(), stderrBuf.String(), -1, err
		}
		// Wait for stderr to finish too (with a short timeout)
		select {
		case <-stderrDone:
		case <-time.After(100 * time.Millisecond):
		}
	}

	stdout = strings.TrimSuffix(stdoutBuf.String(), "\n")
	stderr = strings.TrimSuffix(stderrBuf.String(), "\n")

	// Truncate if needed
	if len(stdout) > s.maxOutputLen {
		stdout = stdout[:s.maxOutputLen] + "\n... (output truncated)"
	}
	if len(stderr) > s.maxOutputLen {
		stderr = stderr[:s.maxOutputLen] + "\n... (output truncated)"
	}

	return stdout, stderr, exitCode, nil
}

// BashToolOptions configures the BashTool
type BashToolOptions struct {
	// WorkspaceDir is the base directory for workspace validation (defaults to cwd)
	WorkspaceDir string
	// MaxOutputLength limits the output size (default: 30000 characters)
	MaxOutputLength int
	// SandboxConfig configures sandboxing (optional)
	SandboxConfig *sandbox.Config
}

// BashTool implements a persistent bash session tool that aligns with
// Anthropic's bash_20250124 tool specification.
type BashTool struct {
	mu             sync.Mutex
	session        *BashSession
	pathValidator  *PathValidator
	maxOutputLen   int
	sandboxManager *sandbox.Manager
}

// NewBashTool creates a new bash tool.
func NewBashTool(opts ...BashToolOptions) *dive.TypedToolAdapter[*BashInput] {
	var resolvedOpts BashToolOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}
	if resolvedOpts.MaxOutputLength <= 0 {
		resolvedOpts.MaxOutputLength = DefaultMaxOutputLength
	}

	pathValidator, _ := NewPathValidator(resolvedOpts.WorkspaceDir)

	var sandboxManager *sandbox.Manager
	if resolvedOpts.SandboxConfig != nil {
		sandboxManager = sandbox.NewManager(resolvedOpts.SandboxConfig)
	}

	return dive.ToolAdapter(&BashTool{
		pathValidator:  pathValidator,
		maxOutputLen:   resolvedOpts.MaxOutputLength,
		sandboxManager: sandboxManager,
	})
}

func (t *BashTool) Name() string {
	return "Bash"
}

func (t *BashTool) Description() string {
	desc := `Execute shell commands in a persistent bash session.

This tool aligns with Anthropic's bash_20250124 tool specification, providing
a persistent bash session that maintains state (environment variables, working
directory, etc.) between commands.

Parameters:
- command: The bash command to run (required unless restart is true)
- restart: Set to true to restart the bash session, clearing all state
- timeout: Timeout in milliseconds (max 600000ms / 10 minutes, default 120000ms / 2 minutes)
- description: Brief description of what the command does (5-10 words)
- working_directory: Initial working directory for the session

Key features:
- Persistent session: Environment variables and working directory persist between calls
- State management: Use 'restart' to clear session state when needed
- Timeout handling: Commands that exceed the timeout will be terminated

Limitations:
- No interactive commands (vim, less, password prompts)
- No GUI applications
- Large outputs may be truncated

`
	desc += fmt.Sprintf("Running on '%s' operating system.", runtime.GOOS)
	return desc
}

func (t *BashTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type: "object",
		Properties: map[string]*schema.Property{
			"command": {
				Type:        "string",
				Description: "The bash command to run. Required unless 'restart' is true.",
			},
			"restart": {
				Type:        "boolean",
				Description: "Set to true to restart the bash session, clearing all state (environment variables, working directory, etc.).",
			},
			"timeout": {
				Type:        "integer",
				Description: "Timeout in milliseconds (max 600000ms / 10 minutes). Default is 120000ms (2 minutes).",
			},
			"description": {
				Type:        "string",
				Description: "A brief description of what this command does (5-10 words).",
			},
			"working_directory": {
				Type:        "string",
				Description: "The initial working directory for the session. Only used when starting a new session.",
			},
		},
	}
}

func (t *BashTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Bash",
		ReadOnlyHint:    false,
		IdempotentHint:  false,
		DestructiveHint: true,
		OpenWorldHint:   true,
	}
}

func (t *BashTool) PreviewCall(ctx context.Context, input *BashInput) *dive.ToolCallPreview {
	if input.Restart {
		return &dive.ToolCallPreview{
			Summary: "Restart bash session",
		}
	}

	summary := fmt.Sprintf("Run `%s`", truncateCommand(input.Command, 50))
	if input.Description != "" {
		summary = input.Description
	}

	return &dive.ToolCallPreview{
		Summary: summary,
	}
}

func (t *BashTool) Call(ctx context.Context, input *BashInput) (*dive.ToolResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Handle restart
	if input.Restart {
		if t.session != nil {
			t.session.Close()
			t.session = nil
		}
		return dive.NewToolResultText("Bash session restarted").
			WithDisplay("Bash session restarted"), nil
	}

	// Validate command is provided
	if input.Command == "" {
		return dive.NewToolResultError("error: 'command' is required unless 'restart' is true"), nil
	}

	// Validate working directory if provided
	if input.WorkingDirectory != "" && t.pathValidator != nil {
		if err := t.pathValidator.ValidateRead(input.WorkingDirectory); err != nil {
			return dive.NewToolResultError(fmt.Sprintf("error: %s", err.Error())), nil
		}
	}

	// Start session if not already running
	if t.session == nil {
		session, err := NewBashSession(BashSessionOptions{
			WorkingDirectory: input.WorkingDirectory,
			MaxOutputLength:  t.maxOutputLen,
			PathValidator:    t.pathValidator,
			SandboxManager:   t.sandboxManager,
		})
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("error starting bash session: %s", err.Error())), nil
		}
		t.session = session
	}

	// Calculate timeout
	timeout := DefaultBashTimeout
	if input.Timeout > 0 {
		timeout = time.Duration(input.Timeout) * time.Millisecond
		if timeout > MaxBashTimeout {
			timeout = MaxBashTimeout
		}
	}

	// Execute command
	stdout, stderr, exitCode, err := t.session.Execute(ctx, input.Command, timeout)
	if err != nil {
		// On error, check if it's a timeout
		if strings.Contains(err.Error(), "timed out") {
			return dive.NewToolResultError(err.Error()), nil
		}
		// For other errors, restart the session and return error
		t.session.Close()
		t.session = nil
		return dive.NewToolResultError(fmt.Sprintf("error: %s", err.Error())), nil
	}

	// Build result
	result := map[string]interface{}{
		"stdout":      stdout,
		"stderr":      stderr,
		"return_code": exitCode,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("error marshaling result: %s", err.Error())), nil
	}

	// Build display
	display := input.Description
	if display == "" {
		display = fmt.Sprintf("Ran `%s`", truncateCommand(input.Command, 40))
	}
	display = fmt.Sprintf("%s (exit %d)", display, exitCode)

	// Return error result if command failed
	if exitCode != 0 {
		return dive.NewToolResultError(string(resultJSON)).WithDisplay(display), nil
	}

	return dive.NewToolResultText(string(resultJSON)).WithDisplay(display), nil
}

// Close closes the bash session if one is active
func (t *BashTool) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.session != nil {
		return t.session.Close()
	}
	return nil
}

// truncateCommand truncates a command string for display, replacing newlines with spaces
func truncateCommand(s string, maxLen int) string {
	// Remove newlines for display
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
