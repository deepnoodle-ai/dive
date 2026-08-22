# Model Control Classification and Planning Registry

**Status:** Proposed
**Date:** 2026-08-22
**Audience:** Dive maintainers and integrators that publish model catalogs
**Target:** Next compatible minor release after v1.26.0

## Summary

Dive already knows which model controls each provider adapter can safely send:
native reasoning efforts, manual reasoning budgets, adaptive thinking,
temperature, and explicit thinking disablement. OpenAI and Grok expose part of
that data through `providers/modelcaps`; Anthropic and Google keep it private to
their provider packages.

This proposal adds two provider-neutral, read-only views in
`providers/modelcaps`:

1. `ClassificationFor` returns independent, static facts suitable for catalog
   display.
2. `Explain` performs a network-free dry run of a concrete `llm.Config` and
   reports which controls would be applied, adjusted, emulated, omitted, or
   rejected.

Each provider package remains authoritative and registers both functions during
`init()`. Request construction and `Explain` share provider-private pure control
planning helpers, so combination rules and request-dependent constraints are
not copied into a second matrix. Provider request paths do not consult the
global registry, and their externally observable behavior remains unchanged.

Public classification is deliberately stricter than runtime lookup. It accepts
only exact provider catalog IDs after documented provider-specific
normalization. Unknown future point releases, gateway IDs, deployments, and
custom models remain unclassified even when a permissive runtime prefix matcher
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

## Goals

1. Publish model-control classifications for provider adapters with maintained
   capability tables.
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
   classify a model.
8. Define a conservative model-ID normalization contract that cannot silently
   classify a future point release through prefix inheritance.
9. Preserve existing public signatures and provider request behavior.
10. Return snapshots that callers can mutate without corrupting package-level
    tables or later plans.

## Non-goals

1. A complete model catalog. Display names, descriptions, context windows,
   modalities, lifecycle, recommendations, and model availability remain owned
   by consumers or provider catalog packages.
2. Account entitlement or deployment discovery. A classification does not
   prove that a particular API key can access the model.
3. Automatic live probing. Capability tables remain maintained and tested as
   they are today.
4. Strict validation inside normal Dive calls. Existing clamping, translation,
   omission, warnings, and unknown-model pass-through remain unchanged.
5. A separate capability version or content hash. Consumers can record the
   loaded Dive module versions through `runtime/debug.ReadBuildInfo` when they
   need to diff or roll back a published contract.
6. Native-provider inference for gateways such as OpenRouter, Amazon Bedrock,
   or Vertex deployments. Those endpoints may expose different controls and
   must register their own provider classification if needed.
7. A serialized provider wire request. `Explain` reports provider-neutral
   control outcomes, not SDK request structs or headers.
8. Moving provider request paths onto the global registry. The registry is an
   inspection facade, not a runtime dependency.

## Terminology and API boundary

The new public type is named `Classification`, not `Capabilities`. Dive already
has two other meanings for that word:

- `modelcaps.Capabilities` is the existing OpenAI/Grok runtime table record.
- `modelcatalog.Model.Capabilities` is a list of broad modalities and features
  such as text, image, or audio support.

`ClassificationFor` is the new exact, provider-registered inspection API.
Existing `Lookup`, `LookupEntry`, `TableFor`, `ResolveEffort`, and
`AcceptsTemperature` keep their current prefix-oriented runtime semantics. In
particular, `Lookup("anthropic", ...)` and `Lookup("google", ...)` do not start
working as a side effect of this proposal.

The distinction is intentional:

- **Classification** answers which independent controls Dive has verified for
  an exact catalog model.
- **Plan** answers how Dive would treat one concrete configuration.
- **Runtime lookup** preserves Dive's existing compatibility behavior for
  unknown, custom, and gateway models.

## Architecture

### Providers register resolvers

Every participating provider package registers a classifier and explainer. This
is consistent across OpenAI, Grok, Anthropic, and Google; there are no implicit
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
       Classify                       Explain
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

### Static facts stay independent

