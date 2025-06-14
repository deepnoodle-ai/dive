package environment

import (
	"context"
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

// ExecutionOptions are the options for creating a new execution.
type ExecutionOptions struct {
	WorkflowName   string
	Inputs         map[string]interface{}
	Outputs        map[string]interface{}
	Logger         slogger.Logger
	Formatter      WorkflowFormatter
	EventStore     ExecutionEventStore
	EventBatchSize int
}

// ExecutionV2Options configures a new Execution
type ExecutionV2Options struct {
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

// NewExecution creates a new deterministic execution
func NewExecution(opts ExecutionV2Options) (*Execution, error) {
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

	// Create event recorder
	recorder := NewBufferedExecutionRecorder(executionID, opts.EventStore, 10)
	recorder.SetReplayMode(opts.ReplayMode)

	// Create workflow state
	state := NewWorkflowState(executionID, recorder)

	// Initialize inputs in state
	processedInputs := make(map[string]interface{})
	for k, v := range opts.Inputs {
		processedInputs[k] = v
		state.Set(k, v) // Store inputs in workflow state
	}

	return &Execution{
		id:               executionID,
		workflow:         opts.Workflow,
		environment:      opts.Environment,
		inputs:           processedInputs,
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
func NewExecutionFromReplay(opts ExecutionV2Options) (*Execution, error) {
	if opts.EventStore == nil {
		return nil, fmt.Errorf("event store is required for replay")
	}

	execution, err := NewExecution(opts)
	if err != nil {
		return nil, err
	}

	// Load and replay events
	if err := execution.LoadFromEvents(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to load events for replay: %w", err)
	}

	return execution, nil
}

// LoadFromEvents loads operation results from previously recorded events
func (e *Execution) LoadFromEvents(ctx context.Context) error {
	events, err := e.recorder.(*BufferedExecutionRecorder).eventStore.GetEventHistory(ctx, e.id)
	if err != nil {
		return fmt.Errorf("failed to get event history: %w", err)
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Collect state mutations
	stateValues := make(map[string]interface{})

	// Process events to reconstruct operation results and state
	for _, event := range events {
		switch event.EventType {
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
		case EventStateSet:
			if key, ok := event.Data["key"].(string); ok {
				if value, ok := event.Data["value"]; ok {
					stateValues[key] = value
				}
			}
		case EventStateDeleted:
			if key, ok := event.Data["key"].(string); ok {
				delete(stateValues, key)
			}
		}
	}

	// Load all state values at once
	e.state.LoadFromMap(stateValues)

	e.logger.Info("loaded from events", "operations", len(e.operationResults), "state_keys", len(stateValues))
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
	e.recorder.RecordCustomEvent(EventOperationStarted, op.PathID, op.StepName, map[string]interface{}{
		"operation_id":   string(op.ID),
		"operation_type": op.Type,
		"parameters":     op.Parameters,
	})

	// Execute the operation
	startTime := time.Now()
	result, err := fn()
	duration := time.Since(startTime)

	// Record operation completion
	eventData := map[string]interface{}{
		"operation_id":   string(op.ID),
		"operation_type": op.Type,
		"duration":       duration,
	}

	var eventType ExecutionEventType
	if err != nil {
		eventType = EventOperationFailed
		eventData["error"] = err.Error()
	} else {
		eventType = EventOperationCompleted
		eventData["result"] = result
	}

	e.recorder.RecordCustomEvent(eventType, op.PathID, op.StepName, eventData)

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
	e.recorder.RecordExecutionStarted(e.workflow.Name(), e.inputs)

	// Start with initial path
	startStep := e.workflow.Start()
	initialPath := &executionPath{
		id:          "start",
		currentStep: startStep,
	}

	e.addPath(initialPath)

	// Record path started
	e.recorder.RecordPathStarted(initialPath.id, startStep.Name())

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
		e.recorder.RecordExecutionFailed(err)
		e.logger.Error("workflow execution failed", "error", err)
	} else {
		e.status = ExecutionStatusCompleted
		e.recorder.RecordExecutionCompleted(e.outputs)
		e.logger.Info("workflow execution completed")
	}
	e.mutex.Unlock()

	// Flush any remaining events
	e.recorder.Flush()

	return err
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
			e.recorder.RecordStepFailed(path.id, currentStep.Name(), err)
			logger.Error("step failed", "step", currentStep.Name(), "error", err)

			e.recorder.RecordPathFailed(path.id, err)
			e.pathUpdates <- pathUpdate{pathID: path.id, err: err}
			return
		}

		// Handle path branching
		newPaths, err := e.handlePathBranching(ctx, currentStep, path.id)
		if err != nil {
			e.recorder.RecordStepFailed(path.id, currentStep.Name(), err)
			e.recorder.RecordPathFailed(path.id, err)
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

			e.recorder.RecordPathBranched(path.id, currentStep.Name(), branchInfo)
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
			e.recorder.RecordPathCompleted(path.id, currentStep.Name())
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
	e.recorder.RecordStepStarted(pathID, step.Name(), step.Type(), step.Parameters())

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
			e.state.Set(varName, result.Content)
			e.logger.Info("stored step result", "variable_name", varName)
		}
	}

	// Print step output if formatter is available
	if e.formatter != nil {
		e.formatter.PrintStepOutput(step.Name(), result.Content)
	}

	// Record step completed
	e.recorder.RecordStepCompleted(pathID, step.Name(), result.Content, step.Name())

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
	globals := make(map[string]any)

	// Add safe built-in functions (same filtering as conditions)
	// unsafeFunctions := map[string]bool{
	// 	"read":    true,
	// 	"write":   true,
	// 	"exec":    true,
	// 	"open":    true,
	// 	"file":    true,
	// 	"http":    true,
	// 	"fetch":   true,
	// 	"request": true,
	// 	"system":  true,
	// 	"shell":   true,
	// 	"env":     true,
	// 	"getenv":  true,
	// 	"setenv":  true,
	// 	"command": true,
	// 	"proc":    true,
	// 	"process": true,
	// 	"socket":  true,
	// 	"network": true,
	// 	"tcp":     true,
	// 	"udp":     true,
	// 	"tls":     true,
	// 	"crypto":  true,
	// 	"hash":    true,
	// 	"rand":    true, // exclude random functions for determinism
	// 	"random":  true,
	// }

	for k, v := range all.Builtins() {
		globals[k] = v
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
	// Deterministic: get agent
	agent := step.Agent()
	if agent == nil {
		var found bool
		agent, found = e.environment.DefaultAgent()
		if !found {
			return nil, fmt.Errorf("no agent specified for prompt step %q", step.Name())
		}
	}

	// Deterministic: prepare prompt by evaluating template
	prompt := step.Prompt()
	if strings.Contains(prompt, "${") {
		evaluatedPrompt, err := e.evaluateTemplate(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate prompt template in step %q (type: %s): %w", step.Name(), step.Type(), err)
		}
		prompt = evaluatedPrompt
	}

	// Deterministic: prepare content
	var content []llm.Content
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
	if prompt != "" {
		content = append(content, &llm.TextContent{Text: prompt})
	}

	// Operation: LLM call (non-deterministic)
	// Use deterministic parameters for operation ID generation
	op := Operation{
		Type:     "agent_response",
		StepName: step.Name(),
		PathID:   pathID,
		Parameters: map[string]interface{}{
			"agent":  agent.Name(),
			"prompt": prompt,
			// Don't include content array in parameters as it contains memory addresses
		},
	}

	responseInterface, err := e.ExecuteOperation(ctx, op, func() (interface{}, error) {
		return agent.CreateResponse(ctx, dive.WithMessage(llm.NewUserMessage(content...)))
	})

	if err != nil {
		return nil, fmt.Errorf("agent response operation failed: %w", err)
	}

	// Deterministic: process result
	response := responseInterface.(*dive.Response)
	var usage llm.Usage
	if response.Usage != nil {
		usage = *response.Usage
	}
	return &dive.StepResult{
		Content: response.OutputText(),
		Usage:   usage,
	}, nil
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

// buildConditionGlobals creates a safe, read-only globals map for condition evaluation
func (e *Execution) buildConditionGlobals() map[string]any {
	globals := make(map[string]any)

	// Add safe built-in functions (exclude potentially dangerous ones)
	unsafeFunctions := map[string]bool{
		"read":    true,
		"write":   true,
		"exec":    true,
		"open":    true,
		"file":    true,
		"http":    true,
		"fetch":   true,
		"request": true,
		"system":  true,
		"shell":   true,
		"env":     true,
		"getenv":  true,
		"setenv":  true,
		"command": true,
		"proc":    true,
		"process": true,
		"socket":  true,
		"network": true,
		"tcp":     true,
		"udp":     true,
		"tls":     true,
		"crypto":  true,
		"hash":    true,
		"rand":    true, // exclude random functions for determinism
		"random":  true,
	}

	for k, v := range all.Builtins() {
		if !unsafeFunctions[k] {
			globals[k] = v
		}
	}

	// Add workflow inputs (read-only)
	globals["inputs"] = e.inputs

	// Add current state variables (read-only snapshot)
	globals["state"] = e.state.Copy()

	return globals
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
	globals := e.buildConditionGlobals()

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

	builtins := all.Builtins()

	globals := make(map[string]any, len(builtins)+len(e.inputs)+2)

	// Add current state variables (read-only snapshot)
	globals["state"] = e.state.Copy()

	for k, v := range builtins {
		globals[k] = v
	}
	for k, v := range e.inputs {
		globals[k] = v
	}
	if e.environment.documentRepo != nil {
		globals["documents"] = objects.NewDocumentRepository(e.environment.documentRepo)
	}
	inputs := map[string]any{}
	for k, v := range e.inputs {
		inputs[k] = v
	}
	globals["inputs"] = inputs

	return eval.Eval(ctx, s, globals)
}
