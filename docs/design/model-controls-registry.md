# Model Controls Registry

**Status:** Implemented
**Date:** 2026-08-22
**Audience:** Dive maintainers and integrators that publish model catalogs or
report served controls
**Target:** Next compatible minor release after v1.26.0

## Summary

Dive already knows which model controls each provider adapter can safely send:
native reasoning efforts, manual reasoning budgets, adaptive thinking,
temperature, and explicit thinking disablement. OpenAI and Grok expose part of
that data through `providers/modelcaps`; Anthropic and Google keep it private to
their provider packages.

Dive is also lenient about those controls: it clamps an unsupported effort,
turns an effort into a budget, represents a manual budget as adaptive thinking,
and drops temperature a request would reject — and the request succeeds. That
is deliberate, and it stays. What it lacks is honesty: today the only trace is a
`Logger.Warn`, so a consumer without a wired logger gets a silent downgrade.

This proposal adds three provider-neutral, read-only views of the same planner,
named for where they sit in time:

| Question                              | API                     |
| ------------------------------------- | ----------------------- |
| What does this exact model take?      | `modelcaps.ControlsFor` |
| What would Dive send for this config? | `modelcaps.Preview`     |
| What did Dive send?                   | `llm.Usage.Controls`    |

1. `ControlsFor` returns independent, static facts suitable for catalog
   display.
2. `Preview` performs a network-free dry run of a concrete `llm.Config` and
   reports which controls would be applied, adjusted, emulated, or omitted, and
   separately reports whether the request would be rejected.
3. `Usage.Controls` reports the controls a completed request actually carried,
   beside the existing `Usage.Speed` and `Usage.ServiceTier`. It is the same
   `llm.EffectiveControls` value `Plan.Effective` carries, so a consumer can
   compare requested against served without parsing a provider payload.

Each provider package remains authoritative and registers the first two
functions during `init()`. Request construction, `Preview`, and the response
projection share provider-private pure control planning helpers, so combination
rules and request-dependent constraints are not copied into a second matrix.
Provider request paths do not consult the global registry, and the requests
they build are unchanged.

The published boundary is deliberately stricter than runtime lookup. It accepts
only exact provider catalog IDs after documented provider-specific
normalization. Unknown future point releases, gateway IDs, deployments, and
custom models stay unpublished even when a permissive runtime prefix matcher
would pass their settings through or inherit a family rule.

Live evidence is also scoped to the endpoint and API surface that was probed. A
Vertex result never becomes an implicit guarantee for the public Gemini API,
Anthropic Messages, Bedrock, or another adapter surface.

## Problem

The current capability ownership is split:

| Provider  | Source of truth                       | Publicly queryable |
| --------- | ------------------------------------- | ------------------ |
| OpenAI    | `providers/modelcaps/tables.go`       | Yes, partially     |
| Grok      | `providers/modelcaps/tables.go`       | Yes, partially     |
| Anthropic | `providers/anthropic/capabilities.go` | No                 |
| Google    | `providers/google/capabilities.go`    | No                 |

Consumers that publish a strict contract need facts that differ from Dive's
runtime policy:

- Dive clamps an unsupported native effort to the nearest usable value.
- Dive may translate an effort into a token budget for a model without a native
  effort parameter.
- Dive omits controls that a model rejects or ignores.
- Unknown or custom models preserve the existing permissive pass-through
  behavior.
- Whether one control survives can depend on other request settings. Anthropic,
  for example, normally constrains a manual thinking budget by `max_tokens`,
  drops temperature while thinking is active, and rejects some forced tool
  choices.

A static catalog needs independent facts such as the native effort ladder and
fixed budget bounds. An admission layer needs the answer to a different
question: "What will Dive do with this exact configuration?" Trying to answer
both with pairwise booleans would duplicate provider request logic and grow
quadratically as controls are added.

There is a third gap, on the other side of the call. Every one of those
adjustments is invisible on the response. Dive already reports two applied
settings there — `Usage.Speed` ("which inference speed served the request") and
`Usage.ServiceTier` — but not reasoning or sampling controls. A consumer that
asked for `max` effort and was served `xhigh`, or asked for a 8,000-token cap
and got "the model decides", has no way to say so to its own users unless it
wired a logger and parses warning text. Leniency without a record is not
leniency; it is a silent downgrade.

## Goals

1. Publish per-model controls for provider adapters with maintained capability
   tables.
2. Keep provider packages authoritative; the registry stores resolvers, not a
   second copy of per-model facts.
3. Distinguish native effort support from effort-to-budget emulation.
4. Let a catalog consumer publish fixed budget bounds without misrepresenting
   request-dependent constraints as model constants.
5. Let an admission consumer determine how one concrete configuration will be
   treated without copying provider request rules.
6. Preserve endpoint-scoped verification evidence without presenting a probe on
   one API surface as proof for another.
7. Distinguish an unlinked provider package from a linked provider that does not
   publish controls for a model.
8. Define a conservative model-ID normalization contract that cannot silently
   publish a future point release through prefix inheritance.
9. Report the controls a completed request actually carried, on the response,
   without a logger.
10. Preserve existing public signatures and provider request behavior.
11. Return snapshots that callers can mutate without corrupting package-level
    tables, later plans, or other usage frames.

## Non-goals

1. A complete model catalog. Display names, descriptions, context windows,
   modalities, lifecycle, recommendations, and model availability remain owned
   by consumers or provider catalog packages.
2. Account entitlement or deployment discovery. A published control set does
   not prove that a particular API key can access the model.
3. Automatic live probing. Capability tables remain maintained and tested as
   they are today.
4. Strict validation inside normal Dive calls. Existing clamping, translation,
   omission, warnings, and unknown-model pass-through remain unchanged. Making
   those adjustments _observable_ on the response is a goal, not a form of
   validation: nothing new is refused, and a consumer that wants a request to
   fail instead of being adjusted builds that on `Preview`, before the call.
5. A separate capability version or content hash. Consumers can record the
   loaded Dive module versions through `runtime/debug.ReadBuildInfo` when they
   need to diff or roll back a published contract.
6. Native-provider inference for gateways such as OpenRouter, Amazon Bedrock,
   or Vertex deployments. Those endpoints may expose different controls and
   must register their own provider controls if needed.
7. A serialized provider wire request. `Preview` reports provider-neutral
   control outcomes, not SDK request structs or headers.
