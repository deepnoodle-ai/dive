// human_approval shows the simplest suspend/resume pattern: a tool suspends
// mid-turn so a human can approve the action from the terminal, then the
// agent resumes with the user's answer.
//
// Run: cd examples && go run ./suspend/human_approval
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/session"
)

type DeployInput struct {
	Environment string `json:"environment" description:"Target environment (staging or production)"`
	Version     string `json:"version" description:"Version tag to deploy"`
}

func main() {
	ctx := context.Background()

	deployTool := dive.FuncTool("deploy",
		"Deploys the application to the given environment. Requires human approval.",
		func(ctx context.Context, in *DeployInput) (*dive.ToolResult, error) {
			prompt := fmt.Sprintf("Deploy %s to %s?", in.Version, in.Environment)
			return dive.NewSuspendResult(prompt, nil), nil
		})

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: "You are a release manager. When asked to deploy, use the deploy tool.",
		Model:        anthropic.New(),
		Tools:        []dive.Tool{deployTool},
		Session:      session.New("human-approval-demo"),
	})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := agent.CreateResponse(ctx, dive.WithInput("Deploy v1.4.2 to production."))
	if err != nil {
		log.Fatal(err)
	}
	if resp.Status != dive.ResponseStatusSuspended {
		fmt.Println("Agent finished without suspending:", resp.OutputText())
		return
	}

	// Show each pending tool call's prompt and ask for confirmation.
	dialog := dive.NewTerminalDialog()
	results := map[string]*dive.ToolResult{}
	for _, pending := range resp.Suspension.PendingToolCalls {
		out, err := dialog.Show(ctx, &dive.DialogInput{
			Title:   "Deployment approval",
			Message: pending.Prompt,
			Confirm: true,
		})
		if err != nil {
			log.Fatal(err)
		}
		if out.Confirmed {
			results[pending.ID] = dive.NewToolResultText("Approved and deployed.")
		} else {
			results[pending.ID] = dive.NewToolResultError("Denied by operator.")
		}
	}

	final, err := agent.CreateResponse(ctx, dive.WithToolResults(results))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\nAgent:", final.OutputText())
}
