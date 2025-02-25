package dive

import (
	"time"
)

// OutputFormat defines the format of task results
type OutputFormat string

const (
	OutputText     OutputFormat = "text"
	OutputMarkdown OutputFormat = "markdown"
	OutputJSON     OutputFormat = "json"
)

type TaskStatus string

const (
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusActive    TaskStatus = "active"
	TaskStatusPaused    TaskStatus = "paused"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusBlocked   TaskStatus = "blocked"
	TaskStatusError     TaskStatus = "error"
	TaskStatusInvalid   TaskStatus = "invalid"
)

// TaskResult holds the output of a completed task
type TaskResult struct {
	// Task is the task that was executed
	Task *Task

	// Raw content of the output
	Content string

	// Format specifies how to interpret the content
	Format OutputFormat

	// For JSON outputs, Object is the parsed JSON object
	Object interface{}

	// Reasoning is the thought process used to arrive at the answer
	Reasoning string

	// Error is the error that occurred during task execution
	Error error

	// StartedAt is the time the task was started
	StartedAt time.Time

	// FinishedAt is the time the task stopped
	FinishedAt time.Time
}
