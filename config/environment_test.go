package config

import (
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/stretchr/testify/assert"
)

func TestDiveConfig_BuildAgents(t *testing.T) {
	// Create a test dive configuration
	config := &DiveConfig{
		Name:        "test-env",
		Description: "Test Environment",
		Config: Config{
			DefaultProvider:  "anthropic",
			DefaultModel:     "claude-3-sonnet-20240229",
			LogLevel:         "info",
			ConfirmationMode: "never",
		},
		Tools: []Tool{
			{
				Name: "read_file",
			},
		},
		Agents: []Agent{
			{
				Name:     "researcher",
				Goal:     "Assist with research",
				Provider: "anthropic",
				Model:    "claude-3-sonnet-20240229",
				Tools:    []string{"read_file"},
			},
			{
				Name:     "writer",
				Goal:     "Write compelling content",
				Provider: "anthropic",
				Model:    "claude-3-sonnet-20240229",
				Tools:    []string{"read_file"},
			},
		},
		Workflows: []Workflow{
			{
				Name:        "Research and Write",
				Description: "Research and write workflow",
				Steps: []Step{
					{
						Name: "Research",
						Next: []NextStep{
							{
								Step: "Write",
							},
						},
					},
					{
						Name: "Write",
					},
				},
			},
		},
		Triggers: []Trigger{
			{
				Name: "manual",
				Type: "manual",
			},
		},
	}

	// Build the agents
	agents, err := config.BuildAgents()
	assert.NoError(t, err)
	assert.NotNil(t, agents)

	// Verify agents
	assert.Len(t, agents, 2)

	// Verify researcher agent
	researcher := findAgentByName(agents, "researcher")
	assert.NotNil(t, researcher)
	assert.False(t, researcher.(*agent.Agent).IsSupervisor())

	// Verify writer agent
	writer := findAgentByName(agents, "writer")
	assert.NotNil(t, writer)
	assert.False(t, writer.(*agent.Agent).IsSupervisor())
}

func findAgentByName(agents []dive.Agent, name string) dive.Agent {
	for _, agent := range agents {
		if agent.Name() == name {
			return agent
		}
	}
	return nil
}
