// Package workflow provides sequential agent pipeline primitives.
//
// Usage:
//
//	pipeline := workflow.Sequential(classifierAgent, researchAgent, writerAgent)
//	response, err := pipeline.Run(ctx, dive.WithInput("Research quantum computing trends"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(response.OutputText())
//
//	// Inspect intermediate results
//	for i, step := range pipeline.Steps() {
//	    fmt.Printf("Step %d: %s\n", i+1, step.OutputText())
//	}
package workflow

import (
	"context"

	"github.com/deepnoodle-ai/dive"
)

// StepMapper transforms one step's response into the next step's input option.
type StepMapper func(ctx context.Context, resp *dive.Response) (dive.CreateResponseOption, error)

// Pipeline chains a sequence of agents, passing each step's output as the
// next step's input. It is not concurrency-safe across simultaneous Run calls.
type Pipeline struct {
	agents  []*dive.Agent
	mapper  StepMapper
	results []*dive.Response
}

// Sequential chains agents using the default mapper: OutputText() → WithInput().
func Sequential(agents ...*dive.Agent) *Pipeline {
	return SequentialWithMapper(agents, defaultMapper)
}

// SequentialWithMapper chains agents with a custom per-step transform.
func SequentialWithMapper(agents []*dive.Agent, mapper StepMapper) *Pipeline {
	return &Pipeline{
		agents: agents,
		mapper: mapper,
	}
}

// Run executes the pipeline. The provided option seeds the first agent's input.
// On the first error, Run returns immediately. The final step's Response is
// returned on success.
func (p *Pipeline) Run(ctx context.Context, input dive.CreateResponseOption) (*dive.Response, error) {
	p.results = make([]*dive.Response, 0, len(p.agents))

	current := input
	var resp *dive.Response
	for i, agent := range p.agents {
		var err error
		resp, err = agent.CreateResponse(ctx, current)
		if err != nil {
			return nil, err
		}
		p.results = append(p.results, resp)

		// Don't call the mapper after the last step.
		if i < len(p.agents)-1 {
			next, mapErr := p.mapper(ctx, resp)
			if mapErr != nil {
				return nil, mapErr
			}
			current = next
		}
	}
	return resp, nil
}

// Steps returns a copy of the intermediate responses from the most recent Run
// call. Index 0 corresponds to the first agent in the pipeline.
func (p *Pipeline) Steps() []*dive.Response {
	out := make([]*dive.Response, len(p.results))
	copy(out, p.results)
	return out
}

func defaultMapper(_ context.Context, resp *dive.Response) (dive.CreateResponseOption, error) {
	return dive.WithInput(resp.OutputText()), nil
}
