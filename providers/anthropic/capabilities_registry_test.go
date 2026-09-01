package anthropic

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestPublishedModelControlsPreserveProviderSemantics(t *testing.T) {
	legacy, ok := modelcaps.ControlsFor("anthropic", ModelClaudeSonnet45)
	assert.True(t, ok)
	assert.Equal(t, []llm.ReasoningEffort(nil), legacy.Reasoning.NativeEfforts)
	assert.Equal(t, legacyEmulatedEfforts, legacy.Reasoning.EmulatedEfforts)
	assert.NotNil(t, legacy.Reasoning.Budget)
	assert.Equal(t, minThinkingBudget, *legacy.Reasoning.Budget.Minimum)
	assert.Nil(t, legacy.Reasoning.Budget.Maximum)
	assert.False(t, legacy.Reasoning.AdaptiveThinking)
	assert.True(t, legacy.Reasoning.CanDisableThinking)
	assert.True(t, legacy.Temperature)

	adaptive, ok := modelcaps.ControlsFor("anthropic", ModelClaudeOpus48)
	assert.True(t, ok)
	assert.Equal(t, effortsFull, adaptive.Reasoning.NativeEfforts)
	assert.Nil(t, adaptive.Reasoning.EmulatedEfforts)
	assert.Nil(t, adaptive.Reasoning.Budget)
	assert.True(t, adaptive.Reasoning.AdaptiveThinking)
	assert.True(t, adaptive.Reasoning.CanDisableThinking)
	assert.False(t, adaptive.Temperature)

	nonReasoning, ok := modelcaps.ControlsFor("anthropic", ModelClaude35Haiku20241022)
	assert.True(t, ok)
	assert.False(t, nonReasoning.Reasoning.CanDisableThinking)
	assert.False(t, nonReasoning.VerifiedOn(modelcaps.VerificationAnthropicMessages))

	alwaysThinking, ok := modelcaps.ControlsFor("anthropic", ModelClaudeFable5)
	assert.True(t, ok)
	assert.False(t, alwaysThinking.Reasoning.CanDisableThinking)

	mythos, ok := modelcaps.ControlsFor("anthropic", ModelClaudeMythos5)
	assert.True(t, ok)
	assert.False(t, mythos.VerifiedOn(modelcaps.VerificationAnthropicMessages))
}

func TestControlsForReturnsMutationIsolatedSnapshots(t *testing.T) {
	controls, ok := modelcaps.ControlsFor("anthropic", ModelClaudeOpus48)
	assert.True(t, ok)
	controls.Reasoning.NativeEfforts[0] = llm.ReasoningEffortMax
	controls.VerificationScopes[0] = modelcaps.VerificationGoogleVertexAI

	again, ok := modelcaps.ControlsFor("anthropic", ModelClaudeOpus48)
	assert.True(t, ok)
	assert.Equal(t, llm.ReasoningEffortLow, again.Reasoning.NativeEfforts[0])
	assert.Equal(t, modelcaps.VerificationAnthropicMessages, again.VerificationScopes[0])

	legacy, ok := modelcaps.ControlsFor("anthropic", ModelClaudeSonnet45)
	assert.True(t, ok)
	*legacy.Reasoning.Budget.Minimum = 1
	legacy.Reasoning.EmulatedEfforts[0] = llm.ReasoningEffortMax

	legacyAgain, ok := modelcaps.ControlsFor("anthropic", ModelClaudeSonnet45)
	assert.True(t, ok)
	assert.Equal(t, minThinkingBudget, *legacyAgain.Reasoning.Budget.Minimum)
	assert.Equal(t, llm.ReasoningEffortMinimal, legacyAgain.Reasoning.EmulatedEfforts[0])
}

func TestControlsForRejectsInheritedAndGatewayModelIDs(t *testing.T) {
	for _, model := range []string{
		"claude-opus-4-9",
		"openrouter/anthropic/claude-opus-4-8",
		"anthropic/anthropic/claude-opus-4-8",
		"us.anthropic.claude-opus-4-8-v1:0",
		"publishers/anthropic/models/claude-opus-4-8",
	} {
		_, ok := modelcaps.ControlsFor("anthropic", model)
		assert.False(t, ok, "model %q", model)
		_, ok = modelcaps.Preview("anthropic", llm.Config{Model: model})
		assert.False(t, ok, "model %q", model)
	}
}

