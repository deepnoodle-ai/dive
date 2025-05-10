package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"

	"github.com/diveagents/dive/llm"
)

type ShellInput struct {
	Name string   `json:"name"`
	Args []string `json:"args"`
}

type ShellToolOptions struct {
	AllowList []string
	DenyList  []string
}

type ShellTool struct {
	allowList []string
	denyList  []string
}

// NewShellTool creates a new tool for reading file contents
func NewShellTool(options ShellToolOptions) *ShellTool {
	return &ShellTool{
		allowList: options.AllowList,
		denyList:  options.DenyList,
	}
}

func (t *ShellTool) Definition() *llm.ToolDefinition {
	description := "A tool that runs a shell command. To use this tool, provide a 'name' parameter with the name of the command you want to run, and an 'args' parameter with the arguments to pass to the command."

	return &llm.ToolDefinition{
		Name:        "shell_run",
		Description: description,
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"name", "args"},
			Properties: map[string]*llm.SchemaProperty{
				"name": {
					Type:        "string",
					Description: "The name of the command to run",
				},
				"args": {
					Type:        "array",
					Description: "The arguments to pass to the command",
					Items: &llm.SchemaProperty{
						Type: "string",
					},
				},
			},
		},
	}
}

func (t *ShellTool) Call(ctx context.Context, input string) (string, error) {
	var params ShellInput
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", err
	}
	name := params.Name
	args := params.Args
	if name == "" {
		return "Error: No command name provided. Please provide a command name to run.", nil
	}
	if t.denyList != nil && slices.Contains(t.denyList, name) {
		return fmt.Sprintf("Error: Command name %s is not allowed.", name), nil
	}
	if t.allowList != nil && !slices.Contains(t.allowList, name) {
		return fmt.Sprintf("Error: Command name %s is not allowed.", name), nil
	}
	fmt.Println("Running command:", name, args)
	output, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}
	return string(output), nil
}

func (t *ShellTool) ShouldReturnResult() bool {
	return true
}
