# Consuming ACP agents: driving Claude Code, Codex and Gemini CLI from Dive, Nvoken and Mobius

_Research snapshot: 2026-09-26 (America/New_York). Source-level review plus issue trackers. No agent was executed end to end for this pass; claims labelled "inferred" come from reading code, not from a run. Pinned commits:_

| Repo | Commit | Notes |
| --- | --- | --- |
| OpenHands/software-agent-sdk | `d77ada7` | `acp_agent.py`, 4,681 lines, read in full |
| openclaw/acpx | `c62ae8c` | v0.19.3 |
| openclaw/openclaw | `ee74d7b` | `extensions/acpx` |
| agentclientprotocol/claude-agent-acp | `e6681d2` | about v0.81 |
| agentclientprotocol/codex-acp (TypeScript, current) | `bf37821` | v1.13.1, on the Codex App Server |
| zed-industries/codex-acp (Rust, **deprecated**) | `296069e` | v0.16.0 |
| google-gemini/gemini-cli | `2fe7c2d` | `packages/cli/src/acp/` |
| coder/acp-go-sdk | `0845a3b` | v0.13.5 |
| Dive | `e9d2810` | local |
| Nvoken Cloud | `deea133` | local |

This document builds on the earlier ACP pass in this directory. Read [acp-spec-sdk.md](acp-spec-sdk.md) for the v1/v2 protocol boundary and [acp-issues.md](acp-issues.md) for protocol-level issues. Those are not repeated here.

---

## 1. Summary and recommendations

**The one-sentence finding.** Everyone who drives ACP agents in production ends up building the same control plane around the protocol, and gets most of it wrong the first time:

- process supervision;
- per-tenant credential homes;
- an explicit resume policy;
- a journal with an unknown-outcome state;
- settlement that waits past the prompt response;
- a watchdog for silent turns;
- a permission policy.

OpenHands spent about 75 commits on `acp_agent.py` finding these. acpx spent 44 releases. Nvoken already has most of the hard half (fenced leases, intent before effect, cursor streams, `waiting` with host-tool results). What is missing is the ACP-facing half and a process that can outlive a worker.

**What ACP gives and doesn't give.** ACP gives a uniform way to start a session, prompt it, watch structured progress, answer permission requests, and cancel. It does **not** give:

- durable identity: `session/load` can "succeed" with lost history (claude-agent-acp #1077 and #1019);
- turn settlement: updates and even permission requests arrive after `end_turn`;
- comparable usage: per-turn, last-request, cumulative or absent, depending on the agent;
- a security boundary. All three agents act on the real disk with their own tools, so ACP permission prompts are agent-side courtesy, not enforcement.

### Recommendations

1. **Build a Go client and a Go "bridge". Do not adopt acpx as the runtime.** acpx is excellent reference material and a good prototyping sidecar. But it is:
   - Node-only;
   - same-user, same-host and file-based ("not a network service");
   - unable to route an interactive permission back to a host from its CLI owner path;
   - pre-1.0 with heavy churn.

   Copy its designs:
   - three identities (local record ID, ACP session ID, provider session ID);
   - a strict versus allow-new resume enum;
   - an owner generation and heartbeat;
   - a raw-frame journal with `turn_started`/`turn_result` markers and opaque cursors;
   - `WATCH_OUTCOME_UNKNOWN`;
   - settle on the local turn result, not on `stopReason`.

   See §8.
2. **Use `coder/acp-go-sdk` as the JSON-RPC base, forked and pinned, behind a Dive-owned wrapper.** It is adequate once you add:
   - a raw-frame tee, because unknown update variants silently decode as `session_info_update`;
   - explicit cancel-and-wait, because cancelling the `Prompt` ctx discards the terminal response;
   - a non-blocking update sink, because a 1024-deep queue overflow kills the connection;
   - a `session/set_model` escape hatch for Gemini;
   - process-group supervision.

   See §8.2.
3. **Ship three shapes in this order:**
   - (a) a **Dive tool** for bounded subtasks: fastest to validate, and it exercises every hard part;
   - (b) an **Nvoken Turn executor** with a sandbox-resident bridge: the product value;
   - (c) a **Dive "agent backend"** that lets a Dive session host an external agent OpenHands-style. Build (c) only after Dive v2's Step/Recorder model lands, because it needs `Observed` tool steps.
4. **Permission requests go through host Policy, never through the calling model.** Map each ACP `session/request_permission` to a synthetic `tool.Call` (`acp/<agent>/<kind>`) so existing rules like `Bash(git push*)` match. The possible decisions are:
   - allow or deny, answered immediately;
   - ask, meaning Dive `Dialog` in-process, or a parked `waiting` Turn with a host-tool-shaped call in Nvoken.

   Pick the ACP option by `kind` (`allow_once` > `allow_always`; `reject_once`), never by position. OpenHands answers `options[0]` for everything; do not copy that.
5. **Make the sandbox the security boundary, and let modes only shape the prompts.** Run each conversation in an isolated environment with a git worktree. Record the base commit at turn start and the diff at settle. A coding agent's worktree **is** its outcome, so uncertain turns can be reconciled by inspection. That is something no generic tool gives us.
6. **Default resume policy: strict.** An Nvoken conversation's ACP session must resume, or the Turn fails as `session_unavailable`. `allow-new` is an explicit per-agent-definition opt-in, and it records an identity change as a transcript event. Always run a continuity probe after load (§4.3).
7. **Treat usage from ACP as display data.** Meter it independently where it matters (gateway or provider billing), store per-agent `usage_semantics` next to every number, and record `nil`, not zero, when it is missing.
8. **Target `@agentclientprotocol/codex-acp` (TypeScript, 1.x), not the Zed Rust adapter.** The Rust repo is deprecated (its README, lines 3–7). OpenHands and acpx already pin the new package.

---

## 2. OpenHands `ACPAgent` in depth

OpenHands runs Claude Code, Codex, Gemini CLI, Kimi, OpenCode and Pi as the agent inside an OpenHands Conversation. Canvas (the UI) talks to the OpenHands agent server; the agent server owns the ACP subprocess. This is the most complete open-source ACP client for **unattended** use, and its commit history is a catalogue of what breaks.

### 2.1 Process lifecycle

- **Library.** They use the official Python `agent-client-protocol`, pinned `>=0.12.1,<0.13`, because 0.11 reordered positional `prompt()` arguments ([pyproject](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/pyproject.toml#L8), [#3996](https://github.com/OpenHands/software-agent-sdk/issues/3996)).
  - The client is `ClientSideConnection` with a bridge class ([L1243-L1707](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L1243-L1707)).
  - Async work runs on an anyio portal thread. Sync `step()` blocks on it ([L4188-L4209](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L4188-L4209)).
- **Spawn** is `create_subprocess_exec` with piped stdio and a **100 MiB line limit** ([L3040-L3087](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3040-L3087)).
  - The limit exists because the default 64 KiB readline raised `LimitOverrunError` on large tool output, which killed the reader silently and hung `prompt()` forever ([L242-L251](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L242-L251)).
  - A stdout filter forwards only lines that look like JSON-RPC and logs the rest, because old claude-code-acp printed logs on stdout ([L1047-L1076](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L1047-L1076)).
  - stderr is drained, redacted and truncated ([L1079-L1090](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L1079-L1090)).
  - The subprocess inherits the server's cwd; the workspace is passed only as ACP `cwd`.
- **Environment** ([L2926-L2969](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L2926-L2969)):
  - Precedence is defaults, then `os.environ`, then the **whole conversation secret registry**, on the grounds that "an ACP CLI is a black box".
  - `npm_*`/`INIT_CWD` are stripped so npx doesn't resolve against a parent package ([L184-L188](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L184-L188)). `CLAUDECODE` is stripped so a nested Claude will start ([L2959-L2960](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L2959-L2960)).
  - Commands are pinned `npx -y --prefer-offline <pkg>@<ver>`: claude-agent-acp 0.63.0, codex-acp 1.10.0, gemini 0.46.0. They are pre-installed in Docker, and the npx cache is warmed before spawn ([install catalog](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/settings/acp_install_catalog.py#L78-L147), [L2536-L2578](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L2536-L2578)).
- **Handshake.** `initialize(protocol_version=1)` is sent **without client capabilities**, so fs and terminal are false ([L3089-L3101](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3089-L3101)).
  - Every `fs/*` and `terminal/*` handler raises `NotImplementedError` ([L1643-L1689](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L1643-L1689)). Elicitation is always declined ([L1621-L1641](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L1621-L1641)).
  - The agent does all file and shell work itself. That is the right default for a headless client.
- **Deadlines.**
  - A **hard startup deadline** of 90 s covers spawn, initialize, authenticate, session and mode/model calls ([L3330-L3342](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3330-L3342)). It was added after Codex hung forever in `authenticate` on an expired `id_token` ([#3629](https://github.com/OpenHands/software-agent-sdk/issues/3629), [PR #4126](https://github.com/OpenHands/software-agent-sdk/issues/4126)).
  - `authenticate` has its own 30 s timeout ([L3144-L3158](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3144-L3158)).
- **Shutdown** ([L4563-L4626](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L4563-L4626)): `conn.close()` (5 s), `terminate` (5 s), `kill`, then cancel the reader tasks, release file credentials and close the executor. `atexit` and `__del__` act as safety nets.
  - **Open zombie bug.** Only the top PID is signalled. With `npx → sh → node → claude`, the leaves survive for hours ([#4901](https://github.com/OpenHands/software-agent-sdk/issues/4901), [#4910](https://github.com/OpenHands/software-agent-sdk/issues/4910); fix [PR #4909](https://github.com/OpenHands/software-agent-sdk/issues/4909) uses `start_new_session` plus `killpg` and is unmerged).
  - An orphaned Codex holding a flock on its session file then made `session/load` fail ([#5094](https://github.com/OpenHands/software-agent-sdk/issues/5094)).
  - **Lesson: own the process group from day one.**

### 2.2 One ACP turn is one OpenHands `step()`

- The prompt is the latest user `MessageEvent` (text plus images) ([L3572-L3603](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3572-L3603)).
  - A one-time "system suffix" is appended to the **first** prompt only. The `acp_suffix_installed` flag is persisted only after that prompt succeeds ([L3605-L3624](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3605-L3624)).
  - `LocalConversation` feeds user messages to ACP one at a time with a persisted cursor ([LC L2199-L2245](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/conversation/impl/local_conversation.py#L2199-L2245)). It does **not** hold the conversation lock during a prompt, so new user messages can be saved while the agent works ([LC L2363-L2366](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/conversation/impl/local_conversation.py#L2363-L2366)).
- **Update handling** ([L1427-L1548](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L1427-L1548)):
  - Every update first resets the idle clock.
  - Message chunks are masked and accumulated, and streamed live through `on_token`. Thought chunks are accumulated but not streamed.
  - `ToolCallStart` emits an `ACPToolCallEvent` immediately. `ToolCallProgress` is merged silently and emits **only on the first transition into `completed`/`failed`**.
    - Emitting one event per progress frame was O(n²), because progress frames carry cumulative output ([PR #3465](https://github.com/OpenHands/software-agent-sdk/issues/3465)).
    - These events are display-only and are never sent to an LLM ([acp_tool_call.py](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/event/acp_tool_call.py#L46-L65)).
  - Plan, mode, commands and config-option updates are **ignored**.
- **Ordering.**
  - The first version slept 100 ms after `prompt()`, citing ACP #554. That lost usage data: 43% of GAIA runs reported $0 ([#2375](https://github.com/OpenHands/software-agent-sdk/issues/2375)).
  - It was replaced with an explicit per-session event, armed before `prompt()`, that waits up to 2 s for a `UsageUpdate` ([PR #2460](https://github.com/OpenHands/software-agent-sdk/issues/2460), [L3626-L3654](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3626-L3654)).
- **Final message** ([L3726-L3808](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3726-L3808)):
  - Mark still-open tool calls `completed`.
  - Join the chunks and **re-mask the joined text**, because a secret can be split across chunks. Emit `FinishAction(message=text)` plus `FinishObservation`.
  - A separate assistant `MessageEvent` was dropped because the UI rendered replies twice ([#3000](https://github.com/OpenHands/software-agent-sdk/issues/3000)).
  - **Known defect:** all of a turn's text becomes one blob at the end, so interleaving between tool calls is lost on reload ([OpenHands#15606](https://github.com/OpenHands/OpenHands/issues/15606)). Nvoken's projection must not repeat this (§6.5).

### 2.3 Permissions: auto-approve everything

- `request_permission` returns `options[0]`, falling back to `allow_once` ([L1602-L1619](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L1602-L1619)).
- Right after session creation they set each provider's most permissive mode ([L3307-L3316](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3307-L3316); [providers](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/settings/acp_providers.py#L514)):

  | Provider | Mode set |
  | --- | --- |
  | Claude | `bypassPermissions` |
  | Codex | `agent-full-access` |
  | Gemini | `default` |
  | Kimi | `yolo` |

  - Gemini gets `default` because `yolo` returned -32603 and crashed headless init ([#3772](https://github.com/OpenHands/software-agent-sdk/issues/3772)).
  - A mode failure fails init.
- Confirmation mode and the security analyzer are **never consulted** for ACP agents. Security rests entirely on the sandbox (the agent server runs inside it).

### 2.4 Session persistence, load and the silent fallback

- **Stored in `agent_state`** ([L2368-L2416](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L2368-L2416)):
  - `acp_session_id`, `acp_session_cwd`
  - agent name and version
  - the model-selection mechanism, current model, available models
  - `acp_suffix_installed`

  Session IDs are treated as bearer tokens: redacted and logged as the last 8 characters ([L203-L230](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L203-L230)).
- **cwd rule:** if the stored cwd differs from the current one, the ID is dropped, because agents key storage by cwd ([L3013-L3038](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3013-L3038)).
- **Load, never resume** ([L3192-L3239](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3192-L3239)):
  - They call `session/load(cwd, id, mcpServers)` and re-send MCP servers.
  - **Any** JSON-RPC error, including `-32603`, falls back to `session/new`. Transport errors propagate.
  - History is not re-seeded. The UI still shows the old transcript while the model sees only the latest message. That is silent context loss ([#5094](https://github.com/OpenHands/software-agent-sdk/issues/5094) open, [PR #5095](https://github.com/OpenHands/software-agent-sdk/issues/5095) open).
  - An attempt to export and import CLI session blobs was merged and then reverted ([PR #3562](https://github.com/OpenHands/software-agent-sdk/issues/3562), [PR #3576](https://github.com/OpenHands/software-agent-sdk/issues/3576)).
- The model is re-applied after load, because load responses omit model metadata ([L881-L932](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L881-L932)).

### 2.5 Credentials

- **Auth selection order** ([L343-L385](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L343-L385)):
  1. `chat-gpt`, only if `$CODEX_HOME/auth.json` holds a refresh token. An API-key-format file used to be selected here and hung in browser OAuth ([#3627](https://github.com/OpenHands/software-agent-sdk/issues/3627)).
  2. `vertex-ai`.
  3. `oauth-personal` (a stale file outranking a working key is still open, [#4629](https://github.com/OpenHands/software-agent-sdk/issues/4629)).
  4. `api-key`.
  5. `gemini-api-key` (with `gateway.baseUrl`).
- **Claude** is authenticated purely by env: `ANTHROPIC_API_KEY`/`ANTHROPIC_BASE_URL` or `CLAUDE_CODE_OAUTH_TOKEN`. When the OAuth token is present, the API key and base URL are stripped ([providers](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/settings/acp_providers.py#L537-L542), [#3588](https://github.com/OpenHands/software-agent-sdk/issues/3588)).
- **Codex** ignores `OPENAI_BASE_URL`. They inject `CODEX_CONFIG={"openai_base_url":…}`, otherwise requests silently hit api.openai.com and fail with 401, reported as -32603 ([L512-L558](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L512-L558)).
- **File secrets:** pasted secrets become files and an env var points at them:
  - `CODEX_AUTH_JSON` → `$CODEX_HOME/auth.json`
  - GCP service-account JSON → `GOOGLE_APPLICATION_CREDENTIALS`
  - Kimi/Pi configs likewise.

  For Codex's rotating ChatGPT tokens, a monitor thread watches the temp `auth.json` and **writes rotated tokens back** with optimistic concurrency ([acp_file_credentials.py](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_file_credentials.py#L103-L280), [PR #4124](https://github.com/OpenHands/software-agent-sdk/issues/4124)).
- `acp_isolate_data_dir` points `CODEX_HOME`, `CLAUDE_CONFIG_DIR` or `HOME` at a per-conversation directory ([L2580-L2626](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L2580-L2626)).
- Every streamed chunk, tool title and raw input/output is masked against the secret registry ([L1383-L1419](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L1383-L1419)).

### 2.6 Cancellation, errors and crashes

- **Idle prompt deadline.** The deadline is 1800 s and is reset by every update. It was a hard cap until that killed long turns that were still working ([PR #3570](https://github.com/OpenHands/software-agent-sdk/issues/3570), [L3662-L3703](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L3662-L3703)).
- **Interrupt** ([L4276-L4343](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L4276-L4343)):
  1. Send `session/cancel` (2 s).
  2. Drain the shielded prompt future (2 s).
  3. Then:
     - if it completed, finalize normally;
     - if it returned `cancelled`, close tool cards as failed and **restart the subprocess next turn**;
     - if it didn't drain, the agent ignored the cancel: restart and `session/load`.
- **Open cancel bugs:**
  - `/pause` reports paused while the agent keeps running tools ([#4990](https://github.com/OpenHands/software-agent-sdk/issues/4990)).
  - A new user message during a prompt interrupts it rather than steering ([#4829](https://github.com/OpenHands/software-agent-sdk/issues/4829)).
- **Retries** ([L4021-L4083](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L4021-L4083)):
  - Up to three retries (5, 15, 30 s) on `OSError`, broken pipe, EOF and **JSON-RPC -32603**.
  - Each retry resends **the same prompt on the same session** without respawning. That is unsafe for side effects: a -32603 after tool execution can re-run work. Do not copy it.
- **Errors** ([L1093-L1240](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L1093-L1240)):
  - JSON-RPC `data` is pulled out (Codex `codex_error_info`, Claude `details`), redacted and capped at 500 characters.
  - Errors are classified into `ACPAuthRequired` (-32000, or -32603 containing auth markers such as "401" or "please run /login"), `UsagePolicyRefusal`, `ACPPromptError`, and startup timeout, spawn and init errors.
- **No process watch.** A dead agent surfaces as EOF or a broken pipe, then retries, then an error.
- **Usage and cost** ([L2061-L2109](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L2061-L2109)):
  - Cost is the **delta of cumulative `UsageUpdate.cost`**. Tokens come from `PromptResponse.usage`, or Gemini `_meta.quota`, which the Python library often strips.
  - The LiteLLM fallback ignores cache and thought tokens ([#4382](https://github.com/OpenHands/software-agent-sdk/issues/4382)).
  - Failed and cancelled turns record **no** usage.

### 2.7 Per-agent workarounds and drift

- **Provider detection** is a substring match on `agentInfo.name` ([providers](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/settings/acp_providers.py#L721-L774)).
- **Model selection** ([L561-L710](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L561-L710)):
  - They prefer config option `model` via `set_config_option`, else `session/set_model`.
  - The mechanism is persisted, because load responses omit it.
  - Codex `gpt-5.5/high` is split into `model` + `reasoning_effort`.
  - Claude ignores `_meta` model hints ([#3654](https://github.com/OpenHands/software-agent-sdk/issues/3654)).
  - Gemini's `models` block was dropped from the Python schema ([#4093](https://github.com/OpenHands/software-agent-sdk/issues/4093)).
- `session/fork`, used for side questions, broke on claude-agent-acp ≥ 0.71 and holds back the pin ([#4884](https://github.com/OpenHands/software-agent-sdk/issues/4884)).
- **A live conformance suite** runs against pinned adapters with a bogus API key, because every upstream change has been silent: modes rejected, `_meta` ignored, models moved to config options ([test_acp_conformance.py](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/tests/sdk/agent/test_acp_conformance.py), [#4830](https://github.com/OpenHands/software-agent-sdk/issues/4830)). **Copy this.** A bogus key completes the whole handshake, so the suite needs no secrets in CI.
- OpenHands' own tools, MCP tool creation and the condenser are disabled for ACP ([L2122-L2139](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L2122-L2139)). Host tools reach the agent only as MCP servers ([OpenHands#16337](https://github.com/OpenHands/OpenHands/issues/16337)).
  - MCP config is forwarded as ACP `mcpServers`. stdio is always sent; http/sse only if advertised ([L739-L820](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-sdk/openhands/sdk/agent/acp_agent.py#L739-L820)).

### 2.8 What to take and what to avoid

| Take | Avoid |
| --- | --- |
| Hard startup deadline plus a separate auth timeout; idle (not wall-clock) prompt deadline | `options[0]` auto-approval and blanket bypass modes |
| Large line buffer; drop non-JSON stdout; drain stderr | Signalling only the top PID |
| Wait explicitly for `UsageUpdate`; cost as cumulative delta | Falling back to `session/new` on any error, without re-seeding or telling anyone |
| Per-session data dir (`CLAUDE_CONFIG_DIR`/`CODEX_HOME`/`HOME`) | Retrying the same prompt on -32603 after possible side effects |
| Credential write-back for rotating tokens | One end-of-turn text blob |
| Mask the joined text, not just chunks | Recording no usage for failed or cancelled turns |
| Conformance tests with a bogus key against pinned adapters | Positional protocol-library calls |

---

## 3. acpx in depth

acpx (MIT, v0.19.3, Node ≥ 22.13, `@agentclientprotocol/sdk` 1.5.0) is a headless ACP client with a CLI (`acpx`), a Node library (`acpx/runtime`), a flow engine (`acpx/flows`) and an agent registry ([package.json](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/package.json#L29-L36)). It has 44 releases since February 2026 and about 326 merged PRs.

### 3.1 Queue owner and IPC

- **One detached queue-owner process per session** holds the live ACP connection. Short-lived CLI or library clients submit over local IPC ([design](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/2026-02-25-warm-session-owner-architecture.md#L92-L121)). A stated non-goal: cross-machine ownership ([#L86-L90](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/2026-02-25-warm-session-owner-architecture.md#L86-L90)).
- **Spawn:**
  - A client first tries the running owner. Otherwise it spawns `node cli.js __queue-owner` detached and polls 120 × 50 ms ([queue-owner-runtime.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/execution/queue-owner-runtime.ts#L605-L652)).
  - **Owner options, including credentials, go over stdin; no bootstrap file is written** ([queue-owner-process.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/execution/queue-owner-process.ts#L203-L292)).
- **Location:**
  - The queue key is sha256(session ID)[:24].
  - The lease is `~/.acpx/queues/<key>.lock`; the socket is `/tmp/acpx-<sha(home)>/<key>.sock`, with 0700 directories and 0600 files ([paths.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/queue/paths.ts#L9-L34)).
- **Protocol:**
  - Newline-delimited JSON over a Unix socket.
  - Requests: `submit_prompt`, `cancel_prompt`, `set_mode`, `set_model`, `set_config_option`, `close_session`, each with `requestId` and optional `ownerGeneration`.
  - Replies: `accepted`, `prompt_started`, `event` (raw ACP JSON-RPC plus direction), `permission_escalation`, `result` and a typed `error` with `retryable` and `outputAlreadyEmitted` ([messages.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/queue/messages.ts#L27-L179)).
  - **Each submit carries its own permission mode, policy, timeout and resume policy.**
  - A stale generation gets `QUEUE_OWNER_GENERATION_MISMATCH` ([ipc-server.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/queue/ipc-server.ts#L453-L468)).
  - The queue depth is 16.
- **Lease** ([lease-store.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/queue/lease-store.ts#L45-L60)):
  - The record is `{pid, sessionId, socketPath, heartbeatAt, ownerGeneration (random 48-bit), processIdentity (OS birth identity), capability flags}`.
  - It is acquired exclusively with create-temp-then-`link()` under a mutation guard.
  - The heartbeat runs every **5 s**; an owner is stale after **15 s** ([#L43](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/queue/lease-store.ts#L43)).
  - Stale owners are terminated (TERM, 12 s, KILL), and the **birth identity is re-checked before every signal**. An owner that can't be verified is never killed ([#L388-L404](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/queue/lease-store.ts#L388-L404)).
- **Slow readers.** Output is spooled to disk (64 MiB per observer, 256 MiB per owner). A reader making no progress for 1 s is cut off with a non-retryable unknown-outcome error while the prompt continues ([CLI.md](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/CLI.md#L545-L570)).
- **Owner death:**
  - The next client reclaims the lease, spawns a new owner and resumes the ACP session.
  - The **replacement owner marks the dead owner's unfinished attempt `failed/WATCH_OUTCOME_UNKNOWN`** before its first turn ([events.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/events.ts#L201-L234)).
  - Submitters that lost their connection get `QUEUE_SUBMISSION_OUTCOME_UNKNOWN`, `retryable:false`, "Do not automatically resubmit" ([ipc.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/queue/ipc.ts#L116-L129)).
  - Adapter child cleanup after an abrupt death is explicitly **not guaranteed**; it is the host's job ([runtime-process-lifecycle.md](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/runtime-process-lifecycle.md#L49-L84)). One user cleaned up about 132 leaked claude-agent-acp pairs with cron ([#185](https://github.com/openclaw/acpx/issues/185)); a native lifeline helper was rejected ([#499](https://github.com/openclaw/acpx/issues/499)).
- **Multi-attach.** The CLI and `createSharedAcpRuntime()` share one owner per `(agentCommand, cwd, name)`:
  - each turn needs a fresh `requestId`;
  - joining doesn't replace the owner's credentials;
  - steer and oneshot are not supported.

  **Trust boundary:** "local, same-user IPC; it is not a network service or an isolation boundary" ([shared-sessions.md](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/shared-sessions.md#L90)).

### 3.2 Records, journal and watch

- **Record** (`acpx.session.v1`, [types.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/types.ts#L471-L505)):
  - `acpxRecordId` (stable) and `acpSessionId` (the wire ID, which can change)
  - an optional `agentSessionId`, taken only from `_meta.agentSessionId` and **never synthesized** ([identity spec](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/2026-02-23-session-identity-spec.md#L36-L96))
  - launch argv, cwd, protocol version and capabilities
  - a normalized conversation (200 messages, 8k characters each)
  - desired and current mode, model and config options

  Everything lives under `os.homedir()` with no override, so **tenant isolation means a separate `HOME`**. The embedded runtime accepts a pluggable `AcpSessionStore {load, save}` ([contract.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/runtime/public/contract.ts#L413-L416)).
- **Journal:** raw ACP JSON-RPC lines interleaved with markers (`segment`, `turn_started`, `turn_result`). A result is `completed|cancelled` with `stopReason`, or `failed` with `{code, detailCode, retryable}` ([journal.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/journal.ts#L13-L68)).
  - Segments rotate at 64 MiB, and 5 are kept ([event-log.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/event-log.ts#L5-L6)).
- **Cursor:** `base64url([recordId, sequence])`, exclusive. Typed errors are `INVALID`, `FOREIGN`, `FUTURE` and `EXPIRED` ([journal.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/journal.ts#L125-L144)). An expired cursor is an error, never a silent gap.
- **Watch** polls the journal file every 100 ms, using file-identity snapshots so rotation can't skip or duplicate ([journal.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/journal.ts#L184-L300)). It does not extend the owner TTL.
  - **Settle on `turn_result`, not on the `stopReason` response**, because the response "can arrive before local finalization finishes" ([session-watch.md](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/session-watch.md#L54-L66)).
- **`WATCH_OUTCOME_UNKNOWN`** ([watch.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/watch.ts#L78-L111)) is raised only when all of these hold:
  - a `turn_started` has no `turn_result`;
  - the owner's incarnation (pid + generation + birth identity) is gone on **two** consecutive caught-up passes;
  - a further re-read shows no late journal writes.

  Late writes always win.

### 3.3 Resume: strict versus fallback

- **Policies:** `allow-new | same-session-only` ([types.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/types.ts#L113-L114)).
- **Order** ([reconnect.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/runtime/engine/reconnect.ts#L751-L792)):
  1. reuse a loaded session;
  2. `session/resume` if advertised;
  3. `session/load`, with replay suppressed and drained until 80 ms idle, 5 s max ([client.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/acp/client.ts#L1221-L1259));
  4. if strict, throw `SessionResumeRequiredError`;
  5. otherwise `session/new`.
- **Fallback to new is allowed only on** resource-not-found or unsupported-load errors, **or** -32603 or "query closed" on a session that has no agent messages yet. **Timeouts and interrupts never fall back** ([#L127-L152](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/runtime/engine/reconnect.ts#L127-L152)).
- **Who is strict:** the shared runtime, embedded persistent sessions, owner controls, flows and **imports always**. Plain CLI prompts fall back "transparently" ([sessions.md](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/sessions.md#L234-L242)).
- **When identity changes:**
  - the record ID stays and `acpSessionId` is swapped;
  - saved mode, model and config are replayed onto the new session, rolling back on failure ([#L171-L384](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/runtime/engine/reconnect.ts#L171-L384));
  - the result reports `resumed:false` plus `loadError`.

  The local message history is kept even though the agent has lost it. That is the same class of divergence OpenHands has.
- **Issue history:** loads failed on non-standard error codes from Claude (-32603, [#29](https://github.com/openclaw/acpx/issues/29)), Cursor ([#152](https://github.com/openclaw/acpx/issues/152)) and Codex ([#121](https://github.com/openclaw/acpx/issues/121)). Mode ([#45](https://github.com/openclaw/acpx/issues/45)) and model ([#489](https://github.com/openclaw/acpx/issues/489)) were lost on fallback.

### 3.4 Permissions and client capabilities

- **Modes:** `approve-all`, `approve-reads` (the default) and `deny-all`. A per-tool policy `{autoApprove, autoDeny, escalate, defaultAction}` is applied in order deny, approve, escalate, default, mode ([permissions.md](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/permissions.md#L8-L42)).
- **Choosing an option:** `pickOption` prefers `allow_once` over `allow_always` and `reject_once` over `reject_always`. With nothing suitable it answers `cancelled` ([permissions.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/permissions.ts#L404-L444)).
  - **Codex quirk:** prefer the non-aborting `decline` over `cancel` ([codex-compat.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/acp/codex-compat.ts#L7-L42), [#535](https://github.com/openclaw/acpx/issues/535)).
- **Non-interactive:** prompting requires a TTY on stdin and stderr. Without one, `nonInteractivePermissions` is `deny` (the current default) or `fail` (exit 5) ([permissions.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/permissions.ts#L328-L355)).
  - The old default of `fail` caused hangs ([#218](https://github.com/openclaw/acpx/issues/218)).
  - Because the queue owner's stdin is a pipe, **persistent sessions are effectively non-interactive** (inferred).
  - The embedded Node runtime does offer per-turn `onPermissionRequest`. The shared runtime can't take callbacks.
- **Client capabilities.** acpx **does** advertise fs read/write and terminal ([client.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/acp/client.ts#L678-L684)):
  - file access is confined to cwd by an fs-safe root ([filesystem.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/filesystem.ts#L216-L234));
  - terminals have a 64 KiB output cap.

  Its docs are honest that this is "not an OS sandbox", because adapters' native tools bypass it ([permissions.md](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/permissions.md#L162-L182)).

### 3.5 Timeouts, late notifications and auth

- **`--timeout`** has no default. When it fires, a response that arrives during the drain keeps its real stop reason; otherwise exit 3, and "partial assistant text does not count as completion". The timed-out connection is **retired**, and the next turn reconnects and resumes ([prompting.md](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/prompting.md#L163-L178)).
- **Retries** happen only with **no side effects** and a retryable error ([runtime.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/session/execution/runtime.ts#L405-L420)).
- **After the prompt response,** acpx drains until **1 s idle, 5 s cap**, comparing observed and processed counters ([prompt-turn.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/runtime/engine/prompt-turn.ts#L16-L17), [client.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/acp/client.ts#L2800-L2832)). Out-of-turn updates are buffered ([manager.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/runtime/engine/manager.ts#L565-L583)). Background work beyond the window is captured only by the journal.
- **Auth:** acpx picks the first advertised method with a credential, checking `ACPX_AUTH_<METHOD>` env, then config, then agent-specific env ([client.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/acp/client.ts#L2054-L2156)).
  - **Ambient keys never trigger auth selection.** This came from [#247](https://github.com/openclaw/acpx/issues/247), where acpx overwrote `~/.codex` OAuth with an API key.
- **Per-agent handling** ([agent-command.ts](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/src/acp/agent-command.ts#L263-L309)):
  - **Claude** gets `_meta.claudeCode.options.settingSources=["project","local"]`, which skips user settings and plugins. A user's plugin killed the parent ([#361](https://github.com/openclaw/acpx/issues/361)).
  - **Gemini:** a version probe and a 15 s init timeout.
  - **Claude:** a 60 s `session/new` timeout.
- **Adapters** are launched with acpx's own Node, not npx, after a Node 18 versus 22 crash ([launch ownership](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/2026-04-06-built-in-agent-launch-ownership.md)).

### 3.6 How OpenClaw uses it

OpenClaw's `extensions/acpx` imports only `acpx/runtime`, **pinned to exactly 0.11.2**, far behind 0.19.3 ([package.json](https://github.com/openclaw/openclaw/blob/ee74d7b8dc410d97683cacd355df2d904b4571e9/extensions/acpx/package.json#L9-L15)).

- It uses **no queue owner and no shared runtime**; the gateway process owns the connections, with a file session store under the workspace.
- **Sessions** are keyed `agent:<id>:acp:<uuid>`, or `…:acp:binding:<channel>:<acct>:<hash>` for chat-thread bindings.
- **Defaults:** `approve-reads` with `nonInteractivePermissions: fail`. `approve-all` is flagged as dangerous. **There is no bridge that forwards permission prompts to chat.**
- **Resume:**
  - stored IDs are passed back;
  - a turn is retried once, only if it produced no output;
  - on `SESSION_RESUME_REQUIRED` the IDs are cleared and a fresh session starts. It matches on the code, because message matching missed Kiro.
- **Timeouts:** it passes `timeoutMs:0` to acpx and enforces its own deadline, because old acpx treated a timeout after partial output as completed ([runtime.ts](https://github.com/openclaw/openclaw/blob/ee74d7b8dc410d97683cacd355df2d904b4571e9/extensions/acpx/src/runtime.ts#L80-L87)).
- **Process leases:** it keeps its own leases (`OPENCLAW_ACPX_LEASE_ID`) and **reaps orphans at gateway start**. That fills the supervision gap acpx declines to cover.

Even acpx's main consumer uses it as an in-process library, not as a daemon.

---

## 4. Agent-side realities

### 4.1 Per-agent table

Link prefixes: Claude = claude-agent-acp `e6681d2`; Codex = the new TypeScript `agentclientprotocol/codex-acp` `bf37821`, with deprecated Rust behaviour marked "(old)"; Gemini = gemini-cli `2fe7c2d`.

| Concern | Claude Code (claude-agent-acp) | Codex (codex-acp) | Gemini CLI (`gemini --acp`) |
| --- | --- | --- | --- |
| **Auth methods** | Empty for a plain headless client. Terminal logins appear only if the client advertises `auth.terminal`; `gateway` only with `_meta.gateway` ([acp-agent.ts](https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L2203-L2390)) | `api-key`, `chat-gpt`, `chat-gpt-device-code`, `gateway` ([CodexAuthMethod.ts](https://github.com/agentclientprotocol/codex-acp/blob/bf37821e8f3c1f1e9b6954171855a9e2579cd2c9/src/CodexAuthMethod.ts)) | `oauth-personal`, `gemini-api-key`, `vertex-ai`, `gateway` ([dispatcher](https://github.com/google-gemini/gemini-cli/blob/2fe7c2d3f065dc40ad573d50b2091116f8a4aa18/packages/cli/src/acp/acpRpcDispatcher.ts#L47-L79)) |
| **Headless credential path** | Env: `ANTHROPIC_API_KEY`, `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_AUTH_TOKEN`/`BASE_URL`, Bedrock/Vertex; or unstable `providers/set` ([L8404-L8431](https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L8404-L8431)) | `CODEX_API_KEY` > `OPENAI_API_KEY`; `CODEX_CONFIG` JSON; `NO_BROWSER=1` hides ChatGPT; `DEFAULT_AUTH_REQUEST` ([README](https://github.com/agentclientprotocol/codex-acp/blob/bf37821e8f3c1f1e9b6954171855a9e2579cd2c9/README.md)) | `GEMINI_API_KEY` or Vertex env; `session/new` checks auth itself, so `authenticate` can be skipped ([session manager](https://github.com/google-gemini/gemini-cli/blob/2fe7c2d3f065dc40ad573d50b2091116f8a4aa18/packages/cli/src/acp/acpSessionManager.ts#L71-L110)) |
| **State home (isolate per tenant)** | `CLAUDE_CONFIG_DIR`. Settings `env` **overrides** process env ([#1009](https://github.com/agentclientprotocol/claude-agent-acp/issues/1009)) | `CODEX_HOME` (`auth.json`, `config.toml`, rollouts, SQLite) | `~/.gemini`. `authenticate` **writes `selectedType` into user settings.json** ([#25687](https://github.com/google-gemini/gemini-cli/issues/25687)) |
| **Session ops** | new, load, resume, list, close, delete, fork. Fork returns a non-live ID ([#1110](https://github.com/agentclientprotocol/claude-agent-acp/issues/1110)) | new, load, resume, list, close, fork ([CodexAcpServer.ts](https://github.com/agentclientprotocol/codex-acp/blob/bf37821e8f3c1f1e9b6954171855a9e2579cd2c9/src/CodexAcpServer.ts#L371-L409)); delete only archives ([#537](https://github.com/agentclientprotocol/codex-acp/issues/537)) | new, load only; "no plans" for list, resume, close ([#24811](https://github.com/google-gemini/gemini-cli/issues/24811)) |
| **Session storage** | `projects/<cwd-slug>/<id>.jsonl` | Codex thread rollout under `CODEX_HOME/sessions` | `~/.gemini/tmp/<proj>/chats/…jsonl`, deleted after 30 days |
| **Known continuity bugs** | Plan "clear context" forks the internal transcript under the same public ID; after restart `load` replays only the first part ([#1077](https://github.com/agentclientprotocol/claude-agent-acp/issues/1077)). A native CLI ID resumes empty ([#1019](https://github.com/agentclientprotocol/claude-agent-acp/issues/1019)). Repeated loads leak about 200 MB children ([#1011](https://github.com/agentclientprotocol/claude-agent-acp/issues/1011)) | Load replays rolled-back turns ([#355](https://github.com/agentclientprotocol/codex-acp/issues/355)); large sessions replay partially ([#516](https://github.com/agentclientprotocol/codex-acp/issues/516)); quadratic line buffering on big loads ([#539](https://github.com/agentclientprotocol/codex-acp/issues/539)); (old) loading a live session orphans its turn ([zed #186](https://github.com/zed-industries/codex-acp/issues/186)) | Load can erase the session it loads ([#28775](https://github.com/google-gemini/gemini-cli/issues/28775), open); load replay is **not awaited** before the response |
| **Work after `end_turn`** | By design: background cycles tagged `_meta["_claude/origin"]`, including **permission requests outside any turn** ([#864](https://github.com/agentclientprotocol/claude-agent-acp/issues/864), [#876](https://github.com/agentclientprotocol/claude-agent-acp/issues/876)). Cancel-then-prompt kills held background subagents ([#976](https://github.com/agentclientprotocol/claude-agent-acp/issues/976)) | Background terminal tasks (AIR capability); "out of turn" ([#365](https://github.com/agentclientprotocol/codex-acp/issues/365)); (old) premature `end_turn` ([zed #302](https://github.com/zed-industries/codex-acp/issues/302)) | Rare |
| **Concurrent permission requests** | Yes, a subagent racing the parent ([#851](https://github.com/agentclientprotocol/claude-agent-acp/issues/851)); the tool call is emitted before the request ([L7295-L7377](https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L7295-L7377)) | Yes, one task per approval | No; tools run sequentially |
| **Modes and approval** | `default`, `acceptEdits`, `plan`, `auto`, `bypassPermissions` (plus unlisted `dontAsk`) as modes and config option `mode`. `_meta` permissionMode is ignored; the initial mode comes from settings `permissions.defaultMode` or a `set_mode` after new. Bypass refuses root unless `IS_SANDBOX` ([modes.ts](https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/permissions/modes.ts#L7-L9)) | `read-only` (on-request, read-only sandbox), `workspace-write` (on-request, workspace-write, no network), `agent` (auto-review; **default**), `agent-full-access` (never, danger-full-access) ([AgentMode.ts](https://github.com/agentclientprotocol/codex-acp/blob/bf37821e8f3c1f1e9b6954171855a9e2579cd2c9/src/AgentMode.ts#L38-L94)). Initial mode via `INITIAL_AGENT_MODE` env ([L138](https://github.com/agentclientprotocol/codex-acp/blob/bf37821e8f3c1f1e9b6954171855a9e2579cd2c9/src/AgentMode.ts#L138)). No mode has workspace-write plus network ([#406](https://github.com/agentclientprotocol/codex-acp/issues/406)) | `default`, `autoEdit`, `yolo`, `plan` as modes only. The initial mode comes from `--approval-mode`. `set_mode` echoes a `[MODE_UPDATE]` text chunk ([acpSession.ts](https://github.com/google-gemini/gemini-cli/blob/2fe7c2d3f065dc40ad573d50b2091116f8a4aa18/packages/cli/src/acp/acpSession.ts#L178-L218)). Untrusted folders force `default` |
| **"Always allow" persists to** | `<cwd>/.claude/settings.local.json` ([effects.ts](https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/permissions/effects.ts#L66-L77)) | execpolicy rules in `CODEX_HOME`; (old) choosing a mode wrote project trust to `config.toml` | `.gemini/policies`, only with `enablePermanentToolApproval` |
| **Sandbox of the underlying agent** | None of its own beyond permission prompts | Seatbelt/bubblewrap. **Fails in unprivileged Docker before initialize** ([#470](https://github.com/agentclientprotocol/codex-acp/issues/470)), so use `agent-full-access` inside an external sandbox | Optional sandbox; don't use it with a non-TTY stdin ([#23959](https://github.com/google-gemini/gemini-cli/issues/23959)) |
| **`usage_update`** | `used` = last message's context, `size` = window, `cost` = SDK **cumulative** USD ([L5238-L5317](https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L5238-L5317)) | `used`/`size` only, **no cost** ([CodexEventHandler.ts](https://github.com/agentclientprotocol/codex-acp/blob/bf37821e8f3c1f1e9b6954171855a9e2579cd2c9/src/CodexEventHandler.ts#L1427-L1442)) | None |
| **`PromptResponse.usage`** | Per turn, including cache read/write ([L8991-L9018](https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L8991-L9018)) | The **last request of the turn** only ([#447](https://github.com/agentclientprotocol/codex-acp/issues/447)); cache-write missing ([#509](https://github.com/agentclientprotocol/codex-acp/issues/509)) | None. `_meta.quota` only; input double-counted, cache and thought tokens dropped ([#27985](https://github.com/google-gemini/gemini-cli/issues/27985)) |
| **MCP passthrough** | stdio, http, sse. Session-scoped stdio servers sometimes never reach the model ([#883](https://github.com/agentclientprotocol/claude-agent-acp/issues/883)) | stdio, http; (old) sse silently dropped. `CODEX_CONFIG` MCP env overrides dropped when a session adds a server ([#489](https://github.com/agentclientprotocol/codex-acp/issues/489)) | stdio, http, sse, merged over user settings |
| **cwd and dirs** | Absolute and must exist; `additionalDirectories` supported; changing cwd respawns the session | `additionalDirectories` supported (README); cwd keys `session/list` ([#431](https://github.com/agentclientprotocol/codex-acp/issues/431)) | `additionalDirectories` ignored (use `--include-directories`); cwd selects the storage bucket, so **load must use the creation cwd** |
| **Subagents** | Task tool calls with `_meta.claudeCode.parentToolUseId`; opt-in native subagent sessions | Opt-in native subagent sessions (draft RFD) with root-routed permissions; a legacy tool call otherwise; (old) dropped entirely, so it looked hung | One opaque `think` call; inner confirmations never reach the client |
| **Uses client fs/terminal** | Never | Never (terminal output via `_meta` opt-in) | fs **writes** only, if advertised; `read_file` bypasses it ([#29108](https://github.com/google-gemini/gemini-cli/issues/29108)) |
| **Model selection** | `set_config_option("model")` only; **no `session/set_model`** (-32601) | Config options `model`, `reasoning_effort` | `session/set_model` only; no config options |
| **Cancel** | Interrupt, then a forced cancel 30 s later ("query may still be wedged") ([L6674-L6697](https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L6674-L6697)). Prompts that never resolve: [#896](https://github.com/agentclientprotocol/claude-agent-acp/issues/896), [#1144](https://github.com/agentclientprotocol/claude-agent-acp/issues/1144) | Interrupt returns `cancelled`; process exit → error code 1001 with stderr tail ([L3408-L3423](https://github.com/agentclientprotocol/codex-acp/blob/bf37821e8f3c1f1e9b6954171855a9e2579cd2c9/src/CodexAcpServer.ts#L3408-L3423)) | Aborts; pending permissions **not** auto-resolved; kill the whole process group ([#25590](https://github.com/google-gemini/gemini-cli/issues/25590)) |
| **A second prompt while one runs** | Queued (advertised) or steered via `_session/steering` | Steering queue | **Silently aborts the in-flight prompt** ([acpSession.ts](https://github.com/google-gemini/gemini-cli/blob/2fe7c2d3f065dc40ad573d50b2091116f8a4aa18/packages/cli/src/acp/acpSession.ts#L312)) |
| **Extensions** | `_session/steering`, `_session/async_task/stop`, `_session/goal`, `_auth/status_update`, `_claude/sdkMessage`; `session/new` `_meta.systemPrompt`, `claudeCode.options` | goal, async tasks, file-change report, AIR config values | `_meta` api-key/gateway/quota |

### 4.2 Consequences for any client

1. **Accept `session/update` and `session/request_permission` at any time**, not only inside a turn. Keep **session activity** separate from **turn state**. A permission request with no turn open needs a policy of its own (§5.4).
2. **Settlement is a client decision.** Settle after the prompt response **and** a drain window (acpx: 1 s idle, 5 s cap), and wait specifically for a trailing `usage_update` (OpenHands: 2 s). Journal anything later as session activity.
3. **Serialize prompts per session in the client.** Gemini aborts on overlap; Codex treats the second prompt as steering; Claude queues. The client must choose, not the agent.
4. **Apply mode and model after `session/new` and after every load.** None of the three honours them in `_meta` at `new`. Use launch-time env or flags where possible: Codex `INITIAL_AGENT_MODE`, Gemini `--approval-mode`/`-m`, Claude settings `permissions.defaultMode` plus `ANTHROPIC_MODEL`.
5. **Isolate credential homes per tenant and per conversation,** and inject credentials by env. Every agent writes global state: settings, trust, `selectedType`, "always" rules, rotated tokens.
6. **Never trust session continuity from a success response.** Verify it (§4.3).
7. **Normalize usage with explicit semantics per agent** (§6.6). Never bill from ACP numbers alone.
8. **Hard-kill the process group** on cancel timeout, dead reader, lease loss and shutdown.
9. **Don't advertise fs/terminal capabilities** unless you mean to implement the sandboxing they imply. They are not a boundary anyway.
10. **Pin adapter versions and run a conformance suite** on each bump (OpenHands' bogus-key approach).

### 4.3 A continuity probe

After `session/load`, compare the replayed history with our own record:

- count of user messages;
- hash of the last user message text;
- the last assistant message's first 200 characters.

After `session/resume`, where there is no replay, we cannot verify cheaply. Either prefer `load` when we need proof, or ask a cheap probe question in a fork, which is costly and not recommended by default.

On mismatch, record `continuity: degraded` with evidence, and apply policy: fail (strict), or continue with a re-seeded context summary (allow-degraded). This catches #1077, #1019 and Gemini #28775 without agent-specific code.

---

## 5. Design: the Dive `acp` client package

### 5.1 Package layout

The client side of ACP is a separate Go module, like `dive/a2a`. The core `dive` package never imports ACP types.

```text
acp/                    module github.com/deepnoodle-ai/dive/acp
  conn/                 process supervision + JSON-RPC connection (wraps a forked acp-go-sdk)
  client/               Session, Turn, settlement, permission routing, journal
  profiles/             Claude, Codex, Gemini, Generic: launch, auth, modes, model, usage, quirks
  journal/              raw frame journal + markers + cursor (file and Postgres implementations)
  divetool/             shape (a): an ACP agent as a Dive tool
  backend/              shape (c): an ACP agent as the engine of a Dive session (v2)
  bridge/               the sandbox-resident owner process used by Nvoken/Mobius (see §6)
  conformance/          live tests with bogus keys against pinned adapters
```

### 5.2 Process and connection

```go
package conn

// A Launcher starts an agent process somewhere: a local exec, a container exec,
// a Sprite, or a bridge-managed child. It never interprets ACP.
type Launcher interface {
    Launch(ctx context.Context, spec LaunchSpec) (Process, error)
}

type LaunchSpec struct {
    Argv     []string          // resolved binary, never "npx" in production
    Env      map[string]string // the complete env: nothing is inherited implicitly
    Dir      string            // process working dir (not the ACP cwd)
    StateDir string            // becomes CLAUDE_CONFIG_DIR / CODEX_HOME / HOME
}

type Process interface {
    Stdin() io.WriteCloser
    Stdout() io.Reader
    Stderr() io.Reader          // drained by the supervisor into a ring buffer
    Wait() error
    Signal(sig syscall.Signal) error // delivered to the whole process group
    Identity() ProcessIdentity       // pid + start time; re-checked before every signal
}

// Conn is one initialized ACP connection to one process.
type Conn struct{ /* sdk conn, process, raw tee, pending permission table */ }

func Dial(ctx context.Context, p profiles.Profile, l Launcher, opts DialOptions) (*Conn, error)

type DialOptions struct {
    StartupTimeout time.Duration // hard; covers spawn..initialize..authenticate (default 90s)
    AuthTimeout    time.Duration // default 30s
    Credentials    profiles.Credentials
    Frames         FrameSink     // every raw inbound/outbound line, before decoding
    Stderr         io.Writer     // redacted
}

func (c *Conn) Capabilities() AgentCapabilities
func (c *Conn) Agent() AgentInfo              // name, version; warn when it differs from the pin
func (c *Conn) Done() <-chan struct{}         // process exit or reader failure
func (c *Conn) Err() error                    // typed: ErrProcessExited{Code, StderrTail}, ErrReader, ...
func (c *Conn) Close(ctx context.Context) error // cancel open turns, close, TERM, grace, KILL the group
```

Rules inside `conn`:

- **Reader and connection.**
  - The reader line limit is ≥ 64 MiB. Non-JSON stdout lines are dropped and logged.
  - A reader failure fails every pending request and closes the connection.
- **Raw frames.** `Frames` receives each raw line **before** SDK decoding. That is the journal, and it protects us from the SDK's `session_info_update` misdecode of unknown variants.
- **Updates.** The SDK notification handler only enqueues into an unbounded or spillable queue owned by `client`. It never blocks and never calls back into the agent. Calling back would deadlock on the SDK's response barrier, and a full queue closes the connection.
- **Env.** It is built from scratch per profile (§6.7), never `os.Environ()`. Strip `CLAUDECODE` and `npm_*`.

### 5.3 Profiles

```go
package profiles

type Profile interface {
    Name() string                                         // "claude", "codex", "gemini", "generic"
    Launch(c Credentials, stateDir string) (conn.LaunchSpec, error)
    ClientCapabilities() acp.ClientCapabilities           // default: no fs, no terminal, no elicitation
    SelectAuth(init acp.InitializeResponse, c Credentials) (*acp.AuthenticateRequest, error) // nil = none needed
    ApplySession(ctx context.Context, s SessionControl, want SessionSettings) (Applied, error) // mode, model, effort
    PermissionOption(req acp.RequestPermissionRequest, d Decision) (acp.RequestPermissionOutcome, error)
    Usage() UsageSemantics                                // how to read usage_update / PromptResponse.usage / _meta
    Quirks() Quirks
}

type Quirks struct {
    ConcurrentPrompts      PromptOverlap // Queue (claude), Steer (codex), AbortsInFlight (gemini)
    PostTurnActivity       bool          // claude: background cycles and out-of-turn permissions
    LoadReplaysBeforeReply bool          // gemini: false
    ModelVia               ModelMechanism // ConfigOption (claude, codex) | SetModel (gemini)
    RejectAborts           bool          // codex patch approvals: reject aborts the turn
    ForbiddenOptions       []string      // claude: "exit-plan-clear-*" (#1077)
}
```

Profiles are data plus small functions, the same pattern acpx and OpenHands converged on. The `Generic` profile uses only the advertised capabilities and configOptions. Pin a version per profile, and bind the conformance suite to that pin.

### 5.4 Sessions, turns and settlement

```go
package client

type ResumePolicy int
const (
    ResumeStrict   ResumePolicy = iota // load/resume or fail with ErrSessionUnavailable
    ResumeAllowNew                     // on NotFound/unsupported only: new session, recorded as IdentityChanged
)

type OpenRequest struct {
    Cwd         string            // absolute; a worktree
    AddDirs     []string
    MCPServers  []acp.McpServer   // never nil (SDK serializes nil as null)
    Resume      *ResumeRef        // nil = new
    Policy      ResumePolicy
    Settings    profiles.SessionSettings // mode, model, effort
    Continuity  ContinuityExpect  // what our record says the history holds
}

type ResumeRef struct{ ACPSessionID, Cwd string }

type Session struct{ /* conn, id, identity, activity state, turn mutex */ }

func (c *Conn) Open(ctx context.Context, r OpenRequest) (*Session, OpenReport, error)

type OpenReport struct {
    Method          string        // "new" | "resume" | "load"
    IdentityChanged bool          // previous ID could not be resumed
    Continuity      Continuity    // Verified | Unverified (resume) | Degraded{Evidence}
    Applied         profiles.Applied
}

// Prompt runs one turn. It returns only after settlement:
// response received + drain window (1s idle, 5s cap) + trailing usage wait (2s).
func (s *Session) Prompt(ctx context.Context, in Input, h Handler) (TurnResult, error)

type Handler interface {
    // Observe gets ordered, isolated updates. It cannot block the connection.
    Observe(u Update)
    // Authorize decides a permission request. It is called concurrently for
    // concurrent requests. It must return; "ask a human" is implemented by the
    // caller (block on a Dialog, or park; see §6.4).
    Authorize(ctx context.Context, p PermissionRequest) (Decision, error)
}

type TurnResult struct {
    Stop        StopReason   // end_turn, max_tokens, max_turn_requests, refusal, cancelled
    Settlement  Settlement   // Settled | CancelledByUs | Uncertain{Cause}
    Text        []Segment    // text segments interleaved with tool calls, never one blob
    ToolCalls   []ToolCallRecord // toolCallId, kind, title, final status, raw input/output, locations
    Usage       *Usage       // nil = unknown
    Frames      journal.Range // [from, to) cursors of this turn in the journal
}

type Settlement int
const (
    Settled Settlement = iota
    CancelledByUs
    Uncertain // process died, reader failed, watchdog killed, or cancel not acknowledged
)
```

**Turn lifecycle (in-process):**

```text
             Prompt()
  Idle ───────────────▶ Sending ──(request written)──▶ Running
                                                      │  updates reset idle timer
                                                      │  permission → Handler.Authorize (concurrent)
       ┌──────────────────────────────────────────────┤
       │ ctx cancelled / Cancel()                      │ response received
       ▼                                               ▼
   Cancelling ──(cancelled response ≤ grace)──▶   Draining (1s idle, 5s cap; wait usage ≤2s)
       │                                               │
       │ grace expired / process died                  ▼
       ▼                                           Settled ──▶ Idle
   Killing ──▶ Uncertain (session marked Broken; next Open must resume)
                                                        (idle watchdog expiry → Cancelling)
```

- **Cancel:**
  - Never cancel the SDK `Prompt` ctx; the SDK then drops the terminal response.
  - Send `session/cancel`, answer every **pending permission** with `cancelled` (the protocol requires it and the SDK won't do it), then wait up to the grace period (default 5 s) for `stopReason=cancelled`.
  - Still running, or the process gone: `Signal(SIGTERM)` to the group, then `SIGKILL`, and the turn is `Uncertain`.
- **Idle watchdog:** default 10 min without any update. Some Claude turns wedge until cancelled (#896).
- **Late activity:** frames after settlement go to the journal as session activity and to `Handler.Observe` with `Turn: nil`. Permission requests with no turn open go to a session-level `OutOfTurnAuthorizer`. The default is **deny**. Claude's #864/#876 are the reason this exists.
- **Retries:** never automatic after `Sending`. A pre-send failure (spawn, init, auth) is safe to retry. After the request is written, the result is `Uncertain` unless the agent answered.

### 5.5 Permission routing through Dive Policy

The calling model never sees or answers a permission request. The route is:

```text
agent ──request_permission──▶ client.Handler.Authorize
                               │ build synthetic tool.Call:
                               │   Name = "acp/<profile>/<kind>"  (execute, edit, read, fetch, other…)
                               │   Args = {title, rawInput, locations, toolCallId, subagent parent}
                               ▼
                         dive Policy.AuthorizeTool(ctx, call, view)   ← same rules/grammar as native tools
                               │ Allow / Deny{feedback} / Ask
                               ▼
                         Profile.PermissionOption(req, decision) → optionId by kind, never by index
```

- **Matching.** The synthetic name lets existing `permission.Manager` rules match. For example, `acp/*/execute` with a specifier from `rawInput.command` supports `Bash(git push*)`-style patterns. Add a specifier extractor per profile, because Claude's rawInput is the tool input, while Codex puts the command in its own shape.
- **Missing input.** If a request has no usable `rawInput` (ACP allows `toolCall` with only an ID), Policy sees `Args.incomplete=true`. The default rule is **ask or deny, never allow**. This is the approval-binding gap raised in [acp-issues.md](acp-issues.md).
- **`allow_always`** is never chosen automatically. "Always" writes persistent rules into the worktree or state dir: `.claude/settings.local.json`, Codex execpolicy. Our Policy grants for the session instead. The profile may map a session grant to `allow_always` only when the state dir is per-session and disposable.
- **Codex:** on deny, prefer the non-aborting reject option. Patch approvals only offer abort, so a deny ends the turn. Record this, rather than treating it as an agent failure.
- **Missing approver.** Today a missing `Dialog` makes Dive `AskRule` auto-allow ([dialog.go](../../dialog.go#L33-L36), [permission.go](../../permission/permission.go#L370)). The ACP adapter must **refuse to construct** without an explicit approver or an explicit `DenyAll`/`AllowAll` choice. This is Dive v2 principle 4, "an absent approver is a configured decision, never an implicit allow".
- **Modes are coarse pre-authorization**, set by the profile:
  - `Supervised`: Claude `default`, Codex `read-only`/`workspace-write`, Gemini `default`.
  - `EditsAllowed`: Claude `acceptEdits`, Codex `workspace-write`/`agent`, Gemini `autoEdit`.
  - `Unattended`: Claude `bypassPermissions` + `IS_SANDBOX`, Codex `agent-full-access`, Gemini `yolo`.

  `Unattended` is allowed only when the Launcher declares an isolated sandbox. The adapter enforces that pairing.

### 5.6 Shape (a): an ACP agent as a Dive tool

The agent is used for bounded subtasks such as "refactor X in this repo", "write tests", or "review this diff".

```go
package divetool

type Config struct {
    Name        string                 // e.g. "codex"
    Profile     profiles.Profile
    Launcher    conn.Launcher
    Workspace   WorkspaceProvider      // returns a worktree for the call; records base commit
    Policy      dive.Policy            // routes permission requests; required
    Mode        profiles.Mode          // Supervised | EditsAllowed | Unattended
    Limits      Limits                 // wall, idle, cost ceiling (best effort)
    Reuse       SessionReuse           // PerCall (default) | PerDiveSession (conversation continues)
}

// Input schema seen by the calling model: {task, paths?, context?}
// Output: summary text + structured result
type Result struct {
    Summary      string
    Diff         string         // git diff base..worktree (truncated, full diff as artifact)
    FilesChanged []string
    External     ExternalRef    // profile, agent version, ACP session id, journal range
    Usage        *Usage
}

func New(cfg Config) (dive.Tool, error)
```

- Progress streams out through `ToolStream`/`ToolProgress`, which exist today ([agent.go](../../agent.go#L3462-L3480)). In v2 this is `Observer` `ToolProgress`.
- **Outcome mapping** (v2 `tool.Outcome`; today `ToolResult` plus `ToolCallStateUnknown`):
  - `Settled` + `end_turn` → `Succeeded`.
  - `refusal`, `max_tokens` → `Failed`, with text.
  - Denied at the first permission, with no effects → `NotExecuted`.
  - `Uncertain` → the Go error path, meaning **Unknown**. The step carries `ExternalRef` plus the worktree base commit, so reconciliation is `git diff` plus the journal.
- **Reuse `PerDiveSession`** keeps the ACP session ID in the Dive session's metadata, keyed by tool name. The next call resumes strictly, and on failure opens a new session and tells the model so in the tool result.

### 5.7 Shape (b): a sub-agent with streamed progress

This is shape (a) with `Detached` semantics:

- `Execute` returns `tool.Outcome{State: Detached, Handle}` immediately.
- The turn continues or waits, and the result arrives by `Accept` (v2 §Commands).
- Progress goes to the Observer, tagged with the parent call ID.

This matches Dive's `subagent` Spawner seam: the Spawner implementation is "an ACP session in a worktree" instead of an in-process Dive agent. A permission request in a detached sub-agent is routed to Policy with `view.Parent = callID`. If Policy says Ask and nobody is attached, the sub-agent's permission waits. It is bounded by the sub-agent idle watchdog, not by the parent turn.

### 5.8 Shape (c): an external agent as the engine of a Dive session

In OpenHands' terms, the Dive session's turns are executed by Claude, Codex or Gemini instead of Dive's model loop. It fits Dive v2 as an alternative engine behind the same `session.Runner` contract:

```go
package backend

// Engine applies Dive Commands to a Turn by driving an ACP session.
// Same signature as dive.Engine.Apply; different executor of the loop.
type Engine struct {
    Pool     *client.Pool   // (profile, stateDir, cwd) → live Conn; single writer per session
    Profile  profiles.Profile
    Policy   dive.Policy
}

func (e Engine) Apply(ctx context.Context, t *dive.Turn, def dive.Definition, cmd dive.Command, deps dive.Deps) (dive.Outcome, error)
```

**Command mapping:**

| Dive command | ACP action |
| --- | --- |
| `Start{Input}` | Open or resume, then `session/prompt` |
| `Deliver{Items}` | Claude `_session/steering` or Codex steering; otherwise queued for the next prompt |
| `Cancel` | `session/cancel` + grace, else kill → `Uncertain` |
| `Accept{CallID, Outcome}` | Answer a pending **permission** whose call ID it names, if the connection still holds it; otherwise reject with `ErrPermissionExpired` |
| `Continue` | After `Uncertain`: strict resume + continuity probe, then a prompt that says the previous turn was interrupted |

**Step mapping.** This needs one addition to the v2 step vocabulary: **observed** tool steps. An external agent's tool calls are facts reported to us, not intents we executed. They must never be re-executed or reconciled as ours.

| ACP | Dive v2 step |
| --- | --- |
| Before `session/prompt` is written | `model_requested` with `Executor: "acp/<profile>"`, the ACP session ID, the prompt digest and the journal cursor. This is the intent: **after this, effects may have happened** |
| `tool_call` | `tool_started{Observed: true, ExternalID: toolCallId, Kind, Title, RawInput}` |
| `tool_call_update` → completed/failed | `tool_completed{Observed: true, State, Output(display)}` |
| Permission request → Policy decision | A new `authorization` step `{ExternalID, Request, Decision, OptionID}`. It is recorded **before** the answer is sent, so a crash leaves the decision on the record |
| Permission request → Ask | `tool_waiting{Observed: true, Wait: permission}`; `Stop{Waiting}` only when the process is kept alive (§6.4) |
| Message chunks | `model_completed` per text segment (split at tool boundaries), keeping interleaving |
| Prompt response + drain | `turn_stopped{Completed or Canceled or Refused or Limited}` with usage and `usage_semantics` |
| Process death, watchdog kill, cancel not acknowledged | `turn_stopped{Uncertain}` with `ExternalRef`; open observed tools become `tool_uncertain{Observed: true}` |
| Identity change on resume | A `context_delivered` item with `Origin: Library`, "session reset: previous agent context lost", plus `IdentityChanged` in the turn's `Started` |

**Projection:**

- The Dive record replaces the agent's transcript for display and audit only. The external agent keeps its own context; we never replay our record into it, except for an explicit re-seed after an identity change.
- `Project()` for an ACP-backed session yields the UI transcript, not a provider request.

Build this after v2's Step/Recorder lands. On v1, the tool shape is enough.

### 5.9 Process ownership in Dive

- `client.Pool` owns `Conn`s, keyed by `(profile, stateDir, cwd)`.
  - It holds one active prompt per ACP session and an idle TTL (default 5 min).
  - `Close` kills the process groups.
  - A `Pool` is process-local; Dive doesn't try to survive its own process death. That is Nvoken's job (§6).
- Every live Conn registers with a reaper keyed by a lease token passed as `DIVE_ACP_LEASE` in the child env. On start, a host can find and kill orphans from a previous incarnation. This is OpenClaw's approach.

---

## 6. Design: Nvoken ACP executor

### 6.1 Where the process lives

Nvoken workers are short-lived Cloud Run executors holding a Postgres lease per Turn claim ([ports/execution.go](https://github.com/deepnoodle-ai/nvoken-cloud/blob/deea133ad2d845b67e3259f63fdca7c09394379d/internal/ports/execution.go#L9-L14), [engine/claim.go](https://github.com/deepnoodle-ai/nvoken-cloud/blob/deea133ad2d845b67e3259f63fdca7c09394379d/internal/engine/claim.go#L19-L45)). An ACP agent is a long-lived stateful subprocess with disk state (`~/.claude`, `~/.codex`, the worktree). Putting the process **inside the worker** means:

- worker death kills the agent mid-turn;
- the agent's session files vanish with the instance, so `session/load` is impossible;
- a `waiting` Turn cannot keep a permission request open.

**Recommendation.** Run the agent in a **per-conversation sandbox** (a Sprite, Cloudflare container, Cloud Run job with a volume, or a customer worker). Put a small **Go bridge** inside it next to the agent, as the bridge's child. The bridge is our network-capable, fenced equivalent of acpx's queue owner.

```text
 Nvoken worker (attempt n)           sandbox (per conversation, volume-backed)
 ┌──────────────────────┐   mTLS/WS  ┌─────────────────────────────────────────────┐
 │ ACPExecutor          │◀──────────▶│ acp-bridge (Go)                              │
 │  claims Turn, fences │  commands  │  owns agent process group                     │
 │  commits to Postgres │  + frames  │  journal: raw frames + markers (disk, cursor) │
 └──────────────────────┘  (cursor)  │  rejects commands with stale attempt         │
                                     │  holds pending permission requests            │
                                     │   ├─ claude-agent-acp / codex-acp / gemini    │
                                     │   └─ worktree /work/<conv>/<turn-base>        │
                                     └─────────────────────────────────────────────┘
```

**Bridge contract:**

- Every command carries `(turn_id, attempt, command_id)`.
- The bridge stores the highest attempt seen per conversation and rejects lower ones. This mirrors Nvoken's `attempt` fence and acpx's `ownerGeneration`.
- `submit_prompt` is idempotent by `turn_id`. A repeat returns the existing prompt's status instead of re-sending. That makes "did the prompt get delivered?" answerable after a worker crash.

```go
package bridge

type Command struct {
    TurnID, CommandID string
    Attempt           int64
    Kind              string          // open, prompt, cancel, answer_permission, set_mode, close
    Body              json.RawMessage
}

type Status struct {
    Process   ProcessState   // starting, ready, exited{code, stderrTail}
    Session   *SessionState  // acp id, method, continuity
    Turn      *TurnState     // turn_id, phase (sending, running, draining, settled, uncertain), stop, usage
    Pending   []PendingPermission // request id, toolCallId, normalized request, received_at
    Cursor    string         // last journal position
}

// Worker-side client of the bridge.
type Bridge interface {
    Do(ctx context.Context, c Command) (Status, error)
    Frames(ctx context.Context, cursor string) (FrameStream, error) // replays from cursor, then follows
}
```

The **same binary** runs locally for Dive and Mobius customer workers. There it is just the in-process `client.Pool` with a file journal.

### 6.2 Turn executor state machine

The executor implements `ports.ExecutionClaimExecutor.Execute(ctx, claim) (TurnExecutionResult, error)` ([runtime.go](https://github.com/deepnoodle-ai/nvoken-cloud/blob/deea133ad2d845b67e3259f63fdca7c09394379d/internal/domain/runtime.go#L279-L292)). Execution state lives in Postgres. The new columns hang off the Turn or a side table `acp_turn_facts`, fenced like `model_call_facts`:

- `bridge_id`
- `acp_session_id`
- `prompt_state` ∈ {`prepared`, `sent`, `settled`, `uncertain`}
- `journal_cursor`
- `pending_permission`
- `continuity`

```text
                  claim (attempt n)
 queued ─────────────────────────────▶ preparing
                                        │ ensure sandbox lease; ensure bridge; bridge.open:
                                        │   spawn/reuse process, initialize, auth,
                                        │   new | resume | load (strict), continuity probe,
                                        │   apply mode/model
                                        │ fail → failed{session_unavailable | auth_required | startup_timeout}
                                        ▼
                                     commit prompt_state=prepared (intent, fenced)
                                        │ bridge.prompt(turn_id)  [idempotent]
                                        ▼
                                     commit prompt_state=sent ───▶ running
   running:                                                    │
     frames → project (§6.5); commit saved messages at segment boundaries + journal_cursor
     permission request → Policy
        allow/deny → commit authorization step → bridge.answer
        ask        → commit pending host call + park ──▶ waiting (lease released; bridge holds request)
                                                            │ POST /tool-results {option}
                                                            ▼
                                                   queued → claim (n+1) → reattach bridge
                                                   → answer_permission (if still pending) → running
     cancel requested  → bridge.cancel → settles cancelled | uncertain
     lease lost        → stop committing; bridge keeps running (a new attempt reattaches)
                                                               │ response + drain
                                                               ▼
                                                            settling: commit final messages,
                                                            usage(+semantics), diff evidence,
                                                            prompt_state=settled
                                                               ▼
                                                            completed | incomplete | failed
   any: bridge reports turn uncertain / process exited / sandbox gone
        → failed{outcome_unknown} + evidence (journal range, diff, stderr tail)
```

The key property is that **worker death is not agent death**. When the lease expires, the reaper requeues the Turn ([Nvoken §3.5](../comparison/nvoken-cloud.md)). Attempt n+1 claims it, finds `prompt_state=sent`, reattaches with `Frames(cursor)`, and continues projecting. Nothing is re-executed and nothing is lost, provided the sandbox survived. Only sandbox or process loss produces an unknown outcome.

### 6.3 Recovery on reclaim

On `Execute` with `prompt_state` set:

| `prompt_state` | Bridge status | Action |
| --- | --- | --- |
| `prepared` | Turn unknown to bridge | Prompt never sent: send it (safe) |
| `prepared` | Bridge has turn_id (sent before crash) | Mark `sent`; reattach from the stored cursor |
| `sent` | running / draining | Reattach from cursor; continue |
| `sent` | settled | Replay the journal from cursor to settlement; commit; settle |
| `sent` | uncertain / process exited | `failed{outcome_unknown}` with evidence; session marked broken |
| `sent` | bridge unreachable, sandbox alive | Retry reach with backoff within the execution deadline, then treat as sandbox lost |
| `sent` | sandbox gone | `failed{outcome_unknown}`; conversation `acp_session` marked **lost**; next Turn must choose re-seed or fail by policy |

### 6.4 Permission requests as `waiting` host calls

Nvoken's only suspension primitive is the external ToolCall ([nvoken-cloud.md §8](../comparison/nvoken-cloud.md)). An ACP permission request maps onto it directly:

1. **Policy first.** Agent-definition rules decide `allow` or `deny` immediately. That covers most requests in `EditsAllowed` mode.
2. **Ask.**
   - Commit a ToolCall with `mode: host`, name `acp.permission`, and arguments `{agent, kind, title, raw_input, locations, options:[{id, kind, name}], external_tool_call_id}`. Also commit the `waiting` transition.
   - The lease is released, but the **bridge keeps the JSON-RPC request open**. The agent is blocked in its own permission wait, so no compute is spent beyond the idle sandbox.
3. **Result.**
   - `POST /v1/turns/{id}/tool-results {option_id}` is bound to that ToolCall (equal replay dedupes; a changed replay is a 409) and queues the Turn.
   - Attempt n+1 claims it and sends `answer_permission`. The bridge checks the request is still pending and answers with that `optionId`.
4. **Deadline.**
   - The Turn's waiting deadline must be ≤ the sandbox keep-alive budget. On expiry the bridge answers `reject_once` (or `cancelled`), and the call settles as `NotExecuted{deadline}`.
   - Claude has no visible agent-side permission timeout. Codex and Gemini should be probed in conformance.
5. **Loss.** If the process died while waiting, the answer can't be delivered. Record the host result as accepted but `not_applied`, and fail the Turn `outcome_unknown`.

This is the **binding** property Nvoken already has for host tools: the approval attaches to one recorded request, not to a later model message.

**Rejected alternative:** deny, park, then re-prompt "approved, continue". It is lossy, the model can do something different, and the approval isn't bound to the action. Use it only as an explicit degraded mode for agents that crash on long permission waits.

**Out-of-turn permission requests** (Claude background work after `end_turn`):

- Default: **deny**, with a transcript event `acp.background_permission_denied`.
- Opt-in: open a conversation-level host call with no Turn. That needs PRD 052-style interaction records; don't invent it in the executor.

### 6.5 Projecting ACP updates into Nvoken's transcript and stream

| ACP | Nvoken |
| --- | --- |
| `agent_message_chunk` | `message.delta` preview (reserved `message_id`, `attempt`, byte `offset`). **Saved** `ConversationMessage` committed at each segment boundary: the next `tool_call`, a permission request, or settle |
| `agent_thought_chunk` | Thinking preview only (display, never saved), as today |
| `tool_call` | Saved `tool_use` block, `mode: "agent"` (a new mode meaning *observed; not executed by Nvoken*), `name = kind/title`, `input = rawInput`, `external_id = toolCallId`. **Use the external `toolCallId` in both preview and saved form**, so Nvoken's preview/saved ID mismatch doesn't recur |
| `tool_call_update` (non-terminal) | Preview only (`tool_progress`), coalesced; never saved per frame (the OpenHands O(n²) lesson) |
| `tool_call_update` → completed/failed | Saved `tool_result` (display content, diffs as structured blocks, truncated with an artifact for the full output) |
| `plan` | A snapshot on the current TurnChange (`plan` field); latest wins |
| `usage_update` | Preview of context fullness; the final value goes to TurnChange `usage` with `usage_source: "acp:<profile>"` and `usage_semantics` |
| `current_mode_update`, `config_option_update` | TurnChange fields (`agent_mode`, `agent_model`) |
| `available_commands_update`, `session_info_update` | Conversation metadata, not transcript |
| Frames outside a Turn | Journal only; surfaced on the next Turn as an `acp.background_activity` summary block |
| Unknown variants | Journal only; counted metric |

The raw journal stays in the bridge, with retention per conversation, and is archived to object storage at Turn settle. The Postgres transcript is the projection, and `journal_cursor` ties them together.

### 6.6 Usage semantics

Store usage as `{input, output, cache_read, cache_write, cost_usd?, semantics, source}`, with `semantics` taken from the profile:

- **Claude:** `per_turn` tokens from `PromptResponse.usage`, and **cumulative** cost from `usage_update.cost`. The per-turn cost is the delta from the last `usage_update` seen in this session, persisted per conversation so the delta survives a worker change. Cost resets when a new ACP session or process starts.
- **Codex:** `last_request` from `PromptResponse.usage` (#447). Mark it `lower_bound`. Cost is unknown.
- **Gemini:** `_meta.quota`, `approximate` (input double-counted, cache dropped).

When Nvoken issues the credential, true billing comes from a Nvoken-controlled gateway: `ANTHROPIC_BASE_URL`, `CODEX_CONFIG.openai_base_url`, or the Gemini gateway method. That is the only honest path to credits and spend limits ([Nvoken PRD 066](https://github.com/deepnoodle-ai/nvoken-cloud/blob/deea133ad2d845b67e3259f63fdca7c09394379d/docs/prds/066-prd-credits-and-spend-limits.md)). With customer subscription credentials (`CLAUDE_CODE_OAUTH_TOKEN`, ChatGPT login), cost is customer-side and unknown to us.

### 6.7 Credentials per tenant

- **Resolution:** resolve at claim from app-scoped provider credentials ([proposal](https://github.com/deepnoodle-ai/nvoken-cloud/blob/deea133ad2d845b67e3259f63fdca7c09394379d/docs/proposals/2026-07-30-app-scoped-provider-credentials.md)) into a `profiles.Credentials` value.
- **Delivery:** send it to the bridge **in the `open` command body over mTLS**, never as a file baked into the image. This is acpx's "stdin, not bootstrap file" rule. The bridge puts it in the child env.
- **Isolation:** each conversation gets its own `StateDir` on the sandbox volume, set as `CLAUDE_CONFIG_DIR`, `CODEX_HOME` or `HOME`.
- **Env conflicts:** strip `ANTHROPIC_API_KEY`/`BASE_URL` when `CLAUDE_CODE_OAUTH_TOKEN` is used. Claude's settings `env` overrides the process env (#1009), so the state dir must be clean.
- **Rotating file credentials** (Codex ChatGPT `auth.json`): the bridge watches the file and reports rotated tokens to the worker. The worker writes them back to the credential store with optimistic concurrency (OpenHands [PR #4124](https://github.com/OpenHands/software-agent-sdk/issues/4124)).
- **Headless rules:**
  - never invoke `terminal` auth methods;
  - hide browser methods with `NO_BROWSER=1`;
  - Codex `chat-gpt-device-code` is the only interactive-but-headless login. Surface it as a host interaction if we ever support subscription login, not as part of a Turn.
- **Auth failures** (-32000, or -32603 with auth markers; Codex [#495](https://github.com/agentclientprotocol/codex-acp/issues/495)) → `failed{auth_required}`, not retried, with an app-level notification.
- **Subscription credentials.** Running a customer's Claude or ChatGPT subscription inside our cloud is a terms question as much as a technical one. Claude's adapter has `--hide-claude-auth` to refuse subscription billing ([hide-claude-auth.ts](https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/hide-claude-auth.ts#L94-L108)). Default to API keys or a gateway in Nvoken Cloud, and leave subscriptions to customer-hosted workers (§7).

### 6.8 Sandboxing and worktrees

- **Boundary:** the sandbox is the security boundary. ACP fs/terminal capabilities are not advertised.
- **Modes:** agent modes are set per agent definition. `Unattended` modes are allowed only in platform sandboxes.
  - Codex's own sandbox fails in unprivileged containers ([#470](https://github.com/agentclientprotocol/codex-acp/issues/470)), so in containers use `agent-full-access` and rely on the container.
  - Claude bypass needs `IS_SANDBOX=1` when running as root.
- **Per-Turn git discipline:**
  - Record `base_commit` at `preparing`.
  - At settle, capture `git status` plus a diff (stat plus artifact) as Turn evidence.
  - Optionally auto-commit to a conversation branch, which makes Turns revertible.
  - For `outcome_unknown`, the diff is the reconciliation evidence.
- **Worktree layout:** one worktree per conversation (the ACP cwd must stay stable, because Gemini and Codex key sessions by cwd). Per-Turn snapshots are commits, not new worktrees. A worktree per Turn would change cwd and respawn Claude sessions.
- **Network egress policy** belongs to the sandbox. Codex's `workspace-write` mode has no network variant (#406).

### 6.9 Failure-mode table

"Recoverable" means the same ACP session continues with no unknown effects.

| # | Crash point | What survives | Recoverable? | Recommended behavior |
| --- | --- | --- | --- | --- |
| 1 | Worker dies before `prompt_state=prepared` commits | Everything | Yes | Reaper requeues; attempt n+1 starts over |
| 2 | Worker dies after `prepared`, before bridge ack | Bridge (maybe with the prompt) | Yes | Idempotent `prompt(turn_id)` returns the existing status or sends it |
| 3 | Worker dies mid-stream (bridge alive) | Agent keeps running; journal | **Yes** | Reattach at `journal_cursor`; previews voided by the higher `attempt` |
| 4 | Worker dies while the Turn is `waiting` | Nothing lost (no lease held) | Yes | Normal host-result path |
| 5 | Worker dies after the prompt response, before settle commit | Journal holds the response | Yes | Attempt n+1 replays to settlement and commits |
| 6 | Zombie worker (lease lost, still running) | — | n/a | Postgres fence rejects its writes; bridge rejects its lower-attempt commands |
| 7 | Agent process crashes mid-turn (sandbox alive) | Worktree, state dir, journal | **No** (turn); session maybe | `failed{outcome_unknown}` with diff + stderr tail. Next Turn: strict resume + continuity probe; the prompt includes "previous turn interrupted" |
| 8 | Agent hangs (no frames past idle watchdog) | All | No | Cancel → grace → kill group → as #7 |
| 9 | Cancel not acknowledged within grace | All | No | Kill group → `cancelled` + `outcome_unknown` flag |
| 10 | Bridge crashes (agent child dies with it) | Worktree, state dir, journal on disk | No (turn) | As #7; the bridge restarts under sandbox supervision and marks the unfinished turn uncertain (acpx's successor-owner rule) |
| 11 | Sandbox lost (disk gone) | Postgres transcript and archived journal only | No | `outcome_unknown`; conversation ACP session **lost**. Next Turn by policy: fail, or start a new session re-seeded with a transcript summary and a recorded identity change |
| 12 | Process dies while a permission is pending | Host result may still arrive | No | Accept the host result but mark it `not_applied`; Turn `outcome_unknown` |
| 13 | Permission waiting deadline expires | All | Yes | Bridge answers reject/cancel; call `NotExecuted{deadline}`; the agent continues or ends the turn |
| 14 | `session/load` returns not-found | Worktree | Only if `allow-new` | Strict: `failed{session_unavailable}`. Allow-new: new session + identity change event |
| 15 | `load` succeeds but history is short (#1077, #1019, #28775) | Worktree | Degraded | Continuity probe → `continuity: degraded` event; policy fails or re-seeds |
| 16 | Auth expired or invalid | All | No | `failed{auth_required}`; no retry; notify |
| 17 | JSON-RPC -32603 after tools ran | All | No | **Not** retried (unlike OpenHands). `failed{agent_error}` with `data`; `outcome_unknown` if any tool call started |
| 18 | Late frames after settle (background work) | Journal | n/a | Journal + summary on the next Turn; out-of-turn permission denied by default |
| 19 | Out-of-order permission before its `tool_call` (Go SDK PR #56) | — | n/a | Accept an unknown `toolCallId`; attach it when the call appears |
| 20 | Frame too large or reader failure | Agent maybe alive | No | Kill the group (the reader is gone); as #7 |

---

## 7. Design: Mobius usage

Mobius already has the right primitives:

- environments with fenced leases and providers `sprites | cloudflare_containers | worker`;
- **customer-hosted workers** with no inbound access;
- jobs with kinds `action_execution | llm_generation`, claimed at most once;
- **interactions** (`request_approval`, with resolution policies, waking a consumer);
- routines that run with nobody present.

See the Mobius Cloud functional areas [environments and workers](https://github.com/deepnoodle-ai/mobius-cloud/blob/5645a5b5ab26ef6c2671fe9ec1c3530c0894f5a7/docs/functional-areas/03-build-and-run/g-environments-and-workers.md#L28-L41), [interactions](https://github.com/deepnoodle-ai/mobius-cloud/blob/5645a5b5ab26ef6c2671fe9ec1c3530c0894f5a7/docs/functional-areas/03-build-and-run/d-interactions.md#L28-L52), [routines](https://github.com/deepnoodle-ai/mobius-cloud/blob/5645a5b5ab26ef6c2671fe9ec1c3530c0894f5a7/docs/functional-areas/03-build-and-run/h-routines.md).

**An ACP agent is a job kind** (`coding_agent`), not a model provider:

```go
type CodingAgentJob struct {
    Agent        string            // "claude" | "codex" | "gemini"; resolves to a pinned profile
    Repo         RepoRef           // url + base ref; checked out into the environment worktree
    Instructions string
    Mode         profiles.Mode     // Supervised | EditsAllowed | Unattended
    Approvals    ApprovalPolicy    // Policy rules; Ask → interaction(request_approval)
    Budget       Limits            // wall, idle, max cost (gateway-enforced when we issue creds)
    Output       OutputSpec        // branch name (deterministic per run), PR?, artifacts
    Session      *SessionRef       // continue a previous job's ACP session (strict)
}

type CodingAgentResult struct {
    Stop         string            // completed | refused | limited | cancelled | outcome_unknown
    Summary      string
    Branch       string; Commit string; PRURL string
    Diff         ArtifactRef
    Transcript   ArtifactRef       // projected; raw journal archived alongside
    Usage        *Usage            // with semantics
    Session      SessionRef        // for follow-up steps
}
```

**Where it runs:**

- **Platform sandbox** with API-key or gateway credentials. This is the default, and the only option for `Unattended`.
- **Customer worker** (`mobius worker` on a laptop or CI box) with the customer's own Claude or ChatGPT subscription and repo credentials. This is the most compelling Mobius-specific angle: "bring your own Claude Code/Codex seat, make it a shared, scheduled, audited company capability". The worker embeds the bridge in-process, so credentials never leave the customer machine.

**How the job fits Mobius:**

- **Approvals.** `Ask` decisions open a `request_approval` interaction with exactly one question, the normalized permission request. The consumer is the waiting job. The worker holds the JSON-RPC request open, as in Nvoken §6.4. For routines "with nobody present":
  - either pre-authorize with a mode plus Policy and put the **review gate on the output** (a PR or `request_review` interaction on the diff); this is the natural shape for unattended coding work;
  - or `deny` and report.
- **Idempotency.** A job retry after a worker crash must not duplicate effects:
  - use a deterministic branch name per `(run_id, step_id)`;
  - push only at the end;
  - on retry, detect an existing branch or commit and resume or reconcile rather than redo;
  - uncertain outcomes wake a `request_review` interaction ("the agent was interrupted; here is the diff so far: accept / discard / continue").
- **Workflow composition.** Coding-agent steps compose with ordinary Mobius steps: triage issue (LLM), then a `coding_agent` fix on a branch, then CI (action), then `request_review` (interaction), then merge (action). The external session reference lets a later step continue the same agent session (strict resume) for "address review comments".
- **Cost attribution.** Put per-person and per-routine usage events on the job, flagged with `usage_semantics`. With subscription credentials, record "customer-billed, tokens approximate".

The existing Mobius worker review ([worker-architecture-review.md](https://github.com/deepnoodle-ai/mobius/blob/c9875d51fcb0f5ca60a9c2948030f744853ea3b2/docs/development/worker-architecture-review.md)) matters here. Its H1–H3 findings (no signal handling, no write deadlines, terminal reports considered delivered on write) become sharper when a job holds a 30-minute agent process. Fix them before shipping `coding_agent`.

---

## 8. Build versus buy

### 8.1 acpx as a sidecar

| Option | Verdict |
| --- | --- |
| **acpx CLI per tenant container.** `acpx --format json --json-strict … sessions ensure`, prompt with `--file -`, parse ACP NDJSON, `sessions watch --cursor` for decoupled observation | **Usable for a prototype.** Gaps: permissions are static policy only (no host round-trip); tenancy only by separate `HOME`; state is local files; leases rely on the PID namespace; it needs Node ≥ 22.13; pre-1.0 client/owner compatibility gates |
| **Speak acpx's owner socket protocol from Go** | **No.** Internal, unversioned, gated by capability flags |
| **Node shim over `acpx/runtime` (`createAcpRuntime`)** exposing our bridge protocol, with `onPermissionRequest` and a custom `AcpSessionStore` | **Viable interim bridge** if Go takes long. It gets acpx's adapter quirk table for free and adds interactive permissions. Cost: a Node runtime in every sandbox, a second language in the critical path, and tracking acpx releases (OpenClaw is stuck on 0.11.2) |
| **Go client plus Go bridge** on a forked acp-go-sdk | **Recommended.** One implementation serves Dive (in-process), Nvoken (sandbox bridge) and Mobius (customer worker). Fencing, journal and Postgres integration are native. We already own the hard durability semantics |

Rough size of the Go build, estimated by analogy to acpx and OpenHands:

| Component | Lines of Go |
| --- | --- |
| `conn` | ~800 |
| `client` (turns, settlement, permissions, resume) | ~1,500 |
| `profiles` (3 + generic) | ~800 |
| `journal` | ~500 |
| `bridge` | ~1,000 |
| `divetool` | ~400 |
| Conformance suite | ~600 |

The work is dominated by conformance, not code.

### 8.2 Is `coder/acp-go-sdk` adequate on the client side?

**Yes, as a base, with a fork and a wrapper.** Its strengths:

- a typed `Client` interface ([types_gen.go](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/types_gen.go#L9431-L9463));
- `NewClientSideConnection(client, w, r)` with no process ownership, which is what we want ([client.go](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/client.go#L16-L21));
- the full outbound API: `Initialize`, `Authenticate`, `NewSession`, `LoadSession`, `ResumeSession`, `ListSessions`, `CloseSession`, `Prompt`, `Cancel`, `SetSessionMode`, `SetSessionConfigOption`, unstable fork/delete, and `CallExtension` ([client_gen.go](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/client_gen.go#L207-L311));
- concurrent inbound requests;
- a single ordered notification worker with a response barrier ([connection.go](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/connection.go#L773-L830));
- tests for ordering and cancel.

Problems and the fix for each:

| Problem | Fix in the wrapper or fork |
| --- | --- |
| Unknown `sessionUpdate` variants silently decode as `session_info_update`, losing the payload ([types_gen.go](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/types_gen.go#L5596-L5606); reproduced in a probe program) | Tee raw lines (`io.TeeReader`) and decode unknown variants ourselves; fix the union decoder in the fork |
| 1024-deep notification queue; **overflow closes the connection** ([connection.go](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/connection.go#L425-L448)) | Handler only enqueues into our own spillable queue; carry open PRs [#50](https://github.com/coder/acp-go-sdk/issues/50)/[#60](https://github.com/coder/acp-go-sdk/issues/60) |
| Cancelling the `Prompt` ctx sends `session/cancel` and returns immediately, discarding the terminal response ([connection.go](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/connection.go#L746-L766)) | Never cancel the ctx; explicit `Cancel` + wait + kill |
| Pending permission requests are not answered `cancelled` on cancel | Track them in the wrapper |
| No `session/set_model`; `CallExtension` refuses non-`_` names, so Gemini's model switch is unreachable | Fork with [PR #61](https://github.com/coder/acp-go-sdk/issues/61) (a `Connection()` accessor) or add a raw call |
| 10 MiB max line tears down the connection | Raise it in the fork (OpenHands needed 100 MiB) |
| Schema pinned to June (v0.13.5); several now-stable v1 types still `Unstable*` | Regenerate from current v1 schema in the fork ([PR #53](https://github.com/coder/acp-go-sdk/issues/53)) |
| `SetLogger` data race ([#57](https://github.com/coder/acp-go-sdk/issues/57)) | Set before start, or fix in the fork |
| An inbound request can overtake earlier notifications ([PR #56](https://github.com/coder/acp-go-sdk/issues/56)) | Tolerate unknown `toolCallId` on permission requests |
| `McpServers` serializes nil as `null` | Always pass an empty slice |

---

## 9. Open questions

1. **Observed tool steps in Dive v2.** Should `Observed` be a flag on `tool_started`/`tool_completed`, or its own step kinds (`agent_tool_observed`)? A flag keeps projections simple. Separate kinds make "never re-execute" impossible to get wrong.
2. **Approval as a typed record.** ACP permission requests are the strongest argument yet for a first-class approval record (Nvoken PRD 052, Mobius interactions) over "approval is a host tool named `acp.permission`". Decide before the executor ships, because the transcript shape is hard to change later.
3. **Keeping a process alive across `waiting`.** How long can a permission wait last before sandbox cost or agent-side timeouts make it unreasonable? Is there any agent-side permission timeout at all? Measure with the conformance suite.
4. **Sandbox provider for Nvoken Cloud.** Sprites (already in Mobius, with session workspaces behind a flag), Cloudflare containers, or Cloud Run jobs with GCS-fuse volumes? The bridge design is provider-neutral; the recovery table assumes a disk that outlives the process.
5. **Resume versus load.** Use `resume` (cheap, unverifiable) or `load` (replay cost, verifiable) for strict continuity? Proposed default: `load` with the probe for Nvoken; `resume` for in-process Dive where the process rarely dies.
6. **ACP v2.** Its `session/prompt` acknowledgement, `state_update(idle)` and `replayFrom` cursor would remove the drain heuristics and make the probe cheaper. Build the client with a version-specific reducer so v2 slots in (see [acp-spec-sdk.md](acp-spec-sdk.md)). Codex already has an issue to expose the turn acknowledgement and ID ([#533](https://github.com/agentclientprotocol/codex-acp/issues/533)).
7. **Subscription credentials in the cloud.** What are the product and terms position on running a customer's Claude Max or ChatGPT seat in Nvoken Cloud versus only on customer workers?
8. **Upstreaming.** Should we contribute the fork fixes (unknown-variant decode, cancel semantics, set_model) to `coder/acp-go-sdk`, or maintain `deepnoodle-ai/acp-go`? Upstream has open PRs for most of them but releases slowly (the last was 2026-06-02).
9. **Background activity product surface.** Claude keeps working after `end_turn` by design. Should Nvoken model a conversation-level "agent busy" state distinct from Turn status, which ACP v2's session state would also want?
10. **Exposing Nvoken as an ACP agent** (the inverse direction, [acp-dive-nvoken.md](acp-dive-nvoken.md)). If both directions exist, an ACP-backed Nvoken conversation could be driven from Zed. The projection in §6.5 would then need to round-trip, which argues for keeping ACP `toolCallId`s and kinds verbatim in saved blocks.

---

_Evidence limits: the code and issues cited establish behavior at the pinned commits. Codex findings were re-checked against the TypeScript adapter only for auth, modes, session capabilities, usage and the process-exit error. Other Codex rows marked "(old)" come from the deprecated Rust adapter. No live cross-agent test was run; the conformance suite in §5.1 is the proposed next proof._
