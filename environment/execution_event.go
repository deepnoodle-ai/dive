package environment

import (
	"fmt"
	"log"
	"time"

	"github.com/diveagents/dive/llm"
	"go.jetify.com/typeid"
)

// NewExecutionID creates a new execution id
func NewExecutionID() string {
	value, err := typeid.WithPrefix("exec")
	if err != nil {
		log.Fatalf("error creating new id: %v", err)
	}
	return value.String()
}

// NewEventID creates a new event id
func NewEventID() string {
	value, err := typeid.WithPrefix("event")
	if err != nil {
		log.Fatalf("error creating new id: %v", err)
	}
	return value.String()
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

	// Operation events
	EventOperationStarted   ExecutionEventType = "operation_started"
	EventOperationCompleted ExecutionEventType = "operation_completed"
	EventOperationFailed    ExecutionEventType = "operation_failed"

	// State events
	EventStateMutated ExecutionEventType = "state_mutated"

	// Deterministic access events
	EventTimeAccessed    ExecutionEventType = "time_accessed"
	EventRandomGenerated ExecutionEventType = "random_generated"

	// Script state management events (removed unused events)
	EventIterationStarted   ExecutionEventType = "iteration_started"
	EventIterationCompleted ExecutionEventType = "iteration_completed"
)

// ExecutionEventData represents the interface that all typed event data must implement
type ExecutionEventData interface {
	EventType() ExecutionEventType
	Validate() error
}

// ExecutionStartedData contains data for execution started events
type ExecutionStartedData struct {
	WorkflowName string                 `json:"workflow_name"`
	Inputs       map[string]interface{} `json:"inputs"`
}

func (d *ExecutionStartedData) EventType() ExecutionEventType { return EventExecutionStarted }
func (d *ExecutionStartedData) Validate() error {
	if d.WorkflowName == "" {
		return fmt.Errorf("workflow_name is required")
	}
	return nil
}

// ExecutionCompletedData contains data for execution completed events
type ExecutionCompletedData struct {
	Outputs map[string]interface{} `json:"outputs"`
	Usage   *llm.Usage             `json:"usage,omitempty"`
}

func (d *ExecutionCompletedData) EventType() ExecutionEventType { return EventExecutionCompleted }
func (d *ExecutionCompletedData) Validate() error               { return nil }

// ExecutionFailedData contains data for execution failed events
type ExecutionFailedData struct {
	Error string `json:"error"`
}

func (d *ExecutionFailedData) EventType() ExecutionEventType { return EventExecutionFailed }
func (d *ExecutionFailedData) Validate() error {
	if d.Error == "" {
		return fmt.Errorf("error is required")
	}
	return nil
}

// PathStartedData contains data for path started events
type PathStartedData struct {
	CurrentStep string `json:"current_step"`
}

func (d *PathStartedData) EventType() ExecutionEventType { return EventPathStarted }
func (d *PathStartedData) Validate() error {
	if d.CurrentStep == "" {
		return fmt.Errorf("current_step is required")
	}
	return nil
}

// PathCompletedData contains data for path completed events
type PathCompletedData struct {
	FinalStep string `json:"final_step"`
}

func (d *PathCompletedData) EventType() ExecutionEventType { return EventPathCompleted }
func (d *PathCompletedData) Validate() error {
	if d.FinalStep == "" {
		return fmt.Errorf("final_step is required")
	}
	return nil
}

// PathFailedData contains data for path failed events
type PathFailedData struct {
	Error string `json:"error"`
}

func (d *PathFailedData) EventType() ExecutionEventType { return EventPathFailed }
func (d *PathFailedData) Validate() error {
	if d.Error == "" {
		return fmt.Errorf("error is required")
	}
	return nil
}

// PathBranchedData contains data for path branched events
type PathBranchedData struct {
	NewPaths []PathBranchInfo `json:"new_paths"`
}

func (d *PathBranchedData) EventType() ExecutionEventType { return EventPathBranched }
func (d *PathBranchedData) Validate() error {
	if len(d.NewPaths) == 0 {
		return fmt.Errorf("new_paths is required and must not be empty")
	}
	for i, path := range d.NewPaths {
		if path.ID == "" {
			return fmt.Errorf("new_paths[%d].id is required", i)
		}
		if path.CurrentStep == "" {
			return fmt.Errorf("new_paths[%d].current_step is required", i)
		}
	}
	return nil
}

// StepStartedData contains data for step started events
type StepStartedData struct {
	StepType   string                 `json:"step_type"`
	StepParams map[string]interface{} `json:"step_params"`
}

