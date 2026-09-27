# ACP ecosystem research

_Snapshot: 2026-09-26. Source-level review of the repositories and issue trackers in the research brief. No adapter was implemented and no cross-client runtime test was run._

## Read in this order

1. [Protocol, SDKs, and Claude agent adapter](acp-spec-sdk.md) — stable v1 methods, capabilities, session/update vocabulary, draft v2, official SDK references, and a source/test-based assessment of `coder/acp-go-sdk`.
2. [Dive and Nvoken fit](acp-dive-nvoken.md) — concrete Dive-to-ACP event and permission mapping, ACP-to-Nvoken stream comparison, and the expose-versus-consume decision.
3. [Clients and control planes](acp-clients.md) — Zed, acpx, registry, OpenHands Canvas/software-agent-sdk, and ACP UI; the state each adds around ACP.
4. [Interoperability issues and agent implementations](acp-issues.md) — current issue states for protocol, Claude adapter, and Go SDK; Codex and Gemini ACP behavior; ordering, resume, permissions, background work, and remote transport.

## Deeper dives (added 2026-09-26)

The four docs above are a broad first pass. These go deeper, and one of them includes an empirical spike:

5. [Dive as an ACP agent: adapter design and spike](acp-agent-adapter.md) covers:
   - Zed's client contract, read from source;
   - how the Claude, Codex and Gemini ACP agents emit updates;
   - the Go SDK decision;
   - a **working spike**: an unmodified Dive agent on a scripted model, driven by acpx 0.19.3 and a Go client (`~/git/lib/acp-spike`, `./run.sh`, wire traces in `traces/`);
   - a Dive-to-ACP mapping spec and a sketch of the `dive/acp` package.
6. [ACP v2, the proposals in flight, and how the streaming designs converge](acp-v2-and-streaming.md) covers:
   - the full v2 draft and its client reducer;
   - the RFDs in flight and who governs the spec;
   - a convergence matrix across ACP v1/v2, Nvoken SSE, OpenHands WS v2, acpx and the Dive v2 proposal;
   - a recommended Dive event vocabulary, with projection rules;
   - whether and how to engage the spec.
7. [Consuming ACP agents from Dive, Nvoken and Mobius](acp-client-orchestration.md) covers:
   - OpenHands' `ACPAgent` and acpx in depth;
   - agent-side realities for each agent;
   - designs for a Dive client package, an Nvoken ACP executor (with a state machine and a failure-mode table) and Mobius;
   - build versus buy.
