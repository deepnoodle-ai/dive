package environment

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
)

// Status represents the execution status
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
)

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

	// Current execution context
	currentStep   *workflow.Step
	currentPathID string

	// Logging
	logger slogger.Logger

	// Synchronization
	mutex sync.RWMutex
}

// ExecutionV2Options configures a new Execution
type ExecutionV2Options struct {
	Workflow    *workflow.Workflow
	Environment *Environment
	Inputs      map[string]interface{}
	EventStore  ExecutionEventStore
	Logger      slogger.Logger
	ReplayMode  bool
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
		currentPathID:    "main", // Simple single-path execution for now
		logger:           opts.Logger.With("execution_id", executionID),
	}, nil
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

	// Set current path ID
	if op.PathID == "" {
		op.PathID = e.currentPathID
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
		"operation_id": string(op.ID),
		"duration":     duration,
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

	// Record path started
	e.recorder.RecordPathStarted(e.currentPathID, e.workflow.Steps()[0].Name())

	// Execute workflow steps sequentially (simplified approach)
	err := e.executeSteps(ctx)

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

// executeSteps executes all workflow steps sequentially
func (e *Execution) executeSteps(ctx context.Context) error {
	steps := e.workflow.Steps()

	for i, step := range steps {
		e.currentStep = step

		e.logger.Info("executing step", "step_name", step.Name(), "step_type", step.Type())

		// Record step started
		e.recorder.RecordStepStarted(e.currentPathID, step.Name(), step.Type(), step.Parameters())

		// Execute the step based on its type
		result, err := e.executeStep(ctx, step)
		if err != nil {
			e.recorder.RecordStepFailed(e.currentPathID, step.Name(), err)
			return fmt.Errorf("step %q failed: %w", step.Name(), err)
		}

		// Store step result in state if it's not the last step
		if i < len(steps)-1 {
			e.state.Set(step.Name(), result.Content)
		} else {
			// Last step result goes to outputs
			e.outputs["result"] = result.Content
		}

		// Record step completed
		e.recorder.RecordStepCompleted(e.currentPathID, step.Name(), result.Content, step.Name())

		// If this is an end step, break
		if step.Type() == "end" {
			break
		}
	}

	// Record path completed
	e.recorder.RecordPathCompleted(e.currentPathID, e.currentStep.Name())

	return nil
}

// executeStep executes a single workflow step
func (e *Execution) executeStep(ctx context.Context, step *workflow.Step) (*dive.StepResult, error) {
	switch step.Type() {
	case "prompt":
		return e.executePromptStep(ctx, step)
	case "action":
		return e.executeActionStep(ctx, step)
	default:
		return nil, fmt.Errorf("unsupported step type: %s", step.Type())
	}
}

// executePromptStep executes a prompt step using an agent
func (e *Execution) executePromptStep(ctx context.Context, step *workflow.Step) (*dive.StepResult, error) {
	// Deterministic: get agent
	agent := step.Agent()
	if agent == nil {
		return nil, fmt.Errorf("no agent specified for prompt step %q", step.Name())
	}

	// Deterministic: prepare prompt by evaluating template
	prompt := step.Prompt()
	if strings.Contains(prompt, "${") {
		evaluatedPrompt, err := e.evaluateTemplate(prompt)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate prompt template: %w", err)
		}
		prompt = evaluatedPrompt
	}

	// Deterministic: prepare content
	content := []llm.Content{&llm.TextContent{Text: prompt}}
	if stepContent := step.Content(); len(stepContent) > 0 {
		content = append(content, stepContent...)
	}

	// Operation: LLM call (non-deterministic)
	op := Operation{
		Type:     "agent_response",
		StepName: step.Name(),
		PathID:   e.currentPathID,
		Parameters: map[string]interface{}{
			"agent":   agent.Name(),
			"prompt":  prompt,
			"content": content,
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
func (e *Execution) executeActionStep(ctx context.Context, step *workflow.Step) (*dive.StepResult, error) {
	actionName := step.Action()
	if actionName == "" {
		return nil, fmt.Errorf("no action specified for action step %q", step.Name())
	}

	// Deterministic: get action
	action, ok := e.environment.GetAction(actionName)
	if !ok {
		return nil, fmt.Errorf("action %q not found", actionName)
	}

	// Deterministic: prepare parameters by evaluating templates
	params := make(map[string]interface{})
	for name, value := range step.Parameters() {
		if strValue, ok := value.(string); ok && strings.Contains(strValue, "${") {
			evaluated, err := e.evaluateTemplate(strValue)
			if err != nil {
				return nil, fmt.Errorf("failed to evaluate parameter template: %w", err)
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
		PathID:   e.currentPathID,
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

	// Deterministic: process result
	var content string
	if result != nil {
		content = fmt.Sprintf("%v", result)
	}

	return &dive.StepResult{Content: content}, nil
}

// evaluateTemplate evaluates a template string using the current state
func (e *Execution) evaluateTemplate(template string) (string, error) {
	// Simple template evaluation for ${var} patterns
	result := template

	// Find all ${...} patterns
	start := 0
	for {
		startIdx := strings.Index(result[start:], "${")
		if startIdx == -1 {
			break
		}
		startIdx += start

		endIdx := strings.Index(result[startIdx:], "}")
		if endIdx == -1 {
			return "", fmt.Errorf("unclosed template variable in: %s", template)
		}
		endIdx += startIdx

		// Extract variable name
		varName := result[startIdx+2 : endIdx]

		// Get value from state
		value, exists := e.state.Get(varName)
		if !exists {
			return "", fmt.Errorf("variable %q not found in state", varName)
		}

		// Replace ${var} with the value
		valueStr := fmt.Sprintf("%v", value)
		result = result[:startIdx] + valueStr + result[endIdx+1:]

		// Update start position
		start = startIdx + len(valueStr)
	}

	return result, nil
}
