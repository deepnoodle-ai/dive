package eval

import (
	"context"
	"fmt"
	"sort"

	"github.com/risor-io/risor/compiler"
	"github.com/risor-io/risor/object"
	"github.com/risor-io/risor/parser"
)

func convertRisorEachValue(obj object.Object) ([]any, error) {
	switch obj := obj.(type) {
	case *object.String:
		return []any{obj.Value()}, nil
	case *object.Int:
		return []any{obj.Value()}, nil
	case *object.Float:
		return []any{obj.Value()}, nil
	case *object.Bool:
		return []any{obj.Value()}, nil
	case *object.Time:
		return []any{obj.Value()}, nil
	case *object.List:
		var values []any
		for _, item := range obj.Value() {
			value, err := convertRisorEachValue(item)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	case *object.Set:
		var values []any
		for _, item := range obj.Value() {
			value, err := convertRisorEachValue(item)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	case *object.Map:
		var values []any
		for _, item := range obj.Value() {
			value, err := convertRisorEachValue(item)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unsupported risor result type: %T", obj)
	}
}

// compileScript compiles a risor script with the given globals
func compileScript(ctx context.Context, code string, globals map[string]any) (*compiler.Code, error) {
	ast, err := parser.Parse(ctx, code)
	if err != nil {
		return nil, err
	}

	var globalNames []string
	for name := range globals {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)

	return compiler.Compile(ast, compiler.WithGlobalNames(globalNames))
}
