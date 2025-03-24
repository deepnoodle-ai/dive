package workflow

import (
	"context"
	"sort"

	"github.com/risor-io/risor"
	"github.com/risor-io/risor/compiler"
	"github.com/risor-io/risor/parser"
)

var _ Condition = &EvalCondition{}

type EvalCondition struct {
	Condition string
	Globals   map[string]any
	Code      *compiler.Code
}

func NewEvalCondition(condition string, globals map[string]any) (*EvalCondition, error) {
	ast, err := parser.Parse(context.Background(), condition)
	if err != nil {
		return nil, err
	}
	var globalNames []string
	for name := range globals {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)
	code, err := compiler.Compile(ast, compiler.WithGlobalNames(globalNames))
	if err != nil {
		return nil, err
	}
	return &EvalCondition{Condition: condition, Globals: globals, Code: code}, nil
}

func (c *EvalCondition) Evaluate(ctx context.Context, inputs map[string]any) (bool, error) {
	globals := make(map[string]any)
	for k, v := range c.Globals {
		globals[k] = v
	}
	for k, v := range inputs {
		globals[k] = v
	}
	result, err := risor.EvalCode(ctx, c.Code, risor.WithGlobals(globals))
	if err != nil {
		return false, err
	}
	return result.IsTruthy(), nil
}
