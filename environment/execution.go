package environment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/eval"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/objects"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/risor-io/risor"
	"github.com/risor-io/risor/compiler"
	"github.com/risor-io/risor/modules/all"
	"github.com/risor-io/risor/object"
	"github.com/risor-io/risor/parser"
)

// Status represents the execution status
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
)

// ExecutionOptions configures a new Execution
type ExecutionOptions struct {
	Workflow    *workflow.Workflow
	Environment *Environment
	Inputs      map[string]interface{}
	EventStore  ExecutionEventStore
	Logger      slogger.Logger
	ReplayMode  bool
	Formatter   WorkflowFormatter
}

// Execution represents a deterministic workflow execution using operations
type Execution struct {
	id          string
	workflow    *workflow.Workflow
	environment *Environment
	inputs      map[string]interface{}
	outputs     map[string]interface{}

	// Status and timing
	status    ExecutionStatus
	startTime time.Time
	endTime   time.Time
	err       error

	// Operation management
	operationResults map[OperationID]*OperationResult
	replayMode       bool

	// State management
	state    *WorkflowState
	recorder ExecutionRecorder

	// Path management for parallel execution
	paths       map[string]*PathState
	activePaths map[string]*executionPath
	pathUpdates chan pathUpdate
	pathCounter int

	// Logging and formatting
	logger    slogger.Logger
	formatter WorkflowFormatter

	// Synchronization
	mutex  sync.RWMutex
	doneWg sync.WaitGroup
}

// ContentFingerprint represents a hash of content for deterministic tracking
type ContentFingerprint struct {
	Hash   string `json:"hash"`
	Source string `json:"source"` // file path, URL, or content type
	Size   int64  `json:"size"`   // content size in bytes
}

// ContentSnapshot captures content and its fingerprint for replay
type ContentSnapshot struct {
	Fingerprint ContentFingerprint `json:"fingerprint"`
	Content     []llm.Content      `json:"content"`
}

// NewExecution creates a new deterministic execution
func NewExecution(opts ExecutionOptions) (*Execution, error) {
	if opts.Workflow == nil {
		return nil, fmt.Errorf("workflow is required")
	}
	if opts.Environment == nil {
		return nil, fmt.Errorf("environment is required")
	}
	if opts.Logger == nil {
		opts.Logger = slogger.DefaultLogger
	}

	executionID := NewExecutionID()

	recorder := NewBufferedExecutionRecorder(executionID, opts.EventStore, 10)
	recorder.SetReplayMode(opts.ReplayMode)

	state := NewWorkflowState(executionID, recorder)

	inputs := make(map[string]any, len(opts.Inputs))

	// Determine input values from inputs map or defaults
	for _, input := range opts.Workflow.Inputs() {
		if v, ok := opts.Inputs[input.Name]; ok {
			inputs[input.Name] = v
		} else {
			if input.Default == nil {
				return nil, fmt.Errorf("input %q is required", input.Name)
			}
			inputs[input.Name] = input.Default
		}
	}

	// Return error for unknown inputs
	for k := range opts.Inputs {
		if _, ok := inputs[k]; !ok {
			return nil, fmt.Errorf("unknown input %q", k)
		}
	}

	return &Execution{
		id:               executionID,
		workflow:         opts.Workflow,
		environment:      opts.Environment,
		inputs:           inputs,
		outputs:          make(map[string]interface{}),
		status:           ExecutionStatusPending,
		operationResults: make(map[OperationID]*OperationResult),
		replayMode:       opts.ReplayMode,
		state:            state,
		recorder:         recorder,
		paths:            make(map[string]*PathState),
		activePaths:      make(map[string]*executionPath),
		pathUpdates:      make(chan pathUpdate, 100),
		logger:           opts.Logger.With("execution_id", executionID),
		formatter:        opts.Formatter,
	}, nil
}

// NewExecutionFromReplay creates a new execution and loads it from recorded events
func NewExecutionFromReplay(opts ExecutionOptions) (*Execution, error) {
	if opts.EventStore == nil {
		return nil, fmt.Errorf("event store is required for replay")
	}
	execution, err := NewExecution(opts)
	if err != nil {
		return nil, err
	}
	if err := execution.LoadFromEvents(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to load events for replay: %w", err)
	}
	return execution, nil
}

// LoadFromEvents loads execution state from previously recorded events
func (e *Execution) LoadFromEvents(ctx context.Context) error {
	events, err := e.recorder.GetEventHistory(ctx, e.id)
	if err != nil {
		return fmt.Errorf("failed to get event history: %w", err)
	}

	return e.ReplayFromEvents(ctx, events)
}

