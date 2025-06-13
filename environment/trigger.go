package environment

import (
	"context"
	"errors"

	"github.com/diveagents/dive/workflow"
)

type Trigger struct {
	name      string
	workflows []*workflow.Workflow
	env       *Environment
}

func NewTrigger(name string) *Trigger {
	return &Trigger{
		name: name,
	}
}

func (t *Trigger) SetEnvironment(env *Environment) {
	t.env = env
}

func (t *Trigger) Name() string {
	return t.name
}

func (t *Trigger) Subscribe(workflow *workflow.Workflow) error {
	t.workflows = append(t.workflows, workflow)
	return nil
}

func (t *Trigger) Unsubscribe(workflow *workflow.Workflow) error {
	for i, w := range t.workflows {
		if w == workflow {
			t.workflows = append(t.workflows[:i], t.workflows[i+1:]...)
		}
	}
	return nil
}

func (t *Trigger) Fire(ctx context.Context, input map[string]interface{}) ([]*EventBasedExecution, error) {
	if t.env == nil {
		return nil, errors.New("trigger not associated with an environment")
	}
	var executions []*EventBasedExecution
	for _, w := range t.workflows {
		execution, err := t.env.ExecuteWorkflow(ctx, ExecutionOptions{
			WorkflowName: w.Name(),
			Inputs:       input,
		})
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, nil
}
