package dive

import (
	"fmt"
	"strings"
	"time"
)

// Task represents a unit of work to be performed by an agent
type Task struct {
	name                   string
	nameTemplate           string
	description            string
	descriptionTemplate    string
	expectedOutput         string
	expectedOutputTemplate string
	outputFormat           OutputFormat
	outputObject           interface{}
	assignedAgent          Agent
	dependencies           []string
	condition              string
	maxIterations          *int
	outputFile             string
	result                 *TaskResult
	timeout                time.Duration
	context                string
	kind                   string
}

func (t *Task) Name() string               { return t.name }
func (t *Task) Description() string        { return t.description }
func (t *Task) ExpectedOutput() string     { return t.expectedOutput }
func (t *Task) OutputFormat() OutputFormat { return t.outputFormat }
func (t *Task) OutputObject() interface{}  { return t.outputObject }
func (t *Task) AssignedAgent() Agent       { return t.assignedAgent }
func (t *Task) Dependencies() []string     { return t.dependencies }
func (t *Task) Condition() string          { return t.condition }
func (t *Task) MaxIterations() *int        { return t.maxIterations }
func (t *Task) OutputFile() string         { return t.outputFile }
func (t *Task) Result() *TaskResult        { return t.result }
func (t *Task) Timeout() time.Duration     { return t.timeout }
func (t *Task) Context() string            { return t.context }
func (t *Task) Kind() string               { return t.kind }

// TaskOptions is used to define a Task
type TaskOptions struct {
	Description    string        `json:"description"`
	Name           string        `json:"name,omitempty"`
	ExpectedOutput string        `json:"expected_output,omitempty"`
	Dependencies   []string      `json:"dependencies,omitempty"`
	Condition      string        `json:"condition,omitempty"`
	MaxIterations  *int          `json:"max_iterations,omitempty"`
	OutputFile     string        `json:"output_file,omitempty"`
	Timeout        time.Duration `json:"timeout,omitempty"`
	Context        string        `json:"context,omitempty"`
	Priority       int           `json:"priority,omitempty"`
	Kind           string        `json:"kind,omitempty"`
	OutputFormat   OutputFormat  `json:"output_format,omitempty"`
	OutputObject   interface{}   `json:"-"`
	AssignedAgent  Agent         `json:"-"`
}

// NewTask creates a new Task from a TaskOptions
func NewTask(opts TaskOptions) *Task {
	if opts.Name == "" {
		opts.Name = fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	return &Task{
		name:                   opts.Name,
		nameTemplate:           opts.Name,
		description:            opts.Description,
		descriptionTemplate:    opts.Description,
		expectedOutput:         opts.ExpectedOutput,
		expectedOutputTemplate: opts.ExpectedOutput,
		outputFormat:           opts.OutputFormat,
		outputObject:           opts.OutputObject,
		assignedAgent:          opts.AssignedAgent,
		dependencies:           opts.Dependencies,
		condition:              opts.Condition,
		maxIterations:          opts.MaxIterations,
		outputFile:             opts.OutputFile,
		timeout:                opts.Timeout,
		context:                opts.Context,
		kind:                   opts.Kind,
	}
}

// Validate checks if the task is properly configured
func (t *Task) Validate() error {
	if t.name == "" {
		return fmt.Errorf("task name required")
	}
	if t.description == "" {
		return fmt.Errorf("description required for task %q", t.name)
	}
	if t.outputObject != nil && t.outputFormat != OutputJSON {
		return fmt.Errorf("expected json output format for task %q", t.name)
	}
	for _, depID := range t.dependencies {
		if depID == t.name {
			return fmt.Errorf("task %q cannot depend on itself", t.name)
		}
	}
	return nil
}

// PromptText returns the LLM prompt text for the task
func (t *Task) PromptText() string {
	var intro string
	if t.name != "" {
		intro = fmt.Sprintf("Let's work on a new task named %q:", t.name)
	} else {
		intro = "Let's work on a new task:"
	}
	lines := []string{}
	if t.description != "" {
		lines = append(lines, t.description)
	}
	if t.expectedOutput != "" {
		lines = append(lines, fmt.Sprintf("Please respond with %s.", t.expectedOutput))
	}
	if t.outputFormat != "" {
		lines = append(lines, fmt.Sprintf("Your response must be in %s format.", t.outputFormat))
	}
	if t.context != "" {
		lines = append(lines, fmt.Sprintf("Use this context while working on the task:\n\n%s\n\n", t.context))
	}
	result := fmt.Sprintf("%s\n\n<task>\n%s\n</task>", intro, strings.Join(lines, "\n\n"))
	result += "\n\nPlease begin working on the task."
	return result
}
