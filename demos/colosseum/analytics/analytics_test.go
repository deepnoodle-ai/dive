package analytics

import (
	"sort"
	"testing"

	"github.com/deepnoodle-ai/dive/demos/colosseum/transcript"
	"github.com/deepnoodle-ai/wonton/assert"
)

// buildMatch assembles a parsed Match from roles + a list of events for testing.
func buildMatch(roles map[string]string, winner string, survivors []string, rounds int, events []transcript.Event) *transcript.Match {
	m := &transcript.Match{
		Winner: winner, Survivors: survivors, Rounds: rounds, Complete: true,
		Usage: map[string]transcript.Usage{},
	}
	for id, role := range roles {
		m.Players = append(m.Players, transcript.PlayerInfo{ID: id, Provider: id, Model: "model-" + id, Role: role})
	}
	// Sort by id so player order is deterministic, mirroring transcript.Parse.
	sort.Slice(m.Players, func(i, j int) bool { return m.Players[i].ID < m.Players[j].ID })
	m.Events = events
	return m
}

func vote(round int, actor, target string) transcript.Event {
	return transcript.Event{Type: transcript.TypeVote, Round: round, Actor: actor, Target: target}
}
func elim(round int, target, role, cause string) transcript.Event {
	return transcript.Event{Type: transcript.TypeElimination, Round: round, Target: target, Role: role,
		Data: map[string]any{"cause": cause}}
}

func resultByID(a *MatchAnalysis, id string) *PlayerResult {
	for i := range a.Players {
		if a.Players[i].ID == id {
			return &a.Players[i]
		}
	}
	return nil
}

func TestAnalyzeBasicOutcome(t *testing.T) {
	roles := map[string]string{"w": "werewolf", "s": "seer", "d": "doctor", "v": "villager"}
	events := []transcript.Event{
		vote(1, "s", "w"), vote(1, "d", "w"), vote(1, "v", "w"), vote(1, "w", "v"),
		elim(1, "w", "werewolf", "vote"),
	}
	a := Analyze(buildMatch(roles, "village", []string{"s", "d", "v"}, 1, events))

	assert.Equal(t, "village", a.Winner)
	s := resultByID(a, "s")
	assert.NotNil(t, s)
	assert.True(t, s.Won)
	assert.True(t, s.Survived)
	assert.Equal(t, "village", s.Team)

	w := resultByID(a, "w")
	assert.False(t, w.Won)
	assert.False(t, w.Survived)
}

func TestDeduction(t *testing.T) {
	roles := map[string]string{"w": "werewolf", "s": "seer", "v": "villager"}
	// s votes the wolf both rounds (perfect); v votes a villager then the wolf (0.5).
	events := []transcript.Event{
		vote(1, "s", "w"), vote(1, "v", "s"),
		vote(2, "s", "w"), vote(2, "v", "w"),
	}
	a := Analyze(buildMatch(roles, "village", []string{"s", "v"}, 2, events))

	s := resultByID(a, "s")
	assert.NotNil(t, s.Deduction)
	assert.Equal(t, 1.0, *s.Deduction)

	v := resultByID(a, "v")
	assert.NotNil(t, v.Deduction)
	assert.Equal(t, 0.5, *v.Deduction)

	// Wolves get no deduction metric (they have a deception metric instead).
	w := resultByID(a, "w")
	assert.Nil(t, w.Deduction)
	assert.NotNil(t, w.Deception)
}

func TestPersuasion(t *testing.T) {
	roles := map[string]string{"a": "villager", "b": "villager", "c": "werewolf"}
	// Round 1: a votes c; b and c both also vote c → all others agree with a (1.0).
	events := []transcript.Event{
		vote(1, "a", "c"), vote(1, "b", "c"), vote(1, "c", "c"),
	}
	a := Analyze(buildMatch(roles, "village", []string{"a", "b"}, 1, events))
	pa := resultByID(a, "a")
	assert.NotNil(t, pa.Persuasion)
	assert.Equal(t, 1.0, *pa.Persuasion)
}

