// a2alib_example shows how to expose a Dive agent as an A2A server using the
// official a2a-go SDK, and how to call it from Go code with RemoteAgent.
//
// The example starts an in-process HTTP server, issues a SendText call
// against it, then demonstrates the suspend/resume flow where an agent pauses
// to ask for human approval before continuing.
//
// Run:
//
//	cd examples && go run ./a2alib_example/...
//
// Environment:
//
//	ANTHROPIC_API_KEY — required unless you swap in a different provider.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http/httptest"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/a2a"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/session"
)

func main() {
	ctx := context.Background()

	// A toy tool that suspends the agent to ask for human approval.
	type approveIn struct {
		Action string `json:"action" description:"action to approve"`
	}
	approve := dive.FuncTool("request_approval",
		"Pause and request human approval before taking an action",
		func(ctx context.Context, in *approveIn) (*dive.ToolResult, error) {
			return dive.NewSuspendResult(
				fmt.Sprintf("Approve action: %q?", in.Action),
				map[string]any{"action": in.Action},
			), nil
		},
	)

	// Build a Dive agent with the approval tool.
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Cautious Assistant",
		SystemPrompt: "You are a careful assistant. For any consequential action, use request_approval before proceeding.",
		Model:        anthropic.New(),
		Tools:        []dive.Tool{approve},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Wrap the agent in an A2A server. Sessions are keyed by A2A contextID
	// so follow-up messages from the same caller resume the same conversation.
	var sessionsMu sync.Mutex
	sessions := map[string]dive.Session{}
	sessionProvider := func(ctx context.Context, contextID string) (dive.Session, error) {
		sessionsMu.Lock()
		defer sessionsMu.Unlock()
		if sess, ok := sessions[contextID]; ok {
			return sess, nil
		}
		sess := session.New(contextID)
		sessions[contextID] = sess
		return sess, nil
	}

	srv, err := a2a.NewServer(a2a.ServerOptions{
		Agent:           agent,
		SessionProvider: sessionProvider,
		Card: a2a.CardOptions{
			Name:        "Cautious Assistant",
			Description: "An assistant that pauses before taking consequential actions.",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Stand the server up on a local ephemeral port.
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	fmt.Printf("A2A server listening at %s\n", ts.URL)

	// Build a RemoteAgent pointed at the test server.
	remote, err := a2a.NewRemoteAgentFromURL(ctx, ts.URL)
	if err != nil {
		log.Fatal(err)
	}

	// --- Simple completion ---
	fmt.Println("\n--- Simple completion ---")
	result, err := remote.SendText(ctx, "What is the capital of France?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("State:    %s\n", result.State)
	fmt.Printf("Response: %s\n", result.Text)

	// --- Suspend / resume flow ---
	fmt.Println("\n--- Suspend and resume ---")
	remote2, err := a2a.NewRemoteAgentFromURL(ctx, ts.URL)
	if err != nil {
		log.Fatal(err)
	}

	// This message should trigger the approval tool and suspend.
	result2, err := remote2.SendText(ctx, "Please delete all the log files.")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("State:  %s\n", result2.State)
	fmt.Printf("Prompt: %s\n", result2.Text)

	if result2.IsInputRequired() {
		// Resume by approving on the same task.
		result3, err := remote2.SendTextOnTask(ctx, result2.ID, "yes, approved")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("State:    %s\n", result3.State)
		fmt.Printf("Response: %s\n", result3.Text)
	}
}
