# Runtime Context and System Reminders

Dive uses typed reminders to give an agent runtime context that the end user did
not type: environment facts, surfaced memory, skill catalogs, mode changes,
budget notices, and tool-loop guidance.

Providers render these blocks in a recognizable form:

```xml
<system-reminder name="environment">
Working directory: /srv/app
</system-reminder>
```

The name is easy to misread. A `<system-reminder>` is not necessarily a system
message, and it is unrelated to scheduled user reminders. The tag is a wire
marker. The enclosing provider message role determines authority, while the
delivery API determines lifetime and persistence.

Dive keeps reminders as `llm.ReminderContent` inside the application and session
layers. Rendering to XML-like text happens only when a provider builds its
request. This lets applications inspect or hide genuine reminders without
mistaking user-authored lookalike text for injected context.

## Choose a tier and lifetime

Every reminder has a validated name, a tier, and content:

```go
type Reminder struct {
    Name    string       // [a-z][a-z0-9-]*
    Tier    ReminderTier // contextual or operator
    Content string
}
```

Choose the tier based on who asserts the fact:

| Tier       | Use for                                                                                                   | Do not use for                                                              |
| :--------- | :-------------------------------------------------------------------------------------------------------- | :-------------------------------------------------------------------------- |
| Contextual | Environment details, memory, catalogs, retrieved data, background results, or tool-produced notifications | Application-asserted mode, budget, or policy changes                        |
| Operator   | Mode switches, budget limits, or other facts asserted directly by the application operator                | Raw model output, tool output, retrieved documents, or remote-agent content |

Operator tier can raise instruction priority when the provider supports it. It is
never an enforcement mechanism. Use permissions, denying hooks, sandboxing, and
application authorization for controls that must hold.

Then choose how long the reminder should live:

| Lifetime   | Allowed tiers          | API                                                            | Model lifetime                                          | Recorded |
| :--------- | :--------------------- | :------------------------------------------------------------- | :------------------------------------------------------ | :------- |
| Recorded   | Contextual or operator | `NewReminderMessage`, `hctx.AppendReminder(..., Recorded)`     | Conversation history until it leaves the active context | Yes      |
| Model-only | Contextual or operator | `WithModelOnlyReminder`, `hctx.AppendReminder(..., ModelOnly)` | Remainder of the current `CreateResponse`               | No       |

“Recorded” means Dive includes the message in `OutputMessages` or the active
session turn. Without a session, the caller still owns long-term storage.

## Append model-only context

Use model-only reminders for values that should be recomputed or supplied on
each request, such as the current directory, branch, feature catalog, or
account limits:

```go
environment, err := dive.NewContextReminder(
    "environment",
    "Working directory: /srv/app\nOS: linux",
)
if err != nil {
    return err
}

response, err := agent.CreateResponse(ctx,
    dive.WithInput("Inspect the staging deployment."),
    dive.WithModelOnlyReminder(environment),
)
```

Model-only reminders are appended in order at the request tail without mutating
the caller's messages. They remain available through later tool iterations in
the same `CreateResponse`, then disappear. A later reminder with the same name
supersedes an earlier one for model interpretation; Dive does not rewrite or
remove the earlier block.

Appending at the tail preserves the long conversation prefix. When a previous
turn's model-only reminder disappears on the next request, cache reuse can stop
at that prior insertion point, but it does not invalidate the session from the
first user message. At session start there is no special case: the model-only
reminder is simply appended after the initial input. Use `Recorded` instead when
the fact should become conversation history.

Pass `WithModelOnlyReminder` on a `CreateResponse`, or install a
`PreGeneration`/`PreIteration` hook that calls `hctx.AppendReminder` with
`ModelOnly`:

```go
Hooks: dive.Hooks{
    PreIteration: []dive.PreIterationHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            branch, dirty := currentGitState(ctx)
            reminder, err := dive.NewContextReminder(
                "workspace",
                fmt.Sprintf("Branch: %s\nDirty: %t", branch, dirty),
            )
            if err != nil {
                return err
            }
            return hctx.AppendReminder(reminder, dive.ModelOnly)
        },
    },
}
```

## Append a recorded reminder

Append facts that happened at a point in the conversation. Between turns, put
the reminder after its accompanying user message:

```go
mode, err := dive.NewOperatorReminder(
    "mode",
    "Auto-approve is off. Ask before mutating state.",
)
if err != nil {
    return err
}

response, err := agent.CreateResponse(ctx,
    dive.WithMessages(
        llm.NewUserTextMessage("Continue the deployment."),
        dive.NewReminderMessage(mode),
    ),
)
```

The order matters for providers with mid-conversation operator roles. The
sequence `assistant → user → operator reminder` can be legal where
`assistant → operator reminder` is not. If native placement is unavailable,
Dive falls back to a tagged user message.

`NewReminderMessage` creates input, so it is recorded by definition. From a
hook, request the same behavior explicitly:

```go
return hctx.AppendReminder(mode, dive.Recorded)
```

Recorded reminders are append-only. Appending another reminder with the same
name does not rewrite history; `FindLatestReminder` returns the newest one.

## Append model-only guidance from hooks

Use `ModelOnly` for a nudge that should affect the rest of the current agent run
without appearing in `OutputMessages` or session history:

```go
Hooks: dive.Hooks{
    PostToolUseFailure: []dive.PostToolUseFailureHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            reminder, err := dive.NewOperatorReminder(
                "recovery",
                "The last tool call failed. Change the path, input, permissions, or approach before retrying.",
            )
            if err != nil {
                return err
            }
            return hctx.AppendReminder(reminder, dive.ModelOnly)
        },
    },
}
```

