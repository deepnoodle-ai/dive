package extended

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

var (
	_ dive.TypedTool[*GetShellOutputInput]          = &GetShellOutputTool{}
	_ dive.TypedToolPreviewer[*GetShellOutputInput] = &GetShellOutputTool{}
)

const (
	DefaultGetShellOutputTimeout = 30 * time.Second
	MaxGetShellOutputTimeout     = 10 * time.Minute
)

// GetShellOutputInput is the input for the get_shell_output tool
type GetShellOutputInput struct {
	ShellID string `json:"shell_id"`
	Block   *bool  `json:"block,omitempty"`   // Whether to wait for completion (default: true)
	Timeout int    `json:"timeout,omitempty"` // Max wait time in milliseconds (default: 30000)
}

// GetShellOutputToolOptions configures the GetShellOutputTool
type GetShellOutputToolOptions struct {
	ShellManager *ShellManager
}

// GetShellOutputTool retrieves output from a background shell process
type GetShellOutputTool struct {
	shellManager *ShellManager
}

// NewGetShellOutputTool creates a new GetShellOutputTool
func NewGetShellOutputTool(opts GetShellOutputToolOptions) *dive.TypedToolAdapter[*GetShellOutputInput] {
	return dive.ToolAdapter(&GetShellOutputTool{
		shellManager: opts.ShellManager,
	})
}

func (t *GetShellOutputTool) Name() string {
	return "GetShellOutput"
}

func (t *GetShellOutputTool) Description() string {
	return `Retrieve output from a running or completed background shell.

Parameters:
- shell_id: The ID of the shell to get output from (required)
- block: Whether to wait for completion (default: true)
- timeout: Max wait time in milliseconds (default: 30000, max: 600000)

Returns the shell's stdout, stderr, status, and exit code (if completed).

Use block=false for non-blocking check of current status.
Use block=true (default) to wait for the command to finish.`
}

func (t *GetShellOutputTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"shell_id"},
		Properties: map[string]*schema.Property{
			"shell_id": {
				Type:        "string",
				Description: "The ID of the background shell to get output from",
			},
			"block": {
				Type:        "boolean",
				Description: "Whether to wait for completion. Default is true.",
			},
			"timeout": {
				Type:        "integer",
				Description: "Max wait time in milliseconds (default: 30000, max: 600000)",
			},
		},
	}
}

func (t *GetShellOutputTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "GetShellOutput",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

func (t *GetShellOutputTool) PreviewCall(ctx context.Context, input *GetShellOutputInput) *dive.ToolCallPreview {
	blocking := "blocking"
	if input.Block != nil && !*input.Block {
		blocking = "non-blocking"
	}
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Get output from shell %s (%s)", input.ShellID, blocking),
	}
}

func (t *GetShellOutputTool) Call(ctx context.Context, input *GetShellOutputInput) (*dive.ToolResult, error) {
	if t.shellManager == nil {
		return dive.NewToolResultError("shell manager not configured"), nil
	}

	if input.ShellID == "" {
		return dive.NewToolResultError("shell_id is required"), nil
	}

	// Determine blocking behavior
	block := true
	if input.Block != nil {
		block = *input.Block
	}

	// Determine timeout
	timeout := DefaultGetShellOutputTimeout
	if input.Timeout > 0 {
		timeout = time.Duration(input.Timeout) * time.Millisecond
		if timeout > MaxGetShellOutputTimeout {
			timeout = MaxGetShellOutputTimeout
		}
	}

	// Get output from shell manager
	stdout, stderr, info, err := t.shellManager.GetOutput(input.ShellID, block, timeout)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("error getting shell output: %s", err.Error())), nil
	}

	// Build result
	result := map[string]interface{}{
		"shell_id": input.ShellID,
		"status":   string(info.Status),
		"stdout":   stdout,
		"stderr":   stderr,
	}

	if info.ExitCode != nil {
		result["exit_code"] = *info.ExitCode
	}
	if info.Error != "" {
		result["error"] = info.Error
	}
	if info.Description != "" {
		result["description"] = info.Description
	}

	resultJSON, _ := json.Marshal(result)

	// Build display
	display := fmt.Sprintf("Shell %s: %s", input.ShellID, info.Status)
	if info.ExitCode != nil {
		display = fmt.Sprintf("%s (exit %d)", display, *info.ExitCode)
	}
	if info.Description != "" {
		display = fmt.Sprintf("%s - %s", info.Description, info.Status)
		if info.ExitCode != nil {
			display = fmt.Sprintf("%s (exit %d)", display, *info.ExitCode)
		}
	}

	return dive.NewToolResultText(string(resultJSON)).WithDisplay(display), nil
}

// ListShellsInput is the input for listing all shells
type ListShellsInput struct {
	OnlyRunning bool `json:"only_running,omitempty"`
}

// ListShellsTool lists all background shells
type ListShellsTool struct {
	shellManager *ShellManager
}

var (
	_ dive.TypedTool[*ListShellsInput]          = &ListShellsTool{}
	_ dive.TypedToolPreviewer[*ListShellsInput] = &ListShellsTool{}
)

// ListShellsToolOptions configures the ListShellsTool
type ListShellsToolOptions struct {
	ShellManager *ShellManager
}

// NewListShellsTool creates a new ListShellsTool
func NewListShellsTool(opts ListShellsToolOptions) *dive.TypedToolAdapter[*ListShellsInput] {
	return dive.ToolAdapter(&ListShellsTool{
		shellManager: opts.ShellManager,
	})
}

func (t *ListShellsTool) Name() string {
	return "ListShells"
}

func (t *ListShellsTool) Description() string {
	return `List all background shell processes.

Returns information about all shells including their ID, command, status, and timing.

Use only_running=true to filter to only running shells.`
}

func (t *ListShellsTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type: "object",
		Properties: map[string]*schema.Property{
			"only_running": {
				Type:        "boolean",
				Description: "Only list currently running shells",
			},
		},
	}
}

func (t *ListShellsTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "ListShells",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

func (t *ListShellsTool) PreviewCall(ctx context.Context, input *ListShellsInput) *dive.ToolCallPreview {
	if input.OnlyRunning {
		return &dive.ToolCallPreview{Summary: "List running shells"}
	}
	return &dive.ToolCallPreview{Summary: "List all shells"}
}

func (t *ListShellsTool) Call(ctx context.Context, input *ListShellsInput) (*dive.ToolResult, error) {
	if t.shellManager == nil {
		return dive.NewToolResultError("shell manager not configured"), nil
	}

	var shells []ShellInfo
	if input.OnlyRunning {
		shells = t.shellManager.ListRunning()
	} else {
		shells = t.shellManager.List()
	}

	result := map[string]interface{}{
		"count":  len(shells),
		"shells": shells,
	}
	resultJSON, _ := json.Marshal(result)

	running := 0
	for _, s := range shells {
		if s.Status == ShellStatusRunning {
			running++
		}
	}

	display := fmt.Sprintf("Found %d shell(s)", len(shells))
	if running > 0 {
		display = fmt.Sprintf("%s (%d running)", display, running)
	}

	return dive.NewToolResultText(string(resultJSON)).WithDisplay(display), nil
}
