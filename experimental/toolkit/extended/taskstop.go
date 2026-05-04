package extended

import (
	"context"
	"fmt"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

var _ dive.TypedTool[*TaskStopToolInput] = &TaskStopTool{}

// TaskStopToolInput is the input for the TaskStopTool
type TaskStopToolInput struct {
	TaskID string `json:"task_id"`
}

// TaskStopToolOptions configures a new TaskStopTool
type TaskStopToolOptions struct {
	Registry *TaskRegistry
}

// TaskStopTool cancels a running task or monitor
type TaskStopTool struct {
	registry *TaskRegistry
}

// NewTaskStopTool creates a new TaskStopTool
func NewTaskStopTool(opts TaskStopToolOptions) *TaskStopTool {
	return &TaskStopTool{registry: opts.Registry}
}

func (t *TaskStopTool) Name() string { return "TaskStop" }

func (t *TaskStopTool) Description() string {
	return `Cancel a running background task or monitor.

- Pass the task_id returned when the task or monitor was started
- Has no effect on already-completed or failed tasks`
}

func (t *TaskStopTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"task_id"},
		Properties: map[string]*schema.Property{
			"task_id": {
				Type:        "string",
				Description: "The task ID to cancel.",
			},
		},
	}
}

func (t *TaskStopTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:        "TaskStop",
		ReadOnlyHint: false,
	}
}

func (t *TaskStopTool) ShouldReturnResult() bool { return true }

func (t *TaskStopTool) Call(ctx context.Context, input *TaskStopToolInput) (*dive.ToolResult, error) {
	if input.TaskID == "" {
		return dive.NewToolResultError("task_id is required"), nil
	}

	record, ok := t.registry.Get(input.TaskID)
	if !ok {
		return dive.NewToolResultError(fmt.Sprintf("task %s not found", input.TaskID)), nil
	}

	snapshot := record.snapshot()
	if snapshot.Status == TaskStatusCompleted || snapshot.Status == TaskStatusFailed {
		return dive.NewToolResultText(fmt.Sprintf(
			"Task %s already %s — nothing to cancel.", input.TaskID, snapshot.Status,
		)), nil
	}

	record.mu.RLock()
	cancel := record.cancel
	record.mu.RUnlock()

	if cancel == nil {
		return dive.NewToolResultError(fmt.Sprintf("task %s does not support cancellation", input.TaskID)), nil
	}

	cancel()

	return dive.NewToolResultText(fmt.Sprintf("Task %s cancelled.", input.TaskID)).
		WithDisplay(fmt.Sprintf("Stopped: %s", snapshot.Description)), nil
}
