package environment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/objects"
	"github.com/diveagents/dive/workflow"
	"github.com/risor-io/risor/modules/all"
)

// Type aliases for workflow persistence types - temporarily commented out
// type (
// 	ExecutionEvent      = workflow.ExecutionEvent
// 	ExecutionEventType  = workflow.ExecutionEventType
// 	ExecutionSnapshot   = workflow.ExecutionSnapshot
// 	ExecutionEventStore = workflow.ExecutionEventStore
// )

// EventBasedExecution extends Execution with event recording capabilities
type EventBasedExecution struct {
	*Execution
	eventStore    workflow.ExecutionEventStore
	eventBuffer   []*workflow.ExecutionEvent
	eventSequence int64
	replayMode    bool
	batchSize     int
	bufferMutex   sync.Mutex
}

// PersistenceConfig configures execution persistence behavior
type PersistenceConfig struct {
	EventStore    workflow.ExecutionEventStore
	BatchSize     int           // Number of events to buffer before flushing (default: 10)
	FlushInterval time.Duration // Not implemented yet
}

// NewEventBasedExecution creates a new event-based execution
func NewEventBasedExecution(env *Environment, opts ExecutionOptions, config *PersistenceConfig) (*EventBasedExecution, error) {
	if config == nil {
		return nil, fmt.Errorf("persistence config is required")
	}
	if config.EventStore == nil {
		return nil, fmt.Errorf("event store is required")
	}

	// Get the workflow
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

	// Build up the input variables with defaults and validation
	processedInputs := make(map[string]interface{})
	for _, input := range wf.Inputs() {
		value, exists := inputs[input.Name]
		if !exists {
			// If input doesn't exist, check if it has a default value
			if input.Default != nil {
				processedInputs[input.Name] = input.Default
				continue
			}
			return nil, fmt.Errorf("required input %q not provided", input.Name)
		}
		// Input exists, use the provided value
		processedInputs[input.Name] = value
	}

	// Create the base execution (following the same pattern as Environment.ExecuteWorkflow)
	execution := &Execution{
		id:            dive.NewID(),
		environment:   env,
		workflow:      wf,
		status:        StatusPending,
		startTime:     time.Now(),
		inputs:        processedInputs,
		logger:        logger,
		paths:         make(map[string]*PathState),
		formatter:     opts.Formatter,
		scriptGlobals: map[string]any{"inputs": processedInputs},
	}

	// Add document repository if available
	if env.documentRepo != nil {
		execution.scriptGlobals["documents"] = objects.NewDocumentRepository(env.documentRepo)
	}

	// Make Risor's default builtins available to embedded scripts
	for k, v := range all.Builtins() {
		execution.scriptGlobals[k] = v
	}

	batchSize := config.BatchSize
	if batchSize <= 0 {
		batchSize = 10 // Default batch size
	}

	eventExec := &EventBasedExecution{
		Execution:     execution,
		eventStore:    config.EventStore,
		eventBuffer:   make([]*workflow.ExecutionEvent, 0, batchSize),
		eventSequence: 0,
		replayMode:    false,
		batchSize:     batchSize,
	}

	return eventExec, nil
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

	// Take a copy of the buffer and clear it
	events := make([]*workflow.ExecutionEvent, len(e.eventBuffer))
	copy(events, e.eventBuffer)
	e.eventBuffer = e.eventBuffer[:0]
	e.bufferMutex.Unlock()

	// Write events to store
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := e.eventStore.AppendEvents(ctx, events); err != nil {
		e.logger.Error("failed to flush events", "error", err, "event_count", len(events))
		// TODO: Implement retry logic or error handling
		return err
	}

	e.logger.Debug("flushed events", "event_count", len(events))
	return nil
}

// ForceFlush forces flushing of any buffered events
func (e *EventBasedExecution) ForceFlush() error {
	return e.flushEvents()
}

// Run overrides the base Run method to add event recording
func (e *EventBasedExecution) Run(ctx context.Context) error {
	// Record execution started event
	e.recordEvent(workflow.EventExecutionStarted, "", "", map[string]interface{}{
		"workflow_name": e.workflow.Name(),
		"inputs":        e.inputs,
	})

	// Call our custom run method that uses event recording
	err := e.run(ctx)

	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.endTime = time.Now()
	if err != nil {
		e.logger.Error("workflow execution failed", "error", err)
		e.status = StatusFailed
		e.err = err

		// Record execution failed event
		e.recordEvent(workflow.EventExecutionFailed, "", "", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		e.logger.Info("workflow execution completed", "execution_id", e.id)
		e.status = StatusCompleted
		e.err = nil

		// Record execution completed event
		e.recordEvent(workflow.EventExecutionCompleted, "", "", map[string]interface{}{
			"outputs": e.outputs,
		})
	}

	// Flush any remaining events
	if flushErr := e.ForceFlush(); flushErr != nil {
		e.logger.Error("failed to flush final events", "error", flushErr)
	}

	// Save final snapshot
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

// Override handleStepExecution to add event recording
func (e *EventBasedExecution) handleStepExecution(ctx context.Context, path *executionPath, agent dive.Agent) (*dive.StepResult, error) {
	step := path.currentStep

	// Record step started event
	e.recordEvent(workflow.EventStepStarted, path.id, step.Name(), map[string]interface{}{
		"step_type":   step.Type(),
		"step_params": step.Parameters(),
	})

	e.updatePathState(path.id, func(state *PathState) {
		state.CurrentStep = step
	})

	var result *dive.StepResult
	var err error

	if step.Each() != nil {
		result, err = e.executeStepEach(ctx, step, agent)
	} else {
		result, err = e.executeStepCore(ctx, step, agent)
	}

	if err != nil {
		return nil, err
	}

	// Store the output in a variable if specified (only for non-each steps)
	if step.Each() == nil {
		if varName := step.Store(); varName != "" {
			e.scriptGlobals[varName] = result.Content
			e.logger.Info("stored step result", "variable_name", varName)
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

// SaveSnapshot saves the current execution state
func (e *EventBasedExecution) SaveSnapshot() error {
	return e.saveSnapshot()
}

// LoadFromSnapshot loads execution state from a snapshot and event history
func LoadFromSnapshot(ctx context.Context, env *Environment, snapshot *workflow.ExecutionSnapshot, eventStore workflow.ExecutionEventStore) (*EventBasedExecution, error) {
	// Verify the workflow exists
	if _, exists := env.workflows[snapshot.WorkflowName]; !exists {
		return nil, fmt.Errorf("workflow not found: %s", snapshot.WorkflowName)
	}

	// Create the event-based execution
	config := &PersistenceConfig{
		EventStore: eventStore,
		BatchSize:  10,
	}

	exec, err := NewEventBasedExecution(env, ExecutionOptions{
		WorkflowName: snapshot.WorkflowName,
		Inputs:       snapshot.Inputs,
		Outputs:      snapshot.Outputs,
		Logger:       env.logger,
	}, config)
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
