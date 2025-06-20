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

// EmitEvent provides backward compatibility for legacy event emission
func (r *BufferedExecutionRecorder) EmitEvent(eventType ExecutionEventType, pathID, stepName string, data map[string]interface{}) {
	typedData := r.convertMapToTypedEvent(eventType, data)
	if typedData != nil {
		r.RecordEvent(pathID, stepName, typedData)
	}
}

// convertMapToTypedEvent converts legacy map data to typed event data
func (r *BufferedExecutionRecorder) convertMapToTypedEvent(eventType ExecutionEventType, data map[string]interface{}) ExecutionEventData {
	switch eventType {
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
