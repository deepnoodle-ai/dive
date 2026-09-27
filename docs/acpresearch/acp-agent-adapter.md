# Dive as a first-class ACP agent

_Snapshot: 2026-09-26. This pass read three client and agent codebases, ran a
throwaway Go agent against two real clients, and turned the result into a
mapping and a package sketch. It builds on the [README](README.md) and its four
companion documents; read those for protocol basics. Sources are pinned: Zed
`bda9c0b`, claude-agent-acp `e6681d2`, codex-acp `296069e` (Zed, retired) and
`bf37821` (agentclientprotocol, successor), gemini-cli `2fe7c2d`, spec
`128845f` (schema v1.23.0), acp-go-sdk `0845a3b` (v0.13.5), Dive `e9d2810`.
Spike code: `~/git/lib/acp-spike` (not committed anywhere)._

## 1. Summary and recommendations

**Dive can be a good ACP agent with a separate adapter module and about eight
small core changes.** The spike wrapped an unmodified Dive agent in about 700
lines of Go. With acpx 0.19.3 and a Go SDK client it streamed text and
thoughts, announced tool calls before asking permission, showed whole-file
diffs, honoured allow-once, allow-always and reject, cancelled in about 1 ms
(including a cancel while a permission prompt was open), and continued the same
conversation after a process restart through both `session/load` and
`session/resume`. Nothing blocked. What stood between "works" and "looks great"
was always a missing fact on Dive's side, never a limit of ACP.

Recommendations, in priority order:

1. **Build `dive/acp` as its own Go module**, like `a2a/`
   ([executor.go][dive-a2a]). It targets ACP wire v1 only, and the
   version-specific code sits behind an internal boundary so v2 can be added
   later.
2. **Own the wire layer rather than depending on `coder/acp-go-sdk`** (§4).
   Regenerating the SDK against v1.23.0 works after two hand patches, and its
   tests pass. However, the generator silently drops the `sessionId` and
   `toolCallId` of an elicitation on both encode and decode (proved in §5.4).
   Every schema bump also breaks its interfaces, and fixes have waited in
   unmerged PRs since June. Use the regenerated SDK only as the *client* in
   conformance tests.
3. **Treat Zed as the acceptance target and acpx as the CI client.** Zed pays
   for rich emission: tool kinds, whole-file diffs, a `configOptions` model and
   mode picker, commands, usage, titles, and replay with final statuses (§2).
   Zed's review UI ("Review Changes", Keep/Reject) is fed **only** by
   `fs/write_text_file`, so Dive's edit tools need a filesystem seam backed by
   the client.
4. **Make the core changes the adapter needs** (§6.5). The most important:
   - put the tool preview on the tool-call item;
   - add a "tool started" item so `in_progress` is true;
   - report usage per model call;
   - give Edit, Write and Read a filesystem seam;
   - re-authorize after a `PreToolUse` rewrite;
   - fail closed when no approver is configured;
   - reserve stable message IDs;
   - persist display content (diffs) for replay.

   Each of these is also a Dive v2 principle: `tool_started` as intent, the
   Observer `Delta` carrying a reserved message ID, and Policy re-authorizing a
   rewrite. So v2 dissolves most of the adapter's workarounds (§6.4).
5. **Accept that the stop mapping loses information and say so on the wire.**
   Dive has 14 incomplete reasons, suspension, uncertain calls, `Next`, and a
   persistence state; v1 has five stop reasons. Put the detail in
   `PromptResponse._meta.dive`, and send failures that are not clean stops as
   JSON-RPC errors with structured `data` (§6.3).

## 2. The Zed client contract

Zed pins `agent-client-protocol` 2.2.0, which has schema 1.9.1 with the
`unstable` feature. It runs the ACP agent as a stdio subprocess and reduces
notifications into thread state in `acp_thread.rs`. Every row below is from
source at `bda9c0b`.

### 2.1 Handshake, auth, sessions

| Concern | What Zed does | Consequence for Dive |
|---|---|---|
| `initialize` | Advertises `fs.readTextFile`/`writeTextFile`, `terminal`, `auth.terminal`, boolean config options, `elicitation.form`/`url`, and `_meta: {terminal_output: true, "terminal-auth": true}`. Compaction and notices are beta-gated. It fails if the agent answers with a version below 1 ([acp.rs L632-668][z-caps], [L837-839][z-version]). | Gate the client filesystem, terminal, elicitation and the terminal `_meta` extension on these flags. |
| `agentInfo` | `name` becomes the telemetry id and `version` is shown. `title` is unused ([L853-861][z-info]). | Send name and version. |
| Auth | `authMethods` become buttons. Error **-32000** from new, load, resume, list or prompt opens the auth UI, and a non-default `message` is rendered as Markdown ([L1952-1964][z-autherr], [conversation_view L1486-1532][z-authui]). | Return -32000 with an actionable message when provider credentials are missing. Optionally add a terminal auth method, for example `dive login`. |
| `session/new` | Registers the session **only after the response**. Updates for an unknown session are dropped with a warning ([L5323-5332][z-drop]). It then re-applies the user's saved mode and config defaults through `set_mode`/`set_config_option` ([L1570-1617][z-reapply]). | Send `available_commands_update` just after the response, never before it. Handle an immediate `set_*` call. |
| History | Prefers `session/load`, falls back to `session/resume`, and shows a "does not support viewing previous messages" banner after a resume ([conversation_view L1163-1190][z-loadpref]). Replayed updates go through the same reducer as live ones, and a replayed `pending` call is never settled ([L1085-1255][z-load]). | Advertise both. Replay tool calls with **terminal statuses**. |
| `session/list`, `close` | List follows `cursor` pages and uses `title`, `updatedAt` (RFC 3339) and `cwd` for import. Close is sent when the last handle drops ([L432-471][z-list], [L145-159][z-close]). | Store cwd and title in session metadata. Implement close as cancel plus release. |
| Missing for ACP agents | Retry, truncate/rewind, set-title and a dedicated model selector are not implemented ([connection.rs L188-233][z-noretry]). | There is no Retry button. The model picker exists only as a `model`-category config option. |

### 2.2 How each `session/update` is reduced

| Variant | Reduction in Zed | Emit this |
|---|---|---|
| `agent_message_chunk`, `agent_thought_chunk` | Chunks append to the current assistant entry. Chunks of the same kind merge when `messageId`s are equal **or either one is missing**. Text is concatenated raw into one Markdown document ([L383-391][z-chunkmerge], [L3260-3359][z-chunks]). Thoughts become a collapsible "Thinking" block. | Deltas, not snapshots, with your own newlines and a stable `messageId` per model message. |
| `user_message_chunk` | Deduplicated against the prompt Zed already inserted; otherwise appended ([L3034-3063][z-user]). | Only during replay. Nobody echoes the user live. |
| `tool_call` | Upsert by id. A repeated `tool_call` **overwrites `content` with `[]` and `status` with `pending`** when those fields are omitted. Content that names an unknown `terminalId` rejects the whole call ([L3742-3810][z-upsert], [L2201-2205][z-termreject]). | Send `tool_call` exactly once per id. Refine it with `tool_call_update`. |
| `tool_call_update` | An unknown id creates a synthetic **failed** "Tool call not found" entry. For a known id, only the fields present are applied. `content` **replaces the whole array**, reconciled by position ([L3668-3706][z-unknown], [L1057-1187][z-apply]). `rawOutput` is shown only when there is no structured content ([L1189-1224][z-rawout]). | Announce before updating. Always send the full content array. |
| `plan` | Full replacement. Completed entries are pruned client-side at the start of each turn. `priority` is stored but never rendered ([L4137-4167][z-plan]). | Full snapshots, exactly one `in_progress` entry. |
| `available_commands_update` | Replaces the `/` menu. `input.hint` becomes an inlay hint. **A prompt starting with an unadvertised `/name` is rejected client-side** ([message_editor L751-864][z-cmdreject]). | Advertise every command the agent accepts. |
| `current_mode_update`, `config_option_update` | If the new, load or resume response has `configOptions`, **`modes` is discarded**. The view is chosen once, so a session that starts without config options ignores later `config_option_update`s ([L4946-4960][z-cfgprec], [L5347-5356][z-cfgonce]). `category` only binds shortcuts: `mode`, `model`, `thought_level`. | Return `configOptions` (model, mode, thought level) in the session response itself. |
| `usage_update` | A ring meter from `used`/`size`, warning color at 85% or more, and `cost` when present ([L3132-3144][z-usage], [thread_view L4638-4670][z-meter]). | `used` is the current context occupancy, not the turn total. |
| `session_info_update` | `title` replaces the provisional title, which is the first line of the prompt ([L3105-3116][z-title]). | Send a title early in the first turn. |

