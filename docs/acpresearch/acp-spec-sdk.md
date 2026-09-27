# ACP protocol, SDKs, and agent adapter

_Research snapshot: 2026-09-26 (America/New_York). This is source-level research, not an ACP implementation or interoperability test. Links pin the upstream commits inspected._

## Version boundary

**ACP wire protocol v1 is current stable.** The spec says this explicitly; the newest inspected v1 schema artifact release was `schema-v1.23.0` (2026-09-18). The v2 schema artifact was `schema-v2.0.0-alpha.5`, and both official SDKs call v2 draft/experimental. Schema package versions are distinct from the integer `protocolVersion` negotiated in `initialize`. For a Dive adapter targeting clients today, implement v1 first and keep version-specific translation behind a boundary. V2 is important to watch because it changes what ends a prompt turn and how updates are applied. [Spec version policy][spec-readme] · [Spec releases][spec-releases] · [TypeScript v2 warning][ts-readme] · [Rust v2 feature gate][rust-readme]

The spec repository has both `docs/protocol/v2/` and `docs/protocol/v2/draft/` plus `schema/v2/schema.json`. The migration guide calls the latter a stable **v2 baseline within the draft**, then says to gate all v2 support behind explicit negotiation and feature flags until v2 stabilizes. A path named `v2` or a non-prerelease Rust crate does not make ACP wire v2 stable. [V2 migration][v2-migration]

## V1 wire contract

ACP is bidirectional JSON-RPC 2.0. Methods return results or errors; notifications have no response. In the standard stdio transport, a client starts an agent subprocess, sends newline-delimited UTF-8 JSON on stdin, and receives the same on stdout. Stdout cannot contain logs; stderr may. Streamable HTTP remains a draft proposal in the spec; custom transports are allowed if they preserve JSON-RPC and lifecycle rules. The official Rust and TypeScript SDKs have HTTP/WebSocket work, but that does not by itself establish cross-client interoperability for remote sessions. [Overview][v1-overview] · [Transports][v1-transports] · [Rust transports][rust-readme] · [TypeScript package exports][ts-package]

| Direction | V1 methods and notifications | Consequence for an agent adapter |
| --- | --- | --- |
| Client → agent | `initialize`, optional `authenticate`; `session/new`, `session/prompt`; optional `session/load`, `session/list`, `session/resume`, `session/close`, `session/delete`, `session/set_mode`, `session/set_config_option`; `session/cancel` notification | Expose only optional operations actually backed by Dive state and persistence. |
| Agent → client | `session/request_permission`; `session/update` notification; optional client `fs/read_text_file`, `fs/write_text_file`, `terminal/*`, `elicitation/create` | Implement permission handling as a call back to the client. Use client filesystem/terminal only after checking advertised capabilities. |

The baseline for an agent's v1 session support is `session/new`, `session/prompt`, `session/cancel`, and `session/update`. `session/load` is gated by top-level `loadSession`; `list`, `resume`, `close`, `delete`, and additional directories have separate session capability markers. The client's baseline includes answering `session/request_permission`; file, terminal, and elicitation services are conditional. [V1 overview][v1-overview] · [Initialization][v1-init] · [Session setup][v1-setup]

`initialize` sends the latest integer protocol version the client supports and client capabilities. The agent returns the chosen version, agent capabilities, implementation info, and authentication methods. If an agent cannot support the requested version it returns its latest supported version; the client then decides whether to continue. Omitted capabilities mean unsupported. V1 client capability examples: `fs.readTextFile`, `fs.writeTextFile`, `terminal`, `auth.terminal`, `elicitation.form/url`, and boolean session config options. Agent examples: prompt image/audio/embedded context, MCP HTTP/SSE, load, session operations, and logout. Text and resource links are baseline prompt blocks; image, audio, and embedded resources need advertised support. [Initialization][v1-init] · [Content][v1-content]

