# ADK for Go (google.golang.org/adk/v2): source analysis for Dive

- **Repo:** `github.com/google/adk-go`, checked out at `~/git/lib/adk-go`
- **Commit analyzed:** `12f7cabca5098141811258e3613b1996b2f50f71` (2026-09-25, "feat(auth/gcp): add GCP credential provider (#1173)")
- **Module:** `google.golang.org/adk/v2`, `go 1.26.6` (`go.mod:1-3`)
- **Size:** 660 Go files. The non-test code is roughly 77k lines, and `internal/` alone is about 19.6k of them.

Every path below is relative to the adk-go repo root. The Go definitions are quoted from source, with comments trimmed where marked.

---

## 1. Overview

ADK-Go is Google's Agent Development Kit for Go. It is explicitly a **port of adk-python**. Almost every non-trivial function carries a "Mirrors adk-python …" comment, and some decisions are justified only by parity. `runner/run_node.go:127-133`, for example, says a wrapper exists "ensuring 100% path parity with Python reference recordings". Wire-format names such as `adk_request_confirmation` and `adk_request_input` are shared across the Python, Java and Go runtimes (`workflow/request_input.go:25-36`).

Four design commitments shape everything else.

1. **The session event log is the source of truth.** An agent never holds a transcript in memory. Each LLM step rebuilds its prompt from `ctx.Session().Events()` (`internal/llminternal/contents_processor.go:51-52`). Human-in-the-loop resume state is reconstructed by scanning history rather than loading a snapshot (`workflow/persistence.go:57-80`).
2. **`iter.Seq2[*session.Event, error]` is the universal execution type.** Agents, flows, workflow nodes, the runner, and `model.LLM.GenerateContent` (as `iter.Seq2[*LLMResponse, error]`) all use it.
3. **`google.golang.org/genai` types are the lingua franca.** `genai.Content`, `genai.Part`, `genai.FunctionDeclaration`, `genai.GenerateContentConfig` and `genai.Schema` appear in the public API of agents, tools, models, sessions, memory and artifacts.
4. **Side effects are declarative data on events.** State deltas, artifact version bumps, agent transfer, escalation, tool-confirmation requests and compaction records all travel as `session.EventActions` on the event that caused them. The session service applies them atomically with the append.

v2 layers a **graph workflow engine** (`workflow/`) on top of the original agent tree (LLM agents plus Sequential, Parallel and Loop agents). Since v2, the runner drives any LLM root agent through a synthetic single-node workflow (`runner/runner.go:570-647`, `runner/run_node.go:127-137`). Two orchestration models coexist as a result.

### Repo layout (non-test Go files / lines)

| Dir | Files / LOC | Role |
|---|---|---|
| `agent/` | 24 / 6.8k | `Agent` interface, contexts, callbacks, `llmagent`, `workflowagents/{sequential,parallel,loop}agent`, `workflowagent`, `remoteagent` (A2A) |
| `runner/` | 3 / 2.0k | `Runner`: session I/O, plugin lifecycle, persistence, compaction, HITL resume routing |
| `workflow/` | 24 / 6.2k | Graph engine: `Node`, `Edge`, scheduler, retries, HITL, dynamic nodes, join, parallel worker |
| `model/` | 12 / 3.2k | `LLM` interface, name registry, `gemini`, `openaimodel` (experimental), `apigee` |
| `session/` | 13 / 5.9k | `Session`, `Event`, `EventActions`, `Service` plus in-memory, GORM `database`, `vertexai`; `compaction`; `sessiontestsuite` |
| `tool/` | 26 / 4.1k | `Tool` and `Toolset`, `functiontool`, `mcptoolset`, `agenttool`, `toolconfirmation`, `geminitool`, `exitlooptool`, `skilltoolset`, memory and artifact tools |
| `memory/`, `artifact/` | 8 / 1.8k | Cross-session memory service; versioned blob service (in-memory, GCS) |
| `plugin/` | 9 / 2.1k | Global callback bundles: logging, analytics, retry-and-reflect, function-call modifier |
| `platform/` | 4 / 0.2k | Seams for time, UUID and concurrency (`WithTimeProvider`, `WithUUIDProvider`, `WithTaskRunner`) |
| `internal/` | 90 / 19.6k | The real engine: `llminternal` (the LLM flow, 1.7k-line `base_flow.go`), contexts, converters, telemetry |
| `server/` | 76 / 11k | REST (`adkrest`), A2A (`adka2a`), Vertex Agent Engine |
| `cmd/` | 21 / 3.7k | Launchers (console, web, prod), the `adkgo` CLI |
| `telemetry/`, `auth/`, `agentregistry/` | | OTel setup, credential providers, agent registry |

---

## 2. Core packages and Go types

### `agent`: the agent abstraction and the contexts

`agent/agent.go:45-54`:

```go
type Agent interface {
	Name() string
	Description() string
	Run(InvocationContext) iter.Seq2[*session.Event, error]
	SubAgents() []Agent
	FindAgent(name string) Agent
	FindSubAgent(name string) Agent

	internal() *agent
}
```

The unexported `internal()` method **seals the interface**. You cannot implement `Agent` yourself, and the comment admits this: "NOTE: in future releases we will allow just implementing this interface. For now agent.New is a correct solution to create custom agents." A custom agent is written as a closure passed to `agent.New` (`agent/agent.go:79-106`):

```go
type Config struct {
	Name        string
	Description string
	SubAgents   []Agent
	BeforeAgentCallbacks []BeforeAgentCallback
	Run func(InvocationContext) iter.Seq2[*session.Event, error]
	AfterAgentCallbacks []AfterAgentCallback
}
type BeforeAgentCallback func(Context) (*genai.Content, error)
type AfterAgentCallback func(Context) (*genai.Content, error)
```

The invocation context (`agent/context.go:63-119`, abridged) documents the invocation, agent-call and step hierarchy in an ASCII diagram at lines 28-62:

```go
type InvocationContext interface {
	context.Context
	Agent() Agent
	Artifacts() Artifacts
	Memory() Memory
	Session() session.Session
	InvocationID() string
	Branch() string          // "agent_1.agent_2.agent_3": history visibility for parallel peers
	IsolationScope() string  // exact-match history filter (task agents)
	UserContent() *genai.Content
	RunConfig() *RunConfig
	EndInvocation()
	Ended() bool
	ResumedInput(interruptID string) (any, bool)
	// "NOTE: This is a temporary solution and will be removed later. The proper solution
	// we plan is to stop embedding go context in adk context types and split it."
	WithContext(ctx context.Context) InvocationContext
	WithICDelta(d *InvocationContextDelta) InvocationContext
}
```

`agent.Context` (`agent/context.go:142-240`) is the **unified context** handed to every callback, tool and workflow node. It embeds `ReadonlyContext` and `InvocationContext` and adds about 17 more methods, for a total of roughly 30:

```go
type Context interface {
	ReadonlyContext
	InvocationContext
	Artifacts() Artifacts
	State() session.State
	FunctionCallID() string
	Actions() *session.EventActions          // mutable side-effects for the current event
	SearchMemory(ctx context.Context, query string) (*memory.SearchResponse, error)
	ToolConfirmation() *toolconfirmation.ToolConfirmation
	RequestConfirmation(hint string, payload any) error
	ResumedInput(interruptID string) (any, bool)
	Path() string                            // workflow node path
	RunID() string
	SubScheduler() DynamicSubScheduler
	WithAgentContext(ctx context.Context) Context
	WithAgentTimeout(timeout time.Duration) (Context, context.CancelFunc)
	WithAgentCancel() (Context, context.CancelFunc)
	OutputForAncestors() []string
	WithDelta(d *CommonContextDelta) Context
}
```

