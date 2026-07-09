# Runtime Context Injection

Dive can supply runtime context that the end user did not type while keeping
that content structurally distinct from user-authored text. Reminders are typed
inside Dive and become `<system-reminder>` blocks only when a provider encodes
the request.

Use contextual reminders for user-adjacent facts such as environment details,
surfaced memory, catalogs, and tool-produced notifications. Use operator
reminders only for facts asserted by the application itself, such as a mode or
budget change.

## Pin stable request context

Pinned reminders are contextual, model-only overlays. Supply them on every
request where they should be visible:

```go
environment, err := dive.NewContextReminder(
    "environment",
    "Working directory: /srv/app\nOS: linux",
)
if err != nil {
    return err
}

response, err := agent.CreateResponse(ctx,
    dive.WithInput("Deploy staging."),
    dive.WithPinnedReminder(environment),
)
```

Dive renders the reminder into a copy of the first user message. It does not
mutate the caller's messages or persist the overlay in a session.

## Append a conversation event

Append late-arriving context after the accompanying user message. Input is
recorded by definition:

```go
mode, _ := dive.NewOperatorReminder(
    "mode",
    "Auto-approve is off. Ask before mutating state.",
)

response, err := agent.CreateResponse(ctx,
    dive.WithMessages(
        llm.NewUserTextMessage("continue"),
        dive.NewReminderMessage(mode),
    ),
)
```

Providers use the strongest operator role they are known to support. OpenAI's
Responses API uses `developer`; Anthropic Opus 4.8 on the first-party endpoint
uses a legal mid-conversation `system` message. Other targets use a tagged user
message.

Set `OperatorAuthority: dive.OperatorAuthorityStrict` on `AgentOptions`, or use
`dive.WithOperatorAuthority(dive.OperatorAuthorityStrict)` for one request, to
return `dive.ErrOperatorAuthorityUnavailable` instead of accepting a fallback.

## Inject from hooks

Hook injection is delivered at an iteration boundary after the complete tool
result batch:

```go
Hooks: dive.Hooks{
    PostToolUse: []dive.PostToolUseHook{
        func(ctx context.Context, hctx *dive.HookContext) error {
            reminder, _ := dive.NewOperatorReminder("budget", "Wrap up now.")
            return hctx.AppendReminder(reminder, dive.Recorded)
        },
    },
}
```

Use `dive.ModelOnly` for a reminder that should live only through the current
`CreateResponse` call. `hctx.PinReminder` updates the pinned overlay. In
parallel tool batches, reminders retain tool-call declaration order even when
tools finish out of order.

`dive.FindLatestReminder`, `dive.FindReminder`, `dive.RemoveReminder`, and
`dive.StripReminders` operate only on typed reminders. They never hide text a
user wrote. `dive.ParseLegacyReminderText` is available only for migrating old
plain-text sessions and is intentionally heuristic.

## Experimental CLI

The experimental CLI exposes the same paths as a demo platform:

```bash
dive --print \
  --context 'environment=cwd=/srv/app' \
  --operator-reminder 'mode=read only' \
  --strict-operator-authority \
  'Inspect the project.'
```

`--context NAME=TEXT` is repeatable and pinned on every request.
`--operator-reminder NAME=TEXT` is repeatable and appended after the first user
input. Strict mode is useful for testing provider capability and placement.

For the full contract and provider matrix, see the
[context injection design](../design/context-injection.md).
