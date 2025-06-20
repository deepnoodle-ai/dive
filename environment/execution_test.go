package environment

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

func TestNewExecution(t *testing.T) {
	// Create a simple workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "test-workflow",
		Inputs: []*workflow.Input{
			{Name: "input1", Type: "string", Required: true},
			{Name: "input2", Type: "string", Default: "default_value"},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "step1",
				Type:   "prompt",
				Prompt: "Test prompt: ${inputs.input1}",
			}),
		},
	})
	require.NoError(t, err)

	// Create test environment
	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	// Create null event store
	eventStore := NewNullEventStore()

	t.Run("valid execution creation", func(t *testing.T) {
		execution, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs:      map[string]interface{}{"input1": "test_value"},
			EventStore:  eventStore,
			Logger:      slogger.NewDevNullLogger(),
		})

		require.NoError(t, err)
		require.NotNil(t, execution)
		require.Equal(t, ExecutionStatusPending, execution.Status())
		require.NotEmpty(t, execution.ID())
		require.Equal(t, "test_value", execution.inputs["input1"])
		require.Equal(t, "default_value", execution.inputs["input2"])
	})

	t.Run("missing required input", func(t *testing.T) {
		_, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs:      map[string]interface{}{}, // Missing required input1
			EventStore:  eventStore,
			Logger:      slogger.NewDevNullLogger(),
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "input \"input1\" is required")
	})

	t.Run("unknown input", func(t *testing.T) {
		_, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs:      map[string]interface{}{"input1": "test", "unknown": "value"},
			EventStore:  eventStore,
			Logger:      slogger.NewDevNullLogger(),
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown input \"unknown\"")
	})

	t.Run("missing workflow", func(t *testing.T) {
		_, err := NewExecution(ExecutionOptions{
			Workflow:    nil,
			Environment: env,
			Inputs:      map[string]interface{}{"input1": "test"},
			EventStore:  eventStore,
			Logger:      slogger.NewDevNullLogger(),
		})

		require.Error(t, err)
		require.Contains(t, err.Error(), "workflow is required")
	})
}

func TestExecutionOperations(t *testing.T) {
	// Create test workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "test-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{Name: "step1", Type: "prompt", Prompt: "Test"}),
		},
	})
	require.NoError(t, err)

	// Create test environment
	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	// Create execution
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  NewNullEventStore(),
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	t.Run("successful operation execution", func(t *testing.T) {
		ctx := context.Background()
		op := NewOperation("test_type", "test_step", "test_path", map[string]interface{}{"param": "value"})

		expectedResult := "operation result"
		result, err := execution.ExecuteOperation(ctx, op, func() (interface{}, error) {
			return expectedResult, nil
		})

		require.NoError(t, err)
		require.Equal(t, expectedResult, result)

		// Verify operation is cached
		cachedResult, found := execution.FindOperationResult(op.ID)
		require.True(t, found)
		require.Equal(t, expectedResult, cachedResult.Result)
		require.NoError(t, cachedResult.Error)
	})

	t.Run("failed operation execution", func(t *testing.T) {
		ctx := context.Background()
		op := NewOperation("test_type", "test_step", "test_path", map[string]interface{}{"param": "value"})

		expectedError := fmt.Errorf("operation failed")
		result, err := execution.ExecuteOperation(ctx, op, func() (interface{}, error) {
			return nil, expectedError
		})

		require.Error(t, err)
		require.Equal(t, expectedError.Error(), err.Error())
		require.Nil(t, result)

		// Verify error is cached
		cachedResult, found := execution.FindOperationResult(op.ID)
		require.True(t, found)
		require.Nil(t, cachedResult.Result)
		require.Error(t, cachedResult.Error)
		require.Equal(t, expectedError.Error(), cachedResult.Error.Error())
	})
}