// ReplayFromEvents replays events to reconstruct execution state
func (e *Execution) ReplayFromEvents(ctx context.Context, events []*ExecutionEvent) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	oldReplayMode := e.replayMode
	e.replayMode = true
	defer func() { e.replayMode = oldReplayMode }()

	// Track state
	stateValues := make(map[string]interface{})
	completedSteps := make(map[string]string)
	pathOutputs := make(map[string]map[string]string) // pathID -> stepName -> output

	e.logger.Info("starting replay", "event_count", len(events))

	// Process events to reconstruct full execution state
	for _, event := range events {
		switch event.EventType {
		case EventExecutionStarted:
			e.status = ExecutionStatusRunning
			e.startTime = event.Timestamp
			// Initialize inputs from event if available
			if inputs, ok := event.Data["inputs"].(map[string]interface{}); ok {
				stateValues["inputs"] = inputs
			}

		case EventPathStarted:
			// Create path state
			currentStep := getStringFromData(event.Data, "current_step")
			e.paths[event.Path] = &PathState{
				ID:          event.Path,
				Status:      PathStatusPending,
				CurrentStep: nil, // Will be set when we find the step
				StartTime:   event.Timestamp,
				StepOutputs: make(map[string]string),
			}
			pathOutputs[event.Path] = make(map[string]string)

			// Find the current step
			if currentStep != "" {
				if step, exists := e.workflow.Graph().Get(currentStep); exists {
					e.paths[event.Path].CurrentStep = step
				}
			}

		case EventStepStarted:
			// Update path current step
			if pathState, exists := e.paths[event.Path]; exists {
				if step, exists := e.workflow.Graph().Get(event.Step); exists {
					pathState.CurrentStep = step
					pathState.Status = PathStatusRunning
				}
			}

		case EventStepCompleted:
			stepOutput := getStringFromData(event.Data, "output")
			completedSteps[event.Step] = stepOutput

			// Update path state with step output
			if pathState, exists := e.paths[event.Path]; exists {
				pathState.StepOutputs[event.Step] = stepOutput
			}
			if pathOutputMap, exists := pathOutputs[event.Path]; exists {
				pathOutputMap[event.Step] = stepOutput
			}

			// Handle stored variables
			if varName := getStringFromData(event.Data, "stored_variable"); varName != "" {
				if storedValue, ok := event.Data["stored_value"]; ok {
					stateValues[varName] = storedValue
				} else {
					stateValues[varName] = stepOutput
				}
			}

		case EventStepFailed:
			errorMsg := getStringFromData(event.Data, "error")
			completedSteps[event.Step+"_error"] = errorMsg

			// Update path state
			if pathState, exists := e.paths[event.Path]; exists {
				pathState.StepOutputs[event.Step+"_error"] = errorMsg
				pathState.Status = PathStatusFailed
				pathState.Error = fmt.Errorf("%s", errorMsg)
			}

		case EventPathBranched:
			// Handle path branching - create new path states
			if newPathsData, ok := event.Data["new_paths"]; ok {
				e.handleReplayPathBranching(event, newPathsData, pathOutputs)
			}

		case EventPathCompleted:
			// Mark path as completed
			if pathState, exists := e.paths[event.Path]; exists {
				pathState.Status = PathStatusCompleted
				pathState.EndTime = event.Timestamp

				// Capture final path outputs
				if finalOutput := getStringFromData(event.Data, "final_output"); finalOutput != "" {
					stateValues["path_"+event.Path+"_output"] = finalOutput
				}
			}

		case EventPathFailed:
			// Mark path as failed
			if pathState, exists := e.paths[event.Path]; exists {
				pathState.Status = PathStatusFailed
				pathState.EndTime = event.Timestamp

				// Capture failure reason
				if failureReason := getStringFromData(event.Data, "failure_reason"); failureReason != "" {
					stateValues["path_"+event.Path+"_error"] = failureReason
					pathState.Error = fmt.Errorf("%s", failureReason)
				}
			}

		case EventExecutionCompleted:
			e.status = ExecutionStatusCompleted
			e.endTime = event.Timestamp
			// Capture final execution outputs
			if outputs, ok := event.Data["outputs"].(map[string]interface{}); ok {
				e.outputs = outputs
				stateValues["outputs"] = outputs
			}

		case EventExecutionFailed:
			e.status = ExecutionStatusFailed
			e.endTime = event.Timestamp
			if errorMsg := getStringFromData(event.Data, "error"); errorMsg != "" {
				e.err = fmt.Errorf("%s", errorMsg)
				stateValues["execution_error"] = errorMsg
			}

		case EventOperationCompleted:
			if opID, ok := event.Data["operation_id"].(string); ok {
				if result, ok := event.Data["result"]; ok {
					e.operationResults[OperationID(opID)] = &OperationResult{
						OperationID: OperationID(opID),
						Result:      result,
						Error:       nil,
						ExecutedAt:  event.Timestamp,
					}
				}
			}

		case EventOperationFailed:
			if opID, ok := event.Data["operation_id"].(string); ok {
				var err error
				if errStr, ok := event.Data["error"].(string); ok {
					err = fmt.Errorf("%s", errStr)
				}
				e.operationResults[OperationID(opID)] = &OperationResult{
					OperationID: OperationID(opID),
					Result:      nil,
					Error:       err,
					ExecutedAt:  event.Timestamp,
				}
			}

		case EventStateMutated:
			if mutations, ok := event.Data["mutations"].([]StateMutation); ok {
				for _, mutation := range mutations {
					if mutation.Type == StateMutationTypeDelete {
						delete(stateValues, mutation.Key)
					} else if mutation.Type == StateMutationTypeSet {
						stateValues[mutation.Key] = mutation.Value
					}
				}
			}

		default:
			e.logger.Debug("unknown event type during replay", "event_type", event.EventType)
		}
	}

	// Load all state values at once
	e.state.LoadFromMap(stateValues)

	e.logger.Info("replay completed",
		"operations", len(e.operationResults),
		"state_keys", len(stateValues),
		"completed_steps", len(completedSteps),
		"paths", len(e.paths),
		"final_status", e.status)

	return nil
}

// handleReplayPathBranching processes path branching during replay
func (e *Execution) handleReplayPathBranching(event *ExecutionEvent, newPathsData interface{}, pathOutputs map[string]map[string]string) {
	parentPathOutputs := pathOutputs[event.Path]

	switch pathsData := newPathsData.(type) {
	case []interface{}:
		for _, pathData := range pathsData {
			if pathMap, ok := pathData.(map[string]interface{}); ok {
				e.createReplayBranchedPath(pathMap, event.Path, parentPathOutputs, pathOutputs)
			}
		}
	case map[string]interface{}:
		e.createReplayBranchedPath(pathsData, event.Path, parentPathOutputs, pathOutputs)
	default:
		e.logger.Warn("unrecognized path branching data format during replay", "type", fmt.Sprintf("%T", pathsData))
	}
}

// createReplayBranchedPath creates a new path state during replay
func (e *Execution) createReplayBranchedPath(pathMap map[string]interface{}, parentPathID string, parentOutputs map[string]string, pathOutputs map[string]map[string]string) {
	pathID := getStringFromMap(pathMap, "id")
	if pathID == "" {
		e.logger.Warn("missing path ID in branch data during replay")
		return
	}

	currentStepName := getStringFromMap(pathMap, "current_step")

	newPathState := &PathState{
		ID:          pathID,
		Status:      PathStatusPending,
		CurrentStep: nil,
		StartTime:   time.Now(), // Use current time for replay
		StepOutputs: make(map[string]string),
	}

	// Find the current step
	if currentStepName != "" {
		if step, exists := e.workflow.Graph().Get(currentStepName); exists {
			newPathState.CurrentStep = step
		}
	}

	// Inherit outputs from parent path if specified
	if inherit, ok := pathMap["inherit_outputs"].(bool); ok && inherit && parentOutputs != nil {
		for stepName, output := range parentOutputs {
			newPathState.StepOutputs[stepName] = output
		}
	}

	e.paths[pathID] = newPathState
	pathOutputs[pathID] = make(map[string]string)

	// Copy parent outputs if inheriting
	if inherit, ok := pathMap["inherit_outputs"].(bool); ok && inherit && parentOutputs != nil {
		for stepName, output := range parentOutputs {
			pathOutputs[pathID][stepName] = output
		}
	}
}

