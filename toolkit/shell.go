package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"

	"github.com/diveagents/dive/llm"
)

var _ llm.ToolWithMetadata = &ShellTool{}

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

func (t *ShellTool) Name() string {
	return "shell_run"
}

func (t *ShellTool) Description() string {
	return "A tool that runs a shell command. To use this tool, provide a 'name' parameter with the name of the command you want to run, and an 'args' parameter with the arguments to pass to the command."
}

func (t *ShellTool) Schema() llm.Schema {
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
				Items: &llm.SchemaProperty{
					Type: "string",
				},
			},
		},
	}
}

func (t *ShellTool) Call(ctx context.Context, input *llm.ToolCallInput) (*llm.ToolCallOutput, error) {
	var params ShellInput
	if err := json.Unmarshal([]byte(input.Input), &params); err != nil {
		return nil, err
	}

	name := params.Name
	args := params.Args

	if name == "" {
		return llm.NewToolCallOutput("Error: No command name provided."), nil
	}
	if t.denyList != nil && slices.Contains(t.denyList, name) {
		return llm.NewToolCallOutput(fmt.Sprintf("Error: Command name %s is not allowed.", name)), nil
	}
	if t.allowList != nil && !slices.Contains(t.allowList, name) {
		return llm.NewToolCallOutput(fmt.Sprintf("Error: Command name %s is not allowed.", name)), nil
	}

	if input.Confirmer != nil {
		confirmed, err := input.Confirmer.Confirm(ctx, llm.ConfirmationRequest{
			Prompt:  "Do you want to run this shell command?",
			Details: fmt.Sprintf("Command: %s\nArgs: %v", name, args),
			Data:    map[string]interface{}{"name": name, "args": args},
		})
		if err != nil {
			return llm.NewToolCallOutput(fmt.Sprintf("Error: Confirmation failed: %s", err.Error())), nil
		}
		if !confirmed {
			return llm.NewToolCallOutput("Command cancelled by user."), nil
		}
	}

	output, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return llm.NewToolCallOutput(fmt.Sprintf("Error: %s", err.Error())), nil
	}
	return llm.NewToolCallOutput(string(output)), nil
}

func (t *ShellTool) Metadata() llm.ToolMetadata {
	return llm.ToolMetadata{
		Version:    "0.0.1",
		Capability: llm.ToolCapabilityReadWrite,
	}
}
