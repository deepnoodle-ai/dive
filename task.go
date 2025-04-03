package dive

import (
	"strings"
	"time"

	"github.com/diveagents/dive/llm"
)

// TaskStatus is used to convey the status of a Task that an Agent is working on
type TaskStatus string

const (
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusActive    TaskStatus = "active"
	TaskStatusPaused    TaskStatus = "paused"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusBlocked   TaskStatus = "blocked"
	TaskStatusError     TaskStatus = "error"
	TaskStatusUnknown   TaskStatus = ""
)

type TaskOptions struct {
	Name    string
	Timeout time.Duration
	Prompt  string
}

type Task struct {
	name    string
	timeout time.Duration
	prompt  string
}

func (t *Task) Name() string {
	return t.name
}

func (t *Task) Timeout() time.Duration {
	return t.timeout
}

func (t *Task) Prompt() string {
	return t.prompt
}

func NewTask(opts TaskOptions) *Task {
	return &Task{
		name:    opts.Name,
		timeout: opts.Timeout,
		prompt:  opts.Prompt,
	}
}

// StepResult holds the output of a completed task.
type StepResult struct {
	// Content contains the raw output
	Content string

	// Format specifies how to interpret the content
	Format OutputFormat

	// Object holds parsed JSON output if applicable
	Object interface{}

	// Usage tracks LLM token usage
	Usage llm.Usage
}

// StructuredResponse is an interpreted response from an Agent which may include
// status, thinking, and content.
type StructuredResponse struct {
	rawText           string
	thinking          string
	content           string
	statusDescription string
	status            TaskStatus
}

func (s *StructuredResponse) Status() TaskStatus {
	return s.status
}

func (s *StructuredResponse) StatusDescription() string {
	return s.statusDescription
}

func (s *StructuredResponse) Thinking() string {
	return s.thinking
}

func (s *StructuredResponse) Content() string {
	return s.content
}

func (s *StructuredResponse) RawText() string {
	return s.rawText
}

func extractStatus(text string) TaskStatus {
	fields := strings.Fields(strings.ToLower(text))
	if len(fields) == 0 {
		return TaskStatusUnknown
	}
	// Find the first matching status
	for _, field := range fields {
		value := strings.TrimPrefix(field, "\"")
		value = strings.TrimSuffix(value, "\"")
		switch value {
		case "active":
			return TaskStatusActive
		case "paused":
			return TaskStatusPaused
		case "completed":
			return TaskStatusCompleted
		case "blocked":
			return TaskStatusBlocked
		case "error":
			return TaskStatusError
		}
	}
	return TaskStatusUnknown
}

func ParseStructuredResponse(text string) StructuredResponse {
	var thinking, statusDescription string
	workingText := text

	// Extract status if present
	statusStart := strings.Index(workingText, "<status>")
	statusEnd := strings.Index(workingText, "</status>")
	if statusStart != -1 && statusEnd != -1 && statusEnd > statusStart {
		statusDescription = strings.TrimSpace(workingText[statusStart+8 : statusEnd])
		// Remove the status tag and its content
		workingText = workingText[:statusStart] + workingText[statusEnd+9:]
	}

	// Extract thinking if present
	thinkStart := strings.Index(workingText, "<think>")
	thinkEnd := strings.Index(workingText, "</think>")
	if thinkStart != -1 && thinkEnd != -1 && thinkEnd > thinkStart {
		thinking = strings.TrimSpace(workingText[thinkStart+7 : thinkEnd])
		// Remove the think tag and its content
		workingText = workingText[:thinkStart] + workingText[thinkEnd+8:]
	}

	// The response is whatever text remains after trimming whitespace
	response := strings.TrimSpace(workingText)

	// Extract a specific status if present
	var status TaskStatus
	if statusDescription != "" {
		status = extractStatus(statusDescription)
	}

	return StructuredResponse{
		rawText:           text,
		thinking:          thinking,
		content:           response,
		statusDescription: statusDescription,
		status:            status,
	}
}