// ValidateEventHistory validates an event history for compatibility with the current workflow
func (e *Execution) ValidateEventHistory(ctx context.Context, events []*ExecutionEvent) error {
	// Build a map of steps in the current workflow
	workflowSteps := make(map[string]*workflow.Step)
	for _, step := range e.workflow.Steps() {
		workflowSteps[step.Name()] = step
	}

	// Validate each event
	for i, event := range events {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("event %d validation failed: %w", i, err)
		}

		// Check step-related events
		if event.Step != "" {
			step, exists := workflowSteps[event.Step]
			if !exists {
				return fmt.Errorf("step %s no longer exists in workflow (event %d)", event.Step, i)
			}

			// Validate step type if recorded
			if expectedType := getStringFromData(event.Data, "step_type"); expectedType != "" {
				if step.Type() != expectedType {
					return fmt.Errorf("step %s changed type from %s to %s (event %d)",
						event.Step, expectedType, step.Type(), i)
				}
			}
		}
	}

	e.logger.Info("event history validation passed", "event_count", len(events))
	return nil
}

// ID returns the execution ID
func (e *Execution) ID() string {
	return e.id
}

// Status returns the current execution status
func (e *Execution) Status() ExecutionStatus {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.status
}

// ExecuteOperation implements the OperationExecutor interface
func (e *Execution) ExecuteOperation(ctx context.Context, op Operation, fn func() (interface{}, error)) (interface{}, error) {
	// Generate operation ID if not set
	if op.ID == "" {
		op.ID = op.GenerateID()
	}

	// PathID should always be set by caller - no fallback needed
	if op.PathID == "" {
		return nil, fmt.Errorf("operation PathID is required")
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	// During replay, return recorded result
	if e.replayMode {
		if result, found := e.operationResults[op.ID]; found {
			return result.Result, result.Error
		}
		return nil, fmt.Errorf("operation %s not found during replay", op.ID)
	}

	// Record operation started
	e.recorder.RecordEvent(op.PathID, string(op.ID), &OperationStartedData{
		OperationID:   string(op.ID),
		OperationType: op.Type,
		Parameters:    op.Parameters,
	})

	// Execute the operation
	startTime := time.Now()
	result, err := fn()
	duration := time.Since(startTime)

	// Record operation completion
	if err != nil {
		e.recorder.RecordEvent(op.PathID, string(op.ID), &OperationFailedData{
			OperationID:   string(op.ID),
			OperationType: op.Type,
			Duration:      duration,
			Error:         err.Error(),
		})
	} else {
		e.recorder.RecordEvent(op.PathID, string(op.ID), &OperationCompletedData{
			OperationID:   string(op.ID),
			OperationType: op.Type,
			Duration:      duration,
			Result:        result,
		})
	}

	// Cache result for potential replay
	e.operationResults[op.ID] = &OperationResult{
		OperationID: op.ID,
		Result:      result,
		Error:       err,
		ExecutedAt:  startTime,
	}

	return result, err
}

// FindOperationResult implements the OperationExecutor interface
func (e *Execution) FindOperationResult(opID OperationID) (*OperationResult, bool) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	result, found := e.operationResults[opID]
	return result, found
}

// Run executes the workflow to completion
func (e *Execution) Run(ctx context.Context) error {
	e.mutex.Lock()
	e.status = ExecutionStatusRunning
	e.startTime = time.Now()
	e.mutex.Unlock()

	// Record execution started
	e.recorder.RecordEvent("main", "", &ExecutionStartedData{
		WorkflowName: e.workflow.Name(),
		Inputs:       e.inputs,
	})

	// Start with initial path
	startStep := e.workflow.Start()
	initialPath := &executionPath{
		id:          "main",
		currentStep: startStep,
	}

	e.addPath(initialPath)

	// Record path started
	e.recorder.RecordEvent(initialPath.id, startStep.Name(), &PathStartedData{
		CurrentStep: startStep.Name(),
	})

	// Start parallel execution
	e.doneWg.Add(1)
	go e.runPath(ctx, initialPath)

	// Process path updates
	err := e.processPathUpdates(ctx)

	// Update final status
	e.mutex.Lock()
	e.endTime = time.Now()
	if err != nil {
		e.status = ExecutionStatusFailed
		e.err = err
		e.recorder.RecordEvent(e.workflow.Name(), "", &ExecutionFailedData{
			Error: err.Error(),
		})
		e.logger.Error("workflow execution failed", "error", err)
	} else {
		e.status = ExecutionStatusCompleted
		e.recorder.RecordEvent(e.workflow.Name(), "", &ExecutionCompletedData{
			Outputs: e.outputs,
		})
		e.logger.Info("workflow execution completed")
	}
	e.mutex.Unlock()

	e.recorder.Flush()

	if err := e.saveSnapshot(ctx); err != nil {
		e.logger.Error("failed to save execution snapshot", "error", err)
		return err
	}
	return err
}

// saveSnapshot saves the current execution state as a snapshot
func (e *Execution) saveSnapshot(ctx context.Context) error {
	hasher := NewBasicWorkflowHasher()
	workflowHash, err := hasher.HashWorkflow(e.workflow)
	if err != nil {
		return fmt.Errorf("failed to hash workflow: %w", err)
	}
	snapshot := &ExecutionSnapshot{
		ID:           e.id,
		WorkflowName: e.workflow.Name(),
		WorkflowHash: workflowHash,
		InputsHash:   HashMapToString(e.inputs),
		Status:       string(e.status),
		StartTime:    e.startTime,
		EndTime:      e.endTime,
		CreatedAt:    e.startTime,
		UpdatedAt:    time.Now(),
		LastEventSeq: e.recorder.GetEventSequence(),
		Inputs:       e.inputs,
		Outputs:      e.outputs,
	}
	if e.err != nil {
		snapshot.Error = e.err.Error()
	}
	return e.recorder.SaveSnapshot(ctx, snapshot)
}

// addPath adds a new execution path
func (e *Execution) addPath(path *executionPath) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	state := &PathState{
		ID:          path.id,
		Status:      PathStatusPending,
		CurrentStep: path.currentStep,
		StartTime:   time.Now(),
		StepOutputs: make(map[string]string),
	}

	e.paths[path.id] = state
	e.activePaths[path.id] = path
}

