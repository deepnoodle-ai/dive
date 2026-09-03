package toolkit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
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
	// DefaultMaxOutputLength is the default maximum retained output in bytes.
	DefaultMaxOutputLength = 30000
	outputTruncationMarker = "\n... (output truncated)"
)

type boundedOutput struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

// Write retains at most limit bytes while reporting the full chunk consumed so
// callers can continue draining the process pipe after the result is bounded.
func (b *boundedOutput) Write(p []byte) (int, error) {
	consumed := len(p)
	if b.limit <= 0 {
		_, _ = b.buf.Write(p)
		return consumed, nil
	}

	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = b.buf.Write(p[:remaining])
	}
	if remaining < len(p) {
		b.truncated = true
	}
	return consumed, nil
}

func (b *boundedOutput) String() string {
	output := b.buf.String()
	if b.truncated {
		return output + outputTruncationMarker
	}
	return output
}

// BashInput represents the input parameters for the Bash tool.
type BashInput struct {
	// Command is the shell command to execute. Required.
	Command string `json:"command"`

	// Timeout specifies the maximum execution time in milliseconds.
	// Valid range: 1-600000 (10 minutes). Defaults to 120000 (2 minutes).
	Timeout int `json:"timeout,omitempty"`

	// Description provides a brief human-readable summary of what the command does.
	// When provided, this is displayed instead of the raw command (5-10 words recommended).
	Description string `json:"description,omitempty"`

	// WorkingDirectory sets the working directory for this invocation.
	// If empty, the command runs in the Dive process's working directory.
	// Directory changes never carry over to a later invocation.
	// Must be within the workspace if path validation is enabled.
	WorkingDirectory string `json:"working_directory,omitempty"`
}

// BashToolOptions configures the behavior of [BashTool].
type BashToolOptions struct {
	// WorkspaceDir restricts command execution to paths within this directory.
	// When set, the working directory must be within this path.
	// Defaults to the current working directory if empty.
	// Ignored when Validator is set.
	WorkspaceDir string

	// Validator is an optional shared PathValidator. When set, it is used
	// instead of creating one from WorkspaceDir.
	Validator *PathValidator

	// MaxOutputLength limits the retained combined stdout/stderr size in bytes.
	// Output exceeding this limit is truncated with a warning.
	// Defaults to [DefaultMaxOutputLength] (30000 characters).
	MaxOutputLength int
}

// BashTool executes shell commands and captures their output.
//
// On Unix systems, commands are executed via /bin/bash -c. On Windows, commands
// are executed via cmd /C. Stdout and stderr are merged in emission order.
//
// Features:
//   - Configurable timeout (default 2 minutes, max 10 minutes)
//   - Output truncation to prevent overwhelming the LLM
//   - Working directory validation when workspace restrictions are enabled
//   - Non-interactive only (no stdin support)
//
// Security: This tool can execute arbitrary shell commands. Use workspace
// restrictions and the agent permission system to control what commands
// are allowed.
type BashTool struct {
	pathValidator *PathValidator
	maxOutputLen  int
	workspaceDir  string
	configErr     error
}

// NewBashTool creates a new BashTool with the given options.
// If no options are provided, defaults are used.
func NewBashTool(opts ...BashToolOptions) *dive.TypedToolAdapter[*BashInput] {
	var resolvedOpts BashToolOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}
	if resolvedOpts.MaxOutputLength <= 0 {
		resolvedOpts.MaxOutputLength = DefaultMaxOutputLength
	}

	var pathValidator *PathValidator
	var configErr error
	if resolvedOpts.Validator != nil {
		pathValidator = resolvedOpts.Validator
	} else {
		pathValidator, configErr = NewPathValidator(resolvedOpts.WorkspaceDir)
		if configErr != nil {
			configErr = fmt.Errorf("invalid workspace configuration for WorkspaceDir %q: %w", resolvedOpts.WorkspaceDir, configErr)
		}
	}

	return dive.ToolAdapter(&BashTool{
		pathValidator: pathValidator,
		maxOutputLen:  resolvedOpts.MaxOutputLength,
		workspaceDir:  resolvedOpts.WorkspaceDir,
		configErr:     configErr,
	})
}

// Name returns "Bash" as the tool identifier.
func (t *BashTool) Name() string {
	return "Bash"
}

// Description returns detailed usage instructions for the LLM.
func (t *BashTool) Description() string {
	desc := `Execute shell commands.

Parameters:
- command: The bash command to run (required)
- timeout: Timeout in milliseconds (max 600000ms / 10 minutes, default 120000ms / 2 minutes)
- description: Brief description of what the command does (5-10 words)
- working_directory: Directory for this invocation; it does not become the default for later calls

State:
- Each call starts a fresh shell in working_directory, or the Dive process working directory when omitted
- Working-directory changes, environment variables, shell options, and exit status do not persist between calls
- Filesystem and other external changes made by commands do persist

Limitations:
- No interactive commands (vim, less, password prompts)
- No GUI applications
- Large outputs may be truncated

`
	shell, shellArgs := shellCommand()
	shellInvocation := strings.Join(append([]string{shell}, shellArgs...), " ")
	desc += fmt.Sprintf("Commands run via '%s' on '%s'.", shellInvocation, runtime.GOOS)
	return desc
}

// Schema returns the JSON schema describing the tool's input parameters.
func (t *BashTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"command"},
		Properties: map[string]*schema.Property{
			"command": {
				Type:        "string",
				Description: "The bash command to run.",
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
				Description: "The working directory for this invocation only. It does not persist to later calls.",
			},
		},
	}
}