v2 merged the former `ToolContext` and `CallbackContext` into this type. The change broke every user mock, so `README-v2.md` recommends embedding `agent.StrictContextMock`. `RunConfig` is tiny (`agent/run_config.go`): `StreamingMode` (`"none"` or `"sse"`) and `SaveInputBlobsAsArtifacts`.

### `agent/llmagent`: the LLM agent

`llmagent.Config` (`agent/llmagent/llmagent.go:184-353`) is a flat struct with about 25 fields: `Model model.LLM`, `Instruction` (a template with `{state_key}`, `{artifact.name}` and `{key?}`), `InstructionProvider`, `GlobalInstruction`, `GenerateContentConfig *genai.GenerateContentConfig`, `Tools []tool.Tool`, `Toolsets []tool.Toolset`, `SubAgents`, `DisallowTransferToParent/Peers`, `IncludeContents`, `InputSchema` and `OutputSchema` (`*genai.Schema`), `OutputKey`, `Mode` (`ModeChat`, `ModeTask` or `ModeSingleTurn`), and **eight callback slices**. The callback types are at lines 374-413:

```go
type BeforeModelCallback func(ctx agent.Context, llmRequest *model.LLMRequest) (*model.LLMResponse, error)
type AfterModelCallback func(ctx agent.Context, llmResponse *model.LLMResponse, llmResponseError error) (*model.LLMResponse, error)
type OnModelErrorCallback func(ctx agent.Context, llmRequest *model.LLMRequest, llmResponseError error) (*model.LLMResponse, error)
type BeforeToolCallback func(ctx agent.Context, tool tool.Tool, args map[string]any) (map[string]any, error)
type AfterToolCallback func(ctx agent.Context, tool tool.Tool, args, result map[string]any, err error) (map[string]any, error)
type OnToolErrorCallback func(ctx agent.Context, tool tool.Tool, args map[string]any, err error) (map[string]any, error)
```

All callbacks use first-non-nil-wins short-circuit semantics. When a callback returns a non-nil result or error, the rest are skipped and that value replaces the model call or tool call.

### `model`: the provider abstraction

`model/llm.go:26-60`:

```go
type LLM interface {
	Name() string
	GenerateContent(ctx context.Context, req *LLMRequest, stream bool) iter.Seq2[*LLMResponse, error]
}

type LLMRequest struct {
	Model    string
	Contents []*genai.Content
	Config   *genai.GenerateContentConfig
	Tools map[string]any `json:"-"`   // name -> tool.Tool, used by the flow for dispatch
}

type LLMResponse struct {
	Content             *genai.Content
	CitationMetadata    *genai.CitationMetadata
	GroundingMetadata   *genai.GroundingMetadata
	UsageMetadata       *genai.GenerateContentResponseUsageMetadata
	CustomMetadata      map[string]any
	LogprobsResult      *genai.LogprobsResult
	InputTranscription  *genai.Transcription
	OutputTranscription *genai.Transcription
	ModelVersion        string
	Partial bool          // streaming chunk; forwarded but never persisted
	TurnComplete bool
	Interrupted bool
	SessionResumptionHandle string
	ErrorCode, ErrorMessage string
	FinishReason genai.FinishReason
	AvgLogprobs  float64
}
```

The interface is small and well chosen, but every field is a Gemini type. `model/registry.go` adds an opt-in regex registry: `model.Register("^(?i)gemini-.*", factory)` and `model.NewLLM(ctx, name)`. Exactly one pattern must match, so the result never depends on import order.

The providers are `model/gemini` (thin, and reuses `llminternal.NewStreamingResponseAggregator`) and `model/openaimodel`. The OpenAI package is **EXPERIMENTAL** and targets the Responses API. Its `doc.go:24-52` lists which `GenerateContentConfig` fields are translated and which are rejected. It also admits that reasoning is not round-tripped ("ADK does not carry those ids") and that strict tool-schema validation is disabled. There is **no Anthropic provider** in-tree.

### `session`: the event log, state and persistence

`session/session.go:40-99`:

```go
type Session interface {
	ID() string
	AppName() string
	UserID() string
	State() State
	Events() Events
	LastUpdateTime() time.Time
}
type State interface {
	Get(string) (any, error)
	Set(string, any) error
	All() iter.Seq2[string, any]
}
type Events interface {
	All() iter.Seq[*Event]
	Len() int
	At(i int) *Event
}
```

`Event` (`session/session.go:100-144`) embeds `model.LLMResponse`, so every event is shaped like a model response:

```go
type Event struct {
	model.LLMResponse
	ID        string
	Timestamp time.Time
	InvocationID string
	Branch string
	IsolationScope string
	Author string                 // agent name or "user"
	Actions EventActions
	LongRunningToolIDs []string   // HITL pause marker
	Routes []string               // workflow routing
	RequestedInput *RequestInput  // workflow HITL prompt
	Output any                    // workflow node output
	NodeInfo *NodeInfo
}

type EventActions struct {           // session/session.go:249
	StateDelta map[string]any
	ArtifactDelta map[string]int64
	RequestedToolConfirmations map[string]toolconfirmation.ToolConfirmation
	SkipSummarization bool
	TransferToAgent string
	Escalate bool
	Compaction *EventCompaction    // framework-only; cleared if a tool/callback sets it
}
```

`Service` (`session/service.go:46-77`) has `Create`, `Get`, `List`, `Delete` and `AppendEvent(ctx, Session, *Event) error`. The doc comment on `AppendEvent` states obligations enforced by the shared conformance suite in `session/sessiontestsuite`: assign IDs to ID-less events, and round-trip `Compaction`. State keys are scoped by prefix: `app:`, `user:` and `temp:` (`session/session.go:417-428`). `temp:` keys are trimmed before persistence (`session/inmemory.go:233`).

### `tool`: tools and toolsets

`tool/tool.go:39-58`:

```go
type Tool interface {
	Name() string
	Description() string
	IsLongRunning() bool
}
type Toolset interface {
	Name() string
	Tools(ctx agent.ReadonlyContext) ([]Tool, error)
}
```

The public `Tool` interface is **not what the runtime needs**. Execution dispatches on interfaces in an internal package (`internal/toolinternal/tool.go:28-49`):

```go
type FunctionTool interface {
	tool.Tool
	Declaration() *genai.FunctionDeclaration
	Run(ctx agent.Context, args any) (result map[string]any, err error)
}
type StreamingFunctionTool interface {
	tool.Tool
	Declaration() *genai.FunctionDeclaration
	RunStream(ctx agent.Context, args any) iter.Seq2[string, error]
}
type RequestProcessor interface {
	ProcessRequest(ctx agent.Context, req *model.LLMRequest) error
}
type ResponseDeferrer interface { DefersResponse() bool }
```

Every tool must also implement `ProcessRequest`, or the step fails with `tool %q does not implement RequestProcessor() method` (`internal/llminternal/base_flow.go:905-909`). Go's structural typing means you *can* satisfy these interfaces without importing the internal package, but they are undocumented in the public API. `tool/tool.go:230-246` redeclares the same shapes privately to avoid an import cycle.

`functiontool` (`tool/functiontool/function.go:38-80`):

```go
type Func[TArgs, TResults any] func(agent.Context, TArgs) (TResults, error)
func New[TArgs, TResults any](cfg Config, handler Func[TArgs, TResults]) (tool.Tool, error)

type Config struct {
	Name, Description string
	InputSchema  *jsonschema.Schema  // inferred from TArgs if nil
	OutputSchema *jsonschema.Schema  // inferred from TResults if nil
	IsLongRunning bool
	RequireConfirmation bool
	RequireConfirmationProvider any  // must be func(TArgs) bool — checked at runtime
}
```

