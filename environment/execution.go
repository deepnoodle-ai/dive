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
	"github.com/gofrs/uuid/v5"
	"github.com/risor-io/risor"
	"github.com/risor-io/risor/compiler"
	"github.com/risor-io/risor/modules/all"
	"github.com/risor-io/risor/object"
	"github.com/risor-io/risor/parser"
)

// NewExecutionID returns a new UUID for execution identification
func NewExecutionID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id.String()
}

// ExecutionStatus represents the execution status
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
)

// ExecutionOptions configures a new execution
type ExecutionOptions struct {
	Workflow        *workflow.Workflow
	Environment     *Environment
	Inputs          map[string]interface{}
	OperationLogger OperationLogger
	Checkpointer    ExecutionCheckpointer
	Logger          slogger.Logger
	Formatter       WorkflowFormatter
	ExecutionID     string
}

// Execution represents a simplified workflow execution with checkpointing
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

	// Simple logging and checkpointing
	operationLogger OperationLogger
	checkpointer    ExecutionCheckpointer

	// State management
	state *WorkflowState

	// Path management for parallel execution
	paths       map[string]*PathState
	activePaths map[string]*executionPath
	pathUpdates chan pathUpdate
	pathCounter int

	// Token usage tracking
	totalUsage llm.Usage

	// Logging and formatting
	logger    slogger.Logger
	formatter WorkflowFormatter

	// Synchronization
	mutex  sync.RWMutex
	doneWg sync.WaitGroup
}

// NewExecution creates a new simplified execution
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
	if opts.OperationLogger == nil {
		opts.OperationLogger = NewNullOperationLogger()
	}
	if opts.Checkpointer == nil {
		opts.Checkpointer = NewNullCheckpointer()
	}

	executionID := opts.ExecutionID
	if executionID == "" {
		executionID = NewExecutionID()
	}

	state := NewWorkflowState(executionID, nil) // No recorder needed

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

	exec := &Execution{
		id:              executionID,
		workflow:        opts.Workflow,
		environment:     opts.Environment,
		inputs:          inputs,
		outputs:         make(map[string]interface{}),
		status:          ExecutionStatusPending,
		operationLogger: opts.OperationLogger,
		checkpointer:    opts.Checkpointer,
		state:           state,
		paths:           make(map[string]*PathState),
		activePaths:     make(map[string]*executionPath),
		pathUpdates:     make(chan pathUpdate, 100),
		logger:          opts.Logger.With("execution_id", executionID),
		formatter:       opts.Formatter,
	}

	return exec, nil
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

// ExecuteOperation implements simple operation execution with logging and checkpointing
func (e *Execution) ExecuteOperation(ctx context.Context, op Operation, fn func() (interface{}, error)) (interface{}, error) {
	// Generate operation ID if not set
	if op.ID == "" {
		op.ID = op.GenerateID()
	}

	// PathID should always be set by caller
	if op.PathID == "" {
		return nil, fmt.Errorf("operation PathID is required")
	}

	// Execute the operation
	startTime := time.Now()
	result, err := fn()
	duration := time.Since(startTime)

	// Log the operation
	logEntry := &OperationLogEntry{
		ID:            string(op.ID),
		ExecutionID:   e.id,
		StepName:      op.StepName,
		PathID:        op.PathID,
		OperationType: op.Type,
		Parameters:    op.Parameters,
		Result:        result,
		StartTime:     startTime,
		Duration:      duration,
	}

	if err != nil {
		logEntry.Error = err.Error()
	}

	// Log operation (fire and forget - don't fail execution if logging fails)
	if logErr := e.operationLogger.LogOperation(ctx, logEntry); logErr != nil {
		e.logger.Warn("failed to log operation", "error", logErr)
	}

	// Checkpoint after every operation for simplicity and reliability
	if checkpointErr := e.saveCheckpoint(ctx); checkpointErr != nil {
		e.logger.Warn("failed to save checkpoint", "error", checkpointErr)
		// Continue anyway - operation succeeded
	}

	return result, err
}

// saveCheckpoint saves the current execution state
func (e *Execution) saveCheckpoint(ctx context.Context) error {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	checkpoint := &ExecutionCheckpoint{
		ID:           e.id + "-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		ExecutionID:  e.id,
		WorkflowName: e.workflow.Name(),
		Status:       string(e.status),
		Inputs:       e.inputs,
		Outputs:      e.outputs,
		State:        e.state.Copy(),
		PathStates:   e.copyPathStates(),
		TotalUsage:   &e.totalUsage,
		StartTime:    e.startTime,
		EndTime:      e.endTime,
		CheckpointAt: time.Now(),
	}

	if e.err != nil {
		checkpoint.Error = e.err.Error()
	}

	return e.checkpointer.SaveCheckpoint(ctx, checkpoint)
}

// copyPathStates creates a copy of current path states
func (e *Execution) copyPathStates() map[string]*PathState {
	pathStates := make(map[string]*PathState)
	for id, state := range e.paths {
		// Create a copy of the path state
		pathStates[id] = &PathState{
			ID:          state.ID,
			Status:      state.Status,
			CurrentStep: state.CurrentStep,
			StartTime:   state.StartTime,
			EndTime:     state.EndTime,
			StepOutputs: make(map[string]string),
			Error:       state.Error,
		}
		// Copy step outputs
		for stepName, output := range state.StepOutputs {
			pathStates[id].StepOutputs[stepName] = output
		}
	}
	return pathStates
}

