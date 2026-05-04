// Package a2alib provides an A2A (Agent-to-Agent) server adapter for Dive
// agents using the official a2a-go SDK (github.com/a2aproject/a2a-go/v2).
//
// It bridges Dive's Agent runtime with the a2a-go server framework: the
// [Executor] translates between Dive's CreateResponse flow and the a2a-go
// event iterator model, while a2a-go handles transport (JSON-RPC, REST),
// task persistence, streaming, and agent card serving.
//
// # Quick start
//
// Expose a Dive agent as an A2A server:
//
//	srv, err := a2alib.NewServer(a2alib.ServerOptions{
//	    Agent:   agent,
//	    BaseURL: "https://my-agent.example.com",
//	    Card: a2a.AgentCard{
//	        Name:        "My Agent",
//	        Description: "Does useful things.",
//	    },
//	})
//	http.ListenAndServe(":8080", srv.Handler())
//
// Call a remote A2A agent from Go code:
//
//	remote, err := a2alib.NewRemoteAgentFromCard(ctx, card)
//	task, err := remote.SendText(ctx, "What is the capital of France?")
//	fmt.Println(a2alib.ResponseText(task))
package a2alib
