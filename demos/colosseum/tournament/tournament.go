// Package tournament runs many matches with the same contestants, aggregating
// an ELO leaderboard, per-match analyses, and highlights. It is the engine
// behind `colosseum tournament` and the leaderboard the web viewer serves.
package tournament

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/deepnoodle-ai/dive/demos/colosseum/analytics"
	"github.com/deepnoodle-ai/dive/demos/colosseum/arena"
	"github.com/deepnoodle-ai/dive/demos/colosseum/transcript"
)

// Options configures a tournament.
type Options struct {
	Contestants []arena.Contestant
	Matches     int           // number of matches to play
	BaseSeed    int64         // seed of match 1; each match uses BaseSeed+i
	OutDir      string        // directory for per-match transcripts (required)
	Leaderboard string        // optional path to persist/accumulate the leaderboard
	Arena       arena.Options // template options (Seed/Transcript/Out are set per match)
	Progress    io.Writer     // progress log (default os.Stdout); nil = silent
}

// Result is the aggregated outcome of a tournament.
type Result struct {
	Leaderboard *analytics.Leaderboard
	Analyses    []*analytics.MatchAnalysis
	Highlights  []analytics.Highlight
	Transcripts []string // paths written, one per match
}

// Run plays the configured matches in sequence, writing a transcript per match
// and folding each into the leaderboard. The leaderboard accumulates on top of
// any existing one at Options.Leaderboard, so tournaments are resumable and
// cumulative.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.Matches <= 0 {
		opts.Matches = 1
	}
	if opts.OutDir == "" {
		return nil, fmt.Errorf("tournament: OutDir is required")
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("tournament: create out dir: %w", err)
	}
	progress := opts.Progress
	if progress == nil {
		progress = io.Discard
	}

	lb := analytics.NewLeaderboard()
	if opts.Leaderboard != "" {
		loaded, err := analytics.LoadLeaderboard(opts.Leaderboard)
		if err != nil {
			return nil, fmt.Errorf("tournament: load leaderboard: %w", err)
		}
		lb = loaded
	}

	res := &Result{Leaderboard: lb}
	for i := 0; i < opts.Matches; i++ {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		seed := opts.BaseSeed + int64(i)
		path := filepath.Join(opts.OutDir, fmt.Sprintf("match-%d.jsonl", seed))
		fmt.Fprintf(progress, "▶ match %d/%d (seed %d) … ", i+1, opts.Matches, seed)

		analysis, err := runOne(ctx, opts, seed, path)
		if err != nil {
			fmt.Fprintf(progress, "error: %v\n", err)
			return res, err
		}
		fmt.Fprintf(progress, "%s in %d rounds\n", winnerLabel(analysis.Winner), analysis.Rounds)

		lb.Update(analysis)
		res.Analyses = append(res.Analyses, analysis)
		res.Highlights = append(res.Highlights, analysis.Highlights...)
		res.Transcripts = append(res.Transcripts, path)

		// Persist incrementally so an interrupted tournament keeps its progress.
		if opts.Leaderboard != "" {
			if err := lb.Save(opts.Leaderboard); err != nil {
				return res, fmt.Errorf("tournament: save leaderboard: %w", err)
			}
		}
	}
	return res, nil
}

// runOne plays a single match, writes its transcript, and returns its analysis.
func runOne(ctx context.Context, opts Options, seed int64, path string) (*analytics.MatchAnalysis, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create transcript: %w", err)
	}
	defer f.Close()

	ao := opts.Arena
	ao.Seed = seed
	ao.Transcript = f
	if ao.Out == nil {
		ao.Out = io.Discard // tournaments are quiet by default; the transcript has all detail
	}

	gm, err := arena.New(ctx, opts.Contestants, ao)
	if err != nil {
		return nil, err
	}
	if _, err := gm.Run(ctx); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	events, err := transcript.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := transcript.Parse(events)
	if err != nil {
		return nil, err
	}
	return analytics.Analyze(m), nil
}

func winnerLabel(winner string) string {
	switch winner {
	case "village":
		return "village wins"
	case "werewolf":
		return "werewolves win"
	default:
		return "undecided"
	}
}
