package dive

import (
	"context"
	"time"

	"github.com/diveagents/dive/llm"
)

// OutputFormat defines the desired output format for a Task
type OutputFormat string

const (
	OutputFormatText     OutputFormat = "text"
	OutputFormatMarkdown OutputFormat = "markdown"
	OutputFormatJSON     OutputFormat = "json"
)

// TaskStatus indicates a Task's execution status
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

// Input defines an expected input parameter
type Input struct {
	Name        string      `json:"name"`
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Default     interface{} `json:"default,omitempty"`
}

// Output defines an expected output parameter
type Output struct {
	Name        string      `json:"name"`
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Format      string      `json:"format,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Document    string      `json:"document,omitempty"`
}

// PromptContext is a named block of information carried by a Prompt
type PromptContext struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Text        string `json:"text,omitempty"`
}

// Prompt is a structured representation of an LLM prompt
type Prompt struct {
	Name         string           `json:"name"`
	Text         string           `json:"text,omitempty"`
	Context      []*PromptContext `json:"context,omitempty"`
	Output       string           `json:"output,omitempty"`
	OutputFormat OutputFormat     `json:"output_format,omitempty"`
}

// Agent represents an intelligent agent that can work on tasks and respond to
// chat messages.
type Agent interface {

	// Name of the Agent
	Name() string

	// Goal of the Agent
	Goal() string

	// IsSupervisor indicates whether the Agent can assign work to other Agents
	IsSupervisor() bool

	// SetEnvironment sets the runtime Environment to which this Agent belongs
	SetEnvironment(env Environment) error

	// Chat gives the agent messages to respond to and returns a stream of events
	Chat(ctx context.Context, messages []*llm.Message, opts ...ChatOption) (EventStream, error)

	// Work gives the agent a task to complete
	Work(ctx context.Context, task Task) (EventStream, error)
}

// RunnableAgent is an agent that must be started and stopped.
type RunnableAgent interface {
	Agent

	// Start the agent
	Start(ctx context.Context) error

	// Stop the agent
	Stop(ctx context.Context) error

	// IsRunning returns true if the agent is running
	IsRunning() bool
}

// Environment is a container for running Agents and Workflows. Interactivity
// between Agents is generally scoped to a single Environment.
type Environment interface {

	// Name of the Environment
	Name() string

	// Agents returns the list of all Agents belonging to this Environment
	Agents() []Agent

	// RegisterAgent adds an Agent to this Environment
	RegisterAgent(agent Agent) error

	// GetAgent returns the Agent with the given name, if found
	GetAgent(name string) (Agent, error)

	// DocumentRepository returns the DocumentRepository for this Environment
	DocumentRepository() DocumentRepository

	// ThreadRepository returns the ThreadRepository for this Environment
	ThreadRepository() ThreadRepository
}

// ChatOptions contains configuration for LLM generations.
type ChatOptions struct {
	ThreadID string
	UserID   string
}

// ChatOption is a type signature for defining new LLM generation options.
type ChatOption func(*ChatOptions)

// Apply invokes any supplied options. Used internally in Dive.
func (o *ChatOptions) Apply(opts []ChatOption) {
	for _, opt := range opts {
		opt(o)
	}
}

// WithThreadID associates the given conversation thread ID with a generation.
// This appends the new messages to any previous messages belonging to this thread.
func WithThreadID(threadID string) ChatOption {
	return func(opts *ChatOptions) {
		opts.ThreadID = threadID
	}
}

// WithUserID associates the given user ID with a generation, indicating what
// person is the speaker in the conversation.
func WithUserID(userID string) ChatOption {
	return func(opts *ChatOptions) {
		opts.UserID = userID
	}
}

// Task represents a unit of work that can be executed by an Agent.
type Task interface {
	// Name returns the name of the task
	Name() string

	// Timeout returns the maximum duration allowed for task execution
	Timeout() time.Duration

	// Prompt returns the LLM prompt for the task
	Prompt() (*Prompt, error)
}

// TaskResult holds the output of a completed task.
type TaskResult struct {
	// Task is the task that was executed
	Task Task

	// Content contains the raw output
	Content string

	// Format specifies how to interpret the content
	Format OutputFormat

	// Object holds parsed JSON output if applicable
	Object interface{}

	// Error is set if task execution failed
	Error error

	// Usage tracks LLM token usage
	Usage llm.Usage
}