8. Moving provider request paths onto the global registry. The registry is an
   inspection facade, not a runtime dependency.

## Terminology and API boundary

The word this design already uses for effort, budget, thinking, and temperature
is **controls**, and the three APIs are named by where they sit in time
relative to the request:

| API                     | Type                    | Answers               |
| ----------------------- | ----------------------- | --------------------- |
| `modelcaps.ControlsFor` | `ModelControls`         | what this model takes |
| `modelcaps.Preview`     | `Plan`                  | what Dive would send  |
| `llm.Usage.Controls`    | `llm.EffectiveControls` | what Dive sent        |

The published type is named `ModelControls`, not `Capabilities`. Dive already
has two other meanings for that word:

- `modelcaps.Capabilities` is the existing OpenAI/Grok runtime table record.
- `modelcatalog.Model.Capabilities` is a list of broad modalities and features
  such as text, image, or audio support.

That constraint is real, but the escape is not "classification": a caller does
not want a model sorted into a bucket, they want to know what it takes.
`ControlsFor` mirrors `providers.PricingFor`, the existing per-model lookup
precedent. `Preview` returns a `Plan` — the familiar plan/apply split — and
reads as a sentence at the call site: `plan, ok := modelcaps.Preview(...)`.
`Plan` cannot be the function name because the type already owns it in the
package.

`ControlsFor` is the new exact, provider-registered inspection API.
Existing `Lookup`, `LookupEntry`, `TableFor`, `ResolveEffort`, and
`AcceptsTemperature` keep their current prefix-oriented runtime semantics. In
particular, `Lookup("anthropic", ...)` and `Lookup("google", ...)` do not start
working as a side effect of this proposal.

The distinction is intentional:

- **ModelControls** answers which independent controls Dive has verified for
  an exact catalog model.
- **Plan** answers how Dive would treat one concrete configuration.
- **EffectiveControls** answers what one request carried. It is `Plan.Effective`
  and `Usage.Controls` — the same type, produced by the same planner, before and
  after the call.
- **Runtime lookup** preserves Dive's existing compatibility behavior for
  unknown, custom, and gateway models.

`EffectiveControls` is defined in `llm` rather than `modelcaps` because
`llm.Usage` must carry it and `modelcaps` imports `llm`. `modelcaps` keeps the
name through a type alias, so `Plan.Effective` and `Usage.Controls` are
literally the same type and neither package owns a second definition.

## Architecture

### Providers register resolvers

Every participating provider package registers a controls function and a
preview function. This is consistent across OpenAI, Grok, Anthropic, and
Google; there are no implicit
OpenAI/Grok built-ins in the registry merely because their raw tables happen to
live in `modelcaps`.

OpenAI and Grok already import the root module's `providers/modelcaps` package,
so registration does not add a dependency cycle. Anthropic is part of the root
module and imports its sibling `providers/modelcaps` package. The separately
released Google module already imports packages from the root module and will
declare the matching minimum root version when this API is adopted.

The dependency and execution flow is:

```text
provider capability table ──► provider-private control planner ──► request builder
          │                              │
          │ exact catalog projection     │ dry-run projection
          ▼                              ▼
       Controls                       Preview
          └──────── provider init registration ────────┐
                                                       ▼
                                          modelcaps registry
                                                       │
                                                       ▼
                                      catalog or admission consumer
```

`modelcaps` never imports a provider package. A provider-private planner may
call existing shared `modelcaps` helpers, but a request builder never resolves
itself back through the registry.

### Published controls contain no combination policy

`ModelControls` contains normalized Dive contract facts that remain meaningful
without a complete request:

- native effort levels;
- which recognized effort values Dive can emulate using a budget;
- fixed effective budget bounds;
- whether adaptive thinking is expressible;
- whether thinking can be disabled; and
- whether temperature is a meaningful control in the absence of another
  control that suppresses it.

These are not raw probe columns. Provider projections may apply semantic guards,
such as reporting disablement only when the model can reason, but they do not
evaluate combinations from a concrete request.

It does not contain pairwise fields such as `EffortWithTemperature`,
`BudgetWithTemperature`, or `EffortBudgetCompatible`. Those fields would copy
branches already present in provider request construction and would expand to
an N-by-N matrix as controls such as tool choice, prefill, and interleaved
thinking become relevant.

`Budget` also does not contain `Dynamic` or `MustBeBelowMaxOutput`:

- Dynamic `-1` budgets are the Google wire representation of the existing
  provider-neutral `AdaptiveThinking` concept.
- The relationship between Anthropic's budget, `max_tokens`, the 1,024-token
  hard floor, and interleaved thinking is request-dependent and belongs in
  `Preview`.

Budget bounds are the effective fixed bounds for the named model. A bound may
be shared by an entire provider generation; its
presence in a per-model snapshot does not claim the source constant is
model-local.

### Preview shares request-planning predicates

Each provider extracts or reuses a pure internal control planner. Both the
actual request builder and the registered preview function call that helper. The
helper performs no network I/O and does not consult the registry.

`Preview` accepts `llm.Config` because request-dependent behavior already relies
on fields such as `Model`, `MaxTokens`, `Features`, `Prefill`, and `ToolChoice`.
A narrower public request type would duplicate that configuration and drift.
The preview function treats the input as immutable and returns only a
provider-neutral control plan.

The plan reports:

- whether effort, budget, thinking, and temperature were applied, emulated,
  omitted, defaulted, or not requested;
- whether a requested value was adjusted;
- the effective provider-neutral values after clamping or translation; and
- any control-related rejection the normal request builder would return, such
  as Anthropic thinking combined with forced tool choice.

Human-readable reason strings are diagnostic and not stable API identifiers.
Consumers branch on `Action`, `Adjusted`, and `Rejected`, then use effective
values for display or admission.

### Effective controls are reported on the response

The planner already computes `EffectiveControls` for every real request and, as
of this change, no longer discards it once the wire request is built. Each
provider carries the value out of request construction and attaches it to
`Response.Usage.Controls`.

This costs nothing extra to compute and cannot drift from the request, because
it is the same value the request was built from — not a re-derivation. It is
also the only projection available to a caller that did not inspect the config
in advance, which is most callers.

Streaming reports the same value. Providers stamp it onto the usage frames they
emit, so `llm.ResponseAccumulator` carries it onto the accumulated response
through the existing `Usage.Absorb` path; no new event type or accumulator
branch is introduced.