// Annotations returns metadata hints about the tool's behavior.
// Bash is marked as destructive (can modify system state) and open-world
// (interacts with external systems).
func (t *BashTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Bash",
		ReadOnlyHint:    false,
		IdempotentHint:  false,
		DestructiveHint: true,
		OpenWorldHint:   true,
	}
}

// PreviewCall returns a summary of what the command will do, used for
// permission prompts and logging.
func (t *BashTool) PreviewCall(ctx context.Context, input *BashInput) *dive.ToolCallPreview {
	summary := fmt.Sprintf("Run `%s`", truncateCommand(input.Command, 50))
	if input.Description != "" {
		summary = input.Description
	}

	return &dive.ToolCallPreview{
		Summary: summary,
	}
}

// Call executes the shell command and returns its output.
//
// A successful command returns its merged stdout/stderr verbatim. A non-zero
// exit returns the same body wrapped in <error> with its numeric exit code, but
// no Go error; the LLM receives the command failure as a tool result.
//
// The context can be used to cancel long-running commands.
func (t *BashTool) Call(ctx context.Context, input *BashInput) (*dive.ToolResult, error) {
	if t.configErr != nil {
		return dive.NewToolResultError(fmt.Sprintf("error: %s", t.configErr.Error())), nil
	}
	if t.pathValidator == nil {
		return dive.NewToolResultError(fmt.Sprintf("error: invalid workspace configuration for WorkspaceDir %q: path validator is not initialized", t.workspaceDir)), nil
	}

	// Validate command is provided
	if input.Command == "" {
		return dive.NewToolResultError("error: 'command' is required"), nil
	}

	// Validate working directory if provided
	if input.WorkingDirectory != "" && t.pathValidator != nil {
		if err := t.pathValidator.ValidateRead(input.WorkingDirectory); err != nil {
			return dive.NewToolResultError(fmt.Sprintf("error: %s", err.Error())), nil
		}
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
	output, exitCode, err := t.execute(ctx, input.Command, input.WorkingDirectory, timeout)
	if err != nil {
		return dive.NewToolResultError(err.Error()), nil
	}

	if exitCode != 0 {
		return dive.NewToolResultError(formatBashFailure(exitCode, output)), nil
	}

	return dive.NewToolResultText(output), nil
}

// execute runs a command with the given timeout and returns its output.
// It handles context cancellation, timeout enforcement, and output truncation.
// If dive.StreamOutput is available in the context, merged output is streamed
// as it arrives. The child's stdout and stderr descriptors point to the same OS
// pipe, so the kernel preserves their write order without a stdout-first or
// stderr-first post-processing step.
func (t *BashTool) execute(ctx context.Context, command, workingDir string, timeout time.Duration) (output string, exitCode int, err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Determine shell based on OS
	shell, shellArgs := shellCommand()
	shellArgs = append(shellArgs, command)

	cmd := exec.CommandContext(ctx, shell, shellArgs...)
	configureBashCommandCancellation(cmd)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	mergedReader, mergedWriter, pipeErr := os.Pipe()
	if pipeErr != nil {
		return "", -1, fmt.Errorf("error creating command output pipe: %s", pipeErr.Error())
	}
	defer mergedReader.Close()
	cmd.Stdout = mergedWriter
	cmd.Stderr = mergedWriter

	runErr := cmd.Start()
	if runErr != nil {
		_ = mergedWriter.Close()
		if ctx.Err() == context.DeadlineExceeded {
			return "", -1, fmt.Errorf("command timed out after %s", timeout)
		}
		return "", -1, fmt.Errorf("error: %s", runErr.Error())
	}
	// The parent must close its copy so the reader observes EOF after the child
	// closes both descriptors.
	_ = mergedWriter.Close()
	// A descendant can inherit the writer after the shell exits. Closing the
	// reader when the context ends prevents that inherited descriptor from
	// holding this call open after cancellation has terminated the process tree.
	readFinished := make(chan struct{})
	defer close(readFinished)
	go func() {
		select {
		case <-ctx.Done():
			_ = mergedReader.Close()
		case <-readFinished:
		}
	}()

	merged := boundedOutput{limit: t.maxOutputLen}
	chunk := make([]byte, 32*1024)
	var captureErr error
	for {
		n, readErr := mergedReader.Read(chunk)
		if n > 0 {
			part := chunk[:n]
			_, _ = merged.Write(part)
			dive.StreamOutput(ctx, string(part))
		}
		if readErr != nil {
			if readErr != io.EOF && ctx.Err() == nil {
				captureErr = readErr
			}
			break
		}
	}

	runErr = cmd.Wait()
	exitCode = 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return "", -1, fmt.Errorf("command timed out after %s", timeout)
		} else {
			return "", -1, fmt.Errorf("error: %s", runErr.Error())
		}
	}

	if captureErr != nil {
		return "", -1, fmt.Errorf("error reading command output: %s", captureErr.Error())
	}

	return merged.String(), exitCode, nil
}

func formatBashFailure(exitCode int, output string) string {
	return fmt.Sprintf("<error>Exit code %d\n%s</error>", exitCode, output)
}

// shellCommand returns the shell and arguments for command execution.
func shellCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C"}
	}
	return "/bin/bash", []string{"-c"}
}

// truncateOutput limits output length to prevent overwhelming the LLM.
// If truncated, a notice is appended to indicate data was cut off.
func truncateOutput(output string, maxLen int) string {
	if maxLen <= 0 || len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + outputTruncationMarker
}

// truncateCommand truncates a command string for display, replacing newlines with spaces
func truncateCommand(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
