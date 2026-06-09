# Implementation Plan — The Colosseum 🏛️

> Cross-provider social-deduction arena with a live leaderboard.
> **Pillar proven:** cross-provider (Claude vs GPT vs Gemini vs Grok in one program).
> **Shareable artifact:** a public leaderboard + recorded "match replays" that reveal
> each model's *private reasoning* after the round.

## The shareable moment

> "We made Claude, GPT-5, Gemini, and Grok play Werewolf against each other. Here's who
> lies best — and the receipts." → a leaderboard people refresh, plus replay clips where
> the hidden chain-of-thought is unmasked ("GPT *knew* Claude was the wolf and said nothing").

This is the idea **only Dive makes easy**. Any other stack means gluing four SDKs together;
Dive does it through one `llm.LLM` interface with self-registering providers. The
"which model is actually smartest at deception?" question is an endless content engine —
every new model release is a new episode.

## What it proves about Dive

- **Cross-provider** in a single binary: `providers/{anthropic,openai,google,grok}` behind one interface.
- **A2A networking** (optional v2): each player runs as its own `a2a.NewServer` agent; the
  game master calls them via `a2a.NewRemoteAgentFromURL`. Demonstrates distributed agents.
- **Hooks** for the referee: a `PreToolUse`/`Stop` hook validates moves and enforces rules.
- **Sessions** per player: each model keeps its own private memory of the game.
- **Structured tool output**: game actions (`vote`, `accuse`, `claim_role`, `night_kill`)
  as typed tools via `FuncTool[T]`.

## Game choice

Start with **One-Night/Multi-Round Werewolf** (simpler state than full Diplomacy, proven
viral via LLM Mafia / Kaggle Werewolf). Roles: Werewolves, Seer, Doctor, Villagers.
Phases: Night (private actions) → Day (discussion + vote). A match is ~5–8 rounds.

Diplomacy is the ambitious follow-up (richer negotiation, but much heavier state).

## Architecture

```
                ┌─────────────────────────────────────┐
                │           Game Master (Go)           │
                │  - phase state machine               │
                │  - role assignment, win detection    │
                │  - referee hook (legal-move check)   │
                │  - transcript + private-reasoning log│
                └───────────────┬─────────────────────┘
                                │ each player = a dive.Agent
       ┌────────────┬───────────┼───────────┬────────────┐
   Claude        GPT-5       Gemini       Grok        (Ollama local)
   (anthropic)   (openai)    (google)     (grok)
   own Session   own Session own Session  own Session
```

- **Player turn**: GM sends the public game state + that player's private info to the
  player agent. Player responds via a typed tool call (its action). GM records both the
  public action and the private reasoning (`WithEventCallback` captures the model's
  thinking/explanation channel).
- **Determinism for fairness**: same prompt template for every player; only the model
  differs. Log seeds and prompts for reproducibility.
- **Scoring**: per-match win/loss by role, plus derived metrics — deception success
  (wolf survived to endgame), deduction accuracy (villager voted the real wolf),
  persuasion (got others to follow its vote). Aggregate into an ELO-style leaderboard.

## Where it lives

`demos/colosseum/` as its own Go module (`github.com/deepnoodle-ai/dive/demos/colosseum`),
following the repo's local-`replace` multi-module pattern (no `go.work`; see `examples/go.mod`).
A separate module keeps the arena/UI deps out of the core `go.mod`, and it can graduate to a
standalone forkable repo later with a directory move + a versioned `require`. Exact `replace`
recipe (core + openai/google/grok; anthropic is in core) is in `kickoff-colosseum.md`.

## Build phases

**Phase 1 — Single-process MVP (no A2A yet)**
- Game state machine + role logic in pure Go (unit-tested).
- Player = `dive.NewAgent` with a per-player `session.New`. One `AgentOptions` template,
  swap `Model:` per provider.
- Typed action tools via `dive.FuncTool[T]` (`SpeakAction`, `VoteAction`, `NightAction`).
- Referee as a `PreToolUse` hook: reject illegal moves, force a retry with feedback.
- Capture private reasoning via `WithEventCallback` → write a structured JSONL transcript.
- CLI runner: `colosseum run --players claude,gpt,gemini,grok` prints the match to terminal.

**Phase 2 — The shareable layer**
- Web replay viewer (React Router v7 + Cloudflare per house style, or a single embedded
  Go static server for the forkable version). Timeline scrubber; toggle "reveal private
  reasoning."
- Leaderboard: run N matches, aggregate ELO + metrics, render a sortable table.
- "Highlight reel" generator: auto-detect dramatic moments (a wolf surviving a 1-vote
  margin, a Seer being ignored) → short shareable clips/cards.

**Phase 3 — A2A distribution (optional, the "agents over the wire" flex)**
- Each player wrapped as `a2a.NewServer`; GM calls them via `a2a.NewRemoteAgentFromURL`.
- Lets contributors host their *own* model as a challenger. "Bring your own agent to the
  arena" = community participation hook.

## Starting points in the repo

- `examples/a2alib_example/` — A2A server + remote client wiring (Phase 3).
- `examples/subagent_example/` — multiple agents in one program; per-agent config.
- `examples/hooks_example/` — `PreToolUse` gating pattern for the referee.
- `tool_progress_example/` — `ReportProgress` + `WithEventCallback` for streaming the show.
- `providers/{anthropic,openai,google,grok}` — confirm default model constants per provider.

## Tech choices

- Core arena: pure Go, single binary (`go install`).
- Frontend: minimal — for the forkable repo, embed a static viewer served by the Go binary;
  for the marketing site, React Router v7 on Cloudflare.
- Transcript format: JSONL (one event per line) → trivially replayable + diffable.

## Risks / open questions

- **Cost**: N models × many rounds × many matches. Mitigate with cheap-tier models for
  bulk leaderboard runs, premium models for showcase matches. Log token usage per match.
- **Provider rate limits / fairness**: stagger calls; retry with backoff; never let a
  timeout count as a forfeit silently — log it.
- **Prompt fairness**: a single shared template is essential or critics will (correctly)
  cry foul. Open-source the exact prompts.
- **Reasoning capture varies by provider** (thinking channels differ). Normalize to a
  "stated reasoning" field the player must emit as part of its action, so it's comparable.

## Effort estimate

- Phase 1 MVP: **~2–3 days**.
- Phase 2 shareable layer: **~3–4 days**.
- Phase 3 A2A: **~2 days**.

## Definition of "done & shareable"

A public leaderboard updated from real matches, plus at least 3 replay clips where the
revealed private reasoning produces a genuine "oh no it *knew*" moment, and a forkable
`go install`-able repo with "bring your own agent" instructions.
