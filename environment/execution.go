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
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/fatih/color"
	"github.com/risor-io/risor"
	"github.com/risor-io/risor/compiler"
	"github.com/risor-io/risor/object"
	"github.com/risor-io/risor/parser"
)

var (
	colorGreen = color.New(color.FgGreen)
)

// PathStatus represents the current state of an execution path
type PathStatus string

const (
	PathStatusPending   PathStatus = "pending"
	PathStatusRunning   PathStatus = "running"
	PathStatusCompleted PathStatus = "completed"
	PathStatusFailed    PathStatus = "failed"
)

// PathState tracks the state of an execution path
type PathState struct {
	ID          string
	Status      PathStatus
	CurrentStep *workflow.Step
	StartTime   time.Time
	EndTime     time.Time
	Error       error
	StepOutputs map[string]string
}

// Copy returns a shallow copy of the path state.
func (p *PathState) Copy() *PathState {
	return &PathState{
		ID:          p.ID,
		Status:      p.Status,
		CurrentStep: p.CurrentStep,
		StartTime:   p.StartTime,
		EndTime:     p.EndTime,
		Error:       p.Error,
		StepOutputs: p.StepOutputs,
	}
}

// Status represents the current state of an execution
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

type executionPath struct {
	id          string
	currentStep *workflow.Step
}

type pathUpdate struct {
	pathID     string
	stepName   string
	stepOutput string
	newPaths   []*executionPath
	err        error
	isDone     bool // true if this path should be removed from active paths
}

// Execution represents a single run of a workflow
type Execution struct {
	id             string                 // Unique identifier for this execution
	environment    *Environment           // Environment that the execution belongs to
	workflow       *workflow.Workflow     // Workflow being executed
	status         Status                 // Current status of the execution
	startTime      time.Time              // When the execution started
	endTime        time.Time              // When the execution completed (or failed/canceled)
	inputs         map[string]interface{} // Input parameters for the workflow
	outputs        map[string]interface{} // Output values from the workflow
	err            error                  // Error if execution failed
	logger         slogger.Logger         // Logger for the execution
	paths          map[string]*PathState  // Track all paths by ID
	scriptGlobals  map[string]any
	showStepOutput bool           // Whether to show step output
	mutex          sync.RWMutex   // Mutex for thread-safe operations
	doneWg         sync.WaitGroup // Wait group for the execution
}

// ExecutionOptions are the options for creating a new execution.
type ExecutionOptions struct {
	WorkflowName   string
	Inputs         map[string]interface{}
	Outputs        map[string]interface{}
	Logger         slogger.Logger
	ShowStepOutput bool
}

func (e *Execution) ID() string {
	return e.id
}

func (e *Execution) Workflow() *workflow.Workflow {
	return e.workflow
}

func (e *Execution) Environment() *Environment {
	return e.environment
}

func (e *Execution) Status() Status {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return e.status
}

func (e *Execution) StartTime() time.Time {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return e.startTime
}

func (e *Execution) EndTime() time.Time {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return e.endTime
}

func (e *Execution) Wait() error {
	e.doneWg.Wait()
	return e.err
}

// Run starts the execution in a goroutine and returns immediately.
// Any validation errors are returned before the goroutine is started.
// Use Wait() to wait for completion and get the final error, if any.
func (e *Execution) Run(ctx context.Context) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.status != StatusPending {
		return fmt.Errorf("execution can only be run from pending state, current status: %s", e.status)
	}

	requiresAgent := false
	for _, step := range e.workflow.Steps() {
		if step.Type() == "prompt" {
			requiresAgent = true
		}
	}
	if requiresAgent && len(e.environment.Agents()) == 0 {
		return fmt.Errorf("execution requires an agent")
	}

	e.status = StatusRunning
	e.startTime = time.Now()
	e.doneWg.Add(1)
	go e.runSync(ctx)
	return nil
}

