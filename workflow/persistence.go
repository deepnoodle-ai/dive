package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// ExecutionEvent represents a single event in the execution history
type ExecutionEvent struct {
	ID          string                 `json:"id"`
	ExecutionID string                 `json:"execution_id"`
	PathID      string                 `json:"path_id"`
	Sequence    int64                  `json:"sequence"`
	Timestamp   time.Time              `json:"timestamp"`
	EventType   ExecutionEventType     `json:"event_type"`
	StepName    string                 `json:"step_name,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// ExecutionEventType represents the type of execution event
type ExecutionEventType string

const (
	EventExecutionStarted       ExecutionEventType = "execution_started"
	EventPathStarted            ExecutionEventType = "path_started"
	EventStepStarted            ExecutionEventType = "step_started"
	EventStepCompleted          ExecutionEventType = "step_completed"
	EventStepFailed             ExecutionEventType = "step_failed"
	EventPathCompleted          ExecutionEventType = "path_completed"
	EventPathFailed             ExecutionEventType = "path_failed"
	EventExecutionCompleted     ExecutionEventType = "execution_completed"
	EventExecutionFailed        ExecutionEventType = "execution_failed"
	EventPathBranched           ExecutionEventType = "path_branched"
	EventSignalReceived         ExecutionEventType = "signal_received"
	EventVersionDecision        ExecutionEventType = "version_decision"
	EventExecutionContinueAsNew ExecutionEventType = "execution_continue_as_new"
)

// ExecutionSnapshot represents the complete state of an execution
type ExecutionSnapshot struct {
	ID           string    `json:"id"`
	WorkflowName string    `json:"workflow_name"`
	WorkflowHash string    `json:"workflow_hash"`
	InputsHash   string    `json:"inputs_hash"`
	Status       string    `json:"status"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastEventSeq int64     `json:"last_event_seq"`

	// Serialized data (for bootstrapping)
	WorkflowData []byte                 `json:"workflow_data"`
	Inputs       map[string]interface{} `json:"inputs"`
	Outputs      map[string]interface{} `json:"outputs"`
	Error        string                 `json:"error,omitempty"`
}

// ExecutionEventStore defines the interface for persisting execution events
type ExecutionEventStore interface {
	// Event operations
	AppendEvents(ctx context.Context, events []*ExecutionEvent) error
	GetEvents(ctx context.Context, executionID string, fromSeq int64) ([]*ExecutionEvent, error)
	GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error)

	// Snapshot operations
	SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error
	GetSnapshot(ctx context.Context, executionID string) (*ExecutionSnapshot, error)

	// Query operations
	ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*ExecutionSnapshot, error)
	DeleteExecution(ctx context.Context, executionID string) error

	// Cleanup operations
	CleanupCompletedExecutions(ctx context.Context, olderThan time.Time) error
}

// ExecutionFilter specifies criteria for querying executions
type ExecutionFilter struct {
	Status       *string `json:"status,omitempty"`
	WorkflowName *string `json:"workflow_name,omitempty"`
	Limit        int     `json:"limit,omitempty"`
	Offset       int     `json:"offset,omitempty"`
}

// RetryOptions configures how to retry a failed execution
type RetryOptions struct {
	Strategy  RetryStrategy          `json:"strategy"`
	NewInputs map[string]interface{} `json:"new_inputs,omitempty"`
}

// RetryStrategy defines different approaches to retrying executions
type RetryStrategy string

