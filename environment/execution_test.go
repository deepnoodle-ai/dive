package environment

import (
	"context"
	"testing"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

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
		Logger:    slogger.New(slogger.LevelDebug),
	})
	require.NoError(t, err)
	require.NoError(t, env.Start(context.Background()))

	execution, err := NewEventBasedExecution(env, ExecutionOptions{
		WorkflowName:   wf.Name(),
		Inputs:         map[string]interface{}{},
		EventStore:     NewNullEventStore(),
		EventBatchSize: 10,
	})
	require.NoError(t, err)
	require.NotNil(t, execution)

	require.Equal(t, wf, execution.Workflow())
	require.Equal(t, env, execution.Environment())
	require.Equal(t, StatusPending, execution.Status())

	var runErr error
	go func() {
		runErr = execution.Run(context.Background())
	}()

	require.NoError(t, execution.Wait())
	require.Equal(t, StatusCompleted, execution.Status())
	require.NoError(t, runErr)
}

// func TestExecutionBasicFlow(t *testing.T) {
// 	wf, err := workflow.New(workflow.Options{
// 		Name: "test-workflow",
// 		Steps: []*workflow.Step{
// 			workflow.NewStep(workflow.StepOptions{
// 				Name:   "test-step",
// 				Agent:  &mockAgent{},
// 				Prompt: "test description",
// 			}),
// 		},
// 	})
// 	require.NoError(t, err)

// 	env, err := New(Options{
// 		Name:      "test-env",
// 		Agents:    []dive.Agent{&mockAgent{}},
// 		Workflows: []*workflow.Workflow{wf},
// 		Logger:    slogger.NewDevNullLogger(),
// 	})
// 	require.NoError(t, err)
// 	require.NoError(t, env.Start(context.Background()))

// 	execution, err := env.ExecuteWorkflow(context.Background(), ExecutionOptions{
// 		WorkflowName: wf.Name(),
// 		Inputs:       map[string]interface{}{},
// 	})
// 	require.NoError(t, err)
// 	require.NotNil(t, execution)

// 	require.NoError(t, execution.Wait())
// 	require.Equal(t, StatusCompleted, execution.Status())

// 	outputs := execution.StepOutputs()
// 	require.Equal(t, "test output", outputs["test-step"])
// }

// func TestExecutionWithBranching(t *testing.T) {
// 	wf, err := workflow.New(workflow.Options{
// 		Name: "branching-workflow",
// 		Steps: []*workflow.Step{
// 			workflow.NewStep(workflow.StepOptions{
// 				Name:   "start",
// 				Agent:  &mockAgent{},
// 				Prompt: "Start Task",
// 				Next: []*workflow.Edge{
// 					{Step: "branch1"},
// 					{Step: "branch2"},
// 				},
// 			}),
// 			workflow.NewStep(workflow.StepOptions{
// 				Name:   "branch1",
// 				Agent:  &mockAgent{},
// 				Prompt: "Branch 1 Task",
// 			}),
// 			workflow.NewStep(workflow.StepOptions{
// 				Name:   "branch2",
// 				Agent:  &mockAgent{},
// 				Prompt: "Branch 2 Task",
// 			}),
// 		},
// 	})
// 	require.NoError(t, err)

// 	env, err := New(Options{
// 		Name:      "test-env",
// 		Agents:    []dive.Agent{&mockAgent{}},
// 		Workflows: []*workflow.Workflow{wf},
// 	})
// 	require.NoError(t, err)
// 	require.NoError(t, env.Start(context.Background()))

// 	execution, err := env.ExecuteWorkflow(context.Background(), ExecutionOptions{
// 		WorkflowName: wf.Name(),
// 		Inputs:       map[string]interface{}{},
// 	})
// 	require.NoError(t, err)

// 	require.NoError(t, execution.Wait())
// 	require.Equal(t, StatusCompleted, execution.Status())

// 	outputs := execution.StepOutputs()
// 	require.Equal(t, "test output", outputs["start"])
// 	require.Equal(t, "test output", outputs["branch1"])
// 	require.Equal(t, "test output", outputs["branch2"])

// 	stats := execution.GetStats()
// 	require.Equal(t, 3, stats.TotalPaths)
// 	require.Equal(t, 0, stats.ActivePaths)
// 	require.Equal(t, 3, stats.CompletedPaths)
// 	require.Equal(t, 0, stats.FailedPaths)
// }

// func TestExecutionWithError(t *testing.T) {
// 	wf, err := workflow.New(workflow.Options{
// 		Name: "error-workflow",
// 		Steps: []*workflow.Step{
// 			workflow.NewStep(workflow.StepOptions{
// 				Name:   "error-step",
// 				Prompt: "Error Task",
// 			}),
// 		},
// 	})
// 	require.NoError(t, err)

// 	mockAgent := &mockAgent{
// 		err: errors.New("simulated error"),
// 	}

// 	env, err := New(Options{
// 		Name:      "test-env",
// 		Agents:    []dive.Agent{mockAgent},
// 		Workflows: []*workflow.Workflow{wf},
// 	})
// 	require.NoError(t, err)
// 	require.NoError(t, env.Start(context.Background()))

// 	execution, err := env.ExecuteWorkflow(context.Background(), ExecutionOptions{
// 		WorkflowName: wf.Name(),
// 		Inputs:       map[string]interface{}{},
// 	})
// 	require.NoError(t, err)

// 	err = execution.Wait()
// 	require.Error(t, err)
// 	require.Contains(t, err.Error(), "simulated error")
// 	require.Equal(t, StatusFailed, execution.Status())

// 	stats := execution.GetStats()
// 	require.Equal(t, 1, stats.TotalPaths)
// 	require.Equal(t, 0, stats.ActivePaths)
// 	require.Equal(t, 0, stats.CompletedPaths)
// 	require.Equal(t, 1, stats.FailedPaths)
// }

