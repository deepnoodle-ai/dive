package environment

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTypedEventData(t *testing.T) {
	t.Run("ExecutionStartedData", func(t *testing.T) {
		data := &ExecutionStartedData{
			WorkflowName: "test-workflow",
			Inputs:       map[string]interface{}{"key": "value"},
		}

		require.Equal(t, EventExecutionStarted, data.EventType())
		require.NoError(t, data.Validate())

		// Test validation
		emptyData := &ExecutionStartedData{}
		require.Error(t, emptyData.Validate())
	})

	t.Run("OperationCompletedData", func(t *testing.T) {
		data := &OperationCompletedData{
			OperationID:   "op-123",
			OperationType: "agent_response",
			Duration:      time.Second * 5,
			Result:        "test result",
		}

		require.Equal(t, EventOperationCompleted, data.EventType())
		require.NoError(t, data.Validate())

		// Test validation
		emptyData := &OperationCompletedData{}
		require.Error(t, emptyData.Validate())
	})

	t.Run("PathBranchedData", func(t *testing.T) {
		data := &PathBranchedData{
			NewPaths: []PathBranchInfo{
				{
					ID:             "path-1",
					CurrentStep:    "step-1",
					InheritOutputs: true,
				},
				{
					ID:             "path-2",
					CurrentStep:    "step-2",
					InheritOutputs: false,
				},
			},
		}

		require.Equal(t, EventPathBranched, data.EventType())
		require.NoError(t, data.Validate())

		// Test validation
		emptyData := &PathBranchedData{}
		require.Error(t, emptyData.Validate())

		invalidData := &PathBranchedData{
			NewPaths: []PathBranchInfo{
				{
					ID:          "", // invalid: empty ID
					CurrentStep: "step-1",
				},
			},
		}
		require.Error(t, invalidData.Validate())
	})
}

func TestExecutionEventTypedData(t *testing.T) {
	t.Run("SetTypedData", func(t *testing.T) {
		event := &ExecutionEvent{
			ID:          "event-123",
			ExecutionID: "exec-456",
			Sequence:    1,
			Timestamp:   time.Now(),
			EventType:   EventStepCompleted,
		}

		data := &StepCompletedData{
			Output:         "Step output",
			StoredVariable: "result_var",
		}

		err := event.SetTypedData(data)
		require.NoError(t, err)

		// Check that TypedData is set
		require.Equal(t, data, event.TypedData)

		// Check that legacy Data field is populated for backward compatibility
		require.Equal(t, "Step output", event.Data["output"])
		require.Equal(t, "result_var", event.Data["stored_variable"])
	})

	t.Run("SetTypedData validation", func(t *testing.T) {
		event := &ExecutionEvent{
			ID:          "event-123",
			ExecutionID: "exec-456",
			Sequence:    1,
			Timestamp:   time.Now(),
			EventType:   EventStepCompleted,
		}

		// Test nil data
		err := event.SetTypedData(nil)
		require.Error(t, err)

		// Test mismatched event type
		data := &ExecutionStartedData{
			WorkflowName: "test",
			Inputs:       map[string]interface{}{},
		}
		err = event.SetTypedData(data)
		require.Error(t, err)
		require.Contains(t, err.Error(), "does not match event type")

		// Test invalid data
		invalidData := &StepFailedData{
			Error: "", // required field is empty
		}
		err = event.SetTypedData(invalidData)
		require.Error(t, err)
		require.Contains(t, err.Error(), "validation failed")
	})

	t.Run("GetTypedData with TypedData set", func(t *testing.T) {
		event := &ExecutionEvent{
			ID:          "event-123",
			ExecutionID: "exec-456",
			Sequence:    1,
			Timestamp:   time.Now(),
			EventType:   EventOperationStarted,
		}

		originalData := &OperationStartedData{
			OperationID:   "op-123",
			OperationType: "test_operation",
			Parameters:    map[string]interface{}{"param1": "value1"},
		}

		err := event.SetTypedData(originalData)
		require.NoError(t, err)

		retrievedData, err := event.GetTypedData()
		require.NoError(t, err)
		require.Equal(t, originalData, retrievedData)
	})

	t.Run("GetTypedData legacy conversion", func(t *testing.T) {
		event := &ExecutionEvent{
			ID:          "event-123",
			ExecutionID: "exec-456",
			Sequence:    1,
			Timestamp:   time.Now(),
			EventType:   EventExecutionStarted,
			Data: map[string]interface{}{
				"workflow_name": "test-workflow",
				"inputs":        map[string]interface{}{"key": "value"},
			},
		}

		// TypedData is nil, should convert from legacy Data
		retrievedData, err := event.GetTypedData()
		require.NoError(t, err)
		require.NotNil(t, retrievedData)

		typedData, ok := retrievedData.(*ExecutionStartedData)
		require.True(t, ok)
		require.Equal(t, "test-workflow", typedData.WorkflowName)
		require.Equal(t, map[string]interface{}{"key": "value"}, typedData.Inputs)
	})
}