func TestExecutionPromptStep(t *testing.T) {
	// Create mock agent
	mockResponse := &dive.Response{
		Items: []*dive.ResponseItem{
			{
				Type:    dive.ResponseItemTypeMessage,
				Message: llm.NewAssistantMessage(&llm.TextContent{Text: "Mock response"}),
			},
		},
		Usage: &llm.Usage{InputTokens: 10, OutputTokens: 5},
	}

	mockAgent := agent.NewMockAgent(agent.MockAgentOptions{
		Name:     "TestAgent",
		Response: mockResponse,
	})

	// Create workflow with prompt step
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "prompt-test",
		Inputs: []*workflow.Input{
			{Name: "topic", Type: "string", Required: true},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "ask_question",
				Type:   "prompt",
				Prompt: "Tell me about ${inputs.topic}",
				Agent:  mockAgent,
			}),
		},
	})
	require.NoError(t, err)

	// Create environment with agent
	env := &Environment{
		agents:    map[string]dive.Agent{"TestAgent": mockAgent},
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	// Create execution
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{"topic": "artificial intelligence"},
		EventStore:  NewNullEventStore(),
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Execute prompt step
	ctx := context.Background()
	step := testWorkflow.Steps()[0]
	result, err := execution.executePromptStep(ctx, step, "test_path")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Mock response", result.Content)
	require.Equal(t, int(10), result.Usage.InputTokens)
	require.Equal(t, int(5), result.Usage.OutputTokens)
}

func TestExecutionScriptStep(t *testing.T) {
	// Create workflow with script step
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "script-test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "calculate",
				Type:   "script",
				Script: "2 + 3",
			}),
		},
	})
	require.NoError(t, err)

	// Create environment
	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	// Create execution
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  NewNullEventStore(),
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Execute script step
	ctx := context.Background()
	step := testWorkflow.Steps()[0]
	result, err := execution.executeScriptStep(ctx, step, "test_path")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "5", result.Content)
	require.Equal(t, int64(5), result.Object)
}

func TestExecutionStateManagement(t *testing.T) {
	// Create execution with state
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "state-test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{Name: "step1", Type: "prompt", Prompt: "Test"}),
		},
	})
	require.NoError(t, err)

	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  NewNullEventStore(),
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Test state operations
	err = execution.state.Set("key1", "value1")
	require.NoError(t, err)

	value, exists := execution.state.Get("key1")
	require.True(t, exists)
	require.Equal(t, "value1", value)

	// Test state copy
	stateCopy := execution.state.Copy()
	require.Equal(t, "value1", stateCopy["key1"])

	// Test state deletion
	err = execution.state.Delete("key1")
	require.NoError(t, err)

	_, exists = execution.state.Get("key1")
	require.False(t, exists)
}

func TestExecutionConditionEvaluation(t *testing.T) {
	// Create workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "condition-test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{Name: "step1", Type: "prompt", Prompt: "Test"}),
		},
	})
	require.NoError(t, err)

	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  NewNullEventStore(),
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("simple boolean conditions", func(t *testing.T) {
		result, err := execution.evaluateCondition(ctx, "true")
		require.NoError(t, err)
		require.True(t, result)

		result, err = execution.evaluateCondition(ctx, "false")
		require.NoError(t, err)
		require.False(t, result)
	})

	t.Run("expression conditions", func(t *testing.T) {
		// Set some state for testing
		execution.state.Set("count", 5)

		result, err := execution.evaluateCondition(ctx, "$(state.count > 3)")
		require.NoError(t, err)
		require.True(t, result)

		result, err = execution.evaluateCondition(ctx, "$(state.count < 3)")
		require.NoError(t, err)
		require.False(t, result)
	})
}