// func TestExecutionWithInputs(t *testing.T) {
// 	wf, err := workflow.New(workflow.Options{
// 		Name: "input-workflow",
// 		Inputs: []*workflow.Input{
// 			{
// 				Name:     "required_input",
// 				Type:     "string",
// 				Required: true,
// 			},
// 			{
// 				Name:     "optional_input",
// 				Type:     "string",
// 				Default:  "default_value",
// 				Required: false,
// 			},
// 		},
// 		Steps: []*workflow.Step{
// 			workflow.NewStep(workflow.StepOptions{
// 				Name:   "input-step",
// 				Agent:  &mockAgent{},
// 				Prompt: "Input Task",
// 			}),
// 		},
// 	})
// 	require.NoError(t, err)

// 	env, err := New(Options{
// 		Name:      "test-env",
// 		Agents:    []dive.Agent{&mockAgent{}},
// 		Workflows: []*workflow.Workflow{wf},
// 	})
// 	require.NoError(t, err)
// 	require.NoError(t, env.Start(context.Background()))

// 	// Test missing required input
// 	_, err = env.ExecuteWorkflow(context.Background(), ExecutionOptions{
// 		WorkflowName: wf.Name(),
// 	})
// 	require.Error(t, err)
// 	require.Contains(t, err.Error(), "required input")

// 	// Test with required input
// 	execution, err := env.ExecuteWorkflow(context.Background(), ExecutionOptions{
// 		WorkflowName: wf.Name(),
// 		Inputs: map[string]interface{}{
// 			"required_input": "test_value",
// 		},
// 	})
// 	require.NoError(t, err)
// 	require.NoError(t, execution.Wait())
// 	require.Equal(t, StatusCompleted, execution.Status())
// }

// func TestExecutionContextCancellation(t *testing.T) {
// 	wf, err := workflow.New(workflow.Options{
// 		Name: "cancellation-workflow",
// 		Steps: []*workflow.Step{
// 			workflow.NewStep(workflow.StepOptions{
// 				Name:   "slow-step",
// 				Agent:  &mockAgent{},
// 				Prompt: "Slow Task",
// 			}),
// 		},
// 	})
// 	require.NoError(t, err)

// 	mockAgent := &mockAgent{
// 		// workFn: func(ctx context.Context, task dive.Task) (dive.EventStream, error) {
// 		// 	stream, publisher := dive.NewEventStream()
// 		// 	go func() {
// 		// 		defer publisher.Close()
// 		// 		time.Sleep(100 * time.Millisecond)
// 		// 		publisher.Send(ctx, &dive.Event{
// 		// 			Type: "task.completed",
// 		// 			Payload: &dive.StepResult{
// 		// 				Content: "completed",
// 		// 			},
// 		// 		})
// 		// 	}()
// 		// 	return stream, nil
// 		// },
// 	}

// 	env, err := New(Options{
// 		Name:      "test-env",
// 		Agents:    []dive.Agent{mockAgent},
// 		Workflows: []*workflow.Workflow{wf},
// 	})
// 	require.NoError(t, err)
// 	require.NoError(t, env.Start(context.Background()))

// 	ctx, cancel := context.WithCancel(context.Background())
// 	execution, err := env.ExecuteWorkflow(ctx, ExecutionOptions{
// 		WorkflowName: wf.Name(),
// 	})
// 	require.NoError(t, err)

// 	// Cancel the context before the task completes
// 	cancel()

// 	err = execution.Wait()
// 	require.Error(t, err)
// 	require.Contains(t, err.Error(), "context canceled")
// 	require.Equal(t, StatusFailed, execution.Status())
// }

// func TestEachStepStoreArray(t *testing.T) {
// 	// Create a workflow with an each step that has a store variable
// 	each := &workflow.EachBlock{
// 		Items: []string{"item1", "item2", "item3"},
// 		As:    "item",
// 	}

// 	wf, err := workflow.New(workflow.Options{
// 		Name: "test-each-store",
// 		Steps: []*workflow.Step{
// 			workflow.NewStep(workflow.StepOptions{
// 				Name:   "process-items",
// 				Agent:  &mockAgent{},
// 				Prompt: "Process item",
// 				Each:   each,
// 				Store:  "results",
// 			}),
// 		},
// 	})
// 	require.NoError(t, err)

// 	env, err := New(Options{
// 		Name:      "test-env",
// 		Agents:    []dive.Agent{&mockAgent{}},
// 		Workflows: []*workflow.Workflow{wf},
// 		Logger:    slogger.NewDevNullLogger(),
// 	})
// 	require.NoError(t, err)
// 	require.NoError(t, env.Start(context.Background()))

// 	execution, err := env.ExecuteWorkflow(context.Background(), ExecutionOptions{
// 		WorkflowName: wf.Name(),
// 		Inputs:       map[string]interface{}{},
// 	})
// 	require.NoError(t, err)
// 	require.NoError(t, execution.Wait())
// 	require.Equal(t, StatusCompleted, execution.Status())

// 	// Check that the stored variable is an array with 3 items
// 	storedValue, exists := execution.scriptGlobals["results"]
// 	require.True(t, exists, "results variable should be stored")

// 	list, ok := storedValue.(*object.List)
// 	require.True(t, ok, "stored value should be a list/array")

// 	items := list.Value()
// 	require.Len(t, items, 3, "should have 3 items in the array")

// 	// Check that each item is a string containing "test output"
// 	for _, item := range items {
// 		str, ok := item.(*object.String)
// 		require.True(t, ok, "each item should be a string")
// 		require.Equal(t, "test output", str.Value())
// 	}
// }
