package environment

import (
	"context"
	"testing"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

func TestDeterministicRuntime_TimeAccess(t *testing.T) {
	// Create test execution with null event store
	eventStore := NewNullEventStore()
	env := setupTestEnvironment(t)

	execution, err := NewEventBasedExecution(env, ExecutionOptions{
		WorkflowName:   "test",
		Inputs:         map[string]interface{}{},
		Logger:         slogger.NewDevNullLogger(),
		EventStore:     eventStore,
		EventBatchSize: 10,
	})
	require.NoError(t, err)

	runtime := execution.deterministicRuntime

	// Test Now() returns different results for different calls (each gets unique operation ID)
	now1 := runtime.Now()
	now2 := runtime.Now()

	// Both calls should return different times (different operations)
	require.NotEqual(t, now1, now2)

	// Test Unix timestamp access (may be same if calls are very fast, just check validity)
	unix1 := runtime.Unix()
	unix2 := runtime.Unix()
	require.Greater(t, unix1, int64(0))
	require.Greater(t, unix2, int64(0))

	// Test UnixNano timestamp access (should be different due to nanosecond precision)
	unixNano1 := runtime.UnixNano()
	unixNano2 := runtime.UnixNano()
	require.Greater(t, unixNano1, int64(0))
	require.Greater(t, unixNano2, int64(0))
}

func TestDeterministicRuntime_RandomAccess(t *testing.T) {
	// Create test execution with null event store
	eventStore := NewNullEventStore()
	env := setupTestEnvironment(t)

	execution, err := NewEventBasedExecution(env, ExecutionOptions{
		WorkflowName:   "test",
		Inputs:         map[string]interface{}{},
		Logger:         slogger.NewDevNullLogger(),
		EventStore:     eventStore,
		EventBatchSize: 10,
	})
	require.NoError(t, err)

	runtime := execution.deterministicRuntime

	// Test Random() returns different results for different calls (each gets unique operation ID)
	rand1 := runtime.Random()
	rand2 := runtime.Random()

	// Both calls should return different values (different operations)
	require.NotEqual(t, rand1, rand2)
	require.GreaterOrEqual(t, rand1, 0.0)
	require.Less(t, rand1, 1.0)
	require.GreaterOrEqual(t, rand2, 0.0)
	require.Less(t, rand2, 1.0)

	// Test RandomInt with valid range
	randInt1 := runtime.RandomInt(1, 100)
	randInt2 := runtime.RandomInt(1, 100)

	require.NotEqual(t, randInt1, randInt2)
	require.GreaterOrEqual(t, randInt1, int64(1))
	require.Less(t, randInt1, int64(100))
	require.GreaterOrEqual(t, randInt2, int64(1))
	require.Less(t, randInt2, int64(100))

	// Test RandomString
	randStr1 := runtime.RandomString(10, "")
	randStr2 := runtime.RandomString(10, "")

	require.NotEqual(t, randStr1, randStr2)
	require.Len(t, randStr1, 10)
	require.Len(t, randStr2, 10)

	// Test RandomString with custom charset
	randStr3 := runtime.RandomString(5, "ABC")
	randStr4 := runtime.RandomString(5, "ABC")

	require.NotEqual(t, randStr3, randStr4)
	require.Len(t, randStr3, 5)
	require.Len(t, randStr4, 5)

	// All characters should be from the charset
	for _, ch := range randStr3 {
		require.Contains(t, "ABC", string(ch))
	}
	for _, ch := range randStr4 {
		require.Contains(t, "ABC", string(ch))
	}
}

func TestDeterministicRuntime_InvalidRandomRange(t *testing.T) {
	eventStore := NewNullEventStore()
	env := setupTestEnvironment(t)

	execution, err := NewEventBasedExecution(env, ExecutionOptions{
		WorkflowName:   "test",
		Inputs:         map[string]interface{}{},
		Logger:         slogger.NewDevNullLogger(),
		EventStore:     eventStore,
		EventBatchSize: 10,
	})
	require.NoError(t, err)

	runtime := execution.deterministicRuntime

	// Test invalid range should panic
	require.Panics(t, func() {
		runtime.RandomInt(100, 1) // min >= max
	})

	require.Panics(t, func() {
		runtime.RandomInt(10, 10) // min == max
	})
}

func TestDeterministicRuntime_Sleep(t *testing.T) {
	eventStore := NewNullEventStore()
	env := setupTestEnvironment(t)

	execution, err := NewEventBasedExecution(env, ExecutionOptions{
		WorkflowName:   "test",
		Inputs:         map[string]interface{}{},
		Logger:         slogger.NewDevNullLogger(),
		EventStore:     eventStore,
		EventBatchSize: 10,
	})
	require.NoError(t, err)

	runtime := execution.deterministicRuntime

	// Test Sleep doesn't panic and completes quickly
	// (During replay, this would be a no-op)
	start := time.Now()
	runtime.Sleep(1 * time.Millisecond)
	elapsed := time.Since(start)

	// Sleep should complete (either execute or replay)
	require.Less(t, elapsed, 100*time.Millisecond)
}

func TestDeterministicRuntime_OperationRecording(t *testing.T) {
	// Create an in-memory SQLite event store to capture events
	eventStore, err := NewSQLiteExecutionEventStore(":memory:", SQLiteStoreOptions{})
	require.NoError(t, err)

	env := setupTestEnvironment(t)

	execution, err := NewEventBasedExecution(env, ExecutionOptions{
		WorkflowName:   "test",
		Inputs:         map[string]interface{}{},
		Logger:         slogger.NewDevNullLogger(),
		EventStore:     eventStore,
		EventBatchSize: 1, // Flush immediately
	})
	require.NoError(t, err)

	runtime := execution.deterministicRuntime

	// Call deterministic functions
	_ = runtime.Now()
	_ = runtime.Random()
	_ = runtime.RandomInt(1, 10)

	// Flush events
	err = execution.Flush()
	require.NoError(t, err)

	// Check that operations were recorded
	events, err := eventStore.GetEventHistory(context.Background(), execution.ID())
	require.NoError(t, err)

	// Find operation events
	var operationEvents []*ExecutionEvent
	for _, event := range events {
		if event.EventType == EventOperationStarted ||
			event.EventType == EventOperationCompleted {
			operationEvents = append(operationEvents, event)
		}
	}

	// Should have recorded operations for time access and random generation
	require.Greater(t, len(operationEvents), 0)

	// Verify operation types
	var timeOp, randomOp, randomIntOp bool
	for _, event := range operationEvents {
		if event.EventType == EventOperationStarted {
			opType := event.Data["operation_type"].(string)
			switch opType {
			case "time_access":
				timeOp = true
			case "random_generation":
				// Could be either Random() or RandomInt()
				if params, ok := event.Data["parameters"].(map[string]interface{}); ok {
					if params["type"] == "float64" {
						randomOp = true
					} else if params["type"] == "int64" {
						randomIntOp = true
					}
				}
			}
		}
	}

	require.True(t, timeOp, "time access operation should be recorded")
	require.True(t, randomOp, "random float operation should be recorded")
	require.True(t, randomIntOp, "random int operation should be recorded")
}

func TestDeterministicRuntime_ScriptGlobals(t *testing.T) {
	eventStore := NewNullEventStore()
	env := setupTestEnvironment(t)

	execution, err := NewEventBasedExecution(env, ExecutionOptions{
		WorkflowName:   "test",
		Inputs:         map[string]interface{}{},
		Logger:         slogger.NewDevNullLogger(),
		EventStore:     eventStore,
		EventBatchSize: 10,
	})
	require.NoError(t, err)

	// Verify deterministic functions are available in script globals
	require.Contains(t, execution.scriptGlobals, "deterministicNow")
	require.Contains(t, execution.scriptGlobals, "deterministicRandom")
	require.Contains(t, execution.scriptGlobals, "deterministicRandomInt")
	require.Contains(t, execution.scriptGlobals, "deterministicRandomString")
	require.Contains(t, execution.scriptGlobals, "deterministicSleep")

	// Test that we can call the functions from script globals
	nowFunc := execution.scriptGlobals["deterministicNow"].(func() time.Time)
	now := nowFunc()
	require.False(t, now.IsZero())

	randomFunc := execution.scriptGlobals["deterministicRandom"].(func() float64)
	random := randomFunc()
	require.GreaterOrEqual(t, random, 0.0)
	require.Less(t, random, 1.0)
}

// Helper function to create a test environment with a simple workflow
func setupTestEnvironment(t *testing.T) *Environment {
	// Create a simple test workflow
	wf, err := workflow.New(workflow.Options{
		Name: "test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "start",
				Agent:  &mockAgent{},
				Prompt: "Hello world",
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

	err = env.Start(context.Background())
	require.NoError(t, err)

	return env
}
