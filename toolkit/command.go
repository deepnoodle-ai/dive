package toolkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/schema"
)

var _ dive.TypedTool[*CommandInput] = &CommandTool{}
var _ dive.TypedToolPreviewer[*CommandInput] = &CommandTool{}

const (
	DefaultCommandTimeout    = 2 * time.Minute
	MaxCommandTimeout        = 10 * time.Minute
	DefaultBackgroundTimeout = 30 * time.Second
)

type CommandInput struct {
	Name             string `json:"name"`
	Args             []any  `json:"args"`
	Description      string `json:"description,omitempty"`
	Timeout          int    `json:"timeout,omitempty"` // Timeout in milliseconds
	RunInBackground  bool   `json:"run_in_background,omitempty"`
	StdoutFile       string `json:"stdout_file"`
	WorkingDirectory string `json:"working_directory"`
}

// CommandToolOptions is used to configure a new CommandTool.
type CommandToolOptions struct {
	// WorkspaceDir is the base directory for workspace validation (defaults to cwd)
	WorkspaceDir string
	// ShellManager is used for background shell tracking (optional)
	ShellManager *ShellManager
}

// CommandTool is a tool that executes external commands. This could run `cat`,
// `ls`, `git`, `curl`, etc. The command must be available on the system where
// the agent is running.
type CommandTool struct {
	pathValidator *PathValidator
	shellManager  *ShellManager
}

// NewCommandTool creates a new tool that executes external commands.
func NewCommandTool(opts ...CommandToolOptions) *dive.TypedToolAdapter[*CommandInput] {
	var resolvedOpts CommandToolOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}
	pathValidator, err := NewPathValidator(resolvedOpts.WorkspaceDir)
	if err != nil {
		pathValidator = &PathValidator{}
	}
	return dive.ToolAdapter(&CommandTool{
		pathValidator: pathValidator,
		shellManager:  resolvedOpts.ShellManager,
	})
}

func (c *CommandTool) Name() string {
	return "command"
}

func (c *CommandTool) Description() string {
	desc := "Execute shell commands with optional timeout and background execution.\n\n"
	desc += "Parameters:\n"
	desc += "- name: The command to run (required)\n"
	desc += "- args: Arguments to pass to the command\n"
	desc += "- description: Brief description of what the command does (5-10 words)\n"
	desc += "- timeout: Timeout in milliseconds (max 600000ms / 10 minutes, default 120000ms / 2 minutes)\n"
	desc += "- run_in_background: Run the command in background, returns a shell ID for later retrieval\n"
	desc += "- working_directory: Directory to run the command in\n"
	desc += "- stdout_file: Redirect output to this file instead of returning it\n\n"
	desc += fmt.Sprintf("Running on '%s' operating system.", runtime.GOOS)
	return desc
}

func (c *CommandTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"name"},
		Properties: map[string]*schema.Property{
			"name": {
				Type:        "string",
				Description: "The name of the command to run",
			},
			"args": {
				Type:        "array",
				Description: "The arguments to pass to the command",
				Items:       &schema.Property{Type: "string"},
			},
			"description": {
				Type:        "string",
				Description: "A brief description of what this command does (5-10 words)",
			},
			"timeout": {
				Type:        "integer",
				Description: "Timeout in milliseconds (max 600000ms / 10 minutes). Default is 120000ms (2 minutes).",
			},
			"run_in_background": {
				Type:        "boolean",
				Description: "Run the command in the background. Use get_shell_output to retrieve results later.",
			},
			"stdout_file": {
				Type:        "string",
				Description: "Path to a file to redirect the command's output to.",
			},
			"working_directory": {
				Type:        "string",
				Description: "The working directory to run the command in.",
			},
		},
	}
}

func (c *CommandTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Command",
		ReadOnlyHint:    false,
		IdempotentHint:  false,
		DestructiveHint: true,
		OpenWorldHint:   true,
	}
}

func (c *CommandTool) PreviewCall(ctx context.Context, input *CommandInput) *dive.ToolCallPreview {
	var args []string
	for _, arg := range input.Args {
		args = append(args, fmt.Sprintf("%v", arg))
	}
	cmdStr := input.Name
	if len(args) > 0 {
		cmdStr = fmt.Sprintf("%s %s", input.Name, strings.Join(args, " "))
	}

	summary := fmt.Sprintf("Run `%s`", cmdStr)
	if input.RunInBackground {
		summary = fmt.Sprintf("Run `%s` (background)", cmdStr)
	}
	if input.Description != "" {
		summary = input.Description
	}

	return &dive.ToolCallPreview{
		Summary: summary,
	}
}

