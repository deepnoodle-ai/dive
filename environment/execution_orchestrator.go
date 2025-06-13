package environment

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
)

// ExecutionOrchestrator orchestrates execution lifecycle with persistence
type ExecutionOrchestrator struct {
	eventStore  workflow.ExecutionEventStore
	replayer    workflow.ExecutionReplayer
	environment *Environment
	logger      slogger.Logger
}

// NewExecutionOrchestrator creates a new execution orchestrator
func NewExecutionOrchestrator(eventStore workflow.ExecutionEventStore, env *Environment) *ExecutionOrchestrator {
	replayer := workflow.NewBasicExecutionReplayer(env.logger)

	return &ExecutionOrchestrator{
		eventStore:  eventStore,
		replayer:    replayer,
		environment: env,
		logger:      env.logger,
	}
}

// CreateExecution creates a new event-based execution
func (eo *ExecutionOrchestrator) CreateExecution(ctx context.Context, opts ExecutionOptions) (*EventBasedExecution, error) {
	opts.Persistence = &PersistenceConfig{
		EventStore: eo.eventStore,
		BatchSize:  10,
	}

	execution, err := NewEventBasedExecution(eo.environment, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create event-based execution: %w", err)
	}

	eo.logger.Info("created new execution",
		"execution_id", execution.ID(),
		"workflow", opts.WorkflowName,
	)
	return execution, nil
}

// RetryExecution retries an existing execution using the specified strategy
func (eo *ExecutionOrchestrator) RetryExecution(ctx context.Context, executionID string, opts workflow.RetryOptions) (*EventBasedExecution, error) {
	snapshot, err := eo.eventStore.GetSnapshot(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load execution snapshot: %w", err)
	}

	// Load event history
	events, err := eo.eventStore.GetEventHistory(ctx, executionID)
	if err != nil {
		eo.logger.Warn("failed to load event history, using snapshot-only retry", "error", err)
		return eo.retryFromStart(ctx, snapshot, opts.NewInputs)
	}

	// Get workflow definition
	wf, err := eo.getWorkflow(snapshot.WorkflowName)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow definition: %w", err)
	}

	switch opts.Strategy {
	case workflow.RetryFromStart:
		return eo.retryFromStart(ctx, snapshot, opts.NewInputs)
	case workflow.RetryFromFailure:
		return eo.retryFromFailure(ctx, snapshot, events, wf)
	case workflow.RetryWithNewInputs:
		return eo.retryWithNewInputs(ctx, snapshot, events, wf, opts.NewInputs)
	case workflow.RetrySkipFailed:
		return eo.retrySkipFailed(ctx, snapshot, events, wf)
	default:
		return nil, fmt.Errorf("unsupported retry strategy: %s", opts.Strategy)
	}
}

// RecoverExecution recovers an interrupted execution from its last known state
func (eo *ExecutionOrchestrator) RecoverExecution(ctx context.Context, executionID string) (*EventBasedExecution, error) {
	snapshot, err := eo.eventStore.GetSnapshot(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load snapshot: %w", err)
	}
	// Only recover executions that were running when interrupted
	if snapshot.Status != string(StatusRunning) {
		return nil, fmt.Errorf("execution %s is not in recoverable state (status: %s)", executionID, snapshot.Status)
	}
	events, err := eo.eventStore.GetEventHistory(ctx, executionID)
	if err != nil {
		eo.logger.Warn("failed to load event history, attempting snapshot-only recovery", "error", err)
		return eo.recoverFromSnapshotOnly(ctx, snapshot)
	}
	// Validate event history compatibility
	wf, err := eo.getWorkflow(snapshot.WorkflowName)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow definition: %w", err)
	}
	if err := eo.replayer.ValidateEventHistory(ctx, events, wf); err != nil {
		eo.logger.Warn("event history incompatible with current workflow, attempting degraded recovery", "error", err)
		return eo.recoverFromSnapshotOnly(ctx, snapshot)
	}
	// Perform full replay
	return eo.replayFromEvents(ctx, snapshot, events, wf)
}

