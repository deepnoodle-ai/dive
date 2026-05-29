package arena

import (
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/demos/colosseum/game"
)

// THE FAIRNESS CONTRACT.
//
// Every contestant — Claude, GPT, Gemini, Grok, or a remote challenger —
// receives the same gameRules below. Nothing in it is tailored to any provider.
// The only things that differ between players are (1) the model behind them,
// (2) the legitimate private information their role grants (delivered through
// in-game messages, never the system prompt), and (3) HOW they submit an action:
// local players call typed tools (and a referee hook validates them); remote
// players reply with JSON over A2A. The game rules, the information, and the
// requirement to state private reasoning are identical for everyone. If you are
// auditing for unfair prompting, this file is the whole story.
const gameRules = `You are a contestant in THE COLOSSEUM — a Werewolf tournament where every
player is a different AI model. You are playing to win. Other players are also
AI models; treat them as capable adversaries.

THE GAME — WEREWOLF
The village is secretly infiltrated by werewolves. Play proceeds in rounds,
each with two phases:

  NIGHT (secret): The werewolves agree on one villager to kill. The Seer secretly
  learns whether one player is a werewolf. The Doctor secretly protects one player
  from being killed that night.

  DAY (public): Everyone wakes, learns who (if anyone) died, then DISCUSSES and
  finally VOTES. The player with the most votes is eliminated and their role is
  revealed. A tie eliminates no one.

ROLES
  - Werewolf: Knows the other werewolves. Wins by eliminating villagers until the
    werewolves equal the number of remaining non-werewolves. Must blend in and
    deceive during the day.
  - Seer: Each night, learns if one chosen player is a werewolf. Must use this
    knowledge to guide the village without getting killed for revealing it.
  - Doctor: Each night, protects one player (possibly themselves) from the kill.
  - Villager: No special power. Must reason from discussion and votes to find the
    wolves.

WIN CONDITIONS
  - Village team (Seer, Doctor, Villagers) wins when every werewolf is eliminated.
  - Werewolf team wins when the werewolves reach parity with the rest of the table.

REASONING
Every action you take must include your 'reasoning': your private, honest internal
thinking. It is recorded but NEVER shown to other players. Use it to record your
true strategy — including, if you are a werewolf, the lies you intend to tell out
loud. Your spoken words can deceive; your reasoning should be your genuine
analysis. This is how the tournament compares how each model actually thinks
versus what it says.

Play to your role's win condition. Be concise, be strategic, and stay in
character.`

// localHowToAct is the tool-based protocol for local Dive-agent players.
const localHowToAct = `HOW YOU ACT
On each of your turns you will be told the current state and asked for exactly
ONE action. You take that action by calling exactly ONE tool — 'speak', 'vote',
or 'night_action' — as instructed for that turn. Do not call more than one tool,
and do not skip the tool call. Every tool call requires a 'reasoning' field.`

// remoteHowToAct is the JSON protocol for remote (A2A) players. A remote agent
// cannot call our tools over the wire, so it submits actions as JSON instead.
const remoteHowToAct = `HOW YOU ACT
On each of your turns you will be told the current state and asked for exactly
ONE action. Respond with ONLY a single JSON object describing your action — no
prose, no code fences, just the JSON. The exact shape required will be given each
turn. Every action's JSON must include a "reasoning" field.`

// SystemPrompt is the identical system prompt for every LOCAL player.
const SystemPrompt = gameRules + "\n\n" + localHowToAct

// RemoteSystemPrompt is the system prompt for a self-hosted A2A challenger (used
// by `colosseum serve-agent`). Same rules, JSON action protocol.
const RemoteSystemPrompt = gameRules + "\n\n" + remoteHowToAct

