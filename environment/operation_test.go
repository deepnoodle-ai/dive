package environment

import (
	"context"
	"errors"
	"testing"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

// TestOperationExecution tests the execution of operations with logging
func TestOperationExecution(t *testing.T) {
	// Create a test environment
	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	// Create a simple test workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "operation-test-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "test_step",
				Type:   "script",
				Script: `"operation test result"`,
			}),
		},
	})
	require.NoError(t, err)

	// Create an execution
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	t.Run("successful operation execution", func(t *testing.T) {
		ctx := context.Background()

		// Create a test operation
		op := Operation{
			Type:     "test_operation",
			StepName: "test_step",
			PathID:   "main",
			Parameters: map[string]interface{}{
				"test_param": "test_value",
			},
		}

		// Execute the operation
		result, err := execution.ExecuteOperation(ctx, op, func() (interface{}, error) {
			return "operation_result", nil
		})

		require.NoError(t, err)
		require.Equal(t, "operation_result", result)
	})

	t.Run("operation with error", func(t *testing.T) {
		ctx := context.Background()

		op := Operation{
			Type:     "failing_operation",
			StepName: "test_step",
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

	t.Run("operation requires path ID", func(t *testing.T) {
		ctx := context.Background()

		op := Operation{
			Type:     "no_path_operation",
			StepName: "test_step",
			// PathID is missing
		}

		_, err := execution.ExecuteOperation(ctx, op, func() (interface{}, error) {
			return "result", nil
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "operation PathID is required")
	})
}

// TestOperationIDGeneration tests deterministic operation ID generation
func TestOperationIDGeneration(t *testing.T) {
	t.Run("consistent ID generation", func(t *testing.T) {
		op := Operation{
			Type:     "test_operation",
			StepName: "test_step",
			PathID:   "main",
			Parameters: map[string]interface{}{
				"param1": "value1",
				"param2": 42,
			},
		}

		// Generate ID multiple times
		id1 := op.GenerateID()
		id2 := op.GenerateID()

		require.NotEmpty(t, id1)
		require.NotEmpty(t, id2)
		require.Equal(t, id1, id2) // Should be deterministic
		require.Contains(t, string(id1), "op_")
	})

	t.Run("different operations have different IDs", func(t *testing.T) {
		op1 := Operation{
			Type:     "test_operation_1",
			StepName: "step1",
			PathID:   "main",
		}

		op2 := Operation{
			Type:     "test_operation_2",
			StepName: "step1",
			PathID:   "main",
		}

		id1 := op1.GenerateID()
		id2 := op2.GenerateID()

		require.NotEqual(t, id1, id2)
	})

	t.Run("parameter order doesn't affect ID", func(t *testing.T) {
		op1 := Operation{
			Type:     "test_operation",
			StepName: "test_step",
			PathID:   "main",
			Parameters: map[string]interface{}{
				"param_a": "value_a",
				"param_b": "value_b",
			},
		}

		op2 := Operation{
			Type:     "test_operation",
			StepName: "test_step",
			PathID:   "main",
			Parameters: map[string]interface{}{
				"param_b": "value_b",
				"param_a": "value_a",
			},
		}

		id1 := op1.GenerateID()
		id2 := op2.GenerateID()

		require.Equal(t, id1, id2) // Should be same regardless of parameter order
	})
}

// TestNewOperation tests the NewOperation constructor function
func TestNewOperation(t *testing.T) {
	op := NewOperation("test_type", "test_step", "test_path", map[string]interface{}{
		"key": "value",
	})

	require.Equal(t, "test_type", op.Type)
	require.Equal(t, "test_step", op.StepName)
	require.Equal(t, "test_path", op.PathID)
	require.Equal(t, "value", op.Parameters["key"])
	require.NotEmpty(t, op.ID) // Should have generated an ID
	require.Contains(t, string(op.ID), "op_")
}
