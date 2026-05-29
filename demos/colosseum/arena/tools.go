package arena

import "github.com/deepnoodle-ai/dive/demos/colosseum/game"

// actionKind identifies which action a player is being asked to take this turn.
type actionKind string

const (
	actSpeak actionKind = "speak"
	actVote  actionKind = "vote"
	actNight actionKind = "night"
)

// toolName returns the tool/action name a kind maps to, used by both the local
// (tool-call) and remote (JSON) protocols.
func (k actionKind) toolName() string {
	switch k {
	case actSpeak:
		return "speak"
	case actVote:
		return "vote"
	case actNight:
		return "night_action"
	}
	return string(k)
}

// The action tool inputs. Each REQUIRES a 'reasoning' field so we capture
// private thinking comparably across providers, rather than depending on any
// one provider's thinking channel. `json` tags drive the auto-generated schema;
// `description` tags become the field docs the model sees.
type SpeakInput struct {
	Message   string `json:"message" description:"What you say out loud to the whole village. This is public; everyone hears it."`
	Reasoning string `json:"reasoning" description:"Your private internal reasoning. Recorded but never shown to other players. Be honest here even if your message is a bluff."`
}

type VoteInput struct {
	Target    string `json:"target" description:"The exact id of the living player you vote to eliminate."`
	Reasoning string `json:"reasoning" description:"Your private internal reasoning for this vote. Recorded but never shown to other players."`
}

type NightActionInput struct {
	Target    string `json:"target" description:"The exact id of the player you target with your night action (kill, inspect, or protect, per your role)."`
	Reasoning string `json:"reasoning" description:"Your private internal reasoning for this night action. Recorded but never shown to other players."`
}

// capturedAction is the validated action a player submitted this turn.
type capturedAction struct {
	kind      actionKind
	target    string
	message   string
	reasoning string
}

// turn describes the single action a player is being asked to take.
type turn struct {
	kind       actionKind
	role       game.Role
	candidates []string // legal targets ([] for speak)
}
