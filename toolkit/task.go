package toolkit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/google/uuid"
)

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

// TaskRecord stores information about a running or completed task
type TaskRecord struct {
	ID          string
	Description string
	Status      TaskStatus
	Output      string
	Error       error
	StartTime   time.Time
	EndTime     time.Time
	Agent       dive.Agent
	done        chan struct{}
}

// TaskRegistry manages running and completed tasks
type TaskRegistry struct {
	mu    sync.RWMutex
	tasks map[string]*TaskRecord
}

// NewTaskRegistry creates a new task registry
func NewTaskRegistry() *TaskRegistry {
	return &TaskRegistry{
		tasks: make(map[string]*TaskRecord),
	}
}

// Register adds a new task to the registry
func (r *TaskRegistry) Register(record *TaskRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[record.ID] = record
}

// Get retrieves a task by ID
func (r *TaskRegistry) Get(id string) (*TaskRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.tasks[id]
	return record, ok
}

// List returns all task IDs
func (r *TaskRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.tasks))
	for id := range r.tasks {
		ids = append(ids, id)
	}
	return ids
}

// AgentFactory creates agents for task execution
type AgentFactory func(ctx context.Context, subagentType string, model string) (dive.Agent, error)

// --- TaskTool ---

var _ dive.TypedTool[*TaskToolInput] = &TaskTool{}

// TaskToolInput is the input for the TaskTool
type TaskToolInput struct {
	Prompt          string `json:"prompt"`
	Description     string `json:"description"`
	SubagentType    string `json:"subagent_type"`
	Model           string `json:"model,omitempty"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
	Resume          string `json:"resume,omitempty"`
}

// TaskToolOptions configures a new TaskTool
type TaskToolOptions struct {
	// Registry is the shared task registry
	Registry *TaskRegistry

	// AgentFactory creates agents for task execution
	AgentFactory AgentFactory

	// DefaultTimeout is the default timeout for synchronous task execution
	DefaultTimeout time.Duration
}

// TaskTool launches specialized agents for complex, multi-step tasks
type TaskTool struct {
	registry       *TaskRegistry
	agentFactory   AgentFactory
	defaultTimeout time.Duration
}

// NewTaskTool creates a new TaskTool
func NewTaskTool(opts TaskToolOptions) *TaskTool {
	if opts.DefaultTimeout <= 0 {
		opts.DefaultTimeout = 10 * time.Minute
	}
	return &TaskTool{
		registry:       opts.Registry,
		agentFactory:   opts.AgentFactory,
		defaultTimeout: opts.DefaultTimeout,
	}
}

func (t *TaskTool) Name() string {
	return "Task"
}

func (t *TaskTool) Description() string {
	return `Launch a specialized agent to handle complex, multi-step tasks autonomously.

The Task tool launches agents that autonomously handle complex tasks. Each agent type has specific capabilities and tools available to it.

Usage notes:
- Always include a short description (3-5 words) summarizing what the agent will do
- Launch multiple agents concurrently whenever possible to maximize performance
- When the agent is done, it will return a single message back to you
- You can run agents in the background using run_in_background parameter
- Agents can be resumed using the resume parameter by passing the agent ID from a previous invocation
- Provide clear, detailed prompts so the agent can work autonomously`
}

func (t *TaskTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type: "object",
		Required: []string{
			"prompt",
			"description",
			"subagent_type",
		},
		Properties: map[string]*schema.Property{
			"prompt": {
				Type:        "string",
				Description: "The task for the agent to perform. Provide detailed instructions.",
			},
			"description": {
				Type:        "string",
				Description: "A short (3-5 word) description of the task.",
			},
			"subagent_type": {
				Type:        "string",
				Description: "The type of specialized agent to use (e.g., general-purpose, Explore, Plan).",
			},
			"model": {
				Type:        "string",
				Description: "Optional model to use: sonnet, opus, or haiku. If not specified, inherits from parent.",
				Enum:        []any{"sonnet", "opus", "haiku"},
			},
			"run_in_background": {
				Type:        "boolean",
				Description: "Set to true to run this agent in the background. Use TaskOutput to read the output later.",
			},
			"resume": {
				Type:        "string",
				Description: "Optional agent ID to resume from. If provided, the agent continues from the previous execution transcript.",
			},
		},
	}
}

func (t *TaskTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Task",
		ReadOnlyHint:    false,
		DestructiveHint: false,
		IdempotentHint:  false,
		OpenWorldHint:   true,
	}
}

func (t *TaskTool) Call(ctx context.Context, input *TaskToolInput) (*dive.ToolResult, error) {
	if input.Prompt == "" {
		return dive.NewToolResultError("prompt is required"), nil
	}
	if input.Description == "" {
		return dive.NewToolResultError("description is required"), nil
	}
	if input.SubagentType == "" {
		return dive.NewToolResultError("subagent_type is required"), nil
	}

	// Handle resume case
	if input.Resume != "" {
		record, ok := t.registry.Get(input.Resume)
		if !ok {
			return dive.NewToolResultError(fmt.Sprintf("task %s not found", input.Resume)), nil
		}
		if record.Agent == nil {
			return dive.NewToolResultError("task cannot be resumed: no agent context available"), nil
		}
		return t.executeTask(ctx, input, record.Agent, record.ID)
	}

	// Create new agent
	agent, err := t.agentFactory(ctx, input.SubagentType, input.Model)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("failed to create agent: %s", err.Error())), nil
	}

	taskID := fmt.Sprintf("task_%s", uuid.New().String()[:8])
	return t.executeTask(ctx, input, agent, taskID)
}

func (t *TaskTool) executeTask(ctx context.Context, input *TaskToolInput, agent dive.Agent, taskID string) (*dive.ToolResult, error) {
	record := &TaskRecord{
		ID:          taskID,
		Description: input.Description,
		Status:      TaskStatusRunning,
		StartTime:   time.Now(),
		Agent:       agent,
		done:        make(chan struct{}),
	}
	t.registry.Register(record)

	executeFunc := func() {
		defer close(record.done)

		message := &llm.Message{Role: llm.User}
		message.Content = append(message.Content, &llm.TextContent{Text: input.Prompt})

		response, err := agent.CreateResponse(ctx, dive.WithMessage(message))
		record.EndTime = time.Now()

		if err != nil {
			record.Status = TaskStatusFailed
			record.Error = err
			record.Output = fmt.Sprintf("Task failed: %s", err.Error())
		} else {
			record.Status = TaskStatusCompleted
			record.Output = response.OutputText()
		}
	}

	if input.RunInBackground {
		go executeFunc()
		return dive.NewToolResultText(fmt.Sprintf("Task started in background. Task ID: %s\nUse TaskOutput to retrieve results.", taskID)), nil
	}

	// Synchronous execution with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, t.defaultTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		executeFunc()
		close(done)
	}()

	select {
	case <-done:
		if record.Status == TaskStatusFailed {
			return dive.NewToolResultError(record.Output), nil
		}
		return dive.NewToolResultText(fmt.Sprintf("Agent ID: %s\n\n%s", taskID, record.Output)), nil
	case <-timeoutCtx.Done():
		record.Status = TaskStatusFailed
		record.Error = timeoutCtx.Err()
		return dive.NewToolResultError(fmt.Sprintf("Task timed out after %s. Task ID: %s", t.defaultTimeout, taskID)), nil
	}
}

func (t *TaskTool) ShouldReturnResult() bool {
	return true
}

// --- TaskOutputTool ---

var _ dive.TypedTool[*TaskOutputToolInput] = &TaskOutputTool{}

// TaskOutputToolInput is the input for the TaskOutputTool
type TaskOutputToolInput struct {
	TaskID  string `json:"task_id"`
	Block   *bool  `json:"block,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// TaskOutputToolOptions configures a new TaskOutputTool
type TaskOutputToolOptions struct {
	// Registry is the shared task registry
	Registry *TaskRegistry
}

// TaskOutputTool retrieves output from running or completed tasks
type TaskOutputTool struct {
	registry *TaskRegistry
}

// NewTaskOutputTool creates a new TaskOutputTool
func NewTaskOutputTool(opts TaskOutputToolOptions) *TaskOutputTool {
	return &TaskOutputTool{
		registry: opts.Registry,
	}
}

func (t *TaskOutputTool) Name() string {
	return "TaskOutput"
}

func (t *TaskOutputTool) Description() string {
	return `Retrieves output from a running or completed task (background shell, agent, or remote session).

- Takes a task_id parameter identifying the task
- Returns the task output along with status information
- Use block=true (default) to wait for task completion
- Use block=false for non-blocking check of current status
- Task IDs can be found using the /tasks command
- Works with all task types: background shells, async agents, and remote sessions`
}

func (t *TaskOutputTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"task_id"},
		Properties: map[string]*schema.Property{
			"task_id": {
				Type:        "string",
				Description: "The task ID to get output from.",
			},
			"block": {
				Type:        "boolean",
				Description: "Whether to wait for completion. Defaults to true.",
			},
			"timeout": {
				Type:        "number",
				Description: "Max wait time in milliseconds. Defaults to 30000, max 600000.",
			},
		},
	}
}

