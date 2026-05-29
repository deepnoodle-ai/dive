package game

import (
	"fmt"
	"math/rand"
	"sort"
)

// Phase identifies what the table is currently doing.
type Phase string

const (
	PhaseNight Phase = "night" // werewolves kill; seer inspects; doctor protects
	PhaseDay   Phase = "day"   // discussion followed by a public vote
	PhaseEnded Phase = "ended" // a team has won
)

// Player is one seat at the table. ID is a stable, human-readable handle (e.g.
// "claude" or "grok-2") chosen by the caller; the engine treats it as opaque.
type Player struct {
	ID    string
	Role  Role
	Alive bool
}

// Game holds the full mutable state of a single match. It is not safe for
// concurrent mutation — the arena drives one action at a time.
type Game struct {
	Players []*Player
	Round   int   // 0 before the first night; incremented by BeginRound
	Phase   Phase // current phase
	Winner  Team  // set when Phase == PhaseEnded; "" while the game is live
	Seed    int64 // the RNG seed, recorded for reproducibility

	rng *rand.Rand
}

// New deals roles to the given player IDs and returns a game ready for its
// first round. The seed makes role assignment (and any later tie-breaks)
// reproducible: the same IDs and seed always yield the same match setup.
func New(playerIDs []string, seed int64) (*Game, error) {
	if len(playerIDs) < 3 {
		return nil, fmt.Errorf("need at least 3 players, got %d", len(playerIDs))
	}
	if dup := firstDuplicate(playerIDs); dup != "" {
		return nil, fmt.Errorf("duplicate player id %q", dup)
	}
	rng := rand.New(rand.NewSource(seed))
	roles := AssignRoles(len(playerIDs), rng)
	players := make([]*Player, len(playerIDs))
	for i, id := range playerIDs {
		players[i] = &Player{ID: id, Role: roles[i], Alive: true}
	}
	return &Game{
		Players: players,
		Round:   0,
		Phase:   PhaseNight,
		Seed:    seed,
		rng:     rng,
	}, nil
}

func firstDuplicate(ids []string) string {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return id
		}
		seen[id] = true
	}
	return ""
}