func (e *Execution) runSync(ctx context.Context) error {
	defer e.doneWg.Done()

	err := e.run(ctx)

	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.endTime = time.Now()
	if err != nil {
		e.logger.Error("workflow execution failed", "error", err)
		e.status = StatusFailed
		e.err = err
		return err
	}

	e.logger.Info("workflow execution completed", "execution_id", e.id)
	e.status = StatusCompleted
	e.err = nil
	return nil
}

func (e *Execution) run(ctx context.Context) error {
	graph := e.workflow.Graph()
	totalUsage := llm.Usage{}

	e.logger.Info(
		"workflow execution started",
		"workflow_name", e.workflow.Name(),
		"start_step", graph.Start().Name(),
	)

	// Channel for path updates
	updates := make(chan pathUpdate)
	activePaths := make(map[string]*executionPath)

	// Start initial path
	startStep := graph.Start()
	initialPath := &executionPath{
		id:          fmt.Sprintf("path-%d", 1),
		currentStep: startStep,
	}
	activePaths[initialPath.id] = initialPath
	e.addPath(initialPath)
	go e.runPath(ctx, initialPath, updates)

	e.logger.Info("started initial path", "path_id", initialPath.id)

	// Main control loop
	for len(activePaths) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case update := <-updates:
			if update.err != nil {
				e.updatePathState(update.pathID, func(state *PathState) {
					state.Status = PathStatusFailed
					state.Error = update.err
					state.EndTime = time.Now()
				})
				return update.err
			}

			// Store task output and update path state
			e.updatePathState(update.pathID, func(state *PathState) {
				state.StepOutputs[update.stepName] = update.stepOutput
				if update.isDone {
					state.Status = PathStatusCompleted
					state.EndTime = time.Now()
				}
			})

			// Remove path if it's done
			if update.isDone {
				delete(activePaths, update.pathID)
			}

			// Start any new paths
			for _, newPath := range update.newPaths {
				activePaths[newPath.id] = newPath
				e.addPath(newPath)
				go e.runPath(ctx, newPath, updates)
			}

			e.logger.Info("path update processed",
				"active_paths", len(activePaths),
				"completed_path", update.isDone,
				"new_paths", len(update.newPaths))
		}
	}

	// Check if any paths failed
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

	e.logger.Info(
		"workflow execution completed",
		"workflow_name", e.workflow.Name(),
		"total_usage", totalUsage,
	)
	return nil
}

