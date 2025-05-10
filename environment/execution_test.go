package environment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
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
					Role: llm.Assistant,
					Content: []*llm.Content{
						{Type: llm.ContentTypeText, Text: "test output"},
					},
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

func TestNewExecution(t *testing.T) {
	wf, err := workflow.New(workflow.Options{
		Name: "test-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "test-step",
				Agent:  &mockAgent{},
				Prompt: "test description",
			}),
		},
	})
	require.NoError(t, err)

	env, err := New(Options{
		Name:      "test-env",
		Agents:    []dive.Agent{&mockAgent{}},
		Workflows: []*workflow.Workflow{wf},
		Logger:    slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)
	require.NoError(t, env.Start(context.Background()))

	execution, err := env.ExecuteWorkflow(context.Background(), ExecutionOptions{
		WorkflowName: wf.Name(),
		Inputs:       map[string]interface{}{},
	})
	require.NoError(t, err)
	require.NotNil(t, execution)

	require.Equal(t, wf, execution.Workflow())
	require.Equal(t, env, execution.Environment())
	require.Equal(t, StatusRunning, execution.Status())
}

func TestExecutionBasicFlow(t *testing.T) {
	wf, err := workflow.New(workflow.Options{
		Name: "test-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "test-step",
				Agent:  &mockAgent{},
				Prompt: "test description",
			}),
		},
	})
	require.NoError(t, err)

	env, err := New(Options{
		Name:      "test-env",
		Agents:    []dive.Agent{&mockAgent{}},
		Workflows: []*workflow.Workflow{wf},
		Logger:    slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)
	require.NoError(t, env.Start(context.Background()))

	execution, err := env.ExecuteWorkflow(context.Background(), ExecutionOptions{
		WorkflowName: wf.Name(),
		Inputs:       map[string]interface{}{},
	})
	require.NoError(t, err)
	require.NotNil(t, execution)

	require.NoError(t, execution.Wait())
	require.Equal(t, StatusCompleted, execution.Status())

	outputs := execution.StepOutputs()
	require.Equal(t, "test output", outputs["test-step"])
}

func TestExecutionWithBranching(t *testing.T) {
	wf, err := workflow.New(workflow.Options{
		Name: "branching-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "start",
				Agent:  &mockAgent{},
				Prompt: "Start Task",
				Next: []*workflow.Edge{
					{Step: "branch1"},
					{Step: "branch2"},
				},
			}),
			workflow.NewStep(workflow.StepOptions{
				Name:   "branch1",
				Agent:  &mockAgent{},
				Prompt: "Branch 1 Task",
			}),
			workflow.NewStep(workflow.StepOptions{
				Name:   "branch2",
				Agent:  &mockAgent{},
				Prompt: "Branch 2 Task",
			}),
		},
	})
	require.NoError(t, err)

	env, err := New(Options{
		Name:      "test-env",
		Agents:    []dive.Agent{&mockAgent{}},
		Workflows: []*workflow.Workflow{wf},
	})
	require.NoError(t, err)
	require.NoError(t, env.Start(context.Background()))

	execution, err := env.ExecuteWorkflow(context.Background(), ExecutionOptions{
		WorkflowName: wf.Name(),
		Inputs:       map[string]interface{}{},
	})
	require.NoError(t, err)

	require.NoError(t, execution.Wait())
	require.Equal(t, StatusCompleted, execution.Status())

	outputs := execution.StepOutputs()
	require.Equal(t, "test output", outputs["start"])
	require.Equal(t, "test output", outputs["branch1"])
	require.Equal(t, "test output", outputs["branch2"])

	stats := execution.GetStats()
	require.Equal(t, 3, stats.TotalPaths)
	require.Equal(t, 0, stats.ActivePaths)
	require.Equal(t, 3, stats.CompletedPaths)
	require.Equal(t, 0, stats.FailedPaths)
}

