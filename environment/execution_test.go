package environment

import (
	"context"
	"testing"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

// TestNewExecution tests the creation of new execution instances
func TestNewExecution(t *testing.T) {
	// Create a simple workflow with inputs
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "test-workflow",
		Inputs: []*workflow.Input{
			{Name: "required_input", Type: "string", Required: true},
			{Name: "optional_input", Type: "string", Default: "default_value"},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "test_step",
				Type:   "script",
				Script: `"Test result"`,
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

	t.Run("valid execution creation", func(t *testing.T) {
		execution, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs: map[string]interface{}{
				"required_input": "test_value",
			},
			Logger: slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)
		require.NotNil(t, execution)
		require.Equal(t, ExecutionStatusPending, execution.Status())
		require.NotEmpty(t, execution.ID())

		// Check that inputs were processed correctly
		require.Equal(t, "test_value", execution.inputs["required_input"])
		require.Equal(t, "default_value", execution.inputs["optional_input"])
	})

	t.Run("missing required input", func(t *testing.T) {
		_, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs:      map[string]interface{}{}, // Missing required input
			Logger:      slogger.NewDevNullLogger(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "input \"required_input\" is required")
	})

	t.Run("unknown input rejected", func(t *testing.T) {
		_, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs: map[string]interface{}{
				"required_input": "test_value",
				"unknown_input":  "some_value",
			},
			Logger: slogger.NewDevNullLogger(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown input \"unknown_input\"")
	})

	t.Run("missing workflow fails", func(t *testing.T) {
		_, err := NewExecution(ExecutionOptions{
			Environment: env,
			Inputs:      map[string]interface{}{},
			Logger:      slogger.NewDevNullLogger(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "workflow is required")
	})
}

// TestExecutionRun tests basic execution functionality
func TestExecutionRun(t *testing.T) {
	// Create a simple script workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "simple-execution",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "script_step",
				Type:   "script",
				Script: `"Execution completed successfully"`,
				Store:  "result",
			}),
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

	t.Run("successful execution", func(t *testing.T) {
		execution, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs:      map[string]interface{}{},
			Logger:      slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		// Execute the workflow
		ctx := context.Background()
		err = execution.Run(ctx)
		require.NoError(t, err)

		// Verify execution completed
		require.Equal(t, ExecutionStatusCompleted, execution.Status())
		require.False(t, execution.startTime.IsZero())
		require.False(t, execution.endTime.IsZero())
		require.True(t, execution.endTime.After(execution.startTime))

		// Verify result was stored
		result, exists := execution.state.Get("result")
		require.True(t, exists)
		require.Equal(t, "Execution completed successfully", result)

		// Verify path state
		require.Len(t, execution.paths, 1)
		mainPath := execution.paths["main"]
		require.Equal(t, PathStatusCompleted, mainPath.Status)
		require.False(t, mainPath.EndTime.IsZero())
	})
}

// TestExecutionWithMultipleSteps tests execution with multiple connected steps
func TestExecutionWithMultipleSteps(t *testing.T) {
	// Create a workflow with multiple steps
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "multi-step-execution",
		Inputs: []*workflow.Input{
			{Name: "input_value", Type: "string", Required: true},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "step1",
				Type:   "script",
				Script: `"Processed: " + inputs.input_value`,
				Store:  "step1_result",
				Next:   []*workflow.Edge{{Step: "step2"}},
			}),
			workflow.NewStep(workflow.StepOptions{
				Name:   "step2",
				Type:   "script",
				Script: `"Final: " + state.step1_result`,
				Store:  "final_result",
			}),
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
		Inputs: map[string]interface{}{
			"input_value": "test data",
		},
		Logger: slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Execute the workflow
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Verify both steps executed and stored results
	step1Result, exists := execution.state.Get("step1_result")
	require.True(t, exists)
	require.Equal(t, "Processed: test data", step1Result)

	finalResult, exists := execution.state.Get("final_result")
	require.True(t, exists)
	require.Equal(t, "Final: Processed: test data", finalResult)
}

