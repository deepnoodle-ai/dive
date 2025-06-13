package environment

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

func TestExecution_SQLiteIntegration(t *testing.T) {
	// Create a temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create SQLite event store
	eventStore, err := NewSQLiteExecutionEventStore(dbPath, DefaultSQLiteStoreOptions())
	require.NoError(t, err)
	defer eventStore.Close()

	// Create a simple mock agent
	mockAgent := &mockAgent{}

	// Create steps
	promptStep := workflow.NewStep(workflow.StepOptions{
		Name:   "greet",
		Type:   "prompt",
		Agent:  mockAgent,
		Prompt: "Say hello to ${name}",
	})

	actionStep := workflow.NewStep(workflow.StepOptions{
		Name:   "print",
		Type:   "action",
		Action: "Print",
		Parameters: map[string]any{
			"Message": "Executed step: ${greet}",
		},
	})

	// Create workflow
	wf, err := workflow.New(workflow.Options{
		Name:  "sqlite-test-workflow",
		Steps: []*workflow.Step{promptStep, actionStep},
	})
	require.NoError(t, err)

	// Create environment
	env, err := New(Options{
		Name:   "sqlite-test-env",
		Agents: []dive.Agent{mockAgent},
	})
	require.NoError(t, err)

	// Start environment
	err = env.Start(context.Background())
	require.NoError(t, err)
	defer env.Stop(context.Background())

	// Create execution with SQLite event store
	execution, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs: map[string]interface{}{
			"name": "SQLite",
		},
		EventStore: eventStore,
		Logger:     slogger.DefaultLogger,
		ReplayMode: false,
	})
	require.NoError(t, err)
	require.NotNil(t, execution)

	executionID := execution.ID()

	// Run execution
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)

	// Verify final state
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Verify events were persisted to SQLite
	events, err := eventStore.GetEventHistory(ctx, executionID)
	require.NoError(t, err)
	require.True(t, len(events) > 0, "Should have persisted events")

	// Check for execution started and completed events
	var hasStartEvent, hasCompleteEvent, hasOperationEvents bool
	for _, event := range events {
		switch event.EventType {
		case EventExecutionStarted:
			hasStartEvent = true
		case EventExecutionCompleted:
			hasCompleteEvent = true
		case EventOperationStarted, EventOperationCompleted:
			hasOperationEvents = true
		}
	}

	require.True(t, hasStartEvent, "Should have execution started event")
	require.True(t, hasCompleteEvent, "Should have execution completed event")
	require.True(t, hasOperationEvents, "Should have operation events")

	// Verify database file was created
	_, err = os.Stat(dbPath)
	require.NoError(t, err, "Database file should exist")
}

func TestExecution_SQLiteReplay(t *testing.T) {
	// Create a temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "replay_test.db")

	// Create SQLite event store
	eventStore, err := NewSQLiteExecutionEventStore(dbPath, DefaultSQLiteStoreOptions())
	require.NoError(t, err)
	defer eventStore.Close()

	// Create a simple mock agent
	mockAgent := &mockAgent{}

	// Create step
	promptStep := workflow.NewStep(workflow.StepOptions{
		Name:   "greet",
		Type:   "prompt",
		Agent:  mockAgent,
		Prompt: "Say hello",
	})

	// Create workflow
	wf, err := workflow.New(workflow.Options{
		Name:  "replay-test-workflow",
		Steps: []*workflow.Step{promptStep},
	})
	require.NoError(t, err)

	// Create environment
	env, err := New(Options{
		Name:   "replay-test-env",
		Agents: []dive.Agent{mockAgent},
	})
	require.NoError(t, err)

	// Start environment
	err = env.Start(context.Background())
	require.NoError(t, err)
	defer env.Stop(context.Background())

	// Create execution in normal mode
	execution, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  eventStore,
		Logger:      slogger.DefaultLogger,
		ReplayMode:  false,
	})
	require.NoError(t, err)

	// Execute an operation to create a result
	op := Operation{
		Type:     "test_operation",
		StepName: "test_step",
		PathID:   "test_path",
		Parameters: map[string]interface{}{
			"test": "value",
		},
	}

	result, err := execution.ExecuteOperation(context.Background(), op, func() (interface{}, error) {
		return "test_result_from_sqlite", nil
	})
	require.NoError(t, err)
	require.Equal(t, "test_result_from_sqlite", result)

	// Flush events to ensure they're persisted
	err = execution.recorder.Flush()
	require.NoError(t, err)

	// Verify operation result was cached
	cachedResult, found := execution.FindOperationResult(op.GenerateID())
	require.True(t, found)
	require.Equal(t, "test_result_from_sqlite", cachedResult.Result)
	require.NoError(t, cachedResult.Error)

	// Verify events were persisted
	events, err := eventStore.GetEventHistory(context.Background(), execution.ID())
	require.NoError(t, err)
	require.True(t, len(events) > 0, "Should have persisted events")

	// Check for operation events in persistence
	var hasOperationStarted, hasOperationCompleted bool
	for _, event := range events {
		switch event.EventType {
		case EventOperationStarted:
			hasOperationStarted = true
		case EventOperationCompleted:
			hasOperationCompleted = true
		}
	}
	require.True(t, hasOperationStarted, "Should have operation started event")
	require.True(t, hasOperationCompleted, "Should have operation completed event")
}
