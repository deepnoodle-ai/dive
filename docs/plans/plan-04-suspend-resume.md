---
Title: Agent Suspend/Resume — Technical Design
Author: Curtis Myzie
Status: Draft
Last Updated: 2026-04-10
PRD: tasks/prd-agent-suspend-resume.md
---

# Agent Suspend/Resume — Technical Design

## 1. Overview

This document describes the concrete implementation of the suspend/resume
primitive specified in `tasks/prd-agent-suspend-resume.md`. The design is
scoped to three surfaces:

1. **Tool author surface.** A tool's `Call` can signal suspension by returning
   a `*ToolResult` whose new `Suspend` field is non-nil. No new interface, no
   change to the `Tool.Call` signature, no new return values.
2. **Agent loop internals.** `generate`, `executeToolCallsParallel`, and
   `executeToolCallsSequential` learn a new terminal state: "batch contained at
   least one suspend." The loop unwinds cleanly, `PostToolUse` is skipped for
   suspending tools, and completed siblings are preserved.
3. **Caller surface.** `CreateResponse` accepts a new `WithToolResults(...)`
   option. The returned `Response` gains a `Status` field, `PendingToolCalls`,
   and `CompletedToolCalls`. A new `OnSuspend` hook fires when the agent
   transitions into a suspended state. A new `ResponseItemTypeSuspended` is
   emitted to streaming callbacks.

The design is **additive** — nothing existing is renamed, removed, or changes
shape. Tools that never suspend pay zero cost: the new `Suspend` field is a
nil pointer check.

## 2. Design Summary

At a high level:

- Suspend is carried **in-band** via `ToolResult.Suspend`. Tool authors use a
  helper like `dive.NewSuspendResult("...")` or construct the result directly.
  This avoids introducing a parallel return type or modifying `Tool.Call`.
- Suspend state is persisted **on the session**, not in the response. The
  session gains an authoritative `Suspended` bool and a `PendingToolCallIDs`
  slice. The structural invariant (assistant message with unmatched tool_use
  blocks) is a correctness check — the persisted flag is what `CreateResponse`
  trusts at the top of the call.
- The suspended `Response` is informational — it tells the caller what to
  resume with. All durable state lives in the session.
- Resume is modeled as **another call to `CreateResponse`** on the same
  session, with `WithToolResults(...)` supplying the external results. The
  agent detects resume mode via the session's `Suspended()` method and skips
  the first LLM call, because the assistant turn that produced the tool_use
  blocks already exists in history.

## 3. Core Types

### 3.1 `SuspendResult` and the tool-author helper

Add to `tool.go`:

```go
// SuspendResult is the signal a tool returns to pause the agent mid-turn and
// hand control back to the caller. It is NOT an error — it is a first-class
// result type. The agent observes it, persists the partial turn, and returns
// a suspended Response. The tool's ToolResult is kept in the session but
// carries no Content: the LLM never sees it until resume supplies a real
// result.
//
// SuspendResult is carried on ToolResult.Suspend; tools construct it via
// NewSuspendResult, not by returning it as an error.
type SuspendResult struct {
    // Prompt is optional human-readable text describing what the agent is
    // waiting for. Surfaced to the caller via PendingToolCall.Prompt so a
    // SaaS UI can render "Waiting for approval from @alice" without the
    // integrator having to reverse-engineer the tool input.
    Prompt string `json:"prompt,omitempty"`

    // Metadata is optional structured data the tool wants to attach. It is
    // opaque to Dive and round-trips through the session unchanged. Useful
    // for request IDs, form URLs, expiration hints, etc.
    Metadata map[string]any `json:"metadata,omitempty"`
}

// NewSuspendResult returns a *ToolResult whose Suspend field is set. Use it
// from a tool's Call method:
//
//     return dive.NewSuspendResult("Waiting for approval from @alice"), nil
func NewSuspendResult(prompt string) *ToolResult {
    return &ToolResult{Suspend: &SuspendResult{Prompt: prompt}}
}
```

And extend `ToolResult`:

```go
type ToolResult struct {
    Content []*ToolResultContent `json:"content"`
    Display string               `json:"display,omitempty"`
    IsError bool                 `json:"isError,omitempty"`

    // Suspend, when non-nil, tells the agent to suspend its turn rather than
    // send this tool result to the LLM. Content, Display, and IsError are
    // ignored when Suspend is set. Only the suspended call's ID, name, and
    // input are retained (the agent reads them from the ToolUseContent).
    Suspend *SuspendResult `json:"suspend,omitempty"`
}
```

Rationale for choosing in-band-on-ToolResult over a sentinel error or a new
interface:

- `Tool.Call` keeps its `(*ToolResult, error)` signature, satisfying the
  "additive only" constraint (FR-15 in the PRD).
- `errors.As` is not a good fit: a SuspendResult is not an error, it's a
  deferral, and modeling it as an error fights the existing
  `PostToolUseFailure` hook semantics.
- A separate `SuspendingTool` interface doubles the tool-author surface and
  forces tools that *sometimes* suspend to choose between two interfaces.

### 3.2 Response status and pending/completed tool calls

Add to `response.go`:

```go
// ResponseStatus indicates the terminal state of a CreateResponse call.
type ResponseStatus string

const (
    // ResponseStatusCompleted is the default: the agent finished normally.
    // An empty Status is treated as Completed for backward compatibility.
    ResponseStatusCompleted ResponseStatus = "completed"

    // ResponseStatusSuspended means one or more tool calls in the final
    // iteration returned SuspendResult. The agent has persisted the partial
    // turn to its session and expects a future CreateResponse call with
    // WithToolResults to supply the missing tool outputs.
    ResponseStatusSuspended ResponseStatus = "suspended"
)

// PendingToolCall describes a tool call awaiting an external result.
type PendingToolCall struct {
    ID       string          `json:"id"`
    Name     string          `json:"name"`
    Input    json.RawMessage `json:"input"`
    Prompt   string          `json:"prompt,omitempty"`   // from SuspendResult.Prompt
    Metadata map[string]any  `json:"metadata,omitempty"` // from SuspendResult.Metadata
}

// CompletedToolCall describes a tool call that ran to completion in the same
// iteration where a sibling suspended. The result is already persisted in the
// session; this struct is informational so the caller's UI can show "here's
// what we finished before pausing."
type CompletedToolCall struct {
    ID     string          `json:"id"`
    Name   string          `json:"name"`
    Input  json.RawMessage `json:"input"`
    Result *ToolResult     `json:"result,omitempty"`
    Error  string          `json:"error,omitempty"` // Go-level error, if any
}
```