// handlePathBranching processes the next steps and creates new paths if needed
func (e *Execution) handlePathBranching(
	ctx context.Context,
	step *workflow.Step,
	currentPathID string,
	getNextPathID func() string,
) ([]*executionPath, error) {
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
		match, err := e.evaluateRisorCondition(ctx, edge.Condition, e.logger)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate condition: %w", err)
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
			nextPathID = getNextPathID()
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

// resolveEachItems resolves the array of items from either a direct array or a risor expression
func (e *Execution) resolveEachItems(ctx context.Context, each *workflow.EachBlock) ([]string, error) {
	// Handle array of strings directly
	if strArray, ok := each.Items.([]string); ok {
		return strArray, nil
	}

	// Handle interface array by converting to strings
	if ifaceArray, ok := each.Items.([]interface{}); ok {
		strArray := make([]string, len(ifaceArray))
		for i, v := range ifaceArray {
			switch val := v.(type) {
			case string:
				strArray[i] = val
			case fmt.Stringer:
				strArray[i] = val.String()
			default:
				strArray[i] = fmt.Sprintf("%v", val)
			}
		}
		return strArray, nil
	}

	// Handle risor expression
	if codeStr, ok := each.Items.(string); ok && strings.HasPrefix(codeStr, "$(") && strings.HasSuffix(codeStr, ")") {
		return e.evaluateRisorExpression(ctx, codeStr)
	}

	return nil, fmt.Errorf("each items must be []string, []interface{}, or risor expression, got %T", each.Items)
}

// evaluateRisorExpression evaluates a risor expression and returns the result as a string array
func (e *Execution) evaluateRisorExpression(ctx context.Context, codeStr string) ([]string, error) {
	code := strings.TrimPrefix(codeStr, "$(")
	code = strings.TrimSuffix(code, ")")

	compiledCode, err := compileScript(ctx, code, e.scriptGlobals)
	if err != nil {
		e.logger.Error("failed to compile each array expression", "error", err)
		return nil, fmt.Errorf("failed to compile expression: %w", err)
	}
	result, err := evalCode(ctx, compiledCode, e.scriptGlobals)
	if err != nil {
		e.logger.Error("failed to evaluate each array expression", "error", err)
		return nil, fmt.Errorf("failed to evaluate expression: %w", err)
	}

	var resultItems []string
	switch v := result.(type) {
	case *object.List:
		for _, item := range v.Value() {
			resultItems = append(resultItems, item.Inspect())
		}
	case *object.String:
		resultItems = []string{v.Value()}
	default:
		return nil, fmt.Errorf("expression must evaluate to string or []string, got %T", result)
	}
	return resultItems, nil
}

func (e *Execution) executeStepCore(ctx context.Context, step *workflow.Step, agent dive.Agent) (*dive.StepResult, error) {
	var err error
	var result *dive.StepResult

	// Handle different step types
	switch step.Type() {
	case "prompt":
		result, err = e.handlePromptStep(ctx, step, agent)
	case "action":
		result, err = e.handleActionStep(ctx, step)
	default:
		return nil, fmt.Errorf("unknown step type: %s", step.Type())
	}
	if err != nil {
		return nil, err
	}

	if e.showStepOutput {
		trimmedContent := strings.TrimSpace(result.Content)
		if trimmedContent != "" {
			fmt.Printf("Step %q output:\n", step.Name())
			fmt.Println(colorGreen.Sprint(trimmedContent))
			fmt.Println()
		} else {
			fmt.Printf("Step %q completed with no output\n", step.Name())
		}
	}

	// Store the output in a variable if specified
	if varName := step.Store(); varName != "" {
		e.scriptGlobals[varName] = object.NewString(result.Content)
		e.logger.Info("stored step result", "variable_name", varName)
	}
	return result, nil
}

// handleStepExecution executes a single step and returns the result
func (e *Execution) handleStepExecution(ctx context.Context, path *executionPath, agent dive.Agent) (*dive.StepResult, error) {
	step := path.currentStep
	e.updatePathState(path.id, func(state *PathState) {
		state.CurrentStep = step
	})
	if step.Each() != nil {
		return e.executeStepEach(ctx, step, agent)
	}
	return e.executeStepCore(ctx, step, agent)
}

// executeStepEach handles the execution of a step that has an each block
func (e *Execution) executeStepEach(ctx context.Context, step *workflow.Step, agent dive.Agent) (*dive.StepResult, error) {
	each := step.Each()

	// Resolve the items to iterate over
	items, err := e.resolveEachItems(ctx, each)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve each items: %w", err)
	}

	// Execute the step for each item and capture the results
	var results []*dive.StepResult
	for _, item := range items {
		if varName := each.As; varName != "" {
			e.scriptGlobals[varName] = object.NewString(item)
		}
		result, err := e.executeStepCore(ctx, step, agent)
		if err != nil {
			return nil, fmt.Errorf("failed to handle step execution: %w", err)
		}
		results = append(results, result)
	}

	// Combine the results into one string which we put in a single task result
	var itemTexts []string
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

func (e *Execution) handlePromptStep(ctx context.Context, step *workflow.Step, agent dive.Agent) (*dive.StepResult, error) {
	promptTemplate := step.Prompt()
	if promptTemplate == "" {
		return nil, fmt.Errorf("prompt step %q has no prompt", step.Name())
	}

	prompt, err := e.evalString(ctx, promptTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to create prompt template: %w", err)
	}

	result, err := agent.CreateResponse(ctx, dive.WithInput(prompt))
	if err != nil {
		e.logger.Error("task execution failed", "step", step.Name(), "error", err)
		return nil, err
	}

	var usage llm.Usage
	for _, item := range result.Items {
		if item.Type == dive.ResponseItemTypeMessage && item.Usage != nil {
			usage.Add(item.Usage)
		}
	}

	return &dive.StepResult{
		Content: result.OutputText(),
		Usage:   usage,
	}, nil
}

func (e *Execution) handleActionStep(ctx context.Context, step *workflow.Step) (*dive.StepResult, error) {
	actionName := step.Action()
	if actionName == "" {
		return nil, fmt.Errorf("action step %q has no action name", step.Name())
	}

	action, ok := e.environment.GetAction(actionName)
	if !ok {
		return nil, fmt.Errorf("action %q not found", actionName)
	}

	params := make(map[string]interface{})
	for name, value := range step.Parameters() {
		// If the value is a string that looks like a template, evaluate it
		if strValue, ok := value.(string); ok && strings.Contains(strValue, "${") {
			evaluated, err := e.evalString(ctx, strValue)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate parameter template: %w", err)
			}
			params[name] = evaluated
		} else {
			params[name] = value
		}
	}

	result, err := action.Execute(ctx, params)
	if err != nil {
		e.logger.Error("action execution failed", "action", actionName, "error", err)
		return nil, err
	}

	var content string
	if result != nil {
		content = fmt.Sprintf("%v", result)
	}
	return &dive.StepResult{Content: content}, nil
}

