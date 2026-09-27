# Eino (CloudWeGo) — Architecture Analysis for Dive

- **Repo:** `github.com/cloudwego/eino`, checked out at `~/git/lib/eino`
- **Commit analyzed:** `ba04fde8` ("feat: expose cache write tokens in model usage (#1309)"), 2026-09-23, 468 commits in history
- **Size:** about 341 non-test Go files. `go.mod` declares `go 1.18`.
- **Method:** I read the source directly. All paths are relative to the repo root.

---

## 1. Overview

Eino is ByteDance/CloudWeGo's Go framework for LLM applications. The core idea is that **everything is a typed, stream-aware component that you can compose into graphs**. The agent layer (`adk`, "Agent Development Kit") came later and sits on top of that graph engine: at this commit a `ChatModelAgent` is a small compose graph that gets compiled when the agent runs.

The code reflects three priorities:

1. **Four stream paradigms, with automatic adaptation between them.** Every executable is a `Runnable[I, O]` with `Invoke`, `Stream`, `Collect`, and `Transform`. If a component implements only one of them, the framework adapts it to the other three (`compose/runnable.go:28-37`).
2. **Orchestration as a first-class concern.** `compose` has `Graph`, `Chain`, and `Workflow` (field-mapped DAG). It supports Pregel and DAG execution modes, branches, per-graph local state, and checkpoint/interrupt.
3. **Heavy investment in interrupt/resume and cancellation.** Much of the `adk` code handles addressable interrupts, nested checkpoints (agents inside tools inside graphs), cancel safe-points, and backward compatibility for gob-encoded checkpoints.

The in-code docs also carry a clear opinion. Agent-to-agent "transfer" (the Google ADK / OpenAI Swarm style) is marked **"NOT RECOMMENDED: Agent transfer with full context sharing between agents has not proven to be more effective empirically. Consider using ChatModelAgent with AgentTool or DeepAgent instead"**. This appears on `TransferToAgentAction`, `RunStep`, `OnSubAgents`, `SetSubAgents`, `OutputKey`, and elsewhere (`adk/interface.go:311-316`, `adk/flow.go:70-75`). The team built the multi-agent machinery and then walked it back in favor of agents-as-tools.

### Repo layout

| Path | Responsibility |
|---|---|
| `schema/` | `Message`, `AgenticMessage` (content-block messages), `ToolInfo`, `ToolResult`, `StreamReader`/`StreamWriter`, `Document`, gob name registry (`RegisterName`) |
| `components/` | Interfaces only: `model`, `tool` (+`tool/utils` for `InferTool`), `prompt`, `retriever`, `embedding`, `indexer`, `document` |
| `compose/` | Graph engine: `Graph`, `Chain`, `Workflow`, `ToolsNode`, branches, state, checkpoint, interrupt, runner (35 files) |
| `callbacks/`, `internal/callbacks/` | Aspect-style `Handler` (OnStart/OnEnd/OnError + stream variants), global and per-call |
| `adk/` | Agents: `Runner`, `ChatModelAgent`, ReAct graph, middleware (`Handlers`), agent-as-tool, workflow agents (Sequential/Parallel/Loop), transfer/flow, cancel, retry, failover, `TurnLoop` |
| `adk/middlewares/` | summarization, reduction (tool-result clearing), filesystem, skill, plantask, toolsearch (dynamic tools), patchtoolcalls, agentsmd |
| `adk/prebuilt/` | `deep` (DeepAgent: todos + filesystem + task/sub-agent tool), `planexecute`, `supervisor` |
| `flow/` | Legacy pre-ADK agents (`flow/agent/react`, `multiagent/host`) and retriever flows |
| `internal/core/` | Interrupt signal tree, addresses, resume info, `CheckPointStore` |
| `ext/`, `examples/` | **Empty at this commit.** Provider implementations (OpenAI, Claude, Ark, …) and MCP tools live in the separate `eino-ext` repo. |

The core repo ships **no provider adapters**. It is interfaces, orchestration, and agent runtime only.

---

## 2. Core packages and Go types

### 2.1 `components/model` — the model contract

```go
// components/model/interface.go:27-39
type messageType interface {
	*schema.Message | *schema.AgenticMessage
}

type BaseModel[M messageType] interface {
	Generate(ctx context.Context, input []M, opts ...Option) (M, error)
	Stream(ctx context.Context, input []M, opts ...Option) (*schema.StreamReader[M], error)
}

type BaseChatModel = BaseModel[*schema.Message]            // :71

// Deprecated: Use [ToolCallingChatModel] instead.          // :73-87
type ChatModel interface {
	BaseChatModel
	BindTools(tools []*schema.ToolInfo) error
}

type ToolCallingChatModel interface {                        // :99-103
	BaseChatModel
	WithTools(tools []*schema.ToolInfo) (ToolCallingChatModel, error)
}

type AgenticModel = BaseModel[*schema.AgenticMessage]      // :109
```

The history here is instructive. `BindTools` mutated the model in place and caused races (the doc comment says so). It was replaced by the immutable `WithTools`. Now ADK ignores both and passes tools **per request** via `model.WithTools(...)` as a call option (`adk/chatmodel.go:1481-1483`, `adk/wrappers.go:1250-1255`). Per-request tools won.

The options are a common struct plus an implementation-specific escape hatch:

```go
// components/model/option.go (fields, comments elided)
type Options struct {
	Temperature *float32; Model *string; TopP *float32
	Tools []*schema.ToolInfo; DeferredTools []*schema.ToolInfo
	ToolSearchTool *schema.ToolInfo
	MaxTokens *int; Stop []string
	ToolChoice *schema.ToolChoice; AllowedToolNames []string
	AgenticToolChoice *schema.AgenticToolChoice
}
type Option struct {
	apply func(opts *Options)
	implSpecificOptFn any
}
func WrapImplSpecificOptFn[T any](optFn func(*T)) Option          // :196
func GetImplSpecificOptions[T any](base *T, opts ...Option) *T    // :239
```

The same `Option{apply, implSpecificOptFn any}` pattern is used for tools, retrievers, and agents (`adk.AgentRunOption`, `adk/call_option.go`).

### 2.2 `schema` — messages and streams

There are **two message models** side by side.

```go
// schema/message.go:498-532
type Message struct {
	Role RoleType
	Content string
	MultiContent []ChatMessagePart            // Deprecated
	UserInputMultiContent []MessageInputPart
	AssistantGenMultiContent []MessageOutputPart
	Name string
	ToolCalls []ToolCall                      // assistant only
	ToolCallID string                         // tool only
	ToolName string                           // tool only
	ResponseMeta *ResponseMeta
	ReasoningContent string
	Extra map[string]any
}

// schema/message.go:133-145
type ToolCall struct {
	Index *int          // used to merge stream chunks
	ID string
	Type string
	Function FunctionCall   // {Name, Arguments string}
	Extra map[string]any
}
```