func TestExecutionReplayMode(t *testing.T) {
	// Create workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "replay-test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{Name: "step1", Type: "prompt", Prompt: "Test"}),
		},
	})
	require.NoError(t, err)

	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	// Create execution in replay mode
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  NewNullEventStore(),
		Logger:      slogger.NewDevNullLogger(),
		ReplayMode:  true,
	})
	require.NoError(t, err)

	// Pre-populate operation result
	opID := OperationID("test-operation")
	execution.operationResults[opID] = &OperationResult{
		OperationID: opID,
		Result:      "cached result",
		Error:       nil,
		ExecutedAt:  time.Now(),
	}

	// Test replay mode
	ctx := context.Background()
	op := Operation{ID: opID, PathID: "test_path"}

	result, err := execution.ExecuteOperation(ctx, op, func() (interface{}, error) {
		// This function should not be called in replay mode
		require.Fail(t, "function should not be called in replay mode")
		return nil, nil
	})

	require.NoError(t, err)
	require.Equal(t, "cached result", result)
}

func TestExecutionTemplateEvaluation(t *testing.T) {
	// Create workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "template-test",
		Inputs: []*workflow.Input{
			{Name: "name", Type: "string", Required: true},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{Name: "step1", Type: "prompt", Prompt: "Test"}),
		},
	})
	require.NoError(t, err)

	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{"name": "Alice"},
		EventStore:  NewNullEventStore(),
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Test template evaluation
	result, err := execution.evaluateTemplate(ctx, "Hello, ${inputs.name}!")
	require.NoError(t, err)
	require.Equal(t, "Hello, Alice!", result)

	// Test with state variables
	execution.state.Set("greeting", "Hi")
	result, err = execution.evaluateTemplate(ctx, "${state.greeting}, ${inputs.name}!")
	require.NoError(t, err)
	require.Equal(t, "Hi, Alice!", result)
}

func TestExecutionEachBlock(t *testing.T) {
	// Create workflow with each block using simple prompt without template
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "each-test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "process_items",
				Type:   "prompt",
				Prompt: "Process this item",
				Each: &workflow.EachBlock{
					Items: []string{"apple", "banana", "cherry"},
					As:    "item",
				},
				Store: "results",
			}),
		},
	})
	require.NoError(t, err)

	// Create mock agent for each block processing
	mockResponse := &dive.Response{
		Items: []*dive.ResponseItem{
			{
				Type:    dive.ResponseItemTypeMessage,
				Message: llm.NewAssistantMessage(&llm.TextContent{Text: "Processed item"}),
			},
		},
	}
	mockAgent := agent.NewMockAgent(agent.MockAgentOptions{
		Name:     "TestAgent",
		Response: mockResponse,
	})

	env := &Environment{
		agents:    map[string]dive.Agent{"TestAgent": mockAgent},
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  NewNullEventStore(),
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Execute step with each block
	ctx := context.Background()
	step := testWorkflow.Steps()[0]
	result, err := execution.executeStepEach(ctx, step, "test_path")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, result.Content, "item: apple")
	require.Contains(t, result.Content, "item: banana")
	require.Contains(t, result.Content, "item: cherry")

	// Check stored results
	storedResults, exists := execution.state.Get("results")
	require.True(t, exists)
	require.IsType(t, []string{}, storedResults)
	resultsList := storedResults.([]string)
	require.Len(t, resultsList, 3)
}

// MockEventStore captures events for testing verification
type MockEventStore struct {
	events []*ExecutionEvent
	mu     sync.Mutex
}

func NewMockEventStore() *MockEventStore {
	return &MockEventStore{
		events: make([]*ExecutionEvent, 0),
	}
}

func (m *MockEventStore) AppendEvents(ctx context.Context, events []*ExecutionEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, events...)
	return nil
}

func (m *MockEventStore) GetEvents(ctx context.Context, executionID string, fromSeq int64) ([]*ExecutionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var filtered []*ExecutionEvent
	for _, event := range m.events {
		if event.ExecutionID == executionID && event.Sequence >= fromSeq {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

func (m *MockEventStore) GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error) {
	return m.GetEvents(ctx, executionID, 0)
}

func (m *MockEventStore) SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error {
	return nil
}

func (m *MockEventStore) GetSnapshot(ctx context.Context, executionID string) (*ExecutionSnapshot, error) {
	return nil, fmt.Errorf("snapshot not found")
}

func (m *MockEventStore) ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*ExecutionSnapshot, error) {
	return nil, nil
}

func (m *MockEventStore) DeleteExecution(ctx context.Context, executionID string) error {
	return nil
}

func (m *MockEventStore) CleanupCompletedExecutions(ctx context.Context, olderThan time.Time) error {
	return nil
}

func (m *MockEventStore) GetAllEvents() []*ExecutionEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy to avoid race conditions
	events := make([]*ExecutionEvent, len(m.events))
	copy(events, m.events)
	return events
}

