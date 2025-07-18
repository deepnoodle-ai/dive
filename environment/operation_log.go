package environment

import (
	"context"
	"time"

	"github.com/diveagents/dive/llm"
)

// OperationLogEntry represents a single operation log entry
type OperationLogEntry struct {
	ID            string                 `json:"id"`
	ExecutionID   string                 `json:"execution_id"`
	StepName      string                 `json:"step_name,omitempty"`
	PathID        string                 `json:"path_id,omitempty"`
	OperationType string                 `json:"operation_type"`
	Parameters    map[string]interface{} `json:"parameters,omitempty"`
	Result        interface{}            `json:"result,omitempty"`
	Error         string                 `json:"error,omitempty"`
	StartTime     time.Time              `json:"start_time"`
	Duration      time.Duration          `json:"duration"`
	Usage         *llm.Usage             `json:"usage,omitempty"`
}

// OperationLogger defines simple operation logging interface
type OperationLogger interface {
	// LogOperation logs a completed operation
	LogOperation(ctx context.Context, entry *OperationLogEntry) error

	// GetOperationHistory retrieves operation log for an execution
	GetOperationHistory(ctx context.Context, executionID string) ([]*OperationLogEntry, error)
}

// NullOperationLogger is a no-op implementation
type NullOperationLogger struct{}

func NewNullOperationLogger() *NullOperationLogger {
	return &NullOperationLogger{}
}

func (l *NullOperationLogger) LogOperation(ctx context.Context, entry *OperationLogEntry) error {
	return nil
}

func (l *NullOperationLogger) GetOperationHistory(ctx context.Context, executionID string) ([]*OperationLogEntry, error) {
	return nil, nil
}
