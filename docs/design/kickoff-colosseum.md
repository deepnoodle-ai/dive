# Kickoff Prompt — The Colosseum 🏛️

> Paste this into a fresh session to begin building. Full design: `docs/design/plan-colosseum.md`.
> Overall series context: `docs/design/demo-ideas.md`.

---

You are building **The Colosseum**: a cross-provider social-deduction arena where each
player is a *different LLM provider's model* (Claude, GPT, Gemini, Grok) competing in
Werewolf, with a live leaderboard and replayable matches that reveal each model's private
reasoning. This is a marketing/demo project for **Dive**, a Go library for building AI
agents. The whole point is to show off the one thing only Dive makes easy:
**many providers behind one interface in a single program.**

The shareable moment we're aiming at: *"We made Claude, GPT-5, Gemini, and Grok play
Werewolf against each other. Here's who lies best — and the receipts."*

## Context about Dive

- Dive lives at `/Users/curtis/git/deepnoodle/dive` (module `github.com/deepnoodle-ai/dive`). Go 1.25.
- Core API: `dive.NewAgent(dive.AgentOptions{...})` returns `*Agent`. The system-prompt
  field is `SystemPrompt`. Run a turn with `agent.CreateResponse(ctx, dive.WithInput(...))`;
  read text with `response.OutputText()`.
- Providers self-register; import e.g. `github.com/deepnoodle-ai/dive/providers/anthropic`
  (also `.../openai`, `.../google`, `.../grok`, `.../ollama`). Each provider's constructor
  takes variadic options; default models are defined in the provider package.
- Custom tools: `dive.FuncTool[T](...)` auto-generates a schema from a struct and returns
  `*dive.TypedToolAdapter[T]` (satisfies `dive.Tool`).
- Hooks: `AgentOptions.Hooks` groups hook slices. `PreToolUseHook` returns `error`
  (nil = allow, error = deny) and can rewrite args via `HookContext.UpdatedInput`. Use this
  for the referee. `StopHook` can return `Continue: true` to re-enter the loop.
- Per-agent memory: `session.New(id)` (in-memory) or a `FileStore`; set via
  `AgentOptions.Session` or `dive.WithSession(...)`.
- Streaming / capturing reasoning: `dive.WithEventCallback(fn)` on `CreateResponse`.
- Tests use `github.com/deepnoodle-ai/wonton/assert`.

**Verify exact signatures against the source before relying on them** — the plan was
written from an exploration pass, so confirm types in `dive.go`, `agent.go`, `hooks.go`,
`tool.go`, and the provider packages rather than trusting prose.

## Before you write code, read these

- `examples/subagent_example/` — multiple agents in one program, per-agent config.
- `examples/hooks_example/` — the `PreToolUse` gating pattern (basis for the referee).
- `tool_progress_example/` and any example using `WithEventCallback` — streaming the show.
- `examples/a2alib_example/` — only if you reach the optional A2A phase.
- `providers/anthropic/`, `providers/openai/`, `providers/google/`, `providers/grok/` —
  confirm constructor options and default model constants.

## Phase 1 deliverable (this session)

A **single-process, terminal-output Werewolf MVP** — no web UI, no A2A yet:

1. A pure-Go game engine: roles (Werewolf, Seer, Doctor, Villager), phase state machine
   (Night → Day → vote), win detection. Unit-tested with `wonton/assert`.
2. Players are `dive.NewAgent` instances built from **one shared `AgentOptions` template**
   with only `Model:` swapped per provider, each with its own `session`. Fairness demands
   an identical prompt template for every player — this is non-negotiable and must be
   easy to read/audit.
3. Typed action tools via `dive.FuncTool[T]`: `Speak`, `Vote`, `NightAction`. Each action
   must include a `reasoning` field so private thinking is captured comparably across
   providers (don't depend on provider-specific thinking channels).
4. A referee `PreToolUseHook` that rejects illegal moves and forces a retry with feedback.
5. A JSONL transcript (one event per line) capturing public actions + private reasoning.
6. A CLI runner: `colosseum run --players claude,gpt,gemini,grok` prints a full match.

**Where it lives:** build under `demos/colosseum/` in the dive repo as its own Go module —
`demos/colosseum/go.mod`, module path `github.com/deepnoodle-ai/dive/demos/colosseum`. The
repo uses local `replace` directives (no `go.work`; see `examples/go.mod` for the pattern), so
add `replace github.com/deepnoodle-ai/dive => ../..` for the core, plus one per *separate*
provider module you import: `providers/openai`, `providers/google`, `providers/grok` (each
`=> ../../providers/<name>`). Note the **anthropic** provider ships *inside* the core module,
so Claude needs no extra replace (same for ollama/mistral/openrouter). A separate module keeps
the arena/UI deps out of the core `go.mod` and lets the demo graduate to its own forkable repo
later via a directory move + a versioned `require` — no rewrite.

## Constraints & conventions

- Match the surrounding Go style; keep the engine provider-agnostic (the engine never
  imports a specific provider — players are injected).
- Cost-aware: make the model-per-player mapping configurable; default bulk runs to cheap
  tiers, reserve premium models for showcase matches. Log token usage per match.
- Handle rate limits / timeouts explicitly — never let a timeout silently become a forfeit;
  log it.
- Open-source the exact prompt template (a critic will check for unfair prompting).

## Definition of done for this session

`colosseum run` plays a complete, rules-valid Werewolf match between ≥3 different providers,
prints the phases to the terminal, and writes a replayable JSONL transcript that includes
each player's private reasoning. Engine logic has passing unit tests.

## Next (not this session)

Web replay viewer with a "reveal private reasoning" toggle → leaderboard (ELO + deception/
deduction/persuasion metrics) → optional A2A so contributors can enter their own agent.
See `plan-colosseum.md` Phases 2–3.
