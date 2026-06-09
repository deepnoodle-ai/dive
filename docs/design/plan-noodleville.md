# Implementation Plan — NoodleVille 🏘️

> A generative-agent town as a single Go binary: one goroutine per villager, live browser
> visualization, emergent unscripted social behavior.
> **Pillar proven:** Go concurrency + single static binary ("serious agents in Go").
> **Shareable artifact:** a live, watchable town (clips + screenshots of emergent drama)
> AND a forkable `go install`-able starter kit.

## The shareable moment

> "100 AI villagers. One 8 MB Go binary. No Python, no venv. They woke up, gossiped, and
> threw a party nobody told them to." → time-lapse clip of the town + the chat log where
> the party self-organized.

This clones a **proven-viral format** (Stanford Generative Agents / Smallville, a16z
AI Town ~10k stars) *and* settles the recurring HN/r/golang argument by demonstration:
goroutine-per-agent concurrency, single-binary distribution, clean `context` cancellation
of the whole swarm. Forkability is the reach multiplier — AI Town's stars came from being
clone-and-run.

## What it proves about Dive

- **Goroutine concurrency**: each villager is a `dive.Agent` driven by its own goroutine;
  the town tick fans out to all of them and gathers results — the Go-native answer to
  "can you run many agents at once?"
- **Single static binary**: `go install` → `noodleville` → browser opens. No runtime.
- **`context.Context` cancellation**: pause/stop the entire town instantly and cleanly.
- **Sessions + memory per agent**: each villager has a persistent `session` (FileStore) —
  its memories survive restarts.
- **Compaction**: villagers have long "days"; non-destructive compaction keeps their
  context bounded while preserving the transcript (audit trail of how a relationship formed).
- **Hooks**: a `PreGeneration` hook injects the villager's current location, nearby agents,
  time of day, and recent memories into context each tick.

## How it works (the generative-agent loop)

Following the Smallville recipe, adapted to Dive:

1. **World tick** (e.g. one tick = 10 in-world minutes). The world model holds a grid/map,
   objects, and each agent's position + state.
2. For each villager, a `PreGeneration` hook composes a **perception**: who/what is nearby,
   the time, and relevant **retrieved memories** (recency + importance + relevance, the
   Smallville memory-stream heuristic).
3. The villager agent **decides an action** via a typed tool (`MoveTo`, `Talk`, `Use`,
   `Reflect`, `Plan`). Conversations between two co-located villagers are short multi-turn
   exchanges, each side its own agent.
4. New observations and dialogue are written to that villager's **memory stream** (session).
5. Periodic **reflection**: villagers summarize the day into higher-level memories
   (this is where compaction + a reflection prompt combine).
6. The web UI renders the map, agent positions, and a live activity/dialogue feed.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                    World (Go, single binary)              │
│  - map/grid + objects + clock                             │
│  - scheduler: fan out tick → goroutine per villager       │
│  - embedded HTTP/WebSocket server → live UI               │
└───────────────┬───────────────────────────────┬──────────┘
                │ goroutine per villager          │ ws stream
        ┌───────┴────────┐                ┌───────┴─────────┐
        │  Villager #1   │  ...  #100     │   Browser UI    │
        │ dive.Agent     │                │ map + feed +    │
        │ + Session(file)│                │ time-lapse      │
        │ + memory stream│                └─────────────────┘
        └────────────────┘
```

- **Concurrency model**: a worker pool bounded by a configurable parallelism (so 100
  villagers don't fire 100 simultaneous LLM calls and melt rate limits). The *swarm* is
  goroutine-cheap; the *LLM calls* are the throttled resource. This distinction is itself
  a teachable, tweet-worthy point.
- **Cost control**: use a cheap/fast model (Haiku-tier, or local via Ollama) for routine
  villager ticks; reserve premium models for reflection or showcase runs. **Ollama support
  means the whole town can run locally for $0** — a huge forkability unlock.

## Where it lives

`demos/noodleville/` as its own Go module (`github.com/deepnoodle-ai/dive/demos/noodleville`),
following the repo's local-`replace` multi-module pattern (see `examples/go.mod`). Embed UI
assets with `embed.FS` for a single binary. The default Ollama path needs no extra provider
replace (ollama is in the core module). Forkability is the point, so it graduates early to its
own `go install`-able repo — the isolated module makes that a move, not a rewrite. Exact
`replace` recipe in `kickoff-noodleville.md`.

## Build phases

**Phase 1 — Tiny town (5 villagers, terminal output)**
- World model: grid, clock, positions, simple objects (house, café, park).
- Villager = `dive.NewAgent` + `session` (FileStore) + action tools via `FuncTool[T]`.
- `PreGeneration` hook builds perception + retrieved memories.
- Scheduler: tick loop, goroutine fan-out, bounded worker pool.
- Memory stream: append observations; basic recency+importance retrieval.
- Output: structured log to terminal. Prove the loop produces coherent behavior.

**Phase 2 — Emergence + reflection (10–25 villagers)**
- Add reflection step (daily summary → higher-level memories) using compaction.
- Add planning (villagers form a daily plan, re-plan on interruption).
- Seed a goal in one villager ("organize a party for Saturday") and watch propagation —
  the canonical Smallville demo. This is the **headline clip**.

**Phase 3 — The watchable layer (the shareable artifact)**
- Embedded web server (HTTP + WebSocket) in the Go binary; static UI (canvas/tile map +
  live dialogue feed + clock). Lead with the single-binary story.
- Time-lapse export: record tick snapshots → render an accelerated clip.
- `go install github.com/deepnoodle-ai/noodleville@latest` → runs out of the box with
  Ollama (free) or any provider key.

**Phase 4 — Scale flex (optional, pairs with idea #5)**
- Push to 100–1000 villagers; publish memory/throughput numbers and a "stop the whole
  town in one keypress" cancellation demo. The direct rebuttal to HN skeptics.

## Starting points in the repo

- `examples/subagent_example/` — multiple agents, per-agent config.
- `examples/background_polling_example/` — concurrent tasks + bounded fan-out pattern.
- `examples/hooks_example/` — `PreGeneration` context injection.
- `experimental/compaction/` — reflection / memory summarization.
- `examples/ollama_example/` — free local model path for forkability.
- `session/` (FileStore) — per-villager persistent memory.

## Tech choices

- Everything in one Go module → one binary. Embed UI assets with `embed.FS`.
- UI: lightweight canvas tile renderer + WebSocket feed. No heavy frontend framework in the
  forkable repo (keeps `go install` clean); a fancier site can come later.
- Memory retrieval: start with recency+importance scoring (no vector DB needed for a small
  town); add embeddings only if relevance retrieval clearly needs it.

## Risks / open questions

- **No built-in memory/embedding layer** (capability map gap): start heuristic; if
  retrieval quality is poor, add a small embedding step. Don't block Phase 1 on it.
- **Cost at scale**: default to Ollama/cheap models; document the cost curve. Showcase
  runs use premium models.
- **Rate limits**: bounded worker pool is mandatory; make parallelism a flag.
- **Coherence over long runs**: lean on compaction + reflection; cap memory stream growth.

## Effort estimate

- Phase 1 tiny town: **~3–4 days**.
- Phase 2 emergence: **~3 days**.
- Phase 3 watchable web layer: **~4–5 days**.
- Phase 4 scale flex: **~2 days**.

## Definition of "done & shareable"

A `go install`-able binary that opens a browser to a live town running on free local models,
at least one captured time-lapse of a genuinely emergent event (a self-organized party,
a friendship, a feud), and a README that leads with "N agents, one binary, no Python."