func TestExecutionEventSequence(t *testing.T) {
	// Create mock agent
	mockResponse := &dive.Response{
		Items: []*dive.ResponseItem{
			{
				Type:    dive.ResponseItemTypeMessage,
				Message: llm.NewAssistantMessage(&llm.TextContent{Text: "Mock response"}),
			},
		},
		Usage: &llm.Usage{InputTokens: 10, OutputTokens: 5},
	}

	mockAgent := agent.NewMockAgent(agent.MockAgentOptions{
		Name:     "TestAgent",
		Response: mockResponse,
	})

	// Create simple workflow with one prompt step
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "event-test-workflow",
		Inputs: []*workflow.Input{
			{Name: "message", Type: "string", Required: true},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "greet",
				Type:   "prompt",
				Prompt: "Say hello to ${inputs.message}",
				Agent:  mockAgent,
			}),
		},
	})
	require.NoError(t, err)

	// Create environment with agent
	env := &Environment{
		agents:    map[string]dive.Agent{"TestAgent": mockAgent},
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	// Create mock event store to capture events
	mockEventStore := NewMockEventStore()

	// Create execution
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{"message": "world"},
		EventStore:  mockEventStore,
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Run the execution
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)

	// Verify execution completed successfully
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Get all captured events
	events := mockEventStore.GetAllEvents()
	require.NotEmpty(t, events, "should have captured events")

	// Verify event sequence - we expect at least these core events:
	// 1. ExecutionStarted
	// 2. PathStarted
	// 3. StepStarted
	// 4. OperationStarted (agent response)
	// 5. OperationCompleted (agent response)
	// 6. StepCompleted
	// 7. PathCompleted
	// 8. ExecutionCompleted

	var eventTypes []string
	for _, event := range events {
		eventTypes = append(eventTypes, string(event.EventType))
	}

	// Check for required events in sequence
	require.Contains(t, eventTypes, "execution_started", "should contain ExecutionStarted event")
	require.Contains(t, eventTypes, "path_started", "should contain PathStarted event")
	require.Contains(t, eventTypes, "step_started", "should contain StepStarted event")
	require.Contains(t, eventTypes, "operation_started", "should contain OperationStarted event")
	require.Contains(t, eventTypes, "operation_completed", "should contain OperationCompleted event")
	require.Contains(t, eventTypes, "step_completed", "should contain StepCompleted event")
	require.Contains(t, eventTypes, "path_completed", "should contain PathCompleted event")
	require.Contains(t, eventTypes, "execution_completed", "should contain ExecutionCompleted event")

	// Verify ExecutionStarted event has correct data
	var executionStartedEvent *ExecutionEvent
	for _, event := range events {
		if event.EventType == "execution_started" {
			executionStartedEvent = event
			break
		}
	}
	require.NotNil(t, executionStartedEvent, "should have ExecutionStarted event")

	executionStartedData, ok := executionStartedEvent.Data.(*ExecutionStartedData)
	require.True(t, ok, "should have ExecutionStartedData")
	require.Equal(t, "event-test-workflow", executionStartedData.WorkflowName)
	require.NotNil(t, executionStartedData.Inputs)

	// Verify StepStarted event has correct data
	var stepStartedEvent *ExecutionEvent
	for _, event := range events {
		if event.EventType == "step_started" {
			stepStartedEvent = event
			break
		}
	}
	require.NotNil(t, stepStartedEvent, "should have StepStarted event")
	require.Equal(t, "greet", stepStartedEvent.Step)

	stepStartedData, ok := stepStartedEvent.Data.(*StepStartedData)
	require.True(t, ok, "should have StepStartedData")
	require.Equal(t, "prompt", stepStartedData.StepType)

	// Verify StepCompleted event has output
	var stepCompletedEvent *ExecutionEvent
	for _, event := range events {
		if event.EventType == "step_completed" {
			stepCompletedEvent = event
			break
		}
	}
	require.NotNil(t, stepCompletedEvent, "should have StepCompleted event")
	require.Equal(t, "greet", stepCompletedEvent.Step)

	stepCompletedData, ok := stepCompletedEvent.Data.(*StepCompletedData)
	require.True(t, ok, "should have StepCompletedData")
	require.NotEmpty(t, stepCompletedData.Output)
}

