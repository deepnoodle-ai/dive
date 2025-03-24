package workflow

import (
	"time"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/llm"
)

// TaskResult holds the output of a completed task
type TaskResult struct {
	// Task is the task that was executed
	Task *dive.Task

	// Raw content of the output
	Content string

	// Format specifies how to interpret the content
	Format dive.OutputFormat

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

	// Usage is the usage of the LLM
	Usage llm.Usage
}
