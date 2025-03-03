package teamconf

import (
	"fmt"
	"os"
	"strings"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// createStandardFunctions creates a set of standard functions available in HCL
func createStandardFunctions() map[string]function.Function {
	return map[string]function.Function{
		"env": function.New(&function.Spec{
			Params: []function.Parameter{
				{
					Name: "name",
					Type: cty.String,
				},
			},
			Type: function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				name := args[0].AsString()
				value := os.Getenv(name)
				return cty.StringVal(value), nil
			},
		}),
		"concat": function.New(&function.Spec{
			Params: []function.Parameter{
				{
					Name:         "lists",
					Type:         cty.DynamicPseudoType,
					AllowUnknown: true,
					AllowNull:    true,
					AllowMarked:  true,
				},
			},
			VarParam: &function.Parameter{
				Name:         "lists",
				Type:         cty.DynamicPseudoType,
				AllowUnknown: true,
				AllowNull:    true,
				AllowMarked:  true,
			},
			Type: function.StaticReturnType(cty.DynamicPseudoType),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				if len(args) == 0 {
					return cty.ListValEmpty(cty.DynamicPseudoType), nil
				}

				// Determine the element type from the first argument
				firstArgType := args[0].Type()
				if !firstArgType.IsListType() && !firstArgType.IsTupleType() {
					return cty.NilVal, fmt.Errorf("all arguments must be lists or tuples")
				}

				var result []cty.Value
				for _, arg := range args {
					if !arg.Type().IsListType() && !arg.Type().IsTupleType() {
						return cty.NilVal, fmt.Errorf("all arguments must be lists or tuples")
					}
					for it := arg.ElementIterator(); it.Next(); {
						_, v := it.Element()
						result = append(result, v)
					}
				}

				if len(result) == 0 {
					return cty.ListValEmpty(cty.DynamicPseudoType), nil
				}

				return cty.ListVal(result), nil
			},
		}),
		"format": function.New(&function.Spec{
			Params: []function.Parameter{
				{
					Name: "format",
					Type: cty.String,
				},
			},
			VarParam: &function.Parameter{
				Name:      "args",
				Type:      cty.DynamicPseudoType,
				AllowNull: true,
			},
			Type: function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				format := args[0].AsString()
				formatArgs := make([]interface{}, len(args)-1)
				for i, arg := range args[1:] {
					switch {
					case arg.Type() == cty.String:
						formatArgs[i] = arg.AsString()
					case arg.Type() == cty.Number:
						formatArgs[i] = arg.AsBigFloat()
					case arg.Type() == cty.Bool:
						formatArgs[i] = arg.True()
					default:
						return cty.NilVal, fmt.Errorf("unsupported argument type %s", arg.Type().FriendlyName())
					}
				}
				return cty.StringVal(fmt.Sprintf(format, formatArgs...)), nil
			},
		}),
		"replace": function.New(&function.Spec{
			Params: []function.Parameter{
				{
					Name: "str",
					Type: cty.String,
				},
				{
					Name: "search",
					Type: cty.String,
				},
				{
					Name: "replace",
					Type: cty.String,
				},
			},
			Type: function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				str := args[0].AsString()
				search := args[1].AsString()
				replace := args[2].AsString()

				result := strings.ReplaceAll(str, search, replace)
				return cty.StringVal(result), nil
			},
		}),
	}
}
