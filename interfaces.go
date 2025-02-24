package dive

import (
	"context"

	"github.com/getstingrai/dive/llm"
)

type Event struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Role struct {
	Name          string
	Description   string
	IsSupervisor  bool
	AcceptsChats  bool
	AcceptsEvents []string
	AcceptsWork   []string
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

	// Work on tasks
	Work(ctx context.Context, tasks ...*Task) ([]*Promise, error)

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
	Role() *Role

	// Join a team. This is only valid if the agent is not yet running and is
	// not yet a member of any team.
	Join(team Team) error

	// Team the agent belongs to. Returns nil if the agent is not on a team.
	Team() Team

	// Chat with the agent
	Chat(ctx context.Context, message *llm.Message) (*llm.Response, error)

	// ChatStream streams the conversation between the agent and the user
	ChatStream(ctx context.Context, message *llm.Message) (llm.Stream, error)

	// Event passes an event to the agent
	Event(ctx context.Context, event *Event) error

	// Work gives the agent a task to complete
	Work(ctx context.Context, task *Task) (*Promise, error)

	// Start the agent
	Start(ctx context.Context) error

	// Stop the agent
	Stop(ctx context.Context) error

	// IsRunning returns true if the agent is running
	IsRunning() bool
}
