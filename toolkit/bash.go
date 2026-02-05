package toolkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	// DefaultMaxOutputLength is the default maximum output length in characters
	DefaultMaxOutputLength = 30000
)

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

	// WorkingDirectory sets the working directory for command execution.
	// If empty, the command runs in the current working directory.
	// Must be within the workspace if path validation is enabled.
	WorkingDirectory string `json:"working_directory,omitempty"`
}

// BashToolOptions configures the behavior of [BashTool].
type BashToolOptions struct {
	// WorkspaceDir restricts command execution to paths within this directory.
	// When set, the working directory must be within this path.
	// Defaults to the current working directory if empty.
	WorkspaceDir string

	// MaxOutputLength limits the combined stdout/stderr output size in characters.
	// Output exceeding this limit is truncated with a warning.
	// Defaults to [DefaultMaxOutputLength] (30000 characters).
	MaxOutputLength int
}

// BashTool executes shell commands and captures their output.
//
// On Unix systems, commands are executed via /bin/bash -c. On Windows, commands
// are executed via cmd /C. The tool captures stdout, stderr, and the exit code.
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

	pathValidator, configErr := NewPathValidator(resolvedOpts.WorkspaceDir)
	if configErr != nil {
		configErr = fmt.Errorf("invalid workspace configuration for WorkspaceDir %q: %w", resolvedOpts.WorkspaceDir, configErr)
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
- working_directory: Directory to run the command in

Limitations:
- No interactive commands (vim, less, password prompts)
- No GUI applications
- Large outputs may be truncated

`
	desc += fmt.Sprintf("Running on '%s' operating system.", runtime.GOOS)
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
				Description: "The working directory for command execution.",
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
// The result includes stdout, stderr, and the exit code as a JSON object.
// If the command fails (non-zero exit code), an error result is returned
// but no Go error is returned - the LLM receives the failure information.
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
	stdout, stderr, exitCode, err := t.execute(ctx, input.Command, input.WorkingDirectory, timeout)
	if err != nil {
		return dive.NewToolResultError(err.Error()), nil
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

// execute runs a command with the given timeout and returns its output.
// It handles context cancellation, timeout enforcement, and output truncation.
func (t *BashTool) execute(ctx context.Context, command, workingDir string, timeout time.Duration) (stdout, stderr string, exitCode int, err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Determine shell based on OS
	shell, shellArgs := shellCommand()
	shellArgs = append(shellArgs, command)

	cmd := exec.CommandContext(ctx, shell, shellArgs...)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()
	exitCode = 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return "", "", -1, fmt.Errorf("command timed out after %s", timeout)
		} else {
			return "", "", -1, fmt.Errorf("error: %s", runErr.Error())
		}
	}

	stdout = truncateOutput(stdoutBuf.String(), t.maxOutputLen)
	stderr = truncateOutput(stderrBuf.String(), t.maxOutputLen)

	return stdout, stderr, exitCode, nil
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
	return output[:maxLen] + "\n... (output truncated)"
}

// truncateCommand truncates a command string for display, replacing newlines with spaces
func truncateCommand(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