// processPathUpdates handles updates from running paths
func (e *Execution) processPathUpdates(ctx context.Context) error {
	for len(e.activePaths) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-e.pathUpdates:
			if update.err != nil {
				e.updatePathState(update.pathID, func(state *PathState) {
					state.Status = PathStatusFailed
					state.Error = update.err
					state.EndTime = time.Now()
				})
				return update.err
			}

			// Store step output
			e.updatePathState(update.pathID, func(state *PathState) {
				state.StepOutputs[update.stepName] = update.stepOutput
				if update.isDone {
					state.Status = PathStatusCompleted
					state.EndTime = time.Now()
				}
			})

			// Remove completed path
			if update.isDone {
				e.mutex.Lock()
				delete(e.activePaths, update.pathID)
				e.mutex.Unlock()
			}

			// Start new paths from branching
			for _, newPath := range update.newPaths {
				e.addPath(newPath)
				e.doneWg.Add(1)
				go e.runPath(ctx, newPath)
			}

			e.logger.Info("path update processed",
				"active_paths", len(e.activePaths),
				"completed_path", update.isDone,
				"new_paths", len(update.newPaths))
		}
	}

	// Wait for all paths to complete
	e.doneWg.Wait()

	// Check for failed paths
	e.mutex.RLock()
	var failedPaths []string
	for _, state := range e.paths {
		if state.Status == PathStatusFailed {
			failedPaths = append(failedPaths, state.ID)
		}
	}
	e.mutex.RUnlock()

	if len(failedPaths) > 0 {
		return fmt.Errorf("execution completed with failed paths: %v", failedPaths)
	}

	return nil
}

// runPath executes a single path
func (e *Execution) runPath(ctx context.Context, path *executionPath) {
	defer e.doneWg.Done()

	logger := e.logger.With("path_id", path.id)
	logger.Info("running path", "step", path.currentStep.Name())

	for {
		// Update path state to running
		e.updatePathState(path.id, func(state *PathState) {
			state.Status = PathStatusRunning
		})

		currentStep := path.currentStep

		// Execute the step with pathID
		result, err := e.executeStep(ctx, currentStep, path.id)
		if err != nil {
			e.recorder.RecordEvent(path.id, currentStep.Name(), &StepFailedData{
				Error: err.Error(),
			})
			logger.Error("step failed", "step", currentStep.Name(), "error", err)

			e.recorder.RecordEvent(path.id, "", &PathFailedData{
				Error: err.Error(),
			})
			e.pathUpdates <- pathUpdate{pathID: path.id, err: err}
			return
		}

		// Handle path branching
		newPaths, err := e.handlePathBranching(ctx, currentStep, path.id)
		if err != nil {
			e.recorder.RecordEvent(path.id, currentStep.Name(), &StepFailedData{
				Error: err.Error(),
			})
			e.recorder.RecordEvent(path.id, "", &PathFailedData{
				Error: err.Error(),
			})
			e.pathUpdates <- pathUpdate{pathID: path.id, err: err}
			return
		}

		// Record path branching if multiple paths created
		if len(newPaths) > 1 {
			branchInfo := make([]PathBranchInfo, len(newPaths))
			for i, newPath := range newPaths {
				branchInfo[i] = PathBranchInfo{
					ID:             newPath.id,
					CurrentStep:    newPath.currentStep.Name(),
					InheritOutputs: true,
				}
			}
			e.recorder.RecordEvent(path.id, currentStep.Name(), &PathBranchedData{
				NewPaths: branchInfo,
			})
		}

		// Path is complete if there are no new paths or multiple paths (branching)
		isDone := len(newPaths) == 0 || len(newPaths) > 1

		// Send update
		var executeNewPaths []*executionPath
		if len(newPaths) > 1 {
			executeNewPaths = newPaths
		}

		e.pathUpdates <- pathUpdate{
			pathID:     path.id,
			stepName:   currentStep.Name(),
			stepOutput: result.Content,
			newPaths:   executeNewPaths,
			isDone:     isDone,
		}

		if isDone {
			e.recorder.RecordEvent(path.id, currentStep.Name(), &PathCompletedData{
				FinalStep: currentStep.Name(),
			})
			logger.Info("path completed", "step", currentStep.Name())
			return
		}

		// Continue with the single path
		path = newPaths[0]
	}
}

// handlePathBranching processes the next steps and creates new paths if needed
func (e *Execution) handlePathBranching(ctx context.Context, step *workflow.Step, currentPathID string) ([]*executionPath, error) {
	nextEdges := step.Next()
	if len(nextEdges) == 0 {
		return nil, nil // Path is complete
	}

	// Evaluate conditions and collect matching edges
	var matchingEdges []*workflow.Edge
	for _, edge := range nextEdges {
		if edge.Condition == "" {
			matchingEdges = append(matchingEdges, edge)
			continue
		}

		// Evaluate condition using safe Risor scripting
		match, err := e.evaluateCondition(ctx, edge.Condition)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate condition %q in step %q: %w", edge.Condition, step.Name(), err)
		}
		if match {
			matchingEdges = append(matchingEdges, edge)
		}
	}

	// Create new paths for each matching edge
	var newPaths []*executionPath
	for _, edge := range matchingEdges {
		nextStep, ok := e.workflow.Graph().Get(edge.Step)
		if !ok {
			return nil, fmt.Errorf("next step not found: %s", edge.Step)
		}

		var nextPathID string
		if len(matchingEdges) > 1 {
			// Generate new path ID for branching
			e.mutex.Lock()
			e.pathCounter++
			nextPathID = fmt.Sprintf("%s-%d", currentPathID, e.pathCounter)
			e.mutex.Unlock()
		} else {
			nextPathID = currentPathID
		}

		newPaths = append(newPaths, &executionPath{
			id:          nextPathID,
			currentStep: nextStep,
		})
	}

	return newPaths, nil
}

// updatePathState updates the state of a path
func (e *Execution) updatePathState(pathID string, updateFn func(*PathState)) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if state, exists := e.paths[pathID]; exists {
		updateFn(state)
	}
}

// executeStep executes a single workflow step
func (e *Execution) executeStep(ctx context.Context, step *workflow.Step, pathID string) (*dive.StepResult, error) {
	e.logger.Info("executing step", "step_name", step.Name(), "step_type", step.Type())

	// Print step start if formatter is available
	if e.formatter != nil {
		e.formatter.PrintStepStart(step.Name(), step.Type())
	}

	// Record step started
	e.recorder.RecordEvent(pathID, step.Name(), &StepStartedData{
		StepType:   step.Type(),
		StepParams: step.Parameters(),
	})

	var result *dive.StepResult
	var err error

	// Handle steps with "each" blocks
	if step.Each() != nil {
		result, err = e.executeStepEach(ctx, step, pathID)
	} else {
		switch step.Type() {
		case "prompt":
			result, err = e.executePromptStep(ctx, step, pathID)
		case "action":
			result, err = e.executeActionStep(ctx, step, pathID)
		case "script":
			result, err = e.executeScriptStep(ctx, step, pathID)
		default:
			err = fmt.Errorf("unsupported step type %q in step %q", step.Type(), step.Name())
		}
	}

	if err != nil {
		// Print step error if formatter is available
		if e.formatter != nil {
			e.formatter.PrintStepError(step.Name(), err)
		}
		return nil, err
	}

	// Store step result in state if not an each step (each steps handle their own storage)
	if step.Each() == nil {
		// Store the result in a state variable if specified
		if varName := step.Store(); varName != "" {
			// For script steps, store the actual converted value; for other steps, store the content
			var valueToStore interface{}
			if step.Type() == "script" && result.Object != nil {
				valueToStore = result.Object
			} else {
				valueToStore = result.Content
			}
			e.state.Set(varName, valueToStore)
			e.logger.Info("stored step result", "variable_name", varName)
		}
	}

	// Print step output if formatter is available
	if e.formatter != nil {
		e.formatter.PrintStepOutput(step.Name(), result.Content)
	}

	// Record step completed
	e.recorder.RecordEvent(pathID, step.Name(), &StepCompletedData{
		Output:         result.Content,
		StoredVariable: step.Store(),
	})

	return result, nil
}

