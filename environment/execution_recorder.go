package environment

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ExecutionRecorder handles all event recording for workflow executions
type ExecutionRecorder interface {
	// RecordExecutionStarted records the start of an execution
	RecordExecutionStarted(workflowName string, inputs map[string]interface{})

	// RecordPathStarted records the start of a new path
	RecordPathStarted(pathID string, currentStep string)

	// RecordStepStarted records the start of a step
	RecordStepStarted(pathID, stepName, stepType string, params map[string]interface{})

	// RecordStepCompleted records successful step completion
	RecordStepCompleted(pathID, stepName string, output string, storedVar string)

	// RecordStepFailed records step failure
	RecordStepFailed(pathID, stepName string, err error)

	// RecordPathBranched records path branching
	RecordPathBranched(parentPathID, stepName string, newPaths []PathBranchInfo)

	// RecordPathCompleted records path completion
	RecordPathCompleted(pathID, finalStep string)

	// RecordPathFailed records path failure
	RecordPathFailed(pathID string, err error)

	// RecordExecutionCompleted records execution completion
	RecordExecutionCompleted(outputs map[string]interface{})

	// RecordExecutionFailed records execution failure
	RecordExecutionFailed(err error)

	// RecordCustomEvent records a custom event
	RecordCustomEvent(eventType ExecutionEventType, pathID, stepName string, data map[string]interface{})

	// Flush forces any buffered events to be written
	Flush() error

	// SetReplayMode enables/disables replay mode
	SetReplayMode(enabled bool)

	// GetExecutionID returns the execution ID
	GetExecutionID() string
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

func (r *BufferedExecutionRecorder) recordEvent(eventType ExecutionEventType, pathID, stepName string, data map[string]interface{}) {
	if r.replayMode {
		return
	}

	event := &ExecutionEvent{
		ID:          NewEventID(),
		ExecutionID: r.executionID,
		PathID:      pathID,
		Sequence:    atomic.AddInt64(&r.eventSequence, 1),
		Timestamp:   time.Now(),
		EventType:   eventType,
		StepName:    stepName,
		Data:        data,
	}

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

// Implementation of all recording methods
func (r *BufferedExecutionRecorder) RecordExecutionStarted(workflowName string, inputs map[string]interface{}) {
	r.recordEvent(EventExecutionStarted, "", "", map[string]interface{}{
		"workflow_name": workflowName,
		"inputs":        inputs,
	})
}

func (r *BufferedExecutionRecorder) RecordPathStarted(pathID string, currentStep string) {
	r.recordEvent(EventPathStarted, pathID, "", map[string]interface{}{
		"current_step": currentStep,
	})
}

func (r *BufferedExecutionRecorder) RecordStepStarted(pathID, stepName, stepType string, params map[string]interface{}) {
	r.recordEvent(EventStepStarted, pathID, stepName, map[string]interface{}{
		"step_type":   stepType,
		"step_params": params,
	})
}

func (r *BufferedExecutionRecorder) RecordStepCompleted(pathID, stepName string, output string, storedVar string) {
	data := map[string]interface{}{
		"output": output,
	}
	if storedVar != "" {
		data["stored_variable"] = storedVar
	}
	r.recordEvent(EventStepCompleted, pathID, stepName, data)
}

func (r *BufferedExecutionRecorder) RecordStepFailed(pathID, stepName string, err error) {
	r.recordEvent(EventStepFailed, pathID, stepName, map[string]interface{}{
		"error": err.Error(),
	})
}

func (r *BufferedExecutionRecorder) RecordPathBranched(parentPathID, stepName string, newPaths []PathBranchInfo) {
	pathData := make([]map[string]interface{}, len(newPaths))
	for i, path := range newPaths {
		pathData[i] = map[string]interface{}{
			"id":              path.ID,
			"current_step":    path.CurrentStep,
			"inherit_outputs": path.InheritOutputs,
		}
	}

	r.recordEvent(EventPathBranched, parentPathID, stepName, map[string]interface{}{
		"new_paths": pathData,
	})
}

func (r *BufferedExecutionRecorder) RecordPathCompleted(pathID, finalStep string) {
	r.recordEvent(EventPathCompleted, pathID, "", map[string]interface{}{
		"final_step": finalStep,
	})
}

func (r *BufferedExecutionRecorder) RecordPathFailed(pathID string, err error) {
	r.recordEvent(EventPathFailed, pathID, "", map[string]interface{}{
		"error": err.Error(),
	})
}

func (r *BufferedExecutionRecorder) RecordExecutionCompleted(outputs map[string]interface{}) {
	r.recordEvent(EventExecutionCompleted, "", "", map[string]interface{}{
		"outputs": outputs,
	})
}

func (r *BufferedExecutionRecorder) RecordExecutionFailed(err error) {
	r.recordEvent(EventExecutionFailed, "", "", map[string]interface{}{
		"error": err.Error(),
	})
}

func (r *BufferedExecutionRecorder) RecordCustomEvent(eventType ExecutionEventType, pathID, stepName string, data map[string]interface{}) {
	r.recordEvent(eventType, pathID, stepName, data)
}

// EmitEvent implements EventEmitter for ScriptStateManager integration
func (r *BufferedExecutionRecorder) EmitEvent(eventType ExecutionEventType, pathID, stepName string, data map[string]interface{}) {
	r.recordEvent(eventType, pathID, stepName, data)
}
