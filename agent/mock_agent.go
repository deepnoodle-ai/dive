package agent

import (
	"context"

	"github.com/deepnoodle-ai/dive"
)

var _ dive.Agent = &MockAgent{}

type MockAgentOptions struct {
	Name           string
	Goal           string
	Backstory      string
	IsSupervisor   bool
	Subordinates   []string
	AcceptedEvents []string
	Response       *dive.Response
}

type MockAgent struct {
	name           string
	goal           string
	backstory      string
	isSupervisor   bool
	subordinates   []string
	acceptedEvents []string
	response       *dive.Response
}

func NewMockAgent(opts MockAgentOptions) *MockAgent {
	return &MockAgent{
		name:           opts.Name,
		goal:           opts.Goal,
		backstory:      opts.Backstory,
		isSupervisor:   opts.IsSupervisor,
		subordinates:   opts.Subordinates,
		acceptedEvents: opts.AcceptedEvents,
		response:       opts.Response,
	}
}

func (a *MockAgent) Name() string {
	return a.name
}

func (a *MockAgent) Goal() string {
	return a.goal
}

func (a *MockAgent) Backstory() string {
	return a.backstory
}

func (a *MockAgent) IsSupervisor() bool {
	return a.isSupervisor
}

func (a *MockAgent) CreateResponse(ctx context.Context, opts ...dive.Option) (*dive.Response, error) {
	return a.response, nil
}
