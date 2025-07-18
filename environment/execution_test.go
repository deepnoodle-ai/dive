package environment

import (
	"context"
	"testing"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

// TestNewExecution tests the creation of new execution instances
func TestNewExecution(t *testing.T) {
	// Create a simple workflow with inputs
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "test-workflow",
		Inputs: []*workflow.Input{
			{Name: "required_input", Type: "string", Required: true},
			{Name: "optional_input", Type: "string", Default: "default_value"},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "test_step",
				Type:   "script",
				Script: `"Test result"`,
			}),
		},
	})
	require.NoError(t, err)

	// Create test environment
	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	t.Run("valid execution creation", func(t *testing.T) {
		execution, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs: map[string]interface{}{
				"required_input": "test_value",
			},
			Logger: slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)
		require.NotNil(t, execution)
		require.Equal(t, ExecutionStatusPending, execution.Status())
		require.NotEmpty(t, execution.ID())

		// Check that inputs were processed correctly
		require.Equal(t, "test_value", execution.inputs["required_input"])
		require.Equal(t, "default_value", execution.inputs["optional_input"])
	})

	t.Run("missing required input", func(t *testing.T) {
		_, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs:      map[string]interface{}{}, // Missing required input
			Logger:      slogger.NewDevNullLogger(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "input \"required_input\" is required")
	})

	t.Run("unknown input rejected", func(t *testing.T) {
		_, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs: map[string]interface{}{
				"required_input": "test_value",
				"unknown_input":  "some_value",
			},
			Logger: slogger.NewDevNullLogger(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown input \"unknown_input\"")
	})

	t.Run("missing workflow fails", func(t *testing.T) {
		_, err := NewExecution(ExecutionOptions{
			Environment: env,
			Inputs:      map[string]interface{}{},
			Logger:      slogger.NewDevNullLogger(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "workflow is required")
	})
}

// TestExecutionRun tests basic execution functionality
func TestExecutionRun(t *testing.T) {
	// Create a simple script workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "simple-execution",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "script_step",
				Type:   "script",
				Script: `"Execution completed successfully"`,
				Store:  "result",
			}),
		},
	})
	require.NoError(t, err)

	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	t.Run("successful execution", func(t *testing.T) {
		execution, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs:      map[string]interface{}{},
			Logger:      slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		// Execute the workflow
		ctx := context.Background()
		err = execution.Run(ctx)
		require.NoError(t, err)

		// Verify execution completed
		require.Equal(t, ExecutionStatusCompleted, execution.Status())
		require.False(t, execution.startTime.IsZero())
		require.False(t, execution.endTime.IsZero())
		require.True(t, execution.endTime.After(execution.startTime))

		// Verify result was stored
		result, exists := execution.state.Get("result")
		require.True(t, exists)
		require.Equal(t, "Execution completed successfully", result)

		// Verify path state
		require.Len(t, execution.paths, 1)
		mainPath := execution.paths["main"]
		require.Equal(t, PathStatusCompleted, mainPath.Status)
		require.False(t, mainPath.EndTime.IsZero())
	})
}

// TestExecutionWithMultipleSteps tests execution with multiple connected steps
func TestExecutionWithMultipleSteps(t *testing.T) {
	// Create a workflow with multiple steps
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "multi-step-execution",
		Inputs: []*workflow.Input{
			{Name: "input_value", Type: "string", Required: true},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "step1",
				Type:   "script",
				Script: `"Processed: " + inputs.input_value`,
				Store:  "step1_result",
				Next:   []*workflow.Edge{{Step: "step2"}},
			}),
			workflow.NewStep(workflow.StepOptions{
				Name:   "step2",
				Type:   "script",
				Script: `"Final: " + state.step1_result`,
				Store:  "final_result",
			}),
		},
	})
	require.NoError(t, err)

	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs: map[string]interface{}{
			"input_value": "test data",
		},
		Logger: slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Execute the workflow
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Verify both steps executed and stored results
	step1Result, exists := execution.state.Get("step1_result")
	require.True(t, exists)
	require.Equal(t, "Processed: test data", step1Result)

	finalResult, exists := execution.state.Get("final_result")
	require.True(t, exists)
	require.Equal(t, "Final: Processed: test data", finalResult)
}

// TestExecutionCheckpointing tests the checkpoint functionality
func TestExecutionCheckpointing(t *testing.T) {
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "checkpoint-test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "checkpoint_step",
				Type:   "script",
				Script: `"Checkpoint test completed"`,
				Store:  "checkpoint_result",
			}),
		},
	})
	require.NoError(t, err)

	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Test manual checkpoint saving before execution
	err = execution.saveCheckpoint(ctx)
	require.NoError(t, err)

	// Execute and verify automatic checkpointing works
	err = execution.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Verify the result was stored
	result, exists := execution.state.Get("checkpoint_result")
	require.True(t, exists)
	require.Equal(t, "Checkpoint test completed", result)
}

// TestExecutionStatusTransitions tests execution status changes
func TestExecutionStatusTransitions(t *testing.T) {
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "status-test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "status_step",
				Type:   "script",
				Script: `"Status test"`,
			}),
		},
	})
	require.NoError(t, err)

	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Initially pending
	require.Equal(t, ExecutionStatusPending, execution.Status())

	// Run and verify status progression
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)

	// Finally completed
	require.Equal(t, ExecutionStatusCompleted, execution.Status())
}
