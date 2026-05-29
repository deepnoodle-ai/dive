package analytics

import (
	"fmt"

	"github.com/deepnoodle-ai/dive/demos/colosseum/transcript"
)

// Highlight is a dramatic moment auto-detected from a match — the raw material
// for shareable clips and cards ("GPT knew Claude was the wolf and said
// nothing"). Each carries a type for filtering, a punchy title, and the round
// and players involved.
type Highlight struct {
	Type    string   `json:"type"`
	Title   string   `json:"title"`
	Round   int      `json:"round"`
	Detail  string   `json:"detail"`
	Players []string `json:"players,omitempty"`
}

// Highlight type identifiers.
const (
	HLWolfVictory  = "wolf_victory"
	HLLoneWolf     = "lone_wolf"
	HLSeerIgnored  = "seer_ignored"
	HLMislynch     = "mislynch_power_role"
	HLDoctorSave   = "doctor_save"
	HLCloseCall    = "wolf_survived_close_vote"
	HLFlawlessTown = "flawless_village"
)

// detectHighlights scans a match for noteworthy moments.
func detectHighlights(m *transcript.Match, votesByRound map[int]map[string]string) []Highlight {
	var out []Highlight

	wolves := playersWithRole(m, roleWerewolf)

	// Outcome-level drama.
	switch m.Winner {
	case teamWerewolf:
		if len(wolves) == 1 {
			out = append(out, Highlight{
				Type:    HLLoneWolf,
				Title:   fmt.Sprintf("Lone wolf %s deceived the entire village", wolves[0]),
				Round:   m.Rounds,
				Detail:  "A single werewolf bluffed its way to victory against the whole table.",
				Players: wolves,
			})
		} else {
			out = append(out, Highlight{
				Type:    HLWolfVictory,
				Title:   "The werewolves took over the village",
				Round:   m.Rounds,
				Detail:  "The pack reached parity — deception beat deduction.",
				Players: wolves,
			})
		}
	case teamVillage:
		if seers := survivingSeers(m); len(seers) > 0 {
			out = append(out, Highlight{
				Type:    HLFlawlessTown,
				Title:   "The village won with its Seer still standing",
				Round:   m.Rounds,
				Detail:  "Town read the room and kept its most valuable role alive.",
				Players: seers,
			})
		}
	}

	// Event-level drama, in chronological order.
	for _, e := range m.Events {
		switch e.Type {
		case transcript.TypeProtected:
			out = append(out, Highlight{
				Type:   HLDoctorSave,
				Title:  "The Doctor saved a life in the night",
				Round:  e.Round,
				Detail: "A werewolf attack was foiled by a well-placed protection.",
			})
		case transcript.TypeElimination:
			if cause, _ := e.Data["cause"].(string); cause == "vote" &&
				(e.Role == roleSeer || e.Role == roleDoctor) {
				out = append(out, Highlight{
					Type:    HLMislynch,
					Title:   fmt.Sprintf("The village turned on its own %s", e.Role),
					Round:   e.Round,
					Detail:  fmt.Sprintf("%s (%s) was voted out by their own side — a gift to the wolves.", e.Target, e.Role),
					Players: []string{e.Target},
				})
			}
		case transcript.TypeSeerResult:
			if isWolf, _ := e.Data["is_werewolf"].(bool); isWolf {
				if hl, ok := seerIgnored(m, e); ok {
					out = append(out, hl)
				}
			}
		}
	}

	// Close votes a wolf survived.
	out = append(out, closeCalls(m, votesByRound)...)
	return out
}

// seerIgnored fires when the Seer identified a wolf but the village never voted
// that wolf out — the "it knew and we didn't listen" moment.
func seerIgnored(m *transcript.Match, e transcript.Event) (Highlight, bool) {
	wolf := e.Target
	for _, ev := range m.Events {
		if ev.Type == transcript.TypeElimination && ev.Target == wolf {
			if cause, _ := ev.Data["cause"].(string); cause == "vote" {
				return Highlight{}, false // the village did act on it
			}
		}
	}
	return Highlight{
		Type:    HLSeerIgnored,
		Title:   fmt.Sprintf("The Seer knew %s was a wolf — and the village never lynched them", wolf),
		Round:   e.Round,
		Detail:  fmt.Sprintf("%s privately identified %s as a werewolf in round %d, but the table failed to vote them out.", e.Actor, wolf, e.Round),
		Players: []string{e.Actor, wolf},
	}, true
}

// closeCalls finds rounds where a wolf drew votes but escaped elimination by a
// one-vote margin (or a tie that saved them).
func closeCalls(m *transcript.Match, votesByRound map[int]map[string]string) []Highlight {
	var out []Highlight
	wolfSet := map[string]bool{}
	for _, w := range playersWithRole(m, roleWerewolf) {
		wolfSet[w] = true
	}
	for round, votes := range votesByRound {
		tally := map[string]int{}
		for _, t := range votes {
			if t != "" {
				tally[t]++
			}
		}
		if len(tally) == 0 {
			continue
		}
		max := 0
		for _, n := range tally {
			if n > max {
				max = n
			}
		}
		for id, n := range tally {
			if wolfSet[id] && n > 0 && max-n <= 1 && !eliminatedByVoteIn(m, id, round) {
				out = append(out, Highlight{
					Type:    HLCloseCall,
					Title:   fmt.Sprintf("Werewolf %s survived a knife-edge vote", id),
					Round:   round,
					Detail:  fmt.Sprintf("%s drew %d of %d top votes in round %d but dodged the lynch.", id, n, max, round),
					Players: []string{id},
				})
			}
		}
	}
	return out
}

func eliminatedByVoteIn(m *transcript.Match, id string, round int) bool {
	for _, e := range m.Events {
		if e.Type == transcript.TypeElimination && e.Target == id && e.Round == round {
			if cause, _ := e.Data["cause"].(string); cause == "vote" {
				return true
			}
		}
	}
	return false
}

func playersWithRole(m *transcript.Match, role string) []string {
	var out []string
	for _, p := range m.Players {
		if p.Role == role {
			out = append(out, p.ID)
		}
	}
	return out
}

func survivingSeers(m *transcript.Match) []string {
	var out []string
	for _, p := range m.Players {
		if p.Role == roleSeer && contains(m.Survivors, p.ID) {
			out = append(out, p.ID)
		}
	}
	return out
}