### 2.3 Tool calls, diffs, terminals, locations

- **Kind picks the icon and the layout.** `edit`, `execute`, anything carrying
  diff content, and anything awaiting permission get a "card". Other kinds are
  compact rows. An `edit` with exactly one location shows the file-type icon
  ([thread_view L9994-10003][z-kindicon], [L8154-8182][z-card]).
- **Title** ([L1017-1055][z-titlerender]):
  - Rendered as Markdown for most kinds, so `` `backticks` `` show as code.
  - Escaped (shown literally) for `edit`.
  - Plain text used as the command header for `execute`.
- **Diff content** is a read-only multibuffer built only from
  `oldText`/`newText`. `oldText: null` means a new file. **It is not compared
  with the buffer and does not feed review** ([diff.rs L18-85][z-diff]).
- **Review, Follow and format-on-save come only from `fs/write_text_file`.**
  Zed diffs the written content against the agent's last read, applies minimal
  edits as an agent transaction, logs `buffer_edited`, runs format-on-save,
  and saves to disk ([L4894-4987][z-write]). `fs/read_text_file` reads unsaved
  buffer contents ([L4814-4892][z-read]).

  The best result uses both: diff content for the card, and the client write
  for review.
- **Locations drive Follow Agent.** The last resolved location becomes the
  agent's location. Paths must be absolute. **Zed reads `line` as a 0-based
  row** (`Point::new(row, ..)` at [L1313][z-locrow] and
  [thread_view L10158][z-locrow2]). The spec says line numbers are 1-based
  ([overview L215][s-1based]), so this is a real divergence; for now, send the
  line Zed will land on correctly (`line-1`) and flag it upstream. "Go to
  File" appears only when there is exactly one location.
- **Terminals come in two forms.**
  - A real client terminal: `terminal/create`, then `{type:"terminal"}`
    content, then `wait_for_exit`/`release`.
  - A display-only terminal for agents that run commands themselves: send
    `_meta.terminal_info {terminal_id, cwd}` on `tool_call`, then
    `_meta.terminal_output {terminal_id, data}` and
    `_meta.terminal_exit {terminal_id, exit_code, signal}` on updates
    ([acp.rs L5374-5476][z-termmeta]). The second form is a Zed convention
    that claude-agent-acp and codex-acp both use (§3).

### 2.4 Permissions, errors, cancellation

- **Permission handling** ([acp.rs L5083-5133][z-perm],
  [thread_view L9772-9890][z-permui], [L3985-4030][z-permresult]):
  - `session/request_permission` upserts the call as WaitingForConfirmation,
    expands the card, and notifies the user.
  - An unannounced id is accepted **only if `title` is present**.
  - There is one button per option, with icons by kind.
  - After the choice, a reject moves the call to a local **Rejected** state
    (there is no such wire status), and an allow moves Pending to InProgress.
  - **Zed does not remember "always".** It is left to the agent, and nothing
    in the ACP path answers automatically ([tool_permissions.rs][z-noalways]).
- **Stop reasons and errors** ([L4286-4470][z-runturn], [conversation_view L165-247][z-err]):
  - A JSON-RPC error shows an "An Error Happened" callout (`message` plus
    pretty `data`) with no Retry.
  - `max_tokens` shows "Output Limit Reached… ask it to continue".
  - `refusal` shows "Request Refused" and **deletes the user message and
    everything after it** unless a Completed tool call with `rawOutput`
    follows it.
  - `cancelled` marks pending calls Canceled.
  - `end_turn` and `max_turn_requests` have no special UI.
- **Cancel** ([L4476-4570][z-cancel]):
  - Zed marks every non-finished call Canceled, answers open permission
    requests `cancelled`, sends `session/cancel`, and goes idle at once.
  - It then awaits the old prompt response **with no timeout** before sending
    the next prompt.
  - Late updates still apply: a `completed` sent after cancel overwrites the
    local Canceled status.

### 2.5 Looks great versus merely works

**Merely works:** answer `initialize`, handle `session/new` and
`session/prompt`, stream `agent_message_chunk`, and return `end_turn`.

**Looks great:**

1. Capabilities: `loadSession`, `sessionCapabilities.{list,resume,close}`,
   and `promptCapabilities.{embeddedContext,image}`.
2. `configOptions` for model, mode and thought level in every session
   response; the full list returned from `set_config_option`.
3. `available_commands_update` just after the response, with `input.hint`.
4. Delta chunks with a stable `messageId`, and thoughts as thought chunks.
5. Tool calls:
   - `tool_call` before anything else, with `kind`, a Markdown `title` (the
     raw command for `execute`), `rawInput`, and absolute `locations`;
   - then `in_progress`, then `completed` or `failed` with `rawOutput`;
   - updates carry full content arrays.
6. Edits: whole-file `diff` content before the permission request, and the
   write done through `fs/write_text_file`.
7. Shell: the `_meta.terminal_*` form, or a real client terminal.
8. `plan` snapshots, `usage_update` after every model call, and a
   `session_info_update` title.
9. Precise permission options, "always" remembered by the agent, and
   `cancelled` treated as deny.
10. Cancel within milliseconds, with final tool updates flushed before the
    `cancelled` response.
11. Replay with terminal statuses and the original diffs.

## 3. How the best agents emit

Four codebases were compared. Zed's Rust `codex-acp` README says development
moved to `agentclientprotocol/codex-acp` ([README L3-6][cx-moved]), so that
TypeScript successor ("CXn") is the Codex reference.

