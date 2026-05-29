package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/deepnoodle-ai/dive/demos/colosseum/arena"
)

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	players := fs.String("players", "claude,gpt,gemini,grok", "comma-separated providers (or a2a:URL seats)")
	premium := fs.Bool("premium", false, "use premium model tiers")
	seed := fs.Int64("seed", 0, "RNG seed (0 = time-based)")
	maxRounds := fs.Int("max-rounds", 8, "safety cap on rounds")
	discussion := fs.Int("discussion-rounds", 1, "speaking passes per day")
	timeout := fs.Duration("timeout", 90*time.Second, "per-player-turn timeout")
	reveal := fs.Bool("reveal", false, "print private reasoning live")
	transcriptPath := fs.String("transcript", "", "JSONL transcript output path")
	overrides := modelOverrides{}
	fs.Var(overrides, "model", "override a provider's model (key=model, repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	contestants, err := buildContestants(*players, overrides, *premium)
	if err != nil {
		return err
	}

	seedVal := *seed
	if seedVal == 0 {
		seedVal = time.Now().UnixNano()
	}
	path := *transcriptPath
	if path == "" {
		path = fmt.Sprintf("colosseum-%d.jsonl", time.Now().Unix())
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create transcript: %w", err)
	}
	defer f.Close()

	// Cancel cleanly on Ctrl-C so an in-flight match stops without a stack dump.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	gm, err := arena.New(ctx, contestants, arena.Options{
		Seed:             seedVal,
		MaxRounds:        *maxRounds,
		DiscussionRounds: *discussion,
		TurnTimeout:      *timeout,
		MaxRetries:       2,
		Reveal:           *reveal,
		Out:              os.Stdout,
		Transcript:       f,
	})
	if err != nil {
		return err
	}
	if _, err := gm.Run(ctx); err != nil {
		return err
	}
	fmt.Printf("\nTranscript written to %s\n", path)
	return nil
}
