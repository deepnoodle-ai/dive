package arena

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/demos/colosseum/game"
	"github.com/deepnoodle-ai/dive/demos/colosseum/transcript"
	"github.com/deepnoodle-ai/dive/llm"
)

// Contestant describes one seat entering the arena. A local seat carries an
// LLM; a remote seat (an agent reached over A2A) carries a RemoteURL instead.
type Contestant struct {
	ID        string  // unique seat id, e.g. "claude" or "grok-2"
	Provider  string  // provider key, e.g. "grok", or "a2a" for remote seats
	Model     string  // resolved model id, for logging
	LLM       llm.LLM // the model behind a local seat
	RemoteURL string  // A2A endpoint for a remote seat (mutually exclusive with LLM)
}

// Options configures a match.
type Options struct {
	Seed             int64         // RNG seed (logged for reproducibility)
	MaxRounds        int           // safety cap on rounds (default 8)
	DiscussionRounds int           // speaking passes per day (default 1)
	TurnTimeout      time.Duration // per-player-turn timeout (default 90s)
	MaxRetries       int           // transient-failure retries per turn (default 2)
	Reveal           bool          // print private reasoning to the terminal live
	Out              io.Writer     // terminal output (default os.Stdout)
	Transcript       io.Writer     // JSONL transcript sink (required)
}

func (o *Options) applyDefaults() {
	if o.MaxRounds <= 0 {
		o.MaxRounds = 8
	}
	if o.DiscussionRounds <= 0 {
		o.DiscussionRounds = 1
	}
	if o.TurnTimeout <= 0 {
		o.TurnTimeout = 90 * time.Second
	}
	if o.MaxRetries < 0 {
		o.MaxRetries = 0
	}
}

// GameMaster runs a full match: it owns the engine, the players, the
// transcript, and per-player message delivery.
type GameMaster struct {
	game    *game.Game
	players map[string]*Player
	order   []string // seating order (stable)
	rec     *transcript.Writer
	ui      *console
	opts    Options

	mu sync.Mutex // guards the message-delivery state below

	// Per-player message delivery. Each player has a private session, so we
	// feed it only what it legitimately knows: new public events since its last
	// turn (deliveredPublic cursor) plus any private notes queued for it.
	briefed         map[string]bool
	deliveredPublic map[string]int
	privateQueue    map[string][]string
	publicLog       []string
}

// New builds a game master for the given contestants and options. The context
// is used to establish connections to any remote (A2A) seats.
func New(ctx context.Context, contestants []Contestant, opts Options) (*GameMaster, error) {
	if opts.Transcript == nil {
		return nil, fmt.Errorf("a transcript writer is required")
	}
	opts.applyDefaults()

	ids := make([]string, len(contestants))
	for i, c := range contestants {
		ids[i] = c.ID
	}
	g, err := game.New(ids, opts.Seed)
	if err != nil {
		return nil, err
	}

	gm := &GameMaster{
		game:            g,
		players:         make(map[string]*Player, len(contestants)),
		order:           ids,
		rec:             transcript.NewWriter(opts.Transcript),
		ui:              newConsole(opts.Out),
		opts:            opts,
		briefed:         make(map[string]bool),
		deliveredPublic: make(map[string]int),
		privateQueue:    make(map[string][]string),
	}
	pol := policy{turnTimeout: opts.TurnTimeout, maxRetries: opts.MaxRetries}
	warn := func(s string) { gm.ui.warn(s) }
	for _, c := range contestants {
		p, err := newPlayer(ctx, c, pol, warn)
		if err != nil {
			return nil, err
		}
		gm.players[c.ID] = p
	}
	return gm, nil
}

// Run plays the match to completion and returns the winning team (or "" if the
// round limit was reached with no decisive winner).
func (gm *GameMaster) Run(ctx context.Context) (game.Team, error) {
	gm.recordMatchStart()

	for !gm.game.IsOver() && gm.game.Round < gm.opts.MaxRounds {
		gm.game.BeginRound()
		gm.runNight(ctx)
		if gm.game.IsOver() {
			break
		}
		gm.game.BeginDay()
		gm.runDay(ctx)
	}

	return gm.recordMatchEnd()
}

// --- Night ----------------------------------------------------------------

