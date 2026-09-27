# ADK for Go: what the issue tracker says (for Dive)

- **Repo:** [google/adk-go](https://github.com/google/adk-go) (module `google.golang.org/adk/v2`)
- **Snapshot:** 2026-09-26
- **Companion doc:** [`adk-go.md`](adk-go.md), a source-level analysis at commit `12f7cab`. This doc covers what users *say*. Where an issue confirms or contradicts a finding in the source analysis, that is noted inline as **[src: confirms]** or **[src: nuance]**.

Links: issues are `https://github.com/google/adk-go/issues/N`, PRs are `.../pull/N`, and discussions are `.../discussions/N`.

---

## 1. Method

### Repo stats

| Metric | Value |
|---|---|
| Stars / forks | ~8.8k / ~1.0k |
| Issues (all time) | 334 (112 open, 222 closed) |
| Issue date range | 2025-08-01 to 2026-09-25 |
| Pull requests | 1,272 (832 merged, 316 closed unmerged, 124 open) |
| Discussions | 29 |
| Median time to close an issue | 11 days |
| Open issues older than 180 days | 38 |
| Top labels | `bug` 177, `enhancement` 83, `needs review` 32, `api` 13, `breaking-change` 6, `wontfix` 1 |

Issue volume came in two bursts. The first, around the v0.2 launch (Nov 2025, 53 issues), was mostly early adopters. The second (Jul–Sep 2026, 134 issues) followed v2.0 (2026-06-30). The recent burst is dominated by a small group of contributors (`jjsasha63`, `ktsoator`, `wolo-lab`, `will-arxed`, `chill-czar`, `dmora`) who file very detailed bug reports, often with line references and failing tests. Roughly 20 issues are spam or empty (for example [#1419](https://github.com/google/adk-go/issues/1419)–[#1423](https://github.com/google/adk-go/issues/1423)), and I ignored them.

### How I found the signal

1. **Pulled every issue and PR** with `gh issue list` / `gh pr list --json ...reactionGroups,comments` and ranked by `2 × reactions + comments`.
2. **Reactions are low in this repo.** The most-reacted *issue* has 15 reactions. The strongest demand signal is on *PRs*: the OpenAI provider PR [#242](https://github.com/google/adk-go/pull/242) has 50 reactions and 21 comments, and the OpenAI-compatible PR [#342](https://github.com/google/adk-go/pull/342) has 34 reactions and 27 comments. Both are larger than any issue. Community forks and third-party modules named in threads are a second demand signal.
3. **Clusters of small issues.** Many themes show up as 5–15 low-reaction bug reports rather than one popular issue. Examples: concurrency races, HITL confirmation bugs, and state deltas dropped during parallel tool calls. Three separate reports of the same empty-ID confirmation bug ([#558](https://github.com/google/adk-go/issues/558), [#587](https://github.com/google/adk-go/issues/587), [#590](https://github.com/google/adk-go/issues/590)) point to a real pain point.
4. **Duplicates and cross-links.** Provider-support requests were filed at least seven times ([#225](https://github.com/google/adk-go/issues/225), [#289](https://github.com/google/adk-go/issues/289), [#295](https://github.com/google/adk-go/issues/295), [#320](https://github.com/google/adk-go/issues/320), [#341](https://github.com/google/adk-go/issues/341), [#596](https://github.com/google/adk-go/issues/596), [#1097](https://github.com/google/adk-go/issues/1097)), plus four provider PRs.
5. **Maintainer comments that reveal a stance.** I searched threads for "parity", "adk-python", "community repo", "intentionally", and "source of truth", and read every comment by maintainers (`dpasiukevich`, `rakyll`, `karolpiotrowicz`, `wolo-lab`, `baptmont`, `mazas-google`, `hyangah`).
6. **Discussions** via `gh api graphql`, especially the roadmap and philosophy threads ([#364](https://github.com/google/adk-go/discussions/364), [#691](https://github.com/google/adk-go/discussions/691), [#1112](https://github.com/google/adk-go/discussions/1112), [#589](https://github.com/google/adk-go/discussions/589), [#1078](https://github.com/google/adk-go/discussions/1078)).
7. **adk-python context.** For themes that come from the wider ADK project, I checked the most-reacted adk-python issues (`gh search issues --repo google/adk-python --sort reactions`).

---

## 2. What users love

Explicit praise is rare in the tracker; issue trackers are where people complain. The positive signal is mostly **revealed preference**: people keep building on adk-go even though its model support is missing, and they write adapters and forks instead of leaving.

1. **"Less magic", building blocks over batteries.** In the philosophy discussion a heavy user wrote: "What I'm looking for in an agentic framework, ADK is pretty close to. I think of Kubernetes, great abstractions, does all the management work, easy to plug into" ([#364](https://github.com/google/adk-go/discussions/364)). The maintainer confirmed that this is the goal: "adk-go less magical. This is the main goal. The public API should be simple and straightforward to use." Users push back on Python features they see as magic (Planner, Memory as a core concept, a built-in code executor), saying they can be built from tools and sub-agents ([#364](https://github.com/google/adk-go/discussions/364), [#277](https://github.com/google/adk-go/discussions/277)).
2. **A small `model.LLM` interface that outsiders can implement.** Because `LLM` has two methods, the community filled the provider gap itself:
   - `adk-anthropic-go` ("we're using this in prod", [#225](https://github.com/google/adk-go/issues/225))
   - `adk-models-go`, which covers OpenAI Responses, Anthropic Messages and Vercel AI Gateway ([#1097](https://github.com/google/adk-go/issues/1097))
   - `adk-utils-go` and `adk-go-openai` ([#242](https://github.com/google/adk-go/pull/242), [#341](https://github.com/google/adk-go/issues/341))
   - adapters over charmbracelet/fantasy ([#225](https://github.com/google/adk-go/issues/225)).

   **[src: confirms]** the source doc calls the interface "small and well chosen".
3. **Storage behind interfaces.** When users objected to GORM, the reply was that core only depends on `session.Service`, and that "you wouldn't need to even download the gorm/sql dependency if you don't import from session/database" ([#236](https://github.com/google/adk-go/issues/236)). The maintainers' rule of in-memory + self-host + free SaaS for each service was well received ([#272](https://github.com/google/adk-go/issues/272)).
4. **Sessions shared across languages, and A2A.** Users value that a Python agent and a Go agent can share one session store. The maintainer said this is an area where "we must have 100% parity" ([#364](https://github.com/google/adk-go/discussions/364)). In practice, [#1251](https://github.com/google/adk-go/issues/1251) found that the Go `Event` JSON encoding silently corrupted events exchanged with Python.
5. **Production users exist and stay.** Examples: the kagent Go runtime ([#1001](https://github.com/google/adk-go/issues/1001)), Alcova ([#225](https://github.com/google/adk-go/issues/225)), a company fork at way-platform ([#757](https://github.com/google/adk-go/issues/757)), and a coding agent built on ADK-Go ([#389](https://github.com/google/adk-go/discussions/389), [#343](https://github.com/google/adk-go/issues/343)). Users who need durable execution asked for, and got, clock, UUID and task-runner seams ([#963](https://github.com/google/adk-go/issues/963), [#1051](https://github.com/google/adk-go/issues/1051)). **[src: confirms]** "Platform seams" is item 4 of the source doc's "excellent" list.
6. **Better maintainer engagement in 2026.** Later closures are detailed and verified. Examples: the Gemini 3 streaming fix in [#782](https://github.com/google/adk-go/issues/782), and [#542](https://github.com/google/adk-go/issues/542), where maintainers reproduced a 429 with a fake model rather than asking the reporter to spend money on a live repro. The "ADK 2.0" discussion was the most-upvoted post in Discussions ([#691](https://github.com/google/adk-go/discussions/691), 14 upvotes).
7. **The `iter.Seq2` surface draws no complaints.** Nobody objects to the iterator-based run surface. Users wrap agents with small iterator adapters without difficulty (for example `escalateClearingAgent` in [#522](https://github.com/google/adk-go/issues/522)). This is weak evidence, but it is consistent with the source doc's view that `iter.Seq2` end to end is the best decision in the codebase.

---

## 3. What users hate / friction

### 3.1 Vendor types in the public API

The strongest complaint is structural: `genai` types are the lingua franca.

- "The `llm.Model` interface is (IMO) overly specific for Gemini and makes it very difficult to use other LLMs … it's not possible to implement a different provider without using the structs from `genai`" ([#225](https://github.com/google/adk-go/issues/225)).
- A user asked whether `model.LLMResponse` would be decoupled. The maintainer said no: "`model.LLMResponse` is coupled to `google/genai` intentionally and we don't plan to change this. We're keeping consistency with other ADK implementations" ([#225](https://github.com/google/adk-go/issues/225)).
- Adapter authors report that the genai-to-provider mapping is lossy. "Many of the fields don't map 1:1 with the Gemini API" ([#225](https://github.com/google/adk-go/issues/225)). adk-models-go "decided to not support a few features like specific token budgets … as we need to map from the GenAI types" ([#1097](https://github.com/google/adk-go/issues/1097)).
- `genai.Schema` has no `oneOf`/`anyOf`, so polymorphic structured output is silently stripped ([#465](https://github.com/google/adk-go/issues/465)).

**[src: confirms]** "Gemini coupling is total" is item 1 of the source doc's "awkward" list.

### 3.2 Interfaces that are sealed or lie

- A maintainer filed [#292](https://github.com/google/adk-go/issues/292) (Nov 2025, still open, `breaking-change`): "It's not possible to implement `tool.Tool` directly with a custom type (technically possible, but this tool won't work)."
- [#1595](https://github.com/google/adk-go/issues/1595) (Sep 2026) lists the five internal interfaces the run loop actually type-asserts on. It shows that a wrapper tool "can be registered on the agent and never called … the wrapper compiles, the run succeeds, and the capability is simply gone."
- [#1054](https://github.com/google/adk-go/issues/1054) asked to expose `PackTool`, because custom toolsets had to re-implement internal logic.
- Context creation is kept internal on purpose: "ADK users should not be able to create the context" ([#391](https://github.com/google/adk-go/issues/391)). This forced duplicate context implementations and import-cycle workarounds.

**[src: confirms]** "Interfaces that lie or are sealed".

### 3.3 Callback and context semantics that surprise people

- `BeforeToolCallback` returning args (rather than `nil`) **skips the tool** and all later callbacks. A user called this "disturbing" ([#498](https://github.com/google/adk-go/issues/498)). The maintainer's answer was that this is ADK convention: "just `return nil, nil`" after mutating args in place. **[src: confirms]** "first-non-nil-wins semantics".
- `Context.WithAgentTimeout` returns `(nil, nil)` on tool and callback contexts ([#1594](https://github.com/google/adk-go/issues/1594)). That bug follows directly from the "god context" merge.
- Guardrail authors cannot tell a policy denial from a runtime error in callbacks ([#1116](https://github.com/google/adk-go/issues/1116)).
- Plugins registered on the runner do not fire inside `agenttool` sub-agents ([#669](https://github.com/google/adk-go/issues/669)). Streaming tools bypass agent and plugin tool callbacks entirely ([#1515](https://github.com/google/adk-go/issues/1515)).

### 3.4 Launchers act like a framework, not a library

- Defining any custom `flag` in your own `main` breaks the launchers ("flag provided but not defined"). The workaround is to filter `os.Args` by hand ([#606](https://github.com/google/adk-go/issues/606), open).
- Plugins were not wired through the launcher or A2A ([#501](https://github.com/google/adk-go/issues/501), 10 comments).
- Web server timeouts were hardcoded at 15s, which broke streaming ([#263](https://github.com/google/adk-go/issues/263)).
- Users asked for an HTTP middleware seam ([#965](https://github.com/google/adk-go/issues/965)) and for an `http.Handler` instead of `SetupRouter` ([#257](https://github.com/google/adk-go/issues/257)).
- "Launchers aren't a well established concept in ADK, but an App is" ([#268](https://github.com/google/adk-go/issues/268), `breaking-change`, open since Nov 2025).

### 3.5 Docs lag behind code, and Python leaks into Go

- Doc examples did not compile against released versions ([#443](https://github.com/google/adk-go/issues/443), [#333](https://github.com/google/adk-go/issues/333), [#275](https://github.com/google/adk-go/issues/275)). The REST API did not match the docs' snake_case and camelCase ([#255](https://github.com/google/adk-go/issues/255), [#264](https://github.com/google/adk-go/issues/264), 10 and 7 comments).
- The shared ADK docs use `python_casing` and have few Go examples ([#389](https://github.com/google/adk-go/discussions/389)).
- ToolContext docs and the Go API disagree ([#467](https://github.com/google/adk-go/discussions/467)). Doc comments in `workflow/` are stale ([#1541](https://github.com/google/adk-go/issues/1541)). Session internals are undocumented ([#539](https://github.com/google/adk-go/issues/539)). Users discovered the optimistic concurrency semantics by trial and error ([#1229](https://github.com/google/adk-go/issues/1229)).

### 3.6 Dependency and release hygiene

- "adk-go has too many heavy dependencies for lightweight agent/tooling use cases" ([#743](https://github.com/google/adk-go/issues/743); closed for lack of detail).
- GORM as the database layer ([#236](https://github.com/google/adk-go/issues/236), [#272](https://github.com/google/adk-go/issues/272)). GORM table names ignore `NamingStrategy`, which blocks multi-tenant prefixes ([#538](https://github.com/google/adk-go/issues/538), [#699](https://github.com/google/adk-go/issues/699)).
- Breakage between releases:
  - v1.1.0 panicked, with no patch tag for a while ([#742](https://github.com/google/adk-go/issues/742));
  - a wrong tag was published ([#860](https://github.com/google/adk-go/issues/860));
  - v2.1.0's deploy Dockerfile used Go 1.25 while `go.mod` required 1.26.5 ([#1196](https://github.com/google/adk-go/issues/1196));
  - an openai-go minor version broke the v2.3.0 build ([#1435](https://github.com/google/adk-go/issues/1435)).

### 3.7 Contributions stall in review

- "Even after tagging on PR, we are not getting response" ([#412](https://github.com/google/adk-go/issues/412)).
- PR [#785](https://github.com/google/adk-go/pull/785), which fixes per-step toolsets, has had five follow-up pings since May and no first review.
- The OpenAI PR [#242](https://github.com/google/adk-go/pull/242) drew comments such as "How is this still not merged? Blistering pace here" and "This issue related to model support has remained unresolved for a year."

---

## 4. Big problems

### 4.1 Provider lock-in (the defining problem)

| Date | Event |
|---|---|
| Nov 2025 | Claude ([#225](https://github.com/google/adk-go/issues/225)) and OpenAI-compatible ([#341](https://github.com/google/adk-go/issues/341)) requested. Four community PRs follow ([#233](https://github.com/google/adk-go/pull/233), [#242](https://github.com/google/adk-go/pull/242), [#342](https://github.com/google/adk-go/pull/342), [#388](https://github.com/google/adk-go/pull/388)). |
| Early 2026 | The maintainer is "not sure on the best mechanism for long-term support of ecosystem models" and promises an `adk-go-community` repo ([#242](https://github.com/google/adk-go/pull/242)). The repo was never created: `gh repo view google/adk-go-community` returns not found, and users are still asking in [#1078](https://github.com/google/adk-go/discussions/1078). |
| Jun 2026 | v2.0 ships without OpenAI or Anthropic support. "ADK-Go is not a viable technology choice without them" ([#1097](https://github.com/google/adk-go/issues/1097)). The maintainer's reply: "Integrations … do not define the ADK versioning and are tracked independently." |
| Jul 2026 | OpenAI lands in v2.1.0 via [#1178](https://github.com/google/adk-go/pull/1178), about 8.5 months after #242. It supports the **Responses API only** and is marked experimental. |
| Aug–Sep 2026 | OpenAI adapter bugs: text-only, where images or files error with "unsupported content part" ([#1333](https://github.com/google/adk-go/issues/1333): "it is the most basic function"); wrong content type on multi-turn → 400 ([#1197](https://github.com/google/adk-go/issues/1197)); refusal deltas dropped ([#1466](https://github.com/google/adk-go/issues/1466)); terminal content dropped ([#1476](https://github.com/google/adk-go/issues/1476)); the caller's schema mutated ([#1450](https://github.com/google/adk-go/issues/1450)). Anthropic PR [#598](https://github.com/google/adk-go/pull/598) is still **open**, even though "the direction call is made". Requests for Ollama ([#320](https://github.com/google/adk-go/issues/320)), Azure/Bedrock ([#289](https://github.com/google/adk-go/issues/289)), a Bifrost-style gateway ([#1358](https://github.com/google/adk-go/issues/1358)) and Chat Completions providers ([#596](https://github.com/google/adk-go/issues/596)) remain open. |

The underlying reason, from a Go team member: "my biggest concern is whether the code will be maintainable by this repo maintainers for long term. If the REST API changes … or if the OpenAI is moving towards Responses … does the team have capacity to investigate, review fixes, and test?" ([#341](https://github.com/google/adk-go/issues/341)).

Adapter authors also describe how provider nuance leaks into the agent layer. For example, "Anthropic has tight conditions around tool input/response correlation and roles that can become tricky in adapters" ([#225](https://github.com/google/adk-go/issues/225); detailed in [#388](https://github.com/google/adk-go/pull/388)). Parallel tool responses sent one at a time break Mistral ("Not the same number of function calls and responses", [#357](https://github.com/google/adk-go/issues/357)).

**[src: confirms and extends]** The source doc notes "no Anthropic provider in-tree" and an experimental OpenAI adapter. The issues show this is the main adoption blocker, and it is organizational (review capacity and a parity-first mandate) as much as technical.

### 4.2 Gemini-model churn breaks the core loop

Even the favored provider regresses whenever Gemini changes.

- Gemini 3 sends leading metadata-only SSE chunks. The aggregator aborted with "empty response" on **40–50% of streaming calls** with search grounding ([#782](https://github.com/google/adk-go/issues/782): "a real blocker for us from using Gemini 3"). It was fixed in v1.4.0.
- Thought signatures are lost across parallel calls, confirmation parts, replays and history serialization ([#656](https://github.com/google/adk-go/issues/656), [#758](https://github.com/google/adk-go/issues/758), [#1633](https://github.com/google/adk-go/issues/1633)). adk-python's version of this bug has 32 comments ([adk-python#3705](https://github.com/google/adk-python/issues/3705)).
- `MALFORMED_FUNCTION_CALL` handling left users unsure which tool was called ([#325](https://github.com/google/adk-go/issues/325), [#492](https://github.com/google/adk-go/issues/492)). Maintainers replied that it is "related to the model's behavior rather than the framework".
- Parallel function calls arriving in separate contents were executed one at a time ([#357](https://github.com/google/adk-go/issues/357)).

### 4.3 Tool calling correctness

- Calling an unregistered tool crashes with a nil pointer instead of returning an error to the model ([#423](https://github.com/google/adk-go/issues/423), open). **[src: confirms]** "custom tools that panic crash the process".
- Tool errors:
  - `functiontool` originally could not return `error` at all. Dmitry Vyukov had to use `panic`/`recover` to pipe errors out ([#260](https://github.com/google/adk-go/issues/260)).
  - Errors were serialized to the model as `{}` ([#278](https://github.com/google/adk-go/issues/278), [#461](https://github.com/google/adk-go/issues/461)).
  - Vyukov's point about error semantics is worth quoting: "these errors have nothing to do with the end task … These shouldn't be passed to LLM … if the agent is helping user to plan a vacation, it shouldn't suddenly start talking about Kubernetes."
- `Toolset.Tools(ctx)` is evaluated once per run, not per step, so state-driven toolsets break ([#757](https://github.com/google/adk-go/issues/757)). **[src: confirms]**
- Nothing bounds the size of a tool result, so a large result is re-sent on every later call ([#1618](https://github.com/google/adk-go/issues/1618)).
- `OutputSchema` combined with tools gave a 400 from Gemini ([#307](https://github.com/google/adk-go/issues/307)). This is the same problem as adk-python's third most-reacted issue ([adk-python#701](https://github.com/google/adk-python/issues/701)). It conflicts with loop exit too ([#393](https://github.com/google/adk-go/issues/393)).
- Function tools accepted only struct inputs ([#330](https://github.com/google/adk-go/issues/330)).
- `agenttool`:
  - drops the child's state updates ([#1644](https://github.com/google/adk-go/issues/1644));
  - drops plugins ([#669](https://github.com/google/adk-go/issues/669));
  - leaks thinking parts into the result ([#697](https://github.com/google/adk-go/issues/697));
  - `SkipSummarization` terminates the parent loop ([#892](https://github.com/google/adk-go/issues/892)).

  **[src: confirms]** "`agenttool` discards child state".
- MCP:
  - the cached MCP session went stale ("connection closed" on every request after the first; [#399](https://github.com/google/adk-go/issues/399), 11 comments);
  - there is no `Close` on toolsets ([#481](https://github.com/google/adk-go/issues/481));
  - non-text results are dropped ([#1391](https://github.com/google/adk-go/issues/1391));
  - `_meta` is dropped, so there is no path for auth challenges ([#1165](https://github.com/google/adk-go/issues/1165));
  - tool names can collide across servers ([#1126](https://github.com/google/adk-go/issues/1126), [#1605](https://github.com/google/adk-go/issues/1605)).

### 4.4 Human-in-the-loop is fragile

HITL produced the densest cluster of bugs, and many are silent.

- Confirmation events were created with empty IDs, reported three times ([#558](https://github.com/google/adk-go/issues/558), [#587](https://github.com/google/adk-go/issues/587), [#590](https://github.com/google/adk-go/issues/590)). `LongRunningToolIDs` were built before IDs were set ([#551](https://github.com/google/adk-go/issues/551)).
- `RequestConfirmation()` caused an infinite agent loop ([#543](https://github.com/google/adk-go/issues/543)). A tool ran before confirmation arrived ([#876](https://github.com/google/adk-go/issues/876)).
- Confirmation broke with persistent stores because its events leaked into LLM history ([#682](https://github.com/google/adk-go/issues/682)). Function responses were not persisted before a pause ([#759](https://github.com/google/adk-go/issues/759)).
- Confirmation parts were emitted in map-iteration order ([#1052](https://github.com/google/adk-go/issues/1052)). `WithConfirmation` silently skips streaming tools, so gated tools ran unconfirmed ([#1408](https://github.com/google/adk-go/issues/1408)).
- v2 workflow resume restarted a nested `WorkflowAgent` from START ([#1125](https://github.com/google/adk-go/issues/1125)) and fails for a graph root with a join ([#1568](https://github.com/google/adk-go/issues/1568), open).

**[src: nuance]** The source doc praises the design ("one interrupt primitive", durable-by-log resume). The issues show the implementation is spread across flow, runner, content processor and three tool adapters, so every new path (streaming tools, nested workflows, persistent stores) re-breaks it. This supports the source doc's recommendation to make approval an Engine Policy applied once.

### 4.5 Sessions, state and persistence

- State writes:
  - `Session.State().Set` on a fetched session does nothing durable ([#324](https://github.com/google/adk-go/issues/324), open);
  - keys cannot be deleted ([#326](https://github.com/google/adk-go/issues/326), open);
  - only the last or first delta survives parallel tool calls, reported for state ([#354](https://github.com/google/adk-go/issues/354)) and twice for artifacts ([#493](https://github.com/google/adk-go/issues/493), [#623](https://github.com/google/adk-go/issues/623)).
- Backends diverge:
  - Vertex sessions drop `FunctionCall.ID` ([#679](https://github.com/google/adk-go/issues/679)), `ArtifactDelta` ([#902](https://github.com/google/adk-go/issues/902)), zero-arg calls ([#1321](https://github.com/google/adk-go/issues/1321)) and client event IDs ([#1329](https://github.com/google/adk-go/issues/1329));
  - `temp:` keys leak to storage ([#1354](https://github.com/google/adk-go/issues/1354), [#1410](https://github.com/google/adk-go/issues/1410), [#1611](https://github.com/google/adk-go/issues/1611));
  - MySQL strict mode fails ([#1177](https://github.com/google/adk-go/issues/1177));
  - Postgres delete fails when a session has events ([#482](https://github.com/google/adk-go/issues/482)).
- Concurrency control: optimistic concurrency on `last_update_time` makes every session handle an implicit exclusive lease, so an out-of-band append kills a running turn ([#1229](https://github.com/google/adk-go/issues/1229)). The "stale session" error is common ([#408](https://github.com/google/adk-go/issues/408), [#1170](https://github.com/google/adk-go/issues/1170)). **[src: confirms]** "Optimistic concurrency uses timestamps".
- Identity: the agent `Name` is used as the storage key, so renaming an agent orphans its state ([#674](https://github.com/google/adk-go/issues/674)).
- Missing capabilities: time travel and forking ([#343](https://github.com/google/adk-go/issues/343), 8 comments, done in a user fork). Events cannot separate thinking from the final answer ([#313](https://github.com/google/adk-go/issues/313)).

### 4.6 Concurrency bugs in a Go library

This cluster is embarrassing for a Go library.

- `State.All()` unlocks during map iteration, which causes a fatal "concurrent map iteration and map write". It was reported twice ([#309](https://github.com/google/adk-go/issues/309), [#561](https://github.com/google/adk-go/issues/561)).
- The in-memory session lacks locking ([#367](https://github.com/google/adk-go/issues/367)). The DB and Vertex services read `events` without a lock ([#1553](https://github.com/google/adk-go/issues/1553), [#1560](https://github.com/google/adk-go/issues/1560)).
- Parallel `single_turn` dispatches race on shared agent state ([#1137](https://github.com/google/adk-go/issues/1137)). `llmagent.New` aliases the caller's `Tools` slice ([#1490](https://github.com/google/adk-go/issues/1490)). `SequentialAgent.RunLive` races ([#1441](https://github.com/google/adk-go/issues/1441)).
- `ParallelWorker` has a non-deterministic error and drops a cancellation ([#1481](https://github.com/google/adk-go/issues/1481)).
- Live mode leaks one goroutine per reconnect ([#1152](https://github.com/google/adk-go/issues/1152)) and reconnects in a hot loop with no backoff ([#1620](https://github.com/google/adk-go/issues/1620)).

### 4.7 Unbounded loops, retries and deadlines

- There is no `MaxLLMCalls` on non-live runs, so "token cost grows quadratically" ([#1287](https://github.com/google/adk-go/issues/1287)). The live field exists but is never enforced ([#1286](https://github.com/google/adk-go/issues/1286)). **[src: confirms]** "No loop bound … the dead `MaxLLMCalls` field".
- There is no retry for 503/429 ([#710](https://github.com/google/adk-go/issues/710), PR [#732](https://github.com/google/adk-go/pull/732) awaiting "a direction call"). Rate limits "seem like silent errors" ([#542](https://github.com/google/adk-go/issues/542)). The maintainer's answer was that the only signal is the `err` from the iterator, and a 429 does not appear on `LLMResponse.ErrorCode`. adk-python had the same request ([adk-python#1214](https://github.com/google/adk-python/issues/1214)). **[src: nuance]** The source doc says LLM retry is delegated to the SDKs. Users could not find that option, which suggests the delegation is undiscoverable.
- When a deadline passes, all work is discarded and the caller gets a transport error instead of a partial answer ([#1617](https://github.com/google/adk-go/issues/1617)).
- Errors are not aggregated across LLM failures ([#798](https://github.com/google/adk-go/issues/798)). Users asked for `OnAgentErrorCallback` ([#709](https://github.com/google/adk-go/issues/709)) and a pipeline-error hook ([#886](https://github.com/google/adk-go/issues/886)).

### 4.8 Security defaults

- `adk web` bound an **unauthenticated** REST API and WebSocket to all interfaces, with no Origin check (CVSS 3.1 high confidentiality impact). Google's bug bounty program redirected the reporter to public issues ([#1154](https://github.com/google/adk-go/issues/1154)).
- Other reports:
  - `--h2c` disables slowloris protection ([#1224](https://github.com/google/adk-go/issues/1224));
  - `artifact://` refs are not scope-validated, so one tenant can read another's artifacts ([#1157](https://github.com/google/adk-go/issues/1157));
  - MCP command arguments are not validated ([#1569](https://github.com/google/adk-go/issues/1569));
  - Vertex `DeleteSession` does not enforce ownership ([#1194](https://github.com/google/adk-go/issues/1194)).

---

## 5. Most desired features

Ranked by combined signal: reactions, duplicates, comment volume and forks.

| # | Feature | Evidence | Maintainer response / status |
|---|---|---|---|
| 1 | **Non-Gemini models** (OpenAI, OpenAI-compatible, Claude, Ollama, Azure/Bedrock, gateways) | PRs [#242](https://github.com/google/adk-go/pull/242) (50 reactions), [#342](https://github.com/google/adk-go/pull/342) (34); issues [#341](https://github.com/google/adk-go/issues/341) (12), [#225](https://github.com/google/adk-go/issues/225) (15 comments), [#1358](https://github.com/google/adk-go/issues/1358) (7), [#320](https://github.com/google/adk-go/issues/320), [#1097](https://github.com/google/adk-go/issues/1097), [#596](https://github.com/google/adk-go/issues/596), [#289](https://github.com/google/adk-go/issues/289); 5+ third-party modules | OpenAI Responses shipped Jul 2026 (text only). Anthropic PR [#598](https://github.com/google/adk-go/pull/598) still open. The promised community repo was never created. "Integrations … do not define the ADK versioning." |
| 2 | **Context compaction** | [#298](https://github.com/google/adk-go/issues/298) (15 reactions, the top issue), [#1001](https://github.com/google/adk-go/issues/1001), [#589](https://github.com/google/adk-go/discussions/589); "without compaction adk-go has a pretty hard limit on its usefulness for long conversations" | Promised for "early March", shipped around Aug 2026 ([#1232](https://github.com/google/adk-go/pull/1232), [#1234](https://github.com/google/adk-go/pull/1234)). "Target is feature parity with adk-python." Requested extensions (manual trigger, access to unfiltered events, no-compact ranges, threshold-based restart) were **deferred**. |
| 3 | **Agent Skills** | [#540](https://github.com/google/adk-go/issues/540) (9 👍, 15 comments; "This is very important"); adk-python's most-reacted issue is also skills ([adk-python#3611](https://github.com/google/adk-python/issues/3611)) | Shipped in v2 (`skilltoolset`). The issue is still open. |
| 4 | **Evaluations** | [#240](https://github.com/google/adk-go/issues/240) (11 👍); the REST eval endpoints are stubs that return `Unimplemented`; community PR [#245](https://github.com/google/adk-go/pull/245) closed | "Finalizing our roadmap" (Nov 2025). **Still open**, with no public roadmap. |
| 5 | **OpenTelemetry parity** with content capture | [#479](https://github.com/google/adk-go/issues/479) (6), [#608](https://github.com/google/adk-go/issues/608) (content capture regressed in v0.5), [#789](https://github.com/google/adk-go/issues/789), [#857](https://github.com/google/adk-go/issues/857) (tool spans are siblings of model spans rather than children), [#439](https://github.com/google/adk-go/issues/439), [#1524](https://github.com/google/adk-go/issues/1524), [#1634](https://github.com/google/adk-go/issues/1634) | OTel shipped Feb 2026. Gaps in span nesting and token accounting remain open. |
| 6 | **Pluggable / lightweight storage** | [#236](https://github.com/google/adk-go/issues/236) (5, 9 comments), [#272](https://github.com/google/adk-go/issues/272), [#538](https://github.com/google/adk-go/issues/538) (5), [#339](https://github.com/google/adk-go/issues/339), [#340](https://github.com/google/adk-go/issues/340) | GORM kept. The rule is three implementations per service (in-memory, self-host, SaaS); database-specific ones go to the (nonexistent) community repo. |
| 7 | **Retry, rate limits, loop bounds** | [#710](https://github.com/google/adk-go/issues/710), [#542](https://github.com/google/adk-go/issues/542), [#1287](https://github.com/google/adk-go/issues/1287), [#1286](https://github.com/google/adk-go/issues/1286), [#1617](https://github.com/google/adk-go/issues/1617), [#1618](https://github.com/google/adk-go/issues/1618) | Retry PR awaits a direction call. The deadline wind-down PR was rejected for parity reasons (see section 6). |
| 8 | **Tool auth (OAuth)** | [#574](https://github.com/google/adk-go/issues/574) (3); `authPreprocessor` is a stub | Open. A GCP credential provider landed Sep 2026 ([#1173](https://github.com/google/adk-go/pull/1173)). |
| 9 | **Open tool/agent interfaces** | [#292](https://github.com/google/adk-go/issues/292), [#1595](https://github.com/google/adk-go/issues/1595), [#1054](https://github.com/google/adk-go/issues/1054), [#391](https://github.com/google/adk-go/issues/391) | Acknowledged since Nov 2025 and still open. |
| 10 | **UI protocols and streaming to frontends** | AG-UI [#1340](https://github.com/google/adk-go/issues/1340); streaming `agenttool` progress to UI ([#292](https://github.com/google/adk-go/issues/292) comment); bidi/live ([#496](https://github.com/google/adk-go/discussions/496), [#550](https://github.com/google/adk-go/issues/550)) | Live shipped in the v2 line. AG-UI has no response. |
| 11 | **Sandboxed code execution** | [#277](https://github.com/google/adk-go/discussions/277), [#319](https://github.com/google/adk-go/issues/319), [#1066](https://github.com/google/adk-go/issues/1066), [#394](https://github.com/google/adk-go/discussions/394), [#549](https://github.com/google/adk-go/discussions/549) | Open. Users built their own (gbash, agentic). |

Smaller requests with a clear signal:

- thinking level / budget ([#315](https://github.com/google/adk-go/issues/315))
- union structured output ([#465](https://github.com/google/adk-go/issues/465))
- a name-based model registry ([#1056](https://github.com/google/adk-go/issues/1056), shipped)
- an OpenAPI toolset ([#1464](https://github.com/google/adk-go/issues/1464))
- `GetUserState` and `AddEventsToMemory` parity ([#1158](https://github.com/google/adk-go/issues/1158), [#1160](https://github.com/google/adk-go/issues/1160))
- AGENTS.md support ([#471](https://github.com/google/adk-go/issues/471)).

---

## 6. Maintainer stance and trajectory

1. **adk-python is the source of truth, and parity overrides Go-native design.** This is the clearest statement in the tracker. A contributor built a deadline wind-down feature ([#1617](https://github.com/google/adk-go/issues/1617)), and a maintainer closed the PR:

   > "None of the other ADKs do this … adk-python is the source of truth for behaviour, so we'd end up the only port of five carrying a RunConfig field for a concept that doesn't exist anywhere else" ([#1627](https://github.com/google/adk-go/pull/1627)).

   The advice was to "raise this with adk-python first so the semantics land there and every port can follow". The same pattern shows up elsewhere:
   - Compaction's "target is feature parity with adk-python", with extensions deferred ([#298](https://github.com/google/adk-go/issues/298)).
   - Claude support should "keep the similar logic with python" ([#225](https://github.com/google/adk-go/issues/225)).
   - `RequireConfirmation` had to be split into two fields to mimic a Python union ([#284](https://github.com/google/adk-go/issues/284)).
   - The BigQuery plugin has a full "parity checklist" ([#1325](https://github.com/google/adk-go/issues/1325)).
   - Several issues are titled "(parity with adk-python)".

   **Implication:** adk-go cannot innovate at the Go layer. Innovation happens in Python first.
2. **Parity is selective on features and absolute on wire formats.** "If a new functionality in adk-python superseded some other already existing one, there's no need for adk-go to add an old one"; Planner is low priority "because gemini 2.5-flash/pro both already have thinking builtin" ([#364](https://github.com/google/adk-go/discussions/364)). Session formats, REST shapes and A2A payloads must match Python exactly ([#255](https://github.com/google/adk-go/issues/255), [#913](https://github.com/google/adk-go/issues/913), [#1251](https://github.com/google/adk-go/issues/1251)).
3. **`genai` coupling is intentional and permanent.** "We don't plan to change this" ([#225](https://github.com/google/adk-go/issues/225)). Every non-Gemini provider will be a translation layer into Gemini's types.
4. **Integrations are second-class work.** Model providers "do not define the ADK versioning" ([#1097](https://github.com/google/adk-go/issues/1097)). The plan was to push third-party code to `adk-go-community` ([#242](https://github.com/google/adk-go/pull/242), [#272](https://github.com/google/adk-go/issues/272)), which never materialized ([#1078](https://github.com/google/adk-go/discussions/1078)). The team later reversed course and took OpenAI in-tree, citing a maintenance horizon: "for a provider the team expects to maintain for years, starting from the more complete conversion layer is the lower-risk path" ([#233](https://github.com/google/adk-go/pull/233)).
5. **Keep the public surface small, even if that means sealing it.** Examples: "ADK users should not be able to create the context … we're free to refactor" ([#391](https://github.com/google/adk-go/issues/391)); exposing `gorm.DB` "should be a last resort" ([#236](https://github.com/google/adk-go/issues/236)); REST controllers moved to `internal` ([#415](https://github.com/google/adk-go/issues/415)). The cost is the sealed `Agent` and `Tool` interfaces users complain about.
6. **Design-first governance, arriving late.** By 2026 maintainers talk about "ask first territory in AGENTS.md" and "direction calls" before review ([#1627](https://github.com/google/adk-go/pull/1627), [#598](https://github.com/google/adk-go/pull/598), [#732](https://github.com/google/adk-go/pull/732)). Triage has improved: `needs review` labels, and a contributor (`indurireddy-TF`) who routes issues to maintainers. Decisions still bottleneck on a few people.
7. **Trajectory.**
   - v0.x (Nov 2025–Jan 2026): API renames and churn ([#53](https://github.com/google/adk-go/issues/53), [#142](https://github.com/google/adk-go/issues/142), [#401](https://github.com/google/adk-go/issues/401), [#418](https://github.com/google/adk-go/issues/418)).
   - v1 (spring 2026): parity work, OTel, Vertex services.
   - v2.0 (Jun 30 2026): graph workflows, collaboration modes and a unified `agent.Context`; the module path moved to `/v2`, and the release focused on "core library feature set" parity.
   - After v2: "expanding our integrations (especially model providers) is exactly where the team's focus is shifting next" ([#1097](https://github.com/google/adk-go/issues/1097)), plus a promised public H2 roadmap ([#1001](https://github.com/google/adk-go/issues/1001)) that I could not find published.
   - Open `breaking-change` issues still to land include finalizing `tool.Tool` ([#292](https://github.com/google/adk-go/issues/292)), replacing launchers with `App` ([#268](https://github.com/google/adk-go/issues/268)), a well-known "not exists" error ([#85](https://github.com/google/adk-go/issues/85)) and renaming `workflowagents` ([#336](https://github.com/google/adk-go/issues/336)), so more churn is likely.

---

## 7. Lessons for Dive

These are ordered by how much user pain they address and how much they differentiate Dive from ADK-Go.

### P0: where Dive can win outright

1. **Treat multi-provider support as the product, not an integration.**
   - This is the single largest unmet need in the ADK-Go tracker (section 4.1). Ship and maintain first-party **Anthropic, OpenAI (both Chat Completions and Responses), Gemini, and OpenAI-compatible endpoints (Ollama, vLLM, OpenRouter, gateways)**.
   - Each should support multimodal input, tool calls and reasoning round-tripped with signatures from day one. ADK's OpenAI adapter shipped text-only ([#1333](https://github.com/google/adk-go/issues/1333)) and without reasoning replay.
   - Keep Dive's public message and content types provider-neutral. ADK's refusal to decouple from `genai` ([#225](https://github.com/google/adk-go/issues/225)) is exactly what third-party adapter authors complain about.
   - Back each provider with a **shared conformance suite**: parallel tool calls, role and alternation constraints ([#388](https://github.com/google/adk-go/pull/388)), streaming aggregation of metadata-only chunks ([#782](https://github.com/google/adk-go/issues/782)), refusals ([#1466](https://github.com/google/adk-go/issues/1466)) and thought signatures ([#1633](https://github.com/google/adk-go/issues/1633)). This is how Dive answers the "can the team maintain this for years" objection in [#341](https://github.com/google/adk-go/issues/341).
2. **Keep every interface open and honest.**
   - A user type that satisfies `Tool` must work, with no hidden internal interfaces ([#292](https://github.com/google/adk-go/issues/292), [#1595](https://github.com/google/adk-go/issues/1595)). Export the helpers the built-ins use ([#1054](https://github.com/google/adk-go/issues/1054)).
   - Make wrappers and decorators a documented, tested pattern, because a wrapper that is silently bypassed is the worst outcome.
   - Never seal `Agent`.
3. **Design innovation is not bound to parity.** ADK-Go explicitly rejects Go-first ideas ([#1627](https://github.com/google/adk-go/pull/1627)). Dive can ship the features ADK declined or deferred:
   - deadline-aware wind-down with a partial answer ([#1617](https://github.com/google/adk-go/issues/1617));
   - tool-result size bounds ([#1618](https://github.com/google/adk-go/issues/1618));
   - manual and threshold-triggered compaction, access to unfiltered history, no-compact ranges ([#298](https://github.com/google/adk-go/issues/298));
   - session fork and time-travel ([#343](https://github.com/google/adk-go/issues/343)).

### P1: correctness users expect from a Go library

4. **Bound the loop and make errors typed.**
   - Provide `MaxSteps` (and token and cost budgets) with a typed stop reason ([#1287](https://github.com/google/adk-go/issues/1287)).
   - Build in retry with backoff for 429/503 that users can discover ([#710](https://github.com/google/adk-go/issues/710)).
   - Rate-limit and provider errors should be matchable with `errors.Is`/`errors.As` ([#542](https://github.com/google/adk-go/issues/542)).
   - Guardrail denials should be a distinct sentinel ([#1116](https://github.com/google/adk-go/issues/1116)).
5. **Get tool semantics right.**
   - An unknown tool or bad arguments should produce an error result to the model, never a panic ([#423](https://github.com/google/adk-go/issues/423)).
   - Separate *model-visible tool failures* from *infrastructure errors that abort the run* (Vyukov, [#260](https://github.com/google/adk-go/issues/260)), and never serialize an error as `{}` ([#278](https://github.com/google/adk-go/issues/278)).
   - Complete all parallel calls before the next model call, merge results in call order and merge every call's side effects ([#357](https://github.com/google/adk-go/issues/357), [#354](https://github.com/google/adk-go/issues/354), [#623](https://github.com/google/adk-go/issues/623)).
   - Resolve toolsets per step ([#757](https://github.com/google/adk-go/issues/757)).
   - Offer a configurable concurrency bound ([#1051](https://github.com/google/adk-go/issues/1051)).
   - Allow structured output together with tools ([#307](https://github.com/google/adk-go/issues/307)).
6. **Build one HITL primitive and test it across every path.** ADK's design is good, but its bugs come from reimplementing approval per path (section 4.4). In Dive, approval should be an Engine policy applied before any dispatch, including streaming tools ([#1408](https://github.com/google/adk-go/issues/1408)). It also needs stable IDs, deterministic ordering ([#1052](https://github.com/google/adk-go/issues/1052)) and resume tests for nested and sub-agent cases ([#1125](https://github.com/google/adk-go/issues/1125)).
7. **Make race-free code a release gate.**
   - Run `-race` in CI with concurrent-session tests.
   - Copy caller slices at construction ([#1490](https://github.com/google/adk-go/issues/1490)).
   - Never unlock inside a map range ([#309](https://github.com/google/adk-go/issues/309), [#561](https://github.com/google/adk-go/issues/561)).
   - Keep no mutable state on shared agent definitions ([#1137](https://github.com/google/adk-go/issues/1137)).
8. **Make session and state semantics explicit.**
   - Mutations should be explicit and durable, including deletes ([#324](https://github.com/google/adk-go/issues/324), [#326](https://github.com/google/adk-go/issues/326)).
   - Use stable IDs as keys, not display names ([#674](https://github.com/google/adk-go/issues/674)).
   - Define a JSON encoding with explicit tags and a golden-file test ([#1251](https://github.com/google/adk-go/issues/1251)).
   - Document multi-writer and concurrency semantics ([#1229](https://github.com/google/adk-go/issues/1229)).
   - Run one conformance suite across all stores, because ADK's backends each drop different fields.
   - Sub-agents must propagate state, plugins and events to the parent ([#1644](https://github.com/google/adk-go/issues/1644), [#669](https://github.com/google/adk-go/issues/669)).

### P2: fit and finish

9. **Be a library, not a framework.** Do not own `main`, flags or the HTTP server. Hand out `http.Handler`s and middleware seams ([#606](https://github.com/google/adk-go/issues/606), [#965](https://github.com/google/adk-go/issues/965), [#257](https://github.com/google/adk-go/issues/257)). Any dev server should bind to loopback with auth or origin checks by default ([#1154](https://github.com/google/adk-go/issues/1154)).
10. **Keep the core dependency graph small.** Put GORM-style or cloud-SDK dependencies in subpackages or modules users opt into. Prefer `database/sql` for a Postgres store, which fits the DeepNoodle stack ([#236](https://github.com/google/adk-go/issues/236), [#743](https://github.com/google/adk-go/issues/743)). Allow table prefixes ([#538](https://github.com/google/adk-go/issues/538)).
11. **Adopt OTel GenAI semantic conventions from day one.** Use correct span nesting, with tool spans as children of the model or step span ([#857](https://github.com/google/adk-go/issues/857)). Record tool definitions and all token categories ([#789](https://github.com/google/adk-go/issues/789), [#1524](https://github.com/google/adk-go/issues/1524)). Content capture should be opt-in ([#608](https://github.com/google/adk-go/issues/608)).
12. **Make MCP robust.** It needs reconnect on a stale session ([#399](https://github.com/google/adk-go/issues/399)), `Close` ([#481](https://github.com/google/adk-go/issues/481)), non-text results and `_meta` passthrough ([#1391](https://github.com/google/adk-go/issues/1391), [#1165](https://github.com/google/adk-go/issues/1165)), and name prefixing with collision detection ([#1126](https://github.com/google/adk-go/issues/1126), [#1605](https://github.com/google/adk-go/issues/1605)).
13. **Fill the evals gap.** ADK-Go has had stubbed eval endpoints for about 10 months ([#240](https://github.com/google/adk-go/issues/240)). A small Go-native eval harness would be a visible differentiator: trajectory matching, response matching, LLM-as-judge, and runs from `go test`.
14. **Provide frontend streaming adapters.** An AG-UI adapter ([#1340](https://github.com/google/adk-go/issues/1340)) and sub-agent progress events that reach the caller ([#292](https://github.com/google/adk-go/issues/292) comment; [adk-python#3984](https://github.com/google/adk-python/issues/3984)) are cheap, and both are wanted.

### Process lessons

15. **Docs must compile against the tagged release.** Test doc snippets in CI and put Go-first examples in the docs ([#443](https://github.com/google/adk-go/issues/443), [#389](https://github.com/google/adk-go/discussions/389)).
16. **Respond to community PRs and don't promise what you won't build.** A promised repo that never appeared ([#1078](https://github.com/google/adk-go/discussions/1078)), PRs left without review for months ([#785](https://github.com/google/adk-go/pull/785)), and a v2 release without the most-requested feature ([#1097](https://github.com/google/adk-go/issues/1097)) cost ADK-Go goodwill that its technical quality had earned. Ship patch releases for build-breaking regressions ([#742](https://github.com/google/adk-go/issues/742), [#1435](https://github.com/google/adk-go/issues/1435)).
17. **Keep what users like about ADK-Go:**
    - the `iter.Seq2` run surface;
    - the small model interface;
    - storage behind interfaces;
    - "less magic";
    - platform seams for durable execution ([#963](https://github.com/google/adk-go/issues/963)).

    None of these drew complaints, and several drew explicit praise.

---

## 8. Appendix: high-signal issue index

The reactions column counts all reaction types; 👍 is shown in parentheses where it differs. C is the comment count. States are as of 2026-09-26.

| # | Title (abridged) | State | React | C | Theme | Takeaway |
|---|---|---|---|---|---|---|
| [PR 242](https://github.com/google/adk-go/pull/242) | Add support for the OpenAI API | closed (superseded) | 50 | 21 | Providers | Strongest demand signal in the repo; waited ~8.5 months, then landed via #1178 |
| [PR 342](https://github.com/google/adk-go/pull/342) | OpenAI-compatible third-party provider | closed | 34 | 27 | Providers | Chat Completions providers (Ollama, OpenRouter) still unserved |
| [298](https://github.com/google/adk-go/issues/298) | ADR-010: Native session history compaction | open | 15 (0) | 11 | Context | Top issue; shipped Aug 2026 as Python parity, extensions deferred |
| [540](https://github.com/google/adk-go/issues/540) | When will skills be supported? | open | 9 | 15 | Skills | Shipped in v2; mirrors adk-python's top issue |
| [225](https://github.com/google/adk-go/issues/225) | Plan to support the Claude model? | open | 7 (6) | 15 | Providers | Maintainer: genai coupling "intentional"; Anthropic still unmerged |
| [341](https://github.com/google/adk-go/issues/341) | Third-party OpenAI-compatible providers | open | 12 | 4 | Providers | Go team's concern is long-term maintenance capacity |
| [240](https://github.com/google/adk-go/issues/240) | [Feature] Evaluations | open | 11 | 5 | Evals | Eval endpoints stubbed; no roadmap; opening for Dive |
| [236](https://github.com/google/adk-go/issues/236) | Not use gorm for session storage? | closed | 5 | 9 | Storage | Library users dislike ORM dependencies; service interface is the escape hatch |
| [1358](https://github.com/google/adk-go/issues/1358) | LLM gateway as model backend | open | 7 (4) | 3 | Providers | Want a Go-native LiteLLM equivalent (Bifrost) |
| [479](https://github.com/google/adk-go/issues/479) | ADK OpenTelemetry | closed | 6 | 3 | Observability | Shipped Feb 2026 for Python parity |
| [399](https://github.com/google/adk-go/issues/399) | mcptoolset stale cached session | closed | 1 | 11 | MCP | Long-lived MCP sessions need health checks and reconnect |
| [357](https://github.com/google/adk-go/issues/357) | Missing parallel function call execution | closed | 4 | 5 | Tools | Gemini 3 parallel calls ran one at a time; breaks strict providers |
| [782](https://github.com/google/adk-go/issues/782) | SSE aggregator aborts on metadata-only chunks | closed | 2 | 8 | Streaming | 40–50% streaming failures on Gemini 3 |
| [1333](https://github.com/google/adk-go/issues/1333) | openaimodel: image/file input | open | 2 | 7 | Providers | New OpenAI adapter is text-only |
| [538](https://github.com/google/adk-go/issues/538) | GORM TablePrefix ignored | open | 5 | 1 | Storage | Hardcoded table names block multi-tenancy |
| [320](https://github.com/google/adk-go/issues/320) | Support Ollama models | open | 3 | 5 | Providers | Local dev without API keys |
| [501](https://github.com/google/adk-go/issues/501) | Launcher doesn't support plugins | closed | 0 | 10 | Launcher | Launchers lag behind the runner's features |
| [255](https://github.com/google/adk-go/issues/255) | REST API doesn't match docs | closed | 0 | 10 | Docs / parity | Python casing quirks forced onto Go |
| [1097](https://github.com/google/adk-go/issues/1097) | V1/V2 parity and OpenAI/Anthropic support | open | 2 | 5 | Providers | "Not a viable technology choice without them" |
| [292](https://github.com/google/adk-go/issues/292) | Finalize tool.Tool interface | open | 3 | 3 | Interfaces | Custom tools cannot implement the public interface |
| [1595](https://github.com/google/adk-go/issues/1595) | Tool-callable interfaces are internal-only | open | 0 | 2 | Interfaces | Wrappers silently bypassed |
| [391](https://github.com/google/adk-go/issues/391) | Unified Invocation/Callback context | open | 0 | 6 | Context API | "Users should not be able to create the context" |
| [498](https://github.com/google/adk-go/issues/498) | BeforeToolCallbacks behavior "disturbing" | closed | 0 | 6 | Callbacks | Non-nil return skips the tool; surprising |
| [260](https://github.com/google/adk-go/issues/260) | functiontool not returning error | closed | 1 | 3 | Tools | Distinguish infra errors from model-visible failures |
| [423](https://github.com/google/adk-go/issues/423) | Invalid tool calls crash | open | 2 | 1 | Tools | Unknown tool should produce an error result, not a nil deref |
| [757](https://github.com/google/adk-go/issues/757) | Toolset.Tools evaluated once per run | open | 2 | 2 | Tools | Dynamic toolsets broken; fix PR unreviewed since May |
| [1644](https://github.com/google/adk-go/issues/1644) | agenttool child state doesn't reach parent | open | 0 | 3 | Multi-agent | Sub-agent isolation loses data |
| [669](https://github.com/google/adk-go/issues/669) | Plugins not propagated in agenttool | open | 2 | 1 | Multi-agent | Cross-cutting policy leaks at sub-agent boundaries |
| [558](https://github.com/google/adk-go/issues/558) / [587](https://github.com/google/adk-go/issues/587) / [590](https://github.com/google/adk-go/issues/590) | Confirmation events with empty ID | closed | 0 | 1 | HITL | Reported three times; DB primary-key violations |
| [543](https://github.com/google/adk-go/issues/543) | RequestConfirmation causes infinite loop | closed | 0 | 0 | HITL | HITL control flow fragile |
| [1408](https://github.com/google/adk-go/issues/1408) | WithConfirmation skips streaming tools | closed | 0 | 1 | HITL | Gated tools ran unconfirmed |
| [1125](https://github.com/google/adk-go/issues/1125) | Nested WorkflowAgent fails to resume | closed | 0 | 6 | HITL / v2 | Resume restarts the sub-workflow from START |
| [324](https://github.com/google/adk-go/issues/324) | Session.State().Get/Set don't work | open | 0 | 5 | State | Mutation semantics unclear |
| [326](https://github.com/google/adk-go/issues/326) | Can't delete state keys | open | 0 | 7 | State | Missing a basic operation |
| [354](https://github.com/google/adk-go/issues/354) | Only last state change of parallel calls committed | closed | 1 | 2 | State | Side-effect merge bug (also #493, #623 for artifacts) |
| [1229](https://github.com/google/adk-go/issues/1229) | No second writer without invalidating handle | open | 0 | 0 | Persistence | Timestamp OCC is an implicit exclusive lease |
| [1251](https://github.com/google/adk-go/issues/1251) | Event JSON silently corrupts Python exchange | closed | 0 | 0 | Parity / encoding | No JSON tags; cross-runtime corruption |
| [343](https://github.com/google/adk-go/issues/343) | Session time travel and forking | open | 0 | 8 | Sessions | Built by a user in a fork; no maintainer uptake |
| [309](https://github.com/google/adk-go/issues/309) / [561](https://github.com/google/adk-go/issues/561) | State.All unlocks during iteration | closed | 0 | 2/5 | Concurrency | Fatal concurrent map panic |
| [1137](https://github.com/google/adk-go/issues/1137) | Parallel single_turn dispatch race | closed | 0 | 2 | Concurrency | Mutable state on shared agent definitions |
| [1287](https://github.com/google/adk-go/issues/1287) | No way to bound LLM calls | closed | 0 | 3 | Limits | Quadratic token cost from unbounded loops |
| [710](https://github.com/google/adk-go/issues/710) | Retry mechanism for model errors | open | 0 | 2 | Resilience | 503s from Gemini; retry PR stalled |
| [542](https://github.com/google/adk-go/issues/542) | Rate limits seem like silent errors | open | 0 | 4 | Errors | Only signal is the iterator's `err` |
| [1617](https://github.com/google/adk-go/issues/1617) | Reserve runway before a hard deadline | open | 1 | 1 | Limits | PR rejected: "adk-python is the source of truth" |
| [1618](https://github.com/google/adk-go/issues/1618) | No way to bound tool result size | open | 0 | 2 | Tools | Oversized results re-sent on every call |
| [608](https://github.com/google/adk-go/issues/608) | Spans missing opt-in content capture | closed | 3 | 2 | Observability | Regression in v0.5 telemetry migration |
| [857](https://github.com/google/adk-go/issues/857) | execute_tool spans are siblings of generate_content | open | 0 | 1 | Observability | Broken trace hierarchy |
| [574](https://github.com/google/adk-go/issues/574) | Support for tool auth | open | 3 | 1 | Auth | OAuth tool auth is a stub |
| [1340](https://github.com/google/adk-go/issues/1340) | AG-UI protocol integration | open | 3 (1) | 2 | Frontend | No official answer |
| [606](https://github.com/google/adk-go/issues/606) | Custom flags break launchers | open | 0 | 1 | Launcher | Framework owns `main` |
| [268](https://github.com/google/adk-go/issues/268) | Replace launchers with App | open | 2 | 3 | Launcher / API | Pending breaking change |
| [1154](https://github.com/google/adk-go/issues/1154) | `adk web` unauthenticated on all interfaces | closed | 0 | 3 | Security | Insecure dev-server defaults |
| [1157](https://github.com/google/adk-go/issues/1157) | artifact:// refs lack scope validation | open | 0 | 3 | Security | Cross-tenant escape |
| [443](https://github.com/google/adk-go/issues/443) | Documented types undefined | closed | 0 | 7 | Docs | Docs and release out of sync |
| [743](https://github.com/google/adk-go/issues/743) | Too many heavy dependencies | closed | 0 | 2 | Dependencies | Lightweight users feel the weight |
| [1196](https://github.com/google/adk-go/issues/1196) | v2.1.0 incompatible Go builder version | closed | 0 | 4 | Release | Release hygiene |
| [963](https://github.com/google/adk-go/issues/963) / [1051](https://github.com/google/adk-go/issues/1051) | Time/UUID providers; TaskRunner seam | closed | 0 | 0/1 | Durability | Shipped; valued by durable-execution users |
| [D 364](https://github.com/google/adk-go/discussions/364) | Design philosophy and parity | — | 3 up | 4 | Stance | "Less magical" is the goal; parity selective for features, strict for wire formats |
| [D 1078](https://github.com/google/adk-go/discussions/1078) | ADK Go Community repo | — | 1 up | 0 | Governance | Promised community repo never created |
| [adk-python 3705](https://github.com/google/adk-python/issues/3705) | Gemini 3 missing thought_signature | closed | — | 32 | Context | Same model-churn failure mode across ports |
| [adk-python 701](https://github.com/google/adk-python/issues/701) | Structured output + tool call | closed | — | 17 | Context | Same limit as Go #307 |
