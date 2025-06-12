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
	config := &PersistenceConfig{
		EventStore: eo.eventStore,
		BatchSize:  10,
	}

	execution, err := NewEventBasedExecution(eo.environment, opts, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create event-based execution: %w", err)
	}

	eo.logger.Info("created new execution", "execution_id", execution.ID(), "workflow", opts.WorkflowName)
	return execution, nil
}

// RetryExecution retries an existing execution using the specified strategy
func (eo *ExecutionOrchestrator) RetryExecution(ctx context.Context, executionID string, opts workflow.RetryOptions) (*EventBasedExecution, error) {
	snapshot, err := eo.eventStore.GetSnapshot(ctx, executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load execution snapshot: %w", err)
	}

	eo.logger.Info("retrying execution",
		"execution_id", executionID,
		"strategy", opts.Strategy,
		"workflow", snapshot.WorkflowName)

	switch opts.Strategy {
	case workflow.RetryFromStart:
		return eo.retryFromStart(ctx, snapshot, opts.NewInputs)
	case workflow.RetryFromFailure:
		return eo.retryFromFailure(ctx, snapshot)
	case workflow.RetryWithNewInputs:
		return eo.retryWithNewInputs(ctx, snapshot, opts.NewInputs)
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
	workflow, err := eo.getWorkflow(snapshot.WorkflowName)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow definition: %w", err)
	}

	if err := eo.replayer.ValidateEventHistory(ctx, events, workflow); err != nil {
		eo.logger.Warn("event history incompatible with current workflow, attempting degraded recovery", "error", err)
		return eo.recoverFromSnapshotOnly(ctx, snapshot)
	}

	// Perform full replay
	return eo.replayFromEvents(ctx, snapshot, events, workflow)
}

// ListRecoverableExecutions returns executions that can be recovered
func (eo *ExecutionOrchestrator) ListRecoverableExecutions(ctx context.Context) ([]*workflow.ExecutionSnapshot, error) {
	status := string(StatusRunning)
	filter := workflow.ExecutionFilter{
		Status: &status,
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

	opts := ExecutionOptions{
		WorkflowName: snapshot.WorkflowName,
		Inputs:       inputs,
		Outputs:      snapshot.Outputs,
		Logger:       eo.logger,
	}

	execution, err := eo.CreateExecution(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create retry execution: %w", err)
	}

	eo.logger.Info("created retry from start",
		"original_execution", snapshot.ID,
		"new_execution", execution.ID())

	return execution, nil
}

// retryFromFailure attempts to resume execution from the point of failure
func (eo *ExecutionOrchestrator) retryFromFailure(ctx context.Context, snapshot *workflow.ExecutionSnapshot) (*EventBasedExecution, error) {
	// TODO: Load event history to find failure point and implement actual resume
	// events, err := eo.eventStore.GetEventHistory(ctx, snapshot.ID)
	// if err != nil {
	//     return nil, fmt.Errorf("failed to load event history for retry: %w", err)
	// }

	// For now, implement this as retry from start
	// TODO: Implement actual resume from failure point
	eo.logger.Warn("retry from failure not fully implemented, falling back to retry from start")
	return eo.retryFromStart(ctx, snapshot, nil)
}

// retryWithNewInputs creates a new execution with updated inputs
func (eo *ExecutionOrchestrator) retryWithNewInputs(ctx context.Context, snapshot *workflow.ExecutionSnapshot, newInputs map[string]interface{}) (*EventBasedExecution, error) {
	if newInputs == nil || len(newInputs) == 0 {
		return nil, fmt.Errorf("new inputs are required for retry with new inputs strategy")
	}

	return eo.retryFromStart(ctx, snapshot, newInputs)
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
