# Model Controls

Dive clamps, translates, and drops reasoning and sampling controls a model
cannot take, and the request succeeds. That keeps one set of options useful
when it is pointed at a different model, but it means the request you sent is
not always the request you wrote. The `modelcaps` package answers the three
questions that follow from that, in the order they arise:

| Question                            | API                     |
| ----------------------------------- | ----------------------- |
| What does this exact model take?    | `modelcaps.ControlsFor` |
| What would Dive send for my config? | `modelcaps.Preview`     |
| What did Dive actually send?        | `llm.Usage.Controls`    |

`ControlsFor` and `Preview` are for publishing a model catalog or validating
controls at an admission boundary. `Usage.Controls` is for every request: it is
how a caller sees a clamp without wiring a logger. Normal Dive requests remain
permissive — known models are clamped or translated when needed, and unknown or
custom models retain their existing pass-through behavior.

## Link the providers you inspect

The registry is populated by provider package `init` functions. Import every
provider your binary expects, even if another package constructs the provider
instances:

```go
import (
    _ "github.com/deepnoodle-ai/dive/providers/anthropic"
    _ "github.com/deepnoodle-ai/dive/providers/google"
    _ "github.com/deepnoodle-ai/dive/providers/grok"
    _ "github.com/deepnoodle-ai/dive/providers/openai"

    "github.com/deepnoodle-ai/dive/providers/modelcaps"
)
```

`Providers` returns a sorted snapshot of the providers registered in the
current binary. Check it at startup so a missing provider module is treated as a
deployment or version error rather than an empty catalog:

```go
expected := []string{"anthropic", "google", "grok", "openai"}
linked := modelcaps.Providers()
for _, provider := range expected {
    if !slices.Contains(linked, provider) {
        return fmt.Errorf("model control resolver %q is not linked", provider)
    }
}
```

Provider names are trimmed and compared case-insensitively. Dive's canonical
xAI provider name is `grok`, not `xai`.

## What an exact model takes

`ControlsFor` returns independent facts for an exact model in the linked
provider's generated catalog:

```go
controls, ok := modelcaps.ControlsFor("google", "gemini-3.7-flash")
if !ok {
    return errors.New("model has no published controls")
}

if !controls.SupportsNativeEffort(llm.ReasoningEffortHigh) {
    return errors.New("high effort is not natively supported")
}
```

The returned `Model` is always the canonical catalog ID. Inputs are trimmed and
compared case-insensitively, with only these provider-specific qualifiers
accepted:

| Provider  | Optional single leading qualifier |
| --------- | --------------------------------- |
| OpenAI    | `openai/`                         |
| Grok      | `x-ai/`                           |
| Anthropic | `anthropic/`                      |
| Google    | `models/`                         |

The boundary is deliberately conservative. OpenRouter paths, Bedrock IDs,
Vertex publisher or deployment paths, fine-tunes, arbitrary gateway IDs, and
unreleased point releases return `ok=false`. A runtime prefix matcher may still
handle those values permissively; prefix inheritance is not evidence about a
model.

### Reasoning fields

- `NativeEfforts` lists values sent through the provider's native effort or
  thinking-level control.
- `EmulatedEfforts` lists recognized effort values Dive translates into a
  manual token budget. It excludes `none` and custom effort strings.
- `Budget` contains fixed effective bounds for manual budgets. A nil bound means
  no fixed bound is published.
- `AdaptiveThinking` reports whether Dive can express provider-selected
  thinking depth.
- `CanDisableThinking` is false both when a model cannot reason and when a model
  always thinks and rejects disablement.
- `Temperature` reports whether temperature is meaningful when no other
  requested control suppresses it.

These fields are static facts, not a compatibility matrix. Use `Preview` for
interactions among effort, budget, thinking, temperature, maximum output,
prefill, tool choice, and provider features.

## Require evidence for the endpoint you use

`VerificationScopes` identifies the endpoint and API surface on which the
entire exact control set was live-probed. A strict publisher checks the same
surface it will call:

```go
if !controls.VerifiedOn(modelcaps.VerificationGoogleVertexAI) {
    return errors.New("controls were not verified on Vertex AI")
}
```

Available built-in scopes are:

- `VerificationOpenAIResponses`
- `VerificationXAIResponses`
- `VerificationAnthropicMessages`
- `VerificationGoogleGeminiAPI`
- `VerificationGoogleVertexAI`

Evidence does not transfer between scopes. For example,
`gemini-3.7-flash` is verified on Vertex AI and does not claim public Gemini API
verification. An empty scope list means the model is deliberately mapped but the
snapshot is historical, inferred, or otherwise not live-probed.

A scope is not proof of current account entitlement, regional availability,
freshness, or successful serving. Record module versions with the published
catalog when rollback provenance matters:

```go
if info, ok := debug.ReadBuildInfo(); ok {
    for _, dependency := range info.Deps {
        if strings.HasPrefix(dependency.Path, "github.com/deepnoodle-ai/dive") {
            log.Printf("%s %s", dependency.Path, dependency.Version)
        }
    }
}
```

## What Dive would send