| Dimension | claude-agent-acp | codex-acp (CX Rust / CXn TS) | gemini-cli `--acp` | Take for Dive |
|---|---|---|---|---|
| Capabilities | v1. load, list, resume, fork, close, delete. Image and embedded context. MCP http and sse. Heavy `_meta` ([acp-agent.ts L2320-2388][ca-init]) | CX: v1 always. load, list, resume, close. MCP http only; SSE servers are **silently dropped** ([codex_agent.rs L345-427][cx-sse]). CXn adds fork and delete | load only, **no `sessionCapabilities`** ([acpRpcDispatcher.ts L47-103][gm-init]) | Advertise only what is backed. Reject unsupported MCP transports loudly. |
| Tool kind and title | Name-to-kind table. `Read path (1 - 50)`; Bash title is the raw command, with the description in `_meta` ([tools.ts L150-513][ca-kinds]) | Shell commands are **parsed** into read, search or execute ([thread.rs L2546-2593][cx-parse]) | Kind comes from the tool's own `Kind` enum ([acpUtils.ts L202-222][gm-kind]) | Give Dive tools a declared kind, as Gemini does. Use the raw command as the execute title. |
| Lifecycle | A `pending` call at `content_block_start`, refined as arguments stream ([acp-agent.ts L10419-10467][ca-stream]). Always emits `tool_call` before permission ([L7301-7400][ca-before]) | CX starts at `in_progress`, and its permission request is the call's **first** appearance ([thread.rs L1932-1961][cx-permfirst]) | Pending, then permission, then completed, **skipping `in_progress`**. Can send `failed` for ids it never announced ([acpSession.ts L923-934][gm-phantom]) | Pending at block start, then refine, then permission, then `in_progress`, then a terminal status. |
| Edits | Before execution: an old_string→new_string fragment; Write **always looks new**. After execution: one diff per hunk ([tools.ts L219-268][ca-editpre], [acp-agent.ts L10061-10100][ca-editpost]) | CX: per-hunk. CXn **switched to whole-file** diffs rebuilt from disk, with `oldText:null` for a new file and `newText:""` for a delete ([CodexToolCallMapper.ts L839-915][cxn-whole]) | Whole-file diff at confirmation. **Routes I/O through client `fs/*`** when offered ([acpFileSystemService.ts L26-87][gm-fs]) | Whole-file diff before approval, re-sent at completion. Client `fs/*` when advertised. |
| Terminals | `_meta.terminal_*` with the terminal id set to the tool call id, gated on client `_meta.terminal_output`; output sent once at the end ([tools.ts L823-969][ca-term]) | Same metas, with output **streamed**. CX's fallback is one buffered block at the end; this fixed an O(n²) crash from resending the growing text ([thread.rs L2094-2112][cx-n2]) | None | Terminal metas when the client opts in; otherwise **one fenced block at completion**. Never resend the accumulated output. |
| Plans | TodoWrite becomes a full-replacement `plan` with no tool call ([acp-agent.ts L10038-10045][ca-plan]) | Plan steps become `plan` ([thread.rs L2708-2724][cx-plan]) | Todos dropped | Map Dive's todo tool to `plan`. |
| Commands | From the SDK with `input.hint`; sent after new, load and resume, and again when they change; GUI-useless commands filtered ([L2392-2444][ca-cmds]) | Sent 200 ms after load; unknown commands fall through as prompt text ([thread.rs L2903-2934][cx-cmds]) | No hints; names contain spaces | Send after the response; one token per name; hints. |
| Modes and config | 5 modes, also exposed as a `mode` config option, plus model, effort and fast mode. Agent-side mode changes push both updates ([session-mode.ts L315-350][ca-modes]) | Approval presets; **choosing auto writes project trust to disk** ([thread.rs L3284-3322][cx-trust]) | Mode changes reported as a `"[MODE_UPDATE]"` text chunk (anti-pattern) | `configOptions` for mode, model and thought level, mirrored in legacy `modes`. |
| Permission options | allow-once; allow_always **shown only when its label can state exactly what it allows**; reject. The CLI stores rules ([options.ts L25-99][ca-always]) | Codex's decisions become options; Codex stores the amendments ([thread.rs L2412-2526][cx-opts]) | proceed_once / always / always_and_save; the policy engine stores rules; "always" on an edit **silently switches to autoEdit** ([policy.ts L190-201][gm-autoedit]) | Agent-side grant store, a precise label or no "always" at all, and no silent mode change. |
| Persistence | load replays everything with message ids; hooks are off during replay, so **post-edit diffs are missing** ([L6956-7268][ca-load]) | Replays from rollout files as Completed ([thread.rs L3376-3686][cx-replay]) | Load does **not await** replay before responding ([acpSessionManager.ts L210-212][gm-noawait]) | Await replay; replay the final diffs, the last plan, commands and usage. |
| Usage | `usage_update` with used = latest context and `cost`; per-turn `PromptResponse.usage` ([L5302-5319][ca-usage]) | CX: `usage_update` only | `_meta.quota` only | Both, with `used` = context occupancy. |
| Stop and errors | All five reasons; the refusal text is streamed first ([L5498-5715][ca-stop]) | end_turn and cancelled only | Safety **and** MAX_TOKENS returned as `end_turn`; HTTP status codes used as JSON-RPC codes ([acpSession.ts L470-542][gm-stop]) | Faithful mapping; standard codes with structured `data`. |
| Cancel | Races permissions against abort, 30 s backstop ([L6467-6700][ca-cancel]) | Detaches permissions, then interrupts ([thread.rs L3328-3335][cx-cancel]) | Abort only | Cancel the context, drop waits, flush, return `cancelled`, with a watchdog. |

**Divergences worth copying on purpose:**
- Claude's "always, only if exact" rule.
- CXn's move from per-hunk to whole-file diffs.
- Gemini's use of the client filesystem.
- Everyone's `_meta.terminal_*` convention.

**Anti-patterns to avoid:**
- Permission-first announcement (CX).
- Unawaited replay (Gemini).
- Stop reasons collapsed into `end_turn` (Gemini).
- Resending growing output (CX, before its fix).
- Mode changes sent as text (Gemini).

## 4. Go SDK decision

### 4.1 What was measured

