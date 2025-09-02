package environment

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/slogger"
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

	t.Run("successful environment creation", func(t *testing.T) {
		env, err := New(Options{
			Name:   "test-env",
			Agents: []dive.Agent{mockAgent},
			Logger: slogger.NewDevNullLogger(),
		})
		require.NoError(t, err)
		require.NotNil(t, env)
		require.Equal(t, "test-env", env.Name())
		require.Len(t, env.Agents(), 1)
	})

	t.Run("environment requires name", func(t *testing.T) {
		_, err := New(Options{
			Agents: []dive.Agent{mockAgent},
			Logger: slogger.NewDevNullLogger(),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "environment name is required")
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