```go
// schema/agentic_message.go:71-83, 102+
type AgenticMessage struct {
	Role AgenticRoleType
	ContentBlocks []*ContentBlock
	ResponseMeta *AgenticResponseMeta  // has OpenAIExtension/GeminiExtension/ClaudeExtension
	Extra map[string]any
}
type ContentBlock struct {
	Type ContentBlockType
	Reasoning *Reasoning
	UserInputText *UserInputText; UserInputImage *UserInputImage; /* audio, video, file */
	AssistantGenText *AssistantGenText; /* image, audio, video */
	FunctionToolCall *FunctionToolCall
	FunctionToolResult *FunctionToolResult
	ToolSearchFunctionToolResult ...
	MCPToolCall *MCPToolCall; MCPToolResult ...   // server-side MCP
	// ... server tool call/result, approval request/response, etc.
}
```

`Message` is the OpenAI-chat-completions shape. It is flat, has role-specific fields, and has three generations of multimodal fields. `AgenticMessage` is a content-block model closer to Anthropic Messages and OpenAI Responses, with provider-extension structs hard-wired into `schema` (`schema/claude`, `schema/gemini`, `schema/openai`). Nearly all of ADK is generic over both through the sealed union `MessageType` (`adk/interface.go:43-45`).

**Streams** are the other foundational type:

```go
// schema/stream.go
func Pipe[T any](cap int) (*StreamReader[T], *StreamWriter[T])          // :99
func (sw *StreamWriter[T]) Send(chunk T, err error) (closed bool)
func (sr *StreamReader[T]) Recv() (T, error)                             // :195, io.EOF at end
func (sr *StreamReader[T]) Close()                                       // :229
func (sr *StreamReader[T]) Copy(n int) []*StreamReader[T]                // :261 fan-out
func (sr *StreamReader[T]) SetAutomaticClose()                           // :279 finalizer-based close
func StreamReaderFromArray[T any](arr []T) *StreamReader[T]
func StreamReaderWithConvert[T, D any](sr *StreamReader[T], convert func(T) (D, error), opts ...ConvertOption) *StreamReader[D]
func MergeStreamReaders[T any](srs []*StreamReader[T]) *StreamReader[T]
```

A `StreamReader` can be read **once**. Fan-out requires `Copy(n)`. Chunk merging is type-registered (`schema.ConcatMessages`, `compose/stream_concat.go`).

### 2.3 `components/tool`

```go
// components/tool/interface.go:32-79
type BaseTool interface {
	Info(ctx context.Context) (*schema.ToolInfo, error)
}
type InvokableTool interface {
	BaseTool
	InvokableRun(ctx context.Context, argumentsInJSON string, opts ...Option) (string, error)
}
type StreamableTool interface {
	BaseTool
	StreamableRun(ctx context.Context, argumentsInJSON string, opts ...Option) (*schema.StreamReader[string], error)
}
type EnhancedInvokableTool interface {
	BaseTool
	InvokableRun(ctx context.Context, toolArgument *schema.ToolArgument, opts ...Option) (*schema.ToolResult, error)
}
type EnhancedStreamableTool interface {
	BaseTool
	StreamableRun(ctx context.Context, toolArgument *schema.ToolArgument, opts ...Option) (*schema.StreamReader[*schema.ToolResult], error)
}
```

```go
// schema/tool.go:128-143, 282-287, 477-492
type ToolInfo struct {
	Name string
	Desc string
	Extra map[string]any
	*ParamsOneOf           // nil => no params
}
type ParamsOneOf struct {
	params map[string]*ParameterInfo   // legacy mini-schema
	jsonschema *jsonschema.Schema      // github.com/eino-contrib/jsonschema
}
type ToolArgument struct { Text string }
type ToolResult  struct { Parts []ToolOutputPart }   // text/image/audio/video/file
```

A design wart: `EnhancedInvokableTool` reuses the method name `InvokableRun` with a different signature. A single Go type therefore **cannot** implement both `InvokableTool` and `EnhancedInvokableTool`. The comment "When a tool implements both a standard and an enhanced interface, ToolsNode prioritises the enhanced interface" (`:65-66`) only works across the invoke/stream pairs. The tool contract is also stringly typed at the core: JSON string in, string out.

### 2.4 `compose` — the orchestration engine

```go
// compose/runnable.go:32-37
type Runnable[I, O any] interface {
	Invoke(ctx context.Context, input I, opts ...Option) (output O, err error)
	Stream(ctx context.Context, input I, opts ...Option) (output *schema.StreamReader[O], err error)
	Collect(ctx context.Context, input *schema.StreamReader[I], opts ...Option) (output O, err error)
	Transform(ctx context.Context, input *schema.StreamReader[I], opts ...Option) (output *schema.StreamReader[O], err error)
}
```

- `Graph[I, O]` (`compose/generic_graph.go:93`) has `AddChatModelNode`, `AddToolsNode`, `AddLambdaNode`, `AddGraphNode`, `AddBranch`, `AddEdge`, and `Compile(ctx, ...GraphCompileOption) (Runnable[I,O], error)`. Type compatibility between nodes is checked at compile time via reflection (`compose/graph.go:561` `updateToValidateMap`).
- `Chain[I, O]` is a linear builder over Graph. `Workflow[I, O]` (`compose/workflow.go:45`) is a DAG with struct field mappings (`AddInput(from, MapFields(...))`).
- Node trigger modes: `AnyPredecessor` (Pregel, cycles allowed, the default) and `AllPredecessor` (DAG, eager) (`compose/types.go:34-47`, `compose/graph.go:674-700`).
- Graph-local state:

```go
// compose/state.go:30, 42-52, 165
type GenLocalState[S any] func(ctx context.Context) (state S)
type StatePreHandler[I, S any]  func(ctx context.Context, in I, state S) (I, error)
type StatePostHandler[O, S any] func(ctx context.Context, out O, state S) (O, error)
func ProcessState[S any](ctx context.Context, handler func(context.Context, S) error) error
```

State lives in `ctx` as a parent-linked chain of `internalState{state any; mu sync.Mutex; parent}`. `ProcessState[S]` walks up the chain to the first state that type-asserts to `S` (lexical scoping by **type**) and runs your closure under that level's mutex (`compose/state.go:165-196`).

- Lambdas: `InvokableLambda`, `StreamableLambda`, `CollectableLambda`, `TransformableLambda`, `AnyLambda` (`compose/types_lambda.go:100-174`).
- Call options can target nodes: `WithChatModelOption`, `WithToolsNodeOption`, `WithCallbacks`, `WithRuntimeMaxSteps`, `WithCheckPointID`, `WithStateModifier` (`compose/graph_call_options.go`, `compose/checkpoint.go:75-105`).

### 2.5 `adk` — agents

```go
// adk/interface.go:440-464
type TypedAgentInput[M MessageType] struct {
	Messages        []M
	EnableStreaming bool
}
type TypedAgent[M MessageType] interface {
	Name(ctx context.Context) string
	Description(ctx context.Context) string
	Run(ctx context.Context, input *TypedAgentInput[M], options ...AgentRunOption) *AsyncIterator[*TypedAgentEvent[M]]
}
type Agent = TypedAgent[*schema.Message]

type TypedResumableAgent[M MessageType] interface {                      // :481
	TypedAgent[M]
	Resume(ctx context.Context, info *ResumeInfo, opts ...AgentRunOption) *AsyncIterator[*TypedAgentEvent[M]]
}
```

