package todo

import "github.com/deepnoodle-ai/dive"

// ItemType is the ResponseItemType for todo list updates.
// Use this to filter ResponseItems in an EventCallback.
const ItemType dive.ResponseItemType = "todo"

// TodoStatus represents the status of a todo item.
type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusCompleted  TodoStatus = "completed"
)

// TodoItem represents a single todo item in a task list.
type TodoItem struct {
	// Content is the task description in imperative form (e.g., "Run tests")
	Content string `json:"content"`
	// Status is the task status: pending, in_progress, or completed
	Status TodoStatus `json:"status"`
	// ActiveForm is the task in present continuous form (e.g., "Running tests")
	ActiveForm string `json:"activeForm"`
}

// TodoEvent is emitted when the todo list is updated.
//
// This event allows consumers to track task progress in real-time. The event
// contains the complete current state of the todo list, not just the changes.
type TodoEvent struct {
	// Todos is the complete current todo list
	Todos []TodoItem `json:"todos"`
}
