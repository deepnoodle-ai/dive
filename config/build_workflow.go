package config

import (
	"context"
	"fmt"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/workflow"
)

func buildWorkflow(
	ctx context.Context,
	repo dive.DocumentRepository,
	workflowDef Workflow,
	agents []dive.Agent,
	basePath string,
) (*workflow.Workflow, error) {
	if len(workflowDef.Steps) == 0 {
		return nil, fmt.Errorf("no steps found")
	}

	agentsByName := make(map[string]dive.Agent)
	for _, agent := range agents {
		agentsByName[agent.Name()] = agent
	}

	steps := []*workflow.Step{}
	for i, step := range workflowDef.Steps {
		// Handle Next steps with conditions
		var edges []*workflow.Edge
		if step.Next != nil {
			for _, next := range step.Next {
				edges = append(edges, &workflow.Edge{
					Step:      next.Step,
					Condition: next.Condition,
				})
			}
		} else if !step.End && i < len(workflowDef.Steps)-1 {
			// Implicit next step if not end and not last step
			edges = append(edges, &workflow.Edge{
				Step: workflowDef.Steps[i+1].Name,
			})
		}

		// Handle Each block if present
		var each *workflow.EachBlock
		if step.Each != nil {
			each = &workflow.EachBlock{
				Items: step.Each.Items,
				As:    step.Each.As,
			}
		}

		// Handle Agent if present
		var agent dive.Agent
		if step.Agent != "" {
			agent = agentsByName[step.Agent]
			if agent == nil {
				return nil, fmt.Errorf("agent %s not found", step.Agent)
			}
		}

		stepType := step.Type
		if stepType == "" {
			if step.Action != "" {
				stepType = "action"
			} else if step.Prompt != "" {
				stepType = "prompt"
			} else if step.Script != "" {
				stepType = "script"
			}
		}

		// Build context content for the step if any
		var contextContent []llm.Content
		if len(step.Content) > 0 {
			var err error
			contextContent, err = buildContextContent(ctx, repo, basePath, step.Content)
			if err != nil {
				return nil, fmt.Errorf("error building step context: %w", err)
			}
		}

		workflowStep := workflow.NewStep(workflow.StepOptions{
			Type:       stepType,
			Name:       step.Name,
			Agent:      agent,
			Prompt:     step.Prompt,
			Script:     step.Script,
			Next:       edges,
			Each:       each,
			Action:     step.Action,
			Parameters: step.Parameters,
			Store:      step.Store,
			Content:    contextContent,
		})
		steps = append(steps, workflowStep)
	}

	// Convert triggers from config to workflow format
	var triggers []*workflow.Trigger
	for _, trigger := range workflowDef.Triggers {
		triggers = append(triggers, &workflow.Trigger{
			Type:   trigger.Type,
			Config: trigger.Config,
		})
	}

	var inputs []*workflow.Input
	for _, input := range workflowDef.Inputs {
		inputs = append(inputs, &workflow.Input{
			Name:        input.Name,
			Type:        input.Type,
			Description: input.Description,
			Required:    input.Required,
			Default:     input.Default,
		})
	}

	var output *workflow.Output
	if workflowDef.Output != nil {
		output = &workflow.Output{
			Name:        workflowDef.Output.Name,
			Type:        workflowDef.Output.Type,
			Description: workflowDef.Output.Description,
			Format:      workflowDef.Output.Format,
			Default:     workflowDef.Output.Default,
		}
	}

	return workflow.New(workflow.Options{
		Name:        workflowDef.Name,
		Description: workflowDef.Description,
		Path:        workflowDef.Path,
		Inputs:      inputs,
		Output:      output,
		Steps:       steps,
		Triggers:    triggers,
	})
}
