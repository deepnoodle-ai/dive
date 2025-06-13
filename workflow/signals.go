package workflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ExecutionSignal represents a signal sent to a running execution
type ExecutionSignal struct {
	ExecutionID string                 `json:"execution_id"`
	SignalType  string                 `json:"signal_type"`
	Data        map[string]interface{} `json:"data"`
	Timestamp   time.Time              `json:"timestamp"`
	ID          string                 `json:"id"`
}

// SignalHandler defines how to process a signal
type SignalHandler interface {
	HandleSignal(ctx context.Context, signal *ExecutionSignal) error
}

// SignalHandlerFunc is a function type that implements SignalHandler
type SignalHandlerFunc func(ctx context.Context, signal *ExecutionSignal) error

// HandleSignal implements the SignalHandler interface
func (f SignalHandlerFunc) HandleSignal(ctx context.Context, signal *ExecutionSignal) error {
	return f(ctx, signal)
}

// SignalRegistry manages signal handlers for different signal types
type SignalRegistry struct {
	handlers map[string]SignalHandler
	mutex    sync.RWMutex
}

// NewSignalRegistry creates a new signal registry
func NewSignalRegistry() *SignalRegistry {
	return &SignalRegistry{
		handlers: make(map[string]SignalHandler),
	}
}

// RegisterHandler registers a handler for a specific signal type
func (sr *SignalRegistry) RegisterHandler(signalType string, handler SignalHandler) {
	sr.mutex.Lock()
	defer sr.mutex.Unlock()
	sr.handlers[signalType] = handler
}

// RegisterHandlerFunc registers a handler function for a specific signal type
func (sr *SignalRegistry) RegisterHandlerFunc(signalType string, handlerFunc SignalHandlerFunc) {
	sr.RegisterHandler(signalType, handlerFunc)
}

// GetHandler retrieves a handler for a signal type
func (sr *SignalRegistry) GetHandler(signalType string) (SignalHandler, bool) {
	sr.mutex.RLock()
	defer sr.mutex.RUnlock()
	handler, exists := sr.handlers[signalType]
	return handler, exists
}

// ProcessSignal processes a signal using the registered handler
func (sr *SignalRegistry) ProcessSignal(ctx context.Context, signal *ExecutionSignal) error {
	handler, exists := sr.GetHandler(signal.SignalType)
	if !exists {
		// If no specific handler exists, this is not an error - signals can be informational
		return nil
	}
	return handler.HandleSignal(ctx, signal)
}

// SignalQueue manages pending signals for executions
type SignalQueue struct {
	signals map[string][]*ExecutionSignal // executionID -> signals
	mutex   sync.RWMutex
}

// NewSignalQueue creates a new signal queue
func NewSignalQueue() *SignalQueue {
	return &SignalQueue{
		signals: make(map[string][]*ExecutionSignal),
	}
}

// EnqueueSignal adds a signal to the queue for an execution
func (sq *SignalQueue) EnqueueSignal(signal *ExecutionSignal) {
	sq.mutex.Lock()
	defer sq.mutex.Unlock()

	sq.signals[signal.ExecutionID] = append(sq.signals[signal.ExecutionID], signal)
}

// DequeueSignals retrieves and removes all pending signals for an execution
func (sq *SignalQueue) DequeueSignals(executionID string) []*ExecutionSignal {
	sq.mutex.Lock()
	defer sq.mutex.Unlock()

	signals := sq.signals[executionID]
	delete(sq.signals, executionID)
	return signals
}

// PeekSignals returns pending signals without removing them
func (sq *SignalQueue) PeekSignals(executionID string) []*ExecutionSignal {
	sq.mutex.RLock()
	defer sq.mutex.RUnlock()

	// Return a copy to avoid race conditions
	signals := sq.signals[executionID]
	if signals == nil {
		return nil
	}

	result := make([]*ExecutionSignal, len(signals))
	copy(result, signals)
	return result
}

// HasSignals checks if there are pending signals for an execution
func (sq *SignalQueue) HasSignals(executionID string) bool {
	sq.mutex.RLock()
	defer sq.mutex.RUnlock()

	signals, exists := sq.signals[executionID]
	return exists && len(signals) > 0
}

// Common signal types
const (
	SignalTypePause        = "pause"
	SignalTypeResume       = "resume"
	SignalTypeCancel       = "cancel"
	SignalTypeUpdateInputs = "update_inputs"
	SignalTypeUpdateParams = "update_params"
	SignalTypeStepComplete = "step_complete"
	SignalTypeStepSkip     = "step_skip"
	SignalTypeCustom       = "custom"
)

// PauseSignalData represents data for pause signal
type PauseSignalData struct {
	Reason  string `json:"reason,omitempty"`
	Timeout string `json:"timeout,omitempty"` // Duration string like "30m"
}

// ResumeSignalData represents data for resume signal
type ResumeSignalData struct {
	Reason string `json:"reason,omitempty"`
}

