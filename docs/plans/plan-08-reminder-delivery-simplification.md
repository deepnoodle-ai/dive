---
Title: Reminder delivery — recorded and model-only
Author: Curtis Myzie
Status: Implemented
Last Updated: 2026-07-10
---

# Reminder delivery: recorded and model-only

**Workflow:** Standard-tier spec followed immediately by implementation in the
same draft PR.

## Context

The typed reminder API originally exposed three delivery shapes: a model-only
value placed in the first user message, an appended recorded reminder, and an
appended model-only reminder. The first shape was called “pinned,” even though it
was neither durable nor fixed. It also rewrote an early prompt position whenever
live context changed.

The first shape is redundant. At session start, a model-only append is already
at the initial request tail. Later, appending preserves the established prompt
prefix. Stable context that belongs in history can be recorded, and operator
policy known before user input usually belongs in the system prompt.

## Goals

- Expose exactly two reminder lifetimes: recorded and model-only.
- Make both lifetimes append-only and chronological.
- Keep lifetime separate from contextual/operator authority.
- Preserve deterministic ordering for parallel tool hooks.
- Migrate the skill catalog, CLI flags, demos, tests, and documentation in the
  same change.

## Non-goals

- Change provider authority resolution or the `<system-reminder>` wire format.
- Make reminders an enforcement mechanism.
- Rewrite recorded history or automatically carry reminders across compaction.
- Remove the legacy plain-text `SetSystemReminder` compatibility helpers.

## Naming exploration

The divergent pass considered these lifetime pairs:

1. recorded / model-only;
2. durable / ephemeral;
3. persistent / transient;
4. history / response-only;
5. retained / temporary;
6. saved / unsaved;
7. committed / provisional;
8. transcript / runtime;
9. stored / volatile;
10. archival / session-local.

The strongest candidates were:

- **Recorded / model-only.** Precisely describes what Dive guarantees. A
  recorded reminder enters `OutputMessages`; a model-only reminder is sent to
  the model but excluded from returned and stored history. Selected.
- **Durable / ephemeral.** Concise, but inaccurate without a session because
  Dive can return a message without durably storing it. “Ephemeral” also leaves
  its exact scope unclear.
- **History / response-only.** Descriptive in prose, but awkward as exported Go
  constants and easy to confuse with message source rather than lifetime.

“Append” remains the operation, not a mode. It accurately describes placement
for both lifetimes and makes chronology obvious.

## Public API

```go
type CreateResponseOptions struct {
    ModelOnlyReminders []Reminder
}

func WithModelOnlyReminder(reminder Reminder) CreateResponseOption

func (hctx *HookContext) AppendReminder(
    reminder Reminder,
    recording ReminderRecording,
) error

const (
    Recorded  ReminderRecording = "recorded"
    ModelOnly ReminderRecording = "model_only"
)
```

`NewReminderMessage` remains the between-turn helper for recorded input. A
message supplied as input is recorded by definition. `WithModelOnlyReminder` is
the direct request helper for the other lifetime.

The pinned API and the intermediate current-reminder API are removed. No
compatibility alias is retained because both were unreleased on the active
branch, and an alias would preserve a third concept.

## Delivery behavior

All reminders are appended in call order:

```text
recorded history
new user or tool-result messages
new recorded or model-only reminders
```

- `Recorded` reminders enter `OutputMessages` and session history.
- `ModelOnly` reminders remain through the current `CreateResponse`, including
  later tool iterations, and then disappear.
- Request-scoped model-only reminders are appended after the initial input.
- Hook reminders are delivered at the next iteration boundary. Per-tool
  deliveries drain in tool declaration order after the complete result batch.
- Contextual reminders use user authority. Operator reminders use a native
  operator role only where the provider and placement support it, with the
  existing tagged-user fallback elsewhere.

Dive does not replace or remove an earlier same-name reminder. The standing
priming rule says that later same-name blocks supersede earlier ones, preserving
append-only history while making refreshed state unambiguous.

### Prompt caching

Appending never rewrites the established prompt prefix. A model-only reminder
from a previous response is absent from recorded history on the next request, so
cache reuse can stop at that prior insertion point. Because the insertion was at
the recent request tail, the long session prefix remains reusable. This avoids
the old behavior where changing live state rewrote the first user message.

### Legacy sessions

A newly appended typed reminder can supersede a same-name legacy plain-text
block for model interpretation. Dive leaves caller-owned and stored messages
unchanged. Typed inspection helpers continue to distinguish genuine reminder
content from legacy text and user-authored lookalikes.

## CLI and built-in adoption

- `--context NAME=TEXT` appends contextual `ModelOnly` reminders on each
  request.
- `--operator-reminder NAME=TEXT` appends recorded operator reminders after the
  first user input.
- The skill catalog appends model-only from its `PreGenerationHook`.
- Workspace, pipeline, and verification-gate demos append a new model-only
  value only when their payload changes during a response.
- Recovery, verification-debt, checkpoint, and security triggers append
  model-only reminders after their triggering tool outcome.
- Interactive traces label the lifetime `model-only` and use `queued` or
  `refreshed` only as UI actions, not delivery modes.

## Alternatives considered

### Rename pinned to current

Rejected after the first implementation pass. Tail placement fixed the cache
problem, but “current” still preserved a separate replacement mode. That did not
actually achieve the requested two-lifetime model.

### Replace same-name model-only reminders in place

Rejected because replacement would recreate a hidden third behavior and make
“append” misleading. Demos instead skip unchanged payloads, while changed
values append chronologically and rely on latest-wins interpretation.

### Use only recorded reminders

Rejected because runtime catalogs, live workspace facts, and retry coaching
should not automatically become conversation history.

## Tradeoffs and consequences

- Repeated changed values within one long response consume context until that
  response ends. Demos suppress identical repeats to bound the common case.
- Recorded same-name reminders remain visible in history; the model relies on
  the latest-wins rule rather than historical rewriting.
- The direct request API is intentionally asymmetric:
  `WithModelOnlyReminder` is needed for non-recorded context, while recorded
  input already has the ordinary `NewReminderMessage` path.
- Removing the unreleased APIs creates a larger branch diff but leaves a smaller
  public model.

## Validation

The implementation is covered by:

1. request-tail and non-persistence tests for `WithModelOnlyReminder`;
2. recorded/model-only hook lifetime tests;
3. chronological same-name and authority-tier tests;
4. deterministic parallel-tool delivery tests;
5. skill and experimental CLI integration tests;
6. root and nested-module test, race, vet, build, and CLI discovery checks.

## Open questions

None. The API now matches the selected two-lifetime model.
