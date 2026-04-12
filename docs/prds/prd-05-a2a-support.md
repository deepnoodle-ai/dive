---
Title: A2A Support for Remote Agent Interoperability
Author: Curtis Myzie
Status: Draft
Last Updated: 2026-04-11
Stakeholders: Dive maintainers, Dive library users, SaaS integrators
---

# A2A Support for Remote Agent Interoperability

## 1. Problem & Opportunity

Dive has a strong local agent runtime:

- provider-agnostic LLM access
- a mature tool-calling loop
- sessions
- hooks
- explicit suspend/resume
- experimental subagents and background tasks

What Dive does **not** have is a first-party remote-agent interoperability
layer.

Today, if a Dive user wants one agent to collaborate with another agent across
a process or network boundary, the options are weak:

- **Wrap the remote agent as a tool.** This flattens an agent into a single
  stateless function call, which loses multi-turn interaction, negotiation,
  task state, and long-running workflow semantics.
- **Build a custom HTTP API.** Every team invents its own task model, streaming
  shape, auth story, and discovery mechanism.
- **Keep everything in-process.** Dive's experimental `Task`/subagent support
  works locally, but it does not generalize to remote, durable, cross-service
  agent collaboration.

This creates a widening product gap. Dive is increasingly good at embedding
agents into applications, but modern multi-agent systems need a standard way
for agents to:

- discover one another
- declare capabilities
- stream progress
- expose long-running task state
- wait on human or auth input
- survive disconnected callers and process boundaries

The A2A protocol is now the clearest standards-based answer to that problem.
The official docs describe the latest released version as `1.0.0`, and the A2A
SDK documentation now lists an official Go SDK. That means A2A is no longer
just interesting research context; it is a viable integration target for Dive.

**If we do nothing:** Dive remains strong as a local runtime plus MCP client,
but it has no first-party story for remote-agent composition. Users who need
agent-to-agent interoperability either build bespoke infrastructure or migrate
to frameworks that treat remote agents as a first-class concept.

## 2. Goals & Success Metrics

**Primary goal:** Give Dive a first-party, experimental A2A adapter layer so
that Dive agents can be exposed as remote A2A agents and can call remote A2A
agents without redesigning Dive core around A2A semantics.

**Success metrics:**
- **Primary:** A Dive app can expose a local agent through A2A in under 100
  lines of integration code using an `experimental/a2a` package.
- **Primary:** A Dive app can call a remote A2A agent and surface its progress
  and final result through Dive-friendly abstractions without requiring callers
  to hand-assemble protocol requests.
- **Secondary:**
  - A suspended Dive run maps cleanly to A2A interrupted task state without
    losing resumability.
  - Dive's experimental task/subagent layer can be evolved to support A2A
    backends without changing the parent-agent UX shape.
  - No breaking changes to the stable `dive` package are required for the
    first shipping phase.
- **Guardrail:** Dive core remains the authoritative local runtime. A2A must be
  an adapter layer, not a new internal control-flow model.
- **Guardrail:** MCP and A2A stay conceptually separate: MCP for tools/data,
  A2A for agents.

## 3. Target Users

**Primary persona: SaaS integrator.** A Go engineer embedding Dive in a backend
service or workflow platform. They want their Dive agent to call specialized
agents owned by other teams or services, or expose their own agent for reuse by
other systems.

**Secondary persona: platform/team maintainer.** Someone building an internal
catalog of agents and wanting standardized discovery, capabilities, task
streaming, and auth semantics rather than custom HTTP contracts.

**Tertiary persona: advanced Dive user building multi-agent systems.** They
already use `experimental/toolkit/extended.TaskTool` and want to evolve from
local-only subagents to remote agents without rewriting their orchestration
layer.

**Not optimized for:** users who only need local tools, local sessions, and
single-process agents. A2A support must not complicate the common local case.

## 4. User Stories

### US-001: Expose a Dive agent as an A2A server
**Description:** As a SaaS integrator, I want to expose a Dive agent through an
HTTP handler that implements A2A so that external orchestrators and other
agents can call it using a standard protocol.

**Acceptance Criteria:**
- [ ] An experimental package provides an HTTP-facing A2A server adapter for a
      Dive agent.
- [ ] The adapter can return task state for both quick completions and
      long-running/suspended runs.
- [ ] The adapter can generate or serve an agent card describing the exposed
      agent's capabilities.
- [ ] Documentation includes a minimal example showing a Dive agent exposed as
      an A2A endpoint.