func (gm *GameMaster) runNight(ctx context.Context) {
	g := gm.game
	gm.ui.night(g.Round)
	gm.rec.Write(transcript.Event{Type: transcript.TypePhaseStart, Round: g.Round, Phase: string(game.PhaseNight)})
	gm.ui.info("The village sleeps. Secret actions are being taken…")

	wolfTarget := gm.runWolves(ctx)
	gm.runSeer(ctx)
	doctorTarget := gm.runDoctor(ctx)

	outcome := g.ResolveNight(wolfTarget, doctorTarget)
	switch {
	case outcome.KilledID != "":
		victim := g.ByID(outcome.KilledID)
		line := fmt.Sprintf("%s was found dead at dawn — killed in the night. They were a %s.",
			victim.ID, victim.Role)
		gm.addPublic(line)
		gm.ui.death(line)
		gm.rec.Write(transcript.Event{
			Type: transcript.TypeElimination, Round: g.Round, Phase: string(game.PhaseNight),
			Target: victim.ID, Role: string(victim.Role), Public: line,
			Data: map[string]any{"cause": "werewolf_kill"},
		})
	case outcome.Saved:
		line := "Screams in the night — but at dawn, everyone is still alive."
		gm.addPublic(line)
		gm.ui.info(line)
		gm.rec.Write(transcript.Event{
			Type: transcript.TypeProtected, Round: g.Round, Phase: string(game.PhaseNight), Public: line,
		})
	default:
		line := "The night passes quietly. No one died."
		gm.addPublic(line)
		gm.ui.info(line)
		gm.rec.Write(transcript.Event{
			Type: transcript.TypeNoDeath, Round: g.Round, Phase: string(game.PhaseNight), Public: line,
		})
	}
}

// runWolves asks every living werewolf for a target and resolves disagreements
// by majority (ties broken with the seeded RNG).
func (gm *GameMaster) runWolves(ctx context.Context) string {
	wolves := gm.game.AliveWithRole(game.RoleWerewolf)
	if len(wolves) == 0 {
		return ""
	}
	candidates := gm.nightCandidates(game.RoleWerewolf, "")
	var targets []string
	for _, w := range wolves {
		p := gm.players[w.ID]
		act := gm.act(ctx, p, turn{kind: actNight, role: game.RoleWerewolf, candidates: candidates})
		gm.rec.Write(transcript.Event{
			Type: transcript.TypeNightAction, Round: gm.game.Round, Phase: string(game.PhaseNight),
			Actor: p.ID, Role: string(game.RoleWerewolf), Target: act.target, Reasoning: act.reasoning,
			Data: map[string]any{"action": "kill"},
		})
		if gm.opts.Reveal && act.target != "" {
			gm.ui.reveal(fmt.Sprintf("%s (werewolf) targets %s — %s", p.ID, act.target, act.reasoning))
		}
		if act.target != "" {
			targets = append(targets, act.target)
		}
	}
	return gm.majorityTarget(targets)
}

func (gm *GameMaster) runSeer(ctx context.Context) {
	for _, s := range gm.game.AliveWithRole(game.RoleSeer) {
		p := gm.players[s.ID]
		candidates := gm.nightCandidates(game.RoleSeer, s.ID)
		act := gm.act(ctx, p, turn{kind: actNight, role: game.RoleSeer, candidates: candidates})
		if act.target != "" {
			isWolf := gm.game.IsWerewolf(act.target)
			var verdict string
			if isWolf {
				verdict = fmt.Sprintf("Your vision is clear: %s IS a werewolf.", act.target)
			} else {
				verdict = fmt.Sprintf("Your vision is clear: %s is NOT a werewolf.", act.target)
			}
			gm.queuePrivate(p.ID, verdict)
			gm.rec.Write(transcript.Event{
				Type: transcript.TypeSeerResult, Round: gm.game.Round, Phase: string(game.PhaseNight),
				Actor: p.ID, Target: act.target, Reasoning: act.reasoning,
				Data: map[string]any{"is_werewolf": isWolf},
			})
			if gm.opts.Reveal {
				gm.ui.reveal(fmt.Sprintf("%s (seer) inspects %s → %v", p.ID, act.target, isWolf))
			}
		} else {
			gm.rec.Write(transcript.Event{
				Type: transcript.TypeNightAction, Round: gm.game.Round, Phase: string(game.PhaseNight),
				Actor: p.ID, Role: string(game.RoleSeer), Reasoning: act.reasoning,
				Data: map[string]any{"action": "inspect", "skipped": true},
			})
		}
	}
}