Extend `Response`:

```go
type Response struct {
    Model          string          `json:"model,omitempty"`
    Items          []*ResponseItem `json:"items,omitempty"`
    OutputMessages []*llm.Message  `json:"output_messages,omitempty"`
    Usage          *llm.Usage      `json:"usage,omitempty"`
    CreatedAt      time.Time       `json:"created_at,omitempty"`
    FinishedAt     *time.Time      `json:"finished_at,omitempty"`

    // Status is ResponseStatusCompleted for normal returns, or
    // ResponseStatusSuspended when at least one tool returned SuspendResult.
    // An empty Status means Completed (back-compat).
    Status ResponseStatus `json:"status,omitempty"`

    // PendingToolCalls is populated when Status == ResponseStatusSuspended.
    PendingToolCalls []*PendingToolCall `json:"pending_tool_calls,omitempty"`

    // CompletedToolCalls lists tool calls that ran alongside the suspending
    // sibling(s) in the suspended iteration. Only populated when
    // Status == ResponseStatusSuspended. For the terminal iteration only;
    // earlier iterations are already reflected in OutputMessages and Items.
    CompletedToolCalls []*CompletedToolCall `json:"completed_tool_calls,omitempty"`
}
```

### 3.3 `WithToolResults` and `CreateResponseOptions`

Add to `dive.go`:

```go
type CreateResponseOptions struct {
    Messages      []*llm.Message
    EventCallback EventCallback
    Values        map[string]any
    Session       Session

    // ToolResults, when non-nil, indicates a resume. Keys are tool_call IDs
    // from a prior suspended Response's PendingToolCalls; values are the
    // results the caller obtained out-of-band. See WithToolResults.
    ToolResults map[string]*ToolResult
}

// WithToolResults supplies externally-obtained tool results to resume a
// previously suspended agent. The keys are tool_call IDs taken from a prior
// Response.PendingToolCalls. The values are caller-constructed ToolResults;
// an IsError result flows through the PostToolUseFailure path as if the tool
// itself had failed.
//
// If the caller supplies results for only a subset of pending IDs, the agent
// stays suspended and returns a new suspended Response listing the remaining
// pending calls. If any supplied ID is not in the pending set, CreateResponse
// returns an error without mutating session state.
//
// Resume is not safe to call concurrently on the same session — session
// stores serialize writes, but the caller must not race two resume calls.
func WithToolResults(results map[string]*ToolResult) CreateResponseOption {
    return func(opts *CreateResponseOptions) {
        opts.ToolResults = results
    }
}
```

Also add a sentinel error:

```go
// ErrNoSuspendedState is returned from CreateResponse when WithToolResults is
// supplied but the session is not in a suspended state.
var ErrNoSuspendedState = errors.New("dive: session is not suspended")

// ErrUnknownPendingToolCall is returned when WithToolResults contains an ID
// that is not in the session's pending set.
var ErrUnknownPendingToolCall = errors.New("dive: unknown pending tool call id")
```

### 3.4 `OnSuspend` hook

Add to `hooks.go`:

```go
// OnSuspendHook fires when the agent transitions into a suspended state,
// BEFORE PostGeneration runs. Use this to notify an external system that
// human input is needed (email, Slack, task creation, webhook dispatch).
//
// hctx.Response is populated with the suspended Response (Status =
// ResponseStatusSuspended, PendingToolCalls, CompletedToolCalls). Errors are
// logged and do NOT change the returned Response — suspend is already
// persisted by the time OnSuspend fires.
type OnSuspendHook func(ctx context.Context, hctx *HookContext) error

type Hooks struct {
    PreGeneration      []PreGenerationHook
    PostGeneration     []PostGenerationHook
    PreToolUse         []PreToolUseHook
    PostToolUse        []PostToolUseHook
    PostToolUseFailure []PostToolUseFailureHook
    Stop               []StopHook
    PreIteration       []PreIterationHook
    OnSuspend          []OnSuspendHook // NEW
}
```

Extension merging in `NewAgent` gains one more line. `HookContext` needs no
new fields — `Response`, `OutputMessages`, and `Usage` are already populated
at the point OnSuspend fires.

### 3.5 Streaming: `ResponseItemTypeSuspended`

Add to `response.go`:

```go
const (
    // ... existing constants ...

    // ResponseItemTypeSuspended is a terminal item emitted when the agent
    // transitions into a suspended state. The Suspended field carries the
    // same pending/completed lists as Response. Stream consumers should
    // treat this as end-of-stream and then observe Response.Status.
    ResponseItemTypeSuspended ResponseItemType = "suspended"
)

type ResponseItem struct {
    // ... existing fields ...
    Suspended *SuspendedItem `json:"suspended,omitempty"`
}

type SuspendedItem struct {
    PendingToolCalls   []*PendingToolCall   `json:"pending_tool_calls,omitempty"`
    CompletedToolCalls []*CompletedToolCall `json:"completed_tool_calls,omitempty"`
}
```

## 4. Agent Loop Changes

### 4.1 New internal types

In `agent.go`:

```go
// toolCallOutcome is the per-tool-call result of an executeToolCalls batch.
// Exactly one of Result or Pending is non-nil on a successful return.
type toolCallOutcome struct {
    // Result is set for tool calls that completed (success, denied, failure).
    Result *ToolCallResult
    // Pending is set for tool calls that returned a SuspendResult.
    Pending *PendingToolCall
}

// toolBatchResult aggregates per-call outcomes for one LLM iteration.
type toolBatchResult struct {
    // Outcomes is index-aligned with the input toolCalls slice. For a
    // sequential batch that unwound early, trailing entries may be zero-
    // valued (both Result and Pending nil) — those are "not started" and
    // must be executed on resume.
    Outcomes []toolCallOutcome

    // Suspended is true if any Outcome has Pending != nil.
    Suspended bool
}
```

### 4.2 `executeToolCalls` dispatch

The existing signature changes to return `*toolBatchResult`:

```go
func (a *Agent) executeToolCalls(
    ctx context.Context,
    hctx *HookContext,
    toolCalls []*llm.ToolUseContent,
    toolsByName map[string]Tool,
    callback EventCallback,
) (*toolBatchResult, error)
```

### 4.3 `executeToolCallsSequential`

```go
func (a *Agent) executeToolCallsSequential(
    ctx context.Context,
    hctx *HookContext,
    toolCalls []*llm.ToolUseContent,
    toolsByName map[string]Tool,
    callback EventCallback,
) (*toolBatchResult, error) {
    batch := &toolBatchResult{Outcomes: make([]toolCallOutcome, len(toolCalls))}
    for i, toolCall := range toolCalls {
        result, err := a.executeOneToolCall(ctx, hctx, toolCall, toolsByName, callback)
        if err != nil {
            return nil, err
        }
        if result.Result != nil && result.Result.Suspend != nil {
            // Capture as pending. Subsequent tool calls in the sequential
            // batch are intentionally skipped (their Outcome stays zero).
            batch.Outcomes[i] = toolCallOutcome{
                Pending: toPendingToolCall(toolCall, result.Result.Suspend),
            }
            batch.Suspended = true
            return batch, nil
        }
        batch.Outcomes[i] = toolCallOutcome{Result: result}
    }
    return batch, nil
}
```

Note: `executeOneToolCall` is responsible for NOT running `PostToolUse` or
`PostToolUseFailure` hooks when the result's `Suspend` field is set. The
tool's `PreToolUse` already ran (the tool was allowed, it ran, it just
decided to suspend). See §4.5.

### 4.4 `executeToolCallsParallel`

Phase 1 (sequential PreToolUse) is unchanged.

Phase 2 (parallel execution, draining) changes in three places:

1. When a tool completes with `result.Result.Suspend != nil`, mark its
   outcome as Pending instead of Result.
2. Do NOT run `PostToolUse` / `PostToolUseFailure` hooks for that tool.
3. Do NOT cancel `childCtx` — other tools must still run to completion
   (FR-8). Cancellation only happens on a true fatal error from a drained
   goroutine.

Sketch:

```go
for remaining > 0 {
    ct := <-ch
    remaining--

    if ct.err != nil {
        cancel()
        return nil, ct.err
    }

    i := ct.index
    result := ct.result
    prep := preps[i]

    if result.Result != nil && result.Result.Suspend != nil {
        batch.Outcomes[i] = toolCallOutcome{
            Pending: toPendingToolCall(toolCalls[i], result.Result.Suspend),
        }
        batch.Suspended = true

        // Still emit a tool-call result event so stream consumers see that
        // this tool terminated — but mark it as a suspend event by reusing
        // ResponseItemTypeToolCallResult with result.Result.Suspend set, so
        // callers can filter. (Alternative: emit a dedicated pending event.
        // See §7.2.)
        if err := callback(ctx, &ResponseItem{
            Type:           ResponseItemTypeToolCallResult,
            ToolCallResult: result,
        }); err != nil {
            return nil, err
        }
        continue
    }

    // ... existing PostToolUse / PostToolUseFailure handling ...
    batch.Outcomes[i] = toolCallOutcome{Result: result}
}
return batch, nil
```

The `continue` (rather than early-return) is critical: FR-8 requires
non-suspending siblings to finish, and the completion-order drain already
handles this without structural change.

### 4.5 `executeOneToolCall`

After the tool runs but before hooks, check `result.Result.Suspend`:

```go
if result.Result != nil && result.Result.Suspend != nil {
    // Emit a tool-call-result event (optional — §7.2) and return.
    // PostToolUse/PostToolUseFailure are intentionally skipped.
    if err := callback(ctx, &ResponseItem{
        Type:           ResponseItemTypeToolCallResult,
        ToolCallResult: result,
    }); err != nil {
        return nil, err
    }
    return result, nil
}
```

The caller (`executeToolCallsSequential`) inspects the returned result and
classifies it as pending.

### 4.6 `generate` changes

Three changes:

1. `generate`'s return value carries suspend state.
2. After `executeToolCalls`, if `batch.Suspended` is true, build a *partial*
   tool_result message containing only completed outcomes' tool_results,
   append it to outputMessages, and return early with the suspended snapshot.
3. Add a new input `resumeState` that, when non-nil, tells `generate` to
   short-circuit the first iteration's LLM call and instead proceed directly
   to tool execution (for sequential "not started" tools) or directly to
   appending the reconstituted tool_result message.

New signature:

```go
func (a *Agent) generate(
    ctx context.Context,
    hctx *HookContext,
    messages []*llm.Message,
    systemPrompt string,
    callback EventCallback,
    model llm.LLM,
    resume *resumeState, // nil on a fresh call
) (*generateResult, error)
```

New `generateResult`:

```go
type generateResult struct {
    OutputMessages []*llm.Message
    Items          []*ResponseItem
    Usage          *llm.Usage

    // Suspended is non-nil if the final iteration unwound into a suspended
    // state. CreateResponse is responsible for persisting + returning.
    Suspended *suspendedSnapshot
}

type suspendedSnapshot struct {
    PendingToolCalls   []*PendingToolCall
    CompletedToolCalls []*CompletedToolCall
    PendingToolCallIDs []string // authoritative, saved to session flag
}
```

