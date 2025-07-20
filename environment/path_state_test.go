package environment

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestPathStateJSONSerialization tests that PathState can be properly serialized and deserialized
func TestPathStateJSONSerialization(t *testing.T) {
	// Create a PathState with all fields populated
	originalPath := &PathState{
		ID:           "test-path-1",
		Status:       PathStatusRunning,
		CurrentStep:  "process_data",
		StartTime:    time.Now(),
		EndTime:      time.Now().Add(time.Hour),
		ErrorMessage: "test error message",
		StepOutputs: map[string]string{
			"step1": "output1",
			"step2": "output2",
		},
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(originalPath)
	require.NoError(t, err)
	require.NotEmpty(t, jsonData)

	// Deserialize from JSON
	var deserializedPath PathState
	err = json.Unmarshal(jsonData, &deserializedPath)
	require.NoError(t, err)

	// Verify all fields are correctly preserved
	require.Equal(t, originalPath.ID, deserializedPath.ID)
	require.Equal(t, originalPath.Status, deserializedPath.Status)
	require.Equal(t, originalPath.CurrentStep, deserializedPath.CurrentStep)
	require.Equal(t, originalPath.StartTime.Unix(), deserializedPath.StartTime.Unix()) // Compare Unix timestamps
	require.Equal(t, originalPath.EndTime.Unix(), deserializedPath.EndTime.Unix())
	require.Equal(t, originalPath.ErrorMessage, deserializedPath.ErrorMessage)
	require.Equal(t, originalPath.StepOutputs, deserializedPath.StepOutputs)
}

// TestPathStateCopy tests that the Copy method works correctly
func TestPathStateCopy(t *testing.T) {
	original := &PathState{
		ID:           "test-path-1",
		Status:       PathStatusRunning,
		CurrentStep:  "process_data",
		StartTime:    time.Now(),
		EndTime:      time.Now().Add(time.Hour),
		ErrorMessage: "test error message",
		StepOutputs: map[string]string{
			"step1": "output1",
			"step2": "output2",
		},
	}

	// Create a copy
	copy := original.Copy()

	// Verify all fields are copied
	require.Equal(t, original.ID, copy.ID)
	require.Equal(t, original.Status, copy.Status)
	require.Equal(t, original.CurrentStep, copy.CurrentStep)
	require.Equal(t, original.StartTime, copy.StartTime)
	require.Equal(t, original.EndTime, copy.EndTime)
	require.Equal(t, original.ErrorMessage, copy.ErrorMessage)
	require.Equal(t, original.StepOutputs, copy.StepOutputs)

	// Verify it's a deep copy for maps
	copy.StepOutputs["step1"] = "modified_output1"
	require.Equal(t, "output1", original.StepOutputs["step1"])      // Original should be unchanged
	require.Equal(t, "modified_output1", copy.StepOutputs["step1"]) // Copy should be modified
}

// TestPathStateJSONOmitEmpty tests that omitempty fields are properly handled
func TestPathStateJSONOmitEmpty(t *testing.T) {
	// Test with minimal PathState (no ErrorMessage)
	minimalPath := &PathState{
		ID:          "minimal-path",
		Status:      PathStatusPending,
		CurrentStep: "start",
		StartTime:   time.Now(),
		StepOutputs: make(map[string]string),
	}

	jsonData, err := json.Marshal(minimalPath)
	require.NoError(t, err)

	// Verify JSON doesn't contain omitted fields when they're empty
	jsonStr := string(jsonData)
	require.NotContains(t, jsonStr, "error_message")

	// Verify required fields are present
	require.Contains(t, jsonStr, "id")
	require.Contains(t, jsonStr, "status")
	require.Contains(t, jsonStr, "current_step")
	require.Contains(t, jsonStr, "start_time")
	require.Contains(t, jsonStr, "step_outputs")

	// Note: end_time will be present with zero value - this is expected behavior for time.Time with omitempty
}
