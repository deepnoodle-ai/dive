// Package a2a provides an A2A (Agent-to-Agent) server adapter and remote
// agent client for Dive, built on the official a2a-go SDK
// (github.com/a2aproject/a2a-go/v2).
//
// The [Executor] bridges Dive's Agent runtime to the a2a-go server framework,
// translating between Dive's CreateResponse flow and the a2a-go event
// iterator model. The a2a-go SDK handles transport (JSON-RPC, REST), task
// persistence, streaming, and agent card serving.
//
// # Exposing a Dive agent as an A2A server
//
//	srv, err := a2a.NewServer(a2a.ServerOptions{
//	    Agent:   agent,
//	    BaseURL: "https://my-agent.example.com",
//	    Card: a2a.CardOptions{
//	        Name:        "My Agent",
//	        Description: "Does useful things.",
//	    },
//	})
//	http.ListenAndServe(":8080", srv.Handler())
//
// # Calling a remote A2A agent
//
//	remote, err := a2a.NewRemoteAgentFromURL(ctx, "https://my-agent.example.com")
//	result, err := remote.SendText(ctx, "What is the capital of France?")
//	fmt.Println(result.Text)
//
// # Suspend and resume
//
//	result, err := remote.SendText(ctx, "Please delete all log files.")
//	if result.IsInputRequired() {
//	    result, err = remote.SendTextOnTask(ctx, result.ID, "yes, approved")
//	}
package a2a
