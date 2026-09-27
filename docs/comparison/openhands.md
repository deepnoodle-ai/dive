# OpenHands (Software Agent SDK + Agent Server + Agent Canvas)

**Date:** 2026-09-26
**Purpose:** Study the most widely used open-source coding-agent platform and compare it with Dive v2's conceptual model, and with Eino, ADK-Go, MAF-Go and Nvoken ([README.md](README.md)).

| Repo | Commit analyzed | Role | Size |
|---|---|---|---|
| `OpenHands/software-agent-sdk` | `d77ada7a` (2026-09-26) | **Primary subject.** Python SDK, tools, workspaces, Agent Server, TS client | `openhands-sdk` 295 files / ~75k LOC; `openhands-tools` 93 / ~17k; `openhands-workspace` 14 / ~3k; `openhands-agent-server` 83 / ~30k; `clients/typescript` 98 files / ~47k; 726 test files |
| `OpenHands/OpenHands` | `47a10808d` (2026-09-25) | "Agent Canvas" React/TS frontend. Studied here as an API consumer | ~1,300 TS/TSX files |

Paths are prefixed `sdk/` for `software-agent-sdk/openhands-sdk/openhands/sdk/`, `server/` for `software-agent-sdk/openhands-agent-server/openhands/agent_server/`, `tools/` for `software-agent-sdk/openhands-tools/openhands/tools/`, and `canvas/` for `OpenHands/src/`. Line numbers are at the commits above.

---

## 1. Overview

**What it is today.** "OpenHands" is now four repositories with a strict ownership split, stated in `software-agent-sdk/AGENTS.md`:

- **software-agent-sdk.** Owns agent and tool behavior, conversations, workspaces, events, and "the canonical REST/WebSocket API". It publishes four Python packages (`openhands-sdk`, `openhands-tools`, `openhands-workspace`, `openhands-agent-server`) and `clients/typescript` (`@openhands/typescript-client` 1.49.6). The TS client mirrors the server's OpenAPI.
- **OpenHands/OpenHands (Agent Canvas).** Only a React UI now. It holds frontend state, backend selection and local-stack orchestration.
- **OpenHands/automation.** Scheduling, webhooks, run history, dispatch, sandbox lifecycle. "This repository executes the dispatched conversations."
- **OpenHands/extensions.** Skills, plugins and the marketplace.

The documented flow is **SDK/Agent Server → OpenAPI → `clients/typescript` → Canvas**. The same `Conversation` API runs in-process (`LocalConversation`) or against a remote Agent Server (`RemoteConversation`). The factory picks one by workspace type: `sdk/conversation/conversation.py:155` returns a `RemoteConversation` when `workspace` is a `RemoteWorkspace`, and a `LocalConversation` otherwise.

**Philosophy.** It is a **harness, not a framework**: one opinionated agent (`Agent`) with a big default prompt, file editor, terminal, task tracker, browser, and subagents. It puts engineering into running coding agents for long periods: a condenser, stuck detection, security analyzers, secret masking, conversation forking, and sandboxed workspaces. Its extension surface follows Claude Code conventions (`.agents/skills`, shell hooks such as `PreToolUse`/`Stop`, and Markdown subagents in `.agents/agents/*.md`). Notable engineering rules in `AGENTS.md`:
- persisted settings and events are compatibility surfaces, guarded by golden fixtures and schema-version migrations;
- every LLM request must start with a `system` message;
- "Cancellation must stop underlying work"; tasks and processes must have owners.

**What a user writes** (`README.md`):

```python
agent = Agent(llm=LLM(model="gpt-5.5", api_key=...),
              tools=[Tool(name=TerminalTool.name), Tool(name=FileEditorTool.name)])
conversation = Conversation(agent=agent, workspace=os.getcwd())
conversation.send_message("Write 3 facts about the current project into FACTS.txt.")
conversation.run()
```

Note what is absent: no graph, no runner, no session service. The nouns are **Agent** (stateless, serializable config), **Conversation** (the aggregate, with an event log), **Workspace** (where tools act) and **Event**.

**Package layout (`openhands-sdk`).**
- `agent/`: `Agent`, `ACPAgent`, the parallel executor, stream context, response dispatch.
- `conversation/`: local and remote implementations, `ConversationState`, `EventLog`, the stuck detector, the goal loop.
- `event/`: event types.
- `llm/`: LiteLLM wrapper, retries, fallback, routers, non-native function calling, profiles.
- `tool/`: `ToolDefinition`, schemas, registry, built-ins, client tools.
- `mcp/`, `context/`: condensers, the `View` projection, prompts, `AgentContext`/skills.
- `security/`, `hooks/`, `skills/`, `plugin/`, `subagent/`, `critic/`, `workspace/`, `settings/`, `profiles/`, `observability/`.

---

## 2. Core packages and types

### Agent: frozen, serializable configuration

`sdk/agent/base.py:101`:

```python
class AgentBase(DiscriminatedUnionMixin, ABC):
    """Agents are stateless and should be fully defined by their configuration."""
    model_config = ConfigDict(frozen=True, arbitrary_types_allowed=True)
    llm: LLM
    tools: list[Tool]                      # specs: {"name": "TerminalTool", "params": {}}
    mcp_config: dict[str, MCPServer]
    agent_context: AgentContext | None     # skills, repo rules, secrets, prompt suffixes
    system_prompt: str | None; system_prompt_filename: str = "system_prompt.j2"
    condenser: CondenserBase | None
    critic: CriticBase | None
    tool_concurrency_limit: int = 1        # ge=1; >1 enables parallel tool calls
    _tools: dict[str, ToolDefinition] = PrivateAttr(...)   # materialized at init
```

- **Tools are specs, not objects.** `Tool{name, params}` is resolved through a process-global registry (`sdk/tool/registry.py:32` `_REG`, `register_tool` at `:113`) when the conversation initializes the agent. The whole agent is therefore JSON. It is stored in `base_state.json` and sent to the Agent Server in `POST /conversations`.
- **Resume checks compatibility; it does not version.** `AgentBase.verify` (`base.py:671`) requires the same agent class and **tools may only be added, never removed**, "because the LLM may have already been told about them". LLM, prompt, condenser and context may change freely. There is no hash or version pin, which is a weaker form of Dive's "configuration is a value".
- **Two concrete agents.**
  - `Agent`, the LLM tool loop (`sdk/agent/agent.py:374`, 1.5k lines, composed from `CriticMixin` and `ResponseDispatchMixin`).
  - `ACPAgent`, which delegates each turn to an external ACP agent (Claude Code, Codex, Gemini CLI) running as a subprocess (`sdk/agent/acp_agent.py`, 4.7k lines).
- **The step contract** (`base.py:631`): `step(conversation, on_event, on_token) -> None`, which "mutates state in-place". Each step either records a condensation, or makes one LLM call and then runs its tool batch. The conversation loop calls `step` repeatedly until the execution status changes. **Every sync method has an async twin** (`astep`, `arun`, `acompletion`, `acondense`...). The default async version runs the sync one in a thread.

### Conversation and ConversationState: the aggregate

`sdk/conversation/state.py:82`. `ConversationState` is a Pydantic model whose public fields are persisted to `base_state.json` and whose events live in `events/`:

```python
class ConversationState(OpenHandsModel):
    id: ConversationID
    agent: AgentBase                        # persisted config
    workspace: BaseWorkspace
    max_iterations: int = 500
    execution_status: ConversationExecutionStatus = IDLE
    confirmation_policy: ConfirmationPolicyBase = NeverConfirm()
    security_analyzer: SecurityAnalyzerBase | None
    activated_knowledge_skills / activated_path_rules / invoked_skills: list[str]
    blocked_actions: dict[str, str]         # action_id -> reason (PreToolUse hooks)
    blocked_messages: dict[str, str]        # message_id -> reason (UserPromptSubmit hooks)
    leaf_event_id: EventID | None           # HEAD of the conversation *tree*
    stats: ConversationStats; secret_registry: SecretRegistry
    agent_state: dict[str, Any]             # "always reassign to trigger autosave"
    _events: EventLog; _view: View          # cached projection of the active branch
    _lock: FIFOLock
```