`Usage` is an aggregate as well as a per-response record, so the merge rules
differ by operation:

- `Absorb` (cumulative streaming frames) supersedes: a later frame's controls
  replace an earlier report, and a frame with none leaves the earlier report
  standing.
- `Add` (separate requests) keeps the value while every contributing request
  that reported controls reported the same ones. A disagreement clears it and
  latches `Usage.ControlsMixed`, which is sticky: a later agreeing request
  cannot resurrect one turn's controls as the answer for the whole run. This
  mirrors how `ServiceTier` collapses to `"mixed"` and stays there.

`Controls` needs a companion flag because its "mixed" state and its "nothing to
report" state are both nil, unlike `ServiceTier`, whose sentinel is a string it
can carry itself. `ControlsMixed` says which nil a caller is looking at.

Nil with `ControlsMixed` false means the provider reported no effective
controls — the four planner providers always do, and other adapters do not. A
zero-valued field inside a non-nil value means something different and more
useful: Dive sent nothing for that control. A `medium` effort served as a budget
on Sonnet 4.5 arrives as an empty `ReasoningEffort` with a populated
`ReasoningBudget`, which is exactly the difference a consumer needs to report.

The response projection is observation, not validation. Nothing new is refused,
and no request changes shape because of it.

### Model IDs are exact at the public boundary

Provider registration defines a small, documented normalizer, then requires an
exact canonical ID present in that provider's generated catalog. The accepted
normalizations for the first release are:

| Provider  | Accepted input in addition to exact catalog ID |
| --------- | ---------------------------------------------- |
| OpenAI    | one leading `openai/` qualifier                |
| Grok      | one leading `x-ai/` qualifier                  |
| Anthropic | one leading `anthropic/` qualifier             |
| Google    | one leading `models/` qualifier                |

Normalization trims surrounding whitespace and compares case-insensitively.
It does not strip arbitrary path segments. OpenRouter paths, Bedrock model or
inference-profile IDs, Vertex publisher/deployment paths, fine-tunes, and other
custom names return `ok=false` unless a separately registered provider owns
their contract.

Both `ModelControls.Model` and `Plan.Model` contain the normalized canonical
catalog ID, not the caller's spelling or qualified input. Consumers therefore
do not need to duplicate provider normalization to key a published catalog.

The public controls function must use an exact map from canonical catalog model ID to
the intended capability entry and verification scopes for that exact ID. The
map does not copy capability facts; it records which authoritative entry applies
and where that mapping was live-probed. It must not call a plain
prefix matcher and assume the match itself is evidence. This is particularly
important for Anthropic: the existing runtime deliberately documents that a
future `claude-opus-4-9` would inherit the older `claude-opus-4` entry. That
permissive runtime behavior remains, but the public controls function returns
`ok=false` until the new ID has an explicit mapping.

The provider-private result includes the canonical model ID and
matched capability-entry key so package tests can assert the exact mapping. The
registered public projection omits that key: it is a testable source pointer,
not a consumer contract, and private table refactors should not become public
API changes.

### Linked, published, and verification scope are separate states

`Providers()` returns the sorted canonical names registered in the current
binary. It lets a caller distinguish "the provider package is not linked" from
"the provider is linked but this model has no published controls."

`ControlsFor` returns `ok=true` for an exact model mapping even when that
mapping has no live verification scope. `VerificationScopes` names the API
surfaces against which the exact control set was successfully probed. An
empty list means the facts are historical, documented, inferred, or otherwise
not live-probed.

Verification on one scope is not evidence for another. For example,
`gemini-3.7-flash` currently carries `google:vertex-ai` but not
`google:gemini-api`, because its documented probe was performed against Vertex
AI. Anthropic's direct Messages API, Bedrock, and Vertex are likewise distinct
surfaces. A strict consumer checks the scope it will actually call.

A scope attests to the entire populated control snapshot on that exact
model and surface: affirmative support, negative support, effort ladders,
bounds, and semantic omission rules. Verifying only the model-to-entry mapping
or a subset of fields is insufficient. When evidence is partial, the provider
omits the scope rather than implying per-field proof that this first API cannot
represent.

Verification scope is not a freshness, entitlement, regional availability, or
served-model guarantee. Consumers that need rollback provenance should record
the loaded module versions from `runtime/debug.ReadBuildInfo` with the published
contract.

## Public API

Add `providers/modelcaps/registry.go`:

```go
package modelcaps

import (
	"errors"

	"github.com/deepnoodle-ai/dive/llm"
)

// BudgetBounds describes fixed manual reasoning-budget
// bounds. Nil means Dive has no fixed bound to publish.
type BudgetBounds struct {
	noUnkeyedLiterals struct{}

	Minimum *int
	Maximum *int
}

// ReasoningControls describes independent reasoning controls Dive can
// express for an exact catalog model.
type ReasoningControls struct {
	noUnkeyedLiterals struct{}

	// NativeEfforts lists provider-native effort or thinking-level values from
	// least to most eager. Budget-only emulation is deliberately excluded.
	NativeEfforts []llm.ReasoningEffort

	// EmulatedEfforts lists recognized effort values Dive can translate into a
	// manual budget when no native effort parameter exists. It excludes "none",
	// which is represented by CanDisableThinking, and arbitrary custom strings.
	EmulatedEfforts []llm.ReasoningEffort

	// Budget is non-nil when Dive can send a manual reasoning budget.
	Budget *BudgetBounds

	AdaptiveThinking bool

	// CanDisableThinking is false when the model has no reasoning support.
	CanDisableThinking bool
}

// ModelControls describes independent model controls Dive can publish
// without applying product policy or evaluating a complete request.
type ModelControls struct {
	noUnkeyedLiterals struct{}

	// Model is the normalized, exact canonical provider catalog ID.
	Model string

	// Temperature reports whether Dive forwards temperature as a meaningful
	// control when no other requested control suppresses it.
	Temperature bool

	Reasoning ReasoningControls

	// VerificationScopes identifies the endpoint and API surfaces on which this
	// entire exact control set was successfully live-probed. An empty slice
	// makes no live-verification claim.
	VerificationScopes []VerificationScope
}

// VerificationScope identifies a provider endpoint and API surface. It is an
// extensible string so third-party providers can define their own scopes.
type VerificationScope string

const (
	VerificationOpenAIResponses   VerificationScope = "openai:responses-api"
	VerificationXAIResponses      VerificationScope = "xai:responses-api"
	VerificationAnthropicMessages VerificationScope = "anthropic:messages-api"
	VerificationGoogleGeminiAPI   VerificationScope = "google:gemini-api"
	VerificationGoogleVertexAI    VerificationScope = "google:vertex-ai"
)

type ControlAction string

const (
	ControlUnspecified  ControlAction = ""
	ControlNotRequested ControlAction = "not_requested"
	ControlApplied      ControlAction = "applied"
	ControlEmulated     ControlAction = "emulated"
	ControlOmitted      ControlAction = "omitted"
	ControlDefaulted    ControlAction = "defaulted"
)

// ControlDecision explains one logical input control. Adjusted is true when
// the effective value differs from the requested value.
type ControlDecision struct {
	noUnkeyedLiterals struct{}

	Action   ControlAction
	Adjusted bool
	Reason   string
}

// EffectiveControls is defined in llm so llm.Usage can carry it; modelcaps
// keeps the name through an alias so Plan.Effective and Usage.Controls are the
// same type.
type EffectiveControls = llm.EffectiveControls

// Plan explains how Dive would treat the model controls in a concrete config.
// When Rejected is true, the normal provider request builder would return an
// error for a control-related interaction and no request should be sent.
type Plan struct {
	noUnkeyedLiterals struct{}

	// Model is the normalized, exact canonical provider catalog ID.
	Model string

	Effort      ControlDecision
	Budget      ControlDecision
	Thinking    ControlDecision
	Temperature ControlDecision
	Effective   EffectiveControls

	Rejected        bool
	RejectionReason string
}

type ControlsFunc func(model string) (ModelControls, bool)
type PreviewFunc func(config llm.Config) (Plan, bool)

// Resolver projects one provider's authoritative model controls and pure
// request-control planner.
type Resolver struct {
	noUnkeyedLiterals struct{}

	Controls ControlsFunc
	Preview  PreviewFunc
}

var (
	ErrInvalidProvider    = errors.New("modelcaps: invalid provider")
	ErrInvalidResolver    = errors.New("modelcaps: invalid resolver")
	ErrProviderRegistered = errors.New("modelcaps: provider already registered")
)

// Register associates a canonical provider name with a resolver. Invalid and
// duplicate registrations return a sentinel error and do not replace the
// existing resolver.
func Register(provider string, resolver Resolver) error

// MustRegister calls Register and panics on error. Dive provider init functions
// use this form because a duplicate canonical owner is a programming error.
func MustRegister(provider string, resolver Resolver)

// Providers returns a sorted snapshot of canonical providers registered in the
// current binary.
func Providers() []string

// ControlsFor returns static facts for an exact catalog model. ok is
// false when the provider is not registered or the normalized model is not an
// exact mapped catalog ID. The returned Model is that canonical ID.
func ControlsFor(provider, model string) (ModelControls, bool)

// Preview performs a network-free dry run for config.Model. ok has the same
// exact-model meaning as ControlsFor, and Plan.Model is canonical.
func Preview(provider string, config llm.Config) (Plan, bool)

// SupportsNativeEffort reports native provider support. Budget emulation does
// not count.
func (c ModelControls) SupportsNativeEffort(
	effort llm.ReasoningEffort,
) bool

// VerifiedOn reports whether the exact model control set was live-probed on
// the requested endpoint and API surface.
func (c ModelControls) VerifiedOn(scope VerificationScope) bool
```

Add `llm/controls.go` and one field to `llm.Usage`:

```go
package llm

// EffectiveControls is the provider-neutral set of reasoning and sampling
// controls Dive resolved for one request. It is not a serialized provider
// request. A zero-valued field means Dive sent nothing for that control.
type EffectiveControls struct {
	noUnkeyedLiterals struct{}

	ReasoningEffort ReasoningEffort `json:"reasoning_effort,omitempty"`
	ReasoningBudget *int            `json:"reasoning_budget,omitempty"`
	Thinking        ThinkingType    `json:"thinking,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
}

// Clone returns a deep copy so callers cannot mutate shared pointer fields.
func (c EffectiveControls) Clone() EffectiveControls

// Equal compares by value, treating nil and a pointer to an equal value as
// different: nil means the control was not sent at all.
func (c EffectiveControls) Equal(other EffectiveControls) bool