func TestPreviewMatchesConstructedAnthropicControls(t *testing.T) {
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
			name: "native effort clamps",
			config: llm.Config{Model: ModelClaudeSonnet46,
				ReasoningEffort: llm.ReasoningEffortXHigh},
			effortAction: modelcaps.ControlApplied, effortAdjusted: true,
			budgetAction: modelcaps.ControlNotRequested, thinkingAction: modelcaps.ControlNotRequested,
			tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "legacy effort owns emulated budget",
			config: llm.Config{Model: ModelClaudeSonnet45,
				ReasoningEffort: llm.ReasoningEffortMedium, Temperature: floatPointer(0.7)},
			effortAction: modelcaps.ControlEmulated, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlOmitted,
		},
		{
			name: "explicit budget clamps below max tokens",
			config: llm.Config{Model: ModelClaudeSonnet45,
				ReasoningBudget: intPointer(8000), MaxTokens: intPointer(4096)},
			effortAction: modelcaps.ControlNotRequested,
			budgetAction: modelcaps.ControlApplied, budgetAdjusted: true,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "budget is omitted when max tokens leaves no room",
			config: llm.Config{Model: ModelClaudeSonnet45,
				ReasoningBudget: intPointer(8000), MaxTokens: intPointer(1000), Temperature: floatPointer(0.7)},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlOmitted,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlApplied,
		},
		{
			name: "interleaved thinking preserves a large budget",
			config: llm.Config{Model: ModelClaudeSonnet45,
				ReasoningBudget: intPointer(8000), MaxTokens: intPointer(4096),
				Features: []string{FeatureInterleavedThinking}},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlApplied,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "manual budget translates to adaptive",
			config: llm.Config{Model: ModelClaudeOpus48,
				ReasoningBudget: intPointer(8000)},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlEmulated,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "enabled thinking supplies a default budget",
			config: llm.Config{Model: ModelClaudeSonnet45,
				Thinking: llm.ThinkingTypeEnabled},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlDefaulted,
			thinkingAction: modelcaps.ControlApplied, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name:         "provider defaults thinking on",
			config:       llm.Config{Model: ModelClaudeSonnet5},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlDefaulted, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "always-thinking model omits disable",
			config: llm.Config{Model: ModelClaudeFable5,
				Thinking: llm.ThinkingTypeDisabled},
			effortAction: modelcaps.ControlNotRequested, budgetAction: modelcaps.ControlNotRequested,
			thinkingAction: modelcaps.ControlOmitted, tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "disabled thinking caps native effort",
			config: llm.Config{Model: ModelClaudeOpus5,
				ReasoningEffort: llm.ReasoningEffortXHigh, Thinking: llm.ThinkingTypeDisabled},
			effortAction: modelcaps.ControlApplied, effortAdjusted: true,
			budgetAction: modelcaps.ControlNotRequested, thinkingAction: modelcaps.ControlApplied,
			tempAction: modelcaps.ControlNotRequested,
		},
		{
			name: "explicit budget wins legacy effort",
			config: llm.Config{Model: ModelClaudeSonnet45,
				ReasoningEffort: llm.ReasoningEffortHigh, ReasoningBudget: intPointer(9000)},
			effortAction: modelcaps.ControlOmitted, budgetAction: modelcaps.ControlApplied,
			thinkingAction: modelcaps.ControlNotRequested, tempAction: modelcaps.ControlNotRequested,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			plan, ok := modelcaps.Preview("anthropic", config)
			assert.True(t, ok)
			assert.False(t, plan.Rejected)

			var request Request
			effective, err := New().applyRequestConfig(&request, &config)
			assert.NoError(t, err)
			assert.NotNil(t, effective)
			assert.True(t, effective.Equal(plan.Effective),
				"controls reported for the response must match the previewed plan")
			assertPlanMatchesAnthropicRequest(t, plan, config, &request)
			assert.Equal(t, tt.effortAction, plan.Effort.Action)
			assert.Equal(t, tt.effortAdjusted, plan.Effort.Adjusted)
			assert.Equal(t, tt.budgetAction, plan.Budget.Action)
			assert.Equal(t, tt.budgetAdjusted, plan.Budget.Adjusted)
			assert.Equal(t, tt.thinkingAction, plan.Thinking.Action)
			assert.Equal(t, tt.tempAction, plan.Temperature.Action)
		})
	}
}

