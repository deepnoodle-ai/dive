package environment

import (
	"context"
	"fmt"
	"time"
)

// ContinueAsNewOptions configures when and how to continue as new
type ContinueAsNewOptions struct {
	MaxEvents         int                    `json:"max_events"`         // Maximum events before triggering continue-as-new (default: 10,000)
	MaxDuration       time.Duration          `json:"max_duration"`       // Maximum execution duration before triggering (default: 24 hours)
	MaxEventSize      int64                  `json:"max_event_size"`     // Maximum total size of events (default: 100MB)
	NewInputs         map[string]interface{} `json:"new_inputs"`         // Optional new inputs for the continued execution
	WorkflowVersion   int                    `json:"workflow_version"`   // Version to use for the new execution
	PreservePaths     bool                   `json:"preserve_paths"`     // Whether to preserve active path states
	PreserveGlobals   bool                   `json:"preserve_globals"`   // Whether to preserve script globals
	TriggerCondition  string                 `json:"trigger_condition"`  // Optional custom condition for triggering
	ContinuationDelay time.Duration          `json:"continuation_delay"` // Delay before starting new execution
}

// DefaultContinueAsNewOptions returns sensible defaults for production use
func DefaultContinueAsNewOptions() ContinueAsNewOptions {
	return ContinueAsNewOptions{
		MaxEvents:         10000,
		MaxDuration:       24 * time.Hour,
		MaxEventSize:      100 * 1024 * 1024, // 100MB
		WorkflowVersion:   1,
		PreservePaths:     true,
		PreserveGlobals:   true,
		ContinuationDelay: 5 * time.Second,
	}
}

// ContinueAsNewReason represents why continue-as-new was triggered
type ContinueAsNewReason string

const (
	ContinueAsNewReasonMaxEvents     ContinueAsNewReason = "max_events_reached"
	ContinueAsNewReasonMaxDuration   ContinueAsNewReason = "max_duration_reached"
	ContinueAsNewReasonMaxEventSize  ContinueAsNewReason = "max_event_size_reached"
	ContinueAsNewReasonCustomTrigger ContinueAsNewReason = "custom_trigger"
	ContinueAsNewReasonManual        ContinueAsNewReason = "manual_request"
)

// ContinueAsNewDecision represents a decision to continue as new
type ContinueAsNewDecision struct {
	ShouldContinue    bool                   `json:"should_continue"`
	Reason            ContinueAsNewReason    `json:"reason"`
	CurrentMetrics    ContinueAsNewMetrics   `json:"current_metrics"`
	NewInputs         map[string]interface{} `json:"new_inputs,omitempty"`
	PreservedState    *PreservedState        `json:"preserved_state,omitempty"`
	ContinuationDelay time.Duration          `json:"continuation_delay,omitempty"`
}

// ContinueAsNewMetrics tracks metrics relevant to continue-as-new decisions
type ContinueAsNewMetrics struct {
	EventCount        int64         `json:"event_count"`
	ExecutionDuration time.Duration `json:"execution_duration"`
	TotalEventSize    int64         `json:"total_event_size"`
	ActivePaths       int           `json:"active_paths"`
	LastEventTime     time.Time     `json:"last_event_time"`
}

// PreservedState contains state to preserve across continue-as-new
type PreservedState struct {
	ScriptGlobals    map[string]interface{} `json:"script_globals"`
	ActivePaths      []*ReplayPathState     `json:"active_paths"`
	CompletedSteps   map[string]string      `json:"completed_steps"`
	ExecutionInputs  map[string]interface{} `json:"execution_inputs"`
	ContinuationInfo *ContinuationInfo      `json:"continuation_info"`
}

// ContinuationInfo tracks continue-as-new chain information
type ContinuationInfo struct {
	OriginalExecutionID string    `json:"original_execution_id"`
	PreviousExecutionID string    `json:"previous_execution_id"`
	ContinuationCount   int       `json:"continuation_count"`
	TotalEvents         int64     `json:"total_events"`
	ChainStartTime      time.Time `json:"chain_start_time"`
}

