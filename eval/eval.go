package eval

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/risor-io/risor"
	"github.com/risor-io/risor/compiler"
	"github.com/risor-io/risor/object"
	"github.com/risor-io/risor/parser"
)

type Expr struct {
	raw   string
	parts []string
	codes []*compiler.Code
}

func Compile(raw string, globals map[string]any) (*Expr, error) {
	e := &Expr{
		raw: raw,
	}

	// First validate that all ${...} expressions are properly closed
	openCount := strings.Count(raw, "${")
	closeCount := strings.Count(raw, "}")
	if openCount > closeCount {
		return nil, fmt.Errorf("unclosed template expression in string: %q", raw)
	}

	if openCount == 0 {
		// No template variables, just return the raw string
		return e, nil
	}

	// Compile all ${...} expressions into Risor code
	re := regexp.MustCompile(`\${([^}]+)}`)
	matches := re.FindAllStringSubmatchIndex(raw, -1)

	if len(matches) == 0 {
		// No template variables, just return the raw string
		return e, nil
	}

	// Get sorted list of global names for compiler
	var globalNames []string
	for name := range globals {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)

	var lastEnd int
	var parts []string
	var codes []*compiler.Code
	for _, match := range matches {
		// Add the text before the match
		if match[0] > lastEnd {
			parts = append(parts, raw[lastEnd:match[0]])
		}

		// Extract and compile the code inside ${...}
		expr := raw[match[2]:match[3]]
		ast, err := parser.Parse(context.Background(), expr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template expression %q: %w", expr, err)
		}

		code, err := compiler.Compile(ast, compiler.WithGlobalNames(globalNames))
		if err != nil {
			return nil, fmt.Errorf("failed to compile template expression %q: %w", expr, err)
		}

		codes = append(codes, code)
		parts = append(parts, "") // Placeholder for the evaluated result
		lastEnd = match[1]
	}

	// Add any remaining text after the last match
	if lastEnd < len(raw) {
		parts = append(parts, raw[lastEnd:])
	}

	return &Expr{
		raw:   raw,
		parts: parts,
		codes: codes,
	}, nil
}

func (e *Expr) Eval(ctx context.Context, globals map[string]any) (string, error) {
	if len(e.codes) == 0 {
		// No template variables, return the raw string
		return e.raw, nil
	}

	// Make a copy of parts since we'll modify it
	parts := make([]string, len(e.parts))
	copy(parts, e.parts)

	// Evaluate each code block and replace the corresponding placeholder
	for _, code := range e.codes {
		result, err := risor.EvalCode(ctx, code, risor.WithGlobals(globals))
		if err != nil {
			return "", fmt.Errorf("failed to evaluate template expression: %w", err)
		}

		// Convert the result to a string based on its type
		var strValue string
		switch v := result.(type) {
		case *object.String:
			strValue = v.Value()
		case *object.Int:
			strValue = fmt.Sprintf("%d", v.Value())
		case *object.Float:
			strValue = fmt.Sprintf("%g", v.Value())
		case *object.Bool:
			strValue = fmt.Sprintf("%t", v.Value())
		case *object.NilType:
			strValue = ""
		default:
			return "", fmt.Errorf("unsupported result type for template expression: %T", result)
		}

		// Find the next empty placeholder and replace it
		for j := range parts {
			if parts[j] == "" {
				parts[j] = strValue
				break
			}
		}
	}

	// Join all parts to create the final string
	return strings.Join(parts, ""), nil
}

func Eval(ctx context.Context, s string, globals map[string]any) (string, error) {
	expr, err := Compile(s, globals)
	if err != nil {
		return "", fmt.Errorf("failed to compile expression: %w", err)
	}
	return expr.Eval(ctx, globals)
}