type Usage struct {
	// ... existing fields ...

	// Controls records the reasoning and sampling controls Dive actually sent.
	// Nil means the provider reports no effective controls for this request.
	Controls *EffectiveControls `json:"controls,omitempty"`

	// ControlsMixed distinguishes an aggregate whose requests were served with
	// different controls from one that has no controls to report.
	ControlsMixed bool `json:"controls_mixed,omitempty"`
}
```

`Equal` exists because `EffectiveControls` contains pointers, so `==` is not
usable and a plain `cmp.Diff` panics on the unexported sentinel. It is the
comparison a consumer wants when checking a served control set against a
previewed one.

### Plan invariants

Each decision belongs to the logical caller input named by its field; it does
not move to the destination field when Dive translates that input. The action
contract is:

| Action                | Meaning                                                                    |
| --------------------- | -------------------------------------------------------------------------- |
| `ControlUnspecified`  | invalid for a successful plan; the zero value is diagnostic                |
| `ControlNotRequested` | no caller input; another decision may populate the related effective field |
| `ControlApplied`      | caller input is honored through its native semantic control                |
| `ControlEmulated`     | caller input is realized through a different effective control             |
| `ControlOmitted`      | caller input has no effect on the final request                            |
| `ControlDefaulted`    | no caller input; a default supplies the related effective field            |

`Adjusted` may accompany `ControlApplied` or `ControlEmulated` when a comparable
requested magnitude was clamped or normalized. It is false for the other
actions. `Effective` contains the final provider-neutral state, regardless of
which input produced it.

Examples:

| Input and outcome                               | Owning decision                                 | Effective state                    |
| ----------------------------------------------- | ----------------------------------------------- | ---------------------------------- |
| effort translated into a budget                 | `Effort=ControlEmulated`                        | `ReasoningBudget` is populated     |
| explicit budget translated to adaptive thinking | `Budget=ControlEmulated`                        | `Thinking=adaptive`                |
| explicit budget wins over simultaneous effort   | `Effort=ControlOmitted`, budget owns its result | final budget/thinking is populated |
| provider activates thinking with no input       | `Thinking=ControlDefaulted`                     | effective thinking is populated    |

When `Rejected=true`, `RejectionReason` is non-empty and all decisions plus
`Effective` are unspecified diagnostics that callers must ignore. A successful
plan has `Rejected=false`, an empty rejection reason, and no
`ControlUnspecified` decisions.

The unexported sentinel fields require external packages to use keyed composite
literals, preserving source compatibility when fields are added. They also
mean a bare `cmp.Diff` on these structs will panic unless the test configures
`cmpopts.IgnoreUnexported` (or an exporter). The implementation guide and Dive
tests must show the correct comparison pattern; Dive's wonton assertion helper
already supplies an exporter.

### Registry behavior

The registry uses a `sync.RWMutex` and a map keyed by normalized provider name.

- `Register` trims whitespace, lowercases the provider name, validates both
  resolver functions, and returns an error for invalid or duplicate ownership.
- `MustRegister` is reserved for provider `init()` functions. Consumer code can
  use `Register` without turning a recoverable conflict into an initialization
  panic.
- Duplicate registration never silently overwrites a provider. Last-write-wins
  behavior would make facts depend on import and initialization order. A
  consumer that owns a different contract registers a different canonical
  provider name and maps product aliases at its boundary.
- Lookup copies the resolver while holding the read lock, releases the lock,
  and only then invokes it. A resolver therefore cannot deadlock the registry.
- Returned values are deep-enough snapshots for public mutation. The registry
  clones `NativeEfforts`, `EmulatedEfforts`, `VerificationScopes`, optional
  budget records and bounds, effective value pointers, and reason-bearing plan
  values as necessary.
- `Providers` returns a newly allocated, sorted slice.
- Resolver registration normally happens during package initialization, but
  the mutex keeps concurrent lookups and plugin-style imports race-safe.

Registration is process-lifetime state; there is no public `Unregister`. Tests
outside `modelcaps` use unique fake provider names. Registry package tests use
an unexported cleanup helper that restores the prior map entry with `t.Cleanup`,
so repeated and parallel tests do not leak registrations into one another.

The executable must import a provider package for its `init()` to run. The
registry removes provider-specific dispatch from lookup sites; it does not make
unlinked Go packages execute automatically.

## Provider projections

### OpenAI and Grok

Add registration files to the OpenAI and Grok provider modules. Their
controls functions use the generated provider catalog for exact ID membership,
then project the existing raw `modelcaps.Entry` so explicitly unverified
entries remain distinguishable.

For these providers:

- `NativeEfforts` comes from `Entry.Caps.Efforts`.
- `EmulatedEfforts` is empty.
- `Temperature` comes from `Entry.Caps.Temperature`.
- Reasoning budgets, adaptive thinking, and explicit disablement are absent.
- `VerificationScopes` comes from the exact model mapping and is empty when
  `Entry.Unverified` is true.

The controls function uses `LookupEntry`, not `Lookup`, because `Lookup` intentionally
hides unverified entries for permissive runtime behavior. Prefix lookup may be
used to locate the underlying record only after exact catalog membership and an
explicit model-to-entry mapping have been established.

Both providers share one planner in `providers/internal/responsescontrol`,
because Grok's adapter embeds the OpenAI Responses provider. It reuses
`ResolveEffort` and the same temperature predicate called by request
construction, and reports native clamping as `ControlApplied` with
`Adjusted=true`, unsupported omission as `ControlOmitted`, and no budget or
adaptive emulation.

`buildRequestParams` returns the planner's effective controls alongside the SDK
params. `Generate` attaches them to the decoded response; `Stream` hands them to
the stream iterator, which stamps them onto the `response.created` message and
onto the usage carried by the terminal `response.completed` and
`response.incomplete` frames.

### Anthropic

Add `providers/anthropic/capabilities_register.go`. The static projection is:

| Public field         | Anthropic source                                |
| -------------------- | ----------------------------------------------- |
| `NativeEfforts`      | `caps.efforts`                                  |
| `EmulatedEfforts`    | recognized legacy budget mappings               |
| `Budget`             | present when `caps.manualBudget`                |
| Budget minimum       | `minThinkingBudget`                             |
| Budget maximum       | no fixed maximum                                |
| `AdaptiveThinking`   | `caps.adaptive`                                 |
| `CanDisableThinking` | guarded semantic disablement                    |
| `Temperature`        | `caps.temperature`                              |
| `VerificationScopes` | exact mapping scopes and private entry evidence |

For legacy budget models, `EmulatedEfforts` lists `minimal`, `low`, `medium`,
`high`, `xhigh`, and `max`, matching `legacyEffortBudget`. It excludes `none`
and unrecognized custom strings because those do not produce an emulated budget.

`CanDisableThinking` is semantic and only meaningful for a model that can
reason. The projection first requires at least one of native effort, manual
budget, adaptive thinking, or thinking-on-by-default; only then does it report
whether Dive can make thinking inactive. This guard keeps a non-reasoning model
such as `claude-3-5-haiku` from vacuously reporting disablement. The wire
representation may be an explicit `thinking:{type:"disabled"}` object or
omission. Private fields such as `thinkingOnByDefault`, `explicitDisable`, and
`disabledEffortCap` continue to drive planning and are not exposed directly.

The private entry evidence marker is named `notLiveProbed`, not `unverified`,
because Google's existing `unverified` field changes runtime lookup semantics.
It suppresses the exact mapping's verification scopes. The Anthropic marker
affects only public evidence and must not accidentally turn a known runtime
entry into passthrough.

Anthropic's pure planner must cover the current request-dependent branches,
including:

- manual-budget clamping below `max_tokens` when interleaved thinking is off;
- omitting thinking when `max_tokens - 1` leaves less than the 1,024-token hard
  floor;
- allowing a larger budget when interleaved thinking is enabled;
- translating legacy effort to a budget;
- translating unsupported manual budgets to adaptive thinking on newer models;
- effort caps while thinking is disabled;
- temperature omission while thinking is active; and
- control-related prefill and forced-tool-choice rejections.

`applyRequestConfig` returns the planner's effective controls with the filled
request. `Generate` records them in `finalizeUsage`, beside cache-thrash
logging and cost estimation. The stream iterator stamps them onto every
usage-bearing frame: `message_start` seeds the accumulated response wholesale,
and the cumulative `message_delta` usage supersedes it.

### Google

Add `providers/google/capabilities_register.go`. The controls function projects
the raw `lookupEntry`, not `lookupCapabilities`, because the latter converts
not-live-probed entries into unknown runtime passthrough.

| Public field         | Google source                                    |
| -------------------- | ------------------------------------------------ |
| `NativeEfforts`      | `caps.efforts`                                   |
| `EmulatedEfforts`    | known effort ladder on budget-only models        |
| `Budget`             | published `caps.minBudget` and `caps.maxBudget`  |
| `AdaptiveThinking`   | model accepts Dive's adaptive-thinking request   |
| `CanDisableThinking` | `caps.canDisableThinking`                        |
| `Temperature`        | `modelAcceptsTemperature(model)`                 |
| `VerificationScopes` | exact mapping scopes, empty if `caps.unverified` |

For budget-only Gemini models, `EmulatedEfforts` publishes the same known
non-`none` ladder. The current runtime fallback for an arbitrary custom effort
string remains permissive behavior and is not advertised as a strict fact.

There is no separate `Dynamic` field. Google's `-1` thinking budget is the wire
encoding used when the provider-private planner applies
`llm.ThinkingTypeAdaptive`.

The temperature projection uses `modelAcceptsTemperature`, not raw status-code
evidence. Some Gemini generations accept and range-check temperature but do not
honor it; Dive deliberately omits it for those models.

The exact `gemini-3.7-flash` mapping includes only
`VerificationGoogleVertexAI`. Its table comment says it was verified on Vertex
AI and was not probed against the public Gemini API. Other Gemini mappings list
only scopes supported by their own probe evidence.

The Google preview function and request builder both use the existing
thinking-plan logic for effort-to-budget emulation, effort and budget
precedence, adaptive thinking, bounds clamping, disablement, and effective
temperature omission.

`applyRequestConfig` returns the planner's effective controls with the filled
request. `Generate` attaches them to the converted response before cost
population; the stream iterator stamps them onto the `message_start` message and
onto the final `message_delta` usage. The exported
`NewStreamIteratorFromSeq` keeps its signature and reports no controls; only the
package-internal constructor takes them.

## Consumer contract

A strict catalog checks provider linkage, exact published controls, and
evidence:

```go
if !slices.Contains(modelcaps.Providers(), "google") {
	return errors.New("google provider package is not linked")
}

