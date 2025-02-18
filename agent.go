package agent

import (
	"context"
	"errors"
	"time"

	"github.com/getstingrai/agents/llm"
)

var (
	ErrAgentBusy = errors.New("agent is busy")
)

type Event struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Role struct {
	Name        string
	Description string
	CanChat     bool
	CanHandle   []string
	CanWork     []string
}

type Goal struct {
	Name        string
	Description string
	Completed   bool
	CompletedAt time.Time
	Error       error
	Priority    int
	Task        *Task
}

type Promise struct {
	agent Agent
	ch    chan *TaskResult
}

func (p *Promise) Get(ctx context.Context) (*TaskResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-p.ch:
		if res.Error != nil {
			return nil, res.Error
		}
		return res, nil
	}
}

// Agent is an entity that can perform tasks and interact with the world
type Agent interface {
	// Name of the agent
	Name() string

	// Role returns the agent's assigned role
	Role() *Role

	// Goals returns the agent's current goals
	Goals() []*Goal

	// Join a team
	Join(ctx context.Context, team *Team) error

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
