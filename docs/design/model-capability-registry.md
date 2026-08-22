# Model Capability Registry: Design Proposal

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

This proposal makes a provider-neutral, read-only projection available from
`providers/modelcaps`. Provider packages remain authoritative for their own
facts and register a resolver during `init()`. Existing request behavior remains
unchanged: providers continue to clamp, translate, omit, or pass through options
exactly as they do today.

The public API serves callers that need to inspect a model before constructing
a request. A model-catalog service can therefore refuse an unsupported control
instead of discovering only after Dive silently clamps or translates it.

## Problem

The current capability ownership is split:

| Provider  | Source of truth                       | Publicly queryable |
| --------- | ------------------------------------- | ------------------ |
| OpenAI    | `providers/modelcaps/tables.go`       | Yes, partially     |
| Grok      | `providers/modelcaps/tables.go`       | Yes, partially     |
| Anthropic | `providers/anthropic/capabilities.go` | No                 |
| Google    | `providers/google/capabilities.go`    | No                 |

This is not only a discoverability problem. Consumers that publish a strict
contract need facts that differ from Dive's runtime policy:

- Dive clamps an unsupported native effort to the nearest usable value.
- Dive may translate an effort into a token budget for a model without a native
  effort parameter.
- Dive omits controls that a model rejects or ignores.
- Unknown or custom models preserve the existing permissive pass-through
  behavior.

Those are appropriate library defaults, but a catalog cannot infer exact native
support from a successful request. Today a strict consumer has to copy private
tables or infer support by calling a resolver and comparing the result. That
workaround is incomplete for Anthropic and Google because their tables are not
reachable outside their packages.

## Goals

1. Publish model-control facts for the provider adapters that have classified
   capability tables.
2. Keep provider packages authoritative; the registry stores resolvers, not a
   second copy of their data.
3. Distinguish native effort support from effort-to-budget emulation.
4. Represent fixed and request-dependent reasoning-budget constraints.
5. Represent control combinations that providers reject or Dive deliberately
   omits.
6. Preserve known-but-unverified classifications without presenting them as
   current live guarantees.
7. Preserve all existing public signatures and provider request behavior.
8. Make returned data safe to inspect and mutate without corrupting Dive's
   package-level tables.

## Non-goals

1. A complete model catalog. Display names, descriptions, context windows,
   modalities, lifecycle, recommendations, and model availability remain owned
   by consumers or provider catalog packages.
2. Account entitlement or deployment discovery. A registered capability does
   not prove that a particular API key can access the model.
3. Automatic live probing. Capability tables remain maintained and tested as
   they are today.
4. Strict request validation inside Dive. Existing clamping, translation,
   omission, warnings, and unknown-model pass-through remain unchanged.
5. A capability version or content hash. Consumers can derive a version from
   the projected facts they publish.
6. Inferring native-provider behavior for gateways such as OpenRouter. A
   gateway may expose different controls from the native endpoint and must be
   classified independently before it registers.
7. Moving provider request paths onto the public registry. The registry is an
   inspection facade, not a new runtime dependency.

## Design decisions

### The API belongs in `providers/modelcaps`

`providers/modelcaps` already owns the shared OpenAI and Grok tables and the
public reasoning-capability vocabulary. Putting a second capability type in the
parent `providers` package would create two overlapping public models and split
future maintenance across package boundaries.

The new registry therefore lives in `providers/modelcaps`. Existing APIs keep
their current meaning:

- `Lookup`
- `LookupEntry`
- `TableFor`
- `ResolveEffort`
- `AcceptsTemperature`

In particular, `Lookup("anthropic", ...)` and `Lookup("google", ...)` do not
change behavior. The generalized path is the new `CapabilitiesFor` function.

### The registry stores resolvers

Each provider registers a function that projects its private table into the
public type. No package exports its table or internal entry type, and no central
map duplicates per-model records.

The dependency direction is:

```text
provider private table
        │
        │ projects through init registration
        ▼
providers/modelcaps registry
        │
        │ read-only lookup
        ▼
consumer catalog or admission layer
```

`providers/modelcaps` never imports a provider package. Anthropic and Google
already depend on the root Dive module, so registering with `modelcaps` does not
create an import cycle.

### Capabilities describe Dive's effective request contract

Fields describe what Dive will forward meaningfully for the named model, not
only what a provider accepts syntactically. This distinction matters for models
that range-check a temperature but ignore it; Dive intentionally omits that
control and the registry reports it as unsupported.

Combination fields describe a request using only the named controls. Other
constraints, such as forced tool choice while thinking is active, remain out of
scope until a consumer needs them and Dive has a provider-neutral representation.

A combination field defaults to false and becomes true only when the current
request path and provider evidence establish that the pair is meaningful
together. Two individually supported controls do not imply that their
combination is supported.

