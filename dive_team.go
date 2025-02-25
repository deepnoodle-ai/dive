package dive

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

var _ Team = &DiveTeam{}

// DiveTeam implements the Team interface
type DiveTeam struct {
	name        string
	description string
	agents      []Agent
	supervisors []Agent
	tasks       map[string]*Task
	state       map[string]interface{}
	started     bool
	taskGraph   *taskGraph
	taskOrder   []string
	mutex       sync.Mutex
}

type TeamOptions struct {
	Name         string
	Description  string
	Agents       []Agent
	Tasks        []*Task
	LogDirectory string
}

// NewTeam creates a new team with the given agents and tasks
func NewTeam(opts TeamOptions) (*DiveTeam, error) {
	t := &DiveTeam{
		name:        opts.Name,
		description: opts.Description,
		agents:      opts.Agents,
		tasks:       make(map[string]*Task, len(opts.Tasks)),
		state:       make(map[string]interface{}),
	}
	if err := t.addTasks(opts.Tasks...); err != nil {
		return nil, err
	}
	for _, agent := range t.agents {
		if err := agent.Join(t); err != nil {
			return nil, err
		}
		if agent.Role().IsSupervisor {
			t.supervisors = append(t.supervisors, agent)
		}
	}
	if len(t.agents) > 1 && len(t.supervisors) == 0 {
		return nil, fmt.Errorf("at least one supervisor is required")
	}
	return t, nil
}

func (t *DiveTeam) Description() string {
	return t.description
}

func (t *DiveTeam) Agents() []Agent {
	// return a copy to help ensure immutability
	agents := make([]Agent, len(t.agents))
	copy(agents, t.agents)
	return agents
}

func (t *DiveTeam) Name() string {
	return t.name
}

func (t *DiveTeam) IsRunning() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	return t.started
}

func (t *DiveTeam) newTaskGraph() *taskGraph {
	var tasks []*Task
	for _, task := range t.tasks {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Name() < tasks[j].Name()
	})
	return newTaskGraph(tasks)
}

func (t *DiveTeam) recalculateTaskOrder() error {
	graph := t.newTaskGraph()
	order, err := graph.TopologicalSort()
	if err != nil {
		return fmt.Errorf("invalid task dependencies: %w", err)
	}
	t.taskGraph = graph
	t.taskOrder = order
	return nil
}

func (t *DiveTeam) Start(ctx context.Context) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.started {
		return fmt.Errorf("team already started")
	}

	if len(t.tasks) > 0 {
		if err := t.recalculateTaskOrder(); err != nil {
			return err
		}
	}

	for _, agent := range t.agents {
		if err := agent.Start(ctx); err != nil {
			return fmt.Errorf("failed to start agent %s: %w", agent.Name(), err)
		}
	}
	t.started = true
	return nil
}

func (t *DiveTeam) Stop(ctx context.Context) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.started {
		return fmt.Errorf("team not started")
	}
	for _, agent := range t.agents {
		if err := agent.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop agent %s: %w", agent.Name(), err)
		}
	}
	t.started = false
	return nil
}

func (t *DiveTeam) Work(ctx context.Context, tasks ...*Task) ([]*TaskResult, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.started {
		return nil, fmt.Errorf("team not started")
	}

	if err := t.addTasks(tasks...); err != nil {
		return nil, err
	}
	if err := t.recalculateTaskOrder(); err != nil {
		return nil, fmt.Errorf("failed to recalculate task order: %w", err)
	}

	// Create promises for tasks in sorted order
	promises := make([]*Promise, len(tasks))
	for i, task := range tasks {
		var assignedAgent Agent
		if task.AssignedAgent() != nil {
			assignedAgent = task.AssignedAgent()
		} else if len(t.supervisors) > 0 {
			assignedAgent = t.supervisors[0]
		} else {
			assignedAgent = t.agents[0]
		}
		promise, err := assignedAgent.Work(ctx, task)
		if err != nil {
			return nil, fmt.Errorf("failed to assign work to agent %s: %w", assignedAgent.Name(), err)
		}
		promises[i] = promise
	}

	results, err := WaitAll(ctx, promises)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for tasks to complete: %w", err)
	}
	return results, nil
}

func (t *DiveTeam) GetAgent(name string) (Agent, bool) {
	for _, agent := range t.agents {
		if agent.Name() == name {
			return agent, true
		}
	}
	return nil, false
}

func (t *DiveTeam) Overview() (string, error) {
	return executeTemplate(teamPromptTemplate, t)
}

func (t *DiveTeam) addTasks(tasks ...*Task) error {
	for _, task := range tasks {
		if err := task.Validate(); err != nil {
			return err
		}
		name := task.Name()
		if t.tasks[name] != nil {
			return fmt.Errorf("task %q already exists", name)
		}
		t.tasks[name] = task
	}
	return nil
}

func (t *DiveTeam) Event(ctx context.Context, event *Event) error {
	for _, agent := range t.agents {
		acceptedEvents := agent.Role().AcceptsEvents
		if !sliceContains(acceptedEvents, "*") &&
			!sliceContains(acceptedEvents, event.Name) {
			continue
		}
		if err := agent.Event(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func sliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