// executeStepEach handles the execution of a step that has an each block
func (e *Execution) executeStepEach(ctx context.Context, step *workflow.Step, pathID string) (*dive.StepResult, error) {
	each := step.Each()

	// Resolve the items to iterate over
	items, err := e.resolveEachItems(ctx, each)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve each items: %w", err)
	}

	// Execute the step for each item and capture the results
	results := make([]*dive.StepResult, 0, len(items))

	// Store original state if we're setting a variable
	var originalValue interface{}
	var hasOriginalValue bool
	if each.As != "" {
		originalValue, hasOriginalValue = e.state.Get(each.As)
	}

	for _, item := range items {
		// Set the loop variable if specified
		if each.As != "" {
			e.state.Set(each.As, item)
		}

		// Execute the step for this item
		var result *dive.StepResult
		switch step.Type() {
		case "prompt":
			result, err = e.executePromptStep(ctx, step, pathID)
		case "action":
			result, err = e.executeActionStep(ctx, step, pathID)
		case "script":
			result, err = e.executeScriptStep(ctx, step, pathID)
		default:
			err = fmt.Errorf("unsupported step type %q in step %q", step.Type(), step.Name())
		}

		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	// Restore original loop variable value
	if each.As != "" {
		if hasOriginalValue {
			e.state.Set(each.As, originalValue)
		} else {
			e.state.Delete(each.As)
		}
	}

	// Store the array of results if a store variable is specified
	if varName := step.Store(); varName != "" {
		resultContents := make([]string, 0, len(results))
		for _, result := range results {
			resultContents = append(resultContents, result.Content)
		}
		e.state.Set(varName, resultContents)
		e.logger.Info("stored results list",
			"variable_name", varName,
			"item_count", len(resultContents),
		)
	}

	// Combine the results into one string which we put in a single task result
	itemTexts := make([]string, 0, len(results))
	for i, result := range results {
		var itemText string
		if each.As != "" {
			itemText = fmt.Sprintf("# %s: %s\n\n%s", each.As, items[i], result.Content)
		} else {
			itemText = fmt.Sprintf("# %s\n\n%s", items[i], result.Content)
		}
		itemTexts = append(itemTexts, itemText)
	}

	return &dive.StepResult{
		Content: strings.Join(itemTexts, "\n\n"),
		Format:  dive.OutputFormatMarkdown,
	}, nil
}

// resolveEachItems resolves the array of items from either a direct array or a risor expression
func (e *Execution) resolveEachItems(ctx context.Context, each *workflow.EachBlock) ([]any, error) {
	// Array of strings
	if strArray, ok := each.Items.([]string); ok {
		var items []any
		for _, item := range strArray {
			items = append(items, item)
		}
		return items, nil
	}

	// Array of any
	if items, ok := each.Items.([]any); ok {
		return items, nil
	}

	// Handle Risor script expression
	if codeStr, ok := each.Items.(string); ok && strings.HasPrefix(codeStr, "$(") && strings.HasSuffix(codeStr, ")") {
		return e.evaluateRisorExpression(ctx, codeStr)
	}

	return nil, fmt.Errorf("unsupported value for 'each' block (got %T)", each.Items)
}

// evaluateRisorExpression evaluates a risor expression and returns the result as an array
func (e *Execution) evaluateRisorExpression(ctx context.Context, codeStr string) ([]any, error) {
	code := strings.TrimSuffix(strings.TrimPrefix(codeStr, "$("), ")")

	// Build safe globals for the expression evaluation
	globals := e.buildEachGlobals()

	compiledCode, err := e.compileEachScript(ctx, code, globals)
	if err != nil {
		e.logger.Error("failed to compile 'each' expression", "error", err)
		return nil, fmt.Errorf("failed to compile expression: %w", err)
	}

	result, err := e.evalEachScript(ctx, compiledCode, globals)
	if err != nil {
		e.logger.Error("failed to evaluate 'each' expression", "error", err)
		return nil, fmt.Errorf("failed to evaluate expression: %w", err)
	}
	return result, nil
}

// buildEachGlobals creates globals for "each" expression evaluation
func (e *Execution) buildEachGlobals() map[string]any {
	return e.buildGlobalsForContext(ScriptContextEach)
}

// compileEachScript compiles a script for "each" expression evaluation
func (e *Execution) compileEachScript(ctx context.Context, code string, globals map[string]any) (*compiler.Code, error) {
	// Parse the script
	ast, err := parser.Parse(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to parse script: %w", err)
	}

	// Get sorted global names for deterministic compilation
	var globalNames []string
	for name := range globals {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)

	// Compile with global names
	return compiler.Compile(ast, compiler.WithGlobalNames(globalNames))
}

// evalEachScript evaluates a compiled script and returns the result as an array
func (e *Execution) evalEachScript(ctx context.Context, code *compiler.Code, globals map[string]any) ([]any, error) {
	result, err := risor.EvalCode(ctx, code, risor.WithGlobals(globals))
	if err != nil {
		return nil, err
	}
	return e.convertRisorEachValue(result)
}