// ByID returns the player with the given ID, or nil if there is none.
func (g *Game) ByID(id string) *Player {
	for _, p := range g.Players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Alive returns the living players in seating order.
func (g *Game) Alive() []*Player {
	var out []*Player
	for _, p := range g.Players {
		if p.Alive {
			out = append(out, p)
		}
	}
	return out
}

// AliveIDs returns the IDs of living players in seating order.
func (g *Game) AliveIDs() []string {
	var out []string
	for _, p := range g.Alive() {
		out = append(out, p.ID)
	}
	return out
}

// AliveWithRole returns living players holding the given role.
func (g *Game) AliveWithRole(role Role) []*Player {
	var out []*Player
	for _, p := range g.Alive() {
		if p.Role == role {
			out = append(out, p)
		}
	}
	return out
}

// countAlive returns the number of living wolves and living non-wolves.
func (g *Game) countAlive() (wolves, villagers int) {
	for _, p := range g.Alive() {
		if p.Role == RoleWerewolf {
			wolves++
		} else {
			villagers++
		}
	}
	return wolves, villagers
}

// IsOver reports whether the match has ended.
func (g *Game) IsOver() bool { return g.Phase == PhaseEnded }

// checkWin updates Winner/Phase if a win condition is now met. Villagers win
// when the last wolf is gone; wolves win once they reach numerical parity with
// the rest of the table (at parity they can no longer be voted out).
func (g *Game) checkWin() {
	if g.IsOver() {
		return
	}
	wolves, villagers := g.countAlive()
	switch {
	case wolves == 0:
		g.Winner = TeamVillage
		g.Phase = PhaseEnded
	case wolves >= villagers:
		g.Winner = TeamWerewolf
		g.Phase = PhaseEnded
	}
}

// BeginRound advances to the next round's Night phase. It is a no-op once the
// game has ended.
func (g *Game) BeginRound() {
	if g.IsOver() {
		return
	}
	g.Round++
	g.Phase = PhaseNight
}

// BeginDay moves an in-progress round from Night to Day. It is a no-op once the
// game has ended (e.g. the night kill triggered a win).
func (g *Game) BeginDay() {
	if g.IsOver() {
		return
	}
	g.Phase = PhaseDay
}

// Eliminate marks a player dead and re-checks win conditions. Eliminating an
// unknown or already-dead player is a no-op.
func (g *Game) Eliminate(id string) {
	p := g.ByID(id)
	if p == nil || !p.Alive {
		return
	}
	p.Alive = false
	g.checkWin()
}

// --- Legal-move validation (used by the referee hook) ---------------------

// ValidVoteTarget reports whether a living player may be voted out. A player
// may vote for any living player, including themselves.
func (g *Game) ValidVoteTarget(target string) error {
	p := g.ByID(target)
	if p == nil {
		return fmt.Errorf("there is no player named %q", target)
	}
	if !p.Alive {
		return fmt.Errorf("%s is already dead and cannot be voted out", target)
	}
	return nil
}

// ValidKillTarget reports whether the wolves may attack the given player: the
// target must be alive and must not themselves be a wolf.
func (g *Game) ValidKillTarget(target string) error {
	p := g.ByID(target)
	if p == nil {
		return fmt.Errorf("there is no player named %q", target)
	}
	if !p.Alive {
		return fmt.Errorf("%s is already dead", target)
	}
	if p.Role == RoleWerewolf {
		return fmt.Errorf("%s is a fellow werewolf; choose a non-wolf to attack", target)
	}
	return nil
}

// ValidInspectTarget reports whether the seer may inspect the given player: a
// living player other than the seer themselves.
func (g *Game) ValidInspectTarget(seerID, target string) error {
	if target == seerID {
		return fmt.Errorf("you already know your own role; inspect someone else")
	}
	p := g.ByID(target)
	if p == nil {
		return fmt.Errorf("there is no player named %q", target)
	}
	if !p.Alive {
		return fmt.Errorf("%s is already dead; inspecting them tells you nothing", target)
	}
	return nil
}

// ValidProtectTarget reports whether the doctor may protect the given player:
// any living player, including the doctor themselves.
func (g *Game) ValidProtectTarget(target string) error {
	p := g.ByID(target)
	if p == nil {
		return fmt.Errorf("there is no player named %q", target)
	}
	if !p.Alive {
		return fmt.Errorf("%s is already dead and cannot be protected", target)
	}
	return nil
}

// IsWerewolf reports whether the named player holds the werewolf role. It is
// the seer's inspection primitive; the caller is responsible for only revealing
// the result to the seer.
func (g *Game) IsWerewolf(id string) bool {
	p := g.ByID(id)
	return p != nil && p.Role == RoleWerewolf
}

// --- Night & vote resolution ----------------------------------------------

// NightOutcome reports the result of resolving a night's actions.
type NightOutcome struct {
	KilledID string // player killed, or "" if nobody died
	Saved    bool   // true if the doctor's protection cancelled the kill
}

// ResolveNight applies the wolves' chosen kill, cancelled if the doctor
// protected the same target. An empty wolfTarget means the wolves abstained.
// The outcome is applied to the game (and may trigger a win).
func (g *Game) ResolveNight(wolfTarget, doctorTarget string) NightOutcome {
	if wolfTarget == "" {
		return NightOutcome{}
	}
	if wolfTarget == doctorTarget {
		return NightOutcome{Saved: true}
	}
	g.Eliminate(wolfTarget)
	return NightOutcome{KilledID: wolfTarget}
}

// VoteOutcome reports the result of tallying a day's votes.
type VoteOutcome struct {
	EliminatedID string         // player eliminated, or "" on a tie / no votes
	Tally        map[string]int // votes received, by player ID
	Tie          bool           // true if the top vote count was shared
}

// TallyVotes counts a voter→target map and decides who (if anyone) is
// eliminated. Abstentions are encoded as an empty target and ignored. A tie for
// the most votes results in no elimination (Tie == true), which keeps the
// engine deterministic — no coin flip decides a life.
func TallyVotes(votes map[string]string) VoteOutcome {
	tally := make(map[string]int)
	for _, target := range votes {
		if target == "" {
			continue
		}
		tally[target]++
	}
	if len(tally) == 0 {
		return VoteOutcome{Tally: tally}
	}

	// Find the maximum vote count, then how many players share it.
	max := 0
	for _, n := range tally {
		if n > max {
			max = n
		}
	}
	var leaders []string
	for id, n := range tally {
		if n == max {
			leaders = append(leaders, id)
		}
	}
	sort.Strings(leaders) // stable reporting regardless of map order
	if len(leaders) != 1 {
		return VoteOutcome{Tally: tally, Tie: true}
	}
	return VoteOutcome{EliminatedID: leaders[0], Tally: tally}
}

// Intn exposes the match RNG for deterministic tie-breaks the arena needs
// (e.g. choosing among wolves who disagree on a target). Using the seeded RNG
// keeps the whole match reproducible from its seed.
func (g *Game) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	return g.rng.Intn(n)
}
