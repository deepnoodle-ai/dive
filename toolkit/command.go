package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/diveagents/dive/llm"
)

var _ llm.ToolWithMetadata = &CommandTool{}

type commandInput struct {
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
func NewCommandTool(options CommandToolOptions) *CommandTool {
	return &CommandTool{
		allowList: options.AllowList,
		denyList:  options.DenyList,
	}
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

func (c *CommandTool) Schema() llm.Schema {
	return llm.Schema{
		Type:     "object",
		Required: []string{"name"},
		Properties: map[string]*llm.SchemaProperty{
			"name": {
				Type:        "string",
				Description: "The name of the command to run",
			},
			"args": {
				Type:        "array",
				Description: "The arguments to pass to the command",
				Items:       &llm.SchemaProperty{Type: "string"},
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

func (c *CommandTool) Call(ctx context.Context, input *llm.ToolCallInput) (*llm.ToolCallOutput, error) {
	var params commandInput
	if err := json.Unmarshal([]byte(input.Input), &params); err != nil {
		return nil, err
	}

	name := params.Name

	var args []string
	for _, arg := range params.Args {
		args = append(args, fmt.Sprintf("%v", arg))
	}

	if name == "" {
		return llm.NewToolCallOutput("error: no command name provided."), nil
	}
	if c.denyList != nil && slices.Contains(c.denyList, name) {
		return llm.NewToolCallOutput(fmt.Sprintf("error: command name %q is not allowed.", name)), nil
	}
	if c.allowList != nil && !slices.Contains(c.allowList, name) {
		return llm.NewToolCallOutput(fmt.Sprintf("error: command name %q is not allowed.", name)), nil
	}

	if input.Confirmer != nil {
		confirmed, err := input.Confirmer.Confirm(ctx, llm.ConfirmationRequest{
			Prompt:  "Do you want to run this command?",
			Details: strings.Join(append([]string{name}, args...), " "),
			Data:    map[string]interface{}{"name": name, "args": args},
		})
		if err != nil {
			return llm.NewToolCallOutput(fmt.Sprintf("error: user confirmation failed: %s", err.Error())), nil
		}
		if !confirmed {
			return llm.NewToolCallOutput("error: command cancelled by user."), nil
		}
	}

	cmd := exec.CommandContext(ctx, name, args...)
	if params.WorkingDirectory != "" {
		cmd.Dir = params.WorkingDirectory
	}

	output, err := cmd.Output()
	if err != nil {
		return llm.NewToolCallOutput(fmt.Sprintf("error: command execution failed: %s", err.Error())), nil
	}

	if params.StdoutFile != "" {
		// create directory if it doesn't exist
		dir := filepath.Dir(params.StdoutFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return llm.NewToolCallOutput(fmt.Sprintf("error: failed to create directory: %s", err.Error())), nil
		}
		if err := os.WriteFile(params.StdoutFile, output, 0644); err != nil {
			return llm.NewToolCallOutput(fmt.Sprintf("error: failed to write command output to file: %s", err.Error())), nil
		}
		return llm.NewToolCallOutput(fmt.Sprintf("Command output written to file: %s", params.StdoutFile)), nil
	}

	return llm.NewToolCallOutput(string(output)), nil
}

func (c *CommandTool) Metadata() llm.ToolMetadata {
	return llm.ToolMetadata{
		Version:    "0.0.1",
		Capability: llm.ToolCapabilityReadWrite,
	}
}