// ContinueAsNewEvaluator evaluates whether an execution should continue as new
type ContinueAsNewEvaluator interface {
	ShouldContinueAsNew(ctx context.Context, metrics ContinueAsNewMetrics, options ContinueAsNewOptions) (*ContinueAsNewDecision, error)
	PrepareNewExecution(ctx context.Context, decision *ContinueAsNewDecision, currentState interface{}) (*PreservedState, error)
}

// DefaultContinueAsNewEvaluator provides the default evaluation logic
type DefaultContinueAsNewEvaluator struct {
	logger interface {
		Info(msg string, keysAndValues ...interface{})
		Warn(msg string, keysAndValues ...interface{})
	}
}

// NewDefaultContinueAsNewEvaluator creates a new default evaluator
func NewDefaultContinueAsNewEvaluator(logger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
}) *DefaultContinueAsNewEvaluator {
	return &DefaultContinueAsNewEvaluator{
		logger: logger,
	}
}

// ShouldContinueAsNew evaluates whether to continue as new based on metrics and options
func (e *DefaultContinueAsNewEvaluator) ShouldContinueAsNew(ctx context.Context, metrics ContinueAsNewMetrics, options ContinueAsNewOptions) (*ContinueAsNewDecision, error) {
	decision := &ContinueAsNewDecision{
		ShouldContinue:    false,
		CurrentMetrics:    metrics,
		ContinuationDelay: options.ContinuationDelay,
	}

	// Check event count threshold
	if options.MaxEvents > 0 && metrics.EventCount >= int64(options.MaxEvents) {
		decision.ShouldContinue = true
		decision.Reason = ContinueAsNewReasonMaxEvents
		e.logger.Info("continue-as-new triggered by max events",
			"current_events", metrics.EventCount,
			"max_events", options.MaxEvents)
		return decision, nil
	}

	// Check duration threshold
	if options.MaxDuration > 0 && metrics.ExecutionDuration >= options.MaxDuration {
		decision.ShouldContinue = true
		decision.Reason = ContinueAsNewReasonMaxDuration
		e.logger.Info("continue-as-new triggered by max duration",
			"current_duration", metrics.ExecutionDuration,
			"max_duration", options.MaxDuration)
		return decision, nil
	}

	// Check event size threshold
	if options.MaxEventSize > 0 && metrics.TotalEventSize >= options.MaxEventSize {
		decision.ShouldContinue = true
		decision.Reason = ContinueAsNewReasonMaxEventSize
		e.logger.Info("continue-as-new triggered by max event size",
			"current_size", metrics.TotalEventSize,
			"max_size", options.MaxEventSize)
		return decision, nil
	}

	// Check custom trigger condition if specified
	if options.TriggerCondition != "" {
		// This would require script evaluation - placeholder for now
		e.logger.Info("custom trigger condition evaluation not yet implemented",
			"condition", options.TriggerCondition)
	}

	return decision, nil
}

// PrepareNewExecution prepares state for the new execution
func (e *DefaultContinueAsNewEvaluator) PrepareNewExecution(ctx context.Context, decision *ContinueAsNewDecision, currentState interface{}) (*PreservedState, error) {
	preserved := &PreservedState{
		ScriptGlobals:   make(map[string]interface{}),
		ActivePaths:     make([]*ReplayPathState, 0),
		CompletedSteps:  make(map[string]string),
		ExecutionInputs: make(map[string]interface{}),
		ContinuationInfo: &ContinuationInfo{
			ContinuationCount: 1,
			TotalEvents:       decision.CurrentMetrics.EventCount,
			ChainStartTime:    time.Now(),
		},
	}

	// In a real implementation, this would extract state from the current execution
	// For now, we'll return the basic structure
	e.logger.Info("prepared state for continue-as-new",
		"reason", decision.Reason,
		"preserved_globals", len(preserved.ScriptGlobals),
		"preserved_paths", len(preserved.ActivePaths))

	return preserved, nil
}

