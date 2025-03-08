package agent

import (
	"context"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/llm"
)

type WorkFunc func(ctx context.Context, task dive.Task) (dive.Stream, error)

type HandleEventFunc func(ctx context.Context, event *dive.Event) error

type MockAgentOptions struct {
	Name           string
	Goal           string
	Backstory      string
	IsSupervisor   bool
	Subordinates   []string
	Work           WorkFunc
	AcceptedEvents []string
	HandleEvent    HandleEventFunc
	Response       *llm.Response
}

type MockAgent struct {
	name           string
	goal           string
	backstory      string
	isSupervisor   bool
	subordinates   []string
	environment    dive.Environment
	work           WorkFunc
	acceptedEvents []string
	handleEvent    HandleEventFunc
	response       *llm.Response
}

func NewMockAgent(opts MockAgentOptions) *MockAgent {
	return &MockAgent{
		name:           opts.Name,
		goal:           opts.Goal,
		backstory:      opts.Backstory,
		isSupervisor:   opts.IsSupervisor,
		subordinates:   opts.Subordinates,
		work:           opts.Work,
		acceptedEvents: opts.AcceptedEvents,
		handleEvent:    opts.HandleEvent,
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

func (a *MockAgent) HandleEvent(ctx context.Context, event *dive.Event) error {
	return a.handleEvent(ctx, event)
}

func (a *MockAgent) IsSupervisor() bool {
	return a.isSupervisor
}

func (a *MockAgent) SetEnvironment(env dive.Environment) {
	a.environment = env
}

func (a *MockAgent) Work(ctx context.Context, task dive.Task) (dive.Stream, error) {
	return a.work(ctx, task)
}

func (a *MockAgent) Generate(ctx context.Context, message *llm.Message, opts ...dive.GenerateOption) (*llm.Response, error) {
	return a.response, nil
}

func (a *MockAgent) Stream(ctx context.Context, message *llm.Message, opts ...dive.GenerateOption) (dive.Stream, error) {
	return nil, nil
}

func (a *MockAgent) Start(ctx context.Context) error {
	return nil
}

func (a *MockAgent) Stop(ctx context.Context) error {
	return nil
}

func (a *MockAgent) IsRunning() bool {
	return true
}
