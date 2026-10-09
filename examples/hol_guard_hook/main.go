// hol_guard_hook shows how to protect a Dive tool with HOL Guard before the
// tool executes. Any missing, invalid, unavailable, review-required, or denied
// Guard result fails closed.
//
// Install and initialize HOL Guard first:
//
//	pipx install hol-guard
//	hol-guard init
//
// The example also requires ANTHROPIC_API_KEY.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
)

type shellInput struct {
	Command string `json:"command" description:"The shell command to run"`
}

func main() {
	ctx := context.Background()

	shellTool := dive.FuncTool("run_shell",
		"Runs a shell command and returns its output.",
		func(ctx context.Context, in *shellInput) (*dive.ToolResult, error) {
			// Keep the example side-effect free. The Guard hook still sees the exact
			// command and decides whether this tool would be allowed to run.
			return dive.NewToolResultText(fmt.Sprintf("(demo) ran: %s", in.Command)), nil
		})

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Guarded Assistant",
		SystemPrompt: "Use run_shell for shell work.",
		Model:        anthropic.New(),
		Tools:        []dive.Tool{shellTool},
		Hooks: dive.Hooks{
			PreToolUse: []dive.PreToolUseHook{
				dive.MatchTool("run_shell", holGuardPreToolUse(runHOLGuard)),
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := agent.CreateResponse(ctx, dive.WithInput(
		"Use run_shell to inspect the current Go version with `go version`."))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.OutputText())
}