| Question | Finding |
|---|---|
| Does the generator run against v1.23.0? | **Yes.** It needs the four `schema-v1.23.0` release assets; the Makefile's `v$(ACP_VERSION)` URL no longer matches the upstream `schema-v*` tag scheme ([Makefile][go-make]; unmerged fix PR #52). `go run ./cmd/generate` succeeds. The stable schema grows from 129 to 170 `$defs`. |
| Does it build? | Two compile errors, each patched in one line: `helpers.go` needs `TerminalId(...)` because the terminal id became a named type, and `CreateElicitationRequest.Validate` references a `Message` field the union type no longer has. After that the library builds. |
| Do the tests pass? | After renaming `UnstableDeleteSession*` to `DeleteSession*` and adding stubs for `DeleteSession`, `CreateElicitation` and `CompleteElicitation` to six test doubles: `go test .` → `ok` (1.47 s). |
| API stability | Every promotion from unstable to stable renames types and **adds a required method** to the `Agent` and `Client` interfaces: `DeleteSession`, `CreateElicitation` and `CompleteElicitation` in this bump. All examples break. Any Dive type implementing `acp.Agent` would break the same way on each bump. |
| Correctness | **The generated `CreateElicitationRequest` cannot carry `sessionId`/`toolCallId`.** The generator drops the scope that the schema nests as `anyOf` inside `allOf`. Proved on the wire in §5.4. Unmerged PR #51 reports the same bug against the older types. |
| Maintenance | Last push 2026-06-05. Open PRs include a schema bump to 1.20 (#53), the release-tag fix (#52), the elicitation `sessionId` fix (#51), a `ContentBlock` metadata loss (#55), inbound request ordering (#56), a logger data race (#59), and a request not to close the connection on notification overflow (#60; the current code shuts down the receive side at [connection.go L446-447][go-overflow]). |
| Behaviors that matter | A second `session/prompt` for a session silently cancels the first prompt's context ([agent_gen.go L415][go-prevcancel]). `session/cancel` cancels the prompt context before calling `Agent.Cancel` ([L301][go-cancelctx]). An outbound request whose context ends sends `$/cancel_request` ([connection.go L701][go-cancelreq]), which is a stable v1 protocol method. The line limit is 10 MiB ([L369][go-maxline]). |

### 4.2 Options

| Option | Cost | Risk | Verdict |
|---|---|---|---|
| Use v0.13.5 as is | 0 | Trails the schema by 3 months; elicitation and usage stay `Unstable*`; the elicitation scope bug remains | No |
| Regenerate in a fork | Small now (proved), plus a regeneration per schema release | The generator silently drops fields (elicitation now, unknown later); interface churn leaks into Dive's API unless wrapped | Only as a test peer |
| Wrap the SDK (use its `Connection`, own the types) | Medium | `NewConnection`/`SendRequest` are exported, but the package is 356 KB of generated types we would not use, plus the open concurrency PRs | Possible fallback |
| **Dive-owned minimal layer** | ~400 lines of JSON-RPC plus ~60 hand-written v1 types for the subset Dive emits and accepts | Drift, mitigated by CI that validates golden frames against upstream `schema/v1/schema.json` and runs acpx and the regenerated SDK as clients | **Recommended** |

**Recommendation: write `dive/acp/internal/jsonrpc` and
`dive/acp/internal/wire/v1`. Keep the regenerated SDK as a test-only client.**

*Why:*
- ACP v1 is additive, and Dive needs a small, stable subset.
- Owning the types gives exact control over the null-versus-absent semantics
  Zed depends on: `oldText: null`, and omitted versus empty `content` on
  repeats.
- It keeps protocol churn out of Dive's public API.
- It avoids depending on a community SDK whose fixes have waited since June.
- A separate `internal/wire/v2` can later hold v2's upsert and `state_update`
  semantics without sharing a reducer with v1.

*Design notes for the layer:*
- Borrow the SDK's proven design, which is Apache-2.0: one ordered
  notification queue, requests handled concurrently, `$/cancel_request` in
  both directions, and a response barrier over earlier notifications.

*What would change the answer:*
- If upstream ships an official Go SDK, or acp-go-sdk resumes regular schema
  releases with the generator fixed, switch to wrapping it.

## 5. Spike results

### 5.1 What was built

The spike lives in `~/git/lib/acp-spike`. It is not part of the Dive
repository, which was only read.

- **`cmd/diveacp`:** an ACP agent built on the regenerated SDK (`sdk-regen/`
  through a `replace`) and on local Dive `e9d2810`.
  - It has a Dive `Agent` per session, with the toolkit's `Edit`, `Write` and
    `Read` tools, sessions in `session.FileStore`, and a `PreToolUse` hook that
    calls `session/request_permission`.
  - `scriptLLM` implements `llm.StreamingLLM` and streams Anthropic-shaped
    events chosen by keywords in the prompt: edit, write, slow, think, limit,
    refuse, echo. The echo reports how many messages the model saw, which
    serves as a continuity probe.
  - `ACP_TRACE` records every wire line in both directions, with timestamps.
- **`cmd/driver`:** a scripted Go client with scenarios `edit`, `reject`,
  `always`, `cancel`, `cancel-perm`, `stops`, `seed`, `list`, `load` and
  `resume`.
- **Client:** acpx 0.19.3, installed locally in `acpx/`.
- **Reproduction:** `./run.sh` runs everything. Traces are written to
  `traces/*.jsonl`.

```sh
cd ~/git/lib/acp-spike && ./run.sh           # all scenarios, both clients
# individual runs
DIVEACP_STATE=$PWD/state ACP_TRACE=$PWD/traces/edit.jsonl \
  ./bin/driver -agent ./bin/diveacp -cwd $PWD/ws -scenario edit
cd ws && ../acpx/node_modules/.bin/acpx --agent ../bin/diveacp --approve-all exec "please edit the greeting"
../acpx/node_modules/.bin/acpx --agent ../bin/diveacp sessions new
../acpx/node_modules/.bin/acpx --agent ../bin/diveacp --ttl 2 prompt "first message"; sleep 4
../acpx/node_modules/.bin/acpx --agent ../bin/diveacp --ttl 2 prompt "continuity probe"
../acpx/node_modules/.bin/acpx --agent ../bin/diveacp cancel
```

### 5.2 Outcomes

| Scenario | Client | Result |
|---|---|---|
| Edit, allow once | driver, acpx `--approve-all` | Works. The file changed. Updates arrived in this order: text chunks, early `tool_call` (pending, title `Edit`), `tool_call_update` with a whole-file diff, `request_permission` with diff content, `tool_call_update` completed with the diff, final text, `usage_update`, response `end_turn`. acpx rendered kind, input, files and output. |
| Write new file, reject | driver, acpx `--deny-all` | Works. The file was not created, and the call ended `failed` with "the user rejected this tool call". The model saw the denial and answered. acpx exits with code 5 after a denial. |
| Allow always | driver | Works. The second identical write was **not** re-prompted, because the grant is session-scoped and held in memory. acpx `--approve-all` always picks `allow_once`. |
| Cancel while streaming | driver, acpx `cancel` | `session/cancel` to the `cancelled` response took about 6 ms (driver) and about 1 ms (acpx). The partial text was kept, and the next prompt worked on the same session. Dive saved the partial turn plus a `turn-incomplete` reminder, so the model saw 4 messages. |
| Cancel while a permission prompt is open | driver | The agent's context ended, the SDK sent `$/cancel_request` for the open request, and the hook failed closed. The call became `failed` **before** the `cancelled` response, as the spec requires ([prompt-turn L365][s-cancelorder]). The file was untouched. The client's late `cancelled` reply was ignored. |
| Thought / limit / refusal | driver | `agent_thought_chunk` then text, ending `end_turn`. `max_tokens` with `_meta.dive {reason: output_limit, next: continue}`. `refusal`. |
| Restart, then list | driver (new process) | `session/list` returned every stored session with `cwd` and `updatedAt`. There is **no title**, because Dive sessions have none by default. |
| Restart, then load | driver | Replayed user chunks, agent chunks and one `tool_call` (completed, with title, kind, locations and rawInput) **before** the response. The continuity probe saw 7 messages. |
| Restart, then resume | driver, acpx (ttl expiry) | No replay; context was restored. The driver's probe saw 9 messages, and acpx's saw 3. **acpx chose `session/resume`** when both were advertised. Zed would choose load. |
| JSON output | acpx `--format json` | acpx prints the raw frames. Our `available_commands_update` (sent 20 ms after `session/new`) arrived after acpx had already sent `session/prompt`. That is harmless, but it shows the race. |

### 5.3 Wire traces (abridged from `traces/edit.jsonl` and `traces/cancel-perm.jsonl`)

```text
0.026s C->A {"id":3,"method":"session/prompt","params":{"prompt":[{"text":"please edit the greeting","type":"text"}],...}}
0.043s A->C session/update {"sessionUpdate":"agent_message_chunk","content":{"text":"I'll ","type":"text"},"messageId":"msg_001"}
...     (5 more chunks)
0.083s A->C session/update {"sessionUpdate":"tool_call","toolCallId":"toolu_msg_001","title":"Edit","kind":"edit","status":"pending"}
0.106s A->C session/update {"sessionUpdate":"tool_call_update","toolCallId":"toolu_msg_001","title":"Edit `hello.txt`","kind":"edit",
        "locations":[{"path":"/…/ws/hello.txt"}],"rawInput":{…},
        "content":[{"type":"diff","path":"/…/ws/hello.txt","oldText":"hello world\nsecond line\n","newText":"hello, ACP world\nsecond line\n"}]}
0.107s A->C {"id":1,"method":"session/request_permission","params":{"toolCall":{"toolCallId":"toolu_msg_001","title":"Edit `hello.txt`",…,"content":[{diff}]},
        "options":[{"optionId":"allow_once","kind":"allow_once"},{"optionId":"allow_always","kind":"allow_always","name":"Always allow Edit this session"},{"optionId":"reject_once","kind":"reject_once"}]}}
0.109s C->A {"id":1,"result":{"outcome":{"outcome":"selected","optionId":"allow_once"}}}
0.111s A->C session/update {"sessionUpdate":"tool_call_update","toolCallId":"toolu_msg_001","status":"completed","content":[{diff}],"rawOutput":{…}}
0.129s A->C session/update {"sessionUpdate":"agent_message_chunk","content":{"text":"Done. "},"messageId":"msg_002"} …
0.175s A->C session/update {"sessionUpdate":"usage_update","used":2480,"size":200000}
0.175s A->C {"id":3,"result":{"stopReason":"end_turn","usage":{"inputTokens":2400,"outputTokens":80,"totalTokens":2480},
        "_meta":{"dive":{"status":"completed","turnId":"turn_2b8…"}}}}

# cancel while the permission prompt is open
0.110s A->C {"id":1,"method":"session/request_permission",…}
0.315s C->A {"method":"session/cancel","params":{"sessionId":"sess_036c…"}}
0.315s A->C {"method":"$/cancel_request","params":{"requestId":1}}
0.316s A->C session/update {"sessionUpdate":"tool_call_update","status":"failed","content":[{"text":"permission request failed; not run: …Request cancelled…"}]}
0.316s C->A {"id":1,"result":{"outcome":{"outcome":"cancelled"}}}
0.319s A->C session/update {"sessionUpdate":"usage_update",…}
0.319s A->C {"id":3,"result":{"stopReason":"cancelled","_meta":{"dive":{"status":"incomplete","reason":"canceled","next":"input"}}}}
```

The `$/cancel_request` and the `failed` update swapped places between runs:
the SDK sends the cancel from a separate goroutine. Both always came before the
prompt response. The traces in the spike directory come from the last
`./run.sh`.

**Ordering.** Dive serializes its callback: model events run on the loop
goroutine, and tool events pass through a one-slot gate ([agent.go
L2811-2860][dive-gate]). Writing inside the callback therefore puts frames on
the wire in Dive's order, and every frame reached the wire before the prompt
response. Only the unsolicited commands notification, sent from a goroutine,
raced other traffic.

### 5.4 What broke or surprised

1. **Colliding `messageId`s.**
   - Replay reused the provider message ids (`msg_001`…). After a restart the
     scripted provider started again at `msg_001`, so a replayed id and a live
     id collided. That violates "opaque, unique identifier"
     ([session-setup L176][s-msgid]).
   - Real provider ids can be empty or, for some providers, very long.
   - **Fix:** mint ids in the adapter from `(turnID, message index)` so they
     are deterministic and stable across replay; in v2, use the reserved
     message id.
2. **No true `in_progress`.** Dive emits the tool-call item before
   `PreToolUse` and emits nothing when execution starts, so the adapter cannot
   honestly send `in_progress` ([prompt-turn L257][s-inprogress]).
3. **Usage is summed per turn.** `Turn.Usage` gave `used: 2480` for a context
   that was about 1,240 tokens. The meter needs the latest call's input plus
   output, which is available only by parsing `ModelEvent` usage.
4. **Replay loses diffs.** Dive persists `tool_result` content, not the preview
   or diff. The file has changed since, so the diff cannot be recomputed on
   load. Replay showed the Edit tool's text diff rendering instead. Claude's
   adapter has the same gap ([acp-agent.ts L6956-7268][ca-load]).
5. **Synthetic messages in the record.** The cancelled turn saved a
   `turn-incomplete` reminder block as a user message. Replay must skip
   reminder blocks, and did so only because they are not `TextContent`. This
   is the v1 "synthetic message in the record" problem that v2's projection
   removes.
6. **Elicitation scope lost by the SDK.** `cmd/elicitcheck` marshals and
   round-trips a form elicitation. `sessionId` and `toolCallId` vanish in both
   directions:

   ```text
   roundtrip: {"message":"Pick one","mode":"form","requestedSchema":{...}}   # sessionId, toolCallId dropped
   ```

   A spec-valid session-scoped elicitation cannot be sent through
   `AgentSideConnection`. The underlying connection is unexported, and
   `CallExtension` accepts only `_`-prefixed methods.
7. **The spike resent accumulated tool output on each `ToolStream`.** That is
   the pattern that crashed codex-acp. No tool in the spike streamed, but the
   production adapter must not do this.
8. **Close did not cancel.** The spike's `session/close` drops the session
   without cancelling in-flight work. The spec says it MUST cancel
   ([session-setup L299][s-close]). This is trivial to fix, and it goes in the
   conformance suite.

**Not run:** Zed itself. It is not installed here, and its UI cannot be
driven headlessly. §2 comes from source. The first manual Zed session should
check the card, Follow, the review path (after the filesystem seam lands), and
the 0-based location row.

## 6. Dive→ACP v1 mapping spec

### 6.1 Identity and ordering

| ACP | Dive source | Rule |
|---|---|---|
| `sessionId` | `session.Session.ID()` | An opaque, minted, URL-safe ID, stable across processes. Session metadata stores `cwd`, `additionalDirectories`, `title`, mode, and config values. |
| `toolCallId` | `llm.ToolUseContent.ID` | The same value in the early `tool_call`, the permission request, every update, and replay. Dive persists it inside messages, so replay needs no mapping table. |
| `messageId` | Derived | `m_<turnID>_<index of assistant message in turn>`, used for both text and thought chunks. It is deterministic across replay. In v2 it is the Observer's reserved message ID. |
| Frame order | The `EventCallback` order | Emit synchronously inside the callback. Do not buffer in a lossy way; the connection write is the backpressure. Put everything before the `PromptResponse` (the `TurnEnded` item arrives after the session write and before `CreateResponse` returns). |
| Concurrency | One turn per session | One mutex per session. A second prompt while one is running is rejected with an error rather than silently cancelling the first, which is the SDK's default. Protect against two processes with `ClaimSession`. |

### 6.2 Current `ResponseItem` → `session/update`

| Dive item ([response.go L17-60][dive-items]) | ACP emission |
|---|---|
| `ModelEvent` `message_start` | Remember the provider message and reserve a `messageId`. |
| `ModelEvent` text delta | `agent_message_chunk {messageId}`. |
| `ModelEvent` thinking delta | `agent_thought_chunk {messageId}`. Redacted thinking is skipped. |
| `ModelEvent` `content_block_start` `tool_use` | `tool_call {id, title: name, kind, status: pending}`. This is the early announcement. |
| `ModelEvent` `input_json_delta` | Optional: parse incrementally and refine `title`/`locations`/`rawInput` (Claude does this). |
| `Message` | Only if nothing was streamed for it (a non-streaming provider): one chunk with the full text and thoughts. |
| `ToolCall` | `tool_call_update {title, kind, locations, rawInput, content: [diff]}`, or `tool_call` if it was not announced (unknown tool, non-streaming). The title comes from the `ToolPreviewer` summary; the diff comes from the tool's diff preview (§6.5). |
| *(permission)* | `session/request_permission` (§6.3). |
| *(missing: tool started)* | `tool_call_update {status: in_progress}`, plus `_meta.terminal_info` for execute tools. |
| `ToolStream` | With terminal metas: `_meta.terminal_output {data: chunk}`. Otherwise buffer and send once at completion. |
| `ToolProgress` | `tool_call_update {content: [text(Display)]}`, throttled to at most 4 per second, latest wins. `Metadata` goes in `_meta.dive.progress`. |
| `ToolCallResult` | `tool_call_update {status, content, rawOutput}`. Status is `completed`, or `failed` when `Result.IsError`, `Error != nil`, denied, not run, or unknown. Content is the final diff for edit tools, otherwise `Display` (Markdown) or the text content. Add `_meta.terminal_exit` for execute tools. |
| `Suspended` (deprecated) | Ignored; use `TurnEnded`. |
| `TurnEnded` | `usage_update {used: last call's input+output, size: model window, cost}`; `plan` if it changed; then the prompt response. |

### 6.3 Permissions, questions, outcomes

**The permission bridge.** In Dive today this is a `PreToolUse` hook. In v2 it
is `Policy.AuthorizeTool`.

1. Rules and mode decide first. Reuse `permission.Manager`: deny rules are
   absolute; then session grants, allow rules, ask rules, and mode
   ([permission.go L171-218][dive-eval]). Only "ask" reaches the client.
2. **Fail closed.** Each of the following is a denial recorded as a failed
   tool result:
   - a transport error;
   - `$/cancel_request` or a cancelled context;
   - an outcome of `cancelled`;
   - an unknown `optionId`;
   - a missing outcome.

   `permission.Manager` auto-allows when its dialog is nil
   ([permission.go L377][dive-nildialog]). The adapter always installs its own
   ACP dialog and refuses to construct otherwise.
3. **Bind the approval.** Record `(sessionId, toolCallId, sha256(effective
   input), optionId, time)`. Today Dive runs *all* `PreToolUse` hooks and
   applies `UpdatedInput` afterwards ([agent.go L2930-2956][dive-rewrite]), so
   a hook that runs after the permission hook can rewrite an approved call.
   Until Dive re-authorizes on a rewrite, the bridge must be the last hook, and
   the adapter must refuse to run when the executed input's hash differs from
   the approved one. In v2, `tool_started` records both inputs and the
   decision.
4. **Options:**
   - `allow_once`.
   - `allow_always`, only when a precise grant exists. Its label names the
     grant, for example "Always allow `go test *` in this session", which is
     `permission.Manager`'s exact-specifier session grant.
   - `reject_once`.
   - `reject_always` adds a session deny rule. Grants live in memory by
     default. Persisting them to session metadata is opt-in, and a restored
     grant is never implied by replay.
5. **Order:** `tool_call` (pending, diff) → request → `in_progress` →
   terminal status. A denial becomes a failed tool result that the model sees,
   and the turn continues. A rejected plan exit ends the turn.

**AskUser to elicitation.**
- *Synchronous `AskUserTool`* calls `Dialog.Show`
  ([ask_user.go L252-264][dive-askuser]). The ACP dialog maps it to
  `elicitation/create` in form mode, scoped to `{sessionId, toolCallId}`. The
  field types map as follows:
  - Confirm → boolean;
  - Options → enum;
  - multi-select → array of enums;
  - free text → string.

  `accept` fills `DialogOutput`, `decline` sets `Canceled`, and `cancel` sets
  `Canceled` and cancels the turn.
- *Without elicitation* (acpx today): use the async `AskUserTool`. It suspends,
  the question is streamed as an `agent_message_chunk`, and the turn returns
  `end_turn` with `_meta.dive.status = "suspended"` and the pending call id.
  The next `session/prompt` answers it through `WithToolResults`. This is the
  A2A input-required pattern ([executor.go L267-310][dive-a2a-suspend]).

  A question is never a permission. An open elicitation is cancelled by
  `session/cancel`.

**Terminal state → v1 `stopReason`** (Dive's `TurnReason` is in
[outcome.go L188-245][dive-reasons]):

| Dive | v1 wire | What is lost |
|---|---|---|
| completed | `end_turn` | — |
| completed with a refusal stop kind | `refusal`. Stream the refusal text first. Beware: Zed deletes the user message unless a completed call with `rawOutput` followed it | the refusal category (`StopDetails`) |
| suspended (AskUser async, external tool) | `end_turn`, with `_meta.dive {status: suspended, pending: [...]}` | that the turn is waiting; which calls; "the next prompt resolves" |
| incomplete: `canceled` | `cancelled` (always, even with unknown calls: [prompt-turn L361][s-cancelmust]) | uncertain effects; state them in the call's `failed` content |
| incomplete: `output_limit` | `max_tokens` | `Next=continue` (Zed tells the user to ask it to continue) |
| incomplete: `context_limit` | `max_tokens` | that the history must be shortened first; Zed's text is misleading |
| incomplete: `iteration_limit`, `pause` | `max_turn_requests` | which limit |
| incomplete: `deadline`, `provider_error`, `stream_interrupted`, `callback_error`, `hook_abort`, `error`, `process_exit`, `provider_stopped` | JSON-RPC error `-32603` with `data {reason, next, turnId, persistence, toolCalls[]}` | none in `data`, but Zed has no Retry and `data` only appears as pretty JSON |
| any call with state `unknown` (`Next=reconcile`) | Tool call `failed` with "outcome unknown — may have taken effect" and `_meta.dive.state = "unknown"`; the prompt follows the table above | the distinction between failed and possibly-done, for clients that ignore `_meta` |
| `Persistence` failed or unknown | `_meta.dive.persistence` on the response | whether the turn was saved |

Every response also carries `_meta.dive {status, reason, next, turnId,
persistence}`. It is an extension that clients may ignore
([extensibility][s-ext]).

### 6.4 Load and resume from a Dive session store

- **Resume** (`sessionCapabilities.resume`): open the store record, rebuild the
  agent for `cwd` and the client's MCP servers, restore mode and config, and
  return. It **MUST NOT replay** ([session-setup L243][s-noreplay]).
- **Load** (`loadSession`): iterate `session.Turns()`
  ([turns.go L175][dive-turns]), which includes turns a compaction summarized.
  For each turn:
  - Each user text block becomes a `user_message_chunk`. **Skip** reminder and
    turn-outcome blocks.
  - Assistant text and thinking become chunks with the derived `messageId`.
  - Each `tool_use` becomes one `tool_call` with its **final** status:
    completed or failed from its `tool_result`; failed with "not run" for
    `not_started`; failed with "unknown" for `unknown`; pending for `waiting`
    on a suspended turn.
  - The call's content is the persisted display content. Its diff comes from
    the display record (§6.5); until then, `Display` text.
  - A superseded incomplete turn is replayed as is.

  Afterwards send the latest `plan`, and **await** all of it before
  responding. Then send `available_commands_update`, the mode and config
  updates, `usage_update`, and `session_info_update`.
- **Open turn at load:** if a step checkpoint left a turn `running`, Dive
  closes it as `process_exit` when it loads. Replay shows its calls as
  unknown, and the next prompt starts a new turn. The adapter never resumes
  that turn automatically.
- **Continuity probe in tests:** after a restart, both load and resume must
  answer with the same model-visible history. The spike checked this through
  the message-count probe. See [acp-issues.md](acp-issues.md#open-issues-that-affect-client-and-adapter-contracts)
  for Claude adapter bugs where it failed.

### 6.5 Core changes the adapter needs

| # | Change | Why | v2 equivalent |
|---|---|---|---|
| 1 | Put `ToolCallPreview` on the `ToolCall` item. Today it is only on `ToolCallResult` ([tool.go L623-633][dive-preview]). | Title before approval | Presentation derived from `tool_started` or the `Def` |
| 2 | A `ToolStarted` item after authorization | Honest `in_progress`; terminal start | `tool_started` step |
| 3 | Per-model-call usage on an item | The context meter | `model_completed.Result.Usage` |
| 4 | A `toolkit.FileSystem` seam on Edit, Write and Read. Only `TextEditorTool` has one ([text_editor.go L97-99][dive-fs]); Write calls `os.WriteFile` ([write_file.go L161][dive-oswrite]). | Route through `fs/*` for Zed review, Follow and unsaved buffers | `tool` operations interface |
| 5 | A tool kind: `ToolAnnotations.Kind` or `tool.Def.Kind`-like presentation hints | Icons and layout; today the adapter keeps a name table | `Annotations` |
| 6 | A diff preview interface (`PreviewDiff(ctx, input) (path, old *string, new string)`), and persisted display content (`ToolResult.Display` plus a structured diff) | Whole-file diff before approval; diffs on replay | `Output.Display` in `tool_completed` |
| 7 | Re-authorize after a `PreToolUse` rewrite, or make `UpdatedInput` visible to later hooks | Approval binding | Policy "a rewrite re-enters authorization" |
| 8 | Configurable fail-closed behavior for a nil dialog in `permission.Manager` | Never approve silently | Policy "an absent approver is a configured decision" |
| 9 | Reserved message ids (deterministic, or in a `message_start` item) | Unique, replay-stable `messageId` | Observer `Delta` with a reserved message id |
| 10 | An optional session title | `session/list`, `session_info_update` | Store convenience |

### 6.6 Dive v2 vocabulary → ACP

The v2 model ([conceptual model][dive-v2]) removes most of the adapter's
guesswork. Committed steps drive the durable parts of the ACP stream, and
Observer deltas drive previews.

| v2 Step or Event | ACP v1 emission | Notes |
|---|---|---|
| `turn_started` | Live: nothing, since Zed shows the prompt itself. Replay: a `user_message_chunk` for each input item with `Origin: User` | Replay no longer needs to skip synthetic messages: the record holds none |
| `context_delivered` (`Origin: User`, a nudge) | Replay: `user_message_chunk`. Live: nothing | Other origins are invisible |
| `model_requested` | Nothing (optionally `_meta` for tracing) | |
| Observer `ModelDelta{text \| reasoning}` | `agent_message_chunk` / `agent_thought_chunk` with the reserved message id | |
| Observer `ModelDelta{tool-arguments}`, first fragment | `tool_call {pending, title: name, kind}` | One canonical call id minted at block start (the Nvoken fix) |
| `model_completed` | `usage_update`; replay: full chunks | Usage per attempt |
| `model_failed` | Nothing live; the turn stop decides | |
| `Policy.AuthorizeTool` returning "ask" | `session/request_permission` with the effective args | The decision is recorded on `tool_started` or `tool_completed{NotExecuted}` |
| `tool_started` | `tool_call_update {in_progress}` + `terminal_info` | Requested and effective args both recorded |
| Observer `ToolOutput` / `ToolProgress` | `terminal_output` meta / throttled content | |
| `tool_completed{Succeeded \| Failed}` | `completed` / `failed` + content (diff) + `rawOutput` + `terminal_exit` | |
| `tool_completed{NotExecuted}` | `failed`, "not run: <decision>" | Zed shows Rejected when the user rejected it |
| `tool_waiting{Waiting}` | Call stays `pending`; `elicitation/create` if supported, otherwise end the invocation | |
| `tool_waiting{Detached}` | `in_progress`; a later `Accept` sends `tool_call_update` **outside a turn**, which v1 permits ([PR #2214][s-2214]) | Background work that outlives the turn remains an open protocol issue ([#1847][s-1847]) |
| `tool_uncertain` | `failed` + "outcome unknown" + `_meta.dive.state = uncertain` | |
| `turn_stopped{Completed}` | `end_turn` | |
| `turn_stopped{Waiting}` | `end_turn` + `_meta.dive` | Lossy in v1; in v2 it is `state_update: requires_action` |
| `turn_stopped{Canceled}` | `cancelled` | |
| `turn_stopped{Limited: output \| context}` | `max_tokens` | |
| `turn_stopped{Limited: model_calls \| pause}` | `max_turn_requests` | |
| `turn_stopped{Refused}` | `refusal` | |
| `turn_stopped{Failed \| Uncertain}` | JSON-RPC error with `data` (uncertain + cancel → `cancelled`) | |
| `StepCommitted{Seq}` | The replay cursor | Maps directly to v2 `session/resume {replayFrom}`; v1 load replays from `Seq 0` |

## 7. Proposed `dive/acp` package API

A separate module, `github.com/deepnoodle-ai/dive/acp`, depending on `dive`,
`dive/session` and `dive/permission`. Wire types are internal.

```go
package acp

// Server serves one ACP client connection (stdio or any byte stream).
type Server struct{ /* unexported */ }

type Options struct {
	Info     Implementation // name, title, version for agentInfo
	Sessions SessionStore   // required; session.FileStore satisfies it via an adapter
	// NewAgent builds the Dive agent for a session. It is called on new, load
	// and resume with the session's cwd, the client's MCP servers and
	// capabilities, and the client-backed services the adapter prepared.
	NewAgent func(ctx context.Context, sc *SessionContext) (*dive.Agent, error)

	Permissions PermissionConfig // required; there is no implicit allow
	Presenter   Presenter        // kind, title, locations and diff per tool; nil uses DefaultPresenter
	Modes       []Mode           // exposed as a "mode" config option and legacy modes
	Models      []ModelChoice    // "model" config option; SessionContext.Model reflects the choice
	Thinking    []ThinkingLevel  // "thought_level" config option
	Commands    []Command        // available_commands_update; Handle runs locally or rewrites the prompt
	Titles      TitleFunc        // optional session_info_update after the first turn
	Auth        []AuthMethod     // advertised; ErrAuthRequired from NewAgent maps to -32000
	Logger      *slog.Logger     // stderr only; stdout is the protocol
}

func NewServer(opts Options) (*Server, error)
func (s *Server) ServeStdio(ctx context.Context) error
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error

// SessionContext is what NewAgent receives.
type SessionContext struct {
	ID             string
	Cwd            string
	AdditionalDirs []string
	MCPServers     []MCPServer
	Client         ClientCapabilities
	Session        dive.Session        // the store's session, ready for WithSession
	FS             toolkit.FileSystem  // fs/read_text_file and fs/write_text_file when advertised, else local disk
	Dialog         dive.Dialog         // elicitation/create when advertised, else ErrNoElicitation
	PermissionHook dive.PreToolUseHook // the fail-closed bridge; must be the last PreToolUse hook
	Mode           string
	Model          string
}

type SessionStore interface {
	Create(ctx context.Context, meta SessionMeta) (dive.Session, error)
	Open(ctx context.Context, id string) (dive.Session, SessionMeta, error)
	List(ctx context.Context, cwd string, cursor string) ([]SessionMeta, string, error)
	Close(ctx context.Context, id string) error
}

type SessionMeta struct {
	ID, Cwd, Title string
	AdditionalDirs []string
	UpdatedAt      time.Time
	Config         map[string]string // mode, model, thought_level
}

// Presenter turns a tool call into what the client renders.
type Presenter interface {
	Present(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent) Presentation
}

type Presentation struct {
	Kind      ToolKind   // read, edit, delete, move, search, execute, think, fetch, switch_mode, other
	Title     string     // Markdown; the raw command for execute
	Locations []Location // absolute paths
	Diff      *Diff      // whole-file; Old == nil for a new file
	Terminal  bool       // render through the terminal _meta extension
}

type PermissionConfig struct {
	Manager     *permission.Manager                        // rules and mode; ask goes to the client
	AlwaysLabel func(tool string, spec string) (string, bool) // false hides allow_always
	PersistGrants bool
}

// Test support: a conformance suite run against any Options.
package acptest
func Conformance(t *testing.T, newServer func(t *testing.T) *acp.Server)
```

`DefaultPresenter` knows the toolkit's tools:

| Tool | Kind |
|---|---|
| Read | read |
| Glob, Grep, GrepSearch, ListDirectory | search |
| Edit, Write, TextEditor | edit |
| Bash | execute |
| Fetch, WebSearch | fetch |
| subagent / Task | think |
| TodoWrite | `plan` instead of a tool call |
| anything else | other |

For any other tool it falls back to annotations: `ReadOnlyHint` → read,
`EditHint` → edit. `acptest.Conformance` would drive the spike's scenarios
with the regenerated Go client and acpx: prompt, permissions, cancel, cancel
during a permission prompt, restart with load and resume, close, and a prompt
while busy. It then checks the frames against `schema/v1/schema.json`.

## 8. Open questions

1. **Zed's 0-based `line`:** file it upstream, or make the location row a
   client quirk in the Presenter? acpx ignores lines.
2. **Client-side writes versus Dive's local tools:** routing Edit and Write
   through `fs/write_text_file` gives Zed review, but it moves the effect into
   the editor. Should a failed client write be `Failed` or `Unknown`? v2 needs
   an answer, because the write may have landed.
3. **Real client terminals** (`terminal/create`) versus the `_meta` display
   terminal. Real terminals run the command in Zed's environment, not Dive's
   sandbox. The recommendation is display-only for now; is that right for
   hosted Dive?
4. **Grant persistence:** should `allow_always` survive a restart, which
   neither Zed nor the spec defines? Default no; revisit with Nvoken's
   approval product (PRD 052).
5. **The suspended-turn UX in v1:** is `end_turn` plus a streamed question
   acceptable in Zed, or should AskUser require elicitation and fail otherwise?
6. **Model and provider switching mid-session** through the `model` config
   option: this touches cross-provider transcript fidelity, which the pi
   survey calls the defining problem.
7. **`usage_update.size` for models with unknown windows:** send 0, which
   suppresses the warning, or omit the update?
8. **v2 timing:** the draft replaces the prompt response with a
   `state_update`, and `tool_call` with upserts. Should `internal/wire/v2` wait
   for Zed to negotiate v2, or ship behind a flag sooner, since Dive v2's
   Step/Observer vocabulary matches it more closely than v1 does?
9. **Nvoken:** should the gateway expose ACP by projecting its durable
   transcript through this same package (a `SessionStore` over Nvoken), or
   keep a separate projection? This package's `SessionStore` seam makes the
   first possible.

<!-- Sources -->
[z-caps]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L632-L668
[z-version]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L837-L839
[z-info]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L853-L861
[z-autherr]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L1952-L1964
[z-authui]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_ui/src/conversation_view.rs#L1486-L1532
[z-drop]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L5323-L5332
[z-reapply]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L1570-L1617
[z-loadpref]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_ui/src/conversation_view.rs#L1163-L1190
[z-load]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L1085-L1255
[z-list]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L432-L471
[z-close]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L145-L159
[z-noretry]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/connection.rs#L188-L233
[z-chunkmerge]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L383-L391
[z-chunks]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L3260-L3359
[z-user]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L3034-L3063
[z-upsert]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L3742-L3810
[z-termreject]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L2201-L2205
[z-unknown]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L3668-L3706
[z-apply]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L1057-L1187
[z-rawout]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L1189-L1224
[z-plan]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L4137-L4167
[z-cmdreject]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_ui/src/message_editor.rs#L751-L864
[z-cfgprec]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L4946-L4960
[z-cfgonce]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L5347-L5356
[z-usage]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L3132-L3144
[z-meter]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_ui/src/conversation_view/thread_view.rs#L4638-L4670
[z-title]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L3105-L3116
[z-kindicon]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_ui/src/conversation_view/thread_view.rs#L9994-L10003
[z-card]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_ui/src/conversation_view/thread_view.rs#L8154-L8182
[z-titlerender]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L1017-L1055
[z-diff]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/diff.rs#L18-L85
[z-write]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L4894-L4987
[z-read]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L4814-L4892
[z-locrow]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L1311-L1316
[z-locrow2]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_ui/src/conversation_view/thread_view.rs#L10158-L10160
[z-termmeta]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L5374-L5476
[z-perm]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_servers/src/acp.rs#L5083-L5133
[z-permui]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_ui/src/conversation_view/thread_view.rs#L9772-L9890
[z-permresult]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L3985-L4030
[z-noalways]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent/src/tool_permissions.rs#L222
[z-runturn]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L4286-L4470
[z-err]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/agent_ui/src/conversation_view.rs#L165-L247
[z-cancel]: https://github.com/zed-industries/zed/blob/bda9c0bd43a8d235d82adb01ea5bc875b861ecfc/crates/acp_thread/src/acp_thread.rs#L4476-L4570
[ca-init]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L2320-L2388
[ca-kinds]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/tools.ts#L150-L513
[ca-stream]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L10419-L10467
[ca-before]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L7301-L7400
[ca-editpre]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/tools.ts#L219-L268
[ca-editpost]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L10061-L10100
[ca-term]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/tools.ts#L823-L969
[ca-plan]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L10038-L10045
[ca-cmds]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L2392-L2444
[ca-modes]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/session-mode.ts#L315-L350
[ca-always]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/permissions/options.ts#L25-L99
[ca-load]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L6956-L7268
[ca-usage]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L5302-L5319
[ca-stop]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L5498-L5715
[ca-cancel]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts#L6467-L6700
[cx-moved]: https://github.com/zed-industries/codex-acp/blob/296069e841634cd4bb9bc4515602d836e49231ec/README.md#L3-L6
[cx-sse]: https://github.com/zed-industries/codex-acp/blob/296069e841634cd4bb9bc4515602d836e49231ec/src/codex_agent.rs#L345-L427
[cx-parse]: https://github.com/zed-industries/codex-acp/blob/296069e841634cd4bb9bc4515602d836e49231ec/src/thread.rs#L2546-L2593
[cx-permfirst]: https://github.com/zed-industries/codex-acp/blob/296069e841634cd4bb9bc4515602d836e49231ec/src/thread.rs#L1932-L1961
[cx-n2]: https://github.com/zed-industries/codex-acp/blob/296069e841634cd4bb9bc4515602d836e49231ec/src/thread.rs#L2094-L2112
[cx-plan]: https://github.com/zed-industries/codex-acp/blob/296069e841634cd4bb9bc4515602d836e49231ec/src/thread.rs#L2708-L2724
[cx-cmds]: https://github.com/zed-industries/codex-acp/blob/296069e841634cd4bb9bc4515602d836e49231ec/src/thread.rs#L2903-L2934
[cx-trust]: https://github.com/zed-industries/codex-acp/blob/296069e841634cd4bb9bc4515602d836e49231ec/src/thread.rs#L3284-L3322
[cx-opts]: https://github.com/zed-industries/codex-acp/blob/296069e841634cd4bb9bc4515602d836e49231ec/src/thread.rs#L2412-L2526
[cx-replay]: https://github.com/zed-industries/codex-acp/blob/296069e841634cd4bb9bc4515602d836e49231ec/src/thread.rs#L3376-L3686
[cx-cancel]: https://github.com/zed-industries/codex-acp/blob/296069e841634cd4bb9bc4515602d836e49231ec/src/thread.rs#L3328-L3335
[cxn-whole]: https://github.com/agentclientprotocol/codex-acp/blob/bf37821e8f3c1f1e9b6954171855a9e2579cd2c9/src/CodexToolCallMapper.ts#L839-L915
[gm-init]: https://github.com/google-gemini/gemini-cli/blob/2fe7c2d3f065dc40ad573d50b2091116f8a4aa18/packages/cli/src/acp/acpRpcDispatcher.ts#L47-L103
[gm-kind]: https://github.com/google-gemini/gemini-cli/blob/2fe7c2d3f065dc40ad573d50b2091116f8a4aa18/packages/cli/src/acp/acpUtils.ts#L202-L222
[gm-phantom]: https://github.com/google-gemini/gemini-cli/blob/2fe7c2d3f065dc40ad573d50b2091116f8a4aa18/packages/cli/src/acp/acpSession.ts#L923-L934
[gm-fs]: https://github.com/google-gemini/gemini-cli/blob/2fe7c2d3f065dc40ad573d50b2091116f8a4aa18/packages/cli/src/acp/acpFileSystemService.ts#L26-L87
[gm-autoedit]: https://github.com/google-gemini/gemini-cli/blob/2fe7c2d3f065dc40ad573d50b2091116f8a4aa18/packages/core/src/scheduler/policy.ts#L190-L201
[gm-noawait]: https://github.com/google-gemini/gemini-cli/blob/2fe7c2d3f065dc40ad573d50b2091116f8a4aa18/packages/cli/src/acp/acpSessionManager.ts#L210-L212
[gm-stop]: https://github.com/google-gemini/gemini-cli/blob/2fe7c2d3f065dc40ad573d50b2091116f8a4aa18/packages/cli/src/acp/acpSession.ts#L470-L542
[go-make]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/Makefile#L13-L17
[go-overflow]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/connection.go#L446-L447
[go-prevcancel]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/agent_gen.go#L415-L417
[go-cancelctx]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/agent_gen.go#L299-L303
[go-cancelreq]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/connection.go#L701
[go-maxline]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/connection.go#L369
[s-1based]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/overview.mdx#L213-L215
[s-cancelorder]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/prompt-turn.mdx#L365
[s-cancelmust]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/prompt-turn.mdx#L354-L361
[s-inprogress]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/prompt-turn.mdx#L257
[s-msgid]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/session-setup.mdx#L176
[s-noreplay]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/session-setup.mdx#L243
[s-close]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/session-setup.mdx#L299
[s-ext]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/extensibility.mdx
[s-2214]: https://github.com/agentclientprotocol/agent-client-protocol/pull/2214
[s-1847]: https://github.com/agentclientprotocol/agent-client-protocol/issues/1847
[dive-a2a]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/a2a/executor.go#L136-L175
[dive-a2a-suspend]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/a2a/executor.go#L267-L310
[dive-items]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/response.go#L17-L60
[dive-gate]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/agent.go#L2811-L2860
[dive-rewrite]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/agent.go#L2930-L2956
[dive-eval]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/permission/permission.go#L171-L218
[dive-nildialog]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/permission/permission.go#L376-L378
[dive-askuser]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/toolkit/ask_user.go#L252-L264
[dive-reasons]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/outcome.go#L188-L245
[dive-turns]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/session/turns.go#L175-L193
[dive-preview]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/tool.go#L623-L633
[dive-fs]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/toolkit/text_editor.go#L97-L99
[dive-oswrite]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/toolkit/write_file.go#L161
[dive-v2]: ../design/2026-09-26-dive-v2-conceptual-model.md
