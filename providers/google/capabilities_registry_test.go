package google

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

func TestStaticClassificationsPreserveGoogleSemantics(t *testing.T) {
	vertex, ok := modelcaps.ClassificationFor("google", ModelGemini37Flash)
	assert.True(t, ok)
	assert.Equal(t, effortsLowThroughHigh, vertex.Reasoning.NativeEfforts)
	assert.Nil(t, vertex.Reasoning.EmulatedEfforts)
	assert.NotNil(t, vertex.Reasoning.Budget)
	assert.Equal(t, 1, *vertex.Reasoning.Budget.Minimum)
	assert.Equal(t, 32768, *vertex.Reasoning.Budget.Maximum)
	assert.True(t, vertex.Reasoning.AdaptiveThinking)
	assert.True(t, vertex.Reasoning.CanDisableThinking)
	assert.False(t, vertex.Temperature)
	assert.True(t, vertex.VerifiedOn(modelcaps.VerificationGoogleVertexAI))
	assert.False(t, vertex.VerifiedOn(modelcaps.VerificationGoogleGeminiAPI))

	budgetOnly, ok := modelcaps.ClassificationFor("google", ModelGemini25Pro)
	assert.True(t, ok)
	assert.Nil(t, budgetOnly.Reasoning.NativeEfforts)
	assert.Equal(t, budgetEmulatedEfforts, budgetOnly.Reasoning.EmulatedEfforts)
	assert.Equal(t, 128, *budgetOnly.Reasoning.Budget.Minimum)
	assert.Equal(t, 32768, *budgetOnly.Reasoning.Budget.Maximum)
	assert.True(t, budgetOnly.Reasoning.AdaptiveThinking)
	assert.False(t, budgetOnly.Reasoning.CanDisableThinking)
	assert.True(t, budgetOnly.Temperature)

	unverified, ok := modelcaps.ClassificationFor("google", ModelGemini3ProPreview)
	assert.True(t, ok)
	assert.Nil(t, unverified.Reasoning.Budget)
	assert.Nil(t, unverified.VerificationScopes)

	keepsTemperature, ok := modelcaps.ClassificationFor("google", ModelGemini35Flash)
	assert.True(t, ok)
	assert.True(t, keepsTemperature.Temperature)
	omitsTemperature, ok := modelcaps.ClassificationFor("google", ModelGemini36Flash)
	assert.True(t, ok)
	assert.False(t, omitsTemperature.Temperature)
}

func TestClassificationReturnsMutationIsolatedSnapshots(t *testing.T) {
	classification, ok := modelcaps.ClassificationFor("google", ModelGemini37Flash)
	assert.True(t, ok)
	classification.Reasoning.NativeEfforts[0] = llm.ReasoningEffortMax
	*classification.Reasoning.Budget.Minimum = 100
	*classification.Reasoning.Budget.Maximum = 200
	classification.VerificationScopes[0] = modelcaps.VerificationGoogleGeminiAPI

	again, ok := modelcaps.ClassificationFor("google", ModelGemini37Flash)
	assert.True(t, ok)
	assert.Equal(t, llm.ReasoningEffortLow, again.Reasoning.NativeEfforts[0])
	assert.Equal(t, 1, *again.Reasoning.Budget.Minimum)
	assert.Equal(t, 32768, *again.Reasoning.Budget.Maximum)
	assert.Equal(t, modelcaps.VerificationGoogleVertexAI, again.VerificationScopes[0])
}

func TestClassificationRejectsInheritedAndDeploymentModelIDs(t *testing.T) {
	for _, model := range []string{
		"gemini-3.8-flash",
		"publishers/google/models/gemini-3.7-flash",
		"projects/example/locations/us-central1/endpoints/123",
		"models/models/gemini-3.7-flash",
		"tunedModels/gemini-3.7-flash",
	} {
		_, ok := modelcaps.ClassificationFor("google", model)
		assert.False(t, ok, "model %q", model)
		_, ok = modelcaps.Explain("google", llm.Config{Model: model})
		assert.False(t, ok, "model %q", model)
	}
}

