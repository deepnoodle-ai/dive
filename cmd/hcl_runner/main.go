package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/slogger"
	"github.com/zclconf/go-cty/cty"
)

func main() {
	// Define command-line flags
	filePath := flag.String("file", "", "Path to the HCL team definition file")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	varsFlag := flag.String("vars", "", "Comma-separated list of variables in format key=value")

	flag.Parse()

	// Check if file path is provided
	if *filePath == "" {
		fmt.Println("Error: file path is required")
		flag.Usage()
		os.Exit(1)
	}

	// Parse variables
	vars := dive.VariableValues{}
	if *varsFlag != "" {
		varPairs := strings.Split(*varsFlag, ",")
		for _, pair := range varPairs {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				fmt.Printf("Error: invalid variable format: %s\n", pair)
				os.Exit(1)
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			vars[key] = cty.StringVal(value)
		}
	}

	// Create context
	ctx := context.Background()

	// Create logger
	logger := slogger.New(slogger.LevelFromString("debug"))

	team, tasks, err := dive.LoadHCLTeam(ctx, *filePath, vars, logger)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("team loaded", team.Name())

	if err := team.Start(ctx); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	defer team.Stop(ctx)

	fmt.Println("team started")

	stream, err := team.Work(ctx, tasks...)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("team working")

	// Collect all results from the stream
	var results []*dive.TaskResult
	for result := range stream.Results() {
		results = append(results, result)
	}

	fmt.Println("team done")

	// Print results
	fmt.Println("\nResults:")
	for _, result := range results {
		fmt.Printf("Task: %s\n", result.Task.Name())
		if *verbose {
			fmt.Printf("Started: %s\n", result.StartedAt.Format(time.RFC3339))
			fmt.Printf("Finished: %s\n", result.FinishedAt.Format(time.RFC3339))
			fmt.Printf("Error: %v\n", result.Error)
		}
		fmt.Printf("Output: %s\n\n", result.Content)
	}
}