func (t *TaskOutputTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Task Output",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

func (t *TaskOutputTool) Call(ctx context.Context, input *TaskOutputToolInput) (*dive.ToolResult, error) {
	if input.TaskID == "" {
		return dive.NewToolResultError("task_id is required"), nil
	}

	record, ok := t.registry.Get(input.TaskID)
	if !ok {
		return dive.NewToolResultError(fmt.Sprintf("task %s not found", input.TaskID)), nil
	}

	// Default to blocking
	block := true
	if input.Block != nil {
		block = *input.Block
	}

	// Default timeout 30 seconds, max 10 minutes
	timeout := 30 * time.Second
	if input.Timeout > 0 {
		timeout = time.Duration(input.Timeout) * time.Millisecond
		if timeout > 10*time.Minute {
			timeout = 10 * time.Minute
		}
	}

	if !block {
		return t.formatTaskStatus(record), nil
	}

	// Wait for completion with timeout
	select {
	case <-record.done:
		return t.formatTaskStatus(record), nil
	case <-time.After(timeout):
		return t.formatTaskStatus(record), nil
	case <-ctx.Done():
		return dive.NewToolResultError("context cancelled while waiting for task"), nil
	}
}

func (t *TaskOutputTool) formatTaskStatus(record *TaskRecord) *dive.ToolResult {
	status := fmt.Sprintf("Task ID: %s\nDescription: %s\nStatus: %s\nStarted: %s\n",
		record.ID,
		record.Description,
		record.Status,
		record.StartTime.Format(time.RFC3339),
	)

	if record.Status == TaskStatusCompleted || record.Status == TaskStatusFailed {
		status += fmt.Sprintf("Ended: %s\nDuration: %s\n",
			record.EndTime.Format(time.RFC3339),
			record.EndTime.Sub(record.StartTime).Round(time.Millisecond),
		)
	}

	if record.Output != "" {
		status += fmt.Sprintf("\nOutput:\n%s", record.Output)
	}

	if record.Error != nil {
		status += fmt.Sprintf("\nError: %s", record.Error.Error())
	}

	return dive.NewToolResultText(status)
}

func (t *TaskOutputTool) ShouldReturnResult() bool {
	return true
}
