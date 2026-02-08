# ADK-Go Session Implementation Reference

Reference documentation for the session system in Google's [Agent Development Kit (ADK) for Go](https://github.com/google/adk-go). This captures patterns and design decisions worth borrowing for Dive.

## Architecture Overview

The session system has three layers:

1. **Core interfaces** (`session` package) - `Session`, `State`, `Events`, `Event`
2. **Service interface** (`session.Service`) - CRUD + event appending
3. **Storage backends** - InMemory, Database (GORM), VertexAI

The Runner owns the interaction loop. It fetches sessions from the service, wraps them in a `MutableSession`, passes them to agents, and calls `AppendEvent` after each interaction.

## Core Interfaces

### Session

```go
type Session interface {
    ID() string
    AppName() string
    UserID() string
    State() State
    Events() Events
    LastUpdateTime() time.Time
}
```

Sessions are identified by a composite key: `(AppName, UserID, SessionID)`. This enables multi-tenant isolation at the app level, per-user session management, and global app-level state via prefixed keys.

### State

A key-value store with three scope levels, distinguished by key prefix:

```go
type State interface {
    Get(string) (any, error)
    Set(string, any) error
    All() iter.Seq2[string, any]
}

type ReadonlyState interface {
    Get(string) (any, error)
    All() iter.Seq2[string, any]
}
```

**Scope prefixes:**

| Prefix   | Scope       | Description                                              |
| -------- | ----------- | -------------------------------------------------------- |
| `app:`   | Application | Shared across all users and sessions for the app         |
| `user:`  | User        | Shared across all sessions for a given user within an app |
| `temp:`  | Invocation  | Exists only for the current invocation, then discarded   |
| *(none)* | Session     | Specific to this conversation thread                     |

State is stored flat in a single map, but on persistence the prefixes are stripped and values are routed to separate storage (app state table, user state table, session state). On retrieval, the three scopes are merged back together with prefixes re-applied so callers see a unified view.

### Events

```go
type Events interface {
    All() iter.Seq[*Event]
    Len() int
    At(i int) *Event
}
```

Uses Go 1.22+ `iter.Seq` for range-over-function iteration. The concrete type is just a named slice (`type events []*Event`).

### Event

```go
type Event struct {
    model.LLMResponse  // Embedded: Content, Partial, UsageMetadata, etc.

    ID               string
    Timestamp        time.Time
    InvocationID     string
    Branch           string      // "agent_1.agent_2.agent_3"
    Author           string      // "user", agent name, etc.
    Actions          EventActions
    LongRunningToolIDs []string
}

type EventActions struct {
    StateDelta                 map[string]any
    ArtifactDelta              map[string]int64
    RequestedToolConfirmations map[string]toolconfirmation.ToolConfirmation
    SkipSummarization          bool
    TransferToAgent            string
    Escalate                   bool
}
```

Events are the unit of persistence. Each event captures a single interaction (user message, model response, function call/response) along with side effects like state changes, artifact updates, and control flow signals (transfer, escalate).

`IsFinalResponse()` determines whether an event represents a terminal response from an agent (no pending function calls, not partial, not a code execution result needing follow-up).

The `Branch` field (format: `agent_1.agent_2.agent_3`) enables sub-agents to have isolated conversation histories invisible to peer agents.

## Service Interface

```go
type Service interface {
    Create(context.Context, *CreateRequest) (*CreateResponse, error)
    Get(context.Context, *GetRequest) (*GetResponse, error)
    List(context.Context, *ListRequest) (*ListResponse, error)
    Delete(context.Context, *DeleteRequest) error
    AppendEvent(context.Context, Session, *Event) error
}
```

Key design choices:

- **`AppendEvent` is the write path.** State is never written directly. It flows through event state deltas. This gives you an event-sourced audit trail of all state changes.
- **Partial events are silently ignored.** `AppendEvent` returns `nil` immediately for streaming partial events (`event.Partial == true`), so callers don't need to filter.
- **Temp state is stripped on persist.** Before saving, `trimTempDeltaState` removes all `temp:`-prefixed keys from the event's `StateDelta`. Temp state is applied to the local session object during the invocation but never reaches storage.
- **Responses return copies.** The in-memory service clones state maps and event slices before returning, preventing external mutation of stored data.

### Request Types

```go
type CreateRequest struct {
    AppName   string
    UserID    string
    SessionID string         // Optional; auto-generated UUID if empty
    State     map[string]any // Initial state
}

type GetRequest struct {
    AppName         string
    UserID          string
    SessionID       string
    NumRecentEvents int       // Return at most N most recent events (0 = all)
    After           time.Time // Return events with timestamp >= this (zero = no filter)
}

type ListRequest struct {
    AppName string
    UserID  string
}

type DeleteRequest struct {
    AppName   string
    UserID    string
    SessionID string
}
```

`GetRequest` supports two event filters that compose: `NumRecentEvents` limits the count and `After` filters by timestamp. The in-memory implementation applies `NumRecentEvents` first (tail of sorted list), then timestamp filter. The database implementation applies both via SQL.

## Storage Backends

### InMemoryService

```go
func InMemoryService() Service
```

Uses `rsc.io/omap` (ordered map) keyed by composite encoded keys via `rsc.io/ordered`. This enables efficient range scanning for `List` operations using `omap.Scan(lo, hi)` rather than iterating all sessions.

State is separated into three maps:
- `sessions` - ordered map of session data
- `appState` - `map[appName]stateMap`
- `userState` - `map[appName]map[userID]stateMap`

Thread-safe via `sync.RWMutex`. Suitable for testing and development only.

### DatabaseService

```go
func NewSessionService(dialector gorm.Dialector, opts ...gorm.Option) (session.Service, error)
func AutoMigrate(service session.Service) error
```