controls, ok := modelcaps.ControlsFor("google", "gemini-3.7-flash")
if !ok {
	return errors.New("model has no published controls")
}
if !controls.VerifiedOn(modelcaps.VerificationGoogleVertexAI) {
	return errors.New("controls were not verified on Vertex AI")
}

if !controls.SupportsNativeEffort(llm.ReasoningEffortHigh) {
	return errors.New("high effort is not natively supported")
}
```

An admission layer evaluates the whole configuration instead of combining
static booleans:

```go
cfg := llm.Config{
	Model:           "claude-opus-4-6",
	MaxTokens:       pointer.To(4096),
	Temperature:     pointer.To(0.7),
	ReasoningBudget: pointer.To(8192),
}

plan, ok := modelcaps.Preview("anthropic", cfg)
if !ok {
	return errors.New("model has no published controls")
}
if plan.Rejected {
	return errors.New(plan.RejectionReason)
}
if plan.Budget.Adjusted || plan.Temperature.Action == modelcaps.ControlOmitted {
	return errors.New("Dive would change the requested control semantics")
}
```

The example's `pointer.To` is illustrative; this proposal does not add that
helper.

A consumer that stays lenient reports the difference instead of refusing it,
reading the served controls off the response it already has:

```go
response, err := provider.Generate(ctx, opts...)
if err != nil {
	return err
}

served := response.Usage.Controls
if served == nil {
	return nil // provider does not report effective controls
}
if served.ReasoningEffort != requestedEffort {
	record("effort", requestedEffort, served.ReasoningEffort,
		"budget", served.ReasoningBudget, "thinking", served.Thinking)
}
if requestedTemperature != nil && served.Temperature == nil {
	record("temperature", requestedTemperature, nil)
}
```

The two modes are one policy with two actions. A strict consumer runs the same
comparison against `Preview` before the call and returns an error; a lenient one
runs it against `Usage.Controls` after the call and records the difference.
Neither reimplements Dive's translation rules, because both read the same
planner.

A consumer may deliberately expose Dive's portable effort emulation, but it
must label that separately from native effort support. Emulation does not imply
that every provider preserves identical effort semantics. It advertises only
values present in `EmulatedEfforts`; arbitrary `llm.ReasoningEffort` strings
remain a permissive runtime compatibility feature, not a strict catalog fact.

Provider naming remains consumer-specific. Dive's canonical xAI adapter name
is `grok`; a consumer that publishes `xai` maps that name at its adapter
boundary. The registry does not register product aliases.

## Backwards compatibility

This design is additive at the public boundary:

- `llm.Usage` gains an optional pointer field and an `omitempty` bool, so
  serialized usage for a provider that reports nothing is byte-identical to
  today.
  `Usage.Copy`, `Absorb`, and `Add` gain matching handling; no existing field's
  merge behavior changes.
- Provider-internal request builders change signature to return their effective
  controls (`applyRequestConfig`, `buildRequestParams`). These are unexported;
  the exported provider surface is unchanged.
- Existing `modelcaps` functions retain their signatures and behavior.
- `TableFor` still returns no Anthropic or Google table.
- Provider request paths keep direct access to provider-private planners and
  tables; they do not depend on global registration.
- Unknown models preserve permissive runtime behavior even though the new
  public APIs publish only exact catalog IDs.
- New structs prevent external unkeyed construction, allowing additive fields
  without breaking keyed literals.
- Public values are snapshots; mutation cannot affect future lookups or
  requests.

The implementation does add provider `init()` side effects. Dive's provider
packages use `MustRegister`, so a duplicate canonical owner fails loudly as a
programming error. Ordinary consumers and tests can use `Register` and handle
`ErrProviderRegistered` without a panic.

The Google, OpenAI, and Grok provider modules are released separately from the
root module. Their `go.mod` files must require the first root version containing
the registry API, and the release must test supported module combinations. An
older nested provider simply will not register; `Providers()` makes that
visible. A newer nested provider cannot compile against a root module lacking
the required API, which is safer than silent schema skew.

A strict catalog publisher asserts its expected provider set at startup and
returns a visible health/configuration error when one is absent. It must not
silently treat a missing resolver as an empty provider catalog.

## Package layout

```text
llm/
├── controls.go                   # EffectiveControls, shared by Plan and Usage
├── controls_test.go
└── usage.go                      # existing; gains Usage.Controls
providers/
├── modelcaps/
│   ├── modelcaps.go              # existing compatibility API
│   ├── tables.go                 # existing OpenAI and Grok facts
│   ├── registry.go               # new public types and registry
│   └── registry_test.go
├── internal/
│   └── responsescontrol/
│       └── plan.go              # shared OpenAI/Grok Responses control planner
├── anthropic/
│   ├── capabilities.go           # private source of truth and planner inputs
│   ├── capabilities_register.go  # exact model controls and preview
│   └── capabilities_test.go      # existing; extend coverage
├── google/
│   ├── capabilities.go
│   ├── capabilities_register.go
│   └── capabilities_test.go      # existing; extend coverage
├── openai/
│   ├── capabilities_register.go
│   └── capabilities_test.go      # existing; extend coverage
└── grok/
    ├── capabilities_register.go
    └── capabilities_test.go      # existing; extend coverage
