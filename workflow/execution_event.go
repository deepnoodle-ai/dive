package workflow

import (
	"fmt"
	"log"
	"time"

	"go.jetify.com/typeid"
)

// ExecutionEventType represents the type of execution event
type ExecutionEventType string

const (
	EventExecutionStarted       ExecutionEventType = "execution_started"
	EventPathStarted            ExecutionEventType = "path_started"
	EventStepStarted            ExecutionEventType = "step_started"
	EventStepCompleted          ExecutionEventType = "step_completed"
	EventStepFailed             ExecutionEventType = "step_failed"
	EventPathCompleted          ExecutionEventType = "path_completed"
	EventPathFailed             ExecutionEventType = "path_failed"
	EventExecutionCompleted     ExecutionEventType = "execution_completed"
	EventExecutionFailed        ExecutionEventType = "execution_failed"
	EventPathBranched           ExecutionEventType = "path_branched"
	EventSignalReceived         ExecutionEventType = "signal_received"
	EventVersionDecision        ExecutionEventType = "version_decision"
	EventExecutionContinueAsNew ExecutionEventType = "execution_continue_as_new"
)

// ExecutionEvent represents a single event in the execution history
type ExecutionEvent struct {
	ID          string                 `json:"id"`
	ExecutionID string                 `json:"execution_id"`
	PathID      string                 `json:"path_id"`
	Sequence    int64                  `json:"sequence"`
	Timestamp   time.Time              `json:"timestamp"`
	EventType   ExecutionEventType     `json:"event_type"`
	StepName    string                 `json:"step_name,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// Validate validates the execution event
func (e *ExecutionEvent) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("event id is required")
	}
	if e.ExecutionID == "" {
		return fmt.Errorf("execution id is required")
	}
	if e.Sequence <= 0 {
		return fmt.Errorf("sequence must be positive")
	}
	if e.EventType == "" {
		return fmt.Errorf("event type is required")
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	return nil
}

// NewEventID creates a new event id
func NewEventID() string {
	value, err := typeid.WithPrefix("event")
	if err != nil {
		log.Fatalf("error creating new id: %v", err)
	}
	return value.String()
}
