# 🏛 The Colosseum

**We made Claude, GPT, and Grok play Werewolf against each other — in one Go
program, no Python, behind a single interface. Here's who lies best, and the
receipts.**

The Colosseum is a cross-provider social-deduction arena. Every player is a
*different LLM provider's model* (Claude, GPT, Gemini, Grok), all competing in a
game of Werewolf where lying, deduction, and persuasion decide who wins. After
each match you get a replayable transcript that **reveals every player's private
reasoning** — the moment where you see that the wolf *knew* it was lying.

It is built with [Dive](https://github.com/deepnoodle-ai/dive), and it exists to
show off the one thing Dive makes trivial: **many providers behind one
interface, in a single binary.** Adding a new contestant is a one-line entry in
[`provider/registry.go`](provider/registry.go) — the engine, the arena, and the
referee never change, because they only ever see Dive's `llm.LLM` interface.

```
$ colosseum run --players claude,gpt,gemini,grok --reveal
```

```
🌙 ROUND 1 — NIGHT
  🔎 grok (werewolf) targets claude-1 — I'll go for claude-1 first; it might be a strong player.
  🔎 claude-2 (seer) inspects gpt → false
☀️  ROUND 1 — DAY
grok: No death last night is interesting. Could be the Doctor saved someone…
    ↳ (grok thinks) As the lone wolf, I need to blend in. I'll stay neutral to avoid suspicion.
  🗳  gpt → grok   grok → gpt   claude-2 → grok   claude-1 → grok
  ☠  The village votes out grok. They were a werewolf.
🏆 The VILLAGE wins — every werewolf has been eliminated.
```

Then turn matches into a **leaderboard you can refresh** and **replays you can
scrub** — with a toggle that unmasks every model's private reasoning:

```bash
colosseum tournament --players claude,gpt,gemini,grok -n 20   # run a leaderboard
colosseum serve --dir transcripts                             # open the replay viewer
```

---

## Commands

| Command | What it does |
| --- | --- |
| `colosseum run` | Play one match and print it live to the terminal. |
| `colosseum tournament` | Run N matches, build an ELO leaderboard + metrics, write a transcript per match. |
| `colosseum serve` | Serve the web replay viewer + leaderboard from a transcripts directory. |
| `colosseum leaderboard <dir-or-json>` | Print the standings table for a transcripts dir or a saved `leaderboard.json`. |
| `colosseum highlights <transcript>` | Analyze one match: per-player metrics + auto-detected dramatic moments. |
| `colosseum serve-agent` | Host a Dive agent as an A2A challenger — "bring your own agent to the arena." |

Run `colosseum <command> -h` for each command's flags.

---

## Quick start

You need an API key for each provider you put in the match. Set whichever you
have; the runner verifies them up front and tells you exactly what's missing.

| Provider | Key flag value | Env var(s) |
| --- | --- | --- |
| Claude (Anthropic) | `claude` | `ANTHROPIC_API_KEY` |
| GPT (OpenAI) | `gpt` | `OPENAI_API_KEY` |
| Gemini (Google) | `gemini` | `GEMINI_API_KEY` or `GOOGLE_API_KEY` |
| Grok (xAI) | `grok` | `XAI_API_KEY` or `GROK_API_KEY` |

```bash
# From this directory (it's its own Go module):
go run . run --players claude,gpt,gemini,grok --reveal

# Or install the binary:
go install github.com/deepnoodle-ai/dive/demos/colosseum@latest
colosseum run --players claude,gpt,grok --reveal
```

You can field **at least three** players. List a provider more than once to seat
several of its models (`--players claude,gpt,grok,claude` → `claude-1`,
`claude-2`). With four or more seats the game also includes a Doctor.

### Flags

| Flag | Default | What it does |
| --- | --- | --- |
| `--players` | `claude,gpt,gemini,grok` | Comma-separated provider keys. |
| `--model key=model` | — | Override a provider's model (repeatable), e.g. `--model claude=claude-opus-4-8`. |
| `--premium` | off | Use premium model tiers instead of the cheap defaults. |
| `--seed` | time-based | RNG seed; the same seed reproduces the exact role assignment. |
| `--max-rounds` | `8` | Safety cap on rounds. |
| `--discussion-rounds` | `1` | Speaking passes per day. |
| `--timeout` | `90s` | Per-player-turn timeout. |
| `--reveal` | off | Print each player's private reasoning live (it is **always** in the transcript). |
| `--transcript` | `colosseum-<ts>.jsonl` | Where to write the JSONL transcript. |

### Cost

Model calls cost money, and a match makes many of them. By default each provider
uses its **cheap tier** (Haiku, GPT-mini, Gemini Flash, Grok-fast) so a full
leaderboard run stays affordable; pass `--premium` (or `--model`) for showcase
matches. Every turn's token usage is logged to the transcript and summarized at
the end. Note that one *action* costs two model calls — one to make the tool
call, one to acknowledge and end the turn — so budget accordingly.

---

## The fairness contract

A model-vs-model arena is only interesting if it's fair, and critics will
(correctly) cry foul if the prompting favors one model. So:

- **Every contestant gets the same game rules.** They live in `const gameRules`
  in [`arena/prompt.go`](arena/prompt.go) — read it; that's the whole story.
  Nothing in it is tailored to any provider.
- The **only** things that differ between players are (1) the model behind the
  seat, (2) the *legitimate* private information their role grants — their own
  role, who their fellow wolves are, what the Seer has learned (delivered through
  in-game messages, **never** the system prompt), and (3) **how** they submit an
  action: local players call typed tools (validated by a referee hook); remote
  challengers reply with JSON over A2A. Same rules, same information, same
  required reasoning — only the transport differs.
- Reasoning is captured the **same way for everyone**: every action must include
  a `reasoning` field. We do not depend on any provider's proprietary "thinking"
  channel, because those differ and would make the comparison unfair. The stated
  reasoning is apples-to-apples across providers.
- Matches are **reproducible**: role assignment and every tie-break run off a
  single logged seed.

---

## How it works

```
                 ┌────────────────────────────────────────┐
                 │              Game Master                │
                 │  • Night → Day → vote state machine     │
                 │  • referee (PreToolUse legal-move hook) │
                 │  • JSONL transcript + private reasoning  │
                 └───────────────┬────────────────────────┘
                                 │  each player = a `decider`
        ┌────────────┬───────────┼────────────┬──────────────────┐
     Claude        GPT-5        Gemini        Grok          a2a:your-host
   (anthropic)   (openai)     (google)       (grok)        (over the wire)
   ── local: a dive.Agent + tools + referee ──        ── remote: A2A + JSON ──
        each with its own private Session / game memory
```

- **One `decider` interface, two implementations.** The game master treats every
  seat the same. A **local** seat ([`arena/local.go`](arena/local.go)) is a
  `dive.NewAgent` built from the shared template — same system prompt, same tools,
  same referee hook — with only the model and its session differing. A **remote**
  seat ([`arena/remote.go`](arena/remote.go)) is an agent reached over A2A that
  replies with JSON. Adding either is the same to the rest of the program.
- **Typed action tools.** For local players, `speak`, `vote`, and `night_action`
  are `dive.FuncTool` tools whose schemas are auto-generated from Go structs.
  Each requires a `reasoning` field. See [`arena/tools.go`](arena/tools.go).
- **The referee is a `PreToolUse` hook.** It rejects illegal moves (voting for a
  dead player, a wolf attacking a wolf, an empty message) by returning an error;
  Dive feeds that error back to the model, which retries with the correction.
- **Per-player sessions.** Each model keeps its own private memory of the game,
  so one player's secret knowledge can never leak into another's context. Remote
  players keep a continuing session too, via the A2A context id.
- **The engine is pure Go and provider-agnostic.** The `game` package never
  imports an LLM provider (or even Dive); players are plain string IDs. That
  keeps the rules independently unit-tested (`go test ./game/`).

### The transcript

One JSON object per line. It is the post-match reveal artifact, so it
deliberately records things players never saw during play — roles, each action's
hidden reasoning, the Seer's visions:

```jsonc
{"type":"match_start","data":{"seed":42,"roles":{"grok":"werewolf",...}}}
{"type":"night_action","actor":"grok","role":"werewolf","target":"claude-1",
 "reasoning":"As the lone wolf, I need to blend in..."}
{"type":"seer_result","actor":"claude-2","target":"gpt","data":{"is_werewolf":false}}
{"type":"speak","actor":"grok","message":"No death last night is interesting...",
 "reasoning":"I'll stay neutral to avoid suspicion."}
{"type":"vote","actor":"claude-1","target":"grok","reasoning":"..."}
{"type":"elimination","target":"grok","role":"werewolf","data":{"cause":"vote"}}
{"type":"match_end","data":{"winner":"village","survivors":["claude-1","gpt","claude-2"]}}
```

Because it's append-only JSONL, it's trivially replayable and diffable — it's the
substrate the leaderboard, the highlight detector, and the web viewer all read.

---

## The leaderboard & metrics

`colosseum tournament -n 20` plays many matches and aggregates an **ELO
leaderboard** keyed by model — the headline "which model is actually best at
Werewolf?" Each match's result feeds pairwise ELO updates (every winner-team
model beats every loser-team model). Beyond raw win rate, three skill metrics are
derived from the transcript:

| Metric | Who | How it's measured |
| --- | --- | --- |
| **Deception** | Werewolves | Fraction of the match's rounds the wolf survived unlynched — a wolf that lasts is deceiving well. |
| **Deduction** | Villagers | Fraction of a player's day-votes that landed on an actual werewolf. |
| **Persuasion** | Everyone | Average fraction of *other* voters who matched this player's vote — a proxy for "did the table follow me." |

The leaderboard persists to `leaderboard.json` and **accumulates across runs**, so
a nightly tournament keeps building history. Print it any time:

```bash
colosseum leaderboard transcripts/        # compute fresh from a transcripts dir
colosseum leaderboard leaderboard.json     # load a saved, accumulated leaderboard
```

```
#  MODEL                ELO   W-L  WIN%  WOLF W%  DECEPT  DEDUCT  PERSUADE
1  claude-haiku-4-5     1023  2-0  100%    —      0.00    1.00    0.67
2  gpt-5.4-mini         1011  1-0  100%    —      0.00    1.00    0.67
3  grok-4-1-fast        966   0-1  0%     0%      1.00    0.00    0.00
```

## The web replay viewer

`colosseum serve --dir transcripts` starts a single embedded Go server (no build
step, no Node, no CDN — the whole UI is `go:embed`-ed into the binary) at
`http://localhost:8723`. It offers:

- A **match picker** and a **timeline scrubber** with play/pause to step through a
  match event by event.
- A **"reveal private reasoning" toggle** — the centerpiece. Off, you see only
  what the table saw. On, it unmasks every player's hidden reasoning, their secret
  roles, the wolves' night targets, and the Seer's visions: the "oh no, it *knew*"
  moment.
- A **highlights** panel and a sortable **leaderboard** tab.

It's a tiny JSON API over the transcripts directory (`/api/matches`,
`/api/matches/{id}`, `/api/leaderboard`), so it's easy to point at your own runs.

## Highlights

`colosseum highlights <transcript>` (and the viewer) auto-detect dramatic
moments — the raw material for shareable clips:

- **The Seer knew** — a Seer identified a wolf the village then failed to lynch.
- **Lone wolf victory** — one wolf bluffed the whole table.
- **Mislynch** — the village voted out its own Seer or Doctor.
- **Knife-edge** — a wolf survived a vote by a single ballot.
- **Doctor save**, **flawless village**, and more.

## Bring your own agent (A2A)

A seat doesn't have to be a model in *this* binary — it can be **any agent reached
over the wire**, using Dive's A2A support. Host a challenger:

```bash
# Terminal 1 — host a Claude challenger as an A2A server:
colosseum serve-agent --provider claude --addr :8090

# Terminal 2 — seat it in a match alongside local players:
colosseum run --players gpt,grok,a2a:http://localhost:8090
```

A seat token is either a provider key (local) or `a2a:URL` / `name@http://URL`
(remote). The game master sends a remote seat the same situation it sends local
players, plus a request to reply with one JSON action object
(`{"action","target","message","reasoning"}`); it validates the reply against the
exact same legal-move rules the referee enforces locally. The challenger keeps a
continuing private game memory across its turns via the A2A context id. This is
how contributors enter their *own* model — or their own scaffolding — as a
competitor. `serve-agent` is a ~60-line reference host; any A2A server that
honours the JSON protocol works.

---

## Rules in brief

Roles are **Werewolf**, **Seer**, **Doctor**, and **Villager**, scaled to the
table size. Each round has two phases:

- **Night** (secret): the werewolves agree on one victim; the Seer learns whether
  one player is a wolf; the Doctor protects one player (cancelling the kill if
  they guess the wolves' target).
- **Day** (public): everyone wakes, learns who died, **discusses**, then **votes**.
  The most-voted player is eliminated and their role revealed. A tie eliminates
  no one.

The **village** wins when every werewolf is dead. The **werewolves** win when
they reach numerical parity with the rest of the table.

A turn that times out or fails is retried with backoff and then **logged and
defaulted to an abstain** — a timeout never silently becomes a forfeit.

---

## Project layout

```
demos/colosseum/
├── main.go, cmd_*.go     CLI dispatch + one file per subcommand
├── contestants.go        parse --players into seats (local or a2a:URL)
├── game/                 Pure-Go Werewolf engine (no provider imports) + tests
├── arena/                The match: decider interface, local (tools+referee) &
│                         remote (A2A) players, shared prompt template, loop
├── transcript/           JSONL event model: Writer, Read, Match parser
├── analytics/            Metrics, ELO leaderboard, highlight detection, renderers
├── tournament/           Run N matches → aggregate leaderboard + highlights
├── web/                  Embedded replay viewer (go:embed) + JSON API
└── provider/             Provider registry — the ONLY place concrete providers
                          are imported
```

This is its own Go module. Inside the Dive repo it builds against the working
tree via local `replace` directives (see [`go.mod`](go.mod)); to spin it out as a
standalone forkable repo, drop the replaces and pin versioned `require`s.

## What it proves about Dive

- **Cross-provider** — Claude, GPT, Gemini, and Grok behind one `llm.LLM`
  interface, in one binary. Adding a contestant is one line.
- **Hooks** — the referee is a `PreToolUse` hook that enforces the rules.
- **Sessions** — each player keeps its own private game memory.
- **Typed tools** — `FuncTool[T]` actions with auto-generated schemas.
- **A2A** — players can live on other hosts and play over the wire.

## Roadmap

All three planned phases are implemented:

- **Phase 1** ✅ — single-process terminal MVP: engine, agents, tools, referee,
  JSONL transcript.
- **Phase 2** ✅ — the shareable layer: ELO leaderboard + skill metrics, the
  embedded web replay viewer with the reveal toggle, and highlight detection.
- **Phase 3** ✅ — A2A distribution: host a challenger with `serve-agent`, seat it
  with `a2a:URL`.

See `docs/design/plan-colosseum.md` in the Dive repo for the full plan.
