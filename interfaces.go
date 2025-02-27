package dive

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/getstingrai/dive/llm"
)

type Event struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Role describes an agent's role on the team: its purpose, responsibilities,
// what agents it can supervise, and what events it can handle.
type Role struct {
	Description    string
	IsSupervisor   bool
	Subordinates   []string
	AcceptedEvents []string
}

func (r Role) String() string {
	var lines []string
	result := strings.TrimSpace(r.Description)
	if result != "" {
		if !strings.HasSuffix(result, ".") {
			result += "."
		}
		lines = append(lines, result)
	}
	if r.IsSupervisor {
		lines = append(lines, "You are a supervisor. You can assign work to other agents.")
	}
	if len(lines) > 0 {
		result = strings.Join(lines, "\n\n")
	}
	return result
}

// Team is a collection of agents that work together to complete tasks
type Team interface {
	// Name of the team
	Name() string

	// Description of the team
	Description() string

	// Overview of the team
	Overview() (string, error)

	// Agents belonging to the team
	Agents() []Agent

	// GetAgent returns an agent by name
	GetAgent(name string) (Agent, bool)

	// Event passes an event to the team
	Event(ctx context.Context, event *Event) error

	// Work on one or more tasks. The returned stream can be read from
	// asynchronously to receive events and task results.
	Work(ctx context.Context, tasks ...*Task) (Stream, error)

	// Start all agents belonging to the team
	Start(ctx context.Context) error

	// Stop all agents belonging to the team
	Stop(ctx context.Context) error

	// IsRunning returns true if the team is running
	IsRunning() bool
}

// Agent is an entity that can perform tasks and interact with the world
type Agent interface {
	// Name of the agent
	Name() string

	// Role returns the agent's assigned role
	Role() Role

	// Join a team. This is only valid if the agent is not yet running and is
	// not yet a member of any team.
	Join(team Team) error

	// Team the agent belongs to. Returns nil if the agent is not on a team.
	Team() Team

	// Chat with the agent in a non-streaming fashion
	Chat(ctx context.Context, message *llm.Message) (*llm.Response, error)

	// Event passes an event to the agent
	Event(ctx context.Context, event *Event) error

	// Work gives the agent a task to complete and returns a stream of events
	// that can be read from asynchronously
	Work(ctx context.Context, task *Task) (Stream, error)

	// Start the agent
	Start(ctx context.Context) error

	// Stop the agent
	Stop(ctx context.Context) error

	// IsRunning returns true if the agent is running
	IsRunning() bool
}

// Stream provides access to a stream of events from a Team or Agent
type Stream interface {
	// Channel returns the channel to be used to receive events
	Channel() <-chan *StreamEvent

	// Close closes the stream
	Close()
}

// StreamEvent is an event from a Stream
type StreamEvent struct {
	// Type of the event
	Type string `json:"type"`

	// TaskName is the name of the task that generated the event, if any
	TaskName string `json:"task_name,omitempty"`

	// AgentName is the name of the agent associated with the event, if any
	AgentName string `json:"agent_name,omitempty"`

	// Data contains the event payload
	Data json.RawMessage `json:"data,omitempty"`

	// Error contains an error message if the event is an error
	Error string `json:"error,omitempty"`

	// TaskResult is the result of a task, if the event is a task result
	TaskResult *TaskResult `json:"task_result,omitempty"`
}

// TODO: TaskResult should not hold a Task. Make it JSON serializable?
