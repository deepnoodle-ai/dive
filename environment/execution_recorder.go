package environment

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type StateMutationType string

const (
	StateMutationTypeSet    StateMutationType = "set"
	StateMutationTypeDelete StateMutationType = "delete"
)

type StateMutation struct {
	Type  StateMutationType `json:"type"`
	Key   string            `json:"key,omitempty"`
	Value interface{}       `json:"value,omitempty"`
}

// ExecutionRecorder handles all event recording for workflow executions
type ExecutionRecorder interface {
	// RecordEvent records a single event using typed event data
	RecordEvent(pathID, stepName string, data ExecutionEventData)

	// Flush forces any buffered events to be written
	Flush() error

	// SetReplayMode enables/disables replay mode
	SetReplayMode(enabled bool)

	// GetExecutionID returns the execution ID
	GetExecutionID() string

	// SaveSnapshot saves an execution snapshot
	SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error

	// GetEventHistory retrieves all events for replay
	GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error)

	// GetEventSequence returns the current event sequence number
	GetEventSequence() int64

	// Convenience methods for recording common events
	RecordExecutionStarted(workflowName string, inputs map[string]interface{})
	RecordExecutionCompleted(outputs map[string]interface{})
	RecordExecutionFailed(err error)
	RecordPathStarted(pathID, currentStep string)
	RecordPathCompleted(pathID, finalStep string)
	RecordPathFailed(pathID string, err error)
	RecordPathBranched(pathID, stepName string, newPaths []PathBranchInfo)
	RecordStepStarted(pathID, stepName, stepType string, stepParams map[string]interface{})
	RecordStepCompleted(pathID, stepName, output, storedVariable string)
	RecordStepFailed(pathID, stepName string, err error)
	RecordOperationStarted(pathID, operationID, operationType string, parameters map[string]interface{})
	RecordOperationCompleted(pathID, operationID, operationType string, duration time.Duration, result interface{})
	RecordOperationFailed(pathID, operationID, operationType string, duration time.Duration, err error)
	RecordStateMutated(mutations []StateMutation)
	RecordSignalReceived(signalType, source string, payload map[string]interface{})
	RecordVersionDecision(decisionType, selectedVersion, reason string, availableVersions []string)
	RecordExecutionContinueAsNew(newExecutionID, reason string, newInputs, stateTransfer map[string]interface{})
}

// PathBranchInfo contains information about a branched path
type PathBranchInfo struct {
	ID             string
	CurrentStep    string
	InheritOutputs bool
}

// BufferedExecutionRecorder implements ExecutionRecorder with buffering
type BufferedExecutionRecorder struct {
	executionID   string
	eventStore    ExecutionEventStore
	eventBuffer   []*ExecutionEvent
	eventSequence int64
	replayMode    bool
	batchSize     int
	bufferMutex   sync.Mutex
}

// NewBufferedExecutionRecorder creates a new buffered execution recorder
func NewBufferedExecutionRecorder(executionID string, eventStore ExecutionEventStore, batchSize int) *BufferedExecutionRecorder {
	if batchSize <= 0 {
		batchSize = 10
	}

	return &BufferedExecutionRecorder{
		executionID: executionID,
		eventStore:  eventStore,
		eventBuffer: make([]*ExecutionEvent, 0, batchSize),
		batchSize:   batchSize,
	}
}

func (r *BufferedExecutionRecorder) GetExecutionID() string {
	return r.executionID
}

func (r *BufferedExecutionRecorder) SetReplayMode(enabled bool) {
	r.replayMode = enabled
}

func (r *BufferedExecutionRecorder) RecordEvent(pathID, stepName string, data ExecutionEventData) {
	if r.replayMode {
		return
	}

	event := &ExecutionEvent{
		ID:          NewEventID(),
		ExecutionID: r.executionID,
		Sequence:    atomic.AddInt64(&r.eventSequence, 1),
		Timestamp:   time.Now(),
		EventType:   data.EventType(),
		Path:        pathID,
		Step:        stepName,
		TypedData:   data,
	}

	// Set the legacy Data field for backward compatibility
	event.updateLegacyData()

	r.bufferMutex.Lock()
	r.eventBuffer = append(r.eventBuffer, event)
	shouldFlush := len(r.eventBuffer) >= r.batchSize
	r.bufferMutex.Unlock()

	if shouldFlush {
		r.Flush()
	}
}

func (r *BufferedExecutionRecorder) Flush() error {
	r.bufferMutex.Lock()
	if len(r.eventBuffer) == 0 {
		r.bufferMutex.Unlock()
		return nil
	}

	events := make([]*ExecutionEvent, len(r.eventBuffer))
	copy(events, r.eventBuffer)
	r.eventBuffer = r.eventBuffer[:0]
	r.bufferMutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return r.eventStore.AppendEvents(ctx, events)
}

