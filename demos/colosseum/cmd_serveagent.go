package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/a2a"
	"github.com/deepnoodle-ai/dive/demos/colosseum/arena"
	"github.com/deepnoodle-ai/dive/demos/colosseum/provider"
	"github.com/deepnoodle-ai/dive/session"
)

// cmdServeAgent hosts a Dive agent as an A2A server speaking the Colosseum's
// JSON decision protocol — this is the "bring your own agent to the arena" path.
// A game master elsewhere adds this endpoint to a match with
// `--players ...,a2a:http://your-host:PORT`.
func cmdServeAgent(args []string) error {
	fs := flag.NewFlagSet("serve-agent", flag.ContinueOnError)
	providerKey := fs.String("provider", "claude", "provider to host (claude, gpt, gemini, grok)")
	model := fs.String("model", "", "model override (defaults to the provider's tier)")
	premium := fs.Bool("premium", false, "use the provider's premium model tier")
	addr := fs.String("addr", ":8090", "listen address")
	name := fs.String("name", "", "agent card name (defaults to the provider key)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	spec, ok := provider.Resolve(*providerKey)
	if !ok {
		return provider.ErrUnknownProvider(*providerKey)
	}
	if ok, keys := spec.EnvSatisfied(); !ok {
		return fmt.Errorf("missing API key for %s (set one of: %v)", spec.Key, keys)
	}
	modelID := spec.ModelFor(*model, *premium)
	agentName := *name
	if agentName == "" {
		agentName = spec.Key
	}

	// The challenger agent runs on the SAME shared game rules as every other
	// player, but uses the JSON action protocol (it cannot call our tools over
	// the wire). It carries no tools and no fixed session — the A2A server
	// supplies a fresh, continuing session per game via the SessionProvider.
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         agentName,
		SystemPrompt: arena.RemoteSystemPrompt,
		Model:        spec.New(modelID),
	})
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	// One session per A2A context id, so each game the challenger plays keeps
	// its own private, continuing memory.
	var mu sync.Mutex
	sessions := map[string]dive.Session{}
	sessionProvider := func(ctx context.Context, contextID string) (dive.Session, error) {
		mu.Lock()
		defer mu.Unlock()
		if s, ok := sessions[contextID]; ok {
			return s, nil
		}
		s := session.New("colosseum-remote-" + contextID)
		sessions[contextID] = s
		return s, nil
	}

	srv, err := a2a.NewServer(a2a.ServerOptions{
		Agent:           agent,
		SessionProvider: sessionProvider,
		Card: a2a.CardOptions{
			Name:        "Colosseum Challenger: " + agentName,
			Description: "A Werewolf challenger for The Colosseum, powered by " + modelID + ". Speaks the JSON action protocol.",
		},
	})
	if err != nil {
		return fmt.Errorf("create A2A server: %w", err)
	}

	fmt.Printf("🏛  Colosseum challenger online\n")
	fmt.Printf("    provider: %s   model: %s\n", spec.Key, modelID)
	fmt.Printf("    A2A endpoint: http://localhost%s\n", normalizeAddr(*addr))
	fmt.Printf("    enter it in a match with:  --players claude,gpt,a2a:http://localhost%s\n", normalizeAddr(*addr))
	return http.ListenAndServe(*addr, srv.Handler())
}
