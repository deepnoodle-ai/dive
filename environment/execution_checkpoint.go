package environment

import (
	"context"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

// ExecutionCheckpoint represents a complete snapshot of execution state
type ExecutionCheckpoint struct {
	ID           string                 `json:"id"`
	ExecutionID  string                 `json:"execution_id"`
	WorkflowName string                 `json:"workflow_name"`
	Status       string                 `json:"status"`
	Inputs       map[string]interface{} `json:"inputs"`
	Outputs      map[string]interface{} `json:"outputs"`
	State        map[string]interface{} `json:"state"`        // Current workflow state variables
	PathStates   map[string]*PathState  `json:"path_states"`  // Current path states
	PathCounter  int                    `json:"path_counter"` // Counter for generating unique path IDs
	TotalUsage   *llm.Usage             `json:"total_usage,omitempty"`
	Error        string                 `json:"error,omitempty"`
	StartTime    time.Time              `json:"start_time"`
	EndTime      time.Time              `json:"end_time,omitempty"`
	CheckpointAt time.Time              `json:"checkpoint_at"`
}

// ExecutionCheckpointer defines simple checkpoint interface
type ExecutionCheckpointer interface {
	// SaveCheckpoint saves the current execution state
	SaveCheckpoint(ctx context.Context, checkpoint *ExecutionCheckpoint) error

	// LoadCheckpoint loads the latest checkpoint for an execution
	LoadCheckpoint(ctx context.Context, executionID string) (*ExecutionCheckpoint, error)

	// DeleteCheckpoint removes checkpoint data for an execution
	DeleteCheckpoint(ctx context.Context, executionID string) error
}

// NullCheckpointer is a no-op implementation
type NullCheckpointer struct{}

func NewNullCheckpointer() *NullCheckpointer {
	return &NullCheckpointer{}
}

func (c *NullCheckpointer) SaveCheckpoint(ctx context.Context, checkpoint *ExecutionCheckpoint) error {
	return nil
}

func (c *NullCheckpointer) LoadCheckpoint(ctx context.Context, executionID string) (*ExecutionCheckpoint, error) {
	return nil, nil
}

func (c *NullCheckpointer) DeleteCheckpoint(ctx context.Context, executionID string) error {
	return nil
}
