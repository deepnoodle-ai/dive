package arena

import (
	"context"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

// decider turns a game situation into an action. It is the seam that lets a
// seat be filled either by a local Dive agent (tools + referee hook, the
// Phase 1 path) or by a remote agent reached over A2A (the "bring your own
// agent" path) — the game master treats both identically.
type decider interface {
	// decide runs one turn. situation is the briefing/state the game master
	// composed; the decider appends its own action instruction. It returns the
	// submitted action, the token usage it consumed (nil if unknown, e.g. over
	// the wire), and an error if it could not produce a valid action.
	decide(ctx context.Context, t turn, situation string) (*capturedAction, *llm.Usage, error)
}

// policy bounds how hard a decider tries before the game master defaults the
// turn to an abstain.
type policy struct {
	turnTimeout time.Duration
	maxRetries  int
}

// backoff sleeps with exponential delay, honouring context cancellation, so a
// rate-limited provider is retried politely rather than hammered.
func backoff(ctx context.Context, attempt int) {
	d := time.Duration(1<<attempt) * time.Second
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func setOf(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, it := range items {
		set[it] = true
	}
	return set
}