Hook-appended reminders are delivered at the next iteration boundary, after the
complete tool-result batch. When tools run in parallel, reminders from tool hooks
follow tool-call declaration order rather than nondeterministic completion order.
Model-only reminders survive additional tool iterations and Stop-hook re-entry
inside the same `CreateResponse`, then disappear.

## Provider rendering and fallback

Contextual reminders render as tagged user content on every provider. Operator
reminders use the strongest role Dive knows the endpoint and model can legally
accept:

| Target                                                               | Operator reminder rendering       |
| :------------------------------------------------------------------- | :-------------------------------- |
| OpenAI Responses API, first-party endpoint                           | `developer` input item            |
| Anthropic first-party API with a supported model and legal placement | Mid-conversation `system` message |
| Other, unsupported, unknown, or illegally placed targets             | Tagged user message               |

The fallback is silent and intentionally best-effort. An operator reminder that
falls back to user role has weaker authority. Dive does not reject the request,
because anything that requires a hard guarantee belongs outside the model.

Raw `llm.System` or `llm.Developer` messages are not reminder messages. Dive
does not normalize their roles or make them portable.

Every `Agent` adds one fixed sentence to its system prompt so models know how to
interpret reminders even before the first one appears:

> Runtime context may appear in `<system-reminder>` blocks. The enclosing
> message role determines its authority; the tag itself does not confer
> authority. Later reminder blocks with the same name supersede earlier ones.

This stable priming rule avoids changing the system-prompt prefix only when a
reminder first appears. It also says that a later reminder with the same name
supersedes an earlier one without requiring history rewrites.

The [runtime context design and contract](../design/context-injection.md)
contains the complete endpoint, model, and placement matrix.

## Sessions, compaction, and replay

Stored Dive sessions contain typed `ReminderContent` JSON, not rendered
`<system-reminder>` text. Provider rendering happens again when the conversation
is replayed, so the same recorded reminder can use a different legal role with a
different provider.

Model-only reminders are never saved. Recorded reminders remain in the full
transcript. If compaction moves one outside the active model window, it is still
auditable but no longer influences the model. Applications should reassert
long-lived state after compaction:

```go
if _, ok := dive.FindLatestReminder(activeMessages, "mode"); !ok {
    // Re-append the current mode before continuing.
}
```

Reminder deliveries emitted by a tool hook are held until the whole parallel
batch completes. If a partial suspend interrupts that batch, deliveries from an
earlier partial resume are not carried across the suspend boundary. Reassert
standing state when the run resumes.

## Inspect and filter reminders safely

Use typed helpers for application UIs, audits, and deduplication:

```go
latest, ok := dive.FindLatestReminder(messages, "mode")
one, ok := dive.FindReminder(message, "mode")
withoutMode := dive.RemoveReminder(messages, "mode")
withoutAny := dive.StripReminders(messages)
```

These helpers use copy-on-write and inspect only `ReminderContent`. They never
remove user-authored text, even if that text contains a convincing
`<system-reminder>` tag.

`ParseLegacyReminderText` exists for old sessions created with the plain-text
API. It is heuristic and must not drive provenance-sensitive hiding, security
decisions, or authorization.

## Legacy plain-text API

`SetSystemReminder`, `RemoveSystemReminder`, and `HasSystemReminder` remain for
compatibility. They mutate plain-text blocks in the first user message, cannot
express operator tier or recording lifetime, and do not have typed provenance.

Use `Reminder`, `WithModelOnlyReminder`, `NewReminderMessage`, and
`AppendReminder` for new agent integrations. A later typed reminder supersedes
a same-name legacy block for model interpretation without rewriting the loaded
session.

## Experimental CLI

The experimental CLI exposes static reminders directly:

```bash
dive --print \
  --context 'environment=cwd=/srv/app' \
  --operator-reminder 'mode=read only' \
  'Inspect the project.'
```

`--context NAME=TEXT` is repeatable and appended model-only on every request.
`--operator-reminder NAME=TEXT` is repeatable and appended after the first user
input.

### Try dynamic context demos

Five opt-in demos derive context from the live agent loop:

```bash
dive --print \
  --context-demo all \
  'Inspect this project, make one small improvement, and verify it.'
```

Use `--context-demo` more than once, or pass a comma-separated set:

```bash
dive --context-demo pipeline,verification --context-demo security
```

Run `dive context-demos` to list the current presets. In interactive mode,
compact trace lines show reminder lifecycle events. `/context` displays the
exact latest-turn demo payloads without relying on the model to recall them.
Skill and application reminders are outside that diagnostic view.

- `workspace` appends a model-only `workspace-pulse` when the Git branch or dirty
  paths change.
- `pipeline` appends a model-only read-only `delivery-pipeline` map. In Go
  workspaces it also adds bounded module topology and `gofmt`/test/vet/race
  guidance.
- `verification` carries edit debt until a later direct check and tracks
  normalized build, test, analysis, and security gate outcomes.
- `recovery` appends model-only guidance after a failed or denied tool call.
- `security` requests model-only review after sensitive edits or high-impact
  commands. It is intentionally quiet during ordinary work.

The demos are advisory and turn-local. Their repository and command classifiers
are bounded heuristics, not evidence that a gate passed, a vulnerability exists,
or a change is approved. Use `--workspace`/`-w` with the Git root when the agent
needs repo-wide build and test access.