### `workflow`: the graph engine

`workflow/workflow.go:35-129`:

```go
type Node interface {
	Name() string
	Description() string
	Config() NodeConfig
	InputSchema() *jsonschema.Resolved
	OutputSchema() *jsonschema.Resolved
	ValidateInput(input any) (any, error)
	ValidateOutput(output any) (any, error)
	Run(ctx agent.Context, input any) iter.Seq2[*session.Event, error]
}
type Route interface { Matches(event *session.Event) bool }   // StringRoute, IntRoute, BoolRoute, MultiRoute[T], Default
type Edge struct { From, To Node; Route Route }
var Start Node
func New(name string, edges []Edge, opts ...Option) (*Workflow, error)   // WithMaxConcurrency, WithStateSchema
```

`NodeConfig` (`workflow/config.go:55-99`) holds `ParallelWorker`, `RerunOnResume *bool`, `WaitForOutput *bool`, `RetryConfig *RetryConfig`, `Timeout` and `EmitsOwnSpan`. `RetryConfig` (`config.go:110-133`) holds `MaxAttempts`, `InitialDelay`, `MaxDelay`, `BackoffFactor`, `Jitter` and `ShouldRetry func(error) bool`.

The node constructors are `NewFunctionNode[IN,OUT]`, `NewEmittingFunctionNode`, `NewFunctionNodeFromState`, `NewAgentNode`, `NewToolNode`, `NewJoinNode`, `NewParallelWorker`, `NewWorkflowNode` (nesting) and `NewDynamicNode`. The dynamic node body is imperative Go (`workflow/dynamic_node.go:34`):

```go
type DynamicFn[IN, OUT any] = func(ctx agent.Context, in IN, emit func(*session.Event) error) (OUT, error)
func RunNode[OUT any](ctx agent.Context, child Node, input any, opts ...RunNodeOption) (OUT, error)  // run_node.go:126
```

Run state (`workflow/state.go:25-189`) uses `NodeStatus` values `NodeInactive`, `NodePending`, `NodeRunning`, `NodeCompleted`, `NodeWaiting`, `NodeFailed` and `NodeCancelled`. `NodeState{Status, Input, Output, TriggeredBy, Branch, Interrupts, Attempt, ResumedInputs}` is stored in `RunState{Nodes map[string]*NodeState}`.

The HITL prompt type is `session.RequestInput` (`session/session.go:184-216`):

```go
type RequestInput struct {
	InterruptID    string
	Message        string
	ResponseSchema *jsonschema.Schema
	Payload        any
}
```

### Services, plugins and platform

- **`artifact.Service`** (`artifact/service.go:32-51`): `Save`, `Load`, `Delete`, `List`, `Versions` and `GetArtifactVersion`. Artifacts are keyed by (app, user, session, filename), are versioned `int64`, and have a `*genai.Part` payload. The agent-facing view is `agent.Artifacts` (`agent/agent.go:109-114`).
- **`memory.Service`** (`memory/service.go:31-40`): `AddSessionToMemory(ctx, session.Session)` and `SearchMemory(ctx, *SearchRequest)`, which returns `[]Entry{ID, Content *genai.Content, Author, Timestamp, CustomMetadata}`. Implementations are in-memory and Vertex AI.
- **`plugin.Config`** (`plugin/plugin.go:30`): a named bundle of `OnUserMessage`, `OnEvent`, `BeforeRun`/`AfterRun`, and before/after hooks for agent, model and tool, plus `OnModelError`, `OnToolError` and `CloseFunc`. Plugins are installed on the `Runner` and run *before* the agent-level callbacks at each seam.
- **`platform`** (`platform/exec.go:36-93`, `time.go`, `uuid.go`): context-carried providers for `Now`, `NewUUID` and `RunTasks`. `type TaskRunner func(ctx context.Context, tasks []func(context.Context))`. `session.NewEvent(ctx, invocationID)` takes `ctx` so that a host can make event IDs and timestamps deterministic (`README-v2.md`, "Breaking changes").

---

## 3. Key concepts and how they relate

```
                   runner.Runner  (AppName, root Agent, SessionService, ArtifactService,
                        |          MemoryService, Plugins, Compaction)
   Run(ctx,user,sess,msg,cfg) -> iter.Seq2[*Event,error]
                        |
         +--------------+-----------------------------+
         | LLM root agent                             | non-LLM root (workflowagent, sequential, ...)
         v                                            v
   synthetic workflow START -> DynamicNode(agent)     rootAgent.Run(ctx)
   (ReconstructRunState / Resume on HITL reply)
         |
   RunLLMAgentAsNode (mode: chat | task | single_turn)
         |
   llminternal.Flow.Run  --loop-->  runOneStep:
         |   request processors (instructions, tools, contents<-session events, compaction,
         |   confirmations, transfer tool, output schema ...)
         |   -> before-model cbs -> model.LLM.GenerateContent (iter) -> after-model cbs
         |   -> yield model Event -> handleFunctionCalls (parallel goroutines)
         |   -> yield merged FunctionResponse Event -> [transfer: run next agent inline]
         v
   every non-partial Event -> plugin OnEvent -> SessionService.AppendEvent (applies
                                                EventActions: state/artifact deltas)
                                              -> yield to caller
```

The concepts compose as follows.

- **Invocation.** One user message produces one `InvocationID`, and a resume turn reuses the paused invocation's ID (`runner/run_node.go:433-448`). An invocation contains one or more **agent calls** (transfers). An agent call contains one or more **steps**, and each step makes exactly one LLM call plus its tool calls (`agent/context.go:28-62`).
- **Agent tree and transfer.** `SubAgents` form a tree. The runner builds a `parentmap` (`runner/runner.go:122`). An LLM agent gets a synthetic `transfer_to_agent` tool whose enum lists legal targets: parent, peers and children, subject to the `Disallow*` flags (`internal/llminternal/agent_transfer.go:99-229`). On the next user turn, `findAgentToRun` (`runner/runner.go:1151-1183`) resumes with the **last agent that authored an event**. Conversation "stickiness" is therefore derived from the log.
- **Workflow agents.** `sequentialagent`, `parallelagent` and `loopagent` wrap `agent.New` with fixed `Run` bodies. The parallel agent gives each sub-agent a distinct `Branch` (`agent/workflowagents/parallelagent/agent.go:81-86`) so that peers don't see each other's history. The loop agent stops on `MaxIterations` or on any event with `Actions.Escalate` (`loopagent/agent.go:75-104`). `exitlooptool` sets `Escalate` and `SkipSummarization`.
- **Graph workflows.** `workflowagent.New(Config{Edges: ...})` wraps a `workflow.Workflow` as an `Agent`. Nodes emit events. One event per activation may carry `Output`, and one may carry `Routes` (`workflow/scheduler.go:37-51`). Edges route on `Routes`, and outputs become successor inputs. LLM agents can be nodes in three modes: `single_turn` (a one-shot function), `task` (multi-turn, finishes via a `finish_task` tool), and `chat`.
- **State.** `ctx.State().Set(k, v)` writes into both the current event's `Actions.StateDelta` and the live session state immediately (`agent/common_context.go:757-774`). The delta becomes durable when the event is appended. `OutputKey` copies an agent's final text into `StateDelta` (`llmagent.go:522-560`). Instructions interpolate state with `{key}`.
- **Artifacts and memory.** These are session-scoped (artifacts) and user-scoped (memory) services, reached through context accessors and a few tools (`loadartifactstool`, `loadmemorytool`, `preloadmemorytool`).
- **Callbacks and plugins.** Callbacks are per-agent and plugins are per-runner. Both use short-circuit semantics. Plugins additionally see run start and end, user messages and every event (`OnEventCallback` can rewrite events before persistence; `runner/runner.go:748-761`).
- **Compaction.** Compaction is non-destructive. A summary event carries an `EventCompaction{StartTimestamp, EndTimestamp, CompactedContent, ExcludedEvents}` record, and the contents processor substitutes the summary for the covered range (`session/session.go:324-373`, `session/compaction/compaction.go`). The runner runs it after each invocation (`runner/runner.go:263`), and a request processor runs it mid-turn.
- **Branch and IsolationScope.** These are two independent history-visibility filters applied when building prompts (`contents_processor.go:142-151`). Events from other agents are rewritten as user-role text beginning "For context:" (`contents_processor.go:703-754`).