`Preview` runs the same provider-private control planner used by request
construction. It performs no network I/O and does not mutate the supplied
configuration, so it is the way to reject a request before it costs money:

```go
maxTokens := 4096
temperature := 0.7
budget := 8192
config := llm.Config{
    Model:           "claude-opus-4-6",
    MaxTokens:       &maxTokens,
    Temperature:     &temperature,
    ReasoningBudget: &budget,
}

plan, ok := modelcaps.Preview("anthropic", config)
if !ok {
    return errors.New("model has no published controls")
}
if plan.Rejected {
    return errors.New(plan.RejectionReason)
}
if plan.Budget.Adjusted ||
    plan.Temperature.Action == modelcaps.ControlOmitted {
    return errors.New("Dive would change the requested control semantics")
}
```

Each decision belongs to the logical caller input named by its field. If effort
is translated into a budget, `Effort.Action` is `ControlEmulated`, `Budget` is
`ControlNotRequested`, and `Effective.ReasoningBudget` contains the result.

| Action                | Meaning                                                                    |
| --------------------- | -------------------------------------------------------------------------- |
| `ControlNotRequested` | no caller input; another decision may populate the related effective field |
| `ControlApplied`      | the input is honored through its native semantic control                   |
| `ControlEmulated`     | the input is realized through another effective control                    |
| `ControlOmitted`      | the input has no effect on the request                                     |
| `ControlDefaulted`    | no caller input; a provider default supplies the effective field           |

`Adjusted` is true when an applied or emulated magnitude was clamped or
normalized. `Effective` contains the final provider-neutral values, not a
serialized provider SDK request.

On a successful plan, every decision has a non-empty action and
`RejectionReason` is empty. When `Rejected` is true, the rejection reason is
diagnostic text and all decisions and effective values are unspecified; ignore
them. Consumers should branch on `Rejected`, `Action`, and `Adjusted`, not on
reason strings.

Anthropic currently reports control-related rejections for thinking combined
with a prefilled assistant response or a forced tool choice. Other provider
validation errors outside the model-control contract are still returned by
normal request construction.

## What Dive actually sent

Anthropic, Google, OpenAI, and Grok report the controls the request carried on
`Response.Usage.Controls`, beside `Usage.Speed` and `Usage.ServiceTier`. The
value is an `llm.EffectiveControls` — the same type `Plan.Effective` carries,
produced by the same planner — so a caller can compare what it asked for
against what was served without parsing a provider payload:

```go
response, err := provider.Generate(ctx,
    llm.WithMessages(llm.NewUserTextMessage("hello")),
    llm.WithReasoningEffort(llm.ReasoningEffortMedium),
    llm.WithTemperature(0.7),
)
if err != nil {
    return err
}

if served := response.Usage.Controls; served != nil {
    if served.ReasoningEffort != llm.ReasoningEffortMedium {
        log.Printf("effort was not served natively: budget=%v thinking=%q",
            served.ReasoningBudget, served.Thinking)
    }
    if served.Temperature == nil {
        log.Print("temperature was dropped for this request")
    }
}
```

A zero-valued field means Dive sent nothing for that control: on Sonnet 4.5 a
requested `medium` effort arrives as `ReasoningEffort: ""` with a populated
`ReasoningBudget`, because that model has no native effort parameter. A nil
`Controls` means the provider reports no effective controls at all — providers
outside the four above do not populate it.

Streamed responses report the same value. The controls ride on the usage frames,
so `llm.ResponseAccumulator` carries them onto the accumulated response.

`Usage.Controls` survives aggregation only while the requests agree.
`Usage.Absorb` supersedes, matching cumulative streaming frames. `Usage.Add`
keeps the value while every contributing request that reported controls reported
the same ones; on a disagreement it clears the value and sets
`Usage.ControlsMixed`, which stays set, so a later agreeing request cannot make
one turn's controls stand for a whole run. Check `ControlsMixed` to tell a mixed
aggregate from one that simply has nothing to report.

## Native support versus portable emulation

Do not advertise `EmulatedEfforts` as native provider support. Emulation keeps a
portable Dive configuration useful across providers, but token-budget mappings
do not guarantee identical effort semantics.

Custom `llm.ReasoningEffort` strings are a permissive runtime compatibility
feature. They are intentionally absent from the published control set even when
a provider's runtime fallback forwards or maps them.

## Snapshot and comparison behavior

`ControlsFor`, `Preview`, and `Usage.Controls` all return defensive snapshots.
Callers may mutate slices and pointers without changing provider tables, request
behavior, later lookups, or another usage frame.

The public structs contain an unexported compatibility sentinel, so configure
`go-cmp` when comparing them outside Dive:

```go
diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(
    modelcaps.BudgetBounds{},
    modelcaps.ReasoningControls{},
    modelcaps.ModelControls{},
    modelcaps.ControlDecision{},
    llm.EffectiveControls{},
    modelcaps.Plan{},
))
```

A bare `cmp.Diff` will panic when it encounters those unexported fields.
`llm.EffectiveControls` also has an `Equal` method, which is usually what you
want for comparing a served control set against a previewed one.