```go
// adk/interface.go:419-435, 320-324, 73-98, 357-369
type TypedAgentEvent[M MessageType] struct {
	AgentName string
	RunPath   []RunStep
	Output    *TypedAgentOutput[M]
	Action    *AgentAction
	Err       error
}
type TypedAgentOutput[M MessageType] struct {
	MessageOutput    *TypedMessageVariant[M]
	CustomizedOutput any
}
type TypedMessageVariant[M MessageType] struct {
	IsStreaming   bool
	Message       M
	MessageStream *schema.StreamReader[M]
	Role          schema.RoleType          // Assistant or Tool (Message path only)
	AgenticRole   schema.AgenticRoleType   // Agentic path only
	ToolName      string
}
type AgentAction struct {
	Exit             bool
	Interrupted      *InterruptInfo
	TransferToAgent  *TransferToAgentAction
	BreakLoop        *BreakLoopAction
	CustomizedAction any
	internalInterrupted *core.InterruptSignal
}
```

The iterator is a thin wrapper over an **unbounded** channel:

```go
// adk/utils.go:31-60
type AsyncIterator[T any] struct { ch *internal.UnboundedChan[T] }
func (ai *AsyncIterator[T]) Next() (T, bool)
type AsyncGenerator[T any] struct { ch *internal.UnboundedChan[T] }
func (ag *AsyncGenerator[T]) Send(v T)
func (ag *AsyncGenerator[T]) Close()
func NewAsyncIteratorPair[T any]() (*AsyncIterator[T], *AsyncGenerator[T])
```

The Go 1.18 floor rules out `iter.Seq`. Because the channel is unbounded, there is **no backpressure**: a slow consumer means memory growth, never a producer stall.

```go
// adk/runner.go:38-62
type TypedRunner[M MessageType] struct {
	a               TypedAgent[M]
	enableStreaming bool
	store           CheckPointStore
}
type TypedRunnerConfig[M MessageType] struct {
	Agent           TypedAgent[M]
	EnableStreaming bool
	CheckPointStore CheckPointStore
}
func (r *TypedRunner[M]) Run(ctx, messages []M, opts ...AgentRunOption) *AsyncIterator[*TypedAgentEvent[M]]
func (r *TypedRunner[M]) Query(ctx, query string, opts ...AgentRunOption) *AsyncIterator[...]
func (r *TypedRunner[M]) Resume(ctx, checkPointID string, opts ...) (*AsyncIterator[...], error)
func (r *TypedRunner[M]) ResumeWithParams(ctx, checkPointID string, params *ResumeParams, opts ...) (*AsyncIterator[...], error)

type ResumeParams struct { Targets map[string]any }   // interrupt ID -> resume data
```

`ChatModelAgentConfig` (`adk/chatmodel.go:260-415`) has these fields: `Name`, `Description`, `Instruction` (an f-string template over session values), `Model model.BaseModel[M]`, `ToolsConfig`, `GenModelInput`, `Exit`, `OutputKey`, `MaxIterations` (default 20), `Middlewares` (deprecated struct hooks), `Handlers []TypedChatModelAgentMiddleware[M]`, `ModelRetryConfig`, and `ModelFailoverConfig`.

`ToolsConfig` embeds `compose.ToolsNodeConfig` and adds `ReturnDirectly map[string]bool` and `EmitInternalEvents bool` (`adk/chatmodel.go:136-156`).

---

## 3. Key concepts and how they relate

```
                    ┌───────────────────────────── adk.Runner ─────────────────────────────┐
 []Message ───────▶ │  runCtx{RootInput, RunPath, Session{Values, Events}}  (ctx value)     │
                    │  CheckPointStore (Get/Set bytes)   ── saves only on interrupt/cancel  │
                    │                                                                       │
                    │  flowAgent (wraps every agent: RunPath, event recording, transfer,    │
                    │    │         callbacks OnStart/OnEnd)                                 │
                    │    ▼                                                                  │
                    │  ChatModelAgent ── builds & compiles per Run ──▶ compose.Chain        │
                    │    Handlers (middleware)                          Lambda(GenModelInput)│
                    │    ModelRetry / Failover                          └▶ ReAct Graph      │
                    └───────────────────────────────────────────────────────┬──────────────┘
                                                                            │
     ReAct Graph (adk/react.go:354-561), local state = adk.State (Messages, RemainingIterations, Extra…)
     START → Init → ChatModel ─(stream branch: any chunk has ToolCalls?)─┬─ no ─▶ [AfterAgent] → END
                      ▲                                                  └─ yes ─▶ CancelCheck
                      │                                                             ▼
                      │   AfterToolCallsCancelCheck ◀── AfterToolCalls ◀──── ToolsNode (parallel)
                      └──────────────(return-directly branch)──────────────▶ ToolNodeToEndConverter → END

   AsyncIterator[*AgentEvent] ◀── events pushed from inside the graph via ctx-held generator:
      - model output (eventSenderModelWrapper, stream Copy(2))
      - tool results (eventSenderToolWrapper, one per tool on completion)
      - Action{Interrupted|Exit|TransferToAgent|BreakLoop}, Err
```

**Concept map**

