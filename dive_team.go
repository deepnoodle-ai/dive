package dive

import (
	"context"
	"fmt"
	"strings"
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
	LogDirectory string
}

// NewTeam creates a new team with the given agents and tasks
func NewTeam(opts TeamOptions) (*DiveTeam, error) {
	t := &DiveTeam{
		name:        opts.Name,
		description: opts.Description,
		agents:      opts.Agents,
		tasks:       make(map[string]*Task),
		state:       make(map[string]interface{}),
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

func (t *DiveTeam) calculateTaskOrder(tasks []*Task) ([]string, error) {
	graph := newTaskGraph(tasks)
	order, err := graph.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("invalid task dependencies: %w", err)
	}
	return order, nil
}

func (t *DiveTeam) Start(ctx context.Context) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.started {
		return fmt.Errorf("team already started")
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

func (t *DiveTeam) Work(ctx context.Context, tasks ...*Task) (Stream, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.started {
		return nil, fmt.Errorf("team not started")
	}

	if err := t.addTasks(tasks...); err != nil {
		return nil, err
	}

	tasksByName := make(map[string]*Task, len(tasks))
	for _, task := range tasks {
		tasksByName[task.Name()] = task
	}

	order, err := t.calculateTaskOrder(tasks)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate task order: %w", err)
	}

	// Create a stream to return immediately
	stream := NewDiveStream()

	fmt.Println("XXX starting task processing")

	// Start a goroutine to process tasks asynchronously
	go func() {
		publisher := NewDiveStreamPublisher(stream)
		defer publisher.Close()

		resultsByTaskName := make(map[string]*TaskResult, len(order))

		for _, taskName := range order {
			task := tasksByName[taskName]
			var assignedAgent Agent
			if task.AssignedAgent() != nil {
				assignedAgent = task.AssignedAgent()
			} else if len(t.supervisors) > 0 {
				assignedAgent = t.supervisors[0]
			} else {
				assignedAgent = t.agents[0]
			}
			agentName := assignedAgent.Name()

			// Set the dependencies output for the task
			if depNames := task.Dependencies(); len(depNames) > 0 {
				var depOutputs []string
				for _, depName := range depNames {
					depTask, ok := resultsByTaskName[depName]
					if !ok {
						publisher.SendEvent(ctx, &StreamEvent{
							Type:      "error",
							TaskName:  taskName,
							AgentName: assignedAgent.Name(),
							Error:     fmt.Sprintf("task %q has dependency %q, but it has not been completed yet", taskName, depName),
						})
						return
					}
					entry := fmt.Sprintf("# Task %q Output\n\n%s", depName, depTask.Content)
					depOutputs = append(depOutputs, entry)
				}
				depOutputsText := strings.Join(depOutputs, "\n\n")
				task.SetDependenciesOutput(depOutputsText)
			}

			fmt.Println("XXX assigning task", taskName, "to", agentName)

			// Start the task
			taskStream, err := assignedAgent.Work(ctx, task)
			if err != nil {
				publisher.SendEvent(ctx, &StreamEvent{
					Type:      "error",
					TaskName:  taskName,
					AgentName: agentName,
					Error:     fmt.Sprintf("failed to assign work to agent %s: %v", agentName, err),
				})
				return
			}

			// Guarantee we close the stream no matter what. This may be
			// redundant for other calls below, but that's fine.
			defer taskStream.Close()

			// Process all events and results from the task stream. We don't
			// move to the next task until this one is complete.
			taskDone := false
			for !taskDone {
				select {

				// Forward events
				case event, ok := <-taskStream.Events():
					if !ok {
						continue
					}
					if !publisher.SendEvent(ctx, event) {
						return // Context canceled
					}

				// Forward results and process
				case result, ok := <-taskStream.Results():
					taskDone = true
					if !ok {
						continue
					}
					if !publisher.SendResult(ctx, result) {
						return // Context canceled
					}
					if result.Error != nil {
						return // The task failed, so we are done
					}
					resultsByTaskName[taskName] = result
					fmt.Println("XXX done with task", taskName, "result", result.Content, "agent", agentName)

				case <-ctx.Done():
					publisher.SendEvent(context.Background(), &StreamEvent{
						Type:      "error",
						TaskName:  taskName,
						AgentName: agentName,
						Error:     fmt.Sprintf("context canceled while waiting for task %s: %v", taskName, ctx.Err()),
					})
					taskStream.Close()
					return
				}
			}
		}

		// Send a final event indicating all tasks are complete
		completeEvent := &StreamEvent{Type: "done"}
		publisher.SendEvent(context.Background(), completeEvent)
	}()

	return stream, nil
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