### US-002: Call a remote A2A agent from Dive
**Description:** As a Dive user, I want to call a remote A2A agent from Go code
without hand-crafting A2A messages so that remote agents feel like a first-
class integration surface.

**Acceptance Criteria:**
- [ ] An experimental client wrapper can resolve a remote agent card and call
      the remote endpoint.
- [ ] The wrapper supports both one-shot requests and long-running tasks.
- [ ] The wrapper surfaces remote progress and final output through
      Dive-friendly types or callbacks.
- [ ] The integration does not require callers to understand raw JSON-RPC or
      protocol binding details for the common case.

### US-003: Represent Dive suspend/resume cleanly in A2A
**Description:** As a maintainer, I want a suspended Dive run to map onto A2A
task state cleanly so that remote callers can understand "waiting on input"
without Dive abandoning its stronger local runtime semantics.

**Acceptance Criteria:**
- [ ] A suspended Dive response maps to an A2A interrupted/waiting state rather
      than to failure.
- [ ] The mapping preserves enough information for the caller to understand why
      the task is waiting.
- [ ] A resumed Dive session can continue to completion without the A2A adapter
      inventing a different internal persistence model.

### US-004: Evolve local Task/subagent orchestration to support remote agents
**Description:** As an advanced Dive user, I want the experimental task/subagent
system to eventually support A2A-backed execution so that I can use local and
remote agents behind one orchestration surface.

**Acceptance Criteria:**
- [ ] The PRD defines a path away from storing `*dive.Agent` directly in the
      task registry as the only execution model.
- [ ] Remote A2A-backed tasks are treated as first-class task executions, not
      merely as stringly-typed tool wrappers.
- [ ] The design allows local and remote backends to coexist.

### US-005: Use A2A discovery/capabilities without polluting Dive core
**Description:** As a platform maintainer, I want to use agent cards,
capabilities, and discovery with Dive while keeping A2A concerns out of
`dive.Agent` and `dive.Response`.

**Acceptance Criteria:**
- [ ] A2A-specific types live in an experimental package, not in the stable
      `dive` core package.
- [ ] The stable core API does not grow agent-card or task-protocol fields just
      to satisfy the adapter.
- [ ] The adapter can still expose capabilities like streaming and
      push-notification support.

## 5. Functional Requirements

- **FR-1:** Dive MUST provide an experimental A2A package group, likely under
  `experimental/a2a/`, rather than placing A2A types directly into the stable
  `dive` package.
- **FR-2:** The A2A support MUST include a server-side adapter capable of
  exposing a Dive agent as a remote A2A agent over HTTP.
- **FR-3:** The server adapter MUST support task-oriented execution, not just
  message passthrough, so that long-running and suspended Dive runs have a
  standard remote representation.
- **FR-4:** The server adapter MUST expose or generate an A2A agent card that
  declares the agent's identity, endpoint, and supported capabilities.
- **FR-5:** The A2A support MUST include a client-side wrapper that can call a
  remote A2A agent from Go without requiring the caller to manually construct
  raw protocol requests.
- **FR-6:** The client wrapper SHOULD build on the official A2A Go SDK where
  possible rather than reimplementing the wire protocol by hand.
- **FR-7:** The server adapter MUST map normal Dive completion to a terminal A2A
  success state.
- **FR-8:** The server adapter MUST map Dive failures to an A2A failure state.
- **FR-9:** The server adapter MUST map `ResponseStatusSuspended` to an
  A2A waiting/interrupted state, defaulting to `INPUT_REQUIRED` semantics.
- **FR-10:** The adapter SHOULD expose suspension prompt/metadata in the A2A
  task status message or metadata so the caller can understand what input is
  required.
- **FR-11:** The adapter MUST support streaming task progress using Dive's
  existing event callback mechanism as the source of truth for local execution
  events.
- **FR-12:** The streaming adapter MUST emit coherent user-visible progress,
  rather than blindly forwarding every provider-specific model delta.
- **FR-13:** The A2A client/server support MUST allow a Dive session-backed run
  to survive process boundaries in the same way local suspend/resume already
  does.
- **FR-14:** The first phase of A2A support MUST NOT require changing the
  fundamental control-flow model of `dive.Agent`.
- **FR-15:** A2A support MUST remain compatible with existing sessions,
  `WithEventCallback(...)`, and suspend/resume semantics.
- **FR-16:** The design MUST preserve a clean separation between:
  - MCP-backed tools and data access
  - A2A-backed remote agent interaction