- **Component** (model, tool, retriever, …) is an interface in `components/*`. Implementations live in `eino-ext`.
- **Runnable / Graph / Chain / Workflow** is the general DAG/Pregel orchestration layer. It is typed at the edges and uses `any` plus reflection inside.
- **Agent** is anything that produces an `AsyncIterator` of events. The concrete agents are `ChatModelAgent` (a ReAct loop), workflow agents (`NewSequentialAgent`, `NewParallelAgent`, `NewLoopAgent`, `adk/workflow.go:694-712`), `flowAgent` (the transfer wrapper), and prebuilt `deep`, `planexecute`, and `supervisor`.
- **Runner** is the entry point. It owns checkpointing and sets up `runCtx`.
- **Session** (`adk/runctx.go:34-47`) is per-run, not per-conversation. It holds `Values map[string]any` (used for instruction templating and `OutputKey`) and `Events []*agentEventWrapper` (the run's transcript, used to build input for transferred agents). **There is no built-in cross-run memory.** The caller passes the full `[]Message` history into each `Runner.Run`.
- **Agent state** (`adk/react.go:35-57`, `typedState`) is the ReAct graph's local state: `Messages`, `ToolInfos`, `RemainingIterations`, `Extra` (via `SetRunLocalValue`), and return-directly bookkeeping. It gets serialized into checkpoints.
- **Middleware** (`TypedChatModelAgentMiddleware`, `adk/handler.go:141-258`) hooks `BeforeAgent`, `AfterAgent`, `BeforeModelRewriteState`, `AfterModelRewriteState`, and `WrapModel`, plus four `Wrap*ToolCall` variants. You embed `*TypedBaseChatModelAgentMiddleware[M]` to get no-op defaults.
- **Callbacks** (`internal/callbacks/interface.go:38-48`) are orthogonal, observability-oriented hooks that fire for every component and graph node. They can be global (`callbacks.AppendGlobalHandlers`) or per-call (`compose.WithCallbacks`). Input and output are `any` (`CallbackInput = any`), and you convert with `model.ConvCallbackInput` and similar helpers.
- **Interrupt/Resume** is an address-based signal tree (`internal/core`) that spans agents, graph nodes, and tools (§7).
- **Agent-as-tool** (`adk.NewAgentTool`, `adk/agent_tool.go:93`) is the recommended multi-agent composition. The inner agent runs in its own Runner with an in-memory "bridge" checkpoint store, and its checkpoint bytes are embedded in the parent's interrupt state.
- **TurnLoop** (`adk/turn_loop.go:562-896`) is a higher-level, long-lived loop. Callers push items of type `T`, and `GenInput`/`PrepareAgent`/`OnAgentEvents` callbacks decide how to batch them into agent turns. It supports preemption at safe points, graceful and immediate stop, and between-turn checkpointing of unprocessed items. This is Eino's answer to a "session actor".

---

## 4. Execution, durability, and guarantees

### 4.1 How a run executes

1. `Runner.Run` (`adk/runner.go:156-203`) wraps the agent in `flowAgent` (`toFlowAgent`), creates a new `runCtx` in `ctx`, and calls `fa.Run`. If a store or cancel option is present, it starts **one goroutine** (`typedRunnerHandleIterImpl`, `:271-342`) that relays events and saves checkpoints on interrupt or cancel.
2. `flowAgent.run` (`adk/flow.go:481-567`) loops over the inner iterator. It stamps `AgentName`/`RunPath`, copies each event into the session (only when the RunPath matches exactly, so nested/tool-internal events are not recorded), tees events to callbacks, and forwards them. After the inner agent ends, it checks `lastAction` for `Exit`, `Interrupted`, or `TransferToAgent` and, on a transfer, runs the destination agent.
3. `ChatModelAgent.Run` (`adk/chatmodel.go:1446-1533`) resolves cancel, runs `BeforeAgent` handlers (which may change the instruction or tools and force a graph rebuild), and starts **one goroutine** that calls the run function.
4. The run function (`adk/chatmodel.go:1115-1239`) **builds a new ReAct graph and compiles a Chain on every Run** (`newReact(ctx, msgConf)` then `chain.Compile(ctx, ...)`). The static parts are memoized in `buildRunFunc` (`sync.Once`, `:1368-1401`), but compilation with reflection-based type validation happens per invocation. It then calls `runnable.Stream` or `runnable.Invoke` depending on `EnableStreaming`.
5. The compose runner (`compose/graph_run.go:108-381`) runs a Pregel superstep loop: it submits ready tasks, waits, computes successors through channels and branches, checks `ctx.Done()` at each step, enforces `maxSteps`, and handles interrupt-before/after nodes.

### 4.2 Concurrency model

- One goroutine per agent run (the generator side), plus one relay goroutine per Runner when checkpointing or cancel is enabled, plus goroutines for graph tasks. The task manager runs a lone task synchronously when it can (`compose/graph_manager.go:346`).
- Parallel tool calls: `parallelRunToolCall` (`compose/tool_node.go:1091-1124`) starts N-1 goroutines, runs task 0 on the caller's goroutine, recovers panics into `safe.NewPanicErr`, and waits on a `sync.WaitGroup`. There is **no concurrency limit**. `ExecuteSequentially: true` is the only knob.
- Shared mutable state is guarded by the per-level state mutex (`ProcessState`). Middleware and tool wrappers mutate `State` under that lock, including from parallel tool goroutines (`adk/wrappers.go:862-870`).

### 4.3 Streaming

Streaming is **a side channel, not the graph's data path**, at least for the agent loop:

- `typedEventSenderModel.Stream` (`adk/wrappers.go:319-345`) calls `result.Copy(2)`. Copy 0 goes to the user as an `AgentEvent` with `IsStreaming=true`. Copy 1 continues inward.
- `typedStateModelWrapper.Stream` (`adk/wrappers.go:1309-1428`) then **fully concatenates** the stream (`concatMessageStream(stream)`, `:1384`), appends the message to state, runs the `AfterModelRewriteState` handlers, and returns a single-element stream. The graph does not advance to tools until the whole model message exists.
- The ReAct branch (`adk/react.go:499-516`) is a `StreamGraphBranch` that reads chunks until one has `ToolCalls`. This is a leftover from the legacy `flow/agent/react` design, which needed a `StreamToolCallChecker` because some providers emit text before tool calls (`flow/agent/react/react.go:179`).
- Tool results are streamed per tool when the tool is a `StreamableTool`. Otherwise each tool emits one event when it completes (`adk/wrappers.go:844-874`). **No "tool call started" event exists.** The assistant message carrying the `ToolCalls` is the only signal.
- With retry, stream events reach the client in real time *before* the retry verdict. A rejected attempt ends with a `WillRetryError` injected into the stream (`adk/wrappers.go:347-365`, `adk/retry_chatmodel.go:67-90`). Clients have to handle "discard the partial output I just rendered".
- Every event stream should have `SetAutomaticClose()` (a finalizer), because unconsumed copies otherwise leak (`adk/interface.go:461-462`).

### 4.4 What is persisted

**Checkpoints are written only on interrupt and on cancel.** The compose runner calls `checkPointer.set` in exactly two places: `handleInterrupt` (`compose/graph_run.go:625`) and `handleInterruptWithSubGraphAndRerunNodes` (`:782`). The ADK Runner saves in `typedRunnerHandleIterImpl` when it sees an interrupt action or a `CancelError` (`adk/runner.go:295-330`). There is **no per-step or per-turn persistence**.

The compose checkpoint:

```go
// compose/checkpoint.go:108-119
type checkpoint struct {
	Channels       map[string]channel
	Inputs         map[string]any      // pending node inputs
	State          any                 // graph local state (e.g. *adk.State)
	SkipPreHandler map[string]bool
	RerunNodes     []string
	SubGraphs      map[string]*checkpoint
	InterruptID2Addr  map[string]Address
	InterruptID2State map[string]core.InterruptState
}
```

The ADK root payload wraps it:

```go
// adk/interrupt.go:210-221
type serialization struct {
	RunCtx *runContext           // RootInput, RunPath, Session{Values, Events}
	Info *InterruptInfo
	InfoDataSourceInterruptID string
	EnableStreaming bool
	InterruptID2Address map[string]Address
	InterruptID2State   map[string]core.InterruptState
}
```

The ChatModelAgent's inner compose checkpoint goes to an in-memory `bridgeStore` (`adk/interrupt.go:369-416`) under the fixed key `"adk_react_mock_key"`. The resulting bytes are then carried as the agent's `InterruptState` inside the outer checkpoint. Nested agents therefore produce **Russian-doll gob blobs**.

The store contract is minimal:

```go
// internal/core (re-exported as adk.CheckPointStore / compose.CheckPointStore)
type CheckPointStore interface {
	Get(ctx context.Context, checkPointID string) ([]byte, bool, error)
	Set(ctx context.Context, checkPointID string, checkPoint []byte) error
}
type CheckPointDeleter interface { Delete(ctx context.Context, checkPointID string) error }
```

### 4.5 Guarantees on crash, restart, idempotency, and ordering

- **Crash mid-run:** everything since the last `Runner.Run` call is lost. No checkpoint exists unless an interrupt or cancel happened, and restart means re-running from the caller's saved history. Eino offers **HITL pause/resume durability**, not **crash durability**.
- **Resume semantics:** resume **re-executes the interrupted node**. Nodes are not replayed from a log. `ToolsNode` records already-completed sibling tools in `toolsInterruptAndRerunState.ExecutedTools` so that only interrupted or canceled tools run again (`compose/tool_node.go:1164-1175, 1197-1246`). That gives partial idempotency for parallel tool batches. Nothing like idempotency keys is passed to tools.
- **Resume requires the same code:** the checkpoint holds node keys, graph structure, and gob-typed state. The agent must be rebuilt with a compatible configuration, and custom types in `any` fields need `schema.RegisterName[T]("name")` in `init()`. `SetRunLocalValue` probes gob encodability eagerly to catch this early (`adk/handler.go:423-451`).
- **Schema evolution** is painful. The code carries byte-level gob patching to migrate v0.8.0-v0.8.3 checkpoints (`adk/interrupt.go:252-288`, `adk/react.go:62-99`, `adk/chatmodel.go:1728+`). This is the clearest warning in the codebase against gob for durable state.
- **Ordering:** tool *result messages* are placed back in tool-call index order (`output[i]`, `compose/tool_node.go:1195-1243`). Tool *events*, however, are emitted in completion order from parallel goroutines. The model-output event always precedes that turn's tool events.
- **Max steps:** Pregel graphs default to `nodes+10` steps (`compose/graph.go:881-885`). ADK overrides this to `math.MaxInt` and uses `RemainingIterations` in state instead (§6).

---

## 5. Tool systems and their Go interfaces

### 5.1 Interfaces

These are the four tool interfaces quoted in §2.3. `ToolsNode` dispatches on them with type assertions in `convTools` (`compose/tool_node.go:595-680`), and adapts invoke↔stream both ways (`streamableToInvokable`, `invokableToStreamable`, `:806-852`).

### 5.2 Schema generation

`components/tool/utils` provides typed constructors built on generics and reflection:

```go
// components/tool/utils/invokable_func.go:33-36, 46, 143
type InvokeFunc[T, D any] func(ctx context.Context, input T) (output D, err error)
type OptionableInvokeFunc[T, D any] func(ctx context.Context, input T, opts ...tool.Option) (output D, err error)
func InferTool[T, D any](toolName, toolDesc string, i InvokeFunc[T, D], opts ...Option) (tool.InvokableTool, error)
func NewTool[T, D any](desc *schema.ToolInfo, i InvokeFunc[T, D], opts ...Option) tool.InvokableTool
// + InferStreamTool, InferEnhancedTool, InferEnhancedStreamTool, GoStruct2ToolInfo, GoStruct2ParamsOneOf
```

- The schema comes from `jsonschema.Reflector{Anonymous: true, DoNotReference: true}` over `T` (`invokable_func.go:118-133`) and is JSON Schema 2020-12. Struct tags follow `invopop/jsonschema` conventions (`jsonschema:"description=..."`).
- Arguments are decoded with `sonic` into `T`. The output `D` is JSON-encoded unless it is a string. Both steps can be overridden with `WithUnmarshalArguments` and `WithMarshalOutput` (`create_options.go`).
- `WithSchemaModifier(func(jsonTagName, reflect.Type, reflect.StructTag, *jsonschema.Schema))` supports custom tags.

### 5.3 Invocation pipeline

`ToolsNodeConfig` (`compose/tool_node.go:186-231`):

```go
type ToolsNodeConfig struct {
	Tools []tool.BaseTool
	ToolAliases map[string]ToolAliasConfig             // name + argument-key aliases (model-hallucination repair)
	UnknownToolsHandler func(ctx context.Context, name, input string) (string, error)
	ExecuteSequentially bool
	ToolArgumentsHandler func(ctx context.Context, name, arguments string) (string, error)
	ToolCallMiddlewares []ToolMiddleware               // {Invokable, Streamable, EnhancedInvokable, EnhancedStreamable}
}
```

Inside `ChatModelAgent`, the wrapping order, outermost first (`adk/chatmodel.go:352-358`), is:

1. event sender
2. `ToolsConfig.ToolCallMiddlewares`
3. deprecated `AgentMiddleware.WrapToolCall`
4. `Handlers[i].Wrap*ToolCall` (first registered is outermost)
5. callback injection
6. the tool itself

Each tool call's ctx carries its call ID (`compose.GetToolCallID(ctx)`) and an address segment `tool:<name>:<callID>` (`compose/tool_node.go:1008-1009`).

`ToolAliases` and `UnknownToolsHandler` are practical features that are easy to overlook. They repair common model mistakes: wrong tool name, wrong argument key, or a hallucinated tool.

### 5.4 Tool errors

**Default behavior: a tool error aborts the whole agent run.** `ToolsNode.Invoke` returns `fmt.Errorf("failed to invoke tool[name:%s id:%s]: %w", ...)` on the first non-interrupt error (`compose/tool_node.go:1205-1209`). That error propagates out of the graph and ends up as `AgentEvent.Err`. Sibling tools in the same batch have already run (the WaitGroup completed), but their results are discarded.

To feed errors back to the model, you wrap each tool:

```go
// components/tool/utils/error_handler.go:28, 42
type ErrorHandler func(context.Context, error) string
func WrapToolWithErrorHandler(t tool.BaseTool, h ErrorHandler) tool.BaseTool
```

Alternatively, use a `WrapInvokableToolCall` middleware. For agent loops this default is the wrong way round: most tool errors should go back to the model as observations.

### 5.5 Approval and confirmation

There is **no dedicated approval API**. Approval is built from the general interrupt primitives:

```go
// components/tool/interrupt.go:43, 69, 101, 142, 185
func Interrupt(ctx context.Context, info any) error
func StatefulInterrupt(ctx context.Context, info any, state any) error
func CompositeInterrupt(ctx context.Context, info any, state any, errs ...error) error
func GetInterruptState[T any](ctx context.Context) (wasInterrupted bool, hasState bool, state T)
func GetResumeContext[T any](ctx context.Context) (isResumeTarget bool, hasData bool, data T)
```

The pattern is:

1. On first execution, the tool returns `tool.Interrupt(ctx, "confirm?")`.
2. On resume, the tool runs **again from the top** and checks `GetInterruptState` and `GetResumeContext`.
3. If the tool is the resume target, it reads the approval data. If it is not the target (a sibling was resumed), it must **re-interrupt itself** to keep its pending state.

That third rule is a subtle contract every approval-gated tool has to implement correctly. It is documented in `adk/runner.go:130-146` and `components/tool/interrupt.go:149-184`.

### 5.6 MCP

The core repo contains **no MCP client**. MCP appears only as schema types for provider-hosted MCP (`schema/agentic_message.go:56-60`: `mcp_tool_call`, `mcp_tool_result`, `mcp_list_tools_result`, `mcp_tool_approval_request/response`; `schema/tool.go:95-113` `AllowedMCPTool`). Client-side MCP tool adapters live in `eino-ext`.

### 5.7 Dynamic tools, return-directly, and tool actions

- The tool list can change per run in `BeforeAgent` (`ChatModelAgentContext.Tools`, which rebuilds the graph) or per model call in `BeforeModelRewriteState` via `state.ToolInfos` (`adk/handler.go:79-106`, `:170-183`). The `toolsearch` middleware and `DeferredToolInfos` plus `model.WithToolSearchTool` support provider-native tool search.
- `ReturnDirectly map[string]bool` ends the loop with that tool's result as the final output (`adk/react.go:521-556`).
- `adk.SendToolGenAction(ctx, toolName, action)` lets a tool attach an `AgentAction` (Exit, Transfer, …) to its result event (`adk/react.go:274-285`).

---

## 6. Execution loop implementation

The core loop is a graph, `newReact` (`adk/react.go:354-561`). Step by step, for `*schema.Message`:

1. **Chain entry** (`adk/chatmodel.go:1158-1170`): a Lambda calls `GenModelInput(ctx, instruction, input)`. By default this adds a system message, runs it through FString formatting when session values exist, and appends the input messages (`:167-193`).
2. **Init node** (`react.go:363-372`): appends the input messages to `State.Messages`. State was created by `genReactState` with `RemainingIterations = MaxIterations` (default **20**, `:340-352`).
3. **ChatModel node** (`react.go:386-394`) has a `StatePreHandler`: if `RemainingIterations <= 0` it returns `ErrExceedMaxIterations` (`react.go:33`), otherwise it decrements. Exceeding the limit is a **hard error**, not a graceful stop.
4. **Model wrapper stack** (`adk/wrappers.go:56-79`, order documented at `adk/chatmodel.go:315-326`). `typedStateModelWrapper.Generate/Stream` **ignores its `input` argument** and reads `State.Messages`. It then:
   - runs `BeforeChatModel` and `BeforeModelRewriteState` handlers and persists the returned state;
   - calls the model through failover → retry → eventSender → user `WrapModel` handlers → callback injection → model;
   - appends the result to state;
   - runs `AfterModelRewriteState` and `AfterChatModel` and persists again (`adk/wrappers.go:1184-1307`).

   The persisted `State.Messages` is the source of truth. That is why middleware such as summarization and reduction can rewrite history durably.
5. **Branch** (`react.go:499-516`): if any stream chunk carries `ToolCalls`, go to `CancelCheck`. Otherwise go to the terminal node (`AfterAgent` when handlers exist, else `END`).
6. **CancelCheck** (`react.go:399-411`) is a safe point. If `CancelAfterChatModel` is pending, it returns a `StatefulInterrupt` that carries the model message.
7. **ToolsNode** (`react.go:436-440`): the pre-handler marks any return-directly call ID. The node runs tools in parallel (§5). The post-handler flushes a deferred return-directly event.
8. **AfterToolCalls** (`react.go:443-473`): appends tool messages to `State.Messages`, reusing the message IDs pre-generated by the event sender so that event messages and state messages share IDs, then runs `afterToolCallsHook`.
9. **AfterToolCallsCancelCheck** (`react.go:476-485`) is the `CancelAfterToolCalls` safe point.
10. **Loop or exit**: with return-directly tools configured, a branch goes to `ToolNodeToEndConverter` (which emits that tool's message as the final output) or back to `ChatModel`. Otherwise there is an unconditional edge back to `ChatModel` (`react.go:521-556`).

**Termination conditions:**

- a model response with no tool calls (normal);
- a return-directly tool was called;
- `ErrExceedMaxIterations`;
- a model or tool error (including retry exhaustion, `ErrExceedMaxRetries`);
- an interrupt (tool or cancel safe point);
- `ctx` cancellation, checked at each superstep (`compose/graph_run.go:251-258`);
- an `Exit` action (from `ExitTool` via `SendToolGenAction`), which ends the flow in `flowAgent` (`adk/flow.go:548-557`).

Compose-level `maxSteps` is set to `math.MaxInt` for this graph (`chatmodel.go:1170, 1176`).

**No-tools path:** if the agent has zero tools, `buildNoToolsRunFunc` builds a single-model-call chain instead of the ReAct graph (`adk/chatmodel.go:1002-1095`, selected at `:1377-1386`).

**Agentic path** (`M = *schema.AgenticMessage`) uses the same graph shape with `AddAgenticModelNode` and `AddAgenticToolsNode` (`react.go:620-800`). Per the doc comments, cancel monitoring on the model stream and retry are "not yet wired" for it (`adk/interface.go:449-452`).

**Where streaming runs through the loop:** `EnableStreaming` on the input selects `runnable.Stream` over `runnable.Invoke` (`chatmodel.go:1216-1220`). The graph then calls the model's `Stream`, and events tee off inside the wrapper (§4.3). The final output stream from `runnable.Stream` is usually just closed (`:1229-1231`), because consumers use the events rather than the return value.

---

## 7. Suspend/resume and error handling

### 7.1 Interrupt model

Interrupts are errors that carry a signal tree:

```go
// internal/core/interrupt.go:47-63
type InterruptSignal struct {
	ID string                 // uuid.NewString() (:105, :116)
	Address                   // []AddressSegment{ID, Type, SubID}
	InterruptInfo             // {Info any, IsRootCause bool}
	InterruptState            // {State any, LayerSpecificPayload any}
	Subs []*InterruptSignal
}
// user-facing view, :128-142
type InterruptCtx struct {
	ID string; Address Address; Info any; IsRootCause bool; Parent *InterruptCtx
}
```

- Addresses are built as execution descends. Segments have types agent / node / tool, and a tool segment uses the call ID as `SubID` to tell parallel same-name calls apart (`internal/core/address.go:69-77`). The string form looks like `agent:A;node:ToolNode;tool:search:call_123`.
- Leaf components raise `Interrupt` or `StatefulInterrupt`. Containers such as `ToolsNode`, graphs, and agent tools wrap child interrupts with `CompositeInterrupt(ctx, info, state, errs...)` (`compose/interrupt.go:110-240`, `components/tool/interrupt.go`, `adk/interrupt.go:59-160`). `ToolsNode` saves its own state (the input message and completed tool results) as the composite's state (`compose/tool_node.go:1246`).
- The Runner converts the signal into `AgentAction.Interrupted{Data, InterruptContexts}` and saves the checkpoint (`adk/runner.go:305-330`). The user picks `InterruptCtx.ID` values, which are **UUIDs, not address strings**, despite the comment at `internal/core/interrupt.go:129-131`, and resumes:

```go
iter, err := runner.ResumeWithParams(ctx, checkpointID, &adk.ResumeParams{
	Targets: map[string]any{interruptID: approvalPayload},
})
```

- On resume, `runnerLoadCheckPointImpl` (`adk/interrupt.go:223-250`) decodes the gob payload and calls `core.PopulateInterruptState` to put the ID→address and ID→state maps into `ctx`. `core.BatchResumeWithData` marks the targets. Each component then asks `GetInterruptState[T]` ("was I interrupted, and with what state?") and `GetResumeContext[T]` ("am I a resume target, and with what data?"). Composites continue so the resume signal reaches their children. Non-target leaves must re-interrupt.
- `Runner.Resume` (without params) means "resume everything, no data". Targets see `isResumeFlow=false` (`adk/runner.go:119-128`).
- `ChatModelAgentResumeData{HistoryModifier}` lets you edit `State.Messages` at resume time (`adk/chatmodel.go:792-796, 1610-1631`).
- Compose graphs also support static breakpoints: `WithInterruptBeforeNodes` and `WithInterruptAfterNodes` (`compose/interrupt.go:31-45`).

### 7.2 Serialization

Everything is **gob**. `schema.RegisterName[T](name)` wraps gob registration with stable names. Built-in types are registered with `_eino_*` names in `init()` functions (`compose/checkpoint.go:30-37`, `adk/react.go:77-99`, `adk/interrupt.go:198-206`). Implications:

- Every concrete type behind an `any` in state, session values, interrupt info, or tool interrupt state must be registered.
- Errors do not survive the round trip, so `WillRetryError` stores `ErrStr` and drops `err` (`adk/retry_chatmodel.go:67-81`).
- Streams in checkpointed events are concatenated into single messages before encoding (`adk/interface.go:146-177`).
- Compatibility across versions needs manual shims, including byte-level renaming of gob type names (§4.5).

### 7.3 Cancellation

There are two mechanisms:

1. **`context.Context`**: checked between supersteps (`compose/graph_run.go:251-258`) and handed to models and tools. Context cancellation produces an error and **no checkpoint**.
2. **`adk.WithCancel()`** (`adk/cancel.go:239-247`), which returns `(AgentRunOption, AgentCancelFunc)`. It is a structured, checkpointing cancel:

```go
// adk/cancel.go:44-67, 96, 180-191
type CancelMode int
const (
	CancelImmediate      CancelMode = 0
	CancelAfterChatModel CancelMode = 1 << iota
	CancelAfterToolCalls
)
type AgentCancelFunc func(...AgentCancelOption) (*CancelHandle, bool)
// options: WithAgentCancelMode, WithAgentCancelTimeout (escalates to immediate), WithRecursive
type CancelError struct {
	Info *AgentCancelInfo
	InterruptContexts []*InterruptCtx   // resumable!
	interruptSignal *InterruptSignal
}
```

A safe-point cancel becomes an interrupt at the `CancelCheck` nodes, so the canceled run is **resumable** from a checkpoint. `CancelImmediate` uses `compose.WithGraphInterrupt` to interrupt the running graph and treats in-flight tasks as rerun nodes (`compose/graph_run.go:273-283`). While a cancel is pending, business interrupts are "absorbed" into `CancelError` and fire again on resume (`adk/cancel.go:163-179`). This is about 1,100 lines of state machine with a CAS-based state enum (`adk/cancel.go:249-300`). It is powerful, and the test file names (`cancel_stream_race_test.go`, `cancel_edge_test.go`) show how hard it was to get right.

### 7.4 Retry and failover

```go
// adk/retry_chatmodel.go
type TypedModelRetryConfig[M MessageType] struct {
	MaxRetries  int
	ShouldRetry func(ctx context.Context, retryCtx *TypedRetryContext[M]) *TypedRetryDecision[M]
	IsRetryAble func(ctx context.Context, err error) bool   // Deprecated
	BackoffFunc func(ctx context.Context, attempt int) time.Duration  // default 100ms·2^n, cap 10s, +0–50% jitter
}
type TypedRetryDecision[M MessageType] struct {
	Retry bool
	RewriteError error                   // turn a "successful" bad output into a fatal error
	ModifiedInputMessages []M            // e.g. compress context, then retry
	PersistModifiedInputMessages bool    // write back into State.Messages
	// Backoff, RejectReason, ...
}
```

`ShouldRetry` can inspect **successful** outputs too, for example to reject a malformed response, which is a good design. `ModelFailoverConfig[M]` (`adk/failover_chatmodel.go:128+`) supplies `ShouldFailover(ctx, partialMsg, err)` and `GetFailoverModel(...)`, which can also rewrite input. It remembers the last model that succeeded. Retry covers **model calls only**. Tool retry is left to middleware.

### 7.5 Error types

| Error | Where |
|---|---|
| `ErrExceedMaxIterations` | `adk/react.go:33` |
| `ErrExceedMaxRetries`, `*RetryExhaustedError{LastErr}`, `*WillRetryError` | `adk/retry_chatmodel.go:33-90` |
| `*CancelError`, `ErrCancelTimeout`, `ErrExecutionEnded`, `ErrStreamCanceled` | `adk/cancel.go:180-236` |
| `compose.ErrExceedMaxSteps`, graph run errors, `*interruptError` / `ExtractInterruptInfo(err)` | `compose/graph_run.go:260`, `compose/interrupt.go:299-360` |
| `*safe.PanicErr` (panics recovered in the agent goroutine, the tool goroutines, and the runner) | `internal/safe`, `adk/chatmodel.go:1508-1512` |
| `*schema.SourceEOF` (named merged streams) | `schema/stream.go:59-73` |

Errors reach the caller **as events** (`AgentEvent.Err`), not as a returned `error`. The iterator just ends. Callers have to check `Err` on every event.

---

## 8. Strengths, weaknesses, and lessons for Dive

### 8.1 What Eino does well

1. **Per-request tools replacing mutable `BindTools`.** They learned this the hard way (`components/model/interface.go:73-103`). Dive should keep tools as request parameters, and keep model clients immutable and shareable.
2. **Persisted state as the source of truth, with a clean state-rewrite hook.** `BeforeModelRewriteState` and `AfterModelRewriteState` return a new state that becomes durable history. The docs explicitly steer message and tool edits there rather than into `WrapModel`, because edits in `WrapModel` are not persisted and break prompt caching (`adk/handler.go:170-183, 235-240`). This separation between **history transforms (durable)** and **call wrappers (ephemeral)** is the most valuable middleware lesson in the codebase.
3. **Middleware as an interface with a no-op base struct**, first registered = outermost, returning `(ctx, ..., error)`. The comment explaining why they moved off the struct-of-funcs `AgentMiddleware` (`adk/handler.go:113-139`) is worth reading. Dive's hooks should be interface-based and context-returning.
4. **Model-mistake repair in the tool layer:** `ToolAliases` (name and argument-key aliases), `UnknownToolsHandler`, and `ToolArgumentsHandler`, plus the `patchtoolcalls` middleware for dangling tool calls. These are small features with real production value.
5. **`ShouldRetry` that sees successful outputs** and can rewrite inputs (compress and retry) or reject output, plus failover with a sticky last-good model. This is a richer contract than error-only retry.
6. **Addressable interrupts across nesting levels**, with parallel tool calls kept apart by call ID and completed sibling tools kept across resume (`ExecutedTools`). If Dive supports approvals in parallel batches, this is the right shape: resume re-runs only the pending calls.
7. **Safe-point cancellation that produces a resumable checkpoint** (`CancelAfterChatModel` / `CancelAfterToolCalls`). "Stop after this model turn, and let me continue later" is a real UX need in chat and coding agents.
8. **Content-block `AgenticMessage`** and the shift toward agents-as-tools over transfer. Both match where providers and practice have gone.
9. **Stable message IDs shared by events and persisted state** (`adk/react.go:443-462`). UIs can reconcile streamed events with stored history.

### 8.2 What is awkward

1. **Two message models and generics throughout.** `TypedX[M]` plus an alias for every type (`Agent = TypedAgent[*schema.Message]`, `ChatModelAgentConfig = TypedChatModelAgentConfig[*schema.Message]`, …) doubles the API surface. The implementation is full of `any(x).(T)` switches and `panic("unreachable")` (e.g. `adk/chatmodel.go:1117-1120`, `adk/runner.go:163-201`), and the agentic path has feature gaps. **Lesson:** pick one content-block message model from the start, and don't make the whole agent API generic over message types.
2. **Graph engine underneath a simple loop.** The ReAct loop is 7 to 9 graph nodes with branches, state pre/post handlers, and a compile step **on every `Run`**. Streaming is then turned into a side channel anyway (Copy(2), full concatenation). Stack traces, debugging, and reasoning about behavior all get harder. **Lesson:** Dive should implement the agent loop as plain Go (`for { call model; if no tool calls break; run tools }`) and offer graphs or workflows as a separate optional layer.
3. **Stringly-typed, reflection-heavy core.** Tools use JSON string in and string out. Callbacks take `any` and need `Conv*` helpers. Graph type checks happen by reflection at compile time. State is found by type via `ProcessState[S]`, and the wrong `S` is a runtime error. Implementation-specific options are `any`.
4. **`EnhancedInvokableTool` method-name collision** means one type can't implement both variants. There are four tool interfaces, and each needs its own middleware method (`Wrap{Invokable,Streamable,EnhancedInvokable,EnhancedStreamable}ToolCall`), so middleware authors write four methods. **Lesson:** one tool interface with a rich result type, such as `Call(ctx, input) (*ToolResult, error)` where `ToolResult` has multimodal parts and an `IsError` flag, with streaming as an optional side capability.
5. **Tool errors abort the run by default** (`compose/tool_node.go:1208`). **Lesson:** in Dive, tool errors should become `is_error` tool results by default. Reserve run-fatal failures for a typed sentinel.
6. **Gob checkpoints.** Byte-patching gob type names for migrations (`adk/interrupt.go:252-288`) is a warning sign. Registration requirements leak into user code (`schema.RegisterName` in `init()`). **Lesson:** use JSON (or versioned JSON) for anything durable: explicit schema versions, human-inspectable, and portable to Postgres `jsonb`.
7. **Durability only at interrupts.** No per-turn persistence means no crash recovery, and resume depends on rebuilding an identical graph. **Lesson for Dive:** persist an append-only event and message log per turn (after each model response and each tool result), so crash recovery and HITL resume use the same mechanism. That is "resume from the last committed step", not "re-enter the node".
8. **Unbounded event channel with no backpressure**, plus required `SetAutomaticClose` finalizers to avoid stream leaks. With `Go ≥ 1.23`, Dive can use `iter.Seq2[*Event, error]` for a pull-based, backpressured stream, or a callback. It should never rely on finalizers.
9. **Errors inside the event stream.** The iterator carries `Err` in events and has no terminal error. Callers can easily miss failures. **Lesson:** return a terminal result/error from the run (e.g. `Response, error`), with events as progress.
10. **Too many ways to do the same thing.** `Middlewares` vs `Handlers`, `IsRetryAble` vs `ShouldRetry`, `MultiContent` vs `UserInputMultiContent`/`AssistantGenMultiContent`, `ChatModel.BindTools` vs `ToolCallingChatModel.WithTools` vs per-call `model.WithTools`, `flow/agent/react` vs `adk`. Deprecations pile up because the public API was frozen early. **Lesson:** keep Dive's public surface small until the agent patterns settle.
11. **HITL contract complexity.** Tools must call `GetInterruptState` and `GetResumeContext` and re-interrupt when they are not the target. Resume re-executes tool code from the top, so side effects before the interrupt point run twice. **Lesson:** have the framework own approval as a first-class step (for example, an `ApprovalPolicy` evaluated *before* invoking a tool, with the pending call persisted and the decision injected on resume). Tool authors then never deal with re-entry.
12. **Instruction templating on by default.** When session values exist, `Instruction` is FString-formatted, and literal `{` in the prompt then fails (`adk/chatmodel.go:173-183`). The error message even advises a workaround. Templating should be opt-in.

### 8.3 Concrete recommendations for Dive

| Area | Recommendation |
|---|---|
| Model interface | `Generate(ctx, *Request) (*Response, error)` and `Stream(ctx, *Request) (StreamIterator, error)`, with tools, tool choice, and options **in the request**. Immutable clients. One content-block message type (Anthropic/Responses-style) with provider extension structs kept out of core. |
| Agent loop | A plain Go loop with an explicit `MaxTurns`. Hitting the limit should be a **graceful terminal state** reported in the result (`StopReason: max_turns`), unlike Eino's hard `ErrExceedMaxIterations`. No graph compile per run. |
| Hooks | Borrow Eino's split: (a) **state/history transforms** that persist (`BeforeModel(ctx, *State) error`), and (b) **call wrappers** that don't (`WrapModel`, `WrapTool`). An interface with a no-op embeddable base, first registered = outermost. One `WrapTool` over a single tool call signature, not four. |
| Tools | One interface; typed generic helper `dive.Func[In, Out](name, desc, fn)` with jsonschema reflection (Eino's `InferTool` is a good model); errors → `is_error` results by default; built-in `UnknownTool` / alias repair; parallel execution with a **concurrency limit** and results in call order. |
| Events | A `ToolCallStarted` event (Eino lacks one), model deltas, `ToolResult`, `TurnComplete`, `Retrying` (with discard semantics). Stable message IDs shared between events and persisted history, as Eino does. |
| Durability | Append-only per-step log (JSON), written after each model response and each tool result. The same log serves crash recovery, HITL resume, and replay. Idempotency keys (`runID/turn/callID`) available to tools via ctx. |
| HITL | A first-class approval step before tool execution, with the pending call persisted. Resume by call ID, not by opaque UUID or address. Allow approving parallel calls independently, and don't re-run approved or completed siblings (the one piece of Eino's `ExecutedTools` design to copy). |
| Cancellation | Standard `ctx` for hard cancel. A small `StopAfterTurn` or `StopAfterTools` request for a graceful, resumable stop, backed by the step log instead of a separate interrupt state machine. |
| Retry | Retry predicate over `(response, err)` that can reject successful-but-bad outputs and optionally rewrite input (context compression). Failover to alternate models. Surface retries as events so UIs can discard partial output. |
| Multi-agent | Agents as tools (sub-agent returns a result to the parent) as the primary pattern. Skip transfer/handoff with shared history; Eino's own docs deprecate it. |
| Iteration API | `iter.Seq2[*Event, error]` or a callback. Bounded buffering. No finalizer-based stream cleanup. A terminal `(*Response, error)` from `Run`. |
| Serialization | JSON with explicit version fields. No global type registry for users to populate. |

**Bottom line:** Eino shows that **interrupt/resume, safe-point cancel, state-persisting middleware, and model-mistake repair** are what separate a production agent runtime from a demo. It also shows the cost of building them on a general graph engine with gob serialization and a generic-over-message-type API: a large, layered, deprecation-heavy surface. Dive can deliver the same capabilities with a plain-Go loop, one message model, a JSON step log, and a much smaller API.