`ConversationExecutionStatus` (`state.py:48`) is the lifecycle: `IDLE, RUNNING, PAUSED, WAITING_FOR_CONFIRMATION, FINISHED, ERROR, STUCK, DELETING`. It is one enum used for **both** the run state and the reason the run stopped. There is no separate stop reason. `MaxIterationsReached` or `MaxBudgetReached` show up only as a `ConversationErrorEvent.code`.

**Hidden autosave.** `__setattr__` (`state.py:596`) persists `base_state.json` on every public field assignment. Inside `with state:` the writes are batched until exit. Each change also emits a `ConversationStateUpdateEvent(key, value)` to subscribers. This is how the UI learns about status changes.

**The conversation is a tree.** Every event has `parent_id`. `leaf_event_id` is a movable HEAD, and `navigate_to(event_id)` and `fork()` (`local_conversation.py:788, 925`) re-root the active branch. `active_branch()` is `path_to_root(leaf)`. None of the Go frameworks surveyed has this.

### Events: one log, discriminated by class name

`sdk/event/base.py`:

```python
class Event(DiscriminatedUnionMixin, ABC):
    model_config = ConfigDict(extra="forbid", frozen=True)
    id: EventID = Field(default_factory=lambda: str(uuid.uuid4()))
    timestamp: str = Field(default_factory=lambda: datetime.now().isoformat())
    source: Literal["agent", "user", "environment", "hook"]
    parent_id: EventID | None

class LLMConvertibleEvent(Event, ABC):
    def to_llm_message(self) -> Message: ...
```

| Event | Key fields | Role |
|---|---|---|
| `SystemPromptEvent` | `system_prompt`, `tools: list[ToolDefinition]`, `dynamic_context` | First event. Static prompt kept separate from dynamic context for cross-conversation caching |
| `MessageEvent` | `llm_message: Message`, `llm_response_id`, `activated_skills`, `extended_content`, `sender`, `critic_result` | User, agent, or **environment/hook-synthesized** messages |
| `ActionEvent` | `action: Action \| None`, `tool_name`, `tool_call_id`, `tool_call`, `llm_response_id`, `thought`, `reasoning_content`, `thinking_blocks`, `responses_reasoning_item`, `security_risk`, `summary`, `critic_result` | **One per tool call**. Calls are regrouped into one assistant message by `llm_response_id` (`base.py` `events_to_messages`). `action=None` means validation failed |
| `ObservationEvent` | `observation`, `action_id`, `tool_call_id` | Tool result |
| `UserRejectObservation` | `rejection_reason`, `rejection_source` (user/hook), `action_id` | Denial |
| `AgentErrorEvent` | `error`, `tool_call_id`, `classification` | Tool or scaffold error **sent to the model** as the tool result |
| `Condensation` / `CondensationRequest` / `CondensationSummaryEvent` | `forgotten_event_ids`, `summary`, `summary_offset` | Non-destructive compaction |
| `ConversationStateUpdateEvent` | `key`, `value` | State sync (`full_state`, `execution_status`, `stats`, `goal`) |
| `ConversationErrorEvent` | `code`, `detail`, `classification` | Conversation-level failure |
| `PauseEvent`, `InterruptEvent`, `HookExecutionEvent`, `LLMCompletionLogEvent`, `TokenEvent`, `ACPToolCallEvent` | — | Control and telemetry |
| `StreamingDeltaEvent` | `content`, `reasoning_content` | Published, **never persisted** |

- **Polymorphism.** `DiscriminatedUnionMixin` (`sdk/utils/models.py:197`) serializes a `kind` field equal to the **Python class name** and resolves it through the live subclass hierarchy. This is runtime type discovery, the same family as MAF-Go's `linkname` registry. `tool/client_tool.py:27-40` shows the cost: dynamically created `ClientAction_<name>` classes "register process-globally", and registering the same one twice "breaks event deserialization", so the code caches the generated types.
- **Classified errors.** `ErrorClassification{kind: FailureKind, retryable, user_action: none|retry|settings}` (`sdk/event/error_classification.py`) is "the only failure metadata that crosses the event/API boundary". Kinds are `auth, quota, rate_limit, config, transient, agent_action, internal, unknown`. It is a small, closed vocabulary, which is good.

### LLM abstraction: LiteLLM underneath

`sdk/llm/llm.py:222`, `class LLM(BaseModel, RetryMixin, NonNativeToolCallingMixin)`, 3.4k lines:

- **Transport.** Calls go through LiteLLM `completion`/`responses`. `generate()` (`:1580`) dispatches to Chat Completions or the Responses API according to `uses_responses_api()`. `LLMResponse{message: Message, metrics: MetricsSnapshot, raw_response: ModelResponse | ResponsesAPIResponse}`.
- **Message model** (`sdk/llm/message.py:219`). It is OpenAI chat-shaped (`role: user|system|assistant|tool`, `content: [Text|Image]`, `tool_calls`, `tool_call_id`) **plus one side field per vendor**: `reasoning_content`, `thinking_blocks` ("Anthropic-specific... not normalized by LiteLLM") and `responses_reasoning_item` (OpenAI). This is the opposite of Dive's `Opaque{Provider, Type, Raw}` rule: each new provider feature adds a field.
- **Retries.** Tenacity with `num_retries=5`, `retry_min_wait=8`, `retry_max_wait=64`, `retry_multiplier=8` (`:349-352`). They apply to `APIConnectionError, RateLimitError, ServiceUnavailableError, Timeout, InternalServerError, LLMNoResponseError` (`:148`), but not to quota exhaustion, "so that... fallback to an alternate model happens immediately" (`:1100`). `FallbackStrategy` (`llm/fallback_strategy.py:39`) switches to alternate LLMs once retries are exhausted.
- **Typed errors.** `sdk/llm/exceptions/types.py` defines `LLMContextWindowExceedError`, `LLMMalformedConversationHistoryError`, `LLMContentPolicyViolationError`, `FunctionCallValidationError`, `LLMAuthenticationError`, `LLMRateLimitError`, and others, mapped from LiteLLM.
- **Other pieces:**
  - `NonNativeToolCallingMixin`: prompt-based tool calling for models without native tools (`mixins/fn_call_converter.py`, 963 lines).
  - `RouterLLM`: multimodal and random routers.
  - `LLMRegistry`, named **profiles** and `switch_llm`/`switch_profile` mid-conversation.
  - Subscription auth (ChatGPT/Codex OAuth, `llm/auth/openai.py`).
  - Cost and metrics (via LiteLLM pricing), and `LLMCallContext{prompt_cache_key, session_id}`, threaded explicitly so the frozen `LLM` can be shared across conversations.
- **Streaming.** A `stream: bool` field plus an `on_token` callback that receives raw LiteLLM chunks. If streaming is requested with no callback, it "degrades gracefully to non-streaming" (`:1685`).

### Tools: Action → Executor → Observation

`sdk/tool/tool.py:347` and `sdk/tool/schema.py`:

```python
class ToolDefinition[ActionT, ObservationT](DiscriminatedUnionMixin, ABC):
    name: ClassVar[str]                       # derived: TerminalTool -> "terminal"
    description: str
    action_type: type[Action]                 # Pydantic model = input JSON schema
    observation_type: type[Observation] | None
    annotations: ToolAnnotations | None       # MCP hints: readOnly/destructive/idempotent/openWorld
    meta: dict[str, Any] | None
    executor: ToolExecutor | None             # runtime-only, excluded from dumps
    response_schema: ResponseSchema | None
    @classmethod
    def create(cls, conv_state, **params) -> Sequence[Self]  # factory; may return several tools
    def declared_resources(self, action) -> DeclaredResources # for lock-based parallelism

class ToolExecutor[ActionT, ObservationT](ABC):
    def __call__(self, action: ActionT, conversation: LocalConversation | None) -> ObservationT
    def close(self) -> None
    def interrupt(self) -> None               # called from another thread on interrupt

class Observation(Schema, ABC):
    content: list[TextContent | ImageContent]
    is_error: bool = False
```