func TestBufferedExecutionRecorderTypedEvents(t *testing.T) {
	// Create a mock event store
	eventStore := &mockEventStore{events: []*ExecutionEvent{}}
	recorder := NewBufferedExecutionRecorder("exec-123", eventStore, 1)

	t.Run("RecordTypedEvent", func(t *testing.T) {
		data := &StepStartedData{
			StepType:   "prompt",
			StepParams: map[string]interface{}{"agent": "test-agent"},
		}

		recorder.RecordEvent("path-1", "step-1", data)

		// Flush to ensure event is written
		err := recorder.Flush()
		require.NoError(t, err)

		// Check that event was recorded
		require.Len(t, eventStore.events, 1)
		event := eventStore.events[0]

		require.Equal(t, "exec-123", event.ExecutionID)
		require.Equal(t, EventStepStarted, event.EventType)
		require.Equal(t, "path-1", event.Path)
		require.Equal(t, "step-1", event.Step)
		require.Equal(t, data, event.TypedData)

		// Check legacy Data field is populated
		require.Equal(t, "prompt", event.Data["step_type"])
		require.Equal(t, map[string]interface{}{"agent": "test-agent"}, event.Data["step_params"])
	})

	t.Run("Convenience methods", func(t *testing.T) {
		// Clear previous events
		eventStore.events = []*ExecutionEvent{}

		// Test convenience method
		recorder.RecordEvent("path-1", "step-1", &ExecutionStartedData{
			WorkflowName: "test-workflow",
			Inputs:       map[string]interface{}{"input": "test"},
		})

		err := recorder.Flush()
		require.NoError(t, err)

		require.Len(t, eventStore.events, 1)
		event := eventStore.events[0]

		require.Equal(t, EventExecutionStarted, event.EventType)

		typedData, ok := event.TypedData.(*ExecutionStartedData)
		require.True(t, ok)
		require.Equal(t, "test-workflow", typedData.WorkflowName)
		require.Equal(t, map[string]interface{}{"input": "test"}, typedData.Inputs)
	})
}

// Mock event store for testing
type mockEventStore struct {
	events []*ExecutionEvent
}

func (m *mockEventStore) AppendEvents(ctx context.Context, events []*ExecutionEvent) error {
	m.events = append(m.events, events...)
	return nil
}

func (m *mockEventStore) GetEvents(ctx context.Context, executionID string, fromSeq int64) ([]*ExecutionEvent, error) {
	return m.events, nil
}

func (m *mockEventStore) GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error) {
	return m.events, nil
}

func (m *mockEventStore) SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error {
	return nil
}

func (m *mockEventStore) GetSnapshot(ctx context.Context, executionID string) (*ExecutionSnapshot, error) {
	return nil, nil
}

func (m *mockEventStore) ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*ExecutionSnapshot, error) {
	return nil, nil
}

func (m *mockEventStore) DeleteExecution(ctx context.Context, executionID string) error {
	return nil
}

func (m *mockEventStore) CleanupCompletedExecutions(ctx context.Context, olderThan time.Time) error {
	return nil
}
