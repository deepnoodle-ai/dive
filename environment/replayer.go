package environment

import (
	"context"
	"fmt"

	"github.com/diveagents/dive/workflow"
)

// ReplayResult contains the result of replaying an execution history
type ReplayResult struct {
	CompletedSteps map[string]string  `json:"completed_steps"` // step_name -> output
	ActivePaths    []*ReplayPathState `json:"active_paths"`    // currently running paths
	ScriptGlobals  map[string]any     `json:"script_globals"`  // variables in scope
	Status         string             `json:"status"`          // final status if execution complete
}

// ReplayPathState represents the state of a path during replay
type ReplayPathState struct {
	ID              string            `json:"id"`
	CurrentStepName string            `json:"current_step_name"`
	StepOutputs     map[string]string `json:"step_outputs"`
}

// ExecutionReplayer handles event history replay for state reconstruction
type ExecutionReplayer interface {
	ReplayExecution(ctx context.Context, events []*ExecutionEvent, workflow *workflow.Workflow) (*ReplayResult, error)
	ValidateEventHistory(ctx context.Context, events []*ExecutionEvent, workflow *workflow.Workflow) error
}

// BasicExecutionReplayer provides a basic implementation of ExecutionReplayer
type BasicExecutionReplayer struct {
	logger interface {
		Info(msg string, keysAndValues ...interface{})
		Error(msg string, keysAndValues ...interface{})
	}
}

// NewBasicExecutionReplayer creates a new basic execution replayer
func NewBasicExecutionReplayer(logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}) *BasicExecutionReplayer {
	return &BasicExecutionReplayer{
		logger: logger,
	}
}

// ReplayExecution replays an event history to reconstruct execution state
func (r *BasicExecutionReplayer) ReplayExecution(ctx context.Context, events []*ExecutionEvent, workflow *workflow.Workflow) (*ReplayResult, error) {
	result := &ReplayResult{
		CompletedSteps: make(map[string]string),
		ScriptGlobals:  make(map[string]any),
		ActivePaths:    make([]*ReplayPathState, 0),
		Status:         "running",
	}

	// Track active paths during replay
	activePaths := make(map[string]*ReplayPathState)

	// Track workflow state for proper reconstruction
	pathHistory := make(map[string][]*ExecutionEvent) // Track events per path
	stepExecutionOrder := make([]string, 0)           // Track order of step execution

	r.logger.Info("starting replay", "event_count", len(events))

	// First pass: collect and organize events by path
	for _, event := range events {
		if event.PathID != "" {
			pathHistory[event.PathID] = append(pathHistory[event.PathID], event)
		}
	}

	// Second pass: replay events in sequence with enhanced logic
	for i, event := range events {
		if err := r.replayEventEnhanced(ctx, event, result, activePaths, &stepExecutionOrder); err != nil {
			r.logger.Error("replay failed", "event_sequence", event.Sequence, "error", err)
			return nil, fmt.Errorf("replay failed at event %d: %w", i, err)
		}
	}

	// Convert active paths map to slice and validate state
	for pathID, pathState := range activePaths {
		// Ensure path state is consistent
		if err := r.validatePathState(pathState, workflow); err != nil {
			r.logger.Error("invalid path state after replay", "path_id", pathID, "error", err)
			// Continue with warning rather than failing completely
		}
		result.ActivePaths = append(result.ActivePaths, pathState)
	}

	// Reconstruct script globals from step outputs and stored variables
	if err := r.reconstructScriptGlobals(result, events, workflow); err != nil {
		r.logger.Error("failed to reconstruct script globals", "error", err)
		// Continue - this is not fatal for basic replay
	}

	r.logger.Info("replay completed",
		"completed_steps", len(result.CompletedSteps),
		"active_paths", len(result.ActivePaths),
		"status", result.Status,
		"script_variables", len(result.ScriptGlobals))

	return result, nil
}