// updatePathError updates the path state with an error
func (e *Execution) updatePathError(pathID string, err error) {
	e.updatePathState(pathID, func(state *PathState) {
		state.Status = PathStatusFailed
		state.Error = err
		state.EndTime = time.Now()
	})
}

// Runs a single execution path in its own goroutine. Returns when the path
// completes, fails, or splits into multiple new paths.
func (e *Execution) runPath(ctx context.Context, path *executionPath, updates chan<- pathUpdate) {
	nextPathID := 0
	getNextPathID := func() string {
		nextPathID++
		return fmt.Sprintf("%s-%d", path.id, nextPathID)
	}

	logger := e.logger.
		With("path_id", path.id).
		With("execution_id", e.id)

	logger.Info("running path", "step", path.currentStep.Name())

	for {
		// Update path state to running
		e.updatePathState(path.id, func(state *PathState) {
			state.Status = PathStatusRunning
			state.StartTime = time.Now()
		})

		currentStep := path.currentStep

		// Get agent for current task if it's a prompt step
		var agent dive.Agent
		if currentStep.Type() == "prompt" {
			if currentStep.Agent() != nil {
				agent = currentStep.Agent()
			} else {
				agent = e.environment.Agents()[0]
			}
		}

		// Execute the step
		result, err := e.handleStepExecution(ctx, path, agent)
		if err != nil {
			e.updatePathError(path.id, err)
			updates <- pathUpdate{pathID: path.id, err: err}
			return
		}

		// Handle path branching
		newPaths, err := e.handlePathBranching(ctx, currentStep, path.id, getNextPathID)
		if err != nil {
			e.updatePathError(path.id, err)
			updates <- pathUpdate{pathID: path.id, err: err}
			return
		}

		// Path is complete if there are no new paths
		isDone := len(newPaths) == 0 || len(newPaths) > 1

		// Send update
		var executeNewPaths []*executionPath
		if len(newPaths) > 1 {
			executeNewPaths = newPaths
		}
		updates <- pathUpdate{
			pathID:     path.id,
			stepName:   currentStep.Name(),
			stepOutput: result.Content,
			newPaths:   executeNewPaths,
			isDone:     isDone,
		}

		if isDone {
			e.updatePathState(path.id, func(state *PathState) {
				state.Status = PathStatusCompleted
				state.EndTime = time.Now()
			})
			return
		}

		// We have exactly one path still. Continue running it.
		path = newPaths[0]
	}
}

