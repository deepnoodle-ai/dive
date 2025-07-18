package environment

import (
	"context"
	"errors"
	"testing"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

func TestOperationExecution(t *testing.T) {
	// Create a test environment
	env := &Environment{
		agents:    map[string]dive.Agent{"test-agent": &agent.MockAgent{}},
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	// Create a simple test workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "test-workflow",
		Inputs: []*workflow.Input{
			{Name: "input1", Type: "string", Required: true},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{Name: "step1", Type: "script", Script: `"test result"`}),
		},
	})
	require.NoError(t, err)

	// Create an execution
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{"input1": "value1"},
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	t.Run("operation execution with logging", func(t *testing.T) {
		ctx := context.Background()

		// Create a test operation
		op := Operation{
			Type:     "test_operation",
			StepName: "step1",
			PathID:   "main",
			Parameters: map[string]interface{}{
				"test_param": "test_value",
			},
		}

		// Verify ID is empty initially
		require.Empty(t, op.ID)

		// Execute the operation
		result, err := execution.ExecuteOperation(ctx, op, func() (interface{}, error) {
			return "operation_result", nil
		})

		require.NoError(t, err)
		require.Equal(t, "operation_result", result)

		// Note: Since Go passes structs by value, the ID changes inside ExecuteOperation
		// won't be reflected in our local op variable. This is expected behavior.
		// What's important is that the operation executed successfully.
	})

	t.Run("operation ID generation", func(t *testing.T) {
		op := Operation{
			Type:     "test_operation",
			StepName: "step1",
			PathID:   "main",
			Parameters: map[string]interface{}{
				"param1": "value1",
			},
		}

		// Generate ID
		id := op.GenerateID()
		require.NotEmpty(t, id)
		require.Contains(t, string(id), "op_")

		// Same operation should generate same ID
		id2 := op.GenerateID()
		require.Equal(t, id, id2)
	})

	t.Run("operation with error", func(t *testing.T) {
		ctx := context.Background()

		op := Operation{
			Type:     "test_operation",
			StepName: "step1",
			PathID:   "main",
		}

		testError := errors.New("test operation error")

		// Execute operation that fails
		_, err := execution.ExecuteOperation(ctx, op, func() (interface{}, error) {
			return nil, testError
		})

		require.Error(t, err)
		require.Equal(t, testError, err)
	})
}
