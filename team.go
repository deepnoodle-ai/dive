package dive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/getstingrai/dive/graph"
	"github.com/getstingrai/dive/slogger"
)

var _ Team = &DiveTeam{}

// DiveTeam is the primary implementation of the Team interface. A Team consists
// of one or more Agents that work together to complete tasks.
type DiveTeam struct {
	name         string
	description  string
	agents       []Agent
	supervisors  []Agent
	running      bool
	initialTasks []*Task
	taskGraph    *graph.Graph
	taskOrder    []string
	outputDir    string
	logLevel     string
	logger       slogger.Logger
	mutex        sync.Mutex
}

// TeamOptions are used to configure a new team.
type TeamOptions struct {
	Name        string
	Description string
	Agents      []Agent
	Tasks       []*Task
	LogLevel    string
	Logger      slogger.Logger
	OutputDir   string
}

// NewTeam creates a new team composed of the given agents.
func NewTeam(opts TeamOptions) (*DiveTeam, error) {
	if opts.Logger == nil {
		opts.Logger = DefaultLogger
	}
	t := &DiveTeam{
		name:         opts.Name,
		description:  opts.Description,
		agents:       opts.Agents,
		initialTasks: opts.Tasks,
		logLevel:     opts.LogLevel,
		logger:       opts.Logger,
		outputDir:    opts.OutputDir,
	}
	for _, task := range opts.Tasks {
		if err := task.Validate(); err != nil {
			return nil, err
		}
	}
	if len(t.agents) == 0 {
		return nil, fmt.Errorf("at least one agent is required")
	}
	for _, agent := range t.agents {
		if name := agent.Name(); name == "" {
			return nil, fmt.Errorf("agent has no name")
		}
		if teamAgent, ok := agent.(TeamAgent); ok {
			if err := teamAgent.Join(t); err != nil {
				return nil, err
			}
			if teamAgent.IsSupervisor() {
				t.supervisors = append(t.supervisors, agent)
			}
		}
	}
	if len(t.agents) > 1 && len(t.supervisors) == 0 {
		return nil, fmt.Errorf("at least one supervisor is required")
	}
	if t.outputDir != "" {
		if err := os.MkdirAll(t.outputDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
	}
	return t, nil
}

// Description of the team.
func (t *DiveTeam) Description() string {
	return t.description
}

// Agents returns a copy of the agents in the team.
func (t *DiveTeam) Agents() []Agent {
	// Make a copy to help ensure immutability on the set
	agents := make([]Agent, len(t.agents))
	copy(agents, t.agents)
	return agents
}

// Name returns the name of the team.
func (t *DiveTeam) Name() string {
	return t.name
}

// OutputDir returns the output directory for the team.
func (t *DiveTeam) OutputDir() string {
	return t.outputDir
}

// IsRunning returns true if the team is active.
func (t *DiveTeam) IsRunning() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	return t.running
}

// Start all agents belonging to the team.
func (t *DiveTeam) Start(ctx context.Context) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if err := t.start(ctx); err != nil {
		t.logger.Error("failed to start team", "error", err)
		return err
	}
	return nil
}

// Start all agents on the team. Call only when the team mutex is held.
func (t *DiveTeam) start(ctx context.Context) error {
	if len(t.agents) == 0 {
		return fmt.Errorf("no agents to start")
	}
	if t.running {
		return fmt.Errorf("team already running")
	}

	// Start all agents, but if any fail, stop them all before returning the error
	var startedAgents []RunnableAgent
	for _, agent := range t.agents {
		runnableAgent, ok := agent.(RunnableAgent)
		if !ok {
			continue
		}
		if err := runnableAgent.Start(ctx); err != nil {
			for _, startedAgent := range startedAgents {
				startedAgent.Stop(ctx)
			}
			return fmt.Errorf("failed to start agent %q: %w", agent.Name(), err)
		}
		startedAgents = append(startedAgents, runnableAgent)
	}

	t.logger.Debug("team started",
		"team_name", t.name,
		"team_description", t.description,
		"agent_count", len(t.agents),
		"agent_names", AgentNames(t.agents),
	)
	t.running = true
	return nil
}

