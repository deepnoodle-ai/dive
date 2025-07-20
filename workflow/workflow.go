package workflow

import (
	"fmt"
)

// Input defines an expected input parameter
type Input struct {
	Name        string      `json:"name"`
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Default     interface{} `json:"default,omitempty"`
}

// Output defines an expected output parameter
type Output struct {
	Name        string      `json:"name"`
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Format      string      `json:"format,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Document    string      `json:"document,omitempty"`
}

type Trigger struct {
	Name   string
	Type   string
	Config map[string]interface{}
}

// Workflow defines a repeatable process as a graph of tasks to be executed.
type Workflow struct {
	name        string
	description string
	path        string
	inputs      []*Input
	output      *Output
	steps       []*Step
	graph       *Graph
	triggers    []*Trigger
}

// Options are used to configure a Workflow.
type Options struct {
	Name        string
	Description string
	Path        string
	Inputs      []*Input
	Output      *Output
	Steps       []*Step
	Triggers    []*Trigger
}

// New returns a new Workflow configured with the given options.
func New(opts Options) (*Workflow, error) {
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
		path:        opts.Path,
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

func (w *Workflow) Path() string {
	return w.path
}

func (w *Workflow) Name() string {
	return w.name
}

func (w *Workflow) Description() string {
	return w.description
}

func (w *Workflow) Inputs() []*Input {
	return w.inputs
}

func (w *Workflow) Output() *Output {
	return w.output
}

func (w *Workflow) Steps() []*Step {
	return w.steps
}

func (w *Workflow) Start() *Step {
	return w.graph.Start()
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