8. [Draft comment for ACP PR #2208 (durable/resumable transport)](pr-2208-draft-comment.md) gives the context and a comment ready to paste. It has not been posted.

### What the deeper dives changed

- **Go SDK: supersedes the earlier "wrap it" recommendation.** Write a small, Dive-owned JSON-RPC layer and v1 types. Keep `coder/acp-go-sdk` only as a test client:
  - It *can* be regenerated against schema v1.23.0.
  - But it silently drops the elicitation session and tool-call IDs in both directions (proven on the wire).
  - It decodes unknown update variants as `session_info_update`.
  - Cancelling a prompt's context discards the final response.
  - Every schema bump adds required interface methods.
  - Nothing has been pushed since June.

  For the *client* side, [acp-client-orchestration.md](acp-client-orchestration.md) still suggests using it forked and pinned behind a wrapper. Settle this in one place before building.
- **The agent adapter is proven feasible.** Streamed text and thinking, tool calls with diffs, allow/always/reject, cancel (including during a pending permission, which fails closed), and list/load/resume across a restart all worked against a real client. What was missing is on Dive's side:
  - message IDs that stay unique across replay;
  - an honest tool-start (`in_progress`) event;
  - per-call usage for context meters;
  - diffs persisted for replay;
  - cancelled-turn synthetic messages that replay must skip.

  All five point at the Dive v2 record model.
- **Zed's review UI needs Dive's file tools to go through the client's filesystem.** Keep/Reject and Review Changes are driven only by `fs/write_text_file`, so Dive's Edit, Write and Read tools need a filesystem seam. Other Zed quirks are worth knowing: config options suppress modes, unadvertised slash commands are rejected client-side, and `line` is treated as 0-based.
- **Consuming ACP means building a control plane around it.** Everyone ends up building the same layer:
  - process-group supervision;
  - per-tenant credential homes;
  - strict versus fallback resume;
  - a raw log with an unknown-outcome state;
  - quiescence-based turn settlement;
  - silent-turn watchdogs;
  - a host permission policy.

  **ACP permission prompts are not a security boundary.** All three agents write to disk with their own tools, so the sandbox or worktree is the boundary. Build a Go client plus a small Go bridge, not acpx. For Nvoken, the bridge lives in the per-conversation sandbox, so worker death doesn't kill the agent. Target the TypeScript `agentclientprotocol/codex-acp`; the Rust Zed one is deprecated.
- **ACP v2 should be a projection of Dive's vocabulary, not its source.** Use three event classes (committed, preview, signal). Keep one output ID stable across retries, plus an attempt number, so the v2 projection is lossless. Build v1 first; ship v2 on the wire behind a flag. Comment on PR #2208 rather than writing a competing RFD.

## Findings that change a decision (first pass)

- **Target ACP v1 today.** The inspected current v1 schema artifact is `1.23.0`; v2 is `2.0.0-alpha.5` and still draft. V2 changes the prompt completion and update reducer semantics, so the two wire versions need separate translation. [Evidence](acp-spec-sdk.md#version-boundary)
- **The Go SDK is usable with a compatibility layer.** `coder/acp-go-sdk` passed `go test ./...` at the inspected commit and has useful JSON-RPC/stdio machinery, but its June `v0.13.5` schema pin trails September's stable v1 schema. Several now-stable types remain named `Unstable*`, and optional operations require interface stubs. Keep the SDK behind a narrow adapter and verify against the current schema plus a real client. [Evidence](acp-spec-sdk.md#sdk-assessment)
- **Expose Dive for editor interoperability.** Dive has the turn, session, callback, tool, suspension, and permission hooks to build an ACP agent adapter. The hard parts are truthful capability advertisement, structured tool updates, permission binding, ordered notifications, session replay, and lossy mapping of Dive's richer incomplete outcomes into v1 stop reasons. Zed is the primary client acceptance target. [Mapping](acp-dive-nvoken.md#dive-as-an-acp-agent) · [Zed findings](acp-clients.md#zed-what-an-editor-client-expects)
- **Consume ACP in an orchestration layer for external agents.** acpx and Canvas show that a control plane must own process lifetime, identity, sequencing, permissions, and recovery around ACP. An ACP conversation is not by itself a durable event log. A client integration can be tool-shaped for bounded subtasks, but persistent agents need a dedicated session manager. [Control-plane findings](acp-clients.md#acpx-the-headless-client-and-its-added-control-plane) · [Dive boundary](acp-dive-nvoken.md#dive-as-an-acp-client)
- **Keep Nvoken's cursor stream authoritative.** ACP v1 `session/load` replays a conversation, and `session/resume` reconnects without replay. Nvoken's durable `transcript.update` cursor and separate ephemeral previews give a stronger, precise reconnect contract. A gateway can project Nvoken into ACP for editor display while keeping Nvoken's Turn and cursor as the source of truth. [Comparison](acp-dive-nvoken.md#acp-versus-nvokens-stream) · [Remote transport evidence](acp-issues.md#resume-and-remote-transport-boundary)

## Suggested next proof

Build one small v1 agent-side Dive adapter with a real session store, then test `initialize`, new/prompt, streamed text and tool calls, permission selection, cancel, process restart, `load`/`resume`, and incomplete turn reporting against Zed and one second client. Separately, if consuming external agents is a near-term product goal, run a client prototype against Claude and Codex ACP with an explicit same-session recovery requirement and a host-owned permission policy. These are proposed experiments, not findings already proven by this source pass.
