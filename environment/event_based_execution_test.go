package environment

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

// MockAgent for testing
type MockAgent struct {
	name      string
	responses []string
	callCount int
}

func (m *MockAgent) Name() string {
	return m.name
}

func (m *MockAgent) IsSupervisor() bool {
	return false
}

func (m *MockAgent) SetEnvironment(env dive.Environment) error {
	return nil
}

func (m *MockAgent) CreateResponse(ctx context.Context, options ...dive.Option) (*dive.Response, error) {
	defer func() { m.callCount++ }()

	var content string
	if m.callCount >= len(m.responses) {
		content = "Default response"
	} else {
		content = m.responses[m.callCount]
	}

	return &dive.Response{
		Items: []*dive.ResponseItem{{
			Type:    dive.ResponseItemTypeMessage,
			Message: llm.NewAssistantTextMessage(content),
		}},
	}, nil
}

func (m *MockAgent) StreamResponse(ctx context.Context, options ...dive.Option) (dive.ResponseStream, error) {
	response, err := m.CreateResponse(ctx, options...)
	if err != nil {
		return nil, err
	}

	stream, publisher := dive.NewEventStream()
	defer publisher.Close()

	publisher.Send(ctx, &dive.ResponseEvent{
		Type:     dive.EventTypeResponseCompleted,
		Response: response,
	})

	return stream, nil
}

func TestEventBasedExecution(t *testing.T) {
	// Create temporary directory for event store
	tempDir, err := os.MkdirTemp("", "dive-event-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create event store
	eventStore := workflow.NewFileExecutionEventStore(tempDir)

	// Create test environment
	env := &Environment{
		agents:    map[string]dive.Agent{"test-agent": &MockAgent{name: "test-agent", responses: []string{"Step 1 output", "Step 2 output"}}},
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.New(slogger.LevelInfo),
	}

	// Create a simple test workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name:        "test-workflow",
		Description: "Test workflow for event recording",
		Inputs: []*workflow.Input{
			{Name: "input1", Type: "string", Required: true},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "step1",
				Type:   "prompt",
				Prompt: "Process input: ${inputs.input1}",
			}),
			workflow.NewStep(workflow.StepOptions{
				Name:   "step2",
				Type:   "prompt",
				Prompt: "Continue processing: ${step1}",
			}),
		},
	})
	require.NoError(t, err)

	env.workflows["test-workflow"] = testWorkflow

	t.Run("EventRecordingDuringExecution", func(t *testing.T) {
		// Create event-based execution
		config := &PersistenceConfig{
			EventStore: eventStore,
			BatchSize:  5,
		}

		execution, err := NewEventBasedExecution(env, ExecutionOptions{
			WorkflowName: "test-workflow",
			Inputs:       map[string]interface{}{"input1": "test value"},
			Logger:       env.logger,
		}, config)
		require.NoError(t, err)

		ctx := context.Background()

		// Run execution
		err = execution.Run(ctx)
		require.NoError(t, err)

		// Force flush events
		err = execution.ForceFlush()
		require.NoError(t, err)

		// Verify events were recorded
		events, err := eventStore.GetEventHistory(ctx, execution.ID())
		require.NoError(t, err)
		require.NotEmpty(t, events, "Events should have been recorded")

		// Check for key event types
		eventTypes := make(map[workflow.ExecutionEventType]bool)
		for _, event := range events {
			eventTypes[event.EventType] = true
		}

		require.True(t, eventTypes[workflow.EventExecutionStarted], "Should have execution started event")
		require.True(t, eventTypes[workflow.EventPathStarted], "Should have path started event")
		require.True(t, eventTypes[workflow.EventStepStarted], "Should have step started events")
		require.True(t, eventTypes[workflow.EventStepCompleted], "Should have step completed events")
		require.True(t, eventTypes[workflow.EventExecutionCompleted], "Should have execution completed event")

		// Verify snapshot was saved
		snapshot, err := eventStore.GetSnapshot(ctx, execution.ID())
		require.NoError(t, err)
		require.Equal(t, execution.ID(), snapshot.ID)
		require.Equal(t, "test-workflow", snapshot.WorkflowName)
		require.Equal(t, string(StatusCompleted), snapshot.Status)
	})

	t.Run("BasicReplay", func(t *testing.T) {
		// Get an existing execution's events
		executions, err := eventStore.ListExecutions(context.Background(), workflow.ExecutionFilter{Limit: 1})
		require.NoError(t, err)
		require.NotEmpty(t, executions, "Should have at least one execution from previous test")

		executionID := executions[0].ID
		events, err := eventStore.GetEventHistory(context.Background(), executionID)
		require.NoError(t, err)

		// Create replayer
		replayer := workflow.NewBasicExecutionReplayer(env.logger)

		// Replay execution
		replayResult, err := replayer.ReplayExecution(context.Background(), events, testWorkflow)
		require.NoError(t, err)
		require.NotNil(t, replayResult)

		// Verify replay results
		require.NotEmpty(t, replayResult.CompletedSteps, "Should have completed steps")
		require.Equal(t, "completed", replayResult.Status, "Should have completed status")
	})

	t.Run("ExecutionOrchestrator", func(t *testing.T) {
		orchestrator := NewExecutionOrchestrator(eventStore, env)

		// Create execution via orchestrator
		execution, err := orchestrator.CreateExecution(context.Background(), ExecutionOptions{
			WorkflowName: "test-workflow",
			Inputs:       map[string]interface{}{"input1": "orchestrator test"},
			Logger:       env.logger,
		})
		require.NoError(t, err)
		require.NotNil(t, execution)

		// Run execution
		err = execution.Run(context.Background())
		require.NoError(t, err)

		// Test retry from start
		retryExecution, err := orchestrator.RetryExecution(context.Background(), execution.ID(), workflow.RetryOptions{
			Strategy: workflow.RetryFromStart,
		})
		require.NoError(t, err)
		require.NotNil(t, retryExecution)
		require.NotEqual(t, execution.ID(), retryExecution.ID(), "Retry should create new execution")
	})

	t.Run("LoadFromSnapshot", func(t *testing.T) {
		// Get an existing execution's snapshot
		executions, err := eventStore.ListExecutions(context.Background(), workflow.ExecutionFilter{Limit: 1})
		require.NoError(t, err)
		require.NotEmpty(t, executions)

		snapshot := executions[0]

		// Load execution from snapshot
		loadedExecution, err := LoadFromSnapshot(context.Background(), env, snapshot, eventStore)
		require.NoError(t, err)
		require.NotNil(t, loadedExecution)
		require.Equal(t, snapshot.ID, loadedExecution.ID())
		require.Equal(t, snapshot.WorkflowName, loadedExecution.workflow.Name())
	})
}

func TestEventRecordingEdgeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dive-event-edge-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	eventStore := workflow.NewFileExecutionEventStore(tempDir)
	env := &Environment{
		agents:    map[string]dive.Agent{"test-agent": &MockAgent{name: "test-agent", responses: []string{"output"}}},
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.New(slogger.LevelInfo),
	}

	t.Run("ReplayModeSkipsEventRecording", func(t *testing.T) {
		// Create workflow
		testWorkflow, err := workflow.New(workflow.Options{
			Name: "replay-test-workflow",
			Steps: []*workflow.Step{
				workflow.NewStep(workflow.StepOptions{Name: "step1", Type: "prompt", Prompt: "Test prompt"}),
			},
		})
		require.NoError(t, err)

		env.workflows["replay-test-workflow"] = testWorkflow

		config := &PersistenceConfig{
			EventStore: eventStore,
			BatchSize:  1,
		}

		execution, err := NewEventBasedExecution(env, ExecutionOptions{
			WorkflowName: "replay-test-workflow",
			Logger:       env.logger,
		}, config)
		require.NoError(t, err)

		// Set replay mode
		execution.replayMode = true

		// Record an event - should be skipped
		execution.recordEvent(workflow.EventStepStarted, "path-1", "step1", map[string]interface{}{})

		// Force flush
		err = execution.ForceFlush()
		require.NoError(t, err)

		// Check that no events were recorded
		events, err := eventStore.GetEventHistory(context.Background(), execution.ID())
		require.NoError(t, err)
		require.Empty(t, events, "No events should be recorded in replay mode")
	})

	t.Run("EventBufferFlushing", func(t *testing.T) {
		config := &PersistenceConfig{
			EventStore: eventStore,
			BatchSize:  2, // Small batch size to test flushing
		}

		execution, err := NewEventBasedExecution(env, ExecutionOptions{
			WorkflowName: "replay-test-workflow",
			Logger:       env.logger,
		}, config)
		require.NoError(t, err)

		// Record events that should trigger flush
		execution.recordEvent(workflow.EventStepStarted, "path-1", "step1", map[string]interface{}{})
		execution.recordEvent(workflow.EventStepCompleted, "path-1", "step1", map[string]interface{}{"output": "test"})

		// Events should be automatically flushed due to batch size
		// Add a small delay to allow async flush
		time.Sleep(10 * time.Millisecond)

		events, err := eventStore.GetEventHistory(context.Background(), execution.ID())
		require.NoError(t, err)
		require.Len(t, events, 2, "Events should be automatically flushed")
	})
}