// replayEventEnhanced processes a single event with enhanced logic for complex scenarios
func (r *BasicExecutionReplayer) replayEventEnhanced(ctx context.Context, event *ExecutionEvent, result *ReplayResult, activePaths map[string]*ReplayPathState, stepOrder *[]string) error {
	switch event.EventType {
	case EventExecutionStarted:
		result.Status = "running"
		// Initialize script globals with inputs if available
		if inputs, ok := event.Data["inputs"].(map[string]interface{}); ok {
			result.ScriptGlobals["inputs"] = inputs
		}
		r.logger.Info("execution started", "execution_id", event.ExecutionID)

	case EventPathStarted:
		pathState := &ReplayPathState{
			ID:              event.PathID,
			CurrentStepName: getStringFromData(event.Data, "current_step"),
			StepOutputs:     make(map[string]string),
		}
		activePaths[event.PathID] = pathState
		r.logger.Info("path started", "path_id", event.PathID, "step", pathState.CurrentStepName)

	case EventStepStarted:
		if pathState, exists := activePaths[event.PathID]; exists {
			pathState.CurrentStepName = event.StepName
		}
		*stepOrder = append(*stepOrder, event.StepName)
		r.logger.Info("step started", "path_id", event.PathID, "step", event.StepName)

	case EventStepCompleted:
		stepOutput := getStringFromData(event.Data, "output")
		result.CompletedSteps[event.StepName] = stepOutput

		// Update path state with step output
		if pathState, exists := activePaths[event.PathID]; exists {
			pathState.StepOutputs[event.StepName] = stepOutput
		}

		// Handle stored variables with proper type preservation
		if varName := getStringFromData(event.Data, "stored_variable"); varName != "" {
			// Try to preserve the original data type if available
			if storedValue, ok := event.Data["stored_value"]; ok {
				result.ScriptGlobals[varName] = storedValue
			} else {
				result.ScriptGlobals[varName] = stepOutput
			}
		}

		// Handle conditional step results
		if condition := getStringFromData(event.Data, "condition_result"); condition == "true" || condition == "false" {
			result.ScriptGlobals[event.StepName+"_condition"] = condition == "true"
		}

		// Handle each block iteration state
		if iterationData, ok := event.Data["iteration_state"]; ok {
			if iterMap, ok := iterationData.(map[string]interface{}); ok {
				result.ScriptGlobals[event.StepName+"_iteration"] = iterMap
			}
		}

		r.logger.Info("step completed", "path_id", event.PathID, "step", event.StepName)

	case EventStepFailed:
		errorMsg := getStringFromData(event.Data, "error")

		// Mark the step as failed but continue replay
		result.CompletedSteps[event.StepName+"_error"] = errorMsg

		// Update path state
		if pathState, exists := activePaths[event.PathID]; exists {
			pathState.StepOutputs[event.StepName+"_error"] = errorMsg
		}

		r.logger.Info("step failed", "path_id", event.PathID, "step", event.StepName, "error", errorMsg)

	case EventPathBranched:
		// Enhanced path branching handling
		if newPathsData, ok := event.Data["new_paths"]; ok {
			if err := r.handlePathBranching(event, newPathsData, activePaths, result); err != nil {
				return fmt.Errorf("failed to handle path branching: %w", err)
			}
		}
		r.logger.Info("path branched", "parent_path", event.PathID, "step", event.StepName)

	case EventPathCompleted:
		// Before removing, capture final path state
		if _, exists := activePaths[event.PathID]; exists {
			// Store any final path outputs
			if finalOutput := getStringFromData(event.Data, "final_output"); finalOutput != "" {
				result.ScriptGlobals["path_"+event.PathID+"_output"] = finalOutput
			}
		}
		delete(activePaths, event.PathID)
		r.logger.Info("path completed", "path_id", event.PathID)

	case EventPathFailed:
		// Capture failure reason before removing path
		if _, exists := activePaths[event.PathID]; exists {
			if failureReason := getStringFromData(event.Data, "failure_reason"); failureReason != "" {
				result.ScriptGlobals["path_"+event.PathID+"_error"] = failureReason
			}
		}
		delete(activePaths, event.PathID)
		r.logger.Info("path failed", "path_id", event.PathID)

	case EventExecutionCompleted:
		result.Status = "completed"
		// Capture final execution outputs
		if outputs, ok := event.Data["outputs"]; ok {
			result.ScriptGlobals["outputs"] = outputs
		}
		r.logger.Info("execution completed", "execution_id", event.ExecutionID)

	case EventExecutionFailed:
		result.Status = "failed"
		if errorMsg := getStringFromData(event.Data, "error"); errorMsg != "" {
			result.ScriptGlobals["execution_error"] = errorMsg
		}
		r.logger.Info("execution failed", "execution_id", event.ExecutionID)

	default:
		r.logger.Info("unknown event type", "event_type", event.EventType, "event_id", event.ID)
	}

	return nil
}

