package environment

// // Execution represents a single run of a workflow
// type Execution struct {
// 	id            string                 // Unique identifier for this execution
// 	environment   *Environment           // Environment that the execution belongs to
// 	workflow      *workflow.Workflow     // Workflow being executed
// 	status        Status                 // Current status of the execution
// 	startTime     time.Time              // When the execution started
// 	endTime       time.Time              // When the execution completed (or failed/canceled)
// 	inputs        map[string]interface{} // Input parameters for the workflow
// 	outputs       map[string]interface{} // Output values from the workflow
// 	err           error                  // Error if execution failed
// 	logger        slogger.Logger         // Logger for the execution
// 	paths         map[string]*PathState  // Track all paths by ID
// 	scriptGlobals map[string]any
// 	formatter     WorkflowFormatter
// 	mutex         sync.RWMutex
// 	doneWg        sync.WaitGroup
// }

// // Run starts the execution in a goroutine and returns immediately.
// // Any validation errors are returned before the goroutine is started.
// // Use Wait() to wait for completion and get the final error, if any.
// func (e *Execution) Run(ctx context.Context) error {
// 	e.mutex.Lock()
// 	defer e.mutex.Unlock()

// 	if e.status != StatusPending {
// 		return fmt.Errorf("execution can only be run from pending state, current status: %s", e.status)
// 	}

// 	requiresAgent := false
// 	for _, step := range e.workflow.Steps() {
// 		if step.Type() == "prompt" {
// 			requiresAgent = true
// 		}
// 	}
// 	if requiresAgent && len(e.environment.Agents()) == 0 {
// 		return fmt.Errorf("execution requires an agent")
// 	}

// 	e.status = StatusRunning
// 	e.startTime = time.Now()
// 	e.doneWg.Add(1)
// 	go e.runSync(ctx)
// 	return nil
// }

// func (e *Execution) runSync(ctx context.Context) error {
// 	defer e.doneWg.Done()

// 	err := e.run(ctx)

// 	e.mutex.Lock()
// 	defer e.mutex.Unlock()

// 	e.endTime = time.Now()
// 	if err != nil {
// 		e.logger.Error("workflow execution failed", "error", err)
// 		e.status = StatusFailed
// 		e.err = err
// 		return err
// 	}

// 	e.logger.Info("workflow execution completed", "execution_id", e.id)
// 	e.status = StatusCompleted
// 	e.err = nil
// 	return nil
// }

// func (e *Execution) run(ctx context.Context) error {
// 	graph := e.workflow.Graph()
// 	totalUsage := llm.Usage{}

// 	e.logger.Info(
// 		"workflow execution started",
// 		"workflow_name", e.workflow.Name(),
// 		"start_step", graph.Start().Name(),
// 	)

// 	// Channel for path updates
// 	updates := make(chan pathUpdate)
// 	activePaths := make(map[string]*executionPath)

// 	// Start initial path
// 	startStep := graph.Start()
// 	initialPath := &executionPath{
// 		id:          fmt.Sprintf("path-%d", 1),
// 		currentStep: startStep,
// 	}
// 	activePaths[initialPath.id] = initialPath
// 	e.addPath(initialPath)
// 	go e.runPath(ctx, initialPath, updates)

// 	e.logger.Info("started initial path", "path_id", initialPath.id)

// 	// Main control loop
// 	for len(activePaths) > 0 {
// 		select {
// 		case <-ctx.Done():
// 			return ctx.Err()
// 		case update := <-updates:
// 			if update.err != nil {
// 				e.updatePathState(update.pathID, func(state *PathState) {
// 					state.Status = PathStatusFailed
// 					state.Error = update.err
// 					state.EndTime = time.Now()
// 				})
// 				return update.err
// 			}

// 			// Store task output and update path state
// 			e.updatePathState(update.pathID, func(state *PathState) {
// 				state.StepOutputs[update.stepName] = update.stepOutput
// 				if update.isDone {
// 					state.Status = PathStatusCompleted
// 					state.EndTime = time.Now()
// 				}
// 			})

// 			// Remove path if it's done
// 			if update.isDone {
// 				delete(activePaths, update.pathID)
// 			}

// 			// Start any new paths
// 			for _, newPath := range update.newPaths {
// 				activePaths[newPath.id] = newPath
// 				e.addPath(newPath)
// 				go e.runPath(ctx, newPath, updates)
// 			}

