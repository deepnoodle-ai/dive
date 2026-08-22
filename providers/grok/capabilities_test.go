package grok

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestEveryCatalogModelHasCapabilities is the drift guard for the Grok table.
// Four Grok models reject the reasoning parameter outright — including
// grok-4.20-0309-reasoning, whose name suggests otherwise — so a new model must
// be classified deliberately rather than inheriting a family default.
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
			spec, declared := grokClassificationSpecs[id]
			assert.True(t, declared, "model %q (%s) has no exact public classification mapping", id, model.GoName)

			entry, found := modelcaps.LookupEntry("grok", id)
			assert.True(t, found,
				"model %q (%s) has no entry in the modelcaps Grok table; add one so its "+
					"reasoning parameters are gated", id, model.GoName)
			assert.Equal(t, spec.entryPrefix, entry.Prefix,
				"model %q must name its intended capability entry, not merely match a prefix", id)

			classification, classified := modelcaps.ClassificationFor("grok", " X-AI/"+strings.ToUpper(id)+" ")
			assert.True(t, classified)
			assert.Equal(t, id, classification.Model)
			assert.True(t, classification.VerifiedOn(modelcaps.VerificationXAIResponses))

			plan, explained := modelcaps.Explain("GROK", llm.Config{Model: " X-AI/" + strings.ToUpper(id) + " "})
			assert.True(t, explained)
			assert.Equal(t, id, plan.Model)
		})
	}
}

func TestClassificationRejectsInheritedAndGatewayModelIDs(t *testing.T) {
	for _, model := range []string{
		"grok-4.7",
		"openrouter/x-ai/grok-4.6",
		"x-ai/x-ai/grok-4.6",
		"deployment/grok-4.6",
	} {
		_, ok := modelcaps.ClassificationFor("grok", model)
		assert.False(t, ok, "model %q", model)
		_, ok = modelcaps.Explain("grok", llm.Config{Model: model})
		assert.False(t, ok, "model %q", model)
	}
}

func TestExplainMatchesConstructedResponsesControls(t *testing.T) {
	temperature := 0.4
	budget := 4096
	tests := []struct {
		name   string
		model  string
		effort llm.ReasoningEffort
		want   llm.ReasoningEffort
	}{
		{name: "native clamp", model: ModelGrok45, effort: llm.ReasoningEffortMax, want: llm.ReasoningEffortXHigh},
		{name: "unsupported effort", model: ModelGrokBuild01, effort: llm.ReasoningEffortMedium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type capturedRequest struct {
				Reasoning struct {
					Effort string `json:"effort"`
				} `json:"reasoning"`
				Temperature *float64 `json:"temperature"`
			}
			captured := make(chan capturedRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request capturedRequest
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				captured <- request
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"resp_1","model":"`+tt.model+`","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0}}`)
			}))
			defer server.Close()

			config := llm.Config{
				Model:           tt.model,
				ReasoningEffort: tt.effort,
				ReasoningBudget: &budget,
				Thinking:        llm.ThinkingTypeAdaptive,
				Temperature:     &temperature,
			}
			plan, ok := modelcaps.Explain("grok", config)
			assert.True(t, ok)

			provider := New(
				WithAPIKey("test"),
				WithEndpoint(server.URL),
				WithModel(tt.model),
				WithMaxRetries(0),
			)
			_, err := provider.Generate(context.Background(),
				llm.WithMessages(llm.NewUserTextMessage("hello")),
				llm.WithReasoningEffort(tt.effort),
				llm.WithReasoningBudget(budget),
				llm.WithThinking(llm.ThinkingTypeAdaptive),
				llm.WithTemperature(temperature),
			)
			assert.NoError(t, err)
			request := <-captured

			assert.Equal(t, string(tt.want), request.Reasoning.Effort)
			assert.Equal(t, tt.want, plan.Effective.ReasoningEffort)
			assert.Equal(t, modelcaps.ControlOmitted, plan.Budget.Action)
			assert.Equal(t, modelcaps.ControlOmitted, plan.Thinking.Action)
			assert.Equal(t, request.Temperature != nil, plan.Effective.Temperature != nil)
			if tt.want == "" {
				assert.Equal(t, modelcaps.ControlOmitted, plan.Effort.Action)
			} else {
				assert.Equal(t, modelcaps.ControlApplied, plan.Effort.Action)
				assert.Equal(t, tt.want != tt.effort, plan.Effort.Adjusted)
			}
		})
	}
}