---

## 4. Execution, durability, and guarantees

### How a run executes

`Runner.Run` (`runner/runner.go:536-788`):

1. Wraps `yield` so that any error marks `invocationFailed`, which suppresses compaction. Resolves options and calls `getOrCreateSession`.
2. **If the root is an LLM agent** (`runner.go:570`), it requires chat mode. It picks `agentToRun` (the root if any sub-agent is task-mode, otherwise `findAgentToRun`) and calls `r.runNode` (`runner/run_node.go:66-258`). That function:
   - builds `workflow.New(appName+"/"+agent.Name(), [START -> newAgentNode(agent)], WithRootWrapper())` (`run_node.go:133`);
   - builds the invocation context, then `appendMessageToSession` (the user event is persisted first), then the plugin `BeforeRun`;
   - calls `wf.ReconstructRunState(session, invocationID)` and `buildResumeResponses(msg, ...)`. If the message answers a pending interrupt it calls `wf.Resume`, otherwise `wf.Run`;
   - for every event: stamps the author, runs plugin `OnEvent`, calls `AppendEvent` if `!Partial`, then `yield`s.
3. **Otherwise** it runs `rootAgent.Run(ctx)` through the same pipeline: OnEvent, then persist non-partial events, then yield (`runner.go:725-775`).
4. Compaction runs from a `defer` so that it still happens when the consumer `break`s out of the range loop (`runner.go:640-664`).

### The commit point is `yield`

The runner persists an event *inside* the consumer side of `yield`, and producers wait for `yield` to return before continuing. ADK makes that ordering explicit with handshakes in both concurrent executors.

- The workflow scheduler's `runNode` (`workflow/scheduler.go:470-492`) creates a `processed` channel for every non-partial event and blocks on it: "Block on non-partial events until the consumer has persisted them". The consumer closes it after `yield` returns (`scheduler.go:600-605`).
- The parallel agent does the same with `ackChan` (`parallelagent/agent.go:150-172`): "Wait for runner to finish processing before continuing to next iteration".

This guarantees **read-your-writes across steps**. When step N+1 rebuilds its prompt from `ctx.Session().Events()`, step N's function responses are already there. The corollary is important: **`agent.Run` is not usable standalone.** Without a runner appending events, the flow never sees its own tool results in history.

### Concurrency model

- **Tool calls in one model turn run concurrently.** Each call is a task in `platform.RunTasks`, which by default spawns one goroutine per task with no bound (`internal/llminternal/base_flow.go:1316-1491`, `platform/exec.go:68-93`). A host can inject a `TaskRunner` to serialize, bound or externally schedule them. Results go into a slice indexed by call position and are merged into **one** function-response event, **in call order** (`mergeParallelFunctionResponseEvents`, `base_flow.go:1607-1639`). Merged `EventActions` deep-merge `StateDelta`, take the max `ArtifactDelta` version (a deliberate divergence from Python), and OR the boolean flags (`base_flow.go:1641-1703`).
- **The parallel agent** runs sub-agents in an `errgroup` with per-branch contexts and fans events into one channel (`parallelagent/agent.go:70-143`). Event order across branches is arrival order.
- **The workflow scheduler** runs one goroutine per active node as a producer and a single consumer goroutine, the caller's, that owns all state (`workflow/scheduler.go:53-112`). The event queue is bounded at `defaultEventQueueCapacity = 16` (line 35) and `WithMaxConcurrency(n)` caps active nodes. When the consumer `break`s, `cancelAll` runs and the queue is drained before return, so no goroutines leak (`scheduler.go:529-651`).

### Streaming

- `RunConfig.StreamingMode == StreamingModeSSE` makes `callLLM` pass `stream=true` (`base_flow.go:972-975`).
- Providers yield `Partial: true` chunks and then **one aggregated non-partial response**. Gemini uses `streamingResponseAggregator` (`internal/llminternal/stream_aggregator.go`), which tracks text, thought and function-call boundaries and thought signatures.
- The flow wraps each chunk as an event and yields it immediately. Tool dispatch only happens on the non-partial response (`base_flow.go:805-807`).
- The runner **never persists partial events** (`runner.go:764-770`), and `InMemoryService.AppendEvent` ignores them as well (`session/inmemory.go:204`).
- If a step ends on a partial event, meaning the producer never sent the aggregate, the flow logs a warning and returns (`base_flow.go:163-174`).
- Bidirectional "live" (audio/video) uses a separate `RunLive` path with channels and reconnect/backoff. It requires the model to expose `Client() *genai.Client` (`base_flow.go:350-356`), so it only works with Gemini.

### What is persisted, and where

- The only durable record is **events, plus the state deltas carried on them**, stored via `session.Service`. Implementations: `InMemoryService`; `session/database` (GORM: SQLite, Postgres, MySQL); `session/vertexai`.
- `database.applyEvent` (`session/database/service.go:400-470`) uses **one transaction per event**. It loads the session row, runs an optimistic-concurrency check comparing `UpdateTime` microsecond timestamps ("stale session error", line 420), merges the app, user and session deltas, inserts the event, and bumps `UpdateTime`. Staleness is **timestamp-based, not versioned**, so it depends on clocks. The in-memory `localSession` is also mutated *before* the transaction (`service.go:381-383`), so a failed commit leaves the caller's session object diverged from the database.
- Artifacts are stored separately, with no transactional link to the event that references their version (`ArtifactDelta`).
- **No workflow snapshot is persisted.** `workflow.go:223-236` says RunState is persisted "under RunStateSessionKey", but that symbol exists nowhere in the repo. The actual mechanism is `ReconstructRunState`, which rebuilds the paused state by scanning events (`workflow/persistence.go:57-116`). The doc comment is stale.

### Guarantees on crash and restart

- **Committed events survive.** Each is appended before the next step proceeds.
- **There is no mid-turn recovery.** Nothing detects or resumes an interrupted invocation. `resolveInvocationID` (`run_node.go:433-448`) reuses an old invocation ID only if the new message carries a `FunctionResponse` matching a prior call. A crash between the model's `FunctionCall` event and the `FunctionResponse` event leaves a dangling call in history. The next turn starts a fresh invocation, and the contents processor's `dropOrphanedFunctionResponses` / rearrange logic (`contents_processor.go:269-606`) tidies the prompt. The tool may or may not have run, and nothing records which.
- The comment on `NodeRunning` says an entry "that has no live task in the run state (e.g. after a process restart) must be re-scheduled" (`workflow/state.go:40-44`). However, `ReconstructRunState` only rebuilds nodes that have **interrupt** history (`persistence.go:245-266`), so running nodes are not resumed after a crash.
- **Idempotency** exists only for HITL resume. `Resume` counts how many times each interrupt was answered (`resolvedCount`) and gates rescheduling on answers that arrived *this* turn, so a duplicate resume is a no-op (`workflow/persistence.go:37-42`, `workflow/state.go:128-136`, `resume.go:104-116`). Tools have no idempotency key. The function-call ID is the natural candidate, but it is generated client-side with `platform.NewUUID` when the model omits it (`internal/utils/utils.go:37-50`).
- **Determinism seams.** `platform.WithTimeProvider`, `WithUUIDProvider` and `WithTaskRunner` exist so that "workflow engines produce deterministic, replay-safe events" (`README-v2.md`). That is the hook for running ADK inside Temporal-style durable execution. ADK itself ships no such engine.