func (gm *GameMaster) runDoctor(ctx context.Context) string {
	doctors := gm.game.AliveWithRole(game.RoleDoctor)
	if len(doctors) == 0 {
		return ""
	}
	// Single doctor by composition; if there were several, the last protection
	// stands (any single protection cancels the matching kill).
	var target string
	for _, d := range doctors {
		p := gm.players[d.ID]
		candidates := gm.nightCandidates(game.RoleDoctor, d.ID)
		act := gm.act(ctx, p, turn{kind: actNight, role: game.RoleDoctor, candidates: candidates})
		target = act.target
		gm.rec.Write(transcript.Event{
			Type: transcript.TypeNightAction, Round: gm.game.Round, Phase: string(game.PhaseNight),
			Actor: p.ID, Role: string(game.RoleDoctor), Target: act.target, Reasoning: act.reasoning,
			Data: map[string]any{"action": "protect"},
		})
		if gm.opts.Reveal && act.target != "" {
			gm.ui.reveal(fmt.Sprintf("%s (doctor) protects %s — %s", p.ID, act.target, act.reasoning))
		}
	}
	return target
}

// --- Day ------------------------------------------------------------------

func (gm *GameMaster) runDay(ctx context.Context) {
	g := gm.game
	gm.ui.day(g.Round)
	gm.rec.Write(transcript.Event{Type: transcript.TypePhaseStart, Round: g.Round, Phase: string(game.PhaseDay)})

	// Discussion: one or more speaking passes, rotated so no one always leads.
	for pass := 0; pass < gm.opts.DiscussionRounds; pass++ {
		for _, id := range gm.rotatedAlive(g.Round + pass) {
			p := gm.players[id]
			act := gm.act(ctx, p, turn{kind: actSpeak, role: gm.roleOf(id)})
			msg := act.message
			if strings.TrimSpace(msg) == "" {
				msg = "(says nothing)"
			}
			line := fmt.Sprintf("%s says: %q", id, msg)
			gm.addPublic(line)
			gm.ui.speak(id, msg)
			if gm.opts.Reveal {
				gm.ui.reasoning(id, act.reasoning)
			}
			gm.rec.Write(transcript.Event{
				Type: transcript.TypeSpeak, Round: g.Round, Phase: string(game.PhaseDay),
				Actor: id, Message: act.message, Reasoning: act.reasoning,
			})
		}
	}

	// Vote.
	gm.ui.info("The village votes…")
	votes := make(map[string]string)
	candidates := g.AliveIDs()
	for _, id := range gm.rotatedAlive(g.Round) {
		p := gm.players[id]
		act := gm.act(ctx, p, turn{kind: actVote, role: gm.roleOf(id), candidates: candidates})
		votes[id] = act.target
		gm.ui.vote(id, act.target)
		gm.rec.Write(transcript.Event{
			Type: transcript.TypeVote, Round: g.Round, Phase: string(game.PhaseDay),
			Actor: id, Target: act.target, Reasoning: act.reasoning,
		})
		if act.target == "" {
			gm.addPublic(fmt.Sprintf("%s abstains.", id))
		} else {
			gm.addPublic(fmt.Sprintf("%s votes for %s.", id, act.target))
		}
	}

	outcome := game.TallyVotes(votes)
	gm.rec.Write(transcript.Event{
		Type: transcript.TypeTally, Round: g.Round, Phase: string(game.PhaseDay),
		Data: map[string]any{"tally": outcome.Tally},
	})
	if outcome.EliminatedID == "" {
		line := "The vote is deadlocked. No one is eliminated today."
		gm.addPublic(line)
		gm.ui.info(line)
		return
	}
	victim := g.ByID(outcome.EliminatedID)
	role := victim.Role
	g.Eliminate(victim.ID)
	line := fmt.Sprintf("The village votes out %s. They were a %s.", victim.ID, role)
	gm.addPublic(line)
	gm.ui.death(line)
	gm.rec.Write(transcript.Event{
		Type: transcript.TypeElimination, Round: g.Round, Phase: string(game.PhaseDay),
		Target: victim.ID, Role: string(role), Public: line,
		Data: map[string]any{"cause": "vote", "tally": outcome.Tally},
	})
}

// --- Per-player turn execution --------------------------------------------

// act runs one player's turn: it composes the player's situation, asks its
// decider for an action, accounts token usage, and falls back to a logged
// abstain if the turn fails or yields no valid action — a timeout never
// silently becomes a forfeit.
func (gm *GameMaster) act(ctx context.Context, p *Player, t turn) *capturedAction {
	situation := gm.composeSituation(p)
	action, usage, err := p.decider.decide(ctx, t, situation)
	if usage != nil {
		gm.accountUsage(p, usage)
	}
	if err != nil || action == nil {
		gm.logFailure(p, t.kind, err)
		return defaultAction(t.kind)
	}
	return action
}