Suspended path inside the loop:

```go
batch, err := a.executeToolCalls(ctx, hctx, toolCalls, toolsByName, collectingCallback)
if err != nil {
    return nil, err
}

// Build a partial tool_result message from completed outcomes only.
completedResults := collectCompleted(batch.Outcomes)
var toolResultMessage *llm.Message
if len(completedResults) > 0 {
    toolResultMessage = llm.NewToolResultMessage(getToolResultContent(completedResults)...)
    for _, tc := range getAdditionalContextContent(completedResults) {
        toolResultMessage.Content = append(toolResultMessage.Content, tc)
    }
    newMessage(toolResultMessage)
}

if batch.Suspended {
    return &generateResult{
        OutputMessages: outputMessages,
        Items:          items,
        Usage:          totalUsage,
        Suspended: &suspendedSnapshot{
            PendingToolCalls:   collectPending(batch.Outcomes),
            CompletedToolCalls: buildCompletedInfo(batch.Outcomes),
            PendingToolCallIDs: collectPendingIDs(batch.Outcomes),
        },
    }, nil
}

// Normal path: everything completed, continue to next iteration.
```

**Critical invariant:** the partial tool_result message is saved to the
session but is NEVER sent to the LLM. The agent returns before the next
iteration's LLM call. On resume, the tool_result message is reused (extended
with the caller-supplied results) and *then* sent to the LLM.

### 4.7 Resume path inside `generate`

When `resume != nil`:

- Skip the first LLM call: use the last assistant message from
  `updatedMessages` as the "response" for this iteration. Its tool_use blocks
  are the tool calls.
- The partial tool_result message is already the last message in
  `updatedMessages` (CreateResponse stitched it before calling generate).
- If `resume.NotStartedToolCalls` is non-empty (sequential skipped tools),
  execute them now using `executeToolCallsSequential`. Any of them may
  themselves suspend, in which case we unwind into another suspended state.
- Otherwise, fall through to the next iteration's LLM call normally.

A minimal sketch of the resume branch:

```go
if resume != nil {
    // updatedMessages already ends with the completed tool_result message,
    // so the next LLM call (the fall-through iteration) will have a
    // coherent conversation.
    if len(resume.NotStartedToolCalls) > 0 {
        batch, err := a.executeToolCallsSequential(
            ctx, hctx, resume.NotStartedToolCalls, toolsByName, collectingCallback,
        )
        if err != nil {
            return nil, err
        }
        // Merge these results into the last tool_result message (the one
        // we already appended during CreateResponse resume prep).
        if err := mergeBatchIntoLastToolResult(updatedMessages, batch); err != nil {
            return nil, err
        }
        if batch.Suspended {
            // Nested suspend during resume — unwind again.
            return &generateResult{...}, nil
        }
    }
    resume = nil // fall through to normal iteration
}
```

### 4.8 `CreateResponse` orchestration

The high-level flow:

```go
func (a *Agent) CreateResponse(ctx, opts...) (*Response, error) {
    // 1. Snapshot model + systemPrompt (unchanged)
    // 2. Determine active session
    // 3. Load session messages (unchanged)
    // 4. Check resume mode:
    resume, err := a.detectResume(ctx, sess, options, messages)
    if err != nil {
        return nil, err
    }
    // 5. If resume != nil: stitch messages, do NOT run PreGeneration input
    //    validation (no new user input), skip session append of input msgs.
    // 6. Run PreGeneration hooks (always run — they may inject context even
    //    on resume; existing hook contract).
    // 7. Call generate(..., resume).
    // 8. If result.Suspended != nil:
    //      - Run OnSuspend hooks
    //      - Run PostGeneration hooks with Status=Suspended
    //      - Persist session via SaveSuspendedTurn
    //      - Emit ResponseItemTypeSuspended to callback
    //      - Return (*Response, nil) with Status=Suspended
    // 9. Else:
    //      - Clear suspension flag if it was set (successful resume)
    //      - Run Stop/PostGeneration hooks as today
    //      - Save turn via SaveTurn
    //      - Return (*Response, nil) with Status=Completed
}
```

#### 4.8.1 Resume detection

```go
type resumeState struct {
    // AssistantMessage is the assistant message with the tool_use blocks.
    AssistantMessage *llm.Message

    // ToolResultMessage is the partial tool_result message (may be nil if no
    // sibling completed; rare). After merging caller-supplied results, this
    // message becomes fully satisfied.
    ToolResultMessage *llm.Message

    // PendingBeforeMerge is the set of pending tool_use IDs at the time of
    // suspend, taken from the session's authoritative flag.
    PendingBeforeMerge map[string]struct{}

    // NotStartedToolCalls are sequential tool calls that were skipped
    // because an earlier sibling suspended. They must be executed on resume
    // BEFORE the next LLM call.
    NotStartedToolCalls []*llm.ToolUseContent

    // RemainingPending is the pending set after applying the caller's
    // WithToolResults. If non-empty after resume, the agent re-suspends.
    RemainingPending map[string]struct{}
}

func (a *Agent) detectResume(
    ctx context.Context,
    sess Session,
    options CreateResponseOptions,
    messages []*llm.Message,
) (*resumeState, error) {
    suspendable, _ := sess.(SuspendableSession)
    sessionSuspended := suspendable != nil && suspendable.Suspended()
    hasToolResults := len(options.ToolResults) > 0

    if !sessionSuspended && !hasToolResults {
        return nil, nil
    }
    if hasToolResults && !sessionSuspended {
        return nil, ErrNoSuspendedState
    }
    if sessionSuspended && !hasToolResults && len(options.Messages) > 0 {
        // Caller sent new user input while session is suspended — reject.
        // Cleaner than silently dropping the input. FR-7 implies resume
        // supplies results, not new input.
        return nil, fmt.Errorf("dive: session is suspended; use WithToolResults to resume")
    }

    // Walk tail of messages to find the suspended assistant + partial
    // tool_result messages. The suspended assistant message is the last
    // assistant message with tool_use blocks whose IDs are not fully
    // matched by the subsequent tool_result message.
    rs, err := reconstructResumeState(messages, suspendable.PendingToolCallIDs())
    if err != nil {
        return nil, err
    }

    // Validate supplied results.
    for id := range options.ToolResults {
        if _, ok := rs.PendingBeforeMerge[id]; !ok {
            return nil, fmt.Errorf("%w: %s", ErrUnknownPendingToolCall, id)
        }
    }

    // Merge caller-supplied results into the partial tool_result message.
    // Keep a RemainingPending set for IDs not yet supplied.
    rs.RemainingPending = mergeToolResults(rs, options.ToolResults)
    return rs, nil
}
```

