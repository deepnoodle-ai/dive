package environment

import (
	"fmt"
	"testing"
	"time"
)

// TestTypeSafetyBenefits demonstrates the benefits of typed events
func TestTypeSafetyBenefits(t *testing.T) {
	eventStore := &mockEventStore{events: []*ExecutionEvent{}}
	recorder := NewBufferedExecutionRecorder("exec-456", eventStore, 1)

	// Type safety: this won't compile if fields are wrong
	operationData := &OperationCompletedData{
		OperationID:   "op-123",
		OperationType: "agent_response",
		Duration:      time.Second * 3,
		Result:        map[string]interface{}{"response": "Success"},
	}

	// Validation: this will catch errors at runtime
	if err := operationData.Validate(); err != nil {
		t.Fatalf("Validation error: %v", err)
	}

	recorder.RecordEvent("path-1", "", operationData)

	// Reading events back with type safety
	recorder.Flush()
	events := eventStore.events

	for _, event := range events {
		typedData, err := event.GetTypedData()
		if err != nil {
			t.Fatalf("Error getting typed data: %v", err)
		}

		// Type-safe access to event data
		switch data := typedData.(type) {
		case *OperationCompletedData:
			t.Logf("Operation %s completed in %v", data.OperationID, data.Duration)
			if data.OperationID != "op-123" {
				t.Errorf("Expected operation ID op-123, got %s", data.OperationID)
			}
		case *StepStartedData:
			t.Logf("Step started with type: %s", data.StepType)
		case *PathBranchedData:
			t.Logf("Path branched into %d new paths", len(data.NewPaths))
		default:
			t.Logf("Event type: %T", data)
		}
	}
}

// TestBackwardCompatibility shows how the system maintains compatibility
func TestBackwardCompatibility(t *testing.T) {
	eventStore := &mockEventStore{events: []*ExecutionEvent{}}
	recorder := NewBufferedExecutionRecorder("exec-789", eventStore, 1)

	// Record an event using convenience method
	recorder.RecordEvent("path-1", "step-1", &ExecutionStartedData{
		WorkflowName: "test-workflow",
		Inputs:       map[string]interface{}{"query": "Hello"},
	})

	recorder.Flush()

	// Read it back with the new typed system
	event := eventStore.events[0]

	// Legacy access still works
	workflowName := event.Data["workflow_name"].(string)
	if workflowName != "test-workflow" {
		t.Errorf("Expected workflow name 'test-workflow', got '%s'", workflowName)
	}
	t.Logf("Legacy access - Workflow: %s", workflowName)

	// New typed access also works (converts automatically)
	typedData, err := event.GetTypedData()
	if err != nil {
		t.Fatalf("Error: %v", err)
	}

	if executionData, ok := typedData.(*ExecutionStartedData); ok {
		if executionData.WorkflowName != "test-workflow" {
			t.Errorf("Expected workflow name 'test-workflow', got '%s'", executionData.WorkflowName)
		}
		t.Logf("Typed access - Workflow: %s", executionData.WorkflowName)
		t.Logf("Typed access - Inputs: %v", executionData.Inputs)
	} else {
		t.Errorf("Expected ExecutionStartedData, got %T", typedData)
	}
}

// TestCustomEventData shows how to create custom typed event data
func TestCustomEventData(t *testing.T) {
	// Define custom event data for a hypothetical new event type
	type CustomAnalysisData struct {
		AnalysisType string                 `json:"analysis_type"`
		Results      map[string]interface{} `json:"results"`
		Confidence   float64                `json:"confidence"`
	}

	// Implement the ExecutionEventData interface
	eventTypeFunc := func(d *CustomAnalysisData) ExecutionEventType {
		return "custom_analysis" // You would define this constant
	}

	validateFunc := func(d *CustomAnalysisData) error {
		if d.AnalysisType == "" {
			return fmt.Errorf("analysis_type is required")
		}
		if d.Confidence < 0 || d.Confidence > 1 {
			return fmt.Errorf("confidence must be between 0 and 1")
		}
		return nil
	}

	// Example usage:
	customData := &CustomAnalysisData{
		AnalysisType: "sentiment",
		Results:      map[string]interface{}{"sentiment": "positive", "score": 0.85},
		Confidence:   0.92,
	}

	// Demonstrate the validation
	if err := validateFunc(customData); err != nil {
		t.Errorf("Validation error: %v", err)
	} else {
		t.Logf("Custom event data validation passed for %s analysis", customData.AnalysisType)
	}

	// Show the event type
	eventType := eventTypeFunc(customData)
	if eventType != "custom_analysis" {
		t.Errorf("Expected event type 'custom_analysis', got '%s'", eventType)
	}
	t.Logf("Event type: %s", eventType)
}

// TestValidationBenefits demonstrates automatic validation
func TestValidationBenefits(t *testing.T) {
	// This would fail validation
	invalidData := &OperationStartedData{
		// Missing required OperationID and OperationType
		Parameters: map[string]interface{}{"param": "value"},
	}

	if err := invalidData.Validate(); err != nil {
		t.Logf("Validation caught error: %v", err)
	} else {
		t.Error("Expected validation to fail for invalid data")
	}

	// This would pass validation
	validData := &OperationStartedData{
		OperationID:   "op-123",
		OperationType: "file_read",
		Parameters:    map[string]interface{}{"file": "config.yaml"},
	}

	if err := validData.Validate(); err != nil {
		t.Errorf("Unexpected validation error: %v", err)
	} else {
		t.Log("Validation passed for valid data")
	}
}
