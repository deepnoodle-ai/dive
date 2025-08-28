package config

import (
	"github.com/deepnoodle-ai/dive/environment"
)

func buildTrigger(triggerDef Trigger) (*environment.Trigger, error) {
	return environment.NewTrigger(triggerDef.Name), nil
}
