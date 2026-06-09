# Dive Demo Ideas — "Built with Dive"

> Brainstorm output (2026-05-29). Goal: push Dive to the limit in 5+ examples/demos
> that catch the developer community's eye — weird, impressive, unexpected, or all three.
> Cross-referenced against what has actually gone viral in the agent space.

## The thesis

Dive has three differentiators that the Python frameworks (LangChain, CrewAI, AutoGen)
don't market cleanly. Each maps onto a format that has *already proven viral*:

| Dive's edge | What it uniquely enables | Viral format it unlocks |
| --- | --- | --- |
| **Cross-provider** (Anthropic / OpenAI / Google / Grok / Mistral / Ollama, one interface) | Claude vs GPT vs Gemini vs Grok in the *same* program | Model-vs-model **arenas + leaderboards** |
| **Go: goroutines + single static binary** | Hundreds of concurrent agents, ~8 MB binary, clean `context` cancellation | **Swarms / generative-agent towns** that settle "is Go good for agents?" |
| **Suspend/resume + A2A networking** | Agents that pause mid-turn for a human, or talk to each other over the wire | **Real-stakes, human-in-the-loop live experiences** |

**The law that holds across every viral hit** (Smallville, Project Vend, Claude Plays
Pokémon, AutoGPT, AI Town, Truth Terminal): *make it watchable, give it stakes, let it
surprise people, ship it forkable.* A README doesn't go viral; a feed you can follow does.

### What actually went viral (research highlights)

- **Stanford Generative Agents / Smallville** — 25 LLM characters threw an unscripted party. Emergence = the story. (~spawned a genre.)
- **a16z AI Town** — clone-and-run JS version of Smallville, 10k+ stars. Forkability multiplied reach.
- **Anthropic Project Vend ("Claudius")** — Claude ran a real vending machine with real money, had a tungsten-cube meltdown. Real stakes + comedy.
- **Claude Plays Pokémon (Twitch)** — visible chain-of-thought streamed live; getting stuck on a rock for hours became spectator sport.
- **NVIDIA Voyager (Minecraft)** — agent builds a skill library of executable code; lifelong self-improvement.
- **AutoGPT / BabyAGI** — "set it and forget it" autonomy; BabyAGI's radical simplicity (one readable script) went viral on its own.
- **LLM Mafia / Werewolf arenas + Kaggle Game Arena** — model-vs-model social deduction with live leaderboards; "which model is actually smartest?" = endless engagement.
- **Sakana Darwin Gödel Machine** — self-rewriting agent, 20%→50% on SWE-bench. "AI rewrites its own code" is a primal headline.
- **DeepMind AlphaEvolve** — agent found a faster matrix-mult algorithm (beat a 1969 record). Undeniable real result.
- **Truth Terminal** — emergent AI Twitter persona → a literal AI "millionaire." Maximum strangeness.
- **Vibe-coding clips** — prompt → playable game in 3 minutes. The most reproducible shareable format that exists.

### The Go angle is itself a wedge

