package workflow

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExecutionEvent_Validation(t *testing.T) {
	tests := []struct {
		name      string
		event     ExecutionEvent
		wantError bool
	}{
		{
			name: "valid event",
			event: ExecutionEvent{
				ID:          "test-id",
				ExecutionID: "exec-123",
				Sequence:    1,
				Timestamp:   time.Now(),
				EventType:   EventExecutionStarted,
			},
			wantError: false,
		},
		{
			name: "missing ID",
			event: ExecutionEvent{
				ExecutionID: "exec-123",
				Sequence:    1,
				Timestamp:   time.Now(),
				EventType:   EventExecutionStarted,
			},
			wantError: true,
		},
		{
			name: "missing execution ID",
			event: ExecutionEvent{
				ID:        "test-id",
				Sequence:  1,
				Timestamp: time.Now(),
				EventType: EventExecutionStarted,
			},
			wantError: true,
		},
		{
			name: "invalid sequence",
			event: ExecutionEvent{
				ID:          "test-id",
				ExecutionID: "exec-123",
				Sequence:    0,
				Timestamp:   time.Now(),
				EventType:   EventExecutionStarted,
			},
			wantError: true,
		},
		{
			name: "missing timestamp",
			event: ExecutionEvent{
				ID:          "test-id",
				ExecutionID: "exec-123",
				Sequence:    1,
				EventType:   EventExecutionStarted,
			},
			wantError: true,
		},
		{
			name: "missing event type",
			event: ExecutionEvent{
				ID:          "test-id",
				ExecutionID: "exec-123",
				Sequence:    1,
				Timestamp:   time.Now(),
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestExecutionEvent_JSONSerialization(t *testing.T) {
	event := ExecutionEvent{
		ID:          "test-id-123",
		ExecutionID: "exec-456",
		PathID:      "path-789",
		Sequence:    42,
		Timestamp:   time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		EventType:   EventStepCompleted,
		StepName:    "test-step",
		Data: map[string]interface{}{
			"output": "test output",
			"count":  5,
		},
	}

	// Test serialization
	jsonData, err := json.Marshal(event)
	require.NoError(t, err)

	// Test deserialization
	var decoded ExecutionEvent
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	// Verify fields
	require.Equal(t, event.ID, decoded.ID)
	require.Equal(t, event.ExecutionID, decoded.ExecutionID)
	require.Equal(t, event.PathID, decoded.PathID)
	require.Equal(t, event.Sequence, decoded.Sequence)
	require.Equal(t, event.EventType, decoded.EventType)
	require.Equal(t, event.StepName, decoded.StepName)
	require.Equal(t, event.Data["output"], decoded.Data["output"])
	require.Equal(t, float64(5), decoded.Data["count"]) // JSON numbers become float64
}

func TestExecutionSnapshot_JSONSerialization(t *testing.T) {
	snapshot := ExecutionSnapshot{
		ID:           "snap-123",
		WorkflowName: "test-workflow",
		WorkflowHash: "hash-456",
		InputsHash:   "inputs-789",
		Status:       "completed",
		StartTime:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		EndTime:      time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		CreatedAt:    time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		LastEventSeq: 100,
		WorkflowData: []byte("workflow data"),
		Inputs: map[string]interface{}{
			"param1": "value1",
			"param2": 42,
		},
		Outputs: map[string]interface{}{
			"result": "success",
		},
	}

	// Test serialization
	jsonData, err := json.Marshal(snapshot)
	require.NoError(t, err)

	// Test deserialization
	var decoded ExecutionSnapshot
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	// Verify fields
	require.Equal(t, snapshot.ID, decoded.ID)
	require.Equal(t, snapshot.WorkflowName, decoded.WorkflowName)
	require.Equal(t, snapshot.Status, decoded.Status)
	require.Equal(t, snapshot.LastEventSeq, decoded.LastEventSeq)
	require.Equal(t, snapshot.Inputs["param1"], decoded.Inputs["param1"])
	require.Equal(t, float64(42), decoded.Inputs["param2"]) // JSON numbers become float64
}

func TestExecutionFilter_Validation(t *testing.T) {
	tests := []struct {
		name      string
		filter    ExecutionFilter
		wantError bool
	}{
		{
			name:      "valid filter",
			filter:    ExecutionFilter{Limit: 10, Offset: 0},
			wantError: false,
		},
		{
			name:      "negative limit",
			filter:    ExecutionFilter{Limit: -1, Offset: 0},
			wantError: true,
		},
		{
			name:      "negative offset",
			filter:    ExecutionFilter{Limit: 10, Offset: -1},
			wantError: true,
		},
		{
			name:      "zero values",
			filter:    ExecutionFilter{Limit: 0, Offset: 0},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEventTypes(t *testing.T) {
	// Test that all event type constants are defined
	eventTypes := []ExecutionEventType{
		EventExecutionStarted,
		EventPathStarted,
		EventStepStarted,
		EventStepCompleted,
		EventStepFailed,
		EventPathCompleted,
		EventPathFailed,
		EventExecutionCompleted,
		EventExecutionFailed,
		EventPathBranched,
		EventSignalReceived,
		EventVersionDecision,
		EventExecutionContinueAsNew,
	}

	for _, eventType := range eventTypes {
		require.NotEmpty(t, string(eventType), "Event type should not be empty")
	}
}
