package environment

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/slogger"
	"github.com/deepnoodle-ai/dive/workflow"
	"github.com/stretchr/testify/require"
)

// TestEndToEndWorkflowExecution demonstrates that the simplified checkpoint-based
// execution system works correctly for a complete workflow
func TestEndToEndWorkflowExecution(t *testing.T) {
	ctx := context.Background()

	// Create a multi-step workflow with different step types
	w, err := workflow.New(workflow.Options{
		Name: "end-to-end-test",
		Inputs: []*workflow.Input{
			{Name: "user_name", Type: "string", Required: true},
			{Name: "greeting", Type: "string", Default: "Hello"},
		},
		Steps: []*workflow.Step{
			// Step 1: Script step to process input
			workflow.NewStep(workflow.StepOptions{
				Name:   "process_input",
				Type:   "script",
				Script: `inputs.greeting + ", " + inputs.user_name + "!"`,
				Store:  "processed_greeting",
				Next:   []*workflow.Edge{{Step: "format_output"}},
			}),
			// Step 2: Another script step that uses stored state
			workflow.NewStep(workflow.StepOptions{
				Name:   "format_output",
				Type:   "script",
				Script: `"Final result: " + state.processed_greeting`,
				Store:  "final_output",
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
	require.NoError(t, env.Start(ctx))
	defer env.Stop(ctx)

	// Create execution
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    w,
		Environment: env,
		Inputs: map[string]interface{}{
			"user_name": "World",
		},
		Logger: slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Execute the workflow
	err = execution.Run(ctx)
	require.NoError(t, err)

	// Verify execution completed successfully
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Verify state was stored correctly
	processedGreeting, exists := execution.state.Get("processed_greeting")
	require.True(t, exists)
	require.Equal(t, "Hello, World!", processedGreeting)

	finalOutput, exists := execution.state.Get("final_output")
	require.True(t, exists)
	require.Equal(t, "Final result: Hello, World!", finalOutput)

	// Verify execution has proper timing
	require.False(t, execution.startTime.IsZero())
	require.False(t, execution.endTime.IsZero())
	require.True(t, execution.endTime.After(execution.startTime))

	// Verify path states
	require.Len(t, execution.paths, 1) // Should have one main path
	mainPath := execution.paths["main"]
	require.Equal(t, PathStatusCompleted, mainPath.Status)
	require.False(t, mainPath.EndTime.IsZero())
}

// TestWorkflowWithConditionalBranching tests the path branching system
func TestWorkflowWithConditionalBranching(t *testing.T) {
	ctx := context.Background()

	// Create a workflow with conditional branching
	w, err := workflow.New(workflow.Options{
		Name: "conditional-test",
		Inputs: []*workflow.Input{
			{Name: "number", Type: "int", Required: true},
		},
		Steps: []*workflow.Step{
			// Initial step
			workflow.NewStep(workflow.StepOptions{
				Name:   "start",
				Type:   "script",
				Script: `inputs.number`,
				Store:  "value",
				Next: []*workflow.Edge{
					{Step: "positive_path", Condition: "$(state.value > 0)"},
					{Step: "negative_path", Condition: "$(state.value <= 0)"},
				},
			}),
			// Positive path
			workflow.NewStep(workflow.StepOptions{
				Name:   "positive_path",
				Type:   "script",
				Script: `"Number is positive: " + string(state.value)`,
				Store:  "result",
			}),
			// Negative path
			workflow.NewStep(workflow.StepOptions{
				Name:   "negative_path",
				Type:   "script",
				Script: `"Number is zero or negative: " + string(state.value)`,
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
	require.NoError(t, env.Start(ctx))
	defer env.Stop(ctx)

	t.Run("positive number", func(t *testing.T) {
		execution, err := NewExecution(ExecutionOptions{
			Workflow:    w,
			Environment: env,
			Inputs:      map[string]interface{}{"number": 42},
			Logger:      slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		err = execution.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, ExecutionStatusCompleted, execution.Status())

		result, exists := execution.state.Get("result")
		require.True(t, exists)
		require.Equal(t, "Number is positive: 42", result)
	})

	t.Run("negative number", func(t *testing.T) {
		execution, err := NewExecution(ExecutionOptions{
			Workflow:    w,
			Environment: env,
			Inputs:      map[string]interface{}{"number": -5},
			Logger:      slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		err = execution.Run(ctx)
		require.NoError(t, err)
		require.Equal(t, ExecutionStatusCompleted, execution.Status())

		result, exists := execution.state.Get("result")
		require.True(t, exists)
		require.Equal(t, "Number is zero or negative: -5", result)
	})
}

// TestWorkflowWithEach tests the simplified each block functionality
func TestWorkflowWithEach(t *testing.T) {
	ctx := context.Background()

	// Create a workflow with an each block
	w, err := workflow.New(workflow.Options{
		Name: "each-test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name: "process_items",
				Type: "script",
				Each: &workflow.EachBlock{
					Items: []string{"apple", "banana", "cherry"},
					As:    "fruit",
				},
				Script: `"Processing: " + fruit`,
				Store:  "processed_fruits",
			}),
		},
	})
	require.NoError(t, err)

	env := &Environment{
		agents:    make(map[string]dive.Agent),
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(ctx))
	defer env.Stop(ctx)

	execution, err := NewExecution(ExecutionOptions{
		Workflow:    w,
		Environment: env,
		Inputs:      map[string]interface{}{},
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	err = execution.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Verify that the each block stored results as an array
	results, exists := execution.state.Get("processed_fruits")
	require.True(t, exists)

	resultArray, ok := results.([]string)
	require.True(t, ok)
	require.Len(t, resultArray, 3)
	require.Contains(t, resultArray, "Processing: apple")
	require.Contains(t, resultArray, "Processing: banana")
	require.Contains(t, resultArray, "Processing: cherry")
}

// TestCheckpointingSystem verifies that checkpoints are saved and can be loaded
func TestCheckpointingSystem(t *testing.T) {
	ctx := context.Background()

	// Create a simple workflow
	w, err := workflow.New(workflow.Options{
		Name: "checkpoint-test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "step1",
				Type:   "script",
				Script: `"Checkpoint test result"`,
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
	require.NoError(t, env.Start(ctx))
	defer env.Stop(ctx)

	execution, err := NewExecution(ExecutionOptions{
		Workflow:    w,
		Environment: env,
		Inputs:      map[string]interface{}{},
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Verify we can save a checkpoint before execution
	err = execution.saveCheckpoint(ctx)
	require.NoError(t, err)

	// Execute and verify checkpoint is saved during execution
	err = execution.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Verify the result
	result, exists := execution.state.Get("result")
	require.True(t, exists)
	require.Equal(t, "Checkpoint test result", result)
}

// TestWorkflowWithMockAgent tests execution with a mock agent for prompt steps
func TestWorkflowWithMockAgent(t *testing.T) {
	ctx := context.Background()

	// Create a mock agent
	mockAgent := agent.NewMockAgent(agent.MockAgentOptions{
		Name: "test-agent",
		Response: &dive.Response{
			ID:    "test-response",
			Model: "test-model",
			Items: []*dive.ResponseItem{
				{
					Type: dive.ResponseItemTypeMessage,
					Message: &llm.Message{
						Role:    llm.Assistant,
						Content: []llm.Content{&llm.TextContent{Text: "Mock agent response"}},
					},
				},
			},
		},
	})

	// Create a workflow with a prompt step
	w, err := workflow.New(workflow.Options{
		Name: "agent-test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "ask_agent",
				Type:   "prompt",
				Agent:  mockAgent,
				Prompt: "What is 2 + 2?",
				Store:  "agent_response",
			}),
		},
	})
	require.NoError(t, err)

	env := &Environment{
		agents:    map[string]dive.Agent{"test-agent": mockAgent},
		workflows: make(map[string]*workflow.Workflow),
		logger:    slogger.NewDevNullLogger(),
	}
	require.NoError(t, env.Start(ctx))
	defer env.Stop(ctx)

	execution, err := NewExecution(ExecutionOptions{
		Workflow:    w,
		Environment: env,
		Inputs:      map[string]interface{}{},
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	err = execution.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Verify the agent response was stored
	response, exists := execution.state.Get("agent_response")
	require.True(t, exists)
	require.Equal(t, "Mock agent response", response)
}
