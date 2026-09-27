# Pi: what the issue tracker says (for Dive)

- **Repo:** [earendil-works/pi](https://github.com/earendil-works/pi). This is the TypeScript monorepo for `pi-ai` (the unified LLM API), `pi-agent-core` (the agent loop, and now the durable `AgentHarness`), `pi-coding-agent` (the CLI/TUI), and `pi-tui`.
- **Previous home:** `badlogic/pi-mono`. In April 2026 the repo was **transferred** to the Earendil org, and the npm scope changed from `@mariozechner/*` to `@earendil-works/*` ([#4349](https://github.com/earendil-works/pi/issues/4349)). GitHub redirects the old repo, so its whole issue history is included here: issue numbers carry over, and old `badlogic/pi-mono/issues/N` links resolve to the same issue. (`badlogic/pi` is a different, unrelated project: a vLLM pod manager.)
- **Snapshot:** 2026-09-26
- **Companion doc:** [`pi.md`](pi.md), a source-level analysis. §7 maps the issue themes onto its findings.

Links: issues are `https://github.com/earendil-works/pi/issues/N`, PRs are `.../pull/N`, and discussions are `.../discussions/N`.

---

## 1. Method

### Repo stats

| Metric | Value |
|---|---|
| Stars / forks | ~109.6k / ~13.9k |
| Issues (all time) | 6,450 (160 open, 6,290 closed) |
| Issue date range | 2025-11-13 to 2026-09-26 |
| Close reasons | 3,395 completed, 2,890 not planned, 128 reopened-then-open, 5 duplicate |
| Pull requests | 3,269 (1,009 merged, 2,194 closed unmerged, 66 open) |
| Discussions | 313, all in "General" |
| Monthly issue volume | 57 (Nov 2025), 290 (Jan 2026), 927 (Apr), 1,015 (Aug), 844 (Sep so far) |
| Median time to close | Minutes. Most issues are auto-closed by a bot on arrival (see below). |
| Top labels | `no-action` 2,811, `bug` 1,791, `possibly-openclaw-clanker` 398, `closed-because-weekend` 307, `last-read` 295, `closed-because-refactor` 224, `closed-because-bigrefactor` 122 |

**The tracker needs to be read differently from the others in this set.** Since early 2026, every issue and PR from a contributor who has not been approved is **auto-closed on arrival** ([CONTRIBUTING.md](https://github.com/earendil-works/pi/blob/main/CONTRIBUTING.md)). Maintainers then review the closed issues daily and reopen the good ones. The approval commands are `lgtmi` (your future issues stay open) and `lgtm` (you may open PRs). As a result:

- "Closed" says almost nothing about whether an issue was resolved. `stateReason: NOT_PLANNED` with the `no-action` label (2,811 issues) means "auto-closed and not picked up." It does not mean "rejected on the merits."
- Reactions are low. The most-reacted issue has 80. Users rarely +1 a closed issue, so duplicates pile up instead. One user said so directly: *"what good does auto-closing of issues do besides encouraging duplicate filings"* ([#5084](https://github.com/earendil-works/pi/issues/5084)).
- There were three blanket freezes. From 2026-04-24 to 2026-05-17, 653 issues were closed under the `closed-because-weekend`, `-refactor`, and `-bigrefactor` labels while the "bigrefactor" landed. The bot's message explained that *"issue triage has been taking about 8 hours per day"* ([#4251](https://github.com/earendil-works/pi/issues/4251)).
- A lot of the volume is AI-generated. The `possibly-openclaw-clanker` label (398 issues) marks suspected agent-written reports. Maintainers regularly tell commenters to *"speak in your natural voice"* ([#6306](https://github.com/earendil-works/pi/issues/6306), [#4877](https://github.com/earendil-works/pi/issues/4877)).

### How I found the signal

1. **Pulled all 6,450 issues** with `gh issue list --json ...reactionGroups,comments,labels,stateReason`, all 3,269 PRs, and all 313 discussions (GraphQL). I ranked them by reactions, by comments, and by `2 × reactions + comments`.
2. **Comments beat reactions here.** Because issues are auto-closed, the long threads mark the real pain. Examples: [#4945](https://github.com/earendil-works/pi/issues/4945) (Codex hangs, 80 comments), [#7547](https://github.com/earendil-works/pi/issues/7547) (Windows, 68), [#5825](https://github.com/earendil-works/pi/issues/5825) (streaming scroll, 42), [#6278](https://github.com/earendil-works/pi/issues/6278) (edit-tool schema, 25).
3. **Reopened issues are a curated signal.** An issue with `REOPENED` state, or with the `inprogress` or `to-discuss` labels, is one a maintainer chose to rescue from the auto-close queue. I read most of the 160 open issues for that reason.
4. **Keyword clustering over titles** to find themes made of many small issues. Counts by title keyword: TUI/rendering 868, extension API 763, streaming/timeout/retry/hang 749, session/branch/tree 694, install/packaging 630, thinking/reasoning 382, tool schema/calls 346, compaction 340, Chinese-lab models (Kimi, GLM, DeepSeek, Qwen, MiMo) 322, OpenAI-compat/local 285. Two narrower clusters matter most for Dive: **thinking-block replay/signatures, 82 issues**, and **transcript integrity (orphaned tool calls/results, "Cannot continue"), 63 issues**.
5. **Maintainer-authored issues** (98 by `badlogic`/Mario Zechner, 18 by `mitsuhiko`/Armin Ronacher). These act as the roadmap and the design documents: session tree, extension unification, SDK, compaction, strict tools.
6. **Maintainer comments** that state a stance: "build it as an extension", "out of scope", "no repro", "I don't need a PR, we need data".
7. **Discussions** are where users praise the product and argue about philosophy: ACP ([D#4444](https://github.com/earendil-works/pi/discussions/4444), 40 upvotes), benchmarks ([D#1637](https://github.com/earendil-works/pi/discussions/1637), [D#6646](https://github.com/earendil-works/pi/discussions/6646)), sandboxing ([D#6253](https://github.com/earendil-works/pi/discussions/6253), [D#1874](https://github.com/earendil-works/pi/discussions/1874)), negative first-run feedback ([D#3735](https://github.com/earendil-works/pi/discussions/3735)), and pi-agent-core as an embedded runtime ([D#3337](https://github.com/earendil-works/pi/discussions/3337)).
8. **Top PRs by reactions** as a second demand signal: Anthropic-on-Vertex [PR#5262](https://github.com/earendil-works/pi/pull/5262) (46), Codemode + MCP [PR#10040](https://github.com/earendil-works/pi/pull/10040) (48, opened by mitsuhiko the day before this snapshot), and Bedrock Mantle [PR#6216](https://github.com/earendil-works/pi/pull/6216) (31).

**Bot and maintainer tracking vs. real user feedback.** I treated maintainer-authored issues as *design records*, not demand. Examples: [#316](https://github.com/earendil-works/pi/issues/316) session tree, [#454](https://github.com/earendil-works/pi/issues/454) extensions, [#6451](https://github.com/earendil-works/pi/issues/6451) harness cleanup, [#7772](https://github.com/earendil-works/pi/issues/7772) memory. I ignored joke issues ([#4609](https://github.com/earendil-works/pi/issues/4609) "Rewrite pi in Rust", body: "joke", 21 reactions; [#4613](https://github.com/earendil-works/pi/issues/4613)), test issues, and obviously agent-generated spam. I counted `possibly-openclaw-clanker` issues only when a human confirmed them in the thread.

---

## 2. What users love

**1. A tiny core that users extend themselves, often by asking pi to write the extension.** This is the main reason people adopt pi. Typical comments: *"the reason that I decided to quit claude code and stick with pi is I don't need a lot of features and bugs that I would never use. And I can build my own extensions within a minute or two"* ([D#3373](https://github.com/earendil-works/pi/discussions/3373)), and *"I've had so much fun tweaking my harness"* ([D#5951](https://github.com/earendil-works/pi/discussions/5951)). The maintainers answer feature requests with *"you can replace the built in bash tool via extension easily and experiment? ask pi."* ([D#1632](https://github.com/earendil-works/pi/discussions/1632)). The extension model was deliberately merged into one concept: `pi.registerTool`, `pi.on(event)`, `registerCommand`, `registerProvider`, all sharing closure state ([#326](https://github.com/earendil-works/pi/issues/326), [#454](https://github.com/earendil-works/pi/issues/454)). As a result, things like MCP, subagents, rewind, sandboxes and permissions all live in a community package ecosystem (`pi install npm:...`) instead of in core.

**2. A lean context budget.** Users running local models with 32k windows say pi is *"the first agent that truly feels usable with a fully local stack"* ([D#1632](https://github.com/earendil-works/pi/discussions/1632)). An independent benchmark found pi *"consistently had the smallest initial prompt and the least total context traffic,"* with 81% less total input than OpenCode on an MCP-heavy task, at about half the cost ([D#6646](https://github.com/earendil-works/pi/discussions/6646)). Users push back when the system prompt grows: *"I would appreciate if pi stayed with a minimal system prompt"* ([#7128](https://github.com/earendil-works/pi/issues/7128), 14 reactions).

**3. Real multi-provider support, including subscriptions, with model switching mid-session.** Early maintainer work went straight to Copilot, Gemini CLI, Antigravity and Codex OAuth ([#84](https://github.com/earendil-works/pi/issues/84)). It also went to cross-provider *hand-off*: take a transcript produced by model A and continue it on model B ([#198](https://github.com/earendil-works/pi/issues/198), [#258](https://github.com/earendil-works/pi/issues/258)). Plugins like "Downshift", which starts on a premium model and drops to a cheap one once context grows ([D#3373](https://github.com/earendil-works/pi/discussions/3373)), only work because hand-off works.

**4. Sessions as a tree (`/tree`, `/fork`).** The JSONL session format is append-only with `id`/`parentId`, and it keeps abandoned branches with summaries ([#316](https://github.com/earendil-works/pi/issues/316), [#290](https://github.com/earendil-works/pi/issues/290)). Some users were puzzled at first (*"What is the point of the tree functionality?"* since files don't roll back), and others then described real workflows. One branches at the root of a book for each question. Another fans out "Do {A,B,C,D}" from a shared primed context ([D#5205](https://github.com/earendil-works/pi/discussions/5205)).

**5. `pi-agent-core` as "just the agent turn."** A team building customer-hosted scheduled agents chose it over LangGraph.js. They wrote: *"pi-agent-core is deliberately 'just the agent turn' which is exactly what a durable-orchestration activity wants to be,"* and also praised the `beforeToolCall`/`afterToolCall` hooks, *"Context as plain JSON persists trivially,"* and *"small surface area, readable end to end"* ([D#3337](https://github.com/earendil-works/pi/discussions/3337)).

**6. Pluggable tool back ends.** Built-in `read`/`write`/`edit`/`bash` accept an `operations` object, so a host can send file I/O and exec over SSH or into a VM without rewriting the tools ([#564](https://github.com/earendil-works/pi/issues/564)). Gondolin, Earendil's micro-VM sandbox, uses this.

**7. Maintainers who ship fast and debug with data.** The Anthropic OAuth break was fixed within the hour (*"Nice, thanks for the quick fix!"*, [#581](https://github.com/earendil-works/pi/issues/581)). On the edit-tool schema dispute, mitsuhiko ran *"500 simulations ... across opus 4.8, sonnet 4.5 and sonnet 5"* before changing anything ([#6278](https://github.com/earendil-works/pi/issues/6278)). Threads like [D#1644](https://github.com/earendil-works/pi/discussions/1644) (*"Pi is now one of my favorite pieces of software EVER"*) and [D#5951](https://github.com/earendil-works/pi/discussions/5951) show how loyal the users are.

---

## 3. What users hate / friction

### 3.1 The contribution gate and blanket closures
This is the most emotionally charged theme. Complaints: *"Issues get closed automatically with silly reasons ... Closed because weekend? Really? ... I think it's time to fork this"* ([D#4285](https://github.com/earendil-works/pi/discussions/4285)). Contributors write fixes, the gate auto-closes them, and they then have to ask for `lgtm` in the comments ([#8845](https://github.com/earendil-works/pi/issues/8845), [#5223](https://github.com/earendil-works/pi/issues/5223): *"I've the fix but can't open a pr unless anyone approves"*). The maintainers frame the gate as protection against burnout and AI slop (CONTRIBUTING FAQ). *Takeaway for Dive:* the gate protects the maintainers, but it hides demand. Pi's real feature signal is buried in duplicates.

### 3.2 Configuration location and defaults
The two most-reacted issues in the whole tracker are about **where config lives**: XDG compliance [#2870](https://github.com/earendil-works/pi/issues/2870) (80 reactions) and [#534](https://github.com/earendil-works/pi/issues/534) (50), plus a follow-up [#7274](https://github.com/earendil-works/pi/issues/7274) and mitsuhiko's own [#5671](https://github.com/earendil-works/pi/issues/5671) (`~/.pi` and `cwd/.pi` overlap). badlogic regrets the layout but won't migrate existing users (*"I regret not doing this from the beginning"*, [#534](https://github.com/earendil-works/pi/issues/534)). The lesson is that on-disk layout is a day-one decision. Related complaints:
- In-session model and thinking changes silently overwrote the global defaults ([#5263](https://github.com/earendil-works/pi/issues/5263), 18 reactions; badlogic: *"yes."*).
- Session folders collide because of the path encoding: `/a-b/c-d` and `/a/b/c/d` map to the same folder ([#4877](https://github.com/earendil-works/pi/issues/4877)).

### 3.3 Docs and onboarding, especially for local models and the SDK
A first-time user needed `compat.supportsDeveloperRole=false` for Qwen and found that *"'Error: 400 Unexpected message role', is not found anywhere in the doc"* ([D#3735](https://github.com/earendil-works/pi/discussions/3735)). SDK adopters say *"there is no detailed documents about the pi-agent-core. So we have to look into the code"*, and they ask whether the new lane-based session API in 0.84 is stable ([D#3337](https://github.com/earendil-works/pi/discussions/3337)).

### 3.4 Packaging and breaking changes for library users
- The scope rename broke extensions ([#1820](https://github.com/earendil-works/pi/issues/1820), 12 reactions; [#1831](https://github.com/earendil-works/pi/issues/1831); [D#4285](https://github.com/earendil-works/pi/discussions/4285)).
- The npm shrinkwrap and supply-chain pinning that suits a CLI clashes with library use. mitsuhiko: *"I'm complete at a loss of what to do here to be honest"* ([#5653](https://github.com/earendil-works/pi/issues/5653), 21 comments).
- Importing the SDK installs pi's nested `undici` as the **process-global fetch dispatcher**, which breaks the host's own streaming ([#9787](https://github.com/earendil-works/pi/issues/9787)).
- The SDK needs a `package.json` next to the package at runtime ([#5226](https://github.com/earendil-works/pi/issues/5226)).

The CLI and the library share one release train, and library users take the damage.

### 3.5 "Build it as an extension" as the answer to knobs
- Sampling parameters: *"my answer stays the same: if you need this, build it as an extension"* ([#1837](https://github.com/earendil-works/pi/issues/1837), [#1392](https://github.com/earendil-works/pi/issues/1392)).
- Structured output was refused. The suggested pattern is a "final answer" tool ([#1086](https://github.com/earendil-works/pi/issues/1086)).
- Enabling built-in tools in settings ([#5084](https://github.com/earendil-works/pi/issues/5084)).

Power users accept this. Newcomers and people running local models find it steep.

### 3.6 TUI performance and Windows
These are the largest clusters by count, but they matter less to Dive: TUI CPU pinned during streaming ([#6665](https://github.com/earendil-works/pi/issues/6665), [#7730](https://github.com/earendil-works/pi/issues/7730)), streaming scroll-jumps ([#5825](https://github.com/earendil-works/pi/issues/5825), 42 comments), and the Windows "sink thread" ([#7547](https://github.com/earendil-works/pi/issues/7547), 68 comments). The Windows path handling feeds into the SDK: custom tool `operations` receive host-OS-resolved paths, so a Windows host driving a Linux remote gets `C:\root\x` ([#5350](https://github.com/earendil-works/pi/issues/5350)). mitsuhiko on translating paths centrally: *"it's a losing game"* ([#7547](https://github.com/earendil-works/pi/issues/7547)).

### 3.7 Defaults that change under users
Project-trust gating landed and annoyed people right away (*"I'm already annoyed by it"*, [#5514](https://github.com/earendil-works/pi/issues/5514), 26 comments). Others defended it: *"as pi reaches a wider audience it is really important that the defaults protect people"*. Separately, adding one line to the system prompt caused unwanted bash calls ([#7128](https://github.com/earendil-works/pi/issues/7128)), and a date in the system prompt broke prefix caching on slow-prefill local hardware ([#6621](https://github.com/earendil-works/pi/issues/6621); badlogic: *"we should remove the date from the system prompt"*).

---

## 4. Big problems

These are the areas where bugs keep coming back, not one-off reports. They are the most relevant part of this doc for Dive's core design.

### 4.1 Cross-provider transcript fidelity (the defining problem)
Pi keeps **one canonical message model** and replays it into every provider's wire format. That design makes hand-off possible, and it is also where most of the hard bugs come from. The thinking/reasoning replay cluster alone has 82 issues:
- **Signed thinking must be replayed byte-exact.** Anthropic rejects any change to thinking blocks in the latest assistant message, including surrogate sanitizing and empty-block filtering ([#5223](https://github.com/earendil-works/pi/issues/5223)). Stale signed blocks replayed after compaction get dropped or rejected ([#9391](https://github.com/earendil-works/pi/issues/9391), [#9652](https://github.com/earendil-works/pi/issues/9652)).
- **`reasoning_content` requirements vary by lab and by gateway.** Kimi, DeepSeek and MiMo require reasoning to be echoed back, and some reject `""` ([#4251](https://github.com/earendil-works/pi/issues/4251), 23 comments; [#4505](https://github.com/earendil-works/pi/issues/4505); [#3636](https://github.com/earendil-works/pi/issues/3636); [#7702](https://github.com/earendil-works/pi/issues/7702)). Gateways that load-balance across back ends return `reasoning` on one call and expect `reasoning_content` on the next. mitsuhiko: *"the actual fix is upstream ... instead of throwing more hacks into Pi"*. The fix eventually shipped anyway, scoped to one provider.
- **Gemini `thoughtSignature`** is lost when Gemini sits behind OpenAI-compatible gateways ([#9444](https://github.com/earendil-works/pi/issues/9444), [#6996](https://github.com/earendil-works/pi/issues/6996), [#6733](https://github.com/earendil-works/pi/issues/6733)).
- **Hand-off between providers:**
  - Tool-call IDs from the Responses API are 450+ characters and contain `|`, while other providers allow at most 40 characters of `[a-zA-Z0-9_-]` ([#198](https://github.com/earendil-works/pi/issues/198)).
  - Claude thinking and text from the same turn produced duplicate item IDs on OpenAI ([#5148](https://github.com/earendil-works/pi/issues/5148)).
  - Thinking gets inlined as plain text when switching models, which then fights with the compat flags that require reasoning content ([#6167](https://github.com/earendil-works/pi/issues/6167)).
  - Anthropic's server-side fallback mid-stream changes the model partway through a turn ([#9074](https://github.com/earendil-works/pi/issues/9074)).
- **Stream-shape assumptions:**
  - Chat Completions deltas can carry `content`, `reasoning_content` and `tool_calls` together, with no ordering between them ([#4228](https://github.com/earendil-works/pi/issues/4228), 19 comments). mitsuhiko concluded that content order *"cannot be treated as faithful order"* and proposed a canonical order instead.
  - `content: null` alongside `tool_calls` crashes iteration ([#6259](https://github.com/earendil-works/pi/issues/6259)). mitsuhiko's fix: stop provider adapters from ever *emitting* malformed messages.
  - Tool-argument parsing is O(n²) when deltas arrive fragmented ([#9062](https://github.com/earendil-works/pi/issues/9062), [#9265](https://github.com/earendil-works/pi/issues/9265)).
- **"OpenAI-compatible" is not one thing.** Pi ships a growing `compat` flag set (`supportsDeveloperRole`, `requiresReasoningContentOnAssistantMessages`, `thinkingFormat`, ...), and it still sends OpenAI-only fields to compatible back ends ([#9508](https://github.com/earendil-works/pi/issues/9508)). Examples: optional-object schemas are not normalized ([#7010](https://github.com/earendil-works/pi/issues/7010)); whitespace-only tool output returns a 400 and *"permanently bricks the session"* ([#8720](https://github.com/earendil-works/pi/issues/8720)); the Anthropic adapter silently drops a root-level `anyOf` from tool schemas ([#9134](https://github.com/earendil-works/pi/issues/9134), [#9557](https://github.com/earendil-works/pi/issues/9557)).
- **Keeping reasoning costs memory.** 0.84.3 started persisting reasoning on every message. Sessions grew 2–3× and processes were OOM-killed at 20 GB or more ([#8746](https://github.com/earendil-works/pi/issues/8746)).

badlogic's answer in December 2025 was an **all-pairs, live hand-off test matrix** across every provider, including OAuth ones ([#258](https://github.com/earendil-works/pi/issues/258)). The flood of issues since then shows the matrix can't keep up with new models and gateways.

### 4.2 Compaction is a second request path that drifts from the first
Compaction has 340 issues. The recurring root cause is that **summarization calls do not go through the same pipeline as normal turns**:
- Compaction requests leave out headers or session IDs the main path sends: Copilot Enterprise returns 421 ([#6768](https://github.com/earendil-works/pi/issues/6768), 22 reactions), and Codex returns "model not found" ([#6477](https://github.com/earendil-works/pi/issues/6477)). The `before_provider_request` hook does not fire for them ([#9773](https://github.com/earendil-works/pi/issues/9773)). OpenRouter summary requests drop `x-session-id` ([#10022](https://github.com/earendil-works/pi/issues/10022)).
- Summaries inherit the session's thinking level and model, and users want to configure them separately ([#7553](https://github.com/earendil-works/pi/issues/7553); badlogic: *"Aye, should also include compaction model."*; [#8133](https://github.com/earendil-works/pi/issues/8133); [#9075](https://github.com/earendil-works/pi/issues/9075)).
- Hardcoded output caps: branch summaries were capped at `maxTokens: 2048` regardless of model, and reasoning models hit the cap every time ([#8845](https://github.com/earendil-works/pi/issues/8845); [#9512](https://github.com/earendil-works/pi/issues/9512)). A truncated summary once got persisted as if complete ([#7048](https://github.com/earendil-works/pi/issues/7048)).
- **When compaction triggers:** it was checked only at `agent_end`, so one long tool loop can run past 100% of the window until the provider rejects it ([#6879](https://github.com/earendil-works/pi/issues/6879), 21 reactions; *"I had to give up that session"*). This was recognized back in [#1884](https://github.com/earendil-works/pi/issues/1884) and [#128](https://github.com/earendil-works/pi/issues/128).
- **Token accounting:**
  - The budget ignores the output reservation ([#8061](https://github.com/earendil-works/pi/issues/8061)).
  - `estimateTokens()` can't see reasoning carried only in signatures ([#9409](https://github.com/earendil-works/pi/issues/9409): *"Sessions wedge permanently at the context ceiling"*).
  - The compaction prompt itself includes thinking and overflows ([#10033](https://github.com/earendil-works/pi/issues/10033), [#9602](https://github.com/earendil-works/pi/issues/9602)).
- Anthropic's refusal classifier can reject a compaction request ([#8017](https://github.com/earendil-works/pi/issues/8017)).

The maintainers' own diagnosis for the new harness: there are *"multiple independent session-entry-to-context projections"*, one each for normal context, compaction and branch summary, and this *"causes semantic drift"* ([#6451](https://github.com/earendil-works/pi/issues/6451)). The proposed fix is **one projection function as the single source of truth**.

### 4.3 Agent-loop lifecycle: "ended" is not "settled"
mitsuhiko's meta-issue ([#5886](https://github.com/earendil-works/pi/issues/5886)) sums up a long series of bugs (*"Cannot continue from message role: assistant"*). The cause: `agent_end` fires when the inner loop is out of work, but the session may still retry, compact-and-retry, drain queued steering or follow-up messages, or run extension handlers that enqueue more work. Hosts that treat `agent_end` as idle race with all of these.
- An `agent_settled` event was requested and spiked, and then dropped (badlogic: *"This is more complex than it seems ... yeah, let's not."*, [#2110](https://github.com/earendil-works/pi/issues/2110)). Its replacement is a scheduler call, `runWhenIdle` ([#2023](https://github.com/earendil-works/pi/issues/2023), [#1884](https://github.com/earendil-works/pi/issues/1884)).
- **Transcript integrity** (63 issues):
  - An aborted or errored turn leaves unmatched `toolCall` blocks, and the next continue call is rejected ([#9306](https://github.com/earendil-works/pi/issues/9306), [#9986](https://github.com/earendil-works/pi/issues/9986), [#9124](https://github.com/earendil-works/pi/issues/9124)).
  - Parallel tool batches persist results only after the whole batch finishes, so one stalled sibling loses results the user watched succeed ([#7053](https://github.com/earendil-works/pi/issues/7053)).
  - An extension message injected mid-batch breaks tool_call→tool adjacency ([#8166](https://github.com/earendil-works/pi/issues/8166), [#8502](https://github.com/earendil-works/pi/issues/8502)).
  - A tool result was persisted before its assistant message ([#1717](https://github.com/earendil-works/pi/issues/1717)).
- **Hangs** (240 "stuck/hang/Working..." issues):
  - A stalled SSE stream is awaited forever ([#8331](https://github.com/earendil-works/pi/issues/8331)). badlogic: *"local models can stall for much much longer ... can't have a low default"*.
  - Codex sits on "Working..." with zero usage ([#4945](https://github.com/earendil-works/pi/issues/4945), 80 comments, still open, cause unclear; the suspects include a proxy, a WebSocket transport, and a Retry-After of more than 60 s).
  - undici's default `bodyTimeout` killed local streams at 5 minutes ([#3715](https://github.com/earendil-works/pi/issues/3715), [#3711](https://github.com/earendil-works/pi/issues/3711)).
  - The OpenAI SDK slept for a Retry-After measured in days, and Escape could not abort it ([#6911](https://github.com/earendil-works/pi/issues/6911)).
  - An extension hook that never settles blocks Escape ([#6234](https://github.com/earendil-works/pi/issues/6234)).
- `pi-agent-core` turned non-abort loop errors into empty assistant messages, which hid them ([#2188](https://github.com/earendil-works/pi/issues/2188)).
- Codex's `end_turn: false` ("the model wants to be sampled again") was ignored, so the loop stopped early ([#7689](https://github.com/earendil-works/pi/issues/7689)).

### 4.4 Tool-call validation vs. model sloppiness
Recent Claude models add keys the schema doesn't define, such as `in_file`, `newText_2` and `type`, to about 20% of multi-edit calls in some sessions. `additionalProperties:false` then rejects the whole call ([#6278](https://github.com/earendil-works/pi/issues/6278), [#5501](https://github.com/earendil-works/pi/issues/5501)). After controlled experiments the fix was to **accept and ignore extra keys**, not to enable strict decoding. mitsuhiko: *"Strict tool invocation is unlikely going to be the right solution ... grammar aware sampling ... negative consequences for the quality."* The strict-tools design issue ([#6306](https://github.com/earendil-works/pi/issues/6306)) points out that Anthropic `strict: true` returns 400 when there are too many tools, and that *"all these features are completely distinct between providers and they are very leaky abstractions."*
- Related: small or local models emit objects where strings are expected ([#1259](https://github.com/earendil-works/pi/issues/1259)). Edit fuzzy-matching doesn't tolerate whitespace differences ([#7836](https://github.com/earendil-works/pi/issues/7836)). MCP tools get JSON-encoded strings for arrays ([#5697](https://github.com/earendil-works/pi/issues/5697), [#4226](https://github.com/earendil-works/pi/issues/4226)).
- Hosts want **validation they control**:
  - A per-tool opt-out so `execute` can return structured correction feedback instead of a generic "Validation failed" ([#7607](https://github.com/earendil-works/pi/issues/7607)).
  - Validation that fails *open* in Cloudflare Workers (AJV can't do codegen there) silently skipped all checks ([#3112](https://github.com/earendil-works/pi/issues/3112)).

### 4.5 Model metadata can't keep up
- Thinking levels:
  - The fixed `off … xhigh` ladder kept breaking. Anthropic and OpenAI added `max` ([#3299](https://github.com/earendil-works/pi/issues/3299); badlogic: *"sigh, thanks anthropic"*; [#6097](https://github.com/earendil-works/pi/issues/6097), 22 reactions).
  - Providers support only some rungs ([#3208](https://github.com/earendil-works/pi/issues/3208), [#3016](https://github.com/earendil-works/pi/issues/3016): *"Users see 'off' but get thinking enabled on the backend"*).
  - Bedrock and GLM drop the effort level ([#9331](https://github.com/earendil-works/pi/issues/9331), [#9678](https://github.com/earendil-works/pi/issues/9678)).
- Catalog and runtime discovery:
  - A local model can't be the default because discovery runs after the default is resolved ([#6922](https://github.com/earendil-works/pi/issues/6922), [#8167](https://github.com/earendil-works/pi/issues/8167)).
  - Context size defaults to 128k even when the real size is known ([#9566](https://github.com/earendil-works/pi/issues/9566)).
  - Extension-registered providers race the startup refresh ([#8810](https://github.com/earendil-works/pi/issues/8810), [#9962](https://github.com/earendil-works/pi/issues/9962)).
- Cost and usage:
  - OpenRouter cost is 2–3× off ([#9980](https://github.com/earendil-works/pi/issues/9980)).
  - Bedrock `usage.input` means different things per model family ([#8752](https://github.com/earendil-works/pi/issues/8752)).
  - 1h cache writes are billed at the 5m rate ([#9457](https://github.com/earendil-works/pi/issues/9457)).
  - A missing `clear_thinking:false` burned 5–10× quota on z.ai ([#6083](https://github.com/earendil-works/pi/issues/6083)).

### 4.6 Embedding pi as a library and running many sessions
- *"The current extension lifecycle assumes one process = one session"* ([D#1546](https://github.com/earendil-works/pi/discussions/1546)). Daemons that host several sessions have extensions overwriting each other's `globalThis` state.
- Live multi-session switching was declared impossible on the current architecture. badlogic: *"this is what pi server will be for. shoehorning this on the current architecture will not work"* ([#5700](https://github.com/earendil-works/pi/issues/5700)).
- Extensions can't reach the model runtime to spawn child sessions that share auth ([#8791](https://github.com/earendil-works/pi/issues/8791)).
- A shared `auth.json` across parallel RPC processes reports "No API key" for about 48 s ([#8928](https://github.com/earendil-works/pi/issues/8928)).
- `SessionManager.create()` reports `isPersisted()` but writes nothing until the first assistant message ([#9792](https://github.com/earendil-works/pi/issues/9792)).
- Hosts need a "waiting on user input" state distinct from "running" ([#5329](https://github.com/earendil-works/pi/issues/5329)).
- RPC regressions ([#9803](https://github.com/earendil-works/pi/issues/9803), ENOBUFS in [#4897](https://github.com/earendil-works/pi/issues/4897)).

### 4.7 Security is left to the host
Pi runs in full "YOLO mode" by design. Users repeatedly ask whether a sandbox is on by default ([D#1874](https://github.com/earendil-works/pi/discussions/1874)). One wrote: *"I've tried at least 10 different extensions ... nothing works"* ([D#6253](https://github.com/earendil-works/pi/discussions/6253)). Requests for native permissions ([#4459](https://github.com/earendil-works/pi/issues/4459)) and an extension approval primitive ([#5954](https://github.com/earendil-works/pi/issues/5954), not planned) got nowhere. The official answer is containerization (Gondolin, Docker, OpenShell in the README).

---

## 5. Most desired features (and maintainer response)

| Request | Signal | Maintainer response |
|---|---|---|
| **XDG / sane config location** | [#2870](https://github.com/earendil-works/pi/issues/2870) (80 reactions), [#534](https://github.com/earendil-works/pi/issues/534) (50), [PR#256](https://github.com/earendil-works/pi/pull/256) (18) | **Refused.** On [#534](https://github.com/earendil-works/pi/issues/534): *"I regret not doing this from the beginning ... migrating this automatically for them is super icky"*. On [#2870](https://github.com/earendil-works/pi/issues/2870): *"that's just ridiculous. things will stay as is"*. The escape hatch is an env var (`PI_CODING_AGENT_DIR`). This is the most-reacted issue in the tracker, and the refusal drew angry replies. |
| **MCP** | [#67](https://github.com/earendil-works/pi/issues/67), [#563](https://github.com/earendil-works/pi/issues/563), [PR#10040](https://github.com/earendil-works/pi/pull/10040) (48 reactions) | For a long time: no. badlogic said exposing MCP tools *"statically, always in context ... is not good"* and pointed to the community `pi-mcp-adapter`. **Changing now:** mitsuhiko's Codemode+MCP PR says *"there are some responsible uses of MCP and at this point the MCP baseline fits decently well into Pi."* |
| **ACP (Agent Client Protocol)** | [D#4444](https://github.com/earendil-works/pi/discussions/4444) (40 upvotes, the top discussion), [#175](https://github.com/earendil-works/pi/issues/175) | *"I currently have no need for ACP support ... trivial to build an ACP adapter on top of RPC mode."* The community bridge `pi-acp` has lifecycle bugs in production ([D#4444](https://github.com/earendil-works/pi/discussions/4444)). ACP agents as back ends: not planned ([#7320](https://github.com/earendil-works/pi/issues/7320)). |
| **Local LLM model discovery** | [#3357](https://github.com/earendil-works/pi/issues/3357) (47 reactions, 30 comments; filed by Hugging Face co-founder `julien-c`) | Rejected in core: *"model lists generally do not contain all the data that is needed to populate a `Model`"*. Pushed to an "official extension" that registers a provider. |
| **Thinking levels per model / `max`** | [#3208](https://github.com/earendil-works/pi/issues/3208), [#6097](https://github.com/earendil-works/pi/issues/6097), [#3299](https://github.com/earendil-works/pi/issues/3299) | Done. Per-model thinking metadata and `thinkingLevelMap` in `models.json`/`registerProvider` ([#3208](https://github.com/earendil-works/pi/issues/3208)); *"max is in."* ([#6097](https://github.com/earendil-works/pi/issues/6097)). |
| **Separate compaction model and thinking level** | [#7553](https://github.com/earendil-works/pi/issues/7553), [#8133](https://github.com/earendil-works/pi/issues/8133) | Accepted, in progress. |
| **Ephemeral in-session model changes** | [#5263](https://github.com/earendil-works/pi/issues/5263) (18 reactions) | *"yes."* Done. |
| **New providers** (Anthropic on Vertex, Bedrock Mantle, OpenCode Go) | [PR#5262](https://github.com/earendil-works/pi/pull/5262) (46), [#5363](https://github.com/earendil-works/pi/issues/5363) (15), [#1757](https://github.com/earendil-works/pi/issues/1757) | Slow. Provider PRs need the tests AGENTS.md requires, and many are still open. |
| **Sampling params** (temperature, top_p) | [#1837](https://github.com/earendil-works/pi/issues/1837), [#1392](https://github.com/earendil-works/pi/issues/1392) | Refused: *"historically, most providers have stopped interpreting/applying those parameters"*; write a custom provider. |
| **Structured output** | [#1086](https://github.com/earendil-works/pi/issues/1086) | Refused in `pi-ai`: *"expose a tool ... it must call at the end of its turn ... more reliable and supported by all providers."* |
| **Subagents / multi-agent** | [#552](https://github.com/earendil-works/pi/issues/552), [#5700](https://github.com/earendil-works/pi/issues/5700) | Kept out of core (*"reluctant to add a subagent abstraction"*), deferred to "pi server". |
| **Sandbox / permissions** | [D#6253](https://github.com/earendil-works/pi/discussions/6253), [#4459](https://github.com/earendil-works/pi/issues/4459), [#5954](https://github.com/earendil-works/pi/issues/5954) | Out of scope. Containerize. Project trust gating was the only concession ([#5514](https://github.com/earendil-works/pi/issues/5514)). |
| **Rewind files with conversation** | [D#1223](https://github.com/earendil-works/pi/discussions/1223), [D#5205](https://github.com/earendil-works/pi/discussions/5205) | Extension (`pi-rewind`). `/tree` manages context only. |
| **Python SDK** | [#4174](https://github.com/earendil-works/pi/issues/4174) | *"entirely out of scope for pi itself."* |
| **External events into a running session** | [#145](https://github.com/earendil-works/pi/issues/145) (mitsuhiko) | Covered by RPC queueing and later by extension `sendMessage`. |
| **Strict tools / grammars** | [#6306](https://github.com/earendil-works/pi/issues/6306), [#6278](https://github.com/earendil-works/pi/issues/6278) | Cautious. Lenient validation won for now; strict/freeform tools remain a to-discuss item. |
| **Agent `settled` / idle signal** | [#2110](https://github.com/earendil-works/pi/issues/2110), [#5886](https://github.com/earendil-works/pi/issues/5886), [#5329](https://github.com/earendil-works/pi/issues/5329) | Acknowledged as a real design gap, partly addressed with `runWhenIdle`. |

---

## 6. Maintainer stance and trajectory

**Philosophy, in their words:**
- *"pi's core is minimal. If your feature does not belong in the core, it should be an extension. PRs that bloat the core will likely be rejected."* (CONTRIBUTING). *"Even hook points for extensions however should be well considered."*
- **No provider special cases in the LLM layer.** badlogic: *"This is still not entirely reliable, and would be special casing specific providers. I do not want to do that in pi-ai"* ([#1086](https://github.com/earendil-works/pi/issues/1086)), and *"i'd rather not special case this in pi"* ([#4251](https://github.com/earendil-works/pi/issues/4251)). In practice the `compat` flags and provider-scoped fixes keep growing anyway.
- **Retries belong to the application, not the LLM library.** *"The ai package is a low-level streaming library. Retry logic requires state management ... that fits better at the application layer"* ([#157](https://github.com/earendil-works/pi/issues/157)). badlogic is also *"not a fan of auto-retries"* in general.
- **The user triggers structural operations.** On automatic session stacking: *"I'm not a fan. I believe this must a concious operation triggered by the user"* ([#290](https://github.com/earendil-works/pi/issues/290)).
- **Closed, exhaustive types.** badlogic kept TypeScript declaration merging for app messages because *"I get exhaustive switch statements on the role field. That makes refactors a lot nicer than thoughts and prayers with a more open architecture"* ([#339](https://github.com/earendil-works/pi/issues/339)).
- **Data, repros and session files before code.** mitsuhiko: *"I don't need a PR, we need data"* ([#6278](https://github.com/earendil-works/pi/issues/6278)); badlogic: *"It can be fixed with a repro. I have no repro."* ([#5886](https://github.com/earendil-works/pi/issues/5886)). `/share` exists so users can send an exact session ([#380](https://github.com/earendil-works/pi/issues/380), [#1259](https://github.com/earendil-works/pi/issues/1259)).
- **Blunt about the ecosystem:** Anthropic *"is blocking pi on their side actively"* for subscriptions ([#5821](https://github.com/earendil-works/pi/issues/5821)), and local inference servers *"need fixing"* for idle sockets ([#5089](https://github.com/earendil-works/pi/issues/5089)).

**Trajectory.**
1. **Nov–Dec 2025:** badlogic built the core fast, one maintainer design issue at a time: compaction ([#92](https://github.com/earendil-works/pi/issues/92)), custom tools ([#190](https://github.com/earendil-works/pi/issues/190)), the `AgentSession` architecture ([#153](https://github.com/earendil-works/pi/issues/153)), the SDK ([#272](https://github.com/earendil-works/pi/issues/272)), the session tree ([#316](https://github.com/earendil-works/pi/issues/316)), and unified extensions ([#454](https://github.com/earendil-works/pi/issues/454)).
2. **Jan–Apr 2026:** explosive adoption, partly via OpenClaw, which builds on pi. Issue volume rose to about 900 a month and the contribution gate went up.
3. **April 2026:** the repo and npm scope moved to Earendil ([#4349](https://github.com/earendil-works/pi/issues/4349)).
4. **May 2026:** the "bigrefactor" with a triage freeze.
5. **Jun–Sep 2026:** a new **`AgentHarness`** in `pi-agent-core`. It has lane-based v4 `Session`/`SessionStorage` with durable operation records, required `Models` provider auth, and context-aware tools (per the agent CHANGELOG). New packages appeared: `pi-durable`, an experimental `pi-server` with a CBOR `pi-protocol`, `chord` (plugin/facet composition), and pluggable session back ends (SQLite). The coding agent has **not yet migrated** to the new harness ([D#3337](https://github.com/earendil-works/pi/discussions/3337)), so there are two copies of compaction and branch summarization ([#8845](https://github.com/earendil-works/pi/issues/8845) found the same bug in both).
6. **Now:** reconsidering MCP via "codemode" ([PR#10040](https://github.com/earendil-works/pi/pull/10040)) and aligning with Codex's turn-attribution metadata ([#9481](https://github.com/earendil-works/pi/issues/9481)).

Put simply, pi is moving from "minimal CLI with a library inside" toward "a durable agent runtime and server that the CLI is one client of." That is Dive's lane.

---

## 7. Lessons for Dive

Dive is a Go library, not a TUI, so pi's biggest clusters (TUI and Windows terminal behavior) mostly don't apply. What does apply is pi's LLM layer and agent loop, which is where its hardest bugs live. The lessons are in priority order.

### How the tracker lines up with the source analysis ([pi.md](pi.md))

- **Compat flags as model data** (pi.md §9.1.1, "best in the survey") are **confirmed, with a nuance**. The design is right, but the tracker shows the flag set growing one incident at a time: 322 Chinese-lab model issues, plus [#4251](https://github.com/earendil-works/pi/issues/4251), [#6083](https://github.com/earendil-works/pi/issues/6083) and [#9508](https://github.com/earendil-works/pi/issues/9508). The ~160 dialect regression tests exist *because of* this tracker. Dive should budget for that corpus from the start.
- **`transformMessages` hand-off** (pi.md §2.1, §9.1.2) skips error and aborted turns and synthesizes "No result provided" for orphans. That is replay-time repair. Users still hit bricked sessions on paths it doesn't cover, such as continue after abort ([#9306](https://github.com/earendil-works/pi/issues/9306)) and whitespace-only results ([#8720](https://github.com/earendil-works/pi/issues/8720)). The synthesized error result also *destroys* real results in parallel batches ([#7053](https://github.com/earendil-works/pi/issues/7053)). Hence P0 #1: enforce invariants at write time, and keep replay-time repair as the backstop. pi.md notes that the README promises `<thinking>`-tagged hand-off text while the code emits untagged text. [#6167](https://github.com/earendil-works/pi/issues/6167) is the user-visible result.
- **`agent_end` vs `agent_settled`** (pi.md §9.1.6): the harness has the split. The tracker shows it took a year of races to get there ([#2110](https://github.com/earendil-works/pi/issues/2110), [#2023](https://github.com/earendil-works/pi/issues/2023), [#5886](https://github.com/earendil-works/pi/issues/5886)).
- **Harness intent-before-effect and `outcome_ready`** (pi.md §9.1.4) are the structural fix for [#7053](https://github.com/earendil-works/pi/issues/7053)-class bugs in the shipping core loop, which uses `Promise.all`. **"Three runtimes at once"** (pi.md §9.2.1) is visible in the tracker as duplicated bugs ([#8845](https://github.com/earendil-works/pi/issues/8845)) and SDK users unsure which API is stable ([D#3337](https://github.com/earendil-works/pi/discussions/3337)).
- **Stringly-typed errors** (pi.md §9.2.3), where retry and overflow are regexes over messages, match the misclassification bugs: real exhaustion treated as recoverable ([#9409](https://github.com/earendil-works/pi/issues/9409)), budget overflow at 78% ([#8061](https://github.com/earendil-works/pi/issues/8061)), and users asking for provider-native failure classes over RPC ([#9247](https://github.com/earendil-works/pi/issues/9247)).
- **Steering queues reconciled by text match** (pi.md §9.2.7) match the RPC correlation regression ([#9803](https://github.com/earendil-works/pi/issues/9803)), `clearQueue()` destroying extension messages ([#9886](https://github.com/earendil-works/pi/issues/9886)), and follow-ups stranded on abort ([#10017](https://github.com/earendil-works/pi/issues/10017)). This supports pi.md rec. #14: inbox items with IDs.
- **Transcript-carried tool changes and cache** (pi.md §9.1.3) come with a nuance. On Anthropic, `defer_loading` keeps the cache warm. [#10024](https://github.com/earendil-works/pi/issues/10024) reports that a mid-run tool-set change still moves the prompt head and re-bills on other paths. Dive should test cache stability per provider, not assume it.
- **No HITL suspend primitive** (pi.md §9.2.9) matches the unmet requests for a "waiting on user" state and a structured approval primitive ([#5329](https://github.com/earendil-works/pi/issues/5329), [#5954](https://github.com/earendil-works/pi/issues/5954)).
- **Refusing `length`-truncated tool calls** (pi.md §9.3.5) was itself learned from the tracker (agent CHANGELOG, [PR#6285](https://github.com/earendil-works/pi/pull/6285)). So was treating a `length`-stopped *summary* as a failure ([#7048](https://github.com/earendil-works/pi/issues/7048)).

### P0: get these right in the core design

1. **Treat the transcript as a validated, provider-neutral log with typed invariants, and repair it before every request.** Pi's most damaging bugs leave a session *permanently* broken: orphaned tool calls after an abort, whitespace-only tool results, mid-batch injections, results persisted before their assistant message ([#9306](https://github.com/earendil-works/pi/issues/9306), [#8720](https://github.com/earendil-works/pi/issues/8720), [#8166](https://github.com/earendil-works/pi/issues/8166), [#1717](https://github.com/earendil-works/pi/issues/1717)). Dive should:
   - enforce "every tool call has exactly one result" *at write time*;
   - synthesize explicit "aborted"/"not executed" results on cancel;
   - persist each tool result as soon as it completes, not per batch ([#7053](https://github.com/earendil-works/pi/issues/7053));
   - never let a provider adapter emit a malformed message, following mitsuhiko's fix direction in [#6259](https://github.com/earendil-works/pi/issues/6259).
2. **Carry provider-native reasoning state opaquely, and treat replay fidelity as a first-class feature.** Store signed thinking, `reasoning_content`, `thoughtSignature` and encrypted reasoning verbatim, tagged with the provider and model that produced them. Rules:
   - Replay them unchanged to the same provider family, and never "sanitize" the latest turn ([#5223](https://github.com/earendil-works/pi/issues/5223)).
   - Degrade them deliberately on hand-off ([#6167](https://github.com/earendil-works/pi/issues/6167), [#5148](https://github.com/earendil-works/pi/issues/5148)).
   - Normalize tool-call IDs per provider at the boundary ([#198](https://github.com/earendil-works/pi/issues/198)).
   - Account for the memory cost of reasoning, and let callers choose retention ([#8746](https://github.com/earendil-works/pi/issues/8746)).

   Build pi's **all-pairs hand-off test matrix** ([#258](https://github.com/earendil-works/pi/issues/258)) as recorded fixtures, so it runs in CI without API keys.
3. **Make "OpenAI-compatible" a set of explicit capability flags on the model or endpoint.** Do not use one adapter with scattered special cases. Pi's `compat` object is the right idea, but it grew in response to incidents ([#9508](https://github.com/earendil-works/pi/issues/9508), [#4251](https://github.com/earendil-works/pi/issues/4251), [#6083](https://github.com/earendil-works/pi/issues/6083)). Dive should declare the flags up front and document each one next to the error message it prevents ([D#3735](https://github.com/earendil-works/pi/discussions/3735)). Parse Chat Completions deltas as **independent streams** (content, reasoning and tool calls), not as one current block ([#4228](https://github.com/earendil-works/pi/issues/4228)). Accumulate tool arguments in linear time ([#9062](https://github.com/earendil-works/pi/issues/9062)).
4. **Separate "run ended" from "session settled" in the event model.** Give hosts an explicit settled/idle state plus a "waiting on human" state ([#5886](https://github.com/earendil-works/pi/issues/5886), [#2110](https://github.com/earendil-works/pi/issues/2110), [#5329](https://github.com/earendil-works/pi/issues/5329)). Post-run work (retry, compaction, queued follow-ups, hook-enqueued turns) should be modeled as loop states, not bolted on after `agent_end`. In Go this maps naturally onto a run handle with `Wait()`, plus a status that is either an enum or a channel.
5. **Every model call goes through one pipeline: turns, compaction, summaries and subagents alike.** This means the same headers, session ID, hooks, retry policy, telemetry and budget checks ([#6768](https://github.com/earendil-works/pi/issues/6768), [#6477](https://github.com/earendil-works/pi/issues/6477), [#9773](https://github.com/earendil-works/pi/issues/9773), [#10022](https://github.com/earendil-works/pi/issues/10022)). Likewise, use **one session-to-context projection function** for normal context, compaction and branch summary ([#6451](https://github.com/earendil-works/pi/issues/6451)).
6. **Check compaction at every safe point in the loop, not only at turn end.** Account for the output reservation and for reasoning tokens ([#6879](https://github.com/earendil-works/pi/issues/6879), [#8061](https://github.com/earendil-works/pi/issues/8061), [#9409](https://github.com/earendil-works/pi/issues/9409)). Let callers set a separate compaction model, thinking level and output budget ([#7553](https://github.com/earendil-works/pi/issues/7553), [#8845](https://github.com/earendil-works/pi/issues/8845)). Treat a `length` stop on a summary as a failure, not a summary ([#7048](https://github.com/earendil-works/pi/issues/7048)). Pi's "stop hook at safepoints" idea ([#1884](https://github.com/earendil-works/pi/issues/1884)) is the right general mechanism: one `ShouldStop`/`BeforeNextStep` hook that covers structured-output stop tools, budgets and compaction.
7. **Deadlines everywhere, with an idle-stream watchdog separate from the total deadline.** Local models legitimately take minutes to prefill ([#8331](https://github.com/earendil-works/pi/issues/8331)). Cloud streams stall silently ([#4945](https://github.com/earendil-works/pi/issues/4945)). Retry-After headers can say "days" ([#6911](https://github.com/earendil-works/pi/issues/6911)). In Go, `context.Context` gives Dive this almost for free. Make per-event idle timeout, per-request deadline and retry budget three separate, configurable settings, and make cancellation always win.

### P1: what makes users love it

8. **Keep the core small, but make every built-in replaceable.** Pi's love comes from "I replaced the bash tool in a minute." Dive should ship built-in tools as ordinary tools built with the same public API, and support pluggable **operations** (filesystem and exec back ends) so hosts can target a VM or remote host ([#564](https://github.com/earendil-works/pi/issues/564)). Don't resolve paths with host-OS semantics inside those tools ([#5350](https://github.com/earendil-works/pi/issues/5350)).
9. **Lenient tool-argument handling by default, with the policy host-controlled.** Ignore unknown keys, coerce JSON-encoded strings where the schema is unambiguous, and use tolerant fuzzy matching in edit tools ([#6278](https://github.com/earendil-works/pi/issues/6278), [#5501](https://github.com/earendil-works/pi/issues/5501), [#7836](https://github.com/earendil-works/pi/issues/7836), [#5697](https://github.com/earendil-works/pi/issues/5697)). Allow per-tool `strict` (provider-lowered, best effort) and per-tool "passthrough" validation so `execute` can return correction feedback ([#6306](https://github.com/earendil-works/pi/issues/6306), [#7607](https://github.com/earendil-works/pi/issues/7607)). Never *silently* skip validation, and never silently drop schema keywords ([#3112](https://github.com/earendil-works/pi/issues/3112), [#9134](https://github.com/earendil-works/pi/issues/9134)).
10. **The session is a tree in an append-only log.** `id`/`parentId` entries with branch summaries and compaction markers ([#316](https://github.com/earendil-works/pi/issues/316)) is a proven format, and users build real workflows on it ([D#5205](https://github.com/earendil-works/pi/discussions/5205)). Add what pi lacks: storage back ends behind an interface (pi only now has SQLite), an explicit flush and eager-create option ([#9792](https://github.com/earendil-works/pi/issues/9792)), collision-free session keys ([#4877](https://github.com/earendil-works/pi/issues/4877)), and an optional file-checkpoint hook so "rewind" can be done correctly outside core ([D#1223](https://github.com/earendil-works/pi/discussions/1223)).
11. **A stable prompt prefix and cache-aware defaults.** Put nothing volatile in the system prompt, such as dates or tool sets that change mid-run ([#6621](https://github.com/earendil-works/pi/issues/6621), [#10024](https://github.com/earendil-works/pi/issues/10024)). Normalize usage and cost across providers so the numbers are correct ([#8752](https://github.com/earendil-works/pi/issues/8752), [#9980](https://github.com/earendil-works/pi/issues/9980), [#9457](https://github.com/earendil-works/pi/issues/9457)). Keep the default prompt and tool set small, because users measure it ([D#6646](https://github.com/earendil-works/pi/discussions/6646)).
12. **Model metadata:** thinking levels should be **per-model capability data** (the supported efforts and how each maps to the wire), not a global ladder ([#3208](https://github.com/earendil-works/pi/issues/3208), [#6097](https://github.com/earendil-works/pi/issues/6097)). Let runtime-discovered models (Ollama, llama.cpp, vLLM `/models`) register with explicit placeholders for fields they can't discover ([#3357](https://github.com/earendil-works/pi/issues/3357), [#6922](https://github.com/earendil-works/pi/issues/6922), [#9566](https://github.com/earendil-works/pi/issues/9566)). Pi refused this in core. For a library it is table stakes. Also support provider-specific stop semantics such as Codex `end_turn:false` ([#7689](https://github.com/earendil-works/pi/issues/7689)).

### P2: library hygiene and protocols

13. **Design for many sessions per process from day one.** No global state, including HTTP clients. Pi's SDK hijacks the global fetch dispatcher ([#9787](https://github.com/earendil-works/pi/issues/9787)). Credential stores must be safe across concurrent processes ([#8928](https://github.com/earendil-works/pi/issues/8928)). Hooks and extensions are scoped to a session, not a process ([D#1546](https://github.com/earendil-works/pi/discussions/1546)). The runtime has to be shareable so child agents inherit providers and auth ([#8791](https://github.com/earendil-works/pi/issues/8791)). Pi is rebuilding for this ("pi server", [#5700](https://github.com/earendil-works/pi/issues/5700)). Go's concurrency model makes it the natural default for Dive.
14. **Keep the library and the app on separate release and dependency tracks.** Pi's shrinkwrap, org-rename and bundling problems ([#5653](https://github.com/earendil-works/pi/issues/5653), [#1820](https://github.com/earendil-works/pi/issues/1820), [#9132](https://github.com/earendil-works/pi/issues/9132)) all come from one package serving as both. Go modules help, but keep provider SDKs in separate modules so importing Dive doesn't pull in every vendor.
15. **Protocols:** ship a first-party, documented wire protocol for hosts. Pi's RPC mode is used heavily and regresses ([#9803](https://github.com/earendil-works/pi/issues/9803), [#4897](https://github.com/earendil-works/pi/issues/4897)). Treat **ACP and MCP as adapters on top of it**, not as the core. Demand for ACP is real ([D#4444](https://github.com/earendil-works/pi/discussions/4444)), and third-party bridges break on session lifecycle. Pi's "codemode" direction for MCP ([PR#10040](https://github.com/earendil-works/pi/pull/10040)), which projects many tools into one sandboxed code tool, is worth tracking as the answer to *"exposing MCP tools statically, always in context ... is not good"* ([#67](https://github.com/earendil-works/pi/issues/67)).
16. **Structured output:** ship both. Offer a "final answer tool" plus a stop hook as the portable default, as badlogic recommends ([#1086](https://github.com/earendil-works/pi/issues/1086)), and native `response_format` where the provider supports it, behind a capability flag. Users want this ([#1086](https://github.com/earendil-works/pi/issues/1086), [#1884](https://github.com/earendil-works/pi/issues/1884)), and a library can't refuse it the way an opinionated CLI can.
17. **Security:** stay unopinionated but hookable. A `BeforeToolCall` policy hook with a structured approve/deny/ask result ([#5954](https://github.com/earendil-works/pi/issues/5954), [D#3337](https://github.com/earendil-works/pi/discussions/3337) "policy-decision-point interface") covers most asks without Dive shipping a sandbox.

### Process lessons
- Pi's growth outran its docs. The SDK surface needs docs as good as the CLI's ([D#3337](https://github.com/earendil-works/pi/discussions/3337), [D#3735](https://github.com/earendil-works/pi/discussions/3735)).
- The auto-close gate kept the maintainers sane, but it produced duplicates and resentment. If Dive grows, use structured templates and labels, not silent closure.
- Pi's best debugging tool is a shareable exact session (`/share`). Dive should make it trivial to export a session and replay it against a live or recorded provider. mitsuhiko's 500-run resampling experiments in [#6278](https://github.com/earendil-works/pi/issues/6278) show what that makes possible.

---

## 8. Appendix: high-signal issue index

R = reactions, C = comments (issues and PRs); discussions show upvotes/comments. All links are to `earendil-works/pi`; the pre-transfer `badlogic/pi-mono` numbers are identical.

| # | Title | State | R | C | Theme | Takeaway |
|---|---|---|---|---|---|---|
| [#2870](https://github.com/earendil-works/pi/issues/2870) | Follow XDG Base Directory | closed | 80 | 22 | Config | Most-reacted issue, and refused ("things will stay as is"). On-disk layout can't be fixed later. |
| [#534](https://github.com/earendil-works/pi/issues/534) | config folder is out of place on Linux | closed | 50 | 16 | Config | An earlier duplicate of the same demand. |
| [#3357](https://github.com/earendil-works/pi/issues/3357) | Official local LLM provider extension | closed | 47 | 30 | Providers | Runtime model discovery was refused in core and pushed to extensions. |
| [#4945](https://github.com/earendil-works/pi/issues/4945) | openai-codex Connection Reliability Issues | open | 34 | 80 | Streaming | Silent "Working..." hangs with zero usage. Still unexplained. |
| [#7547](https://github.com/earendil-works/pi/issues/7547) | [Windows] How do you use Pi on windows? | open | 3 | 68 | Platform | Central path translation is "a losing game". |
| [#5825](https://github.com/earendil-works/pi/issues/5825) | Streaming markdown forces scroll to bottom | closed | 0 | 42 | TUI | Rendering streamed output is its own product. |
| [#6879](https://github.com/earendil-works/pi/issues/6879) | auto-compaction never triggers past 100% | closed | 21 | 24 | Compaction | Compaction was checked only at turn end, so long tool loops overflow. |
| [#6768](https://github.com/earendil-works/pi/issues/6768) | Compaction using Copilot Enterprise not possible | closed | 22 | 19 | Compaction | The summary request path lacked the main path's headers. |
| [#6477](https://github.com/earendil-works/pi/issues/6477) | Compaction summary requests omit the session ID | closed | 11 | 8 | Compaction | Same root cause: a second request pipeline. |
| [#7553](https://github.com/earendil-works/pi/issues/7553) | Configurable thinking level/model for compaction | open | 5 | 9 | Compaction | Accepted: compaction needs its own model and effort. |
| [#8845](https://github.com/earendil-works/pi/issues/8845) | Branch summarization hardcodes maxTokens: 2048 | closed | 0 | 14 | Compaction | Hardcoded caps. The bug is duplicated in both harness copies. |
| [#9409](https://github.com/earendil-works/pi/issues/9409) | Sessions wedge permanently at the context ceiling | open | 0 | 2 | Compaction | Token estimates can't see reasoning carried in signatures. |
| [#8061](https://github.com/earendil-works/pi/issues/8061) | Context budget ignores maxTokens reservation | open | 2 | 9 | Compaction | Budget = input + output reservation. |
| [#6451](https://github.com/earendil-works/pi/issues/6451) | Clean up new harness session projection and compaction | open | 0 | 3 | Sessions (maint.) | One projection function as the single source of truth. |
| [#128](https://github.com/earendil-works/pi/issues/128) | Fix under-compaction | closed | 1 | 14 | Compaction (maint.) | Early taxonomy of overflow scenarios. |
| [#316](https://github.com/earendil-works/pi/issues/316) | Session tree format | closed | 2 | 5 | Sessions (maint.) | Append-only JSONL with id/parentId, and migrations on load. |
| [#290](https://github.com/earendil-works/pi/issues/290) | Session stacking | closed | 5 | 9 | Sessions (maint.) | Structural ops must be user-triggered, and replayed via marker events. |
| [D#5205](https://github.com/earendil-works/pi/discussions/5205) | What is the point of the tree functionality? | disc. | 1 | 5 | Sessions | Users explain real branching workflows. |
| [#5263](https://github.com/earendil-works/pi/issues/5263) | Make in-session model changes ephemeral | closed | 18 | 11 | Config | Runtime changes must not overwrite persistent defaults. |
| [#4251](https://github.com/earendil-works/pi/issues/4251) | Kimi k2.6: reasoning_content is missing | closed | 18 | 23 | Reasoning replay | Gateways mix field names, and "no special cases" gave way. |
| [#5223](https://github.com/earendil-works/pi/issues/5223) | Anthropic provider modifies thinking blocks | closed | 6 | 17 | Reasoning replay | The latest turn's signed thinking must be byte-exact. |
| [#5148](https://github.com/earendil-works/pi/issues/5148) | GPT 5.5 after Opus extended thinking returns 400 | closed | 6 | 4 | Hand-off | Duplicate item IDs across providers. |
| [#6167](https://github.com/earendil-works/pi/issues/6167) | transformMessages thinking normalization vs compat flag | open | 0 | 5 | Hand-off | Hand-off degradation conflicts with per-provider requirements. |
| [#9444](https://github.com/earendil-works/pi/issues/9444) | openai-completions drops Gemini thoughtSignature | open | 0 | 3 | Reasoning replay | Opaque reasoning state is lost behind gateways. |
| [#8746](https://github.com/earendil-works/pi/issues/8746) | 0.84.3 keeps reasoning in every message, OOM 20GB+ | closed | 0 | 4 | Reasoning replay | Retaining reasoning has a real memory cost. |
| [#198](https://github.com/earendil-works/pi/issues/198) | Copilot: handoff fails due to tool call ID incompatibilities | closed | 0 | 2 | Hand-off (maint.) | Normalize tool-call IDs at the provider boundary. |
| [#258](https://github.com/earendil-works/pi/issues/258) | Comprehensive hand-off test | closed | 3 | 0 | Hand-off (maint.) | All-pairs live provider test matrix. |
| [#4228](https://github.com/earendil-works/pi/issues/4228) | openai-completions deltas with content and tool calls | closed | 0 | 19 | Streaming | Delta fields are independent streams with no ordering. |
| [#9508](https://github.com/earendil-works/pi/issues/9508) | pi-ai sends OpenAI-specific fields to compatible providers | open | 0 | 6 | Providers | "OpenAI-compatible" is a spectrum. |
| [#8720](https://github.com/earendil-works/pi/issues/8720) | whitespace-only tool result bricks the session | open | 0 | 6 | Transcript | One bad message poisons every later request. |
| [#9134](https://github.com/earendil-works/pi/issues/9134) | Anthropic adapter drops root anyOf from tool schemas | open | 0 | 3 | Tools | Schema lowering silently loses constraints. |
| [#6278](https://github.com/earendil-works/pi/issues/6278) | New Claude models fail ~20% of edits (extra keys) | closed | 11 | 25 | Tools | Ignore extra keys rather than decode strictly. Decided with experiments. |
| [#6306](https://github.com/earendil-works/pi/issues/6306) | Support Strict Tools / Grammar | closed | 0 | 22 | Tools (maint.) | Strict/grammar features are "very leaky abstractions". |
| [#7607](https://github.com/earendil-works/pi/issues/7607) | per-tool opt-out of argument validation | open | 0 | 4 | Tools / SDK | Hosts want to own validation and correction feedback. |
| [#3112](https://github.com/earendil-works/pi/issues/3112) | Tool arguments not validated in Cloudflare Workers | closed | 1 | 13 | Tools / SDK | Validation failed open silently in one runtime. |
| [#5886](https://github.com/earendil-works/pi/issues/5886) | AgentSession settlement/continuation lifecycle bugs | open | 4 | 12 | Agent loop (maint.) | `agent_end` is not settlement. |
| [#2110](https://github.com/earendil-works/pi/issues/2110) | Support agent_settled event | closed | 0 | 4 | Agent loop | Spiked, then dropped: "more complex than it seems". |
| [#1884](https://github.com/earendil-works/pi/issues/1884) | Add stop hook to agent-core | closed | 0 | 8 | Agent loop | A general safepoint stop hook covers structured output and budgets. |
| [#9306](https://github.com/earendil-works/pi/issues/9306) | Aborted turn leaves unmatched toolCall blocks | open | 0 | 5 | Transcript | Synthesize results on abort. |
| [#7053](https://github.com/earendil-works/pi/issues/7053) | Parallel tool batches lose completed results | open | 0 | 4 | Transcript | Persist per tool, not per batch. |
| [#8166](https://github.com/earendil-works/pi/issues/8166) | custom message mid-tool-batch breaks adjacency | closed | 0 | 11 | Transcript | Injections must respect tool-call pairing. |
| [#8331](https://github.com/earendil-works/pi/issues/8331) | Agent loop hangs forever when stream stalls | open | 2 | 6 | Streaming | An idle watchdog must allow for slow local prefill. |
| [#3715](https://github.com/earendil-works/pi/issues/3715) | local-llm streams terminate at 5 min (undici bodyTimeout) | closed | 5 | 12 | Streaming | Hidden HTTP-client defaults kill long streams. |
| [#6911](https://github.com/earendil-works/pi/issues/6911) | OpenAI SDK retries sleep full Retry-After (days) | closed | 0 | 5 | Streaming | Cap retry waits, and make abort always win. |
| [#157](https://github.com/earendil-works/pi/issues/157) | Auto-retry on provider error | closed | 1 | 4 | Agent loop (maint.) | Retry belongs in the app layer, not the LLM library. |
| [#7689](https://github.com/earendil-works/pi/issues/7689) | Handle end_turn: false for codex | closed | 5 | 4 | Providers (maint.) | Provider-specific "continue" signals. |
| [#3208](https://github.com/earendil-works/pi/issues/3208) | Custom Thinking Levels per Model | closed | 15 | 14 | Model metadata | Effort levels are per-model data. |
| [#6097](https://github.com/earendil-works/pi/issues/6097) | Add support for 'max' thinking level | closed | 22 | 4 | Model metadata | A fixed global ladder breaks with each launch. |
| [#6083](https://github.com/earendil-works/pi/issues/6083) | Cache not working with z.ai GLM coding plan | closed | 11 | 8 | Caching | One missing provider flag caused 5–10× quota burn. |
| [#6621](https://github.com/earendil-works/pi/issues/6621) | Cache invalidation due to dynamic system prompt | closed | 1 | 6 | Caching | Nothing volatile in the prefix. |
| [#7128](https://github.com/earendil-works/pi/issues/7128) | New PI_* guideline over-encourages bash calls | closed | 14 | 12 | Prompt | Users guard the minimal prompt. |
| [#326](https://github.com/earendil-works/pi/issues/326) | Unified extension loading system | closed | 6 | 14 | Extensibility (maint.) | Packages from npm or git with atomic install. A hard fork happened over limits. |
| [#454](https://github.com/earendil-works/pi/issues/454) | Merge hooks and custom tools into unified extensions | closed | 3 | 3 | Extensibility (maint.) | One concept: tools, events and commands share closure state. |
| [#564](https://github.com/earendil-works/pi/issues/564) | Pluggable operations for built-in tools | closed | 0 | 3 | Extensibility (maint.) | Swappable file and exec back ends for remote or VM execution. |
| [#272](https://github.com/earendil-works/pi/issues/272) | coding-agent: Agent SDK equivalent | closed | 0 | 10 | SDK | Defaults match the CLI, and every part can be replaced. |
| [D#3337](https://github.com/earendil-works/pi/discussions/3337) | Using pi-agent-core as runtime for a scheduled-agent platform | disc. | 6 | 1 | SDK | Chosen over LangGraph because it is "just the agent turn". Docs are lacking. |
| [D#1546](https://github.com/earendil-works/pi/discussions/1546) | Session-aware extension lifecycle for multi-session processes | disc. | 5 | 1 | SDK | One process = one session is a hidden assumption. |
| [#5700](https://github.com/earendil-works/pi/issues/5700) | Multiple live agent sessions | closed | 0 | 10 | Multi-session | Deferred to "pi server". The architecture can't support it. |
| [#8791](https://github.com/earendil-works/pi/issues/8791) | Expose the model runtime to extensions | open | 5 | 4 | SDK | Child agents need to share providers and auth. |
| [#5350](https://github.com/earendil-works/pi/issues/5350) | SDK: custom tool operations get host-OS paths | open | 0 | 7 | SDK | Remote back ends need target-OS path semantics. |
| [#9787](https://github.com/earendil-works/pi/issues/9787) | Importing the SDK installs a global undici dispatcher | open | 0 | 2 | SDK | A library must not mutate process-global state. |
| [#9792](https://github.com/earendil-works/pi/issues/9792) | SessionManager.create() doesn't write until first reply | open | 0 | 2 | Persistence | Offer eager create and an explicit flush. |
| [#8928](https://github.com/earendil-works/pi/issues/8928) | Parallel startup: "No API key found" for ~48s | open | 0 | 11 | Auth | Credential stores need multi-process locking. |
| [#5329](https://github.com/earendil-works/pi/issues/5329) | Expose when Pi is waiting on user input | closed | 9 | 3 | Host integration | Hosts need a "blocked on human" state. |
| [#1086](https://github.com/earendil-works/pi/issues/1086) | Add structured output (JSON schema) support | closed | 0 | 5 | Features | Refused. Use a final-answer tool instead. |
| [#1837](https://github.com/earendil-works/pi/issues/1837) | Add temperature, top_p, other params | closed | 8 | 3 | Features | "build it as an extension". |
| [#552](https://github.com/earendil-works/pi/issues/552) | RFC: extract subagent execution into library | closed | 1 | 4 | Multi-agent | Subagents kept out of core. |
| [#67](https://github.com/earendil-works/pi/issues/67) | Add MCP support | closed | 1 | 4 | Protocols (maint.) | Static MCP tools in context are "not good". |
| [#563](https://github.com/earendil-works/pi/issues/563) | Add MCP extension example | closed | 5 | 7 | Protocols | Left to the community `pi-mcp-adapter`. |
| [PR#10040](https://github.com/earendil-works/pi/pull/10040) | Codemode and MCP | open | 48 | 3 | Protocols (maint.) | The MCP stance is softening, via codemode. |
| [D#4444](https://github.com/earendil-works/pi/discussions/4444) | Supporting the Agent Client Protocol (ACP) | disc. | 40 | 4 | Protocols | Top discussion. Community bridges break on lifecycle. |
| [#175](https://github.com/earendil-works/pi/issues/175) | ACP Support | closed | 0 | 4 | Protocols | "no need for ACP ... build an adapter on RPC". |
| [#4174](https://github.com/earendil-works/pi/issues/4174) | Add a Python SDK | closed | 8 | 4 | SDK | Out of scope. |
| [#5514](https://github.com/earendil-works/pi/issues/5514) | Project Trust Feature Feedback | closed | 13 | 26 | Security | A safety default versus power-user annoyance. |
| [D#6253](https://github.com/earendil-works/pi/discussions/6253) | We need a working, functional sandbox feature | disc. | 5 | 4 | Security | The extension ecosystem failed to deliver sandboxing. |
| [#4349](https://github.com/earendil-works/pi/issues/4349) | Organization change explanation | closed | 0 | 3 | Governance | Moved to Earendil. The npm scope alias will eventually go away. |
| [#5653](https://github.com/earendil-works/pi/issues/5653) | Move off Shrinkwrap | open | 0 | 21 | Packaging | CLI and library packaging needs conflict. |
| [#1820](https://github.com/earendil-works/pi/issues/1820) | Cannot find module '@mariozechner/pi-tui' after update | closed | 12 | 5 | Packaging | Package splits broke extensions. |
| [#5084](https://github.com/earendil-works/pi/issues/5084) | Allow/disallow built-in tools in settings.json | closed | 12 | 3 | Governance | "Auto-closing ... encourag[es] duplicate filings". |
| [D#4285](https://github.com/earendil-works/pi/discussions/4285) | Isn't the org change breaking every extension? | disc. | 3 | 4 | Governance | Resentment at "closed because weekend". |
| [D#1632](https://github.com/earendil-works/pi/discussions/1632) | Love Pi for its clean context usage | disc. | 8 | 7 | Love | Usable on 32k local windows. "ask pi" to write the tool. |
| [D#6646](https://github.com/earendil-works/pi/discussions/6646) | Benchmark: Pi vs OpenCode vs Codex token overhead | disc. | 3 | 2 | Love | 81% less input than OpenCode on an MCP task. |
| [D#3373](https://github.com/earendil-works/pi/discussions/3373) | Which extensions do you most enjoy? | disc. | 9 | 19 | Love | "I can build my own extensions within a minute or two". |
| [D#3735](https://github.com/earendil-works/pi/discussions/3735) | Some negative feedback as first-time user | disc. | 10 | 4 | Docs | Local-model setup errors are undocumented. |
| [#4609](https://github.com/earendil-works/pi/issues/4609) | Rewrite pi in Rust | closed | 21 | 12 | (joke) | Maintainer joke. Ignored as signal. |