// ListRecoverableExecutions returns executions that can be recovered
func (eo *ExecutionOrchestrator) ListRecoverableExecutions(ctx context.Context) ([]*workflow.ExecutionSnapshot, error) {
	filter := workflow.ExecutionFilter{
		Status: string(StatusRunning),
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
func (eo *ExecutionOrchestrator) retryFromStart(ctx context.Context, snapshot *workflow.ExecutionSnapshot, newInputs map[string]interface{}) (*EventBasedExecution, error) {
	inputs := snapshot.Inputs
	if newInputs != nil {
		inputs = newInputs
	}
	execution, err := eo.CreateExecution(ctx, ExecutionOptions{
		WorkflowName: snapshot.WorkflowName,
		Inputs:       inputs,
		Outputs:      snapshot.Outputs,
		Logger:       eo.logger,
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
func (eo *ExecutionOrchestrator) retryFromFailure(ctx context.Context, snapshot *workflow.ExecutionSnapshot, events []*workflow.ExecutionEvent, wf *workflow.Workflow) (*EventBasedExecution, error) {
	eo.logger.Info("retrying from failure point",
		"execution_id", snapshot.ID,
		"workflow", snapshot.WorkflowName,
	)

	// Find the failure point in the event history
	failureInfo, err := eo.findFailurePoint(events)
	if err != nil {
		eo.logger.Warn("could not determine failure point, falling back to retry from start", "error", err)
		return eo.retryFromStart(ctx, snapshot, nil)
	}

	eo.logger.Info("found failure point",
		"failed_step", failureInfo.FailedStep,
		"failed_path", failureInfo.FailedPath,
		"failure_sequence", failureInfo.FailureSequence)

	// Validate that we can resume from this point
	if err := eo.validateResumePoint(failureInfo, wf); err != nil {
		eo.logger.Warn("cannot resume from failure point, falling back to retry from start", "error", err)
		return eo.retryFromStart(ctx, snapshot, nil)
	}

	// Replay events up to the failure point to reconstruct state
	preFailureEvents := eo.getEventsBeforeFailure(events, failureInfo.FailureSequence)
	replayResult, err := eo.replayer.ReplayExecution(ctx, preFailureEvents, wf)
	if err != nil {
		return nil, fmt.Errorf("failed to replay execution to failure point: %w", err)
	}

	// Create new execution with replayed state
	newExecution, err := eo.createExecutionFromReplay(ctx, snapshot, replayResult, wf)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution from replay: %w", err)
	}

	// Set up the execution to resume from the failed step
	if err := eo.setupResumeFromFailure(newExecution, failureInfo, replayResult); err != nil {
		return nil, fmt.Errorf("failed to setup resume from failure: %w", err)
	}

	eo.logger.Info("created execution for retry from failure",
		"new_execution_id", newExecution.ID(),
		"resume_step", failureInfo.FailedStep,
		"resume_path", failureInfo.FailedPath)

	return newExecution, nil
}

// retryWithNewInputs implements retry with new inputs
func (eo *ExecutionOrchestrator) retryWithNewInputs(ctx context.Context, snapshot *workflow.ExecutionSnapshot, events []*workflow.ExecutionEvent, wf *workflow.Workflow, newInputs map[string]interface{}) (*EventBasedExecution, error) {
	eo.logger.Info("retrying with new inputs", "execution_id", snapshot.ID)

	if len(newInputs) == 0 {
		return nil, fmt.Errorf("new inputs are required for RetryWithNewInputs strategy")
	}

	// Detect which steps might be affected by input changes
	affectedSteps, err := eo.detectInputAffectedSteps(events, newInputs, wf)
	if err != nil {
		eo.logger.Warn("could not detect affected steps, performing full retry", "error", err)
		return eo.retryFromStart(ctx, snapshot, newInputs)
	}

	eo.logger.Info("detected input-affected steps", "affected_steps", affectedSteps)

	// Find the earliest affected step to determine replay point
	replayFromSequence, err := eo.findEarliestAffectedStep(events, affectedSteps)
	if err != nil {
		eo.logger.Warn("could not find replay point, performing full retry", "error", err)
		return eo.retryFromStart(ctx, snapshot, newInputs)
	}

	// Replay events up to the replay point
	preReplayEvents := eo.getEventsBeforeSequence(events, replayFromSequence)
	replayResult, err := eo.replayer.ReplayExecution(ctx, preReplayEvents, wf)
	if err != nil {
		return nil, fmt.Errorf("failed to replay execution: %w", err)
	}

	// Update inputs in replay result
	replayResult.ScriptGlobals["inputs"] = newInputs

	// Create new execution with updated inputs
	newSnapshot := *snapshot
	newSnapshot.Inputs = newInputs
	newSnapshot.InputsHash = eo.hashInputs(newInputs)

	newExecution, err := eo.createExecutionFromReplay(ctx, &newSnapshot, replayResult, wf)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution from replay: %w", err)
	}

	eo.logger.Info("created execution for retry with new inputs",
		"new_execution_id", newExecution.ID(),
		"replay_from_sequence", replayFromSequence)

	return newExecution, nil
}

// retrySkipFailed implements retry that skips failed steps
func (eo *ExecutionOrchestrator) retrySkipFailed(ctx context.Context, snapshot *workflow.ExecutionSnapshot, events []*workflow.ExecutionEvent, wf *workflow.Workflow) (*EventBasedExecution, error) {
	eo.logger.Info("retrying with skip failed steps", "execution_id", snapshot.ID)

	// Find all failed steps
	failedSteps := eo.findAllFailedSteps(events)
	if len(failedSteps) == 0 {
		return nil, fmt.Errorf("no failed steps found to skip")
	}

	eo.logger.Info("found failed steps to skip", "failed_steps", failedSteps)

	// Replay all events, but mark failed steps as skipped
	replayResult, err := eo.replayWithSkippedSteps(ctx, events, wf, failedSteps)
	if err != nil {
		return nil, fmt.Errorf("failed to replay with skipped steps: %w", err)
	}

	// Create new execution from replay
	newExecution, err := eo.createExecutionFromReplay(ctx, snapshot, replayResult, wf)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution from replay: %w", err)
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
func (eo *ExecutionOrchestrator) findFailurePoint(events []*workflow.ExecutionEvent) (*FailureInfo, error) {
	// Look for the last step failure or execution failure
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]

		switch event.EventType {
		case workflow.EventStepFailed:
			return &FailureInfo{
				FailedStep:      event.StepName,
				FailedPath:      event.PathID,
				FailureSequence: event.Sequence,
				ErrorMessage:    getStringFromData(event.Data, "error"),
				CanResume:       true,
			}, nil
		case workflow.EventExecutionFailed:
			// Find the step that caused execution failure
			for j := i - 1; j >= 0; j-- {
				if events[j].EventType == workflow.EventStepFailed {
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
func (eo *ExecutionOrchestrator) getEventsBeforeFailure(events []*workflow.ExecutionEvent, failureSequence int64) []*workflow.ExecutionEvent {
	var preFailureEvents []*workflow.ExecutionEvent

	for _, event := range events {
		if event.Sequence < failureSequence {
			preFailureEvents = append(preFailureEvents, event)
		}
	}

	return preFailureEvents
}

// getEventsBeforeSequence returns events that occurred before the given sequence
func (eo *ExecutionOrchestrator) getEventsBeforeSequence(events []*workflow.ExecutionEvent, sequence int64) []*workflow.ExecutionEvent {
	var preEvents []*workflow.ExecutionEvent

	for _, event := range events {
		if event.Sequence < sequence {
			preEvents = append(preEvents, event)
		}
	}

	return preEvents
}

// createExecutionFromReplay creates a new execution from replay results
func (eo *ExecutionOrchestrator) createExecutionFromReplay(ctx context.Context, snapshot *workflow.ExecutionSnapshot, replayResult *workflow.ReplayResult, wf *workflow.Workflow) (*EventBasedExecution, error) {
	// Create new execution with fresh ID
	newExecution, err := NewEventBasedExecution(eo.environment, ExecutionOptions{
		WorkflowName: snapshot.WorkflowName,
		Inputs:       snapshot.Inputs,
		Logger:       eo.logger,
		Persistence: &PersistenceConfig{
			EventStore: eo.eventStore,
			BatchSize:  10,
		},
	})
	if err != nil {
		return nil, err
	}

	// Apply replayed state
	newExecution.replayMode = true // Prevent event recording during state setup

	// Restore script globals
	for key, value := range replayResult.ScriptGlobals {
		newExecution.scriptGlobals[key] = value
	}

	// Restore completed steps state (this would need integration with the actual execution state)
	// For now, we'll rely on the script globals containing the necessary state

	newExecution.replayMode = false // Re-enable event recording

	return newExecution, nil
}

// setupResumeFromFailure prepares execution to resume from a failed step
func (eo *ExecutionOrchestrator) setupResumeFromFailure(execution *EventBasedExecution, failureInfo *FailureInfo, replayResult *workflow.ReplayResult) error {
	// Find the active path that needs to resume
	var resumePath *workflow.ReplayPathState
	for _, path := range replayResult.ActivePaths {
		if path.ID == failureInfo.FailedPath {
			resumePath = path
			break
		}
	}

	if resumePath == nil {
		return fmt.Errorf("failed path %s not found in active paths", failureInfo.FailedPath)
	}

	// Set up path state for resume (this would need integration with actual execution paths)
	// For now, we'll ensure the script globals contain the failure information
	execution.scriptGlobals["__resume_from_failure"] = map[string]interface{}{
		"failed_step": failureInfo.FailedStep,
		"failed_path": failureInfo.FailedPath,
		"error":       failureInfo.ErrorMessage,
	}

	eo.logger.Info("setup resume from failure",
		"step", failureInfo.FailedStep,
		"path", failureInfo.FailedPath)

	return nil
}

// detectInputAffectedSteps determines which steps might be affected by input changes
func (eo *ExecutionOrchestrator) detectInputAffectedSteps(events []*workflow.ExecutionEvent, newInputs map[string]interface{}, wf *workflow.Workflow) ([]string, error) {
	// For now, conservatively assume all steps might be affected
	// Future enhancement: analyze step dependencies and input usage
	var allSteps []string
	for _, step := range wf.Steps() {
		allSteps = append(allSteps, step.Name())
	}
	return allSteps, nil
}

// findEarliestAffectedStep finds the earliest step that might be affected by changes
func (eo *ExecutionOrchestrator) findEarliestAffectedStep(events []*workflow.ExecutionEvent, affectedSteps []string) (int64, error) {
	affectedStepMap := make(map[string]bool)
	for _, step := range affectedSteps {
		affectedStepMap[step] = true
	}

	for _, event := range events {
		if event.EventType == workflow.EventStepStarted {
			if affectedStepMap[event.StepName] {
				return event.Sequence, nil
			}
		}
	}

	return 0, fmt.Errorf("no affected steps found in event history")
}

// findAllFailedSteps returns all steps that failed during execution
func (eo *ExecutionOrchestrator) findAllFailedSteps(events []*workflow.ExecutionEvent) []string {
	var failedSteps []string

	for _, event := range events {
		if event.EventType == workflow.EventStepFailed {
			failedSteps = append(failedSteps, event.StepName)
		}
	}

	return failedSteps
}

// replayWithSkippedSteps replays events while treating specified steps as skipped
func (eo *ExecutionOrchestrator) replayWithSkippedSteps(ctx context.Context, events []*workflow.ExecutionEvent, wf *workflow.Workflow, skippedSteps []string) (*workflow.ReplayResult, error) {
	// Create a modified event list that converts failed steps to completed steps
	skippedStepMap := make(map[string]bool)
	for _, step := range skippedSteps {
		skippedStepMap[step] = true
	}

	var modifiedEvents []*workflow.ExecutionEvent
	for _, event := range events {
		if event.EventType == workflow.EventStepFailed && skippedStepMap[event.StepName] {
			// Convert failed step to completed step with placeholder output
			completedEvent := &workflow.ExecutionEvent{
				ID:          event.ID + "_skipped",
				ExecutionID: event.ExecutionID,
				PathID:      event.PathID,
				Sequence:    event.Sequence,
				Timestamp:   event.Timestamp,
				EventType:   workflow.EventStepCompleted,
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

	// Replay with modified events
	return eo.replayer.ReplayExecution(ctx, modifiedEvents, wf)
}

// hashInputs creates a hash of the inputs for change detection
func (eo *ExecutionOrchestrator) hashInputs(inputs map[string]interface{}) string {
	// Simple implementation - in production this should be more robust
	data, _ := json.Marshal(inputs)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes for brevity
}

// Helper function for extracting string data from event data maps
func getStringFromData(data map[string]interface{}, key string) string {
	if value, ok := data[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

// replayFromEvents performs full replay from event history
func (eo *ExecutionOrchestrator) replayFromEvents(ctx context.Context, snapshot *workflow.ExecutionSnapshot, events []*workflow.ExecutionEvent, wf *workflow.Workflow) (*EventBasedExecution, error) {
	eo.logger.Info("performing full replay", "execution_id", snapshot.ID, "event_count", len(events))

	// Replay events to reconstruct state
	replayResult, err := eo.replayer.ReplayExecution(ctx, events, wf)
	if err != nil {
		return nil, fmt.Errorf("failed to replay execution: %w", err)
	}

	// Create new execution with replayed state
	execution, err := LoadFromSnapshot(ctx, eo.environment, snapshot, eo.eventStore)
	if err != nil {
		return nil, fmt.Errorf("failed to load from snapshot: %w", err)
	}

	// Apply replayed state
	execution.replayMode = true // Set replay mode to prevent duplicate events

	// Restore script globals
	for key, value := range replayResult.ScriptGlobals {
		execution.scriptGlobals[key] = value
	}

	execution.replayMode = false // Re-enable event recording

	eo.logger.Info("replay completed successfully",
		"execution_id", snapshot.ID,
		"completed_steps", len(replayResult.CompletedSteps),
		"active_paths", len(replayResult.ActivePaths))

	return execution, nil
}

// getWorkflow retrieves a workflow by name
func (eo *ExecutionOrchestrator) getWorkflow(workflowName string) (*workflow.Workflow, error) {
	wf, exists := eo.environment.workflows[workflowName]
	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", workflowName)
	}
	return wf, nil
}

// computeWorkflowHash computes a hash of the workflow definition for change detection
func (eo *ExecutionOrchestrator) computeWorkflowHash(wf *workflow.Workflow) (string, error) {
	// Serialize workflow to JSON for hashing
	data, err := json.Marshal(map[string]interface{}{
		"name":        wf.Name(),
		"description": wf.Description(),
		"inputs":      wf.Inputs(),
		"steps":       wf.Steps(),
		"output":      wf.Output(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to serialize workflow: %w", err)
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// computeInputsHash computes a hash of the input parameters for change detection
func (eo *ExecutionOrchestrator) computeInputsHash(inputs map[string]interface{}) (string, error) {
	data, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("failed to serialize inputs: %w", err)
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// recoverFromSnapshotOnly recovers using only snapshot data (degraded mode)
func (eo *ExecutionOrchestrator) recoverFromSnapshotOnly(ctx context.Context, snapshot *workflow.ExecutionSnapshot) (*EventBasedExecution, error) {
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
