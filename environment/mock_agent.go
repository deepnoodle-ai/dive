package environment

import (
	"context"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
)

// mockAgent implements dive.Agent for testing
type mockAgent struct {
	err error
}

func (m *mockAgent) Name() string {
	return "mock-agent"
}

func (m *mockAgent) Goal() string {
	return "Mock agent for testing"
}

func (m *mockAgent) Backstory() string {
	return "Mock backstory"
}

func (m *mockAgent) IsSupervisor() bool {
	return false
}

func (m *mockAgent) SetEnvironment(env dive.Environment) error {
	return nil
}

func (m *mockAgent) CreateResponse(ctx context.Context, opts ...dive.Option) (*dive.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &dive.Response{
		ID:        "test-response",
		Model:     "mock-model",
		CreatedAt: time.Now(),
		Items: []*dive.ResponseItem{
			{
				Type: dive.ResponseItemTypeMessage,
				Message: &llm.Message{
					Role:    llm.Assistant,
					Content: []llm.Content{&llm.TextContent{Text: "test output"}},
				},
			},
		},
	}, nil
}

func (m *mockAgent) StreamResponse(ctx context.Context, opts ...dive.Option) (dive.ResponseStream, error) {
	if m.err != nil {
		return nil, m.err
	}
	stream, publisher := dive.NewEventStream()
	publisher.Send(ctx, &dive.ResponseEvent{
		Type: dive.EventTypeResponseCompleted,
		Response: &dive.Response{
			ID:        "test-response",
			Model:     "mock-model",
			CreatedAt: time.Now(),
			Items:     []*dive.ResponseItem{},
		},
	})
	publisher.Close()
	return stream, nil
}