`session/new` takes an absolute `cwd` and MCP server descriptions and returns an opaque `sessionId`. `session/load` reconnects and **replays** history as `session/update` notifications before returning; `session/resume` reconnects **without replay**. The agent can send updates outside an active turn, including during load. These operations do not imply a durable event log or reconnect cursor; an implementation has to supply its own persistence and replay policy. [Session setup][v1-setup] · [Prompt turn][v1-prompt]

For v1 `session/prompt`, the request stays pending through model and tool work. The agent streams `session/update` notifications, may call `session/request_permission`, and finally returns a `stopReason` (`end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, or `cancelled`). `session/cancel` is a client notification, not a reply; the agent should stop work, flush final updates, then answer the outstanding prompt with `cancelled`. Pending permission requests must be answered `cancelled` by the client. [Prompt turn][v1-prompt]

### The v1 update vocabulary

`params.update.sessionUpdate` discriminates the variants. The current stable v1 schema has: `user_message_chunk`, `agent_message_chunk`, `agent_thought_chunk`, `tool_call`, `tool_call_update`, `plan`, `available_commands_update`, `current_mode_update`, `config_option_update`, `session_info_update`, and `usage_update`. Chunks can carry `messageId`; tool calls use a stable `toolCallId`. `tool_call` establishes a call; later `tool_call_update` patches fields such as status, content, raw input/output, and locations. Tool status includes `pending`, `in_progress`, `completed`, and `failed`. Tool content may be an ordinary content block, a file diff, or a client terminal reference. The schema and docs specify absolute file paths and 1-based line numbers. [Prompt turn][v1-prompt] · [Tool calls][v1-tools] · [Schema][v1-schema]

Permission is a separate agent → client **request**, carrying `sessionId`, a `toolCall` description (which can reference the existing `toolCallId`), and options with `optionId`, display name, and kind (`allow_once`, `allow_always`, `reject_once`, `reject_always`). The reply is `selected` with an option ID or `cancelled`. ACP does not decide the policy: the client can choose automatically from its own settings. An adapter must not infer that a displayed permission card itself authorizes tool execution. [Tool calls: permissions][v1-tools]

MCP is adjacent to ACP, not a replacement for it. The ACP client supplies MCP server definitions during session setup; the agent connects to those servers and advertises the MCP transport types it accepts. This is separate from ACP's own client↔agent JSON-RPC transport and from ACP's optional client filesystem/terminal requests. In v1 the agent capabilities include `mcpCapabilities.http` and legacy `sse`; the docs mark the latter deprecated by MCP. [Session setup][v1-setup] · [Initialization][v1-init]

### Draft v2 delta that affects design

V2 changes `session/prompt` to acknowledge acceptance and return `messageId`; a later `state_update` reports `running`, `requires_action`, or `idle` with the stop reason. Message, tool, and plan updates become ID-based upserts: omitted = unchanged, `null` = clear, value = replace, and chunk = append. `tool_call` is removed in favor of the first `tool_call_update`; plan becomes `plan_update`. `session/load` becomes `session/resume` with replay control. V2 removes client filesystem, terminal execution, and session modes in favor of client-provided MCP servers/config options, while agent-owned terminal updates remain a display surface. Capabilities become object support markers under `capabilities.session`; list/resume/close become baseline for a session-capable agent. This is a materially different reducer and lifecycle, so sharing one unversioned ACP update model between v1 and v2 would be risky. [V2 migration][v2-migration]

## SDK assessment

| SDK | Inspected state | What it offers |
| --- | --- | --- |
| Official TypeScript `@agentclientprotocol/sdk` | `1.5.0` package; default entry point is stable v1; v2 explicitly imports from `experimental/v2` | Agent/client registration, connections, generated schema/types, and experimental remote transport work. Useful reference for wire shape and negotiated version handling. [README][ts-readme] · [package][ts-package] |
| Official Rust `agent-client-protocol` | `2.2.0` crate version, with the protocol schema crate pinned to `1.9.1`; default builders are stable v1, `.v2()` requires the `unstable_protocol_v2` feature | Agent/client/proxy/conductor abstractions, stdio and HTTP/WebSocket transport crates, MCP integration. Crate `2.2.0` is **not** ACP wire v2. [README][rust-readme] · [Cargo][rust-cargo] |
| Community Go `coder/acp-go-sdk` | Latest inspected release `v0.13.5`, published 2026-06-02; source pins schema artifact `0.13.5` / wire v1 | A typed agent/client interface, generated JSON unions and dispatch, line-delimited JSON-RPC connection, examples, extension methods, and tests. It has not tracked the September spec artifacts. [README][go-readme] · [version][go-version] · [schema metadata][go-meta] · [releases][go-releases] |

The Go SDK's core is credible: generated files come from its checked-in stable and unstable schemas, it has JSON golden round trips, cancellation and notification ordering tests, and the local `go test ./...` passed on 2026-09-26. Its connection serializes incoming notifications while requests can run concurrently, and bounds the notification queue at 1,024. It uses a 10 MiB maximum scanner line, which is a real limit for large inline ACP payloads. [Generator][go-generator] · [connection][go-connection] · [golden tests][go-golden] · [notification tests][go-notify-tests]

The maintenance gap is concrete rather than just a quiet commit graph. The Go SDK's checked-in **stable** schema has 129 `$defs`; current upstream v1 has 170. For example, Go generated `UsageUpdate`, elicitation, and session deletion from its *unstable* overlay and still labels these types/methods `Unstable*`, whereas the September upstream v1 schema/docs include them in stable v1. It has no v2 wire API. Its `Agent` interface also requires handlers for optional v1 operations (`ListSessions`, `ResumeSession`, `CloseSession`, mode/config changes); examples return method-not-found from those stubs. This is an API ergonomics issue, not proof that a peer must support the operations: capability advertising remains authoritative. [Go schema][go-schema] · [Go unstable schema][go-unstable-schema] · [Go generated types][go-types] · [Go interface][go-types] · [Current v1 schema][v1-schema]

**Recommendation for a v1 Dive agent:** depend on the Go SDK behind a narrow Dive-owned adapter for JSON-RPC, framing, and baseline types, then run compatibility fixtures against the current v1 schema and at least one real client. Do not make the SDK's generated types Dive's public domain model. For features that have moved from Go's `Unstable*` surface to stable v1, either keep a tightly scoped compatibility shim or refresh/fork the schema generator; avoid claiming conformance to the September schema solely because `go test` passes on the June pin. Reassess this choice before implementing v2. The SDK is sufficient to prototype a v1 agent, but currently insufficient as an unqualified source of the latest protocol types. [Go README][go-readme] · [Go generator][go-generator] · [Spec version policy][spec-readme]

## What the Claude adapter demonstrates

`claude-agent-acp` is an agent-side adapter, not a native change to Claude's model/tool protocol. Its `initialize` responds with `protocolVersion: 1`, advertises session capabilities (including list/load/resume/close/delete in its implementation), prompt content and MCP support, plus extension metadata. A persistent Claude SDK query consumes turns; `prompt()` queues a turn and waits for a deferred result. Claude message chunks become ACP message/thought chunks, model tool uses become `tool_call` / `tool_call_update`, and SDK usage becomes `usage_update`. [Initialize and capabilities][claude-agent] · [Prompt queue][claude-agent] · [Event mappings][claude-agent] · [Tool mappings][claude-tools]

The hard parts are sequencing and semantics. The adapter emits a pending `tool_call` before asking ACP permission so the client already knows the referenced call; a later streamed tool-use block refines that same ID. It translates selected ACP permission options back to Claude SDK allow/deny effects. It also normalizes ACP prompts: text and links become text, embedded text becomes tagged context, images become Claude image blocks, while unsupported blob/audio inputs are ignored. These are adapter-specific choices, not requirements of ACP. [Permission path][claude-agent] · [Prompt conversion][claude-agent] · [Tool notification helper][claude-agent]

For Dive, the closest source boundary is `CreateResponse(..., WithEventCallback(...))`: `ResponseItem` already carries model events, complete messages, tool calls/results, tool stream/progress, usage, and terminal turn outcome. A v1 adapter could map those into ACP chunks/tool updates and return the prompt response only after Dive's turn ends. It must serialize concurrently invoked Dive callbacks, maintain stable ACP message/tool IDs, and decide how Dive's suspended/incomplete turns map to ACP's much smaller stop-reason set. Permission should bridge through Dive's `PreToolUse`/`Dialog` path and block the actual tool attempt until the ACP client replies. Exact replay needs explicit reconstruction from stored turns; Dive's callback stream is not itself a durable ACP update log. [Dive response items][dive-response] · [Dive callbacks][dive-api] · [Dive permissions][dive-permission] · [Dive session store][dive-session]

## Source index

The upstream commit snapshots inspected were: spec `128845f5bd4c7fca5f23374e9b4853e470ecf99a`, TypeScript `4356253eb95ad02d9278af93e9ae6a28cf1d7a67`, Rust `52d92831d9c0ddc621afdfe18a44ea9de006b43e`, Go `0845a3bb9eddda5bfc22a94dd3598c90cb842451`, and Claude adapter `e6681d2a5734857727352474c8c9aa848f9210ee`. Local Dive source was `e9d281049a3d51a2594ee258eeb8599517299e42`.

[spec-readme]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/README.md#L13-L29
[spec-releases]: https://github.com/agentclientprotocol/agent-client-protocol/releases
[v1-overview]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/overview.mdx
[v1-init]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/initialization.mdx
[v1-setup]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/session-setup.mdx
[v1-prompt]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/prompt-turn.mdx
[v1-tools]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/tool-calls.mdx
[v1-transports]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/transports.mdx
[v1-content]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v1/content.mdx
[v1-schema]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/schema/v1/schema.json
[v2-migration]: https://github.com/agentclientprotocol/agent-client-protocol/blob/128845f5bd4c7fca5f23374e9b4853e470ecf99a/docs/protocol/v2/migration.mdx
[ts-readme]: https://github.com/agentclientprotocol/typescript-sdk/blob/4356253eb95ad02d9278af93e9ae6a28cf1d7a67/README.md#L17-L31
[ts-package]: https://github.com/agentclientprotocol/typescript-sdk/blob/4356253eb95ad02d9278af93e9ae6a28cf1d7a67/package.json#L1-L60
[rust-readme]: https://github.com/agentclientprotocol/rust-sdk/blob/52d92831d9c0ddc621afdfe18a44ea9de006b43e/README.md#L1-L72
[rust-cargo]: https://github.com/agentclientprotocol/rust-sdk/blob/52d92831d9c0ddc621afdfe18a44ea9de006b43e/Cargo.toml#L1-L43
[go-readme]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/README.md
[go-releases]: https://github.com/coder/acp-go-sdk/releases/tag/v0.13.5
[go-version]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/schema/version
[go-meta]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/schema/meta.json
[go-schema]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/schema/schema.json
[go-unstable-schema]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/schema/schema.unstable.json
[go-generator]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/cmd/generate/main.go#L15-L62
[go-connection]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/connection.go#L367-L446
[go-golden]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/json_parity_test.go
[go-notify-tests]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/connection_notification_barrier_test.go
[go-types]: https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/types_gen.go
[claude-agent]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/acp-agent.ts
[claude-tools]: https://github.com/agentclientprotocol/claude-agent-acp/blob/e6681d2a5734857727352474c8c9aa848f9210ee/src/tools.ts
[dive-response]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/response.go#L10-L62
[dive-api]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/dive.go#L307-L311
[dive-permission]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/permission/hooks.go#L1-L44
[dive-session]: https://github.com/deepnoodle-ai/dive/blob/e9d281049a3d51a2594ee258eeb8599517299e42/session/turns.go#L121-L238