// EmitEvent implements EventEmitter for ScriptStateManager integration
func (r *BufferedExecutionRecorder) EmitEvent(eventType ExecutionEventType, pathID, stepName string, data map[string]interface{}) {
	typedData := r.convertMapToTypedEvent(eventType, data)
	if typedData != nil {
		r.RecordEvent(pathID, stepName, typedData)
	}
}

// convertMapToTypedEvent converts legacy map data to typed event data
func (r *BufferedExecutionRecorder) convertMapToTypedEvent(eventType ExecutionEventType, data map[string]interface{}) ExecutionEventData {
	switch eventType {
	case EventVariableChanged:
		result := &VariableChangedData{}
		if variableName, ok := data["variable_name"].(string); ok {
			result.VariableName = variableName
		}
		if oldValue, ok := data["old_value"]; ok {
			result.OldValue = oldValue
		}
		if newValue, ok := data["new_value"]; ok {
			result.NewValue = newValue
		}
		if existed, ok := data["existed"].(bool); ok {
			result.Existed = existed
		}
		return result

	case EventTemplateEvaluated:
		result := &TemplateEvaluatedData{}
		if template, ok := data["template"].(string); ok {
			result.Template = template
		}
		if resultStr, ok := data["result"].(string); ok {
			result.Result = resultStr
		}
		return result

	case EventConditionEvaluated:
		result := &ConditionEvaluatedData{}
		if condition, ok := data["condition"].(string); ok {
			result.Condition = condition
		}
		if resultBool, ok := data["result"].(bool); ok {
			result.Result = resultBool
		}
		return result

	case EventIterationStarted:
		result := &IterationStartedData{}
		if iterationIndex, ok := data["iteration_index"].(int); ok {
			result.IterationIndex = iterationIndex
		}
		if item, ok := data["item"]; ok {
			result.Item = item
		}
		if itemKey, ok := data["item_key"].(string); ok {
			result.ItemKey = itemKey
		}
		return result

	case EventIterationCompleted:
		result := &IterationCompletedData{}
		if iterationIndex, ok := data["iteration_index"].(int); ok {
			result.IterationIndex = iterationIndex
		}
		if item, ok := data["item"]; ok {
			result.Item = item
		}
		if itemKey, ok := data["item_key"].(string); ok {
			result.ItemKey = itemKey
		}
		if resultData, ok := data["result"]; ok {
			result.Result = resultData
		}
		return result

	case EventSignalReceived:
		result := &SignalReceivedData{}
		if signalType, ok := data["signal_type"].(string); ok {
			result.SignalType = signalType
		}
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			result.Payload = payload
		}
		if source, ok := data["source"].(string); ok {
			result.Source = source
		}
		return result

	case EventVersionDecision:
		result := &VersionDecisionData{}
		if decisionType, ok := data["decision_type"].(string); ok {
			result.DecisionType = decisionType
		}
		if selectedVersion, ok := data["selected_version"].(string); ok {
			result.SelectedVersion = selectedVersion
		}
		if availableVersions, ok := data["available_versions"].([]string); ok {
			result.AvailableVersions = availableVersions
		}
		if reason, ok := data["reason"].(string); ok {
			result.Reason = reason
		}
		return result

	case EventExecutionContinueAsNew:
		result := &ExecutionContinueAsNewData{}
		if newExecutionID, ok := data["new_execution_id"].(string); ok {
			result.NewExecutionID = newExecutionID
		}
		if newInputs, ok := data["new_inputs"].(map[string]interface{}); ok {
			result.NewInputs = newInputs
		}
		if reason, ok := data["reason"].(string); ok {
			result.Reason = reason
		}
		if stateTransfer, ok := data["state_transfer"].(map[string]interface{}); ok {
			result.StateTransfer = stateTransfer
		}
		return result

	default:
		// For unknown event types, we can't create typed data
		// This should be rare and only happen during transitions
		return nil
	}
}

// SaveSnapshot saves an execution snapshot via the event store
func (r *BufferedExecutionRecorder) SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error {
	return r.eventStore.SaveSnapshot(ctx, snapshot)
}

// GetEventHistory retrieves all events for replay via the event store
func (r *BufferedExecutionRecorder) GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error) {
	return r.eventStore.GetEventHistory(ctx, executionID)
}

// GetEventSequence returns the current event sequence number
func (r *BufferedExecutionRecorder) GetEventSequence() int64 {
	return atomic.LoadInt64(&r.eventSequence)
}

// Convenience methods for recording common typed events

// RecordExecutionStarted records an execution started event
func (r *BufferedExecutionRecorder) RecordExecutionStarted(workflowName string, inputs map[string]interface{}) {
	r.RecordEvent("", "", &ExecutionStartedData{
		WorkflowName: workflowName,
		Inputs:       inputs,
	})
}

