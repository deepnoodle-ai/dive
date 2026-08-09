package grok

import (
	"testing"

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
			_, found := modelcaps.LookupEntry("grok", id)
			assert.True(t, found,
				"model %q (%s) has no entry in the modelcaps Grok table; add one so its "+
					"reasoning parameters are gated", id, model.GoName)
		})
	}
}
