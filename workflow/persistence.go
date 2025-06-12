package workflow

import (
	"context"
	"fmt"
	"time"
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

// ExecutionSnapshot represents the complete state of an execution
type ExecutionSnapshot struct {
	ID           string    `json:"id"`
	WorkflowName string    `json:"workflow_name"`
	WorkflowHash string    `json:"workflow_hash"`
	InputsHash   string    `json:"inputs_hash"`
	Status       string    `json:"status"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastEventSeq int64     `json:"last_event_seq"`

	// Serialized data (for bootstrapping)
	WorkflowData []byte                 `json:"workflow_data"`
	Inputs       map[string]interface{} `json:"inputs"`
	Outputs      map[string]interface{} `json:"outputs"`
	Error        string                 `json:"error,omitempty"`
}

// ExecutionEventStore defines the interface for persisting execution events
type ExecutionEventStore interface {
	// Event operations
	AppendEvents(ctx context.Context, events []*ExecutionEvent) error
	GetEvents(ctx context.Context, executionID string, fromSeq int64) ([]*ExecutionEvent, error)
	GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error)

	// Snapshot operations
	SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error
	GetSnapshot(ctx context.Context, executionID string) (*ExecutionSnapshot, error)

	// Query operations
	ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*ExecutionSnapshot, error)
	DeleteExecution(ctx context.Context, executionID string) error

	// Cleanup operations
	CleanupCompletedExecutions(ctx context.Context, olderThan time.Time) error
}

// ExecutionFilter specifies criteria for querying executions
type ExecutionFilter struct {
	Status       *string `json:"status,omitempty"`
	WorkflowName *string `json:"workflow_name,omitempty"`
	Limit        int     `json:"limit,omitempty"`
	Offset       int     `json:"offset,omitempty"`
}

// RetryOptions configures how to retry a failed execution
type RetryOptions struct {
	Strategy  RetryStrategy          `json:"strategy"`
	NewInputs map[string]interface{} `json:"new_inputs,omitempty"`
}

// RetryStrategy defines different approaches to retrying executions
type RetryStrategy string

const (
	RetryFromStart     RetryStrategy = "from_start"      // Complete replay
	RetryFromFailure   RetryStrategy = "from_failure"    // Resume from failed step
	RetryWithNewInputs RetryStrategy = "with_new_inputs" // Replay with different inputs
	RetrySkipFailed    RetryStrategy = "skip_failed"     // Continue past failed steps
)

// ReplayResult contains the result of replaying an execution history
type ReplayResult struct {
	CompletedSteps map[string]string  `json:"completed_steps"` // step_name -> output
	ActivePaths    []*ReplayPathState `json:"active_paths"`    // currently running paths
	ScriptGlobals  map[string]any     `json:"script_globals"`  // variables in scope
	Status         string             `json:"status"`          // final status if execution complete
}

// ReplayPathState represents the state of a path during replay
type ReplayPathState struct {
	ID              string            `json:"id"`
	CurrentStepName string            `json:"current_step_name"`
	StepOutputs     map[string]string `json:"step_outputs"`
}

// Validate validates the execution event
func (e *ExecutionEvent) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("event ID is required")
	}
	if e.ExecutionID == "" {
		return fmt.Errorf("execution ID is required")
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

// ExecutionReplayer handles event history replay for state reconstruction
type ExecutionReplayer interface {
	ReplayExecution(ctx context.Context, events []*ExecutionEvent, workflow *Workflow) (*ReplayResult, error)
	ValidateEventHistory(ctx context.Context, events []*ExecutionEvent, workflow *Workflow) error
}

// BasicExecutionReplayer provides a basic implementation of ExecutionReplayer
type BasicExecutionReplayer struct {
	logger interface {
		Info(msg string, keysAndValues ...interface{})
		Error(msg string, keysAndValues ...interface{})
	}
}

// NewBasicExecutionReplayer creates a new basic execution replayer
func NewBasicExecutionReplayer(logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}) *BasicExecutionReplayer {
	return &BasicExecutionReplayer{
		logger: logger,
	}
}

// ReplayExecution replays an event history to reconstruct execution state
func (r *BasicExecutionReplayer) ReplayExecution(ctx context.Context, events []*ExecutionEvent, workflow *Workflow) (*ReplayResult, error) {
	result := &ReplayResult{
		CompletedSteps: make(map[string]string),
		ScriptGlobals:  make(map[string]any),
		ActivePaths:    make([]*ReplayPathState, 0),
		Status:         "running",
	}

	// Track active paths during replay
	activePaths := make(map[string]*ReplayPathState)

	r.logger.Info("starting replay", "event_count", len(events))

	// Replay events in sequence
	for i, event := range events {
		if err := r.replayEvent(ctx, event, result, activePaths); err != nil {
			r.logger.Error("replay failed", "event_sequence", event.Sequence, "error", err)
			return nil, fmt.Errorf("replay failed at event %d: %w", i, err)
		}
	}

	// Convert active paths map to slice
	for _, pathState := range activePaths {
		result.ActivePaths = append(result.ActivePaths, pathState)
	}

	r.logger.Info("replay completed",
		"completed_steps", len(result.CompletedSteps),
		"active_paths", len(result.ActivePaths),
		"status", result.Status)

	return result, nil
}

// replayEvent processes a single event during replay
func (r *BasicExecutionReplayer) replayEvent(ctx context.Context, event *ExecutionEvent, result *ReplayResult, activePaths map[string]*ReplayPathState) error {
	switch event.EventType {
	case EventExecutionStarted:
		result.Status = "running"
		r.logger.Info("execution started", "execution_id", event.ExecutionID)

	case EventPathStarted:
		pathState := &ReplayPathState{
			ID:              event.PathID,
			CurrentStepName: getStringFromData(event.Data, "current_step"),
			StepOutputs:     make(map[string]string),
		}
		activePaths[event.PathID] = pathState
		r.logger.Info("path started", "path_id", event.PathID, "step", pathState.CurrentStepName)

	case EventStepStarted:
		if pathState, exists := activePaths[event.PathID]; exists {
			pathState.CurrentStepName = event.StepName
		}
		r.logger.Info("step started", "path_id", event.PathID, "step", event.StepName)

	case EventStepCompleted:
		stepOutput := getStringFromData(event.Data, "output")
		result.CompletedSteps[event.StepName] = stepOutput

		// Update path state
		if pathState, exists := activePaths[event.PathID]; exists {
			pathState.StepOutputs[event.StepName] = stepOutput
		}

		// Handle stored variables
		if varName := getStringFromData(event.Data, "stored_variable"); varName != "" {
			result.ScriptGlobals[varName] = stepOutput
		}

		r.logger.Info("step completed", "path_id", event.PathID, "step", event.StepName)

	case EventStepFailed:
		errorMsg := getStringFromData(event.Data, "error")
		r.logger.Info("step failed", "path_id", event.PathID, "step", event.StepName, "error", errorMsg)

	case EventPathBranched:
		// Handle path branching - create new path states
		if newPathsData, ok := event.Data["new_paths"].([]interface{}); ok {
			for _, pathData := range newPathsData {
				if pathMap, ok := pathData.(map[string]interface{}); ok {
					pathID := getStringFromMap(pathMap, "id")
					currentStep := getStringFromMap(pathMap, "current_step")

					newPathState := &ReplayPathState{
						ID:              pathID,
						CurrentStepName: currentStep,
						StepOutputs:     make(map[string]string),
					}
					activePaths[pathID] = newPathState
				}
			}
		}
		r.logger.Info("path branched", "parent_path", event.PathID, "step", event.StepName)

	case EventPathCompleted:
		delete(activePaths, event.PathID)
		r.logger.Info("path completed", "path_id", event.PathID)

	case EventPathFailed:
		delete(activePaths, event.PathID)
		r.logger.Info("path failed", "path_id", event.PathID)

	case EventExecutionCompleted:
		result.Status = "completed"
		r.logger.Info("execution completed", "execution_id", event.ExecutionID)

	case EventExecutionFailed:
		result.Status = "failed"
		r.logger.Info("execution failed", "execution_id", event.ExecutionID)

	default:
		r.logger.Info("unknown event type", "event_type", event.EventType, "event_id", event.ID)
	}

	return nil
}

// ValidateEventHistory validates an event history for compatibility with a workflow
func (r *BasicExecutionReplayer) ValidateEventHistory(ctx context.Context, events []*ExecutionEvent, workflow *Workflow) error {
	// Build a map of steps in the current workflow
	workflowSteps := make(map[string]*Step)
	for _, step := range workflow.Steps() {
		workflowSteps[step.Name()] = step
	}

	// Validate each event
	for i, event := range events {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("event %d validation failed: %w", i, err)
		}

		// Check step-related events
		if event.StepName != "" {
			step, exists := workflowSteps[event.StepName]
			if !exists {
				return fmt.Errorf("step %s no longer exists in workflow (event %d)", event.StepName, i)
			}

			// Validate step type if recorded
			if expectedType := getStringFromData(event.Data, "step_type"); expectedType != "" {
				if step.Type() != expectedType {
					return fmt.Errorf("step %s changed type from %s to %s (event %d)",
						event.StepName, expectedType, step.Type(), i)
				}
			}
		}
	}

	r.logger.Info("event history validation passed", "event_count", len(events))
	return nil
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