func TestExplainMatchesConstructedGoogleControls(t *testing.T) {
	tests := []struct {
		name           string
		config         llm.Config
		effortAction   modelcaps.ControlAction
		effortAdjusted bool
		budgetAction   modelcaps.ControlAction
		budgetAdjusted bool
		thinkingAction modelcaps.ControlAction
		tempAction     modelcaps.ControlAction
	}{
		{
			name: "native effort clamps to high",
			config: llm.Config{Model: ModelGemini36Flash,
				ReasoningEffort: llm.ReasoningEffortMax},
			effortAction: modelcaps.ControlApplied, effortAdjusted: true,
			budgetAction: modelcaps.ControlNotRequested, thinkingAction: modelcaps.ControlNotRequested,
			tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "model-specific native floor",
			config: llm.Config{Model: ModelGemini37Flash,
				ReasoningEffort: llm.ReasoningEffortMinimal},
			effortAction: modelcaps.ControlApplied, effortAdjusted: true,
			budgetAction: modelcaps.ControlNotRequested, thinkingAction: modelcaps.ControlNotRequested,
			tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "budget-only model emulates effort",
			config: llm.Config{Model: ModelGemini25Flash,
				ReasoningEffort: llm.ReasoningEffortHigh},
			effortAction: modelcaps.ControlEmulated, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "explicit budget wins effort",
			config: llm.Config{Model: ModelGemini36Flash,
				ReasoningEffort: llm.ReasoningEffortHigh, ReasoningBudget: intPointer(4096)},
			effortAction: modelcaps.ControlOmitted, budgetAction: modelcaps.ControlApplied,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "budget clamps to model range",
			config: llm.Config{Model: ModelGemini25Pro,
				ReasoningBudget: intPointer(60000)},
			effortAction: modelcaps.ControlNotRequested,
			budgetAction: modelcaps.ControlApplied, budgetAdjusted: true,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "adaptive thinking uses dynamic budget",
			config: llm.Config{Model: ModelGemini36Flash,
				Thinking: llm.ThinkingTypeAdaptive},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlApplied, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "supported disable uses zero budget",
			config: llm.Config{Model: ModelGemini35Flash,
				Thinking: llm.ThinkingTypeDisabled},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlApplied, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "unsupported disable uses least effort",
			config: llm.Config{Model: ModelGemini36Flash,
				Thinking: llm.ThinkingTypeDisabled},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlEmulated, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "none effort disables through budget",
			config: llm.Config{Model: ModelGemini35Flash,
				ReasoningEffort: llm.ReasoningEffortNone},
			effortAction: modelcaps.ControlEmulated, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "none effort degrades to minimum budget",
			config: llm.Config{Model: ModelGemini25Pro,
				ReasoningEffort: llm.ReasoningEffortNone},
			effortAction: modelcaps.ControlEmulated, effortAdjusted: true,
			budgetAction: modelcaps.ControlNotRequested, thinkingAction: modelcaps.ControlNotRequested,
			tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "newer model omits temperature",
			config: llm.Config{Model: ModelGemini36Flash,
				Temperature: floatPointer(0.7)},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlOmitted,
		},
		{
			name: "older model applies temperature",
			config: llm.Config{Model: ModelGemini35Flash,
				Temperature: floatPointer(0.7)},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlApplied,
		},
		{
			name: "enabled thinking uses provider default",
			config: llm.Config{Model: ModelGemini36Flash,
				Thinking: llm.ThinkingTypeEnabled},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlApplied, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "unknown effort is omitted on native model",
			config: llm.Config{Model: ModelGemini36Flash,
				ReasoningEffort: llm.ReasoningEffort("custom")},
			effortAction: modelcaps.ControlOmitted, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "custom effort keeps permissive budget emulation",
			config: llm.Config{Model: ModelGemini25Flash,
				ReasoningEffort: llm.ReasoningEffort("custom")},
			effortAction: modelcaps.ControlEmulated, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "dynamic explicit budget projects adaptive",
			config: llm.Config{Model: ModelGemini36Flash,
				ReasoningBudget: intPointer(dynamicThinkingBudget)},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlEmulated,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlNotRequested,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			plan, ok := modelcaps.Explain("google", config)
			assert.True(t, ok)
			assert.False(t, plan.Rejected)

			var request Request
			assert.NoError(t, New().applyRequestConfig(&request, &config))
			assertPlanMatchesGoogleRequest(t, plan, config, &request)
			assert.Equal(t, tt.effortAction, plan.Effort.Action)
			assert.Equal(t, tt.effortAdjusted, plan.Effort.Adjusted)
			assert.Equal(t, tt.budgetAction, plan.Budget.Action)
			assert.Equal(t, tt.budgetAdjusted, plan.Budget.Adjusted)
			assert.Equal(t, tt.thinkingAction, plan.Thinking.Action)
			assert.Equal(t, tt.tempAction, plan.Temperature.Action)
		})
	}
}

func assertPlanMatchesGoogleRequest(t *testing.T, plan modelcaps.Plan, config llm.Config, request *Request) {
	t.Helper()
	budget, hasBudget := thinkingBudget(request.Thinking)
	effort, hasEffort := thinkingEffort(request.Thinking)
	switch {
	case hasBudget && budget == dynamicThinkingBudget:
		assert.Equal(t, llm.ThinkingTypeAdaptive, plan.Effective.Thinking)
		assert.Nil(t, plan.Effective.ReasoningBudget)
	case hasBudget && budget == 0:
		assert.Equal(t, llm.ThinkingTypeDisabled, plan.Effective.Thinking)
		assert.Nil(t, plan.Effective.ReasoningBudget)
	case hasBudget:
		assert.Equal(t, llm.ThinkingTypeEnabled, plan.Effective.Thinking)
		assert.NotNil(t, plan.Effective.ReasoningBudget)
		assert.Equal(t, budget, *plan.Effective.ReasoningBudget)
	case hasEffort:
		assert.Equal(t, effort, plan.Effective.ReasoningEffort)
		if config.Thinking == llm.ThinkingTypeDisabled {
			assert.Equal(t, llm.ThinkingTypeEnabled, plan.Effective.Thinking)
		}
	case request.Thinking != nil && request.Thinking.IncludeThoughts && config.Thinking == llm.ThinkingTypeEnabled:
		assert.Equal(t, llm.ThinkingTypeEnabled, plan.Effective.Thinking)
	default:
		assert.Equal(t, llm.ReasoningEffort(""), plan.Effective.ReasoningEffort)
		assert.Nil(t, plan.Effective.ReasoningBudget)
	}

	assert.Equal(t, request.Temperature != nil, plan.Effective.Temperature != nil)
	if request.Temperature != nil {
		assert.Equal(t, *request.Temperature, *plan.Effective.Temperature)
	}

	if request.Thinking != nil && request.Thinking.ThinkingLevel != genai.ThinkingLevel("") {
		assert.True(t, hasEffort)
	}
}

func intPointer(value int) *int {
	return &value
}

func floatPointer(value float64) *float64 {
	return &value
}