// convertRisorEachValue converts a Risor object to an array of values
func (e *Execution) convertRisorEachValue(obj object.Object) ([]any, error) {
	switch obj := obj.(type) {
	case *object.String:
		return []any{obj.Value()}, nil
	case *object.Int:
		return []any{obj.Value()}, nil
	case *object.Float:
		return []any{obj.Value()}, nil
	case *object.Bool:
		return []any{obj.Value()}, nil
	case *object.Time:
		return []any{obj.Value()}, nil
	case *object.List:
		var values []any
		for _, item := range obj.Value() {
			value, err := e.convertRisorEachValue(item)
			if err != nil {
				return nil, err
			}
			values = append(values, value...)
		}
		return values, nil
	case *object.Set:
		var values []any
		for _, item := range obj.Value() {
			value, err := e.convertRisorEachValue(item)
			if err != nil {
				return nil, err
			}
			values = append(values, value...)
		}
		return values, nil
	case *object.Map:
		var values []any
		for _, item := range obj.Value() {
			value, err := e.convertRisorEachValue(item)
			if err != nil {
				return nil, err
			}
			values = append(values, value...)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unsupported risor result type for 'each': %T", obj)
	}
}

// executePromptStep executes a prompt step using an agent
func (e *Execution) executePromptStep(ctx context.Context, step *workflow.Step, pathID string) (*dive.StepResult, error) {
	// Get agent for this step
	agent, err := e.getStepAgent(step)
	if err != nil {
		return nil, err
	}

	// Prepare prompt content
	prompt, err := e.preparePrompt(ctx, step)
	if err != nil {
		return nil, err
	}

	// Prepare all content for the LLM call
	content, err := e.preparePromptContent(ctx, step, agent, prompt)
	if err != nil {
		return nil, err
	}

	// Execute the agent response operation
	response, err := e.executeAgentResponse(ctx, step, pathID, agent, prompt, content)
	if err != nil {
		return nil, err
	}

	// Process and return the result
	return e.processAgentResponse(response), nil
}

// getStepAgent gets the agent for a step, falling back to the default agent if none specified
func (e *Execution) getStepAgent(step *workflow.Step) (dive.Agent, error) {
	agent := step.Agent()
	if agent == nil {
		var found bool
		agent, found = e.environment.DefaultAgent()
		if !found {
			return nil, fmt.Errorf("no agent specified for prompt step %q", step.Name())
		}
	}
	return agent, nil
}

// preparePrompt evaluates the prompt template if needed
func (e *Execution) preparePrompt(ctx context.Context, step *workflow.Step) (string, error) {
	prompt := step.Prompt()
	if strings.Contains(prompt, "${") {
		evaluatedPrompt, err := e.evaluateTemplate(ctx, prompt)
		if err != nil {
			return "", fmt.Errorf("failed to evaluate prompt template in step %q (type: %s): %w", step.Name(), step.Type(), err)
		}
		prompt = evaluatedPrompt
	}
	return prompt, nil
}

// preparePromptContent prepares all content for the LLM call
func (e *Execution) preparePromptContent(ctx context.Context, step *workflow.Step, agent dive.Agent, prompt string) ([]llm.Content, error) {
	var content []llm.Content

	// Process step content
	if stepContent := step.Content(); len(stepContent) > 0 {
		for _, item := range stepContent {
			if dynamicContent, ok := item.(DynamicContent); ok {
				processedContent, err := dynamicContent.Content(ctx, map[string]any{
					"workflow": e.workflow.Name(),
					"step":     step.Name(),
					"agent":    agent.Name(),
				})
				if err != nil {
					return nil, fmt.Errorf("failed to process dynamic content: %w", err)
				}
				content = append(content, processedContent...)
			} else {
				content = append(content, item)
			}
		}
	}

	// Add prompt as text content
	if prompt != "" {
		content = append(content, &llm.TextContent{Text: prompt})
	}

	return content, nil
}

// executeAgentResponse executes the agent response operation
func (e *Execution) executeAgentResponse(ctx context.Context, step *workflow.Step, pathID string, agent dive.Agent, prompt string, content []llm.Content) (*dive.Response, error) {
	// Create content fingerprint for deterministic tracking
	contentFingerprint, err := e.createContentFingerprint(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("failed to create content fingerprint: %w", err)
	}

	// Use deterministic parameters for operation ID generation
	op := Operation{
		Type:     "agent_response",
		StepName: step.Name(),
		PathID:   pathID,
		Parameters: map[string]interface{}{
			"agent":           agent.Name(),
			"prompt":          prompt,
			"content_hash":    contentFingerprint.Hash,
			"content_sources": contentFingerprint.Source,
			"content_size":    contentFingerprint.Size,
		},
	}

	responseInterface, err := e.ExecuteOperation(ctx, op, func() (interface{}, error) {
		return agent.CreateResponse(ctx, dive.WithMessage(llm.NewUserMessage(content...)))
	})

	if err != nil {
		return nil, fmt.Errorf("agent response operation failed: %w", err)
	}

	return responseInterface.(*dive.Response), nil
}

// processAgentResponse processes the agent response and returns a StepResult
func (e *Execution) processAgentResponse(response *dive.Response) *dive.StepResult {
	var usage llm.Usage
	if response.Usage != nil {
		usage = *response.Usage
	}
	return &dive.StepResult{
		Content: response.OutputText(),
		Usage:   usage,
	}
}

// executeActionStep executes an action step
func (e *Execution) executeActionStep(ctx context.Context, step *workflow.Step, pathID string) (*dive.StepResult, error) {
	actionName := step.Action()
	if actionName == "" {
		return nil, fmt.Errorf("no action specified for action step %q", step.Name())
	}
	action, ok := e.environment.GetAction(actionName)
	if !ok {
		return nil, fmt.Errorf("action %q not found", actionName)
	}

	// Deterministic: prepare parameters by evaluating templates
	params := make(map[string]interface{})
	for name, value := range step.Parameters() {
		if strValue, ok := value.(string); ok && strings.Contains(strValue, "${") {
			evaluated, err := e.evaluateTemplate(ctx, strValue)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate parameter template %q in step %q (action: %s): %w",
					name, step.Name(), actionName, err)
			}
			params[name] = evaluated
		} else {
			params[name] = value
		}
	}

	// Operation: execute action (non-deterministic)
	op := Operation{
		Type:     "action_execution",
		StepName: step.Name(),
		PathID:   pathID,
		Parameters: map[string]interface{}{
			"action_name": actionName,
			"params":      params,
		},
	}

	result, err := e.ExecuteOperation(ctx, op, func() (interface{}, error) {
		return action.Execute(ctx, params)
	})
	if err != nil {
		return nil, fmt.Errorf("action execution operation failed: %w", err)
	}
	var content string
	if result != nil {
		content = fmt.Sprintf("%v", result)
	}
	return &dive.StepResult{Content: content}, nil
}

