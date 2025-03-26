package environment

import (
	"context"
	"fmt"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/google/uuid"
)

// Environment is a container for running agents and workflow executions
type Environment struct {
	id              string
	name            string
	description     string
	agents          map[string]dive.Agent
	workflows       map[string]*workflow.Workflow
	triggers        []*Trigger
	executions      map[string]*Execution
	logger          slogger.Logger
	defaultWorkflow string
	documentRepo    dive.DocumentRepository
	threadRepo      dive.ThreadRepository
	actions         map[string]Action
	started         bool
}

// Options are used to configure an Environment.
type Options struct {
	ID                 string
	Name               string
	Description        string
	Agents             []dive.Agent
	Workflows          []*workflow.Workflow
	Triggers           []*Trigger
	Executions         []*Execution
	Logger             slogger.Logger
	DefaultWorkflow    string
	DocumentRepository dive.DocumentRepository
	ThreadRepository   dive.ThreadRepository
	Actions            []Action
}

// New returns a new Environment configured with the given options.
func New(opts Options) (*Environment, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("environment name is required")
	}
	if opts.Logger == nil {
		opts.Logger = slogger.DefaultLogger
	}

	agents := make(map[string]dive.Agent, len(opts.Agents))
	for _, agent := range opts.Agents {
		if _, exists := agents[agent.Name()]; exists {
			return nil, fmt.Errorf("agent already registered: %s", agent.Name())
		}
		agents[agent.Name()] = agent
	}

	workflows := make(map[string]*workflow.Workflow, len(opts.Workflows))
	for _, workflow := range opts.Workflows {
		if _, exists := workflows[workflow.Name()]; exists {
			return nil, fmt.Errorf("workflow already registered: %s", workflow.Name())
		}
		workflows[workflow.Name()] = workflow
	}

	executions := make(map[string]*Execution, len(opts.Executions))
	for _, execution := range opts.Executions {
		executions[execution.ID()] = execution
	}

	actions := make(map[string]Action, len(opts.Actions))

	// Register document actions if we have a document repository
	if opts.DocumentRepository != nil {
		writeAction := NewDocumentWriteAction(opts.DocumentRepository)
		readAction := NewDocumentReadAction(opts.DocumentRepository)
		actions[writeAction.Name()] = writeAction
		actions[readAction.Name()] = readAction
	}
	getTimeAction := NewGetTimeAction()
	actions[getTimeAction.Name()] = getTimeAction

	for _, action := range opts.Actions {
		if _, exists := actions[action.Name()]; exists {
			return nil, fmt.Errorf("action already registered: %s", action.Name())
		}
		actions[action.Name()] = action
	}

	if opts.DefaultWorkflow != "" {
		if _, exists := workflows[opts.DefaultWorkflow]; !exists {
			return nil, fmt.Errorf("default workflow not found: %s", opts.DefaultWorkflow)
		}
	}

	env := &Environment{
		id:              opts.ID,
		name:            opts.Name,
		description:     opts.Description,
		agents:          agents,
		workflows:       workflows,
		triggers:        opts.Triggers,
		executions:      executions,
		logger:          opts.Logger,
		defaultWorkflow: opts.DefaultWorkflow,
		documentRepo:    opts.DocumentRepository,
		threadRepo:      opts.ThreadRepository,
		actions:         actions,
	}
	for _, trigger := range env.triggers {
		trigger.SetEnvironment(env)
	}
	for _, agent := range env.Agents() {
		agent.SetEnvironment(env)
	}
	return env, nil
}

func (e *Environment) ID() string {
	return e.id
}

func (e *Environment) Name() string {
	return e.name
}

func (e *Environment) Description() string {
	return e.description
}

func (e *Environment) DocumentRepository() dive.DocumentRepository {
	return e.documentRepo
}

func (e *Environment) ThreadRepository() dive.ThreadRepository {
	return e.threadRepo
}

