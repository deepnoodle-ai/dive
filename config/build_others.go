package config

import (
	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/environment"
)

func buildTrigger(triggerDef Trigger) (*environment.Trigger, error) {
	return environment.NewTrigger(triggerDef.Name), nil
}

func buildPrompt(promptDef Prompt) (*dive.Prompt, error) {
	return &dive.Prompt{
		Name:         promptDef.Name,
		Text:         promptDef.Text,
		Output:       promptDef.Output,
		OutputFormat: promptDef.OutputFormat,
	}, nil
}
