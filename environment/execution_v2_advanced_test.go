package environment

import (
	"context"
	"testing"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

func TestExecution_EventReplay(t *testing.T) {
	// Create a simple mock agent
	mockAgent := &mockAgent{}

	// Create step - using simple prompt without template for replay test
	promptStep := workflow.NewStep(workflow.StepOptions{
		Name:   "greet",
		Type:   "prompt",
		Agent:  mockAgent,
		Prompt: "Say hello World",
	})

	// Create workflow
	wf, err := workflow.New(workflow.Options{
		Name:  "replay-test-workflow",
		Steps: []*workflow.Step{promptStep},
	})
	require.NoError(t, err)

	// Create environment
	env, err := New(Options{
		Name:   "replay-test-env",
		Agents: []dive.Agent{mockAgent},
	})
	require.NoError(t, err)

	// Start environment
	err = env.Start(context.Background())
	require.NoError(t, err)
	defer env.Stop(context.Background())

	// Create event store
	eventStore := &mockEventStore{}

	// First execution - record events
	execution1, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  eventStore,
		Logger:      slogger.DefaultLogger,
		ReplayMode:  false,
	})
	require.NoError(t, err)

	// Run first execution
	ctx := context.Background()
	err = execution1.Run(ctx)
	require.NoError(t, err)

	// Verify events were recorded
	events, err := eventStore.GetEventHistory(ctx, execution1.ID())
	require.NoError(t, err)
	require.True(t, len(events) > 0, "Should have recorded events")

	// Verify we have the expected events
	t.Logf("Recorded %d events", len(events))

	// Find operation events
	var hasAgentOperation bool
	for _, event := range events {
		if event.EventType == EventOperationCompleted {
			if opType, ok := event.Data["operation_type"].(string); ok && opType == "agent_response" {
				hasAgentOperation = true
				break
			}
		}
	}
	require.True(t, hasAgentOperation, "Should have recorded agent operation")

	// Create new execution for replay using same ID
	execution2, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  eventStore,
		Logger:      slogger.DefaultLogger,
		ReplayMode:  true,
	})
	require.NoError(t, err)

	// Manually set same ID for replay
	execution2.id = execution1.ID()

	// Load events for replay
	err = execution2.LoadFromEvents(ctx)
	require.NoError(t, err)

	// Verify operation results were loaded
	require.True(t, len(execution2.operationResults) > 0, "Should have loaded operation results")

	// Run the execution in replay mode - should use cached operation results
	err = execution2.Run(ctx)
	require.NoError(t, err)

	// Verify execution completed successfully using replay
	require.Equal(t, ExecutionStatusCompleted, execution2.Status())
}

func TestExecution_PathBranching(t *testing.T) {
	// Create mock agent
	mockAgent := &mockAgent{}

	// Create steps with branching
	step1 := workflow.NewStep(workflow.StepOptions{
		Name:   "start",
		Type:   "prompt",
		Agent:  mockAgent,
		Prompt: "Starting step",
		Next: []*workflow.Edge{
			{Step: "branch1", Condition: "true"},
			{Step: "branch2", Condition: "true"},
		},
	})

	step2 := workflow.NewStep(workflow.StepOptions{
		Name:   "branch1",
		Type:   "action",
		Action: "Print",
		Parameters: map[string]any{
			"Message": "Branch 1 executed",
		},
	})

	step3 := workflow.NewStep(workflow.StepOptions{
		Name:   "branch2",
		Type:   "action",
		Action: "Print",
		Parameters: map[string]any{
			"Message": "Branch 2 executed",
		},
	})

	// Create workflow
	wf, err := workflow.New(workflow.Options{
		Name:  "branching-test-workflow",
		Steps: []*workflow.Step{step1, step2, step3},
	})
	require.NoError(t, err)

	// Create environment
	env, err := New(Options{
		Name:   "branching-test-env",
		Agents: []dive.Agent{mockAgent},
	})
	require.NoError(t, err)

	// Start environment
	err = env.Start(context.Background())
	require.NoError(t, err)
	defer env.Stop(context.Background())

	// Create event store
	eventStore := &mockEventStore{}

	// Create execution
	execution, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  eventStore,
		Logger:      slogger.DefaultLogger,
		ReplayMode:  false,
	})
	require.NoError(t, err)

	// Run execution
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)

	// Verify final state
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Check that multiple paths were created
	execution.mutex.RLock()
	pathCount := len(execution.paths)
	execution.mutex.RUnlock()

	require.True(t, pathCount >= 2, "Should have created multiple paths for branching")

	// Verify events include path branching
	events, err := eventStore.GetEventHistory(ctx, execution.ID())
	require.NoError(t, err)

	var hasPathBranching bool
	for _, event := range events {
		if event.EventType == EventPathBranched {
			hasPathBranching = true
			break
		}
	}
	require.True(t, hasPathBranching, "Should have recorded path branching event")
}

