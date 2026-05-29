package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/demos/colosseum/analytics"
)

// cmdLeaderboard prints standings for a transcripts directory or a saved
// leaderboard.json.
func cmdLeaderboard(args []string) error {
	fs := flag.NewFlagSet("leaderboard", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	path := fs.Arg(0)
	if path == "" {
		path = "transcripts"
	}

	lb, err := leaderboardFrom(path)
	if err != nil {
		return err
	}
	analytics.RenderStandings(os.Stdout, lb)
	return nil
}

// leaderboardFrom loads a leaderboard from either a leaderboard JSON file or a
// directory of transcripts (computed fresh).
func leaderboardFrom(path string) (*analytics.Leaderboard, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() && strings.HasSuffix(path, ".json") {
		return analytics.LoadLeaderboard(path)
	}
	if info.IsDir() {
		_, lb, err := analytics.AnalyzeDir(path)
		return lb, err
	}
	return nil, fmt.Errorf("expected a transcripts directory or a leaderboard .json file, got %q", path)
}

// cmdHighlights analyzes a single transcript: per-player metrics + highlights.
func cmdHighlights(args []string) error {
	fs := flag.NewFlagSet("highlights", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	path := fs.Arg(0)
	if path == "" {
		return fmt.Errorf("usage: colosseum highlights <transcript.jsonl>")
	}
	a, err := analytics.AnalyzeFile(path)
	if err != nil {
		return err
	}
	analytics.RenderMatch(os.Stdout, a)
	return nil
}