```

No new top-level package or interface hierarchy is needed.

## Validation plan

### Registry tests

Add root-module tests covering:

1. canonical and case-insensitive provider lookup;
2. deterministic, defensive `Providers()` snapshots;
3. unknown provider versus linked-provider/unknown-model behavior;
4. mapped-but-not-live-probed entries returning `ok=true` with no
   verification scopes;
5. invalid and duplicate `Register` errors plus `MustRegister` panics;
6. concurrent registration and lookup safety;
7. defensive cloning of native/emulated effort and verification-scope slices,
   budget records, bound pointers, and plan effective-value pointers;
8. resolvers being invoked outside the registry lock; and
9. unchanged behavior of existing `Lookup`, `ResolveEffort`, and
   `AcceptsTemperature` APIs.

Comparison tests that use `go-cmp` configure `cmpopts.IgnoreUnexported` for all
new sentinel-bearing structs.

### Usage projection tests

Add `llm` tests covering `EffectiveControls.Clone` deep-copying its pointer
fields, `Equal` distinguishing an unsent control from a sent zero,
`Usage.Copy` isolating the copy and carrying `ControlsMixed`, `Absorb`
superseding while a control-free frame leaves the earlier report standing, `Add`
keeping agreeing controls and latching `ControlsMixed` on a disagreement so a
later agreeing request cannot resurrect one turn's value, and `Usage` JSON
round-tripping with the field absent when nil.

### Provider control-publication tests

Each provider module verifies:

1. every catalogued text model with an ID has one exact controls mapping;
2. the mapping names the intended capability entry key and exact-ID verification
   scopes, not merely any matching prefix;
3. qualified and case-varied input returns the exact canonical catalog ID in
   both control and plan results;
4. representative native effort ladders and budget bounds match the
   authoritative table;
5. effort-to-budget emulation lists exactly the recognized translated ladder
   and remains distinct from native effort;
6. temperature, adaptive thinking, and disablement match provider behavior;
7. endpoint scopes attest to the complete populated snapshot and reflect exact
   probe evidence, including Vertex-only
   `gemini-3.7-flash`, while not-live-probed entries have no scopes;
8. invented future point releases such as `claude-opus-4-9`, `gpt-5.7`, and
   `gemini-3.8-flash` return `ok=false`;
9. arbitrary gateway and deployment paths return `ok=false`; and
10. mutating a public result cannot change a later lookup or provider request.

### Planner parity tests

Provider tests use the same table of concrete `llm.Config` cases to assert both
the dry-run plan and the constructed provider request. Representative cases
include native clamping, budget emulation, simultaneous controls, temperature
omission, adaptive thinking, disablement, defaults, request-dependent budget
floors, interleaved-thinking exceptions, and hard rejections.

The table also enforces the plan invariants: cross-control translations remain
owned by their source decision, successful plans contain no unspecified actions,
and rejected plans provide a non-empty reason without consumable effective
values.

The parity test must inspect the constructed request rather than reimplementing
the expected branch in test code. This is the primary defense against plan and
runtime drift.

The same table asserts the third projection: the effective controls returned by
request construction must `Equal` the previewed `Plan.Effective`. That single
assertion is what keeps "what Dive would send" and "what Dive sent" from
drifting apart.

Each provider additionally covers the response path end to end — a `Generate`
against an httptest server and a stream driven through
`llm.ResponseAccumulator` — asserting that `Usage.Controls` arrives populated,
reflects the clamp or translation, and hands each frame an isolated copy.

### Commands

Run module suites independently because Google, OpenAI, and Grok are separate
Go modules:

```sh
go test ./llm ./providers/modelcaps ./providers/anthropic ./providers
(cd providers/google && go test ./...)
(cd providers/openai && go test ./...)
(cd providers/grok && go test ./...)
make provider-catalog-check
make fmt-md-check
make check
git diff --check
```

The normal GitHub workflow remains the final cross-module check.

## Implementation and release sequence

1. Add the public types, error-returning registry, `MustRegister`, defensive
   cloning, `Providers`, and registry unit tests to `providers/modelcaps`.
2. Add provider-private exact controls functions and
   model-to-entry-and-scope maps for OpenAI, Grok, Anthropic, and Google, without
   registering incomplete resolvers. Include Anthropic's private
   `notLiveProbed` entry marker and Google's effective temperature rule.
3. Extract or consolidate one pure control planner per provider and switch each
   request builder to it without changing request behavior.
4. Add each provider's preview projection, then register its complete
   `Resolver` only after both `Controls` and `Preview` are real implementations.
5. Add `llm.EffectiveControls` and `Usage.Controls`, and carry each provider's
   planner result onto the response and its stream frames.
6. Add exact-ID coverage, dry-run/request parity, response-projection, registry
   integration, and mutation-isolation tests.
7. Create `docs/guides/model-controls.md` with provider import
   requirements, model-ID and verification-scope rules, `go-cmp` guidance, and
   both strict-consumer and served-controls examples.
8. Release the root and nested provider modules at the same Dive version and
   validate their declared root-module requirements.
9. Migrate consumers separately. A consumer deletes copied facts only after its
   catalog and admission tests use both published controls and planning
   successfully.

Steps 1 and 2 form a reviewable publication foundation and compile without
placeholder registrations. Steps 3 through 7 complete the pure-planner,
`Preview`, and response-projection phases. They may land as separate commits,
but steps 1 through 7 release together so every registered resolver is complete
and all three projections come from one planner. Consumer adoption does not
block the Dive release.

The three views are not equally urgent. `Usage.Controls` is what a default,
lenient consumer depends on and is the smallest piece, because the planner
already computes the value. `ControlsFor` is needed regardless, so consumers
stop hand-copying provider tables. `Preview` is the enabler for an opt-in strict
mode: strict cannot be built on `Usage`, because by then the provider call has
cost money. All three ship together here since the rename is cheapest before a
tagged release.

## Alternatives considered

### Publish only static model controls

Rejected. Static facts are useful for catalog display but cannot answer how
temperature, budget, effort, thinking, tool choice, and token limits interact
in a concrete request. Pairwise booleans would become a duplicated rules engine.

### Publish only `Preview`

Rejected. A catalog still needs an enumerable native effort ladder, fixed
budget bounds, and evidence status without manufacturing many candidate
requests. And a lenient consumer needs the served value on a response it did
not preview. The three views serve different consumers.

### Report the clamp only through the logger

Rejected — this is the status quo. A `Logger.Warn` reaches an operator reading
logs, not the program holding the response, and a consumer that has not wired a
logger sees nothing at all. Warning text is also not a stable contract: a
consumer that scraped it would break on any rewording. `Usage.Controls` gives
the same information a typed value, at no extra computation, on the object the
caller already has. The warnings stay; they are for humans.

### Put the served controls on `Response` rather than `Usage`

Rejected. `Usage` already carries "what actually served this request" —
`Speed` documents "which inference speed served the request", and `ServiceTier`
sits beside it. Effective controls are the same kind of fact, and putting them
there means they inherit the existing `Copy`/`Absorb`/`Add` plumbing and the
streaming accumulation path instead of needing their own.

### Flatten the served controls into `Usage` fields

Rejected, though it is close. Flat fields would match the `Speed` and
`ServiceTier` precedent, but they would be a fourth spelling of a value the
planner already produces as a struct, and `Plan.Effective` could no longer be
compared to the served value with one call. One nested `EffectiveControls`
keeps preview and observation literally the same type.

### Add pairwise compatibility booleans

Rejected. Fields such as `EffortWithTemperature` duplicate request predicates,
cannot represent three-way interactions, and grow quadratically. `Preview`
derives combinations from the same planner as request construction.

### Represent Google's dynamic budget separately

Rejected. `thinkingBudget: -1` is a provider wire encoding of Dive's existing
`AdaptiveThinking` concept, not a second provider-neutral capability.

### Publish one `LiveVerified` boolean

Rejected. Verification belongs to an endpoint and API surface. A single bit
would incorrectly let the Vertex-only `gemini-3.7-flash` probe stand in for the
public Gemini API and would create the same ambiguity for Anthropic Messages,
Bedrock, and Vertex. Exact mappings publish scoped evidence instead.

### Register OpenAI and Grok implicitly from `modelcaps`

Rejected. It would make those providers appear linked while Anthropic and
Google depend on provider imports, and the root package does not own the nested
providers' exact generated catalogs. Consistent provider-package registration
makes linkage and exact-ID ownership explicit.

### Silently replace duplicate registrations

Rejected. Last-write-wins makes canonical facts depend on import order and lets
an accidental consumer override replace the provider's authoritative resolver.
`Register` returns an error; provider packages opt into `MustRegister`.

### Put the registry in `providers`

Rejected. Pricing uses the parent package, but model-control ownership already
has a focused public package. A second type in `providers` would split related
maintenance and worsen the existing terminology overlap.

### Export each provider's private table

Rejected. It would couple consumers to provider-specific implementation types,
expose runtime-only fields, and make private table refactors public compatibility
events.

### Add Anthropic and Google to `TableFor`

Rejected. `TableFor` participates in existing prefix lookup and clamping.
Teaching it new providers would change requests that currently pass through
untouched. The exact registry is intentionally separate.

### Make provider request paths consult the registry

Rejected. Request builders already have direct access to their authoritative
tables and planners. Global registration would add missing-import and
initialization-order failure modes to runtime without removing data duplication.

## Tradeoffs and consequences

- The public representation is broader than the existing OpenAI/Grok
  `Capabilities` type, but naming it `ModelControls` makes the semantic
  difference explicit.
- `EffectiveControls` lives in `llm` because `llm.Usage` carries it and
  `modelcaps` imports `llm`. The alias keeps one name for one type, but a reader
  of `modelcaps` has to follow it to find the definition.
- `Usage.Add` clears `Controls` and latches `ControlsMixed` when contributing
  requests disagree, so an aggregate over a mixed run reports nothing rather
  than a wrong answer. A consumer that wants per-turn controls must read them
  per turn; this is the same limitation `Speed` and `ServiceTier` already have,
  and it costs one extra bool to keep "mixed" distinguishable from "unknown".
- Only the four planner providers populate `Usage.Controls`. Adapters such as
  `openaicompletions` report nil, so a consumer must treat nil as "unknown",
  not as "nothing was sent".
- Exact model publication is conservative. A new catalog ID requires an
  explicit mapping before a strict consumer can advertise it, even when runtime
  compatibility lookup would inherit a family rule.
- Provider linkage depends on Go imports. `Providers()` turns that property into
  inspectable state instead of leaving `ok=false` ambiguous.
- Sharing pure planners requires a careful provider-internal refactor, but it
  eliminates a permanent second matrix of combination rules.
- Verification scopes prevent cross-endpoint overclaiming but remain modest
  evidence. Contractual catalogs still need account-specific and freshness
  qualification.
- Fixed budget bounds appear in per-model snapshots even when backed by a
  provider-wide constant. Documentation identifies them as effective bounds,
  while `Preview` owns request-dependent floors and ceilings.
- `MustRegister` can panic during provider initialization, but only for invalid
  code or duplicate canonical ownership. The underlying `Register` API remains
  recoverable and no registration is silently overwritten.
- The sentinel fields preserve additive compatibility but require explicit
  `go-cmp` configuration in external tests.

## Future extensions

Gateway registration and additional control families remain future proposals
because they require their own verified source of truth and consumer need.
