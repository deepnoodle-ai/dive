// Package game implements a provider-agnostic Werewolf engine: roles, the
// Night → Day → vote phase machine, legal-move validation, and win detection.
//
// The engine never imports an LLM provider (or even Dive) — players are plain
// string IDs injected by the caller. This keeps the rules independently
// unit-testable and lets the arena layer wire any model behind any player.
package game

import "math/rand"

// Role is a player's secret identity for the duration of a match.
type Role string

const (
	RoleWerewolf Role = "werewolf" // kills at night, wins at parity
	RoleSeer     Role = "seer"     // inspects one player each night
	RoleDoctor   Role = "doctor"   // protects one player each night
	RoleVillager Role = "villager" // no special power; deduces and votes
)

// Team is the faction a role belongs to. A match ends when one team's win
// condition is met.
type Team string

const (
	TeamVillage  Team = "village"
	TeamWerewolf Team = "werewolf"
)

// Team returns the faction this role fights for.
func (r Role) Team() Team {
	if r == RoleWerewolf {
		return TeamWerewolf
	}
	return TeamVillage
}

// numWolves scales the werewolf count with the table size. Small tables get a
// single wolf so villagers have a fighting chance; larger tables get more so
// the wolves can coordinate.
func numWolves(n int) int {
	switch {
	case n <= 5:
		return 1
	case n <= 8:
		return 2
	default:
		return n / 4
	}
}

// RoleComposition returns the multiset of roles for an n-player match, in a
// fixed (unshuffled) order: wolves first, then one seer, then one doctor (if
// there is room), then villagers. AssignRoles shuffles the result.
func RoleComposition(n int) []Role {
	roles := make([]Role, 0, n)
	for i := 0; i < numWolves(n); i++ {
		roles = append(roles, RoleWerewolf)
	}
	if len(roles) < n {
		roles = append(roles, RoleSeer)
	}
	// Only seat a doctor if at least one villager can still be dealt — a table
	// of all special roles makes for a degenerate game.
	if n-len(roles) > 1 {
		roles = append(roles, RoleDoctor)
	}
	for len(roles) < n {
		roles = append(roles, RoleVillager)
	}
	return roles
}

// AssignRoles returns a randomly-shuffled role for each of the n seats. The
// shuffle is driven by the supplied rng so a logged seed reproduces the exact
// role assignment — essential for fair, replayable matches.
func AssignRoles(n int, rng *rand.Rand) []Role {
	roles := RoleComposition(n)
	rng.Shuffle(len(roles), func(i, j int) {
		roles[i], roles[j] = roles[j], roles[i]
	})
	return roles
}