`Classification` contains facts that remain meaningful without a complete
request:

- native effort levels;
- whether Dive can emulate effort using a budget;
- fixed effective budget bounds;
- whether adaptive thinking is expressible;
- whether thinking can be disabled; and
- whether temperature is a meaningful control in the absence of another
  control that suppresses it.

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
  `Explain`.

Budget bounds are the effective fixed bounds for the named model
classification. A bound may be shared by an entire provider generation; its
presence in a per-model snapshot does not claim the source constant is
model-local.

### Explain shares request-planning predicates

Each provider extracts or reuses a pure internal control planner. Both the
actual request builder and the registered explainer call that helper. The
helper performs no network I/O and does not consult the registry.

`Explain` accepts `llm.Config` because request-dependent behavior already relies
on fields such as `Model`, `MaxTokens`, `Features`, `Prefill`, and `ToolChoice`.
A narrower public request type would duplicate that configuration and drift.
The explainer treats the input as immutable and returns only a
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

The public classifier must use an exact map from canonical catalog model ID to
the intended capability entry and verification scopes for that exact ID. The
map does not copy capability facts; it records which authoritative entry applies
and where that mapping was live-probed. A classifier must not call a plain
prefix matcher and assume the match itself is evidence. This is particularly
important for Anthropic: the existing runtime deliberately documents that a
future `claude-opus-4-9` would inherit the older `claude-opus-4` entry. That
permissive runtime behavior remains, but the public classifier returns
`ok=false` until the new ID has an explicit classification mapping.

The provider-private classification result includes the canonical model ID and
matched capability-entry key so package tests can assert the exact mapping. The
registered public projection omits that key: it is a testable source pointer,
not a consumer contract, and private table refactors should not become public
API changes.

### Linked, classified, and verification scope are separate states

`Providers()` returns the sorted canonical names registered in the current
binary. It lets a caller distinguish "the provider package is not linked" from
"the provider is linked but this model is not classified."

`ClassificationFor` returns `ok=true` for an exact model mapping even when that
mapping has no live verification scope. `VerificationScopes` names the API
surfaces against which the exact classification was successfully probed. An
empty list means the facts are historical, documented, inferred, or otherwise
not live-probed.

Verification on one scope is not evidence for another. For example,
`gemini-3.7-flash` currently carries `google:vertex-ai` but not
`google:gemini-api`, because its documented probe was performed against Vertex
AI. Anthropic's direct Messages API, Bedrock, and Vertex are likewise distinct
surfaces. A strict consumer checks the scope it will actually call.

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

// ReasoningBudgetClassification describes fixed manual reasoning-budget
// bounds. Nil means Dive has no fixed bound to publish.
type ReasoningBudgetClassification struct {
	noUnkeyedLiterals struct{}

	Minimum *int
	Maximum *int
}

// ReasoningClassification describes independent reasoning controls Dive can
// express for an exact catalog model.
type ReasoningClassification struct {
	noUnkeyedLiterals struct{}

	// NativeEfforts lists provider-native effort or thinking-level values from
	// least to most eager. Budget-only emulation is deliberately excluded.
	NativeEfforts []llm.ReasoningEffort

	// EffortEmulatedAsBudget reports that Dive can translate recognized effort
	// values into a manual budget when no native effort parameter exists.
	EffortEmulatedAsBudget bool

	// Budget is non-nil when Dive can send a manual reasoning budget.
	Budget *ReasoningBudgetClassification

	AdaptiveThinking bool

	// CanDisableThinking is false when the model has no reasoning support.
	CanDisableThinking bool
}

