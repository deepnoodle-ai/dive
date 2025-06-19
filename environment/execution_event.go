package environment

import (
	"fmt"
	"log"
	"time"

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

	// Script state management events
	EventVariableChanged    ExecutionEventType = "variable_changed"
	EventTemplateEvaluated  ExecutionEventType = "template_evaluated"
	EventConditionEvaluated ExecutionEventType = "condition_evaluated"
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
	Output         string `json:"output"`
	StoredVariable string `json:"stored_variable,omitempty"`
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

// VariableChangedData contains data for variable changed events
type VariableChangedData struct {
	VariableName string      `json:"variable_name"`
	OldValue     interface{} `json:"old_value"`
	NewValue     interface{} `json:"new_value"`
	Existed      bool        `json:"existed"`
}

func (d *VariableChangedData) EventType() ExecutionEventType { return EventVariableChanged }
func (d *VariableChangedData) Validate() error {
	if d.VariableName == "" {
		return fmt.Errorf("variable_name is required")
	}
	return nil
}

// TemplateEvaluatedData contains data for template evaluated events
type TemplateEvaluatedData struct {
	Template string `json:"template"`
	Result   string `json:"result"`
}

func (d *TemplateEvaluatedData) EventType() ExecutionEventType { return EventTemplateEvaluated }
func (d *TemplateEvaluatedData) Validate() error {
	if d.Template == "" {
		return fmt.Errorf("template is required")
	}
	return nil
}

// ConditionEvaluatedData contains data for condition evaluated events
type ConditionEvaluatedData struct {
	Condition string `json:"condition"`
	Result    bool   `json:"result"`
}

func (d *ConditionEvaluatedData) EventType() ExecutionEventType { return EventConditionEvaluated }
func (d *ConditionEvaluatedData) Validate() error {
	if d.Condition == "" {
		return fmt.Errorf("condition is required")
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

// ExecutionEvent represents a single event in the execution history
// This maintains backward compatibility while allowing for typed event data
type ExecutionEvent struct {
	ID          string             `json:"id"`
	ExecutionID string             `json:"execution_id"`
	Sequence    int64              `json:"sequence"`
	Timestamp   time.Time          `json:"timestamp"`
	EventType   ExecutionEventType `json:"event_type"`
	Path        string             `json:"path,omitempty"`
	Step        string             `json:"step,omitempty"`

	// Legacy field for backward compatibility
	Data map[string]interface{} `json:"data,omitempty"`

	// New typed data field
	TypedData ExecutionEventData `json:"typed_data,omitempty"`
}

// GetTypedData returns the typed event data, converting from legacy Data if needed
func (e *ExecutionEvent) GetTypedData() (ExecutionEventData, error) {
	if e.TypedData != nil {
		return e.TypedData, nil
	}

	// Convert from legacy Data map
	return e.convertLegacyData()
}

// convertLegacyData converts the legacy Data map to typed event data
func (e *ExecutionEvent) convertLegacyData() (ExecutionEventData, error) {
	if e.Data == nil {
		return nil, nil
	}

	switch e.EventType {
	case EventExecutionStarted:
		data := &ExecutionStartedData{}
		if workflowName, ok := e.Data["workflow_name"].(string); ok {
			data.WorkflowName = workflowName
		}
		if inputs, ok := e.Data["inputs"].(map[string]interface{}); ok {
			data.Inputs = inputs
		}
		return data, nil

	case EventExecutionCompleted:
		data := &ExecutionCompletedData{}
		if outputs, ok := e.Data["outputs"].(map[string]interface{}); ok {
			data.Outputs = outputs
		}
		return data, nil

	case EventExecutionFailed:
		data := &ExecutionFailedData{}
		if errorStr, ok := e.Data["error"].(string); ok {
			data.Error = errorStr
		}
		return data, nil

	case EventPathStarted:
		data := &PathStartedData{}
		if currentStep, ok := e.Data["current_step"].(string); ok {
			data.CurrentStep = currentStep
		}
		return data, nil

	case EventPathCompleted:
		data := &PathCompletedData{}
		if finalStep, ok := e.Data["final_step"].(string); ok {
			data.FinalStep = finalStep
		}
		return data, nil

	case EventPathFailed:
		data := &PathFailedData{}
		if errorStr, ok := e.Data["error"].(string); ok {
			data.Error = errorStr
		}
		return data, nil

	case EventPathBranched:
		data := &PathBranchedData{}
		if newPathsData, ok := e.Data["new_paths"].([]interface{}); ok {
			data.NewPaths = make([]PathBranchInfo, 0, len(newPathsData))
			for _, pathData := range newPathsData {
				if pathMap, ok := pathData.(map[string]interface{}); ok {
					pathInfo := PathBranchInfo{}
					if id, ok := pathMap["id"].(string); ok {
						pathInfo.ID = id
					}
					if currentStep, ok := pathMap["current_step"].(string); ok {
						pathInfo.CurrentStep = currentStep
					}
					if inheritOutputs, ok := pathMap["inherit_outputs"].(bool); ok {
						pathInfo.InheritOutputs = inheritOutputs
					}
					data.NewPaths = append(data.NewPaths, pathInfo)
				}
			}
		}
		return data, nil

	case EventStepStarted:
		data := &StepStartedData{}
		if stepType, ok := e.Data["step_type"].(string); ok {
			data.StepType = stepType
		}
		if stepParams, ok := e.Data["step_params"].(map[string]interface{}); ok {
			data.StepParams = stepParams
		}
		return data, nil

	case EventStepCompleted:
		data := &StepCompletedData{}
		if output, ok := e.Data["output"].(string); ok {
			data.Output = output
		}
		if storedVariable, ok := e.Data["stored_variable"].(string); ok {
			data.StoredVariable = storedVariable
		}
		return data, nil

	case EventStepFailed:
		data := &StepFailedData{}
		if errorStr, ok := e.Data["error"].(string); ok {
			data.Error = errorStr
		}
		return data, nil

	case EventOperationStarted:
		data := &OperationStartedData{}
		if operationID, ok := e.Data["operation_id"].(string); ok {
			data.OperationID = operationID
		}
		if operationType, ok := e.Data["operation_type"].(string); ok {
			data.OperationType = operationType
		}
		if parameters, ok := e.Data["parameters"].(map[string]interface{}); ok {
			data.Parameters = parameters
		}
		return data, nil

	case EventOperationCompleted:
		data := &OperationCompletedData{}
		if operationID, ok := e.Data["operation_id"].(string); ok {
			data.OperationID = operationID
		}
		if operationType, ok := e.Data["operation_type"].(string); ok {
			data.OperationType = operationType
		}
		if duration, ok := e.Data["duration"].(time.Duration); ok {
			data.Duration = duration
		}
		if result, ok := e.Data["result"]; ok {
			data.Result = result
		}
		return data, nil

	case EventOperationFailed:
		data := &OperationFailedData{}
		if operationID, ok := e.Data["operation_id"].(string); ok {
			data.OperationID = operationID
		}
		if operationType, ok := e.Data["operation_type"].(string); ok {
			data.OperationType = operationType
		}
		if duration, ok := e.Data["duration"].(time.Duration); ok {
			data.Duration = duration
		}
		if errorStr, ok := e.Data["error"].(string); ok {
			data.Error = errorStr
		}
		return data, nil

	case EventStateMutated:
		data := &StateMutatedData{}
		if mutations, ok := e.Data["mutations"].([]StateMutation); ok {
			data.Mutations = mutations
		}
		return data, nil

	case EventTimeAccessed:
		data := &TimeAccessedData{}
		if accessedAt, ok := e.Data["accessed_at"].(time.Time); ok {
			data.AccessedAt = accessedAt
		}
		if value, ok := e.Data["value"].(time.Time); ok {
			data.Value = value
		}
		return data, nil

	case EventRandomGenerated:
		data := &RandomGeneratedData{}
		if seed, ok := e.Data["seed"].(int64); ok {
			data.Seed = seed
		}
		if value, ok := e.Data["value"]; ok {
			data.Value = value
		}
		if method, ok := e.Data["method"].(string); ok {
			data.Method = method
		}
		return data, nil

	case EventVariableChanged:
		data := &VariableChangedData{}
		if variableName, ok := e.Data["variable_name"].(string); ok {
			data.VariableName = variableName
		}
		if oldValue, ok := e.Data["old_value"]; ok {
			data.OldValue = oldValue
		}
		if newValue, ok := e.Data["new_value"]; ok {
			data.NewValue = newValue
		}
		if existed, ok := e.Data["existed"].(bool); ok {
			data.Existed = existed
		}
		return data, nil

	case EventTemplateEvaluated:
		data := &TemplateEvaluatedData{}
		if template, ok := e.Data["template"].(string); ok {
			data.Template = template
		}
		if result, ok := e.Data["result"].(string); ok {
			data.Result = result
		}
		return data, nil

	case EventConditionEvaluated:
		data := &ConditionEvaluatedData{}
		if condition, ok := e.Data["condition"].(string); ok {
			data.Condition = condition
		}
		if result, ok := e.Data["result"].(bool); ok {
			data.Result = result
		}
		return data, nil

	case EventIterationStarted:
		data := &IterationStartedData{}
		if iterationIndex, ok := e.Data["iteration_index"].(int); ok {
			data.IterationIndex = iterationIndex
		}
		if item, ok := e.Data["item"]; ok {
			data.Item = item
		}
		if itemKey, ok := e.Data["item_key"].(string); ok {
			data.ItemKey = itemKey
		}
		return data, nil

	case EventIterationCompleted:
		data := &IterationCompletedData{}
		if iterationIndex, ok := e.Data["iteration_index"].(int); ok {
			data.IterationIndex = iterationIndex
		}
		if item, ok := e.Data["item"]; ok {
			data.Item = item
		}
		if itemKey, ok := e.Data["item_key"].(string); ok {
			data.ItemKey = itemKey
		}
		if result, ok := e.Data["result"]; ok {
			data.Result = result
		}
		return data, nil

	default:
		return nil, fmt.Errorf("unknown event type: %s", e.EventType)
	}
}

// SetTypedData sets the typed event data and updates the legacy Data field for compatibility
func (e *ExecutionEvent) SetTypedData(data ExecutionEventData) error {
	if data == nil {
		return fmt.Errorf("typed data cannot be nil")
	}

	if err := data.Validate(); err != nil {
		return fmt.Errorf("typed data validation failed: %w", err)
	}

	if data.EventType() != e.EventType {
		return fmt.Errorf("typed data event type %s does not match event type %s", data.EventType(), e.EventType)
	}

	e.TypedData = data

	// Update legacy Data field for backward compatibility
	e.updateLegacyData()

	return nil
}

// updateLegacyData updates the legacy Data field from the typed data
func (e *ExecutionEvent) updateLegacyData() {
	if e.TypedData == nil {
		return
	}

	// Use reflection or type assertion to convert typed data back to map
	// This is a simplified implementation - in practice you might want to use reflection
	// or a more sophisticated marshaling approach
	e.Data = make(map[string]interface{})

	switch data := e.TypedData.(type) {
	case *ExecutionStartedData:
		e.Data["workflow_name"] = data.WorkflowName
		e.Data["inputs"] = data.Inputs
	case *ExecutionCompletedData:
		e.Data["outputs"] = data.Outputs
	case *ExecutionFailedData:
		e.Data["error"] = data.Error
	case *PathStartedData:
		e.Data["current_step"] = data.CurrentStep
	case *PathCompletedData:
		e.Data["final_step"] = data.FinalStep
	case *PathFailedData:
		e.Data["error"] = data.Error
	case *PathBranchedData:
		pathData := make([]map[string]interface{}, 0, len(data.NewPaths))
		for _, path := range data.NewPaths {
			pathData = append(pathData, map[string]interface{}{
				"id":              path.ID,
				"current_step":    path.CurrentStep,
				"inherit_outputs": path.InheritOutputs,
			})
		}
		e.Data["new_paths"] = pathData
	case *StepStartedData:
		e.Data["step_type"] = data.StepType
		e.Data["step_params"] = data.StepParams
	case *StepCompletedData:
		e.Data["output"] = data.Output
		if data.StoredVariable != "" {
			e.Data["stored_variable"] = data.StoredVariable
		}
	case *StepFailedData:
		e.Data["error"] = data.Error
	case *OperationStartedData:
		e.Data["operation_id"] = data.OperationID
		e.Data["operation_type"] = data.OperationType
		e.Data["parameters"] = data.Parameters
	case *OperationCompletedData:
		e.Data["operation_id"] = data.OperationID
		e.Data["operation_type"] = data.OperationType
		e.Data["duration"] = data.Duration
		e.Data["result"] = data.Result
	case *OperationFailedData:
		e.Data["operation_id"] = data.OperationID
		e.Data["operation_type"] = data.OperationType
		e.Data["duration"] = data.Duration
		e.Data["error"] = data.Error
	case *StateMutatedData:
		e.Data["mutations"] = data.Mutations
	case *TimeAccessedData:
		e.Data["accessed_at"] = data.AccessedAt
		e.Data["value"] = data.Value
	case *RandomGeneratedData:
		e.Data["seed"] = data.Seed
		e.Data["value"] = data.Value
		e.Data["method"] = data.Method
	case *VariableChangedData:
		e.Data["variable_name"] = data.VariableName
		e.Data["old_value"] = data.OldValue
		e.Data["new_value"] = data.NewValue
		e.Data["existed"] = data.Existed
	case *TemplateEvaluatedData:
		e.Data["template"] = data.Template
		e.Data["result"] = data.Result
	case *ConditionEvaluatedData:
		e.Data["condition"] = data.Condition
		e.Data["result"] = data.Result
	case *IterationStartedData:
		e.Data["iteration_index"] = data.IterationIndex
		e.Data["item"] = data.Item
		e.Data["item_key"] = data.ItemKey
	case *IterationCompletedData:
		e.Data["iteration_index"] = data.IterationIndex
		e.Data["item"] = data.Item
		e.Data["item_key"] = data.ItemKey
		e.Data["result"] = data.Result
	}
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

	// Validate typed data if present
	if e.TypedData != nil {
		if err := e.TypedData.Validate(); err != nil {
			return fmt.Errorf("typed data validation failed: %w", err)
		}
		if e.TypedData.EventType() != e.EventType {
			return fmt.Errorf("typed data event type %s does not match event type %s", e.TypedData.EventType(), e.EventType)
		}
	}

	return nil
}
