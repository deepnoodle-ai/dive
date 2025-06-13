package environment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
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
	"go.jetify.com/typeid"
)

// NewExecutionID creates a new execution id
func NewExecutionID() string {
	value, err := typeid.WithPrefix("exec")
	if err != nil {
		log.Fatalf("error creating new id: %v", err)
	}
	return value.String()
}

// ExecutionOptions are the options for creating a new execution.
type ExecutionOptions struct {
	WorkflowName   string
	Inputs         map[string]interface{}
	Outputs        map[string]interface{}
	Logger         slogger.Logger
	Formatter      WorkflowFormatter
	EventStore     workflow.ExecutionEventStore
	EventBatchSize int
}

// EventBasedExecution extends Execution with event recording capabilities
type EventBasedExecution struct {
	id            string
	environment   *Environment
	workflow      *workflow.Workflow
	status        Status
	startTime     time.Time
	endTime       time.Time
	inputs        map[string]interface{}
	outputs       map[string]interface{}
	err           error
	logger        slogger.Logger
	paths         map[string]*PathState
	scriptGlobals map[string]any
	formatter     WorkflowFormatter
	mutex         sync.RWMutex
	doneWg        sync.WaitGroup
	eventStore    workflow.ExecutionEventStore
	eventBuffer   []*workflow.ExecutionEvent
	eventSequence int64
	replayMode    bool
	batchSize     int
	bufferMutex   sync.Mutex
}

// NewEventBasedExecution creates a new event-based execution
func NewEventBasedExecution(env *Environment, opts ExecutionOptions) (*EventBasedExecution, error) {
	if opts.EventStore == nil {
		return nil, fmt.Errorf("event store is required")
	}
	if opts.WorkflowName == "" {
		return nil, fmt.Errorf("workflow name is required")
	}
	if !env.started {
		return nil, fmt.Errorf("environment not started")
	}

	workflowName := opts.WorkflowName
	wf, exists := env.workflows[workflowName]
	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", workflowName)
	}

	inputs := opts.Inputs
	if inputs == nil {
		inputs = make(map[string]interface{})
	}

	logger := opts.Logger
	if logger == nil {
		logger = env.logger
	}

	batchSize := opts.EventBatchSize
	if batchSize <= 0 {
		batchSize = 10
	}

	processedInputs := make(map[string]interface{}, len(wf.Inputs()))
	for _, input := range wf.Inputs() {
		value, exists := inputs[input.Name]
		if !exists {
			if input.Default != nil {
				processedInputs[input.Name] = input.Default
				continue
			}
			return nil, fmt.Errorf("required input %q not provided", input.Name)
		}
		processedInputs[input.Name] = value
	}

	// Set up global variables for execution scripting
	scriptGlobals := map[string]any{
		"inputs": processedInputs,
	}
	for k, v := range all.Builtins() {
		scriptGlobals[k] = v
	}
	if env.documentRepo != nil {
		scriptGlobals["documents"] = objects.NewDocumentRepository(env.documentRepo)
	}

	execution := &EventBasedExecution{
		id:            NewExecutionID(),
		environment:   env,
		workflow:      wf,
		status:        StatusPending,
		startTime:     time.Now(),
		inputs:        processedInputs,
		logger:        logger,
		paths:         make(map[string]*PathState),
		formatter:     opts.Formatter,
		scriptGlobals: scriptGlobals,
		eventStore:    opts.EventStore,
		eventBuffer:   make([]*workflow.ExecutionEvent, 0, batchSize),
		eventSequence: 0,
		replayMode:    false,
		batchSize:     batchSize,
	}
	execution.doneWg.Add(1)
	return execution, nil
}

func (e *EventBasedExecution) ID() string {
	return e.id
}

func (e *EventBasedExecution) Workflow() *workflow.Workflow {
	return e.workflow
}

func (e *EventBasedExecution) Environment() *Environment {
	return e.environment
}

func (e *EventBasedExecution) Status() Status {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return e.status
}

func (e *EventBasedExecution) StartTime() time.Time {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return e.startTime
}

func (e *EventBasedExecution) EndTime() time.Time {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return e.endTime
}

func (e *EventBasedExecution) Wait() error {
	e.doneWg.Wait()
	return e.err
}

// recordEvent records an execution event
func (e *EventBasedExecution) recordEvent(eventType workflow.ExecutionEventType, pathID, stepName string, data map[string]interface{}) {
	if e.replayMode {
		return // Don't record events during replay
	}

	event := &workflow.ExecutionEvent{
		ID:          generateEventID(),
		ExecutionID: e.id,
		PathID:      pathID,
		Sequence:    atomic.AddInt64(&e.eventSequence, 1),
		Timestamp:   time.Now(),
		EventType:   eventType,
		StepName:    stepName,
		Data:        data,
	}

	e.bufferMutex.Lock()
	e.eventBuffer = append(e.eventBuffer, event)
	shouldFlush := len(e.eventBuffer) >= e.batchSize
	e.bufferMutex.Unlock()

	if shouldFlush {
		e.flushEvents()
	}
}