func TestPreviewMatchesAnthropicControlRejections(t *testing.T) {
	tests := []llm.Config{
		{
			Model: ModelClaudeOpus46, ReasoningBudget: intPointer(8000),
			Prefill: "prefilled answer",
		},
		{
			Model: ModelClaudeOpus46, ReasoningBudget: intPointer(8000),
			Tools: []llm.Tool{reasoningTestTool()}, ToolChoice: &llm.ToolChoice{
				Type: llm.ToolChoiceTypeTool, Name: "lookup",
			},
		},
		{
			Model: ModelClaudeFable5,
			Tools: []llm.Tool{reasoningTestTool()}, ToolChoice: llm.ToolChoiceAny,
		},
	}

	for i, config := range tests {
		t.Run(config.Model+string(rune('a'+i)), func(t *testing.T) {
			plan, ok := modelcaps.Preview("anthropic", config)
			assert.True(t, ok)
			assert.True(t, plan.Rejected)
			assert.NotEqual(t, "", plan.RejectionReason)
			assert.Equal(t, modelcaps.ControlUnspecified, plan.Effort.Action)
			assert.Equal(t, modelcaps.ControlUnspecified, plan.Budget.Action)
			assert.Equal(t, modelcaps.ControlUnspecified, plan.Thinking.Action)
			assert.Equal(t, modelcaps.ControlUnspecified, plan.Temperature.Action)
			assert.Nil(t, plan.Effective.ReasoningBudget)
			assert.Nil(t, plan.Effective.Temperature)

			var request Request
			_, err := New().applyRequestConfig(&request, &config)
			assert.Error(t, err)
			assert.Equal(t, err.Error(), plan.RejectionReason)
		})
	}
}

func assertPlanMatchesAnthropicRequest(t *testing.T, plan modelcaps.Plan, config llm.Config, request *Request) {
	t.Helper()
	if request.OutputConfig == nil {
		assert.Equal(t, llm.ReasoningEffort(""), plan.Effective.ReasoningEffort)
	} else {
		assert.Equal(t, llm.ReasoningEffort(request.OutputConfig.Effort), plan.Effective.ReasoningEffort)
	}

	if request.Thinking != nil {
		assert.Equal(t, llm.ThinkingType(request.Thinking.Type), plan.Effective.Thinking)
		if request.Thinking.Type == string(llm.ThinkingTypeEnabled) {
			assert.NotNil(t, plan.Effective.ReasoningBudget)
			assert.Equal(t, request.Thinking.BudgetTokens, *plan.Effective.ReasoningBudget)
		} else {
			assert.Nil(t, plan.Effective.ReasoningBudget)
		}
	} else if modelRunsThinkingByDefault(request.Model) {
		assert.Equal(t, llm.ThinkingTypeAdaptive, plan.Effective.Thinking)
	} else if config.Thinking == llm.ThinkingTypeDisabled {
		assert.Equal(t, llm.ThinkingTypeDisabled, plan.Effective.Thinking)
	} else {
		assert.Equal(t, llm.ThinkingType(""), plan.Effective.Thinking)
	}

	assert.Equal(t, request.Temperature != nil, plan.Effective.Temperature != nil)
	if request.Temperature != nil {
		assert.Equal(t, *request.Temperature, *plan.Effective.Temperature)
	}
}

func intPointer(value int) *int {
	return &value
}

func floatPointer(value float64) *float64 {
	return &value
}
