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

func TestExecution_BasicWorkflow(t *testing.T) {
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
		Name:  "test-workflow",
		Steps: []*workflow.Step{promptStep, actionStep},
	})
	require.NoError(t, err)

	// Create environment
	env, err := New(Options{
		Name:   "test-env",
		Agents: []dive.Agent{mockAgent},
	})
	require.NoError(t, err)

	// Start environment
	err = env.Start(context.Background())
	require.NoError(t, err)
	defer env.Stop(context.Background())

	// Create mock event store
	eventStore := &mockEventStore{}

	// Create execution
	execution, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs: map[string]interface{}{
			"name": "World",
		},
		EventStore: eventStore,
		Logger:     slogger.DefaultLogger,
		ReplayMode: false,
	})
	require.NoError(t, err)
	require.NotNil(t, execution)

	// Verify initial state
	require.Equal(t, ExecutionStatusPending, execution.Status())
	require.NotEmpty(t, execution.ID())

	// Run execution
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)

	// Verify final state
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Verify events were recorded
	require.True(t, len(eventStore.events) > 0)

	// Check for execution started and completed events
	var hasStartEvent, hasCompleteEvent bool
	for _, event := range eventStore.events {
		switch event.EventType {
		case EventExecutionStarted:
			hasStartEvent = true
		case EventExecutionCompleted:
			hasCompleteEvent = true
		}
	}
	require.True(t, hasStartEvent, "Should have execution started event")
	require.True(t, hasCompleteEvent, "Should have execution completed event")
}

func TestExecution_OperationReplay(t *testing.T) {
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
		Name:  "test-workflow",
		Steps: []*workflow.Step{promptStep},
	})
	require.NoError(t, err)

	// Create environment
	env, err := New(Options{
		Name:   "test-env",
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
		EventStore:  &mockEventStore{},
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
		return "test_result", nil
	})
	require.NoError(t, err)
	require.Equal(t, "test_result", result)

	// Verify operation result was cached
	cachedResult, found := execution.FindOperationResult(op.GenerateID())
	require.True(t, found)
	require.Equal(t, "test_result", cachedResult.Result)
	require.NoError(t, cachedResult.Error)
}

// mockEventStore implements ExecutionEventStore for testing
type mockEventStore struct {
	events    []*ExecutionEvent
	snapshots map[string]*ExecutionSnapshot
}

func (m *mockEventStore) AppendEvents(ctx context.Context, events []*ExecutionEvent) error {
	m.events = append(m.events, events...)
	return nil
}

func (m *mockEventStore) GetEvents(ctx context.Context, executionID string, fromSeq int64) ([]*ExecutionEvent, error) {
	var result []*ExecutionEvent
	for _, event := range m.events {
		if event.ExecutionID == executionID && event.Sequence >= fromSeq {
			result = append(result, event)
		}
	}
	return result, nil
}

func (m *mockEventStore) GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error) {
	return m.GetEvents(ctx, executionID, 0)
}

func (m *mockEventStore) SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error {
	if m.snapshots == nil {
		m.snapshots = make(map[string]*ExecutionSnapshot)
	}
	m.snapshots[snapshot.ID] = snapshot
	return nil
}

func (m *mockEventStore) GetSnapshot(ctx context.Context, executionID string) (*ExecutionSnapshot, error) {
	if m.snapshots == nil {
		return nil, nil
	}
	return m.snapshots[executionID], nil
}

func (m *mockEventStore) ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*ExecutionSnapshot, error) {
	var result []*ExecutionSnapshot
	for _, snapshot := range m.snapshots {
		result = append(result, snapshot)
	}
	return result, nil
}

func (m *mockEventStore) DeleteExecution(ctx context.Context, executionID string) error {
	if m.snapshots != nil {
		delete(m.snapshots, executionID)
	}
	return nil
}

func (m *mockEventStore) CleanupCompletedExecutions(ctx context.Context, olderThan time.Time) error {
	return nil
}
