# Session Design

## Goals

1. Persistent conversation state across multiple turns with storage to/from a pluggable backend.
2. Storage abstracted behind a `Store` interface.
3. Sessions easily appended to as a conversation continues (append is the hot path).
4. Sessions are optional — agents are stateless when no session is provided.

## Design Principles

| Principle                                            | Origin               |
| ---------------------------------------------------- | -------------------- |
| Sessions are opt-in; agents are stateless by default | Dive                 |
| Events are the unit of persistence, not raw messages | ADK, Codex           |
| Append is the hot path; full rewrites are rare       | Codex                |
| Storage is behind an interface                       | All three references |
| Agent handles session load/save internally           | ADK                  |
| Deep-copy on read prevents mutation of stored data   | ADK                  |

## Core Interface

The `dive.Session` interface lives in the core package:

```go
type Session interface {
    ID() string
    Messages(ctx context.Context) ([]*llm.Message, error)
    SaveTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error
}
```

This is the only session concept the agent knows about. The `session` package
provides concrete implementations.

## Agent Integration

Sessions integrate directly into `AgentOptions`:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Model:   anthropic.New(),
    Session: sess,
})
```

The agent handles session load/save internally in `CreateResponse`:

1. **Before PreGeneration hooks**: `sess.Messages(ctx)` loads history, prepended to input.
2. **After PostGeneration hooks**: `sess.SaveTurn(ctx, turnMessages, usage)` persists the turn.

Per-call override via `WithSession(sess)` takes priority over `AgentOptions.Session`.

Flow: SessionLoad → PreGeneration → [Generation Loop] → PostGeneration → SessionSave

## Session Package

### Session struct

The `session.Session` struct implements `dive.Session`. Internally it tracks
an ordered sequence of events (one per `CreateResponse` call). Events are
unexported — the public interface deals only in messages.

```go
sess := session.New("my-session")           // in-memory
sess, _ := store.Open(ctx, "my-session")    // store-backed
```

Key methods beyond the interface:
- `Title() string` / `SetTitle(string)`
- `EventCount() int`
- `TotalUsage() *llm.Usage`
- `Fork(newID string) *Session`
- `Compact(ctx, summarizer) error`
- `AllMessages(ctx) ([]*llm.Message, error)` — full uncompacted transcript
- `CompactionHistory(ctx) ([]CompactionRecord, error)` — one record per checkpoint

### Event (internal)

Each `CreateResponse` call produces one event containing the turn delta:

```go
type event struct {
    ID        string
    Type      eventType        // "turn" or "compaction"
    Timestamp time.Time
    Messages  []*llm.Message
    Usage     *llm.Usage
    Metadata  map[string]any
}
```

Events are unexported. `Messages()` reconstructs the **active window** — the
most recent compaction summary plus everything after it — which is what the
agent sends to the model. `AllMessages()` reconstructs the **full transcript**
from every event, including the originals that compaction replaced.

### Store Interface

```go
type Store interface {
    Open(ctx context.Context, id string) (*Session, error)
    Put(ctx context.Context, sess *Session) error
    List(ctx context.Context, opts *ListOptions) (*ListResult, error)
    Delete(ctx context.Context, id string) error
}
```

- `Open` loads an existing session or creates a new one. The returned session
  is connected to the store: `SaveTurn` calls persist automatically.
- `Put` saves a session (used after Fork to persist the forked copy).

### Store Implementations

**MemoryStore**: In-memory with `sync.RWMutex`. Session data is shared
directly between the store and session (no deep-copy needed since the
session manages its own locking).

**FileStore**: JSONL-based, inspired by Codex. Each session is a file:

```
{dir}/{session_id}.jsonl
```

Line 1 is a session metadata header. Subsequent lines are events.

```jsonl
{"line_type":"header","data":{"id":"sess-123","title":"...","created_at":"..."}}
{"line_type":"event","data":{"type":"turn","id":"evt-1","timestamp":"...","messages":[...]}}
```

- `SaveTurn` → `appendEvent`: opens file in append mode, writes one JSON line.
- `Open` (existing): reads all lines, reconstructs session data.
- `Put`: rewrites the entire file (rare: fork, compact).

## Forking

Fork is a method on Session:

```go
forked := original.Fork("new-branch")
store.Put(ctx, forked)  // persist to store
```

The `ForkSession(ctx, store, fromID, newID)` utility combines open + fork + put.

## Compaction

Compaction is **non-destructive**: it appends a compaction checkpoint rather
than deleting history.

```go
sess.Compact(ctx, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
    // Summarize the active window using an LLM or custom logic
    return summarized, nil
})
```

`Compact` summarizes the current active window and appends a `compaction`
event whose summary becomes the new active window. The original turn events
stay in the log. From there:

- `Messages()` returns the summary plus any turns after it — fewer tokens to
  the model.
- `AllMessages()` returns every original message, still intact.
- `CompactionHistory()` returns one `CompactionRecord` per checkpoint
  (`Summary`, `ReplacedMessages`, `CompactedAt`), in order.

Compaction does **not** special-case the most recent turns — the whole active
window is summarized uniformly. Keeping recent exchanges in high detail is a
summarizer-prompt concern (content policy in the `CompactFunc`), not a
structural carve-out in the session: an oversized recent turn is summarized
like any other and its bytes stay recoverable via `AllMessages`.

The trade-off is storage: a compacted session is larger on disk than the
summary alone, because the originals are retained. That is deliberate —
summarization is lossy compression, and keeping the originals is what makes
**post-compaction retrieval** possible (recovering the specific tool results,
file contents, and rationale a summary flattens). See
[`session-codex.md`](../reference/session-codex.md) for the retrieval-centric
model this follows.

If the session is store-backed, the compacted state is persisted via
`putSession`.

## Usage Example

```go
// In-memory (simplest)
sess := session.New("my-session")
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        anthropic.New(),
    Session:      sess,
})

