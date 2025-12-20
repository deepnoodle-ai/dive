// Todo Tracking Example
//
// This example demonstrates how to track todo list progress in real-time
// using the TodoTracker helper and event callbacks.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/providers/openai"
	"github.com/deepnoodle-ai/dive/toolkit"
)

func main() {
	ctx := context.Background()

	// Create a TodoTracker to monitor progress
	tracker := dive.NewTodoTracker()

	// Create an agent with the TodoWrite tool
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name: "Task Manager",
		Instructions: `You are a task management assistant. When given a complex task:
1. Break it down into smaller steps using the TodoWrite tool
2. Update the status of each step as you work on it
3. Mark each step complete when done

Always use the TodoWrite tool to track your progress on multi-step tasks.`,
		Model: openai.New(),
		Tools: []dive.Tool{
			toolkit.NewTodoWriteTool(),
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating agent: %v\n", err)
		os.Exit(1)
	}

	// Create a response with an event callback that tracks todos
	_, err = agent.CreateResponse(ctx,
		dive.WithInput("Create a todo list for setting up a new Go project with testing"),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			// Track todo updates
			if err := tracker.HandleEvent(ctx, item); err != nil {
				return err
			}

			// Display progress when todos are updated
			if item.Type == dive.ResponseItemTypeTodo {
				fmt.Println("\n--- Todo List Updated ---")
				tracker.DisplayProgress(os.Stdout)
				fmt.Println()
			}

			return nil
		}),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Final summary
	completed, _, total := tracker.Progress()
	fmt.Printf("\nFinal: %d/%d tasks completed\n", completed, total)
}
