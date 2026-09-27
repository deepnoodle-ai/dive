# OpenHands — What the Issue Tracker Says

- **Repos:** `github.com/OpenHands/OpenHands` (formerly `All-Hands-AI/OpenHands`, originally `OpenDevin/OpenDevin`; since mid-2026 it hosts the Agent Canvas frontend) and `github.com/OpenHands/software-agent-sdk` (the V1 SDK, tools, workspaces and Agent Server)
- **Snapshot:** 2026-09-26
- **Companion doc:** [`openhands.md`](./openhands.md) is the source-level analysis. This doc covers user experience as reported in issues and PR threads. Where an issue theme maps to a code-level finding there, this doc says so.

Link shorthand: `#N` is `https://github.com/OpenHands/OpenHands/issues/N`, and `sdk#N` is `https://github.com/OpenHands/software-agent-sdk/issues/N`. Every reference below is a full link. "V0" means the original monolith (the `AgentController` + `EventStream` + `Runtime` design). "V1" means the SDK-based rewrite that shipped at the end of 2025.

---

## 1. Method

### Repo stats

| | OpenHands/OpenHands | OpenHands/software-agent-sdk |
|---|---|---|
| Created | 2024-03-13 | 2025-08-23 (issues go back to 2024-11 because old issues were transferred in) |
| Stars / forks | 89.2k / 11.8k | 1.2k / 569 |
| Issues (all time) | 4,888 | 1,755 |
| Open / closed | 442 / 4,446 | 250 / 1,505 |
| Pull requests | 11,849 | 3,551 |
| Discussions | 0 (disabled; community talk happens in Slack) | 0 (disabled) |
| Median time to close | 12 days | 6 days |
| Issues filed by staff (approx.) / bots / everyone else | ~1,880 / 146 / ~2,860 | ~1,360 / 41 / ~360 |
| Issue comments / comments by bots | 21,452 / ~5,700 | 5,812 / ~3,100 |
| Issues closed by the stale bot | 1,290 | 310 |
| Issues with ≥10 reactions | 20 | 2 |

Monthly volume in the main repo tells the product story: 528 issues in April 2024 (the OpenDevin launch wave), a trough of 46 in November 2025 (the V1 cut-over, when new work moved to the SDK repo), and 321 in August 2026 (the Agent Canvas launch). External (non-staff, non-bot) issues fell from 1,289 in 2024 to 974 in 2025 and 599 so far in 2026.

The SDK repo is **mostly a staff work tracker**: about 77% of its issues come from the core team, and many are filed or written by the team's own agents. That matters when reading it. An SDK issue with 20 comments is usually an internal design thread, not user demand.

### How I found the signal

