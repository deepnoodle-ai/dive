# ACP v2, protocol direction, and convergence with Dive v2, Nvoken, and OpenHands streaming

_Research snapshot: 2026-09-26 (America/New_York). This is a source review, not an interoperability test. Unless noted, ACP links pin the spec repository at [`128845f`](https://github.com/agentclientprotocol/agent-client-protocol/tree/128845f5bd4c7fca5f23374e9b4853e470ecf99a), the same commit the [earlier ACP research](README.md) used. Open PRs are pinned to their head commits at the time of reading. Where a statement is an inference rather than spec text, it says so._

Read with: [acp-spec-sdk.md](acp-spec-sdk.md) (v1 and SDKs), [acp-dive-nvoken.md](acp-dive-nvoken.md) (Dive/Nvoken mapping), [acp-issues.md](acp-issues.md), [comparison README §11 and §14](../comparison/README.md), [nvoken-cloud.md §4–5](../comparison/nvoken-cloud.md), [openhands.md §8](../comparison/openhands.md), and the [Dive v2 conceptual model](../design/2026-09-26-dive-v2-conceptual-model.md).

---

## 1. Summary and recommendations

**What ACP v2 is.** v2 is a consolidation release that is still labeled draft. Its baseline schema is `schema-v2.0.0-alpha.5` (2026-09-18). Four changes matter for Dive:

1. **The prompt response no longer ends the turn.** `session/prompt` returns `{messageId}` once the user message is *inserted*. After that, a `state_update` notification reports `running`, then `requires_action` or `idle` with a `stopReason`.
2. **Every displayed entity is an ID-keyed upsert.** This covers messages, thoughts, tool calls, terminals, and plans. An omitted field means unchanged, `null` means clear, a value means replace, and chunks append.
3. **`session/load` is replaced by `session/resume` with a `replayFrom` cursor.** Only `{type:"start"}` is defined so far.
4. **Some v1 surface is gone.** Client-side filesystem access, terminal execution, and session modes are removed.

**Where the field agrees.** Nvoken's SSE protocol, OpenHands' WebSocket v2, acpx's watch journal, and the Dive v2 proposal converge on the same core ideas:

- identity that the agent or server mints;
- a foreground lifecycle signal kept separate from content;
- replay from a cursor, then live delivery.

ACP v2 has moved toward the same model. Its lifecycle is now carried entirely in notifications, IDs are required and owned by the agent, and whole-message replacement lets a record overwrite an earlier preview.

**Where ACP v2 is weaker than Nvoken or OpenHands:**

- The wire does not mark which updates are durable and which are previews.
- There is no attempt or supersession field, and no removal operation.
- The core protocol has no cursor for resuming a live stream. The open transport proposal ([PR #2208](https://github.com/agentclientprotocol/agent-client-protocol/pull/2208)) adds only an in-memory cursor per connection.
- `replayFrom` cannot start from a position.
- Nothing synchronizes current state after replay.
- Approvals are RPCs bound to one connection, not durable waiting state.
- No stop reason means "failed".
- No turn identity links a state change to the input that caused it.
- Tool arguments cannot be streamed.

**Where ACP v2 is stronger:**

- Replace and clear semantics let a later record correct any earlier preview. This self-heals lossy previews without a resync frame.
- The extensibility rules are strict: open enums, a `_` prefix for private values, and a requirement to preserve unknown variants.
- Structured diffs and agent-owned terminals use snapshot-plus-chunk replay.
- The permission prompt's `title` and `subject` are separated.
- It has real ecosystem reach: Zed, JetBrains, and the agents listed in the registry.

**Recommendations:**

1. **Design Dive v2's Observer vocabulary so it projects onto ACP v2 without loss for the rendered view, and onto v1 with documented loss.** Do not use ACP as Dive's source vocabulary. Dive's event classes are *committed* (Recorder-acknowledged Steps) and *preview* (deltas carrying slot identity, attempt, and offset). Both carry strictly more information than ACP, and Nvoken needs them regardless. Treat ACP v1, ACP v2, Nvoken SSE, and AG-UI as pure projections from one stream. The Go sketches are in [§6](#6-recommended-dive-event-vocabulary-go-sketches-and-projection-rules).
2. **Adopt OpenHands' slot model for previews.** Use a stable slot ID that equals the future record ID, plus a monotonic `Attempt`. With that model, an attempt change maps onto ACP v2 as "reset message content, then append", which is lossless. Nvoken's current model (retry allocates a new `message_id`) would leave empty ghost messages in ACP. Mint tool-call IDs at block start, which also fixes [Nvoken discrepancy 1](../comparison/nvoken-cloud.md).
3. **Make "prompt accepted" mean "Recorder acknowledged the input step".** ACP's rule that acceptance means insertion is exactly Dive's commit receipt. Specify this in the adapter.
4. **Build now** (a staged version is in [§6.5](#65-build-now-vs-wait)):
   - the vocabulary;
   - a reference reducer with fixtures;
   - the ACP v1 agent adapter, with Zed as the target;
   - the ACP v2 projection *as a pure function with reducer-equivalence tests*.

   **Wait for alpha to end, or for the TS/Rust SDKs to ship v2 by default, before:**
   - shipping v2 on the wire;
   - the remote HTTP/WS transport;
   - `session/inject`;
   - multi-client attach;
   - session cursors.
5. **Engage the spec process, narrowly and now.** Comment on PR #2208, the durable/resumable transport proposal, from Nvoken's design. The asks, in priority order:
   1. Allow **state-equivalent replay**: missed chunks can be replaced by whole-entity upserts. Durable-log servers can then resume without a preview ring buffer, and resumption survives an agent restart.
   2. Scope the cursor to the **session stream**, not the connection, so a second reader can resume.
   3. Compose the cursor with `replayFrom`, through a replay-from-position variant, possibly unified with the [session cursors proposal (#2114)](https://github.com/agentclientprotocol/agent-client-protocol/pull/2114).
   4. Require `state_update` after *any* resume or replay, not only after transport resumption.
   5. Re-issue pending permission requests after reattach.
   6. Add a failure stop reason.

   Separately, add Nvoken's usage-bucket semantics to [#1860](https://github.com/agentclientprotocol/agent-client-protocol/issues/1860) and its detached/waiting tool model to [#1847](https://github.com/agentclientprotocol/agent-client-protocol/issues/1847). Details are in [§7](#7-spec-engagement-recommendation).

---

## 2. ACP v2 in depth

### 2.1 Status and packaging

- **Two v2 doc trees.**
  - `docs/protocol/v2/` describes the stable v2 baseline (`schema/v2/schema.json`).
  - `docs/protocol/v2/draft/` layers unstable features from `schema.unstable.json` on top: `plan_removed`, markdown and file plans, `notice`, `compaction_update`, `compaction_summary_chunk`, per-turn `usage` on idle, `session/fork`, and providers.
  - The migration guide says the whole v2 surface is still draft. Implementers should gate it behind version negotiation *and* feature flags. Negotiating `protocolVersion: 2` implies none of the unstable features. ([migration.mdx L18–22](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/migration.mdx#L18-L22))
- **Schema cadence.** Schema v2 moved from `alpha.1` (2026-07-20) to `alpha.5` (2026-09-18), released in lockstep with v1 schema minors `1.19.1` to `1.23.0` ([releases](https://github.com/agentclientprotocol/agent-client-protocol/releases), [schema/v2/CHANGELOG.md](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/schema/v2/CHANGELOG.md)).
- **Tracking RFD.** The v2 RFD is "Active". It lists twelve component RFDs, including the remote transport, which it counts as v2-targeted even though the transport is specified as additive ([rfds/v2/overview.mdx](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/rfds/v2/overview.mdx)).
- **No schema-level conversion.** The Rust schema crate deliberately provides no conversion between v1 and v2. SDKs are expected to expose separate versioned surfaces, or to build an adapter at their runtime boundary. This is the stated design, so an adapter that serves both versions should share application logic and keep two thin protocol surfaces ([migration.mdx L770–772](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/migration.mdx#L770-L772)).

### 2.2 Capabilities model

- **Unified naming.** Both sides send `info` (required) and `capabilities`. The v1 names `clientCapabilities`, `agentCapabilities`, `clientInfo`, and `agentInfo` are gone.
- **Presence markers.** Every support marker is an object: `{}` means supported, while omission or `null` means unsupported. Booleans remain only for actual configuration data.
- **Session group.** Everything session-scoped sits under `capabilities.session`, which is itself optional so that agents without sessions (for example next-edit-suggestion-only agents) can omit it.
- **Baseline.** If `session` is present at all, the agent must implement `session/new`, `list`, `resume`, `close`, `prompt`, `cancel`, and `update`.
- **Optional extras keep markers:** `session.delete`, `session.additionalDirectories`, `session.prompt.{image,audio,embeddedContext}`, and `session.mcp.{stdio,http}`. MCP over SSE is removed.
- **Auth.** Returning any `authMethods` commits the agent to both `auth/login` and `auth/logout`.
- **No client capabilities.** Stable v2 defines no standard client capability fields.

Sources: [migration.mdx L78–L191](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/migration.mdx#L78-L191) and [L639–L664](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/migration.mdx#L639-L664).

For Dive, the required baseline matters. A v2 Dive agent must support `list`, `resume`, and `close`, so it needs a real session store. The in-memory-only v1 shortcut does not exist in v2.

### 2.3 Prompt acceptance and `state_update`

**Acceptance means insertion.** The agent MUST respond to `session/prompt` once the user message has been inserted into the ACP conversation. It MUST NOT respond earlier because the input was received, queued, or assigned an ID. The response is `{messageId}`, a required, non-null, agent-generated ID. ([prompt-lifecycle.mdx §2](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/prompt-lifecycle.mdx#L137-L186), [PromptResponse schema](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/schema/v2/schema.json))

**Consequences:**

- The agent MUST also report the inserted message as a `user_message` update, or as `user_message_chunk` updates, with the same ID. The update may arrive before or after the response. Clients reconcile by `(sessionId, messageId)` and never by content or arrival order.
- Insertion is not a durability promise. The message may be live-only, and it may be missing from later replay. If it is replayed, it MUST keep its ID.
- A lost prompt response leaves an ambiguous outcome. There is no client idempotency key, and retrying can create a second submission. The RFD considered a client-generated `promptId` and deferred it. ([rfds/v2/prompt.mdx](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/rfds/v2/prompt.mdx))
- The RFD explicitly does **not** specify queueing, steering, or whether a busy agent inserts new prompts. A prompt "may contribute to work already in progress".

**`state_update`** reports *foreground* work for the whole session. Its states are `running`, `requires_action`, `idle`, and open extension values.

- The agent MUST send `running` when foreground work starts or resumes.
- It SHOULD send `requires_action` while blocked on a permission response or other user action, and `running` again once the block clears.
- It MUST send `idle` when it is ready for a new prompt, with a `stopReason` when the transition ends foreground work.
- Background activity MAY continue while the session is `idle` and does not change the state. ([prompt-lifecycle.mdx "Session States"](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/prompt-lifecycle.mdx#L512-L527), [StateUpdate schema](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/schema/v2/schema.json))

**Stop reasons** are unchanged from v1 (`end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, `cancelled`) but are now open: custom values must start with `_`. **No stop reason means "failed".** The prompt RFD lists "post-insertion failure reporting" as an unresolved follow-up, alongside cancellation races and "current-state synchronization on resume".

The unstable schema adds an optional per-turn `usage` to the idle update, carried over from the End-Turn Token Usage RFD, which is in Draft.

**Cancellation.** `session/cancel` is still a notification. The agent MUST flush pending updates and then send `idle` with `cancelled`. Any updates after the cancel MUST precede that idle. Clients answer all pending permission requests with `cancelled`, and they SHOULD mark unfinished tool calls cancelled optimistically. ([prompt-lifecycle.mdx "Cancellation"](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/prompt-lifecycle.mdx#L528-L560))

**What `state_update` does not carry:**

- **No turn identity.** An `idle` cannot be tied to the prompt or prompts it answers.
- **No reason for `requires_action`.** The RFD only muses that "we could explore adding which permission or elicitation it is waiting on".
- **No required current-state snapshot on resume.**

### 2.4 The `session/update` vocabulary (v2)

| Variant | Kind | Keyed by | Semantics |
|---|---|---|---|
| `user_message`, `agent_message`, `agent_thought` | upsert | `messageId` | `content` patch: omitted leaves it unchanged, `null` or `[]` clears it, an array replaces it. Top-level `_meta` follows the same patch rules. |
| `user_message_chunk`, `agent_message_chunk`, `agent_thought_chunk` | append | `messageId` (now required) | Appends one `ContentBlock` to current content. `_meta` applies to the chunk only. |
| `state_update` | state | session | `running`, `requires_action`, `idle`(+`stopReason`) |
| `tool_call_update` | upsert | `toolCallId` | Creates on first sight. `name`, `title`, `kind`, `status`, `content`, `locations`, `rawInput`, and `rawOutput` are patch fields. Arrays replace, and `[]` or `null` clear them. |
| `tool_call_content_chunk` | append | `toolCallId` | Appends one `ToolCallContent` item. |
| `terminal_update` | upsert | `terminalId` | `command`, `cwd`, `output` (a base64 **replacement snapshot**), and `exitStatus` |
| `terminal_output_chunk` | append | `terminalId` | Base64 bytes, decoded independently per chunk and then appended |
| `plan_update` | replace | `plan.planId` | Stable type is `items`. Each update replaces that plan's entries. |
| `available_commands_update` | snapshot | session | The full list |
| `config_option_update` | snapshot | session | The full `configOptions` (these replace modes) |
| `session_info_update` | patch | session | `title`, `updatedAt`, `_meta` |
| `usage_update` | snapshot | session | `used` and `size` (context tokens), plus optional cumulative `cost` |
| _draft:_ `plan_removed`, `notice`, `compaction_update`, `compaction_summary_chunk` | | | `notice` is **live-only** and SHOULD NOT be replayed. |

Sources: [prompt-lifecycle.mdx L12–L39](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/prompt-lifecycle.mdx#L12-L39), [draft variant list](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/draft/prompt-lifecycle.mdx), [migration.mdx L323–L435](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/migration.mdx#L323-L435), [session-notices RFD](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/rfds/session-notices.mdx).

**Tool calls.**

- `tool_call` is gone. The first `tool_call_update` for an unseen ID creates the call, and the agent SHOULD include `title` on it.
- `status` adds `cancelled` and is open to extension values. It defaults to `pending`, which the spec describes as "input is either streaming or awaiting approval".
- The spec defines **no monotonic transition rule**, and **no tool-argument streaming field**. `rawInput` is a whole-value replacement.
- `tool_call_content_chunk` exists so that `tool_call_update.content` can keep replacement semantics for replay, correction, and redaction. ([rfds/v2/tool-call-updates.mdx](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/rfds/v2/tool-call-updates.mdx), [tool-calls.mdx "Status"](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/tool-calls.mdx#L350-L373))

**Diffs** are now structured `changes` (add, delete, modify, move, copy, each with `fileType` and `mimeType`) plus an optional `git_patch` text. There is no mechanical mapping back to v1's `oldText`/`newText`.

**Terminals** are agent-owned and display-only. A `terminal` content item is a bare `terminalId` reference. `terminal_update.output` is an authoritative snapshot that replaces all bytes, so replay does not need every historical chunk.

### 2.5 Permission requests

`session/request_permission` now carries the following ([migration.mdx L478–L530](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/migration.mdx#L478-L530)):

- a required `title` for the prompt itself, which does *not* change the tool call's displayed title;
- an optional `description`;
- an optional `subject` tagged union:
  - `tool_call`: the same `ToolCallUpdate` upsert shape used in session updates;
  - `command`: `command` and an absolute `cwd`, with optional `toolCallId` and `terminalId` associations. Approving it authorizes the *agent* to run the command.
  - unknown subject types, which are preserved or declined.
- `options` and `outcome`, unchanged from v1. The outcome union is open, and **an outcome the agent does not understand MUST NOT be treated as approval**.

The agent SHOULD report `requires_action` while a request is pending.

What this does not fix:

1. **`subject` is optional.** An approval without a subject is still valid.
2. **The request is still a JSON-RPC request on one connection.** Nothing re-issues it after a reconnect or `session/resume`, and nothing tells other clients that it was resolved. [Multi-client attach (#533)](https://github.com/agentclientprotocol/agent-client-protocol/pull/533) proposes both re-issue and a `permission_resolved` notification.
3. **Patch fields can degrade silently.** The reference Rust v2 types still deserialize patch fields with `DefaultOnError`, and the default of `MaybeUndefined` is `Undefined`. A malformed but known `rawInput` therefore becomes "unchanged", not an error ([tool_call.rs L62–L81](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/agent-client-protocol-schema/src/v2/tool_call.rs#L62-L81), [serde_util.rs L750](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/agent-client-protocol-schema/src/serde_util.rs#L745-L760)). This is the v2 form of [#1979](https://github.com/agentclientprotocol/agent-client-protocol/issues/1979). The migration guide's "strict where it counts" SDK rule ([L766](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/migration.mdx#L762-L768)) points the other way, and the two have not been reconciled. **Implication for Dive:** a Dive client or gateway must bind an approval to the arguments *it* recorded, never to what the ACP payload happened to deserialize.

### 2.6 `session/resume` and `replayFrom`

- **Without replay.** Omitting `replayFrom`, or passing `null`, means reattach without replay. The agent MUST NOT replay before responding.
- **Full replay.** `replayFrom: {type:"start"}` means replay **all retained** history as ordinary `session/update` notifications, *then* respond.
- **Cursor semantics.** Cursors are **inclusive**, which matters for future variants that name a message. Unknown cursor types MUST be rejected rather than guessed.
- **Chunked replay.** A message replayed as chunks MUST first be reset with a whole-message update carrying `content: []`, so chunks do not append to a copy the client already holds.
- **Not replayed.** Live-only messages, such as locally handled commands, may be absent. Replaying a command does not re-execute it.
- **State after replay is unspecified.** The spec does not require the replay to end with a `state_update` or with the current values of config, commands, usage, or info. It also does not say whether a pending permission is re-requested.

Sources: [session-setup.mdx L118–L233](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/session-setup.mdx#L118-L233), [rfds/v2/session-resume-replay.mdx](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/rfds/v2/session-resume-replay.mdx).

`replayFrom` is a **conversation replay cursor for rebuilding history**. It is not a position in a live stream. The RFD anticipates future variants ("from a specific message, checkpoint, or server-provided cursor"), but only `start` exists today.

### 2.7 Removed surface

v2 removes:

- `fs/*`, `terminal/*` (client execution), `clientCapabilities.fs`, and `clientCapabilities.terminal`;
- `session/set_mode`, `modes`, and `current_mode_update` (use config options with `category: "mode"`);
- `session/load`;
- the `authenticate` and `logout` names (now `auth/login` and `auth/logout`);
- the MCP SSE transport.

Clients that want to expose files, editor state, or command execution do so by passing an **MCP server** in `mcpServers`. ([migration.mdx L606–L637](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/migration.mdx#L606-L637))

For Dive this is good news. A Dive agent never needs a second, client-side execution path for files or shell. Dive's own tools report diffs and agent-owned terminal output.

### 2.8 Transport details that affect a reducer

- stdio now follows JSON-RPC 2.0 batch rules.
- The receiver **"MAY process batch entries as concurrent tasks, in any order"** ([transports.mdx L44–L80](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/transports.mdx#L44-L80)). The spec only warns against batching lifecycle requests. It does not warn against batching order-dependent `session/update` notifications, such as a replacement followed by chunks for the same `messageId`.
- **Rule for Dive's adapter:** never batch `session/update`. A Dive client should apply the notifications in a received batch in array order. That is conservative and allowed, because the batch rules permit but do not require concurrent processing.

### 2.9 The v2 client reducer, precisely

The following is a normative-as-possible reading of the stable v2 schema and guides. Bracketed notes mark inferences where the spec is silent.

```go
// Three-state patch field. The spec distinguishes Omitted, Null, and Value.
type Patch[T any] struct {
    Present bool // key present on the wire
    Null    bool // value was JSON null
    Val     T
}

type View struct {
    Order     []EntityRef                 // first-seen order of messages, tool calls, plans [inference]
    Messages  map[string]*Message         // by messageId
    Tools     map[string]*ToolCall        // by toolCallId
    Terms     map[string]*Terminal        // by terminalId
    Plans     map[string]json.RawMessage  // by planId (entries replaced wholesale)
    State     string                      // "running" | "requires_action" | "idle" | unknown raw
    Stop      *string                     // stopReason from the last idle transition
    Commands  []json.RawMessage           // available_commands_update snapshot
    Config    []json.RawMessage           // config_option_update snapshot
    Info      SessionInfo                 // title, updatedAt, _meta (patched)
    Usage     *Usage                      // used, size, cost (snapshot)
    Unknown   []json.RawMessage           // unrecognized variants, preserved verbatim
}

type Message struct {
    Role    string          // "user" | "agent" | "thought", fixed by the first variant seen
    Content []ContentBlock
    Meta    json.RawMessage // top-level _meta; patch semantics on upserts
}

// Apply is called in receipt order. Batch entries are applied in array order (see §2.8).
func (v *View) Apply(u Update) {
    switch u.Kind {
    case "user_message", "agent_message", "agent_thought":
        m := v.message(u.MessageID, roleOf(u.Kind))   // create on first sight, empty content
        switch {
        case !u.Content.Present:                      // omitted: leave content
        case u.Content.Null || len(u.Content.Val) == 0:
            m.Content = nil                           // null or [] clears
        default:
            m.Content = clone(u.Content.Val)          // replaces chunk-accumulated content too
        }
        patchMeta(&m.Meta, u.Meta)                    // omitted: keep; null: clear; object: replace

    case "user_message_chunk", "agent_message_chunk", "agent_thought_chunk":
        m := v.message(u.MessageID, roleOf(u.Kind))   // create on first sight
        m.Content = append(m.Content, u.Chunk)        // chunk _meta is chunk-scoped; not stored on m

    case "tool_call_update":
        t := v.tool(u.ToolCallID)                     // create on first sight; Status defaults to "pending"
        patch(&t.Name, u.Name); patch(&t.Title, u.Title); patch(&t.Kind, u.Kind)
        patch(&t.Status, u.Status)                    // no transition rule; a later value simply wins
        patchSlice(&t.Content, u.Content)             // array replaces; [] or null clears
        patchSlice(&t.Locations, u.Locations)
        patch(&t.RawInput, u.RawInput); patch(&t.RawOutput, u.RawOutput)
        patchMeta(&t.Meta, u.Meta)

    case "tool_call_content_chunk":
        t := v.tool(u.ToolCallID)                     // [inference] create on first sight, like message chunks
        t.Content = append(t.Content, u.ToolContent)

    case "terminal_update":
        term := v.term(u.TerminalID)                  // chunk-before-update is legal: keep state for first-seen IDs
        patch(&term.Command, u.Command); patch(&term.Cwd, u.Cwd)
        if u.Output.Present {
            if u.Output.Null { term.Bytes = nil } else { term.Bytes = b64(u.Output.Val.Data) } // snapshot REPLACES
        }
        patch(&term.Exit, u.ExitStatus)               // concrete value marks the terminal exited

    case "terminal_output_chunk":
        v.term(u.TerminalID).Bytes = append(v.term(u.TerminalID).Bytes, b64(u.Data)...) // decode each chunk, then append

    case "plan_update":
        v.plans(u.Plan.PlanID, u.Plan)                // replace that plan's content; unknown plan.type preserved
    case "plan_removed":                              // draft only
        delete(v.Plans, u.PlanID)

    case "state_update":
        switch u.State {
        case "idle":
            v.State, v.Stop = "idle", u.StopReason    // stopReason may be absent: idle without ending work
        case "running", "requires_action":
            v.State, v.Stop = u.State, nil           // [inference] clear the stale stop reason
        default:
            v.State = u.State                         // unknown: preserve; do NOT infer idle
        }

    case "available_commands_update": v.Commands = u.Commands   // full list
    case "config_option_update":      v.Config = u.ConfigOptions // full list
    case "session_info_update":       patch(&v.Info.Title, u.Title); patch(&v.Info.UpdatedAt, u.UpdatedAt); patchMeta(&v.Info.Meta, u.Meta)
    case "usage_update":              v.Usage = &Usage{u.Used, u.Size, u.Cost} // snapshot, not a patch
    default:
        v.Unknown = append(v.Unknown, u.Raw)          // preserve for storage, replay, and proxying
    }
}
```

Properties and edge cases worth pinning with fixtures:

1. **Per-ID order is authoritative; cross-ID placement is by first sight.** [Inference] The spec says to apply updates "in the order they are received for each `messageId`/`toolCallId`" and never defines placement otherwise. A later upsert to an old ID changes its content, not its position. OpenHands' `anchor_seq` exists precisely to pin a preview's position; ACP has no equivalent.
2. **Replacement makes previews self-healing.** If chunks were lost or duplicated, a later `agent_message{content:[…]}` restores the exact record. This is ACP's substitute for Nvoken's `stream.resync` and OpenHands' "durable frame retires the slot".
3. **Nothing can be removed.** The stable schema has no removal operation for messages, tool calls, or terminals; only draft plans have one. A cleared message is an empty entity, not an absent one. Replay after reconnect also cannot tell a client that an entity it holds is gone, for example a live-only message.
4. **An empty idle is legal.** `idle` without `stopReason` means "ready, and this transition did not end foreground work".
5. **Replay feeds the same reducer.** Resuming with `start` into an existing `View` converges for replayed entities. Stale entities remain unless the client resets its view first. The spec does not say whether a client should reset.
6. **Messages and tools have separate ID spaces.** Nothing forbids a `messageId` equal to a `toolCallId`. Key each map separately.
7. **Silent degradation.** With the reference Rust types, a malformed *known* patch field decodes as "omitted" (§2.5). A strict Dive client should decode patch fields itself and treat a malformed known field as a protocol error.

### 2.10 What projects between v1 and v2, and what is lost

| Direction | Representable | Lost or stateful |
|---|---|---|
| v1 → v2 | `tool_call` and `tool_call_update` become `tool_call_update`. `plan` becomes `plan_update` with a synthesized `planId`. `session/load` becomes `resume{start}`. The `stopReason` of a prompt response becomes `idle`. | v1 client-side fs and terminal calls have no v2 equivalent. Modes become config options. |
| v2 → v1 | Upserts with concrete values. Collection clears as `[]`. Message chunks. | `null` clears of scalar or raw fields ("a strict adapter must reject"). `tool_call_content_chunk` needs a stateful bridge that accumulates chunks. Whole-message *replacement* has no v1 op, because v1 chunks only append. `requires_action` and background idle have no v1 form. Agent-owned terminals have no v1 form, because v1 terminals execute on the client. Subject-less and `command` permissions need a synthetic `toolCall`. Multiple `planId`s collapse to one plan. |

Sources: [tool-call-updates RFD "Compatibility"](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/rfds/v2/tool-call-updates.mdx), [migration.mdx](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/migration.mdx).

---

## 3. RFDs, open proposals, and governance

### 3.1 Proposals that bear on streaming, durability, and lifecycle

| Proposal | Stage (at snapshot) | What it does | Relevance to Dive/Nvoken |
|---|---|---|---|
| [Streamable HTTP and WebSocket transport](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/rfds/streamable-http-websocket-transport.mdx) | **Active** (Transports WG). Authors alexhancock and jh-block; champion anna239. | One `/acp` endpoint. POST returns 202, except `initialize`, which returns 200 with a JSON body. Long-lived SSE GET streams: one per connection (`Acp-Connection-Id`) and one per session (`+Acp-Session-Id`). WebSocket upgrade on the same endpoint, and clients MUST support both. HTTP/2 and cookies (for affinity) are required. Reference implementation: Goose. | Its [v1 durability section](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/rfds/streamable-http-websocket-transport.mdx#L64-L86) says "in-flight messages are not replayed". Sequencing, `Last-Event-ID` resumption, and keepalive are deferred to "v2". The TS SDK already ships it and repeats that v1 reconnect is `initialize`→`session/load` with no replay ([http-stream.ts L55–L66](https://github.com/agentclientprotocol/typescript-sdk/blob/4356253eb95ad02d9278af93e9ae6a28cf1d7a67/src/http-stream.ts#L55-L66)). |
| [Durable and resumable HTTP/WS, PR #2208](https://github.com/agentclientprotocol/agent-client-protocol/pull/2208) ([text @fc269b3](https://github.com/agentclientprotocol/agent-client-protocol/blob/fc269b332c7422eeeb88438a7e6d26fe3e845544/docs/rfds/durable-resumable-http-transport.mdx)) | Open PR, opened 2026-09-22. No comments or reviews yet. | A cursor per stream on every server→client message: SSE `id:` plus `_meta["acp/cursor"]`, opaque and monotonic *within that stream*. Resume uses `Last-Event-ID`, or `stream/resume` on WebSocket, and replays "the same messages, including their cursors". If the server cannot replay, it MUST refuse (409 or `stream/resume_failed`) and never silently open live. Fallback is `session/resume`+`replayFrom`. Servers advertise `capabilities.transport.resumable{retentionMessages,retentionSeconds}`. After a successful resume the server **MUST send `state_update`** before live data. Streams dropping must not cancel work. Keepalive: a client that sees 5 silent intervals reconnects. v1 gets a `_meta["acp/turnState"]` shim. | **Closest to Nvoken, and the best place to engage** (§7). It explicitly rejects durable replay logs as "scope creep" and expects in-memory ring buffers. Its cursor is per connection stream, so a second reader cannot use it. It has no preview/durable distinction and requires byte-identical replay. |
| [Session cursors, PR #2114](https://github.com/agentclientprotocol/agent-client-protocol/pull/2114) ([text @7a93424](https://github.com/agentclientprotocol/agent-client-protocol/blob/7a93424abf432d99104ef482044518e15c9b76a1/docs/rfds/session-cursors.mdx)) | Open, 2026-09-07, no reviews | Adds a `sessionUpdate: "cursor"` update: an opaque cursor, `validFor:[ops]`, and an optional `beforeMessageId`/`afterMessageId` anchor. `session/fork`, `resume`, and `load` accept `cursor`. Motivated by fork-at-message. | A *durable, agent-issued position in history*. It is the natural carrier for `replayFrom: {type:"cursor"}` and aligns with Dive's `(TurnID, Seq)`. |
| [Multi-client session attach, PR #533](https://github.com/agentclientprotocol/agent-client-protocol/pull/533) | Open since 2026-02-18. No champion. One approval, from denofevil. | Adds `session/attach` with `historyPolicy` (`full`, `pending_only`, `none`, `after_message`). Permissions are broadcast with first-writer-wins, plus a `permission_resolved` notification; pending permissions are re-issued to attachers. Also proposes `turn_complete` and `prompt_received` echo. Implemented as a proxy (acp-multiplex, hydra-acp). | v2's `user_message` echo and `state_update` absorb `prompt_received` and `turn_complete`. The permission re-issue and resolution semantics are still unowned. |
| [Mid-turn input (queue/steer), PR #1261](https://github.com/agentclientprotocol/agent-client-protocol/pull/1261) ([text @7b7c86c](https://github.com/agentclientprotocol/agent-client-protocol/blob/7b7c86c98d41d77e974593432cae640efbdeaf0e/docs/rfds/v2/session-inject.mdx)) and [schema PR #2043](https://github.com/agentclientprotocol/agent-client-protocol/pull/2043) | Open since May, v2-scoped. Strong adopter feedback: Raft calls it "an adoption blocker". Schema and Rust SDK PRs exist. | Adds `session/inject{mode: queue\|steer}`, which returns a `messageId`, delivers a `user_message` later, and adds `revoke_inject` and optional `replace_inject`. A `steer_in_stream` capability declares interrupt or finish. | Maps directly to Dive's `Deliver` command and to Nvoken nudges, which drain at seams. Dive should be able to declare `steer_in_stream: ["finish"]` today. |
| [Subagents, PR #1992](https://github.com/agentclientprotocol/agent-client-protocol/pull/1992) (Vadim Briliantov, core maintainer) | Open, 17 reviews | Adds `subagent_update`, which associates a child `sessionId` with a parent. Child events flow on the same connection. Child sessions are restricted (cancel only). Baseline in v2. | One path for background and delegated work to become visible. Relevant to Dive subagents and to [#1847](https://github.com/agentclientprotocol/agent-client-protocol/issues/1847). |
| [session/status, PR #986](https://github.com/agentclientprotocol/agent-client-protocol/pull/986) | Open since April | A liveness and state query | PR #2208 argues this is redundant given state-on-resume. |
| [turn_complete, PR #644](https://github.com/agentclientprotocol/agent-client-protocol/pull/644) / [#554](https://github.com/agentclientprotocol/agent-client-protocol/issues/554) | #644 **closed** 2026-07-02 ("given where we are going in v2… I don't think we'll add this"). #554 is still open. | v1 barrier | Superseded by v2 `idle`. #554 remains the v1 reality: OpenHands' 100 ms sleep, and happy's 500 ms grace timer. |
| [#1847](https://github.com/agentclientprotocol/agent-client-protocol/issues/1847), tool calls that outlive their turn | Open, 0 comments | Is `in_progress` background or blocking? Can a tool update arrive after the turn? | v2 permits background updates while idle but does not answer the tool-status question. |
| [#1860](https://github.com/agentclientprotocol/agent-client-protocol/issues/1860), usage semantics / [PR #2061](https://github.com/agentclientprotocol/agent-client-protocol/pull/2061) | Open. The PR clarifies placeholders. | Per-turn and cumulative usage conflict. Copilot reports cumulative totals; the Claude and Codex bridges report per turn. | v2 idle `usage` (unstable) inherits the ambiguity unless fixed. |
| [#1979](https://github.com/agentclientprotocol/agent-client-protocol/issues/1979), approval context / [PR #2138](https://github.com/agentclientprotocol/agent-client-protocol/pull/2138) | Open. The PR is a docs clarification for v1. | `DefaultOnError` can hide a bad `rawInput`. | The same pattern exists in v2 patch fields (§2.5). |
| [Session fork](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/rfds/session-fork.mdx) | Draft; unstable schema in both versions | `session/fork` duplicates a session; the fork-at-point idea moved to #2114. | Dive's record, derived by projection, makes fork-at-Seq cheap. |
| Session rewind, [PR #1321](https://github.com/agentclientprotocol/agent-client-protocol/pull/1321) | Open | Truncate and edit history (v2) | Would introduce the first removal-like semantics. |

### 3.2 Who drives the spec, and how

- **Governance.** ACP is "jointly governed by Zed and JetBrains … working toward transitioning to an independent foundation" ([governance.mdx](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/community/governance.mdx)).
  - **Lead maintainers (BDFL, with veto):** Ben Brandt (Zed) and Sergey Ignatov (JetBrains).
  - **Core maintainers:** Agus Zubiaga, Anna Zhdan, Niko Matsakis, Vadim Briliantov.
  - **Transports working group maintainers:** Anna Zhdan and Alex Hancock.
  - Source: [MAINTAINERS.md](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/MAINTAINERS.md).
  - Core maintainers meet biweekly. Detailed discussion happens on Zulip, and process decisions happen in PR comments.
- **RFD process** ([rfds/about.mdx](https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/rfds/about.mdx)):
  1. Open a PR with the template.
  2. It merges to **Draft** only when a core member *champions* it.
  3. It moves to **Active** when maintainers or a working group spend bandwidth on it. This is a visibility signal, not approval.
  4. It moves to **Preview** when fully implemented. That PR stays open "for a few days".
  5. It moves to **Completed**. "Final decision is always made by the core team lead."
  - Experimental implementations are feature-gated by RFD name, which shows up as the `unstable_*` Cargo features.
- **Observed practice.**
  - Ben Brandt authored the v2 tracking RFD and all eleven RFDs in `docs/rfds/v2/`. The twelfth v2-targeted RFD, the transport, is by alexhancock and jh-block. He also moves nearly every RFD between stages. In the merged PRs returned for since 2026-06-01, excluding release and dependabot bots, he authored 17 of roughly 50; no other human authored more than 2.
  - Community RFDs without a champion stay open for months: #533 since February, #986 since April, #1261 since May.
  - Adopter feedback posted in PR threads ("adoption blocker", "implemented end-to-end in hydra-acp") is visibly how community proposals gain weight.
- **Cadence.** Schema minors ship about every two to four weeks (v1 `1.18.0` on 2026-07-06 through `1.23.0` on 2026-09-18). v2 alphas ship in lockstep. The v2 tracking RFD reports no stabilization date.

**Implication.** Influence runs through (a) a core champion, in practice Ben Brandt for lifecycle and v2 and anna239 or the Transports WG for transport, and (b) concrete implementation evidence. A well-evidenced comment on an Active or just-opened RFD is the cheapest high-leverage move. A standalone new RFD without a champion is the most expensive.

---

## 4. Convergence matrix

Columns: ACP v1 (stable); ACP v2 core (draft), with PR #2208 noted where it changes the answer; Nvoken SSE ([§11](../comparison/README.md), [nvoken-cloud.md §4–5](../comparison/nvoken-cloud.md), [spec @deea133](https://github.com/deepnoodle-ai/nvoken-cloud/blob/deea133ad2d845b67e3259f63fdca7c09394379d/docs/design/streaming-protocol-specification.md)); OpenHands WS v2 ([openhands.md §8](../comparison/openhands.md), [session_protocol.py @d77ada7](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-agent-server/openhands/agent_server/session_protocol.py#L1-L21)); acpx watch ([session-watch.md @c62ae8c](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/session-watch.md)); Dive v2 proposal ([conceptual model](../design/2026-09-26-dive-v2-conceptual-model.md), [§11 table](../comparison/README.md)).

| Concern | ACP v1 | ACP v2 (+#2208) | Nvoken SSE | OpenHands WS v2 | acpx watch | Dive v2 proposal |
|---|---|---|---|---|---|---|
| **Stream scope** | One stdio connection | Connection. With #2208: one stream per connection and per session. | Per Turn (closes on settle) or per Conversation (never closes) | Per conversation socket | Local session record | Library callback; the host chooses scope |
| **Record identity** | `messageId` (optional on chunks), `toolCallId` | `messageId` required and agent-owned; `toolCallId`, `terminalId`, `planId` | `message.id` + `sequence`; `(turn_id, revision)` for changes; ToolCall UUID | `Event.id` + `seq` (log index) | Raw ACP IDs + acpx `requestId` | `(TurnID, Seq)` per Step; call IDs; attempt IDs |
| **Preview identity vs record** | Same thing: chunks *are* the record | Same entity: chunks and upserts on one ID | `message_id` reserved at the first delta = saved ID. Tool preview ID mismatch is a known bug. | `item_id` == finished `Event.id` | Same (raw messages) | Proposed: reserved message ID and a call ID minted at block start |
| **Attempt / supersession** | None | None on the wire. A reset is expressible as `content: []` on the same ID. | `attempt` = lease fencing token; a higher attempt voids previews; retry mints a new `message_id` | `attempt` on the slot; a higher attempt supersedes a lower one on the **same** `item_id` | A replacement owner marks the unfinished attempt failed | Attempt ID on `model_requested` |
| **Correct or retract shown content** | Tool fields replace; message chunks cannot be corrected | **Yes:** replace or clear any message, tool, or terminal. No removal (draft `plan_removed` only). | Previews voided by the saved message, attempt, resync, or terminal change. Saved messages are immutable. | `ItemAborted` retires a slot. Durables are immutable. | No | The record is immutable; projection is recomputed |
| **Replay cursor** | None (`load` = everything) | `replayFrom{start}`, inclusive, retained history only. #2208: opaque transport cursor per stream, exclusive (`Last-Event-ID`), in memory. | Opaque `(message_sequence, lifecycle_revision)`, scope-bound, exclusive, durable in Postgres | `after_seq` (durable log index); `SyncFrame{from_seq,through_seq}` | Opaque, exclusive; expiry raises an explicit error | Host maps `Seq` to its own cursor |
| **Live resume after disconnect** | No | Core: no (resume + full replay). #2208: yes within retention, else 409 and fall back. | Yes, exact: "repeats nothing and skips nothing" | Yes; live frames are buffered during replay and deduplicated | Yes (`watch --cursor`) | Host responsibility |
| **Durable vs lossy classes on the wire** | None | None. Draft `notice` is live-only by rule, and user messages *may* be live-only; neither is marked per update. | Explicit: `transcript.update` (durable, has an `id`) vs `message.delta`/`stream.resync`/`connection.closing` | Explicit: `Durable`, `Transient`, `ItemStarted`/`Delta`/`ItemAborted`, `Error` | Everything journaled locally | Proposed: Recorder (committed) vs Observer (preview) |
| **Loss detection** | None | None. #2208 cursor gaps are server-detected (409). | Per-block byte `offset`; `stream.resync` | `Delta.order` monotonic per `(item, attempt)` | Cursor-expired error | Offset proposed |
| **Foreground boundary** | `session/prompt` response (ordering race, #554) | `state_update` running / requires_action / idle + `stopReason` | TurnChange `terminal: true` + `status` | `ConversationStateUpdateEvent.execution_status` | `turn_started` / `turn_result` (local) | `turn_stopped{Stop}` |
| **Input↔work correlation** | JSON-RPC request = the turn | `messageId` of the inserted user message. **No turn ID;** `idle` is session-scoped. | `turn_id` on everything | Conversation plus event causality | `requestId` | `TurnID`; input is a `ContextItem` step |
| **Background work after turn** | Unspecified (#1847) | Updates allowed while idle. Tool status semantics are open; subagents are proposed (#1992). | N/A: the Turn parks `waiting` | Not modeled beyond status | N/A | `tool_waiting{Detached}` + `Accept` |
| **Approvals as waiting state** | `request_permission` RPC on the connection | Same RPC with `title`/`subject`; `requires_action` SHOULD. Not durable or re-issued (#533 proposes it). | **Host tool:** Turn `waiting` with no lease; `arguments` and `deadline_at` on the current change; result bound to the ToolCall ID | `WAITING_FOR_CONFIRMATION` status + REST `{accept, reason}` | Passes through ACP | `tool_waiting{Waiting}` + `Accept` (CommandID idempotent) |
| **Tool lifecycle** | pending, in_progress, completed, failed | + `cancelled`; open enum; no transition rules | pending, running, completed, failed, cancelled; `mode` | Action and Observation events | ACP | Succeeded, Failed, NotExecuted, Waiting, Detached, Uncertain |
| **Tool argument streaming** | No (`rawInput` replace) | No (`pending` = "input streaming", but no field) | `message.delta{kind: tool_arguments}` carrying id and name | No (text and reasoning deltas only) | ACP | Proposed via Delta |
| **Usage** | `usage_update` context/cost; unstable `PromptResponse.usage` (#1860) | `usage_update`; unstable idle `usage` | Disjoint buckets on the current terminal change | `stats` in the state update | ACP | Per model step; turn totals |
| **Slow consumer** | N/A | Unspecified. #2208 uses bounded retention and keepalive. | 10 s write deadline → `connection.closing{slow_consumer}`; preview drop → resync | Byte budget → drop the connection (1013); "disconnection is the backpressure" | Bounded pages | Observer isolation (to be specified) |
| **Multiple readers** | No | The lifecycle is designed for it, but there is no attach method (#533). #2208 cursors are per connection. | Any number, independent, and none affects execution | Yes | Same-host watchers | Host |
| **Failure reporting** | JSON-RPC error on prompt | **No failure stop reason**; only a custom `_…` value | `failed` + closed `TurnFailure` codes | Error events and status | `turn_result: failed`, `WATCH_OUTCOME_UNKNOWN` | `Stop{Failed}`, `Uncertain`, `Persistence` |
| **Forward compatibility** | `_meta`, `_` methods | Open enums, `_` prefix, preserve unknowns, `_meta` patch rules | Ignore unknowns; "MUST NOT infer ended from an unknown status"; server-computed `terminal` | The URL is the version | Local markers vs JSON-RPC | Versioned Step |

### 4.1 Where they agree

1. **The producer mints identity, and previews carry the future record's identity.** Nvoken, OpenHands, and ACP v2 all make the displayed item and the durable item the same keyed entity. This removes the "one row disappears and another appears" problem. ACP v2 goes furthest, because the preview *is* the entity.
2. **Lifecycle is separate from content.** Every protocol has a distinct state or change channel: ACP `state_update`, Nvoken `TurnChange`, OpenHands `execution_status`, acpx `turn_started`/`turn_result`, and Dive `turn_stopped`. ACP v2's move from "response = turn" to "notification = state" follows the path Nvoken and OpenHands already took.
3. **Replay, then live, is standard**, although only Nvoken, OpenHands, and acpx specify an exact cursor today. #2208 would bring ACP partway.
4. **Waiting on a human is visible state on the stream.** ACP has `requires_action`, Nvoken has `waiting` plus host-call `arguments`, OpenHands has `WAITING_FOR_CONFIRMATION`, and Dive has `tool_waiting`.
5. **Disconnect rather than block.** Nvoken, OpenHands, and #2208's bounded retention all prefer dropping a reader to stalling execution.
6. **Cancellation is confirmed by a terminal signal, not by the request.** This holds for ACP v2 `idle{cancelled}`, Nvoken's terminal change, and Dive's `turn_stopped{Canceled}`.

### 4.2 Where ACP v2 is weaker

1. **No class distinction.** Everything is a session update, so a client cannot tell a replayable fact from a disposable preview. The upsert model *mitigates* this, because a later record overwrites, but durable logs cannot store "only the facts". A Nvoken-backed ACP server has to decide for itself which updates to journal.
2. **No attempt field.** Supersession can be expressed only by reusing the ID and resetting it. The spec gives no guidance that agents should do so on a retry.
3. **No durable live-stream cursor.** The core has only `replayFrom{start}`. #2208 adds an in-memory transport cursor per connection that is not usable by a second reader, does not survive an agent restart, and falls back to a full replay.
4. **Nothing synchronizes state after replay.** The core does not require one; #2208 adds it only after transport resumption.
5. **Approvals are connection-bound RPCs.** A disconnect loses the request. There is no re-issue and no resolution notification. Binding the approval to the recorded arguments is left to implementers.
6. **Missing vocabulary:** no failure stop reason, no turn ID, no reason for `requires_action`, no tool-argument streaming, and no removal operation.
7. **Unresolved semantics:** usage, background tool status, and silent degradation of patch fields.

### 4.3 Where ACP v2 is stronger

1. **Correction by replacement.** It is simpler than resync frames and self-healing. Neither Nvoken nor OpenHands can correct a *saved* message; ACP can correct any entity.
2. **Terminal snapshot plus chunk.** An explicit replacement snapshot makes replay cheap. Nvoken has no terminal surface at all.
3. **Permission `title`/`subject` separation and the `command` subject.** This is cleaner than Nvoken's "an approval is a host tool" for *display*, although weaker for *binding*.
4. **The extensibility discipline.** The `_` prefix, reserved non-underscore values, and a mandate to preserve unknowns are stricter than Nvoken's "ignore unknown".
5. **Ecosystem reach.** A Dive agent that speaks ACP reaches Zed, JetBrains, and registry clients. Nvoken's SSE reaches Nvoken SDKs.

---

## 5. Gaps, and where ACP is heading

**Direction inferred from the RFD set.** ACP is moving from an editor RPC protocol toward a *session-stream* protocol that can run remotely, take multiple readers, and work in the background:

- notification-only lifecycle (v2 prompt);
- ID upserts;
- remote transport (Active);
- resumable streams (#2208);
- agent-issued history cursors (#2114);
- attach (#533);
- queue and steer (#1261);
- subagent sessions (#1992);
- compaction and notices (Preview).

This is the shape Nvoken already has. The durability *authority* stays at the agent's retention policy. ACP keeps saying "insertion is not durability", and #2208 rejects durable replay logs.

Gaps to watch, and ones Deep Noodle has standing to fill:

| Gap | Likely resolution path | Our position |
|---|---|---|
| Replay from a position | #2114 cursors + a `replayFrom` variant; #2208 fallback | Dive `Seq` / Nvoken position is the natural cursor |
| State sync after any resume | Prompt RFD follow-up; #2208 for transport | Require `state_update` + snapshots (config, commands, usage, pending approvals) at the end of replay |
| Durable approvals | #533 re-issue plus resolved notification; nothing for restart | Approval = waiting effect bound to the recorded call; re-issue on every attach |
| Failure reporting | Prompt RFD follow-up | Standard `failed` stop reason plus structured error |
| Turn identity | Not proposed | A `turnId` on `state_update`, even optional, would let gateways correlate usage and outcomes |
| Background tools | #1847 (no owner), #1992 | Distinct `detached` status or a subagent session |
| Usage semantics | #1860 / #2061 | Nvoken's disjoint buckets, with per-turn meaning stated explicitly |
| Tool argument previews | Not proposed | Low priority; `rawInput` snapshots suffice for display |

**Timing risk.** v2 is five alphas in with no stated stabilization date, and each alpha has changed wire shapes (message ID on prompt insertion landed in alpha.5). Major adapters still negotiate v1 (see [acp-issues.md](acp-issues.md)). Expect v1 to remain the interop target through at least early 2027. That is an inference from cadence and adoption, not a stated plan.

---

## 6. Recommended Dive event vocabulary (Go sketches) and projection rules

### 6.1 Design principles

1. **Three event classes with stated guarantees.**
   - *Committed:* exactly one event per Recorder-acknowledged Step, in `Seq` order, and replayable.
   - *Preview:* display-only. It can be dropped, it is superseded by a commit or a higher attempt, and it is never replayed.
   - *Signal:* local to one observer, for gaps and lag.

   This is the Nvoken/OpenHands split, placed in the library.
2. **Every preview names its future record.** It carries a slot that equals the future message ID, an attempt, a block index, and a byte offset. Tool-call IDs are minted by Dive at block start and preserved into `model_completed`; the provider ID is kept as secondary metadata.
3. **Committed events carry the Step, never a synthetic message.** Projections compute display entities with the same rules as `Project`.
4. **Stable derived IDs.** Any ID a projection needs, such as an ACP thought `messageId` per reasoning block or a user `messageId` per input item, is a *pure function* of Dive identity. Live projection and replay projection therefore emit identical IDs.
5. **State is derived, not emitted separately.** Running, waiting, and stopped fold from committed steps. Projections that need a state event, such as ACP `state_update`, synthesize it from the fold. That keeps one source of truth.
6. **Observation cannot block or control** (conceptual model principle 5). The library delivers to each observer through a bounded buffer. On overflow it drops *previews only* and emits `Gap`. Committed events are never dropped for an observer. A host that needs lossless committed delivery reads the Recorder, not the Observer.

### 6.2 Types

```go
package dive

// Class states what a consumer may rely on.
type Class uint8

const (
    Committed Class = iota + 1 // a Recorder-acknowledged Step; replayable by (TurnID, Seq)
    Preview                    // display-only; droppable; superseded by a commit or a higher Attempt
    Signal                     // local to one observer (gaps); never replayed, never persisted
)

type Event interface {
    Class() Class
    Head() Head
}

type Head struct {
    SessionID string    // set by session.Runner; empty for bare Engine use
    TurnID    string
    Emitted   time.Time
}

// Observer receives isolated copies. Implementations must not block. The engine
// never waits on an observer (see §6.1 principle 6).
type Observer interface{ Observe(Event) }

// ---- Committed -------------------------------------------------------------

// StepCommitted is emitted after Recorder.Commit acknowledges the step.
// Receipt is the host's opaque position (Nvoken: sequence/revision; file journal: offset).
type StepCommitted struct {
    Head
    Step    Step    // Seq, Kind, payload (turn_started, model_completed, tool_waiting, ...)
    Receipt Receipt
}

// ---- Preview ---------------------------------------------------------------

// Slot identifies one model output being written.
// OutputID is reserved when model_requested commits and becomes the ID of the
// assistant message recorded in model_completed. It is stable across retries
// of the same logical output. Attempt increments per model_requested for that
// slot, and a higher Attempt voids all previews of lower attempts.
type Slot struct {
    OutputID string
    Attempt  int
}

type BlockKind uint8

const (
    BlockText BlockKind = iota + 1
    BlockReasoning  // display-only; never becomes a public record (Nvoken R29)
    BlockToolInput  // tool-call argument JSON fragments
)

// BlockStarted opens a content block within a slot.
type BlockStarted struct {
    Head
    Slot
    Block    int       // provider content-block index within this attempt
    Kind     BlockKind
    CallID   string    // BlockToolInput: Dive-minted; equals tool.Call.ID in model_completed
    ToolName string
    After    int64     // anchor: Seq of the last committed step when the slot opened (OpenHands anchor_seq)
}

// BlockDelta appends to a block. Offset is the byte count of earlier deltas in this
// (Slot, Block). A consumer whose accumulated length differs discards the block and
// waits for the commit (Nvoken R28).
type BlockDelta struct {
    Head
    Slot
    Block    int
    Kind     BlockKind
    Offset   int64
    Text     string
    CallID   string // repeated on every tool-input fragment (Nvoken R27)
    ToolName string
}

// SlotClosed retires a slot without a model_completed for this attempt:
// a provider failure, a cancel, or a retry that follows. Mirrors OpenHands ItemAborted.
type SlotClosed struct {
    Head
    Slot
    Reason   CloseReason // Retrying, Failed, Canceled
}

// ToolProgress is displayable progress from an executing tool effect. Index is
// monotonic per (CallID, EffectID).
type ToolProgress struct {
    Head
    CallID   string
    EffectID string
    Index    int
    Content  []tool.Content // complete items (maps to tool_call_content_chunk)
}

// ToolBytes is a raw byte stream, such as a process's stdout, for terminal-like display.
type ToolBytes struct {
    Head
    CallID   string
    StreamID string  // stable per stream; the projection derives an ACP terminalId from it
    Offset   int64
    Data     []byte
}

// ---- Signal ----------------------------------------------------------------

// Gap: previews were dropped for this observer. Scope empty = every open slot.
type Gap struct {
    Head
    Slot   *Slot
    Reason string // "observer_overflow"
}
```

The payloads of `Step` should carry the few fields a projection needs so that projections never reach back into the engine:

- `tool_waiting` carries `WaitInfo{Kind: Approval|Question|External|Detached, Title, Description, Subject, Options, Deadline}`.
- `model_completed` carries the `OutputID`, the ordered content blocks, and the tool calls with Dive IDs.
- `turn_started` carries input `ContextItem`s, each with a stable item ID.

A folded helper removes the need for a separate state event:

```go
type Phase struct {
    Kind    PhaseKind   // Running, AwaitingHuman, AwaitingExternal, Stopped
    Stop    *Stop       // when Stopped
    Waiting []WaitRef   // open waits (call ID + WaitInfo) when Awaiting*
}
func (t *Turn) Phase() Phase
```

### 6.3 Projection rules

Every projection is a pure function `Project(prev ProjectorState, e Event) ([]WireMsg, ProjectorState)`. It needs only small state: open slots and accumulated lengths, which previews have been emitted per slot, and for v1, accumulated tool content.

| Dive event | ACP v2 | ACP v1 (loss noted) | Nvoken SSE (gateway) | AG-UI |
|---|---|---|---|---|
| `turn_started` committed (input items) | `user_message{messageId: itemID, content}`; `state_update{running}`. **Respond to `session/prompt` only now**: acceptance = Recorder receipt. | Nothing (the prompt request is the input). Replay: `user_message_chunk` | Saved user message + `queued`/`running` changes | `RUN_STARTED{runId: TurnID, input}` |
| `context_delivered` (Origin User) | `user_message` (nudge or inject delivery) | `user_message_chunk` (display only) | Saved message (nudge checkpoint) | `TEXT_MESSAGE_*` role user or `MESSAGES_SNAPSHOT` |
| `context_delivered` (Operator, Library, Tool) | Not shown, or `_meta` | Not shown | Reminder block | Not shown |
| `model_requested` committed | If `Attempt>1` and previews were shown: `agent_message{messageId: OutputID, content: []}` (reset) | Nothing. A retry cannot reset: **loss** (duplicated partial text) unless v1 previews are buffered until commit | Nothing (`attempt` bump voids previews client-side) | Nothing |
| `BlockStarted{Text}` | Nothing (or `agent_message{…, content: []}` to pin order) | Nothing | Nothing | `TEXT_MESSAGE_START{messageId}` |
| `BlockDelta{Text}` | `agent_message_chunk{messageId: OutputID}` | `agent_message_chunk{messageId}` | `message.delta{kind:text, attempt, message_id, content_index, offset}` | `TEXT_MESSAGE_CONTENT` |
| `BlockDelta{Reasoning}` | `agent_thought_chunk{messageId: OutputID+"/r"+block}` | `agent_thought_chunk` | `message.delta{kind:thinking}` | `REASONING_MESSAGE_CONTENT` |
| `BlockStarted{ToolInput}` | `tool_call_update{toolCallId: CallID, name, title, kind, status: pending}` | `tool_call{…, status: pending}` | Nothing | `TOOL_CALL_START{toolCallId, name, parentMessageId}` |
| `BlockDelta{ToolInput}` | Dropped (no field). Optionally a throttled `rawInput` snapshot when the prefix parses. | Dropped | `message.delta{kind:tool_arguments, tool_call_id, name}` | `TOOL_CALL_ARGS` |
| `SlotClosed{Retrying}` | Nothing (the next attempt resets) | Nothing | Nothing (attempt bump) | `TEXT_MESSAGE_END` + later `MESSAGES_SNAPSHOT` |
| `SlotClosed{Failed\|Canceled}` | `agent_message{content: partial or null}`; `_meta{"dive/slot":"aborted"}` | Nothing (**loss:** partial text remains) | Nothing | `TEXT_MESSAGE_END` |
| `model_completed` committed | `agent_message{messageId: OutputID, content: fullText}` (**authoritative replace; heals lost previews**); `agent_thought{…}` if displayable; per call `tool_call_update{status: pending, rawInput: args, title}` | If previews matched the committed text exactly, nothing. Otherwise **loss**: chunks cannot be corrected. Options: buffer previews on v1, or append a corrective message with a new ID. `tool_call_update{rawInput}` | Saved assistant message (id = OutputID; `tool_use.id` = CallID) | `TEXT_MESSAGE_END`, `TOOL_CALL_END` |
| `model_failed` committed | `agent_message{content: partial \| []}` | Nothing | Change row (per Nvoken rules) | `RUN_ERROR` only if the turn stops |
| `tool_started` committed | `tool_call_update{status: in_progress, rawInput: effectiveArgs}` | `tool_call_update{status: in_progress}` | Tool summary `running` (next change) | Nothing |
| `ToolProgress` | `tool_call_content_chunk{content}` | `tool_call_update{content: accumulated}` (stateful) | Not streamed today | `ACTIVITY_DELTA` or `CUSTOM` |
| `ToolBytes` | Once: `tool_call_update{content:[{type: terminal, terminalId: derive(StreamID)}]}` + `terminal_update{command, cwd}`; then `terminal_output_chunk{data: b64}` | **Loss:** v1 terminals run on the client. Degrade to text content chunks (sanitized). | Not streamed | `CUSTOM` |
| `tool_waiting{Approval}` committed | `session/request_permission{title, description, subject: {tool_call: {toolCallId, rawInput: recorded args}}, options}` + `state_update{requires_action}`. **Re-issue on every (re)attach while still waiting.** | `session/request_permission{toolCall: {toolCallId, rawInput}, options}` (no title field; put copy in `toolCall.title`, which mutates the displayed title, a v1 limitation) | Host tool `waiting` + `arguments` on the current change | `RUN_FINISHED{outcome: {type: interrupt, interrupts}}` |
| Permission answered (client) | → `Accept{CallID, Outcome, CommandID: hash(sessionID, callID, rpcID)}`. An unknown outcome = deny. First writer wins; later answers are acknowledged no-ops. | Same | `POST tool-results` | New run with `resume` |
| `tool_waiting{External\|Detached}` | `tool_call_update{status: in_progress, _meta{"dive/wait": kind}}`. State stays `running` while the Turn blocks. For `Detached`, see §8 Q3. | Same | Host or callback call `running` | `RUN_FINISHED{interrupt}` (external) |
| `tool_completed{Succeeded}` | `tool_call_update{status: completed, content, rawOutput}` | Same | Tool result message + change | `TOOL_CALL_RESULT` |
| `tool_completed{Failed}` | `status: failed` | Same | `is_error: true`, `failed` | `TOOL_CALL_RESULT` (error) |
| `tool_completed{NotExecuted}` (denied or unstarted on cancel) | `status: cancelled` if it followed a cancel, else `status: failed` + content "Not executed: <decision>" + `_meta{"dive/state":"not_executed"}` | `failed` + content | `cancelled` or `failed` | `TOOL_CALL_RESULT` |
| `tool_uncertain` | `status: failed` + content "Outcome unknown" + `_meta{"dive/state":"uncertain"}` (a custom `_dive_uncertain` status renders generically in unaware clients) | `failed` + content | Unknown-outcome tool result | `TOOL_CALL_RESULT` (error) |
| `turn_stopped{Completed}` | `state_update{idle, stopReason: end_turn}` (+ unstable `usage`, per-turn totals) | Prompt response `end_turn` | Terminal change `completed` | `RUN_FINISHED{outcome: success}` |
| `turn_stopped{Canceled}` | `idle{cancelled}` | `cancelled` | `cancelled` | `RUN_FINISHED` |
| `turn_stopped{Limited: model_calls}` / `{output}` | `idle{max_turn_requests}` / `idle{max_tokens}` | Same | `incomplete` + `stop_reason` | `RUN_FINISHED` |
| `turn_stopped{Refused}` | `idle{refusal}` | `refusal` | Per Nvoken | `RUN_FINISHED` |
| `turn_stopped{Failed}` | `idle{stopReason: "_dive_failed"}` + `agent_message` or draft `notice` explaining it (**gap:** no standard reason) | **Loss:** JSON-RPC error on the prompt, or `end_turn` + message | `failed` + `TurnFailure` | `RUN_ERROR` |
| `turn_stopped{Uncertain}` | `idle{"_dive_uncertain"}` + message | Error | `failed` (unknown outcome) | `RUN_ERROR` |
| `turn_stopped{Waiting}` (human) | Already `requires_action`; no idle | Prompt stays pending (the v1 turn is the RPC) | `waiting` | `RUN_FINISHED{interrupt}` |
| Usage (from `model_completed`) | `usage_update{used: inputTokensOfLastCall, size: contextWindow, cost}` | Same | Terminal-change `usage` | `CUSTOM` |
| `Gap` | Nothing. The next commit replaces content; optionally `agent_message{content: []}` for affected slots. | Nothing (**loss** until commit) | `stream.resync{turn_id}` | `MESSAGES_SNAPSHOT` at commit |

**Losslessness claim to test.** For every event sequence $E$ and every prefix ending at a committed event:

$$\mathrm{ACPv2.Fold}(\mathrm{ProjectV2}(E)) \equiv \mathrm{Render}(\mathrm{Dive.Fold}(E))$$

This must hold under any subset of dropped previews. It holds because every committed entity is re-emitted as a whole-entity upsert.

For ACP v1, equality holds only when no preview was lost and no attempt was retried. The v1 adapter should therefore offer `PreviewPolicy{Stream|BufferUntilCommit}` and default to `Stream` for Zed responsiveness. Build the equivalence tests as table fixtures shared with a Dive reference reducer, the same approach as Nvoken's `sdk/conformance/fixtures/reducer.json`.

**Replay rules.**

- *ACP v2 `resume{start}`:* project *only committed events* from the Recorder. Each message is a whole `agent_message`, with no chunks, so the `content: []` reset rule is moot. Then emit the current snapshots:
  - `state_update` from `Turn.Phase()`;
  - `config_option_update`;
  - `available_commands_update`;
  - `usage_update`;
  - for each open approval wait, re-issue `request_permission`.

  This goes beyond the core spec, which requires none of it, but is compatible with it.
- *Nvoken:* unchanged. The gateway maps `Receipt` to `(message_sequence, lifecycle_revision)`.
- *#2208, if adopted:* a Dive-backed server can satisfy the transport cursor for committed messages from the Recorder, and for previews from a small ring. If the ring has evicted previews, it can serve **state-equivalent** replay instead (see §7), provided the spec allows it.

**Identity rules.**

| Dive identity | Projected ACP ID | Notes |
|---|---|---|
| `OutputID` | `agent_message.messageId` | Stable across attempts, which makes the reset-by-ID mapping lossless |
| `OutputID/r{n}` | `agent_thought.messageId` | One thought message per reasoning block |
| Input `ContextItem.ID` | `user_message.messageId` and `session/prompt` result | Must be stable in the record so replay reuses it (ACP MUST) |
| `tool.Call.ID` | `toolCallId` | Minted at block start; the provider ID goes in `_meta` |
| `(CallID, StreamID)` | `terminalId` | Derived deterministically |
| `TurnID` | `_meta{"dive/turnId"}` on `state_update` | ACP has no turn ID; `_meta` keeps it available to aware clients |

### 6.4 Package placement

- `dive` owns `Event`, `Observer`, `Slot`, `Phase`, and a **reference reducer** (`dive/fold`) with fixtures.
- `acp` (a separate module, or `x/acp`) owns `v1` and `v2` projectors plus the agent server. It depends on `coder/acp-go-sdk` for v1 framing and hand-written v2 types until a Go v2 schema exists. The core `dive` package never imports ACP types. This carries forward the earlier recommendation in [acp-dive-nvoken.md](acp-dive-nvoken.md).
- The Nvoken gateway replaces its `divegen` delta emitter with a `dive.Observer`. That deletes the provider-ID and message-ID reservation code that exists today.
- `agui` is optional and last. AG-UI's `RUN_FINISHED{outcome: interrupt}` maps cleanly to `tool_waiting` ([AG-UI events @b8ebd02](https://github.com/ag-ui-protocol/ag-ui/blob/b8ebd02c84a3a2757990da47aebf7c55b708b1ef/docs/concepts/events.mdx)).

### 6.5 Build now vs. wait

| Now | Next (after an ACP v1 adapter proves the vocabulary in Zed) | Wait |
|---|---|---|
| The §6.2 vocabulary in Dive v2, and `Slot` stable across attempts | ACP v2 wire behind a flag, tested against the TS SDK `experimental/v2` | v2 as the default (wait for the alpha to end) |
| Reference fold + fixtures (Dive ↔ Nvoken ↔ ACP v2 equivalence) | Replay-with-snapshots on `resume` | Remote ACP transport (the RFD is Active; #2208 is unsettled) |
| ACP v1 agent adapter (Zed; `PreviewPolicy`) | `session/inject`, only if the RFD reaches Draft with a champion | Session cursors (#2114), multi-client attach (#533), subagent sessions (#1992) |
| Nvoken gateway consumes `dive.Observer` | | |

---

## 7. Spec-engagement recommendation

**Yes: engage narrowly, through comments backed by implementation evidence, not new RFDs.** Deep Noodle has something ACP lacks: a production streaming protocol with numbered MUST rules, a server algorithm, a client fold, and cross-SDK reducer fixtures, plus the Dive v2 record model. PR #2208 is days old, has no comments, and is the declared home for durability ("this is the RFD that does so"). Early comments shape an RFD more cheaply than late ones.

**On PR #2208**, one consolidated comment from Curtis, in priority order:

1. **Allow state-equivalent replay.** Say explicitly that a server MAY replace missed chunk updates with whole-entity upserts: `agent_message`, `tool_call_update` with `content`, and a `terminal_update` output snapshot. v2's own replacement semantics make this state-equivalent. It lets log-backed servers resume without buffering previews, and lets resumption survive an agent restart, which the RFD currently rejects as scope creep. Cite Nvoken's split between `transcript.update` and `message.delta`, and OpenHands' rule that deltas are "never replayed".
2. **Scope cursors to the session stream, not the connection.** A per-connection cursor cannot serve a second reader, which PR #533 needs, or a reconnect under a new `Acp-Connection-Id` after the connection was garbage-collected. Nvoken's cursor is valid on any connection for its scope.
3. **Unify with `replayFrom`.** Define a `replayFrom: {type: "cursor", cursor}` variant, with the transport cursor or a #2114 session cursor as the payload. Then the fallback after a refusal is "replay from a position", not "replay everything".
4. **Move state-on-resume into core v2.** After *any* `session/resume` (with or without replay) and after transport resume, require `state_update` plus current snapshots of config, commands, usage, and info.
5. **Re-issue pending permission requests after resume or attach.** Recommend `permission_resolved`, borrowing from #533, so approvals survive reconnects.
6. **Make the lossy class explicit.** Allow `_meta["acp/lossy"]`, or document per variant which updates a server MAY omit from replay: chunks when a whole update follows, and `notice`.

**Also:**

- **#1860.** Add Nvoken's disjoint input buckets. State that idle `usage` is per foreground work and that `usage_update.cost` is cumulative.
- **#1847.** Propose a `detached` tool status, or a subagent session, informed by Dive's `Waiting`/`Detached` distinction.
- **Failure stop reason.** Raise it on the prompt RFD follow-ups list.
- **Venue.** Post the same points in the Transports WG Zulip channel. Tag anna239 (the transport champion) and benbrandt.
- **Go SDK.** Consider contributing v2 types to `coder/acp-go-sdk`. It trails even v1, and a maintained Go v2 surface would give Deep Noodle standing in the Go ecosystem at low cost.

**Do not** write a competing durability RFD, and do not push Nvoken's wire format. The goal is for ACP to allow Nvoken-style servers to be conformant, not to become Nvoken.

---

## 8. Open questions

1. **Retry identity.** Should Dive keep `OutputID` stable across model retries (the OpenHands model, which makes ACP reset-by-ID lossless), or mint a new ID per attempt (Nvoken today)? This doc recommends stable. Changing Nvoken means its clients must tolerate a replaced preview under the same `message_id`.
2. **Detached effects and prompts while waiting.** When a Dive turn waits on an external executor (not a human), should ACP show `running` indefinitely, or `idle` with a custom stop reason so the client can prompt? Dive's `Deliver` is valid while waiting, so prompting is possible. The ACP semantics are undefined.
3. **Background work.** If a `Detached` tool settles after `turn_stopped`, emitting `tool_call_update` while idle is legal in v2. Does the late `Accept` start a new foreground run (`running` → `idle`) without a user prompt? v2 allows agent-initiated work, but no client has been tested against it.
4. **v1 preview policy.** Should the v1 adapter stream previews (responsive, but with occasional duplicated or uncorrectable text on retry) or buffer until commit (correct, but laggy)? Test in Zed.
5. **Strict decoding.** Should a Dive ACP *client* reject a malformed known patch field, which deviates from the reference Rust `DefaultOnError`, and is that interoperable in practice?
6. **Replay reset.** When a v2 client resumes with `start` into an existing view, should Dive's server send `content: []` resets for entities it knows the client may hold? Is a "reset view" signal worth proposing?
7. **Observer drop policy.** Is dropping previews per observer with `Gap` enough, or should the engine offer a lossless committed-only observer that blocks with a deadline (Nvoken's slow-consumer disconnect)?
8. **Where the ACP server lives.** Does it belong in Dive (a `session.Runner` plus projector) or in Nvoken (the gateway projects Nvoken Turns)? Both are possible from one vocabulary. The first serves local editors, and the second serves remote ACP once the transport stabilizes.