// MockCheckpointer is a simple in-memory checkpointer for testing
type MockCheckpointer struct {
	checkpoints map[string]*ExecutionCheckpoint
}

func NewMockCheckpointer() *MockCheckpointer {
	return &MockCheckpointer{
		checkpoints: make(map[string]*ExecutionCheckpoint),
	}
}

func (m *MockCheckpointer) SaveCheckpoint(ctx context.Context, checkpoint *ExecutionCheckpoint) error {
	m.checkpoints[checkpoint.ExecutionID] = checkpoint
	return nil
}

func (m *MockCheckpointer) LoadCheckpoint(ctx context.Context, executionID string) (*ExecutionCheckpoint, error) {
	checkpoint, exists := m.checkpoints[executionID]
	if !exists {
		return nil, nil
	}
	return checkpoint, nil
}

func (m *MockCheckpointer) DeleteCheckpoint(ctx context.Context, executionID string) error {
	delete(m.checkpoints, executionID)
	return nil
}

// TestExecutionCheckpointing tests the checkpointing and restoration functionality
func TestExecutionCheckpointing(t *testing.T) {
	// Create a multi-step workflow for testing
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "checkpoint-test-workflow",
		Inputs: []*workflow.Input{
			{Name: "start_value", Type: "string", Default: "initial"},
		},
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "step1",
				Type:   "script",
				Script: `inputs.start_value + "_step1"`,
				Store:  "step1_result",
				Next: []*workflow.Edge{
					{Step: "step2"},
				},
			}),
			workflow.NewStep(workflow.StepOptions{
				Name:   "step2",
				Type:   "script",
				Script: `state.step1_result + "_step2"`,
				Store:  "step2_result",
				Next: []*workflow.Edge{
					{Step: "step3"},
				},
			}),
			workflow.NewStep(workflow.StepOptions{
				Name:   "step3",
				Type:   "script",
				Script: `state.step2_result + "_step3"`,
				Store:  "final_result",
			}),
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

	// Test checkpointing and restoration
	t.Run("checkpoint and restore execution", func(t *testing.T) {
		ctx := context.Background()
		checkpointer := NewMockCheckpointer()
		executionID := "test-execution-" + NewExecutionID()

		// Create first execution
		execution1, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs: map[string]interface{}{
				"start_value": "test",
			},
			ExecutionID:  executionID,
			Checkpointer: checkpointer,
			Logger:       slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		// Execute partially (we'll let it run and verify checkpointing happened)
		err = execution1.Run(ctx)
		require.NoError(t, err)

		// Verify checkpoint was saved
		checkpoint, err := checkpointer.LoadCheckpoint(ctx, executionID)
		require.NoError(t, err)
		require.NotNil(t, checkpoint)
		require.Equal(t, executionID, checkpoint.ExecutionID)
		require.Equal(t, "checkpoint-test-workflow", checkpoint.WorkflowName)
		require.Equal(t, "completed", checkpoint.Status)

		// Verify state was checkpointed
		require.NotEmpty(t, checkpoint.State)
		require.Contains(t, checkpoint.State, "step1_result")
		require.Contains(t, checkpoint.State, "step2_result")
		require.Contains(t, checkpoint.State, "final_result")

		// Verify path states were checkpointed
		require.NotEmpty(t, checkpoint.PathStates)
		require.Contains(t, checkpoint.PathStates, "main")

		mainPath := checkpoint.PathStates["main"]
		require.Equal(t, "main", mainPath.ID)
		require.Equal(t, PathStatusCompleted, mainPath.Status)
		require.Equal(t, "step3", mainPath.CurrentStep) // Should be the last step executed

		// Create second execution from checkpoint
		execution2, err := NewExecution(ExecutionOptions{
			Workflow:    testWorkflow,
			Environment: env,
			Inputs: map[string]interface{}{
				"start_value": "test",
			},
			ExecutionID:  executionID,
			Checkpointer: checkpointer,
			Logger:       slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		// Load from checkpoint
		err = execution2.LoadFromCheckpoint(ctx)
		require.NoError(t, err)

		// Verify state was restored
		require.Equal(t, ExecutionStatusCompleted, execution2.Status())

		value, exists := execution2.state.Get("final_result")
		require.True(t, exists)
		require.Equal(t, "test_step1_step2_step3", value)

		// Verify path states were restored
		require.Len(t, execution2.paths, 1)
		restoredPath := execution2.paths["main"]
		require.Equal(t, "main", restoredPath.ID)
		require.Equal(t, PathStatusCompleted, restoredPath.Status)
		require.Equal(t, "step3", restoredPath.CurrentStep)

		// Verify pathCounter was restored
		require.Equal(t, execution1.pathCounter, execution2.pathCounter)
	})

	t.Run("checkpoint with multiple paths", func(t *testing.T) {
		// Create a workflow with branching paths
		branchWorkflow, err := workflow.New(workflow.Options{
			Name: "branch-workflow",
			Steps: []*workflow.Step{
				workflow.NewStep(workflow.StepOptions{
					Name:   "start",
					Type:   "script",
					Script: `"started"`,
					Store:  "start_result",
					Next: []*workflow.Edge{
						{Step: "branch1", Condition: "true"},
						{Step: "branch2", Condition: "true"},
					},
				}),
				workflow.NewStep(workflow.StepOptions{
					Name:   "branch1",
					Type:   "script",
					Script: `"branch1_done"`,
					Store:  "branch1_result",
				}),
				workflow.NewStep(workflow.StepOptions{
					Name:   "branch2",
					Type:   "script",
					Script: `"branch2_done"`,
					Store:  "branch2_result",
				}),
			},
		})
		require.NoError(t, err)

		ctx := context.Background()
		checkpointer := NewMockCheckpointer()
		executionID := "branch-execution-" + NewExecutionID()

		execution, err := NewExecution(ExecutionOptions{
			Workflow:     branchWorkflow,
			Environment:  env,
			Inputs:       map[string]interface{}{},
			ExecutionID:  executionID,
			Checkpointer: checkpointer,
			Logger:       slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		// Execute the workflow
		err = execution.Run(ctx)
		require.NoError(t, err)

		// Verify checkpoint has multiple paths
		checkpoint, err := checkpointer.LoadCheckpoint(ctx, executionID)
		require.NoError(t, err)
		require.NotNil(t, checkpoint)

		// Should have multiple paths due to branching
		require.True(t, len(checkpoint.PathStates) >= 2)

		// Verify pathCounter was incremented for branching
		require.True(t, checkpoint.PathCounter > 0)

		// Test restoration with multiple paths
		execution2, err := NewExecution(ExecutionOptions{
			Workflow:     branchWorkflow,
			Environment:  env,
			Inputs:       map[string]interface{}{},
			ExecutionID:  executionID,
			Checkpointer: checkpointer,
			Logger:       slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		err = execution2.LoadFromCheckpoint(ctx)
		require.NoError(t, err)

		// Verify all paths were restored
		require.Equal(t, len(checkpoint.PathStates), len(execution2.paths))
		require.Equal(t, checkpoint.PathCounter, execution2.pathCounter)

		// Verify execution status
		require.Equal(t, ExecutionStatusCompleted, execution2.Status())
	})
}

// TestExecutionStatusTransitions tests execution status changes
func TestExecutionStatusTransitions(t *testing.T) {
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "status-test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "status_step",
				Type:   "script",
				Script: `"Status test"`,
			}),
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
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Initially pending
	require.Equal(t, ExecutionStatusPending, execution.Status())

	// Run and verify status progression
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)

	// Finally completed
	require.Equal(t, ExecutionStatusCompleted, execution.Status())
}
