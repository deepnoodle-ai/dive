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
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/demos/colosseum/analytics"
	"github.com/deepnoodle-ai/dive/demos/colosseum/transcript"
)

//go:embed static/*
var staticFS embed.FS

// Server serves the viewer and API over a transcripts directory.
type Server struct {
	dir          string // match transcripts
	artifactsDir string // optional directory of renderable artifacts (images, video, markdown…)
	mux          *http.ServeMux
}

// Option configures a Server.
type Option func(*Server)

// WithArtifactsDir serves files from dir under the /api/artifacts endpoints,
// powering the Artifacts tab in the viewer. The directory is optional: if it is
// empty or missing, the artifacts list simply renders empty.
func WithArtifactsDir(dir string) Option {
	return func(s *Server) { s.artifactsDir = dir }
}

// NewServer builds a server reading transcripts from dir.
func NewServer(dir string, opts ...Option) (*Server, error) {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("transcripts directory %q does not exist", dir)
	}
	s := &Server{dir: dir, mux: http.NewServeMux()}
	for _, opt := range opts {
		opt(s)
	}
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
	s.mux.HandleFunc("GET /api/artifacts", s.handleArtifacts)
	s.mux.HandleFunc("GET /api/artifacts/{name}", s.handleArtifactContent)
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

// artifactInfo is one entry in the Artifacts tab: enough metadata for the
// detail panel plus a kind hint so the frontend knows how to render it.
type artifactInfo struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Modified    string `json:"modified"`     // RFC3339 UTC
	ContentType string `json:"content_type"` // best-effort MIME type by extension
	Kind        string `json:"kind"`         // image|video|audio|markdown|text|pdf|other
}

// handleArtifacts lists the (top-level) files in the artifacts directory,
// newest first. A missing or unconfigured directory yields an empty list so the
// frontend can show a friendly empty state rather than an error.
func (s *Server) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	infos := []artifactInfo{}
	if s.artifactsDir != "" {
		entries, err := os.ReadDir(s.artifactsDir)
		if err != nil && !os.IsNotExist(err) {
			httpError(w, http.StatusInternalServerError, err)
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			infos = append(infos, newArtifactInfo(e.Name(), info))
		}
		// Newest first; RFC3339 UTC strings sort chronologically.
		sort.Slice(infos, func(i, j int) bool {
			if infos[i].Modified != infos[j].Modified {
				return infos[i].Modified > infos[j].Modified
			}
			return infos[i].Name < infos[j].Name
		})
	}
	writeJSON(w, map[string]any{"artifacts": infos})
}

// handleArtifactContent serves a single artifact's raw bytes. It uses
// http.ServeFile so range requests work (important for seeking video/audio) and
// the Content-Type is set from the extension.
func (s *Server) handleArtifactContent(w http.ResponseWriter, r *http.Request) {
	if s.artifactsDir == "" {
		httpError(w, http.StatusNotFound, fmt.Errorf("no artifacts configured"))
		return
	}
	name := r.PathValue("name")
	if !validName(name) {
		httpError(w, http.StatusBadRequest, fmt.Errorf("invalid artifact name"))
		return
	}
	full := filepath.Join(s.artifactsDir, name)
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		httpError(w, http.StatusNotFound, fmt.Errorf("artifact not found"))
		return
	}
	http.ServeFile(w, r, full)
}

// newArtifactInfo builds the metadata record for a single file.
func newArtifactInfo(name string, info os.FileInfo) artifactInfo {
	ct := mime.TypeByExtension(filepath.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return artifactInfo{
		Name:        name,
		Size:        info.Size(),
		Modified:    info.ModTime().UTC().Format(time.RFC3339),
		ContentType: ct,
		Kind:        artifactKind(name, ct),
	}
}

// artifactKind classifies a file so the frontend can pick a renderer.
func artifactKind(name, contentType string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return "markdown"
	}
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	case strings.HasPrefix(contentType, "audio/"):
		return "audio"
	case contentType == "application/pdf":
		return "pdf"
	case strings.HasPrefix(contentType, "text/"),
		contentType == "application/json",
		contentType == "application/xml":
		return "text"
	}
	return "other"
}

// validName rejects names that could escape the artifacts directory.
func validName(name string) bool {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return false
	}
	return true
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