// flushEvents writes buffered events to the event store
func (e *EventBasedExecution) flushEvents() error {
	e.bufferMutex.Lock()
	if len(e.eventBuffer) == 0 {
		e.bufferMutex.Unlock()
		return nil
	}

	// We'll work with a copy of the buffer and clear the primary one
	events := make([]*workflow.ExecutionEvent, len(e.eventBuffer))
	copy(events, e.eventBuffer)
	e.eventBuffer = e.eventBuffer[:0]
	e.bufferMutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if e.eventStore == nil {
		fmt.Println("WTF")
	}

	if err := e.eventStore.AppendEvents(ctx, events); err != nil {
		e.logger.Error("failed to flush events", "error", err, "event_count", len(events))
		return err
	}

	e.logger.Debug("flushed events", "event_count", len(events))
	return nil
}

// Flush any buffered events immediately
func (e *EventBasedExecution) Flush() error {
	return e.flushEvents()
}

// Run the execution to completion. This is a blocking call.
func (e *EventBasedExecution) Run(ctx context.Context) error {
	defer e.doneWg.Done()

	// Record execution started event
	e.recordEvent(workflow.EventExecutionStarted, "", "", map[string]interface{}{
		"workflow_name": e.workflow.Name(),
		"inputs":        e.inputs,
	})

	// Run the workflow to completion
	err := e.run(ctx)

	// Lock while we do post-execution cleanup
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.endTime = time.Now()
	if err != nil {
		e.logger.Error("workflow execution failed", "error", err)
		e.status = StatusFailed
		e.err = err
		e.recordEvent(workflow.EventExecutionFailed, "", "", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		e.logger.Info("workflow execution completed", "execution_id", e.id)
		e.status = StatusCompleted
		e.err = nil
		e.recordEvent(workflow.EventExecutionCompleted, "", "", map[string]interface{}{
			"outputs": e.outputs,
		})
	}

	if flushErr := e.Flush(); flushErr != nil {
		e.logger.Error("failed to flush final events", "error", flushErr)
	}
	if snapshotErr := e.saveSnapshot(); snapshotErr != nil {
		e.logger.Error("failed to save final snapshot", "error", snapshotErr)
	}
	return err
}

// run is our custom implementation that uses event recording
func (e *EventBasedExecution) run(ctx context.Context) error {
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

// addPath adds a new path to the execution
func (e *EventBasedExecution) addPath(path *executionPath) *PathState {
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

// saveSnapshot saves the current execution state as a snapshot
func (e *EventBasedExecution) saveSnapshot() error {
	snapshot := &workflow.ExecutionSnapshot{
		ID:           e.id,
		WorkflowName: e.workflow.Name(),
		Status:       string(e.status), // Use direct field access to avoid mutex lock
		StartTime:    e.startTime,      // Use direct field access
		EndTime:      e.endTime,        // Use direct field access
		LastEventSeq: atomic.LoadInt64(&e.eventSequence),
		Inputs:       e.inputs,
		Outputs:      e.outputs,
	}

	if e.err != nil {
		snapshot.Error = e.err.Error()
	}

	// TODO: Add workflow hash and inputs hash computation

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return e.eventStore.SaveSnapshot(ctx, snapshot)
}

// generateEventID generates a unique event ID
func generateEventID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Override runPath to add event recording
func (e *EventBasedExecution) runPath(ctx context.Context, path *executionPath, updates chan<- pathUpdate) {
	// Record path started event
	e.recordEvent(workflow.EventPathStarted, path.id, "", map[string]interface{}{
		"current_step": path.currentStep.Name(),
	})

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

		// Execute the step with event recording
		result, err := e.handleStepExecution(ctx, path, agent)
		if err != nil {
			// Record step failed event
			e.recordEvent(workflow.EventStepFailed, path.id, currentStep.Name(), map[string]interface{}{
				"error": err.Error(),
			})

			e.updatePathError(path.id, err)

			// Record path failed event
			e.recordEvent(workflow.EventPathFailed, path.id, "", map[string]interface{}{
				"error": err.Error(),
			})

			updates <- pathUpdate{pathID: path.id, err: err}
			return
		}

		// Handle path branching
		newPaths, err := e.handlePathBranching(ctx, currentStep, path.id, getNextPathID)
		if err != nil {
			e.recordEvent(workflow.EventStepFailed, path.id, currentStep.Name(), map[string]interface{}{
				"error": err.Error(),
			})
			e.updatePathError(path.id, err)
			e.recordEvent(workflow.EventPathFailed, path.id, "", map[string]interface{}{
				"error": err.Error(),
			})
			updates <- pathUpdate{pathID: path.id, err: err}
			return
		}

		// Record path branching if multiple paths created
		if len(newPaths) > 1 {
			pathData := make([]map[string]interface{}, len(newPaths))
			for i, newPath := range newPaths {
				pathData[i] = map[string]interface{}{
					"id":           newPath.id,
					"current_step": newPath.currentStep.Name(),
				}
			}

			e.recordEvent(workflow.EventPathBranched, path.id, currentStep.Name(), map[string]interface{}{
				"new_paths": pathData,
			})
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

			// Record path completed event
			e.recordEvent(workflow.EventPathCompleted, path.id, "", map[string]interface{}{
				"final_step": currentStep.Name(),
			})
			return
		}

		// We have exactly one path still. Continue running it.
		path = newPaths[0]
	}
}

// updatePathError updates the path state with an error
func (e *EventBasedExecution) updatePathError(pathID string, err error) {
	e.updatePathState(pathID, func(state *PathState) {
		state.Status = PathStatusFailed
		state.Error = err
		state.EndTime = time.Now()
	})
}

// handlePathBranching processes the next steps and creates new paths if needed
func (e *EventBasedExecution) handlePathBranching(
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

// Override handleStepExecution to add event recording
func (e *EventBasedExecution) handleStepExecution(ctx context.Context, path *executionPath, agent dive.Agent) (*dive.StepResult, error) {
	step := path.currentStep

	// Record step started event
	e.recordEvent(workflow.EventStepStarted, path.id, step.Name(), map[string]interface{}{
		"step_type":   step.Type(),
		"step_params": step.Parameters(),
	})

	e.updatePathState(path.id, func(state *PathState) { state.CurrentStep = step })

	var err error
	var result *dive.StepResult

	if step.Each() != nil {
		result, err = e.executeStepEach(ctx, step, agent)
	} else {
		result, err = e.executeStepCore(ctx, step, agent)
	}
	if err != nil {
		return nil, err
	}

	if step.Each() == nil {
		// Store the output in a variable if specified
		if varName := step.Store(); varName != "" {
			e.scriptGlobals[varName] = result.Content
			e.logger.Debug("stored step result", "variable_name", varName)
		}
		// Update path state with step output
		e.updatePathState(path.id, func(state *PathState) {
			if state.StepOutputs == nil {
				state.StepOutputs = make(map[string]string)
			}
			state.StepOutputs[step.Name()] = result.Content
		})
	}

	// Record step completed event
	eventData := map[string]interface{}{
		"output": result.Content,
	}
	if varName := step.Store(); varName != "" {
		eventData["stored_variable"] = varName
	}
	e.recordEvent(workflow.EventStepCompleted, path.id, step.Name(), eventData)

	return result, nil
}

func (e *EventBasedExecution) executeStepCore(ctx context.Context, step *workflow.Step, agent dive.Agent) (*dive.StepResult, error) {
	var err error
	var result *dive.StepResult

	if e.formatter != nil {
		e.formatter.PrintStepStart(step.Name(), step.Type())
	}

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
		if e.formatter != nil {
			e.formatter.PrintStepError(step.Name(), err)
		}
		return nil, err
	}

	if e.formatter != nil {
		e.formatter.PrintStepOutput(step.Name(), result.Content)
	}
	return result, nil
}

// executeStepEach handles the execution of a step that has an each block
func (e *EventBasedExecution) executeStepEach(ctx context.Context, step *workflow.Step, agent dive.Agent) (*dive.StepResult, error) {
	each := step.Each()

	// Resolve the items to iterate over
	items, err := e.resolveEachItems(ctx, each)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve each items: %w", err)
	}

	// Execute the step for each item and capture the results
	results := make([]*dive.StepResult, 0, len(items))
	for _, item := range items {
		if varName := each.As; varName != "" {
			e.scriptGlobals[varName] = item
		}
		result, err := e.executeStepCore(ctx, step, agent)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	// Store the array of results if a store variable is specified
	if varName := step.Store(); varName != "" {
		resultContents := make([]object.Object, 0, len(results))
		for _, result := range results {
			resultContents = append(resultContents, object.NewString(result.Content))
		}
		e.scriptGlobals[varName] = object.NewList(resultContents)
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

func (e *EventBasedExecution) handlePromptStep(ctx context.Context, step *workflow.Step, agent dive.Agent) (*dive.StepResult, error) {
	promptTemplate := step.Prompt()
	if promptTemplate == "" && len(step.Content()) == 0 {
		return nil, fmt.Errorf("prompt step %q has no prompt", step.Name())
	}

	prompt, err := e.evalString(ctx, promptTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to create prompt template: %w", err)
	}

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
	if promptTemplate != "" {
		content = append(content, &llm.TextContent{Text: prompt})
	}

	result, err := agent.CreateResponse(ctx, dive.WithMessage(llm.NewUserMessage(content...)))
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

func (e *EventBasedExecution) handleActionStep(ctx context.Context, step *workflow.Step) (*dive.StepResult, error) {
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

// updatePathState updates the state of a path
func (e *EventBasedExecution) updatePathState(pathID string, updateFn func(*PathState)) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if state, exists := e.paths[pathID]; exists {
		updateFn(state)
	}
}

// SaveSnapshot saves the current execution state
func (e *EventBasedExecution) SaveSnapshot() error {
	return e.saveSnapshot()
}

func (e *EventBasedExecution) StepOutputs() map[string]string {
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

func evalEachScript(ctx context.Context, code *compiler.Code, globals map[string]any) ([]any, error) {
	result, err := risor.EvalCode(ctx, code, risor.WithGlobals(globals))
	if err != nil {
		return nil, err
	}
	return convertRisorEachValue(result)
}

// evaluateRisorCondition evaluates a risor condition and returns the result as a boolean
func (e *EventBasedExecution) evaluateRisorCondition(ctx context.Context, codeStr string, logger slogger.Logger) (bool, error) {
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

func convertRisorEachValue(obj object.Object) ([]any, error) {
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
			value, err := convertRisorEachValue(item)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	case *object.Set:
		var values []any
		for _, item := range obj.Value() {
			value, err := convertRisorEachValue(item)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	case *object.Map:
		var values []any
		for _, item := range obj.Value() {
			value, err := convertRisorEachValue(item)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unsupported risor result type: %T", obj)
	}
}

func (e *EventBasedExecution) evalString(ctx context.Context, s string) (string, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	result, err := eval.Eval(ctx, s, e.scriptGlobals)
	if err != nil {
		return "", err
	}
	return result, nil
}

// resolveEachItems resolves the array of items from either a direct array or a risor expression
func (e *EventBasedExecution) resolveEachItems(ctx context.Context, each *workflow.EachBlock) ([]any, error) {
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

// evaluateRisorExpression evaluates a risor expression and returns the result as a string array
func (e *EventBasedExecution) evaluateRisorExpression(ctx context.Context, codeStr string) ([]any, error) {
	code := strings.TrimSuffix(strings.TrimPrefix(codeStr, "$("), ")")

	compiledCode, err := compileScript(ctx, code, e.scriptGlobals)
	if err != nil {
		e.logger.Error("failed to compile 'each' expression", "error", err)
		return nil, fmt.Errorf("failed to compile expression: %w", err)
	}

	result, err := evalEachScript(ctx, compiledCode, e.scriptGlobals)
	if err != nil {
		e.logger.Error("failed to evaluate 'each' expression", "error", err)
		return nil, fmt.Errorf("failed to evaluate expression: %w", err)
	}
	return result, nil
}

// GetError returns the top-level execution error, if any.
func (e *EventBasedExecution) GetError() error {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return e.err
}

// PathStates returns a copy of all path states in the execution.
func (e *EventBasedExecution) PathStates() []*PathState {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	paths := make([]*PathState, 0, len(e.paths))
	for _, state := range e.paths {
		paths = append(paths, state.Copy())
	}
	return paths
}

// LoadFromSnapshot loads execution state from a snapshot and event history
func LoadFromSnapshot(ctx context.Context, env *Environment, snapshot *workflow.ExecutionSnapshot, eventStore workflow.ExecutionEventStore) (*EventBasedExecution, error) {
	// Verify the workflow exists
	if _, exists := env.workflows[snapshot.WorkflowName]; !exists {
		return nil, fmt.Errorf("workflow not found: %s", snapshot.WorkflowName)
	}

	exec, err := NewEventBasedExecution(env, ExecutionOptions{
		WorkflowName:   snapshot.WorkflowName,
		Inputs:         snapshot.Inputs,
		Outputs:        snapshot.Outputs,
		Logger:         env.logger,
		EventStore:     eventStore,
		EventBatchSize: 10,
	})
	if err != nil {
		return nil, err
	}

	// Restore state from snapshot
	exec.id = snapshot.ID
	exec.status = Status(snapshot.Status)
	exec.startTime = snapshot.StartTime
	exec.endTime = snapshot.EndTime
	exec.inputs = snapshot.Inputs
	exec.outputs = snapshot.Outputs
	exec.eventSequence = snapshot.LastEventSeq

	if snapshot.Error != "" {
		exec.err = fmt.Errorf(snapshot.Error)
	}
	return exec, nil
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
