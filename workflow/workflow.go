package workflow

import (
	"fmt"

	"github.com/getstingrai/dive"
)

type Trigger struct {
	Name   string
	Type   string
	Config map[string]interface{}
}

// Workflow defines a repeatable process as a graph of tasks to be executed
type Workflow struct {
	name        string
	description string
	inputs      []*dive.Input
	output      *dive.Output
	steps       []*Step
	graph       *Graph
	triggers    []*Trigger
}

// WorkflowOptions configures a new workflow
type WorkflowOptions struct {
	Name        string
	Description string
	Inputs      []*dive.Input
	Output      *dive.Output
	Steps       []*Step
	Triggers    []*Trigger
}

// NewWorkflow creates and validates a Workflow
func NewWorkflow(opts WorkflowOptions) (*Workflow, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("workflow name required")
	}
	if len(opts.Steps) == 0 {
		return nil, fmt.Errorf("steps required")
	}
	graph := NewGraph(opts.Steps, opts.Steps[0])
	if err := graph.Validate(); err != nil {
		return nil, fmt.Errorf("graph validation failed: %w", err)
	}
	w := &Workflow{
		name:        opts.Name,
		description: opts.Description,
		inputs:      opts.Inputs,
		output:      opts.Output,
		steps:       opts.Steps,
		graph:       graph,
		triggers:    opts.Triggers,
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Workflow) Name() string {
	return w.name
}

func (w *Workflow) Description() string {
	return w.description
}

func (w *Workflow) Inputs() []*dive.Input {
	return w.inputs
}

func (w *Workflow) Output() *dive.Output {
	return w.output
}

func (w *Workflow) Steps() []*Step {
	return w.steps
}

func (w *Workflow) Graph() *Graph {
	return w.graph
}

func (w *Workflow) Triggers() []*Trigger {
	return w.triggers
}

// Validate checks if the workflow is properly configured
func (w *Workflow) Validate() error {
	if w.name == "" {
		return fmt.Errorf("workflow name required")
	}
	if w.graph == nil {
		return fmt.Errorf("graph required")
	}
	startStep := w.graph.Start()
	if startStep == nil {
		return fmt.Errorf("graph start task required")
	}
	return nil
}