// 			e.logger.Info("path update processed",
// 				"active_paths", len(activePaths),
// 				"completed_path", update.isDone,
// 				"new_paths", len(update.newPaths))
// 		}
// 	}

// 	// Check if any paths failed
// 	e.mutex.RLock()
// 	var failedPaths []string
// 	for _, state := range e.paths {
// 		if state.Status == PathStatusFailed {
// 			failedPaths = append(failedPaths, state.ID)
// 		}
// 	}
// 	e.mutex.RUnlock()

// 	if len(failedPaths) > 0 {
// 		return fmt.Errorf("execution completed with failed paths: %v", failedPaths)
// 	}

// 	e.logger.Info(
// 		"workflow execution completed",
// 		"workflow_name", e.workflow.Name(),
// 		"total_usage", totalUsage,
// 	)
// 	return nil
// }

// // handleStepExecution executes a single step and returns the result
// func (e *Execution) handleStepExecution(ctx context.Context, path *executionPath, agent dive.Agent) (*dive.StepResult, error) {
// 	step := path.currentStep
// 	e.updatePathState(path.id, func(state *PathState) {
// 		state.CurrentStep = step
// 	})
// 	if step.Each() != nil {
// 		return e.executeStepEach(ctx, step, agent)
// 	}
// 	result, err := e.executeStepCore(ctx, step, agent)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Store the output in a variable if specified (only for non-each steps)
// 	if varName := step.Store(); varName != "" {
// 		e.scriptGlobals[varName] = object.NewString(result.Content)
// 		e.logger.Info("stored step result", "variable_name", varName)
// 	}
// 	return result, nil
// }

// // Runs a single execution path in its own goroutine. Returns when the path
// // completes, fails, or splits into multiple new paths.
// func (e *Execution) runPath(ctx context.Context, path *executionPath, updates chan<- pathUpdate) {
// 	nextPathID := 0
// 	getNextPathID := func() string {
// 		nextPathID++
// 		return fmt.Sprintf("%s-%d", path.id, nextPathID)
// 	}

// 	logger := e.logger.
// 		With("path_id", path.id).
// 		With("execution_id", e.id)

// 	logger.Info("running path", "step", path.currentStep.Name())

// 	for {
// 		// Update path state to running
// 		e.updatePathState(path.id, func(state *PathState) {
// 			state.Status = PathStatusRunning
// 			state.StartTime = time.Now()
// 		})

// 		currentStep := path.currentStep

// 		// Get agent for current task if it's a prompt step
// 		var agent dive.Agent
// 		if currentStep.Type() == "prompt" {
// 			if currentStep.Agent() != nil {
// 				agent = currentStep.Agent()
// 			} else {
// 				agent = e.environment.Agents()[0]
// 			}
// 		}

// 		// Execute the step
// 		result, err := e.handleStepExecution(ctx, path, agent)
// 		if err != nil {
// 			e.updatePathError(path.id, err)
// 			updates <- pathUpdate{pathID: path.id, err: err}
// 			return
// 		}

// 		// Handle path branching
// 		newPaths, err := e.handlePathBranching(ctx, currentStep, path.id, getNextPathID)
// 		if err != nil {
// 			e.updatePathError(path.id, err)
// 			updates <- pathUpdate{pathID: path.id, err: err}
// 			return
// 		}

// 		// Path is complete if there are no new paths
// 		isDone := len(newPaths) == 0 || len(newPaths) > 1

// 		// Send update
// 		var executeNewPaths []*executionPath
// 		if len(newPaths) > 1 {
// 			executeNewPaths = newPaths
// 		}
// 		updates <- pathUpdate{
// 			pathID:     path.id,
// 			stepName:   currentStep.Name(),
// 			stepOutput: result.Content,
// 			newPaths:   executeNewPaths,
// 			isDone:     isDone,
// 		}

// 		if isDone {
// 			e.updatePathState(path.id, func(state *PathState) {
// 				state.Status = PathStatusCompleted
// 				state.EndTime = time.Now()
// 			})
// 			return
// 		}

// 		// We have exactly one path still. Continue running it.
// 		path = newPaths[0]
// 	}
// }

// // updatePathState updates the state of a path
// func (e *Execution) updatePathState(pathID string, updateFn func(*PathState)) {
// 	e.mutex.Lock()
// 	defer e.mutex.Unlock()

// 	if state, exists := e.paths[pathID]; exists {
// 		updateFn(state)
// 	}
// }