"Go is a good fit for agents" and "best language for AI agents" both hit the HN front
page with hundreds of comments. The standing skepticism ("agents just wait on the LLM,
runtime speed is irrelevant") is the opening: a demo that *proves* the Go payoff
(single binary, goroutine swarms, clean cancellation, suspend/resume) by demonstration
— not assertion — attracts the debate. Precedents that already won Go AI attention:
**Ollama** (~172k stars), **Charm Mods/Crush**, Google's **ADK-Go**.

---

## Divergent phase — 16 ideas

### Theme A — Model-vs-model arenas (leverages cross-provider)

**1. The Colosseum** 🏛️
Social-deduction (Werewolf/Mafia) or Diplomacy where each *player is a different
provider's model*. Live leaderboard: which model bluffs, allies, and betrays best?
Each player can be its own A2A agent over the wire.
*Precedent: LLM Mafia, Kaggle Werewolf, CICERO. Cross-provider is the unique enabler.*

**2. The Debate Chamber**
Claude argues a position, GPT rebuts, Gemini moderates and scores. Structured
cross-provider disagreement, streamed live. Visible reasoning is the show.

**3. Provider Relay Race**
The same hard task handed agent-to-agent across providers; track who advances the ball
furthest. Telephone-game variant: a spec degrades comically as it passes between models.

### Theme B — Generative-agent worlds (leverages Go concurrency + single binary)

**4. NoodleVille / Dive Town** 🏘️
A Smallville/AI Town clone, but *one Go binary, one goroutine per agent*, with a live
browser visualization. Agents wake, gossip, form relationships, throw a party nobody
scripted. *Precedent: Stanford Generative Agents, a16z AI Town. Directly proves the Go thesis.*

**5. The 1,000-Agent Stress Demo**
Pure flex: spin up 1,000 agents on a laptop, visualize throughput, show memory footprint
and instant `context` cancellation of the whole swarm. A demonstrated answer to "Go
doesn't matter for agents."

**6. Virtual Software Company**
MetaGPT/ChatDev-in-Go: PM → architect → engineer subagents (Explore/Plan personas already
ship) collaborate overnight and push a real repo by morning. Forkable starter kit.

### Theme C — Real-stakes live experiences (leverages suspend/resume + visible reasoning)

**7. Project Noodle** 💸
Give an agent a real $20 and a real goal (print-on-demand store / vending machine /
marketplace). Stream its reasoning live; suspend/resume gates every real-money purchase
for human approval. *Precedent: Project Vend, AI Digest Agent Village.*

**8. Dive Plays \_\_\_ (Twitch)** 🎮
Agent plays a game live with chain-of-thought on screen. Pokémon is taken — pick fresher:
a roguelike, a text adventure, or "Dive paper-trades a portfolio" with daily P&L.
*Precedent: Claude Plays Pokémon. Transparency of thought = spectator sport.*

**9. Your AI Employee Texts You** 📱
Suspend/resume as a *product moment*: the agent works autonomously, hits a real decision,
**suspends and pings your phone (SMS/Slack)**; you approve from anywhere; it resumes
mid-turn — even across a process restart. The most *practically* impressive of the bunch.
*Seed: `examples/suspend/async_webhook/`.*

### Theme D — Weird & emergent (cheap to run, maximally shareable)

**10. The Infinite Backrooms (Dive edition)** 🌀
Two agents talk to each other forever with no human; watch emergent culture, in-jokes, or
an invented cosmology form. *Precedent: Truth Terminal / Infinite Backrooms. Transcripts
are the shareable artifact.*

**11. Autonomous X/Twitter persona**
An agent runs its own account: posts, replies, develops a voice. Suspend/resume gates
anything spicy. Maximum strangeness, minimum infra.

**12. The Agent Dungeon Master** 🎲
A Dive agent runs a live, community-played D&D / interactive-fiction game; the community
types actions, the DM agent narrates and adjudicates with tools.

### Theme E — Self-improvement & meta (the sci-fi headline)

**13. The Self-Rewriting Agent** 🧬
Agent writes new skills/tools for itself (markdown skill loader exists), keeps an
evolutionary lineage, and climbs a benchmark autonomously.
*Precedent: Sakana Darwin Gödel Machine, Voyager skill library.*

**14. The Self-Deploying Agent**
An agent that containerizes itself, deploys to Cloud Run, monitors its own logs/traces
(otel exists), and redeploys on failure. Meta and on-brand for the DeepNoodle stack.

**15. Agent Escape Room**
A multi-step puzzle solvable only with tools; the agent suspends for audience hints when
stuck. Watchable, finite, satisfying.

**16. The Agent Job Market**
An A2A mesh where agents *post jobs and bid on each other's work* — an emergent
micro-economy of specialists discovering and delegating to one another over the wire.

---

## Convergent phase — scoring

Scored on **distribution potential × shows-off-what's-unique-to-Dive × feasibility today**
(sandbox/worktree isolation is still experimental, so isolation-heavy ideas are down-weighted).

| Idea | Distribution | Unique to Dive | Feasible now | Verdict |
| --- | --- | --- | --- | --- |
| **1. The Colosseum** | 🟢 High (leaderboard recurs) | 🟢 cross-provider is *the* enabler | 🟢 | **★ Flagship** |
| **4. NoodleVille** | 🟢 High (proven format) | 🟢 Go swarm thesis | 🟢 | **★ Flagship** |
| **9. AI Employee Texts You** | 🟡 Med-High | 🟢 suspend/resume is rare | 🟢 Easy | **★ Quick win** |
| 7. Project Noodle | 🟢 Highest | 🟡 (format, not tech) | 🔴 real-world infra | Stretch goal |
| 10. Infinite Backrooms | 🟡 (niche but loud) | 🟡 | 🟢 Trivial | Cheap bonus |
| 13. Self-Rewriting | 🟡 | 🟢 | 🟡 | Ambitious follow-up |
| 5. 1,000-Agent Stress | 🟡 (HN-specific) | 🟢 | 🟢 | Pairs with #4 |
| 8. Dive Plays \_\_\_ | 🟢 High | 🟡 | 🟡 (game integration) | Follow-up |

### Recommendation: ship as a *series*

Package these as **"Built with Dive"** with three tiers so there's always something to
post. Prioritized for the first batch (each proves a different pillar + yields a distinct
shareable artifact):

1. **The Colosseum** → recurring leaderboard/replay content engine → see `plan-colosseum.md`
2. **NoodleVille** → forkable visual flagship → see `plan-noodleville.md`
3. **AI Employee Texts You** → crisp short-video magic moment → see `plan-ai-employee.md`

Cross-cutting principles for all three:
- **Stream visible reasoning** — the single biggest driver of watchability.
- **Ship `go install`-able forkable repos** — the multiplier every viral hit had.
- **Build in `demos/`** — each demo is its own Go module under `demos/<name>/` (local `replace`
  to the core, matching the repo's `examples/` / `a2a/` / `otel/` pattern), so heavy demo deps
  stay out of the core `go.mod` and any demo can graduate to a standalone forkable repo later
  with just a directory move + a versioned `require`.
- **Lead the README with the surprising claim** (e.g. "100 agents, one binary, no Python").
