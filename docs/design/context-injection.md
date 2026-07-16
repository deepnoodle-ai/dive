# Runtime Context Injection: Design and Contract

**Status:** Implemented
**Last updated:** 2026-07-10

This document records Dive's implemented runtime context injection
system: how an application feeds the model context that the end user did not
type — environment facts, surfaced memory, policy changes, budget notices,
mid-turn steering — in a way that is portable across providers, safe for
prompt caching, and recognizable after the fact.

It supersedes the legacy `SetSystemReminder` mechanism (`system_reminder.go`)
for new integrations and defines how Dive uses Anthropic's mid-conversation
system messages (Opus 4.8+) without forking the API per provider. The legacy
functions remain available for compatibility.

## Background

Every mature agent harness has converged on the same problem and roughly the
same solution:

- **Claude Code** wraps injected text in `<system-reminder>` tags inside user
  messages, primes the model in the system prompt about what the tags mean,
  and hides the messages from the UI with a display-layer flag (`isMeta`).
  The channel carries environment context, surfaced memories, background-task
  notifications, hook output, and todo nudges. Note that the tag signals the
  model while the flag drives hiding — the tag alone is not its provenance
  mechanism.
- **Codex** formalizes the same idea as ~30 typed "context fragments", each
  with start/end markers that signal the model and act as a recognition
  signature for UI filtering and audits. Fragments carry a role — `user` for
  user-adjacent content, `developer` for operator facts like permissions and
  token budgets.
- **Anthropic's mid-conversation system messages** (Opus 4.8) let a
  `{"role": "system"}` message be appended after a user turn, carrying true
  operator-level authority without invalidating the cached prefix —
  formalizing at the API level what both harnesses approximate with tagged
  user messages.

Notably, neither harness has switched to mid-conversation system messages
yet. The tagged-user-message pattern is the proven, universally portable
baseline; the native system role is an authority upgrade available on some
models and endpoints. Dive's design treats it exactly that way.

