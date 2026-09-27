# ACP fit for Dive and Nvoken

_Research snapshot: 2026-09-26. Local source: Dive `e9d281049a3d51a2594ee258eeb8599517299e42`; Nvoken Cloud `deea133ad2d845b67e3259f63fdca7c09394379d`. This is an integration assessment, not an implementation plan or an ACP conformance claim._

## What each system owns

| System | Current contract | Relevant source |
| --- | --- | --- |
| Dive | Runs an agent turn with `Agent.CreateResponse`; emits live `ResponseItem`s through `WithEventCallback`; returns a `Response` with a terminal `Turn`. A `TurnStore` can persist an open turn, checkpoint revisions, and load it for continuation. | [response.go](../../response.go), [outcome.go](../../outcome.go), [dive.go](../../dive.go), [agent.go](../../agent.go) |
| Dive permissions | `PreToolUse` hooks and the `permission.Manager` decide tool execution. `AskUserTool` can suspend a turn for an external answer. | [permissions guide](../guides/permissions.md), [AskUserTool](../../toolkit/ask_user.go), [suspend/resume guide](../guides/suspend-resume.md) |
| Dive A2A precedent | A separate adapter owns protocol IDs, sessions, cancellation, content conversion, and the translation from `ResponseItem` to wire updates. Its callback uses a bounded channel and cancels if its consumer disconnects. | [A2A executor](../../a2a/executor.go) |
| Nvoken Cloud | Runs remotely managed Turns and exposes an authoritative transcript plus SSE streams. `transcript.update` is durable and cursor-bearing; `message.delta` is a live preview; `stream.resync` invalidates previews. Clients fold saved messages by sequence and Turn changes by `(turn_id, revision)`. | [stream protocol specification](https://github.com/deepnoodle-ai/nvoken-cloud/blob/deea133ad2d845b67e3259f63fdca7c09394379d/docs/design/streaming-protocol-specification.md), [source locator](https://github.com/deepnoodle-ai/nvoken-cloud/blob/deea133ad2d845b67e3259f63fdca7c09394379d/docs/guides/streaming-protocol.md) |

## Dive as an ACP agent

The natural first integration is an **agent-side adapter** in a separate `acp` package or command. It would let an ACP client create a session, prompt a Dive agent, receive updates, approve tool calls, and cancel work. The existing A2A adapter demonstrates this packaging boundary, but its coarse `Calling tool` text is insufficient for ACP's structured tool-call UI.

| Dive fact | ACP projection to investigate | Required translation |
| --- | --- | --- |
| `Session.ID()` and `TurnStore.Load`/`CheckpointTurn` | ACP session ID, new/load/resume capabilities | The adapter must own an ACP-session-ID to Dive-session mapping and reopen state after process restart. Advertise only operations the configured store really supports. |
| `ResponseItemTypeModelEvent`, `Message` | Agent message and thought chunks | Normalize provider-specific deltas and avoid duplicating text when a complete `Message` follows streamed deltas. Preserve content type or declare it unsupported. |
| `ToolCall`, `ToolCallResult`, `ToolStream`, `ToolProgress` | Tool-call start/update/end and content | Keep stable Dive tool-call IDs; map rich result content and errors, not just human-readable status text. Define progress throttling and ordering. |
| `ResponseItemTypeTurnEnded`, `Turn.Status`, `Turn.Outcome` | Prompt completion/stop reason, or session state update depending on ACP version | Preserve completed, suspended, incomplete, canceled, and uncertain tool outcomes instead of reporting every return as success. |
| `permission.Manager`, `PreToolUse` | ACP `session/request_permission` | Ask on the final effective tool input, propagate the client's decision into the same tool attempt, and fail closed if approval cannot be obtained. A missing `Dialog` currently makes Dive `AskRule` auto-allow, so adapter construction must guard this explicitly. |
| `AskUserTool` in async mode | ACP elicitation, if the client advertises it | Treat an ordinary question as separate from tool authorization. Map unsupported elicitation to an explicit suspended/unsupported outcome, not permission approval. |
| `context` cancellation | `session/cancel` | Cancel the in-flight `CreateResponse`, then wait for Dive's terminal turn and persistence outcome before treating the session as ready for the next prompt. |

This mapping is conceptual until the chosen ACP wire version and SDK are fixed. In published ACP v1, [`session/load` replays the conversation as `session/update` notifications](https://agentclientprotocol.com/protocol/v1/session-setup), while `session/resume` restores context *without* replay when the agent advertises that capability. Neither operation is a cursor into a durable live event log. Dive's current session persists message/turn state; it does not persist every `ResponseItem` with an external replay cursor. A load implementation must reconstruct honest history from saved turns, and must not claim exact reproduction of live deltas or transient progress. Dive's [v2 conceptual model](../design/2026-09-26-dive-v2-conceptual-model.md) and [recommendations](../design/2026-09-26-dive-v2-recommendations.md) propose an acknowledged execution record that would make this separation cleaner, but these are provisional designs, not current behavior.

## Dive as an ACP client

Consuming ACP would mean spawning or connecting to external agents, forwarding prompts, handling their permission and filesystem requests, and translating their updates back into Dive or a host application's event model. This is a different product surface from exposing a Dive agent. A generic Dive `Tool` wrapper can support a bounded subtask, but a persistent interactive ACP session needs a client connection manager, session identity, cancellation, lifecycle handling, and host policy. Those concerns belong in an opt-in integration package or in Nvoken/Mobius orchestration rather than in `Agent.CreateResponse` itself.

For a tool-shaped integration, the tool result should summarize a completed ACP turn and retain a reference to the external session. Live updates could be forwarded as `ToolStream`/`ToolProgress`. Permission requests from the external agent must be resolved by the host's policy/UI, not by the model that called the tool. A disconnected or timed-out external agent cannot be silently retried if its side effects may have run; Dive already represents uncertain calls in `TurnOutcome` as `ToolCallStateUnknown`.

## ACP versus Nvoken's stream

[ACP v1's JSON-RPC session/update vocabulary](https://agentclientprotocol.com/protocol/v1/overview) is a practical interoperability surface for an agent and an interactive client. Its published stdio transport is a client-launched subprocess; [Streamable HTTP remains a draft](https://agentclientprotocol.com/protocol/v1/transports). Nvoken's published stream contract answers a separate durability question: which saved facts can be replayed after disconnect, in what order, from which exact position. Its `transcript.update` cursor is scoped to a Turn or Conversation; only saved frames carry that cursor. `message.delta` is explicitly ephemeral and `stream.resync` invalidates the preview. [Nvoken requirements R1, R3, R10, R35-R40, R47-R51](https://github.com/deepnoodle-ai/nvoken-cloud/blob/deea133ad2d845b67e3259f63fdca7c09394379d/docs/design/streaming-protocol-specification.md) specify those properties.

| ACP v1 update or operation | Closest Nvoken stream/read concept | Difference an adapter must handle |
| --- | --- | --- |
| `user_message_chunk`, `agent_message_chunk`, `agent_thought_chunk` | Saved `ConversationMessage` rows in `transcript.update`; live `message.delta` for text/thinking previews | Nvoken messages are durable, ordered by `sequence`; thinking previews are display-only and never saved. ACP chunks do not carry a Nvoken resume cursor. |
| `tool_call`, `tool_call_update` | `TurnChange.tool_calls` plus `tool_use`/`tool_result` content blocks; live `tool_arguments` previews | Nvoken records tool status changes as lifecycle revisions. ACP tool updates serve an interactive UI and need stable `toolCallId`, display content, and a reducer. |
| `plan`, `available_commands_update`, `current_mode_update`, `config_option_update`, `session_info_update` | No one-to-one `StreamEvent` frame | These need explicit projection from agent configuration or application state if a client requires them; they should not be invented from transcript deltas. |
| `usage_update` and prompt `stopReason` | Current terminal `TurnChange.usage`/`stop_reason` and authoritative Turn read | Nvoken's summed usage is on the current terminal change for machine credentials. ACP v1 usage interpretation varies across agents; do not use it as the accounting authority for Nvoken. [Usage issue #1860](https://github.com/agentclientprotocol/agent-client-protocol/issues/1860). |
| `session/load` or `session/resume` | Snapshot plus cursor-bearing Turn/Conversation SSE | ACP v1 load replays retained conversation; resume skips replay. Nvoken can resume exactly after a durable position. Neither ACP operation supplies Nvoken's stream cursor contract. |

This comparison uses the [stable ACP v1 update schema](acp-spec-sdk.md#the-v1-update-vocabulary) and [Nvoken's stream frame specification](https://github.com/deepnoodle-ai/nvoken-cloud/blob/deea133ad2d845b67e3259f63fdca7c09394379d/docs/design/streaming-protocol-specification.md). It compares the public stream, not every Nvoken application or REST operation.

An ACP gateway in front of Nvoken should read the authoritative snapshot and cursor stream, then project the current state into ACP updates. It must keep the Nvoken cursor internally for reconnect and de-duplicate by saved identity. A client that only sees ACP updates should not be promised Nvoken's cursor semantics unless a separate, explicit extension carries them. Conversely, ACP's editor-facing tool and permission shapes are useful views over Nvoken Turns; they do not replace Nvoken's execution record, lease, accounting, or replay authority.

## Decision to take after the ecosystem pass

1. **Expose Dive first if the goal is editor interoperability.** Scope a small agent-side adapter around one configured Dive agent and a real session store. Test it against Zed and at least one other client, covering prompt, tool UI, approval, cancel, process restart, and load. This tests the actual consumer contract before adding more abstraction to Dive.
2. **Consume ACP separately if the goal is orchestration.** Prototype a client against a Claude or Codex ACP adapter and test session persistence, permission delegation, cancellation, and process failure. Keep Nvoken's durable Turn/cursor model as the authority when integrating it into a control plane.
3. **Use the Go SDK as a transport base, with compatibility checks.** At the inspected snapshot, `coder/acp-go-sdk` passed its own tests but its June `v0.13.5` schema pin trails the September stable v1 schema, and several now-stable methods remain on its `Unstable*` surface. Keep it behind a Dive-owned adapter, run fixtures against the current v1 schema and a real client, and shim or refresh only the missing types. Do not make the core `dive` package depend on ACP types. [SDK assessment](acp-spec-sdk.md#sdk-assessment)

## Evidence limits

The local code and the Nvoken source contract establish the mapping opportunities and gaps above. They do not prove that Dive currently speaks ACP, that every Dive event can be represented losslessly, or that an ACP client will retain updates across a connection break. Those claims require a concrete adapter and interoperability tests.