// CancelSignalData represents data for cancel signal
type CancelSignalData struct {
	Reason string `json:"reason,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

// UpdateInputsSignalData represents data for updating inputs
type UpdateInputsSignalData struct {
	NewInputs map[string]interface{} `json:"new_inputs"`
	Merge     bool                   `json:"merge,omitempty"` // If true, merge with existing inputs
}

// UpdateParamsSignalData represents data for updating step parameters
type UpdateParamsSignalData struct {
	StepName  string                 `json:"step_name"`
	NewParams map[string]interface{} `json:"new_params"`
	Merge     bool                   `json:"merge,omitempty"`
}

// StepCompleteSignalData represents data for marking a step as complete
type StepCompleteSignalData struct {
	StepName string `json:"step_name"`
	Output   string `json:"output"`
	Force    bool   `json:"force,omitempty"`
}

// StepSkipSignalData represents data for skipping a step
type StepSkipSignalData struct {
	StepName string `json:"step_name"`
	Reason   string `json:"reason,omitempty"`
}

// Built-in signal handlers

// CreatePauseHandler creates a handler for pause signals
func CreatePauseHandler() SignalHandlerFunc {
	return func(ctx context.Context, signal *ExecutionSignal) error {
		// Implementation would be in the execution engine
		// This is a placeholder showing the pattern
		return nil
	}
}

// CreateResumeHandler creates a handler for resume signals
func CreateResumeHandler() SignalHandlerFunc {
	return func(ctx context.Context, signal *ExecutionSignal) error {
		// Implementation would be in the execution engine
		return nil
	}
}

// CreateCancelHandler creates a handler for cancel signals
func CreateCancelHandler() SignalHandlerFunc {
	return func(ctx context.Context, signal *ExecutionSignal) error {
		// Implementation would be in the execution engine
		return nil
	}
}

// CreateUpdateInputsHandler creates a handler for input update signals
func CreateUpdateInputsHandler() SignalHandlerFunc {
	return func(ctx context.Context, signal *ExecutionSignal) error {
		// Implementation would be in the execution engine
		return nil
	}
}

// Validate validates the execution signal
func (s *ExecutionSignal) Validate() error {
	if s.ExecutionID == "" {
		return fmt.Errorf("execution ID is required")
	}
	if s.SignalType == "" {
		return fmt.Errorf("signal type is required")
	}
	if s.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	if s.ID == "" {
		return fmt.Errorf("signal ID is required")
	}
	return nil
}

// NewExecutionSignal creates a new execution signal with generated ID and timestamp
func NewExecutionSignal(executionID, signalType string, data map[string]interface{}) *ExecutionSignal {
	return &ExecutionSignal{
		ID:          generateSignalID(),
		ExecutionID: executionID,
		SignalType:  signalType,
		Data:        data,
		Timestamp:   time.Now(),
	}
}

// generateSignalID generates a unique signal ID
func generateSignalID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Helper functions for creating specific signal types

// NewPauseSignal creates a pause signal
func NewPauseSignal(executionID, reason string) *ExecutionSignal {
	data := map[string]interface{}{
		"reason": reason,
	}
	return NewExecutionSignal(executionID, SignalTypePause, data)
}

// NewResumeSignal creates a resume signal
func NewResumeSignal(executionID, reason string) *ExecutionSignal {
	data := map[string]interface{}{
		"reason": reason,
	}
	return NewExecutionSignal(executionID, SignalTypeResume, data)
}

// NewCancelSignal creates a cancel signal
func NewCancelSignal(executionID, reason string, force bool) *ExecutionSignal {
	data := map[string]interface{}{
		"reason": reason,
		"force":  force,
	}
	return NewExecutionSignal(executionID, SignalTypeCancel, data)
}

// NewUpdateInputsSignal creates an update inputs signal
func NewUpdateInputsSignal(executionID string, newInputs map[string]interface{}, merge bool) *ExecutionSignal {
	data := map[string]interface{}{
		"new_inputs": newInputs,
		"merge":      merge,
	}
	return NewExecutionSignal(executionID, SignalTypeUpdateInputs, data)
}

// NewUpdateParamsSignal creates an update parameters signal
func NewUpdateParamsSignal(executionID, stepName string, newParams map[string]interface{}, merge bool) *ExecutionSignal {
	data := map[string]interface{}{
		"step_name":  stepName,
		"new_params": newParams,
		"merge":      merge,
	}
	return NewExecutionSignal(executionID, SignalTypeUpdateParams, data)
}

// NewStepCompleteSignal creates a step complete signal
func NewStepCompleteSignal(executionID, stepName, output string, force bool) *ExecutionSignal {
	data := map[string]interface{}{
		"step_name": stepName,
		"output":    output,
		"force":     force,
	}
	return NewExecutionSignal(executionID, SignalTypeStepComplete, data)
}

// NewStepSkipSignal creates a step skip signal
func NewStepSkipSignal(executionID, stepName, reason string) *ExecutionSignal {
	data := map[string]interface{}{
		"step_name": stepName,
		"reason":    reason,
	}
	return NewExecutionSignal(executionID, SignalTypeStepSkip, data)
}

// NewCustomSignal creates a custom signal
func NewCustomSignal(executionID, customType string, data map[string]interface{}) *ExecutionSignal {
	// Add the custom type to the data
	if data == nil {
		data = make(map[string]interface{})
	}
	data["custom_type"] = customType

	return NewExecutionSignal(executionID, SignalTypeCustom, data)
}
