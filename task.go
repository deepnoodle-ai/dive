package agent

import (
	"fmt"
	"time"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

// TaskResult holds the output of a completed task
type TaskResult struct {
	Task   *Task
	Output TaskOutput
	Error  error
}

// TaskState holds the state of a task
type TaskState struct {
	IterationCount int
	LastError      error
	StartTime      time.Time
	CompletionTime time.Time
	Status         TaskStatus
}

// OutputFormat defines the structure of task outputs
type OutputFormat string

const (
	OutputText     OutputFormat = "text"
	OutputMarkdown OutputFormat = "markdown"
	OutputJSON     OutputFormat = "json"
)

// TaskOutput represents the result of a task execution
type TaskOutput struct {
	// Raw content of the output
	Content string

	// Format specifies how to interpret the content
	Format OutputFormat

	// For JSON outputs, Object is the parsed JSON object
	Object interface{}

	// Reasoning is the thought process used to arrive at the answer
	Reasoning string
}

// Task represents a discrete unit of work to be performed by an agent
type Task struct {
	name                   string
	nameTemplate           string
	description            string
	descriptionTemplate    string
	expectedOutput         string
	expectedOutputTemplate string
	outputFormat           OutputFormat
	outputObject           interface{}
	agent                  Agent
	dependencies           []string
	condition              string
	maxIterations          *int
	outputFile             string
	result                 *TaskResult
	state                  *TaskState
	timeout                time.Duration
	context                string
}

// Getters
func (t *Task) Name() string               { return t.name }
func (t *Task) Description() string        { return t.description }
func (t *Task) ExpectedOutput() string     { return t.expectedOutput }
func (t *Task) OutputFormat() OutputFormat { return t.outputFormat }
func (t *Task) OutputObject() interface{}  { return t.outputObject }
func (t *Task) Agent() Agent               { return t.agent }
func (t *Task) Dependencies() []string     { return t.dependencies }
func (t *Task) Condition() string          { return t.condition }
func (t *Task) MaxIterations() *int        { return t.maxIterations }
func (t *Task) OutputFile() string         { return t.outputFile }
func (t *Task) Result() *TaskResult        { return t.result }
func (t *Task) Timeout() time.Duration     { return t.timeout }
func (t *Task) Context() string            { return t.context }

// TaskSpec defines the configuration for creating a new Task
type TaskSpec struct {
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	ExpectedOutput string        `json:"expected_output"`
	OutputFormat   OutputFormat  `json:"output_format"`
	OutputObject   interface{}   `json:"output_object"`
	Agent          Agent         `json:"-"`
	Dependencies   []string      `json:"dependencies"`
	Condition      string        `json:"condition"`
	MaxIterations  *int          `json:"max_iterations"`
	OutputFile     string        `json:"output_file,omitempty"`
	Timeout        time.Duration `json:"timeout,omitempty"`
	Context        string        `json:"context,omitempty"`
}

// NewTask creates a new Task from a TaskSpec
func NewTask(spec TaskSpec) *Task {
	return &Task{
		name:                   spec.Name,
		nameTemplate:           spec.Name,
		description:            spec.Description,
		descriptionTemplate:    spec.Description,
		expectedOutput:         spec.ExpectedOutput,
		expectedOutputTemplate: spec.ExpectedOutput,
		outputFormat:           spec.OutputFormat,
		outputObject:           spec.OutputObject,
		agent:                  spec.Agent,
		dependencies:           spec.Dependencies,
		condition:              spec.Condition,
		maxIterations:          spec.MaxIterations,
		outputFile:             spec.OutputFile,
		timeout:                spec.Timeout,
		context:                spec.Context,
	}
}

func (t *Task) InterpolateInputs(input any) error {
	var err error
	if t.name, err = interpolateTemplate(
		"name", t.nameTemplate, input,
	); err != nil {
		return err
	}
	if t.description, err = interpolateTemplate(
		"description", t.descriptionTemplate, input,
	); err != nil {
		return err
	}
	if t.expectedOutput, err = interpolateTemplate(
		"expected_output", t.expectedOutputTemplate, input,
	); err != nil {
		return err
	}
	return nil
}

// Validate checks if the task is properly configured
func (t *Task) Validate() error {
	if t.name == "" {
		return fmt.Errorf("task name required")
	}
	if t.description == "" {
		return fmt.Errorf("task description required")
	}
	if t.outputObject != nil && t.outputFormat != OutputJSON {
		return fmt.Errorf("output object provided but output format is not json")
	}
	// Validate dependencies exist
	for _, depID := range t.dependencies {
		if depID == t.name {
			return fmt.Errorf("task cannot depend on itself")
		}
	}
	return nil
}

// SetResult updates the task's result
func (t *Task) SetResult(result *TaskResult) {
	t.result = result
}
