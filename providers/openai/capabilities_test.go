package openai

import (
	"testing"

	"github.com/deepnoodle-ai/dive/providers/modelcaps"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestEveryCatalogModelHasCapabilities is the drift guard for the OpenAI table:
// adding a model to catalog.json without classifying its reasoning and
// temperature support fails here rather than at runtime against the API. An
// entry marked Unverified counts as classified — the point is that someone
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
			_, found := modelcaps.LookupEntry("openai", id)
			assert.True(t, found,
				"model %q (%s) has no entry in modelcaps.OpenAI; add one so its "+
					"reasoning and temperature parameters are gated, or mark it "+
					"Unverified if it cannot be reached", id, model.GoName)
		})
	}
}