`reconstructResumeState` cross-checks the structural invariant against the
flag. If they disagree, it returns an error — per FR-19 this is a bug and we
fail loudly rather than guess.

#### 4.8.2 Handling partial resume

If after merging, `len(rs.RemainingPending) > 0`, the agent does NOT call
`generate`. It instead:

1. Updates the session's `PendingToolCallIDs` to the remaining set.
2. Saves the updated tool_result message via `SaveSuspendedTurn`.
3. Runs `OnSuspend` and `PostGeneration` hooks.
4. Returns a fresh suspended `Response` listing only the remaining pending
   calls.

This also covers US-007's cancel path: the caller supplies
`IsError: true` results for all pending IDs in a single call — merge
completes, nothing stays pending, generate runs normally, and the LLM reacts
to the error results.

## 5. Session & Store Changes

### 5.1 Optional `SuspendableSession` interface

Per FR-19, suspend state must be queryable authoritatively. Adding methods
to `dive.Session` would technically break third-party implementations. The
design adds an *optional* interface in `dive.go`:

```go
// SuspendableSession is an optional extension of Session. Sessions that
// implement it participate in suspend/resume; the agent prefers the flag
// they expose over the structural invariant. Sessions that do not implement
// it cannot be resumed — any tool returning SuspendResult against such a
// session causes CreateResponse to return an error.
type SuspendableSession interface {
    Session

    // Suspended reports whether the session is currently in a suspended
    // state awaiting external tool results.
    Suspended() bool

    // PendingToolCallIDs returns the tool_call IDs currently awaiting
    // external results. Valid only when Suspended() is true.
    PendingToolCallIDs() []string

    // SaveSuspendedTurn persists a partial turn: messages ending in an
    // assistant tool_use message and an (optionally partial) tool_result
    // message. It must set Suspended=true and record pendingIDs.
    SaveSuspendedTurn(
        ctx context.Context,
        messages []*llm.Message,
        usage *llm.Usage,
        pendingIDs []string,
    ) error

    // ClearSuspension marks the session as no longer suspended and clears
    // the pending set. Called after a successful non-suspended return from
    // CreateResponse that transitioned out of resume mode.
    ClearSuspension(ctx context.Context) error
}
```

### 5.2 `session.Session` implementation

Extend `sessionData` in `session/session.go`:

```go
type sessionData struct {
    ID                 string         `json:"id"`
    Title              string         `json:"title,omitempty"`
    CreatedAt          time.Time      `json:"created_at"`
    UpdatedAt          time.Time      `json:"updated_at"`
    Events             []*event       `json:"events"`
    Metadata           map[string]any `json:"metadata,omitempty"`
    ForkedFrom         string         `json:"forked_from,omitempty"`

    // Suspended tracks the authoritative suspend flag. Cleared on a normal
    // SaveTurn or on ClearSuspension.
    Suspended          bool     `json:"suspended,omitempty"`
    PendingToolCallIDs []string `json:"pending_tool_call_ids,omitempty"`
}
```

New `Session` methods:

```go
func (s *Session) Suspended() bool {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.data.Suspended
}

func (s *Session) PendingToolCallIDs() []string {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return slices.Clone(s.data.PendingToolCallIDs)
}

func (s *Session) SaveSuspendedTurn(
    ctx context.Context,
    messages []*llm.Message,
    usage *llm.Usage,
    pendingIDs []string,
) error {
    evt := &event{
        ID:        newEventID(),
        Type:      eventTypeTurn,
        Timestamp: time.Now(),
        Messages:  messages,
        Usage:     usage,
        Metadata: map[string]any{
            "suspended":      true,
            "pending_ids":    pendingIDs,
        },
    }
    s.mu.Lock()
    defer s.mu.Unlock()

    // Replace the last event if it was also a suspended turn for this same
    // assistant message — otherwise append. This keeps resume idempotent
    // and avoids unbounded event growth across many partial resumes.
    if n := len(s.data.Events); n > 0 {
        last := s.data.Events[n-1]
        if last.Type == eventTypeTurn && s.data.Suspended {
            s.data.Events[n-1] = evt
        } else {
            s.data.Events = append(s.data.Events, evt)
        }
    } else {
        s.data.Events = append(s.data.Events, evt)
    }

    s.data.Suspended = true
    s.data.PendingToolCallIDs = slices.Clone(pendingIDs)
    s.data.UpdatedAt = evt.Timestamp

    if s.appender != nil {
        // For append-only stores, we rewrite the whole session when
        // transitioning suspend state because we may have replaced the
        // last event. putSession is the right primitive.
        return s.appender.putSession(ctx, s.data)
    }
    return nil
}

func (s *Session) ClearSuspension(ctx context.Context) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if !s.data.Suspended {
        return nil
    }
    s.data.Suspended = false
    s.data.PendingToolCallIDs = nil
    s.data.UpdatedAt = time.Now()
    if s.appender != nil {
        return s.appender.putSession(ctx, s.data)
    }
    return nil
}
```