func (d *StepStartedData) EventType() ExecutionEventType { return EventStepStarted }
func (d *StepStartedData) Validate() error {
	if d.StepType == "" {
		return fmt.Errorf("step_type is required")
	}
	return nil
}

// StepCompletedData contains data for step completed events
type StepCompletedData struct {
	Output         string     `json:"output"`
	StoredVariable string     `json:"stored_variable,omitempty"`
	Usage          *llm.Usage `json:"usage,omitempty"`
}

func (d *StepCompletedData) EventType() ExecutionEventType { return EventStepCompleted }
func (d *StepCompletedData) Validate() error               { return nil }

// StepFailedData contains data for step failed events
type StepFailedData struct {
	Error string `json:"error"`
}

func (d *StepFailedData) EventType() ExecutionEventType { return EventStepFailed }
func (d *StepFailedData) Validate() error {
	if d.Error == "" {
		return fmt.Errorf("error is required")
	}
	return nil
}

// OperationStartedData contains data for operation started events
type OperationStartedData struct {
	OperationID   string                 `json:"operation_id"`
	OperationType string                 `json:"operation_type"`
	Parameters    map[string]interface{} `json:"parameters"`
}

func (d *OperationStartedData) EventType() ExecutionEventType { return EventOperationStarted }
func (d *OperationStartedData) Validate() error {
	if d.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if d.OperationType == "" {
		return fmt.Errorf("operation_type is required")
	}
	return nil
}

// OperationCompletedData contains data for operation completed events
type OperationCompletedData struct {
	OperationID   string        `json:"operation_id"`
	OperationType string        `json:"operation_type"`
	Duration      time.Duration `json:"duration"`
	Result        interface{}   `json:"result"`
}

func (d *OperationCompletedData) EventType() ExecutionEventType { return EventOperationCompleted }
func (d *OperationCompletedData) Validate() error {
	if d.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if d.OperationType == "" {
		return fmt.Errorf("operation_type is required")
	}
	return nil
}

// OperationFailedData contains data for operation failed events
type OperationFailedData struct {
	OperationID   string        `json:"operation_id"`
	OperationType string        `json:"operation_type"`
	Duration      time.Duration `json:"duration"`
	Error         string        `json:"error"`
}

func (d *OperationFailedData) EventType() ExecutionEventType { return EventOperationFailed }
func (d *OperationFailedData) Validate() error {
	if d.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if d.OperationType == "" {
		return fmt.Errorf("operation_type is required")
	}
	if d.Error == "" {
		return fmt.Errorf("error is required")
	}
	return nil
}

// StateMutatedData contains data for state mutated events
type StateMutatedData struct {
	Mutations []StateMutation `json:"mutations"`
}

func (d *StateMutatedData) EventType() ExecutionEventType { return EventStateMutated }
func (d *StateMutatedData) Validate() error {
	if len(d.Mutations) == 0 {
		return fmt.Errorf("mutations is required and must not be empty")
	}
	for i, mutation := range d.Mutations {
		if mutation.Type == "" {
			return fmt.Errorf("mutations[%d].type is required", i)
		}
		if mutation.Type == StateMutationTypeSet && mutation.Key == "" {
			return fmt.Errorf("mutations[%d].key is required for set mutation", i)
		}
		if mutation.Type == StateMutationTypeDelete && mutation.Key == "" {
			return fmt.Errorf("mutations[%d].key is required for delete mutation", i)
		}
	}
	return nil
}

// TimeAccessedData contains data for time accessed events
type TimeAccessedData struct {
	AccessedAt time.Time `json:"accessed_at"`
	Value      time.Time `json:"value"`
}

func (d *TimeAccessedData) EventType() ExecutionEventType { return EventTimeAccessed }
func (d *TimeAccessedData) Validate() error {
	if d.AccessedAt.IsZero() {
		return fmt.Errorf("accessed_at is required")
	}
	if d.Value.IsZero() {
		return fmt.Errorf("value is required")
	}
	return nil
}

// RandomGeneratedData contains data for random generated events
type RandomGeneratedData struct {
	Seed   int64       `json:"seed"`
	Value  interface{} `json:"value"`
	Method string      `json:"method"` // "int", "float", "string", etc.
}

func (d *RandomGeneratedData) EventType() ExecutionEventType { return EventRandomGenerated }
func (d *RandomGeneratedData) Validate() error {
	if d.Method == "" {
		return fmt.Errorf("method is required")
	}
	if d.Value == nil {
		return fmt.Errorf("value is required")
	}
	return nil
}

// IterationStartedData contains data for iteration started events
type IterationStartedData struct {
	IterationIndex int         `json:"iteration_index"`
	Item           interface{} `json:"item"`
	ItemKey        string      `json:"item_key,omitempty"`
}