func TestExecution_ParallelExecution(t *testing.T) {
	// Create mock agent
	mockAgent := &mockAgent{}

	// Create steps that can run in parallel
	step1 := workflow.NewStep(workflow.StepOptions{
		Name:   "start",
		Type:   "prompt",
		Agent:  mockAgent,
		Prompt: "Starting parallel execution",
		Next: []*workflow.Edge{
			{Step: "parallel1"},
			{Step: "parallel2"},
		},
	})

	step2 := workflow.NewStep(workflow.StepOptions{
		Name:   "parallel1",
		Type:   "action",
		Action: "Print",
		Parameters: map[string]any{
			"Message": "Parallel task 1",
		},
	})

	step3 := workflow.NewStep(workflow.StepOptions{
		Name:   "parallel2",
		Type:   "action",
		Action: "Print",
		Parameters: map[string]any{
			"Message": "Parallel task 2",
		},
	})

	// Create workflow
	wf, err := workflow.New(workflow.Options{
		Name:  "parallel-test-workflow",
		Steps: []*workflow.Step{step1, step2, step3},
	})
	require.NoError(t, err)

	// Create environment
	env, err := New(Options{
		Name:   "parallel-test-env",
		Agents: []dive.Agent{mockAgent},
	})
	require.NoError(t, err)

	// Start environment
	err = env.Start(context.Background())
	require.NoError(t, err)
	defer env.Stop(context.Background())

	// Create event store
	eventStore := &mockEventStore{}

	// Create execution
	execution, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  eventStore,
		Logger:      slogger.DefaultLogger,
		ReplayMode:  false,
	})
	require.NoError(t, err)

	// Run execution
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)

	// Verify execution completed successfully
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Check that both parallel paths completed
	execution.mutex.RLock()
	paths := make([]*PathState, 0, len(execution.paths))
	for _, path := range execution.paths {
		paths = append(paths, path)
	}
	execution.mutex.RUnlock()

	// Should have at least the main path and the parallel paths
	require.True(t, len(paths) >= 2, "Should have multiple paths")

	// All paths should be completed
	for _, path := range paths {
		require.Equal(t, PathStatusCompleted, path.Status, "All paths should be completed")
	}
}

func TestExecution_ConditionEvaluation(t *testing.T) {
	// Create mock agent
	mockAgent := &mockAgent{}

	// Create steps with conditions
	step1 := workflow.NewStep(workflow.StepOptions{
		Name:   "start",
		Type:   "prompt",
		Agent:  mockAgent,
		Prompt: "Starting conditional execution",
		Next: []*workflow.Edge{
			{Step: "true_branch", Condition: "true"},
			{Step: "false_branch", Condition: "false"},
		},
	})

	step2 := workflow.NewStep(workflow.StepOptions{
		Name:   "true_branch",
		Type:   "action",
		Action: "Print",
		Parameters: map[string]any{
			"Message": "True branch executed",
		},
	})

	step3 := workflow.NewStep(workflow.StepOptions{
		Name:   "false_branch",
		Type:   "action",
		Action: "Print",
		Parameters: map[string]any{
			"Message": "False branch executed",
		},
	})

	// Create workflow
	wf, err := workflow.New(workflow.Options{
		Name:  "conditional-test-workflow",
		Steps: []*workflow.Step{step1, step2, step3},
	})
	require.NoError(t, err)

	// Create environment
	env, err := New(Options{
		Name:   "conditional-test-env",
		Agents: []dive.Agent{mockAgent},
	})
	require.NoError(t, err)

	// Start environment
	err = env.Start(context.Background())
	require.NoError(t, err)
	defer env.Stop(context.Background())

	// Create event store
	eventStore := &mockEventStore{}

	// Create execution
	execution, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  eventStore,
		Logger:      slogger.DefaultLogger,
		ReplayMode:  false,
	})
	require.NoError(t, err)

	// Run execution
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)

	// Verify execution completed successfully
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Check that only the true branch was executed
	execution.mutex.RLock()
	stepOutputs := make(map[string]string)
	for _, path := range execution.paths {
		for stepName, output := range path.StepOutputs {
			stepOutputs[stepName] = output
		}
	}
	execution.mutex.RUnlock()

	// Should have executed the true branch but not the false branch
	require.Contains(t, stepOutputs, "true_branch", "True branch should have been executed")
	require.NotContains(t, stepOutputs, "false_branch", "False branch should not have been executed")
}
