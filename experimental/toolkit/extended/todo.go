package extended

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/todo"
	"github.com/deepnoodle-ai/wonton/schema"
)

var (
	_ dive.TypedTool[*TodoWriteInput]          = &TodoWriteTool{}
	_ dive.TypedToolPreviewer[*TodoWriteInput] = &TodoWriteTool{}
)

// Type aliases — canonical types are in experimental/todo
type (
	TodoStatus = todo.TodoStatus
	TodoItem   = todo.TodoItem
)

// Status constants for convenience
const (
	TodoStatusPending    = todo.TodoStatusPending
	TodoStatusInProgress = todo.TodoStatusInProgress
	TodoStatusCompleted  = todo.TodoStatusCompleted
)

// TodoWriteInput is the input for the TodoWrite tool
type TodoWriteInput struct {
	Todos []TodoItem `json:"todos"`
}

// TodoWriteToolOptions configures the TodoWriteTool
type TodoWriteToolOptions struct {
	// OnUpdate is called whenever the todo list is updated
	OnUpdate func(todos []TodoItem)
}

// TodoWriteTool manages a structured task list for tracking progress
type TodoWriteTool struct {
	mu       sync.RWMutex
	todos    []TodoItem
	onUpdate func(todos []TodoItem)
}

// NewTodoWriteTool creates a new TodoWriteTool
func NewTodoWriteTool(opts ...TodoWriteToolOptions) *dive.TypedToolAdapter[*TodoWriteInput] {
	var resolvedOpts TodoWriteToolOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}
	return dive.ToolAdapter(&TodoWriteTool{
		onUpdate: resolvedOpts.OnUpdate,
	})
}

func (t *TodoWriteTool) Name() string {
	return "TodoWrite"
}

func (t *TodoWriteTool) Description() string {
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

func (t *TodoWriteTool) Schema() *schema.Schema {
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

func (t *TodoWriteTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "TodoWrite",
		ReadOnlyHint:    false,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

func (t *TodoWriteTool) PreviewCall(ctx context.Context, input *TodoWriteInput) *dive.ToolCallPreview {
	pending := 0
	inProgress := 0
	completed := 0
	for _, todo := range input.Todos {
		switch todo.Status {
		case TodoStatusPending:
			pending++
		case TodoStatusInProgress:
			inProgress++
		case TodoStatusCompleted:
			completed++
		}
	}
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Update todos: %d pending, %d in progress, %d completed", pending, inProgress, completed),
	}
}

func (t *TodoWriteTool) Call(ctx context.Context, input *TodoWriteInput) (*dive.ToolResult, error) {
	// Validate input
	if input.Todos == nil {
		return dive.NewToolResultError("todos array is required"), nil
	}

	// Validate each todo
	for i, todo := range input.Todos {
		if todo.Content == "" {
			return dive.NewToolResultError(fmt.Sprintf("todo[%d].content is required", i)), nil
		}
		if todo.ActiveForm == "" {
			return dive.NewToolResultError(fmt.Sprintf("todo[%d].activeForm is required", i)), nil
		}
		if todo.Status != TodoStatusPending && todo.Status != TodoStatusInProgress && todo.Status != TodoStatusCompleted {
			return dive.NewToolResultError(fmt.Sprintf("todo[%d].status must be 'pending', 'in_progress', or 'completed'", i)), nil
		}
	}

	// Update the stored todos
	t.mu.Lock()
	t.todos = make([]TodoItem, len(input.Todos))
	copy(t.todos, input.Todos)
	todosCopy := make([]TodoItem, len(t.todos))
	copy(todosCopy, t.todos)
	t.mu.Unlock()

	// Call the update callback if set
	if t.onUpdate != nil {
		t.onUpdate(todosCopy)
	}

	// Count statuses for display
	pending := 0
	inProgress := 0
	completed := 0
	var currentTask string
	for _, todo := range input.Todos {
		switch todo.Status {
		case TodoStatusPending:
			pending++
		case TodoStatusInProgress:
			inProgress++
			if currentTask == "" {
				currentTask = todo.ActiveForm
			}
		case TodoStatusCompleted:
			completed++
		}
	}

	// Build response
	response := map[string]interface{}{
		"total":       len(input.Todos),
		"pending":     pending,
		"in_progress": inProgress,
		"completed":   completed,
	}
	responseJSON, _ := json.Marshal(response)

	display := fmt.Sprintf("Todos: %d pending, %d in progress, %d completed", pending, inProgress, completed)
	if currentTask != "" {
		display = fmt.Sprintf("%s • %s", currentTask, display)
	}

	return dive.NewToolResultText(string(responseJSON)).WithDisplay(display), nil
}

// GetTodos returns a copy of the current todo list
func (t *TodoWriteTool) GetTodos() []TodoItem {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]TodoItem, len(t.todos))
	copy(result, t.todos)
	return result
}

// GetCurrentTask returns the currently in-progress task, if any
func (t *TodoWriteTool) GetCurrentTask() *TodoItem {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, todo := range t.todos {
		if todo.Status == TodoStatusInProgress {
			todoCopy := todo
			return &todoCopy
		}
	}
	return nil
}
