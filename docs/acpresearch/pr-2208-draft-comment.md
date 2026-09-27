# ACP PR #2208: durable and resumable HTTP/WS transport, with a draft comment

_Prepared 2026-09-26. Status: draft for Curtis to edit and post. Nothing has been posted._

## The PR

- **PR:** [agentclientprotocol/agent-client-protocol#2208](https://github.com/agentclientprotocol/agent-client-protocol/pull/2208), "docs(rfd): add durable & resumable HTTP/WS transport RFD".
- **Author:** alexhancock. Opened 2026-09-22.
- **Status:** open, with **no comments or reviews** as of 2026-09-26.
- **Text:** [`docs/rfds/durable-resumable-http-transport.mdx` @ fc269b3](https://github.com/agentclientprotocol/agent-client-protocol/blob/fc269b332c7422eeeb88438a7e6d26fe3e845544/docs/rfds/durable-resumable-http-transport.mdx).
- **Why it exists:** the original [Streamable HTTP/WebSocket transport RFD](https://github.com/agentclientprotocol/agent-client-protocol/blob/main/docs/rfds/streamable-http-websocket-transport.mdx#durability-and-reliability-expectations) put durability off to "a later revision". This is that revision: "this is the RFD that does so."

### What it proposes

1. **A cursor on every server-to-client message.** Each stream gets an opaque cursor that is monotonic within that stream. It goes in the SSE `id:` field and in `_meta["acp/cursor"]`, so WebSocket connections get it too. The RFD defines two kinds of stream, one per connection and one per session.
2. **Resume by cursor.** The client sends `Last-Event-ID`, or a `stream/resume` message on WebSocket. The server then does one of two things:
   - replays the missed messages *exactly*, cursors included, and then continues live; or
   - refuses with `409` or `stream/resume_failed`, and never silently opens a live stream.

   After a refusal, the client falls back to `session/resume` with `replayFrom`.
3. **Retention is advertised** at `initialize`, as `capabilities.transport.resumable{retentionMessages, retentionSeconds}`.
4. **Reconnect rules:**
   - Clients reconnect to the same `Acp-Connection-Id` with backoff and jitter.
   - A dropped stream must not cancel work on the server.
   - A keepalive is sent regularly; a client that hears nothing for five keepalive intervals reconnects.
5. **State on resume.** After a successful resume, the server **MUST** send a `state_update` (`running`, `idle`, or `requires_action`) before any live data. v1 gets an advisory `_meta["acp/turnState"]` shim instead.
6. **Out of scope, explicitly:** durable replay logs. The FAQ says "In-memory is fine and is the expected implementation … if the agent process dies … the cursor should be refused."

The RFD also leaves two TODOs open:
- whether the grace period before the server tears down a connection is set ACP-wide or by each server;
- whether to bother with the v1 shim at all.

## Why we should comment

- **We've already built this layer, and it works.** Nvoken's SSE protocol is in production and does exactly what this RFD is aiming for, with one difference: it is backed by a durable log. Its main features:
  - an opaque cursor tied to a Turn or Conversation, not to a connection;
  - replay that "repeats nothing and skips nothing";
  - a split between permanent `transcript.update` frames and throwaway `message.delta` previews;
  - an explicit `stream.resync` message when previews were lost;
  - `connection.closing` messages that say *why* the connection is closing;
  - 55 numbered MUST rules with shared reducer fixtures.
- **Another team reached the same design independently.** OpenHands' [WebSocket v2 session protocol](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-agent-server/openhands/agent_server/session_protocol.py#L1-L21) has an `after_seq` cursor, and splits `Durable` frames from `Delta`/`ItemStarted`/`ItemAborted` frames. acpx's [watch journal](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/session-watch.md) makes the same choices. When independent designs agree like this, it is evidence the spec can cite, not just our opinion. See [comparison README §11 and §14](../comparison/README.md), and [acp-v2-and-streaming.md §4](acp-v2-and-streaming.md).
- **Cheap now, expensive later.** The PR is four days old and nobody has commented. The RFD shows up in the "Shiny future" section of the spec docs, and its SDK work (Rust first, then TypeScript) will fix the semantics in place.
- **It decides whether Nvoken can be a conformant ACP server.** As written, the RFD requires exact replay and expects resume to fail across an agent restart. A server backed by a durable log could honor those rules only by weakening its own guarantees. We don't need ACP to adopt Nvoken's format. We need the spec to *permit* a server like Nvoken.

### What the RFD gets right, and we should say so

- Cursors are separate from `messageId` and `toolCallId`, and clients may not parse them.
- When a server can't resume, it must refuse rather than silently open a live stream.
- State is sent on resume, which lets a client tell "the connection broke" apart from "the agent broke".
- A dropped stream doesn't cancel work.
- Retention is advertised, and "resumption is an optimization over the fallback, never a replacement".

### Where it falls short for us, in priority order

1. **Exact replay only.** "Replayed messages are the same as the originals" makes a server that is backed by a log keep every preview chunk. It also rules out resuming after the agent restarts. v2's replace-by-ID upserts already make a **state-equivalent** replay safe: send whole `agent_message` or `tool_call_update` objects in place of the chunks that were missed.
2. **Cursors tied to one connection.** A cursor "that belongs to a connection the server no longer has" gets refused. That blocks two things: a second reader ([#533, multi-client attach](https://github.com/agentclientprotocol/agent-client-protocol/pull/533)), and reattaching once the server has discarded the connection record. Cursors on session-scoped streams should be valid for that session on any connection.
3. **The fallback replays everything.** After a refusal the fallback is `session/resume` plus `replayFrom`, but in the draft `replayFrom` only accepts `start`. So a missed blip turns into a full-history replay. Letting `replayFrom` take a position would fix this, either the transport cursor or a [#2114 session cursor](https://github.com/agentclientprotocol/agent-client-protocol/pull/2114).
4. **`requires_action` can be a dead end.** The RFD says it most likely means "a permission request you never received", but nothing re-sends that request. Any agent-to-client request still outstanding (such as `session/request_permission`) needs to be re-issued after a resume.
5. **The durable/throwaway distinction isn't stated.** If servers can mark which updates are safe to drop on replay (chunks followed by a whole-object update, and notices), clients can reason about gaps and servers can keep smaller buffers.

We should **not**:
- propose a competing RFD;
- push Nvoken's wire format;
- ask for durable logs to be *required*.

What we want is permission. In-memory ring buffers remain the expected implementation, and durable-log servers are also conformant.

## Venue and follow-ups

- **Where:** post one consolidated comment on the PR. Consider putting the same summary in the Transports WG Zulip channel as well. Per [acp-v2-and-streaming.md §3.2](acp-v2-and-streaming.md), the transport champion is anna239 and the v2 lead is benbrandt, so it's worth mentioning both.
- **Who:** Curtis, as a Deep Noodle / Dive maintainer.
- **Separate threads, not in this comment:**
  - usage semantics on [#1860](https://github.com/agentclientprotocol/agent-client-protocol/issues/1860) (Nvoken's disjoint input buckets);
  - detached tool calls on [#1847](https://github.com/agentclientprotocol/agent-client-protocol/issues/1847).
- **Before posting:**
  - Re-read the latest revision of the RFD. It may have changed since fc269b3.
  - Decide whether to link Dive's docs publicly. Nvoken is private, so describe its behavior and don't link it.
  - Confirm we're willing to back the conformance-test offer with actual cases.

---

## Draft comment

> Paste-ready. It's about 750 words. Trim the "Prior art" paragraph if a shorter comment is preferred.

---

Thanks for writing this up. Remote ACP needs exactly this, and the core choices look right to me: cursors that are opaque and separate from `messageId` and `toolCallId`, refusing rather than silently going live, a mandatory `state_update` on resume, and a dropped stream never cancelling work. The "connection broke vs. agent broke" framing is the clearest statement of the problem I've seen.

Some context on where the comments below come from. We run a remote agent gateway, built on our Go agent library [Dive](https://github.com/deepnoodle-ai/dive), that streams sessions to browser and server clients over SSE. It has an opaque, scope-bound cursor. It splits durable frames from ephemeral preview frames. It replays so that it "repeats nothing and skips nothing", and it has explicit resync and closing-reason frames. OpenHands' agent server independently arrived at nearly the same design ([session protocol](https://github.com/OpenHands/software-agent-sdk/blob/d77ada7a030b3acaa82593d402632680361dfe42/openhands-agent-server/openhands/agent_server/session_protocol.py#L1-L21): `after_seq`, and durable frames kept apart from delta/started/aborted frames), and so did acpx's [watch journal](https://github.com/openclaw/acpx/blob/c62ae8cd86757e2dba644ed7b402d6a36d75031e/docs/session-watch.md). Most of the suggestions below would let servers like these be conformant *without* changing what the RFD expects of a simple in-memory implementation.

**1. Allow state-equivalent replay, not only byte-identical replay.**
Today replayed messages "are the same as the originals, including their cursors." For a server that doesn't keep every chunk, v2's upsert semantics already give a safe alternative: in place of missed `agent_message_chunk` or `tool_call_update` content chunks, the server replays one whole-entity update per message or tool call (replace-by-ID). The client reaches the same state. I'd suggest: a server MUST replay messages *or* state-equivalent upserts for every entity touched after the cursor, in order, and cursors stay monotonic. This lets servers backed by a log resume without buffering previews. It also lets them honor a cursor across an agent restart when they still can, instead of being told the cursor "should be refused." Ring-buffer servers are unaffected.

**2. Scope session-stream cursors to the session, not the connection.**
A cursor that "belongs to a connection the server no longer has" is refused. That blocks two cases: reattaching after the server has garbage-collected the `Acp-Connection-Id`, and a second reader of the same session ([#533](https://github.com/agentclientprotocol/agent-client-protocol/pull/533)). If a session-scoped stream's cursor were valid for that session on any connection, both would work, with no change for single-client deployments. Connection-scoped streams could keep their current rule.

**3. Make the fallback resume from a position.**
On refusal the client falls back to `session/resume` + `replayFrom`, but the draft's `replayFrom` only supports `start`. A short blip past the retention window then becomes a full-history replay. A `replayFrom: { type: "cursor", cursor }` variant would make the fallback proportional to what was missed. It could take the transport cursor, or a session cursor from [#2114](https://github.com/agentclientprotocol/agent-client-protocol/pull/2114). It would be worth saying explicitly how the transport cursor and #2114 relate.

**4. Re-issue outstanding agent→client requests on resume.**
The RFD notes that `requires_action` after resume most likely means "a permission request you never received." Nothing currently re-sends that request, so the client knows it's blocked but can't unblock. I'd suggest that after a successful resume, and after the fallback `session/resume`, the server MUST re-issue any still-pending `session/request_permission` (and other client-directed requests) after the `state_update`. #533's `permission_resolved` notification would complement this well for multi-reader cases.

**5. (Smaller) Say which updates are lossy.**
It would help implementers if the RFD (or v2 core) said which update kinds a server MAY omit from replay when a whole-entity update supersedes them: chunks, and perhaps notices. It could be per variant, or via something like `_meta["acp/lossy"]`. Clients can then reason about gaps, and servers can keep smaller buffers.

On the open TODOs, for what it's worth:
- The teardown grace period seems like it should be server-specific but advertised next to `retentionSeconds`, so clients know how long a reconnect can take.
- I'd lean toward v2-only for the turn-state half. `state_update` is the clean mechanism, and a v1 `_meta` shim adds a second path to test for a version that's on its way out.

None of this asks for durable logs to be required. In-memory ring buffers should stay the expected implementation. The goal is to make sure the rules don't *exclude* servers that can do better. We're happy to contribute conformance cases from our implementation: mid-stream drop, expired cursor, duplicate resume, resume across an agent restart, and a slow consumer.