// ContinueAsNewManager manages the continue-as-new process
type ContinueAsNewManager struct {
	evaluator    ContinueAsNewEvaluator
	eventStore   ExecutionEventStore
	orchestrator interface{} // Would be ExecutionOrchestrator in real implementation
	options      ContinueAsNewOptions
	logger       interface {
		Info(msg string, keysAndValues ...interface{})
		Warn(msg string, keysAndValues ...interface{})
		Error(msg string, keysAndValues ...interface{})
	}
}

// NewContinueAsNewManager creates a new continue-as-new manager
func NewContinueAsNewManager(
	evaluator ContinueAsNewEvaluator,
	eventStore ExecutionEventStore,
	options ContinueAsNewOptions,
	logger interface {
		Info(msg string, keysAndValues ...interface{})
		Warn(msg string, keysAndValues ...interface{})
		Error(msg string, keysAndValues ...interface{})
	},
) *ContinueAsNewManager {
	return &ContinueAsNewManager{
		evaluator:  evaluator,
		eventStore: eventStore,
		options:    options,
		logger:     logger,
	}
}

// EvaluateContinuation evaluates whether an execution should continue as new
func (m *ContinueAsNewManager) EvaluateContinuation(ctx context.Context, executionID string) (*ContinueAsNewDecision, error) {
	// Get current execution metrics
	metrics, err := m.getExecutionMetrics(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get execution metrics: %w", err)
	}

	// Evaluate using the configured evaluator
	decision, err := m.evaluator.ShouldContinueAsNew(ctx, *metrics, m.options)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate continue-as-new: %w", err)
	}

	return decision, nil
}

// ExecuteContinuation executes a continue-as-new operation
func (m *ContinueAsNewManager) ExecuteContinuation(ctx context.Context, executionID string, decision *ContinueAsNewDecision) (string, error) {
	if !decision.ShouldContinue {
		return "", fmt.Errorf("decision indicates should not continue as new")
	}

	// Get current execution state
	snapshot, err := m.eventStore.GetSnapshot(ctx, executionID)
	if err != nil {
		return "", fmt.Errorf("failed to get execution snapshot: %w", err)
	}

	// Prepare state for new execution
	preservedState, err := m.evaluator.PrepareNewExecution(ctx, decision, snapshot)
	if err != nil {
		return "", fmt.Errorf("failed to prepare new execution: %w", err)
	}

	// Record continue-as-new event in current execution
	continueEvent := &ExecutionEvent{
		ID:          NewEventID(),
		ExecutionID: executionID,
		Sequence:    snapshot.LastEventSeq + 1,
		Timestamp:   time.Now(),
		EventType:   EventExecutionContinueAsNew,
		Data: map[string]interface{}{
			"reason":             decision.Reason,
			"new_inputs":         decision.NewInputs,
			"preserved_state":    preservedState,
			"continuation_delay": decision.ContinuationDelay.String(),
		},
	}

	if err := m.eventStore.AppendEvents(ctx, []*ExecutionEvent{continueEvent}); err != nil {
		return "", fmt.Errorf("failed to record continue-as-new event: %w", err)
	}

	// Create new execution ID
	newExecutionID := NewExecutionID()

	// Prepare inputs for new execution
	newInputs := snapshot.Inputs
	if decision.NewInputs != nil {
		newInputs = decision.NewInputs
	}

	// Create new execution snapshot
	newSnapshot := &ExecutionSnapshot{
		ID:           newExecutionID,
		WorkflowName: snapshot.WorkflowName,
		WorkflowHash: snapshot.WorkflowHash,
		InputsHash:   snapshot.InputsHash,
		Status:       "pending",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		LastEventSeq: 0,
		WorkflowData: snapshot.WorkflowData,
		Inputs:       newInputs,
		Outputs:      make(map[string]interface{}),
	}

	// Save new execution snapshot
	if err := m.eventStore.SaveSnapshot(ctx, newSnapshot); err != nil {
		return "", fmt.Errorf("failed to save new execution snapshot: %w", err)
	}

	// Record initial event for new execution with continuation info
	startEvent := &ExecutionEvent{
		ID:          NewEventID(),
		ExecutionID: newExecutionID,
		Sequence:    1,
		Timestamp:   time.Now(),
		EventType:   EventExecutionStarted,
		Data: map[string]interface{}{
			"workflow_name":         snapshot.WorkflowName,
			"inputs":                newInputs,
			"continued_from":        executionID,
			"continuation_reason":   decision.Reason,
			"preserved_state":       preservedState,
			"original_execution_id": getOriginalExecutionID(preservedState, executionID),
		},
	}

	if err := m.eventStore.AppendEvents(ctx, []*ExecutionEvent{startEvent}); err != nil {
		return "", fmt.Errorf("failed to record start event for new execution: %w", err)
	}

	m.logger.Info("continue-as-new completed",
		"original_execution", executionID,
		"new_execution", newExecutionID,
		"reason", decision.Reason)

	return newExecutionID, nil
}