resp, _ := agent.CreateResponse(ctx, dive.WithInput("Hello"))
resp, _ = agent.CreateResponse(ctx, dive.WithInput("Tell me more"))

// Persistent
store, _ := session.NewFileStore("~/.myapp/sessions")
sess, _ := store.Open(ctx, "my-session")

agent, _ := dive.NewAgent(dive.AgentOptions{
    Model:   anthropic.New(),
    Session: sess,
})

// Per-call override (one agent, many sessions)
resp, _ := agent.CreateResponse(ctx,
    dive.WithInput("Hello"),
    dive.WithSession(userSession),
)
```

## Migration from Previous Design

| Previous                          | Current                                                       |
| --------------------------------- | ------------------------------------------------------------- |
| Hook-based session load/save      | Agent handles session internally                              |
| `session.WithID("id")` per call   | `AgentOptions{Session: sess}` or `WithSession(sess)` per call |
| Exported `Event`, `EventType`     | Unexported `event`, `eventType`                               |
| `Store.GetSession` / `PutSession` | `Store.Open` / `Put`                                          |
| `Store.AppendEvent`               | Internal `eventAppender` interface                            |
| `session.Hooks(store)` returns hooks | Not needed — agent handles it                              |
| `session.Loader` / `session.Saver` | Not needed — agent handles it                               |

## Ideas Considered and Not Adopted

| Idea                                                   | Source | Reason                                                     |
| ------------------------------------------------------ | ------ | ---------------------------------------------------------- |
| Mandatory sessions                                     | ADK    | Dive is library-first; stateless is the default            |
| State scoping with prefixes (`app:`, `user:`, `temp:`) | ADK    | Over-engineered; Metadata suffices                         |
| Event-sourced state deltas                             | ADK    | Too complex; simple Metadata key-value is enough           |
| SQLite index alongside JSONL                           | Codex  | Useful for CLI listing but overkill for a library          |
| Composite key (AppName, UserID, SessionID)             | ADK    | Simple string ID is sufficient                             |
| Async writer with channel                              | Codex  | Adds complexity; synchronous writes are fine for a library |
| Branch field for multi-agent                           | ADK    | Can be added later when sub-agent sessions need it         |
| Hook-based session integration                         | v1     | Direct agent integration is simpler and more discoverable  |
