package dive

import (
	"context"
)

var _ Agent = &MockAgent{}

type MockAgentOptions struct {
	Name         string
	Goal         string
	Instructions string
	IsSupervisor bool
	Subordinates []string
	Response     *Response
}

type MockAgent struct {
	name         string
	goal         string
	instructions string
	isSupervisor bool
	subordinates []string
	response     *Response
}

func NewMockAgent(opts MockAgentOptions) *MockAgent {
	return &MockAgent{
		name:         opts.Name,
		goal:         opts.Goal,
		instructions: opts.Instructions,
		isSupervisor: opts.IsSupervisor,
		subordinates: opts.Subordinates,
		response:     opts.Response,
	}
}

func (a *MockAgent) Name() string {
	return a.name
}

func (a *MockAgent) Goal() string {
	return a.goal
}

func (a *MockAgent) Instructions() string {
	return a.instructions
}

func (a *MockAgent) HasTools() bool {
	return false
}

func (a *MockAgent) IsSupervisor() bool {
	return a.isSupervisor
}

func (a *MockAgent) CreateResponse(ctx context.Context, opts ...Option) (*Response, error) {
	return a.response, nil
}
