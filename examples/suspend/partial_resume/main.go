// partial_resume shows a single suspend that holds multiple pending tool
// calls, then resumes them one at a time. After each partial resume the agent
// re-suspends with a shrinking PendingToolCalls list until the final result
// lands and the turn completes.
//
// Each notify_team call attaches a dialogspec.Spec describing the wait, and
// the caller renders a terminal select dialog for each pending call so you
// can choose how to ack it (ok / escalate / ignore). The dialog input round-
// trips through the suspend as metadata.
//
// Requires Anthropic parallel tool_use: the system prompt strongly nudges the
// model to call notify_team three times in one assistant turn.
//
// Run: cd examples && go run ./suspend/partial_resume
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/examples/suspend/dialogspec"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/session"
)

type NotifyInput struct {
	Team    string `json:"team" description:"Team to notify (alpha, beta, or gamma)"`
	Message string `json:"message" description:"Message to send"`
}

func notifyTool() dive.Tool {
	return dive.FuncTool("notify_team",
		"Notifies a team. Delivery is external; suspends until acknowledged.",
		func(ctx context.Context, in *NotifyInput) (*dive.ToolResult, error) {
			return dialogspec.NewSuspend(dialogspec.Spec{
				Kind:    dialogspec.KindSelect,
				Title:   fmt.Sprintf("Ack from %s team", in.Team),
				Message: fmt.Sprintf("Message sent to %s: %q. How should it be acknowledged?", in.Team, in.Message),
				Default: "ok",
				Options: []dialogspec.Option{
					{Value: "ok", Label: "Acknowledged"},
					{Value: "escalate", Label: "Escalate", Description: "Page the on-call lead"},
					{Value: "ignore", Label: "Ignore", Description: "Team did not respond"},
				},
			}), nil
		})
}

func teamFor(p *dive.PendingToolCall) string {
	var in NotifyInput
	_ = p.UnmarshalInput(&in)
	return in.Team
}

func main() {
	ctx := context.Background()
	dialog := dive.NewTerminalDialog()

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: "You are an incident commander. When asked to notify teams, " +
			"emit a SINGLE assistant message that calls notify_team once for EACH team " +
			"in parallel (do not serialize them).",
		Model:                 anthropic.New(),
		Tools:                 []dive.Tool{notifyTool()},
		Session:               session.New("partial-resume-demo"),
		ParallelToolExecution: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := agent.CreateResponse(ctx, dive.WithInput(
		"Notify teams alpha, beta, and gamma in parallel that the deploy is complete."))
	if err != nil {
		log.Fatal(err)
	}

	if resp.Status != dive.ResponseStatusSuspended {
		fmt.Println("Agent completed without suspending:", resp.OutputText())
		return
	}
	fmt.Printf("Initial suspend: %d pending tool call(s)\n", len(resp.Suspension.PendingToolCalls))

	// Resume one at a time. After each call the agent stays suspended
	// (Status=Suspended, Suspension.PendingToolCalls shrinks by one) until
	// the last result lands and the turn completes.
	for resp.Status == dive.ResponseStatusSuspended && len(resp.Suspension.PendingToolCalls) > 0 {
		next := resp.Suspension.PendingToolCalls[0]
		team := teamFor(next)
		fmt.Printf("\nResuming %s (team=%s) — %d remaining before\n", next.ID, team, len(resp.Suspension.PendingToolCalls))

		out, err := dialog.Show(ctx, dialogspec.FromPending(next).ToDialogInput())
		if err != nil {
			log.Fatalf("dialog error: %v", err)
		}
		choice := "ok"
		if len(out.Values) > 0 {
			choice = out.Values[0]
		}
		resp, err = agent.CreateResponse(ctx, dive.WithToolResults(map[string]*dive.ToolResult{
			next.ID: dive.NewToolResultText(fmt.Sprintf("%s team: %s", team, choice)),
		}))
		if err != nil {
			log.Fatal(err)
		}
		remaining := 0
		if resp.Suspension != nil {
			remaining = len(resp.Suspension.PendingToolCalls)
		}
		fmt.Printf("  -> status=%s remaining=%d\n", resp.Status, remaining)
	}

	fmt.Println("\nAgent:", resp.OutputText())
}
