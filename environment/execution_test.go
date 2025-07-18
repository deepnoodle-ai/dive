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
	// Create a simple workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "test-workflow",
		Inputs: []*workflow.Input{
			{Name: "input1", Type: "string", Required: true},
			{Name: "input2", Type: "string", Default: "default_value"},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "step1",
				Type:   "prompt",
				Prompt: "Test prompt: ${inputs.input1}",
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
				"input1": "test_value",
			},
			Logger: slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)
		require.NotNil(t, execution)
		require.Equal(t, ExecutionStatusPending, execution.Status())
		require.NotEmpty(t, execution.ID())
	})

	t.Run("missing required input", func(t *testing.T) {
		_, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs:      map[string]interface{}{}, // Missing required input1
			Logger:      slogger.NewDevNullLogger(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "input \"input1\" is required")
	})

	t.Run("unknown input", func(t *testing.T) {
		_, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs: map[string]interface{}{
				"input1":      "test_value",
				"unknown_key": "some_value",
			},
			Logger: slogger.NewDevNullLogger(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown input \"unknown_key\"")
	})

	t.Run("default values used", func(t *testing.T) {
		execution, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs: map[string]interface{}{
				"input1": "test_value",
				// input2 should use default value
			},
			Logger: slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)
		require.Equal(t, "default_value", execution.inputs["input2"])
	})
}

func TestExecutionBasicOperations(t *testing.T) {
	// Create a simple script workflow that doesn't require external services
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "script-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "script_step",
				Type:   "script",
				Script: `"Hello, World!"`,
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

	t.Run("basic script execution", func(t *testing.T) {
		execution, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs:      map[string]interface{}{},
			Logger:      slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		ctx := context.Background()
		err = execution.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, ExecutionStatusCompleted, execution.Status())

		// Check that the result was stored
		result, exists := execution.state.Get("result")
		require.True(t, exists)
		require.Equal(t, "Hello, World!", result)
	})
}

func TestExecutionCheckpointing(t *testing.T) {
	// Create a simple workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "checkpoint-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "step1",
				Type:   "script",
				Script: `"Step 1 complete"`,
				Store:  "step1_result",
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

	t.Run("checkpoint creation", func(t *testing.T) {
		execution, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs:      map[string]interface{}{},
			Logger:      slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		// Save a checkpoint manually
		ctx := context.Background()
		err = execution.saveCheckpoint(ctx)
		require.NoError(t, err)
	})
}
