package environment

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

// LoadFromSnapshot loads execution state from a snapshot and event history
func LoadFromSnapshot(ctx context.Context, env *Environment, snapshot *ExecutionSnapshot, eventStore ExecutionEventStore) (*Execution, error) {
	// Verify the workflow exists
	if _, exists := env.workflows[snapshot.WorkflowName]; !exists {
		return nil, fmt.Errorf("workflow not found: %s", snapshot.WorkflowName)
	}

	exec, err := NewExecution(ExecutionOptions{
		Environment: env,
		Workflow:    env.workflows[snapshot.WorkflowName],
		Inputs:      snapshot.Inputs,
		EventStore:  eventStore,
		Logger:      env.logger,
	})
	if err != nil {
		return nil, err
	}

	// Restore state from snapshot
	exec.id = snapshot.ID
	exec.status = ExecutionStatus(snapshot.Status)
	exec.startTime = snapshot.StartTime
	exec.endTime = snapshot.EndTime
	exec.inputs = snapshot.Inputs
	exec.outputs = snapshot.Outputs
	// exec.eventSequence = snapshot.LastEventSeq

	if snapshot.Error != "" {
		exec.err = fmt.Errorf(snapshot.Error)
	}
	return exec, nil
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
