package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive/demos/colosseum/transcript"
	"github.com/deepnoodle-ai/wonton/assert"
)

// writeSampleTranscript writes a minimal complete match to dir/<name>.jsonl.
func writeSampleTranscript(t *testing.T, dir, name string) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, name+".jsonl"))
	assert.NoError(t, err)
	defer f.Close()
	w := transcript.NewWriter(f)
	events := []transcript.Event{
		{Type: transcript.TypeMatchStart, Data: map[string]any{
			"seed": float64(1),
			"players": []any{
				map[string]any{"id": "claude", "provider": "claude", "model": "haiku"},
				map[string]any{"id": "gpt", "provider": "gpt", "model": "mini"},
				map[string]any{"id": "grok", "provider": "grok", "model": "fast"},
			},
			"roles": map[string]any{"claude": "seer", "gpt": "villager", "grok": "werewolf"},
		}},
		{Type: transcript.TypeSpeak, Round: 1, Actor: "grok", Message: "I'm innocent", Reasoning: "lying"},
		{Type: transcript.TypeVote, Round: 1, Actor: "claude", Target: "grok"},
		{Type: transcript.TypeElimination, Round: 1, Target: "grok", Role: "werewolf", Data: map[string]any{"cause": "vote"}},
		{Type: transcript.TypeMatchEnd, Data: map[string]any{
			"winner": "village", "rounds": float64(1), "survivors": []any{"claude", "gpt"},
		}},
	}
	for _, e := range events {
		assert.NoError(t, w.Write(e))
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	writeSampleTranscript(t, dir, "match-1")
	srv, err := NewServer(dir)
	assert.NoError(t, err)
	return httptest.NewServer(srv.Handler())
}

// newTestServerWithArtifacts spins up a server backed by both a transcripts dir
// (with one sample match) and an artifacts dir seeded by the caller.
func newTestServerWithArtifacts(t *testing.T, seed func(artifactsDir string)) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	writeSampleTranscript(t, dir, "match-1")
	artDir := t.TempDir()
	if seed != nil {
		seed(artDir)
	}
	srv, err := NewServer(dir, WithArtifactsDir(artDir))
	assert.NoError(t, err)
	return httptest.NewServer(srv.Handler())
}

func getJSON(t *testing.T, url string, dst any) int {
	t.Helper()
	resp, err := http.Get(url)
	assert.NoError(t, err)
	defer resp.Body.Close()
	if dst != nil {
		assert.NoError(t, json.NewDecoder(resp.Body).Decode(dst))
	}
	return resp.StatusCode
}

func TestAPIMatches(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	var body struct {
		Matches []struct {
			ID      string `json:"id"`
			Winner  string `json:"winner"`
			Players []struct {
				ID   string `json:"id"`
				Role string `json:"role"`
			} `json:"players"`
		} `json:"matches"`
	}
	code := getJSON(t, ts.URL+"/api/matches", &body)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, len(body.Matches))
	assert.Equal(t, "match-1", body.Matches[0].ID)
	assert.Equal(t, "village", body.Matches[0].Winner)
	assert.Equal(t, 3, len(body.Matches[0].Players))
}

func TestAPIMatchDetail(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	var body struct {
		Match struct {
			Winner string             `json:"winner"`
			Events []transcript.Event `json:"events"`
		} `json:"match"`
		Analysis struct {
			Winner     string `json:"winner"`
			Highlights []struct {
				Type string `json:"type"`
			} `json:"highlights"`
			Players []struct {
				ID  string `json:"id"`
				Won bool   `json:"won"`
			} `json:"players"`
		} `json:"analysis"`
	}
	code := getJSON(t, ts.URL+"/api/matches/match-1", &body)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "village", body.Match.Winner)
	assert.True(t, len(body.Match.Events) > 0)
	assert.Equal(t, "village", body.Analysis.Winner)
	assert.Equal(t, 3, len(body.Analysis.Players))
}

func TestAPIMatchNotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	code := getJSON(t, ts.URL+"/api/matches/nope", nil)
	assert.Equal(t, http.StatusNotFound, code)
}

func TestAPIMatchRejectsTraversal(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	// A path-traversal id must not escape the transcripts dir.
	resp, err := http.Get(ts.URL + "/api/matches/..%2f..%2fetc%2fpasswd")
	assert.NoError(t, err)
	resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}

func TestAPILeaderboard(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	var body struct {
		Standings []struct {
			Rank    int     `json:"rank"`
			Model   string  `json:"model"`
			Elo     float64 `json:"elo"`
			WinRate float64 `json:"win_rate"`
		} `json:"standings"`
	}
	code := getJSON(t, ts.URL+"/api/leaderboard", &body)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 3, len(body.Standings))
	assert.Equal(t, 1, body.Standings[0].Rank)
	// Village winners (seer, villager) should outrank the lynched wolf.
	assert.Equal(t, "werewolf", lastModelRole(body.Standings))
}

func TestAPIArtifactsList(t *testing.T) {
	ts := newTestServerWithArtifacts(t, func(dir string) {
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "diagram.png"), []byte("\x89PNG\r\n"), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Title\n\nbody"), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "clip.mp4"), []byte("fakevideo"), 0o644))
		// A subdirectory must be skipped (only top-level files are listed).
		assert.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o755))
	})
	defer ts.Close()

	var body struct {
		Artifacts []struct {
			Name        string `json:"name"`
			Size        int64  `json:"size"`
			Kind        string `json:"kind"`
			ContentType string `json:"content_type"`
			Modified    string `json:"modified"`
		} `json:"artifacts"`
	}
	code := getJSON(t, ts.URL+"/api/artifacts", &body)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 3, len(body.Artifacts))

	kinds := map[string]string{}
	for _, a := range body.Artifacts {
		kinds[a.Name] = a.Kind
		assert.True(t, a.Modified != "")
	}
	assert.Equal(t, "image", kinds["diagram.png"])
	assert.Equal(t, "markdown", kinds["notes.md"])
	assert.Equal(t, "video", kinds["clip.mp4"])
}

func TestAPIArtifactsEmptyWhenMissingDir(t *testing.T) {
	// artifactsDir points somewhere that does not exist: list is empty, not an error.
	dir := t.TempDir()
	writeSampleTranscript(t, dir, "match-1")
	srv, err := NewServer(dir, WithArtifactsDir(filepath.Join(dir, "does-not-exist")))
	assert.NoError(t, err)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var body struct {
		Artifacts []json.RawMessage `json:"artifacts"`
	}
	code := getJSON(t, ts.URL+"/api/artifacts", &body)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, len(body.Artifacts))
}

func TestAPIArtifactContent(t *testing.T) {
	ts := newTestServerWithArtifacts(t, func(dir string) {
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0o644))
	})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/artifacts/notes.md")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	data, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "# Hello", string(data))
}

func TestAPIArtifactNotFound(t *testing.T) {
	ts := newTestServerWithArtifacts(t, nil)
	defer ts.Close()
	code := getJSON(t, ts.URL+"/api/artifacts/missing.png", nil)
	assert.Equal(t, http.StatusNotFound, code)
}

func TestAPIArtifactRejectsTraversal(t *testing.T) {
	ts := newTestServerWithArtifacts(t, nil)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/artifacts/..%2f..%2fetc%2fpasswd")
	assert.NoError(t, err)
	resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)
}

// lastModelRole returns the model name of the lowest-ranked standing; in the
// sample the werewolf model ("fast") loses and should be last.
func lastModelRole(standings []struct {
	Rank    int     `json:"rank"`
	Model   string  `json:"model"`
	Elo     float64 `json:"elo"`
	WinRate float64 `json:"win_rate"`
}) string {
	if len(standings) == 0 {
		return ""
	}
	last := standings[len(standings)-1]
	if last.Model == "fast" {
		return "werewolf"
	}
	return last.Model
}
