package todo

import (
	"context"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

// ToolName is the canonical name the LLM sees for the TodoWrite tool.
// Hooks that walk message history use this constant to detect prior calls.
const ToolName = "TodoWrite"

var (
	_ dive.TypedTool[*WriteInput]          = &Tool{}
	_ dive.TypedToolPreviewer[*WriteInput] = &Tool{}
)

// WriteInput is the input for the TodoWrite tool.
type WriteInput struct {
	Todos []TodoItem `json:"todos"`
}

// ToolOption configures a Tool.
type ToolOption func(*Tool)

// WithOnUpdate registers a callback invoked synchronously each time the tool
// is called with a valid todo list. Use this for external observation (UI
// rendering, telemetry). The callback receives a defensive copy.
//
// For per-CreateResponse observation prefer the [TodoTracker] event handler;
// OnUpdate is fine when the same Tool instance is dedicated to one session.
func WithOnUpdate(fn func(todos []TodoItem)) ToolOption {
	return func(t *Tool) {
		t.onUpdate = fn
	}
}

// Tool is the stateless TodoWrite tool. It validates input, optionally
// notifies an OnUpdate callback, and returns a tool result whose text matches
// the canonical Claude TodoWrite acknowledgment shape (which Claude is tuned
// on) followed by a one-line progress summary.
//
// State is intentionally NOT stored on the tool. The conversation message
// history is the source of truth for the current todo list. See [Extension]
// for the recommended way to wire the tool into an agent — it adds a
// PreGenerationHook that walks message history and injects a stale-list
// reminder via [dive.SetSystemReminder] when the model has not used the tool
// in many turns.
type Tool struct {
	onUpdate func(todos []TodoItem)
}

// NewTool creates a new TodoWrite tool. Wrap with the adapter so it satisfies
// dive.Tool. Most callers should prefer [New] (the Extension) which also
// installs the stale-list reminder hook.
func NewTool(opts ...ToolOption) *dive.TypedToolAdapter[*WriteInput] {
	t := &Tool{}
	for _, opt := range opts {
		opt(t)
	}
	return dive.ToolAdapter(t)
}

func (t *Tool) Name() string {
	return ToolName
}

func (t *Tool) Description() string {
	return `Manage a structured task list for tracking progress. Helps organize complex tasks and gives users visibility into progress on their requests.

When to use (proactively):
- Complex multi-step tasks requiring 3+ distinct steps
- Non-trivial tasks needing careful planning
- User explicitly requests a todo list
- User provides multiple tasks (numbered or comma-separated)
- After receiving new instructions: immediately capture requirements
- When starting a task: mark in_progress BEFORE beginning work
- After completing: mark complete and add any follow-up tasks discovered

When NOT to use:
- Single, straightforward task
- Trivial tasks under 3 simple steps
- Purely conversational or informational requests

Task statuses:
- pending: Task not yet started
- in_progress: Currently working on (only ONE at a time)
- completed: Task finished successfully

Each todo has two forms:
- content: Imperative form (e.g., "Run tests")
- activeForm: Present continuous form (e.g., "Running tests")

Management rules:
- One in_progress at a time
- Mark in_progress BEFORE starting work
- Mark completed immediately after finishing (don't batch)
- Each call replaces the entire list, so include all items
- Only mark completed if truly finished, not if blocked
- Add follow-up tasks discovered during implementation
- Remove tasks no longer relevant`
}

func (t *Tool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"todos"},
		Properties: map[string]*schema.Property{
			"todos": {
				Type:        "array",
				Description: "The complete updated todo list",
				Items: &schema.Property{
					Type: "object",
					Properties: map[string]*schema.Property{
						"content": {
							Type:        "string",
							Description: "The task description in imperative form (e.g., 'Run tests')",
						},
						"status": {
							Type:        "string",
							Enum:        []any{"pending", "in_progress", "completed"},
							Description: "The task status: pending, in_progress, or completed",
						},
						"activeForm": {
							Type:        "string",
							Description: "The task in present continuous form (e.g., 'Running tests')",
						},
					},
				},
			},
		},
	}
}

func (t *Tool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           ToolName,
		ReadOnlyHint:    false,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

func (t *Tool) PreviewCall(_ context.Context, input *WriteInput) *dive.ToolCallPreview {
	pending, inProgress, completed, _ := countByStatus(input.Todos)
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Update todos: %d pending, %d in progress, %d completed",
			pending, inProgress, completed),
	}
}

func (t *Tool) Call(_ context.Context, input *WriteInput) (*dive.ToolResult, error) {
	if input.Todos == nil {
		return dive.NewToolResultError("todos array is required"), nil
	}
	for i, item := range input.Todos {
		if item.Content == "" {
			return dive.NewToolResultError(fmt.Sprintf("todo[%d].content is required", i)), nil
		}
		if item.ActiveForm == "" {
			return dive.NewToolResultError(fmt.Sprintf("todo[%d].activeForm is required", i)), nil
		}
		if item.Status != TodoStatusPending && item.Status != TodoStatusInProgress && item.Status != TodoStatusCompleted {
			return dive.NewToolResultError(fmt.Sprintf("todo[%d].status must be 'pending', 'in_progress', or 'completed'", i)), nil
		}
	}

	if t.onUpdate != nil {
		todosCopy := make([]TodoItem, len(input.Todos))
		copy(todosCopy, input.Todos)
		t.onUpdate(todosCopy)
	}

	pending, inProgress, completed, currentTask := countByStatus(input.Todos)

	// Tool result text shaped like Claude Code's canonical TodoWrite
	// acknowledgment (which Claude is tuned on), followed by a single
	// progress line so the model retains the running counts without
	// re-deriving them from history.
	var sb strings.Builder
	sb.WriteString("Todos have been modified successfully. Ensure that you continue to use the todo list to track your progress. Please proceed with the current tasks if applicable.")
	if len(input.Todos) > 0 {
		sb.WriteString(fmt.Sprintf("\n\nProgress: %d completed, %d in progress, %d pending (%d total).",
			completed, inProgress, pending, len(input.Todos)))
		if pending == 0 && inProgress == 0 && completed > 0 {
			sb.WriteString(" All tasks complete — if more work remains, write a fresh list; otherwise wrap up.")
		}
	}

	display := fmt.Sprintf("Todos: %d pending, %d in progress, %d completed", pending, inProgress, completed)
	if currentTask != "" {
		display = fmt.Sprintf("%s • %s", currentTask, display)
	}

	return dive.NewToolResultText(sb.String()).WithDisplay(display), nil
}

// countByStatus tallies items by status and returns the active form of the
// first in_progress item, if any.
func countByStatus(items []TodoItem) (pending, inProgress, completed int, currentTask string) {
	for _, item := range items {
		switch item.Status {
		case TodoStatusPending:
			pending++
		case TodoStatusInProgress:
			inProgress++
			if currentTask == "" {
				currentTask = item.ActiveForm
			}
		case TodoStatusCompleted:
			completed++
		}
	}
	return
}