// LoadFromCheckpoint loads execution state from the latest checkpoint
func (e *Execution) LoadFromCheckpoint(ctx context.Context) error {
	checkpoint, err := e.checkpointer.LoadCheckpoint(ctx, e.id)
	if err != nil {
		return fmt.Errorf("failed to load checkpoint: %w", err)
	}

	if checkpoint == nil {
		// No checkpoint found - start fresh
		return nil
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.status = ExecutionStatus(checkpoint.Status)
	e.outputs = checkpoint.Outputs
	e.startTime = checkpoint.StartTime
	e.endTime = checkpoint.EndTime

	if checkpoint.Error != "" {
		e.err = fmt.Errorf("%s", checkpoint.Error)
	}

	if checkpoint.TotalUsage != nil {
		e.totalUsage = *checkpoint.TotalUsage
	}

	// Load state
	e.state.LoadFromMap(checkpoint.State)

	// Load path states
	e.paths = make(map[string]*PathState)
	for id, pathState := range checkpoint.PathStates {
		e.paths[id] = pathState
	}

	e.logger.Info("loaded execution from checkpoint",
		"status", e.status,
		"paths", len(e.paths),
		"state_keys", len(checkpoint.State))

	return nil
}

// Run executes the workflow to completion with simple checkpointing
func (e *Execution) Run(ctx context.Context) error {
	e.mutex.Lock()
	e.status = ExecutionStatusRunning
	e.startTime = time.Now()
	e.mutex.Unlock()

	// Try to load from checkpoint first
	if err := e.LoadFromCheckpoint(ctx); err != nil {
		e.logger.Warn("failed to load from checkpoint, starting fresh", "error", err)
	}

	// If already completed, return early
	if e.status == ExecutionStatusCompleted {
		e.logger.Info("execution already completed from checkpoint")
		return nil
	}

	// If failed, return the error
	if e.status == ExecutionStatusFailed {
		e.logger.Info("execution already failed from checkpoint")
		return e.err
	}

	// Start with initial path
	startStep := e.workflow.Start()
	initialPath := &executionPath{
		id:          "main",
		currentStep: startStep,
	}

	e.addPath(initialPath)

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
		e.logger.Error("workflow execution failed", "error", err)
	} else {
		e.status = ExecutionStatusCompleted
		e.logger.Info("workflow execution completed")
	}
	e.mutex.Unlock()

	// Final checkpoint
	if checkpointErr := e.saveCheckpoint(ctx); checkpointErr != nil {
		e.logger.Error("failed to save final checkpoint", "error", checkpointErr)
	}

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
			logger.Error("step failed", "step", currentStep.Name(), "error", err)
			e.pathUpdates <- pathUpdate{pathID: path.id, err: err}
			return
		}

		// Handle path branching
		newPaths, err := e.handlePathBranching(ctx, currentStep, path.id)
		if err != nil {
			e.pathUpdates <- pathUpdate{pathID: path.id, err: err}
			return
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

	// Update total usage if step has usage information
	if result.Usage.InputTokens > 0 || result.Usage.OutputTokens > 0 ||
		result.Usage.CacheCreationInputTokens > 0 || result.Usage.CacheReadInputTokens > 0 {
		e.mutex.Lock()
		e.totalUsage.Add(&result.Usage)
		e.mutex.Unlock()
	}

	return result, nil
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

// ScriptContext represents different contexts where scripts can run
type ScriptContext int

const (
	ScriptContextCondition ScriptContext = iota
	ScriptContextTemplate
	ScriptContextEach
	ScriptContextActivity
)

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

func (e *Execution) evaluateTemplate(ctx context.Context, s string) (string, error) {
	if strings.HasPrefix(s, "$(") && strings.HasSuffix(s, ")") {
		s = strings.TrimPrefix(s, "$(")
		s = strings.TrimSuffix(s, ")")
	}

	// Use deterministic context for template evaluation
	globals := e.buildGlobalsForContext(ScriptContextTemplate)

	return eval.Eval(ctx, s, globals)
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
	// Use simple parameters for operation ID generation
	op := Operation{
		Type:     "agent_response",
		StepName: step.Name(),
		PathID:   pathID,
		Parameters: map[string]interface{}{
			"agent":  agent.Name(),
			"prompt": prompt,
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

	// Prepare parameters by evaluating templates
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

	// Operation: execute action
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
	return &dive.StepResult{
		Content: content,
		Usage:   llm.Usage{}, // Explicitly initialize usage as zero
	}, nil
}

// executeScriptStep executes a script activity step
func (e *Execution) executeScriptStep(ctx context.Context, step *workflow.Step, pathID string) (*dive.StepResult, error) {
	script := step.Script()
	if script == "" {
		return nil, fmt.Errorf("no script specified for script activity step %q", step.Name())
	}

	// Prepare script globals
	globals := e.buildGlobalsForContext(ScriptContextActivity)

	// Operation: execute script
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
	return &dive.StepResult{
		Content: content,
		Object:  result,
		Usage:   llm.Usage{}, // Explicitly initialize usage as zero
	}, nil
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
	var combinedUsage llm.Usage
	for i, result := range results {
		var itemText string
		if each.As != "" {
			itemText = fmt.Sprintf("# %s: %s\n\n%s", each.As, items[i], result.Content)
		} else {
			itemText = fmt.Sprintf("# %s\n\n%s", items[i], result.Content)
		}
		itemTexts = append(itemTexts, itemText)

		// Aggregate token usage from each iteration
		combinedUsage.Add(&result.Usage)
	}

	return &dive.StepResult{
		Content: strings.Join(itemTexts, "\n\n"),
		Format:  dive.OutputFormatMarkdown,
		Usage:   combinedUsage,
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
