package agents

import (
	"context"
	"fmt"
	"os"
)

// Add a dedicated state type to make team state more type-safe
type TeamState struct {
	// Global state shared across all tasks
	Global map[string]interface{}

	// Per-task state to track execution
	TaskStates map[string]*TaskState

	// Cached results to avoid recomputation
	ResultCache map[string]TaskResult
}

// Team orchestrates multiple agents working together
type Team struct {
	// Agents participating in this team
	agents []Agent

	// Tasks to be performed
	tasks map[string]*Task

	// State maintains shared context
	state map[string]interface{}

	// Helps ensure a team is used only once
	started bool

	// Description is a short description of the team
	description string

	// ConversationLogger logs the conversation between the agent and the user
	conversationLogger ConversationLogger
}

type TeamSpec struct {
	Agents       []Agent
	Tasks        []*Task
	Description  string
	LogDirectory string
}

// NewTeam creates a new team with the given agents and tasks
func NewTeam(spec TeamSpec) (*Team, error) {
	t := &Team{
		agents:      spec.Agents,
		tasks:       make(map[string]*Task, len(spec.Tasks)),
		state:       make(map[string]interface{}),
		description: spec.Description,
	}
	for _, task := range spec.Tasks {
		if err := t.addTask(task); err != nil {
			return nil, err
		}
	}
	if spec.LogDirectory != "" {
		if err := os.MkdirAll(spec.LogDirectory, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
		t.conversationLogger = NewFileConversationLogger(spec.LogDirectory)
	}
	return t, nil
}

func (t *Team) Description() string {
	return t.description
}

func (t *Team) Agents() []Agent {
	return t.agents
}

func (t *Team) ConversationLogger() ConversationLogger {
	return t.conversationLogger
}

func (t *Team) Tasks() []*Task {
	var tasks []*Task
	for _, task := range t.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

func (t *Team) GetAgent(name string) (Agent, bool) {
	for _, agent := range t.agents {
		if agent.Name() == name {
			return agent, true
		}
	}
	return nil, false
}

func (t *Team) Overview() (string, error) {
	return ExecuteTemplate(teamPromptTemplate, t)
}

// AddTask adds a task to the team's workflow
func (t *Team) addTask(task *Task) error {
	name := task.Name()
	if name == "" {
		return fmt.Errorf("task name is required")
	}
	if t.tasks[name] != nil {
		return fmt.Errorf("task %q already exists", name)
	}
	t.tasks[name] = task
	return nil
}

// Execute runs all tasks in the appropriate order
func (t *Team) Execute(ctx context.Context, input ...any) (map[string]TaskResult, error) {
	if len(input) > 1 {
		return nil, fmt.Errorf("only one input is supported")
	}
	var theInput any
	if len(input) == 1 {
		theInput = input[0]
	}

	if t.started {
		return nil, fmt.Errorf("team already started")
	}
	t.started = true

	for _, agent := range t.agents {
		if err := agent.Join(ctx, t); err != nil {
			return nil, fmt.Errorf("failed to join team: %w", err)
		}
	}
	// for _, agent := range t.agents {
	// 	if err := agent.InterpolateInputs(theInput); err != nil {
	// 		return nil, fmt.Errorf("failed to interpolate inputs: %w", err)
	// 	}
	// }
	for _, task := range t.tasks {
		if err := task.InterpolateInputs(theInput); err != nil {
			return nil, fmt.Errorf("failed to interpolate inputs: %w", err)
		}
	}

	// Get execution order
	order, err := newTaskGraph(t.tasks).TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("invalid task dependencies: %w", err)
	}

	// Execute tasks in order
	results := make(map[string]TaskResult, len(t.tasks))
	for _, taskID := range order {
		task := t.tasks[taskID]

		// Get dependency results
		var depResults []TaskResult
		for _, depID := range task.Dependencies() {
			depResults = append(depResults, results[depID])
		}

		// Execute the task
		result := t.executeTask(ctx, task, depResults)
		results[taskID] = result
		if result.Error != nil {
			return results, result.Error
		}
	}
	return results, nil
}

func (t *Team) executeTask(ctx context.Context, task *Task, deps []TaskResult) TaskResult {
	if task.Timeout() > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, task.Timeout())
		defer cancel()
	}
	return t.executeSingleAttempt(ctx, task, deps)
}

func (t *Team) executeSingleAttempt(ctx context.Context, task *Task, deps []TaskResult) TaskResult {

	// Execute the task using the agent
	promise, err := task.Agent().Work(ctx, task)
	if err != nil {
		return TaskResult{
			Task:  task,
			Error: err,
		}
	}
	result, err := promise.Get(ctx)
	if err != nil {
		return TaskResult{
			Task:  task,
			Error: err,
		}
	}

	// Handle file output if specified
	if task.OutputFile() != "" && result.Error == nil {
		// TODO: Implement file saving logic
		// if err := saveToFile(task.OutputFile, result.Output.Content); err != nil {
		// 	result.Error = err
		// }
	}
	return *result
}

func (t *Team) Validate() error {
	for _, task := range t.tasks {
		if err := task.Validate(); err != nil {
			return fmt.Errorf("invalid task %s: %w", task.Name(), err)
		}
		// Validate dependencies exist
		for _, depID := range task.Dependencies() {
			if _, exists := t.tasks[depID]; !exists {
				return fmt.Errorf("task %s depends on non-existent task %s", task.Name(), depID)
			}
		}
	}
	return nil
}
