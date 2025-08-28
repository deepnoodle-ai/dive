package config

import (
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/workflow"
	"github.com/stretchr/testify/assert"
)

func TestEnvironment_Build(t *testing.T) {
	// Create a test environment configuration
	env := &Environment{
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

	// Build the environment
	result, err := env.Build()
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Verify environment properties
	assert.Equal(t, "test-env", result.Name())
	assert.Equal(t, "Test Environment", result.Description())

	// Verify agents
	agents := result.Agents()
	assert.Len(t, agents, 2)

	// Verify researcher agent
	researcher := findAgentByName(agents, "researcher")
	assert.NotNil(t, researcher)
	assert.False(t, researcher.(*agent.Agent).IsSupervisor())

	// Verify writer agent
	writer := findAgentByName(agents, "writer")
	assert.NotNil(t, writer)
	assert.False(t, writer.(*agent.Agent).IsSupervisor())

	// Verify workflows
	workflows := result.Workflows()
	assert.Len(t, workflows, 1)

	// Verify research-and-write workflow
	researchWorkflow := findWorkflowByName(workflows, "Research and Write")
	assert.NotNil(t, researchWorkflow)
	assert.Equal(t, "Research and write workflow", researchWorkflow.Description())

	// Verify workflow steps
	steps := researchWorkflow.Steps()
	assert.Len(t, steps, 2)

	// Verify step names
	stepNames := make([]string, len(steps))
	for i, step := range steps {
		stepNames[i] = step.Name()
	}
	assert.Contains(t, stepNames, "Research")
	assert.Contains(t, stepNames, "Write")
}

func findAgentByName(agents []dive.Agent, name string) dive.Agent {
	for _, agent := range agents {
		if agent.Name() == name {
			return agent
		}
	}
	return nil
}

func findWorkflowByName(workflows []*workflow.Workflow, name string) *workflow.Workflow {
	for _, wf := range workflows {
		if wf.Name() == name {
			return wf
		}
	}
	return nil
}
