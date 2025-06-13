package environment

import (
	"context"
	"os"
	"path"
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
	eventStore, err := workflow.NewSQLiteExecutionEventStore(path.Join(tempDir, "executions.db"),
		workflow.DefaultSQLiteStoreOptions())
	require.NoError(t, err)

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
		execution, err := NewEventBasedExecution(env, ExecutionOptions{
			WorkflowName:   "test-workflow",
			Inputs:         map[string]interface{}{"input1": "test value"},
			Logger:         env.logger,
			EventStore:     eventStore,
			EventBatchSize: 5,
		})
		require.NoError(t, err)

		ctx := context.Background()

		// Run execution
		err = execution.Run(ctx)
		require.NoError(t, err)

		err = execution.Flush()
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

	t.Run("Phase2Integration", func(t *testing.T) {
		// Test enhanced replay, retry from failure, and change detection
		t.Run("RetryFromFailure", func(t *testing.T) {
			orchestrator := NewExecutionOrchestrator(eventStore, env)

			// Create an execution that will fail
			failingExecution, err := orchestrator.CreateExecution(context.Background(), ExecutionOptions{
				WorkflowName: "test-workflow",
				Inputs:       map[string]interface{}{"input1": "fail please"},
				Logger:       env.logger,
			})
			require.NoError(t, err)

			// Simulate execution failure by manually creating failure events
			failingExecution.recordEvent(workflow.EventExecutionStarted, "", "", map[string]interface{}{
				"inputs":        map[string]interface{}{"input1": "fail please"},
				"workflow_hash": "test-hash-123",
			})

			pathID := "path-1"
			failingExecution.recordEvent(workflow.EventPathStarted, pathID, "", map[string]interface{}{
				"current_step": "step1",
			})

			failingExecution.recordEvent(workflow.EventStepStarted, pathID, "step1", map[string]interface{}{
				"step_type": "agent_step",
			})

			failingExecution.recordEvent(workflow.EventStepFailed, pathID, "step1", map[string]interface{}{
				"error": "simulated failure",
			})

			failingExecution.recordEvent(workflow.EventExecutionFailed, pathID, "", map[string]interface{}{
				"error": "execution failed due to step failure",
			})

			// Flush events and save snapshot
			err = failingExecution.flushEvents()
			require.NoError(t, err)

			snapshot := &workflow.ExecutionSnapshot{
				ID:           failingExecution.ID(),
				WorkflowName: "test-workflow",
				Status:       "failed",
				Inputs:       map[string]interface{}{"input1": "fail please"},
				Error:        "execution failed due to step failure",
			}
			err = eventStore.SaveSnapshot(context.Background(), snapshot)
			require.NoError(t, err)

			// Now test retry from failure
			retryExecution, err := orchestrator.RetryExecution(context.Background(), failingExecution.ID(), workflow.RetryOptions{
				Strategy: workflow.RetryFromFailure,
			})
			require.NoError(t, err)
			require.NotNil(t, retryExecution)
			require.NotEqual(t, failingExecution.ID(), retryExecution.ID(), "Retry should create new execution")

			// Verify the retry execution has proper state
			require.Contains(t, retryExecution.scriptGlobals, "__resume_from_failure", "Should have resume information")
			resumeInfo := retryExecution.scriptGlobals["__resume_from_failure"].(map[string]interface{})
			require.Equal(t, "step1", resumeInfo["failed_step"])
			require.Equal(t, pathID, resumeInfo["failed_path"])
		})

		t.Run("RetryWithNewInputs", func(t *testing.T) {
			orchestrator := NewExecutionOrchestrator(eventStore, env)

			// Get an existing execution
			executions, err := eventStore.ListExecutions(context.Background(), workflow.ExecutionFilter{Limit: 1})
			require.NoError(t, err)
			require.NotEmpty(t, executions, "Should have at least one execution")

			originalExecution := executions[0]
			newInputs := map[string]interface{}{
				"input1": "new value",
				"input2": "additional input",
			}

			// Test retry with new inputs
			retryExecution, err := orchestrator.RetryExecution(context.Background(), originalExecution.ID, workflow.RetryOptions{
				Strategy:  workflow.RetryWithNewInputs,
				NewInputs: newInputs,
			})
			require.NoError(t, err)
			require.NotNil(t, retryExecution)

			// Verify new inputs are applied
			require.Equal(t, newInputs, retryExecution.inputs)
			require.Contains(t, retryExecution.scriptGlobals, "inputs")
			require.Equal(t, newInputs, retryExecution.scriptGlobals["inputs"])
		})

		t.Run("WorkflowHashingAndChangeDetection", func(t *testing.T) {
			hasher := workflow.NewBasicWorkflowHasher()

			// Test workflow hashing
			hash1, err := hasher.HashWorkflow(testWorkflow)
			require.NoError(t, err)
			require.NotEmpty(t, hash1)

			// Same workflow should produce same hash
			hash2, err := hasher.HashWorkflow(testWorkflow)
			require.NoError(t, err)
			require.Equal(t, hash1, hash2, "Same workflow should produce same hash")

			// Test input hashing
			inputs1 := map[string]interface{}{"key1": "value1", "key2": 42}
			inputHash1, err := hasher.HashInputs(inputs1)
			require.NoError(t, err)
			require.NotEmpty(t, inputHash1)

			inputs2 := map[string]interface{}{"key1": "value1", "key2": 42}
			inputHash2, err := hasher.HashInputs(inputs2)
			require.NoError(t, err)
			require.Equal(t, inputHash1, inputHash2, "Same inputs should produce same hash")

			inputs3 := map[string]interface{}{"key1": "different", "key2": 42}
			inputHash3, err := hasher.HashInputs(inputs3)
			require.NoError(t, err)
			require.NotEqual(t, inputHash1, inputHash3, "Different inputs should produce different hash")
		})

		t.Run("ChangeDetector", func(t *testing.T) {
			hasher := workflow.NewBasicWorkflowHasher()
			detector := workflow.NewBasicChangeDetector(hasher)

			// Test input change detection
			oldInputs := map[string]interface{}{"key1": "value1", "key2": 42}
			newInputs := map[string]interface{}{"key1": "new_value", "key3": "added"}

			changes, err := detector.DetectInputChanges(oldInputs, newInputs)
			require.NoError(t, err)
			require.Contains(t, changes, "key2 (removed)")
			require.Contains(t, changes, "key3 (added)")
			require.Contains(t, changes, "key1 (value changed)")

			// Test affected steps detection
			affectedSteps, err := detector.FindAffectedSteps(changes, testWorkflow)
			require.NoError(t, err)
			require.NotEmpty(t, affectedSteps, "Should identify affected steps")
		})

		t.Run("EnhancedReplayWithPathBranching", func(t *testing.T) {
			// Create execution with path branching scenario
			execution, err := NewEventBasedExecution(env, ExecutionOptions{
				WorkflowName:   "test-workflow",
				Inputs:         map[string]interface{}{"input1": "branch test"},
				Logger:         env.logger,
				EventStore:     eventStore,
				EventBatchSize: 5,
			})
			require.NoError(t, err)

			// Record execution with path branching
			execution.recordEvent(workflow.EventExecutionStarted, "", "", map[string]interface{}{
				"inputs": execution.inputs,
			})

			// Main path
			mainPathID := "main-path"
			execution.recordEvent(workflow.EventPathStarted, mainPathID, "", map[string]interface{}{
				"current_step": "initial_step",
			})

			execution.recordEvent(workflow.EventStepStarted, mainPathID, "initial_step", map[string]interface{}{
				"step_type": "agent_step",
			})

			execution.recordEvent(workflow.EventStepCompleted, mainPathID, "initial_step", map[string]interface{}{
				"output":           "initial output",
				"stored_variable":  "initial_result",
				"stored_value":     "initial output",
				"condition_result": "true",
			})

			// Path branching
			branchPath1 := "branch-path-1"
			branchPath2 := "branch-path-2"
			execution.recordEvent(workflow.EventPathBranched, mainPathID, "initial_step", map[string]interface{}{
				"new_paths": []map[string]interface{}{
					{
						"id":              branchPath1,
						"current_step":    "branch_step_1",
						"inherit_outputs": true,
					},
					{
						"id":           branchPath2,
						"current_step": "branch_step_2",
					},
				},
			})

			// Complete branch paths
			execution.recordEvent(workflow.EventStepStarted, branchPath1, "branch_step_1", map[string]interface{}{})
			execution.recordEvent(workflow.EventStepCompleted, branchPath1, "branch_step_1", map[string]interface{}{
				"output": "branch 1 output",
			})
			execution.recordEvent(workflow.EventPathCompleted, branchPath1, "", map[string]interface{}{
				"final_output": "branch 1 final",
			})

			execution.recordEvent(workflow.EventStepStarted, branchPath2, "branch_step_2", map[string]interface{}{})
			execution.recordEvent(workflow.EventStepCompleted, branchPath2, "branch_step_2", map[string]interface{}{
				"output": "branch 2 output",
			})
			execution.recordEvent(workflow.EventPathCompleted, branchPath2, "", map[string]interface{}{
				"final_output": "branch 2 final",
			})

			execution.recordEvent(workflow.EventExecutionCompleted, "", "", map[string]interface{}{
				"outputs": map[string]interface{}{"final": "completed"},
			})

			// Flush events
			err = execution.flushEvents()
			require.NoError(t, err)

			// Test enhanced replay
			events, err := eventStore.GetEventHistory(context.Background(), execution.ID())
			require.NoError(t, err)

			replayer := workflow.NewBasicExecutionReplayer(env.logger)
			replayResult, err := replayer.ReplayExecution(context.Background(), events, testWorkflow)
			require.NoError(t, err)

			// Verify enhanced replay results
			require.Equal(t, "completed", replayResult.Status)
			require.NotEmpty(t, replayResult.CompletedSteps)
			require.Contains(t, replayResult.CompletedSteps, "initial_step")
			require.Contains(t, replayResult.CompletedSteps, "branch_step_1")
			require.Contains(t, replayResult.CompletedSteps, "branch_step_2")

			// Verify script globals reconstruction
			require.Contains(t, replayResult.ScriptGlobals, "initial_result")
			require.Equal(t, "initial output", replayResult.ScriptGlobals["initial_result"])
			require.Contains(t, replayResult.ScriptGlobals, "initial_step_condition")
			require.Equal(t, true, replayResult.ScriptGlobals["initial_step_condition"])
			require.Contains(t, replayResult.ScriptGlobals, "outputs")

			// Verify path state tracking
			require.Empty(t, replayResult.ActivePaths, "All paths should be completed")
		})
	})
}

func TestEventRecordingEdgeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dive-event-edge-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	eventStore, err := workflow.NewSQLiteExecutionEventStore(path.Join(tempDir, "executions.db"),
		workflow.DefaultSQLiteStoreOptions())
	require.NoError(t, err)

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

		execution, err := NewEventBasedExecution(env, ExecutionOptions{
			WorkflowName:   "replay-test-workflow",
			Logger:         env.logger,
			EventStore:     eventStore,
			EventBatchSize: 1,
		})
		require.NoError(t, err)

		// Set replay mode
		execution.replayMode = true

		// Record an event - should be skipped
		execution.recordEvent(workflow.EventStepStarted, "path-1", "step1", map[string]interface{}{})

		// Force flush
		err = execution.Flush()
		require.NoError(t, err)

		// Check that no events were recorded
		events, err := eventStore.GetEventHistory(context.Background(), execution.ID())
		require.NoError(t, err)
		require.Empty(t, events, "No events should be recorded in replay mode")
	})

	t.Run("EventBufferFlushing", func(t *testing.T) {
		execution, err := NewEventBasedExecution(env, ExecutionOptions{
			WorkflowName:   "replay-test-workflow",
			Logger:         env.logger,
			EventStore:     eventStore,
			EventBatchSize: 2, // Small batch size to test flushing
		})
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
