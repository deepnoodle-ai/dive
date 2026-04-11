// human_approval shows the agent suspending mid-turn so a human can approve
// a tool call from the terminal, then resuming with the user's answer.
//
// The deploy tool packs a dialogspec.Spec (kind=confirm, title, message)
// into the SuspendResult's metadata. The caller rebuilds a dive.DialogInput
// from it, prompts via dive.NewTerminalDialog(), and resumes the agent with
// WithToolResults.
//
// Run: cd examples && go run ./suspend/human_approval
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/examples/suspend/dialogspec"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/session"
)

type DeployInput struct {
	Environment string `json:"environment" description:"Target environment (staging or production)"`
	Version     string `json:"version" description:"Version tag to deploy"`
}

func deployTool() dive.Tool {
	return dive.FuncTool("deploy",
		"Deploys the application to the given environment. Requires human approval.",
		func(ctx context.Context, in *DeployInput) (*dive.ToolResult, error) {
			return dialogspec.NewSuspend(dialogspec.Spec{
				Kind:    dialogspec.KindConfirm,
				Title:   "Deployment approval",
				Message: fmt.Sprintf("About to deploy version %q to %q. Proceed?", in.Version, in.Environment),
			}), nil
		})
}

func main() {
	ctx := context.Background()
	dialog := dive.NewTerminalDialog()

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: "You are a release manager. When asked to deploy, call the deploy tool exactly once.",
		Model:        anthropic.New(),
		Tools:        []dive.Tool{deployTool()},
		Session:      session.New("human-approval-demo"),
	})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := agent.CreateResponse(ctx, dive.WithInput("Please deploy version v1.4.2 to production."))
	if err != nil {
		log.Fatal(err)
	}

	if resp.Status != dive.ResponseStatusSuspended {
		fmt.Println("Agent finished without suspending:", resp.OutputText())
		return
	}

	results := map[string]*dive.ToolResult{}
	for _, pending := range resp.PendingToolCalls {
		var args DeployInput
		_ = json.Unmarshal(pending.Input, &args)

		spec := dialogspec.FromPending(pending)
		out, err := dialog.Show(ctx, spec.ToDialogInput())
		if err != nil {
			log.Fatalf("dialog error: %v", err)
		}
		if out.Confirmed {
			results[pending.ID] = dive.NewToolResultText(
				fmt.Sprintf("Deploy of %s to %s approved and completed.", args.Version, args.Environment))
		} else {
			results[pending.ID] = dive.NewToolResultError(
				fmt.Sprintf("Deploy of %s to %s denied by operator.", args.Version, args.Environment))
		}
	}

	final, err := agent.CreateResponse(ctx, dive.WithToolResults(results))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\nAgent:", final.OutputText())
}
