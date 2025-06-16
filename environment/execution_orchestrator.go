package environment

import (
	"context"
	"fmt"

	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
)

// ExecutionOrchestrator orchestrates execution lifecycle with persistence
type ExecutionOrchestrator struct {
	eventStore  ExecutionEventStore
	environment *Environment
	logger      slogger.Logger
}

// NewExecutionOrchestrator creates a new execution orchestrator
func NewExecutionOrchestrator(eventStore ExecutionEventStore, env *Environment) *ExecutionOrchestrator {
	return &ExecutionOrchestrator{
		eventStore:  eventStore,
		environment: env,
		logger:      env.logger,
	}
}

// CreateExecution creates a new execution
func (eo *ExecutionOrchestrator) CreateExecution(ctx context.Context, opts ExecutionOptions) (*Execution, error) {
	execution, err := NewExecution(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create event-based execution: %w", err)
	}
	eo.logger.Info("created new execution",
		"execution_id", execution.ID(),
		"workflow", opts.Workflow.Name(),
	)
	return execution, nil
}

// RetryExecution retries an existing execution using the specified strategy
func (eo *ExecutionOrchestrator) RetryExecution(ctx context.Context, executionID string, opts RetryOptions) (*Execution, error) {
	snapshot, err := eo.eventStore.GetSnapshot(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load execution snapshot: %w", err)
	}

	// Load event history
	events, err := eo.eventStore.GetEventHistory(ctx, executionID)
	if err != nil {
		eo.logger.Warn("failed to load event history, using snapshot-only retry", "error", err)
		return eo.retryFromStart(ctx, snapshot)
	}

	// Get workflow definition
	wf, err := eo.getWorkflow(snapshot.WorkflowName)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow definition: %w", err)
	}

	switch opts.Strategy {
	case RetryFromStart:
		return eo.retryFromStart(ctx, snapshot)
	case RetryFromFailure:
		return eo.retryFromFailure(ctx, snapshot, events, wf)
	case RetrySkipFailed:
		return eo.retrySkipFailed(ctx, snapshot, events, wf)
	default:
		return nil, fmt.Errorf("unsupported retry strategy: %s", opts.Strategy)
	}
}

// RecoverExecution recovers an interrupted execution from its last known state
func (eo *ExecutionOrchestrator) RecoverExecution(ctx context.Context, executionID string) (*Execution, error) {
	snapshot, err := eo.eventStore.GetSnapshot(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load snapshot: %w", err)
	}

	// Only recover executions that were running when interrupted
	if snapshot.Status != string(ExecutionStatusRunning) {
		return nil, fmt.Errorf("execution %s is not in recoverable state (status: %s)", executionID, snapshot.Status)
	}
	events, err := eo.eventStore.GetEventHistory(ctx, executionID)
	if err != nil {
		eo.logger.Warn("failed to load event history, attempting snapshot-only recovery", "error", err)
		return eo.recoverFromSnapshotOnly(ctx, snapshot)
	}

	// Create execution from snapshot
	execution, err := LoadFromSnapshot(ctx, eo.environment, snapshot, eo.eventStore)
	if err != nil {
		return nil, fmt.Errorf("failed to load from snapshot: %w", err)
	}

	// Validate event history compatibility and replay
	if err := execution.ValidateEventHistory(ctx, events); err != nil {
		eo.logger.Warn("event history incompatible with current workflow, attempting degraded recovery", "error", err)
		return eo.recoverFromSnapshotOnly(ctx, snapshot)
	}

	// Perform full replay
	if err := execution.ReplayFromEvents(ctx, events); err != nil {
		return nil, fmt.Errorf("failed to replay events: %w", err)
	}

	return execution, nil
}

// ListRecoverableExecutions returns executions that can be recovered
func (eo *ExecutionOrchestrator) ListRecoverableExecutions(ctx context.Context) ([]*ExecutionSnapshot, error) {
	filter := ExecutionFilter{
		Status: string(ExecutionStatusRunning),
		Limit:  100, // Reasonable limit for recoverable executions
	}
	executions, err := eo.eventStore.ListExecutions(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list executions: %w", err)
	}
	eo.logger.Info("found recoverable executions", "count", len(executions))
	return executions, nil
}

// retryFromStart creates a new execution with the same workflow and inputs
func (eo *ExecutionOrchestrator) retryFromStart(ctx context.Context, snapshot *ExecutionSnapshot) (*Execution, error) {
	// Get workflow definition
	wf, err := eo.getWorkflow(snapshot.WorkflowName)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow definition: %w", err)
	}

	execution, err := eo.CreateExecution(ctx, ExecutionOptions{
		Workflow:    wf,
		Environment: eo.environment,
		Inputs:      snapshot.Inputs,
		EventStore:  eo.eventStore,
		Logger:      eo.logger,
		ReplayMode:  false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create retry execution: %w", err)
	}
	eo.logger.Info("created retry from start",
		"original_execution", snapshot.ID,
		"new_execution", execution.ID())
	return execution, nil
}

