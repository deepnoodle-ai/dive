# Kickoff Prompt — NoodleVille 🏘️

> Paste this into a fresh session to begin building. Full design: `docs/design/plan-noodleville.md`.
> Overall series context: `docs/design/demo-ideas.md`.

---

You are building **NoodleVille**: a generative-agent town (à la Stanford's Smallville /
a16z AI Town) implemented as a **single Go binary with one goroutine per villager** and a
live browser visualization. This is a marketing/demo project for **Dive**, a Go library for
building AI agents. The whole point is to settle, by demonstration, the recurring
"is Go good for agents?" debate: goroutine-per-agent concurrency, a ~8 MB static binary,
clean `context` cancellation of the whole swarm — and to clone a proven-viral, forkable format.

The shareable moment we're aiming at: *"100 AI villagers. One Go binary. No Python, no venv.
They woke up, gossiped, and threw a party nobody told them to."*

## Context about Dive

- Dive lives at `/Users/curtis/git/deepnoodle/dive` (module `github.com/deepnoodle-ai/dive`). Go 1.25.
- Core API: `dive.NewAgent(dive.AgentOptions{...})` → `*Agent`. System-prompt field is
  `SystemPrompt`. `agent.CreateResponse(ctx, dive.WithInput(...))`; read `response.OutputText()`.
- Providers self-register; import e.g. `.../providers/ollama` for **free local models**
  (critical for forkability — the town should run at $0 out of the box), or
  `.../providers/anthropic` etc. Constructors take variadic options.
- Custom tools: `dive.FuncTool[T](...)` → `*dive.TypedToolAdapter[T]`. Villager actions
  (`MoveTo`, `Talk`, `Use`, `Reflect`, `Plan`) are typed tools.
- Hooks: `AgentOptions.Hooks`. Use a `PreGenerationHook` to inject each villager's
  perception (nearby agents/objects, time of day, retrieved memories) before its turn.
- Per-villager memory: a `FileStore`-backed `session` per agent so memories survive restart.
  Set via `AgentOptions.Session` / `dive.WithSession(...)`.
- Long runs: `experimental/compaction` summarizes old context non-destructively — use it for
  daily "reflection" and to bound each villager's memory stream.
- Streaming the live feed: `dive.WithEventCallback(fn)`.
- Tests use `github.com/deepnoodle-ai/wonton/assert`.

**Verify exact signatures against the source before relying on them** — confirm types in
`dive.go`, `agent.go`, `hooks.go`, `session/`, and `experimental/compaction/` rather than
trusting prose.

## Before you write code, read these

- `examples/subagent_example/` — multiple agents, per-agent config.
- `examples/background_polling_example/` — concurrent tasks + bounded fan-out (the swarm pattern).
- `examples/hooks_example/` — `PreGeneration` context injection.
- `examples/ollama_example/` — the free local-model path.
- `experimental/compaction/` — reflection / memory summarization.
- `session/` — `FileStore` for per-villager persistent memory.

## Phase 1 deliverable (this session)

A **tiny town (5 villagers) with terminal output** — no web UI yet. Prove the generative-
agent loop produces coherent behavior:

1. World model: a small grid/map, a clock, villager positions, a few objects (house, café,
   park). Pure Go, unit-tested.
2. A scheduler that ticks the world and **fans out to one goroutine per villager**, bounded
   by a configurable worker-pool limit (the villagers are goroutine-cheap; the *LLM calls*
   are the throttled resource — keep that distinction explicit, it's the teachable point).
3. Each villager = `dive.NewAgent` + a `FileStore` session + action tools via `FuncTool[T]`.
4. A `PreGenerationHook` that composes each villager's perception: who/what is nearby, the
   time, and retrieved memories (start with a recency+importance heuristic — **do not**
   block on building a vector store; the capability map confirms there's no built-in
   embedding layer and a small town doesn't need one yet).
5. A memory stream: append observations and dialogue to each villager's session.
6. Structured terminal output of the tick loop so you can read what the town is doing.

**Where it lives:** build under `demos/noodleville/` in the dive repo as its own Go module —
`demos/noodleville/go.mod`, module path `github.com/deepnoodle-ai/dive/demos/noodleville`, with
`replace github.com/deepnoodle-ai/dive => ../..`. The default free path uses the **ollama**
provider, which ships *inside* the core module, so the out-of-the-box run needs no extra
provider replace (add `=> ../../providers/openai` etc. only if you wire in those separate
modules). The repo uses local `replace` directives, not a `go.work` (see `examples/go.mod`).
Embed UI assets with `embed.FS` so the binary stays single-file. Because forkability is this
demo's whole point, plan to graduate it early to its own
`go install github.com/deepnoodle-ai/noodleville@latest` repo — built as an isolated module
from day one, that's a directory move + swapping `replace` for a versioned `require`.

## Constraints & conventions

- One Go module → one binary. No mandatory external services for the default run
  (Ollama optional/local; provider keys optional).
- The worker-pool parallelism cap is mandatory — never fire N simultaneous LLM calls for N
  villagers. Make it a flag.
- Default to a cheap/fast or local model for routine ticks; reserve premium models for
  reflection or showcase runs. Document the cost curve.
- Match surrounding Go style; keep the world model independent of any specific provider.

## Definition of done for this session

A 5-villager town runs for multiple ticks on a local/cheap model, villagers perceive their
surroundings, take coherent actions, hold short conversations when co-located, and
accumulate persistent memories. World logic has passing unit tests. The goroutine fan-out +
bounded worker pool is in place and a single `context` cancel stops the whole town cleanly.

## Next (not this session)

Reflection + planning (10–25 villagers) → seed a "throw a party Saturday" goal and capture
the emergent propagation (the headline clip) → embedded web UI (canvas tile map + live
dialogue feed via WebSocket) → scale flex to 100–1000 villagers with published memory/
throughput numbers. See `plan-noodleville.md` Phases 2–4.
