# A2A (Agent-to-Agent) Support

> **Experimental**: This package is in `experimental/a2a/`. The API will
> change. See [PRD-05](../../prds/prd-05-a2a-support.md) for the motivation
> and roadmap.

The A2A protocol is a standards-based way for agents to discover each
other, exchange messages, and drive long-running tasks across process and
network boundaries. Dive's `experimental/a2a` package lets a Dive agent
play either end of that protocol:

- **Server**: wrap a `*dive.Agent` so it is reachable as a remote A2A agent.
- **Client**: call a remote A2A agent from Go code without hand-assembling
  JSON-RPC requests.

This guide walks through both sides, explains how Dive's suspend/resume
maps to A2A's `input-required` state, and calls out the phase-1 limits.

## Package layout

```
experimental/a2a/
  doc.go              — package overview
  types.go            — wire types (AgentCard, Task, Message, Part, …)
  rpc.go              — JSON-RPC envelope + error codes + method names
  task_store.go       — TaskStore interface + in-memory implementation
  server.go           — HTTP server adapter wrapping a *dive.Agent
  client.go           — JSON-RPC client + SSE streaming parser
  remoteagent.go      — higher-level wrapper around Client
```

Everything lives in a single package for the first pass. If the surface
grows we will split along the PRD's `card/`, `client/`, `server/`,
`remoteagent/` boundaries.

## Exposing a Dive agent as an A2A server

```go
import (
    "context"
    "net/http"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/experimental/a2a"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
    "github.com/deepnoodle-ai/dive/session"
)

func main() {
    agent, _ := dive.NewAgent(dive.AgentOptions{
        Name:         "Research Assistant",
        SystemPrompt: "You are a concise researcher.",
        Model:        anthropic.New(),
    })

    sessions := map[string]dive.Session{}
    provider := func(ctx context.Context, contextID string) (dive.Session, error) {
        if s, ok := sessions[contextID]; ok {
            return s, nil
        }
        s := session.New(contextID)
        sessions[contextID] = s
        return s, nil
    }

    server, _ := a2a.NewServer(a2a.ServerOptions{
        Agent:           agent,
        BaseURL:         "https://agent.example.com",
        SessionProvider: provider,
    })

    http.ListenAndServe(":8080", server.Handler())
}
```

The returned `http.Handler` serves these paths:

- `GET /.well-known/agent-card.json` — the canonical A2A agent card path
- `GET /.well-known/agent.json` — the legacy path, kept for older clients
- `POST /` (configurable via `ServerOptions.Path`) — the JSON-RPC endpoint

### Supported JSON-RPC methods

| Method | What it does |
|---|---|
| `message/send` | Single-shot request, blocks until the agent finishes its turn |
| `message/stream` | Same as above but streams progress over Server-Sent Events |
| `tasks/get` | Retrieve a task by ID |
| `tasks/cancel` | Mark an in-flight task as canceled |

The dispatcher also recognizes `tasks/resubscribe`,
`tasks/pushNotificationConfig/{set,get,list,delete}`, and
`agent/getAuthenticatedExtendedCard` — but they are not implemented and
return `-32004 UnsupportedOperation`. Any other method returns
`-32601 MethodNotFound`.

### Sessions

`SessionProvider` is optional. When nil, the server runs the agent with no
session — safe but single-turn. Plug in a provider to get multi-turn
conversations keyed by A2A `contextId`. Any `dive.Session` implementation
works (`session.New`, file-backed, custom).

### Agent card

`ServerOptions.Card` is the static portion of the card. The server fills in
`Name`, `Version`, `URL`, and `Capabilities.Streaming` with sensible
defaults. Skills, input/output modes, and security schemes pass through
unchanged.

## Calling a remote A2A agent

```go
client, _ := a2a.NewClient(a2a.ClientOptions{
    Endpoint: "https://agent.example.com/",
})

remote := a2a.NewRemoteAgent(client)
card, _ := remote.Card(ctx)
fmt.Println("remote:", card.Name)

task, _ := remote.SendText(ctx, "Plan a route from SFO to Lake Tahoe.")
fmt.Println(task.Status.State)
fmt.Println(a2a.ResponseText(task))
```

`RemoteAgent` remembers the context ID returned on the first call, so
subsequent `SendText` calls continue the same conversation.

For streaming progress, call `remote.StreamText(ctx, prompt, onEvent)`.
The callback receives `*a2a.StreamEvent` values — each carries exactly one
of `Task`, `StatusUpdate`, or `ArtifactUpdate`.

Low-level callers who need full control can bypass `RemoteAgent` and use
`Client.SendMessage`, `Client.GetTask`, `Client.CancelTask`, and
`Client.StreamMessage` directly.

## Suspend → input-required / auth-required mapping

A Dive tool can pause the agent mid-turn with `dive.NewSuspendResult` or
`dive.NewSuspendResultWithReason`:

```go
func approveTool() dive.Tool {
    return dive.FuncTool("approve", "Ask the human for approval.",
        func(ctx context.Context, _ *struct{}) (*dive.ToolResult, error) {
            return dive.NewSuspendResult("Approve the deployment?", nil), nil
        })
}

func authGateTool() dive.Tool {
    return dive.FuncTool("auth_gate", "Require authentication.",
        func(ctx context.Context, _ *struct{}) (*dive.ToolResult, error) {
            return dive.NewSuspendResultWithReason("Sign in to continue",
                dive.SuspendReasonAuth, map[string]any{"auth_url": url}), nil
        })
}
```

The A2A adapter maps the suspend reason to the correct A2A task state:

| `SuspendReason` | A2A state |
|---|---|
| `""` or `SuspendReasonInput` | `input-required` |
| `SuspendReasonAuth` | `auth-required` |

The suspended tool's `Prompt` becomes `task.Status.Message`, and its
`Metadata` appears under `task.metadata.suspend`:

```json
{
  "id": "task-abc",
  "contextId": "ctx-xyz",
  "status": {
    "state": "input-required",
    "message": {
      "messageId": "msg-123",
      "role": "agent",
      "parts": [{ "kind": "text", "text": "Approve the deployment?" }]
    },
    "timestamp": "2026-04-11T19:20:00Z"
  }
}
```

A remote caller resumes the task by sending another message targeting the
same `taskId`. The next message text is plumbed through Dive's
`WithResume`/`WithToolResults` API as the suspended tool's result, so the
agent continues exactly where it paused.

### Cancellation

`tasks/cancel` does two things: marks the stored task record as canceled
**and** cancels the Go context for any in-flight `CreateResponse` on that
task. This means a cancel request will interrupt a running LLM call,
stopping token generation and freeing resources. If no turn is in
flight (e.g. the task is already in `input-required`), only the store
record is updated.

### Multi-pending-tool-call resume

When a Dive agent uses parallel tool execution and multiple tools
suspend simultaneously, the adapter supports resuming all of them in a
single message. Two conventions are available:

**Structured DataPart** (preferred for distinct per-call answers):

```json
{
  "parts": [
    { "kind": "data", "data": {
        "toolResults": {
          "call_1": "approved",
          "call_2": "denied"
        }
    }}
  ]
}
```

**Text broadcast** (when one answer applies to all pending calls): send
a plain text message and it will be used as the result for every pending
call.

### Cancellation and session cleanup

`tasks/cancel` now also clears the underlying Dive session's suspension
state when a `SessionProvider` is configured and the session implements
`SuspendableSession`. This means a canceled A2A task won't leave stale
pending tool calls in the session.

For direct session-level cleanup (outside of A2A), use
`session.CancelSuspension(ctx)` to abandon a suspended turn and return
the session to a clean state.

### Current limits

- Non-text input parts (`DataPart`) are rendered as JSON code blocks in
  the agent's prompt. `FilePart` is projected to the appropriate typed
  content: image MIME types become `ImageContent`, everything else becomes
  `DocumentContent`. The agent sees typed multi-modal content, not
  flattened text references.
- `Client.SendMessage` always returns `*Task`. When a peer returns a
  bare `Message` (spec-allowed for direct replies), the client wraps it
  in a synthesized completed task whose single `"response"` artifact
  carries the message's text and whose metadata has
  `a2a.syntheticFromMessage: true`. Callers who need to tell a real
  task from a wrapped message can check that flag.
- `tasks/resubscribe`, the `tasks/pushNotificationConfig/*` family, and
  `agent/getAuthenticatedExtendedCard` are recognized by the dispatcher
  but not implemented: probing them returns
  `-32004 UnsupportedOperation` (not `-32601 MethodNotFound`) so peers
  get a meaningful signal.
- Phase 1 was validated against `a2a-sdk` (Python, current main as of
  April 2026) in both directions. Our server passes the Python client's
  agent-card validation, single-shot send, streaming, `tasks/get`, and
  `tasks/cancel` checks. Our client successfully fetches the card,
  sends/receives messages, and parses streamed events from a stock
  Python `A2AStarletteApplication`. See `interop_test.go` for the
  automated cross-validation (build-tag gated: `go test -tags interop
  ./experimental/a2a/...`).

## Task storage

`ServerOptions.TaskStore` defaults to `a2a.NewMemoryTaskStore()`, which is
fine for local development and tests. Production callers should provide a
database-backed implementation so tasks and suspensions survive restarts.
The interface is intentionally small:

```go
type TaskStore interface {
    Put(ctx context.Context, rec *TaskRecord) error
    Get(ctx context.Context, id string) (*TaskRecord, bool, error)
    Delete(ctx context.Context, id string) error
    List(ctx context.Context) ([]*TaskRecord, error)
}
```

