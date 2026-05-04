// background_example demonstrates background task execution using the
// ContinueWithBackground convenience loop.
//
// A "compile" tool dispatches a goroutine and returns immediately. The agent
// substitutes a "started" message to the LLM and the caller loops until all
// tasks complete.
//
// Run: cd examples && go run ./background_example
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

type CompileInput struct {
	Target string `json:"target" description:"Build target (e.g. './cmd/server')"`
}

func main() {
	ctx := context.Background()

	compileTool := dive.FuncTool("compile",
		"Compiles the given Go target. Runs in the background; reports success or failure when done.",
		func(ctx context.Context, in *CompileInput) (*dive.ToolResult, error) {
			return dive.NewBackgroundResult(ctx, fmt.Sprintf("compiling %s", in.Target),
				func(ctx context.Context) (string, error) {
					// Simulate a 2-second build.
					select {
					case <-time.After(2 * time.Second):
					case <-ctx.Done():
						return "", ctx.Err()
					}
					return fmt.Sprintf("Build successful: %s (3 packages, 0 warnings)", in.Target), nil
				}), nil
		})

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: "You are a build assistant. Use the compile tool when asked to build targets.",
		Model:        anthropic.New(),
		Tools:        []dive.Tool{compileTool},
		Session:      session.New("background-demo"),
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Asking agent to compile...")
	resp, err := agent.CreateResponse(ctx, dive.WithInput("Please compile ./cmd/server"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Agent: %s\n\n", resp.OutputText())

	// Loop until all background tasks have been delivered back to the agent.
	for len(resp.BackgroundTasks) > 0 {
		fmt.Printf("Waiting for %d background task(s) to complete...\n", len(resp.BackgroundTasks))
		resp, err = dive.ContinueWithBackground(ctx, agent, resp)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Agent: %s\n", resp.OutputText())
	}
}