func (c *CommandTool) Call(ctx context.Context, input *CommandInput) (*dive.ToolResult, error) {
	name := input.Name
	if name == "" {
		return NewToolResultError("error: no command name provided."), nil
	}

	// Validate and convert args
	args, err := c.validateArgs(input.Args)
	if err != nil {
		return NewToolResultError(err.Error()), nil
	}

	// Validate working directory is within workspace
	if input.WorkingDirectory != "" && c.pathValidator != nil && c.pathValidator.WorkspaceDir != "" {
		if err := c.pathValidator.ValidateRead(input.WorkingDirectory); err != nil {
			return NewToolResultError(fmt.Sprintf("error: %s", err.Error())), nil
		}
	}

	// Validate stdout file is within workspace
	if input.StdoutFile != "" && c.pathValidator != nil && c.pathValidator.WorkspaceDir != "" {
		if err := c.pathValidator.ValidateWrite(input.StdoutFile); err != nil {
			return NewToolResultError(fmt.Sprintf("error: %s", err.Error())), nil
		}
	}

	// Handle background execution
	if input.RunInBackground {
		return c.runInBackground(ctx, name, args, input)
	}

	// Run synchronously with timeout
	return c.runSync(ctx, name, args, input)
}

func (c *CommandTool) validateArgs(inputArgs []any) ([]string, error) {
	var args []string
	for _, arg := range inputArgs {
		switch v := arg.(type) {
		case string:
			args = append(args, v)
		case float64:
			if v == float64(int64(v)) {
				args = append(args, strconv.FormatInt(int64(v), 10))
			} else {
				args = append(args, strconv.FormatFloat(v, 'f', -1, 64))
			}
		case int:
			args = append(args, strconv.Itoa(v))
		default:
			return nil, fmt.Errorf("error: args must be strings or numbers, got %T", arg)
		}
	}
	return args, nil
}

func (c *CommandTool) runInBackground(ctx context.Context, name string, args []string, input *CommandInput) (*dive.ToolResult, error) {
	if c.shellManager == nil {
		return NewToolResultError("error: background execution not supported (no shell manager configured)"), nil
	}

	description := input.Description
	if description == "" {
		if len(args) > 0 {
			description = fmt.Sprintf("%s %s", name, strings.Join(args, " "))
		} else {
			description = name
		}
	}

	id, err := c.shellManager.StartBackground(ctx, name, args, description, input.WorkingDirectory)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("error starting background command: %s", err.Error())), nil
	}

	result := map[string]string{
		"shell_id": id,
		"status":   "running",
		"message":  fmt.Sprintf("Command started in background. Use get_shell_output with shell_id '%s' to retrieve results.", id),
	}
	resultJSON, _ := json.Marshal(result)

	display := fmt.Sprintf("Started `%s` in background (id: %s)", name, id)
	return NewToolResultText(string(resultJSON)).WithDisplay(display), nil
}

func (c *CommandTool) runSync(ctx context.Context, name string, args []string, input *CommandInput) (*dive.ToolResult, error) {
	// Calculate timeout
	timeout := DefaultCommandTimeout
	if input.Timeout > 0 {
		timeout = time.Duration(input.Timeout) * time.Millisecond
		if timeout > MaxCommandTimeout {
			timeout = MaxCommandTimeout
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd := exec.CommandContext(ctx, name, args...)
	if input.WorkingDirectory != "" {
		cmd.Dir = input.WorkingDirectory
	}
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()

	output := stdoutBuf.String()
	stderr := stderrBuf.String()

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		return NewToolResultError(fmt.Sprintf("command timed out after %s", timeout)), nil
	}

	resultText, err := json.Marshal(map[string]string{
		"stdout":      output,
		"stderr":      stderr,
		"return_code": strconv.Itoa(cmd.ProcessState.ExitCode()),
	})
	if err != nil {
		return NewToolResultError(fmt.Sprintf("error: failed to marshal command output: %s", err.Error())), nil
	}
	if runErr != nil {
		return NewToolResultError(string(resultText)), nil
	}

	exitCode := cmd.ProcessState.ExitCode()
	display := input.Description
	if display == "" {
		display = fmt.Sprintf("Ran `%s`", name)
	}
	display = fmt.Sprintf("%s (exit %d)", display, exitCode)

	if input.StdoutFile != "" {
		return c.writeOutputToFile(input.StdoutFile, output, display)
	}

	return NewToolResultText(string(resultText)).WithDisplay(display), nil
}

func (c *CommandTool) writeOutputToFile(filePath, output, display string) (*dive.ToolResult, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return NewToolResultError(fmt.Sprintf("error: failed to create directory: %s", err.Error())), nil
	}
	if err := os.WriteFile(filePath, []byte(output), 0644); err != nil {
		return NewToolResultError(fmt.Sprintf("error: failed to write command output to file: %s", err.Error())), nil
	}
	return NewToolResultText(fmt.Sprintf("Command output written to file: %s", filePath)).
		WithDisplay(fmt.Sprintf("%s - output written to %s", display, filePath)), nil
}