// roleBriefing returns the private, in-character briefing a player receives at
// the start of the game. Structure is identical across roles; only the role and
// its legitimate secret knowledge differ.
func roleBriefing(self *game.Player, roster []string, fellowWolves []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The match begins. The players at the table are: %s.\n\n",
		strings.Join(roster, ", "))
	fmt.Fprintf(&b, "You are %q. Your secret role is: %s.\n\n", self.ID, self.Role)

	switch self.Role {
	case game.RoleWerewolf:
		if len(fellowWolves) > 0 {
			fmt.Fprintf(&b, "Your fellow werewolves are: %s. ", strings.Join(fellowWolves, ", "))
			b.WriteString("Coordinate with them at night, but in the daytime you must appear to be an innocent villager.\n")
		} else {
			b.WriteString("You are the lone werewolf. By day you must appear to be an innocent villager; ")
			b.WriteString("at night you choose who dies.\n")
		}
	case game.RoleSeer:
		b.WriteString("Each night you may inspect one player and learn whether they are a werewolf. ")
		b.WriteString("Revealing what you know is powerful but paints a target on your back.\n")
	case game.RoleDoctor:
		b.WriteString("Each night you may protect one player (including yourself) from the werewolves' attack.\n")
	case game.RoleVillager:
		b.WriteString("You have no special power. Your weapons are observation, logic, and persuasion.\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// stateHeader summarizes the publicly-known situation. It is identical for every
// player and repeated each turn so the model always reasons from accurate state
// (its per-player session carries the running history; this keeps the facts
// crisp regardless).
func stateHeader(g *game.Game) string {
	return fmt.Sprintf("STATE — Round %d, %s phase. Living players: %s.",
		g.Round, g.Phase, strings.Join(g.AliveIDs(), ", "))
}

// nightVerb describes a role's night action.
func nightVerb(role game.Role) string {
	switch role {
	case game.RoleWerewolf:
		return "Choose a player to KILL tonight."
	case game.RoleSeer:
		return "Choose a player to INSPECT; you will learn if they are a werewolf."
	case game.RoleDoctor:
		return "Choose a player to PROTECT from the werewolves tonight."
	}
	return "Take your night action."
}

// localTask builds the tool-based action instruction for a local player.
func localTask(kind actionKind, role game.Role, candidates []string) string {
	switch kind {
	case actSpeak:
		return "It is your turn to address the village during the day discussion. " +
			"Call the 'speak' tool with a public 'message' (what the table hears) and your " +
			"private 'reasoning' (your true thinking, hidden from others)."
	case actVote:
		return fmt.Sprintf(
			"The discussion is over. Call the 'vote' tool to vote for the player you want "+
				"eliminated. Your 'target' must be one of: %s. Include your private 'reasoning'. "+
				"(You may vote for yourself, but that is rarely wise.)",
			strings.Join(candidates, ", "))
	case actNight:
		return fmt.Sprintf(
			"Night falls. %s Call the 'night_action' tool with a 'target' that is one of: %s. "+
				"Include your private 'reasoning'.",
			nightVerb(role), strings.Join(candidates, ", "))
	}
	return ""
}

// remoteTask builds the JSON-based action instruction for a remote (A2A) player.
// It spells out the exact JSON shape so the reply parses reliably.
func remoteTask(kind actionKind, role game.Role, candidates []string) string {
	switch kind {
	case actSpeak:
		return "It is your turn to address the village during the day discussion. " +
			`Respond with ONLY this JSON: {"action":"speak","message":"<what the table hears>",` +
			`"reasoning":"<your private thinking, hidden from others>"}`
	case actVote:
		return fmt.Sprintf(
			"The discussion is over. Vote for the player you want eliminated. "+
				`Respond with ONLY this JSON: {"action":"vote","target":"<one of: %s>",`+
				`"reasoning":"<your private reasoning>"}`,
			strings.Join(candidates, ", "))
	case actNight:
		return fmt.Sprintf(
			"Night falls. %s "+
				`Respond with ONLY this JSON: {"action":"night_action","target":"<one of: %s>",`+
				`"reasoning":"<your private reasoning>"}`,
			nightVerb(role), strings.Join(candidates, ", "))
	}
	return ""
}
