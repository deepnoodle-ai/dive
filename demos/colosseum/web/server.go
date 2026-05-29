// Package web serves the Colosseum replay viewer and leaderboard: a static
// single-page app (embedded in the binary, no build step) plus a small JSON API
// over a directory of match transcripts. This is the "forkable" UI from the
// plan — one Go binary serves everything, so `colosseum serve` just works.
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/deepnoodle-ai/dive/demos/colosseum/analytics"
	"github.com/deepnoodle-ai/dive/demos/colosseum/transcript"
)

//go:embed static/*
var staticFS embed.FS

// Server serves the viewer and API over a transcripts directory.
type Server struct {
	dir string
	mux *http.ServeMux
}

// NewServer builds a server reading transcripts from dir.
func NewServer(dir string) (*Server, error) {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("transcripts directory %q does not exist", dir)
	}
	s := &Server{dir: dir, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

// Handler returns the HTTP handler for the viewer + API.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	sub, _ := fs.Sub(staticFS, "static")
	fileServer := http.FileServer(http.FS(sub))

	s.mux.HandleFunc("GET /api/matches", s.handleMatches)
	s.mux.HandleFunc("GET /api/matches/{id}", s.handleMatch)
	s.mux.HandleFunc("GET /api/leaderboard", s.handleLeaderboard)
	s.mux.Handle("GET /", fileServer)
}

// matchSummary is the card shown in the match list.
type matchSummary struct {
	ID       string                  `json:"id"`
	Seed     int64                   `json:"seed"`
	Winner   string                  `json:"winner"`
	Rounds   int                     `json:"rounds"`
	Complete bool                    `json:"complete"`
	Players  []transcript.PlayerInfo `json:"players"`
}

func (s *Server) handleMatches(w http.ResponseWriter, r *http.Request) {
	ids, err := s.transcriptIDs()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	summaries := make([]matchSummary, 0, len(ids))
	for _, id := range ids {
		m, err := s.loadMatch(id)
		if err != nil {
			continue
		}
		summaries = append(summaries, matchSummary{
			ID: id, Seed: m.Seed, Winner: m.Winner, Rounds: m.Rounds,
			Complete: m.Complete, Players: m.Players,
		})
	}
	writeJSON(w, map[string]any{"matches": summaries})
}

func (s *Server) handleMatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		httpError(w, http.StatusBadRequest, fmt.Errorf("invalid match id"))
		return
	}
	m, err := s.loadMatch(id)
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, map[string]any{
		"match":    m,
		"analysis": analytics.Analyze(m),
	})
}

// standingRow is the leaderboard row with metric averages pre-computed, so the
// frontend can render directly without recombining sum/count fields.
type standingRow struct {
	Rank        int     `json:"rank"`
	Model       string  `json:"model"`
	Provider    string  `json:"provider"`
	Elo         float64 `json:"elo"`
	Matches     int     `json:"matches"`
	Wins        int     `json:"wins"`
	Losses      int     `json:"losses"`
	WinRate     float64 `json:"win_rate"`
	WolfWinRate float64 `json:"wolf_win_rate"`
	Deception   float64 `json:"deception"`
	Deduction   float64 `json:"deduction"`
	Persuasion  float64 `json:"persuasion"`
}

func (s *Server) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	_, lb, err := analytics.AnalyzeDir(s.dir)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	standings := lb.Standings()
	rows := make([]standingRow, len(standings))
	for i, r := range standings {
		var wolfWin float64
		if r.GamesAsWolf > 0 {
			wolfWin = float64(r.WolfWins) / float64(r.GamesAsWolf)
		}
		rows[i] = standingRow{
			Rank: i + 1, Model: r.Model, Provider: r.Provider, Elo: r.Elo,
			Matches: r.Matches, Wins: r.Wins, Losses: r.Losses,
			WinRate: r.WinRate(), WolfWinRate: wolfWin,
			Deception: r.DeceptionAvg(), Deduction: r.DeductionAvg(), Persuasion: r.PersuasionAvg(),
		}
	}
	writeJSON(w, map[string]any{"standings": rows})
}

// transcriptIDs lists the available match ids (filenames without .jsonl),
// newest first by modification time.
func (s *Server) transcriptIDs() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	type ent struct {
		id      string
		modTime int64
	}
	var ents []ent
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		ents = append(ents, ent{id: strings.TrimSuffix(e.Name(), ".jsonl"), modTime: info.ModTime().UnixNano()})
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].modTime > ents[j].modTime })
	ids := make([]string, len(ents))
	for i, e := range ents {
		ids[i] = e.id
	}
	return ids, nil
}

func (s *Server) loadMatch(id string) (*transcript.Match, error) {
	if !validID(id) {
		return nil, fmt.Errorf("invalid id")
	}
	events, err := transcript.ReadFile(filepath.Join(s.dir, id+".jsonl"))
	if err != nil {
		return nil, err
	}
	return transcript.Parse(events)
}

// validID rejects ids that could escape the transcripts directory.
func validID(id string) bool {
	if id == "" || strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		httpError(w, http.StatusInternalServerError, err)
	}
}

func httpError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