Also: `SaveTurn` (the existing method) clears `Suspended` before appending.
Once the agent successfully completes a resumed turn via `generate`, it
calls `SaveTurn` to write the full, completed turn — which implicitly
ends the suspended state.

Actually no — on a successful resume we want to *replace* the last
suspended event rather than append a new one (the suspended event's
assistant msg is still part of the turn). So the resume-completion path
should:

1. Replace the last (suspended) event with a new event whose Messages
   contain the complete turn (assistant + full tool_result + any follow-up
   iterations).
2. Clear `Suspended` and `PendingToolCallIDs`.

We add a helper `Session.ReplaceLastEventWithCompleted(ctx, messages, usage)`
for this, or we reuse `putSession` through a new `SaveResumedTurn` method.
For clarity:

```go
// SaveResumedTurn replaces the last (suspended) event with a complete turn
// and clears the suspended flag. Called by CreateResponse when a resumed
// generate run completes normally.
func (s *Session) SaveResumedTurn(
    ctx context.Context,
    messages []*llm.Message,
    usage *llm.Usage,
) error
```

`SuspendableSession` gains `SaveResumedTurn` accordingly.

### 5.3 `SessionInfo` and `Store.List()`

Extend `SessionInfo`:

```go
type SessionInfo struct {
    ID         string         `json:"id"`
    Title      string         `json:"title,omitempty"`
    CreatedAt  time.Time      `json:"created_at"`
    UpdatedAt  time.Time      `json:"updated_at"`
    EventCount int            `json:"event_count"`
    Metadata   map[string]any `json:"metadata,omitempty"`

    // Suspended indicates the session is awaiting external tool results.
    Suspended bool `json:"suspended,omitempty"`
}
```

Both `MemoryStore.List` and `FileStore.List` copy `sessionData.Suspended`
into `SessionInfo.Suspended`.

### 5.4 `MemoryStore` round-trip

`MemoryStore` already shares `sessionData` by pointer with `Session`, so
adding fields to `sessionData` is enough — no changes to the store aside
from List populating the new field.

The one subtle point: `SaveSuspendedTurn` calls `putSession` on the
appender, and `MemoryStore.putSession` is a no-op. That's fine, because the
pointer-sharing means the changes are already visible. We keep the no-op
for consistency with existing behavior.

### 5.5 `FileStore` round-trip

`FileStore` currently persists via two paths:

- `appendEvent`: O(1) append of a single event line.
- `writeSession`: full rewrite (used by `putSession`).

The suspend flag and pending IDs live on the `sessionHeader`. Two changes:

1. Extend `sessionHeader`:

   ```go
   type sessionHeader struct {
       ID                 string         `json:"id"`
       Title              string         `json:"title,omitempty"`
       CreatedAt          time.Time      `json:"created_at"`
       UpdatedAt          time.Time      `json:"updated_at"`
       Metadata           map[string]any `json:"metadata,omitempty"`
       ForkedFrom         string         `json:"forked_from,omitempty"`
       Suspended          bool           `json:"suspended,omitempty"`
       PendingToolCallIDs []string       `json:"pending_tool_call_ids,omitempty"`
   }
   ```

2. Ensure `writeSession` writes these fields (trivial — just copy from
   `sessionData`), and `readSession` parses them back into `sessionData`.

Because suspend transitions call `putSession` (full rewrite), the append-only
hot path is untouched for normal turns.

**Round-trip test:** write a session with a partial turn, close the file,
reopen, verify `Suspended() == true` and `PendingToolCallIDs` are the same
and the messages round-trip byte-identical.

### 5.6 Subagent restriction

Per PRD §6, subagents must not suspend in v1. The subagent tool implementation
(in `experimental/subagent`) constructs a child agent and calls
`CreateResponse`. If the child returns `ResponseStatusSuspended`, the
subagent tool wraps it as a regular `NewToolResultError("subagent suspension
is not supported")`. This is a change in `experimental/subagent` only.

## 6. `CreateResponse` Resume Flow (Worked Example)

Given a session in the following state:

```
messages:
  user: "Please approve the purchase order and then send the summary"
  assistant: [tool_use approve_po, tool_use fetch_summary]
  tool_result: [result for fetch_summary]         // partial
Suspended: true
PendingToolCallIDs: ["toolu_approve"]
```

Caller invokes:

```go
resp, err := agent.CreateResponse(ctx,
    dive.WithSession(sess),
    dive.WithToolResults(map[string]*dive.ToolResult{
        "toolu_approve": dive.NewToolResultText("approved by alice"),
    }),
)
```

Flow:

1. Load messages (as above).
2. `detectResume` sees `Suspended=true`, validates the supplied ID is in
   the pending set, builds `resumeState`:
   - `AssistantMessage` = assistant turn
   - `ToolResultMessage` = existing partial tool_result msg (merged to
     append the new result in place)
   - `NotStartedToolCalls` = none (parallel case)
   - `RemainingPending` = empty
3. `generate(ctx, ..., resume)` is called. Because `RemainingPending` is
   empty and `NotStartedToolCalls` is empty, `generate`'s resume branch
   simply falls through to the next iteration, which calls the LLM with a
   complete conversation.
4. LLM produces a final assistant message (no tool_use blocks) → loop
   exits normally.
5. `CreateResponse`:
   - `result.Suspended == nil` → normal path.
   - Calls `sess.SaveResumedTurn(ctx, allTurnMessages, totalUsage)` to
     replace the last (suspended) event with the complete turn.
   - Calls `PostGeneration` hooks with `Status=Completed`.
   - Returns the completed `Response`.

If the caller had supplied only `"toolu_approve"` but pending included two
IDs, step 2 would produce `len(RemainingPending) > 0`, and
`CreateResponse` would NOT call `generate`. It would update the session
via `SaveSuspendedTurn` (with updated tool_result msg and shrunk pending
set) and return a new suspended Response.

## 7. Hooks, Streaming, and Edge Cases