func TestDeceptionFraction(t *testing.T) {
	roles := map[string]string{"w": "werewolf", "v1": "villager", "v2": "villager"}
	// Wolf eliminated in round 2 of a 4-round match → survived 2/4 = 0.5.
	events := []transcript.Event{elim(2, "w", "werewolf", "vote")}
	a := Analyze(buildMatch(roles, "village", []string{"v1", "v2"}, 4, events))
	w := resultByID(a, "w")
	assert.NotNil(t, w.Deception)
	assert.Equal(t, 0.5, *w.Deception)
	assert.Equal(t, 2, w.RoundsSurvived)
}

func TestHighlightSeerIgnoredAndLoneWolf(t *testing.T) {
	roles := map[string]string{"w": "werewolf", "s": "seer", "v": "villager"}
	events := []transcript.Event{
		{Type: transcript.TypeSeerResult, Round: 1, Actor: "s", Target: "w",
			Data: map[string]any{"is_werewolf": true}},
		// village never lynches w; wolves win
	}
	a := Analyze(buildMatch(roles, "werewolf", []string{"w"}, 2, events))

	types := map[string]bool{}
	for _, h := range a.Highlights {
		types[h.Type] = true
	}
	assert.True(t, types[HLSeerIgnored], "seer-ignored should fire")
	assert.True(t, types[HLLoneWolf], "lone-wolf win should fire")
}

func TestHighlightMislynchAndDoctorSave(t *testing.T) {
	roles := map[string]string{"w": "werewolf", "s": "seer", "d": "doctor", "v": "villager"}
	events := []transcript.Event{
		{Type: transcript.TypeProtected, Round: 1},
		elim(1, "s", "seer", "vote"), // town lynches its own seer
	}
	a := Analyze(buildMatch(roles, "werewolf", []string{"w", "d"}, 2, events))
	types := map[string]bool{}
	for _, h := range a.Highlights {
		types[h.Type] = true
	}
	assert.True(t, types[HLDoctorSave])
	assert.True(t, types[HLMislynch])
}

func TestLeaderboardEloAndAggregates(t *testing.T) {
	lb := NewLeaderboard()
	roles := map[string]string{"w": "werewolf", "s": "seer", "v": "villager"}
	events := []transcript.Event{
		vote(1, "s", "w"), vote(1, "v", "w"), vote(1, "w", "v"),
		elim(1, "w", "werewolf", "vote"),
	}
	a := Analyze(buildMatch(roles, "village", []string{"s", "v"}, 1, events))
	lb.Update(a)

	// Winners' models should rise above 1000, the loser's should fall.
	sWin := lb.Ratings["model-s"]
	wLose := lb.Ratings["model-w"]
	assert.NotNil(t, sWin)
	assert.NotNil(t, wLose)
	assert.Greater(t, sWin.Elo, DefaultElo)
	assert.Less(t, wLose.Elo, DefaultElo)
	assert.Equal(t, 1, sWin.Wins)
	assert.Equal(t, 1, wLose.GamesAsWolf)
	assert.Equal(t, 0, wLose.WolfWins)

	standings := lb.Standings()
	assert.Equal(t, 3, len(standings))
	// The lynched wolf is decisively last; the two village winners lead it.
	assert.Equal(t, "model-w", standings[2].Model)
	assert.Greater(t, standings[0].Elo, standings[2].Elo)
}

func TestLeaderboardSaveLoad(t *testing.T) {
	lb := NewLeaderboard()
	roles := map[string]string{"w": "werewolf", "s": "seer", "v": "villager"}
	a := Analyze(buildMatch(roles, "village", []string{"s", "v"}, 1, []transcript.Event{
		elim(1, "w", "werewolf", "vote"),
	}))
	lb.Update(a)

	path := t.TempDir() + "/lb.json"
	assert.NoError(t, lb.Save(path))

	loaded, err := LoadLeaderboard(path)
	assert.NoError(t, err)
	assert.Equal(t, lb.Ratings["model-s"].Elo, loaded.Ratings["model-s"].Elo)
	assert.Equal(t, 1, loaded.Ratings["model-w"].GamesAsWolf)

	// Missing file → fresh empty leaderboard, no error.
	fresh, err := LoadLeaderboard(t.TempDir() + "/does-not-exist.json")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(fresh.Ratings))
}