### Ordering

- Within one agent: model event, then merged function-response event, then an optional confirmation-request event, then an optional structured-output final event (`base_flow.go:797-846`). Function responses are yielded *before* confirmation requests so that completed tool results are persisted even if the client pauses (`base_flow.go:820-832`).
- Across parallel branches or workflow nodes, the order is arrival order, serialized through a single consumer, so the persisted order matches the yielded order.

---

## 5. Tool systems and their Go interfaces

### Definition and schema

- `functiontool.New[TArgs, TResults]` infers the input and output JSON Schemas with `github.com/google/jsonschema-go` (`jsonschema.For[T]`) and resolves them once (`function.go:275-287`). `TArgs` must be a struct, a map, or a pointer to one (`function.go:83-91`). The TODO at line 81 acknowledges that "functions that does not require an argument" are awkward.
- Schemas go into `genai.FunctionDeclaration.ParametersJsonSchema` and `ResponseJsonSchema` (`function.go:161-183`). Long-running tools get an appended description, "NOTE: This is a long-running operation. Do not call this tool again…".
- `toolutils.PackTool` (`tool/toolutils/toolutils.go:40-75`) puts the tool into `req.Tools[name]` for dispatch and its declaration into `req.Config.Tools[].FunctionDeclarations`. Duplicate names are an error.
- Model-side built-ins such as `geminitool.GoogleSearch{}` implement only `ProcessRequest` and add a `genai.Tool{GoogleSearch: ...}`. The flow never executes them.
- Toolsets are expanded **once per agent run**. The `toolProcessor` caches the result in `f.Tools` (`internal/llminternal/tools_processor.go:28-48`), so a dynamic toolset sees the context from the first step only.

### Invocation (`base_flow.go:1299-1605`)

For each `FunctionCall`:

1. Build a per-call `agent.Context` with a fresh `EventActions` and any `ToolConfirmation` for this call ID (`base_flow.go:1326-1330`).
2. **Unknown tool:** call `OnToolError` callbacks with a descriptive error ("LLM hallucinated the function name…", `base_flow.go:1198-1215`). If no callback handles it, the model receives `{"error": msg}`.
3. **`StreamingFunctionTool`:** in non-live mode, chunks are concatenated into `{"result": s}`. In live mode the tool runs in a goroutine that feeds chunks back as user content.
4. **`FunctionTool`:** `callTool` (`base_flow.go:1513-1558`) runs plugin `BeforeTool`, agent `BeforeTool`, `tool.Run`, then (on error) plugin and agent `OnToolError`, then plugin and agent `AfterTool`.
5. **Any error becomes `{"error": err.Error()}`** in the `FunctionResponse`. Tool errors never abort the loop; the model sees them. Error identity (`errors.Is`) is lost at this boundary.
6. A nil result from a long-running tool, or from a `ResponseDeferrer`, means **no function response is emitted** (`base_flow.go:1418-1425`).
7. When `SkipSummarization` is set and the tool implements `SkipSummarizationResultDisplayer` (as `agenttool` does), the result is also attached as a visible text part.

**Results are `map[string]any`.** Non-map results are wrapped as `{"result": v}` (`function.go:228-254`). No multimodal tool results exist. The MCP adapter replaces images and audio with placeholder text such as `[MCP image: …]` (`tool/mcptoolset/tool.go:182-195`).

**Panics.** `functiontool` recovers them and turns them into errors (`function.go:188-192`), and so does the workflow node runner (`scheduler.go:450-457`). The goroutines in `handleFunctionCalls` and `platform.RunTasks` **do not recover**, so a custom `FunctionTool` that panics crashes the process.

### Long-running tools

`IsLongRunning()` puts the call ID into `Event.LongRunningToolIDs` (`findLongRunningFunctionCallIDs`, `base_flow.go:1174-1188`). `IsFinalResponse()` then returns true (`session/session.go:218-231`), so the agent loop ends and the invocation pauses. The client later sends a `FunctionResponse` with the same ID. The runner routes it back (`resolveInvocationID`, `buildResumeResponses`, `openLongRunningCallIDs`; `run_node.go:326-383`) and re-runs the agent node with `RerunOnResume` (`runner/agent_node.go:35-50`). Because history now contains the response, the model continues from it.

### Confirmation (HITL approval)

1. The tool calls `ctx.RequestConfirmation(hint, payload)` and returns `ErrConfirmationRequired` with `SkipSummarization = true` (`tool/tool.go:192-214`).
2. The flow persists the error function response, then yields a synthetic `adk_request_confirmation` `FunctionCall` event that wraps `originalFunctionCall` and `toolConfirmation` and is marked long-running (`internal/llminternal/functions.go:32-90`).
3. The client replies with a `FunctionResponse{Name:"adk_request_confirmation", Response:{"confirmed":bool,...}}`. Three payload shapes are accepted, including a JSON string under `"response"` for the web UI (`request_confirmation_processor.go:58-100`).
4. On the next step, `RequestConfirmationRequestProcessor` finds that reply, re-executes the **original** call through `handleFunctionCalls` with `ToolConfirmation` set, and yields the result (`request_confirmation_processor.go:251-256`). The tool sees `ctx.ToolConfirmation().Confirmed`.

The gating logic is implemented **three times**: `tool.WithConfirmation` (the toolset wrapper), `functiontool.Run` (`function.go:203-226`) and `mcptoolset`'s `Run` (`mcptoolset/tool.go:96-120`). `ConfirmationProvider` is marked EXPERIMENTAL.

### MCP

`mcptoolset.New(Config{Transport | Endpoint, Client, Auth, ToolFilter, RequireConfirmation, RequireConfirmationProvider})` (`tool/mcptoolset/set.go:51-140`) uses the official `modelcontextprotocol/go-sdk`. It connects lazily with a connection refresher and supports per-request auth through an HTTP `RoundTripper` (streamable HTTP only). `IsError` results become Go errors. Structured content is returned as `{"output": StructuredContent}`, and anything else is flattened to text (`mcptoolset/tool.go:122-151`).

### Agents as tools

`agenttool.New(agent, cfg)` (`tool/agenttool/agent_tool.go:105-236`) runs the sub-agent in a **fresh in-memory `Runner`, session, artifact service and memory service** on every call. Parent state (minus `_adk*` keys) is copied in, but **no state deltas, artifacts or events flow back**. The parent receives only the final text, validated against `OutputSchema` if one is set. There is a `TODO(dpasiukevich): verify agent loop termination`. The newer v2 alternatives are task-mode and single-turn sub-agents, which are auto-installed as tools (`llmagent.go:139-181`) and run inside the same session under an isolation scope.

### Structured output

When `OutputSchema` is set, a `set_model_response` tool is injected, so the agent can still call other tools before answering (`internal/llminternal/outputschema_processor.go:34-40`). The tool's result is re-emitted as a final model event (`base_flow.go:835-845`).

---

## 6. Execution loop implementation