### 7.1 Hook ordering on suspend

```
CreateResponse
  PreGeneration
  generate
    PreIteration
    LLM call
    PreToolUse  (for each tool, including suspender)
    tool.Call   (suspender returns SuspendResult)
    PostToolUse / PostToolUseFailure (SKIPPED for suspender; run for siblings)
  (generate returns suspended=true)
  OnSuspend         ← NEW, fires before PostGeneration
  PostGeneration    ← fires with Response.Status=Suspended
  SaveSuspendedTurn
  return (*Response, nil) with Status=Suspended
```

`Stop` hooks do NOT fire on suspend — they represent "about to produce a
final answer" and a suspended response is not final. This is a judgment
call worth calling out: stop hooks that run compaction, for instance, would
be surprised to run mid-turn.

### 7.2 Streaming: `ResponseItemTypeSuspended` emission

During a suspended iteration, stream events are emitted as normal for:
- the assistant message (tool_use blocks)
- each completed sibling's tool_call_result event

For the suspending tool(s), we emit `ResponseItemTypeToolCallResult` with a
`ToolCallResult` whose `Result.Suspend` is set. Stream consumers that only
care about text content won't be confused; consumers that inspect results
can detect the suspend.

After `generate` returns with suspension, but before `CreateResponse`
returns, emit a terminal:

```go
callback(ctx, &ResponseItem{
    Type: ResponseItemTypeSuspended,
    Suspended: &SuspendedItem{
        PendingToolCalls:   result.Suspended.PendingToolCalls,
        CompletedToolCalls: result.Suspended.CompletedToolCalls,
    },
})
```

This gives stream consumers a typed end-of-stream marker without having to
reconcile against `Response.Status` after the stream ends.

### 7.3 Cancellation during resume

Per FR-23, context cancellation during resume behaves exactly like today:
the loop unwinds at the next cancellation point. This means a partial write
may occur if cancellation happens between `SaveResumedTurn` and the
function return. Documented as a caller-side constraint — don't cancel
during resume if you need atomicity.

### 7.4 Timeout reset

`a.responseTimeout` wraps `ctx` per-call in `CreateResponse` today. Suspend
returns from the call, so the wrapping `cancel()` fires via `defer`. The
next resume call is a fresh `CreateResponse`, so the timeout naturally
resets. FR-21 is satisfied by the existing pattern — no new code needed.

### 7.5 Cancel-by-error (US-007)

