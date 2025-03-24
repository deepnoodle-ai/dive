package config

import (
	"testing"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/agent"
	"github.com/getstingrai/dive/workflow"
	"github.com/stretchr/testify/assert"
)

func TestEnvironment_Build(t *testing.T) {
	// Create a test environment configuration
	env := &Environment{
		Name:        "test-env",
		Description: "Test Environment",
		Config: Config{
			LLM: LLMConfig{
				DefaultProvider: "anthropic",
				DefaultModel:    "claude-3-sonnet-20240229",
			},
			Logging: LoggingConfig{
				Level: "info",
			},
		},
		Tools: []Tool{
			{
				Name:    "google_search",
				Enabled: true,
			},
			{
				Name:    "file_read",
				Enabled: true,
			},
		},
		Agents: []Agent{
			{
				Name:     "researcher",
				Goal:     "Assist with research",
				Provider: "anthropic",
				Model:    "claude-3-sonnet-20240229",
				Tools:    []string{"google_search", "file_read"},
			},
			{
				Name:     "writer",
				Goal:     "Write compelling content",
				Provider: "anthropic",
				Model:    "claude-3-sonnet-20240229",
				Tools:    []string{"file_read"},
			},
		},
		Workflows: []Workflow{
			{
				Name:        "research-and-write",
				Description: "Research and write workflow",
				Steps: []Step{
					{
						Name: "research-step",
						Next: []NextStep{
							{
								Step: "write-step",
							},
						},
					},
					{
						Name: "write-step",
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
	researchWorkflow := findWorkflowByName(workflows, "research-and-write")
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
	assert.Contains(t, stepNames, "research-step")
	assert.Contains(t, stepNames, "write-step")
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
