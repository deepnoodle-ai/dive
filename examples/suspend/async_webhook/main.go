// async_webhook shows suspend/resume across process restarts using FileStore.
//
// A send_email tool suspends the agent until an external webhook confirms
// delivery. The partial turn is persisted to disk. A second invocation
// reopens the session, supplies the tool result, and lets the agent finish.
//
// Run twice against the same session:
//
//	cd examples
//	go run ./suspend/async_webhook -mode=suspend
//	go run ./suspend/async_webhook -mode=resume
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/session"
)

type EmailInput struct {
	To      string `json:"to" description:"Recipient email address"`
	Subject string `json:"subject" description:"Email subject"`
	Body    string `json:"body" description:"Email body"`
}

func main() {
	mode := flag.String("mode", "suspend", "suspend | resume")
	sessionID := flag.String("session", "demo-email-1", "session ID")
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

	emailTool := dive.FuncTool("send_email",
		"Sends an email. Suspends until the provider acknowledges delivery.",
		func(ctx context.Context, in *EmailInput) (*dive.ToolResult, error) {
			prompt := fmt.Sprintf("Awaiting delivery confirmation for email to %s.", in.To)
			return dive.NewSuspendResult(prompt, nil), nil
		})

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: "You are an ops bot. Use send_email to notify users.",
		Model:        anthropic.New(),
		Tools:        []dive.Tool{emailTool},
		Session:      sess,
	})
	if err != nil {
		log.Fatal(err)
	}

	switch *mode {
	case "suspend":
		if sess.LoadSuspension() != nil {
			log.Fatalf("session %q already suspended — run with -mode=resume", *sessionID)
		}
		resp, err := agent.CreateResponse(ctx,
			dive.WithInput("Email alice@example.com subject 'Nightly report' body 'All jobs green.'"))
		if err != nil {
			log.Fatal(err)
		}
		pending := 0
		if resp.Suspension != nil {
			pending = len(resp.Suspension.PendingToolCalls)
		}
		fmt.Printf("status=%s pending=%d (re-run with -mode=resume)\n", resp.Status, pending)

	case "resume":
		state := sess.LoadSuspension()
		if state == nil {
			log.Fatalf("session %q is not suspended — run -mode=suspend first", *sessionID)
		}
		results := map[string]*dive.ToolResult{}
		for _, p := range state.PendingToolCalls {
			fmt.Printf("[webhook] delivering result for tool call %s\n", p.ID)
			results[p.ID] = dive.NewToolResultText("Email delivered (message-id: msg_" + p.ID + ")")
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
