package modelcaps

import (
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive/llm"
)

// BudgetBounds describes fixed manual reasoning-budget bounds. Nil means Dive
// has no fixed bound to publish.
type BudgetBounds struct {
	noUnkeyedLiterals struct{}

	Minimum *int
	Maximum *int
}

// ReasoningControls describes independent reasoning controls Dive can express
// for an exact catalog model.
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

// ModelControls describes independent model controls Dive can publish without
// applying product policy or evaluating a complete request.
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

// ControlAction describes how one logical caller input affects a request.
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

// EffectiveControls is the provider-neutral result of control planning. It is
// defined in the llm package so a provider can report the same value on
// llm.Usage that Preview reports before the call.
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

// ControlsFunc returns static facts for one exact catalog model.
type ControlsFunc func(model string) (ModelControls, bool)

// PreviewFunc performs a network-free dry run for one exact catalog model.
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

var resolverRegistry = struct {
	sync.RWMutex
	resolvers map[string]Resolver
}{resolvers: make(map[string]Resolver)}

// Register associates a canonical provider name with a resolver. Invalid and
// duplicate registrations return a sentinel error and do not replace the
// existing resolver.
func Register(provider string, resolver Resolver) error {
	provider = normalizeProvider(provider)
	if provider == "" {
		return ErrInvalidProvider
	}
	if resolver.Controls == nil || resolver.Preview == nil {
		return ErrInvalidResolver
	}

	resolverRegistry.Lock()
	defer resolverRegistry.Unlock()
	if _, exists := resolverRegistry.resolvers[provider]; exists {
		return ErrProviderRegistered
	}
	resolverRegistry.resolvers[provider] = resolver
	return nil
}

// MustRegister calls Register and panics on error. Dive provider init functions
// use this form because a duplicate canonical owner is a programming error.
func MustRegister(provider string, resolver Resolver) {
	if err := Register(provider, resolver); err != nil {
		panic(err)
	}
}

// Providers returns a sorted snapshot of canonical providers registered in the
// current binary.
func Providers() []string {
	resolverRegistry.RLock()
	providers := make([]string, 0, len(resolverRegistry.resolvers))
	for provider := range resolverRegistry.resolvers {
		providers = append(providers, provider)
	}
	resolverRegistry.RUnlock()
	sort.Strings(providers)
	return providers
}

// ControlsFor returns static facts for an exact catalog model. ok is false
// when the provider is not registered or the normalized model is not an exact
// mapped catalog ID. The returned Model is that canonical ID.
func ControlsFor(provider, model string) (ModelControls, bool) {
	resolver, ok := lookupResolver(provider)
	if !ok {
		return ModelControls{}, false
	}
	controls, ok := resolver.Controls(model)
	if !ok {
		return ModelControls{}, false
	}
	return cloneModelControls(controls), true
}

// Preview performs a network-free dry run for config.Model, reporting what
// Dive would send without sending it. ok has the same exact-model meaning as
// ControlsFor, and Plan.Model is canonical.
func Preview(provider string, config llm.Config) (Plan, bool) {
	resolver, ok := lookupResolver(provider)
	if !ok {
		return Plan{}, false
	}
	plan, ok := resolver.Preview(config)
	if !ok {
		return Plan{}, false
	}
	return clonePlan(plan), true
}

// SupportsNativeEffort reports native provider support. Budget emulation does
// not count.
func (c ModelControls) SupportsNativeEffort(effort llm.ReasoningEffort) bool {
	return slices.Contains(c.Reasoning.NativeEfforts, effort)
}

// VerifiedOn reports whether the exact model control set was live-probed on
// the requested endpoint and API surface.
func (c ModelControls) VerifiedOn(scope VerificationScope) bool {
	return slices.Contains(c.VerificationScopes, scope)
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func lookupResolver(provider string) (Resolver, bool) {
	resolverRegistry.RLock()
	resolver, ok := resolverRegistry.resolvers[normalizeProvider(provider)]
	resolverRegistry.RUnlock()
	return resolver, ok
}

func cloneModelControls(controls ModelControls) ModelControls {
	controls.Reasoning.NativeEfforts = slices.Clone(controls.Reasoning.NativeEfforts)
	controls.Reasoning.EmulatedEfforts = slices.Clone(controls.Reasoning.EmulatedEfforts)
	controls.Reasoning.Budget = cloneBudgetBounds(controls.Reasoning.Budget)
	controls.VerificationScopes = slices.Clone(controls.VerificationScopes)
	return controls
}

func cloneBudgetBounds(budget *BudgetBounds) *BudgetBounds {
	if budget == nil {
		return nil
	}
	clone := *budget
	clone.Minimum = cloneInt(budget.Minimum)
	clone.Maximum = cloneInt(budget.Maximum)
	return &clone
}

func clonePlan(plan Plan) Plan {
	plan.Effective = plan.Effective.Clone()
	return plan
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