The call path for an LLM root: `Runner.Run` → `runNode` → `workflow.RunNode` → scheduler → `runner.newAgentNode` (a `DynamicNode`) → `llmagent.RunLLMAgentAsNode` → `llmAgent.run` (`llmagent.go:450-473`) → `llminternal.Flow.Run`.

### `Flow.Run` (`internal/llminternal/base_flow.go:127-176`)

```go
for {
    var lastEvent *session.Event
    for ev, err := range f.runOneStep(ctx) {
        if err != nil { yield(nil, err); return }
        if !yield(ev, nil) { return }
        lastEvent = ev
    }
    if lastEvent == nil { return }
    if lastEvent.IsFinalResponse() {
        if !isThoughtOnlyTurn(lastEvent) { return }
        thoughtOnlyTurns++                                  // cap: maxConsecutiveThoughtOnlyTurns = 10
        if thoughtOnlyTurns >= maxConsecutiveThoughtOnlyTurns { return }
    } else { thoughtOnlyTurns = 0 }
    if lastEvent.LLMResponse.Partial { log...; return }
}
```

**Termination conditions:**

- `IsFinalResponse()` (`session/session.go:218-231`) returns true in any of these cases:
  - `SkipSummarization` is set;
  - the event has `LongRunningToolIDs`;
  - the event has no function calls, no function responses, is not partial, and has no trailing code-execution result.
- The step yields no events.
- The step ends on a partial event.
- `ctx.Ended()` becomes true, via `EndInvocation()` from a before-agent callback (`agent/agent.go:258-300`).
- Ten consecutive thought-only turns.
- A step error.

**There is no max-iteration or max-LLM-call bound.** `LiveRunConfig.MaxLLMCalls` is declared (`agent/live.go:48`) and set to 100 in the REST server (`server/adkrest/controllers/runtime.go:449`), but nothing reads it. A model that keeps calling tools loops forever. The thought-only cap is explicitly "a safety net against a degenerate model, not a tuning knob" and is not configurable (`base_flow.go:113-125`).

### `runOneStep` (`base_flow.go:739-874`)

1. `req := &model.LLMRequest{Model: f.Model.Name()}`.
2. **`preprocess`** runs `DefaultRequestProcessors` in order (`base_flow.go:84-104`): basic (config), tools (resolve toolsets), auth, **request-confirmation** (which may execute confirmed tools and yield their events here), instructions (templating), identity, **compaction** (which may yield a summary event), **contents** (rebuild history from session events: branch and scope filtering, foreign-agent rewrite, function-response rearrangement), NL planning, code execution, output schema, **agent transfer** (add `transfer_to_agent` and instructions), and remove-display-name. It then calls every tool's `ProcessRequest` (`toolPreprocess`) and toolset processors. Request processors are themselves `iter.Seq2` producers, so preprocessing can emit events.
3. If `ctx.Ended()`, return.
4. **`callLLM`** (`base_flow.go:942-1030`):
   - Plugin `BeforeModel` runs, then agent `BeforeModel`. Either can short-circuit with a canned response, which is the documented caching hook.
   - `generateContent` wraps `m.GenerateContent(ctx, req, useStream)` in a `generate_content` OTel span and logs the request and response (`base_flow.go:1036-1083`).
   - For each response: on error, run `OnModelError` callbacks, which can substitute a response. `PopulateClientFunctionCallID` fills missing call IDs. Then plugin and agent `AfterModel` run, and either can replace the response.
5. Run response processors (NL planning, code execution). Skip empty responses with no error code.
6. **`finalizeModelResponseEvent`** creates an event with a pre-allocated ID, the author and branch, the `StateDelta` accumulated by callbacks, and `LongRunningToolIDs`. **Yield it.** If `resp.Partial`, continue with the next chunk.
7. **`handleFunctionCalls`** (see §5) produces one merged event or nil. Yield it, then yield the confirmation-request event if one exists, then yield the structured-output final event if one exists.
8. **Transfer.** If `ev.Actions.TransferToAgent != ""`, resolve the target through `agentToRun`, which is restricted to legal `transferTargets` rather than the whole tree. This is a deliberate divergence from Python, commented at `base_flow.go:1139-1151`. The flow then **runs the target agent inline** (its `RunNode` if it has one, otherwise `Run`) and forwards all of its events (`base_flow.go:850-872`). A transfer therefore nests the target's full run inside the current step's iterator. After the invocation, `findAgentToRun` makes the target the entry point for the next user turn.

### Other loops

- **Mode wrappers** (`agent/llmagent/llm_agent_wrapper.go:38-...`):
  - `single_turn` seeds the node input as user content, runs once, and turns the reply into `Output`.
  - `task` runs until the `finish_task` tool succeeds.
  - `chat` runs an outer dispatch loop. It scans the session for unresolved task-delegation calls and runs each through `workflow.RunNode` with `WithRunID(fc.ID)`, which gives a stable ID for replay. It then synthesizes a user-role function response and loops.
- **`loopagent`** loops over sub-agents until `MaxIterations` (0 means unbounded) or `Escalate`.

### How streaming is threaded through

Every layer is a pull iterator returning `iter.Seq2`: `GenerateContent`, `callLLM`, `runOneStep`, `Flow.Run`, `agent.Run` (with telemetry span wrapping via `telemetry.WrapYield`, `agent/agent.go:165-172`), the node body, and the scheduler consumer before the runner. No layer buffers except the provider aggregator. Breaking out of the outer `range` returns `false` from every nested `yield`, and each layer checks for that and returns. In the workflow engine, a consumer break triggers `cancelAll()` on the node contexts. There are **no channels on the single-agent path**; channels appear only where concurrency requires them (parallel agent, workflow scheduler, live mode).

---

## 7. Suspend/resume and error handling

### Interrupt mechanisms

All four mechanisms reduce to one primitive: a `FunctionCall` whose ID is listed in `LongRunningToolIDs`, answered later by a `FunctionResponse` with the same ID in a new user message.

| Mechanism | Emitted by | Wire name | Resume behavior |
|---|---|---|---|
| Long-running tool | `IsLongRunning()` tool returns nil | tool's own name | Agent node re-runs and sees the function response in history |
| Tool confirmation | `ctx.RequestConfirmation` | `adk_request_confirmation` | Request processor re-executes the original call with `ToolConfirmation` set |
| Workflow input request | `workflow.NewRequestInputEvent(ctx, RequestInput{...})` (`workflow/request_input.go:72-114`) | `adk_request_input` | **Handoff** (default): the response becomes the successors' input. **Re-entry** (`RerunOnResume=&true`): the node re-runs and reads `ctx.ResumedInput(id)` |
| Re-entry helper | `workflow.ResumeOrRequestInput(ctx, emit, req)` (`request_input.go:123-131`) | `adk_request_input` | Returns the reply on re-entry; otherwise emits the request and returns `ErrNodeInterrupted` |

### Resume algorithm

1. **Pick the invocation ID.** `resolveInvocationID` reuses the invocation ID of the event containing the matching `FunctionCall`, so the pause and its answer share an invocation (`run_node.go:433-448`).
2. **Reconstruct state.** `ReconstructRunState(sess, invocationID)` (`workflow/persistence.go:80-116`) scans only that invocation's events:
   - it records, per static node, which interrupts were raised (from `LongRunningToolIDs`, attributed through `NodeInfo.Path`), which were answered by `Author=="user"` responses (last answer wins, with a count), and the declared response schemas (re-extracted from the call args);
   - it collects every node's last `Output` so that re-entry nodes can rebuild their inputs;
   - it infers `NodeWaiting`, `NodePending` with `ResumedInputs`, or `NodeCompleted`.