const (
	RetryFromStart     RetryStrategy = "from_start"      // Complete replay
	RetryFromFailure   RetryStrategy = "from_failure"    // Resume from failed step
	RetryWithNewInputs RetryStrategy = "with_new_inputs" // Replay with different inputs
	RetrySkipFailed    RetryStrategy = "skip_failed"     // Continue past failed steps
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

// WorkflowVersion defines a version decision point in workflow evolution
type WorkflowVersion struct {
	ChangeID     string `json:"change_id"`     // Unique identifier for this version decision
	Version      int    `json:"version"`       // Selected version number
	MinVersion   int    `json:"min_version"`   // Minimum supported version
	MaxVersion   int    `json:"max_version"`   // Maximum available version
	ChangeReason string `json:"change_reason"` // Description of what changed
}

// WorkflowCompatibility represents compatibility between workflow versions
type WorkflowCompatibility struct {
	IsCompatible      bool     `json:"is_compatible"`
	IncompatibleSteps []string `json:"incompatible_steps,omitempty"`
	ChangedInputs     []string `json:"changed_inputs,omitempty"`
	ChangesSummary    string   `json:"changes_summary"`
}

// WorkflowHasher provides methods for generating workflow hashes
type WorkflowHasher interface {
	HashWorkflow(workflow *Workflow) (string, error)
	HashInputs(inputs map[string]interface{}) (string, error)
	CompareWorkflows(oldHash, newHash string, oldWorkflow, newWorkflow *Workflow) (*WorkflowCompatibility, error)
}

// ChangeDetector analyzes workflow and input changes for replay compatibility
type ChangeDetector interface {
	DetectWorkflowChanges(events []*ExecutionEvent, currentWorkflow *Workflow) (*WorkflowCompatibility, error)
	DetectInputChanges(oldInputs, newInputs map[string]interface{}) ([]string, error)
	FindAffectedSteps(changedInputs []string, workflow *Workflow) ([]string, error)
}

// Validate validates the execution event
func (e *ExecutionEvent) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("event ID is required")
	}
	if e.ExecutionID == "" {
		return fmt.Errorf("execution ID is required")
	}
	if e.Sequence <= 0 {
		return fmt.Errorf("sequence must be positive")
	}
	if e.EventType == "" {
		return fmt.Errorf("event type is required")
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	return nil
}

// Validate validates the execution filter
func (f *ExecutionFilter) Validate() error {
	if f.Limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	if f.Offset < 0 {
		return fmt.Errorf("offset cannot be negative")
	}
	return nil
}

