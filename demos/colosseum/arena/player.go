package arena

import (
	"context"

	"github.com/deepnoodle-ai/dive/llm"
)

// Player is one seat at the table: its identity plus the decider that produces
// its actions (a local Dive agent or a remote A2A agent).
type Player struct {
	ID       string // matches game.Player.ID
	Provider string // provider key, e.g. "claude", or "a2a" for remote seats
	Model    string // resolved model id (or the URL for remote seats)
	Remote   bool   // true if this seat is reached over A2A

	decider decider
	usage   llm.Usage // accumulated token usage across the match (local only)
}

// newPlayer builds a seat's decider from its contestant spec: a remote A2A
// agent when RemoteURL is set, otherwise a local Dive agent built from the
// shared template.
func newPlayer(ctx context.Context, c Contestant, pol policy, log func(string)) (*Player, error) {
	p := &Player{ID: c.ID, Provider: c.Provider, Model: c.Model}
	if c.RemoteURL != "" {
		p.Remote = true
		if p.Model == "" {
			p.Model = c.RemoteURL
		}
		rd, err := newRemoteDecider(ctx, c.ID, c.RemoteURL, pol, log)
		if err != nil {
			return nil, err
		}
		p.decider = rd
		return p, nil
	}
	ld, err := newLocalDecider(c.ID, c.LLM, pol, log)
	if err != nil {
		return nil, err
	}
	p.decider = ld
	return p, nil
}