`ToolDefinition.__call__` (`:607`) runs the executor, coerces the output to `observation_type`, and **masks registered secret values** in every observation ("masking once keeps a new tool covered by default").

- **Built-ins** (`sdk/tool/builtins`): `finish`, `think`, `invoke_skill`, `switch_llm`, `classify_and_switch_llm`, `vision_inspect`.
- **Shipped tools** (`openhands-tools`): terminal, file_editor, apply_patch, glob, grep, browser_use, task_tracker, planning_file_editor, delegate, task (subagents), ask_oracle, tom_consult.

### Workspace

`sdk/workspace/base.py:27`: `BaseWorkspace{working_dir}` with `execute_command`, `file_upload/download`, `git_changes/diff` and `pause/resume`. Implementations:
- `LocalWorkspace`;
- `RemoteWorkspace` (talks to an Agent Server);
- in `openhands-workspace`: `docker`, `apptainer`, `cloud`, `remote_api` and `agent_sandbox` (Kubernetes).

The sandbox **is** a workspace that runs an Agent Server, so "remote execution" means the whole agent loop runs inside the sandbox, next to the tools.

### Security analyzer and confirmation policy

`sdk/security/`. Risk is `UNKNOWN|LOW|MEDIUM|HIGH`.

- **The risk usually comes from the model itself.** When a security analyzer is set, `_get_tool_schema` (`tool.py:697`) injects a `security_risk` enum parameter (and always a `summary` parameter) into **every non-read-only tool's schema**. `LLMSecurityAnalyzer` just returns `action.security_risk`.
- **Other analyzers:** `PatternSecurityAnalyzer` and `PolicyRailSecurityAnalyzer` (defense in depth, with shell AST parsing), `GraySwanAnalyzer`, `ToolShieldLLMSecurityAnalyzer`, and `EnsembleSecurityAnalyzer`. The ensemble takes the worst concrete risk, and a child that raises counts as **HIGH (fail-closed)**.
- **Policies:** `ConfirmationPolicyBase.should_confirm(risk)`, with `NeverConfirm`, `AlwaysConfirm` and `ConfirmRisky{threshold=HIGH, confirm_unknown=True}` (`confirmation_policy.py`).

### Condensers and the View projection

