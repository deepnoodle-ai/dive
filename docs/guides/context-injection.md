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
uses a legal mid-conversation `system` message. Every other target — and any
placement where the native role would be illegal — falls back to a tagged user
message. The fallback is silent: operator authority is never an enforcement
boundary (real policy checks live in the `permission` package and hooks), so a
downgraded reminder is a weaker instruction, not a failed request.

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
  'Inspect the project.'
```

`--context NAME=TEXT` is repeatable and pinned on every request.
`--operator-reminder NAME=TEXT` is repeatable and appended after the first user
input.

### Try dynamic context demos

The CLI also includes eight opt-in demos that derive context from the live agent
loop:

```bash
dive --print \
  --context-demo all \
  'Inspect this project, make one small improvement, and verify it.'
```

Use `--context-demo` more than once, or pass a comma-separated set, to run only
the patterns you want:

```bash
dive --context-demo pipeline,quality --context-demo security
```

Run `dive context-demos` to list every preset and its purpose. In interactive
mode, each reminder lifecycle event produces a compact trace line. `/context`
then shows the exact latest-turn context-demo payloads without relying on the
model to remember or describe them. Skill and application reminders are outside
that diagnostic view.

- `workspace` replaces a pinned `workspace-pulse` before each model call with
  the current Git branch and dirty paths. It demonstrates live overlays without
  persisting a stale snapshot.
- `sources` pins an `evidence-ledger` built from successful file reads, searches,
  directory listings, and web fetches during the current response. The ledger
  records what was consulted; it does not claim that a source is correct. Its
  deterministic list is capped at 12 entries and reports omitted observations.
- `verification` appends model-only operator reminders after `Write` and `Edit`,
  then reports a checkpoint when a later tool batch successfully runs a
  recognized direct test, lint, or check command. The verifier must be the final
  shell segment so masked commands such as `go test || true` cannot clear debt.
  A check launched in parallel with an edit intentionally does not clear the
  debt, and tracked paths use the same 12-entry bound.
- `recovery` appends a model-only operator reminder after a failed tool call,
  identifying the failed input and asking the model to change its retry strategy.
- `pipeline` pins a read-only `delivery-pipeline` map derived from recognized
  build files, allowlisted package scripts and Make targets, CI configuration,
  containers, and dependency automation. It exposes only fixed labels and
  counts—not file contents or workflow names—and reports presence rather than
  claiming a gate ran. Discovery reads at most 64 KiB from a recognized file
  and samples at most 256 workflow-directory entries, so gaps in larger
  repositories are intentional.
- `go` pins a `go-workflow` reminder when the workspace contains a Go module or
  workspace. It reports the declared Go version, bounded nested-module counts,
  and an advisory `gofmt`/test/vet/race-check loop. It explicitly warns that
  root-module checks do not cover nested modules automatically.
- `quality` pins a `quality-gates` ledger for recognized build, test,
  static-analysis, and security commands observed during the current response.
  It records normalized labels and tool outcomes; a failed or blocked gate
  dominates a passing observation in the same category.
- `security` appends a model-only operator `security-review` trigger after
  sensitive file changes or high-impact dependency, privilege, credential,
  cryptography, and deployment commands. It contains fixed risk categories and
  counts only, so repository or shell text is not promoted to operator
  authority. The trigger is advisory, not a vulnerability finding or approval.
  Its categories are heuristic risk indicators, not a complete policy model or
  security audit.

These presets demonstrate reminder delivery and authority. They are advisory
and turn-local; they are not enforcement boundaries. Use permissions and
denying hooks for hard policy.

Pinned reminders are refreshed request overlays. Model-only reminders disappear
after the response and are not available for the model to recall on a later
turn; `/context` is the reliable diagnostic surface. When the selected workspace
is below the Git root, the splash and model prompt also identify that boundary.
Use `--workspace`/`-w` with the Git root when the agent should be able to inspect
and test the whole repository.

For the full contract and provider matrix, see the
[context injection design](../design/context-injection.md).
