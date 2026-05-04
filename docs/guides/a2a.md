# A2A (Agent-to-Agent) Support

The `a2a` package lets a Dive agent act as either end of the
[Agent-to-Agent protocol](https://google.github.io/A2A/):

- **Server**: expose a `*dive.Agent` as a reachable A2A endpoint.
- **Client**: call a remote A2A agent from Go code via `RemoteAgent`.

It uses the official [`a2a-go/v2`](https://github.com/a2aproject/a2a-go)
SDK for transport (JSON-RPC, REST), task persistence, streaming, and agent
card serving — so protocol-level concerns are handled by the SDK and Dive
focuses on the translation layer.

## Exposing a Dive agent as an A2A server

```go
import (
    "net/http"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/a2a"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
)

agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:         "My Agent",
    SystemPrompt: "You are a helpful assistant.",
    Model:        anthropic.New(),
})

srv, err := a2a.NewServer(a2a.ServerOptions{
    Agent:   agent,
    BaseURL: "https://my-agent.example.com",
    Card: a2a.CardOptions{
        Name:        "My Agent",
        Description: "A helpful assistant.",
    },
})

http.ListenAndServe(":8080", srv.Handler())
```

`srv.Handler()` mounts two routes:
- `/.well-known/agent-card.json` — agent card discovery
- `/` — the A2A protocol endpoint (JSON-RPC by default; set `Transport: "rest"` for REST)

### Transport selection

```go
srv, _ := a2a.NewServer(a2a.ServerOptions{
    Agent:     agent,
    Transport: "rest",   // "jsonrpc" (default) or "rest"
})
```

### Dynamic agent card

When card metadata may change without restarting the server (e.g. available
tools vary by user, version metadata is hot-reloaded), use `CardProvider`
instead of a static `Card`. The provider receives the full SDK `AgentCard`
type for complete control:

```go
import a2asdk "github.com/a2aproject/a2a-go/v2/a2a"

srv, _ := a2a.NewServer(a2a.ServerOptions{
    Agent: agent,
    CardProvider: func(ctx context.Context) (*a2asdk.AgentCard, error) {
        return resolveCardFromDB(ctx)
    },
})
```

The provider is called on every request to `/.well-known/agent-card.json`.
Callers wanting caching should wrap their implementation themselves.
`Server.Card()` returns nil when a `CardProvider` is set.

### Session persistence across turns

Without a `SessionProvider`, each A2A task runs in a fresh stateless agent
turn. To maintain conversation history across multiple messages in the same
context, supply a `SessionProvider`:

```go
import "github.com/deepnoodle-ai/dive/session"

var mu sync.Mutex
store := make(map[string]dive.Session)

srv, _ := a2a.NewServer(a2a.ServerOptions{
    Agent: agent,
    SessionProvider: func(ctx context.Context, contextID string) (dive.Session, error) {
        mu.Lock()
        defer mu.Unlock()
        if sess, ok := store[contextID]; ok {
            return sess, nil
        }
        sess := session.New(contextID)
        store[contextID] = sess
        return sess, nil
    },
})
```

## Calling a remote A2A agent

`RemoteAgent` wraps the a2a-go client with ergonomic text-based methods and
automatic context-ID tracking. The return type is `*TaskResult` — a simple
struct with `ID`, `ContextID`, `State`, and `Text` fields — so most callers
need no SDK imports at all.

```go
import "github.com/deepnoodle-ai/dive/a2a"

// Connect by URL (most common).
remote, err := a2a.NewRemoteAgentFromURL(ctx, "https://my-agent.example.com")

// Simple send-and-receive.
result, err := remote.SendText(ctx, "What is the capital of France?")
fmt.Println(result.Text)           // extracted response text
fmt.Println(result.IsCompleted())  // true

// Streaming with a chunk callback.
result, err = remote.StreamText(ctx, "Tell me a story", func(chunk string) {
    fmt.Print(chunk)
})
```

`remote.ContextID()` is automatically updated from each response so
follow-up calls continue the same A2A context without manual tracking.

### Power-user escape hatches

When you need full SDK access, use:

- `remote.LastTask() *a2asdk.Task` — raw SDK task from the most recent call.
- `remote.Client() *a2aclient.Client` — underlying SDK client.
- `remote.StreamEvents(ctx, prompt)` — raw `iter.Seq2[a2asdk.Event, error]` iterator.

## Suspend / resume

Dive's [suspend/resume](./suspend-resume.md) mechanism maps naturally to
A2A's `input-required` state. When a tool returns `dive.NewSuspendResult`,
the executor emits a `TaskStateInputRequired` event and stashes the
suspension state in the task's metadata. On the next message targeting the
same task ID, the executor reconstructs the suspension and resumes the agent
turn.

```go
// Server-side tool that pauses for approval.
approve := dive.FuncTool("request_approval", "Pause for human approval",
    func(ctx context.Context, in *approveIn) (*dive.ToolResult, error) {
        return dive.NewSuspendResult("Approve this action?", nil), nil
    },
)

// Client-side: handle input-required and resume.
result, _ := remote.SendText(ctx, "Do something consequential")
if result.IsInputRequired() {
    result, _ = remote.SendTextOnTask(ctx, result.ID, "yes")
}
fmt.Println(result.Text)
```

Both `SuspendReasonInput` and `SuspendReasonAuth` map to `TaskStateInputRequired`
so the executor goroutine terminates and resume works via a new `SendMessage`.
The suspend reason is preserved in `dive.suspension` task metadata for clients
that need to distinguish the two cases.

## Structured tool results on resume

When a suspension has multiple pending tool calls, resume messages can carry
a `toolResults` data part that maps call IDs to result strings:

```json
{
  "role": "user",
  "parts": [{
    "kind": "data",
    "data": {
      "toolResults": {
        "call_abc": "approved",
        "call_def": "denied"
      }
    }
  }]
}
```

Plain text resume messages broadcast the same text to all pending calls.

## See also

- [`examples/a2alib_example/`](../../examples/a2alib_example/main.go) — runnable end-to-end example
- [Suspend/resume guide](./suspend-resume.md)
- [a2a-go SDK docs](https://github.com/a2aproject/a2a-go)