// Stop all agents belonging to the team.
func (t *DiveTeam) Stop(ctx context.Context) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.running {
		return fmt.Errorf("team not running")
	}
	t.running = false

	var lastErr error
	for _, agent := range t.agents {
		runnableAgent, ok := agent.(RunnableAgent)
		if !ok {
			continue
		}
		if err := runnableAgent.Stop(ctx); err != nil {
			lastErr = fmt.Errorf("failed to stop agent %s: %w", agent.Name(), err)
		}
	}

	t.logger.Debug("team stopped", "team_name", t.name)
	return lastErr
}

// Work on one or more tasks. The returned stream will deliver events and
// results to the caller as progress is made. This batch of work is considered
// independent of any other work the team may be doing. If the team has not yet
// started, it is automatically started.
func (t *DiveTeam) Work(ctx context.Context, tasks ...*Task) (Stream, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	var todo []*Task

	// Automatically start as needed
	if !t.running {
		if err := t.start(ctx); err != nil {
			return nil, err
		}
	}
	// Any initial tasks are enqueued first
	if len(t.initialTasks) > 0 {
		todo = append(todo, t.initialTasks...)
		t.initialTasks = nil
	}
	// Add any tasks provided by the caller
	if len(tasks) > 0 {
		todo = append(todo, tasks...)
	}
	// Make sure we have something to do
	if len(todo) == 0 {
		return nil, fmt.Errorf("no tasks to work on")
	}

	// Validate and index tasks by name
	tasksByName := make(map[string]*Task, len(todo))
	for _, task := range todo {
		if err := task.Validate(); err != nil {
			return nil, err
		}
		name := task.Name()
		if tasksByName[name] != nil {
			return nil, fmt.Errorf("duplicate task name: %q", name)
		}
		tasksByName[name] = task
	}

	// Sort tasks into execution order
	orderedNames, err := OrderTasks(todo)
	if err != nil {
		return nil, fmt.Errorf("failed to determine task execution order: %w", err)
	}
	var orderedTasks []*Task
	for _, taskName := range orderedNames {
		orderedTasks = append(orderedTasks, tasksByName[taskName])
	}

	// This stream will be used to deliver events and results to the caller
	stream := NewDiveStream()

	// Run work and process events in a separate goroutine
	go t.workOnTasks(ctx, orderedTasks, stream)

	return stream, nil
}

func (t *DiveTeam) workOnTasks(ctx context.Context, tasks []*Task, stream *DiveStream) {
	publisher := NewStreamPublisher(stream)
	defer publisher.Close()

	t.logger.Debug("team work started",
		"team_name", t.name,
		"task_count", len(tasks),
		"task_names", TaskNames(tasks),
	)

	resultsByTaskName := make(map[string]*TaskResult, len(tasks))

	// Work on tasks sequentially
	for _, task := range tasks {

		// Determine which agent should take the task
		var agent Agent
		if task.AssignedAgent() != nil {
			agent = task.AssignedAgent()
		} else if len(t.supervisors) > 0 {
			agent = t.supervisors[0]
		} else {
			agent = t.agents[0]
		}

		// Capture the output of any dependencies and store on the task
		if dependencies := task.Dependencies(); len(dependencies) > 0 {
			var outputs []string
			for _, dep := range dependencies {
				depResult, ok := resultsByTaskName[dep]
				if !ok {
					// This should never happen since the tasks were sorted into
					// execution order! If it does, it indicates a severe bug so
					// a panic is appropriate.
					panic(fmt.Sprintf("task execution failure: task %q dependency %q", task.Name(), dep))
				}
				outputs = append(outputs,
					fmt.Sprintf("<output task=%q>\n%s\n</output>", dep, depResult.Content))
			}
			task.SetDependenciesOutput(strings.Join(outputs, "\n\n"))
		}

		// Work the task to completion
		result, err := t.workOnTask(ctx, task, agent, publisher)
		if err != nil {
			publisher.Send(context.Background(), &StreamEvent{
				Type:      "work.error",
				TaskName:  task.Name(),
				AgentName: agent.Name(),
				Error: fmt.Sprintf("work failed on task %q agent %q: %v",
					task.Name(), agent.Name(), err),
			})
			return
		}
		resultsByTaskName[task.Name()] = result
	}

	// Send a final event indicating all work is done. Use a clean context since
	// the provided context may have been canceled.
	publisher.Send(context.Background(), &StreamEvent{Type: "work.done"})
}

