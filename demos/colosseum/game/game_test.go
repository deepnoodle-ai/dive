package game

import (
	"math/rand"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestRoleComposition(t *testing.T) {
	cases := []struct {
		n       int
		wolves  int
		seers   int
		doctors int
	}{
		{3, 1, 1, 0}, // too small for a doctor
		{4, 1, 1, 1},
		{5, 1, 1, 1},
		{6, 2, 1, 1},
		{8, 2, 1, 1},
		{12, 3, 1, 1},
	}
	for _, c := range cases {
		roles := RoleComposition(c.n)
		assert.Equal(t, c.n, len(roles), "composition length for n=%d", c.n)
		counts := map[Role]int{}
		for _, r := range roles {
			counts[r]++
		}
		assert.Equal(t, c.wolves, counts[RoleWerewolf], "wolves for n=%d", c.n)
		assert.Equal(t, c.seers, counts[RoleSeer], "seers for n=%d", c.n)
		assert.Equal(t, c.doctors, counts[RoleDoctor], "doctors for n=%d", c.n)
		// Every seat gets exactly one role.
		assert.Equal(t, c.n, counts[RoleWerewolf]+counts[RoleSeer]+counts[RoleDoctor]+counts[RoleVillager])
	}
}

func TestRoleTeam(t *testing.T) {
	assert.Equal(t, TeamWerewolf, RoleWerewolf.Team())
	assert.Equal(t, TeamVillage, RoleSeer.Team())
	assert.Equal(t, TeamVillage, RoleDoctor.Team())
	assert.Equal(t, TeamVillage, RoleVillager.Team())
}

func TestAssignRolesDeterministic(t *testing.T) {
	a := AssignRoles(7, rand.New(rand.NewSource(42)))
	b := AssignRoles(7, rand.New(rand.NewSource(42)))
	assert.Equal(t, a, b, "same seed must produce identical role assignment")
}

func TestNewSeatsEveryone(t *testing.T) {
	ids := []string{"claude", "gpt", "gemini", "grok"}
	g, err := New(ids, 1)
	assert.NoError(t, err)
	assert.Equal(t, 4, len(g.Players))
	for i, p := range g.Players {
		assert.Equal(t, ids[i], p.ID)
		assert.True(t, p.Alive)
	}
	assert.Equal(t, 1, len(g.AliveWithRole(RoleWerewolf)))
	assert.Equal(t, PhaseNight, g.Phase)
	assert.Equal(t, 0, g.Round)
}

func TestNewRejectsBadRosters(t *testing.T) {
	_, err := New([]string{"a", "b"}, 1)
	assert.Error(t, err, "fewer than 3 players should be rejected")

	_, err = New([]string{"a", "b", "a"}, 1)
	assert.Error(t, err, "duplicate ids should be rejected")
}

// buildGame returns a game with an explicit, known role layout for testing the
// rules without relying on the shuffle.
func buildGame(roles map[string]Role) *Game {
	var players []*Player
	for _, id := range []string{"w", "s", "d", "v1", "v2"} {
		if r, ok := roles[id]; ok {
			players = append(players, &Player{ID: id, Role: r, Alive: true})
		}
	}
	return &Game{Players: players, Phase: PhaseNight, rng: rand.New(rand.NewSource(0))}
}

func TestVillageWinsWhenLastWolfDies(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "d": RoleDoctor, "v1": RoleVillager,
	})
	g.Eliminate("w")
	assert.True(t, g.IsOver())
	assert.Equal(t, TeamVillage, g.Winner)
}

func TestWolvesWinAtParity(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "d": RoleDoctor, "v1": RoleVillager,
	})
	// 1 wolf vs 3 villagers. Remove two villagers → 1 wolf vs 1 villager = parity.
	g.Eliminate("s")
	assert.False(t, g.IsOver())
	g.Eliminate("d")
	assert.True(t, g.IsOver())
	assert.Equal(t, TeamWerewolf, g.Winner)
}

func TestEliminateUnknownOrDeadIsNoop(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "v1": RoleVillager,
	})
	g.Eliminate("nobody")
	assert.Equal(t, 3, len(g.Alive()))
	g.Eliminate("s")
	g.Eliminate("s") // already dead
	assert.Equal(t, 2, len(g.Alive()))
}

func TestResolveNightKill(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "d": RoleDoctor, "v1": RoleVillager,
	})
	out := g.ResolveNight("v1", "s") // wolf kills v1, doctor guards s
	assert.Equal(t, "v1", out.KilledID)
	assert.False(t, out.Saved)
	assert.False(t, g.ByID("v1").Alive)
}

func TestResolveNightDoctorSaves(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "d": RoleDoctor, "v1": RoleVillager,
	})
	out := g.ResolveNight("v1", "v1") // doctor guards the wolves' target
	assert.True(t, out.Saved)
	assert.Equal(t, "", out.KilledID)
	assert.True(t, g.ByID("v1").Alive)
}