### Classified and verified are separate states

`CapabilitiesFor` returns `ok=true` when the provider has classified the model,
including an explicitly unverified entry. `LiveVerified=false` means the facts
were not confirmed against a live provider API or are based on historical or
documented evidence.

`ok=false` means the provider is not registered or the model is not classified.
Callers that fail closed should treat both `!ok` and `!LiveVerified` as reasons
not to advertise a control unless they have their own qualification evidence.

`LiveVerified` means “confirmed successfully against a live endpoint at least
once.” It is not a freshness, entitlement, regional availability, or
served-model guarantee.

## Public API

Add `providers/modelcaps/registry.go`:

```go
package modelcaps

import "github.com/deepnoodle-ai/dive/llm"

// ReasoningBudgetCapabilities describes a manual reasoning or thinking budget.
// Nil bounds mean Dive has no fixed provider/model bound to publish.
type ReasoningBudgetCapabilities struct {
	noUnkeyedLiterals struct{}

	Minimum *int
	Maximum *int

	// Dynamic reports whether the provider accepts a sentinel that lets the model
	// choose its own budget, such as Gemini's -1 thinking budget.
	Dynamic bool

	// MustBeBelowMaxOutput reports a request-dependent upper bound. Anthropic
	// normally requires budget_tokens < max_tokens when interleaved thinking is
	// not enabled.
	MustBeBelowMaxOutput bool
}

// ReasoningCapabilities describes the reasoning controls Dive can express for
// a model.
type ReasoningCapabilities struct {
	noUnkeyedLiterals struct{}

	// NativeEfforts lists values accepted by the provider's native effort or
	// thinking-level parameter, ordered from least to most eager. It deliberately
	// excludes efforts that Dive can only emulate with a token budget.
	NativeEfforts []llm.ReasoningEffort

	// EffortEmulatedAsBudget reports that Dive can translate recognized effort
	// values into a manual budget when the model has no native effort parameter.
	EffortEmulatedAsBudget bool

	// Budget is non-nil when Dive can send a manual reasoning budget.
	Budget *ReasoningBudgetCapabilities

	AdaptiveThinking   bool
	CanDisableThinking bool

	// EffortBudgetCompatible reports whether native effort and a manual budget
	// may be sent together. It is false for emulated effort because both controls
	// target the same budget field.
	EffortBudgetCompatible bool

	EffortWithTemperature bool
	BudgetWithTemperature bool
}

// ModelControlCapabilities describes model controls that Dive can expose
// without applying product policy.
type ModelControlCapabilities struct {
	noUnkeyedLiterals struct{}

	// Temperature reports whether Dive forwards temperature as a meaningful
	// control when no reasoning control is present.
	Temperature bool

	Reasoning ReasoningCapabilities

	// LiveVerified reports whether this classification was confirmed against a
	// live provider endpoint at least once. It is not a freshness or access claim.
	LiveVerified bool
}

// Resolver projects one provider's private capability source for a model.
// It returns ok=true for classified but unverified entries.
type Resolver func(model string) (ModelControlCapabilities, bool)

// Register associates a canonical provider name with a resolver. It is intended
// for provider init functions and panics on an empty name, nil resolver, or
// duplicate registration.
func Register(provider string, resolver Resolver)

// CapabilitiesFor returns the classified controls for a model from a registered
// provider. ok is false when the provider is not registered or the model is not
// classified.
func CapabilitiesFor(provider, model string) (ModelControlCapabilities, bool)

// SupportsNativeEffort reports whether the provider accepts effort through its
// native effort or thinking-level parameter. It does not count budget emulation.
func (c ModelControlCapabilities) SupportsNativeEffort(
	effort llm.ReasoningEffort,
) bool
```

The unexported sentinel fields prevent external unkeyed composite literals.
That makes later additive fields source-compatible for callers using the
required keyed form. Provider packages can still construct values with keyed
literals and omit the sentinel.

### Registry behavior

The registry uses a `sync.RWMutex` and a map keyed by normalized provider name.

- `Register` trims whitespace, lowercases the name, and panics on invalid or
  duplicate registration.
- `CapabilitiesFor` normalizes the provider name the same way.
- The registry reads the resolver under the lock, releases the lock, and only
  then invokes it. A resolver therefore cannot deadlock the registry.
- `CapabilitiesFor` returns a deep-enough copy for public mutation: it clones
  `NativeEfforts`, the optional budget value, and both optional bounds.
- Resolver registration normally happens during package initialization, but the
  mutex keeps tests and unusual plugin-style imports race-safe.

The executable must import a provider package for that provider's `init()` to
run. The registry removes provider-specific dispatch from the lookup site; it
does not make unlinked Go packages execute automatically.

## Provider projections

### OpenAI and Grok