func TestExecutionWithError(t *testing.T) {
	wf, err := workflow.New(workflow.Options{
		Name: "error-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "error-step",
				Prompt: "Error Task",
			}),
		},
	})
	require.NoError(t, err)

	mockAgent := &mockAgent{
		err: errors.New("simulated error"),
	}

	env, err := New(Options{
		Name:      "test-env",
		Agents:    []dive.Agent{mockAgent},
		Workflows: []*workflow.Workflow{wf},
	})
	require.NoError(t, err)
	require.NoError(t, env.Start(context.Background()))

	execution, err := env.ExecuteWorkflow(context.Background(), ExecutionOptions{
		WorkflowName: wf.Name(),
		Inputs:       map[string]interface{}{},
	})
	require.NoError(t, err)

	err = execution.Wait()
	require.Error(t, err)
	require.Contains(t, err.Error(), "simulated error")
	require.Equal(t, StatusFailed, execution.Status())

	stats := execution.GetStats()
	require.Equal(t, 1, stats.TotalPaths)
	require.Equal(t, 0, stats.ActivePaths)
	require.Equal(t, 0, stats.CompletedPaths)
	require.Equal(t, 1, stats.FailedPaths)
}

func TestExecutionWithInputs(t *testing.T) {
	wf, err := workflow.New(workflow.Options{
		Name: "input-workflow",
		Inputs: []*workflow.Input{
			{
				Name:     "required_input",
				Type:     "string",
				Required: true,
			},
			{
				Name:     "optional_input",
				Type:     "string",
				Default:  "default_value",
				Required: false,
			},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "input-step",
				Agent:  &mockAgent{},
				Prompt: "Input Task",
			}),
		},
	})
	require.NoError(t, err)

	env, err := New(Options{
		Name:      "test-env",
		Agents:    []dive.Agent{&mockAgent{}},
		Workflows: []*workflow.Workflow{wf},
	})
	require.NoError(t, err)
	require.NoError(t, env.Start(context.Background()))

	// Test missing required input
	_, err = env.ExecuteWorkflow(context.Background(), ExecutionOptions{
		WorkflowName: wf.Name(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "required input")

	// Test with required input
	execution, err := env.ExecuteWorkflow(context.Background(), ExecutionOptions{
		WorkflowName: wf.Name(),
		Inputs: map[string]interface{}{
			"required_input": "test_value",
		},
	})
	require.NoError(t, err)
	require.NoError(t, execution.Wait())
	require.Equal(t, StatusCompleted, execution.Status())
}

func TestExecutionContextCancellation(t *testing.T) {
	wf, err := workflow.New(workflow.Options{
		Name: "cancellation-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "slow-step",
				Agent:  &mockAgent{},
				Prompt: "Slow Task",
			}),
		},
	})
	require.NoError(t, err)

	mockAgent := &mockAgent{
		// workFn: func(ctx context.Context, task dive.Task) (dive.EventStream, error) {
		// 	stream, publisher := dive.NewEventStream()
		// 	go func() {
		// 		defer publisher.Close()
		// 		time.Sleep(100 * time.Millisecond)
		// 		publisher.Send(ctx, &dive.Event{
		// 			Type: "task.completed",
		// 			Payload: &dive.StepResult{
		// 				Content: "completed",
		// 			},
		// 		})
		// 	}()
		// 	return stream, nil
		// },
	}

	env, err := New(Options{
		Name:      "test-env",
		Agents:    []dive.Agent{mockAgent},
		Workflows: []*workflow.Workflow{wf},
	})
	require.NoError(t, err)
	require.NoError(t, env.Start(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	execution, err := env.ExecuteWorkflow(ctx, ExecutionOptions{
		WorkflowName: wf.Name(),
	})
	require.NoError(t, err)

	// Cancel the context before the task completes
	cancel()

	err = execution.Wait()
	require.Error(t, err)
	require.Contains(t, err.Error(), "context canceled")
	require.Equal(t, StatusFailed, execution.Status())
}