- **FR-17:** The experimental task/subagent system SHOULD be refactorable
  toward a transport-neutral task backend so that local and A2A-backed
  execution can share orchestration concepts.
- **FR-18:** The current `experimental/toolkit/extended.TaskTool` assumption
  that suspended subagent work is a failure MUST be removed or made pluggable
  before A2A-backed task execution is treated as supported.
- **FR-19:** Dive SHOULD add a small optional suspend reason/category if needed
  to distinguish generic `input_required` from `auth_required` when projecting
  suspension onto A2A task state.
- **FR-20:** Dive SHOULD add an explicit way to abandon or cancel suspended
  local work if A2A task cancellation cannot otherwise be mapped cleanly.
- **FR-21:** The first shipping phase MUST document the supported A2A scope
  clearly, including which operations/capabilities are implemented and which are
  deferred.
- **FR-22:** The experimental package MUST include end-to-end examples for:
  - exposing a Dive agent as an A2A server
  - calling a remote A2A agent from Dive code
  - mapping a suspended Dive run to remote task state

## 6. Non-Goals (Out of Scope)

- **Redesigning `dive.Agent` around A2A.** Dive core remains a local runtime,
  not an A2A-native runtime.
- **Replacing sessions with A2A task history.** Session persistence remains a
  Dive concern; the A2A layer projects it externally.
- **Turning all remote agents into ordinary tools by default.** A convenience
  wrapper may exist, but it is not the primary model.
- **Building a full agent marketplace or public registry in Dive core.**
  Discovery may support static cards, well-known URLs, or organization-local
  registries, but Dive is not building a global registry product.
- **Supporting every A2A capability in v1.** Push notifications, advanced
  multi-binding support, rich extension negotiation, and full history exposure
  may be phased.
- **Moving A2A out of `experimental/` immediately.** Initial support is
  explicitly experimental.
- **General multi-protocol abstraction in core.** This PRD is about A2A
  specifically, not about a grand unified protocol layer for every future
  transport.

**Future considerations (deferred but worth designing for):**
- A2A-backed discovery integrated into subagent selection.
- A remote-agent convenience wrapper that can be used as a Dive tool when the
  simpler abstraction is appropriate.
- Better mapping of approval/auth workflows to explicit `AUTH_REQUIRED`
  semantics.
- Push-notification support for disconnected task consumers.

## 7. Dependencies & Risks

| Risk / Dependency | Impact | Mitigation |
|---|---|---|
| Official A2A Go SDK API may still evolve quickly. | Dive's experimental adapter may need churn. | Keep all A2A support under `experimental/`; wrap the SDK behind Dive-owned interfaces where useful. |
| A2A protocol complexity may tempt core leakage. | Stable Dive APIs become polluted with protocol-specific concerns. | Keep A2A types/packages separate; only make additive core changes when a real local runtime gap is proven. |
| Dive's current task/subagent model is in-memory and local-only. | Remote-agent integration becomes awkward or duplicated. | Introduce a transport-neutral backend concept in the experimental task layer rather than teaching `TaskTool` A2A details directly. |
| Streaming semantics may not align perfectly between provider deltas and A2A artifacts. | Remote consumers receive noisy or low-value updates. | Emit coherent output-oriented updates rather than raw provider event passthrough. |
| Task cancellation is clearer in A2A than in current local suspend semantics. | Adapter cannot map remote cancel cleanly to local state. | Add explicit abandonment/cancel semantics if the first implementation proves the gap is real. |
| A2A interrupted states distinguish `INPUT_REQUIRED` and `AUTH_REQUIRED`, while Dive suspend is generic today. | Adapter has to guess or collapse states. | Start with `INPUT_REQUIRED`; add optional suspend reason/category if needed. |
| Over-scoping the first release. | A large protocol surface delays useful delivery. | Phase the work: server adapter first, client wrapper second, task backend integration third. |

## 8. Assumptions & Constraints

**Assumptions:**
- The official A2A docs and SDKs are sufficiently stable to target
  experimentally as of 2026-04-11.
- Dive users who care about remote-agent interoperability are willing to adopt
  an experimental package surface first.
- Existing Dive session and suspend/resume semantics remain the preferred local
  execution model.

**Constraints:**
- Must not break the stable `dive` public API.
- Must preserve Dive's library-first philosophy: explicit configuration, no CLI
  coupling, no hidden global agent registry.
- Must coexist cleanly with MCP support rather than competing with it.
- Go 1.25, existing provider set, existing session architecture.