func (d *IterationStartedData) EventType() ExecutionEventType { return EventIterationStarted }
func (d *IterationStartedData) Validate() error {
	if d.IterationIndex < 0 {
		return fmt.Errorf("iteration_index must be non-negative")
	}
	return nil
}

// IterationCompletedData contains data for iteration completed events
type IterationCompletedData struct {
	IterationIndex int         `json:"iteration_index"`
	Item           interface{} `json:"item"`
	ItemKey        string      `json:"item_key,omitempty"`
	Result         interface{} `json:"result"`
}

func (d *IterationCompletedData) EventType() ExecutionEventType { return EventIterationCompleted }
func (d *IterationCompletedData) Validate() error {
	if d.IterationIndex < 0 {
		return fmt.Errorf("iteration_index must be non-negative")
	}
	return nil
}

// SignalReceivedData contains data for signal received events
type SignalReceivedData struct {
	SignalType string                 `json:"signal_type"` // Type of signal received (e.g., "interrupt", "pause", "resume")
	Payload    map[string]interface{} `json:"payload"`     // Signal-specific data
	Source     string                 `json:"source"`      // Source of the signal
}

func (d *SignalReceivedData) EventType() ExecutionEventType { return EventSignalReceived }
func (d *SignalReceivedData) Validate() error {
	if d.SignalType == "" {
		return fmt.Errorf("signal_type is required")
	}
	return nil
}

// VersionDecisionData contains data for version decision events
type VersionDecisionData struct {
	DecisionType      string   `json:"decision_type"`      // Type of version decision (e.g., "workflow_version", "agent_version")
	SelectedVersion   string   `json:"selected_version"`   // Version that was selected
	AvailableVersions []string `json:"available_versions"` // Versions that were available for selection
	Reason            string   `json:"reason"`             // Reason for the version selection
}

func (d *VersionDecisionData) EventType() ExecutionEventType { return EventVersionDecision }
func (d *VersionDecisionData) Validate() error {
	if d.DecisionType == "" {
		return fmt.Errorf("decision_type is required")
	}
	if d.SelectedVersion == "" {
		return fmt.Errorf("selected_version is required")
	}
	return nil
}

// ExecutionContinueAsNewData contains data for execution continue as new events
type ExecutionContinueAsNewData struct {
	NewExecutionID string                 `json:"new_execution_id"` // ID of the new execution
	NewInputs      map[string]interface{} `json:"new_inputs"`       // Inputs for the new execution
	Reason         string                 `json:"reason"`           // Reason for continuing as new
	StateTransfer  map[string]interface{} `json:"state_transfer"`   // State data transferred to new execution
}

func (d *ExecutionContinueAsNewData) EventType() ExecutionEventType {
	return EventExecutionContinueAsNew
}
func (d *ExecutionContinueAsNewData) Validate() error {
	if d.NewExecutionID == "" {
		return fmt.Errorf("new_execution_id is required")
	}
	if d.Reason == "" {
		return fmt.Errorf("reason is required")
	}
	return nil
}

// ExecutionEvent represents a single event in the execution history
type ExecutionEvent struct {
	ID          string             `json:"id"`
	ExecutionID string             `json:"execution_id"`
	Sequence    int64              `json:"sequence"`
	Timestamp   time.Time          `json:"timestamp"`
	EventType   ExecutionEventType `json:"event_type"`
	Path        string             `json:"path,omitempty"`
	Step        string             `json:"step,omitempty"`

	// Typed event data
	Data ExecutionEventData `json:"data"`
}

// GetData returns the typed event data
func (e *ExecutionEvent) GetData() ExecutionEventData {
	return e.Data
}

// SetData sets the typed event data
func (e *ExecutionEvent) SetData(data ExecutionEventData) error {
	if data == nil {
		return fmt.Errorf("data cannot be nil")
	}

	if err := data.Validate(); err != nil {
		return fmt.Errorf("data validation failed: %w", err)
	}

	if data.EventType() != e.EventType {
		return fmt.Errorf("data event type %s does not match event type %s", data.EventType(), e.EventType)
	}

	e.Data = data
	return nil
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

	// Validate typed data
	if e.Data == nil {
		return fmt.Errorf("data is required")
	}

	if err := e.Data.Validate(); err != nil {
		return fmt.Errorf("data validation failed: %w", err)
	}

	if e.Data.EventType() != e.EventType {
		return fmt.Errorf("data event type %s does not match event type %s", e.Data.EventType(), e.EventType)
	}

	return nil
}