1. **Reactions are weak here.** The maximum in the main repo is 46 ([#9354](https://github.com/OpenHands/OpenHands/issues/9354), Gitea support), and only 20 issues ever reached 10. The SDK maximum is 29 ([sdk#1060](https://github.com/OpenHands/software-agent-sdk/issues/1060), A2A). Users vote with comments and with duplicates, not thumbs.
2. **Comment count, after removing bots.** Raw counts are polluted. The top "most commented" main-repo issue is [#17546](https://github.com/OpenHands/OpenHands/issues/17546), a port-selection bug with 203 comments, 202 of them written by the `openhands-ai` bot. The next are daily integration-test trackers. I dropped trackers and bot-dominated threads, then ranked by `comments + 3 × reactions`.
3. **Keyword clustering over all titles.** I excluded bot authors and counted issues per theme across both repos. Large clusters of small issues are the strongest signal of pain that many users hit:

   | Theme (title regex) | Main repo | SDK repo |
   |---|---|---|
   | Sandbox / runtime / Docker / SSH | 621 | 120 |
   | LLM provider / model / API key / rate limit | 541 | 273 |
   | Events / WebSocket / streaming / REST | 352 | 242 |
   | Tools / editor / bash / browser / function calling | 246 | 233 |
   | Stuck / loop / iterations / cost | 240 | 80 |
   | Persistence / resume / history / fork | 169 | 129 |
   | Condensation / context window / memory | 98 | 71 |
   | Security / confirmation / permissions | 91 | 60 |
   | MCP | 87 | 56 |
   | Delegation / sub-agents / ACP / A2A | 71 | 131 |
   | Windows / WSL | 78 | 18 |

   Across both repos, "litellm" appears in 111 titles, Ollama/LM Studio/local models in 99, prompt caching in 68, and `security_risk` in 13.
4. **Design threads.** I read the proposals and RFCs that led to breaking changes, found through `proposal`, `architecture`, `roadmap` labels and titles: [#2404](https://github.com/OpenHands/OpenHands/issues/2404), [#8111](https://github.com/OpenHands/OpenHands/issues/8111), [#9585](https://github.com/OpenHands/OpenHands/issues/9585), [#10577](https://github.com/OpenHands/OpenHands/issues/10577), [#10649](https://github.com/OpenHands/OpenHands/issues/10649), [#11528](https://github.com/OpenHands/OpenHands/issues/11528), [#12578](https://github.com/OpenHands/OpenHands/issues/12578), [#14374](https://github.com/OpenHands/OpenHands/issues/14374), [sdk#1451](https://github.com/OpenHands/software-agent-sdk/issues/1451), [sdk#1787](https://github.com/OpenHands/software-agent-sdk/issues/1787), [sdk#1824](https://github.com/OpenHands/software-agent-sdk/issues/1824).
5. **Bot versus human.** OpenHands dogfoods its own agent heavily. The `openhands-agent`/`openhands-ai` accounts fix issues on request ("@openhands-agent any idea?" in [#5480](https://github.com/OpenHands/OpenHands/issues/5480)). `all-hands-bot` posts triage nags, sometimes 20 times on one issue ([sdk#1697](https://github.com/OpenHands/software-agent-sdk/issues/1697), [sdk#3992](https://github.com/OpenHands/software-agent-sdk/issues/3992)). Many staff comments carry "This comment was created by an AI agent (OpenHands) on behalf of the user" (417 in the SDK repo). Several detailed "bug reports" are AI-written audits ([sdk#4157](https://github.com/OpenHands/software-agent-sdk/issues/4157), [sdk#5092](https://github.com/OpenHands/software-agent-sdk/issues/5092)). I treat those as evidence of code hot spots, not of user demand, unless humans corroborate them.

---

## 2. What users love

Direct praise is rare on the tracker, as on most trackers. The love shows up as the kinds of requests people make and the reasons they give.

- **An open, self-hostable Devin.** The first issues after launch are about the idea itself: "Use devin to code opendevin?" ([#4](https://github.com/OpenHands/OpenHands/issues/4), 22 reactions) and "Create a competitive agent with open LLMs" ([#1085](https://github.com/OpenHands/OpenHands/issues/1085), 11 reactions). One user in a bug thread sums up the value: "I can't really complain, it's a FOSS project. Cheaper than paying $500 for Devin" ([#5637](https://github.com/OpenHands/OpenHands/issues/5637)).
- **Bring any model.** The biggest request cluster is people who want to plug in *their* model: Ollama ([#143](https://github.com/OpenHands/OpenHands/issues/143)), LM Studio ([#419](https://github.com/OpenHands/OpenHands/issues/419), 65 comments), GitHub Copilot subscriptions ([#6468](https://github.com/OpenHands/OpenHands/issues/6468), 21 reactions), Azure, OpenRouter, Bedrock. "I really want to use this with local llms so I can tinker with it without worrying about costs" ([#1052](https://github.com/OpenHands/OpenHands/issues/1052)). LiteLLM is what made this cheap to offer. Maintainers' standard answer is "you should be able to use any model that is compatible with litellm" ([#11777](https://github.com/OpenHands/OpenHands/issues/11777)).
- **A real sandbox and a real workspace.** Users want the agent to run code, not just write it. The asks are *more* sandbox, not less: sudo and languages ([#424](https://github.com/OpenHands/OpenHands/issues/424), 36 comments), Podman ([#5325](https://github.com/OpenHands/OpenHands/issues/5325), 24 reactions), Docker-in-Docker ([#5569](https://github.com/OpenHands/OpenHands/issues/5569)), snapshots ([#6163](https://github.com/OpenHands/OpenHands/issues/6163)), and non-Debian images ([#9221](https://github.com/OpenHands/OpenHands/issues/9221)).
- **Git-forge automation.** The single most-reacted issue is Gitea/Forgejo support ([#9354](https://github.com/OpenHands/OpenHands/issues/9354), 46 reactions), followed by GitLab ([#5210](https://github.com/OpenHands/OpenHands/issues/5210), [#7055](https://github.com/OpenHands/OpenHands/issues/7055), [#8603](https://github.com/OpenHands/OpenHands/issues/8603)). The resolver ("label an issue, get a PR") pulled people in, and they wanted it everywhere.
- **Benchmark credibility.** The team led with SWE-bench, and the eval harness is a large, visible part of the tracker (157 eval-titled issues). Users treated the benchmark numbers as the reason to try it.
- **Open extension points that match Claude Code.** Requests to adopt Claude Code's conventions get implemented quickly: hooks ([#9482](https://github.com/OpenHands/OpenHands/issues/9482), shipped in the SDK), `.cursorrules`/`AGENTS.md`-style repo instructions ([#7358](https://github.com/OpenHands/OpenHands/issues/7358)), and MCP ([#5781](https://github.com/OpenHands/OpenHands/issues/5781), 18 reactions). Users like that they can reuse what they already wrote for other agents.
- **Fast, visible responsiveness.** Median close time is 12 days in the main repo and 6 in the SDK. Many fixes ship within days of a report, often by the team's own agent.

---

## 3. What users hate / friction

### 3.1 Getting it to start at all

The largest cluster in the whole tracker is "it won't start". Early threads are OpenDevin-era setup failures with 25 to 50 comments each: "No agent started" ([#1025](https://github.com/OpenHands/OpenHands/issues/1025), 50 comments), SSH/pexpect errors ([#1156](https://github.com/OpenHands/OpenHands/issues/1156), [#911](https://github.com/OpenHands/OpenHands/issues/911), [#2148](https://github.com/OpenHands/OpenHands/issues/2148)), "Initializing agent never completes" ([#1649](https://github.com/OpenHands/OpenHands/issues/1649), [#873](https://github.com/OpenHands/OpenHands/issues/873)), and "Waiting for client to become ready" ([#5968](https://github.com/OpenHands/OpenHands/issues/5968), 45 comments). V1 did not end it. It moved it: "Sandbox failed to start within 120s" ([#12528](https://github.com/OpenHands/OpenHands/issues/12528), 61 comments) and "500 error new conversation" ([#12083](https://github.com/OpenHands/OpenHands/issues/12083), 27 comments) are the two biggest V1-era user threads. On #12528 a maintainer admits "most of us are on MacOS so it's not trivial for us to debug" Linux failures.

### 3.2 Docker as a hard requirement

Running OpenHands meant mounting the host Docker socket so the app could spawn a sandbox container per conversation. Users pushed back on security grounds: mounting the socket "essentially gives full access to the host", and "the whole point of using docker is that I don't have to trust the authors" ([sdk#1563](https://github.com/OpenHands/software-agent-sdk/issues/1563), 34 comments). Startup was slow: "Starting a new thread takes about 15 seconds on a modern laptop" ([#8555](https://github.com/OpenHands/OpenHands/issues/8555), 12 reactions). Podman users were left to share workaround scripts until the stale bot closed the issue as not planned ([#5325](https://github.com/OpenHands/OpenHands/issues/5325)). The team eventually conceded the point in the Agent Canvas announcement: one container per conversation was "complicated, slow, and very error-prone. Worse, many of our would-be users can't use Docker at all" ([#14374](https://github.com/OpenHands/OpenHands/issues/14374)).

### 3.3 Too many configuration systems

Config lived in `config.toml`, environment variables, a web settings page, CLI flags and Python args, and they disagreed. "Configuration behavior is unpredictable" ([#3220](https://github.com/OpenHands/OpenHands/issues/3220)). "Not documented that docker ignores config.toml" ([#11928](https://github.com/OpenHands/OpenHands/issues/11928)). "The search API Key gets reset when you save settings" ([#9497](https://github.com/OpenHands/OpenHands/issues/9497), 24 comments). The maintainers wrote this up themselves as "disconnected configuration systems" ([#9585](https://github.com/OpenHands/OpenHands/issues/9585)), and it became a central motivation for V1 (§6).

### 3.4 Local and weaker models break the agent in model-specific ways

Local-model users hit a stream of failures that look like OpenHands bugs but are really dialect or capability gaps: Ollama native tool calling crashes inside LiteLLM ([sdk#1064](https://github.com/OpenHands/software-agent-sdk/issues/1064)), "Qwen3 thinks correctly but doesn't actually tool call" ([#8140](https://github.com/OpenHands/OpenHands/issues/8140)), `str_replace` errors with Qwen3-coder ([#10039](https://github.com/OpenHands/OpenHands/issues/10039)), ignored context-window settings for Ollama ([#9573](https://github.com/OpenHands/OpenHands/issues/9573)), timeouts for slow local LLMs ([#8768](https://github.com/OpenHands/OpenHands/issues/8768), [sdk#4255](https://github.com/OpenHands/software-agent-sdk/issues/4255)). The sharpest one is [sdk#3992](https://github.com/OpenHands/software-agent-sdk/issues/3992): the SDK treats any prose reply without a tool call as "finished", so weaker models that narrate ("Let me now check X before I proceed...") silently quit mid-task. In a controlled comparison the reporter got 1/4 completions with OpenHands versus 3/4 with mini-swe-agent on the same model. Three earlier fix PRs were not merged, because "bare prose *is* a legitimate completion signal for strong models."

### 3.5 Cost anxiety

"The unknown API costs are a fairly intimidating part of the current user experience. I tried running a relatively simple query and wasn't sure if it was going to cost 1 cent or 10 dollars" ([#5257](https://github.com/OpenHands/OpenHands/issues/5257)). Follow-ups ask for per-task budgets ([sdk#1337](https://github.com/OpenHands/software-agent-sdk/issues/1337)), a context-usage meter ([#7554](https://github.com/OpenHands/OpenHands/issues/7554)), and forking so a user can drop "the context from its 100 circular steps... This would also dramatically help control costs" ([#8560](https://github.com/OpenHands/OpenHands/issues/8560)). A staff issue found that "Claude Code may be cheaper than OpenHands for the same query" because of prompt-cache behavior ([sdk#1808](https://github.com/OpenHands/software-agent-sdk/issues/1808)).

### 3.6 Context-management side effects leak into the task

When the context overflowed, V0 injected "Trimming prompt to meet context window limitations" as an observation. The model read it as an instruction and started trimming the *user's* prompts: "it was quite the nightmare having it condense my prompts every 10 minutes" ([#6634](https://github.com/OpenHands/OpenHands/issues/6634)). Another user reports that turning on condensation "once again made OpenHands unusable in real life" ([#7268](https://github.com/OpenHands/OpenHands/issues/7268)). This is the "policy wearing a message costume" problem that [`openhands.md`](./openhands.md) §9 flags in the SDK's corrective nudges.

### 3.7 The stale bot closes real demand

The stale bot closed 1,290 main-repo issues. Among them are some of the highest-demand requests: Podman ([#5325](https://github.com/OpenHands/OpenHands/issues/5325), 24 reactions, "Up to fight stale label xD"), A2A ([sdk#1060](https://github.com/OpenHands/software-agent-sdk/issues/1060), 29 reactions, closed as not planned while a community PR was in flight), token-based condensation ([#6707](https://github.com/OpenHands/OpenHands/issues/6707)), the VS Code extension ([#2469](https://github.com/OpenHands/OpenHands/issues/2469), stale-closed by the bot twice across 49 comments), lazy MCP connection ([sdk#1418](https://github.com/OpenHands/software-agent-sdk/issues/1418)) and parallel STDIO MCP start-up ([sdk#2413](https://github.com/OpenHands/software-agent-sdk/issues/2413)). Users notice and resent having to bump threads.

### 3.8 Bot noise

The team's use of its own agent makes threads hard to read: 202 bot comments on one port-selection bug ([#17546](https://github.com/OpenHands/OpenHands/issues/17546)), repeated triage nags ([sdk#1697](https://github.com/OpenHands/software-agent-sdk/issues/1697)), and a readiness bot that tells users their bug report needs "a screenshot or video" before it can be worked on ([#16300](https://github.com/OpenHands/OpenHands/issues/16300)).

---

## 4. Big problems

### 4.1 The provider layer is a treadmill, and LiteLLM is both the engine and the risk

LiteLLM gave OpenHands breadth for free, and nearly every provider bug flows through it:
- **Parameter dialects.** Temperature and `top_p` rules differ per model and change per release. An RFC asks whether to "always use the default temperature" because hard-coded `0.0` was "preventing models from being deployed on the cloud, like claude-4.6" ([sdk#1913](https://github.com/OpenHands/software-agent-sdk/issues/1913)). A regression sent both `temperature` and `top_p` to Anthropic through the LiteLLM proxy ([sdk#2686](https://github.com/OpenHands/software-agent-sdk/issues/2686)). Azure GPT-5 rejected `stop` ([sdk#1062](https://github.com/OpenHands/software-agent-sdk/issues/1062)) and a hard-coded temperature ([sdk#986](https://github.com/OpenHands/software-agent-sdk/issues/986)). Bedrock GPT models reject `thinking` ([sdk#5292](https://github.com/OpenHands/software-agent-sdk/issues/5292)).
- **Response-shape drift.** Three open issues in September 2026 are the same crash: `'PromptTokensDetailsWrapper' object has no attribute 'cache_creation_tokens'` for providers without prompt caching ([sdk#5168](https://github.com/OpenHands/software-agent-sdk/issues/5168), [sdk#5213](https://github.com/OpenHands/software-agent-sdk/issues/5213), [sdk#5326](https://github.com/OpenHands/software-agent-sdk/issues/5326)). A LiteLLM wrapper deletes unset fields, and a telemetry helper reads them directly.
- **Reasoning replay.** "Invalid `signature` in `thinking` block" and context-length errors stalled conversations with no visible error ([sdk#1575](https://github.com/OpenHands/software-agent-sdk/issues/1575)). DeepSeek `reasoning_content` needed its own fixes ([sdk#3267](https://github.com/OpenHands/software-agent-sdk/issues/3267)).
- **Supply chain.** In March 2026, LiteLLM versions 1.82.7 and 1.82.8 shipped a credential stealer, PyPI quarantined the package, and OpenHands CI was blocked ([#13573](https://github.com/OpenHands/OpenHands/issues/13573), [#13567](https://github.com/OpenHands/OpenHands/issues/13567)). In August 2026 a LiteLLM release that needed a newer Rust compiler broke fresh Agent Canvas installs ([#16300](https://github.com/OpenHands/OpenHands/issues/16300)).

The SDK added a per-model feature table and a "verified models" list with integration tests per model ([sdk#2849](https://github.com/OpenHands/software-agent-sdk/issues/2849), [sdk#3005](https://github.com/OpenHands/software-agent-sdk/issues/3005)), and a qwen3-coder run showed a "77.1%" conversation error rate before fixes ([sdk#2818](https://github.com/OpenHands/software-agent-sdk/issues/2818)). This matches [`openhands.md`](./openhands.md) §9, which lists "LiteLLM and OpenAI shapes as the core" as an awkward part of the design.

### 4.2 Stuck loops, and the detector that catches the wrong thing

"Agent got stuck in a loop" is a top-five user thread in V0 ([#7183](https://github.com/OpenHands/OpenHands/issues/7183), [#5480](https://github.com/OpenHands/OpenHands/issues/5480), [#2705](https://github.com/OpenHands/OpenHands/issues/2705), [#9645](https://github.com/OpenHands/OpenHands/issues/9645)). Two problems recur:
- **Recovery.** After the detector fired, users "cannot send subsequent messages to the agent anymore" ([#5480](https://github.com/OpenHands/OpenHands/issues/5480)). Once that was fixed, the agent often got stuck again a few steps later, and the maintainer's first question is always "What model are you using?"
- **False positives.** The V1 monologue check (three consecutive agent messages) fired deterministically on extended-thinking models that returned empty responses, failing the same GAIA instance "in 100% of runs" ([sdk#2482](https://github.com/OpenHands/software-agent-sdk/issues/2482)). A condenser browsing loop ([sdk#1063](https://github.com/OpenHands/software-agent-sdk/issues/1063)) happened because the condenser replaced page contents with just the URL, so the agent revisited pages it had "forgotten".

The iteration cap has the same flavor. `max_iterations` is invisible to the model, and hitting it produces a generic `ERROR` "indistinguishable from real errors", so "the LLM never gets a chance to synthesize its findings" ([sdk#2406](https://github.com/OpenHands/software-agent-sdk/issues/2406)). A maintainer's view is that interactive users should simply be able to continue: "a new user message would clear the error and the agent continues for another round." [`openhands.md`](./openhands.md) §6 confirms the cap is a hard `ERROR` at 500.

### 4.3 Tool-call/result pairing corrupts conversations permanently

The most damaging V1 bug class is a history that the provider rejects forever:
- a duplicate `ObservationEvent` on resume makes Anthropic return "`tool_use` ids were found without `tool_result` blocks", and "every subsequent attempt to run the agent fails with the same error" ([sdk#1782](https://github.com/OpenHands/software-agent-sdk/issues/1782), 27 comments);
- a user message sent while a tool is running lands between `tool_use` and `tool_result` ([sdk#1841](https://github.com/OpenHands/software-agent-sdk/issues/1841));
- after an agent-server restart mid-tool, recovery parents the synthetic error to a stale HEAD, and the next turn raises `KeyError` "making the persisted conversation unrecoverable without manual event repair" ([sdk#4487](https://github.com/OpenHands/software-agent-sdk/issues/4487));
- Bedrock rejects mismatched `toolResult` counts ([#4912](https://github.com/OpenHands/OpenHands/issues/4912)).

The team's response is the "view properties" layer (tool pairing, batch atomicity, thinking-block rules) that both constrains condensation and repairs broken histories. [`openhands.md`](./openhands.md) §9 calls it one of the best ideas in the codebase. The issues show why it had to exist.

### 4.4 Persistence is fragile across versions and configurations

- **One unknown event kills the conversation.** Events are discriminated by Python class name. If a custom tool's module is not imported at load time, "one unknown event kind takes down the whole conversation", and it silently 404s ([sdk#4080](https://github.com/OpenHands/software-agent-sdk/issues/4080), open). The team's own Canvas hit this migrating a tool from Python to client-defined JSON ([sdk#4118](https://github.com/OpenHands/software-agent-sdk/issues/4118), open). Custom tools also fail in PyInstaller binary builds because they are loaded by `importlib` ([sdk#1531](https://github.com/OpenHands/software-agent-sdk/issues/1531)).
- **A frozen agent fights real users.** V1 made the Agent immutable for reproducibility, and the result was "quite a number of bug reports on restoring conversations with different settings": model name, reasoning settings, MCP servers, skills ([sdk#1451](https://github.com/OpenHands/software-agent-sdk/issues/1451)). Changing tools mid-conversation was refused outright, so a maintainer proposed forking the conversation whenever tools change ([sdk#1787](https://github.com/OpenHands/software-agent-sdk/issues/1787)).
- **Silent in-memory fallback.** Without a persistence directory, state falls back to memory with no warning, so on Cloud Run "events are lost between requests and there's no indication why" ([sdk#2915](https://github.com/OpenHands/software-agent-sdk/issues/2915)).
- **Full-history scans don't scale.** "Conversations can have like 30k events... currently 1k leads to slowdowns or crashes" ([sdk#1824](https://github.com/OpenHands/software-agent-sdk/issues/1824)).

### 4.5 The security model trusts the model

V1 added a required `security_risk` argument to every tool so the LLM could rate its own actions. Two problems followed:
- **It broke tool calling.** "Missing required parameters for function 'str_replace_editor': {'security_risk'}" hit Claude Sonnet 4, GLM and small models ([#11661](https://github.com/OpenHands/OpenHands/issues/11661), [#11378](https://github.com/OpenHands/OpenHands/issues/11378), [sdk#1653](https://github.com/OpenHands/software-agent-sdk/issues/1653), [sdk#1560](https://github.com/OpenHands/software-agent-sdk/issues/1560), [sdk#1911](https://github.com/OpenHands/software-agent-sdk/issues/1911)). It is still open in one form ([sdk#4248](https://github.com/OpenHands/software-agent-sdk/issues/4248)). The fixes: inject the field only when the LLM analyzer is on ([sdk#300](https://github.com/OpenHands/software-agent-sdk/issues/300)), then make it optional with `UNKNOWN` as the default.
- **It is not a gate.** With the default analyzer, "only actions the model *self-labels* `HIGH` get gated". A maintainer confirmed that a prompt-injected model can label exfiltration `LOW` and it auto-executes ([sdk#4157](https://github.com/OpenHands/software-agent-sdk/issues/4157)). The proposed fix is to compose deterministic rails into the default.
- **Implicit consent.** A user message sent while an action waits for confirmation, even "Wait — do NOT run that", causes the action to execute on the next `run()` ([sdk#5092](https://github.com/OpenHands/software-agent-sdk/issues/5092), open). [`openhands.md`](./openhands.md) §4 traces this to one code path that treats "pending approval", "crashed in flight" and "interrupted" the same way.

Confirmation mode itself drew few user complaints ([#5608](https://github.com/OpenHands/OpenHands/issues/5608), [#8996](https://github.com/OpenHands/OpenHands/issues/8996)). The trouble is in the plumbing around it.

### 4.6 Condensation is necessary and keeps misbehaving

"Long running sessions slow to a crawl!" motivated the condenser ([#5715](https://github.com/OpenHands/OpenHands/issues/5715)). Its follow-ups are a list of edge cases:
- event-count triggers don't track tokens, so "even with condensation, we still exceed model max input token limitations" ([#6707](https://github.com/OpenHands/OpenHands/issues/6707), [sdk#250](https://github.com/OpenHands/software-agent-sdk/issues/250));
- "Cannot condense 0 events" crashes ([sdk#1518](https://github.com/OpenHands/software-agent-sdk/issues/1518), [sdk#2255](https://github.com/OpenHands/software-agent-sdk/issues/2255));
- a model with a 100% failure rate on `NoCondensationAvailableException` ([sdk#2703](https://github.com/OpenHands/software-agent-sdk/issues/2703));
- triggered skills "silently stop applying after condensation" ([sdk#4544](https://github.com/OpenHands/software-agent-sdk/issues/4544));
- summaries that resend the whole history and waste the prompt cache ([sdk#1496](https://github.com/OpenHands/software-agent-sdk/issues/1496)).

Prompt-cache efficiency is its own cluster (68 titles). One example: a timestamp at the top of the system prompt invalidated the cache on every request ([sdk#3690](https://github.com/OpenHands/software-agent-sdk/issues/3690)).

### 4.7 Server concurrency and resource ownership

As the Agent Server became the product, bugs moved from "the agent is dumb" to classic server problems:
- `max_concurrent_runs` doesn't limit the async path, so a burst of conversations can get the server killed ([sdk#4063](https://github.com/OpenHands/software-agent-sdk/issues/4063), open);
- a sub-agent holds the parent's state lock for its whole run, freezing the UI ([sdk#4537](https://github.com/OpenHands/software-agent-sdk/issues/4537), open);
- MCP clients are owned per conversation and not reliably closed, so a long-lived server piles up MCP processes ([sdk#2603](https://github.com/OpenHands/software-agent-sdk/issues/2603));
- STDIO MCP servers start serially under one hard 30-second timeout, so a third server makes conversations never start "and provides no error messages" ([sdk#2413](https://github.com/OpenHands/software-agent-sdk/issues/2413)).

---

## 5. Most desired features (and maintainer response)

| Request | Evidence | Maintainer response |
|---|---|---|
| **More git forges** (Gitea, GitLab, Bitbucket) | [#9354](https://github.com/OpenHands/OpenHands/issues/9354) 46 reactions, [#12351](https://github.com/OpenHands/OpenHands/issues/12351), [#7055](https://github.com/OpenHands/OpenHands/issues/7055), [#5210](https://github.com/OpenHands/OpenHands/issues/5210) | GitLab and Bitbucket shipped. Gitea is left to MCP and the "Integrations Hub" in the cloud product ([#14374](https://github.com/OpenHands/OpenHands/issues/14374)). |
| **MCP** | [#5781](https://github.com/OpenHands/OpenHands/issues/5781) 18 reactions, [#5760](https://github.com/OpenHands/OpenHands/issues/5760), [#7547](https://github.com/OpenHands/OpenHands/issues/7547), [#11881](https://github.com/OpenHands/OpenHands/issues/11881) (OAuth) | Embraced fully. [#10577](https://github.com/OpenHands/OpenHands/issues/10577) made the V1 SDK "MCP-first". Lifecycle and scaling issues remain ([sdk#2603](https://github.com/OpenHands/software-agent-sdk/issues/2603), [sdk#2413](https://github.com/OpenHands/software-agent-sdk/issues/2413)). |
| **Bring existing subscriptions** (Copilot, Claude Code, Codex) | [#6468](https://github.com/OpenHands/OpenHands/issues/6468) 21 reactions | Answered through ACP: Canvas hosts Claude Code, Codex and Gemini CLI as agents ([#14374](https://github.com/OpenHands/OpenHands/issues/14374), [#15746](https://github.com/OpenHands/OpenHands/issues/15746)). ACP is now the fastest-growing SDK cluster (125 titles). |
| **No Docker / Podman / no socket** | [#5325](https://github.com/OpenHands/OpenHands/issues/5325) 24 reactions, [sdk#1563](https://github.com/OpenHands/software-agent-sdk/issues/1563), [#8555](https://github.com/OpenHands/OpenHands/issues/8555) | Ignored for 18 months, then adopted as strategy: one agent-server, runnable directly on a laptop or VM, "Dockerless installation" ([#14374](https://github.com/OpenHands/OpenHands/issues/14374)). |
| **Fork, edit and trim a conversation** | [#8560](https://github.com/OpenHands/OpenHands/issues/8560) 24 comments, [#6163](https://github.com/OpenHands/OpenHands/issues/6163), [#12564](https://github.com/OpenHands/OpenHands/issues/12564) | Forking shipped in V1 as a conversation tree. Editing past events did not. |
| **Switch models mid-conversation** | [#9887](https://github.com/OpenHands/OpenHands/issues/9887), [sdk#416](https://github.com/OpenHands/software-agent-sdk/issues/416), [sdk#781](https://github.com/OpenHands/software-agent-sdk/issues/781) | Shipped, but only after the immutable-agent fight in [sdk#1451](https://github.com/OpenHands/software-agent-sdk/issues/1451). |
| **Cost visibility and budgets** | [#5257](https://github.com/OpenHands/OpenHands/issues/5257), [sdk#1337](https://github.com/OpenHands/software-agent-sdk/issues/1337) | Shipped: cost display and a per-task USD budget. The team debated whether hitting it should block the user or allow one more turn. |
| **Planning, critic, retry** | [#8964](https://github.com/OpenHands/OpenHands/issues/8964), [#8963](https://github.com/OpenHands/OpenHands/issues/8963), [#2221](https://github.com/OpenHands/OpenHands/issues/2221), [#9970](https://github.com/OpenHands/OpenHands/issues/9970) | Staff-driven. Plan mode, a TODO/task tracker and a critic model shipped (the critic is tied to the paid provider). |
| **Structured output from `run()`** | [sdk#1566](https://github.com/OpenHands/software-agent-sdk/issues/1566), [sdk#2566](https://github.com/OpenHands/software-agent-sdk/issues/2566) ("Critical - Blocking my work") | The first request was stale-closed. The second was answered with a schema-typed finish tool, not a new `run()` signature. |
| **Parallel tool execution** | [sdk#2350](https://github.com/OpenHands/software-agent-sdk/issues/2350) | "An old dream of mine... On V0 it was quite complicated and risky." Shipped with resource locks, but sequential by default. |
| **Custom tools on a remote server** | [sdk#1381](https://github.com/OpenHands/software-agent-sdk/issues/1381), [sdk#1531](https://github.com/OpenHands/software-agent-sdk/issues/1531), [sdk#4118](https://github.com/OpenHands/software-agent-sdk/issues/4118) | Moving to JSON "client tools" defined by the caller. The migration path for persisted history is still open. |
| **Immediate interrupt** | [sdk#2208](https://github.com/OpenHands/software-agent-sdk/issues/2208) | Shipped `conversation.interrupt`, because `pause` could take "up to minutes" while a reasoning call finished. |
| **Streaming** (tokens in the UI, bash output) | [#12742](https://github.com/OpenHands/OpenHands/issues/12742), [sdk#1765](https://github.com/OpenHands/software-agent-sdk/issues/1765) 15 reactions | Token streaming shipped. Streaming bash output was dropped: "i barely looked at agent's action observation nowadays." |
| **Observability** (OpenTelemetry) | [#9670](https://github.com/OpenHands/OpenHands/issues/9670) 11 reactions, [sdk#1390](https://github.com/OpenHands/software-agent-sdk/issues/1390) | Deferred until "after the new AgentSDK is stabilized". Laminar/OTel tracing followed. |
| **A2A / OpenAI-compatible agent endpoint** | [sdk#1060](https://github.com/OpenHands/software-agent-sdk/issues/1060) 29 reactions, [sdk#3540](https://github.com/OpenHands/software-agent-sdk/issues/3540) | Welcomed as optional, off-by-default extras "as long as it doesn't get in the way of normal operations". Community-built. |
| **IDE integration** | [#2469](https://github.com/OpenHands/OpenHands/issues/2469) 18 reactions, 49 comments | Repeatedly stale-closed. It was later covered by the VS Code extension and ACP, not built as asked. |

The pattern: requests that fit the team's direction (MCP, forking, budgets, ACP) ship quickly, often built by the team's agent. Requests about deployment environments (Podman, no Docker, Linux-specific failures) waited until the business strategy changed.

---

## 6. Maintainer stance and trajectory

### 6.1 What drove the product's success

- **Timing and framing.** OpenDevin launched in March 2024 as the open answer to Devin and drew 528 issues in its second month. The research team (CMU and UIUC) brought credible SWE-bench numbers and treated the harness as an evaluation platform ([#2140](https://github.com/OpenHands/OpenHands/issues/2140), [#9743](https://github.com/OpenHands/OpenHands/issues/9743)).
- **Model neutrality through LiteLLM.** The agent worked with any provider from day one. That drew a community of local-model tinkerers, even if many of them hit the problems in §3.4.
- **Dogfooding at scale.** The team uses OpenHands to triage, fix and review its own issues and PRs ([#1178](https://github.com/OpenHands/OpenHands/issues/1178), [#5480](https://github.com/OpenHands/OpenHands/issues/5480), [sdk#1913](https://github.com/OpenHands/software-agent-sdk/issues/1913)). That speeds up delivery and sets the agenda: the team fixes what its own agent runs into.
- **Fast adoption of emerging conventions.** Microagents became skills, and the team followed MCP, Claude Code hooks, `AGENTS.md`, ACP and plan mode as each appeared.

### 6.2 Why V0 had to be rewritten

The rewrite was argued in the open, mostly by the core team. The reasons, in their own words:

1. **Too many entry points, each configured differently.** "There are many different ways to run OpenHands today: the UI, the CLI, headless mode, Python code (sorta), the evaluation pipeline... Worse, each of these has its own way of configuring OpenHands" ([#10577](https://github.com/OpenHands/OpenHands/issues/10577)). See also [#9585](https://github.com/OpenHands/OpenHands/issues/9585).
2. **Global state everywhere.** A maintainer describes an agent spending "1h silent work... *only to fix CI*" because of "globals on import paths" ([#10577](https://github.com/OpenHands/OpenHands/issues/10577)). The V1 requirements say "No reliance on global state: no config files, no environment variables."
3. **An overgrown controller.** "`agent_controller.py` has grown too much. It got over 1,400 lines of code, and it does a lot of things" ([#8111](https://github.com/OpenHands/OpenHands/issues/8111)). Context-window truncation lived there as a hidden second condenser.
4. **A monorepo that "has grown like a weed."** Prompts, agent logic, web server, frontend, runtimes, CLI and evals were "mingled". "CI/CD takes forEVER... Package/image sizes are massive" ([#10649](https://github.com/OpenHands/OpenHands/issues/10649)). One maintainer measured the repo at "~5 million tokens".
5. **Dependency weight.** The V1 target was "< 1GB of dependencies, no docker dependency, no browser dependency", with tools as a separate package ([#10577](https://github.com/OpenHands/OpenHands/issues/10577)).
6. **The dual Action/ToolCall model.** In V0 the agent emitted typed Actions onto an async `EventStream`, and the runtime answered with Observations. The proposal was: "The EventStream goes away in the new world. Instead, the agent makes ToolCalls directly, and gets the ToolResults synchronously." A maintainer agreed the old model was "dual (redundant)... an Action contains both its 'regular' properties AND the full tool call metadata" ([#10577](https://github.com/OpenHands/OpenHands/issues/10577)).
7. **Claude Code as the reference.** "Claude Code is a good example here—it's a tightly packaged, self-contained agent, which can be deployed and run in a wide variety of contexts" ([#10577](https://github.com/OpenHands/OpenHands/issues/10577)).

V1's headline API is `Conversation(agent).send_message(...).run()`. It is synchronous and blocking, and the caller wraps it in a thread or asyncio. The SDK later grew an async twin anyway, and [`openhands.md`](./openhands.md) §9 counts "every path exists twice, sync and async" among its main costs. The migration broke the public REST API: V1 split conversations from sandboxes and dropped several endpoints (workspace zip, file upload, trajectory) ([#12578](https://github.com/OpenHands/OpenHands/issues/12578)).

This was the second architectural reset. The first, in mid-2024, replaced SSH into the sandbox with a runtime client inside the container that spoke the event stream. The goal was to support "arbitrary docker image" sandboxes and remove `sshd` ([#2404](https://github.com/OpenHands/OpenHands/issues/2404)). That fix is why the SSH-era cluster in §3.1 disappears after 2024.

### 6.3 V1 in practice: immutability, then relaxation

V1 froze the Agent for reproducibility and serializability. Within months a maintainer reported that the freeze "seemed to imply LLM immutability, agent context immutability... It's useful for reproducibility, but it's maybe not so good for user experience. As of now, we seem to patch things one by one" ([sdk#1451](https://github.com/OpenHands/software-agent-sdk/issues/1451)). The compromise is "composable, of immutable parts": swap whole instances in one place, and don't mutate fields. The same thinking produced "fork on tool change" ([sdk#1787](https://github.com/OpenHands/software-agent-sdk/issues/1787)).

### 6.4 The 2026 pivot: Agent Canvas

In May 2026 the team announced, after user interviews, that ([#14374](https://github.com/OpenHands/OpenHands/issues/14374)):
- the flagship repo becomes Agent Canvas, a local frontend that can "bring your own agent (OpenHands, Claude Code, Codex)" over ACP;
- enterprise code leaves the open-source repo because it "harms our reputation as an open source project";
- the default backend becomes one agent-server with git worktrees instead of one container per conversation;
- the CLI moves to "sandbox" status: "The OpenHands team has been stretched thin trying to support many different ways of using OpenHands";
- "The SDK will continue to get strong, official support from us, since it sits at the foundation of everything we do. But it's an advanced step along the user journey, not the starting point."

So the trajectory is: coding agent → platform with four front doors → rewrite to a core SDK → an agent-neutral control plane where OpenHands' own harness is one option among several. The SDK is now a well-supported but staff-driven library. External SDK issues are only about 360 in total, and most external traffic still arrives through the app.

### 6.5 Recurring design stances

- **Model quality comes first.** Many loop and tool-call complaints are answered with "What model are you using?" ([#5480](https://github.com/OpenHands/OpenHands/issues/5480), [#7183](https://github.com/OpenHands/OpenHands/issues/7183), [#5637](https://github.com/OpenHands/OpenHands/issues/5637)). Default policies are tuned for frontier models, and weaker models get opt-ins ([sdk#3992](https://github.com/OpenHands/software-agent-sdk/issues/3992)).
- **Evals decide.** Behavior changes like parallel tools, temperature defaults and iteration limits are gated on benchmark runs ([sdk#2350](https://github.com/OpenHands/software-agent-sdk/issues/2350), [sdk#1913](https://github.com/OpenHands/software-agent-sdk/issues/1913), [sdk#2406](https://github.com/OpenHands/software-agent-sdk/issues/2406)).
- **Opt-in hardening.** Security rails, content-response policies and parallelism ship off by default, and the defaults stay as they were.

---

## 7. Lessons for Dive

These are ordered by how much user pain they would have prevented. Where [`openhands.md`](./openhands.md) makes the same recommendation from the code, I say so.

### P0: get these right from day one

1. **Make the history valid by construction, and repairable.** Tool-call/result pairing corruption was the most destructive V1 bug class, and it is permanent once persisted ([sdk#1782](https://github.com/OpenHands/software-agent-sdk/issues/1782), [sdk#1841](https://github.com/OpenHands/software-agent-sdk/issues/1841), [sdk#4487](https://github.com/OpenHands/software-agent-sdk/issues/4487)). Dive's projection should enforce provider invariants (pairing, ordering, thinking-block rules), and user messages that arrive mid-tool should queue behind results. A cold load should repair a broken record and record the repair. This is `openhands.md` recommendation 2 ("view properties as a first-class `projection` package").
2. **Keep pending approval, in-flight tools and interrupted tools as distinct states.** A single "unmatched action" path caused implicit consent ([sdk#5092](https://github.com/OpenHands/software-agent-sdk/issues/5092)) and at-least-once re-execution after a crash ([sdk#4487](https://github.com/OpenHands/software-agent-sdk/issues/4487)). A new user message while waiting must never count as approval. This is `openhands.md` recommendations 3 and 4.
3. **Tolerate unknown events on load.** Use stable, explicit `Kind` strings with an opaque fallback, so a missing tool type degrades one event instead of hiding the whole conversation ([sdk#4080](https://github.com/OpenHands/software-agent-sdk/issues/4080), [sdk#4118](https://github.com/OpenHands/software-agent-sdk/issues/4118)). No process-global registries, and no dynamic imports ([sdk#1531](https://github.com/OpenHands/software-agent-sdk/issues/1531)). Go's static binaries make this easier to get right.
4. **Own the provider layer, with dialects as data.** LiteLLM bought OpenHands breadth, and it also brought a steady stream of bugs, a supply-chain compromise and a Rust build dependency ([§4.1](#41-the-provider-layer-is-a-treadmill-and-litellm-is-both-the-engine-and-the-risk)). Dive already speaks providers natively. The lessons:
   - parameter rules (temperature, `top_p`, `stop`, reasoning effort) belong in per-model capability tables, not in code branches;
   - "send no temperature" should be the default ([sdk#1913](https://github.com/OpenHands/software-agent-sdk/issues/1913));
   - usage parsing must tolerate missing cache fields ([sdk#5168](https://github.com/OpenHands/software-agent-sdk/issues/5168));
   - thinking signatures must round-trip exactly ([sdk#1575](https://github.com/OpenHands/software-agent-sdk/issues/1575));
   - run an integration suite per verified model ([sdk#2849](https://github.com/OpenHands/software-agent-sdk/issues/2849)).
5. **Configuration is a value passed in.** No config files and no environment reads inside the library. That was the V1 lesson in its own words ([#10577](https://github.com/OpenHands/OpenHands/issues/10577), [#9585](https://github.com/OpenHands/OpenHands/issues/9585)). Fail loudly instead of silently falling back to memory ([sdk#2915](https://github.com/OpenHands/software-agent-sdk/issues/2915)).
6. **Surface every LLM error.** No silent stalls ([sdk#1575](https://github.com/OpenHands/software-agent-sdk/issues/1575)), and no validation errors that make the user read raw tool arguments ([sdk#1653](https://github.com/OpenHands/software-agent-sdk/issues/1653)). Classify errors as retryable, agent-fixable or fatal, and send agent-fixable ones back to the model.

### P1: the differentiators users begged OpenHands for

7. **Limits the model can see, and a graceful stop.** Tell the model its step and cost budget, warn it near the limit, and end with a typed `Limited` stop that includes a final no-tools answer, not a generic `ERROR` ([sdk#2406](https://github.com/OpenHands/software-agent-sdk/issues/2406), [sdk#1337](https://github.com/OpenHands/software-agent-sdk/issues/1337)). Interactive hosts should be able to continue after a limit. This is `openhands.md` recommendation 13 ("stop gracefully with `Limited`").
8. **Stuck detection as a stock Policy that is model-aware.** OpenHands shows both that stuck detection is needed ([#7183](https://github.com/OpenHands/OpenHands/issues/7183)) and that naive rules misfire, for example counting empty reasoning-only replies as a monologue ([sdk#2482](https://github.com/OpenHands/software-agent-sdk/issues/2482)). Ship configurable detectors, nudge before stopping, and treat "prose without a tool call" as a Policy decision (finish, nudge or require a tool) instead of a hard-coded rule ([sdk#3992](https://github.com/OpenHands/software-agent-sdk/issues/3992)). This is `openhands.md` recommendation 8.
9. **Library nudges are context items, never fake user or observation text.** "Trimming prompt to meet context window limitations" made the agent trim the user's prompts ([#6634](https://github.com/OpenHands/OpenHands/issues/6634)). Anything the library injects must be clearly labeled as coming from the harness. This is `openhands.md` recommendation 9.
10. **Real gates for approval, not self-assessment.** A model-predicted risk is a useful *input*, but it must be optional in the schema, because making it required broke tool calls across models ([#11661](https://github.com/OpenHands/OpenHands/issues/11661)). It must never be the only gate ([sdk#4157](https://github.com/OpenHands/software-agent-sdk/issues/4157)). Approvals should be per call. This is `openhands.md` recommendation 5.
11. **Token-aware, cache-aware compaction.** Trigger on tokens, not event counts ([#6707](https://github.com/OpenHands/OpenHands/issues/6707)). Keep dynamic content (timestamps and similar) at the end of the prefix ([sdk#3690](https://github.com/OpenHands/software-agent-sdk/issues/3690)). Summarize without discarding the cache ([sdk#1496](https://github.com/OpenHands/software-agent-sdk/issues/1496)). Re-apply triggered instructions after compaction ([sdk#4544](https://github.com/OpenHands/software-agent-sdk/issues/4544)). Handle the degenerate cases, such as nothing to condense ([sdk#1518](https://github.com/OpenHands/software-agent-sdk/issues/1518)). Cost parity with Claude Code on the same query is a fair test ([sdk#1808](https://github.com/OpenHands/software-agent-sdk/issues/1808)).
12. **Change configuration mid-conversation by swapping whole values.** Users want to switch models, add MCP servers and change skills mid-conversation ([#9887](https://github.com/OpenHands/OpenHands/issues/9887), [sdk#1451](https://github.com/OpenHands/software-agent-sdk/issues/1451)). Record a versioned definition per turn. Allow compatible changes, such as a new model or added tools. Offer forking for incompatible ones ([sdk#1787](https://github.com/OpenHands/software-agent-sdk/issues/1787)).
13. **Structured results as a first-class return type.** Users asked for a typed result from `run()` ([sdk#1566](https://github.com/OpenHands/software-agent-sdk/issues/1566), [sdk#2566](https://github.com/OpenHands/software-agent-sdk/issues/2566)). In Go this is a generic `Run[T]` or a schema'd finish tool that decodes into `T`.

### P2: ergonomics and operations that compound

14. **Cancellation that actually cancels.** `pause` that waits minutes for a reasoning call is not a stop ([sdk#2208](https://github.com/OpenHands/software-agent-sdk/issues/2208)). In Go, `ctx` cancellation must reach the HTTP call and the tool.
15. **Resource ownership for MCP and sub-agents.** Share MCP clients across conversations, start them in parallel and lazily, time out each server separately with a clear error, and always close them ([sdk#2603](https://github.com/OpenHands/software-agent-sdk/issues/2603), [sdk#2413](https://github.com/OpenHands/software-agent-sdk/issues/2413), [sdk#1418](https://github.com/OpenHands/software-agent-sdk/issues/1418)). Sub-agents must not hold the parent's lock for their whole run ([sdk#4537](https://github.com/OpenHands/software-agent-sdk/issues/4537)). Admission limits must cover every execution path ([sdk#4063](https://github.com/OpenHands/software-agent-sdk/issues/4063)).
16. **Parallel tools with declared resources.** Users and staff wanted it for years ([sdk#2350](https://github.com/OpenHands/software-agent-sdk/issues/2350)). Read-only calls should run in parallel, and writes should take locks. This is `openhands.md` recommendation 6.
17. **Replaceable built-ins.** Every built-in tool (think, finish) should be optional ([sdk#1592](https://github.com/OpenHands/software-agent-sdk/issues/1592)). Host-defined tools must work when the loop runs remotely ([sdk#1381](https://github.com/OpenHands/software-agent-sdk/issues/1381)).
18. **History access that scales.** Page or cursor through events. Never load the whole history to render or to build a prompt ([sdk#1824](https://github.com/OpenHands/software-agent-sdk/issues/1824)).
19. **OpenTelemetry from day one** ([#9670](https://github.com/OpenHands/OpenHands/issues/9670)). It was deferred for a year because the architecture was changing underneath it.
20. **Protocol adapters stay optional.** A2A, ACP and an OpenAI-compatible endpoint are all requested ([sdk#1060](https://github.com/OpenHands/software-agent-sdk/issues/1060), [sdk#3540](https://github.com/OpenHands/software-agent-sdk/issues/3540), [#15746](https://github.com/OpenHands/OpenHands/issues/15746)). Keep them as off-by-default adapter packages without hard dependencies, which is the maintainers' own condition.

### Strategic lessons

- **One front door.** OpenHands spent two years supporting five entry points with five config systems, then rewrote the core and demoted most of them ([#10577](https://github.com/OpenHands/OpenHands/issues/10577), [#14374](https://github.com/OpenHands/OpenHands/issues/14374)). Dive should be a library first. Products built on it (Nvoken, CLIs) are consumers of the library, not alternative cores.
- **Don't make a container runtime the price of entry.** The one-container-per-conversation model generated the largest failure cluster in the tracker and was finally abandoned ([§3.1](#31-getting-it-to-start-at-all), [§3.2](#32-docker-as-a-hard-requirement)). Dive should run the loop in-process, next to the tools, and treat sandboxing as the host's choice.
- **Tune defaults for strong models, and give weaker ones policies, not forks.** Many OpenHands threads are "this breaks on my model". A Policy seam for finish detection, tool-call requirements and nudges lets users fix that without patching the library.
- **Keep the tracker human.** OpenHands shows the cost of stale bots and agent-written threads: high-demand issues closed as "not planned", and signal buried under bot comments. If Dive uses agents for triage, keep their output separate from human discussion.

---

## 8. Appendix: high-signal issue index

Reactions (Rx) and comments (C) are as of 2026-09-26. Comment counts include bot comments.

| Repo | # | Title | State | Rx | C | Theme | Takeaway |
|---|---|---|---|---|---|---|---|
| main | [4](https://github.com/OpenHands/OpenHands/issues/4) | Use devin to code opendevin? | Closed | 22 | 8 | Success | The launch framing: an open Devin. |
| main | [1085](https://github.com/OpenHands/OpenHands/issues/1085) | Create a competitive agent with open LLMs | Closed | 11 | 15 | Providers | Demand for open-model parity from the start. |
| main | [9354](https://github.com/OpenHands/OpenHands/issues/9354) | Gitea/Forgejo Support | Closed | 46 | 14 | Integrations | Most-reacted issue; git-forge automation drew users. |
| main | [6468](https://github.com/OpenHands/OpenHands/issues/6468) | Add GitHub Copilot provider | Closed | 21 | 26 | Providers | Users want to bring existing subscriptions; later answered by ACP. |
| main | [5781](https://github.com/OpenHands/OpenHands/issues/5781) | Allow us to use MCP servers | Closed | 18 | 22 | MCP | MCP was "table stakes"; embraced. |
| main | [7547](https://github.com/OpenHands/OpenHands/issues/7547) | Proposal: Simplify microagents + support MCP natively | Closed | 4 | 24 | Skills/MCP | Custom extension formats confused users; moved to standards. |
| main | [5325](https://github.com/OpenHands/OpenHands/issues/5325) | Support podman | Closed (not planned) | 24 | 26 | Sandbox | High demand, stale-closed; users traded workarounds. |
| sdk | [1563](https://github.com/OpenHands/software-agent-sdk/issues/1563) | Allow running in docker without mounting docker socket | Closed | 2 | 34 | Sandbox/security | Socket mounting "gives full access to the host". |
| main | [8555](https://github.com/OpenHands/OpenHands/issues/8555) | Slow docker sandbox startup | Closed | 12 | 10 | Sandbox | 15-second conversation start; "much faster in v1". |
| main | [1025](https://github.com/OpenHands/OpenHands/issues/1025) | No agent started. Please wait a second. | Closed | 0 | 50 | Setup | Emblematic early setup failure. |
| main | [12528](https://github.com/OpenHands/OpenHands/issues/12528) | Sandbox failed to start within 120s | Closed | 2 | 61 | Sandbox (V1) | Biggest V1 user thread; Linux hard to debug for a macOS team. |
| main | [12083](https://github.com/OpenHands/OpenHands/issues/12083) | 500 error new conversation | Closed | 5 | 27 | Setup (V1) | V0/V1 coexistence confused self-hosters. |
| main | [2404](https://github.com/OpenHands/OpenHands/issues/2404) | Deprecating SSH-based communication and use EventStream | Closed | 6 | 9 | Architecture | First reset: runtime client in the sandbox, arbitrary images. |
| main | [8111](https://github.com/OpenHands/OpenHands/issues/8111) | Refactor agent controller | Closed | 3 | 10 | Architecture | 1,400-line controller with hidden truncation logic. |
| main | [9585](https://github.com/OpenHands/OpenHands/issues/9585) | Unify Configuration Architecture | Closed | 4 | 7 | Config | Parallel config systems named as a root problem. |
| main | [10577](https://github.com/OpenHands/OpenHands/issues/10577) | Proposal: Minimal Python SDK | Closed | 7 | 16 | V1 rewrite | The V1 charter: no globals, sync `run()`, MCP-first, light deps. |
| main | [10649](https://github.com/OpenHands/OpenHands/issues/10649) | Proposal: Separate Repositories | Closed | 6 | 27 | V1 rewrite | Monorepo "grown like a weed"; split into SDK/CLI/app/benchmarks. |
| main | [12578](https://github.com/OpenHands/OpenHands/issues/12578) | V0 to V1 Migration Guide | Closed | 1 | 9 | API | V1 decoupled sandboxes from conversations; dropped endpoints. |
| main | [14374](https://github.com/OpenHands/OpenHands/issues/14374) | Agent Canvas Initiative | Closed | 20 | 6 | Strategy | Pivot: Dockerless, bring-your-own-agent via ACP, CLI deprioritized. |
| main | [9689](https://github.com/OpenHands/OpenHands/issues/9689) | [EPIC] Open the closed hands | Open | 0 | 29 | Roadmap | Community parity checklist vs Devin/Manus/Factory. |
| main | [5480](https://github.com/OpenHands/OpenHands/issues/5480) | Cannot recover from "Agent stuck in loop" | Closed | 0 | 27 | Loops | Stuck state blocked further input; loops recur by model. |
| main | [7183](https://github.com/OpenHands/OpenHands/issues/7183) | AgentStuckInLoopError | Closed | 5 | 17 | Loops | Hit on Sonnet 3.7, Haiku and o3-mini alike. |
| main | [5715](https://github.com/OpenHands/OpenHands/issues/5715) | Memory Condensation | Closed | 6 | 23 | Context | "Long running sessions slow to a crawl!" |
| main | [6707](https://github.com/OpenHands/OpenHands/issues/6707) | Token-based condensation triggers | Closed (stale) | 5 | 3 | Context | Event counts don't bound tokens. |
| main | [6634](https://github.com/OpenHands/OpenHands/issues/6634) | "Trimming prompt..." message making agent change actions | Closed | 0 | 19 | Context | Injected harness text hijacked the task. |
| main | [5257](https://github.com/OpenHands/OpenHands/issues/5257) | Display API costs in frontend | Closed | 4 | 15 | Cost | "1 cent or 10 dollars?" |
| main | [8560](https://github.com/OpenHands/OpenHands/issues/8560) | [PRD] Fork a Conversation | Closed | 7 | 24 | Persistence | Users want to edit, fork and trim history. |
| main | [9887](https://github.com/OpenHands/OpenHands/issues/9887) | Switching models within a conversation | Closed | 6 | 11 | Providers | Workaround was stop/restart; later shipped. |
| main | [11661](https://github.com/OpenHands/OpenHands/issues/11661) | Missing required parameters ... {'security_risk'} | Closed | 7 | 16 | Security/tools | A required self-risk field broke tool calls. |
| main | [2469](https://github.com/OpenHands/OpenHands/issues/2469) | VSCode Extension for Prompt Requests | Closed | 18 | 49 | IDE | Stale-closed repeatedly despite demand. |
| main | [13573](https://github.com/OpenHands/OpenHands/issues/13573) | Compromised LiteLLM dependency | Closed | 0 | 1 | Supply chain | LiteLLM 1.82.7/1.82.8 shipped a credential stealer. |
| main | [16300](https://github.com/OpenHands/OpenHands/issues/16300) | litellm build fails with rustc version mismatch | Open | 0 | 4 | Dependencies | A transitive Rust build broke fresh installs. |
| sdk | [1060](https://github.com/OpenHands/software-agent-sdk/issues/1060) | Google a2a support | Closed (not planned) | 29 | 21 | Protocols | Top SDK reaction; allowed as optional extra. |
| sdk | [1765](https://github.com/OpenHands/software-agent-sdk/issues/1765) | Support for streaming for bash commands | Closed | 15 | 14 | Streaming | Dropped; team no longer watches observations. |
| sdk | [1451](https://github.com/OpenHands/software-agent-sdk/issues/1451) | Proposal: agent as composed of immutable instances | Closed | 3 | 17 | Config | V1's frozen agent caused restore bugs; swap whole parts. |
| sdk | [1787](https://github.com/OpenHands/software-agent-sdk/issues/1787) | Proposal: Fork conversation when tools change | Closed | 0 | 23 | Persistence | One conversation = one system prompt = one tool set. |
| sdk | [1824](https://github.com/OpenHands/software-agent-sdk/issues/1824) | Don't use full events history | Closed | 1 | 14 | Scale | 1k events caused slowdowns; plan for 30k. |
| sdk | [1782](https://github.com/OpenHands/software-agent-sdk/issues/1782) | Duplicate ObservationEvent ... on conversation resume | Closed | 0 | 27 | Pairing | Permanent 400s after resume. |
| sdk | [1841](https://github.com/OpenHands/software-agent-sdk/issues/1841) | send_message() during pending tool execution corrupts history | Closed | 1 | 7 | Pairing | Mid-tool user messages break provider ordering. |
| sdk | [4487](https://github.com/OpenHands/software-agent-sdk/issues/4487) | Crash recovery orphans interrupted tool result | Closed | 1 | 2 | Durability | Restart mid-tool made conversations unrecoverable. |
| sdk | [4080](https://github.com/OpenHands/software-agent-sdk/issues/4080) | One unregistered event kind fails the entire conversation load | Open | 0 | 13 | Persistence | Class-name discriminators plus strict load = data loss. |
| sdk | [4118](https://github.com/OpenHands/software-agent-sdk/issues/4118) | Migration path from Python custom tools to client-defined tools | Open | 1 | 2 | Tools | Tool evolution vs persisted history is unsolved. |
| sdk | [1531](https://github.com/OpenHands/software-agent-sdk/issues/1531) | Custom tools do not work with binary agent server builds | Closed | 0 | 13 | Tools | Dynamic imports don't survive packaging. |
| sdk | [1381](https://github.com/OpenHands/software-agent-sdk/issues/1381) | Custom tool support for remote agent server | Closed | 0 | 12 | Tools | Host tools must work when the loop is remote. |
| sdk | [2915](https://github.com/OpenHands/software-agent-sdk/issues/2915) | InMemoryFileStore fallback is silent | Closed | 1 | 1 | Persistence | Silent fallback lost events on Cloud Run. |
| sdk | [2406](https://github.com/OpenHands/software-agent-sdk/issues/2406) | Make Max Iteration Limit visible ... Graceful Termination | Closed | 0 | 13 | Limits | Hard cap loses work and looks like a crash. |
| sdk | [2482](https://github.com/OpenHands/software-agent-sdk/issues/2482) | Monologue detector false positive on extended thinking | Closed | 0 | 9 | Loops | Stuck rules must be model-aware. |
| sdk | [3992](https://github.com/OpenHands/software-agent-sdk/issues/3992) | Content-without-tool-call terminates weaker/local models | Closed | 0 | 15 | Loop policy | Prose = finish rule ends local-model runs early. |
| sdk | [1337](https://github.com/OpenHands/software-agent-sdk/issues/1337) | Support for max_budget_per_task | Closed | 0 | 9 | Cost | Budget semantics differ for interactive vs headless. |
| sdk | [1653](https://github.com/OpenHands/software-agent-sdk/issues/1653) | Missing security_risk ... unrecoverable validation error | Closed | 0 | 21 | Security/tools | Made optional with `UNKNOWN` default. |
| sdk | [300](https://github.com/OpenHands/software-agent-sdk/issues/300) | Only inject security_risk when LLM-risk analyzer enabled | Closed | 2 | 1 | Security | The field hurt open-source models. |
| sdk | [4157](https://github.com/OpenHands/software-agent-sdk/issues/4157) | LLMSecurityAnalyzer trusts model self-assessed risk | Closed | 0 | 10 | Security | Self-rating is not a gate; compose rails. |
| sdk | [5092](https://github.com/OpenHands/software-agent-sdk/issues/5092) | Any user message implicitly approves pending actions | Open | 0 | 4 | Approval | "Do NOT run that" still ran it. |
| sdk | [1063](https://github.com/OpenHands/software-agent-sdk/issues/1063) | Memory Condensation ends up on loop when browsing | Closed | 3 | 17 | Context | Masked pages invite revisiting loops. |
| sdk | [2703](https://github.com/OpenHands/software-agent-sdk/issues/2703) | NoCondensationAvailableException: 100% failure rate | Closed | 0 | 18 | Context | Condenser edge cases fail whole runs. |
| sdk | [1575](https://github.com/OpenHands/software-agent-sdk/issues/1575) | Conversations stalling due to unhandled LLM API errors | Closed | 0 | 15 | Errors | Thinking-signature and context errors hung silently. |
| sdk | [3690](https://github.com/OpenHands/software-agent-sdk/issues/3690) | Move DateTime block to end of prompt for cache reuse | Closed | 0 | 20 | Caching | Dynamic content at the top kills the cache. |
| sdk | [1808](https://github.com/OpenHands/software-agent-sdk/issues/1808) | Prompt caching differences between Claude Code and OpenHands | Closed | 0 | 13 | Cost | Same query cost more than Claude Code. |
| sdk | [1913](https://github.com/OpenHands/software-agent-sdk/issues/1913) | RFC: Always set temperature to default? | Closed | 0 | 18 | Providers | Hard-coded sampling params break new models. |
| sdk | [1064](https://github.com/OpenHands/software-agent-sdk/issues/1064) | LLM_NATIVE_TOOL_CALLING doesn't work with Ollama | Closed | 1 | 16 | Providers | Local dialect failures surface inside LiteLLM. |
| sdk | [2686](https://github.com/OpenHands/software-agent-sdk/issues/2686) | LiteLLM proxy sends both temperature and top_p to Anthropic | Closed | 0 | 11 | Providers | Parameter-rule regressions through a proxy. |
| sdk | [5168](https://github.com/OpenHands/software-agent-sdk/issues/5168) | AttributeError in Telemetry._cache_buckets | Open | 1 | 14 | Providers | Usage-shape drift crashes non-caching providers. |
| sdk | [2413](https://github.com/OpenHands/software-agent-sdk/issues/2413) | STDIO MCP Usage is not currently scalable | Closed (not planned) | 0 | 6 | MCP | Serial start plus 30 s timeout; a third server hangs. |
| sdk | [1418](https://github.com/OpenHands/software-agent-sdk/issues/1418) | Lazy MCP Server Connection via Adapter Tool | Closed (not planned) | 0 | 6 | MCP | Don't authenticate servers you won't use. |
| sdk | [2603](https://github.com/OpenHands/software-agent-sdk/issues/2603) | MCP lifecycle is conversation-owned and not cleaned up | Closed | 0 | 5 | MCP/ops | MCP processes pile up on long-lived servers. |
| sdk | [2350](https://github.com/OpenHands/software-agent-sdk/issues/2350) | Proposal: Parallel Tool Execution | Closed | 2 | 12 | Tools | "Old dream"; shipped with locks, off by default. |
| sdk | [2208](https://github.com/OpenHands/software-agent-sdk/issues/2208) | Add ability to immediately terminate agent | Closed | 2 | 4 | Cancellation | Pause waited minutes; added interrupt. |
| sdk | [1592](https://github.com/OpenHands/software-agent-sdk/issues/1592) | Create a way to exclude built in tools | Closed | 1 | 12 | Tools | Built-ins must be replaceable. |
| sdk | [2566](https://github.com/OpenHands/software-agent-sdk/issues/2566) | Structured Output | Closed | 0 | 16 | API | "Blocking my work"; answered with a typed finish tool. |
| sdk | [1566](https://github.com/OpenHands/software-agent-sdk/issues/1566) | Blocking structured output calls | Closed (stale) | 2 | 2 | API | Typed `run()` result requested. |
| sdk | [4063](https://github.com/OpenHands/software-agent-sdk/issues/4063) | max_concurrent_runs does not limit native async conversations | Open | 1 | 14 | Server ops | Admission control missed the async path. |
| sdk | [4537](https://github.com/OpenHands/software-agent-sdk/issues/4537) | TaskToolSet delegation holds the parent lock | Open | 1 | 5 | Sub-agents | Sub-agent run froze the parent and the UI. |
| sdk | [3540](https://github.com/OpenHands/software-agent-sdk/issues/3540) | OpenAI-compatible /v1/chat/completions gateway | Closed | 0 | 24 | Protocols | "Agent as a model" endpoint for any client. |
| sdk | [2721](https://github.com/OpenHands/software-agent-sdk/issues/2721) | Replace regex-based shell command analysis with tree-sitter-bash | Open | 2 | 16 | Security | Regex command analysis is too weak for rails. |
