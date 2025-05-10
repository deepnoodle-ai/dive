package workflow

import (
	"context"
	"errors"

	"github.com/diveagents/dive"
)

type Condition interface {
	Evaluate(ctx context.Context, inputs map[string]interface{}) (bool, error)
}

type Variable interface {
	GetValue() any
	SetValue(value any)
}

type Edge struct {
	Step      string
	Condition string
}

type EachBlock struct {
	Items any
	As    string
}

// Step represents a single step in a workflow
type Step struct {
	stepType    string
	name        string
	description string
	agent       dive.Agent
	prompt      string
	store       string
	action      string
	parameters  map[string]any
	each        *EachBlock
	next        []*Edge
	seconds     float64
	end         bool
}

// StepOptions configures a new workflow step
type StepOptions struct {
	Type        string
	Name        string
	Description string
	Agent       dive.Agent
	Prompt      string
	Store       string
	Action      string
	Parameters  map[string]any
	Each        *EachBlock
	Next        []*Edge
	Seconds     float64
	End         bool
}

func NewStep(opts StepOptions) *Step {
	if opts.Type == "" && opts.Prompt != "" {
		opts.Type = "prompt"
	}
	return &Step{
		stepType:    opts.Type,
		name:        opts.Name,
		description: opts.Description,
		agent:       opts.Agent,
		prompt:      opts.Prompt,
		store:       opts.Store,
		action:      opts.Action,
		parameters:  opts.Parameters,
		each:        opts.Each,
		next:        opts.Next,
		seconds:     opts.Seconds,
		end:         opts.End,
	}
}

func (s *Step) Type() string {
	return s.stepType
}

func (s *Step) Name() string {
	return s.name
}

func (s *Step) Description() string {
	return s.description
}

func (s *Step) Agent() dive.Agent {
	return s.agent
}

func (s *Step) Prompt() string {
	return s.prompt
}

func (s *Step) Store() string {
	return s.store
}

func (s *Step) Action() string {
	return s.action
}

func (s *Step) Parameters() map[string]any {
	return s.parameters
}

func (s *Step) Each() *EachBlock {
	return s.each
}

// Next returns the next edges for this step
func (s *Step) Next() []*Edge {
	return s.next
}

func (s *Step) Compile(ctx context.Context) error {
	// if s.withCode != nil {
	// 	return nil // already compiled
	// }
	// withCode := make(map[string]*compiler.Code)
	// for name, value := range s.with {
	// 	strValue, ok := value.(string)
	// 	if !ok {
	// 		continue
	// 	}
	// 	if !strings.HasPrefix(strValue, "$(") {
	// 		continue
	// 	}
	// 	if !strings.HasSuffix(strValue, ")") {
	// 		return fmt.Errorf("step: %q variable: %q error: invalid with value", s.name, name)
	// 	}
	// 	// Extract and compile the code
	// 	trimmedValue := strings.TrimPrefix(strValue, "$(")
	// 	trimmedValue = strings.TrimSuffix(trimmedValue, ")")
	// 	code, err := compileScript(ctx, trimmedValue, map[string]any{
	// 		"inputs":    nil,
	// 		"documents": nil,
	// 	})
	// 	if err != nil {
	// 		return fmt.Errorf("step: %q variable: %q error: %w", s.name, name, err)
	// 	}
	// 	withCode[name] = code
	// }
	// s.withCode = withCode
	return errors.New("not implemented")
}

// func (s *Step) Code(name string) (*compiler.Code, bool) {
// 	code, ok := s.withCode[name]
// 	return code, ok
// }
