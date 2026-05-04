// background_polling_example demonstrates multiple concurrent background tasks
// using the lower-level AwaitBackgroundTasks + WithBackgroundResults API.
//
// Two health-check tools run concurrently. The caller awaits both and delivers
// all results in a single CreateResponse call.
//
// Run: cd examples && go run ./background_polling_example
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/session"
)

type HealthInput struct {
	Service string `json:"service" description:"Service name to check (e.g. 'api', 'database')"`
}

func main() {
	ctx := context.Background()

	healthTool := dive.FuncTool("check_health",
		"Checks the health of a named service. Returns results asynchronously.",
		func(ctx context.Context, in *HealthInput) (*dive.ToolResult, error) {
			// Simulate different latencies per service.
			delay := time.Second
			if in.Service == "database" {
				delay = 2 * time.Second
			}
			return dive.NewBackgroundResultFull(ctx, fmt.Sprintf("checking %s health", in.Service),
				func(ctx context.Context) *dive.ToolResult {
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return dive.NewToolResultError(ctx.Err().Error())
					}
					text := fmt.Sprintf("%s: healthy (latency %dms)", in.Service, delay.Milliseconds())
					display := fmt.Sprintf("✓ %s health check passed (%dms)", in.Service, delay.Milliseconds())
					return dive.NewToolResultText(text).WithDisplay(display)
				}), nil
		})

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: "You are an infrastructure monitor. Use check_health to verify services.",
		Model:        anthropic.New(),
		Tools:        []dive.Tool{healthTool},
		Session:      session.New("health-check-demo"),
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Asking agent to check services...")
	resp, err := agent.CreateResponse(ctx,
		dive.WithInput("Check the health of both the api and database services."))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Agent: %s\n\n", resp.OutputText())

	if len(resp.BackgroundTasks) == 0 {
		fmt.Println("No background tasks started.")
		return
	}

	fmt.Printf("Waiting for %d health check(s) to complete...\n", len(resp.BackgroundTasks))
	start := time.Now()
	results, err := dive.AwaitBackgroundTasks(ctx, resp.BackgroundTasks)
	if err != nil {
		log.Fatal(err)
	}
	elapsed := time.Since(start).Round(time.Millisecond)
	fmt.Printf("All checks completed in %s (ran concurrently)\n", elapsed)

	// Print the Display field for each result (richer than the LLM-facing text).
	for _, handle := range resp.BackgroundTasks {
		if r := results[handle.TaskID]; r != nil && r.Display != "" {
			fmt.Printf("  %s\n", r.Display)
		}
	}
	fmt.Println()

	// Deliver results to the agent for its final summary.
	final, err := agent.CreateResponse(ctx,
		dive.WithBackgroundResults(resp.BackgroundTasks, results))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Agent summary:\n%s\n", final.OutputText())
}