func TestExecutionEventSequenceWithFailure(t *testing.T) {
	// Create simple workflow with a script that will fail
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "failing-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "fail_step",
				Type:   "script",
				Script: "this_function_does_not_exist()", // This will cause a script error
			}),
		},
	})
	require.NoError(t, err)

	// Create environment
	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	// Create mock event store
	mockEventStore := NewMockEventStore()

	// Create execution
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  mockEventStore,
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Run the execution (should fail)
	ctx := context.Background()
	err = execution.Run(ctx)
	require.Error(t, err, "execution should fail")

	// Verify execution failed
	require.Equal(t, ExecutionStatusFailed, execution.Status())

	// Get all captured events
	events := mockEventStore.GetAllEvents()
	require.NotEmpty(t, events, "should have captured events even on failure")

	var eventTypes []string
	for _, event := range events {
		eventTypes = append(eventTypes, string(event.EventType))
	}

	// Check for failure-related events
	require.Contains(t, eventTypes, "execution_started", "should contain ExecutionStarted event")
	require.Contains(t, eventTypes, "step_started", "should contain StepStarted event")
	require.Contains(t, eventTypes, "operation_started", "should contain OperationStarted event")
	require.Contains(t, eventTypes, "operation_failed", "should contain OperationFailed event")
	require.Contains(t, eventTypes, "step_failed", "should contain StepFailed event")
	require.Contains(t, eventTypes, "path_failed", "should contain PathFailed event")
	require.Contains(t, eventTypes, "execution_failed", "should contain ExecutionFailed event")

	// Verify OperationFailed event has error details
	var operationFailedEvent *ExecutionEvent
	for _, event := range events {
		if event.EventType == "operation_failed" {
			operationFailedEvent = event
			break
		}
	}
	require.NotNil(t, operationFailedEvent, "should have OperationFailed event")

	operationFailedData, ok := operationFailedEvent.Data.(*OperationFailedData)
	require.True(t, ok, "should have OperationFailedData")
	require.NotEmpty(t, operationFailedData.Error)

	// Verify ExecutionFailed event has error details
	var executionFailedEvent *ExecutionEvent
	for _, event := range events {
		if event.EventType == "execution_failed" {
			executionFailedEvent = event
			break
		}
	}
	require.NotNil(t, executionFailedEvent, "should have ExecutionFailed event")

	executionFailedData, ok := executionFailedEvent.Data.(*ExecutionFailedData)
	require.True(t, ok, "should have ExecutionFailedData")
	require.NotEmpty(t, executionFailedData.Error)
}

func TestExecutionExactEventSequence(t *testing.T) {
	// Create simple workflow with a single script step
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "simple-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "calculate",
				Type:   "script",
				Script: "1 + 1",
			}),
		},
	})
	require.NoError(t, err)

	// Create environment
	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(context.Background()))
	defer env.Stop(context.Background())

	// Create mock event store
	mockEventStore := NewMockEventStore()

	// Create execution
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  mockEventStore,
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Run the execution
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)

	// Verify execution completed successfully
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Get all captured events in order
	events := mockEventStore.GetAllEvents()
	require.NotEmpty(t, events, "should have captured events")

	data, err := json.MarshalIndent(events, "", "  ")
	require.NoError(t, err)
	fmt.Println(string(data))

	require.Len(t, events, 8)
}
