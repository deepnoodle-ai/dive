package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/deepnoodle-ai/dive/demos/colosseum/analytics"
	"github.com/deepnoodle-ai/dive/demos/colosseum/arena"
	"github.com/deepnoodle-ai/dive/demos/colosseum/tournament"
)

func cmdTournament(args []string) error {
	fs := flag.NewFlagSet("tournament", flag.ContinueOnError)
	players := fs.String("players", "claude,gpt,gemini,grok", "comma-separated providers (or a2a:URL seats)")
	matches := fs.Int("matches", 5, "number of matches to play")
	premium := fs.Bool("premium", false, "use premium model tiers")
	baseSeed := fs.Int64("seed", 1, "seed of the first match (each match uses seed+i)")
	maxRounds := fs.Int("max-rounds", 8, "safety cap on rounds")
	discussion := fs.Int("discussion-rounds", 1, "speaking passes per day")
	timeout := fs.Duration("timeout", 90*time.Second, "per-player-turn timeout")
	outDir := fs.String("dir", "transcripts", "directory for per-match transcripts")
	lbPath := fs.String("leaderboard", "", "leaderboard JSON to accumulate (default: <dir>/leaderboard.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	contestants, err := buildContestants(*players, modelOverrides{}, *premium)
	if err != nil {
		return err
	}
	lb := *lbPath
	if lb == "" {
		lb = *outDir + "/leaderboard.json"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	res, err := tournament.Run(ctx, tournament.Options{
		Contestants: contestants,
		Matches:     *matches,
		BaseSeed:    *baseSeed,
		OutDir:      *outDir,
		Leaderboard: lb,
		Progress:    os.Stdout,
		Arena: arena.Options{
			MaxRounds:        *maxRounds,
			DiscussionRounds: *discussion,
			TurnTimeout:      *timeout,
			MaxRetries:       2,
		},
	})
	if res != nil && res.Leaderboard != nil {
		fmt.Printf("\n=== LEADERBOARD (after %d matches) ===\n", len(res.Analyses))
		analytics.RenderStandings(os.Stdout, res.Leaderboard)
		if len(res.Highlights) > 0 {
			fmt.Printf("\n=== HIGHLIGHTS ===\n")
			analytics.RenderHighlights(os.Stdout, res.Highlights)
		}
		fmt.Printf("\nTranscripts in %s · leaderboard at %s\n", *outDir, lb)
	}
	return err
}
