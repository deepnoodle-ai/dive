package orchestration

import (
	"context"
	"fmt"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

// run is a single cancellable background run tracked by Runs.
type run struct {
	description string
	cancel      context.CancelFunc
}

// Runs tracks cancellable background runs — background subagent spawns and
// monitors — keyed by task_id, so the TaskStop tool can cancel them. It is the
// entire shared substrate for the control axis: construct one with NewRuns and
// hand it to the Agent, Monitor, and TaskStop tools. Safe for concurrent use.
type Runs struct {
	mu sync.Mutex
	m  map[string]run
}

// NewRuns creates an empty run tracker.
func NewRuns() *Runs {
	return &Runs{m: make(map[string]run)}
}

// add registers a cancellable run under id. Called by the tools that start
// background work (the Agent spawner and Monitor).
func (r *Runs) add(id, description string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[id] = run{description: description, cancel: cancel}
}

// remove drops a run that has finished on its own.
func (r *Runs) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, id)
}

// stop cancels the run with the given id and drops it, returning the run's
// description and whether a matching run was found.
func (r *Runs) stop(id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.m[id]
	if !ok {
		return "", false
	}
	delete(r.m, id)
	h.cancel()
	return h.description, true
}

// --- TaskStop tool ---

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
