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
	"slices"
	"strconv"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/schema"
)

var _ dive.TypedTool[*CommandInput] = &CommandTool{}
var _ dive.TypedToolPreviewer[*CommandInput] = &CommandTool{}

type CommandInput struct {
	Name             string `json:"name"`
	Args             []any  `json:"args"`
	StdoutFile       string `json:"stdout_file"`
	WorkingDirectory string `json:"working_directory"`
}

// CommandToolOptions is used to configure a new CommandTool.
type CommandToolOptions struct {
	AllowList []string
	DenyList  []string
}

// CommandTool is a tool that executes external commands. This could run `cat`,
// `ls`, `git`, `curl`, etc. The command must be available on the system where
// the agent is running.
type CommandTool struct {
	allowList []string
	denyList  []string
}

// NewCommandTool creates a new tool that executes external commands.
func NewCommandTool(opts ...CommandToolOptions) *dive.TypedToolAdapter[*CommandInput] {
	var resolvedOpts CommandToolOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}
	return dive.ToolAdapter(&CommandTool{
		allowList: resolvedOpts.AllowList,
		denyList:  resolvedOpts.DenyList,
	})
}

func (c *CommandTool) Name() string {
	return "command"
}

func (c *CommandTool) Description() string {
	desc := "A tool that runs an external command. "
	desc += "To use this tool, provide a 'name' parameter with the name of the command you want to run, and an 'args' parameter with the arguments to pass to the command. "
	desc += "For example, to run `ls -l`, you would provide `name` with the value `ls` and `args` with the value [\"-l\"]. "
	desc += "The command must be available on the system where the agent is running. "
	desc += fmt.Sprintf("This agent is running on the '%s' operating system, so keep that in mind when specifying commands. ", runtime.GOOS)
	desc += "If the command is not available, the tool will return an error. "
	if len(c.allowList) > 0 {
		desc += fmt.Sprintf("The command must be in the allow list: %v. ", c.allowList)
	}
	if len(c.denyList) > 0 {
		desc += fmt.Sprintf("The command must not be in the deny list: %v. ", c.denyList)
	}
	return strings.TrimSpace(desc)
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
			"stdout_file": {
				Type:        "string",
				Description: "The path to a file to redirect the command's output to. If provided, the command's output will be written to this file instead of being returned to the agent.",
			},
			"working_directory": {
				Type:        "string",
				Description: "The working directory to run the command in. If not provided, the command will run in the current working directory.",
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
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Run `%s`", cmdStr),
	}
}

func (c *CommandTool) Call(ctx context.Context, input *CommandInput) (*dive.ToolResult, error) {

	name := input.Name

	var args []string
	for _, arg := range input.Args {
		args = append(args, fmt.Sprintf("%v", arg))
	}

	if name == "" {
		return NewToolResultError("error: no command name provided."), nil
	}
	if c.denyList != nil && slices.Contains(c.denyList, name) {
		return NewToolResultError(fmt.Sprintf("error: command name %q is not allowed.", name)), nil
	}
	if c.allowList != nil && !slices.Contains(c.allowList, name) {
		return NewToolResultError(fmt.Sprintf("error: command name %q is not allowed.", name)), nil
	}

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
	display := fmt.Sprintf("Ran `%s` (exit %d)", name, exitCode)

	if input.StdoutFile != "" {
		// create directory if it doesn't exist
		dir := filepath.Dir(input.StdoutFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return NewToolResultError(fmt.Sprintf("error: failed to create directory: %s", err.Error())), nil
		}
		if err := os.WriteFile(input.StdoutFile, []byte(output), 0644); err != nil {
			return NewToolResultError(fmt.Sprintf("error: failed to write command output to file: %s", err.Error())), nil
		}
		return NewToolResultText(fmt.Sprintf("Command output written to file: %s", input.StdoutFile)).
			WithDisplay(fmt.Sprintf("%s - output written to %s", display, input.StdoutFile)), nil
	}

	return NewToolResultText(string(resultText)).WithDisplay(display), nil
}
