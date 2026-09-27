# Microsoft Agent Framework for Go: What the Issue Trackers Say

- **Date:** 2026-09-26
- **Purpose:** find out what users of Microsoft Agent Framework (MAF) love, hate, struggle with and most want, to inform Dive's design.
- **Companion:** [agent-framework-go.md](agent-framework-go.md) is the source-level analysis. This document links issue themes back to its findings and says where the issues confirm or contradict them.
- **Link convention:** `go#N` is `microsoft/agent-framework-go` and `maf#N` is `microsoft/agent-framework` (the .NET and Python repo). PRs and discussions are labeled as such.

---

## 1. Method

### Repo stats

| | `microsoft/agent-framework-go` | `microsoft/agent-framework` (.NET + Python) |
|---|---|---|
| Created | 2025-10-24 | 2025-04-28 |
| Stars / forks | 633 / 59 | 13,816 / 2,374 |
| Issues | 307 (291 closed, 16 open) | 3,538 (3,006 closed, 532 open) |
| Issue date range | 2025-11-25 to 2026-09-26 | 2025-04-28 to 2026-09-26 |
| Issue authorship | 271 (88%) filed by the `github-actions` bot; 36 by humans, about 15 of them from outside users | about 1,430 (40%) filed by the ~49 staff accounts; the rest from the community. Labels: 1,918 `python`, 1,436 `.NET` |
| Reactions | Effectively none. One issue in the whole repo has a reaction ([go#28](https://github.com/microsoft/agent-framework-go/issues/28)) | Thin. Only 446 issues (13%) have any reaction, the median is 0, and the maximum is 33 ([maf#3499](https://github.com/microsoft/agent-framework/issues/3499)) |
| Discussions | Enabled, 0 threads | 366 threads, low engagement (the maximum is 15 upvotes, [maf disc #1090](https://github.com/microsoft/agent-framework/discussions/1090)) |
| PRs | 882 (167 Dependabot). The most active authors are `PratikDhanave` (270, an outside contributor), `qmuntal` (224, maintainer) and `michelle-clayton-work` (152) | not mined |

### What the Go repo's tracker is

The Go tracker is mostly a porting log, not a feedback channel. The bot-filed issues come from GitHub agentic workflows ("gh-aw") that diff the Go code against .NET. By title prefix: 83 `[aw]` (workflow failures such as "Go API Consistency Review Agent failed" or ".NET to Go Porting Agent exceeded max AI credits"), 77 `[dotnet-code]`, 43 `[dotnet-port]`, 41 `[dotnet-port-api]` and 32 `[dotnet-port-fixes]`. This is **maintainer tracking, and I excluded it from user feedback**. It is still evidence of maintainer stance (section 6).

Of the human issues, the real user reports come from a handful of people who clearly run MAF-Go in production-like setups (`lifeofzero`, `gabisonia`, `gdams`, `sozercan`, `hlgone`, `dagood`). They file high-quality issues with root-cause analysis. Because the Go issue volume is small, the most revealing Go material is in **PR review threads**, where maintainers accept, rework or reject contributions. I mined the 50 most-discussed PRs.

### Why I also mined the main repo

The Go port has too few users to show patterns, and it copies .NET semantics on purpose (section 6). So the main repo's issues predict what Go users will hit. I kept only themes that apply to the Go design: the agent and session model, workflows and checkpointing, HITL and approvals, tool calling, middleware, providers, streaming, durability and observability. I ignored DevUI internals, Python packaging, Azure Foundry hosting quirks and declarative YAML workflows, except as demand signals.

### How I found the signal

1. **Ranked** all issues in both repos by total reactions, 👍 and comment count, then read the top ~60 in full.
2. **Clustered by keyword** over main-repo titles (counts are issues whose title matches, with open issues in brackets): session/thread 196 (34), MCP 193 (13), telemetry/tracing 180 (27), AG-UI 178 (28), streaming 171 (17), approval 114 (17), skills 102 (8), DevUI 95 (28), checkpoint 88 (19), handoff 88 (11), durable/resume 84 (23), middleware 66 (9), compaction/reducer 59 (19), reasoning/thinking 55 (7). Clusters of many small bugs on one theme (approvals, session persistence, reasoning replay) weigh more than any single high-reaction issue.
3. **Searched every comment** for maintainer stance phrases ("by design", "intentional", "as designed", "parity"). This found about 60 design-position statements.
4. **Followed cross-references and repeats.** Some reporters file repeatedly (`marcominerva` about 36 issues, `rwjdk`, `helloxubo`). They act as power-user panels and their issues are consistently high quality.
5. **Separated noise.** The main repo has an agent-authored triage bot ("Automated triage reproduction notes, agent-authored, trust but verify") that posts on most bugs. Several long threads are vendor pitches, not user needs: "Agent Identity and Trust" [maf#4842](https://github.com/microsoft/agent-framework/issues/4842) (17 comments), "Cryptographic proof of authorization" [maf#4203](https://github.com/microsoft/agent-framework/issues/4203) (12) and "Pre-action authority receipt" [maf disc #6078](https://github.com/microsoft/agent-framework/discussions/6078) (13). One comment on [maf#5538](https://github.com/microsoft/agent-framework/issues/5538) is a product plug. I excluded all of these from the rankings.

Reaction counts are low across the board, so no single number is decisive. Most conclusions below rest on a cluster of issues plus a maintainer response, not on votes.

---

## 2. What users love

Explicit praise is rare in both trackers. Users mostly show what they value by what they build on, and by what they ask for more of.

**1. Maintainer responsiveness and fast fixes.** This is the most consistent positive signal. In Go, `gabisonia` filed four bugs in September 2026 and each was fixed by a merged PR within about a day: MCP pagination [go#1067](https://github.com/microsoft/agent-framework-go/issues/1067) → [go PR #1068](https://github.com/microsoft/agent-framework-go/pull/1068), MCP image results [go#1072](https://github.com/microsoft/agent-framework-go/issues/1072) → [go PR #1073](https://github.com/microsoft/agent-framework-go/pull/1073), cancellation [go#1076](https://github.com/microsoft/agent-framework-go/issues/1076) → [go PR #1077](https://github.com/microsoft/agent-framework-go/pull/1077), raw-JSON compaction [go#1087](https://github.com/microsoft/agent-framework-go/issues/1087) → [go PR #1088](https://github.com/microsoft/agent-framework-go/pull/1088). On [go#488](https://github.com/microsoft/agent-framework-go/issues/488), after a before/after check: "Confirmed: #490 fixes it. Thanks for the quick turnaround." In the main repo: "I can confirm that the latest NuGet package resolves the behavior... Thank you for the quick turnaround" [maf#3897](https://github.com/microsoft/agent-framework/issues/3897), and "MAF is becoming better and stronger" after an approval fix [maf#6264](https://github.com/microsoft/agent-framework/issues/6264). Users forgive a lot when bugs close fast.

**2. Deliberate, test-documented semantics.** In [go#492](https://github.com/microsoft/agent-framework-go/issues/492), a user reported that MCP `isError` results come back as `err == nil`. They then retracted it after finding `TestCallPreservesMCPErrorResult`: "the design is deliberate: the failed result is passed through to the model as content, which is exactly what MCP's in-band `isError` is FOR... That is the right call." The maintainer agreed: "`autocall` shouldn't special-case MCP results with `IsError == true`." Tests that pin intent turned a bug report into an endorsement. This matches the source analysis (§8.1.5, "tool-error hygiene").

**3. Building on standards the framework adopts quickly.** The most-reacted main-repo issues are all about adopting ecosystem surfaces: Agent Skills [maf#3499](https://github.com/microsoft/agent-framework/issues/3499) (33 reactions, the top issue), DevUI [maf#1283](https://github.com/microsoft/agent-framework/issues/1283) (31), AG-UI dynamic endpoints [maf#2988](https://github.com/microsoft/agent-framework/issues/2988) (20), Magentic orchestration [maf#1275](https://github.com/microsoft/agent-framework/issues/1275) (18), realtime agents [maf#728](https://github.com/microsoft/agent-framework/issues/728) (14), a public AG-UI conversion API [maf#5209](https://github.com/microsoft/agent-framework/issues/5209) (13) and ACP [maf#3521](https://github.com/microsoft/agent-framework/issues/3521) (11). Users value MAF as the place where new protocols show up already wired in. The source analysis lists bidirectional MCP, A2A and AG-UI adapters as a strength (§8.1.10), and the demand confirms that.

**4. Observability built on OTel GenAI conventions.** Users rely on it enough to file detailed gap reports. "Love a lot of the functionality of Agent Framework," opens a request for richer workflow spans [maf#3075](https://github.com/microsoft/agent-framework/issues/3075). In Go, `lifeofzero` showed a 5-agent workflow with per-agent token counts and argued that tool spans must be "on by default and not on when you remembered" [go#494](https://github.com/microsoft/agent-framework-go/issues/494). Maintainers shipped it ([go PR #446](https://github.com/microsoft/agent-framework-go/pull/446)), along with token usage on agent spans ([go PR #495](https://github.com/microsoft/agent-framework-go/pull/495) for [go#493](https://github.com/microsoft/agent-framework-go/issues/493)).

**5. Composability of agents, tools and workflows.** Workflow-as-agent, agent-as-tool and agent-as-executor are the most-used compositions in the issue reports: [maf#6327](https://github.com/microsoft/agent-framework/issues/6327), [maf#6329](https://github.com/microsoft/agent-framework/issues/6329), [maf#4544](https://github.com/microsoft/agent-framework/issues/4544), [go#1139](https://github.com/microsoft/agent-framework-go/issues/1139), [go#488](https://github.com/microsoft/agent-framework-go/issues/488). Users hit bugs there because they use these compositions heavily, not because they avoid them.

**6. Goodwill that survives expensive bugs.** After a runaway loop cost them about $120 [maf#7472](https://github.com/microsoft/agent-framework/issues/7472), a user wrote: "Consider it my payment to Microsoft for you guys build an awesome framework." Another wrote "Thanks! Loving the framework!" on a bug report [maf#2724](https://github.com/microsoft/agent-framework/issues/2724).

---

## 3. What users hate / friction

### 3.1 Approvals behave per batch, not per call

This is the densest friction cluster (114 approval issues, 17 open). When the model returns several tool calls in one response and any one of them needs approval, **every** call in the batch is surfaced for approval.

- "FunctionApprovalRequest get applied to all functions although expected to only one" [maf#3054](https://github.com/microsoft/agent-framework/issues/3054) (14 comments, still open). Maintainer: "This is by design... The function calling middleware can't call back to the model with only a subset of the requested calls." The user pushed back through five rounds.
- Repeats of the same complaint: [maf#6264](https://github.com/microsoft/agent-framework/issues/6264) (resolved by an opt-in `EnableNonApprovalRequiredFunctionBypassing`), [maf#4879](https://github.com/microsoft/agent-framework/issues/4879) (closed "by design"), [maf#6922](https://github.com/microsoft/agent-framework/issues/6922) (frontend and backend tool calls mixed: "by design in FICC... FICC is stateless") and [maf#8577](https://github.com/microsoft/agent-framework/issues/8577) (two approval requests for one call in handoff, open).
- Security-relevant variant: in Python, an `always_require` tool could be **silently bypassed** depending on its position in the batch [maf#8079](https://github.com/microsoft/agent-framework/issues/8079) (17 comments).
- Semantic gaps: rejecting without a reason makes some models re-request the same call [maf#8503](https://github.com/microsoft/agent-framework/issues/8503), because the synthesized result "Tool call invocation rejected." does not say the decision is final.

**Root cause:** MEAI's `FunctionInvokingChatClient` (FICC) is stateless, so it has nowhere to hold the results of safe calls while it waits for approval. The fix came late and as an opt-in that needs session storage.

**Connection to the source analysis.** MAF-Go already does the better thing. It auto-approves and hides the safe calls when a session is available (§5.3, `prepareApprovalContents`). Its harness audit notes that .NET has since removed the flags that disable this [go#1141](https://github.com/microsoft/agent-framework-go/issues/1141). The Go port inherits the fixed semantics, but only when a session is present.

### 3.2 Too many layers, too many places to configure one thing

MAF stacks `IChatClient` decorators (MEAI), FICC, `AIAgent` decorators, context providers and history providers. Users can't tell where a behavior belongs, and ordering bugs follow.

- **Compaction** can be registered in three places with different semantics. "What is the recommended way to register a compaction strategy?" A maintainer answered with a three-part essay [maf#7358](https://github.com/microsoft/agent-framework/issues/7358). A related bug: `CompactionProvider` mutates messages in place and silently drops the first user message [maf#6972](https://github.com/microsoft/agent-framework/issues/6972).
- **Telemetry** has to be enabled twice (on the chat client and on the agent) before `execute_tool` spans appear [maf#2015](https://github.com/microsoft/agent-framework/issues/2015). "+1 towards not requiring .UseOpenTelemetry twice. This was quite unexpected." Tool exceptions are logged only if `ILoggerFactory` also reaches `UseFunctionInvocation` [maf#2211](https://github.com/microsoft/agent-framework/issues/2211).
- **Middleware order is inverted** for function-call middleware. The maintainer's answer: "changing the behavior at this point would be a breaking change... labeled as required for a future V2" [maf#6260](https://github.com/microsoft/agent-framework/issues/6260).
- **Option merging drops fields.** Per-request `ChatOptions` silently lost the agent's `Reasoning` settings [maf#5460](https://github.com/microsoft/agent-framework/issues/5460) (8 reactions). "I have a couple of customers who have been blocked for quite a while."
- **Features don't compose.** `RunAsync<T>` structured output breaks as soon as function middleware is added [maf#4118](https://github.com/microsoft/agent-framework/issues/4118) (7 reactions).

**Connection to the source analysis:** §8.2.1 argues that fusing the model and the agent removes per-model-call hooks. The .NET experience shows the opposite failure. Too *many* independently composable layers confuse users just as much. What users want is one obvious place per concern.

### 3.3 The agent is too static

- "`ChatClientAgent` having internal `ChatOptions` is overly restrictive" [maf#1710](https://github.com/microsoft/agent-framework/issues/1710) (13 comments). Users want to change reasoning effort, tools and instructions mid-conversation ("Claude Code for example will change thinking parameters during a thread"). Stephen Toub's reply: "Encouraging mutating the agent seems like a footgun... The options effectively define what the agent is."
- Dynamic, in-run tool loading, citing Anthropic's tool-search pattern [maf#3083](https://github.com/microsoft/agent-framework/issues/3083) (10 comments). The first answer was "This is not really a feature of LLMs," but the team eventually fixed FICC's tool snapshot so tools can add tools mid-run [maf#5325](https://github.com/microsoft/agent-framework/issues/5325). Runtime-defined tool parameters: "the only missing functionality that blocks us from moving from .NET Semantic Kernel to MAF" [maf#2909](https://github.com/microsoft/agent-framework/issues/2909).

### 3.4 Structured output feels worse than the layer below it

"After playing around with structured Output in AF, it feels like a step down from MEAI and SK" [maf#1057](https://github.com/microsoft/agent-framework/issues/1057) (14 comments). This led to `RunAsync<T>`, which then broke with middleware [maf#4118](https://github.com/microsoft/agent-framework/issues/4118). Mixing structured output with tools fails on providers that emit text on intermediate turns [maf#6467](https://github.com/microsoft/agent-framework/issues/6467). In Go, structured output for Claude first went through a synthetic tool, until the maintainer pushed for native structured outputs ([go PR #98](https://github.com/microsoft/agent-framework-go/pull/98)). The same thread noted that "structured output is filled in by the agent in key-alphabetical order, not struct order."

**Connection to the source analysis:** §6 flags that MAF-Go's `structuredOutputMiddleware` unmarshals after *every* model call, including tool-call turns, and that no test covers structured output combined with tools. The .NET issues show this combination is where users actually get hurt, so the concern in the source analysis is well founded.

### 3.5 Tools can't return rich results or see their context

- **Multimodal tool results.** "How to make a tool return both text and image?" [maf#1569](https://github.com/microsoft/agent-framework/issues/1569) has 21 comments, the most of any issue in the main repo. It needed an upstream OpenAI SDK change, and a later commenter says it still serializes to a string. See also [maf#2513](https://github.com/microsoft/agent-framework/issues/2513). In Go, a tool returning `*mcp.ImageContent` reached the client as text [go#1072](https://github.com/microsoft/agent-framework-go/issues/1072), and `json.RawMessage` results were printed as byte arrays by compaction [go#1087](https://github.com/microsoft/agent-framework-go/issues/1087). Both stem from the untyped `(any, error)` tool result that the source analysis criticizes (§8.2.7).
- **Context propagation into tools.** Users want to pass a request-scoped user ID, token, headers or session into tools and sub-agents: [maf#2694](https://github.com/microsoft/agent-framework/issues/2694) (18 comments), [maf#4808](https://github.com/microsoft/agent-framework/issues/4808) (11, dynamic MCP headers), [maf#3746](https://github.com/microsoft/agent-framework/issues/3746) (per-request data for DI singletons) and [maf disc #1193](https://github.com/microsoft/agent-framework/discussions/1193). In Go, `agenttool` can't reach the caller's session because "`tool.Tool.Call(ctx, args)` receives only `ctx` and the JSON args — not the options" [go#1139](https://github.com/microsoft/agent-framework-go/issues/1139) (open). Enterprise gateways also need the tool **call ID** for policy, audit and consume-once approval records [go#949](https://github.com/microsoft/agent-framework-go/issues/949) (open). That need was partly met by `Config.FunctionMiddlewares` with call identity ([go PR #1093](https://github.com/microsoft/agent-framework-go/pull/1093)).
- **Class-to-tools boilerplate.** "This is a lot of boilerplate code" [maf#726](https://github.com/microsoft/agent-framework/issues/726) (18 comments). The maintainers declined an automatic `FromType` API because "we are avoiding adding APIs that bring more questions than answers."

### 3.6 Naming churn, breaking changes and version coupling

- `ExecutorIsh` [maf#758](https://github.com/microsoft/agent-framework/issues/758), [maf#1305](https://github.com/microsoft/agent-framework/issues/1305) (16 comments): "I assumed this was an intended-to-be-internal type that was accidentally exposed." It was later renamed `ExecutorRegistration`. Its implicit conversions also broke F# users.
- `AgentThread` was renamed to `AgentSession` [maf#775](https://github.com/microsoft/agent-framework/issues/775), `ChatMessageStore` was reshaped into `ChatHistoryProvider` [maf#2518](https://github.com/microsoft/agent-framework/issues/2518), and `DeleteSessionAsync` disappeared in 1.22 with no explanation [maf#8586](https://github.com/microsoft/agent-framework/issues/8586).
- Releases shipped without a changelog: "There aren't even any tags on the repository" [maf#6167](https://github.com/microsoft/agent-framework/issues/6167), [maf#6100](https://github.com/microsoft/agent-framework/issues/6100).
- Dependency lockstep breaks. MAF rc4 plus MEAI 10.4 gave a `TypeLoadException` [maf#4709](https://github.com/microsoft/agent-framework/issues/4709). MAF 1.5 broke the Anthropic and Google providers with `MissingMethodException` [maf#5707](https://github.com/microsoft/agent-framework/issues/5707). "Minor release on a 'stable' package breaking stuff."

### 3.7 Docs lag the code

DevUI's limits were undocumented [maf#2084](https://github.com/microsoft/agent-framework/issues/2084) (16 comments, open since 2025-11). There is no official migration path from Semantic Kernel's OpenAPI plugin [maf#1809](https://github.com/microsoft/agent-framework/issues/1809) (18 comments). Declarative workflow docs are out of date [maf#7977](https://github.com/microsoft/agent-framework/issues/7977). The observability docs had no example of the logger wiring that tool-failure logging needs [maf#2211](https://github.com/microsoft/agent-framework/issues/2211). Compaction registration is unexplained [maf#7358](https://github.com/microsoft/agent-framework/issues/7358).

---

## 4. Big problems

These are bugs or architectural limits with real cost: data loss, duplicated side effects, blocked customers or runaway spend.

### 4.1 Conversation state is lost on cancellation, failure or mid-run crash

This is the clearest architectural limit, and the maintainers hold it "by design."

- "AgentThread is not persisted when RunStreamingAsync exits due to error or cancellation" [maf#2889](https://github.com/microsoft/agent-framework/issues/2889). Maintainer: "This is by design since storing partial state is problematic. E.g. if you have a FunctionCallContent with no FunctionResultContent..."
- The UI and the stored session drift apart after the user presses Stop: "the user may have already received a partial assistant response, while the AgentSession does not contain either the user message or the partial assistant response" [maf#8157](https://github.com/microsoft/agent-framework/issues/8157) (6 reactions, open).
- Thread storage is filled only at the end of a run, so a 40-step Playwright tool loop can't be paused or recovered [maf#2201](https://github.com/microsoft/agent-framework/issues/2201). The maintainers "decided that Threads are not the way to solve for this."
- In production, a page refresh during a long harness run lost the transcript, and "a completed run's stored transcript can be silently and permanently replaced by a shorter one" [maf#7215](https://github.com/microsoft/agent-framework/issues/7215).
- A failed approval-resume run "permanently corrupts the session", and every later run throws [maf#8575](https://github.com/microsoft/agent-framework/issues/8575). The fix: commit approval state only after success, and "it's important to ensure that functions are idempotent if retrying."

**Connection to the source analysis:** this confirms §4.1 and §4.3 exactly. MAF-Go stores history only after a successful run and skips the store when `InvokedContext.Err != nil`. "Nothing is persisted mid-run." MAF-Go inherits every one of these complaints.

### 4.2 Workflow checkpoints replay side effects

- "Workaround for tool call side-effect replay on checkpoint-based retry after executor failure" [maf#3938](https://github.com/microsoft/agent-framework/issues/3938) (12 comments, open). Emails get sent twice and database writes are duplicated. Maintainer: "Idempotency should be implemented at the executor layer. We will consider adding mid super step checkpoints."
- "Thoughts on supporting Durable execution" [maf disc #1092](https://github.com/microsoft/agent-framework/discussions/1092) (13 comments). "Checkpointing is expensive and unreliable compared to durable execution... you run the atomicity gap risk all the time." The eventual answer was a separate Durable Task extension for Azure, not a change to the core.
- Distributed or alternative workflow runtimes [maf#1445](https://github.com/microsoft/agent-framework/issues/1445) has been open since 2025-10 ("Want to use orleans").

**Connection to the source analysis:** this confirms §4.3: "at-least-once per superstep... no idempotency keys... checkpoint/restore, not durable execution."

### 4.3 Checkpoint and workflow identity are brittle

- Resuming a checkpoint fails when agents are re-created with new GUID IDs [maf#4793](https://github.com/microsoft/agent-framework/issues/4793) (12 reactions). The workaround is an `IdFixingAgent` decorator. The source analysis §4.3 notes the same Go requirement: "A rebuilt workflow must use identical IDs."
- A workflow registered as a DI singleton can't run twice concurrently: "Cannot use a Workflow that is already owned by another runner" [maf#3620](https://github.com/microsoft/agent-framework/issues/3620) (9 reactions). MAF-Go has the same `TakeOwnership` CAS (§4.2).

### 4.4 Message history gets malformed in multi-agent flows

Handoff interleaves a specialist's text between a coordinator's function call and its result. "That violates OpenAI's adjacency rule" [maf#4544](https://github.com/microsoft/agent-framework/issues/4544) (12 comments). Handoffs also send a stale `previous_response_id` and lose context [maf#4053](https://github.com/microsoft/agent-framework/issues/4053), drop messages after `finish_reason=stop` [maf#1972](https://github.com/microsoft/agent-framework/issues/1972), and put tool calls in the wrong order under AG-UI [maf#6909](https://github.com/microsoft/agent-framework/issues/6909). Sequential orchestration passes the whole history, image bytes included, to a model without vision [maf#1266](https://github.com/microsoft/agent-framework/issues/1266). The root cause is the dual local-versus-service history model ([maf#2054](https://github.com/microsoft/agent-framework/issues/2054), [maf#4577](https://github.com/microsoft/agent-framework/issues/4577)).

**Connection to the source analysis:** this confirms §8.2.9, which calls the history and server-state duality a source of conditionals and bugs.

### 4.5 Streaming and non-streaming paths diverge

A preview release made `RunStreamingAsync` ignore the session entirely [maf#3848](https://github.com/microsoft/agent-framework/issues/3848), [maf#3897](https://github.com/microsoft/agent-framework/issues/3897): "the LLM never sees any previous conversation context". `RunAsync` still worked. Python users reported that intermediate text was dropped during tool loops [maf#4868](https://github.com/microsoft/agent-framework/issues/4868) and that reasoning text bled into output [maf#3030](https://github.com/microsoft/agent-framework/issues/3030).

**Connection to the source analysis:** this is a point *in favor of* MAF-Go's single `iter.Seq2` primitive (§8.1.1). The Go port can't have this class of bug because the non-streaming path is `Collect()` over the stream.

### 4.6 Provider replay of reasoning and tool arguments breaks

This is a steady stream of 400 errors from replaying history across turns:
- Go, OpenAI Responses: replaying a reasoning item gave four stacked errors (missing `summary`, empty `id`, plaintext `content`, then a duplicate id under `store=true`) [go#783](https://github.com/microsoft/agent-framework-go/issues/783).
- Go, Anthropic: tool arguments were sent as a string [go#35](https://github.com/microsoft/agent-framework-go/issues/35), then became concatenated JSON (`{...}{...}`) only on the workflow executor path [go#488](https://github.com/microsoft/agent-framework-go/issues/488).
- DeepSeek `reasoning_content` is not replayed, in both .NET and Python [maf#5538](https://github.com/microsoft/agent-framework/issues/5538), [maf#2603](https://github.com/microsoft/agent-framework/issues/2603).
- In Go, an unknown annotation type from a newer service failed the whole message unmarshal [go#513](https://github.com/microsoft/agent-framework-go/issues/513). The fix added `RawAnnotation`, extending the `RawContent` fallback that the source analysis praises (§8.1.6).

### 4.7 Runaway loops burn money

A sample with auto-approved skill loading looped for three days and used "100+ Millions tokens" [maf#7472](https://github.com/microsoft/agent-framework/issues/7472). The root cause was `ToolApprovalAgent` re-invoking the inner agent from unbounded `while (true)` loops, so the FICC iteration cap reset on every pass. **Connection to the source analysis:** MAF-Go's harness has `MaxAutoApprovalIterations` (§5.3), so the Go port has at least one outer bound.

### 4.8 Silent correctness bugs in the tool path

MCP: tool discovery returned only the first page [go#1067](https://github.com/microsoft/agent-framework-go/issues/1067); `content` and `structuredContent` were duplicated, doubling tokens [maf#7866](https://github.com/microsoft/agent-framework/issues/7866); `structuredContent` never reached AG-UI [maf#7959](https://github.com/microsoft/agent-framework/issues/7959). Cancellation: the next tool still ran after the request was cancelled [go#1076](https://github.com/microsoft/agent-framework-go/issues/1076). A stock reducer silently dropped tool calls from stored history [maf#4494](https://github.com/microsoft/agent-framework/issues/4494). Observability: there were no tool spans [go#494](https://github.com/microsoft/agent-framework-go/issues/494), no token usage [go#493](https://github.com/microsoft/agent-framework-go/issues/493), and executors were missing from traces [maf#4029](https://github.com/microsoft/agent-framework/issues/4029).

**Contradiction with the source analysis:** §5.2 says the loop checks `ctx.Err()` before every tool call. That is true at the analyzed commit, but only since [go PR #1077](https://github.com/microsoft/agent-framework-go/pull/1077) (2026-09-16). Before that, a cancelled request still started the next sequential tool.

---

## 5. Most desired features

Ranked by reactions, then by the size of the issue cluster.

| Rank | Request | Evidence | Maintainer response |
|---|---|---|---|
| 1 | **Agent Skills** (SKILL.md) | [maf#3499](https://github.com/microsoft/agent-framework/issues/3499) 33 reactions; [maf#4348](https://github.com/microsoft/agent-framework/issues/4348); [maf#4636](https://github.com/microsoft/agent-framework/issues/4636) ("Why is .NET much slower than Python in supporting Skills?") | Shipped. .NET lagged because "the team is working with the team who own Microsoft.Extensions.AI... this takes extra time that is not needed on Python." MAF-Go has `agent/skills`. |
| 2 | **Dev UI / visual debugger** | [maf#1283](https://github.com/microsoft/agent-framework/issues/1283) 31; [maf#1686](https://github.com/microsoft/agent-framework/issues/1686) 12; [maf#1545](https://github.com/microsoft/agent-framework/issues/1545) 12; [maf#2084](https://github.com/microsoft/agent-framework/issues/2084) 16 comments | Shipped Python-first, then .NET; limits still undocumented. Not in MAF-Go. |
| 3 | **AG-UI hosting** | [maf#2988](https://github.com/microsoft/agent-framework/issues/2988) 20; [maf#896](https://github.com/microsoft/agent-framework/issues/896) 12; [maf#1774](https://github.com/microsoft/agent-framework/issues/1774) 11; [maf#5209](https://github.com/microsoft/agent-framework/issues/5209) 13 | Shipped. Maintainers refuse protocol-divergent extensions ([maf#3790](https://github.com/microsoft/agent-framework/issues/3790), [maf#3684](https://github.com/microsoft/agent-framework/issues/3684)). MAF-Go has AG-UI both ways. |
| 4 | **Magentic / handoff orchestration** | [maf#1275](https://github.com/microsoft/agent-framework/issues/1275) 18; [go#520](https://github.com/microsoft/agent-framework-go/issues/520); [go#564](https://github.com/microsoft/agent-framework-go/issues/564) | Shipped in .NET and Python. In Go, the community PRs were closed: "Too many parity issues. I don't think this porting can be done in a purely automated manner" ([go PR #545](https://github.com/microsoft/agent-framework-go/pull/545), [go PR #565](https://github.com/microsoft/agent-framework-go/pull/565)). |
| 5 | **Realtime / voice agents** | [maf#728](https://github.com/microsoft/agent-framework/issues/728) 14 reactions, open since 2025-09 ("We really hope this will make it in there soon, otherwise we will have to look for other alternatives") | No commitment; only an automated "waiting on your response" ping. |
| 6 | **Agent Client Protocol (ACP)** | [maf#3521](https://github.com/microsoft/agent-framework/issues/3521) 11, filed by Stephen Toub; a commenter notes "almost all coding cli already support acp" | Open, no plan. |
| 7 | **Durable / distributed execution** | [maf disc #1092](https://github.com/microsoft/agent-framework/discussions/1092); [maf#1445](https://github.com/microsoft/agent-framework/issues/1445); [maf#3938](https://github.com/microsoft/agent-framework/issues/3938) | Durable Task extension (Azure); mid-superstep checkpoints "will consider." |
| 8 | **Persist partial turns on cancel** | [maf#8157](https://github.com/microsoft/agent-framework/issues/8157) 6; [maf#2889](https://github.com/microsoft/agent-framework/issues/2889); [maf#7215](https://github.com/microsoft/agent-framework/issues/7215) | Declined "by design" in 2025, reopened as a feature request in 2026. |
| 9 | **Dynamic agents: mutable options, in-run tools** | [maf#1710](https://github.com/microsoft/agent-framework/issues/1710); [maf#3083](https://github.com/microsoft/agent-framework/issues/3083) 6; [maf#2909](https://github.com/microsoft/agent-framework/issues/2909); [maf#5325](https://github.com/microsoft/agent-framework/issues/5325) | Options stay immutable, with per-run overrides. The FICC clone fix allows tools to be added mid-run. |
| 10 | **Prompt templates for instructions** | [maf#121](https://github.com/microsoft/agent-framework/issues/121) (open since 2025-07); [maf disc #1090](https://github.com/microsoft/agent-framework/discussions/1090) 15 upvotes, the top discussion | "Likely we don't want to duplicate the SK templated prompt support." Still open. |
| 11 | **Context propagation into tools / sub-agents** | [maf#2694](https://github.com/microsoft/agent-framework/issues/2694) 18 comments; [maf#4808](https://github.com/microsoft/agent-framework/issues/4808); [go#1139](https://github.com/microsoft/agent-framework-go/issues/1139); [go#949](https://github.com/microsoft/agent-framework-go/issues/949) | .NET: session-scoped options and `header_provider` for MCP. Go: `FunctionMiddlewares` with call ID merged; session-to-tool plumbing still under design. |
| 12 | **Go-specific gaps** | [maf#5017](https://github.com/microsoft/agent-framework/issues/5017) ("Go language support", answered with a link to the Go repo); [go#1190](https://github.com/microsoft/agent-framework-go/issues/1190) ("Is the RAG feature in the developing plan", unanswered) | MAF-Go is porting the .NET harness next [go#1141](https://github.com/microsoft/agent-framework-go/issues/1141). |
| 13 | **Anthropic prompt caching in Go** | [go PR #496](https://github.com/microsoft/agent-framework-go/pull/496) (it measured 83% of cost as repeated input, with `cached_input_tokens = 0` on every span) | Asked to be reworked to match .NET's per-content `WithCacheControl`, and to "leave higher level constructs for a future PR." The PR then stalled and was closed. |

---

## 6. Maintainer stance and trajectory

### 6.1 MAF-Go: .NET parity is the spec

The Go maintainers accept, rework or reject contributions by checking them against .NET behavior. The tooling makes this explicit: a weekly .NET-to-Go symbol-mapping audit ([go#1188](https://github.com/microsoft/agent-framework-go/issues/1188) is a failed run) and gh-aw agents that file `[dotnet-port*]` issues.

- "We try to follow .NET MAF, and it still doesn't implement these new APIs. Closing for now." ([go PR #659](https://github.com/microsoft/agent-framework-go/pull/659), MCP sampling and `list_changed`)
- "This PR is not in semantic parity with .NET." ([go PR #708](https://github.com/microsoft/agent-framework-go/pull/708))
- Prompt caching must follow ".NET Anthropic SDK... explicit on individual content blocks through `WithCacheControl(...)`" rather than an agent-level flag ([go PR #496](https://github.com/microsoft/agent-framework-go/pull/496)).
- Strict JSON schemas: "parity should follow [.NET's] separation of schemas" ([go PR #689](https://github.com/microsoft/agent-framework-go/pull/689)).
- Even defaults are copied: `MaximumIterationsPerRequest = 40` and `MaximumConsecutiveErrorsPerRequest = 3` were checked against .NET by a parity bot ([go PR #205](https://github.com/microsoft/agent-framework-go/pull/205)).

**There is a limit on automation.** Agent-generated porting PRs are repeatedly rejected: "Too many parity issues. I don't think this porting can be done in a purely automated manner" ([go PR #545](https://github.com/microsoft/agent-framework-go/pull/545), [go PR #565](https://github.com/microsoft/agent-framework-go/pull/565)) and "Too many parity findings, this needs some human love" ([go PR #644](https://github.com/microsoft/agent-framework-go/pull/644), [go PR #694](https://github.com/microsoft/agent-framework-go/pull/694)).

**Idiomatic Go is accepted when it costs nothing in parity.** Examples: removing `tool.Context` in favor of plain `context.Context` ([go PR #205](https://github.com/microsoft/agent-framework-go/pull/205)); passing options by value ([go#28](https://github.com/microsoft/agent-framework-go/issues/28) → [go PR #37](https://github.com/microsoft/agent-framework-go/pull/37)); open questions in the harness audit about "whether to preserve idiomatic Go behavior or match .NET exactly" [go#1141](https://github.com/microsoft/agent-framework-go/issues/1141). **Contradiction with the source analysis:** §8.2.3 says "idiomatic Go is not a goal." The issues show idiomatic Go is a *secondary* goal that wins small, local choices, but never semantics.

### 6.2 MAF-Go: small core, extension through middleware

- Per-tool interception: "We should have this, but I don't like the approach taken. .NET supports hooking into tool calls via a delegating agent, which we map as a `agent.Middleware`... Closing for now." ([go PR #638](https://github.com/microsoft/agent-framework-go/pull/638)). The eventual design put `Config.FunctionMiddlewares` around every tool call "without requiring users to get the middleware ordering right" ([go PR #1093](https://github.com/microsoft/agent-framework-go/pull/1093)). This partly answers the source analysis's complaint in §6 that users can't add their own interception.
- MCP errors: "If someone needs to deal with them, then a custom middleware is the best approach" [go#492](https://github.com/microsoft/agent-framework-go/issues/492).
- Declined: workflow Mermaid/DOT export ("Will not implement this for now. Closing to reduce noise", [go PR #633](https://github.com/microsoft/agent-framework-go/pull/633)) and `json.Number` for large integers (the exported type must not change; a "broader serialization design" is needed, [go PR #528](https://github.com/microsoft/agent-framework-go/pull/528)).

### 6.3 Main MAF: stateless core, "by design" boundaries

| Position | Where stated |
|---|---|
| FICC is stateless, so approvals are per batch and mixed frontend/backend calls terminate the loop | [maf#3054](https://github.com/microsoft/agent-framework/issues/3054), [maf#6922](https://github.com/microsoft/agent-framework/issues/6922), [maf#4879](https://github.com/microsoft/agent-framework/issues/4879) |
| Partial state is never persisted, because dangling calls make invalid history | [maf#2889](https://github.com/microsoft/agent-framework/issues/2889), [maf#2201](https://github.com/microsoft/agent-framework/issues/2201) |
| Tool failures go back to the LLM so it can self-correct; they don't become exceptions | [maf#5325](https://github.com/microsoft/agent-framework/issues/5325) |
| An agent is immutable ("the options effectively define what the agent is") | [maf#1710](https://github.com/microsoft/agent-framework/issues/1710) |
| Idempotency is the user's job (executor or tool) | [maf#3938](https://github.com/microsoft/agent-framework/issues/3938), [maf#8575](https://github.com/microsoft/agent-framework/issues/8575) |
| Protocol fidelity over convenience: no non-standard AG-UI fields | [maf#3790](https://github.com/microsoft/agent-framework/issues/3790), [maf#3684](https://github.com/microsoft/agent-framework/issues/3684), [maf#7629](https://github.com/microsoft/agent-framework/issues/7629) |
| Handoff is decentralized: the active agent picks the target, and there is no override callback | [maf#7760](https://github.com/microsoft/agent-framework/issues/7760) |
| `AsAgent()` on a workflow loses executor detail by design | [maf#4445](https://github.com/microsoft/agent-framework/issues/4445) |
| Behavioral breaks are deferred to "V2" | [maf#6260](https://github.com/microsoft/agent-framework/issues/6260) |
| .NET waits on shared MEAI abstractions, so it ships later than Python | [maf#4636](https://github.com/microsoft/agent-framework/issues/4636) |

### 6.4 Trajectory

Both repos are moving from "chat client plus tool loop" toward a **harness**: agent mode, todos, background agents, file memory and file access, compaction and tool-approval agents. The Go port is catching up on this layer [go#1141](https://github.com/microsoft/agent-framework-go/issues/1141), [go#1163](https://github.com/microsoft/agent-framework-go/issues/1163), [go#1164](https://github.com/microsoft/agent-framework-go/issues/1164). Durability is moving from "the user serializes the session" toward session-backed stores (`AgentSessionStore`, per-service-call persistence) and a Durable Task extension. Session-level approval storage (the opt-in in [maf#6264](https://github.com/microsoft/agent-framework/issues/6264)) and retry-safe approval commits [maf#8575](https://github.com/microsoft/agent-framework/issues/8575) show the team slowly accepting that **the loop needs state**. That is the position the stateless-FICC design argued against.

---

## 7. Lessons for Dive

Prioritized by (evidence strength) × (cost to users when wrong). Where Dive already has the feature (per `CLAUDE.md`), the issues confirm it is the right bet, and the lesson is to keep it prominent and well documented.

### P0: the problems that cost users data or money

1. **Keep the durable, closeable turn as Dive's headline feature.** MAF's biggest limit (§4.1, §4.2) is exactly what Dive's incomplete-turn design addresses: `closeturn` answers every call, `DurabilityOptions` adds step checkpoints, `WithContinue` resumes, and `WithSoftCancel` stops cleanly. MAF refuses to persist partial state because dangling calls make invalid history [maf#2889](https://github.com/microsoft/agent-framework/issues/2889). Dive answers every unanswered call before saving, which is the missing piece. Add three things:
   - A documented guarantee that the saved session matches what the user saw after a Stop [maf#8157](https://github.com/microsoft/agent-framework/issues/8157).
   - Idempotency keys for tools (`sessionID/turnID/callID`) in context, because both MAF teams push idempotency onto users [maf#3938](https://github.com/microsoft/agent-framework/issues/3938), [maf#8575](https://github.com/microsoft/agent-framework/issues/8575).
   - Tests for "resume after a failed resume" [maf#8575](https://github.com/microsoft/agent-framework/issues/8575).
2. **Make approvals per call, not per batch, and order-independent.** Dive's loop owns session state, so it can run safe calls, hold their results, and suspend only the calls that need approval. That is the behavior MAF could not offer [maf#3054](https://github.com/microsoft/agent-framework/issues/3054), [maf#6264](https://github.com/microsoft/agent-framework/issues/6264). Also:
   - Classify the whole batch before acting [maf#8079](https://github.com/microsoft/agent-framework/issues/8079).
   - Send the model a clear, final rejection result even when no reason is given [maf#8503](https://github.com/microsoft/agent-framework/issues/8503).
   - Expose pending approvals through a public API on the suspended turn [maf#7862](https://github.com/microsoft/agent-framework/issues/7862).
   - Bind approval responses to recorded requests (MAF-Go's binding; source analysis §8.1.3).
3. **Bound every loop layer, including costs.** Enforce a single iteration and token/cost budget across hooks that re-enter the loop (Stop hooks with `Continue`), auto-approval and subagents, so that no layer resets another's counter [maf#7472](https://github.com/microsoft/agent-framework/issues/7472).
4. **Keep one execution path for streaming and non-streaming.** A whole class of MAF regressions came from the two paths diverging [maf#3848](https://github.com/microsoft/agent-framework/issues/3848), [maf#3897](https://github.com/microsoft/agent-framework/issues/3897). Stream intermediate assistant text and reasoning before tool calls [maf#4868](https://github.com/microsoft/agent-framework/issues/4868), and keep reasoning separate from text [maf#3030](https://github.com/microsoft/agent-framework/issues/3030).

### P1: the friction that drives people away

5. **One obvious place per concern.** Each of compaction, telemetry, retries and caching should have a single registration point, with defaults that work without extra wiring [maf#7358](https://github.com/microsoft/agent-framework/issues/7358), [maf#2015](https://github.com/microsoft/agent-framework/issues/2015), [maf#2211](https://github.com/microsoft/agent-framework/issues/2211). Hook order must be the registration order, and a test must pin that [maf#6260](https://github.com/microsoft/agent-framework/issues/6260).
6. **Give tools their context.** Tools and subagents should get typed access to the session ID, turn ID, call ID and caller-supplied request metadata (user, auth token, headers) through `context.Context`, not through model-visible arguments [go#949](https://github.com/microsoft/agent-framework-go/issues/949), [go#1139](https://github.com/microsoft/agent-framework-go/issues/1139), [maf#2694](https://github.com/microsoft/agent-framework/issues/2694), [maf#4808](https://github.com/microsoft/agent-framework/issues/4808), [maf#3746](https://github.com/microsoft/agent-framework/issues/3746). MCP clients need per-call header providers [maf#4808](https://github.com/microsoft/agent-framework/issues/4808).
7. **Let agents change per run.** Allow per-call overrides of instructions, tools, model and reasoning effort, and tool sets that change mid-turn (Dive's `Toolset` resolves per request, which already covers this) [maf#1710](https://github.com/microsoft/agent-framework/issues/1710), [maf#3083](https://github.com/microsoft/agent-framework/issues/3083), [maf#5325](https://github.com/microsoft/agent-framework/issues/5325). When merging options, fail a test if a new field is not merged, so the [maf#5460](https://github.com/microsoft/agent-framework/issues/5460) class of bug can't happen.
8. **Rich, typed tool results.** Support multimodal content, an explicit `IsError`, preserved raw JSON, and a stated policy for MCP `content` versus `structuredContent` [maf#1569](https://github.com/microsoft/agent-framework/issues/1569), [go#1072](https://github.com/microsoft/agent-framework-go/issues/1072), [go#1087](https://github.com/microsoft/agent-framework-go/issues/1087), [maf#7866](https://github.com/microsoft/agent-framework/issues/7866). Handle MCP pagination [go#1067](https://github.com/microsoft/agent-framework-go/issues/1067). Dive's `ToolResultBlocks` and image-lifting helpers are the right direction. Document them as a headline feature, since [maf#1569](https://github.com/microsoft/agent-framework/issues/1569) is the most-commented issue in the main repo.
9. **Structured output that works with tools and hooks.** Validate only the final turn, allow text on intermediate turns, use native provider structured output where it exists, and keep struct field order [maf#1057](https://github.com/microsoft/agent-framework/issues/1057), [maf#4118](https://github.com/microsoft/agent-framework/issues/4118), [maf#6467](https://github.com/microsoft/agent-framework/issues/6467), [go PR #98](https://github.com/microsoft/agent-framework-go/pull/98). This also answers source analysis §6's open question.
10. **Observability on by default.** Tool spans, token usage, model name and error details should all be present with a single `otel.NewTracer` [go#494](https://github.com/microsoft/agent-framework-go/issues/494), [go#493](https://github.com/microsoft/agent-framework-go/issues/493), [maf#2015](https://github.com/microsoft/agent-framework/issues/2015), [maf#2211](https://github.com/microsoft/agent-framework/issues/2211). Nest spans correctly under parallel tool calls [maf#4029](https://github.com/microsoft/agent-framework/issues/4029).
11. **Cross-provider replay conformance tests.** Write a shared suite that round-trips reasoning (signatures, encrypted content, Responses reasoning items, DeepSeek `reasoning_content`), tool arguments and server-stored IDs through multi-turn tool loops for every provider [go#783](https://github.com/microsoft/agent-framework-go/issues/783), [go#488](https://github.com/microsoft/agent-framework-go/issues/488), [go#35](https://github.com/microsoft/agent-framework-go/issues/35), [maf#5538](https://github.com/microsoft/agent-framework/issues/5538). Dive's `llm.AnswerUnansweredToolCalls` applied in every encoder is the kind of shared invariant to extend.

### P2: strategy and positioning

12. **Stable names and honest changelogs.** MAF lost trust over `ExecutorIsh`, the thread-to-session rename, removed methods and releases without a changelog [maf#758](https://github.com/microsoft/agent-framework/issues/758), [maf#775](https://github.com/microsoft/agent-framework/issues/775), [maf#8586](https://github.com/microsoft/agent-framework/issues/8586), [maf#6167](https://github.com/microsoft/agent-framework/issues/6167). Dive's Keep-a-Changelog discipline is a real differentiator, so keep it. Avoid lockstep dependencies that break at runtime [maf#4709](https://github.com/microsoft/agent-framework/issues/4709), [maf#5707](https://github.com/microsoft/agent-framework/issues/5707).
13. **Go-first is the opening.** MAF-Go will copy .NET semantics even where Go users want something else (§6.1). Community feature PRs die on parity review ([go PR #545](https://github.com/microsoft/agent-framework-go/pull/545), [go PR #565](https://github.com/microsoft/agent-framework-go/pull/565), [go PR #616](https://github.com/microsoft/agent-framework-go/pull/616), [go PR #644](https://github.com/microsoft/agent-framework-go/pull/644)). Dive can take the unanswered Go requests: RAG helpers [go#1190](https://github.com/microsoft/agent-framework-go/issues/1190), handoff patterns [go#520](https://github.com/microsoft/agent-framework-go/issues/520), and prompt caching with both a per-block primitive *and* an opinionated default ([go PR #496](https://github.com/microsoft/agent-framework-go/pull/496) measured 83% of cost as uncached input).
14. **Pick a small set of standards and adopt them fast.** The most-reacted items are protocol adoptions (Skills, AG-UI, ACP, realtime). Dive already aligns with Claude Code tools, Skills and A2A. ACP [maf#3521](https://github.com/microsoft/agent-framework/issues/3521) is an open, unclaimed request, and a good fit for Dive's coding-agent orientation.
15. **If Dive adds multi-agent orchestration, validate history before sending.** Check tool-call and result adjacency, choose explicitly what context is passed between agents, and never let workflow bookkeeping messages enter model history [maf#4544](https://github.com/microsoft/agent-framework/issues/4544), [maf#1266](https://github.com/microsoft/agent-framework/issues/1266), [maf#4053](https://github.com/microsoft/agent-framework/issues/4053). Require explicit, stable agent IDs for anything that is checkpointed [maf#4793](https://github.com/microsoft/agent-framework/issues/4793), and keep workflow definitions reusable across concurrent runs [maf#3620](https://github.com/microsoft/agent-framework/issues/3620).
16. **Tests as the design spec.** [go#492](https://github.com/microsoft/agent-framework-go/issues/492) shows that a test pinning intended behavior can turn a bug report into agreement. For every "by design" decision Dive makes (approval, persistence, error handling), write a test named for the decision and link it from the docs.

---

## 8. Appendix: high-signal issue index

Reactions (R) and comments (C) as of 2026-09-26. Theme codes: **APR** approvals/HITL, **SES** session/persistence, **WF** workflows/checkpoints, **TOOL** tool calling, **MW** middleware/layering, **PROV** providers/replay, **STR** streaming, **OBS** observability, **DOC** docs, **API** API design/churn, **REQ** feature demand.

### microsoft/agent-framework-go

| # | Title (short) | State | R | C | Theme | Takeaway |
|---|---|---|---|---|---|---|
| [go#28](https://github.com/microsoft/agent-framework-go/issues/28) | `*Options` to `Options` by value | closed | 1 | 0 | API | Small Go idioms are accepted ([go PR #37](https://github.com/microsoft/agent-framework-go/pull/37)). |
| [go#35](https://github.com/microsoft/agent-framework-go/issues/35) | Anthropic tool_use.input passed as string | closed | 0 | 0 | PROV | The first provider-replay bug; typed args needed. |
| [go#96](https://github.com/microsoft/agent-framework-go/issues/96) | anthropicagent structured output | closed | 0 | 1 | TOOL | Maintainer pushed native structured output over a synthetic tool ([go PR #98](https://github.com/microsoft/agent-framework-go/pull/98)). |
| [go#488](https://github.com/microsoft/agent-framework-go/issues/488) | Anthropic args concatenated JSON via workflow executor | closed | 0 | 2 | PROV | A second execution path hid a streaming-accumulation bug. |
| [go#492](https://github.com/microsoft/agent-framework-go/issues/492) | mcptool never checks `IsError` | closed | 0 | 2 | TOOL | Reporter retracted: pass-through to the model is deliberate and test-pinned. |
| [go#493](https://github.com/microsoft/agent-framework-go/issues/493) | otel spans lack token usage and model | closed | 0 | 0 | OBS | Cost is why people trace; fixed by [go PR #495](https://github.com/microsoft/agent-framework-go/pull/495). |
| [go#494](https://github.com/microsoft/agent-framework-go/issues/494) | No `execute_tool` spans | closed | 0 | 1 | OBS | Framework-owned tool spans, on by default ([go PR #446](https://github.com/microsoft/agent-framework-go/pull/446)). |
| [go#513](https://github.com/microsoft/agent-framework-go/issues/513) | Unknown annotation aborts whole message unmarshal | closed | 0 | 1 | PROV | Forward-compatible raw fallbacks everywhere. |
| [go#520](https://github.com/microsoft/agent-framework-go/issues/520) | Proposal: Handoff builder | open | 0 | 0 | REQ | Community demand; implementing PR closed on parity. |
| [go#564](https://github.com/microsoft/agent-framework-go/issues/564) | Magentic builder | open | 0 | 0 | REQ | Same; [go PR #565](https://github.com/microsoft/agent-framework-go/pull/565) closed "purely automated". |
| [go#783](https://github.com/microsoft/agent-framework-go/issues/783) | Responses reasoning replay → 4 stacked 400s | closed | 0 | 0 | PROV | Reasoning round-trip needs conformance tests. |
| [go#949](https://github.com/microsoft/agent-framework-go/issues/949) | Expose tool invocation identity (call ID) | open | 0 | 1 | MW | Enterprise gateways need call ID in middleware ([go PR #1093](https://github.com/microsoft/agent-framework-go/pull/1093)). |
| [go#1067](https://github.com/microsoft/agent-framework-go/issues/1067) | MCP discovery returns only first page | closed | 0 | 1 | TOOL | Handle pagination; fixed same day. |
| [go#1072](https://github.com/microsoft/agent-framework-go/issues/1072) | MCP image result returned as text | closed | 0 | 2 | TOOL | Untyped `any` results lose content type. |
| [go#1076](https://github.com/microsoft/agent-framework-go/issues/1076) | Next tool still runs after cancellation | closed | 0 | 0 | TOOL | Check ctx between sequential calls ([go PR #1077](https://github.com/microsoft/agent-framework-go/pull/1077)). |
| [go#1087](https://github.com/microsoft/agent-framework-go/issues/1087) | Tool-result compaction prints JSON as bytes | closed | 0 | 0 | TOOL | `fmt.Sprint` on `any` results is a trap. |
| [go#1139](https://github.com/microsoft/agent-framework-go/issues/1139) | agent-as-tool session propagation | open | 0 | 1 | TOOL | `Tool.Call(ctx, args)` can't reach the session. |
| [go#1141](https://github.com/microsoft/agent-framework-go/issues/1141) | Complete .NET harness port | open | 0 | 4 | API | Trajectory; "idiomatic Go vs match .NET exactly." |
| [go#1190](https://github.com/microsoft/agent-framework-go/issues/1190) | Is RAG in the plan? | open | 0 | 0 | REQ | Unanswered Go demand. |
| [go PR #496](https://github.com/microsoft/agent-framework-go/pull/496) | Anthropic opt-in prompt caching | closed | – | 19 | PROV | Parity rework demanded; PR died; caching still hard. |
| [go PR #545](https://github.com/microsoft/agent-framework-go/pull/545) / [go PR #565](https://github.com/microsoft/agent-framework-go/pull/565) | Handoff / Magentic builders | closed | – | 14 / 11 | API | "Porting can't be done in a purely automated manner." |
| [go PR #638](https://github.com/microsoft/agent-framework-go/pull/638) | Function-invocation middleware | closed | – | – | MW | Rejected for a .NET-style delegating agent; became [go PR #1093](https://github.com/microsoft/agent-framework-go/pull/1093). |
| [go PR #659](https://github.com/microsoft/agent-framework-go/pull/659) | MCP sampling / list_changed | closed | – | 12 | API | "We try to follow .NET MAF... Closing." |
| [go PR #708](https://github.com/microsoft/agent-framework-go/pull/708) | Unwrap PortableValue at edges | closed | – | 14 | WF | "Not in semantic parity with .NET." |

### microsoft/agent-framework

| # | Title (short) | State | R | C | Theme | Takeaway |
|---|---|---|---|---|---|---|
| [maf#3499](https://github.com/microsoft/agent-framework/issues/3499) | Support for Agent Skills | closed | 33 | 7 | REQ | Top-voted item; standards adoption drives love. |
| [maf#1283](https://github.com/microsoft/agent-framework/issues/1283) | .NET DevUI support | closed | 31 | 15 | REQ | Visual debugging is a top want. |
| [maf#2988](https://github.com/microsoft/agent-framework/issues/2988) | AG-UI dynamic agent resolution | closed | 20 | 9 | REQ | Hosting needs request-time agent selection. |
| [maf#1275](https://github.com/microsoft/agent-framework/issues/1275) | Magentic orchestration | closed | 18 | 10 | REQ | Multi-agent patterns in demand. |
| [maf#728](https://github.com/microsoft/agent-framework/issues/728) | RealtimeAgent / realtime APIs | open | 14 | 9 | REQ | Unmet since 2025-09; users threaten to leave. |
| [maf#5209](https://github.com/microsoft/agent-framework/issues/5209) | Public AG-UI conversion API | closed | 13 | 1 | API | Don't hide useful converters as internal. |
| [maf#4793](https://github.com/microsoft/agent-framework/issues/4793) | Checkpoint resume fails with new agent IDs | closed | 12 | 10 | WF | Checkpoints need explicit stable IDs. |
| [maf#3521](https://github.com/microsoft/agent-framework/issues/3521) | Agent Client Protocol (ACP) | open | 11 | 3 | REQ | Open opportunity. |
| [maf#3620](https://github.com/microsoft/agent-framework/issues/3620) | Singleton workflow → ownership conflicts | closed | 9 | 1 | WF | Definitions must be reusable concurrently. |
| [maf#5460](https://github.com/microsoft/agent-framework/issues/5460) | Options merge omits `Reasoning` | closed | 8 | 3 | MW | Customers blocked; exhaustive merge tests. |
| [maf#2084](https://github.com/microsoft/agent-framework/issues/2084) | DevUI limitations undocumented | open | 8 | 16 | DOC | Docs lag features. |
| [maf#4494](https://github.com/microsoft/agent-framework/issues/4494) | Tool messages dropped with ChatReducer | closed | 7 | 2 | SES | Reducers must keep call/result pairs. |
| [maf#4118](https://github.com/microsoft/agent-framework/issues/4118) | `RunAsync<T>` breaks with function middleware | closed | 7 | 1 | TOOL | Features must compose. |
| [maf#8157](https://github.com/microsoft/agent-framework/issues/8157) | Save partial session on streaming cancel | open | 6 | 1 | SES | UI and session drift after Stop. |
| [maf#3083](https://github.com/microsoft/agent-framework/issues/3083) | In-run dynamic tool loading | closed | 6 | 10 | TOOL | Tool-search pattern; fixed via FICC clone. |
| [maf#2054](https://github.com/microsoft/agent-framework/issues/2054) | Manually add messages to thread | closed | 6 | 15 | SES | Service-stored history blocks history editing. |
| [maf#1445](https://github.com/microsoft/agent-framework/issues/1445) | Distributed workflow runtimes | open | 6 | 2 | WF | In-process only; open since 2025-10. |
| [maf#6167](https://github.com/microsoft/agent-framework/issues/6167) | Version update without changelog | closed | 6 | 2 | API | "No tags on the repository." |
| [maf#3848](https://github.com/microsoft/agent-framework/issues/3848) | Streaming ignores session (regression) | closed | 5 | 2 | STR | Divergent streaming path. |
| [maf#4709](https://github.com/microsoft/agent-framework/issues/4709) | MAF rc4 + MEAI 10.4 incompatible | closed | 4 | 6 | API | Dependency lockstep breaks at runtime. |
| [maf#5707](https://github.com/microsoft/agent-framework/issues/5707) | 1.5.0 breaks Anthropic and Google providers | closed | 4 | 4 | PROV | Same, with `MissingMethodException`. |
| [maf#5538](https://github.com/microsoft/agent-framework/issues/5538) | DeepSeek `reasoning_content` not replayed | closed | 4 | 4 | PROV | Provider-specific reasoning must round-trip. |
| [maf#121](https://github.com/microsoft/agent-framework/issues/121) | Templated instructions | open | 4 | 2 | REQ | Open since 2025-07; top discussion ask. |
| [maf#1569](https://github.com/microsoft/agent-framework/issues/1569) | Tool returning text + image | closed | 0 | 21 | TOOL | Most-commented issue; multimodal results. |
| [maf#2694](https://github.com/microsoft/agent-framework/issues/2694) | ContextId propagation to A2A tools | closed | 0 | 18 | TOOL | Context must flow to tools and sub-agents. |
| [maf#1809](https://github.com/microsoft/agent-framework/issues/1809) | OpenAPI tool in .NET | closed | 3 | 18 | DOC | No clear SK migration path. |
| [maf#726](https://github.com/microsoft/agent-framework/issues/726) | Improve tool assignment from classes | closed | 0 | 18 | TOOL | Boilerplate; API minimalism stance. |
| [maf#8079](https://github.com/microsoft/agent-framework/issues/8079) | `always_require` bypassed by batch order | closed | 0 | 17 | APR | Security bug; classify the whole batch. |
| [maf#1305](https://github.com/microsoft/agent-framework/issues/1305) | WorkflowBuilder needs `ExecutorIsh` | closed | 0 | 16 | API | Implicit conversions and odd names. |
| [maf#3054](https://github.com/microsoft/agent-framework/issues/3054) | Approval applied to all functions | open | 0 | 14 | APR | Per-batch approval "by design"; long pushback. |
| [maf#1057](https://github.com/microsoft/agent-framework/issues/1057) | Structured output feels cumbersome | closed | 3 | 14 | TOOL | "A step down from MEAI and SK." |
| [maf#1710](https://github.com/microsoft/agent-framework/issues/1710) | ChatOptions internal is too restrictive | closed | 0 | 13 | API | Users want per-run mutable agents. |
| [maf#3871](https://github.com/microsoft/agent-framework/issues/3871) | JsonException at SSE stream end | closed | 0 | 13 | STR | Wrapper vs raw SDK behavior differs. |
| [maf#3938](https://github.com/microsoft/agent-framework/issues/3938) | Tool side-effect replay on checkpoint retry | open | 0 | 12 | WF | At-least-once; idempotency pushed to users. |
| [maf#4544](https://github.com/microsoft/agent-framework/issues/4544) | Handoff race / message sandwich | closed | 1 | 12 | WF | Adjacency violations in multi-agent history. |
| [maf#1191](https://github.com/microsoft/agent-framework/issues/1191) | Streaming improvements | closed | 2 | 12 | STR | Workflows only streamed per completed task. |
| [maf#2211](https://github.com/microsoft/agent-framework/issues/2211) | `execute_tool` doesn't capture failure | closed | 0 | 11 | OBS | Logging needed extra hidden wiring. |
| [maf#4808](https://github.com/microsoft/agent-framework/issues/4808) | Pass AgentContext to MCP tool headers | closed | 0 | 11 | TOOL | Per-call header provider added. |
| [maf#2015](https://github.com/microsoft/agent-framework/issues/2015) | Emit `execute_tool` spans | closed | 0 | 9 | OBS | Telemetry had to be enabled twice. |
| [maf#4029](https://github.com/microsoft/agent-framework/issues/4029) | Executors missing from workflow traces | closed | 0 | 15 | OBS | Span nesting breaks under parallelism. |
| [maf#3075](https://github.com/microsoft/agent-framework/issues/3075) | Workflow middleware for telemetry | open | 0 | 9 | OBS | Wants I/O on spans for evals. |
| [maf#7472](https://github.com/microsoft/agent-framework/issues/7472) | Sample loops, 100M+ tokens | closed | 0 | 9 | APR | Unbounded outer loop reset the inner cap. |
| [maf#7866](https://github.com/microsoft/agent-framework/issues/7866) | MCP content + structuredContent duplicated | closed | 0 | 9 | TOOL | 2x tokens; needs an explicit policy. |
| [maf#6264](https://github.com/microsoft/agent-framework/issues/6264) | Non-approval function surfaced for approval | closed | 0 | 9 | APR | Fixed via opt-in session-backed bypass. |
| [maf#2201](https://github.com/microsoft/agent-framework/issues/2201) | Thread storage only filled at run end | closed | 0 | 9 | SES | "Threads are not the way to solve for this." |
| [maf#1266](https://github.com/microsoft/agent-framework/issues/1266) | Sequential passes entire context | closed | 0 | 9 | WF | Context-passing policy must be explicit. |
| [maf#4053](https://github.com/microsoft/agent-framework/issues/4053) | Handoff stale `previous_response_id` | closed | 0 | 9 | WF | Service-history duality bugs. |
| [maf#7358](https://github.com/microsoft/agent-framework/issues/7358) | Compaction registration: three places | open | 0 | 8 | MW | One place per concern. |
| [maf#7862](https://github.com/microsoft/agent-framework/issues/7862) | Pending approvals have no public read API | closed | 1 | 8 | APR | Durable hosts need pending-approval APIs. |
| [maf#2889](https://github.com/microsoft/agent-framework/issues/2889) | Thread not persisted on error / cancel | closed | 0 | 2 | SES | "By design"; the core durability gap. |
| [maf#7215](https://github.com/microsoft/agent-framework/issues/7215) | Persist AG-UI snapshots incrementally | open | 0 | 1 | SES | Production data loss on refresh. |
| [maf#8575](https://github.com/microsoft/agent-framework/issues/8575) | Failed approval-resume corrupts session | closed | 2 | 3 | APR | Commit approval state only on success. |
| [maf#8503](https://github.com/microsoft/agent-framework/issues/8503) | Rejected approvals re-requested without reason | open | 2 | 2 | APR | Rejection text must be final. |
| [maf#6922](https://github.com/microsoft/agent-framework/issues/6922) | FICC drops sibling calls with frontend tool | closed | 0 | 4 | APR | Stateless loop can't mix call kinds. |
| [maf#4879](https://github.com/microsoft/agent-framework/issues/4879) | `never_require` ignored in mixed batch | closed | 0 | 1 | APR | Closed "by design". |
| [maf#6260](https://github.com/microsoft/agent-framework/issues/6260) | Function middleware order inverted | closed | 0 | 3 | MW | Deferred to V2 to avoid a break. |
| [maf#5325](https://github.com/microsoft/agent-framework/issues/5325) | Tools snapshot vs mid-run mutation | closed | 0 | 1 | TOOL | Failures to LLM by design; dynamic tools fixed. |
| [maf#6972](https://github.com/microsoft/agent-framework/issues/6972) | CompactionProvider drops first user message | open | 2 | 2 | SES | In-place mutation bug. |
| [maf#758](https://github.com/microsoft/agent-framework/issues/758) | `ExecutorIsh` confusingly named | closed | 2 | 7 | API | Naming and implicit operators. |
| [maf#775](https://github.com/microsoft/agent-framework/issues/775) | Reconsider AgentThread naming | closed | 0 | 3 | API | Became AgentSession; breaking rename. |
| [maf#8586](https://github.com/microsoft/agent-framework/issues/8586) | Why was `DeleteSessionAsync` removed? | open | 2 | 0 | API | Silent API removal. |
| [maf#6467](https://github.com/microsoft/agent-framework/issues/6467) | Structured output prevents tool calls | closed | 2 | 5 | TOOL | Provider support varies; intermediate text. |
| [maf#4636](https://github.com/microsoft/agent-framework/issues/4636) | Why is .NET slower than Python on Skills? | closed | 0 | 2 | API | Shared-abstraction tax. |
| [maf#5017](https://github.com/microsoft/agent-framework/issues/5017) | Go language support | open | 4 | 1 | REQ | Answered with MAF-Go. |
| [maf disc #1092](https://github.com/microsoft/agent-framework/discussions/1092) | Thoughts on durable execution | – | 3↑ | 13 | WF | Checkpointing vs durable execution debate. |
| [maf disc #1090](https://github.com/microsoft/agent-framework/discussions/1090) | Prompt template support | – | 15↑ | 4 | REQ | Top discussion. |
