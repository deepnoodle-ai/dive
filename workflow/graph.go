package workflow

import (
	"fmt"
	"sort"
)

type Graph struct {
	steps map[string]*Step
	start *Step
}

type GraphOptions struct {
	Nodes map[string]*Step
}

// NewGraph creates a new graph containing the given steps
func NewGraph(steps []*Step, start *Step) *Graph {
	graphNodes := make(map[string]*Step, len(steps))
	for _, step := range steps {
		if step.name == "" {
			step.name = step.Name()
		}
		graphNodes[step.name] = step
	}
	return &Graph{
		steps: graphNodes,
		start: start,
	}
}

// Start returns the start step(s) of the graph
func (g *Graph) Start() *Step {
	return g.start
}

// Get returns a step by name
func (g *Graph) Get(name string) (*Step, bool) {
	step, ok := g.steps[name]
	return step, ok
}

// Names returns the names of all steps in the graph
func (g *Graph) Names() []string {
	names := make([]string, 0, len(g.steps))
	for name := range g.steps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (g *Graph) Validate() error {
	if len(g.steps) == 0 {
		return fmt.Errorf("graph must have at least one step")
	}
	for _, step := range g.steps {
		if step.name == "" {
			return fmt.Errorf("step name cannot be empty")
		}
		for _, edge := range step.next {
			if _, ok := g.steps[edge.Step]; !ok {
				return fmt.Errorf("edge to step %q not found", edge.Step)
			}
		}
	}
	return nil
}
