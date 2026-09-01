package openai

import (
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestEveryCatalogModelHasCapabilities is the drift guard for the OpenAI table:
// adding a model to catalog.json without recording its reasoning and
// temperature support fails here rather than at runtime against the API. An
// entry marked Unverified counts as recorded — the point is that someone
// decided, not that every model is gated.
func TestEveryCatalogModelHasCapabilities(t *testing.T) {
	for _, model := range Catalog().Models {
		if model.Kind != "" && model.Kind != "text" {
			continue // only text models take reasoning parameters
		}
		id := model.ID
		if id == "" {
			continue // alias-only entries carry no id of their own
		}
		t.Run(id, func(t *testing.T) {
			spec, declared := openAIControlsSpecs[id]
			assert.True(t, declared, "model %q (%s) has no published model-controls mapping", id, model.GoName)

			entry, found := modelcaps.LookupEntry("openai", id)
			assert.True(t, found,
				"model %q (%s) has no entry in the modelcaps OpenAI table; add one so its "+
					"reasoning and temperature parameters are gated, or mark it "+
					"Unverified if it cannot be reached", id, model.GoName)
			assert.Equal(t, spec.entryPrefix, entry.Prefix,
				"model %q must name its intended capability entry, not merely match a prefix", id)

			controls, known := modelcaps.ControlsFor("openai", " OPENAI/"+strings.ToUpper(id)+" ")
			assert.True(t, known)
			assert.Equal(t, id, controls.Model)
			assert.Equal(t, !entry.Unverified, controls.VerifiedOn(modelcaps.VerificationOpenAIResponses))

			plan, previewed := modelcaps.Preview("OPENAI", llm.Config{Model: " OPENAI/" + strings.ToUpper(id) + " "})
			assert.True(t, previewed)
			assert.Equal(t, id, plan.Model)
		})
	}
}

func TestControlsForRejectsInheritedAndGatewayModelIDs(t *testing.T) {
	for _, model := range []string{
		"gpt-5.7",
		"openrouter/openai/gpt-5.6",
		"openai/openai/gpt-5.6",
		"ft:gpt-5.6:example",
	} {
		_, ok := modelcaps.ControlsFor("openai", model)
		assert.False(t, ok, "model %q", model)
		_, ok = modelcaps.Preview("openai", llm.Config{Model: model})
		assert.False(t, ok, "model %q", model)
	}
}

func TestPreviewMatchesConstructedResponsesControls(t *testing.T) {
	temperature := 0.4
	budget := 4096
	tests := []struct {
		name   string
		model  string
		effort llm.ReasoningEffort
		want   llm.ReasoningEffort
	}{
		{name: "native clamp", model: ModelGPT54, effort: llm.ReasoningEffortMax, want: llm.ReasoningEffortXHigh},
		{name: "unsupported effort", model: ModelGPT4o, effort: llm.ReasoningEffortMedium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := llm.Config{
				Model:           tt.model,
				Messages:        []*llm.Message{llm.NewUserTextMessage("hello")},
				ReasoningEffort: tt.effort,
				ReasoningBudget: &budget,
				Thinking:        llm.ThinkingTypeAdaptive,
				Temperature:     &temperature,
			}
			plan, ok := modelcaps.Preview("openai", config)
			assert.True(t, ok)

			provider := New(WithAPIKey("test"), WithModel(tt.model))
			params, effective, err := provider.buildRequestParams(&config)
			assert.NoError(t, err)
			assert.NotNil(t, effective)
			assert.True(t, effective.Equal(plan.Effective),
				"controls reported for the response must match the previewed plan")
			assert.Equal(t, string(tt.want), string(params.Reasoning.Effort))
			assert.Equal(t, tt.want, plan.Effective.ReasoningEffort)
			assert.Equal(t, modelcaps.ControlOmitted, plan.Budget.Action)
			assert.Equal(t, modelcaps.ControlOmitted, plan.Thinking.Action)
			assert.Equal(t, params.Temperature.Valid(), plan.Effective.Temperature != nil)
			if tt.want == "" {
				assert.Equal(t, modelcaps.ControlOmitted, plan.Effort.Action)
			} else {
				assert.Equal(t, modelcaps.ControlApplied, plan.Effort.Action)
				assert.Equal(t, tt.want != tt.effort, plan.Effort.Adjusted)
			}
		})
	}
}