OpenAI and Grok tables already live in `providers/modelcaps`. Their resolvers
are registered from inside that package and project raw `Entry` values so
unverified entries remain distinguishable:

```go
func init() {
	Register("openai", resolveOpenAI)
	Register("grok", resolveGrok)
}
```

For these providers:

- `NativeEfforts` comes from `Entry.Caps.Efforts`.
- `Temperature` comes from `Entry.Caps.Temperature`.
- `EffortWithTemperature` is true only for entries whose combined controls are
  separately verified; it is not derived by AND-ing the two individual fields.
- Reasoning budgets, adaptive thinking, and explicit disablement are absent.
- `LiveVerified` is `!Entry.Unverified`.

The resolver must use `LookupEntry`, not `Lookup`, because `Lookup` intentionally
hides unverified entries for permissive runtime behavior.

### Anthropic

Add `providers/anthropic/capabilities_register.go`. Its resolver projects the
existing private `modelCapabilities` record:

| Public field             | Anthropic source                                      |
| ------------------------ | ----------------------------------------------------- |
| `NativeEfforts`          | `caps.efforts`                                        |
| `EffortEmulatedAsBudget` | no native efforts and `caps.manualBudget`             |
| `Budget`                 | present when `caps.manualBudget`                      |
| Budget minimum           | `minThinkingBudget`                                   |
| Budget maximum           | no fixed maximum                                      |
| `MustBeBelowMaxOutput`   | true for the normal non-interleaved request path      |
| `AdaptiveThinking`       | `caps.adaptive`                                       |
| `CanDisableThinking`     | whether Dive can make thinking inactive for the model |
| `Temperature`            | `caps.temperature`                                    |
| `EffortBudgetCompatible` | native efforts and `caps.manualBudget`                |
| `EffortWithTemperature`  | true only where the combined request is verified      |
| `BudgetWithTemperature`  | false; an active manual budget makes Dive omit it     |
| `LiveVerified`           | inverse of a new private `unverified` marker          |

`CanDisableThinking` is intentionally provider-neutral. It reports whether Dive
can make thinking inactive, whether the wire representation is an explicit
`thinking:{type:"disabled"}` object or omission. Private fields such as
`thinkingOnByDefault`, `explicitDisable`, and `disabledEffortCap` continue to
drive runtime behavior and are not exposed directly.

Anthropic's private table needs an explicit unverified marker for entries whose
behavior is documented or inferred but was not live-probed. Adding that private
field is not a public API change.

### Google

Add `providers/google/capabilities_register.go`. The resolver must project
`lookupEntry`, not `lookupCapabilities`, because the latter converts unverified
entries into unknown runtime passthrough.

| Public field             | Google source                                       |
| ------------------------ | --------------------------------------------------- |
| `NativeEfforts`          | `caps.efforts`                                      |
| `EffortEmulatedAsBudget` | no native efforts on a verified budget model        |
| `Budget`                 | present for verified models with classified bounds  |
| Budget bounds            | `caps.minBudget`, `caps.maxBudget`                  |
| `Dynamic`                | true for models that accept the dynamic `-1` budget |
| `CanDisableThinking`     | `caps.canDisableThinking`                           |
| `Temperature`            | `modelAcceptsTemperature(model)`                    |
| `EffortBudgetCompatible` | false; Gemini accepts one or the other              |
| Combination fields       | true only where the combined request is verified    |
| `LiveVerified`           | `!caps.unverified`                                  |

The projection must use `modelAcceptsTemperature`, not raw status-code evidence.
Some Gemini generations accept and range-check temperature but do not honor it;
Dive deliberately omits it for those models.

## Consumer contract

A strict consumer checks classification and evidence before publishing native
effort support:

```go
caps, ok := modelcaps.CapabilitiesFor("google", "gemini-3.7-flash")
if !ok || !caps.LiveVerified {
	// Do not advertise controls without separate qualification evidence.
}

if !caps.SupportsNativeEffort(llm.ReasoningEffortHigh) {
	// Refuse the requested native effort rather than relying on Dive to clamp it.
}
```

A consumer may deliberately expose Dive's portable emulation behavior, but it
must label that separately from native support. `EffortEmulatedAsBudget` does
not imply that all native effort semantics are preserved; it says only that Dive
maps recognized effort values to token budgets.

Provider naming remains consumer-specific. Dive's canonical xAI adapter name is
`grok`; a consumer that publishes `xai` maps that name at its adapter boundary.
The registry does not register product aliases.

## Backwards compatibility

This design is additive:

- Existing `modelcaps` functions retain their signatures and behavior.
- `TableFor` still returns no Anthropic or Google table.
- Provider request paths continue reading their private tables directly.
- Unknown models continue to preserve permissive request behavior.
- The new structs prevent external unkeyed construction, allowing additive
  fields without breaking unkeyed literals.
- New provider `init()` work changes only data visible through the new registry.

