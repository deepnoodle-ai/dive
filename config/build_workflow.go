package config

import (
	"encoding/json"
	"fmt"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/workflow"
)

func buildWorkflow(workflowDef Workflow, agents []dive.Agent, prompts []*dive.Prompt) (*workflow.Workflow, error) {
	if len(workflowDef.Steps) == 0 {
		return nil, fmt.Errorf("no steps found")
	}

	agentsByName := make(map[string]dive.Agent)
	for _, agent := range agents {
		agentsByName[agent.Name()] = agent
	}

	promptsByName := make(map[string]*dive.Prompt)
	for _, prompt := range prompts {
		promptsByName[prompt.Name] = prompt
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

		// Handle Prompt if present
		var prompt *dive.Prompt
		if step.Prompt != nil {
			switch promptDef := step.Prompt.(type) {
			case string:
				prompt = &dive.Prompt{Text: promptDef}
			case map[string]any:
				var promptObj Prompt
				data, err := json.Marshal(promptDef)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal prompt: %w", err)
				}
				if err := json.Unmarshal(data, &promptObj); err != nil {
					return nil, fmt.Errorf("failed to unmarshal prompt: %w", err)
				}
				var context []*dive.PromptContext
				for _, contextObj := range promptObj.Context {
					context = append(context, &dive.PromptContext{Text: contextObj})
				}
				prompt = &dive.Prompt{
					Name:         promptObj.Name,
					Text:         promptObj.Text,
					Output:       promptObj.Output,
					OutputFormat: promptObj.OutputFormat,
					Context:      context,
				}
			default:
				return nil, fmt.Errorf("invalid prompt: %v", step.Prompt)
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
			} else if step.Prompt != nil {
				stepType = "prompt"
			}
		}

		workflowStep := workflow.NewStep(workflow.StepOptions{
			Type:       stepType,
			Name:       step.Name,
			Agent:      agent,
			Prompt:     prompt,
			Next:       edges,
			Each:       each,
			Action:     step.Action,
			Parameters: step.Parameters,
			Store:      step.Store,
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

	var inputs []*dive.Input
	for _, input := range workflowDef.Inputs {
		inputs = append(inputs, &dive.Input{
			Name:        input.Name,
			Type:        input.Type,
			Description: input.Description,
			Required:    input.Required,
			Default:     input.Default,
		})
	}

	var output *dive.Output
	if workflowDef.Output != nil {
		output = &dive.Output{
			Name:        workflowDef.Output.Name,
			Type:        workflowDef.Output.Type,
			Description: workflowDef.Output.Description,
			Format:      workflowDef.Output.Format,
			Default:     workflowDef.Output.Default,
		}
	}

	return workflow.NewWorkflow(workflow.WorkflowOptions{
		Name:        workflowDef.Name,
		Description: workflowDef.Description,
		Inputs:      inputs,
		Output:      output,
		Steps:       steps,
		Triggers:    triggers,
	})
}