// executeScriptStep executes a script activity step
func (e *Execution) executeScriptStep(ctx context.Context, step *workflow.Step, pathID string) (*dive.StepResult, error) {
	script := step.Script()
	if script == "" {
		return nil, fmt.Errorf("no script specified for script activity step %q", step.Name())
	}

	// Deterministic: prepare script globals with non-deterministic context (allows all functions)
	globals := e.buildGlobalsForContext(ScriptContextActivity)

	// Operation: execute script (non-deterministic)
	op := Operation{
		Type:     "script_execution",
		StepName: step.Name(),
		PathID:   pathID,
		Parameters: map[string]interface{}{
			"script": script,
		},
	}

	result, err := e.ExecuteOperation(ctx, op, func() (interface{}, error) {
		// Compile the script
		compiledCode, err := e.compileActivityScript(ctx, script, globals)
		if err != nil {
			return nil, fmt.Errorf("failed to compile script: %w", err)
		}

		// Execute the compiled script
		risorResult, err := risor.EvalCode(ctx, compiledCode, risor.WithGlobals(globals))
		if err != nil {
			return nil, fmt.Errorf("failed to execute script: %w", err)
		}

		// Convert Risor object to Go type
		return e.convertRisorValue(risorResult), nil
	})
	if err != nil {
		return nil, fmt.Errorf("script execution operation failed: %w", err)
	}

	var content string
	if result != nil {
		content = fmt.Sprintf("%v", result)
	}
	return &dive.StepResult{Content: content, Object: result}, nil
}

// evaluateCondition evaluates a workflow condition using Risor scripting
func (e *Execution) evaluateCondition(ctx context.Context, condition string) (bool, error) {
	// Handle simple boolean conditions
	switch strings.ToLower(strings.TrimSpace(condition)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}

	// Handle Risor expressions wrapped in $()
	codeStr := condition
	if strings.HasPrefix(codeStr, "$(") && strings.HasSuffix(codeStr, ")") {
		codeStr = strings.TrimPrefix(codeStr, "$(")
		codeStr = strings.TrimSuffix(codeStr, ")")
	}

	// Build safe globals for condition evaluation
	globals := e.buildGlobalsForContext(ScriptContextCondition)

	// Compile the script
	compiledCode, err := e.compileConditionScript(ctx, codeStr, globals)
	if err != nil {
		return false, fmt.Errorf("failed to compile condition: %w", err)
	}

	// Evaluate the condition
	result, err := risor.EvalCode(ctx, compiledCode, risor.WithGlobals(globals))
	if err != nil {
		return false, fmt.Errorf("failed to evaluate condition: %w", err)
	}

	// Convert result to boolean
	return e.convertToBool(result), nil
}

// compileConditionScript compiles a condition script with deterministic global names
func (e *Execution) compileConditionScript(ctx context.Context, code string, globals map[string]any) (*compiler.Code, error) {
	// Parse the script
	ast, err := parser.Parse(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to parse script: %w", err)
	}

	// Get sorted global names for deterministic compilation
	var globalNames []string
	for name := range globals {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)

	// Compile with global names
	return compiler.Compile(ast, compiler.WithGlobalNames(globalNames))
}

// convertToBool safely converts a Risor object to a boolean
func (e *Execution) convertToBool(obj object.Object) bool {
	switch obj := obj.(type) {
	case *object.Bool:
		return obj.Value()
	case *object.Int:
		return obj.Value() != 0
	case *object.Float:
		return obj.Value() != 0.0
	case *object.String:
		val := obj.Value()
		return val != "" && strings.ToLower(val) != "false"
	case *object.List:
		return len(obj.Value()) > 0
	case *object.Map:
		return len(obj.Value()) > 0
	default:
		// Use Risor's built-in truthiness evaluation
		return obj.IsTruthy()
	}
}

func (e *Execution) evaluateTemplate(ctx context.Context, s string) (string, error) {
	if strings.HasPrefix(s, "$(") && strings.HasSuffix(s, ")") {
		s = strings.TrimPrefix(s, "$(")
		s = strings.TrimSuffix(s, ")")
	}

	// Use deterministic context for template evaluation
	globals := e.buildGlobalsForContext(ScriptContextTemplate)

	return eval.Eval(ctx, s, globals)
}

// ScriptContext represents different contexts where scripts can run
type ScriptContext int

const (
	ScriptContextCondition ScriptContext = iota
	ScriptContextTemplate
	ScriptContextEach
	ScriptContextActivity
)

// getSafeGlobals returns the map of safe globals for deterministic contexts
func (e *Execution) getSafeGlobals() map[string]bool {
	return map[string]bool{
		"all":         true,
		"any":         true,
		"base64":      true,
		"bool":        true,
		"buffer":      true,
		"byte_slice":  true,
		"byte":        true,
		"bytes":       true,
		"call":        true,
		"chr":         true,
		"chunk":       true,
		"coalesce":    true,
		"decode":      true,
		"encode":      true,
		"error":       true,
		"errorf":      true,
		"errors":      true,
		"filepath":    true,
		"float_slice": true,
		"float":       true,
		"fmt":         true,
		"getattr":     true,
		"int":         true,
		"is_hashable": true,
		"iter":        true,
		"json":        true,
		"keys":        true,
		"len":         true,
		"list":        true,
		"map":         true,
		"math":        true,
		"ord":         true,
		"regexp":      true,
		"reversed":    true,
		"set":         true,
		"sorted":      true,
		"sprintf":     true,
		"string":      true,
		"strings":     true,
		"try":         true,
		"type":        true,
	}
}

// buildGlobalsForContext creates globals appropriate for the given script context
func (e *Execution) buildGlobalsForContext(ctx ScriptContext) map[string]any {
	globals := make(map[string]any)

	// Add built-in functions based on context
	switch ctx {
	case ScriptContextCondition, ScriptContextTemplate, ScriptContextEach:
		// Deterministic contexts - only include safe functions
		safeGlobals := e.getSafeGlobals()
		for k, v := range all.Builtins() {
			if safeGlobals[k] {
				globals[k] = v
			}
		}
	case ScriptContextActivity:
		// Non-deterministic context - allow all functions
		for k, v := range all.Builtins() {
			globals[k] = v
		}
	}

	// Add workflow inputs (read-only)
	globals["inputs"] = e.inputs

	// Add current state variables (read-only snapshot)
	globals["state"] = e.state.Copy()

	// Add documents if available
	if e.environment.documentRepo != nil {
		globals["documents"] = objects.NewDocumentRepository(e.environment.documentRepo)
	}

	return globals
}

// compileActivityScript compiles a script for activity step execution
func (e *Execution) compileActivityScript(ctx context.Context, code string, globals map[string]any) (*compiler.Code, error) {
	// Parse the script
	ast, err := parser.Parse(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to parse script: %w", err)
	}

	// Get sorted global names for deterministic compilation
	var globalNames []string
	for name := range globals {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)

	// Compile with global names
	return compiler.Compile(ast, compiler.WithGlobalNames(globalNames))
}