The public structs are read-only snapshots by convention. Mutating a returned
slice or bound changes only the caller's copy.

## Package layout

```text
providers/
├── modelcaps/
│   ├── modelcaps.go              # existing compatibility API
│   ├── tables.go                 # existing OpenAI and Grok facts
│   ├── registry.go               # new public types and registry
│   ├── registry_builtin.go       # OpenAI and Grok registration
│   └── registry_test.go
├── anthropic/
│   ├── capabilities.go           # existing private source of truth
│   ├── capabilities_register.go  # new projection
│   └── capabilities_test.go
└── google/
    ├── capabilities.go           # existing private source of truth
    ├── capabilities_register.go  # new projection
    └── capabilities_test.go
```

No new top-level package or interface hierarchy is needed.

## Validation plan

### Registry tests

Add root-module tests covering:

1. canonical and case-insensitive provider lookup;
2. unknown provider and unknown model behavior;
3. known-but-unverified entries returning `ok=true` and
   `LiveVerified=false`;
4. nil, empty-name, and duplicate registration panics;
5. concurrent lookup safety;
6. defensive cloning of effort slices, budget records, and bound pointers;
7. representative OpenAI and Grok projections;
8. unchanged behavior of the existing `Lookup`, `ResolveEffort`, and
   `AcceptsTemperature` APIs.

### Provider projection tests

Anthropic and Google tests should verify:

1. every catalogued text model is classified by the public resolver;
2. representative native effort ladders match the private table;
3. effort-to-budget emulation is distinct from native effort;
4. budget bounds and disablement match request behavior;
5. temperature and reasoning compatibility match request construction;
6. unverified entries remain classified but are not marked live-verified;
7. mutating a public result cannot change a later lookup or provider request.

OpenAI and Grok nested-module tests continue to validate that every catalogued
text model has a `modelcaps` entry.

### Commands

Run the relevant module suites independently because Google, OpenAI, and Grok
are separate Go modules:

```sh
go test ./providers/modelcaps ./providers/anthropic ./providers
(cd providers/google && go test ./...)
(cd providers/openai && go test ./...)
(cd providers/grok && go test ./...)
make provider-catalog-check
make fmt-md-check
git diff --check
```

The normal GitHub workflow remains the final cross-module check.

## Implementation and release sequence

1. Add the public types, registry, defensive cloning, and registry unit tests to
   `providers/modelcaps`.
2. Register the existing OpenAI and Grok tables from inside `modelcaps`.
3. Add the Anthropic projection and private verification marker.
4. Add the Google projection using raw entry lookup and the effective
   temperature rule.
5. Add provider-level projection and mutation-isolation tests.
6. Update the model-capability guide or release notes with the import/registration
   requirement and strict-consumer example.
7. Release the root and nested provider modules at the same Dive version.
8. Migrate consumers separately. A consumer should delete copied facts only
   after its catalog and admission tests use the registry successfully.

Steps 1 through 5 should land together so the first released registry is useful
for all four classified native providers. Consumer adoption does not block the
Dive release.

## Alternatives considered

### Put the registry in `providers`

Rejected. Pricing uses the parent package, but capability ownership already has
a focused public package. A second type in `providers` would duplicate concepts
and make `providers/modelcaps` narrower than its name and responsibility.

### Export each provider's private table

Rejected. It would couple consumers to provider-specific implementation types,
expose fields that exist only for runtime translation, and make private table
refactors public compatibility events.

### Add Anthropic and Google to `TableFor`

Rejected. `TableFor` participates in existing lookup and clamping behavior.
Teaching it new providers would change requests that currently pass through
untouched. The new registry is intentionally separate.

### Make provider request paths consult the registry

Rejected. The private tables are already the single source of truth. Routing the
runtime through global registration would add initialization order and missing
import failure modes without removing duplicated data.

### Publish only a `SupportsEffort` boolean helper

Rejected. A single boolean cannot distinguish native effort, budget emulation,
unknown models, and known-but-unverified classifications. Strict consumers need
the evidence and mechanism, not only a collapsed answer.

## Tradeoffs and consequences

- The public representation is broader than the existing OpenAI/Grok
  `Capabilities` type, but it remains limited to controls Dive already models.
- Registration depends on Go package imports, matching Dive's existing provider
  and pricing registration approach.
- `LiveVerified` is intentionally modest evidence. Consumers that publish a
  contractual catalog still need their own account-specific qualification.
- Combination fields add maintenance work when provider rules change, but they
  prevent consumers from advertising individually valid controls in an invalid
  combination.
- Some constraints remain request-dependent. The budget type makes that
  explicit instead of inventing a misleading fixed maximum.

## Open questions

None for the first implementation. Gateway-specific registration and additional
control families should be proposed only when a provider adapter has a verified
source of truth and a consumer needs them.