The caller cancels a suspended agent by providing an `IsError: true`
`ToolResult` for every pending ID. The merged tool_result message now has
real (error) results for every tool_use block, so the structural invariant
is satisfied, `generate` runs the follow-up iteration, the LLM sees the
errors and produces a final answer (typically a graceful "Okay, I'll stop
and wait for a new instruction"). Also: `PostToolUseFailure` hooks fire for
each error result, per FR-17.

One subtlety: `PostToolUseFailure` is a *tool-level* hook that today fires
from inside `executeToolCalls`. On resume, we're not executing tools at
all — we're injecting pre-computed results. The resume path must explicitly
fire `PostToolUseFailure` (or `PostToolUse`) for each merged result so the
contract holds. Add a `firePostToolUseHooksForResumed` helper.

### 7.6 Concurrent resume

Two goroutines calling `CreateResponse` on the same session simultaneously
with different `WithToolResults` maps would race. We document the
constraint and lean on the session store's internal locking. Both
`MemoryStore` and `FileStore` serialize writes at the store level, so
corruption is prevented, but the outcome (which resume "wins") is
undefined. Callers needing stronger guarantees build a higher-level lock.

### 7.7 Model/system prompt changes between suspend and resume

Per FR-22, `SetModel` and `SetSystemPrompt` between suspend and resume are
honored — the existing snapshot-under-lock pattern at the top of
`CreateResponse` already does this. No new code needed; document it.

## 8. Test Plan

All tests live under `agent_suspend_test.go` (new file) plus targeted
additions to `session/session_test.go`. Uses the project convention
`github.com/deepnoodle-ai/wonton/assert`.

### 8.1 Test fixtures

**Scriptable mock LLM** in `testing/mockllm` (or inline in the test file):

```go
// scriptedLLM returns pre-programmed responses in order. Each call to
// Generate consumes the next script entry. Optionally records the
// messages it received for assertion.
type scriptedLLM struct {
    name    string
    script  []scriptedTurn
    idx     int
    seen    [][]*llm.Message
}

type scriptedTurn struct {
    // Either text (final answer) or tool calls.
    text     string
    toolUses []*llm.ToolUseContent
    usage    llm.Usage
}

func (s *scriptedLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
    // Record received messages, return next script entry.
}
```

**Scriptable mock tool** — a `dive.Tool` whose `Call` is driven by a queue
of outcomes:

```go
type scriptedTool struct {
    name     string
    outcomes []toolOutcome
    idx      int
    calls    []json.RawMessage // recorded inputs
}

type toolOutcome struct {
    result  *dive.ToolResult // may have Suspend set
    err     error
}
```

This lets a test script "first call suspends, second call returns success,
third call suspends again" deterministically.

### 8.2 Core cases

| # | Name | Script | Assertion |
|---|------|--------|-----------|
| 1 | `TestSuspendSimple` | LLM→[tool A], tool A suspends | `resp.Status==Suspended`, `len(pending)==1`, session Suspended=true, no LLM call after suspend |
| 2 | `TestResumeSimple` | as #1, then resume with result | final resp Completed, LLM called once more with full tool_result, session Suspended=false |
| 3 | `TestSuspendResumeSuspendAgain` | LLM turn 1: [A], A suspends; resume with A's result → LLM turn 2: [B], B suspends; resume with B's result → LLM turn 3: final text | final Completed, total 3 LLM calls, session flag toggled correctly across transitions |
| 4 | `TestParallelOneSuspends` | LLM→[A, B], A completes, B suspends (parallelToolExecution=true) | 1 pending, 1 completed in Response; session tool_result msg has only A's result; resume with B → full completion |
| 5 | `TestParallelMultipleSuspend` | LLM→[A, B, C], A completes, B+C suspend | 2 pending, 1 completed; resume with only B → stays suspended with C pending; resume with C → completes |
| 6 | `TestSequentialSuspendSkipsTail` | LLM→[A, B, C], A completes, B suspends (parallelToolExecution=false) | C is NOT called (assert scripted tool C's call count is 0); resume supplying B's result executes C, then final LLM call |
| 7 | `TestResumeUnknownID` | Suspended state with pending [A]; resume with {X: ...} | returns `ErrUnknownPendingToolCall`, session unchanged |
| 8 | `TestResumeNoSuspendedState` | Fresh session; resume with {A: ...} | returns `ErrNoSuspendedState`, session unchanged |
| 9 | `TestResumeErrorResultCancelsTurn` | Suspended on [A]; resume with IsError result for A | PostToolUseFailure fires once, LLM sees error, final completed Response |
| 10 | `TestOnSuspendHookOrder` | Suspended turn with OnSuspend + PostGeneration hooks | OnSuspend runs strictly before PostGeneration; both receive Status=Suspended |
| 11 | `TestOnSuspendHookSeesPending` | OnSuspend hook captures `hctx.Response.PendingToolCalls` | matches expected IDs |
| 12 | `TestStreamingSuspendedItem` | Streaming suspend turn | callback receives `ResponseItemTypeSuspended` as terminal item, with correct pending/completed lists |
| 13 | `TestSuspendNoRegressionForNonSuspendingTools` | Run a normal turn with no suspend | Response.Status is "" or Completed, no new allocations on the hot path (verified via a benchmark or an allocs-per-op assertion) |
| 14 | `TestSetModelBetweenSuspendResume` | Suspend, `agent.SetModel(newModel)`, resume | newModel.Generate is called, not the original |

### 8.3 Store round-trip cases (in `session/session_test.go`)

| # | Name | Assertion |
|---|------|-----------|
| S1 | `TestMemoryStoreSuspendRoundTrip` | `SaveSuspendedTurn` then read via Messages + Suspended + PendingToolCallIDs returns the same values |
| S2 | `TestFileStoreSuspendRoundTrip` | `SaveSuspendedTurn`, close, reopen from disk via `NewFileStore(dir)`, `Open(id)`; verify Suspended=true, PendingToolCallIDs equal, messages byte-identical |
| S3 | `TestFileStoreListReportsSuspended` | Multiple sessions, some suspended; `List()` returns `SessionInfo.Suspended` matching state |
| S4 | `TestSaveTurnClearsSuspension` | After SaveSuspendedTurn, a follow-up SaveTurn (or SaveResumedTurn) clears Suspended + PendingToolCallIDs |
| S5 | `TestCrossProcessResume` | FileStore-backed: suspend in "process A" (first agent instance), instantiate a fresh agent + session from disk, resume; final response matches the single-process baseline |

### 8.4 Integration test across `agent_test.go`

A focused end-to-end test that drives an agent with a scripted LLM + a
real tool backed by a channel (the test goroutine reads the pending tool
call, sends a response via a channel, and the test harness calls resume).
This exercises the public API surface and validates the "<50 lines of
integration code" success metric from the PRD.

### 8.5 Provider-level sanity check

Not automated in CI (requires API keys). A manual checklist:

- Anthropic: suspend + resume with real Claude 4.6, verify the resumed
  request is accepted (no "unmatched tool_use" error).
- OpenAI: same with gpt-5.2.
- Google: same with gemini-2.5-pro.
- Ollama: same with a local model.

The primary defense is that resumed requests never send un-satisfied
tool_use blocks to the provider (FR-7 + §4.7 invariant).

## 9. Migration & Compatibility

- **`Tool` implementors**: no change required. Adding a `Suspend` field to
  `ToolResult` is a purely additive struct change.
- **`Response` consumers**: existing code that ignores `Status` continues to
  work; `Status == ""` means completed.
- **`Session` implementors**: third-party session types continue to work but
  cannot be resumed. If a tool running against such a session returns
  `SuspendResult`, the agent returns a clear error: "session does not
  implement SuspendableSession; suspend/resume unavailable." Upgrading a
  session type to support suspend is purely additive (implement the four new
  methods).
- **`Hooks` struct**: adding a new field (`OnSuspend`) is a struct-field
  addition; zero value is nil-safe.
- **Streaming consumers**: new `ResponseItemTypeSuspended` is only emitted
  when the agent actually suspends. Existing streams are unchanged.
- **Provider packages**: no changes. Providers only see complete, matched
  tool_use/tool_result pairs.
- **Extensions** (`skill`, `compaction`, etc.): no changes required; they
  read `hctx.Response` which now carries `Status`. Extensions that want to
  react specifically to suspension can register an `OnSuspend` hook.

## 10. Open Implementation Questions

These are intentionally deferred until implementation:

1. **Should `ToolCallResult` gain a `Suspend` convenience field** mirroring
   `Result.Suspend` so stream consumers don't have to unwrap? Leaning no —
   one source of truth.
2. **Event ID stability across suspend/resume rewrites**. When
   `SaveSuspendedTurn` replaces the last event, does the event ID change?
   Current sketch generates a new ID; alternative is to preserve the
   original. Preserving seems cleaner for observers. Decide during
   implementation.
3. **`Compact()` interaction with suspended sessions**. Compacting a
   suspended session is almost certainly wrong (it would collapse the
   suspended turn's assistant + partial tool_result into a summary and
   lose resumability). Proposal: `Compact` returns an error if
   `Suspended() == true`. Document and test.
4. **Dedicated `ResponseItemTypePending` event** during streaming for each
   suspending tool, instead of reusing `ToolCallResult` with
   `Result.Suspend` set. Low priority; decide after stream-consumer UX
   feedback.
