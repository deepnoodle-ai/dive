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

// TestNewEnvironment tests basic environment creation and setup
func TestNewEnvironment(t *testing.T) {
	// Create a simple agent and workflow
	mockAgent := agent.NewMockAgent(agent.MockAgentOptions{
		Name: "test-agent",
		Response: &dive.Response{
			ID:    "test-response",
			Model: "test-model",
		},
	})

	simpleWorkflow, err := workflow.New(workflow.Options{
		Name: "simple-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "simple_step",
				Type:   "script",
				Script: `"Hello from workflow"`,
			}),
		},
	})
	require.NoError(t, err)

	t.Run("successful environment creation", func(t *testing.T) {
		env, err := New(Options{
			Name:      "test-env",
			Agents:    []dive.Agent{mockAgent},
			Workflows: []*workflow.Workflow{simpleWorkflow},
			Logger:    slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)
		require.NotNil(t, env)
		require.Equal(t, "test-env", env.Name())
		require.Len(t, env.Agents(), 1)
		require.Len(t, env.Workflows(), 1)
	})

	t.Run("environment requires name", func(t *testing.T) {
		_, err := New(Options{
			Agents:    []dive.Agent{mockAgent},
			Workflows: []*workflow.Workflow{simpleWorkflow},
			Logger:    slogger.NewDevNullLogger(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "environment name is required")
	})

	t.Run("duplicate agent names not allowed", func(t *testing.T) {
		duplicateAgent := agent.NewMockAgent(agent.MockAgentOptions{
			Name: "test-agent", // Same name as mockAgent
		})

		_, err := New(Options{
			Name:      "test-env",
			Agents:    []dive.Agent{mockAgent, duplicateAgent},
			Workflows: []*workflow.Workflow{simpleWorkflow},
			Logger:    slogger.NewDevNullLogger(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "agent already registered")
	})
}

// TestEnvironmentStartStop tests environment lifecycle
func TestEnvironmentStartStop(t *testing.T) {
	env, err := New(Options{
		Name:   "lifecycle-test",
		Logger: slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Initially not running
	require.False(t, env.IsRunning())

	// Start the environment
	err = env.Start(ctx)
	require.NoError(t, err)
	require.True(t, env.IsRunning())

	// Can't start twice
	err = env.Start(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already started")

	// Stop the environment
	err = env.Stop(ctx)
	require.NoError(t, err)
	require.False(t, env.IsRunning())

	// Can't stop twice
	err = env.Stop(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not started")
}

// TestEnvironmentDefaultAgent tests default agent selection
func TestEnvironmentDefaultAgent(t *testing.T) {
	t.Run("no agents", func(t *testing.T) {
		env, err := New(Options{
			Name:   "no-agents",
			Logger: slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		agent, found := env.DefaultAgent()
		require.False(t, found)
		require.Nil(t, agent)
	})

	t.Run("single agent becomes default", func(t *testing.T) {
		mockAgent := agent.NewMockAgent(agent.MockAgentOptions{
			Name: "solo-agent",
		})

		env, err := New(Options{
			Name:   "single-agent",
			Agents: []dive.Agent{mockAgent},
			Logger: slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		defaultAgent, found := env.DefaultAgent()
		require.True(t, found)
		require.Equal(t, "solo-agent", defaultAgent.Name())
	})

	t.Run("supervisor agent is preferred as default", func(t *testing.T) {
		regularAgent := agent.NewMockAgent(agent.MockAgentOptions{
			Name:         "regular-agent",
			IsSupervisor: false,
		})

		supervisorAgent := agent.NewMockAgent(agent.MockAgentOptions{
			Name:         "supervisor-agent",
			IsSupervisor: true,
		})

		env, err := New(Options{
			Name:   "multi-agent",
			Agents: []dive.Agent{regularAgent, supervisorAgent},
			Logger: slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)

		defaultAgent, found := env.DefaultAgent()
		require.True(t, found)
		require.Equal(t, "supervisor-agent", defaultAgent.Name())
	})
}

// TestEnvironmentAgentManagement tests agent addition and retrieval
func TestEnvironmentAgentManagement(t *testing.T) {
	env, err := New(Options{
		Name:   "agent-management",
		Logger: slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Initially no agents
	require.Len(t, env.Agents(), 0)

	// Add an agent
	mockAgent := agent.NewMockAgent(agent.MockAgentOptions{
		Name: "new-agent",
	})

	err = env.AddAgent(mockAgent)
	require.NoError(t, err)
	require.Len(t, env.Agents(), 1)

	// Can retrieve the agent
	retrievedAgent, err := env.GetAgent("new-agent")
	require.NoError(t, err)
	require.Equal(t, "new-agent", retrievedAgent.Name())

	// Can't retrieve non-existent agent
	_, err = env.GetAgent("missing-agent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent not found")

	// Can't add duplicate agent
	duplicateAgent := agent.NewMockAgent(agent.MockAgentOptions{
		Name: "new-agent",
	})
	err = env.AddAgent(duplicateAgent)
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent already present")
}

// TestEnvironmentWorkflowManagement tests workflow addition and retrieval
func TestEnvironmentWorkflowManagement(t *testing.T) {
	env, err := New(Options{
		Name:   "workflow-management",
		Logger: slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Initially no workflows
	require.Len(t, env.Workflows(), 0)

	// Add a workflow
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "test-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "test_step",
				Type:   "script",
				Script: `"test"`,
			}),
		},
	})
	require.NoError(t, err)

	err = env.AddWorkflow(testWorkflow)
	require.NoError(t, err)
	require.Len(t, env.Workflows(), 1)

	// Can retrieve the workflow
	retrievedWorkflow, err := env.GetWorkflow("test-workflow")
	require.NoError(t, err)
	require.Equal(t, "test-workflow", retrievedWorkflow.Name())

	// Can't retrieve non-existent workflow
	_, err = env.GetWorkflow("missing-workflow")
	require.Error(t, err)
	require.Contains(t, err.Error(), "workflow not found")

	// Can't add duplicate workflow
	duplicateWorkflow, err := workflow.New(workflow.Options{
		Name: "test-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "another_step",
				Type:   "script",
				Script: `"duplicate"`,
			}),
		},
	})
	require.NoError(t, err)

	err = env.AddWorkflow(duplicateWorkflow)
	require.Error(t, err)
	require.Contains(t, err.Error(), "workflow already present")
}

// TestEnvironmentWithWorkflowExecution demonstrates executing a workflow in an environment
func TestEnvironmentWithWorkflowExecution(t *testing.T) {
	// Create a mock agent with proper response
	mockAgent := agent.NewMockAgent(agent.MockAgentOptions{
		Name: "execution-agent",
		Response: &dive.Response{
			ID:    "test-response",
			Model: "test-model",
			Items: []*dive.ResponseItem{
				{
					Type: dive.ResponseItemTypeMessage,
					Message: &llm.Message{
						Role:    llm.Assistant,
						Content: []llm.Content{&llm.TextContent{Text: "Agent executed successfully"}},
					},
				},
			},
		},
	})

	// Create a workflow with both script and prompt steps
	testWorkflow, err := workflow.New(workflow.Options{
		Name: "execution-workflow",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "script_step",
				Type:   "script",
				Script: `"Script executed"`,
				Store:  "script_result",
				Next:   []*workflow.Edge{{Step: "prompt_step"}},
			}),
			workflow.NewStep(workflow.StepOptions{
				Name:   "prompt_step",
				Type:   "prompt",
				Agent:  mockAgent,
				Prompt: "Execute this prompt",
				Store:  "prompt_result",
			}),
		},
	})
	require.NoError(t, err)

	// Create environment with agent and workflow
	env, err := New(Options{
		Name:      "execution-env",
		Agents:    []dive.Agent{mockAgent},
		Workflows: []*workflow.Workflow{testWorkflow},
		Logger:    slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	// Start the environment
	ctx := context.Background()
	err = env.Start(ctx)
	require.NoError(t, err)
	defer env.Stop(ctx)

	// Create and run an execution
	execution, err := NewExecution(ExecutionOptions{
		Workflow:    testWorkflow,
		Environment: env,
		Inputs:      map[string]interface{}{},
		Logger:      slogger.NewDevNullLogger(),
	})
	require.NoError(t, err)

	err = execution.Run(ctx)
	require.NoError(t, err)
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Verify both steps executed and stored results
	scriptResult, exists := execution.state.Get("script_result")
	require.True(t, exists)
	require.Equal(t, "Script executed", scriptResult)

	promptResult, exists := execution.state.Get("prompt_result")
	require.True(t, exists)
	require.Equal(t, "Agent executed successfully", promptResult)
}
