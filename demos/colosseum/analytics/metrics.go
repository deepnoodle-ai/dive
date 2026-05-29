// Package analytics turns match transcripts into the shareable layer: per-player
// skill metrics (deception, deduction, persuasion), dramatic highlights, and an
// ELO leaderboard aggregated across many matches. Everything here reads a parsed
// transcript.Match — it never talks to a model — so it is fast, deterministic,
// and unit-testable.
package analytics

import (
	"github.com/deepnoodle-ai/dive/demos/colosseum/transcript"
)

const (
	teamVillage  = "village"
	teamWerewolf = "werewolf"
	roleWerewolf = "werewolf"
	roleSeer     = "seer"
	roleDoctor   = "doctor"
)

// teamOf maps a role string to its faction.
func teamOf(role string) string {
	if role == roleWerewolf {
		return teamWerewolf
	}
	return teamVillage
}

// PlayerResult is one seat's outcome and derived skill metrics for a single
// match. Metrics that don't apply to a role are left nil (e.g. deception is
// werewolf-only) so aggregation can average only the games where they're
// meaningful.
type PlayerResult struct {
	ID             string   `json:"id"`
	Provider       string   `json:"provider"`
	Model          string   `json:"model"`
	Role           string   `json:"role"`
	Team           string   `json:"team"`
	Won            bool     `json:"won"`
	Survived       bool     `json:"survived"`
	RoundsSurvived int      `json:"rounds_survived"`
	Deception      *float64 `json:"deception,omitempty"`  // werewolves: fraction of rounds survived unlynched
	Deduction      *float64 `json:"deduction,omitempty"`  // villagers: fraction of votes cast on actual wolves
	Persuasion     *float64 `json:"persuasion,omitempty"` // all: fraction of other voters who matched this player's vote
}

// MatchAnalysis is the full analysis of one match.
type MatchAnalysis struct {
	Seed       int64          `json:"seed"`
	Winner     string         `json:"winner"`
	Rounds     int            `json:"rounds"`
	Players    []PlayerResult `json:"players"`
	Highlights []Highlight    `json:"highlights"`
}

// Analyze computes per-player metrics and highlights from a parsed match.
func Analyze(m *transcript.Match) *MatchAnalysis {
	votesByRound, elimRound := indexEvents(m)

	a := &MatchAnalysis{Seed: m.Seed, Winner: m.Winner, Rounds: m.Rounds}
	for _, p := range m.Players {
		team := teamOf(p.Role)
		pr := PlayerResult{
			ID:       p.ID,
			Provider: p.Provider,
			Model:    p.Model,
			Role:     p.Role,
			Team:     team,
			Won:      m.Winner != "" && team == m.Winner,
			Survived: contains(m.Survivors, p.ID),
		}
		if r, ok := elimRound[p.ID]; ok {
			pr.RoundsSurvived = r
		} else {
			pr.RoundsSurvived = m.Rounds
		}

		pr.Persuasion = persuasion(p.ID, votesByRound)
		if team == teamVillage {
			pr.Deduction = deduction(p.ID, votesByRound, m)
		} else {
			pr.Deception = deception(&pr, m.Rounds)
		}
		a.Players = append(a.Players, pr)
	}

	a.Highlights = detectHighlights(m, votesByRound)
	return a
}

// indexEvents groups votes by round and records when each player was eliminated.
func indexEvents(m *transcript.Match) (votesByRound map[int]map[string]string, elimRound map[string]int) {
	votesByRound = map[int]map[string]string{}
	elimRound = map[string]int{}
	for _, e := range m.Events {
		switch e.Type {
		case transcript.TypeVote:
			if votesByRound[e.Round] == nil {
				votesByRound[e.Round] = map[string]string{}
			}
			votesByRound[e.Round][e.Actor] = e.Target
		case transcript.TypeElimination:
			if e.Target != "" {
				elimRound[e.Target] = e.Round
			}
		}
	}
	return votesByRound, elimRound
}

// deduction is the fraction of a (village) player's cast votes that landed on an
// actual werewolf. Voting to eliminate wolves is the village's whole job.
func deduction(id string, votesByRound map[int]map[string]string, m *transcript.Match) *float64 {
	var onWolf, total int
	for _, votes := range votesByRound {
		t := votes[id]
		if t == "" {
			continue
		}
		total++
		if tp := m.PlayerByID(t); tp != nil && tp.Role == roleWerewolf {
			onWolf++
		}
	}
	if total == 0 {
		return nil
	}
	v := float64(onWolf) / float64(total)
	return &v
}

// persuasion is the average fraction of OTHER voters who matched this player's
// vote in the rounds it voted — a proxy for "did the table follow me." It is
// correlation, not proven causation, but it tracks influence well across many
// matches.
func persuasion(id string, votesByRound map[int]map[string]string) *float64 {
	var sum float64
	var n int
	for _, votes := range votesByRound {
		t := votes[id]
		if t == "" {
			continue
		}
		var others, agree int
		for voter, vt := range votes {
			if voter == id || vt == "" {
				continue
			}
			others++
			if vt == t {
				agree++
			}
		}
		if others > 0 {
			sum += float64(agree) / float64(others)
			n++
		}
	}
	if n == 0 {
		return nil
	}
	v := sum / float64(n)
	return &v
}

// deception measures how long a werewolf evaded the village: the fraction of the
// match's rounds it survived. A wolf that lasts to the end is deceiving well.
func deception(pr *PlayerResult, totalRounds int) *float64 {
	if totalRounds <= 0 {
		return nil
	}
	v := float64(pr.RoundsSurvived) / float64(totalRounds)
	if v > 1 {
		v = 1
	}
	return &v
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
