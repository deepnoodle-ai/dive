package extended

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

var (
	_ dive.TypedTool[*KillShellInput]          = &KillShellTool{}
	_ dive.TypedToolPreviewer[*KillShellInput] = &KillShellTool{}
)

// KillShellInput is the input for the kill_shell tool
type KillShellInput struct {
	ShellID string `json:"shell_id"`
}

// KillShellToolOptions configures the KillShellTool
type KillShellToolOptions struct {
	ShellManager *ShellManager
}

// KillShellTool terminates a running background shell process
type KillShellTool struct {
	shellManager *ShellManager
}

// NewKillShellTool creates a new KillShellTool
func NewKillShellTool(opts KillShellToolOptions) *dive.TypedToolAdapter[*KillShellInput] {
	return dive.ToolAdapter(&KillShellTool{
		shellManager: opts.ShellManager,
	})
}

func (t *KillShellTool) Name() string {
	return "KillShell"
}

func (t *KillShellTool) Description() string {
	return `Terminate a running background shell process.

Takes a shell_id parameter identifying the shell to kill.
Returns success or failure status.

Use this tool when you need to stop a long-running background command.`
}

func (t *KillShellTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"shell_id"},
		Properties: map[string]*schema.Property{
			"shell_id": {
				Type:        "string",
				Description: "The ID of the background shell to kill",
			},
		},
	}
}

func (t *KillShellTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "KillShell",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

func (t *KillShellTool) PreviewCall(ctx context.Context, input *KillShellInput) *dive.ToolCallPreview {
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Kill shell %s", input.ShellID),
	}
}

func (t *KillShellTool) Call(ctx context.Context, input *KillShellInput) (*dive.ToolResult, error) {
	if t.shellManager == nil {
		return dive.NewToolResultError("shell manager not configured"), nil
	}

	if input.ShellID == "" {
		return dive.NewToolResultError("shell_id is required"), nil
	}

	// Get shell info before killing
	info, exists := t.shellManager.Get(input.ShellID)
	if !exists {
		return dive.NewToolResultError(fmt.Sprintf("shell not found: %s", input.ShellID)), nil
	}

	// Check if already terminated
	if info.Status != ShellStatusRunning {
		result := map[string]interface{}{
			"shell_id": input.ShellID,
			"status":   string(info.Status),
			"message":  "shell is not running",
		}
		resultJSON, _ := json.Marshal(result)
		return dive.NewToolResultText(string(resultJSON)).
			WithDisplay(fmt.Sprintf("Shell %s is not running (status: %s)", input.ShellID, info.Status)), nil
	}

	// Kill the shell
	if err := t.shellManager.Kill(input.ShellID); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("failed to kill shell: %s", err.Error())), nil
	}

	// Get updated info
	info, _ = t.shellManager.Get(input.ShellID)

	result := map[string]interface{}{
		"shell_id": input.ShellID,
		"status":   "killed",
		"message":  "shell terminated successfully",
	}
	resultJSON, _ := json.Marshal(result)

	display := fmt.Sprintf("Killed shell %s", input.ShellID)
	if info != nil && info.Description != "" {
		display = fmt.Sprintf("Killed shell %s (%s)", input.ShellID, info.Description)
	}

	return dive.NewToolResultText(string(resultJSON)).WithDisplay(display), nil
}
