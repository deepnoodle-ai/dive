// async_webhook shows suspend/resume across process restarts using FileStore.
//
// Run twice against the same session ID:
//
//	cd examples
//	go run ./suspend/async_webhook -mode=suspend
//	go run ./suspend/async_webhook -mode=resume
//
// "suspend" sends a new request, the send_email tool returns SuspendResult
// with a dialogspec.Spec describing the pending interaction, an OnSuspend
// hook logs what a webhook dispatch would look like, and the partial turn
// is persisted to ./async_webhook_sessions/. "resume" reopens the same
// session from disk and supplies the tool result — simulating a webhook
// callback arriving minutes or days later.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/examples/suspend/dialogspec"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/session"
)

type EmailInput struct {
	To      string `json:"to" description:"Recipient email address"`
	Subject string `json:"subject" description:"Email subject"`
	Body    string `json:"body" description:"Email body"`
}

func sendEmailTool() dive.Tool {
	return dive.FuncTool("send_email",
		"Sends an email. Delivery is asynchronous; the tool suspends until the provider acknowledges via webhook.",
		func(ctx context.Context, in *EmailInput) (*dive.ToolResult, error) {
			// Kind is left empty: no user dialog, just an async wait.
			return dialogspec.NewSuspend(dialogspec.Spec{
				Title:   "Email delivery pending",
				Message: fmt.Sprintf("Awaiting delivery webhook for email to %s (subject: %q).", in.To, in.Subject),
			}), nil
		})
}

func webhookNotifier(ctx context.Context, hctx *dive.HookContext) error {
	for _, p := range hctx.Response.PendingToolCalls {
		payload, _ := json.MarshalIndent(map[string]any{
			"webhook_url": "https://example.com/webhooks/tool-result",
			"tool_call":   p,
			"spec":        dialogspec.FromPending(p),
		}, "", "  ")
		fmt.Printf("\n[OnSuspend] would POST:\n%s\n\n", payload)
	}
	return nil
}

func main() {
	mode := flag.String("mode", "suspend", "suspend | resume")
	sessionID := flag.String("session", "demo-email-1", "session id")
	flag.Parse()

	ctx := context.Background()
	store, err := session.NewFileStore("./async_webhook_sessions")
	if err != nil {
		log.Fatal(err)
	}
	sess, err := store.Open(ctx, *sessionID)
	if err != nil {
		log.Fatal(err)
	}

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: "You are an ops bot. Use send_email to notify users.",
		Model:        anthropic.New(),
		Tools:        []dive.Tool{sendEmailTool()},
		Session:      sess,
		Hooks:        dive.Hooks{OnSuspend: []dive.OnSuspendHook{webhookNotifier}},
	})
	if err != nil {
		log.Fatal(err)
	}

	switch *mode {
	case "suspend":
		if sess.Suspended() {
			log.Fatalf("session %q is already suspended — run with -mode=resume", *sessionID)
		}
		resp, err := agent.CreateResponse(ctx,
			dive.WithInput("Email alice@example.com subject 'Nightly report' body 'All jobs green.'"))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("status=%s pending=%d (re-run with -mode=resume)\n", resp.Status, len(resp.PendingToolCalls))
	case "resume":
		if !sess.Suspended() {
			log.Fatalf("session %q is not suspended — run with -mode=suspend first", *sessionID)
		}
		results := map[string]*dive.ToolResult{}
		for _, p := range sess.PendingCalls() {
			fmt.Printf("[webhook callback] delivering result for %s (%s)\n", p.ID, p.Prompt)
			results[p.ID] = dive.NewToolResultText("Email delivered successfully (message-id: msg_" + p.ID + ")")
		}
		resp, err := agent.CreateResponse(ctx, dive.WithToolResults(results))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("\nAgent:", resp.OutputText())
	default:
		log.Fatalf("unknown mode %q", *mode)
	}
}
