# Eino (CloudWeGo) — What the Issue Tracker Says

- **Repos:** `github.com/cloudwego/eino` (core) and `github.com/cloudwego/eino-ext` (providers, MCP, callbacks, retrievers)
- **Snapshot:** 2026-09-26
- **Companion doc:** [`eino.md`](./eino.md) is the source-level analysis. This doc covers user experience as reported in issues, discussions, and PR threads. Where the two docs agree or disagree, this doc says so.
- **Language:** About 60% of threads are in Chinese. Quotes below are translated unless they were in English. The original is kept where the wording matters.

Link shorthand: `#N` is `https://github.com/cloudwego/eino/issues/N`, `ext#N` is `https://github.com/cloudwego/eino-ext/issues/N`, and `disc#N` is `https://github.com/cloudwego/eino/discussions/N`. Every reference below is a full link.

---

## 1. Method

### Repo stats

| | eino | eino-ext |
|---|---|---|
| Issues (all time) | 415 | 227 |
| Open / closed | 116 / 299 | 48 / 179 |
| Date range | 2025-01-14 to 2026-09-25 | 2024-12-26 to 2026-09-26 |
| Stars / forks / PRs | 13.2k / 1.1k / 892 | — |
| Discussions | 10 (all maintainer announcements; only maintainers can create them, see [#155](https://github.com/cloudwego/eino/issues/155)) | — |
| Median time to close | 8 days | — |
| Issues with zero comments | 89 | — |

Issue volume is steady at 15 to 35 a month. There is a spike in September 2026 (28 issues). Most of that spike is detailed race, panic, and leak reports that cite `file:line` and include `-race` repros ([#1236](https://github.com/cloudwego/eino/issues/1236), [#1254](https://github.com/cloudwego/eino/issues/1254), [#1256](https://github.com/cloudwego/eino/issues/1256), [#1263](https://github.com/cloudwego/eino/issues/1263), [#1279](https://github.com/cloudwego/eino/issues/1279), [#1318](https://github.com/cloudwego/eino/issues/1318)). They read like tool-assisted audits, not organic user reports. I count them as evidence of code-quality hot spots, not of user demand.

### How I found the signal

1. **Reactions are a weak signal here.** The maximum is 6 reactions ([#996](https://github.com/cloudwego/eino/issues/996), [#68](https://github.com/cloudwego/eino/issues/68)). Chinese-speaking users rarely react on GitHub. I used reactions only as a tiebreaker.
2. **Comment count** was the main engagement metric. Threads with 10 or more comments are almost all streaming or tool-calling breakage ([#806](https://github.com/cloudwego/eino/issues/806) 18, [#120](https://github.com/cloudwego/eino/issues/120) 17, [#613](https://github.com/cloudwego/eino/issues/613) 12, [#631](https://github.com/cloudwego/eino/issues/631) 12), interrupt semantics ([#523](https://github.com/cloudwego/eino/issues/523) 17, [#859](https://github.com/cloudwego/eino/issues/859) 10), or tooling/DSL ([#109](https://github.com/cloudwego/eino/issues/109) 16, [#127](https://github.com/cloudwego/eino/issues/127) 15, [ext#128](https://github.com/cloudwego/eino-ext/issues/128) 17).
3. **Duplicate clusters.** I ran keyword regexes over all titles and bodies and counted issues per theme. The largest clusters are listed below. Repeated duplicates are the strongest signal that a problem hurt many users.

   | Theme | Issues (title match) | Issues (title + body) |
   |---|---|---|
   | Interrupt / checkpoint / resume / HITL | 20 | 45 |
   | Memory / session / history | 17 | 44 |
   | Multi-agent / transfer / supervisor | 15 | 51 |
   | Callbacks / tracing | 16 | 46 |
   | Reasoning / thinking content | 15 | 33 |
   | Streaming tool-call detection | 10 | 31 |
   | `kin-openapi` / OpenAPI3 build breaks | 13 | 19 |
   | Skills | 10 | 11 |
   | Tool-call ID concat / "tool not found in toolsNode" | 6 | 8 |
   | Empty tool arguments | 4 | 4 |

4. **Maintainer statements.** I read every thread that had a comment from the core maintainers (`shentongmartin`, `meguminnnnnnnnn`, `mrh997`, `hi-pender`, `kuhahalong`, `JonXSnow`, `tangchaojun-bytedance`). I looked for "by design", "no plan", and "next version" statements.
5. **Roadmap artifacts.** These include the `C-feature-accepted` batch that a maintainer filed in March 2026 ([#861](https://github.com/cloudwego/eino/issues/861) to [#877](https://github.com/cloudwego/eino/issues/877)), the release discussions ([disc#397](https://github.com/cloudwego/eino/discussions/397), [disc#527](https://github.com/cloudwego/eino/discussions/527), [disc#710](https://github.com/cloudwego/eino/discussions/710), [disc#1159](https://github.com/cloudwego/eino/discussions/1159)), and issues that caused breaking changes.
6. **Long-open issues with sustained discussion.** Examples are [#127](https://github.com/cloudwego/eino/issues/127) (open since 2025-03), [#524](https://github.com/cloudwego/eino/issues/524), [#631](https://github.com/cloudwego/eino/issues/631), and [#461](https://github.com/cloudwego/eino/issues/461).
7. **eino-ext.** I applied the same ranking and kept the provider and MCP issues that change a design conclusion.

---

## 2. What users love

Explicit praise is rare in the tracker. Most of it is a preface to a bug report. What exists is consistent:

- **Go-native, typed, and debuggable compared with Python frameworks.** In a thread titled "the concept is good but it's very hard to debug", another user answered: *"I think eino is the easiest to debug of these frameworks. It's Go, everything is predefined structs, debugging is just following the code. LangChain, Dify, and AutoGen are much thornier"* ([#614](https://github.com/cloudwego/eino/issues/614)).
- **Production credibility.** Teams that ask whether Eino can replace LangChain + LangGraph get this maintainer answer: *"validated in large-scale production; core capability and extensibility are sufficient; out-of-the-box ecosystem integrations are not as good as LangChain + LangGraph"* ([#830](https://github.com/cloudwego/eino/issues/830)). The adopters thread lists ByteDance and several Chinese companies in production ([#68](https://github.com/cloudwego/eino/issues/68)).
- **Smooth development once it works.** A user titled an issue *"The development process was extremely smooth, but I would appreciate an example of OpenAI's responses API"* ([#1023](https://github.com/cloudwego/eino/issues/1023)). Other threads open with "thank you for this great library" ([#567](https://github.com/cloudwego/eino/issues/567), [#369](https://github.com/cloudwego/eino/issues/369)) or "Loving eino so far" ([disc#397](https://github.com/cloudwego/eino/discussions/397)).
- **Interrupt/resume, once it exists.** HITL was the most-asked-for missing feature in 2025 ([#58](https://github.com/cloudwego/eino/issues/58), [#68](https://github.com/cloudwego/eino/issues/68)). After v0.7 ([disc#527](https://github.com/cloudwego/eino/discussions/527)), the threads change from "how do I pause" to edge cases such as multi-interrupt rollback and callbacks on resume ([#683](https://github.com/cloudwego/eino/issues/683), [#859](https://github.com/cloudwego/eino/issues/859)). That shift suggests people are using it in real work.
- **Middleware as the extension point.** Several requests were closed by "do it in `ChatModelAgentMiddleware`": dynamic model or tool switching per iteration ([#848](https://github.com/cloudwego/eino/issues/848)), tool-error conversion ([#928](https://github.com/cloudwego/eino/issues/928), [#1010](https://github.com/cloudwego/eino/issues/1010)), and persistence ([#728](https://github.com/cloudwego/eino/issues/728)). One user called the new middleware chapter of the docs "very helpful" ([#889](https://github.com/cloudwego/eino/issues/889)).
- **Agent-as-tool with isolated context.** A user expected a cheap sub-agent to hide its context from an expensive supervisor. The maintainer pointed to `NewAgentTool`, and the user replied "You are absolutely correct" ([#567](https://github.com/cloudwego/eino/issues/567)). Community members now tell each other that "transfer ... has been effectively deprecated; wrap agents as tools" ([#1291](https://github.com/cloudwego/eino/issues/1291)).
- **Responsive maintainers.** The median close time is 8 days. A small core team answers almost every question, often within hours, in both Chinese and English.

**Connection to eino.md:** these match §8.1 of `eino.md`: middleware, interrupts, and agents-as-tools. Nothing in the tracker praises the graph engine (`compose`) specifically. Graph threads are mostly questions about how to express something ([#69](https://github.com/cloudwego/eino/issues/69), [#353](https://github.com/cloudwego/eino/issues/353), [#424](https://github.com/cloudwego/eino/issues/424), [#462](https://github.com/cloudwego/eino/issues/462)).

---

## 3. What users hate / friction

### 3.1 Streaming output needs framework-specific workarounds

This is the single largest source of anger. The legacy `flow/agent/react` agent decides whether a turn is a tool call by inspecting the stream (`StreamToolCallChecker`). The default checker looks only at the **first chunk** ([#892](https://github.com/cloudwego/eino/issues/892)). Models that emit text before tool calls (DeepSeek, Claude, many Qwen deployments) therefore silently end the loop without running tools. The documented fix is a checker that drains the whole stream, and that fix **destroys streaming**: output arrives all at once ([#477](https://github.com/cloudwego/eino/issues/477)). The escape hatch is `WithMessageFuture`, which was not in the docs ("we haven't synced WithMessageFuture usage to the docs yet", [#477](https://github.com/cloudwego/eino/issues/477)). It also only works if you start the reader goroutine *before* calling `Stream` ([#806](https://github.com/cloudwego/eino/issues/806)).

- [#806](https://github.com/cloudwego/eino/issues/806) (18 comments), titled "**Fatal problem**: streaming tool-call detection". The reporter's second comment: *"planning to switch back to langchain"*.
- [#477](https://github.com/cloudwego/eino/issues/477): a user wrote *"the documented solution doesn't solve the problem at all; a whole pile of issues are about this."*
- [#445](https://github.com/cloudwego/eino/issues/445) asks for the text → tool → text pattern. The maintainer answered that the root fix is *"stop relying on the react agent's final output; rely on callback or MessageFuture process data. ADK's event stream embodies this."*
- Reasoning content has the same problem. It does not stream through `agent.Stream`, only through callbacks ([#306](https://github.com/cloudwego/eino/issues/306), where one user asked *"why is it designed so that normal logic lives in callbacks?"*; also [#316](https://github.com/cloudwego/eino/issues/316) and [#674](https://github.com/cloudwego/eino/issues/674)).
- Duplicates: [#120](https://github.com/cloudwego/eino/issues/120), [#192](https://github.com/cloudwego/eino/issues/192), [#243](https://github.com/cloudwego/eino/issues/243), [#296](https://github.com/cloudwego/eino/issues/296), [#333](https://github.com/cloudwego/eino/issues/333), [#348](https://github.com/cloudwego/eino/issues/348), [#613](https://github.com/cloudwego/eino/issues/613), [#772](https://github.com/cloudwego/eino/issues/772).

**Connection to eino.md:** this confirms §4.3 and §8.2.2. The branch-on-stream design leaks into the user experience. ADK still carries a `StreamGraphBranch` inherited from this design, and streaming middleware keeps reintroducing blocking: the reduction middleware drains tool streams synchronously ([#1064](https://github.com/cloudwego/eino/issues/1064)).

### 3.2 Two agent APIs, and much "you're using the wrong one"

`flow/agent/react` and `adk` coexist. A large share of support answers amount to "use ADK", "use `adk.Runner`", or "that option only works through Runner":

- `WithSessionValues` does nothing unless the agent runs through `adk.Runner` ([#843](https://github.com/cloudwego/eino/issues/843)).
- Interrupt info appears empty unless you use `adk.Runner` ([#615](https://github.com/cloudwego/eino/issues/615)).
- `WithToolList` required a second, nested option to take effect ([#215](https://github.com/cloudwego/eino/issues/215)).
- A user asked whether react would be removed, since "everything react has, adk has." The answer was no: both have scenarios ([#445](https://github.com/cloudwego/eino/issues/445)).
- Passing provider options into an agent's model "has to be wrapped several layers deep" ([ext#265](https://github.com/cloudwego/eino-ext/issues/265)): `agent.WithComposeOptions(compose.WithChatModelOption(qwen.WithEnableThinking(false)))`.

### 3.3 Docs lag the code; examples are the real docs

- *"It's genuinely painful to use; the docs are terrible, all technical theory, too few examples"* ([#614](https://github.com/cloudwego/eino/issues/614)). The maintainer responded by adding a cookbook section.
- Broken links, missing images, and 404s recur: [#112](https://github.com/cloudwego/eino/issues/112), [#342](https://github.com/cloudwego/eino/issues/342), [#561](https://github.com/cloudwego/eino/issues/561), [#570](https://github.com/cloudwego/eino/issues/570), [#818](https://github.com/cloudwego/eino/issues/818), [#1208](https://github.com/cloudwego/eino/issues/1208), and [#1212](https://github.com/cloudwego/eino/issues/1212) (`llms.txt` has six dead links). Quickstart code is outdated in [#1265](https://github.com/cloudwego/eino/issues/1265). Official indexer examples don't run ([#545](https://github.com/cloudwego/eino/issues/545)).
- Import-path confusion: `tool.NewTool` vs `utils.NewTool` produced an 11-comment thread ([#543](https://github.com/cloudwego/eino/issues/543)).
- AI coding assistants hallucinate Eino APIs. Users asked for an official prompt or AGENTS.md ([#616](https://github.com/cloudwego/eino/issues/616), [#715](https://github.com/cloudwego/eino/issues/715)). The maintainers accepted this as roadmap items ([#863](https://github.com/cloudwego/eino/issues/863), [#864](https://github.com/cloudwego/eino/issues/864)).

### 3.4 Naming and API-shape complaints

- "Some design patterns are really strange." For example, a ReAct demo when models already do tool calling natively ([#663](https://github.com/cloudwego/eino/issues/663)). The first reply: "written by Java people".
- `planexecute` uses `Model` in one config and `ChatModel` in the next. The maintainer agreed: *"It is a bit. It's hard to change now."* (是有点哈。现在也不好改了, [#700](https://github.com/cloudwego/eino/issues/700)).
- Instruction templating is on by default and breaks on LaTeX braces ([#685](https://github.com/cloudwego/eino/issues/685)). This confirms `eino.md` §8.2.12.
- Graph fan-in only merges maps. A 12-comment proposal added `RegisterValuesMergeFunc` ([#132](https://github.com/cloudwego/eino/issues/132)).
- Callbacks fire twice depending on whether you pass `Model` or `ToolCallingModel` ([#174](https://github.com/cloudwego/eino/issues/174)). Callbacks carried in `ctx` leak into model calls a tool makes internally, so tool-internal LLM output shows up as agent events ([#517](https://github.com/cloudwego/eino/issues/517)). Callbacks are lost after resume because the resume `ctx` lacks the callback manager ([#859](https://github.com/cloudwego/eino/issues/859)). These three all come from **ambient context-propagated callbacks**.

### 3.5 Dependency and versioning pain

- **`kin-openapi`** is the largest duplicate cluster in the tracker: 13 issues about build breaks and a CVE ([#52](https://github.com/cloudwego/eino/issues/52), [#246](https://github.com/cloudwego/eino/issues/246), [#255](https://github.com/cloudwego/eino/issues/255), [#299](https://github.com/cloudwego/eino/issues/299), [#364](https://github.com/cloudwego/eino/issues/364), [#381](https://github.com/cloudwego/eino/issues/381), [#467](https://github.com/cloudwego/eino/issues/467), [#476](https://github.com/cloudwego/eino/issues/476), [#487](https://github.com/cloudwego/eino/issues/487), [#644](https://github.com/cloudwego/eino/issues/644), [#706](https://github.com/cloudwego/eino/issues/706), [#948](https://github.com/cloudwego/eino/issues/948)). Eino pinned an old version to keep its Go 1.18 floor. A user wrote: *"This renders Eino not usable in highly regulated environments"* ([#369](https://github.com/cloudwego/eino/issues/369)). The eventual fix was a breaking migration to JSON Schema in v0.6 ([disc#397](https://github.com/cloudwego/eino/discussions/397), [disc#550](https://github.com/cloudwego/eino/discussions/550)).
- **The Go 1.18 floor misleads users.** Core claims 1.18, but every useful ext module needs 1.23 or later. *"If you can't call an AI without ext, the 1.18 in eino's docs is empty talk"* ([ext#339](https://github.com/cloudwego/eino-ext/issues/339)). A `sonic` dependency broke builds on new Go releases ([#188](https://github.com/cloudwego/eino/issues/188), [#801](https://github.com/cloudwego/eino/issues/801), [ext#997](https://github.com/cloudwego/eino-ext/issues/997)).
- **Multi-module eino-ext** confuses private proxies and pseudo-versions ([ext#145](https://github.com/cloudwego/eino-ext/issues/145)).
- A `gob` registration of `map[string]any` panicked at `init` alongside `go-openapi/spec` ([#1120](https://github.com/cloudwego/eino/issues/1120)).

### 3.6 Tooling that users want but can't get

The Eino Dev IDE plugin (visual graph editor and debugger) generated long threads ([#109](https://github.com/cloudwego/eino/issues/109), [ext#133](https://github.com/cloudwego/eino-ext/issues/133), [ext#248](https://github.com/cloudwego/eino-ext/issues/248)). It is closed-source, IDE-only ("less demand for Web, no plan", [#109](https://github.com/cloudwego/eino/issues/109)), and it breaks often ([#727](https://github.com/cloudwego/eino/issues/727)). Users are now asking whether it is maintained at all ([#977](https://github.com/cloudwego/eino/issues/977)), and whether it can debug ADK agents rather than only graphs ([#1046](https://github.com/cloudwego/eino/issues/1046)).

---

## 4. Big problems

### 4.1 Streaming tool-call assembly is fragile across providers

Each provider or gateway emits tool-call deltas differently, and Eino's concat logic assumes OpenAI-shaped chunks:

- Deltas **without `index`** cause each fragment to become a separate ToolCall ([#631](https://github.com/cloudwego/eino/issues/631), open). The maintainer's answer was *"adjust it in your ChatModel implementation."*
- **The same `index` with different IDs** across consecutive calls triggers `cannot concat ToolCalls with different tool id` ([#927](https://github.com/cloudwego/eino/issues/927), open). Users are running forks via `replace` directives or wrapping models to rewrite indexes. Retry mid-stream reportedly produces the same error ([#1059](https://github.com/cloudwego/eino/issues/1059), which the maintainer could not reproduce).
- **Empty tool arguments** are sent back as a missing `arguments` field, so providers return 400 on the next turn ([#493](https://github.com/cloudwego/eino/issues/493), [#384](https://github.com/cloudwego/eino/issues/384), [#196](https://github.com/cloudwego/eino/issues/196)). The community workaround is to default to `"{}"`.
- **Empty tool-result content** is omitted during serialization, which causes a provider 500 ([#815](https://github.com/cloudwego/eino/issues/815)).
- "tool not found in toolsNode indexes" when streaming ([#217](https://github.com/cloudwego/eino/issues/217), [#296](https://github.com/cloudwego/eino/issues/296), [#348](https://github.com/cloudwego/eino/issues/348)).

**Why it matters:** every OpenAI-compatible gateway (vLLM, sglang, Ollama, LM Studio, DeepSeek's Anthropic endpoint) is its own dialect. Failures show up at the framework layer, and users blame the framework.

### 4.2 Provider compatibility is a treadmill

- Reasoning content: first recommended as a custom `Extra` key "until reasoning_content becomes an industry norm" ([#54](https://github.com/cloudwego/eino/issues/54)), later made first-class. Round-tripping still broke. The OpenAI adapter dropped `reasoning_content` on assistant tool-call messages, and Kimi then returned 400 ([ext#700](https://github.com/cloudwego/eino-ext/issues/700)). Gemini 3 requires `thought_signature` echoes ([ext#550](https://github.com/cloudwego/eino-ext/issues/550), [#621](https://github.com/cloudwego/eino/issues/621)). vLLM thinking output parses badly ([#1136](https://github.com/cloudwego/eino/issues/1136)).
- Thinking on/off flags differ per vendor (`enable_thinking` and similar). They work through `ExtraFields` for some backends and silently don't for others (Ollama in [#538](https://github.com/cloudwego/eino/issues/538)).
- DeepSeek's Anthropic-protocol endpoint drops tool results in parallel calls ([#1022](https://github.com/cloudwego/eino/issues/1022)). Native Ollama tool calling breaks supervisor transfers, while Ollama's OpenAI-compatible mode works ([#881](https://github.com/cloudwego/eino/issues/881), [#786](https://github.com/cloudwego/eino/issues/786)).
- The third-party `go-openai` SSE parser broke on `data:` without a trailing space. The maintainers forked the library ([ext#98](https://github.com/cloudwego/eino-ext/issues/98)). Gemini's old SDK was deprecated for months before migration ([ext#291](https://github.com/cloudwego/eino-ext/issues/291)).
- The maintainers concluded that pointing the OpenAI component at a custom `BaseURL` is not enough, and are adding first-class GLM, Kimi, MiniMax, and Grok modules ([ext#718](https://github.com/cloudwego/eino-ext/issues/718)).

### 4.3 Tool errors kill the run

- *"ReAct agent doesn't attempt to recover from a failed tool call"* ([#258](https://github.com/cloudwego/eino/issues/258)). The maintainer argued that treating tool errors as observations "is not universal" and that users should wrap tools.
- MCP tool errors never reach the LLM, which contradicts the MCP spec ([ext#436](https://github.com/cloudwego/eino-ext/issues/436)).
- The built-in skill tool crashes the agent when the model hallucinates a skill name ([#928](https://github.com/cloudwego/eino/issues/928)). The answer was "developer decides; add middleware."
- There is an RFC for a structured tool-error format ([#1010](https://github.com/cloudwego/eino/issues/1010)).

This confirms `eino.md` §8.2.5. Users keep hitting it, including through Eino's *own* built-in tools.

### 4.4 Durability: interrupt-only checkpoints, gob, and nesting

- Checkpoints are saved only at interrupts. A request for per-node auto-checkpointing was declined: it "conflicts with DAG concurrency" and "products expose rollback points explicitly" ([#347](https://github.com/cloudwego/eino/issues/347)). Node inputs aren't saved on `InterruptAndRerun` "by design" ([#363](https://github.com/cloudwego/eino/issues/363)). This confirms `eino.md` §8.2.7.
- An interrupt pauses the **whole** graph. Independent branches cannot keep running, and the maintainer said *"this really can't be done"* ([#523](https://github.com/cloudwego/eino/issues/523), 17 comments).
- There is only one checkpoint per ID, so a user cannot roll back to an earlier interrupt ([#683](https://github.com/cloudwego/eino/issues/683)). There is no TTL or cleanup for abandoned checkpoints ([#598](https://github.com/cloudwego/eino/issues/598), [#870](https://github.com/cloudwego/eino/issues/870)).
- **gob fragility:** users hit `gob: type not registered for interface: []*schema.Message` and then `planexecute.defaultPlan`, a prebuilt type users cannot register themselves ([#554](https://github.com/cloudwego/eino/issues/554)). An unexported `compose.internalError` in the event history made later checkpoint saves fail ([#782](https://github.com/cloudwego/eino/issues/782)). Array deserialization panics ([#607](https://github.com/cloudwego/eino/issues/607)). Resume panics on a corrupted checkpoint instead of returning an error ([#1246](https://github.com/cloudwego/eino/issues/1246)). All of this confirms `eino.md` §8.2.6.
- Cancel races: an accepted `CancelImmediate` sometimes produced no checkpoint. The reporter reproduced it reliably with `go test -count=1000` against a local SSE provider ([#1148](https://github.com/cloudwego/eino/issues/1148)). Closing the Eino stream does **not** cancel the provider HTTP request, so transports leak ([#1148](https://github.com/cloudwego/eino/issues/1148), [#1068](https://github.com/cloudwego/eino/issues/1068)).
- Resume addresses depend on model-generated tool-call IDs, which can be empty or duplicated ([#1290](https://github.com/cloudwego/eino/issues/1290)). This confirms the `eino.md` §8.3 recommendation to resume by call ID with framework-controlled uniqueness.

### 4.5 Concurrency and lifecycle bugs

These come from the channel-based streams, shared mutable config, and goroutine-per-stage design:

- A shared `ChatModelAgent` reused across concurrent runs races on per-run cancel state ([#1177](https://github.com/cloudwego/eino/issues/1177)).
- The OpenAI adapter sorts the shared tool schema's `required` slice in place, which gives intermittent 400 "non-unique elements" under concurrency ([ext#1005](https://github.com/cloudwego/eino-ext/issues/1005)). `Message.Format` mutates template parts ([#1318](https://github.com/cloudwego/eino/issues/1318)). The Claude stream races on a named return ([ext#1004](https://github.com/cloudwego/eino-ext/issues/1004)).
- `send on closed channel` panics after cancellation in agent-as-tool ([#1236](https://github.com/cloudwego/eino/issues/1236)). The sibling bug fixed in PR #929 had "the same panic shape".
- `ToolsNode.Stream` leaks sibling streams when one tool fails ([#1256](https://github.com/cloudwego/eino/issues/1256)). A sub-agent `Stream()` never returns ([#1137](https://github.com/cloudwego/eino/issues/1137)). TurnLoop hangs after a tool-level resume ([#1119](https://github.com/cloudwego/eino/issues/1119)).
- A user proposed iterator-based streams instead of channels, citing pre-allocation, extra goroutines, and leaks when consumers don't drain ([#733](https://github.com/cloudwego/eino/issues/733)). The maintainer answered: *"use StreamReaderWithConvert; close it; if you don't want to, use SetAutomaticClose."* That is, finalizers. This confirms `eino.md` §8.2.8.

### 4.6 Memory and conversation persistence were missing for 18 months

- In 2025, users asked how to persist history *including tool calls* ([#113](https://github.com/cloudwego/eino/issues/113)), for memory hooks ([#203](https://github.com/cloudwego/eino/issues/203)), for a persistence API "like Spring AI and LangChain have" ([#365](https://github.com/cloudwego/eino/issues/365)), for multi-replica state ([#393](https://github.com/cloudwego/eino/issues/393), answer: "persist state yourself"), and for short- and long-term memory ([#524](https://github.com/cloudwego/eino/issues/524)).
- The maintainer position in [#524](https://github.com/cloudwego/eino/issues/524) was that memory is "strongly business-specific engineering, so no plan to provide a component abstraction". It was then softened: "we will support short-term memory in adk". In March 2026 a user asked about Google ADK-style memory and got *"计划赶不上变化"* ("plans can't keep up with change", [#858](https://github.com/cloudwego/eino/issues/858)).
- v0.10 (alpha) finally adds `SessionStore`, an append-only session event log, replayable middleware state, and `automemory` ([disc#1159](https://github.com/cloudwego/eino/discussions/1159)). Early users already report that `automemory` injects topic memory only once per session, so topic switches get stale memory ([#1130](https://github.com/cloudwego/eino/issues/1130)). Summarization can itself overflow the summarizer's context window ([#940](https://github.com/cloudwego/eino/issues/940)).

This **contradicts** part of `eino.md` §3 ("There is no built-in cross-run memory"). That statement is true of the analyzed commit's stable surface, but the v0.10 alpha line is adding exactly this, and it adopts the append-only event log that `eino.md` §8.2.7 recommends for Dive.

### 4.7 Multi-agent semantics surprise people

- The supervisor shares all sub-agent context with the parent, which defeats cost offload ([#567](https://github.com/cloudwego/eino/issues/567)). Transfer messages are injected as *user* input ("For context: [supervisor] called tool transfer_to_agent…", [#690](https://github.com/cloudwego/eino/issues/690)).
- A sub-agent error still lets the supervisor transfer to the next sub-agent ([#782](https://github.com/cloudwego/eino/issues/782)). This is "not expected", and it is still open.
- Users want handoff graphs where any agent can jump back to any other and hold a multi-turn conversation ([#624](https://github.com/cloudwego/eino/issues/624), [#1291](https://github.com/cloudwego/eino/issues/1291)). The maintainer's answers (`BreakLoopAction`, interrupt/resume) are workarounds.

---

## 5. Most desired features

| Rank | Request | Evidence | Maintainer response |
|---|---|---|---|
| 1 | **Streaming that just works with tool calls and reasoning** | [#806](https://github.com/cloudwego/eino/issues/806), [#477](https://github.com/cloudwego/eino/issues/477), [#445](https://github.com/cloudwego/eino/issues/445), [#306](https://github.com/cloudwego/eino/issues/306), [#892](https://github.com/cloudwego/eino/issues/892) and 25+ related | Redirected to ADK's event stream. The legacy default checker is still first-chunk-only ([#892](https://github.com/cloudwego/eino/issues/892) open, community PR offered). |
| 2 | **Human-in-the-loop / interrupt / approval** | [#58](https://github.com/cloudwego/eino/issues/58), [#68](https://github.com/cloudwego/eino/issues/68), [#456](https://github.com/cloudwego/eino/issues/456), [#996](https://github.com/cloudwego/eino/issues/996) (highest-reacted issue) | Delivered in v0.7 ([disc#527](https://github.com/cloudwego/eino/discussions/527)), with HITL patterns kept in examples, "not yet finalized into core". A tool-policy + approval middleware ([#996](https://github.com/cloudwego/eino/issues/996)) converged on a "thin hook": framework owns the seam, users own policy storage and audit. A community PR is in progress. |
| 3 | **Memory / session persistence** | [#113](https://github.com/cloudwego/eino/issues/113), [#203](https://github.com/cloudwego/eino/issues/203), [#365](https://github.com/cloudwego/eino/issues/365), [#393](https://github.com/cloudwego/eino/issues/393), [#524](https://github.com/cloudwego/eino/issues/524), [#728](https://github.com/cloudwego/eino/issues/728), [#858](https://github.com/cloudwego/eino/issues/858), [#995](https://github.com/cloudwego/eino/issues/995) | Reversed from "no plan" to v0.10 SessionStore + automemory ([disc#1159](https://github.com/cloudwego/eino/discussions/1159)). |
| 4 | **Per-request model parameters and structured output** | [#350](https://github.com/cloudwego/eino/issues/350), [#883](https://github.com/cloudwego/eino/issues/883), [#108](https://github.com/cloudwego/eino/issues/108), [#538](https://github.com/cloudwego/eino/issues/538), [#652](https://github.com/cloudwego/eino/issues/652), [ext#265](https://github.com/cloudwego/eino-ext/issues/265) | Initially: "if every call's params differ, why share a model instance?" ([#350](https://github.com/cloudwego/eino/issues/350)). Settled on a per-provider `WithExtraFields` escape hatch. No provider-neutral `ResponseFormat` / JSON-schema option ([#883](https://github.com/cloudwego/eino/issues/883) still open, 4 reactions). |
| 5 | **Responses API / content-block messages** | [#461](https://github.com/cloudwego/eino/issues/461), [#1023](https://github.com/cloudwego/eino/issues/1023), [#541](https://github.com/cloudwego/eino/issues/541) | Delivered as `AgenticMessage` / `AgenticModel` in v0.9 ([disc#710](https://github.com/cloudwego/eino/discussions/710)). ADK integration is still pending ([#877](https://github.com/cloudwego/eino/issues/877)). |
| 6 | **Dynamic tools / tool search** | [#189](https://github.com/cloudwego/eino/issues/189), [#215](https://github.com/cloudwego/eino/issues/215), [#502](https://github.com/cloudwego/eino/issues/502), [#664](https://github.com/cloudwego/eino/issues/664), [#848](https://github.com/cloudwego/eino/issues/848), [#1187](https://github.com/cloudwego/eino/issues/1187) | Early answer: "split into multiple agents" ([#189](https://github.com/cloudwego/eino/issues/189)). Later: `model.WithTools` per call, a toolsearch middleware (PR #725, [#862](https://github.com/cloudwego/eino/issues/862)), and a PR for dynamic return-directly ([#1187](https://github.com/cloudwego/eino/issues/1187)). |
| 7 | **Skills (Claude-style)** | [#653](https://github.com/cloudwego/eino/issues/653) (5 reactions), [#709](https://github.com/cloudwego/eino/issues/709), [#716](https://github.com/cloudwego/eino/issues/716), [#816](https://github.com/cloudwego/eino/issues/816), [#981](https://github.com/cloudwego/eino/issues/981), [#1043](https://github.com/cloudwego/eino/issues/1043) | Delivered as middleware. Follow-ups include preload ([#867](https://github.com/cloudwego/eino/issues/867)) and "skill output leaks into the answer" ([#1043](https://github.com/cloudwego/eino/issues/1043)). |
| 8 | **Observability: OTel, trace IDs, input/output on traces** | [#1028](https://github.com/cloudwego/eino/issues/1028), [#997](https://github.com/cloudwego/eino/issues/997), [#603](https://github.com/cloudwego/eino/issues/603), [#159](https://github.com/cloudwego/eino/issues/159), [ext#128](https://github.com/cloudwego/eino-ext/issues/128), [ext#533](https://github.com/cloudwego/eino-ext/issues/533) | The maintainer first said a custom trace ID "is usually useless", then added `InitTrace(WithID)` after the user explained their service-mesh setup ([ext#128](https://github.com/cloudwego/eino-ext/issues/128)). OTel GenAI semconv is being built by the community in ext ([ext#980](https://github.com/cloudwego/eino-ext/pull/980)). |
| 9 | **JSON/DSL-defined workflows** (Dify/Coze-style) | [#127](https://github.com/cloudwego/eino/issues/127) (15 comments, open since 2025-03) | "Exploring a DSL; no bandwidth short-term." Still open. |
| 10 | **Testing support** | [#1029](https://github.com/cloudwego/eino/issues/1029) | The only mock is in `internal/`. A community PR adds `modeltest`. |
| — | Smaller but telling | Mid-run message injection ([#947](https://github.com/cloudwego/eino/issues/947)), retry for agent-as-tool ([#889](https://github.com/cloudwego/eino/issues/889)), per-node timeout and fallback ([#625](https://github.com/cloudwego/eino/issues/625)), cache-write token accounting ([#1150](https://github.com/cloudwego/eino/issues/1150)), message timestamps ([#888](https://github.com/cloudwego/eino/issues/888)), sandbox ([#1042](https://github.com/cloudwego/eino/issues/1042)), A2A and AG-UI ([#865](https://github.com/cloudwego/eino/issues/865), [#866](https://github.com/cloudwego/eino/issues/866)), Bedrock ([#937](https://github.com/cloudwego/eino/issues/937)) | Mostly accepted or in community PRs. |

---

## 6. Maintainer stance and trajectory

1. **"Mechanism, not policy", then gradual retreat.** The early default answer was that the framework provides primitives and users build the rest. Memory is business logic ([#524](https://github.com/cloudwego/eino/issues/524), [#365](https://github.com/cloudwego/eino/issues/365)). Tool-error handling is the developer's call ([#258](https://github.com/cloudwego/eino/issues/258), [#928](https://github.com/cloudwego/eino/issues/928)). Graph-level context types would "reduce applicability without real convenience" ([#261](https://github.com/cloudwego/eino/issues/261)). Model listing is platform-specific ([#571](https://github.com/cloudwego/eino/issues/571)). Over 2026 the team has shipped more opinionated batteries: HITL patterns, skills, summarization, reduction, filesystem, a deep agent, `automemory`, a background task manager, and tool policy under discussion. **Trajectory: from a composable-components toolkit toward a Claude Code-style agent harness.** The v0.10 notes read almost like a Claude Code architecture list ([disc#1159](https://github.com/cloudwego/eino/discussions/1159)).
2. **ADK over graphs for agents; graphs kept for workflows.** Users are steered to ADK's event stream ([#445](https://github.com/cloudwego/eino/issues/445), [#806](https://github.com/cloudwego/eino/issues/806)). `host` multi-agent is frozen apart from bug fixes ([#499](https://github.com/cloudwego/eino/issues/499)). Legacy `flow/react` will not be removed ([#445](https://github.com/cloudwego/eino/issues/445)).
3. **Agents-as-tools over transfer.** Context sharing versus isolation is framed as a trade-off ([#567](https://github.com/cloudwego/eino/issues/567)), but the code comments deprecate transfer (see `eino.md` §1), and the community has absorbed that ([#1291](https://github.com/cloudwego/eino/issues/1291)).
4. **Conservative about non-standard provider fields, then pragmatic.** Reasoning content was "wait for industry consensus" ([#54](https://github.com/cloudwego/eino/issues/54)) and is now first-class. They decided *not* to migrate to the official `openai-go` SDK, because the OpenAI-compatible ecosystem depends on `reasoning_content`, which the official SDK won't type ([ext#534](https://github.com/cloudwego/eino-ext/issues/534)). They are instead adding per-vendor modules ([ext#718](https://github.com/cloudwego/eino-ext/issues/718)).
5. **Breaking changes are acceptable pre-1.0.** Examples: removing `GetState` ([disc#323](https://github.com/cloudwego/eino/discussions/323)), JSON Schema replacing OpenAPI in v0.6 ([disc#397](https://github.com/cloudwego/eino/discussions/397)), the Message multimodal refactor ([disc#471](https://github.com/cloudwego/eino/discussions/471)), `AgenticMessage` in v0.9, and v0.10 alpha "expect many breaking changes". Some naming mistakes are nonetheless declared too late to fix ([#700](https://github.com/cloudwego/eino/issues/700)).
6. **Triage style.** Maintainers are fast and terse. They often answer "can't reproduce", "provide a minimal case" (the `E-needs-mvce` label), or "this is a model problem" ([#789](https://github.com/cloudwego/eino/issues/789), [#804](https://github.com/cloudwego/eino/issues/804), [#859](https://github.com/cloudwego/eino/issues/859), [#1059](https://github.com/cloudwego/eino/issues/1059)). Many "how do I" issues close with a one-line API pointer. That is efficient, but it is also why users feel the docs don't answer their questions.
7. **The community increasingly implements proposals.** Since mid-2026, external contributors turn feature requests into PRs within days: modeltest ([#1029](https://github.com/cloudwego/eino/issues/1029)), agent-tool retry ([#889](https://github.com/cloudwego/eino/issues/889)), OTel ([#1028](https://github.com/cloudwego/eino/issues/1028)), dynamic return-directly ([#1187](https://github.com/cloudwego/eino/issues/1187)), tool policy ([#996](https://github.com/cloudwego/eino/issues/996)), and the cancel/checkpoint race ([#1148](https://github.com/cloudwego/eino/issues/1148)). The core team sets direction and reviews.
8. **Explicit "won't do" list:**
   - partial-branch interrupt ([#523](https://github.com/cloudwego/eino/issues/523))
   - per-node auto-checkpoint ([#347](https://github.com/cloudwego/eino/issues/347))
   - saving inputs on rerun ([#363](https://github.com/cloudwego/eino/issues/363))
   - a web-based Eino Dev ([#109](https://github.com/cloudwego/eino/issues/109))
   - a unified model-list API ([#571](https://github.com/cloudwego/eino/issues/571))
   - multi-parent agent nodes ([#624](https://github.com/cloudwego/eino/issues/624), "not sure it's a real need")
   - iterator-based streams ([#733](https://github.com/cloudwego/eino/issues/733))

---

## 7. Lessons for Dive

Items are ordered by impact on users, judged by how much pain each theme caused in Eino's tracker.

### P0: get these right from day one

1. **Streaming is the primary API, and it never needs a "checker".** The loop must consume the model's full stream while forwarding every delta (text, reasoning, tool-call argument deltas) as events in real time. It decides "tool calls or done" at end of stream. Text → tool → text interleaving is the normal case, not an edge case. This is Eino's number-one complaint cluster ([#806](https://github.com/cloudwego/eino/issues/806), [#477](https://github.com/cloudwego/eino/issues/477), [#445](https://github.com/cloudwego/eino/issues/445), [#892](https://github.com/cloudwego/eino/issues/892), [#306](https://github.com/cloudwego/eino/issues/306)). Middleware must never silently turn a stream into a blocking call ([#1064](https://github.com/cloudwego/eino/issues/1064)). The plain-Go loop recommended in `eino.md` §8.2.2 solves this structurally.
2. **Provider stream normalization with a conformance suite.** Build a tool-call delta assembler that tolerates:
   - missing `index` (key by ID, then position)
   - a reused index with a new ID (start a new call)
   - empty arguments (normalize to `{}` outbound)
   - empty tool-result content
   - reasoning content and signatures that must round-trip on the next request
   - parallel calls whose results must all be returned

   Add recorded-fixture tests for OpenAI, Anthropic, Gemini, DeepSeek, Qwen/vLLM, Ollama native, Ollama-OpenAI, and OpenRouter. Evidence: [#631](https://github.com/cloudwego/eino/issues/631), [#927](https://github.com/cloudwego/eino/issues/927), [#493](https://github.com/cloudwego/eino/issues/493), [#815](https://github.com/cloudwego/eino/issues/815), [#1022](https://github.com/cloudwego/eino/issues/1022), [ext#700](https://github.com/cloudwego/eino-ext/issues/700), [ext#550](https://github.com/cloudwego/eino-ext/issues/550). Own the SSE parser, or use official SDKs that parse `data:` correctly ([ext#98](https://github.com/cloudwego/eino-ext/issues/98)).
3. **Tool errors become `is_error` results by default,** including for MCP and Dive's own built-in tools. Make aborting the run the opt-in behavior ([#258](https://github.com/cloudwego/eino/issues/258), [#928](https://github.com/cloudwego/eino/issues/928), [#1010](https://github.com/cloudwego/eino/issues/1010), [ext#436](https://github.com/cloudwego/eino-ext/issues/436)). This confirms `eino.md` §8.2.5.
4. **Cancellation is real cancellation.** When the consumer stops reading or `ctx` is cancelled, the provider HTTP request must be aborted and every goroutine must exit. Test this with a fake SSE server under `-race -count=1000`, which is how [#1148](https://github.com/cloudwego/eino/issues/1148) was found. Provide a "stop" that yields a resumable state, because users click stop and want to continue or restart ([#456](https://github.com/cloudwego/eino/issues/456), [#641](https://github.com/cloudwego/eino/issues/641), [#231](https://github.com/cloudwego/eino/issues/231)).
5. **Immutable shared objects.** Agents, models, and tool schemas are shared across concurrent requests. Never mutate them per run. Per-run state lives in a per-run struct ([#1177](https://github.com/cloudwego/eino/issues/1177), [ext#1005](https://github.com/cloudwego/eino-ext/issues/1005), [#1318](https://github.com/cloudwego/eino/issues/1318)). Put a concurrent-reuse test in CI for every provider.

### P1: the differentiators users begged Eino for

6. **Session history and memory in the box.** Provide a `SessionStore` interface with an append-only JSON event log (messages, tool calls, tool results, compaction/summary replacements) and a Postgres reference implementation. It must persist tool calls and results, not just user and assistant turns ([#113](https://github.com/cloudwego/eino/issues/113), [#728](https://github.com/cloudwego/eino/issues/728)). Eino took 18 months and a reversal to get here ([#524](https://github.com/cloudwego/eino/issues/524) → [disc#1159](https://github.com/cloudwego/eino/discussions/1159)). Keep long-term memory as a pluggable middleware, and re-select memory per turn, not once per session ([#1130](https://github.com/cloudwego/eino/issues/1130)).
7. **First-class tool approval policy.** Build `Allow / Deny / RequireApproval(+rewrite args)` evaluated *before* tool execution, persisted as a pending call, and resumed by call ID. The approval must bind to the exact arguments ([#996](https://github.com/cloudwego/eino/issues/996), whose thread already contains a good spec, including "resume must satisfy the same envelope"). Also support checkpoint TTL and deletion ([#598](https://github.com/cloudwego/eino/issues/598), [#870](https://github.com/cloudwego/eino/issues/870)) and resuming an earlier pending step ([#683](https://github.com/cloudwego/eino/issues/683)). This confirms `eino.md` §8.2.11.
8. **JSON, versioned, never gob.** Every durable blob must round-trip with no user registration step, including built-in agent types and error values ([#554](https://github.com/cloudwego/eino/issues/554), [#782](https://github.com/cloudwego/eino/issues/782), [#1120](https://github.com/cloudwego/eino/issues/1120), [#1246](https://github.com/cloudwego/eino/issues/1246)). A corrupt blob returns an error; it never panics.
9. **Per-request everything.** Tools, tool choice, response format / JSON schema, thinking on/off and budget, and a provider-specific `Extra` map all go on the request, without threading options through wrapper layers ([#350](https://github.com/cloudwego/eino/issues/350), [#883](https://github.com/cloudwego/eino/issues/883), [#538](https://github.com/cloudwego/eino/issues/538), [ext#265](https://github.com/cloudwego/eino-ext/issues/265)). Provider-neutral structured output is an easy win that Eino still lacks.
10. **Explicit, non-ambient observability.** Hooks and tracers attach to an agent or run explicitly. They must not leak through `ctx` into nested model calls made by tools ([#517](https://github.com/cloudwego/eino/issues/517)). They must survive resume ([#859](https://github.com/cloudwego/eino/issues/859)) and never double-fire ([#174](https://github.com/cloudwego/eino/issues/174)). Ship an OTel GenAI-semconv exporter and allow caller-supplied trace IDs ([#1028](https://github.com/cloudwego/eino/issues/1028), [ext#128](https://github.com/cloudwego/eino-ext/issues/128)). Usage should include cache-write tokens ([#1150](https://github.com/cloudwego/eino/issues/1150)).

### P2: ergonomics that compound

11. **One way to do each thing, and docs that are runnable examples.** Eino's two agent stacks and "only works through Runner" options generated a large share of support load ([#843](https://github.com/cloudwego/eino/issues/843), [#615](https://github.com/cloudwego/eino/issues/615), [#215](https://github.com/cloudwego/eino/issues/215)). Ship an `AGENTS.md` / `llms.txt`, and check links in CI ([#616](https://github.com/cloudwego/eino/issues/616), [#715](https://github.com/cloudwego/eino/issues/715), [#1212](https://github.com/cloudwego/eino/issues/1212)). Review names before 1.0, because Eino says it's "too late to change now" ([#700](https://github.com/cloudwego/eino/issues/700)).
12. **Public scripted mock model** for user tests ([#1029](https://github.com/cloudwego/eino/issues/1029)).
13. **Minimal core dependencies and an honest Go floor.** The Go version core claims should match what a working "hello model" needs. Avoid dependencies that force pins or carry CVEs ([#369](https://github.com/cloudwego/eino/issues/369), [ext#339](https://github.com/cloudwego/eino-ext/issues/339)). Tag every module ([ext#145](https://github.com/cloudwego/eino-ext/issues/145)).
14. **Dynamic tools and loop control from inside tools.** Support a per-turn tool set (via hook), tool search / deferred tools, and a tool result that can say "stop and return this" ([#848](https://github.com/cloudwego/eino/issues/848), [#664](https://github.com/cloudwego/eino/issues/664), [#1187](https://github.com/cloudwego/eino/issues/1187)).
15. **Agent-as-tool with isolated context by default,** plus retry and partial-failure tolerance for parallel sub-agents ([#567](https://github.com/cloudwego/eino/issues/567), [#889](https://github.com/cloudwego/eino/issues/889)). A sub-agent error must stop or surface to the parent deterministically ([#782](https://github.com/cloudwego/eino/issues/782)).
16. **Mid-run steering.** Let the caller push a user message into a running session, to be consumed at the next safe point ([#947](https://github.com/cloudwego/eino/issues/947)). Eino's TurnLoop is heading there, and it is a natural fit for a session actor.
17. **Templating is opt-in** ([#685](https://github.com/cloudwego/eino/issues/685)).

**Bottom line:** the tracker validates `eino.md`'s structural critique (graph-on-a-loop, gob, abort-on-tool-error, finalizer streams). It also adds two things the source analysis underweights. First, **provider-dialect normalization for streamed tool calls** is where users actually bleed. Second, **session and memory persistence** was the top missing feature for over a year. A Dive that streams cleanly across messy OpenAI-compatible backends, keeps runs alive through tool errors, and ships a JSON session log with approvals would answer the majority of Eino's highest-engagement threads.

---

## 8. Appendix: high-signal issue index

R = total reactions, C = comments (at snapshot). Titles are translated where needed.

| # | Title | State | R | C | Theme | Takeaway |
|---|---|---|---|---|---|---|
| [806](https://github.com/cloudwego/eino/issues/806) | "Fatal": streaming tool-call detection | closed | 0 | 18 | Streaming | Text-before-tool breaks react; user threatens to return to LangChain. Fix required a goroutine ordering trick. |
| [120](https://github.com/cloudwego/eino/issues/120) | ReAct Stream method unusable | closed | 0 | 17 | Streaming | Streams returned EOF; root cause was a backend (sglang) returning JSON instead of SSE with tools. |
| [613](https://github.com/cloudwego/eino/issues/613) | DeepSeek react agent can't call tools when streaming | closed | 0 | 12 | Streaming | Same checker problem; "the docs example is too complex". |
| [477](https://github.com/cloudwego/eino/issues/477) | StreamToolCallChecker makes output arrive only at end | closed | 1 | 8 | Streaming | Documented fix kills streaming; real fix undocumented. |
| [445](https://github.com/cloudwego/eino/issues/445) | Support text → tool call → text | closed | 1 | 5 | Streaming | Maintainer: rely on events or callbacks, not final output. |
| [892](https://github.com/cloudwego/eino/issues/892) | Default checker fails when text precedes tool calls | open | 0 | 2 | Streaming | First-chunk-only default is still shipping. |
| [306](https://github.com/cloudwego/eino/issues/306) | reasoning_content can't be streamed | closed | 0 | 9 | Streaming / reasoning | Reasoning available only via callbacks. |
| [1064](https://github.com/cloudwego/eino/issues/1064) | Reduction middleware blocks streaming tools | closed | 0 | 0 | Streaming | Middleware silently de-streams. |
| [631](https://github.com/cloudwego/eino/issues/631) | How to merge streamed function-call args (no index) | open | 0 | 12 | Provider compat | "Fix it in your ChatModel." |
| [927](https://github.com/cloudwego/eino/issues/927) | cannot concat ToolCalls with different tool id | open | 0 | 5 | Provider compat | Users run forks via `replace`. |
| [493](https://github.com/cloudwego/eino/issues/493) | ADK tool call with empty args errors | open | 0 | 6 | Provider compat | Need `{}` normalization. |
| [1022](https://github.com/cloudwego/eino/issues/1022) | Parallel tool calls not all returned (DeepSeek via Anthropic API) | closed | 0 | 6 | Provider compat | Gateway dialect bug surfaces as a framework bug. |
| [350](https://github.com/cloudwego/eino/issues/350) | No flexible per-request model params | closed | 1 | 10 | API ergonomics | Resolved by `WithExtraFields` escape hatch. |
| [883](https://github.com/cloudwego/eino/issues/883) | Better support for JSON format | open | 4 | 0 | Structured output | No provider-neutral JSON-schema output. |
| [461](https://github.com/cloudwego/eino/issues/461) | Support Responses API? | open | 2 | 7 | Messages | Led to AgenticMessage (v0.9). |
| [54](https://github.com/cloudwego/eino/issues/54) | Expose chain-of-thought in message | closed | 0 | 5 | Reasoning | "Wait for industry norm", later reversed. |
| [538](https://github.com/cloudwego/eino/issues/538) | How to use non-thinking mode | open | 0 | 5 | Provider compat | Thinking flags vary; ExtraFields unreliable. |
| [258](https://github.com/cloudwego/eino/issues/258) | ReAct doesn't recover from failed tool call | closed | 0 | 6 | Tool errors | Maintainer: aborting is valid; wrap tools yourself. |
| [928](https://github.com/cloudwego/eino/issues/928) | Skill tool Go error crashes agent | closed | 0 | 4 | Tool errors | Even built-ins abort the run. |
| [1010](https://github.com/cloudwego/eino/issues/1010) | RFC: structured tool error format | open | 0 | 1 | Tool errors | Demand for LLM-actionable errors. |
| [70](https://github.com/cloudwego/eino/issues/70) | Concurrent function call surprises | closed | 0 | 8 | Tools | Parallel tool calls with side effects confuse users. |
| [58](https://github.com/cloudwego/eino/issues/58) | Interactive Q&A node (HITL) | closed | 3 | 6 | HITL | Early answer "use two graphs" was rejected by users. |
| [996](https://github.com/cloudwego/eino/issues/996) | Standard tool policy + human approval middleware | open | 6 | 9 | HITL / governance | Highest-reacted issue; thin-hook consensus. |
| [523](https://github.com/cloudwego/eino/issues/523) | Branch-local pause in workflows | closed | 0 | 17 | Interrupts | Interrupt is global to the graph, by design. |
| [456](https://github.com/cloudwego/eino/issues/456) | Set breakpoints at runtime / external stop | open | 0 | 6 | Interrupts | External pause+save wanted; "next version". |
| [347](https://github.com/cloudwego/eino/issues/347) | Auto-checkpoint before/after every node | open | 0 | 4 | Durability | Declined: conflicts with DAG concurrency. |
| [363](https://github.com/cloudwego/eino/issues/363) | Why is input not saved on rerun? | closed | 0 | 7 | Durability | By design; save to state yourself. |
| [683](https://github.com/cloudwego/eino/issues/683) | Resume from an earlier of several interrupts | open | 0 | 4 | Durability | One checkpoint per ID; no history. |
| [598](https://github.com/cloudwego/eino/issues/598) | Resume timeout / checkpoint cleanup | closed | 0 | 4 | Durability | No TTL or cleanup callback. |
| [554](https://github.com/cloudwego/eino/issues/554) | failed to save checkpoint (gob not registered) | closed | 0 | 3 | Serialization | Prebuilt types unregistrable by users. |
| [782](https://github.com/cloudwego/eino/issues/782) | Sub-agent internal error breaks later checkpoint | open | 0 | 2 | Serialization / multi-agent | Unexported error in history; also wrong continue-after-error. |
| [1120](https://github.com/cloudwego/eino/issues/1120) | gob registering `map[string]any` panics with go-openapi | closed | 0 | 1 | Serialization | Global registry collides with other libraries. |
| [1148](https://github.com/cloudwego/eino/issues/1148) | CancelImmediate can skip checkpoint | closed | 0 | 5 | Cancellation | Race fixed; provider HTTP not cancelled on close. |
| [1290](https://github.com/cloudwego/eino/issues/1290) | Interrupt address depends on model-generated call ID | open | 0 | 0 | Durability | Need framework-owned call identity. |
| [859](https://github.com/cloudwego/eino/issues/859) | Callbacks don't fire OnEnd after resume | closed | 0 | 10 | Observability | Resume ctx loses callback manager. |
| [517](https://github.com/cloudwego/eino/issues/517) | Model call inside tool leaks into agent events | closed | 0 | 3 | Observability | Ambient ctx callbacks leak. |
| [174](https://github.com/cloudwego/eino/issues/174) | Agent callback fires twice | closed | 0 | 5 | Observability | Callback wiring differs by config field. |
| [1028](https://github.com/cloudwego/eino/issues/1028) | Native OpenTelemetry handler (GenAI semconv) | open | 1 | 3 | Observability | Community building it in ext. |
| [997](https://github.com/cloudwego/eino/issues/997) | Structured AgentEvent types | open | 0 | 2 | Events | Event taxonomy vs session events unresolved. |
| [113](https://github.com/cloudwego/eino/issues/113) | Best way to persist message history? | closed | 0 | 4 | Memory | Tool calls were "internal"; needed callbacks. |
| [365](https://github.com/cloudwego/eino/issues/365) | Message persistence API? | closed | 0 | 3 | Memory | "No plan for now." |
| [524](https://github.com/cloudwego/eino/issues/524) | Short- and long-term memory? | open | 0 | 5 | Memory | "Business-specific" → "will support in adk". |
| [858](https://github.com/cloudwego/eino/issues/858) | Context and long-term memory in ADK | closed | 0 | 3 | Memory | "Plans can't keep up with change." |
| [1130](https://github.com/cloudwego/eino/issues/1130) | automemory injects topic memory only once | open | 0 | 1 | Memory | New v0.10 memory has semantics bugs. |
| [947](https://github.com/cloudwego/eino/issues/947) | Insert messages during agent run | open | 1 | 0 | Steering | Mid-run input is a "very common need". |
| [567](https://github.com/cloudwego/eino/issues/567) | Sub-agent content visible to main agent | closed | 0 | 2 | Multi-agent | Use agent-as-tool for isolation. |
| [690](https://github.com/cloudwego/eino/issues/690) | Transfer process injected as user input | closed | 0 | 2 | Multi-agent | Framework-authored context pollutes prompts. |
| [624](https://github.com/cloudwego/eino/issues/624) | Multi-in/multi-out agent nodes | closed | 0 | 8 | Multi-agent | Handoff graphs wanted; workarounds offered. |
| [881](https://github.com/cloudwego/eino/issues/881) | Supervisor transfer, sub-agent never runs | open | 0 | 2 | Multi-agent / provider | Ollama-native tool format breaks transfer. |
| [889](https://github.com/cloudwego/eino/issues/889) | Retry for AgentAsTool | open | 0 | 3 | Multi-agent | One failed sub-agent kills a parallel batch. |
| [189](https://github.com/cloudwego/eino/issues/189) | Dynamically add or remove tools | closed | 0 | 2 | Dynamic tools | Early answer: "use multiple agents". |
| [664](https://github.com/cloudwego/eino/issues/664) | Lazy-loaded tool discovery | closed | 0 | 2 | Dynamic tools | Led to toolsearch middleware. |
| [1187](https://github.com/cloudwego/eino/issues/1187) | End ReAct loop based on tool result | open | 0 | 4 | Loop control | Users poke deprecated internal state to do it. |
| [653](https://github.com/cloudwego/eino/issues/653) | Plans to support skills? | closed | 5 | 2 | Skills | Among the most-reacted issues; delivered as middleware. |
| [369](https://github.com/cloudwego/eino/issues/369) | Release v2 to fix security issues (kin-openapi) | closed | 4 | 7 | Dependencies | "Not usable in regulated environments" → v0.6 JSON Schema. |
| [614](https://github.com/cloudwego/eino/issues/614) | Good concepts but very hard to debug | closed | 0 | 4 | DX / docs | Split opinion; docs criticized, cookbook added. |
| [700](https://github.com/cloudwego/eino/issues/700) | planexecute naming is sloppy | closed | 0 | 1 | API design | "Hard to change now." |
| [715](https://github.com/cloudwego/eino/issues/715) | AI coding models don't know the framework | closed | 0 | 2 | DX | Led to AGENTS.md and skills roadmap. |
| [543](https://github.com/cloudwego/eino/issues/543) | NewTool not found | closed | 0 | 11 | DX | Import-path confusion. |
| [127](https://github.com/cloudwego/eino/issues/127) | Build workflows from JSON | open | 0 | 15 | DSL | Long-open; no bandwidth. |
| [109](https://github.com/cloudwego/eino/issues/109) | Visual graph editor for VS Code / web | closed | 0 | 16 | Tooling | Closed-source plugin, no web. |
| [132](https://github.com/cloudwego/eino/issues/132) | Custom fan-in merge methods | closed | 0 | 12 | Graph | Community PR added merge registry. |
| [733](https://github.com/cloudwego/eino/issues/733) | Iterator-based streams instead of channels | closed | 0 | 1 | Streams | Declined; use finalizers. |
| [1177](https://github.com/cloudwego/eino/issues/1177) | Data race reusing ChatModelAgent concurrently | open | 0 | 0 | Concurrency | Per-run state in shared config. |
| [1236](https://github.com/cloudwego/eino/issues/1236) | Panic: send on closed channel in agent tool | open | 0 | 1 | Concurrency | Cancellation path races. |
| [1256](https://github.com/cloudwego/eino/issues/1256) | ToolsNode.Stream leaks sibling streams | open | 0 | 1 | Concurrency | Stream ownership on error paths. |
| [1029](https://github.com/cloudwego/eino/issues/1029) | Scriptable MockChatModel for user tests | open | 1 | 1 | Testing | Only internal mocks exist. |
| [1150](https://github.com/cloudwego/eino/issues/1150) | Track cache-write tokens | open | 1 | 2 | Usage | Cost accounting gap. |
| [68](https://github.com/cloudwego/eino/issues/68) | Are you using Eino? | open | 6 | 7 | Adoption | Production at ByteDance and others; HITL was the first question. |
| [830](https://github.com/cloudwego/eino/issues/830) | Can Eino fully replace LangChain + LangGraph? | closed | 0 | 1 | Positioning | "Core yes; ecosystem no." |
| [ext#98](https://github.com/cloudwego/eino-ext/issues/98) | go-openai SSE parsing breaks some providers | closed | 0 | 11 | Provider SDKs | Maintainers forked the SDK. |
| [ext#534](https://github.com/cloudwego/eino-ext/issues/534) | Use official OpenAI SDK? | closed | 1 | 5 | Provider SDKs | Declined: official SDK lacks `reasoning_content`. |
| [ext#291](https://github.com/cloudwego/eino-ext/issues/291) | Migrate to go-genai (Gemini) | closed | 2 | 7 | Provider SDKs | Months on a deprecated SDK. |
| [ext#265](https://github.com/cloudwego/eino-ext/issues/265) | ExtraFields for OpenAI-like APIs | closed | 2 | 5 | Params | Per-call options must pierce agent wrappers. |
| [ext#436](https://github.com/cloudwego/eino-ext/issues/436) | MCP: errors never reach the LLM | closed | 1 | 3 | MCP / tool errors | Violates MCP spec; wrap-it-yourself answer. |
| [ext#550](https://github.com/cloudwego/eino-ext/issues/550) | Gemini: missing thought_signature | closed | 2 | 0 | Provider compat | Opaque reasoning tokens must round-trip. |
| [ext#700](https://github.com/cloudwego/eino-ext/issues/700) | OpenAI adapter drops reasoning_content on tool-call round-trip | closed | 1 | 0 | Provider compat | Kimi 400s on turn 2. |
| [ext#718](https://github.com/cloudwego/eino-ext/issues/718) | First-class GLM / Kimi / MiniMax / Grok modules | open | 1 | 0 | Provider strategy | Custom BaseURL isn't enough. |
| [ext#1005](https://github.com/cloudwego/eino-ext/issues/1005) | Data race: in-place sort of shared tool schema | open | 0 | 1 | Concurrency | Intermittent 400s under load. |
| [ext#128](https://github.com/cloudwego/eino-ext/issues/128) | Langfuse: pass custom traceID | closed | 0 | 17 | Observability | "Usually useless", then added after the user explained. |
| [ext#339](https://github.com/cloudwego/eino-ext/issues/339) | Core says Go 1.18 but ext needs 1.23 | closed | 0 | 9 | Versioning | Misleading compatibility claim. |