A `TaskRecord` carries both the A2A `Task` (wire state) and the Dive
`*SuspensionState` needed to resume. Keeping them together means a resume
can happen on a different process from the original `message/send` as
long as the store is shared.

`List` exists so callers can implement expiration and cleanup policies.
For example, a periodic goroutine that deletes tasks older than 24 hours:

```go
recs, _ := store.List(ctx)
for _, rec := range recs {
    if time.Since(rec.Task.Status.Timestamp) > 24*time.Hour {
        store.Delete(ctx, rec.Task.ID)
    }
}
```

## Wire format

The package targets the current A2A JSON-RPC protocol binding:

- Hyphenated lowercase task state strings (`input-required`, `completed`,
  …) on the wire.
- `kind` discriminators on `Part` (`text`/`file`/`data`), `Message`
  (`message`), `Task` (`task`), `TaskStatusUpdateEvent` (`status-update`),
  and `TaskArtifactUpdateEvent` (`artifact-update`). The Go marshalers
  fill these in automatically when the field is unset.
- JSON-RPC method names: `message/send`, `message/stream`, `tasks/get`,
  `tasks/cancel`.
- Agent card served at `/.well-known/agent-card.json` (canonical) **and**
  `/.well-known/agent.json` (legacy alias for older clients). The client
  fetches the canonical path and falls back to the legacy path on 404
  unless `ClientOptions.CardURL` is set explicitly.

Phase 1 has been validated against the official `a2a-sdk` (Python) in
both directions; see the Phase 1 limits section for caveats.

## Content projection

### Outbound (Dive response → A2A artifacts)

| Dive content type | A2A part |
|---|---|
| `TextContent` | `text` part |
| `ImageContent` | `file` part (with URI or base64, MIME type) |
| `DocumentContent` | `file` part (with title as name, MIME type) |
| `RefusalContent` | `text` part |
| Tool-internal types (ToolUse, ToolResult, Thinking) | skipped |

Each assistant message with renderable content becomes its own artifact.
Streaming emits all artifacts as `artifact-update` events before the
final status event.

### Inbound (A2A message → Dive user message)

| A2A part | Dive content type |
|---|---|
| `text` part | `TextContent` |
| `file` part (image/* MIME) | `ImageContent` |
| `file` part (other MIME) | `DocumentContent` |
| `data` part | `TextContent` (JSON code block) |

Inbound parts are projected as typed `llm.Content` on the user message,
preserving multi-modal fidelity for images and documents. Data parts are
rendered as JSON since LLMs don't have a native structured-data input.

## Library philosophy

This package provides the hard parts of A2A that are tightly coupled to
the protocol and to Dive's runtime — and stays out of concerns that
belong in your HTTP infrastructure.

**The library provides:**

- Protocol-correct JSON-RPC dispatch and SSE streaming
- Faithful projection of Dive responses onto A2A task state
- Suspend/resume mapping including multi-pending-tool-call resume
- Cancellation propagation to in-flight LLM calls
- `TaskStore` and `SessionProvider` interfaces for pluggable persistence
- `TaskStore.List` so you can implement your own expiration
- Agent card generation and well-known path serving
- Client with card fetching, fallback, and stream parsing

**Your code provides:**

- **Authentication and authorization**: wrap `server.Handler()` in your
  auth middleware, or use a reverse proxy. The client's
  `ClientOptions.Headers` carries bearer tokens. The library does not
  interpret `AgentCard.SecuritySchemes`.
- **Rate limiting and request size limits**: configure on your
  `http.Server` or middleware layer.
- **Timeouts**: set `ReadTimeout`/`WriteTimeout` on your `http.Server`.
  The agent's own `ResponseTimeout` caps turn duration.
- **Graceful shutdown**: call your `http.Server.Shutdown(ctx)` — the
  in-flight turn contexts will be canceled via the standard Go mechanism.
- **Durable TaskStore**: implement the four-method interface backed by
  your database. `MemoryTaskStore` is for prototyping.
- **Task expiration**: use `TaskStore.List` to find and delete stale
  records on your own schedule.
- **Observability**: instrument your middleware with metrics and tracing.
  The library accepts an `llm.Logger` for structured debug logging.

The goal is that `experimental/a2a` handles protocol fidelity and runtime
integration so you can focus on deployment concerns specific to your
environment.

## Boundaries

`experimental/a2a` does not leak A2A types into the stable `dive`
package. `*dive.Agent`, `*dive.Response`, and `dive.Session` are unchanged.
If you move away from A2A later, removing this package does not force a
core-API migration.

MCP support remains a separate story. MCP is for tools and data; A2A is
for agents. They live in different experimental packages and are
independently adoptable.

## End-to-end example

See `examples/a2a_example/main.go` for a runnable example that:

1. Builds a Dive agent
2. Exposes it over A2A via `httptest`
3. Fetches the agent card from `/.well-known/agent.json`
4. Sends a message and prints the response
5. Sends a follow-up on the same context to exercise session reuse