func (gm *GameMaster) accountUsage(p *Player, usage *llm.Usage) {
	if usage == nil || (usage.InputTokens == 0 && usage.OutputTokens == 0) {
		return
	}
	p.usage.Add(usage)
	gm.rec.Write(transcript.Event{
		Type: transcript.TypeUsage, Round: gm.game.Round, Phase: string(gm.game.Phase), Actor: p.ID,
		Data: map[string]any{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
			"model":         p.Model,
		},
	})
}

func (gm *GameMaster) logFailure(p *Player, kind actionKind, err error) {
	detail := "no valid action"
	if err != nil {
		detail = err.Error()
	}
	gm.ui.warn(fmt.Sprintf("%s forfeited its %s action (%s) — defaulting to abstain", p.ID, kind, detail))
	gm.rec.Write(transcript.Event{
		Type: transcript.TypeForfeit, Round: gm.game.Round, Phase: string(gm.game.Phase),
		Actor: p.ID, Detail: detail, Data: map[string]any{"action": string(kind)},
	})
}

// defaultAction is the safe fallback when a player fails to act: it abstains.
func defaultAction(kind actionKind) *capturedAction {
	switch kind {
	case actSpeak:
		return &capturedAction{kind: actSpeak, message: "(remains silent)", reasoning: "(no response)"}
	default: // vote / night: empty target = abstain
		return &capturedAction{kind: kind, reasoning: "(no response)"}
	}
}

// --- Message delivery ------------------------------------------------------

// composeSituation builds the briefing/state portion of a player's turn input:
// a first-turn role briefing, any new public events since its last turn, any
// private notes queued for it (e.g. a seer's vision), and the current public
// state. The decider appends the action instruction.
func (gm *GameMaster) composeSituation(p *Player) string {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	var parts []string
	if !gm.briefed[p.ID] {
		gm.briefed[p.ID] = true
		gp := gm.game.ByID(p.ID)
		parts = append(parts, roleBriefing(gp, gm.order, gm.fellowWolves(p.ID)))
	}

	cursor := gm.deliveredPublic[p.ID]
	if cursor < len(gm.publicLog) {
		newLines := gm.publicLog[cursor:]
		gm.deliveredPublic[p.ID] = len(gm.publicLog)
		parts = append(parts, "Since your last turn:\n"+bullets(newLines))
	}

	if priv := gm.privateQueue[p.ID]; len(priv) > 0 {
		gm.privateQueue[p.ID] = nil
		parts = append(parts, "PRIVATE (only you know this):\n"+bullets(priv))
	}

	parts = append(parts, stateHeader(gm.game))
	return strings.Join(parts, "\n\n")
}

func (gm *GameMaster) fellowWolves(self string) []string {
	var out []string
	for _, w := range gm.game.AliveWithRole(game.RoleWerewolf) {
		if w.ID != self {
			out = append(out, w.ID)
		}
	}
	sort.Strings(out)
	return out
}

// addPublic appends a line to the shared narration feed. Players receive new
// lines on their next turn.
func (gm *GameMaster) addPublic(line string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.publicLog = append(gm.publicLog, line)
}

func (gm *GameMaster) queuePrivate(id, line string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()
	gm.privateQueue[id] = append(gm.privateQueue[id], line)
}

// --- Helpers ---------------------------------------------------------------

func (gm *GameMaster) roleOf(id string) game.Role {
	if p := gm.game.ByID(id); p != nil {
		return p.Role
	}
	return game.RoleVillager
}

// nightCandidates returns the legal targets for a role's night action.
func (gm *GameMaster) nightCandidates(role game.Role, actorID string) []string {
	var out []string
	for _, p := range gm.game.Alive() {
		switch role {
		case game.RoleWerewolf:
			if p.Role != game.RoleWerewolf {
				out = append(out, p.ID)
			}
		case game.RoleSeer:
			if p.ID != actorID {
				out = append(out, p.ID)
			}
		case game.RoleDoctor:
			out = append(out, p.ID)
		}
	}
	return out
}

// majorityTarget returns the most-chosen target, breaking ties with the seeded
// RNG so the whole match stays reproducible from its seed.
func (gm *GameMaster) majorityTarget(targets []string) string {
	if len(targets) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, t := range targets {
		counts[t]++
	}
	max := 0
	for _, n := range counts {
		if n > max {
			max = n
		}
	}
	var leaders []string
	for id, n := range counts {
		if n == max {
			leaders = append(leaders, id)
		}
	}
	sort.Strings(leaders)
	if len(leaders) == 1 {
		return leaders[0]
	}
	return leaders[gm.game.Intn(len(leaders))]
}