// convertRisorValue converts a Risor object to a Go type
func (e *Execution) convertRisorValue(obj object.Object) interface{} {
	switch o := obj.(type) {
	case *object.String:
		return o.Value()
	case *object.Int:
		return o.Value()
	case *object.Float:
		return o.Value()
	case *object.Bool:
		return o.Value()
	case *object.Time:
		return o.Value()
	case *object.List:
		var result []interface{}
		for _, item := range o.Value() {
			result = append(result, e.convertRisorValue(item))
		}
		return result
	case *object.Map:
		result := make(map[string]interface{})
		for key, value := range o.Value() {
			result[key] = e.convertRisorValue(value)
		}
		return result
	case *object.Set:
		var result []interface{}
		for _, item := range o.Value() {
			result = append(result, e.convertRisorValue(item))
		}
		return result
	default:
		// Fallback to string representation
		return obj.Inspect()
	}
}

// createContentFingerprint creates a combined fingerprint for content items
func (e *Execution) createContentFingerprint(ctx context.Context, content []llm.Content) (ContentFingerprint, error) {
	fingerprints, err := e.calculateContentFingerprints(ctx, content)
	if err != nil {
		return ContentFingerprint{}, err
	}

	// Create a combined hash from all fingerprints
	var hashBuilder strings.Builder
	for _, fp := range fingerprints {
		hashBuilder.WriteString(fp.Hash)
		hashBuilder.WriteString(":")
		hashBuilder.WriteString(fp.Source)
		hashBuilder.WriteString(":")
	}

	combinedHash := sha256.Sum256([]byte(hashBuilder.String()))
	return ContentFingerprint{
		Hash:   hex.EncodeToString(combinedHash[:]),
		Source: "combined",
		Size:   int64(hashBuilder.Len()),
	}, nil
}

// calculateContentFingerprints calculates fingerprints for all content items
func (e *Execution) calculateContentFingerprints(ctx context.Context, content []llm.Content) ([]ContentFingerprint, error) {
	var fingerprints []ContentFingerprint

	for _, item := range content {
		fingerprint, err := e.calculateContentFingerprint(ctx, item)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate content fingerprint: %w", err)
		}
		fingerprints = append(fingerprints, fingerprint)
	}

	return fingerprints, nil
}

// calculateContentFingerprint calculates a fingerprint for a single content item
func (e *Execution) calculateContentFingerprint(ctx context.Context, content llm.Content) (ContentFingerprint, error) {
	switch c := content.(type) {
	case *llm.TextContent:
		hash := sha256.Sum256([]byte(c.Text))
		return ContentFingerprint{
			Hash:   hex.EncodeToString(hash[:]),
			Source: "text",
			Size:   int64(len(c.Text)),
		}, nil
	case *llm.ImageContent:
		if c.Source != nil {
			return e.calculateSourceFingerprint(ctx, c.Source, "image")
		}
		return ContentFingerprint{Hash: "empty-image", Source: "image"}, nil
	case *llm.DocumentContent:
		if c.Source != nil {
			return e.calculateSourceFingerprint(ctx, c.Source, "document")
		}
		return ContentFingerprint{Hash: "empty-document", Source: "document"}, nil
	case DynamicContent:
		// For dynamic content, we need to resolve it first to get the actual content
		resolvedContent, err := c.Content(ctx, map[string]any{
			"workflow": e.workflow.Name(),
			"step":     "fingerprint",
		})
		if err != nil {
			return ContentFingerprint{}, fmt.Errorf("failed to resolve dynamic content for fingerprinting: %w", err)
		}

		// Calculate fingerprint of the resolved content
		var allText strings.Builder
		for _, resolved := range resolvedContent {
			if textContent, ok := resolved.(*llm.TextContent); ok {
				allText.WriteString(textContent.Text)
			}
		}

		hash := sha256.Sum256([]byte(allText.String()))
		return ContentFingerprint{
			Hash:   hex.EncodeToString(hash[:]),
			Source: "dynamic",
			Size:   int64(allText.Len()),
		}, nil
	default:
		// Fallback for unknown content types
		hash := sha256.Sum256([]byte(fmt.Sprintf("%T", content)))
		return ContentFingerprint{
			Hash:   hex.EncodeToString(hash[:]),
			Source: "unknown",
			Size:   0,
		}, nil
	}
}

// calculateSourceFingerprint calculates a fingerprint for content with a source
func (e *Execution) calculateSourceFingerprint(ctx context.Context, source *llm.ContentSource, contentType string) (ContentFingerprint, error) {
	switch source.Type {
	case llm.ContentSourceTypeBase64:
		hash := sha256.Sum256([]byte(source.Data))
		return ContentFingerprint{
			Hash:   hex.EncodeToString(hash[:]),
			Source: contentType + "-base64",
			Size:   int64(len(source.Data)),
		}, nil
	case llm.ContentSourceTypeURL:
		// For URLs, we hash the URL itself since we can't guarantee the remote content
		hash := sha256.Sum256([]byte(source.URL))
		return ContentFingerprint{
			Hash:   hex.EncodeToString(hash[:]),
			Source: contentType + "-url:" + source.URL,
			Size:   0, // Unknown size for remote content
		}, nil
	default:
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s-%s", contentType, source.Type)))
		return ContentFingerprint{
			Hash:   hex.EncodeToString(hash[:]),
			Source: contentType + "-" + string(source.Type),
			Size:   0,
		}, nil
	}
}

// createContentSnapshot creates a snapshot of content with its fingerprint
func (e *Execution) createContentSnapshot(ctx context.Context, content []llm.Content) (*ContentSnapshot, error) {
	fingerprints, err := e.calculateContentFingerprints(ctx, content)
	if err != nil {
		return nil, err
	}

	// Create a combined hash from all fingerprints
	var hashBuilder strings.Builder
	for _, fp := range fingerprints {
		hashBuilder.WriteString(fp.Hash)
		hashBuilder.WriteString(":")
		hashBuilder.WriteString(fp.Source)
		hashBuilder.WriteString(":")
	}

	combinedHash := sha256.Sum256([]byte(hashBuilder.String()))
	combinedFingerprint := ContentFingerprint{
		Hash:   hex.EncodeToString(combinedHash[:]),
		Source: "combined",
		Size:   int64(hashBuilder.Len()),
	}

	return &ContentSnapshot{
		Fingerprint: combinedFingerprint,
		Content:     content,
	}, nil
}
