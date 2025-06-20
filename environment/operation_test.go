package environment

import (
	"context"
	"testing"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

func TestOperationExecution(t *testing.T) {
	// Create a test environment like in existing tests
	env := &Environment{
		agents:    map[string]dive.Agent{"test-agent": &agent.MockAgent{}},
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.New(slogger.LevelInfo),
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
			workflow.NewStep(workflow.StepOptions{Name: "step1", Type: "prompt", Prompt: "Test prompt"}),
		},
	})
	require.NoError(t, err)
	env.workflows["test-workflow"] = testWorkflow

	// Create a null event store for testing
	eventStore := NewNullEventStore()

	// Create an execution
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{"input1": "value1"},
		EventStore:  eventStore,
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Test operation execution
	ctx := context.Background()

	// Create a test operation
	op := NewOperation(
		"test_operation",
		"test_step",
		"test_path",
		map[string]interface{}{
			"param1": "value1",
			"param2": 42,
		},
	)

	// Execute the operation
	expectedResult := "test result"
	result, err := execution.ExecuteOperation(ctx, op, func() (interface{}, error) {
		return expectedResult, nil
	})

	require.NoError(t, err)
	require.Equal(t, expectedResult, result)

	// Verify operation result is cached
	cachedResult, found := execution.FindOperationResult(op.ID)
	require.True(t, found)
	require.Equal(t, expectedResult, cachedResult.Result)
	require.NoError(t, cachedResult.Error)
	require.Equal(t, op.ID, cachedResult.OperationID)
}

func TestWorkflowState(t *testing.T) {
	// Create a null event store for testing
	eventStore := NewNullEventStore()

	// Create execution recorder
	recorder := NewBufferedExecutionRecorder("test-exec", eventStore, 5)

	// Create workflow state
	state := NewWorkflowState("test-exec", recorder)

	// Test state operations
	err := state.Set("key1", "value1")
	require.NoError(t, err)

	value, exists := state.Get("key1")
	require.True(t, exists)
	require.Equal(t, "value1", value)

	// Test state copy
	stateCopy := state.Copy()
	require.Equal(t, "value1", stateCopy["key1"])

	// Test state deletion
	err = state.Delete("key1")
	require.NoError(t, err)

	_, exists = state.Get("key1")
	require.False(t, exists)

	// Test keys
	err = state.Set("key2", "value2")
	require.NoError(t, err)
	err = state.Set("key3", "value3")
	require.NoError(t, err)

	keys := state.Keys()
	require.Len(t, keys, 2)
	require.Contains(t, keys, "key2")
	require.Contains(t, keys, "key3")
}

func TestRisorStateObject(t *testing.T) {
	// Create a null event store for testing
	eventStore := NewNullEventStore()

	// Create execution recorder
	recorder := NewBufferedExecutionRecorder("test-exec", eventStore, 5)

	// Create workflow state
	state := NewWorkflowState("test-exec", recorder)
	risorState := NewRisorStateObject(state)

	// Test Risor state operations
	err := risorState.Set("key1", "value1")
	require.NoError(t, err)

	value := risorState.Get("key1")
	require.Equal(t, "value1", value)

	has := risorState.Has("key1")
	require.True(t, has)

	has = risorState.Has("nonexistent")
	require.False(t, has)

	err = risorState.Delete("key1")
	require.NoError(t, err)

	has = risorState.Has("key1")
	require.False(t, has)
}

func TestOperationDeterministicID(t *testing.T) {
	// Create two identical operations
	op1 := NewOperation(
		"test_type",
		"test_step",
		"test_path",
		map[string]interface{}{
			"param1": "value1",
			"param2": 42,
		},
	)

	op2 := NewOperation(
		"test_type",
		"test_step",
		"test_path",
		map[string]interface{}{
			"param1": "value1",
			"param2": 42,
		},
	)

	// IDs should be identical for identical operations
	require.Equal(t, op1.ID, op2.ID)

	// Create a different operation
	op3 := NewOperation(
		"test_type",
		"test_step",
		"test_path",
		map[string]interface{}{
			"param1": "different_value",
			"param2": 42,
		},
	)

	// ID should be different
	require.NotEqual(t, op1.ID, op3.ID)
}