// handlePathBranching processes path branching events with enhanced logic
func (r *BasicExecutionReplayer) handlePathBranching(event *ExecutionEvent, newPathsData interface{}, activePaths map[string]*ReplayPathState, result *ReplayResult) error {
	switch pathsData := newPathsData.(type) {
	case []interface{}:
		// Standard array format
		for _, pathData := range pathsData {
			if pathMap, ok := pathData.(map[string]interface{}); ok {
				if err := r.createBranchedPath(pathMap, activePaths, event.PathID); err != nil {
					return err
				}
			}
		}
	case map[string]interface{}:
		// Single path format
		if err := r.createBranchedPath(pathsData, activePaths, event.PathID); err != nil {
			return err
		}
	default:
		r.logger.Error("unrecognized path branching data format", "type", fmt.Sprintf("%T", pathsData))
		return fmt.Errorf("unrecognized path branching data format")
	}

	return nil
}

// createBranchedPath creates a new path state from branching data
func (r *BasicExecutionReplayer) createBranchedPath(pathMap map[string]interface{}, activePaths map[string]*ReplayPathState, parentPathID string) error {
	pathID := getStringFromMap(pathMap, "id")
	if pathID == "" {
		return fmt.Errorf("missing path ID in branch data")
	}

	currentStep := getStringFromMap(pathMap, "current_step")

	newPathState := &ReplayPathState{
		ID:              pathID,
		CurrentStepName: currentStep,
		StepOutputs:     make(map[string]string),
	}

	// Inherit outputs from parent path if specified
	if parentPath, exists := activePaths[parentPathID]; exists {
		if inherit, ok := pathMap["inherit_outputs"].(bool); ok && inherit {
			for stepName, output := range parentPath.StepOutputs {
				newPathState.StepOutputs[stepName] = output
			}
		}
	}

	activePaths[pathID] = newPathState
	return nil
}

// validatePathState ensures a path state is consistent with workflow definition
func (r *BasicExecutionReplayer) validatePathState(pathState *ReplayPathState, workflow *workflow.Workflow) error {
	if pathState.CurrentStepName == "" {
		return nil // Empty step name is valid for completed paths
	}

	// Check if current step exists in workflow
	step, exists := workflow.Graph().Get(pathState.CurrentStepName)
	if !exists || step == nil {
		return fmt.Errorf("current step %s not found in workflow", pathState.CurrentStepName)
	}

	return nil
}

// reconstructScriptGlobals rebuilds script globals from event history
func (r *BasicExecutionReplayer) reconstructScriptGlobals(result *ReplayResult, events []*ExecutionEvent, workflow *workflow.Workflow) error {
	// Initialize with workflow inputs if available
	for _, event := range events {
		if event.EventType == EventExecutionStarted {
			if inputs, ok := event.Data["inputs"].(map[string]interface{}); ok {
				result.ScriptGlobals["inputs"] = inputs
			}
			break
		}
	}

	// Reconstruct variables from step completions in execution order
	for _, event := range events {
		if event.EventType == EventStepCompleted {
			// Restore stored variables
			if varName := getStringFromData(event.Data, "stored_variable"); varName != "" {
				if storedValue, ok := event.Data["stored_value"]; ok {
					result.ScriptGlobals[varName] = storedValue
				} else {
					result.ScriptGlobals[varName] = getStringFromData(event.Data, "output")
				}
			}

			// Restore step-specific globals
			if globals, ok := event.Data["script_globals"].(map[string]interface{}); ok {
				for key, value := range globals {
					result.ScriptGlobals[key] = value
				}
			}
		}
	}

	return nil
}

// ValidateEventHistory validates an event history for compatibility with a workflow
func (r *BasicExecutionReplayer) ValidateEventHistory(ctx context.Context, events []*ExecutionEvent, w *workflow.Workflow) error {
	// Build a map of steps in the current workflow
	workflowSteps := make(map[string]*workflow.Step)
	for _, step := range w.Steps() {
		workflowSteps[step.Name()] = step
	}

	// Validate each event
	for i, event := range events {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("event %d validation failed: %w", i, err)
		}

		// Check step-related events
		if event.StepName != "" {
			step, exists := workflowSteps[event.StepName]
			if !exists {
				return fmt.Errorf("step %s no longer exists in workflow (event %d)", event.StepName, i)
			}

			// Validate step type if recorded
			if expectedType := getStringFromData(event.Data, "step_type"); expectedType != "" {
				if step.Type() != expectedType {
					return fmt.Errorf("step %s changed type from %s to %s (event %d)",
						event.StepName, expectedType, step.Type(), i)
				}
			}
		}
	}

	r.logger.Info("event history validation passed", "event_count", len(events))
	return nil
}
