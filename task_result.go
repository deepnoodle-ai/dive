package dive

import (
	"time"

	"github.com/getstingrai/dive/llm"
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
	TaskStatusError     TaskStatus = "error"
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

type TaskState struct {
	task           *Task
	promise        *Promise
	status         TaskStatus
	priority       int
	started        time.Time
	output         string
	reasoning      string
	reportedStatus string
	messages       []*llm.Message
	suspended      bool
	chatResult     chan *llm.Response
	chatError      chan error
}

func (s *TaskState) Task() *Task {
	return s.task
}

func (s *TaskState) Output() string {
	return s.output
}

func (s *TaskState) Reasoning() string {
	return s.reasoning
}

func (s *TaskState) Status() TaskStatus {
	return s.status
}

func (s *TaskState) ReportedStatus() string {
	return s.reportedStatus
}

func (s *TaskState) Messages() []*llm.Message {
	return s.messages
}

func (s *TaskState) String() string {
	text, err := executeTemplate(taskStatePromptTemplate, s)
	if err != nil {
		panic(err)
	}
	return text
}