- **Condensers.** `CondenserBase.condense(view) -> View | Condensation` (`sdk/context/condenser/base.py:33`). `RollingCondenser` splits this into `condensation_requirement() -> HARD|SOFT|None` and `get_condensation()`. `LLMSummarizingCondenser` defaults: `max_size=240` events, `keep_first=2`, `hard_context_reset_max_retries=5`.
- **The View.** `View` (`sdk/context/view/view.py:22`) is the provider projection. `View.from_events` replays events: a `Condensation` removes `forgotten_event_ids` and inserts the summary at `summary_offset`; a `CondensationRequest` sets a flag; everything that isn't LLM-convertible is skipped.
- **View properties** (`context/view/properties/`): `batch_atomicity`, `tool_call_matching`, `observation_uniqueness`, `tool_loop_atomicity`. Each property yields **manipulation indices** (positions where history may be cut without breaking provider invariants, such as Anthropic's rule that a thinking block must lead a tool loop) and an `enforce()` fallback that drops events and logs a warning. Condensers may only cut at the intersection of all indices. This is the most transferable idea in the SDK.

### Other notable types

- **`HookConfig`.** Claude-Code-style shell hooks (`PreToolUse, PostToolUse, UserPromptSubmit, SessionStart, SessionEnd, Stop`) run with JSON on stdin (`sdk/hooks/types.py:9`).
- **`SecretRegistry`.** Env injection for commands, masking of output, and a `StreamOutputMask` that holds back a partial secret split across stream chunks.
- **`StuckDetector`.** Covered in §6.
- **`CriticBase`.** An optional evaluator that attaches `critic_result` to actions and messages, and can drive "iterative refinement" after `finish`.
- **`GoalController`** (`conversation/goal/controller.py`). A `/goal` loop in which an LLM judge decides after each run whether to continue with a follow-up message. The result is `GoalOutcome{status: complete|capped, iterations, verdict}`.
- **Subagents.** Registered from Markdown files, plugins or code (`sdk/subagent/`). `TaskTool` runs each as a child `LocalConversation` persisted under the parent's directory and **resumable by task ID** (`tools/task/manager.py:132-203`).

---

## 3. Key concepts and how they relate

```text
                 ┌──────────────── Agent (frozen config, JSON) ────────────────┐
                 │ LLM ─ tools[Tool spec] ─ mcp_config ─ condenser ─ context   │
                 └───────────────┬─────────────────────────────────────────────┘
                                 │ step(conversation, on_event, on_token)
┌──────────────────────── Conversation (Local | Remote) ────────────────────────┐
│ run()/arun(): while status allows → stuck check → agent.step → limits         │
│ send_message · pause · interrupt · reject_pending_actions · fork · navigate  │
│                                                                                │
│  ConversationState ──(autosave on setattr)──► base_state.json                  │
│    execution_status, confirmation_policy, security_analyzer, secrets, HEAD    │
│  EventLog ──(append under file lock)──► events/event-00042-<uuid>.json        │
│    tree via parent_id; active_branch = path_to_root(HEAD)                      │
│  View (cached, incremental) = project(active_branch, Condensations, props)    │
└───────┬───────────────────────────────┬──────────────────────────────┬─────────┘
        │ on_event chain                 │ on_token / on_stream          │ tools act on
        ▼                                ▼                               ▼
 visualizer → persist → hooks → user callbacks      StreamContext            Workspace
 (Agent Server: AsyncCallbackWrapper → PubSub)       (item_id, attempt,       Local | Docker |
        │                                             order; masked)          K8s | Cloud
        ▼                                                │
 WebSocket /sockets/events (legacy, Canvas)  ◄───────────┤
 WebSocket /sockets/session (seq cursor, durable/delta)  ◄┘
```

**Relationships that matter:**

1. **The event log is the record. The message list is a projection.** This is Dive v2 principle 1, already in production.
   - `Agent._step` never keeps its own transcript. It reads `state.view`, which is maintained incrementally, so a linear append costs O(k) (`state.py:340`). That fixes ADK-Go's O(n) rescan on every step.
   - Tool batches are re-grouped into assistant messages by `llm_response_id` at projection time.
2. **Conversation, not Turn, is the aggregate.** There is no turn noun. The run boundary is `run()`, and turn state is inferred from `execution_status` plus unmatched `ActionEvent`s (`get_unmatched_actions`, `state.py:677`). "Pending confirmation", "in flight at crash" and "interrupted" are all the same query.
3. **Callbacks are the only extension channel, and they can control.**
   - `_on_event` composes, in order: the visualizer, `_default_callback` (persist), and the user callbacks (`local_conversation.py:417-455`).
   - Hooks sit on the same chain. A `PreToolUse` hook blocks an action by writing `state.blocked_actions[action_id]`, which `_ActionBatch.prepare` pops before dispatch (`agent.py:238-245`).
   - This is exactly what Dive v2 principle 5 ("observation cannot control") forbids.
4. **Agent config travels with the conversation.** `base_state.json` holds the agent, so a server can resume a conversation with `agent=None` and adopt the persisted one (`local_conversation.py:390-410`).
5. **Local and remote share one event vocabulary.** `RemoteConversation` mirrors the server's events into a `RemoteEventsList`. The TS client and Canvas consume the same JSON (`kind`-discriminated).

---

## 4. Execution, durability, and guarantees

### What is persisted, and when

| Artifact | Where | Granularity | Write |
|---|---|---|---|
| Events | `events/event-{idx:05d}-{uuid}.json` (`persistence_const.py`) | **One file per event**, `atomic_write_text` | `EventLog.append` (`event_store.py:188`) under a per-conversation `flock` (30 s timeout). It re-syncs from disk if another process wrote (length-marker file), and **rejects duplicate event IDs** and unknown `parent_id`s |
| State snapshot | `base_state.json` | Whole model | On every public field change (`state.py:596`), batched inside `with state:` |
| Server metadata | `meta.json`, settings, secrets, workspaces | JSON files with fcntl/msvcrt locks | `server/persistence/store.py` |

There is **no database**: everything is files behind a `FileStore` (`LocalFileStore`, `InMemoryFileStore`). The `EventLog` docstring warns that `flock` "does NOT work reliably on NFS".

**Commit order.** `_default_callback` persists **before** the caller's callbacks run, "so no subscriber is told about an event that is not on disk yet" (`local_conversation.py:433-440`). In the server, subscribers are fed by an `AsyncCallbackWrapper` after persistence. The new session socket warns if it ever sees `event_published_before_persist` (`server/session_socket.py:205-222`).

### Intent before effect: mostly yes

This is where OpenHands is ahead of Eino, ADK-Go and MAF-Go. In `_handle_tool_calls` (`agent/response_dispatch.py:145`), every tool call becomes an `ActionEvent` that is **emitted, and so persisted, before any tool runs**. Only then does `_execute_actions` dispatch the batch. A persisted `ActionEvent` without a matching observation is therefore a recorded intent: the tool may have run.

The gaps against Dive's "intent before effect, result before advance":

- **Results are persisted per batch, not per call.** `_ActionBatch.prepare` executes the whole batch, and only then does `emit` write observations, in call order (`agent.py:229-338`). If the process crashes after tools A and B finish while C is running, **A's and B's results are lost**.
- **No `started` marker.** Nothing distinguishes "intent recorded, never dispatched" from "dispatched, may have run". Nvoken's `running` record does, and so does Dive's `tool_started`.
- **The model call has no intent record.** There is no `model_requested` step. A crash during the LLM call leaves nothing behind, which is harmless for correctness but invisible to audit and billing.
- **Two files, not atomic together.** An event append and the `leaf_event_id` autosave are separate writes. The server's crash-recovery code notes that "the persisted HEAD can lag this action" (`server/event_service.py:1219-1225`).

### Crash and resume

**Agent Server** (`server/event_service.py:1195-1240`). A conversation loaded with status `RUNNING` is treated as crashed:
1. The status becomes `ERROR`.
2. The code scans the **full log** for unmatched actions.
3. For **the first** unmatched action only, it appends `AgentErrorEvent("A restart occurred while this tool was in progress... did not complete", classification=INTERNAL)`, parented explicitly to the action.

**SDK only (`LocalConversation`).** There is no recovery step. A persisted `RUNNING` status stays `RUNNING`, and the next `run()` enters `Agent._step`, whose first branch is:

```python
pending_actions = ConversationState.get_unmatched_actions(state.active_branch())
if pending_actions:   # "Confirmation mode: Executing %d pending action(s)"
    self._execute_actions(conversation, pending_actions, on_event)
```

This "implicit confirmation" path (`agent.py:652-660`) is how an approved confirmation resumes. It also means that, as the code reads:
- a plain SDK resume **re-executes every in-flight tool**;
- a server resume after a crash mid-batch marks the first call failed and **re-executes the rest** on the next run.

Nothing consults `idempotentHint`/`readOnlyHint` first, even though `ToolAnnotations` carries them. The effect is at-least-once tool execution, with no Nvoken-style `RetryIfSafe` or `StopUncertain` policy.

**Fencing.** The server does have real split-brain protection.
- `ConversationLease` (`server/conversation_lease.py`) is a file lease: `owner_instance_id`, `generation`, TTL 45 s, plus host and pid for crash detection. Every event and state write runs inside `lease.guarded_write(generation)` through `state.set_write_guard` (`event_service.py:372-375`).
- A takeover bumps `generation`, so a resurrected owner "can be fenced off". This is the same design as Nvoken's `attempt` fence, over files instead of Postgres.

### Ordering and idempotency

- **Ordering.** The event index (the filename prefix) is a gap-free per-conversation sequence assigned under the file lock. Observations for a batch are emitted in **call order**, not completion order. The tree adds a second order (`parent_id`), and replay uses `path_to_root`.
- **Idempotency.**
  - Event IDs are unique (a duplicate append raises).
  - `respond_to_confirmation{accept:true}` while already running is a no-op.
  - `send_message` has **no client idempotency key** (`SendMessageRequest{role, content, run}`, `sdk/conversation/request.py:64`), so a retried POST duplicates the message.
  - Tools get no idempotency key beyond `tool_call_id`.

### Concurrency inside one conversation

- **The lock.** `ConversationState` carries a `FIFOLock`. `run()` holds it for the whole step. `arun()` **releases it around the awaited LLM call** (`_released_state_lock_during_io`, `local_conversation.py:1879`), so `send_message()` and state reads stay responsive.
- **Steering is implicit.** A user message sent mid-run is appended to the log, and the next step's `View` includes it.
- **The run loop never checks for `FINISHED` after a step** (comment at `:2000-2006`). That way a message that arrives just as the agent finishes resets the status to `IDLE` and is processed by the same loop.

### Local vs remote, and the sandbox model

- **Local:** the agent loop, tools and persistence are all in-process. `workspace` is a directory.
- **Remote:** `DockerWorkspace`/`APIRemoteWorkspace`/`CloudWorkspace` start a container or pod running the Agent Server. `RemoteConversation` sends the serialized agent plus workspace config, then drives the conversation over REST and WebSocket.
  - Tools execute inside the sandbox, where `LocalConversation` actually runs. The `ToolExecutor` docstring states "even when tools are invoked via RemoteConversation, the remote agent server creates a LocalConversation".
  - `RemoteConversation.run(blocking=True)` waits on the WebSocket and **falls back to REST polling** (`remote_conversation.py:1291-1420`).
- **Sandbox = process boundary.** There is no per-tool sandbox. Isolation is the container or pod, and the security analyzer and confirmation are the in-loop guardrails.

---

## 5. Tool systems and their interfaces

### Definition

- **Inputs** are a Pydantic `Action` subclass, and its JSON schema is generated with `to_mcp_schema()`. **Outputs** are an `Observation` (`content: [Text|Image]`, `is_error`).
- **Subclasses** implement `create(conv_state, **params) -> Sequence[Self]`, which returns tool instances with executors bound to the conversation (for example the terminal's working directory).
- **Registration** is by name in a global registry. The agent stores only `Tool{name, params}`.
- **Schema augmentation** (`tool.py:697-720`, `:860-889`):
  - Every tool schema gets a `summary` field, a one-line description the UI shows.
  - Non-read-only tools get a `security_risk` enum when an analyzer is set.
  - Both are prioritized to the top of `properties`, and both are popped from the arguments before `action_from_arguments`.
- **Go translation.** The generic `ToolDefinition[ActionT, ObservationT]` maps directly onto the survey's `tool.Func[In, Out]`, which reflects a schema from `In` and returns a `Def` plus an executor. The Python class-level `name` derived from the class name (`TerminalTool` → `terminal`) should *not* be copied. Go should use explicit names, because wire names must be stable.

### Argument repair and errors

`_get_action_event` (`agent.py:1227`) runs in this order:
1. `parse_tool_call_arguments`
2. `normalize_tool_call` (tool-name aliasing and a "terminal fallback", as Eino does with `ToolAliases`)
3. `fix_malformed_tool_arguments` against the action type
4. extract `security_risk` and `summary`
5. Pydantic validation

On any failure it emits an `ActionEvent(action=None)` **and** an `AgentErrorEvent` whose text lists parameter *names*, never values ("Parameters provided: [...]"). The model sees the error and retries. Unknown tools get `"Tool 'x' not found. Available: [...]"` the same way.

Execution errors (`parallel_executor.py:_run_safe`):
- `ValueError` → `AgentErrorEvent(classification=AGENT_OUTCOME)`, meaning "the agent can correct itself".
- Any other `Exception` → `AgentErrorEvent(classification=INTERNAL)`.

**Both go to the model with `str(e)`**, unlike MAF-Go's opaque default. Exceptions never abort the run, unlike Eino. A tool can also return `Observation(is_error=True)`, which is prefixed `"[An error occurred during execution.]"` in the projection.

### Parallelism

`ParallelToolExecutor` (`agent/parallel_executor.py`):
- **Serial by default.** `tool_concurrency_limit=1` on `AgentBase`, as in MAF-Go.
- **When parallel**, it uses a per-batch `ThreadPoolExecutor(max_workers)` plus a `ResourceLockManager`. Each call takes locks from `tool.declared_resources(action)`:
  - `DeclaredResources(declared=False)` → a tool-wide mutex `tool:<name>` ("I haven't thought about it");
  - `declared=True, keys=()` → no lock ("I have, and I'm safe");
  - `keys=("file:/a.py",)` → lock exactly those resources.
- This beats every Go framework surveyed: parallelism is opt-in per tool and **per argument**, so two edits to different files run concurrently and two edits to the same file serialize.
- **Results** are returned in input order and emitted after the whole batch finishes.
- **Go translation:** `errgroup` with `SetLimit`, plus a keyed-mutex map with sorted lock acquisition. A tool implements an optional `Resources(call) ([]string, bool)` interface.
- **`FinishTool` truncates the batch:** calls after `finish` are discarded with a warning (`agent.py:201-227`).

### Confirmation and security gating

`_requires_user_confirmation` (`agent.py:1046`) runs on the whole batch *after* the `ActionEvent`s are persisted and *before* execution:
- A lone `finish` or `think` call never needs confirmation.
- Otherwise the analyzer rates every action, and if **any** risk makes `should_confirm` true, the status becomes `WAITING_FOR_CONFIRMATION` and the step returns without executing.
- Approval is **per batch**: `respond_to_confirmation{accept}` calls `run()`, which executes *all* unmatched actions, and reject calls `reject_pending_actions`, which writes a `UserRejectObservation` for *all* of them (`local_conversation.py:2641`).
- The approval is **bound to the recorded actions**: the client cannot alter arguments, because the pending set comes from the log. But the client cannot approve some calls and deny others. This is MAF-Go's 114-issue design flaw again, in a different language.

### MCP, client tools, and ACP tools

- **MCP.** `MCPToolDefinition` and `MCPToolExecutor` (`sdk/mcp/tool.py`) use FastMCP with a per-call timeout (`MCP_TOOL_TIMEOUT_SECONDS = 300`). Action types are built dynamically from the server's `inputSchema`. A `MCPToolProvider` reconciles tool-list changes at runtime (`AgentBase._on_mcp_tools_changed`). Server config is the SDK's own `dict[str, MCPServer]`, and FastMCP's config is built only at the boundary (`AGENTS.md`). There is also an MCP OAuth store in the server.
- **Client tools** (`sdk/tool/client_tool.py`). A frontend registers JSON-schema tools in `POST /conversations`. When the model calls one, the `ActionEvent` goes over the WebSocket, and the SDK **immediately returns `"Tool call dispatched to client."`** (`:167-172`). There is no suspension and no waiting for a result, so this is fire-and-forget. It compares badly with Nvoken's host tools, which park the turn.
- **ACP.** In ACP mode, tools run inside the external agent. `ACPAgent`'s bridge **auto-approves every `request_permission`** by picking the first option (`acp_agent.py:1602-1620`) and declines elicitations. Tool activity comes back as `ACPToolCallEvent{status, tool_kind, raw_input, raw_output}` for display only.

---

## 6. Execution loop implementation

### Conversation loop: `LocalConversation.run()` (`local_conversation.py:1904`)

```text
ensure agent ready (lazy: plugins, MCP, init_state → SystemPromptEvent)
status IDLE|PAUSED|ERROR|STUCK → RUNNING
loop:
  with state (lock):
    PAUSED|STUCK → break
    FINISHED → run Stop hooks; a hook may deny → append "environment" feedback msg,
               status=RUNNING, continue; else break
    _check_stuck_or_nudge(): nudge once on an action-error streak (appends an
               environment MessageEvent), or set STUCK and continue (→ break)
    WAITING_FOR_CONFIRMATION → RUNNING   (re-entering run() == approval)
    agent.step(self, on_event, on_token); iteration += 1
    WAITING_FOR_CONFIRMATION → break
    budget exceeded (max_budget_per_run, USD across all LLMs) → ERROR + ConversationErrorEvent("MaxBudgetReached")
    iteration >= max_iteration_per_run (default 500) → ERROR + "MaxIterationsReached" (unless FINISHED)
except LLMAuthenticationError / Exception → ERROR + ConversationErrorEvent, raise ConversationRunError
```

`arun()` (`:2090`) has the same shape, about 450 lines. It adds `asyncio` cancellation handling, ACP prompt bookkeeping, and the lock release around LLM I/O.

### Agent step: `Agent._step` (`agent.py:645`)

1. **Pending actions?** Execute them and return. This one branch covers confirmation approval and crash recovery.
2. **Blocked user message?** A `UserPromptSubmit` hook blocked it, so set `FINISHED` and return.
3. **Resolve runtime metadata** for the routed model, so the condensation threshold uses the real context window.
4. **`prepare_llm_messages(state.view, condenser, llm)`** (`agent/utils.py:581`). If the condenser returns a `Condensation`, emit it and **return; condensation is a whole step**.
5. **Image input to a non-vision model:** swap images for references when `vision_inspect` exists; otherwise emit an explanatory agent message and finish.
6. **`llm.generate(messages, tools, add_security_risk_prediction=True, on_token=stream.token_callback, call_context)`**. Error mapping:
   - `FunctionCallValidationError` → append the error as a **user** message and return (the model retries next step).
   - `LLMContentPolicyViolationError` → append a synthetic user nudge ("please continue, rephrasing...").
   - `LLMMalformedConversationHistoryError` → `rebuild_view()` with full property enforcement plus a `CondensationRequest`, when a condenser can handle it; otherwise raise.
   - `LLMContextWindowExceedError` → `CondensationRequest`, or raise.
7. **`classify_response(message)`** (`response_dispatch.py:54`) gives `TOOL_CALLS | CONTENT | REASONING_ONLY | EMPTY`.
   - **TOOL_CALLS:** build and emit an `ActionEvent` per call. The first one carries the thought, reasoning and thinking blocks, and claims the stream's `item_id`. Then comes the confirmation gate, and then `_execute_actions`: prepare (truncate at finish, pop hook-blocked actions, execute), emit in order, and finalize. If the last action is `finish`, iterative refinement may inject a follow-up user message; otherwise the status becomes `FINISHED`.
   - **CONTENT:** emit the agent `MessageEvent` (with an optional critic result) and set `FINISHED`. **A plain text answer ends the run**; no finish tool is required.
   - **REASONING_ONLY / EMPTY:** emit the message and a **corrective nudge** user message, and continue.

### Stuck detection (`conversation/stuck_detector.py`)

It scans at most 20 events since the last user message and checks:
- the same action with the same observation, 4 times by default;
- the same action ending in an error, 3 times. This is **nudged once** with an environment message before it is declared stuck;
- an agent monologue (a run of consecutive agent messages);
- an alternating A/B action-observation pattern;
- a loop of context-window errors.

The result is `STUCK`, a terminal status that a new user message resets. Thresholds are configurable through `StuckDetectionThresholds`. None of the Go frameworks has this. It is a cheap, practical guard for the "100M tokens" failure mode that MAF-Go users hit.

### Termination summary

A run ends on:
- a text answer or `finish` → `FINISHED` (the Stop hook may veto);
- a confirmation gate → `WAITING_FOR_CONFIRMATION`;
- `pause()` or `interrupt()` → `PAUSED`;
- the stuck detector → `STUCK`;
- an iteration or budget cap, or an unhandled exception → `ERROR`.

The iteration cap is a **hard error**, as in Eino, not MAF-Go's graceful final call without tools. Each step (one LLM call, a condensation, or a pending-action execution) counts as one iteration.

### How streaming is threaded

`StreamContext` (`agent/stream_context.py`) opens once per step (`Agent.step` wraps `_step` in `with StreamContext.open(conversation, on_token)`):

- **The slot ID is the future event ID.** It mints `item_id = uuid4()` up front. The durable event built from the stream (the first `ActionEvent` or the agent `MessageEvent`) **takes that ID as its `Event.id`** through `stream.claim()`/`commit()`. A client retires its preview slot when `frame.event.id == slot.item_id`. This is Nvoken's "preview identity equals the future record's identity" (README idea 20), implemented in the SDK.
- **Frames:**
  - `StreamStarted{item_id, attempt, anchor_seq}`, where `anchor_seq` pins the slot's position so that "a user message landing mid-stream cannot split it";
  - `StreamDelta{item_id, attempt, order, kind: text|reasoning, content, chunk_id}`;
  - `StreamAborted{item_id, attempt, reason}`, which is **guaranteed on exit** when no durable event claimed the slot (`__exit__` → `close()`, with the reason `cancelled`, the exception name, or `no_durable_event`).
- **Retries restart the stream.** A retry changes LiteLLM's `chunk_id`, which calls `new_attempt()`: the attempt number goes up, `order` resets, and the old tail is superseded.
- **Deltas are secret-masked per kind.** A masker holds back a possible partial secret across chunks and flushes it before claim.
- **Two channels coexist.** The legacy `on_token` passes raw LiteLLM chunks through (the server republishes them as `StreamingDeltaEvent`). The new `on_stream` carries the stamped frames.
- **Sink failures never fail a turn**, because "progress is a UX affordance".
- **Only text and reasoning are streamed.** Tool-call argument deltas are not.

---

## 7. Suspend/resume and error handling

### Pause, interrupt, confirmation

- **`pause()`** (`local_conversation.py:2711`) sets `PAUSED` and emits `PauseEvent`. The loop checks it between steps, so "if called during an LLM completion, the pause will not take effect until the current LLM call completes".
- **`interrupt()`** (`:2736`) is the hard stop:
  1. It sets the `CancellationToken`, which makes the executor skip tools that haven't started (`"Tool call cancelled by interrupt."`).
  2. It cancels the `arun()` task **from any thread** with `loop.call_soon_threadsafe(task.cancel)`.
  3. `CancelledError` propagates through the LLM HTTP stream, the step and the loop "without needing per-layer interrupt APIs, because LLM and Agent are frozen/stateless" (`AGENTS.md`).
  4. `arun()` catches it, calls `_emit_orphaned_action_errors()` (a synthetic `AgentErrorEvent` "Tool call interrupted before completion" for every unmatched action, so the history stays valid for providers), sets `PAUSED`, and emits `InterruptEvent`.
- **The sharp edge.** A tool thread keeps running after cancellation. `_arun_safe` calls `tool.executor.interrupt()` (for example Ctrl-C to the terminal), but "the thread still runs to completion". The log then says "interrupted before completion" about a command that may in fact complete, so the record can be false. Dive's `tool_uncertain` is the honest version.
- **Go translation.** This is the one area where Go is simply better. `ctx` cancellation reaches tools natively. The same honesty problem remains, though: a goroutine that ignores `ctx` outlives the turn, so the engine must record "uncertain", not "failed".
- **Confirmation mode is a suspension without a suspension object.**
  - Suspending means setting `WAITING_FOR_CONFIRMATION` with unmatched actions in the log.
  - Resuming means calling `run()` again (accept all) or `reject_pending_actions(reason)` (reject all).
  - It survives restarts, because the state is in the log plus `base_state.json`.
  - There is no typed pending list, no request ID, and no per-call decision.

### Condensation as error recovery

Context-window and malformed-history errors from the provider are turned into a `CondensationRequest`, which the next step handles. The summarizing condenser can do a **hard context reset** with up to 5 retries, scaling the context by 0.8 each time. `Condensation` events are durable and non-destructive: the log keeps everything, and the view applies `forgotten_event_ids`. This matches ADK-Go's compaction and is better than it, because the manipulation-index properties guarantee the cut never splits a tool loop or a thinking block. `POST /{id}/condense` forces a condensation.

### Retries and LLM errors

- **Transport retries** with tenacity: 5 attempts, exponential 8 to 64 s. They are visible to streaming clients as a new `attempt`. Then comes `FallbackStrategy` to alternate models.
- **Deterministic model mistakes** (a malformed function call, a content-policy block, an empty or reasoning-only answer) become **synthetic user messages in the log**. The stuck detector bounds repeats.
- **Authentication errors** → `ERROR` + `ConversationErrorEvent("LLMAuthenticationError")`, raised as `ConversationRunError`.
- **Other unhandled exceptions** → `ERROR` + a generic `ConversationErrorEvent`, unless the agent already emitted a typed one (`_agent_already_surfaced_error`).
- **`classify_error(code, detail)`** maps any failure to the closed `FailureKind` vocabulary, so the UI can show "fix settings" or "retry" without parsing strings.

### Resume across processes

The same `conversation_id` plus `persistence_dir` reopens the conversation. `ConversationState.create` (`state.py:455`) loads `base_state.json`, attaches the `EventLog`, calls `rebuild_view()` with full property enforcement ("persisted events may come from an older code version or be corrupted"), and runs `agent.verify` when an agent is supplied. For crash semantics, see §4.

---

## 8. Agent Server API and event protocol

### REST (FastAPI, `server/conversation_router.py`, `event_router.py`)

- **Conversations:** `POST /api/conversations` (start: agent, workspace, `confirmation_policy`, `security_analyzer`, `client_tools`, secrets, plugins, hooks, `max_iterations`); `GET /search`, `/count`, `/{id}`; `DELETE /{id}`; `PATCH /{id}`.
- **Control:** `POST /{id}/run`, `/pause`, `/interrupt`, `/condense`, `/fork`, `/navigate`, `/ask_agent` (a stateless side question), `/goal` + `/goal/stop` + `/goal/resume`, `/switch_llm`, `/switch_profile`, `/switch_acp_model`, `/load_plugin`, `/secrets`, `/confirmation_policy`, `/security_analyzer`, `/agent_final_response`, `/runtime` + `/runtime/reprovision`.
- **Events:** `GET /api/conversations/{id}/events/search` (`page_id`, `limit`, `sort_order`, `timestamp__gte/__lt`, kind and body filters), `/count`, `/{event_id}`, and a batch get. `POST /events` sends a message (`{role, content, run}`). `POST /events/respond_to_confirmation` takes `{accept, reason}`.
- **Plus** file, bash, git, MCP, hooks, skills, plugins, settings, profiles, provider-connections, VS Code and workspaces routers. The Agent Server is a full sandbox control plane, not just an agent API.

### WebSocket v1: `/sockets/events/{conversation_id}` (`server/sockets.py:227`)

- **Auth.** A first-message `{"type":"auth","session_api_key":...}` within 10 s. Query-string and header keys still work but are deprecated. The socket closes with 4001 on failure, 4004 for an unknown conversation, and 1013 when over the subscriber limit.
- **Replay.**
  - `resend_mode=all` pages the whole log to the client.
  - `resend_mode=since&after_timestamp=T` sends events with **timestamp ≥ T**.
  - The server **subscribes first, then replays**, so history and live traffic can interleave and duplicate. Clients must dedupe by `event.id`.
  - The cursor is a wall-clock timestamp in server-local time, not a sequence number.
- **Frames** are raw `Event` JSON (the persisted record itself), plus `StreamingDeltaEvent`s and a `ServerErrorEvent` on errors.
- **Inbound frames** are user `Message`s, sent with `run=True`.

### WebSocket v2: `/sockets/session/{conversation_id}?after_seq=N` (`server/session_protocol.py`, `session_socket.py`)

Its docstring lists what it fixes: "frames are envelopes rather than the disk record; history and live traffic cannot interleave; and a slow consumer cannot wedge the publisher".

```python
SessionFrame = SyncFrame{from_seq, through_seq}         # once, before replay
             | DurableFrame{seq, event}                 # persisted; seq = log index = filename idx
             | TransientFrame{event}                    # published, not persisted; no cursor advance
             | ItemStartedFrame{item_id, attempt, anchor_seq}
             | DeltaFrame{item_id, attempt, order, kind, content, chunk_id, choice_index}
             | ItemAbortedFrame{item_id, attempt, reason}
             | ErrorFrame{code, detail}                  # socket problem, never persisted
```

The delivery rules are numbered in the module docstring:
1. A durable frame survives a reconnect through `after_seq`.
2. Deltas may be dropped; a gap marks the slot lossy.
3. Every `ItemStarted` is retired by exactly one `Durable` whose `event.id == item_id`, or by one `ItemAborted`.
4. Progress frames are never replayed.
5. No ordering is promised between two open items.

**Implementation details:**
- Live events are **buffered during replay** and deduped against `through_seq` in `go_live`.
- `_ConnectionWriter` gives each connection one writer task, with byte-bounded admission (`MAX_PENDING_BYTES = 16 MiB`, `MAX_FRAME_BYTES = 4 MiB`, both marked "PROVISIONAL"). On overflow, "Overflow drops the connection, never a frame... a reconnect with `after_seq` loses nothing. Disconnection is the backpressure." The close code is 1013 `slow_consumer`.

This is structurally the same protocol as Nvoken's (README §11): durable versus preview frames, a cursor, preview identity equal to record identity, attempt supersession, an explicit abort, and dropping slow consumers. Two independent teams converged on it, which is strong evidence that Dive should standardize the event classes in the library.

### How Canvas consumes it (`OpenHands/src`)

- **Canvas still uses v1.**
  - `buildWebSocketUrl` returns `.../sockets/events/{id}` (`canvas/utils/websocket-url.ts`).
  - It first loads the newest page of history over REST (`useConversationHistory`, `TIMESTAMP_DESC`).
  - It then opens the socket with `resend_mode=since&after_timestamp=<last preloaded timestamp>`, or `resend_mode=all` (`canvas/contexts/conversation-websocket-context.tsx:995-1002`).
  - Reconnects use exponential backoff from 1 s up to 30 s, with 30% jitter and unlimited attempts (`canvas/hooks/use-websocket.ts:112-136`).
  - Dedupe is a `Set` of event IDs in the Zustand store (`canvas/stores/use-event-store.ts:100, 172`), and out-of-order arrivals are re-sorted by ISO timestamp.
  - The WS handler skips side effects for already-seen IDs (a comment cites #1656).
- **The TS client doesn't implement the session socket yet** (no `after_seq` in `clients/typescript/src`), so v2 is server-side only so far.
- **UI reduction** (`canvas/utils/handle-event-for-ui.ts`):
  - An observation replaces its matching action in `uiEvents`.
  - Streaming deltas merge into one preview event and are stripped when the durable message arrives. They are batched per `requestAnimationFrame`.
  - `ConversationStateUpdateEvent` keys (`full_state`, `execution_status`, `stats`, `goal`) drive status, metrics and the goal chip.
  - Confirmation buttons appear on `WAITING_FOR_CONFIRMATION` and post `{accept, reason}`.
- **Messages** go over the socket (`{...message, run: true}`), with REST as the fallback.
- **Types.** `OpenHandsEvent` is a hand-maintained TS union of 16 event types (`canvas/types/agent-server/core/openhands-event.ts:25-46`), discriminated by `kind`. Some types are re-exported from `@openhands/typescript-client`, which about 85 Canvas files import.

### Multi-backend and ACP

- **Backends.** Canvas keeps a registry of `Backend{id, name, host, apiKey, kind: "local" | "cloud", authMode: "api-key" | "cookie"}` in localStorage (`canvas/api/backend-registry/`), selected per tab or by `?backend=`.
  - Local backends are raw Agent Servers using `X-Session-API-Key`.
  - The cloud backend (OpenHands Cloud) uses bearer or cookie auth through a proxy (`/api/v1/conversation/{id}/events/search`), with organization scoping.
  - Protocol calls that only work locally throw `NoBackendAvailableError` against cloud.
- **ACP agents are an agent setting, not a backend.** `agent_kind: "acp"` with `acp_command`/`acp_model` makes the server run an `ACPAgent`, with Claude Code, Codex or Gemini CLI as a subprocess (`canvas/constants/acp-providers.ts`, mirroring `sdk/settings/acp_providers.py`).
  - Canvas probes login state with shell commands over the bash socket (`claude auth status --json`, `codex login status`).
  - OpenHands is thus an ACP **client**: it hosts other agents inside its conversation, event log, workspace and UI. Each ACP turn is one `step()` ending in a synthetic `FinishAction`.
- **Automations.** Canvas has a full automation UI: cron and event triggers with JMESPath filters, runs, dispatch and cancel (`canvas/api/automation-service/`). The automation service owns scheduling and dispatches conversations to Agent Servers.

---

## 9. Strengths, weaknesses, and lessons for Dive

### What is excellent

1. **The event log as the record, with a cached incremental projection.** `state.view` extends in O(k) on a linear append and rebuilds only on branch switches or recovery. This is Dive principle 1 done well, and it avoids ADK-Go's per-step rescan.
2. **The action is persisted before execution.** It is the only system in the survey besides Nvoken that records tool intent before effect. Tool-call validation failures are also recorded (`ActionEvent(action=None)` plus an error), so the model's mistakes stay in the record.
3. **View properties with manipulation indices.** Provider invariants (tool pairing, batch atomicity, thinking-block tool loops, unique observations) are declared as properties that both *constrain condensers* and *repair corrupt histories*. This is the right way to make compaction safe across providers.
4. **Stream identity protocol.** The durable event's ID is minted before the first token, with `attempt` supersession, a guaranteed abort, anchoring, masking and a seq cursor. It independently confirms Nvoken's design (§8).
5. **Lease fencing on every write**, with a generation counter and pid/host checks for takeover.
6. **Resource-declared parallelism.** Tools declare per-call lock keys, and the undeclared default is a safe mutex.
7. **Defense-in-depth security.** A model-predicted risk parameter plus pattern, policy-rail and third-party analyzers, combined by a fail-closed ensemble.
8. **Operational guards that coding agents need:** stuck detection with a one-time nudge, a USD budget across all of a run's LLMs, a small `ErrorClassification` vocabulary, secret masking that covers stream chunks, and a system-first message invariant.
9. **The conversation tree** (`fork`, `navigate_to`) and **subagents as persisted child conversations**, resumable by ID.
10. **Contract discipline.** OpenAPI → generated TS client, golden fixtures for persisted settings, schema-version migrations, and a monorepo AGENTS.md that forces a PR to trace every layer (SDK → server → TS client → Canvas).

### What is awkward

1. **Size and duplication.**
   - `local_conversation.py` is 3.2k lines, `acp_agent.py` 4.7k and `llm.py` 3.5k.
   - Every path exists twice, sync and async (`run`/`arun` alone is about 700 lines of near-duplicate control flow).
   - Go should have exactly one path: `ctx` plus goroutines make the async twin unnecessary.
2. **Hidden persistence.** `__setattr__` autosaves `base_state.json`, and callers must remember to "always reassign" `agent_state`. There are two non-atomic stores (the events directory and the snapshot), and the HEAD can lag.
3. **Status as the only vocabulary.** One enum mixes lifecycle and stop reason. Limits show up as `ERROR` plus an error code. There is no turn noun, and no typed pending list for a suspension.
4. **Synthetic messages in the record.** Corrective nudges, content-policy nudges, malformed-call errors, stuck nudges, Stop-hook feedback and iterative-refinement follow-ups are all persisted as `MessageEvent`s. This is Dive v1 finding 1 ("a fact or a policy wearing a message costume").
5. **Observers control execution.** Hooks block actions through the event callback chain plus mutable `blocked_actions` state.
6. **Crash semantics are at-least-once by accident.**
   - Unmatched actions are re-executed by the confirmation-mode branch, and the server backfills only the *first* one.
   - Results are persisted per batch.
   - Interrupt records "interrupted before completion" while the tool thread may finish.
   - `ToolAnnotations` carries `idempotentHint`/`readOnlyHint` but recovery ignores them.
7. **Batch-level approval** and **fire-and-forget client tools.** No per-call decisions, and no way to park the loop until an external result arrives.
8. **A hard iteration cap at 500**, reported as `ERROR`.
9. **Legacy protocol still in production.** Canvas uses the timestamp-cursor v1 socket, which subscribes before replaying and needs client-side dedupe. The v2 session socket exists only on the server.
10. **LiteLLM and OpenAI shapes as the core.**
    - Provider features accumulate as fields on `Message`.
    - Retries, pricing and dialects are inherited from LiteLLM.
    - `DiscriminatedUnionMixin` uses class names as wire names, resolved through a process-global subclass registry.

### Contrasts with the other analyses

| Concern | OpenHands | Eino | ADK-Go | MAF-Go | Nvoken |
|---|---|---|---|---|---|
| Record | Append-only event log (tree), one file per event | Checkpoint on interrupt only | Session event log | None mid-run | Postgres commit groups |
| Prompt build | **Incremental cached View** | Graph state | Rescan log every step | Local slice | Rebuild from durable prefix |
| Tool intent before effect | **Yes** (`ActionEvent` first); no `started` marker | No | No | No | Yes, with a fenced `running` record |
| Results persisted | Per batch, in call order | On interrupt | Per merged event | After `Run` | Per tool |
| Crash mid-tool | Re-runs unmatched (SDK), or errors the first and re-runs the rest (server) | Lost | Dangling call | Lost | Typed policy by annotations |
| Fencing | File lease + generation | — | Timestamp OCC | Workflow CAS | Lease + `attempt` |
| Approval | Per batch, bound to the log | Interrupt, tool re-entry | Synthetic confirmation call | Per batch, bound | Host tool, per call |
| Parallel tools | Opt-in, **per-resource locks** | Unbounded | Unbounded | Serial, or unbounded | Serial |
| Iteration cap | 500, hard `ERROR` | 20, hard | None | 40, graceful | Budgets |
| Stuck detection | **Yes** (5 patterns, nudge) | No | 10 thought-only turns | Error budget of 3 | No |
| Compaction | Non-destructive, with **property-safe cut points** | Middleware | Non-destructive range | Once per `Run` | — |
| Stream protocol | v2: durable/delta/abort, seq cursor, drops slow consumers | Side channel | Partial events | Buffered after the first call | The same design over SSE |
| Provider layer | LiteLLM plus vendor fields | Two message models | `genai` lock-in | Provider `RunFunc` | Dive v1 |
| ACP | **Client** (hosts Claude Code, Codex, Gemini) | — | — | Requested, unclaimed | — |

### Recommendations for Dive

1. **Adopt the incremental projection.** Keep `Project(record) → []llm.Message` as a pure function, and have the Engine keep a cached projection that extends on commit and rebuilds on replay or branch change.
2. **Adopt view properties as a first-class `projection` package.** Each provider invariant is a type with `CutPoints(steps) Set` and `Repair(steps) (drop []StepID)`. Compaction must cut at the intersection. `Repair` runs on cold load, and every repair it performs is recorded as a projection adjustment, never silently.
3. **Close the gaps OpenHands leaves in "intent before effect":**
   - commit `tool_started` per call, as the fence before dispatch;
   - commit `tool_completed` per call, in completion order, with the projection ordering results by call;
   - commit `model_requested` before each attempt;
   - on recovery, apply a typed policy that consults annotations (`RetryIfSafe` when read-only or idempotent, else `Uncertain`), never the approval path.
4. **Never let one code path mean two things.** OpenHands' "unmatched action" means *pending approval*, *crashed in flight* and *interrupted* at once. Dive's `Waiting`, `tool_started` without a result (`Uncertain`), and `NotExecuted` must stay distinct step kinds.
5. **Per-call approval as Policy.** Keep OpenHands' binding to recorded calls, but take an `Accept{CallID, Outcome, CommandID}` per call. Model-predicted `security_risk` is a good *input* to `AuthorizeTool`: offer it as an optional schema augmentation that the Policy can read. It must never be the only gate, which OpenHands also avoids through the ensemble.
6. **Resource-keyed concurrency in `tool`.** Add an optional `Resources(call) (keys []string, declared bool)` interface. Undeclared means a per-tool mutex. The Engine takes the keys in sorted order under `Limits.ToolConcurrency`. This is strictly better than a boolean `ConcurrencySafe` annotation.
7. **Ship the stream vocabulary in the library.** `Delta{ItemID, Attempt, Order, Kind}`, `ItemAborted` and `anchor` in the Observer, with **ItemID equal to the Step or message ID** that will be committed. OpenHands and Nvoken arrived at the same rules independently, so encode them once, with a reference reducer.
8. **Stuck detection and budgets as a stock Policy.** Implement `OnStop`/`BeforeModel` policies that detect repeated (call, result) pairs and error streaks, and return a typed `Stop{Limited, "stuck"}` after an optional **recorded context item** (a `ContextItem{Origin: Library}`, not a fake user message).
9. **Replace synthetic user messages with `ContextItem`s.** Every OpenHands nudge (empty response, content filter, malformed call, stuck, Stop-hook feedback) is a projection-time context item with an origin, which keeps the record honest.
10. **Put fencing in the Recorder contract.** Add `Commit(ctx, step, fence)`, where the fence is a generation. OpenHands (a file lease) and Nvoken (Postgres) both needed it, so the interface should carry it even though the storage stays with the host.
11. **Configuration as a value, done properly.** OpenHands persists the agent JSON with the conversation and checks that tools were only added. Dive should record `Definition{ID, Version, Hash}` in `turn_started` and fail `Continue` on a hash change unless it is accepted. Adopt OpenHands' "tools may be added, never removed" rule as the default compatibility check.
12. **ACP in both directions as adapters.** Hosting external agents as a Dive "agent" (OpenHands' `ACPAgent`) and exposing a Dive agent over ACP (the MAF-Go request) both belong in `adapters/acp`. Do not copy OpenHands' auto-approve bridge. Route `request_permission` through `Policy.AuthorizeTool` so an ACP turn can suspend.
13. **Don't port the Python idioms.** Specifically avoid:
    - autosave on attribute assignment (commit explicitly);
    - class-name `kind` discriminators (use explicit, stable `Kind` strings plus a registry of decoders);
    - sync/async twins;
    - mixins (use composition);
    - process-global tool registries (use a `tool.Resolver` value on the `Definition`);
    - the hard iteration cap (stop gracefully with `Limited`).
14. **Workspace is out of scope, except as a seam.** OpenHands' `Workspace` (local, Docker, K8s, same API) is product surface for a coding platform. For Dive, the lesson is only that **the loop should run next to the tools** (the whole Agent Server inside the sandbox) rather than tunneling each tool call. That favors hosts embedding the Dive Engine in the sandbox and streaming committed steps out.
