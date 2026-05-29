package analytics

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/deepnoodle-ai/dive/demos/colosseum/transcript"
)

// AnalyzeDir reads every *.jsonl transcript in dir, analyzes each completed
// match, and folds them into a fresh leaderboard. Files are processed in sorted
// order for determinism; unreadable or incomplete transcripts are skipped.
// Returns the analyses (one per completed match) and the aggregated leaderboard.
func AnalyzeDir(dir string) ([]*MatchAnalysis, *Leaderboard, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)

	lb := NewLeaderboard()
	var analyses []*MatchAnalysis
	for _, path := range files {
		a, err := AnalyzeFile(path)
		if err != nil {
			continue // skip partial/corrupt transcripts rather than fail the whole view
		}
		analyses = append(analyses, a)
		lb.Update(a)
	}
	return analyses, lb, nil
}

// AnalyzeFile reads, parses, and analyzes a single transcript file.
func AnalyzeFile(path string) (*MatchAnalysis, error) {
	events, err := transcript.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := transcript.Parse(events)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return Analyze(m), nil
}