func (e *Environment) Start(ctx context.Context) error {
	if e.started {
		return fmt.Errorf("environment already started")
	}
	e.started = true
	for _, agent := range e.Agents() {
		if runnableAgent, ok := agent.(dive.RunnableAgent); ok {
			runnableAgent.Start(ctx)
		}
	}
	return nil
}

func (e *Environment) Stop(ctx context.Context) error {
	if !e.started {
		return fmt.Errorf("environment not started")
	}
	for _, agent := range e.Agents() {
		if runnableAgent, ok := agent.(dive.RunnableAgent); ok {
			runnableAgent.Stop(ctx)
		}
	}
	// TODO: stop executions?
	e.started = false
	return nil
}

func (e *Environment) IsRunning() bool {
	return e.started
}

func (e *Environment) Agents() []dive.Agent {
	agents := make([]dive.Agent, 0, len(e.agents))
	for _, agent := range e.agents {
		agents = append(agents, agent)
	}
	return agents
}

func (e *Environment) GetAgent(name string) (dive.Agent, error) {
	if agent, exists := e.agents[name]; exists {
		return agent, nil
	}
	return nil, fmt.Errorf("agent not found: %s", name)
}

func (e *Environment) RegisterAgent(agent dive.Agent) error {
	if _, exists := e.agents[agent.Name()]; exists {
		return fmt.Errorf("agent already present: %s", agent.Name())
	}
	e.agents[agent.Name()] = agent
	return nil
}

func (e *Environment) Workflows() []*workflow.Workflow {
	workflows := make([]*workflow.Workflow, 0, len(e.workflows))
	for _, workflow := range e.workflows {
		workflows = append(workflows, workflow)
	}
	return workflows
}

func (e *Environment) GetWorkflow(name string) (*workflow.Workflow, error) {
	if workflow, exists := e.workflows[name]; exists {
		return workflow, nil
	}
	return nil, fmt.Errorf("workflow not found: %s", name)
}

func (e *Environment) AddWorkflow(workflow *workflow.Workflow) error {
	if _, exists := e.workflows[workflow.Name()]; exists {
		return fmt.Errorf("workflow already present: %s", workflow.Name())
	}
	e.workflows[workflow.Name()] = workflow
	return nil
}

// ExecuteWorkflow starts a new workflow and immediately returns the execution,
// which will be running in the background.
func (e *Environment) ExecuteWorkflow(ctx context.Context, name string, inputs map[string]interface{}) (*Execution, error) {
	if !e.started {
		return nil, fmt.Errorf("environment not started")
	}
	if name == "" {
		if e.defaultWorkflow == "" {
			return nil, fmt.Errorf("a workflow name is required")
		}
		name = e.defaultWorkflow
	}

	workflow, exists := e.workflows[name]
	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", name)
	}

	// Build up the input variables with defaults and validation
	processedInputs := make(map[string]interface{})
	for _, input := range workflow.Inputs() {
		value, exists := inputs[input.Name]
		if !exists {
			// If input doesn't exist, check if it has a default value
			if input.Default != nil {
				processedInputs[input.Name] = input.Default
				continue
			}
			return nil, fmt.Errorf("required input %q not provided", input.Name)
		}
		// Input exists, use the provided value
		processedInputs[input.Name] = value
	}

	execution := NewExecution(ExecutionOptions{
		ID:          uuid.New().String(),
		Environment: e,
		Workflow:    workflow,
		Status:      StatusPending,
		StartTime:   time.Now(),
		Inputs:      processedInputs,
		Logger:      e.logger,
	})
	e.executions[execution.ID()] = execution

	go func() {
		if err := execution.Run(ctx); err != nil {
			e.logger.Error("workflow execution failed", "error", err)
			return
		}
	}()

	return execution, nil
}

// GetAction returns an action by name
func (e *Environment) GetAction(name string) (Action, bool) {
	action, ok := e.actions[name]
	return action, ok
}

// RegisterAction registers a new action
func (e *Environment) RegisterAction(action Action) error {
	if _, exists := e.actions[action.Name()]; exists {
		return fmt.Errorf("action already registered: %s", action.Name())
	}
	e.actions[action.Name()] = action
	return nil
}