// RecordExecutionCompleted records an execution completed event
func (r *BufferedExecutionRecorder) RecordExecutionCompleted(outputs map[string]interface{}) {
	r.RecordEvent("", "", &ExecutionCompletedData{
		Outputs: outputs,
	})
}

// RecordExecutionFailed records an execution failed event
func (r *BufferedExecutionRecorder) RecordExecutionFailed(err error) {
	r.RecordEvent("", "", &ExecutionFailedData{
		Error: err.Error(),
	})
}

// RecordPathStarted records a path started event
func (r *BufferedExecutionRecorder) RecordPathStarted(pathID, currentStep string) {
	r.RecordEvent(pathID, "", &PathStartedData{
		CurrentStep: currentStep,
	})
}

// RecordPathCompleted records a path completed event
func (r *BufferedExecutionRecorder) RecordPathCompleted(pathID, finalStep string) {
	r.RecordEvent(pathID, "", &PathCompletedData{
		FinalStep: finalStep,
	})
}

// RecordPathFailed records a path failed event
func (r *BufferedExecutionRecorder) RecordPathFailed(pathID string, err error) {
	r.RecordEvent(pathID, "", &PathFailedData{
		Error: err.Error(),
	})
}

// RecordPathBranched records a path branched event
func (r *BufferedExecutionRecorder) RecordPathBranched(pathID, stepName string, newPaths []PathBranchInfo) {
	r.RecordEvent(pathID, stepName, &PathBranchedData{
		NewPaths: newPaths,
	})
}

// RecordStepStarted records a step started event
func (r *BufferedExecutionRecorder) RecordStepStarted(pathID, stepName, stepType string, stepParams map[string]interface{}) {
	r.RecordEvent(pathID, stepName, &StepStartedData{
		StepType:   stepType,
		StepParams: stepParams,
	})
}

// RecordStepCompleted records a step completed event
func (r *BufferedExecutionRecorder) RecordStepCompleted(pathID, stepName, output, storedVariable string) {
	r.RecordEvent(pathID, stepName, &StepCompletedData{
		Output:         output,
		StoredVariable: storedVariable,
	})
}

// RecordStepFailed records a step failed event
func (r *BufferedExecutionRecorder) RecordStepFailed(pathID, stepName string, err error) {
	r.RecordEvent(pathID, stepName, &StepFailedData{
		Error: err.Error(),
	})
}

// RecordOperationStarted records an operation started event
func (r *BufferedExecutionRecorder) RecordOperationStarted(pathID, operationID, operationType string, parameters map[string]interface{}) {
	r.RecordEvent(pathID, "", &OperationStartedData{
		OperationID:   operationID,
		OperationType: operationType,
		Parameters:    parameters,
	})
}

// RecordOperationCompleted records an operation completed event
func (r *BufferedExecutionRecorder) RecordOperationCompleted(pathID, operationID, operationType string, duration time.Duration, result interface{}) {
	r.RecordEvent(pathID, "", &OperationCompletedData{
		OperationID:   operationID,
		OperationType: operationType,
		Duration:      duration,
		Result:        result,
	})
}

// RecordOperationFailed records an operation failed event
func (r *BufferedExecutionRecorder) RecordOperationFailed(pathID, operationID, operationType string, duration time.Duration, err error) {
	r.RecordEvent(pathID, "", &OperationFailedData{
		OperationID:   operationID,
		OperationType: operationType,
		Duration:      duration,
		Error:         err.Error(),
	})
}

// RecordStateMutated records a state mutated event
func (r *BufferedExecutionRecorder) RecordStateMutated(mutations []StateMutation) {
	r.RecordEvent("", "", &StateMutatedData{
		Mutations: mutations,
	})
}

// RecordSignalReceived records a signal received event
func (r *BufferedExecutionRecorder) RecordSignalReceived(signalType, source string, payload map[string]interface{}) {
	r.RecordEvent("", "", &SignalReceivedData{
		SignalType: signalType,
		Source:     source,
		Payload:    payload,
	})
}

// RecordVersionDecision records a version decision event
func (r *BufferedExecutionRecorder) RecordVersionDecision(decisionType, selectedVersion, reason string, availableVersions []string) {
	r.RecordEvent("", "", &VersionDecisionData{
		DecisionType:      decisionType,
		SelectedVersion:   selectedVersion,
		AvailableVersions: availableVersions,
		Reason:            reason,
	})
}

// RecordExecutionContinueAsNew records an execution continue as new event
func (r *BufferedExecutionRecorder) RecordExecutionContinueAsNew(newExecutionID, reason string, newInputs, stateTransfer map[string]interface{}) {
	r.RecordEvent("", "", &ExecutionContinueAsNewData{
		NewExecutionID: newExecutionID,
		NewInputs:      newInputs,
		Reason:         reason,
		StateTransfer:  stateTransfer,
	})
}