// rotatedAlive returns living player IDs rotated by `by`, so the starting
// speaker/voter changes each round and no model gets a permanent position edge.
func (gm *GameMaster) rotatedAlive(by int) []string {
	ids := gm.game.AliveIDs()
	n := len(ids)
	if n == 0 {
		return ids
	}
	by = ((by % n) + n) % n
	out := make([]string, 0, n)
	out = append(out, ids[by:]...)
	out = append(out, ids[:by]...)
	return out
}

func bullets(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("- ")
		b.WriteString(l)
	}
	return b.String()
}

// --- Transcript bookends ---------------------------------------------------

func (gm *GameMaster) recordMatchStart() {
	players := make([]map[string]any, 0, len(gm.order))
	for _, id := range gm.order {
		p := gm.players[id]
		players = append(players, map[string]any{
			"id": id, "provider": p.Provider, "model": p.Model, "remote": p.Remote,
		})
	}
	// Roles are recorded up front for replay/analysis. The transcript is the
	// post-match reveal artifact; players never see it during the game.
	roles := make(map[string]string, len(gm.game.Players))
	for _, gp := range gm.game.Players {
		roles[gp.ID] = string(gp.Role)
	}
	gm.rec.Write(transcript.Event{
		Type: transcript.TypeMatchStart,
		Data: map[string]any{
			"seed":              gm.game.Seed,
			"players":           players,
			"roles":             roles,
			"max_rounds":        gm.opts.MaxRounds,
			"discussion_rounds": gm.opts.DiscussionRounds,
		},
	})

	gm.ui.banner(cPurple, "🏛  THE COLOSSEUM — WEREWOLF")
	gm.ui.info(fmt.Sprintf("Seed %d · %d players · %s",
		gm.game.Seed, len(gm.order), strings.Join(gm.rosterLabels(), ", ")))
}

func (gm *GameMaster) rosterLabels() []string {
	out := make([]string, 0, len(gm.order))
	for _, id := range gm.order {
		p := gm.players[id]
		out = append(out, fmt.Sprintf("%s (%s)", id, p.Model))
	}
	return out
}

func (gm *GameMaster) recordMatchEnd() (game.Team, error) {
	winner := gm.game.Winner
	survivors := gm.game.AliveIDs()

	usageByPlayer := make(map[string]any, len(gm.players))
	var totalIn, totalOut int
	for _, id := range gm.order {
		p := gm.players[id]
		usageByPlayer[id] = map[string]any{
			"input_tokens":  p.usage.InputTokens,
			"output_tokens": p.usage.OutputTokens,
			"model":         p.Model,
		}
		totalIn += p.usage.InputTokens
		totalOut += p.usage.OutputTokens
	}

	gm.rec.Write(transcript.Event{
		Type:   transcript.TypeMatchEnd,
		Round:  gm.game.Round,
		Target: string(winner),
		Data: map[string]any{
			"winner":              string(winner),
			"survivors":           survivors,
			"rounds":              gm.game.Round,
			"usage_by_player":     usageByPlayer,
			"total_input_tokens":  totalIn,
			"total_output_tokens": totalOut,
		},
	})

	gm.ui.banner(cGreen, "🏆 RESULT")
	switch winner {
	case game.TeamVillage:
		gm.ui.result("The VILLAGE wins — every werewolf has been eliminated.")
	case game.TeamWerewolf:
		gm.ui.result("The WEREWOLVES win — they have taken over the village.")
	default:
		gm.ui.result("No decisive winner — the round limit was reached.")
	}
	gm.printRoleReveal()
	gm.printUsage(usageByPlayer, totalIn, totalOut)
	return winner, nil
}

func (gm *GameMaster) printRoleReveal() {
	gm.ui.line("")
	gm.ui.reveal("Final roles:")
	for _, gp := range gm.game.Players {
		status := "alive"
		if !gp.Alive {
			status = "dead"
		}
		gm.ui.info(fmt.Sprintf("  %-12s %-9s [%s]", gp.ID, gp.Role, status))
	}
}

func (gm *GameMaster) printUsage(byPlayer map[string]any, totalIn, totalOut int) {
	gm.ui.line("")
	gm.ui.info(fmt.Sprintf("Token usage — %d in / %d out total", totalIn, totalOut))
	for _, id := range gm.order {
		u := gm.players[id].usage
		gm.ui.info(fmt.Sprintf("  %-12s %d in / %d out", id, u.InputTokens, u.OutputTokens))
	}
}

// PlayerCount returns the number of seats (handy for callers/tests).
func (gm *GameMaster) PlayerCount() int { return len(gm.players) }

// Winner returns the winning team after Run (or "" if undecided).
func (gm *GameMaster) Winner() game.Team { return gm.game.Winner }
