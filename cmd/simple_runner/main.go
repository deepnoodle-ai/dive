package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/diveagents/dive/config"
	"github.com/diveagents/dive/environment"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/fatih/color"
)

var (
	errorStyle = color.New(color.FgRed)
)

func fatal(msg string, args ...interface{}) {
	fmt.Printf(errorStyle.Sprint(msg)+"\n", args...)
	os.Exit(1)
}

func main() {
	var varsFlag, workflowName string
	flag.StringVar(&varsFlag, "var", "", "Comma-separated list of variables in format key=value")
	flag.StringVar(&workflowName, "workflow", "", "Workflow name")
	flag.Parse()

	if flag.NArg() == 0 {
		fatal("Error: file path is required")
	}

	filePath := flag.Arg(0)

	vars := map[string]any{}
	if varsFlag != "" {
		varPairs := strings.Split(varsFlag, ",")
		for _, pair := range varPairs {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				fatal("Error: invalid variable format: %s", pair)
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			vars[key] = value
		}
	}

	logger := slogger.New(slogger.LevelDebug)

	ctx := context.Background()

	// Load the environment
	env, err := config.LoadDirectory(filePath, config.WithLogger(logger))
	if err != nil {
		fatal(err.Error())
	}
	if err := env.Start(ctx); err != nil {
		fatal(err.Error())
	}

	// Choose the workflow to run
	var w *workflow.Workflow
	if workflowName != "" {
		w, err = env.GetWorkflow(workflowName)
		if err != nil {
			fatal(err.Error())
		}
	} else {
		workflows := env.Workflows()
		if len(workflows) != 1 {
			fatal("You must specify a workflow name")
		}
		w = workflows[0]
	}

	// Start the workflow
	execution, err := environment.NewEventBasedExecution(env, environment.ExecutionOptions{
		WorkflowName:   w.Name(),
		Inputs:         vars,
		EventStore:     workflow.NewNullEventStore(),
		EventBatchSize: 10,
	})
	if err != nil {
		fatal(err.Error())
	}

	// Wait for the workflow to complete
	if err := execution.Wait(); err != nil {
		fatal(err.Error())
	}

	// Print the outputs
	outputs := execution.StepOutputs()
	for name, output := range outputs {
		if output != "" {
			fmt.Printf("%q output:\n\n%s\n\n", name, output)
		}
	}
}
