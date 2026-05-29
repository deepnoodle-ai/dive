// Package transcript defines the Colosseum's match record: a newline-delimited
// JSON event stream plus helpers to write, read, and structure it.
//
// The transcript is the post-match reveal artifact. It deliberately records
// things players never saw during play — every player's role, each action's
// hidden reasoning, the Seer's visions — so a replay viewer can "unmask the
// chain of thought" after the fact. It is append-only JSONL, which makes it
// trivially replayable and diffable, and the substrate the leaderboard and web
// viewer are built on.
package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Event is one line of the transcript. A single flexible struct keeps the
// format easy to diff and replay; unused fields are omitted.
type Event struct {
	Type      string         `json:"type"`
	Time      string         `json:"time"`
	Round     int            `json:"round,omitempty"`
	Phase     string         `json:"phase,omitempty"`
	Actor     string         `json:"actor,omitempty"`
	Role      string         `json:"role,omitempty"`
	Target    string         `json:"target,omitempty"`
	Message   string         `json:"message,omitempty"`   // public statement
	Reasoning string         `json:"reasoning,omitempty"` // PRIVATE reasoning
	Public    string         `json:"public,omitempty"`    // public narration line
	Detail    string         `json:"detail,omitempty"`    // errors / notes
	Data      map[string]any `json:"data,omitempty"`      // type-specific extras
}

// Event type constants. Using named constants keeps producers (arena) and
// consumers (analytics, web) in sync.
const (
	TypeMatchStart  = "match_start"
	TypeMatchEnd    = "match_end"
	TypePhaseStart  = "phase_start"
	TypeSpeak       = "speak"
	TypeVote        = "vote"
	TypeTally       = "tally"
	TypeNightAction = "night_action"
	TypeSeerResult  = "seer_result"
	TypeElimination = "elimination"
	TypeProtected   = "protected"
	TypeNoDeath     = "no_death"
	TypeForfeit     = "forfeit"
	TypeUsage       = "usage"
)

// Writer appends events as JSON lines. It is safe for concurrent use.
type Writer struct {
	mu  sync.Mutex
	enc *json.Encoder
	now func() time.Time
}

// NewWriter returns a Writer that encodes events to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{enc: json.NewEncoder(w), now: time.Now}
}

// Write stamps the event with the current time (if unset) and appends it as one
// JSON line.
func (w *Writer) Write(e Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e.Time == "" {
		e.Time = w.now().UTC().Format(time.RFC3339Nano)
	}
	if err := w.enc.Encode(e); err != nil {
		return fmt.Errorf("transcript write: %w", err)
	}
	return nil
}

// Read parses a JSONL transcript into its events, in order. Blank lines are
// skipped so partially-flushed files read cleanly.
func Read(r io.Reader) ([]Event, error) {
	var out []Event
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // reasoning fields can be long
	for sc.Scan() {
		line := sc.Bytes()
		if len(trimSpace(line)) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("transcript parse: %w", err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("transcript read: %w", err)
	}
	return out, nil
}

// ReadFile reads and parses a transcript file.
func ReadFile(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}

func trimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && (b[i] == ' ' || b[i] == '\t' || b[i] == '\r' || b[i] == '\n') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\t' || b[j-1] == '\r' || b[j-1] == '\n') {
		j--
	}
	return b[i:j]
}