3. **Build responses.** `buildResumeResponses` keeps only responses whose IDs are pending (`run_node.go:326-356`).
4. **Resume.** `Workflow.Resume(ctx, state, responses)` (`workflow/resume.go:73-...`) validates each payload against its `ResponseSchema` (failure yields `ErrInvalidResumeResponse` and the node stays waiting), then reschedules. It is two-pass so that a `JoinNode` sees all predecessors completed. If no waiting node matched, it yields `ErrNothingToResume`.
5. **Replay dynamic nodes.** Dynamic nodes (and the runner's agent node) **re-run from the top**. `dynamicSubScheduler.rehydrateCache` (`workflow/dynamic_scheduler.go:160-182`) pre-loads completed child outputs keyed by `"<parentPath>/<child>@<runID>"` from the current invocation's events. `RunNode` calls with stable run IDs return cached outputs instead of re-executing. This is **Temporal-style deterministic replay, memoized through the event log**, but it applies only to HITL resume, not crash recovery. Failures and interrupts are not cached.

Constraints and sharp edges:

- At most one pending HITL per dynamic-node activation (`ErrParallelHITLUnsupported`, `workflow/errors.go:47`).
- Interrupt IDs should be unique per run. Clients remember answered IDs, and a reused literal silently fails to re-prompt (`session/session.go:189-206`).
- `workflow.New` has a TODO noting that a graph change between deploys "silently corrupts the resume path", and there is no graph fingerprint (`workflow.go:243-249`).

### How state is serialized

No run-state snapshot exists. Everything durable is JSON on events: `Event.Output`, `RequestedInput.Payload`, `StateDelta` values, and `NodeState` Input and Output when reconstructed. Values are typed `any` and must be JSON-encodable. After a round trip, `any` becomes `map[string]any`, `float64` and so on, so **Go types are lost across a pause**. The docs recommend storing binary data as artifacts and passing URIs (`workflow/state.go:78-85`). `EventActions.MarshalJSON` has careful nil-versus-empty map handling for cross-runtime compatibility with Python (`session/session.go:278-310`).

### Retry

- **Workflow nodes:** `NodeConfig.RetryConfig`, with exponential backoff, jitter and a `ShouldRetry` predicate. Input-validation errors are never retried by default (`workflow/config.go:23-44`, `workflow/retry.go`). A nil `RetryConfig` means no retries. `DefaultRetryConfig()` gives 5 attempts from 1s to 60s at 2x. Retries use timers owned by the scheduler.
- **LLM calls:** the flow has **no retry**. It relies on the provider SDK (genai `HTTPOptions.RetryOptions`, openai-go's built-in retry) or on `OnModelErrorCallbacks`. Live mode has a reconnect policy with backoff (`internal/llminternal/live_reconnect.go`).
- **Tools:** the `plugin/retryandreflect` plugin counts per-tool failures, per invocation or globally, and returns a templated "reflect and retry" instruction to the model instead of the raw error. After `maxRetries` (default 3) it either tells the model to stop using the tool or returns the error (`plugin/retryandreflect/plugin.go`).

### Error types

The library uses sentinels throughout:

- `session.ErrNotFound`
- `tool.ErrConfirmationRequired` and `tool.ErrConfirmationRejected`
- `functiontool.ErrInvalidArgument`
- `llminternal.ErrModelNotConfigured`
- the workflow errors `ErrNodeFailed`, `ErrNodeInterrupted`, `ErrNodeWaitingForOutput` (which wraps `ErrNodeInterrupted`), `ErrInputValidation`, `ErrInvalidRunID`, `ErrParallelHITLUnsupported`, `ErrOutputAlreadyDelegated`, `ErrMultipleOutputs`, `ErrMultipleRoutingEvents`, `ErrMultipleTerminalOutputs`, `ErrInvalidResumeResponse` and `ErrNothingToResume`
- about 20 `openaimodel.Err*` values.

There is one structured error, `workflow.NodeRunError{ChildName, ChildPath, RunID, Cause}` with `Unwrap` (`workflow/errors.go:58-89`). Errors travel in-band as `yield(nil, err)`.

Error semantics differ by layer:

- `Flow.Run` stops on the first error.
- The runner's agent path **yields the error and continues** consuming the agent (`runner.go:725-731`).
- The workflow scheduler records the first node error, cancels siblings, drains, and reports it. A node's own error outranks the cancellation cause (`scheduler.go:628-640`).
- Model provider errors can also arrive as data (`LLMResponse.ErrorCode` and `ErrorMessage`) rather than as `error`.

### Cancellation

- `context.Context` is **embedded** in `InvocationContext` and `agent.Context`, so cancellation flows to the model SDK and tools naturally.
- `WithAgentTimeout` and `WithAgentCancel` derive child contexts. `NodeConfig.Timeout` bounds a node activation.
- The scheduler distinguishes echoes of external cancellation from genuine node failures (`echoesCancellation`, `scheduler.go:512-516`) and records `context.Cause`.
- `EndInvocation()` is a cooperative flag checked between steps.
- The flow itself does not check `ctx.Done()` between steps. It relies on the model call failing.

---

## 8. Strengths, weaknesses, and lessons for Dive

### What is excellent and worth borrowing

1. **`iter.Seq2[*Event, error]` end to end.** A single stream type composes every layer. Early `break` propagates as cancellation for free, and there are no channels on the hot path. This is the most idiomatic modern-Go decision in the codebase.
2. **Yield is the commit point, with an explicit handshake.** The `processed` and `ackChan` pattern (`scheduler.go:470-492`, `parallelagent/agent.go:150-172`) means a producer cannot run ahead of persistence. Every durable agent runtime needs this invariant, and ADK states it in code.
3. **Side effects as data on the event.** `EventActions{StateDelta, ArtifactDelta, TransferToAgent, Escalate, ...}` makes "what happened" and "what changed" one atomic append. Tools and callbacks mutate a scratch `Actions()`, and the framework decides when to commit. Parallel tool calls merge these deterministically. The framework also forbids callers from forging `Compaction`, which shows good judgment about which fields belong to the framework.
4. **Platform seams.** Clock, UUID and task runner are carried on `context.Context` (`platform/`). The cost is tiny, and the payoff is deterministic tests and the ability to run under an external durable-execution engine without ADK depending on one.
5. **Durable-by-log HITL with memoized replay.** Rebuilding the paused state from events and serving completed children from a cache keyed by stable run IDs (`rehydrateCache`) is a clean, storage-agnostic design. Resume is idempotent (`resolvedCount`) and gives clear feedback (`ErrNothingToResume`, `ErrInvalidResumeResponse`).
6. **One interrupt primitive.** Long-running tools, approvals and input requests all reduce to "a function call ID in `LongRunningToolIDs`, answered by a function response". Any client that understands function calls can render them.
7. **The graph engine's engineering.** A single-consumer scheduler, bounded queue, max-concurrency cap, per-node timeouts and retries, panic recovery, typed routes, fan-out and join, a parallel worker, and a static validation pass (`workflow/validation.go`). `DynamicNode` plus `RunNode[OUT]` gives an imperative escape hatch inside a declarative graph.
8. **Non-destructive, well-reasoned compaction.** Summaries are events with coverage ranges and exclusion lists, and the docs include measured recall and size data (`session/compaction/compaction.go:15-100`).
9. **A conformance suite for storage backends** (`session/sessiontestsuite`) turns interface contracts into tests.
10. **"Why" comments.** Many code comments explain the failure mode a line prevents, not just what it does. The two exceptions noted above are the stale `RunStateSessionKey` reference and the dead `MaxLLMCalls` field.

### What is awkward

1. **Gemini coupling is total.** `genai.Content`, `Part`, `FunctionDeclaration`, `GenerateContentConfig`, `Schema` and usage metadata make up the public API of agents, tools, events, memory and artifacts. The OpenAI adapter has to reject about 20 config fields, cannot round-trip reasoning, and disables strict tool schemas (`model/openaimodel/doc.go`). Live mode type-asserts `Client() *genai.Client`. `InputSchema` and `OutputSchema` are `*genai.Schema`, while tools use `jsonschema.Schema`: two schema systems in one config.
2. **Interfaces that lie or are sealed.**
   - `Agent` has an unexported method.
   - `tool.Tool` is three methods, but the runtime requires `ProcessRequest`, `Declaration` and `Run`, all defined in `internal/toolinternal`.
   - `DynamicSubScheduler.RunNode(any, any, any) (any, error)` is untyped to dodge an import cycle (`agent/dynamic_scheduler.go:19-25`).
3. **A god context.** `agent.Context` has about 30 methods covering invocation, callback, tool and workflow-node concerns. It embeds `context.Context` (acknowledged as temporary) and breaks mocks whenever it grows (`README-v2.md`).
4. **Two orchestration runtimes glued together.** The agent tree with transfers and Sequential, Parallel and Loop agents coexists with the graph engine. LLM roots are secretly wrapped in a one-node workflow. Agent modes (`chat`, `task`, `single_turn`) change semantics based on placement. The "resolve mode, bind mode, re-bind mode" plumbing in `llm_agent_wrapper.go` is hard to reason about. Python parity, not Go ergonomics, drives much of this.
5. **Stringly-typed protocol.** Reserved names include `transfer_to_agent`, `set_model_response`, `finish_task`, `adk_request_confirmation`, `adk_request_input`, the `"user"` author, the `_adk` state prefix and `{key?}` templates. Confirmation replies accept three JSON shapes.
6. **Weak tool results.** `map[string]any` everywhere, errors flattened to strings, and no images or files from tools.
7. **No loop bound and no step accounting.** There is no max-steps setting, no token or cost budget, and no typed stop reason. The caller infers termination from `IsFinalResponse()` on the last event.
8. **Durability stops at the event log.** There is no mid-turn crash recovery and no record of "tool started but not finished". Optimistic concurrency uses timestamps. Artifacts are stored outside the event transaction. The full history is re-scanned on every step (contents, confirmations, `ReconstructRunState`, `openLongRunningCallIDs`), which is O(history) per step.
9. **Callback sprawl.** Twelve-plus callback slices on configs, each with first-non-nil-wins semantics, duplicated between plugins and agents. Every seam in `base_flow.go` repeats the plugin-then-agent-callbacks boilerplate.
10. **Other sharp edges:**
    - `agenttool` discards child state, artifacts and events;
    - toolsets are resolved once per run;
    - custom tools that panic crash the process;
    - `RequireConfirmationProvider any` is checked at runtime;
    - tri-state `*bool` config fields (`rerun := true; RerunOnResume: &rerun`);
    - there is no Anthropic provider.

### Concrete recommendations for Dive

These map onto the Dive v2 vocabulary in `docs/design/2026-09-26-dive-v2-conceptual-model.md`: the Turn as aggregate; Engine, Recorder, Policy and Observer; and tool definition, execution and outcome as separate things.

1. **Adopt `iter.Seq2[Event, error]` as the run surface, and specify its contract in writing.**
   - Define what `yield` returning means: the event has been handed to the Recorder and committed.
   - Define what `break` means: cooperative cancellation, with a typed "abandoned" stop reason recorded.
   - Unlike ADK, keep **the Engine's working transcript in memory** and treat the store as a journal, not the read path. `Agent.Run` without a store should still work, and nothing should re-scan O(n) history per step.
2. **Make commit-before-proceed a first-class Engine guarantee.** Borrow the `processed` handshake, but put it in the Engine–Recorder boundary rather than relying on the consumer's `yield`. ADK's guarantee only holds when the Runner is the consumer.
3. **Put side effects on events, and commit them atomically with the step.** A `StepOutcome` / `Actions`-like struct with state delta, artifact refs and control signals (handoff, stop, approval requested) lets the Recorder persist "what happened" and "what changed" in one write. Forbid tools from forging framework-owned fields, as ADK does for `Compaction`.
4. **Record step boundaries so crash recovery is possible.** ADK cannot tell a tool that never ran from one that ran and whose result was lost. Journal `tool_call_started` with the call ID as the idempotency key, and journal the model response before dispatch. Recovery can then either re-dispatch idempotent tools or surface an "incomplete turn" to the application.
5. **Provide `platform`-style seams:** `Clock`, `IDSource` and `TaskRunner`, as Engine options or on the context. They are cheap, they make golden tests deterministic, and they are the entire integration surface a Temporal or Restate host needs.
6. **Build one interrupt primitive with typed payloads.**
   - Borrow `RequestInput{InterruptID, Message, ResponseSchema, Payload}` and the sentinels `ErrNothingToResume` and `ErrInvalidResumeResponse`.
   - Resume by ID, reuse the original turn ID, and make duplicate resumes no-ops.
   - Unlike ADK, keep approvals as an **Engine Policy** applied uniformly before dispatch, rather than re-implemented in each tool adapter (ADK has three copies).
   - Return a typed `Suspended{Pending []Interrupt}` outcome instead of making callers check `LongRunningToolIDs`.
7. **Bound the loop and report why it stopped.** Add `MaxSteps` (and optionally token and cost budgets) with a typed `StopReason` such as `completed`, `max_steps`, `suspended`, `cancelled`, `error` or `handoff`. ADK's missing bound, together with the dead `MaxLLMCalls` field, is the clearest gap to exploit.
8. **Keep Dive's `llm` layer provider-neutral and honest about capabilities.** Do not let one vendor's wire types become the public API. Where a provider cannot honor a setting, report it explicitly through capabilities or typed errors, as `openaimodel` does, rather than silently dropping it. Round-trip reasoning and thinking items and signatures per provider. ADK cannot do this for OpenAI.
9. **Tools:**
   - Offer one honest public interface, with no hidden `ProcessRequest` requirement. Keep generic typed function tools with `jsonschema-go` inference; ADK's `Func[TArgs,TResults]` is good.
   - Results should be **content blocks** (text, image, file, structured JSON) plus an `IsError` flag and a preserved Go `error` for Observers, not `map[string]any`.
   - Recover panics once, at the executor boundary.
   - Run parallel calls with a **configurable concurrency bound** and merge results in call order; ADK's default is unbounded goroutines.
   - Resolve toolsets per step, not per run.
10. **Keep `context.Context` separate from a small typed call context.** Use `func(ctx context.Context, call *ToolCall) (*ToolResult, error)` style signatures rather than a 30-method interface that embeds `context.Context`. Small interfaces keep mocks stable.
11. **Prefer composable middleware over callback slices.** A single `Interceptor` / `func(next Handler) Handler` shape for model calls and tool calls replaces ADK's twelve callback slices and the duplicated plugin-then-agent dispatch at every seam. Keep Observers read-only and separate from Policies that can change behavior.
12. **If Dive adds orchestration, borrow `DynamicNode` + `RunNode[OUT]` + memoized replay rather than the dual agent-tree and graph runtime.** Child results keyed by stable run IDs and rehydrated from the journal give deterministic resume with plain Go control flow. Add the graph fingerprint that ADK's TODO asks for. Sub-agents-as-tools should share the parent's Recorder under a scoped branch, not a throwaway session as in `agenttool`.
13. **Borrow the non-destructive compaction design** (summary events with covered range and exclusions) **and ship a conformance suite for Recorder and store implementations**, modeled on `sessiontestsuite`.
