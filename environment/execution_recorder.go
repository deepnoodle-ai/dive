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
	// RecordEvent records a single event
	RecordEvent(eventType ExecutionEventType, pathID, stepName string, data map[string]interface{})

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

func (r *BufferedExecutionRecorder) RecordEvent(eventType ExecutionEventType, pathID, stepName string, data map[string]interface{}) {
	if r.replayMode {
		return
	}

	event := &ExecutionEvent{
		ID:          NewEventID(),
		ExecutionID: r.executionID,
		Sequence:    atomic.AddInt64(&r.eventSequence, 1),
		Timestamp:   time.Now(),
		EventType:   eventType,
		Path:        pathID,
		Step:        stepName,
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

// EmitEvent implements EventEmitter for ScriptStateManager integration
func (r *BufferedExecutionRecorder) EmitEvent(eventType ExecutionEventType, pathID, stepName string, data map[string]interface{}) {
	r.RecordEvent(eventType, pathID, stepName, data)
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