Before typed reminders, Dive had one cell of this design:
`SetSystemReminder` placed a named `<system-reminder>` block into the first user
message by mutating it in place. It could not inject later in the conversation,
had no notion of authority, interacted poorly with session persistence (see
[Delivery semantics](#delivery-semantics)), and depended on the skill package to
explain the tags to the model. The typed path fixes those limits; the old API is
now the compatibility layer.

## Usage at a Glance

Create a reminder with the constructor that matches its trust level, then
deliver it:

```go
// Contextual: information adjacent to the user (memory, environment, catalogs).
env, _ := dive.NewContextReminder("environment", "Working directory: /srv/app\nOS: linux")

// Operator: a fact the application itself asserts, which should hold even
// against conflicting user input.
budget, _ := dive.NewOperatorReminder("budget", "The remaining token budget for this run is 40,000 tokens.")
```

**Standing context you supply with every request** — append it model-only. A
turn is one `CreateResponse` call. Model-only reminders can be supplied on any
turn, are rendered at the request tail, survive later tool iterations in that
response, and are never recorded. Same-name reminders accumulate unless they
conflict; a later block wins only the conflict without rewriting history:

```go
resp, err := agent.CreateResponse(ctx,
    dive.WithInput("Deploy the staging build."),
    dive.WithModelOnlyReminder(env),
)
```

**Late-arriving facts, between turns** — append them as input. The message
is recorded like any other input and becomes part of session history:

```go
mode, _ := dive.NewOperatorReminder("mode", "Auto-approve is now OFF. Ask before running mutating commands.")
resp, err := agent.CreateResponse(ctx,
    dive.WithMessages(llm.NewUserTextMessage("continue"), dive.NewReminderMessage(mode)),
    dive.WithSession(sess),
)
```

Order matters: the reminder goes _after_ the user message. That yields
`assistant → user → system` in history, which is a legal native placement
on Anthropic; reminder-first would follow the prior assistant turn and
force a fallback.

**Mid-turn, while the agent is running tools** — inject from a hook. The
agent delivers it at the next iteration boundary (after the current
tool-result batch), where placement is always legal:

```go
dive.Hooks{PostToolUse: []dive.PostToolUseHook{
    func(ctx context.Context, hctx *dive.HookContext) error {
        if spent := meter.Spent(); spent > softLimit {
            r, _ := dive.NewOperatorReminder("budget", fmt.Sprintf("%d tokens spent; wrap up.", spent))
            return hctx.AppendReminder(r, dive.Recorded) // or dive.ModelOnly for this request only
        }
        return nil
    },
}}
```

**Checking state / re-asserting after compaction:**

```go
if _, ok := dive.FindLatestReminder(messages, "mode"); !ok {
    // The reminder fell out of the active window (e.g. compaction) — re-append it.
}
```

What happens underneath, in one line each: reminders travel as typed
content blocks (never confusable with user text), providers render them to
`<system-reminder>` tags at the wire, and operator-tier reminders get the
strongest authority the target is _known_ to support — a native `system`
message on Anthropic Opus 4.8, a `developer` item on OpenAI, and a tagged
user message everywhere else. Appending never invalidates the prompt-cache
prefix. The downgrade to a tagged user message is silent: operator authority
raises instruction priority, never enforces (Non-Goal 6), so a weaker role is
a weaker instruction rather than a failed request.

| I want to…                                     | Use                                                                  |
| :--------------------------------------------- | :------------------------------------------------------------------- |
| Provide per-request context (env, catalog)     | `WithModelOnlyReminder`                                              |
| Assert a fact for the rest of the conversation | `NewReminderMessage` as input, or `hctx.AppendReminder(r, Recorded)` |
| Nudge the model for this request only          | `hctx.AppendReminder(r, ModelOnly)`                                  |
| Check whether a reminder is still in context   | `FindLatestReminder`                                                 |
| Hide reminders in my UI                        | typed strip helpers (never the legacy text parser)                   |

The rest of this document defines the contracts behind these calls. For a
task-oriented introduction, see the
[Runtime Context and System Reminders guide](../guides/context-injection.md).

## Goals

1. **One primitive** — A single appended reminder concept with a validated name
   (identity), a tier (authority), and a lifetime (recorded or model-only).
2. **Portable with graceful upgrade** — The same application code works on
   every provider. Where native operator-authority rendering is _known_ to
   be supported (Anthropic system, OpenAI developer), Dive uses it;
   everywhere else — including targets whose behavior is unknown — it uses
   the tagged-user-message pattern Claude Code ships today.
3. **Cache-conscious by construction** — Appending never rewrites the cached
   prefix. When a prior model-only reminder disappears on the next request,
   reuse can stop at its recent tail position rather than at the first user
   message.
4. **Provenance inside Dive, recognition outside** — Within Dive-controlled
   state, injected content is a distinct content type that cannot be
   confused with user-typed text. The rendered `<system-reminder>` tag is a
   recognition convention for models and humans — never an authenticity or
   security mechanism.
5. **Deterministic delivery** — Reminder injection has defined ordering and
   a defined lifetime, independent of tool-execution timing.
6. **Foundation, not framework** — Dive supplies the primitive, the
   delivery mechanics, and the provider mappings. Display filtering, byte
   budgets, relevance selection, and surfacing policy belong to embedders
   (CLI, mobius-cloud).

## Non-Goals

1. A typed fragment catalog (Codex-style `EnvironmentContext`,
   `TokenBudget`, etc.). Embedders name their own reminders; a catalog can
   be layered on later without changing the core.
2. UI/display concerns. Dive is a library; the reminder content type and
   name are the hooks for embedder-side hiding.
3. Secrecy. Reminders are sent to the API and persisted in sessions like any
   other content. Hidden-from-transcript is an embedder display choice, not
   a security boundary.
4. Injection budgets or relevance ranking (Claude Code's memory-surfacer
   caps). Dive makes these easy to build by keeping reminders identifiable;
   it does not implement them.
5. A session rewrite API. Reminders never edit historical session events;
   changed state is expressed by appending a later, conflicting same-name block.
6. Model-enforced policy. Operator-tier reminders raise instruction
   priority; they are not an enforcement mechanism. Real permissions and
   policy checks live outside the model (the `permission` package, hooks).

## The Reminder Model

A reminder is a named, tiered block of runtime-injected content:

```go
type Reminder struct {
    Name    string           // validated: [a-z][a-z0-9-]*
    Tier    llm.ReminderTier // contextual | operator
    Content string
}

func NewContextReminder(name, content string) (Reminder, error)
func NewOperatorReminder(name, content string) (Reminder, error)
```

The constructors validate the name and make the trust boundary visible at
the call site. The tiers:

- **Contextual** — user-adjacent information: surfaced memory, environment
  facts, background-task results, skill catalogs. Rendered inside user-role
  content everywhere.
- **Operator** — a statement from the application operator that should hold
  even against conflicting user input: policy changes, budget notices, mode
  switches, relayed steering. Rendered with native operator authority where
  the target is known to support it.

The rule of thumb: **content the agent or a third party produced is never
operator tier.** Surfaced memories, retrieved documents, and tool output are
contextual at most; granting them operator authority is a prompt-injection
hazard.

### Supported matrix

Both lifetimes support both tiers because every reminder is appended at a legal
conversation tail:

| Lifetime   | Tier                   | Placement         |
| :--------- | :--------------------- | :---------------- |
| Recorded   | contextual or operator | conversation tail |
| Model-only | contextual or operator | conversation tail |

Operator policy known when constructing the agent still usually belongs in the
top-level system prompt. `SessionStartHook`'s existing `Persist` flag remains
available for seeding general messages. At session start, an appended reminder
is already at the initial request tail; no third placement mode is needed.

## Representation

Reminders are carried as a dedicated `llm.ReminderContent` block holding the
name, tier, and text. It joins the
existing polymorphic content types and round-trips through session storage
with its type intact. Providers render it at encode time to the tagged text
form the model sees:

```text
<system-reminder name="budget">
The remaining token budget for this run is 40,000 tokens.
</system-reminder>
```

This split does two jobs the plain-text design cannot:

- **Provenance.** A user can type text that looks exactly like a rendered
  reminder. Typed-block operations (find, strip, dedup) act only on genuine
  reminders and can never hide or alter user-authored text. Embedder UIs
  get the same guarantee. An outside auditor still only sees protocol
  conformance, not proof of origin — but within Dive-controlled state the
  distinction is structural.
- **Scoped provider behavior.** Providers can distinguish reminder messages
  from arbitrary `llm.System` messages and apply portability rules only to
  the former (see below).

**Tier lives in the content block and the block is authoritative.**
`ReminderTier` is defined in `llm` (the root `dive` package aliases it —
`dive` imports `llm`, so the dependency can only point that way). The
enclosing message role is _derived_ from the tier at construction: operator
reminders become standalone `Role: llm.System` messages, contextual
reminders live in user-role messages. If a hand-built message disagrees
with its block's tier, providers normalize to the block's tier. Because
tier is stored in the block, it survives persistence and replay against a
different provider.

Note that stored Dive sessions contain the typed reminder JSON (with its
`name` field), not the rendered `<system-reminder>` tags — the tags exist
only in provider wire traffic. Auditing stored sessions greps the typed
form; auditing captured API traffic greps the tag.

## Provider Rendering

Providers translate reminders at encode time. The rendering contract for
reminder messages is: **rendering never knowingly emits an unsupported
reminder role or placement** — it downgrades to a tagged user message
instead. (Remote services can always reject a request for unrelated reasons;
the contract is about what Dive knowingly sends.) The contract applies to reminder messages only — a raw
`llm.System` message constructed by the caller passes through verbatim,
exactly as today: this design does not change the meaning of an existing
low-level role. Callers using the raw role own the compatibility
consequences.

Capability resolution for operator reminders:

- **Known native support** → native operator role.
- **Unknown or known-unsupported** → tagged user-role rendering. Unknown
  means fallback — never "send the native role and hope."

Dive ships a single policy: best-effort. There is no configuration axis and no
error path for authority — the downgrade is always silent. A strict "native
authority or error" mode is a deliberate follow-up (see
[Implementation status](#implementation-status)), deferred until a concrete
consumer needs a hard guarantee, because operator authority is not an
enforcement mechanism (Non-Goal 6) and no current caller does.

| Provider (basis)                                           | Operator reminder rendering                                                                                                                                                                                                             |
| :--------------------------------------------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Anthropic — first-party API, model supports it (Opus 4.8+) | Known-supported: native mid-conversation `system` message.                                                                                                                                                                              |
| Anthropic — Bedrock/Vertex, or older models                | Known-unsupported: tagged user-role message.                                                                                                                                                                                            |
| OpenAI (Responses API, first-party)                        | Known-supported: `developer`-role input item — the role this provider already uses for the top-level prompt, and the choice Codex made.                                                                                                 |
| Grok (x.ai Responses-compatible endpoint)                  | Unknown: tagged user-role message. Grok embeds Dive's OpenAI provider, but x.ai's documentation demonstrates only `system`/`user` roles; `developer` support is unproven. Promote to known-supported only via a provider contract test. |
| Mistral, OpenRouter, generic Chat Completions              | Unknown: tagged user-role message. (Mistral documents the system prompt as conversation-leading; OpenRouter behavior depends on the routed model.) Native rendering can be enabled per target as support is verified.                   |
| Ollama (Anthropic-compatible endpoint)                     | Unknown (depends on local server/model): tagged user-role message.                                                                                                                                                                      |
| Google (Gemini)                                            | Known-unsupported: tagged user-role message (Gemini contents allow only `user`/`model`).                                                                                                                                                |

Capability detection is a function of **endpoint + model**, not model
alone — Anthropic's feature is first-party-API (plus AWS Claude Platform /
Microsoft Foundry) and excludes Bedrock and Vertex. The Anthropic provider
already gates automatic caching on the endpoint
(`supportsAutomaticCaching`); native system-role rendering follows the same
pattern alongside the model gate in `features.go`.

### Placement

Anthropic's placement rules: a system message must immediately follow a
user turn (or qualifying server-tool-use turn), must precede an assistant
turn or end the array, is never first, and never sits between a `tool_use`
and its `tool_result`. Dive's mid-turn injection point (the iteration
boundary after a complete tool-result batch) satisfies this by
construction — but not every seam does. Between-turn appends can follow an
assistant message, and the Stop-hook continuation seam always does.

Providers therefore normalize **every** recognized reminder message in a
position where the native role is illegal — not just leading or consecutive
ones — by rendering it in the tagged-user form. Placement and capability are
treated identically: either one that rules out the native role produces the
same silent tagged-user fallback.

Placement is judged per message against the caller's ordering, so
**adjacent operator reminder messages all demote**: each breaks the other's
seam, mirroring Anthropic's no-consecutive-system-messages rule. Callers that
need several operator facts at one seam should put them in a single reminder.
Merging adjacent operator reminders into one native system message is a
possible future refinement, not part of this design.

### Fallback is an authority downgrade

When an operator reminder renders as a tagged user message, it loses its
"holds against conflicting user input" property — that is a semantic
change, not a cosmetic one. The design makes this explicit rather than
hiding it:

- The current policy is **best-effort only**: render with the strongest authority the
  target is known to support, downgrading silently on unknown capability or
  illegal placement. There is no per-agent or per-request configuration knob.
- The silent downgrade is acceptable because anything that actually matters
  is enforced outside the model (Non-Goal 6). Operator authority raises
  instruction priority; it is not a guarantee, so there is nothing for a
  failed request to protect.
- A **strict** mode — "native authority or a typed error before the network
  call" — is a deliberate follow-up and is not shipped. It was cut because
  no current consumer needs a hard authority guarantee, and adding the
  configuration axis (agent- and request-scoped, threaded through the
  provider layer) ahead of that need is speculative surface. When a real
  consumer appears, strict mode can be added without changing the primitive:
  the resolver already reports whether the native role is available, so a
  policy layer can turn "not available" into an error instead of a fallback.
  Deferring it also keeps Dive-internal injections (Stop-hook continuation,
  background results) from having to reason about a mode they must never trip.

## Delivery Semantics

The legacy `SetSystemReminder` mutates caller-owned message objects in
place. Combined with how the agent persists turns (input messages plus
generated output; historical messages are never rewritten), mutation
produces two opposite failure modes: an intended-ephemeral reminder written
into a fresh conversation's input message gets persisted with it, while an
intended-durable update to a legacy reminder in loaded history is
model-visible for one request and then lost. The skill package exhibits a
concrete case: its catalog hash check can accept a stale persisted catalog
block after a mid-session change, because presence is checked by name, not
content.

The typed delivery path is therefore **agent-owned and copy-on-write**. A
free function that returns ordinary messages cannot guarantee "model-only"
— the caller could pass its output back as input and Dive would save it. So
the model-only paths are expressed only through the agent:

```go
dive.WithModelOnlyReminder(reminder)           // request input, model-only

hctx.AppendReminder(reminder, dive.Recorded)   // from hooks: recorded
hctx.AppendReminder(reminder, dive.ModelOnly)

dive.NewReminderMessage(reminder)              // between turns: builds an
                                               // input message (recorded by
                                               // definition — it is input)
```

The two lifetimes:

- **Recorded reminders** flow through the same path as generated
  messages: into the model-facing history _and_ `OutputMessages`, so
  session saving captures them naturally. Without a session, they are
  returned on the response for the caller to persist — which is why the
  lifetime is called _recorded_ rather than _durable_: Dive records it; storage
  is wherever the conversation lives.
- **Model-only reminders** join the model-facing working history only. Their
  lifetime is precise: from append through the remainder of the current
  `CreateResponse`, including subsequent tool iterations and Stop-hook
  re-entry, and excluded from `OutputMessages` and persistence. (The
  existing `PreIteration` slice-rewrite remains as the low-level escape
  hatch with the same non-persistence semantics.)

All reminders are append-only. Same-name blocks remain jointly applicable when
they carry independent facts or instructions; a later block wins only where it
conflicts with an earlier block. Nothing edits an earlier model-facing or saved
block. This structurally fixes the skill package's staleness case: the catalog
header is a completeness claim ("any skill not listed here is unavailable"),
so a new catalog snapshot conflicts with the old snapshot — including removed
skills — without rewriting caller-owned history. Snapshot-shaped blocks must
state completeness in their content this way; a merely additive list never
conflicts with an earlier one, and absence alone asserts nothing. A later
typed reminder can likewise override conflicting facts in a same-name legacy
text block for model interpretation.

### Ordering

Parallel tool execution completes in nondeterministic order: PostToolUse
hooks fire in completion order while tool outcomes are retained in
declaration order (see `executeToolCallsParallel`). A shared FIFO reminder
queue would make recorded history nondeterministic across runs.

The delivery invariant: **reminders injected from per-tool hooks attach to
that tool's outcome and are drained in tool-call declaration order** —
exactly how tool results and `AdditionalContext` already behave. Reminders
injected from non-tool hooks (PreIteration, Stop) drain in hook
registration order. This invariant gets a regression test alongside the
existing parallel-tool ordering tests.

The invariant holds within the resume round that completes the batch.
Recorded reminders from a per-tool hook that fired in an _earlier_ partial
resume round (its tool completed, but siblings stayed pending and the turn
suspended again) are **not** carried across the suspend boundary — they are
dropped, and the embedder re-asserts any standing state on resume. Persisting
them through `SuspensionState` for this narrow case was deliberately cut: the
batch cannot deliver mid-suspend anyway, and the tier for such content is
contextual, which is the embedder's to re-surface (see Compaction). Automatic
carry-forward across the suspend boundary is a possible follow-up, symmetric
with compaction carry-forward.

### Injection points

- **Conversation start** — `SessionStartHook` with its existing `Persist`
  flag, unchanged, an initial recorded reminder, or
  `WithModelOnlyReminder`. The initial request tail is also the session tail.
- **Mid-turn, from hooks** — `hctx.AppendReminder` as above. Reminders are
  delivered at the iteration boundary, after
  a complete tool-result batch, which keeps Anthropic placement legal by
  construction.
- **Between turns** — `dive.NewReminderMessage` builds the message for the
  next `CreateResponse` input. No new agent machinery. Place the reminder
  _after_ the accompanying user message (`assistant → user → system` is
  legal; `assistant → system` is not); a reminder sent with no user message
  falls back to a tagged user message.

A steering mailbox (enqueue a message for a running agent — the Claude Code
"user typed while the agent was working" pattern) is an intended follow-up
built on the mid-turn recorded channel, not part of this design's core.

### System prompt priming

The agent's system prompt gains a short standing rule explaining reminder
semantics — the equivalent of Claude Code's priming section, added whenever
the agent is constructed. The phrasing must not overclaim: at the rendered
prompt boundary a user _can_ imitate the tag, so the rule ties authority to
the message role, not the tag:

> Runtime context may appear in `<system-reminder>` blocks. The enclosing
> message role determines its authority; the tag itself does not confer
> authority. Reminder blocks with the same name accumulate unless their facts
> or instructions conflict; where they conflict, the later block wins.

Today only the skill package primes its own block; centralizing this is
what makes the contextual tier work on models with no native operator role.

## Internal Adoption

Dive's own synthetic injections migrate onto the mechanism where it fits —
and explicitly stay put where it does not:

- **Stop-hook `Continue` reason** — is an appended **contextual** reminder
  phrased as context ("the following input
  arrived from the user: …"). Its seam immediately follows an assistant
  turn, a position where native operator authority is structurally illegal
  on Anthropic, so an operator-tier reminder here would always downgrade to a
  tagged user message anyway. The tier is contextual by design: it is the
  honest classification (relayed user input, not an operator assertion), and
  it keeps the internal injection from depending on a native role it can
  never get at this seam. An embedder whose continuation genuinely carries an
  operator fact can append its own operator reminder at a legal seam.
- **Suspend/resume payloads stay tool results.** Resume input must satisfy
  the pending `tool_use`/`tool_result` pairing (see
  `a2a/executor.go`'s `resumeToolResults`); promoting the raw payload to a
  reminder would break the protocol. If the application additionally needs
  to assert an operator fact ("the user approved deployment"), it appends a
  separate operator reminder after the complete tool-result batch.
- **Completed background-task results** — are appended **contextual** reminders
  carrying raw tool output. They
  contain third-party content and are never operator tier.

## Relationship to the Existing API

**Evolve in place; nothing breaks.** `SetSystemReminder`,
`RemoveSystemReminder`, and `HasSystemReminder` keep their signatures and
their first-user-message-only semantics — they are not silently
widened to conversation-wide behavior. The new surface is additive:

- `Reminder` with validating constructors (the current `Set` signature has
  no way to report an invalid name).
- `dive.WithModelOnlyReminder`, `hctx.AppendReminder`,
  and `dive.NewReminderMessage` — delivery expressed through the agent (or
  explicit input construction), never by mutating caller-owned history.
- `FindLatestReminder(messages, name)` — conversation-wide, latest-wins
  lookup over typed blocks.
- **Typed and legacy recognition are separate APIs.** Typed operations
  (`FindReminder`, `StripReminders`, removal) act on `ReminderContent`
  blocks only — provenance-safe, suitable for UI hiding and dedup. A
  clearly named heuristic parser (`ParseLegacyReminderText`) recognizes the
  plain-text form in pre-migration sessions; it can mistake user-authored
  text for injected content and must not drive security-sensitive
  filtering. The current `strings.HasPrefix` discriminator is too
  permissive to export as-is; the legacy parser matches complete,
  well-formed blocks only.

The skill package appends its catalog model-only. Its catalog header states
completeness, so a later catalog snapshot conflicts with and replaces stale
legacy catalog facts without rewriting loaded history; when no skills remain
it appends an explicit no-skills notice rather than an empty block, since an
empty block asserts nothing and cannot conflict. Its public API is untouched.

## Compaction

Dive compaction is non-destructive: the active window starts at the latest
checkpoint, and earlier events remain in the log for `AllMessages` but
leave the model's context. Recorded reminders therefore **expire from
model context at compaction** while remaining auditable in the full
transcript.

The implemented contract leaves long-lived state (an active mode, a standing
policy) for the embedder to reassert. `FindLatestReminder` over the active
window makes "is my reminder still in context?" a one-liner. Automatic
carry-forward of the latest reminder per name across a compaction checkpoint is
a natural follow-up in the compaction package, not part of the current contract.
(Claude Code's surfacer gets this behavior implicitly: its transcript-scan dedup
resets at compaction, allowing re-surfacing.)

## Security Considerations

- **Prompt-authority discipline.** Operator tier is reserved for facts the
  application itself asserts. Content that originated from the model,
  tools, retrieved documents, or remote agents must not be promoted to
  operator tier, no matter how it is wrapped. This governs prompt
  authority only — actual enforcement is external (Non-Goal 6).
- **The tag is not authenticity.** User input can imitate the rendered
  form; only the typed content block within Dive-controlled state indicates
  genuine injection, and even that proves protocol conformance in storage,
  not origin, to an outside auditor. Legacy text parsing is heuristic and
  excluded from security-sensitive paths by API design.
- **Fallback weakens authority.** When an operator reminder downgrades to a
  tagged user message it silently loses priority; this is never a substitute
  for out-of-model enforcement, which is where anything that matters lives.
- Reminders are visible in API traffic and stored sessions. Embedders that
  hide them from a transcript view are cleaning the display, not concealing
  data.
- The constrained name charset keeps reminders greppable — as typed JSON in
  stored sessions, as rendered tags in captured wire traffic.

## Implementation status

The shipped implementation includes:

1. **Core** — `Reminder` type and constructors, `llm.ReminderTier` +
   `llm.ReminderContent` block, `NewReminderMessage`, conversation-wide
   typed lookup, typed strip helpers, the separate legacy parser, system
   prompt priming rule.
2. **Provider rendering** — reminder-scoped rendering contract across all
   providers; capability registry (known-native / unknown / unsupported)
   keyed on endpoint + model; Anthropic native pass-through; OpenAI
   developer-role mapping (Grok stays tagged-user until a contract test
   proves developer-role support on x.ai); best-effort position
   normalization for all illegal placements; contract tests per provider.
3. **Delivery** — agent-owned recorded/model-only paths,
   `WithModelOnlyReminder` / `hctx.AppendReminder`,
   model-only lifetime, declaration-order draining with regression tests,
   skill package migration (model-only append + conflicting legacy handling).
4. **Internal adoption** — Stop-hook reasons and background-task completion
   messages use typed contextual reminders (resume payloads stay tool results).

Potential follow-ups require separate designs:

1. **Strict operator-authority mode** — native-or-error behavior, added when a
   concrete consumer needs a hard guarantee.
2. **Steering mailbox** — enqueue input while an agent is already running.
3. **Compaction carry-forward** — re-surface selected long-lived reminders.
4. **Embedder helpers** — byte budgets, deduplication, and a typed fragment
   catalog if repetition demands it.

## References

- Anthropic: Mid-conversation system messages (Claude API docs) — placement
  rules, endpoint availability, caching interplay
- Claude Code `<system-reminder>` + `isMeta` research:
  `mobius-cloud/docs/research/technical/2026-07-09-claude-system-reminders.md`
- Codex context fragments research:
  `mobius-cloud/docs/research/technical/2026-07-09-codex-system-reminders.md`
- Implementation: `reminder.go`, `llm/reminder.go`, `system_reminder.go`, `skill/agent.go`
  (catalog hook), `providers/anthropic/anthropic.go` (`convertMessages`,
  `applyCaching`, `supportsAutomaticCaching`), `agent.go`
  (`executeToolCallsParallel` ordering, Stop-hook continuation seam),
  `background.go` (`backgroundCompletedMessage`), `a2a/executor.go`
  (`resumeToolResults`), `session/session.go` (compaction checkpoint model)