## 9. Design Considerations

**Recommended package shape** (illustrative, not final):

```text
experimental/a2a/
  card/
  client/
  server/
  remoteagent/
```

Responsibilities:

- `card/`: agent-card helpers, capability declarations, card serialization
- `client/`: low-level client wrapper around the official Go SDK
- `server/`: HTTP handler / server adapter exposing a Dive agent via A2A
- `remoteagent/`: ergonomic higher-level adapter for calling remote A2A agents

**Runtime mapping principles:**
- Dive local runtime semantics stay authoritative.
- One `CreateResponse(...)` execution generally maps to one A2A task.
- `Session.ID()` is the natural default source for A2A `contextId`.
- A suspended Dive response should project to interrupted/waiting task state,
  not failure.

**Task/subagent integration direction:**
- Do not teach A2A details directly into today's `TaskTool` implementation.
- Instead, move toward a task execution backend abstraction so local and remote
  executions can share orchestration concepts.

Illustrative shape:

```go
type TaskBackend interface {
    Start(ctx context.Context, req *TaskRequest) (*TaskHandle, error)
    Resume(ctx context.Context, id string, input *TaskResumeInput) (*TaskHandle, error)
    Get(ctx context.Context, id string) (*TaskSnapshot, error)
    Cancel(ctx context.Context, id string) error
    Subscribe(ctx context.Context, id string, onEvent func(*dive.ResponseItem) error) error
}
```

**Discovery direction:**
- Static agent-card configuration should be supported first.
- Well-known URI discovery is desirable next.
- Catalog/registry-backed discovery is a later integration concern, not a v1
  requirement.

## 10. Technical Considerations

- **Protocol scope for v1.** The initial implementation should focus on the
  small subset that provides real value:
  - agent card
  - send message / start task
  - stream task updates
  - get task
  - cancel task
- **Message vs task response shape.** Even though A2A allows direct message
  responses, Dive should likely materialize tasks first for simplicity and
  consistency.
- **Streaming translation.** The adapter should translate Dive events into
  artifact/status updates in a way that is semantically meaningful to remote
  consumers, not provider-specific.
- **Suspend fidelity.** The first implementation can map all suspends to
  `INPUT_REQUIRED`; if real auth/approval workflows demand it, add a small
  suspend reason/category in core later.
- **Cancellation semantics.** A2A task cancellation may reveal the need for a
  local "abandon suspended work" operation on sessions or on a higher-level
  controller API.
- **SDK wrapping.** It is acceptable for Dive to wrap the official A2A Go SDK
  behind narrower interfaces so the rest of the experimental package is not
  tightly coupled to external API churn.
- **Examples and tests.** The feature needs end-to-end tests covering:
  - local completion projected to A2A task completion
  - local suspend projected to waiting/interrupted task state
  - remote A2A client call surfacing progress and final output

## 11. Suggested Rollout

### Phase 1: Experimental A2A server adapter
- Expose a Dive agent as an A2A endpoint
- Generate/serve an agent card
- Support task creation, status, streaming, and cancellation
- Map local suspend to `INPUT_REQUIRED`

### Phase 2: Experimental A2A client + remote agent wrapper
- Resolve/fetch remote agent cards
- Call remote A2A agents from Go code
- Surface progress and final results through Dive-friendly abstractions

### Phase 3: Transport-neutral task backend
- Refactor the experimental task/subagent layer away from direct `*dive.Agent`
  storage
- Support local and A2A-backed task backends
- Stop treating interrupted remote work as failure-by-definition

### Phase 4: Optional core refinements
- Add suspend reason/category if needed for `AUTH_REQUIRED`
- Add explicit abandonment/cancellation for suspended local work if needed

## 12. Open Questions

- **Exact package boundaries:** Should `card/`, `client/`, `server/`, and
  `remoteagent/` all exist initially, or should the first pass start with fewer
  packages and split later?
- **SDK dependency shape:** How much of the official A2A Go SDK should be
  wrapped versus exposed directly?
- **Cancellation mapping:** Is a session-level abandon/cancel operation needed
  immediately, or can the first server adapter treat cancellation as an adapter-
  local concern?
- **Task backend API:** What is the narrowest transport-neutral abstraction that
  lets local and A2A-backed tasks coexist without overengineering the
  experimental task layer?
- **Streaming granularity:** What is the right artifact-update strategy so
  remote consumers get meaningful progress without overwhelming event volume?