// ExecutionReplayer handles event history replay for state reconstruction
type ExecutionReplayer interface {
	ReplayExecution(ctx context.Context, events []*ExecutionEvent, workflow *Workflow) (*ReplayResult, error)
	ValidateEventHistory(ctx context.Context, events []*ExecutionEvent, workflow *Workflow) error
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
func (r *BasicExecutionReplayer) ReplayExecution(ctx context.Context, events []*ExecutionEvent, workflow *Workflow) (*ReplayResult, error) {
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
func (r *BasicExecutionReplayer) validatePathState(pathState *ReplayPathState, workflow *Workflow) error {
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
func (r *BasicExecutionReplayer) reconstructScriptGlobals(result *ReplayResult, events []*ExecutionEvent, workflow *Workflow) error {
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
func (r *BasicExecutionReplayer) ValidateEventHistory(ctx context.Context, events []*ExecutionEvent, workflow *Workflow) error {
	// Build a map of steps in the current workflow
	workflowSteps := make(map[string]*Step)
	for _, step := range workflow.Steps() {
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

// Helper functions for extracting data from event data maps
func getStringFromData(data map[string]interface{}, key string) string {
	if value, ok := data[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func getStringFromMap(data map[string]interface{}, key string) string {
	if value, ok := data[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

// BasicWorkflowHasher provides a simple implementation of WorkflowHasher
type BasicWorkflowHasher struct{}

// NewBasicWorkflowHasher creates a new basic workflow hasher
func NewBasicWorkflowHasher() *BasicWorkflowHasher {
	return &BasicWorkflowHasher{}
}

// HashWorkflow generates a deterministic hash of a workflow definition
func (h *BasicWorkflowHasher) HashWorkflow(workflow *Workflow) (string, error) {
	// Create a normalized representation of the workflow for hashing
	workflowData := map[string]interface{}{
		"name":        workflow.Name(),
		"description": workflow.Description(),
		"inputs":      h.normalizeInputs(workflow.Inputs()),
		"output":      h.normalizeOutput(workflow.Output()),
		"steps":       h.normalizeSteps(workflow.Steps()),
		"graph_edges": h.normalizeGraph(workflow.Graph()),
	}

	// Convert to JSON for consistent hashing
	jsonData, err := json.Marshal(workflowData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal workflow data: %w", err)
	}

	// Generate hash
	hash := sha256.Sum256(jsonData)
	return fmt.Sprintf("%x", hash), nil
}

// HashInputs generates a hash of input parameters
func (h *BasicWorkflowHasher) HashInputs(inputs map[string]interface{}) (string, error) {
	// Normalize inputs for consistent hashing
	normalizedInputs := make(map[string]interface{})
	for key, value := range inputs {
		normalizedInputs[key] = value
	}

	jsonData, err := json.Marshal(normalizedInputs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal inputs: %w", err)
	}

	hash := sha256.Sum256(jsonData)
	return fmt.Sprintf("%x", hash), nil
}

// CompareWorkflows compares two workflows and determines compatibility
func (h *BasicWorkflowHasher) CompareWorkflows(oldHash, newHash string, oldWorkflow, newWorkflow *Workflow) (*WorkflowCompatibility, error) {
	if oldHash == newHash {
		return &WorkflowCompatibility{
			IsCompatible:   true,
			ChangesSummary: "No changes detected",
		}, nil
	}

	// Analyze specific changes
	incompatibleSteps := h.findIncompatibleSteps(oldWorkflow, newWorkflow)
	changedInputs := h.findChangedInputs(oldWorkflow, newWorkflow)

	isCompatible := len(incompatibleSteps) == 0

	summary := fmt.Sprintf("Workflow changed: %d incompatible steps, %d input changes",
		len(incompatibleSteps), len(changedInputs))

	return &WorkflowCompatibility{
		IsCompatible:      isCompatible,
		IncompatibleSteps: incompatibleSteps,
		ChangedInputs:     changedInputs,
		ChangesSummary:    summary,
	}, nil
}

// normalizeInputs creates a normalized representation of workflow inputs
func (h *BasicWorkflowHasher) normalizeInputs(inputs []*Input) []map[string]interface{} {
	var normalized []map[string]interface{}
	for _, input := range inputs {
		normalized = append(normalized, map[string]interface{}{
			"name":        input.Name,
			"type":        input.Type,
			"required":    input.Required,
			"description": input.Description,
			"default":     input.Default,
		})
	}
	return normalized
}

// normalizeOutput creates a normalized representation of workflow output
func (h *BasicWorkflowHasher) normalizeOutput(output *Output) map[string]interface{} {
	if output == nil {
		return nil
	}
	return map[string]interface{}{
		"name":        output.Name,
		"type":        output.Type,
		"description": output.Description,
		"format":      output.Format,
		"document":    output.Document,
	}
}

// normalizeSteps creates a normalized representation of workflow steps
func (h *BasicWorkflowHasher) normalizeSteps(steps []*Step) []map[string]interface{} {
	var normalized []map[string]interface{}
	for _, step := range steps {
		normalized = append(normalized, map[string]interface{}{
			"name":       step.Name(),
			"type":       step.Type(),
			"parameters": step.Parameters(),
		})
	}
	return normalized
}

// normalizeGraph creates a normalized representation of the workflow graph
func (h *BasicWorkflowHasher) normalizeGraph(graph *Graph) map[string]interface{} {
	// Simple representation - in a full implementation this would be more detailed
	stepNames := graph.Names()
	return map[string]interface{}{
		"start_step": graph.Start().Name(),
		"step_count": len(stepNames),
		"step_names": stepNames,
	}
}

// findIncompatibleSteps identifies steps that have changed in incompatible ways
func (h *BasicWorkflowHasher) findIncompatibleSteps(oldWorkflow, newWorkflow *Workflow) []string {
	var incompatible []string

	oldSteps := make(map[string]*Step)
	for _, step := range oldWorkflow.Steps() {
		oldSteps[step.Name()] = step
	}

	newSteps := make(map[string]*Step)
	for _, step := range newWorkflow.Steps() {
		newSteps[step.Name()] = step
	}

	// Check for removed steps
	for name := range oldSteps {
		if _, exists := newSteps[name]; !exists {
			incompatible = append(incompatible, name+" (removed)")
		}
	}

	// Check for changed step types
	for name, newStep := range newSteps {
		if oldStep, exists := oldSteps[name]; exists {
			if oldStep.Type() != newStep.Type() {
				incompatible = append(incompatible, name+" (type changed)")
			}
		}
	}

	return incompatible
}

// findChangedInputs identifies inputs that have changed between workflow versions
func (h *BasicWorkflowHasher) findChangedInputs(oldWorkflow, newWorkflow *Workflow) []string {
	var changed []string

	oldInputs := make(map[string]*Input)
	for _, input := range oldWorkflow.Inputs() {
		oldInputs[input.Name] = input
	}

	newInputs := make(map[string]*Input)
	for _, input := range newWorkflow.Inputs() {
		newInputs[input.Name] = input
	}

	// Check for removed inputs
	for name := range oldInputs {
		if _, exists := newInputs[name]; !exists {
			changed = append(changed, name+" (removed)")
		}
	}

	// Check for new inputs
	for name := range newInputs {
		if _, exists := oldInputs[name]; !exists {
			changed = append(changed, name+" (added)")
		}
	}

	// Check for type changes
	for name, newInput := range newInputs {
		if oldInput, exists := oldInputs[name]; exists {
			if oldInput.Type != newInput.Type || oldInput.Required != newInput.Required {
				changed = append(changed, name+" (modified)")
			}
		}
	}

	return changed
}

// BasicChangeDetector provides a simple implementation of ChangeDetector
type BasicChangeDetector struct {
	hasher WorkflowHasher
}

// NewBasicChangeDetector creates a new basic change detector
func NewBasicChangeDetector(hasher WorkflowHasher) *BasicChangeDetector {
	return &BasicChangeDetector{hasher: hasher}
}

// DetectWorkflowChanges analyzes event history to detect workflow changes
func (d *BasicChangeDetector) DetectWorkflowChanges(events []*ExecutionEvent, currentWorkflow *Workflow) (*WorkflowCompatibility, error) {
	// Find the workflow hash from execution start event
	var originalHash string
	for _, event := range events {
		if event.EventType == EventExecutionStarted {
			if hash, ok := event.Data["workflow_hash"].(string); ok {
				originalHash = hash
				break
			}
		}
	}

	if originalHash == "" {
		return &WorkflowCompatibility{
			IsCompatible:   false,
			ChangesSummary: "No original workflow hash found in event history",
		}, nil
	}

	// Get current workflow hash
	currentHash, err := d.hasher.HashWorkflow(currentWorkflow)
	if err != nil {
		return nil, fmt.Errorf("failed to hash current workflow: %w", err)
	}

	// For now, we can't reconstruct the original workflow from just the hash
	// In a full implementation, we'd store workflow definitions alongside events
	if originalHash == currentHash {
		return &WorkflowCompatibility{
			IsCompatible:   true,
			ChangesSummary: "Workflow unchanged",
		}, nil
	}

	return &WorkflowCompatibility{
		IsCompatible:   false,
		ChangesSummary: "Workflow hash changed (detailed comparison not available)",
	}, nil
}

// DetectInputChanges compares old and new inputs to find changes
func (d *BasicChangeDetector) DetectInputChanges(oldInputs, newInputs map[string]interface{}) ([]string, error) {
	var changed []string

	// Check for removed inputs
	for key := range oldInputs {
		if _, exists := newInputs[key]; !exists {
			changed = append(changed, key+" (removed)")
		}
	}

	// Check for new inputs
	for key := range newInputs {
		if _, exists := oldInputs[key]; !exists {
			changed = append(changed, key+" (added)")
		}
	}

	// Check for changed values
	for key, newValue := range newInputs {
		if oldValue, exists := oldInputs[key]; exists {
			// Simple comparison - in production this should handle complex types better
			if fmt.Sprintf("%v", oldValue) != fmt.Sprintf("%v", newValue) {
				changed = append(changed, key+" (value changed)")
			}
		}
	}

	return changed, nil
}

// FindAffectedSteps determines which steps might be affected by input changes
func (d *BasicChangeDetector) FindAffectedSteps(changedInputs []string, workflow *Workflow) ([]string, error) {
	// Conservative approach - assume all steps might be affected by any input change
	// In a more sophisticated implementation, this would analyze step dependencies
	if len(changedInputs) == 0 {
		return []string{}, nil
	}

	var allSteps []string
	for _, step := range workflow.Steps() {
		allSteps = append(allSteps, step.Name())
	}

	return allSteps, nil
}
