package modelcaps

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestRegisterValidationAndDuplicateProtection(t *testing.T) {
	valid := testResolver("model")
	assert.ErrorIs(t, Register(" ", valid), ErrInvalidProvider)
	assert.ErrorIs(t, Register("registry-invalid-controls", Resolver{Preview: valid.Preview}), ErrInvalidResolver)
	assert.ErrorIs(t, Register("registry-invalid-preview", Resolver{Controls: valid.Controls}), ErrInvalidResolver)

	provider := "registry-duplicate"
	assert.NoError(t, Register(provider, valid))
	assert.ErrorIs(t, Register(" REGISTRY-DUPLICATE ", testResolver("replacement")), ErrProviderRegistered)

	controls, ok := ControlsFor(provider, "model")
	assert.True(t, ok)
	assert.Equal(t, "model", controls.Model)
}

func TestMustRegisterPanicsOnInvalidRegistration(t *testing.T) {
	assert.Panics(t, func() {
		MustRegister("", testResolver("model"))
	})
}

func TestProviderLookupIsCanonicalAndProvidersAreSortedSnapshots(t *testing.T) {
	MustRegister("registry-zulu", testResolver("model-z"))
	MustRegister(" Registry-Alpha ", testResolver("model-a"))

	controls, ok := ControlsFor(" REGISTRY-ALPHA ", "model-a")
	assert.True(t, ok)
	assert.Equal(t, "model-a", controls.Model)
	_, ok = ControlsFor("registry-alpha", "unknown")
	assert.False(t, ok)
	_, ok = ControlsFor("registry-missing", "model-a")
	assert.False(t, ok)

	providers := Providers()
	assert.True(t, slices.IsSorted(providers))
	assert.True(t, slices.Contains(providers, "registry-alpha"))
	assert.True(t, slices.Contains(providers, "registry-zulu"))
	providers[0] = "mutated"
	assert.False(t, slices.Contains(Providers(), "mutated"))
}

func TestControlsForReturnsDefensiveSnapshot(t *testing.T) {
	minimum, maximum := 1, 32
	controls := ModelControls{
		Model:       "model",
		Temperature: true,
		Reasoning: ReasoningControls{
			NativeEfforts:   []llm.ReasoningEffort{llm.ReasoningEffortLow},
			EmulatedEfforts: []llm.ReasoningEffort{llm.ReasoningEffortMedium},
			Budget: &BudgetBounds{
				Minimum: &minimum,
				Maximum: &maximum,
			},
		},
		VerificationScopes: []VerificationScope{VerificationOpenAIResponses},
	}
	provider := "registry-classification-clone"
	MustRegister(provider, Resolver{
		Controls: func(model string) (ModelControls, bool) {
			return controls, model == "model"
		},
		Preview: testResolver("model").Preview,
	})

	first, ok := ControlsFor(provider, "model")
	assert.True(t, ok)
	assert.True(t, first.SupportsNativeEffort(llm.ReasoningEffortLow))
	assert.True(t, first.VerifiedOn(VerificationOpenAIResponses))
	first.Reasoning.NativeEfforts[0] = llm.ReasoningEffortMax
	first.Reasoning.EmulatedEfforts[0] = llm.ReasoningEffortHigh
	*first.Reasoning.Budget.Minimum = 100
	*first.Reasoning.Budget.Maximum = 200
	first.VerificationScopes[0] = VerificationGoogleVertexAI

	second, ok := ControlsFor(provider, "model")
	assert.True(t, ok)
	assert.Equal(t, llm.ReasoningEffortLow, second.Reasoning.NativeEfforts[0])
	assert.Equal(t, llm.ReasoningEffortMedium, second.Reasoning.EmulatedEfforts[0])
	assert.Equal(t, 1, *second.Reasoning.Budget.Minimum)
	assert.Equal(t, 32, *second.Reasoning.Budget.Maximum)
	assert.Equal(t, VerificationOpenAIResponses, second.VerificationScopes[0])
}

func TestPreviewReturnsDefensiveSnapshot(t *testing.T) {
	budget := 2048
	temperature := 0.5
	plan := Plan{
		Model:       "model",
		Effort:      ControlDecision{Action: ControlApplied},
		Budget:      ControlDecision{Action: ControlApplied},
		Thinking:    ControlDecision{Action: ControlNotRequested},
		Temperature: ControlDecision{Action: ControlApplied},
		Effective: EffectiveControls{
			ReasoningBudget: &budget,
			Temperature:     &temperature,
		},
	}
	provider := "registry-plan-clone"
	MustRegister(provider, Resolver{
		Controls: testResolver("model").Controls,
		Preview: func(config llm.Config) (Plan, bool) {
			return plan, config.Model == "model"
		},
	})

	first, ok := Preview(provider, llm.Config{Model: "model"})
	assert.True(t, ok)
	*first.Effective.ReasoningBudget = 1
	*first.Effective.Temperature = 1

	second, ok := Preview(provider, llm.Config{Model: "model"})
	assert.True(t, ok)
	assert.Equal(t, 2048, *second.Effective.ReasoningBudget)
	assert.Equal(t, 0.5, *second.Effective.Temperature)
}

func TestResolverRunsOutsideRegistryLock(t *testing.T) {
	provider := "registry-unlocked-parent"
	child := "registry-unlocked-child"
	MustRegister(provider, Resolver{
		Controls: func(model string) (ModelControls, bool) {
			if err := Register(child, testResolver("child-model")); err != nil && !errors.Is(err, ErrProviderRegistered) {
				t.Fatalf("register child resolver: %v", err)
			}
			return ModelControls{Model: model}, true
		},
		Preview: testResolver("model").Preview,
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = ControlsFor(provider, "model")
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("controls func was invoked while the registry lock was held")
	}
}

func TestConcurrentRegistrationAndLookup(t *testing.T) {
	const count = 32
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			provider := fmt.Sprintf("registry-concurrent-%02d", i)
			model := fmt.Sprintf("model-%02d", i)
			if err := Register(provider, testResolver(model)); err != nil {
				t.Errorf("register %s: %v", provider, err)
				return
			}
			controls, ok := ControlsFor(provider, model)
			if !ok || controls.Model != model {
				t.Errorf("lookup %s: got %#v, %v", provider, controls, ok)
			}
		}()
	}
	wg.Wait()
}

func testResolver(model string) Resolver {
	return Resolver{
		Controls: func(candidate string) (ModelControls, bool) {
			return ModelControls{Model: model}, candidate == model
		},
		Preview: func(config llm.Config) (Plan, bool) {
			if config.Model != model {
				return Plan{}, false
			}
			return Plan{
				Model:       model,
				Effort:      ControlDecision{Action: ControlNotRequested},
				Budget:      ControlDecision{Action: ControlNotRequested},
				Thinking:    ControlDecision{Action: ControlNotRequested},
				Temperature: ControlDecision{Action: ControlNotRequested},
			}, true
		},
	}
}