func TestResolveNightWolvesAbstain(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "v1": RoleVillager,
	})
	out := g.ResolveNight("", "s")
	assert.Equal(t, "", out.KilledID)
	assert.False(t, out.Saved)
	assert.Equal(t, 3, len(g.Alive()))
}

func TestTallyVotes(t *testing.T) {
	out := TallyVotes(map[string]string{
		"a": "x", "b": "x", "c": "y",
	})
	assert.Equal(t, "x", out.EliminatedID)
	assert.False(t, out.Tie)
	assert.Equal(t, 2, out.Tally["x"])
	assert.Equal(t, 1, out.Tally["y"])
}

func TestTallyVotesTie(t *testing.T) {
	out := TallyVotes(map[string]string{
		"a": "x", "b": "y",
	})
	assert.True(t, out.Tie)
	assert.Equal(t, "", out.EliminatedID)
}

func TestTallyVotesAbstentions(t *testing.T) {
	out := TallyVotes(map[string]string{
		"a": "", "b": "", "c": "x",
	})
	assert.Equal(t, "x", out.EliminatedID)

	empty := TallyVotes(map[string]string{"a": "", "b": ""})
	assert.Equal(t, "", empty.EliminatedID)
	assert.False(t, empty.Tie)
}

func TestValidVoteTarget(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "v1": RoleVillager,
	})
	assert.NoError(t, g.ValidVoteTarget("w"))
	assert.Error(t, g.ValidVoteTarget("ghost"))
	g.Eliminate("v1")
	assert.Error(t, g.ValidVoteTarget("v1"))
}

func TestValidKillTarget(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "v1": RoleVillager,
	})
	assert.NoError(t, g.ValidKillTarget("s"))
	assert.Error(t, g.ValidKillTarget("w"), "cannot attack a fellow wolf")
	assert.Error(t, g.ValidKillTarget("ghost"))
}

func TestValidInspectTarget(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "v1": RoleVillager,
	})
	assert.NoError(t, g.ValidInspectTarget("s", "w"))
	assert.Error(t, g.ValidInspectTarget("s", "s"), "cannot inspect self")
	assert.Error(t, g.ValidInspectTarget("s", "ghost"))
}

func TestValidProtectTarget(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "d": RoleDoctor, "v1": RoleVillager,
	})
	assert.NoError(t, g.ValidProtectTarget("d"), "doctor may protect self")
	assert.NoError(t, g.ValidProtectTarget("v1"))
	g.Eliminate("v1")
	assert.Error(t, g.ValidProtectTarget("v1"))
}

func TestIsWerewolf(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "v1": RoleVillager,
	})
	assert.True(t, g.IsWerewolf("w"))
	assert.False(t, g.IsWerewolf("s"))
	assert.False(t, g.IsWerewolf("ghost"))
}

func TestPhaseTransitions(t *testing.T) {
	g, err := New([]string{"a", "b", "c", "d"}, 7)
	assert.NoError(t, err)
	assert.Equal(t, 0, g.Round)

	g.BeginRound()
	assert.Equal(t, 1, g.Round)
	assert.Equal(t, PhaseNight, g.Phase)

	g.BeginDay()
	assert.Equal(t, PhaseDay, g.Phase)

	g.BeginRound()
	assert.Equal(t, 2, g.Round)
	assert.Equal(t, PhaseNight, g.Phase)
}

func TestEndedGameIgnoresTransitions(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "v1": RoleVillager,
	})
	g.Eliminate("w")
	assert.True(t, g.IsOver())
	g.BeginRound()
	assert.Equal(t, PhaseEnded, g.Phase, "ended game must stay ended")
	g.BeginDay()
	assert.Equal(t, PhaseEnded, g.Phase)
}

// A full scripted match exercises the engine end-to-end: night kills, a doctor
// save, a day vote, and a village victory.
func TestScriptedMatchVillageVictory(t *testing.T) {
	g := buildGame(map[string]Role{
		"w": RoleWerewolf, "s": RoleSeer, "d": RoleDoctor, "v1": RoleVillager, "v2": RoleVillager,
	})

	// Round 1 night: wolf attacks v1, doctor guards v1 → saved.
	g.BeginRound()
	out := g.ResolveNight("v1", "v1")
	assert.True(t, out.Saved)
	assert.False(t, g.IsOver())

	// Round 1 day: seer (correctly) rallies the village against the wolf.
	g.BeginDay()
	vo := TallyVotes(map[string]string{"s": "w", "d": "w", "v1": "w", "v2": "s", "w": "v2"})
	assert.Equal(t, "w", vo.EliminatedID)
	g.Eliminate(vo.EliminatedID)

	assert.True(t, g.IsOver())
	assert.Equal(t, TeamVillage, g.Winner)
}
