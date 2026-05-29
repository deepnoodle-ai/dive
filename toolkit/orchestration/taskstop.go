package orchestration

import (
	"context"
	"fmt"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

// TaskStopToolInput is the input for the TaskStop tool.
type TaskStopToolInput struct {
	TaskID string `json:"task_id"`
}

// TaskStopToolOptions configures a new TaskStop tool.
type TaskStopToolOptions struct {
	Runs *Runs
}

type taskStopTool struct {
	runs *Runs
}

var _ dive.TypedTool[*TaskStopToolInput] = &taskStopTool{}

// NewTaskStopTool creates the TaskStop tool, which cancels a running background
// task or monitor by its task_id.
func NewTaskStopTool(opts TaskStopToolOptions) *dive.TypedToolAdapter[*TaskStopToolInput] {
	return dive.ToolAdapter(&taskStopTool{runs: opts.Runs})
}

func (t *taskStopTool) Name() string { return "TaskStop" }

func (t *taskStopTool) Description() string {
	return `Cancel a running background task or monitor.

- Pass the task_id returned when the task or monitor was started
- Has no effect on tasks that have already finished`
}

func (t *taskStopTool) Schema() *schema.Schema {
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

func (t *taskStopTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:        "TaskStop",
		ReadOnlyHint: false,
	}
}

func (t *taskStopTool) Call(ctx context.Context, input *TaskStopToolInput) (*dive.ToolResult, error) {
	if input.TaskID == "" {
		return dive.NewToolResultError("task_id is required"), nil
	}
	if t.runs == nil {
		return dive.NewToolResultText(fmt.Sprintf(
			"Task %s not found or already finished — nothing to cancel.", input.TaskID)), nil
	}

	description, ok := t.runs.stop(input.TaskID)
	if !ok {
		return dive.NewToolResultText(fmt.Sprintf(
			"Task %s not found or already finished — nothing to cancel.", input.TaskID)), nil
	}

	return dive.NewToolResultText(fmt.Sprintf("Task %s cancelled.", input.TaskID)).
		WithDisplay(fmt.Sprintf("Stopped: %s", description)), nil
}