// compileScript compiles a risor script with the given globals
func compileScript(ctx context.Context, code string, globals map[string]any) (*compiler.Code, error) {
	ast, err := parser.Parse(ctx, code)
	if err != nil {
		return nil, err
	}

	var globalNames []string
	for name := range globals {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)

	return compiler.Compile(ast, compiler.WithGlobalNames(globalNames))
}

// updatePathState updates the state of a path
func (e *Execution) updatePathState(pathID string, updateFn func(*PathState)) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if state, exists := e.paths[pathID]; exists {
		updateFn(state)
	}
}

// addPath adds a new path to the execution
func (e *Execution) addPath(path *executionPath) *PathState {
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
	return state
}

func (e *Execution) evalString(ctx context.Context, s string) (string, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	result, err := eval.Eval(ctx, s, e.scriptGlobals)
	if err != nil {
		return "", err
	}
	return result, nil
}

// GetError returns the top-level execution error, if any.
func (e *Execution) GetError() error {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return e.err
}

// PathStates returns a copy of all path states in the execution.
func (e *Execution) PathStates() []*PathState {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	paths := make([]*PathState, 0, len(e.paths))
	for _, state := range e.paths {
		paths = append(paths, state.Copy())
	}
	return paths
}

// ExecutionStats provides statistics about the execution
type ExecutionStats struct {
	TotalPaths     int           `json:"total_paths"`
	ActivePaths    int           `json:"active_paths"`
	CompletedPaths int           `json:"completed_paths"`
	FailedPaths    int           `json:"failed_paths"`
	StartTime      time.Time     `json:"start_time"`
	EndTime        time.Time     `json:"end_time"`
	Duration       time.Duration `json:"duration"`
}

// GetStats returns current execution statistics
func (e *Execution) GetStats() ExecutionStats {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	stats := ExecutionStats{
		TotalPaths: len(e.paths),
		StartTime:  e.startTime,
		EndTime:    e.endTime,
	}
	for _, state := range e.paths {
		switch state.Status {
		case PathStatusRunning:
			stats.ActivePaths++
		case PathStatusCompleted:
			stats.CompletedPaths++
		case PathStatusFailed:
			stats.FailedPaths++
		}
	}
	if !e.endTime.IsZero() {
		stats.Duration = e.endTime.Sub(e.startTime)
	} else if !e.startTime.IsZero() {
		stats.Duration = time.Since(e.startTime)
	}
	return stats
}

func (e *Execution) StepOutputs() map[string]string {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	outputs := make(map[string]string)
	for _, pathState := range e.paths {
		for stepName, output := range pathState.StepOutputs {
			outputs[stepName] = output
		}
	}
	return outputs
}

func evalCode(ctx context.Context, code *compiler.Code, globals map[string]any) (any, error) {
	result, err := risor.EvalCode(ctx, code, risor.WithGlobals(globals))
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate code: %w", err)
	}
	switch result := result.(type) {
	case *object.String:
		return result.Value(), nil
	case *object.Int:
		return result.Value(), nil
	case *object.Float:
		return result.Value(), nil
	case *object.Bool:
		return result.Value(), nil
	default:
		return nil, fmt.Errorf("unsupported result type: %T", result)
	}
}

// evaluateRisorCondition evaluates a risor condition and returns the result as a boolean
func (e *Execution) evaluateRisorCondition(ctx context.Context, codeStr string, logger slogger.Logger) (bool, error) {
	if strings.HasPrefix(codeStr, "$(") && strings.HasSuffix(codeStr, ")") {
		codeStr = strings.TrimPrefix(codeStr, "$(")
		codeStr = strings.TrimSuffix(codeStr, ")")
	}
	compiledCode, err := compileScript(ctx, codeStr, e.scriptGlobals)
	if err != nil {
		logger.Error("failed to compile condition", "error", err)
		return false, fmt.Errorf("failed to compile expression: %w", err)
	}
	result, err := risor.EvalCode(ctx, compiledCode, risor.WithGlobals(e.scriptGlobals))
	if err != nil {
		return false, fmt.Errorf("failed to evaluate code: %w", err)
	}
	return result.IsTruthy(), nil
}