// retryFromFailure implements intelligent retry from the point of failure
func (eo *ExecutionOrchestrator) retryFromFailure(ctx context.Context, snapshot *ExecutionSnapshot, events []*ExecutionEvent, wf *workflow.Workflow) (*Execution, error) {
	eo.logger.Info("retrying from failure point",
		"execution_id", snapshot.ID,
		"workflow", snapshot.WorkflowName,
	)

	// Find the failure point in the event history
	failureInfo, err := eo.findFailurePoint(events)
	if err != nil {
		eo.logger.Warn("could not determine failure point, falling back to retry from start", "error", err)
		return eo.retryFromStart(ctx, snapshot)
	}
	eo.logger.Info("found failure point",
		"failed_step", failureInfo.FailedStep,
		"failed_path", failureInfo.FailedPath,
		"failure_sequence", failureInfo.FailureSequence)

	// Validate that we can resume from this point
	if err := eo.validateResumePoint(failureInfo, wf); err != nil {
		eo.logger.Warn("cannot resume from failure point, falling back to retry from start", "error", err)
		return eo.retryFromStart(ctx, snapshot)
	}

	// Create new execution
	newExecution, err := NewExecution(ExecutionOptions{
		Environment: eo.environment,
		Workflow:    wf,
		Inputs:      snapshot.Inputs,
		EventStore:  eo.eventStore,
		Logger:      eo.logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	// Replay events up to the failure point to reconstruct state
	preFailureEvents := eo.getEventsBeforeFailure(events, failureInfo.FailureSequence)
	if err := newExecution.ReplayFromEvents(ctx, preFailureEvents); err != nil {
		return nil, fmt.Errorf("failed to replay execution to failure point: %w", err)
	}
	// Set up the execution to resume from the failed step
	if err := eo.setupResumeFromFailure(newExecution, failureInfo); err != nil {
		return nil, fmt.Errorf("failed to setup resume from failure: %w", err)
	}

	eo.logger.Info("created execution for retry from failure",
		"new_execution_id", newExecution.ID(),
		"resume_step", failureInfo.FailedStep,
		"resume_path", failureInfo.FailedPath)

	return newExecution, nil
}

// retrySkipFailed implements retry that skips failed steps
func (eo *ExecutionOrchestrator) retrySkipFailed(ctx context.Context, snapshot *ExecutionSnapshot, events []*ExecutionEvent, wf *workflow.Workflow) (*Execution, error) {
	eo.logger.Info("retrying with skip failed steps", "execution_id", snapshot.ID)

	// Find all failed steps
	failedSteps := eo.findAllFailedSteps(events)
	if len(failedSteps) == 0 {
		return nil, fmt.Errorf("no failed steps found to skip")
	}

	eo.logger.Info("found failed steps to skip", "failed_steps", failedSteps)

	// Create new execution
	newExecution, err := NewExecution(ExecutionOptions{
		Environment: eo.environment,
		Workflow:    wf,
		Inputs:      snapshot.Inputs,
		EventStore:  eo.eventStore,
		Logger:      eo.logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	// Replay all events, but mark failed steps as skipped
	skippedEvents, err := eo.createSkippedEvents(events, failedSteps)
	if err != nil {
		return nil, fmt.Errorf("failed to create skipped events: %w", err)
	}

	if err := newExecution.ReplayFromEvents(ctx, skippedEvents); err != nil {
		return nil, fmt.Errorf("failed to replay with skipped steps: %w", err)
	}

	eo.logger.Info("created execution for skip failed retry",
		"new_execution_id", newExecution.ID(),
		"skipped_steps", len(failedSteps))

	return newExecution, nil
}

// FailureInfo contains information about a failure point in execution
type FailureInfo struct {
	FailedStep      string
	FailedPath      string
	FailureSequence int64
	ErrorMessage    string
	CanResume       bool
}

// findFailurePoint analyzes events to find the failure point
func (eo *ExecutionOrchestrator) findFailurePoint(events []*ExecutionEvent) (*FailureInfo, error) {
	// Look for the last step failure or execution failure
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]

		switch event.EventType {
		case EventStepFailed:
			return &FailureInfo{
				FailedStep:      event.StepName,
				FailedPath:      event.PathID,
				FailureSequence: event.Sequence,
				ErrorMessage:    getStringFromData(event.Data, "error"),
				CanResume:       true,
			}, nil
		case EventExecutionFailed:
			// Find the step that caused execution failure
			for j := i - 1; j >= 0; j-- {
				if events[j].EventType == EventStepFailed {
					return &FailureInfo{
						FailedStep:      events[j].StepName,
						FailedPath:      events[j].PathID,
						FailureSequence: events[j].Sequence,
						ErrorMessage:    getStringFromData(events[j].Data, "error"),
						CanResume:       true,
					}, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no failure point found in event history")
}

// validateResumePoint checks if we can resume from the given failure point
func (eo *ExecutionOrchestrator) validateResumePoint(failureInfo *FailureInfo, wf *workflow.Workflow) error {
	// Check if the failed step still exists in workflow
	step, exists := wf.Graph().Get(failureInfo.FailedStep)
	if !exists || step == nil {
		return fmt.Errorf("failed step %s no longer exists in workflow", failureInfo.FailedStep)
	}

	// For now, assume all steps can be resumed
	// Future enhancements could add step-specific resume validation
	return nil
}

// getEventsBeforeFailure returns events that occurred before the failure
func (eo *ExecutionOrchestrator) getEventsBeforeFailure(events []*ExecutionEvent, failureSequence int64) []*ExecutionEvent {
	var preFailureEvents []*ExecutionEvent

	for _, event := range events {
		if event.Sequence < failureSequence {
			preFailureEvents = append(preFailureEvents, event)
		}
	}

	return preFailureEvents
}

// setupResumeFromFailure prepares execution to resume from a failed step
func (eo *ExecutionOrchestrator) setupResumeFromFailure(execution *Execution, failureInfo *FailureInfo) error {
	// Set up state for resume - the execution already has the replayed state
	// We just need to mark where to resume from
	execution.state.Set("__resume_from_failure", map[string]interface{}{
		"failed_step": failureInfo.FailedStep,
		"failed_path": failureInfo.FailedPath,
		"error":       failureInfo.ErrorMessage,
	})

	eo.logger.Info("setup resume from failure",
		"step", failureInfo.FailedStep,
		"path", failureInfo.FailedPath)

	return nil
}

// findAllFailedSteps returns all steps that failed during execution
func (eo *ExecutionOrchestrator) findAllFailedSteps(events []*ExecutionEvent) []string {
	var failedSteps []string

	for _, event := range events {
		if event.EventType == EventStepFailed {
			failedSteps = append(failedSteps, event.StepName)
		}
	}

	return failedSteps
}

// createSkippedEvents creates a modified event list that treats failed steps as completed
func (eo *ExecutionOrchestrator) createSkippedEvents(events []*ExecutionEvent, skippedSteps []string) ([]*ExecutionEvent, error) {
	skippedStepMap := make(map[string]bool)
	for _, step := range skippedSteps {
		skippedStepMap[step] = true
	}

	var modifiedEvents []*ExecutionEvent
	for _, event := range events {
		if event.EventType == EventStepFailed && skippedStepMap[event.StepName] {
			// Convert failed step to completed step with placeholder output
			completedEvent := &ExecutionEvent{
				ID:          event.ID + "_skipped",
				ExecutionID: event.ExecutionID,
				PathID:      event.PathID,
				Sequence:    event.Sequence,
				Timestamp:   event.Timestamp,
				EventType:   EventStepCompleted,
				StepName:    event.StepName,
				Data: map[string]interface{}{
					"output":         "SKIPPED: " + getStringFromData(event.Data, "error"),
					"skipped":        true,
					"original_error": getStringFromData(event.Data, "error"),
				},
			}
			modifiedEvents = append(modifiedEvents, completedEvent)
		} else {
			modifiedEvents = append(modifiedEvents, event)
		}
	}

	return modifiedEvents, nil
}

// replayFromEvents performs full replay from event history (method removed - functionality moved to Execution.ReplayFromEvents)

// getWorkflow retrieves a workflow by name
func (eo *ExecutionOrchestrator) getWorkflow(workflowName string) (*workflow.Workflow, error) {
	wf, exists := eo.environment.workflows[workflowName]
	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", workflowName)
	}
	return wf, nil
}

// recoverFromSnapshotOnly recovers using only snapshot data (degraded mode)
func (eo *ExecutionOrchestrator) recoverFromSnapshotOnly(ctx context.Context, snapshot *ExecutionSnapshot) (*Execution, error) {
	eo.logger.Warn("recovering from snapshot only (degraded mode)", "execution_id", snapshot.ID)

	// Load execution from snapshot
	execution, err := LoadFromSnapshot(ctx, eo.environment, snapshot, eo.eventStore)
	if err != nil {
		return nil, fmt.Errorf("failed to load from snapshot: %w", err)
	}

	// Mark as recovered but in degraded state
	execution.logger.Warn("execution recovered in degraded mode - event history may be incomplete")

	return execution, nil
}