// Classification describes independent model controls Dive can publish
// without applying product policy or evaluating a complete request.
type Classification struct {
	noUnkeyedLiterals struct{}

	// Temperature reports whether Dive forwards temperature as a meaningful
	// control when no other requested control suppresses it.
	Temperature bool

	Reasoning ReasoningClassification

	// VerificationScopes identifies the endpoint and API surfaces on which this
	// exact classification was successfully live-probed. An empty slice makes no
	// live-verification claim.
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

// EffectiveControls is the provider-neutral result of control planning. It is
// not a serialized provider request.
type EffectiveControls struct {
	noUnkeyedLiterals struct{}

	ReasoningEffort llm.ReasoningEffort
	ReasoningBudget *int
	Thinking        llm.ThinkingType
	Temperature     *float64
}

// Plan explains how Dive would treat the model controls in a concrete config.
// When Rejected is true, the normal provider request builder would return an
// error for a control-related interaction and no request should be sent.
type Plan struct {
	noUnkeyedLiterals struct{}

	Model       string
	Effort      ControlDecision
	Budget      ControlDecision
	Thinking    ControlDecision
	Temperature ControlDecision
	Effective   EffectiveControls

	Rejected        bool
	RejectionReason string
}

type Classifier func(model string) (Classification, bool)
type Explainer func(config llm.Config) (Plan, bool)

// Resolver projects one provider's authoritative classification and pure
// request-control planner.
type Resolver struct {
	noUnkeyedLiterals struct{}

	Classify Classifier
	Explain  Explainer
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

// ClassificationFor returns static facts for an exact catalog model. ok is
// false when the provider is not registered or the normalized model is not an
// exact classified catalog ID.
func ClassificationFor(provider, model string) (Classification, bool)

// Explain performs a network-free dry run for config.Model. ok has the same
// exact-model meaning as ClassificationFor.
func Explain(provider string, config llm.Config) (Plan, bool)

// SupportsNativeEffort reports native provider support. Budget emulation does
// not count.
func (c Classification) SupportsNativeEffort(
	effort llm.ReasoningEffort,
) bool

// VerifiedOn reports whether the exact model classification was live-probed on
// the requested endpoint and API surface.
func (c Classification) VerifiedOn(scope VerificationScope) bool
```

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
  clones `NativeEfforts`, `VerificationScopes`, optional budget records and
  bounds, effective value pointers, and reason-bearing plan values as necessary.
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
classifiers use the generated provider catalog for exact ID membership, then
project the existing raw `modelcaps.Entry` so explicitly unverified entries
remain distinguishable.

For these providers:

- `NativeEfforts` comes from `Entry.Caps.Efforts`.
- `Temperature` comes from `Entry.Caps.Temperature`.
- Reasoning budgets, adaptive thinking, and explicit disablement are absent.
- `VerificationScopes` comes from the exact model mapping and is empty when
  `Entry.Unverified` is true.

The classifier uses `LookupEntry`, not `Lookup`, because `Lookup` intentionally
hides unverified entries for permissive runtime behavior. Prefix lookup may be
used to locate the underlying record only after exact catalog membership and an
explicit model-to-entry mapping have been established.

The explainer reuses `ResolveEffort` and the same temperature predicate called
by request construction. It reports native clamping as `ControlApplied` with
`Adjusted=true`, unsupported omission as `ControlOmitted`, and no budget or
adaptive emulation.

### Anthropic

Add `providers/anthropic/capabilities_register.go`. The static projection is:

| Public field             | Anthropic source                                |
| ------------------------ | ----------------------------------------------- |
| `NativeEfforts`          | `caps.efforts`                                  |
| `EffortEmulatedAsBudget` | no native efforts and `caps.manualBudget`       |
| `Budget`                 | present when `caps.manualBudget`                |
| Budget minimum           | `minThinkingBudget`                             |
| Budget maximum           | no fixed maximum                                |
| `AdaptiveThinking`       | `caps.adaptive`                                 |
| `CanDisableThinking`     | guarded semantic disablement                    |
| `Temperature`            | `caps.temperature`                              |
| `VerificationScopes`     | exact mapping scopes and private entry evidence |

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

The current `lookupCapabilities` prefix matcher remains on the runtime path.
The public classifier uses an exact catalog-ID map and records the intended
capability entry key; its coverage test asserts both the exact ID and matched
key. A future `claude-opus-4-9` therefore cannot pass coverage by inheriting
`claude-opus-4`.

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

### Google

Add `providers/google/capabilities_register.go`. The classifier projects the
raw `lookupEntry`, not `lookupCapabilities`, because the latter converts
not-live-probed entries into unknown runtime passthrough.

| Public field             | Google source                                    |
| ------------------------ | ------------------------------------------------ |
| `NativeEfforts`          | `caps.efforts`                                   |
| `EffortEmulatedAsBudget` | no native efforts on a classified budget model   |
| `Budget`                 | classified `caps.minBudget` and `caps.maxBudget` |
| `AdaptiveThinking`       | model accepts Dive's adaptive-thinking request   |
| `CanDisableThinking`     | `caps.canDisableThinking`                        |
| `Temperature`            | `modelAcceptsTemperature(model)`                 |
| `VerificationScopes`     | exact mapping scopes, empty if `caps.unverified` |

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

As with Anthropic, exact catalog-ID membership and an explicit model-to-entry
mapping precede any private prefix lookup. Vertex deployment and publisher
paths are not normalized into native Gemini IDs.

The Google explainer and request builder both use the existing thinking-plan
logic for effort-to-budget emulation, effort and budget precedence, adaptive
thinking, bounds clamping, disablement, and effective temperature omission.

## Consumer contract

A strict catalog checks provider linkage, exact classification, and evidence:

```go
if !slices.Contains(modelcaps.Providers(), "google") {
	return errors.New("google provider package is not linked")
}

class, ok := modelcaps.ClassificationFor("google", "gemini-3.7-flash")
if !ok {
	return errors.New("model is not classified")
}
if !class.VerifiedOn(modelcaps.VerificationGoogleVertexAI) {
	return errors.New("classification was not verified on Vertex AI")
}

if !class.SupportsNativeEffort(llm.ReasoningEffortHigh) {
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

plan, ok := modelcaps.Explain("anthropic", cfg)
if !ok {
	return errors.New("model is not classified")
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

A consumer may deliberately expose Dive's portable effort emulation, but it
must label that separately from native effort support. Emulation does not imply
that every provider preserves identical effort semantics.

Provider naming remains consumer-specific. Dive's canonical xAI adapter name
is `grok`; a consumer that publishes `xai` maps that name at its adapter
boundary. The registry does not register product aliases.

## Backwards compatibility

This design is additive at the public boundary:

- Existing `modelcaps` functions retain their signatures and behavior.
- `TableFor` still returns no Anthropic or Google table.
- Provider request paths keep direct access to provider-private planners and
  tables; they do not depend on global registration.
- Unknown models preserve permissive runtime behavior even though the new
  public APIs classify only exact catalog IDs.
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
providers/
├── modelcaps/
│   ├── modelcaps.go              # existing compatibility API
│   ├── tables.go                 # existing OpenAI and Grok facts
│   ├── registry.go               # new public types and registry
│   └── registry_test.go
├── anthropic/
│   ├── capabilities.go           # private source of truth and planner inputs
│   ├── capabilities_register.go  # exact classification and explanation
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
4. classified-but-not-live-probed entries returning `ok=true` with no
   verification scopes;
5. invalid and duplicate `Register` errors plus `MustRegister` panics;
6. concurrent registration and lookup safety;
7. defensive cloning of effort and verification-scope slices, budget records,
   bound pointers, and plan effective-value pointers;
8. resolvers being invoked outside the registry lock; and
9. unchanged behavior of existing `Lookup`, `ResolveEffort`, and
   `AcceptsTemperature` APIs.

Comparison tests that use `go-cmp` configure `cmpopts.IgnoreUnexported` for all
new sentinel-bearing structs.

### Provider classification tests

Each provider module verifies:

1. every catalogued text model with an ID has one exact classification mapping;
2. the mapping names the intended capability entry key and exact-ID evidence
   state, not merely any matching prefix;
3. representative native effort ladders and budget bounds match the
   authoritative table;
4. effort-to-budget emulation remains distinct from native effort;
5. temperature, adaptive thinking, and disablement match provider behavior;
6. endpoint scopes reflect exact probe evidence, including Vertex-only
   `gemini-3.7-flash`, while not-live-probed entries have no scopes;
7. invented future point releases such as `claude-opus-4-9`, `gpt-5.7`, and
   `gemini-3.8-flash` return `ok=false`;
8. arbitrary gateway and deployment paths return `ok=false`; and
9. mutating a public result cannot change a later lookup or provider request.

### Planner parity tests

Provider tests use the same table of concrete `llm.Config` cases to assert both
the dry-run plan and the constructed provider request. Representative cases
include native clamping, budget emulation, simultaneous controls, temperature
omission, adaptive thinking, disablement, defaults, request-dependent budget
floors, interleaved-thinking exceptions, and hard rejections.

The parity test must inspect the constructed request rather than reimplementing
the expected branch in test code. This is the primary defense against plan and
runtime drift.

### Commands

Run module suites independently because Google, OpenAI, and Grok are separate
Go modules:

```sh
go test ./providers/modelcaps ./providers/anthropic ./providers
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
2. Add exact catalog-ID registration in OpenAI and Grok provider packages,
   including per-ID verification scopes, while projecting their existing
   `modelcaps` entries.
3. Add the Anthropic projection, exact model-to-entry-and-scope map, and private
   `notLiveProbed` entry marker.
4. Add the Google projection using raw entry lookup, exact
   model-to-entry-and-scope mapping, and effective temperature rules.
5. Extract or consolidate one pure control planner per provider; use it from
   both request construction and `Explain` without changing request behavior.
6. Add exact-ID coverage, dry-run/request parity, and mutation-isolation tests.
7. Create `docs/guides/model-control-classification.md` with provider import
   requirements, model-ID and verification-scope rules, `go-cmp` guidance, and
   strict-consumer examples.
8. Release the root and nested provider modules at the same Dive version and
   validate their declared root-module requirements.
9. Migrate consumers separately. A consumer deletes copied facts only after its
   catalog and admission tests use both classification and planning
   successfully.

Steps 1 through 4 form a reviewable static-classification phase. Steps 5 through
7 are a second implementation phase for the real pure-planner refactor and
public `Explain` contract. They may land as separate commits, but steps 1 through
7 release together so the registry never substitutes static combination
guesses or a parallel dry-run implementation for planner parity. Consumer
adoption does not block the Dive release.

## Alternatives considered

### Publish only static classification

Rejected. Static facts are useful for catalog display but cannot answer how
temperature, budget, effort, thinking, tool choice, and token limits interact
in a concrete request. Pairwise booleans would become a duplicated rules engine.

### Publish only `Explain`

Rejected. A catalog still needs an enumerable native effort ladder, fixed
budget bounds, and evidence status without manufacturing many candidate
requests. The static and dynamic views serve different consumers.

### Add pairwise compatibility booleans

Rejected. Fields such as `EffortWithTemperature` duplicate request predicates,
cannot represent three-way interactions, and grow quadratically. `Explain`
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
  `Capabilities` type, but naming it `Classification` makes the semantic
  difference explicit.
- Exact model classification is conservative. A new catalog ID requires an
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
  while `Explain` owns request-dependent floors and ceilings.
- `MustRegister` can panic during provider initialization, but only for invalid
  code or duplicate canonical ownership. The underlying `Register` API remains
  recoverable and no registration is silently overwritten.
- The sentinel fields preserve additive compatibility but require explicit
  `go-cmp` configuration in external tests.

## Resolved review questions

Three issues that were open in the earlier draft are now explicit design
decisions:

1. Model-ID normalization is provider-specific, conservative, and exact at the
   public boundary; runtime prefix inheritance is not classification evidence.
2. Combination behavior is derived by `Explain` from the provider's shared pure
   request planner, not maintained as static pairwise booleans.
3. Verification evidence is scoped to a named endpoint and API surface; no
   provider-wide boolean can promote a Vertex probe to a direct-API guarantee.

Gateway registration and additional control families remain future proposals
because they require their own verified source of truth and consumer need.