// getExecutionMetrics calculates current execution metrics
func (m *ContinueAsNewManager) getExecutionMetrics(ctx context.Context, executionID string) (*ContinueAsNewMetrics, error) {
	snapshot, err := m.eventStore.GetSnapshot(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}

	events, err := m.eventStore.GetEventHistory(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get event history: %w", err)
	}

	metrics := &ContinueAsNewMetrics{
		EventCount:        int64(len(events)),
		ExecutionDuration: time.Since(snapshot.StartTime),
		TotalEventSize:    calculateEventSize(events),
		ActivePaths:       1, // Simplified - would count actual paths
	}

	if len(events) > 0 {
		metrics.LastEventTime = events[len(events)-1].Timestamp
	}

	return metrics, nil
}

// calculateEventSize estimates the total size of events
func calculateEventSize(events []*ExecutionEvent) int64 {
	var totalSize int64
	for _, event := range events {
		// Rough estimation - in practice you'd serialize and measure
		totalSize += int64(len(event.ID) + len(event.ExecutionID) + len(event.StepName))
		totalSize += 50 // Approximate overhead for other fields
		if event.Data != nil {
			// Very rough estimation of data size
			totalSize += 100 // Placeholder
		}
	}
	return totalSize
}

// getOriginalExecutionID extracts the original execution ID from the chain
func getOriginalExecutionID(preserved *PreservedState, currentExecutionID string) string {
	if preserved != nil && preserved.ContinuationInfo != nil && preserved.ContinuationInfo.OriginalExecutionID != "" {
		return preserved.ContinuationInfo.OriginalExecutionID
	}
	return currentExecutionID
}

// MonitorForContinuation provides continuous monitoring for continue-as-new conditions
func (m *ContinueAsNewManager) MonitorForContinuation(ctx context.Context, executionID string, checkInterval time.Duration) <-chan *ContinueAsNewDecision {
	decisions := make(chan *ContinueAsNewDecision, 1)

	go func() {
		defer close(decisions)
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				decision, err := m.EvaluateContinuation(ctx, executionID)
				if err != nil {
					m.logger.Error("failed to evaluate continuation", "error", err)
					continue
				}

				if decision.ShouldContinue {
					select {
					case decisions <- decision:
					case <-ctx.Done():
						return
					}
					return // Stop monitoring after deciding to continue
				}
			}
		}
	}()

	return decisions
}

// Validate validates continue-as-new options
func (o *ContinueAsNewOptions) Validate() error {
	if o.MaxEvents < 0 {
		return fmt.Errorf("max events cannot be negative")
	}
	if o.MaxDuration < 0 {
		return fmt.Errorf("max duration cannot be negative")
	}
	if o.MaxEventSize < 0 {
		return fmt.Errorf("max event size cannot be negative")
	}
	if o.ContinuationDelay < 0 {
		return fmt.Errorf("continuation delay cannot be negative")
	}
	return nil
}
