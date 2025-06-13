package environment

import (
	"context"
	"fmt"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/mcp"
	"github.com/diveagents/dive/objects"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/risor-io/risor/modules/all"
)

// Environment is a container for running agents and workflow executions
type Environment struct {
	id              string
	name            string
	description     string
	agents          map[string]dive.Agent
	workflows       map[string]*workflow.Workflow
	triggers        []*Trigger
	executions      map[string]*EventBasedExecution
	logger          slogger.Logger
	defaultWorkflow string
	documentRepo    dive.DocumentRepository
	threadRepo      dive.ThreadRepository
	actions         map[string]Action
	started         bool
	confirmer       dive.Confirmer
	mcpManager      *mcp.Manager
	mcpServers      []*mcp.ServerConfig
}

// Options are used to configure an Environment.
type Options struct {
	ID                 string
	Name               string
	Description        string
	Agents             []dive.Agent
	Workflows          []*workflow.Workflow
	Triggers           []*Trigger
	Executions         []*EventBasedExecution
	Logger             slogger.Logger
	DefaultWorkflow    string
	DocumentRepository dive.DocumentRepository
	ThreadRepository   dive.ThreadRepository
	Actions            []Action
	AutoStart          bool
	Confirmer          dive.Confirmer
	MCPServers         []*mcp.ServerConfig
	MCPManager         *mcp.Manager
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

	executions := make(map[string]*EventBasedExecution, len(opts.Executions))
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
	for _, action := range actionsRegistry {
		actions[action.Name()] = action
	}
	for _, action := range opts.Actions {
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
		mcpManager:      opts.MCPManager,
		mcpServers:      opts.MCPServers,
	}
	for _, trigger := range env.triggers {
		trigger.SetEnvironment(env)
	}
	for _, agent := range env.Agents() {
		agent.SetEnvironment(env)
	}

	if opts.AutoStart {
		if err := env.Start(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to start environment: %w", err)
		}
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

func (e *Environment) Confirmer() dive.Confirmer {
	return e.confirmer
}

func (e *Environment) Start(ctx context.Context) error {
	if e.started {
		return fmt.Errorf("environment already started")
	}

	if e.mcpManager != nil {
		if err := e.mcpManager.InitializeServers(ctx, e.mcpServers); err != nil {
			e.logger.Error("failed to initialize MCP servers", "error", err)
			return err
		}
	}

	e.started = true
	return nil
}

func (e *Environment) Stop(ctx context.Context) error {
	if !e.started {
		return fmt.Errorf("environment not started")
	}

	if e.mcpManager != nil {
		if err := e.mcpManager.Close(); err != nil {
			e.logger.Error("failed to close MCP connections", "error", err)
			return err
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

func (e *Environment) AddAgent(agent dive.Agent) error {
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
func (e *Environment) ExecuteWorkflow(ctx context.Context, opts ExecutionOptions) (*EventBasedExecution, error) {
	if !e.started {
		return nil, fmt.Errorf("environment not started")
	}
	if opts.WorkflowName == "" {
		if e.defaultWorkflow == "" {
			return nil, fmt.Errorf("a workflow name is required")
		}
		opts.WorkflowName = e.defaultWorkflow
	}

	workflow, exists := e.workflows[opts.WorkflowName]
	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", opts.WorkflowName)
	}

	inputs := opts.Inputs
	if inputs == nil {
		inputs = make(map[string]interface{})
	}

	logger := opts.Logger
	if logger == nil {
		logger = e.logger
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

	execution := &EventBasedExecution{
		id:            dive.NewID(),
		environment:   e,
		workflow:      workflow,
		status:        StatusPending,
		startTime:     time.Now(),
		inputs:        processedInputs,
		logger:        logger,
		paths:         make(map[string]*PathState),
		formatter:     opts.Formatter,
		scriptGlobals: map[string]any{"inputs": processedInputs},
	}
	if e.documentRepo != nil {
		execution.scriptGlobals["documents"] = objects.NewDocumentRepository(e.documentRepo)
	}
	e.executions[execution.ID()] = execution

	// Make Risor's default builtins available to embedded scripts
	for k, v := range all.Builtins() {
		execution.scriptGlobals[k] = v
	}

	if err := execution.Run(ctx); err != nil {
		logger.Error("failed to start workflow", "error", err)
		return nil, err
	}
	return execution, nil
}

// GetAction returns an action by name
func (e *Environment) GetAction(name string) (Action, bool) {
	action, ok := e.actions[name]
	return action, ok
}

// GetMCPTools returns all MCP tools from all connected servers
func (e *Environment) GetMCPTools() map[string]dive.Tool {
	if e.mcpManager == nil {
		return make(map[string]dive.Tool)
	}
	return e.mcpManager.GetAllTools()
}

// GetMCPToolsByServer returns MCP tools from a specific server
func (e *Environment) GetMCPToolsByServer(serverName string) []dive.Tool {
	if e.mcpManager == nil {
		return nil
	}
	return e.mcpManager.GetToolsByServer(serverName)
}

// GetMCPTool returns a specific MCP tool by name (with server prefix)
func (e *Environment) GetMCPTool(toolKey string) dive.Tool {
	if e.mcpManager == nil {
		return nil
	}
	return e.mcpManager.GetTool(toolKey)
}

// GetMCPServerStatus returns the connection status of all MCP servers
func (e *Environment) GetMCPServerStatus() map[string]bool {
	if e.mcpManager == nil {
		return make(map[string]bool)
	}
	return e.mcpManager.GetServerStatus()
}
