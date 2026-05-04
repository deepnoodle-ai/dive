// a2alib_example shows how to expose a Dive agent as an A2A server using the
// official a2a-go SDK, and how to call it from Go code with RemoteAgent.
//
// The example starts an in-process HTTP server, issues a SendMessage call
// against it, then demonstrates the suspend/resume flow where an agent pauses
// to ask for human approval before continuing.
//
// Run:
//
//	cd examples && go run ./a2alib_example
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
	"github.com/deepnoodle-ai/dive/a2alib"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/session"

	"github.com/a2aproject/a2a-go/v2/a2a"
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

	srv, err := a2alib.NewServer(a2alib.ServerOptions{
		Agent:           agent,
		SessionProvider: sessionProvider,
		Card: a2a.AgentCard{
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
	card := srv.Card()
	card.SupportedInterfaces = []*a2a.AgentInterface{{
		URL:             ts.URL,
		ProtocolBinding: a2a.TransportProtocolJSONRPC,
		ProtocolVersion: a2a.Version,
	}}

	remote, err := a2alib.NewRemoteAgentFromCard(ctx, card)
	if err != nil {
		log.Fatal(err)
	}

	// --- Simple completion ---
	fmt.Println("\n--- Simple completion ---")
	task, err := remote.SendText(ctx, "What is the capital of France?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("State:    %s\n", task.Status.State)
	fmt.Printf("Response: %s\n", a2alib.ResponseText(task))

	// --- Suspend / resume flow ---
	fmt.Println("\n--- Suspend and resume ---")
	remote2, err := a2alib.NewRemoteAgentFromCard(ctx, card)
	if err != nil {
		log.Fatal(err)
	}

	// This message should trigger the approval tool and suspend.
	task2, err := remote2.SendText(ctx, "Please delete all the log files.")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("State:  %s\n", task2.Status.State)
	if task2.Status.Message != nil {
		fmt.Printf("Prompt: %s\n", a2alib.ResponseText(task2))
	}

	if task2.Status.State == a2a.TaskStateInputRequired {
		// Resume by approving on the same task.
		task3, err := remote2.SendTextOnTask(ctx, task2.ID, "yes, approved")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("State:    %s\n", task3.Status.State)
		fmt.Printf("Response: %s\n", a2alib.ResponseText(task3))
	}
}