Uses GORM with four storage models:

| Model               | Primary Key                        | Purpose                |
| ------------------- | ---------------------------------- | ---------------------- |
| `storageSession`    | `(AppName, UserID, ID)`            | Session metadata + state |
| `storageEvent`      | `(ID, AppName, UserID, SessionID)` | Event history          |
| `storageAppState`   | `(AppName)`                        | App-scoped state       |
| `storageUserState`  | `(AppName, UserID)`                | User-scoped state      |

**Stale session detection:** On `AppendEvent`, the service compares the session's `UpdateTime` (from when the caller fetched it) against the database's current `UpdateTime`. If the database is newer, it returns a stale session error. This prevents lost updates from concurrent modifications.

**Transactions:** `AppendEvent` wraps the entire operation (fetch, validate, update state, insert event, update timestamp) in a single GORM transaction.

**Timestamp precision:** Event timestamps are truncated to microsecond precision to match database column precision and prevent rounding errors.

Custom GORM types handle JSON serialization for state maps and event content, with support for JSONB (Postgres), LONGTEXT (MySQL), and STRING(MAX) (Spanner).

### VertexAI Service

```go
func NewSessionService(ctx context.Context, cfg VertexAIServiceConfig, opts ...option.ClientOption) (session.Service, error)

type VertexAIServiceConfig struct {
    ProjectID       string
    Location        string
    ReasoningEngine string
}
```

Delegates to Google Cloud Vertex AI APIs. Does not support client-provided session IDs (always auto-generated). Uses `errgroup` for parallel session and event fetching.

## State Delta Flow

This is the most interesting design pattern. Here's the full flow:

1. Agent calls `session.State().Set("key", value)` during processing
2. The `MutableSession` wrapper delegates to the underlying session's `State.Set()`
3. The Runner collects state changes into `event.Actions.StateDelta`
4. Runner calls `service.AppendEvent(ctx, session, event)`
5. Inside `AppendEvent`:
   a. Local session state is updated with all deltas (including `temp:` keys)
   b. `trimTempDeltaState` removes `temp:` prefixed keys from the event
   c. `extractStateDeltas` splits remaining deltas by prefix into app/user/session buckets
   d. Each bucket is persisted to its respective storage
   e. The event (with trimmed deltas) is persisted

This means:
- **Temp state is visible during the invocation** (step 5a applies it locally) but **never persisted** (step 5b strips it)
- **All persistent state changes have an audit trail** via the event's `StateDelta`
- **State scoping is transparent to agents** - they just use prefixed keys

## MutableSession Wrapper

```go
// internal/sessioninternal/mutablesession.go
type MutableSession struct {
    service       session.Service
    storedSession session.Session
}
```

The Runner wraps the session from `Service.Get()` in a `MutableSession` before passing it to agents. This wrapper:
- Implements both `session.Session` and `session.State`
- Delegates all reads to the stored session
- For `Set()`, checks that the underlying state implements `MutableState` (type assertion)
- `State()` returns itself, so agents interact with a single object for both session metadata and state

## Patterns Worth Borrowing

### Event-sourced state changes
State is never written directly to storage. It always flows through events, giving you a complete audit trail. The `StateDelta` on each event records exactly what changed and when.

### Prefix-based state scoping
A flat key-value map with prefixes (`app:`, `user:`, `temp:`) is simpler than separate state objects for each scope. The routing is handled transparently by the service layer. Agents don't need to know about scope boundaries.

### Temp state for invocation-scoped scratch data
The `temp:` prefix lets agents store working data that's visible during processing but automatically discarded. No manual cleanup needed.

### Stale session detection
Comparing timestamps before writing prevents silent data loss from concurrent modifications. Simple but effective optimistic concurrency control.

### Copy-on-read
Returning cloned state maps and event slices from service methods prevents callers from accidentally mutating stored data. This is especially important for the in-memory implementation where the storage is shared mutable state.

### Composite key encoding for range scans
Using `rsc.io/ordered` to encode `(AppName, UserID, SessionID)` into a single comparable key enables efficient prefix-based range scanning in ordered maps. This avoids the need for secondary indexes.

### Streaming-aware persistence
Partial events (streaming tokens) are silently dropped by `AppendEvent`. This keeps the event log clean while still allowing streaming at the transport layer.

## Package Structure

```
session/
    session.go              # Core interfaces: Session, State, Events, Event
    service.go              # Service interface and request/response types
    inmemory.go             # In-memory implementation
    database/
        service.go          # GORM-based database implementation
        session.go          # localSession (mirrors inMemory session struct)
        storage_session.go  # GORM models
        gorm_datatypes.go   # Custom GORM type serializers
    vertexai/
        vertexai.go         # Vertex AI service wrapper
        session.go          # localSession for Vertex AI
        vertexai_client.go  # Vertex AI API client

internal/
    sessioninternal/
        session.go          # MutableState interface
        mutablesession.go   # MutableSession (Runner's wrapper)
    sessionutils/
        utils.go            # ExtractStateDeltas, MergeStates helpers
```

## Things to Be Aware Of

- Each storage backend has its own concrete session type (`*session`, `*localSession`) and `AppendEvent` does a type assertion to verify the session came from that service. This means you can't mix service instances.
- The database implementation duplicates `extractStateDeltas` and `mergeStates` rather than using the shared `sessionutils` package (likely an oversight or intentional to avoid import cycles).
- There's a TODO in the code noting that `localSession` in the database package is identical to the in-memory `session` struct and should be moved to `sessioninternal`.
- The `Events` interface returns `*Event` pointers, which means callers could mutate events after retrieval. The in-memory implementation clones the slice but not the individual events.
