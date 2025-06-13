package environment

import (
	"context"
	"fmt"
	"time"
)

// ExecutionEventStore defines the interface for persisting execution events
type ExecutionEventStore interface {
	AppendEvents(ctx context.Context, events []*ExecutionEvent) error

	GetEvents(ctx context.Context, executionID string, fromSeq int64) ([]*ExecutionEvent, error)

	GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error)

	SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error

	GetSnapshot(ctx context.Context, executionID string) (*ExecutionSnapshot, error)

	ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*ExecutionSnapshot, error)

	DeleteExecution(ctx context.Context, executionID string) error

	CleanupCompletedExecutions(ctx context.Context, olderThan time.Time) error
}

// ExecutionFilter specifies criteria for querying executions
type ExecutionFilter struct {
	Status       string `json:"status,omitempty"`
	WorkflowName string `json:"workflow_name,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Offset       int    `json:"offset,omitempty"`
}

// Validate validates the execution filter
func (f *ExecutionFilter) Validate() error {
	if f.Limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	if f.Offset < 0 {
		return fmt.Errorf("offset cannot be negative")
	}
	return nil
}

// WorkflowVersion defines a version decision point in workflow evolution
type WorkflowVersion struct {
	ChangeID     string `json:"change_id"`     // Unique identifier for this version decision
	Version      int    `json:"version"`       // Selected version number
	MinVersion   int    `json:"min_version"`   // Minimum supported version
	MaxVersion   int    `json:"max_version"`   // Maximum available version
	ChangeReason string `json:"change_reason"` // Description of what changed
}

// WorkflowCompatibility represents compatibility between workflow versions
type WorkflowCompatibility struct {
	IsCompatible      bool     `json:"is_compatible"`
	IncompatibleSteps []string `json:"incompatible_steps,omitempty"`
	ChangedInputs     []string `json:"changed_inputs,omitempty"`
	ChangesSummary    string   `json:"changes_summary"`
}

// Helper functions for extracting data from event data maps
func getStringFromData(data map[string]interface{}, key string) string {
	if value, ok := data[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func getStringFromMap(data map[string]interface{}, key string) string {
	if value, ok := data[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}
