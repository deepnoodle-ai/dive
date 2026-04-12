// a2a_example shows how to expose a Dive agent as a remote A2A agent and
// how to call a remote A2A agent from Go code. The example starts an HTTP
// server in-process, serves /.well-known/agent.json, then issues a
// message/send call against the server via the A2A client wrapper.
//
// Run:
//
//	cd examples && go run ./a2a_example
//
// Environment:
//
//	ANTHROPIC_API_KEY — required unless you swap in a different provider.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/a2a"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/session"
)

func main() {
	ctx := context.Background()

	// 1. Build a plain Dive agent. The A2A adapter does not require any
	//    special configuration on the agent itself.
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Research Assistant",
		SystemPrompt: "You are an enthusiastic and deeply curious researcher. Answer concisely.",
		Model:        anthropic.New(),
	})
	if err != nil {
		log.Fatal(err)
	}

	// 2. Wrap it in an A2A server. The SessionProvider hands out a
	//    dedicated in-memory session per contextId so follow-up messages
	//    from the same remote caller resume the same conversation.
	sessions := map[string]dive.Session{}
	provider := func(ctx context.Context, contextID string) (dive.Session, error) {
		if sess, ok := sessions[contextID]; ok {
			return sess, nil
		}
		sess := session.New(contextID)
		sessions[contextID] = sess
		return sess, nil
	}

	server, err := a2a.NewServer(a2a.ServerOptions{
		Agent:           agent,
		BaseURL:         "http://127.0.0.1", // the httptest server will overwrite this in the card below
		SessionProvider: provider,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 3. Stand the server up on a local ephemeral port.
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// 4. Fetch the agent card from the well-known URL to demonstrate
	//    discovery.
	resp, err := http.Get(ts.URL + a2a.DefaultAgentCardPath)
	if err != nil {
		log.Fatal(err)
	}
	resp.Body.Close()
	fmt.Printf("GET %s%s -> %s\n", ts.URL, a2a.DefaultAgentCardPath, resp.Status)

	// 5. Call the agent as a remote A2A client. This is the client-side
	//    flow a different service would use to talk to our agent.
	client, err := a2a.NewClient(a2a.ClientOptions{Endpoint: ts.URL + "/"})
	if err != nil {
		log.Fatal(err)
	}
	remote := a2a.NewRemoteAgent(client)

	card, err := remote.Card(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Remote agent: %s (streaming=%v)\n", card.Name, card.Capabilities.Streaming)

	task, err := remote.SendText(ctx, "What is the capital of France?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Task state: %s\n", task.Status.State)
	fmt.Printf("Response:   %s\n", a2a.ResponseText(task))

	// 6. Follow-up on the same contextId to see the server resume the
	//    same Dive session rather than starting a fresh one.
	task2, err := remote.SendText(ctx, "And what about Italy?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Follow-up response: %s\n", a2a.ResponseText(task2))
}
