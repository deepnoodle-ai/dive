package config

import (
	"github.com/diveagents/dive/environment"
)

func buildTrigger(triggerDef Trigger) (*environment.Trigger, error) {
	return environment.NewTrigger(triggerDef.Name), nil
}