func (t *DiveTeam) workOnTask(ctx context.Context, task *Task, agent Agent, pub *StreamPublisher) (*TaskResult, error) {
	workerAgent, ok := agent.(TeamAgent)
	if !ok {
		return nil, fmt.Errorf("agent %q does not accept tasks", agent.Name())
	}

	taskStream, err := workerAgent.Work(ctx, task)
	if err != nil {
		return nil, err
	}
	defer taskStream.Close()

	logger := t.logger.With(
		"task_name", task.Name(),
		"agent_name", agent.Name(),
	)
	logger.Info("assigned task")

	// Heartbeats will indicate to the client that we're still going
	heartbeatTicker := time.NewTicker(time.Second * 3)
	defer heartbeatTicker.Stop()

	// Process all events from the task stream. Return when a task result is
	// found or the context is canceled. The worker handles the task timeouts.
	done := false
	for !done {
		select {
		// Forward all events via the publisher
		case event, ok := <-taskStream.Channel():
			if !ok {
				done = true
				continue
			}
			if err := pub.Send(ctx, event); err != nil {
				return nil, err // Canceled context probably
			}
			if event.Error != "" {
				logger.Error("task failed", "error", event.Error)
				return nil, errors.New(event.Error)
			}
			if event.TaskResult != nil {
				logger.Info("task completed")
				return event.TaskResult, nil
			}

		// Send heartbeats periodically
		case <-heartbeatTicker.C:
			pub.Send(ctx, &StreamEvent{
				Type:      "task.heartbeat",
				TaskName:  task.Name(),
				AgentName: agent.Name(),
			})

		// Abort if the context is canceled
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Reaching this point may indicate a bug since a task result should have
	// been returned, even if it failed. Return an error in any case.
	return nil, fmt.Errorf("task %q did not return a result", task.Name())
}

// GetAgent returns the agent with the given name
func (t *DiveTeam) GetAgent(name string) (Agent, bool) {
	for _, agent := range t.agents {
		if agent.Name() == name {
			return agent, true
		}
	}
	return nil, false
}

// Overview returns a string representation of the team, which can be included
// in agent prompts to help them understand the team's capabilities.
func (t *DiveTeam) Overview() (string, error) {
	return executeTemplate(teamPromptTemplate, t)
}

// HandleEvent passes an event to any agents that accept it.
func (t *DiveTeam) HandleEvent(ctx context.Context, event *Event) error {
	for _, agent := range t.agents {
		eventedAgent, ok := agent.(EventHandlerAgent)
		if !ok {
			continue
		}
		acceptedEvents := eventedAgent.AcceptedEvents()
		if !sliceContains(acceptedEvents, "*") && !sliceContains(acceptedEvents, event.Name) {
			continue
		}
		if err := eventedAgent.HandleEvent(ctx, event); err != nil {
			return err
		}
		t.logger.Debug("passed event to agent",
			"event_name", event.Name,
			"agent_name", agent.Name(),
		)
	}
	return nil
}

func AgentNames(agents []Agent) []string {
	var agentNames []string
	for _, agent := range agents {
		agentNames = append(agentNames, agent.Name())
	}
	return agentNames
}

func TaskNames(tasks []*Task) []string {
	var taskNames []string
	for _, task := range tasks {
		taskNames = append(taskNames, task.Name())
	}
	return taskNames
}
