package dive

import (
	"fmt"
	"strings"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
)

// TaskOptions is used to define a Task
type TaskOptions struct {
	Name           string
	Description    string
	ExpectedOutput string
	OutputFormat   OutputFormat
	OutputFile     string
	Dependencies   []string
	Timeout        time.Duration
	Context        string
	OutputObject   interface{}
	AssignedAgent  Agent
}

// Task represents a unit of work to be performed by an agent
type Task struct {
	name           string
	description    string
	expectedOutput string
	outputFormat   OutputFormat
	outputObject   interface{}
	assignedAgent  Agent
	dependencies   []string
	maxIterations  *int
	outputFile     string
	result         *TaskResult
	timeout        time.Duration
	context        string
	depOutput      string
	kind           string
	nameIsRandom   bool
}

// NewTask creates a new Task from a TaskOptions
func NewTask(opts TaskOptions) *Task {
	var nameIsRandom bool
	if opts.Name == "" {
		opts.Name = fmt.Sprintf("task-%s", petname.Generate(2, "-"))
		nameIsRandom = true
	}
	return &Task{
		name:           opts.Name,
		description:    opts.Description,
		expectedOutput: opts.ExpectedOutput,
		outputFormat:   opts.OutputFormat,
		outputObject:   opts.OutputObject,
		assignedAgent:  opts.AssignedAgent,
		dependencies:   opts.Dependencies,
		outputFile:     opts.OutputFile,
		timeout:        opts.Timeout,
		context:        opts.Context,
		nameIsRandom:   nameIsRandom,
	}
}

func (t *Task) Name() string               { return t.name }
func (t *Task) Description() string        { return t.description }
func (t *Task) ExpectedOutput() string     { return t.expectedOutput }
func (t *Task) OutputFormat() OutputFormat { return t.outputFormat }
func (t *Task) OutputObject() interface{}  { return t.outputObject }
func (t *Task) AssignedAgent() Agent       { return t.assignedAgent }
func (t *Task) Dependencies() []string     { return t.dependencies }
func (t *Task) OutputFile() string         { return t.outputFile }
func (t *Task) Result() *TaskResult        { return t.result }
func (t *Task) Timeout() time.Duration     { return t.timeout }
func (t *Task) Context() string            { return t.context }
func (t *Task) DependenciesOutput() string { return t.depOutput }

func (t *Task) SetContext(ctx string)               { t.context = ctx }
func (t *Task) SetDependenciesOutput(output string) { t.depOutput = output }
func (t *Task) SetResult(result *TaskResult)        { t.result = result }
func (t *Task) SetAssignedAgent(agent Agent)        { t.assignedAgent = agent }

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

// Prompt returns the LLM prompt for the task
func (t *Task) Prompt() string {
	var intro string
	if t.name != "" && !t.nameIsRandom {
		intro = fmt.Sprintf("Let's work on a new task named %q.", t.name)
	} else {
		intro = "Let's work on a new task."
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
	result := fmt.Sprintf("%s\n\n```TASK\n%s\n```", intro, strings.Join(lines, "\n\n"))

	if t.context != "" {
		result += fmt.Sprintf("\n\nUse this context while working on the task:\n\n```CONTEXT\n%s\n```", t.context)
	}
	if t.depOutput != "" {
		result += fmt.Sprintf("\n\nHere is the output from this task's dependencies:\n\n```DEPENDENCIES\n%s\n```", t.depOutput)
	}
	result += "\n\nPlease begin working on the task."
	return result
}
