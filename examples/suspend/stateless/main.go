// stateless shows suspend/resume with no Session at all.
//
// The caller manages the message history and the SuspensionState
// themselves, serializing both to disk between runs. This mirrors the
// async_webhook example structurally, but replaces FileStore with a
// plain JSON file holding the history and state, proving that Dive's
// suspend/resume works without buying into the Session abstraction.
//
// Run twice against the same state file:
//
//	cd examples
//	go run ./suspend/stateless -mode=suspend
//	go run ./suspend/stateless -mode=resume
//
// Use -state path/to/state.json to change the state file location.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/examples/suspend/dialogspec"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
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
			return dialogspec.NewSuspend(dialogspec.Spec{
				Title:   "Email delivery pending",
				Message: fmt.Sprintf("Awaiting delivery webhook for email to %s (subject: %q).", in.To, in.Subject),
			}), nil
		})
}

// savedState is everything the stateless caller tracks between calls.
// It's the exact data a session would store on the caller's behalf —
// persisted here as plain JSON to make it concrete.
type savedState struct {
	Messages   []*llm.Message         `json:"messages"`
	Suspension *dive.SuspensionState  `json:"suspension,omitempty"`
}

func loadState(path string) (*savedState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s savedState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func writeState(path string, s *savedState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func main() {
	var (
		mode      string
		statePath string
	)
	flag.StringVar(&mode, "mode", "suspend", "suspend | resume")
	flag.StringVar(&statePath, "state", "./stateless_state.json", "path to saved state file")
	flag.Parse()

	ctx := context.Background()

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: "You are an ops bot. Use send_email to notify users.",
		Model:        anthropic.New(),
		Tools:        []dive.Tool{sendEmailTool()},
		// Deliberately no Session: we manage history ourselves.
	})
	if err != nil {
		log.Fatal(err)
	}

	switch mode {
	case "suspend":
		if _, err := os.Stat(statePath); err == nil {
			log.Fatalf("state file %q already exists — run with -mode=resume or delete it", statePath)
		}

		history := []*llm.Message{
			llm.NewUserTextMessage("Email alice@example.com subject 'Nightly report' body 'All jobs green.'"),
		}
		resp, err := agent.CreateResponse(ctx, dive.WithMessages(history...))
		if err != nil {
			log.Fatal(err)
		}
		if resp.Status != dive.ResponseStatusSuspended {
			fmt.Println("Agent completed without suspending:", resp.OutputText())
			return
		}

		// Save (history ++ OutputMessages) and the suspension state.
		history = append(history, resp.OutputMessages...)
		s := &savedState{Messages: history, Suspension: resp.Suspension}
		if err := writeState(statePath, s); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("suspended, %d pending tool call(s), state saved to %s\n",
			len(resp.Suspension.PendingToolCalls), statePath)
		fmt.Println("Re-run with -mode=resume to deliver the tool result.")

	case "resume":
		s, err := loadState(statePath)
		if err != nil {
			log.Fatalf("load state: %v", err)
		}
		if s.Suspension == nil {
			log.Fatalf("state file %q has no suspension — nothing to resume", statePath)
		}

		results := map[string]*dive.ToolResult{}
		for _, p := range s.Suspension.PendingToolCalls {
			fmt.Printf("[webhook callback] delivering result for %s (%s)\n", p.ID, p.Prompt)
			results[p.ID] = dive.NewToolResultText("Email delivered successfully (message-id: msg_" + p.ID + ")")
		}

		resp, err := agent.CreateResponse(ctx,
			dive.WithMessages(s.Messages...),
			dive.WithSuspension(s.Suspension),
			dive.WithToolResults(results),
		)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("\nAgent:", resp.OutputText())

		// Clean up the state file on success.
		_ = os.Remove(statePath)

	default:
		log.Fatalf("unknown mode %q", mode)
	}
}
