package transcript

import (
	"encoding/json"
	"fmt"
	"sort"
)

// PlayerInfo identifies one seat: its in-game id, the provider behind it, and
// the exact model used.
type PlayerInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Role     string `json:"role"`
}

// Usage is the token usage attributed to a player over a match.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Match is a structured view of a single match, reconstructed from its events.
// The raw Events are retained so a replay can step through them verbatim.
type Match struct {
	Seed      int64            `json:"seed"`
	Players   []PlayerInfo     `json:"players"`
	Winner    string           `json:"winner"` // "village", "werewolf", or ""
	Survivors []string         `json:"survivors"`
	Rounds    int              `json:"rounds"`
	Usage     map[string]Usage `json:"usage"`
	Events    []Event          `json:"events"`
	Complete  bool             `json:"complete"` // false if no match_end was recorded
}

// Parse reconstructs a Match from its ordered events. It tolerates an
// incomplete transcript (e.g. a run interrupted before match_end): Complete is
// set to false and whatever is known is filled in.
func Parse(events []Event) (*Match, error) {
	m := &Match{Events: events, Usage: map[string]Usage{}}
	var start, end *Event
	for i := range events {
		switch events[i].Type {
		case TypeMatchStart:
			start = &events[i]
		case TypeMatchEnd:
			end = &events[i]
		}
	}
	if start == nil {
		return nil, fmt.Errorf("transcript has no %s event", TypeMatchStart)
	}

	m.Seed = int64(num(start.Data["seed"]))
	roles := strMap(start.Data["roles"])
	for _, raw := range slice(start.Data["players"]) {
		p := mapOf(raw)
		id := str(p["id"])
		m.Players = append(m.Players, PlayerInfo{
			ID:       id,
			Provider: str(p["provider"]),
			Model:    str(p["model"]),
			Role:     roles[id],
		})
	}
	// Stable order by seat id keeps downstream rendering deterministic.
	sort.SliceStable(m.Players, func(i, j int) bool { return m.Players[i].ID < m.Players[j].ID })

	if end != nil {
		m.Complete = true
		m.Winner = str(end.Data["winner"])
		m.Rounds = int(num(end.Data["rounds"]))
		for _, s := range slice(end.Data["survivors"]) {
			m.Survivors = append(m.Survivors, str(s))
		}
		for id, raw := range mapOf(end.Data["usage_by_player"]) {
			u := mapOf(raw)
			m.Usage[id] = Usage{
				InputTokens:  int(num(u["input_tokens"])),
				OutputTokens: int(num(u["output_tokens"])),
			}
		}
	}
	return m, nil
}

// PlayerByID returns the seat with the given id, or nil.
func (m *Match) PlayerByID(id string) *PlayerInfo {
	for i := range m.Players {
		if m.Players[i].ID == id {
			return &m.Players[i]
		}
	}
	return nil
}

// --- defensive JSON helpers (Data round-trips through map[string]any) -------

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func slice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func mapOf(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func strMap(v any) map[string]string {
	out := map[string]string{}
	for k, val := range mapOf(v) {
		out[k] = str(val)
	}
	return out
}
