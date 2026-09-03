package meta

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestDefaults(t *testing.T) {
	p := New(WithAPIKey("test"))
	assert.Equal(t, p.Name(), "meta")
	assert.Equal(t, DefaultModel, ModelMuseSpark13)
	assert.Equal(t, DefaultEndpoint, "https://api.meta.ai/v1")
}

// The registry routes on the model id, so a bare "muse-spark-1.3" has to reach
// this provider without the caller naming it.
func TestRegistryRoutesMuseIDs(t *testing.T) {
	for _, model := range []string{
		ModelMuseSpark13,
		ModelMuseSpark13Contributor,
		ModelMuseSpark12,
		ModelMuseSpark11,
	} {
		t.Run(model, func(t *testing.T) {
			created := providers.CreateModel(model, "")
			assert.NotNil(t, created, "registry should route "+model)
			assert.Equal(t, created.Name(), "meta")
		})
	}
}

// "meta-llama/..." is an OpenRouter id for Meta's open-weight Llama models, not
// a Model API id, and Model API does not serve it. Matching on the "muse-"
// family rather than on the vendor name is what keeps it falling through to
// OpenRouter's matcher.
func TestVendorPrefixedLlamaIDsDoNotMatch(t *testing.T) {
	entries := providers.DefaultRegistry().Entries()
	var meta providers.ProviderEntry
	for _, entry := range entries {
		if entry.Name == "meta" {
			meta = entry
		}
	}
	assert.Equal(t, meta.Name, "meta", "meta should be registered")
	assert.False(t, meta.Match("meta-llama/llama-3-70b-instruct"))
	assert.False(t, meta.Match("muse"), "the family prefix carries a trailing dash")
	assert.True(t, meta.Match("muse-spark-1.3"))
}

func TestPricingRegistered(t *testing.T) {
	for model := range TextModelPricing {
		_, ok := providers.PricingFor(model, false)
		assert.True(t, ok, "meta pricing should be registered: "+model)
	}
}

// The contributor tier is an order of magnitude cheaper because Meta trains on
// the traffic. Pinning the gap keeps a careless catalog edit from quietly
// costing 12x on input, or quietly opting a workload into training.
func TestTierPricing(t *testing.T) {
	standard := TextModelPricing[ModelMuseSpark13]
	assert.Equal(t, standard.InputPrice, 1.25)
	assert.Equal(t, standard.CacheReadPrice, 0.15)
	assert.Equal(t, standard.OutputPrice, 4.25)

	contributor := TextModelPricing[ModelMuseSpark13Contributor]
	assert.Equal(t, contributor.InputPrice, 0.10)
	assert.Equal(t, contributor.CacheReadPrice, 0.002)
	assert.Equal(t, contributor.OutputPrice, 0.20)

	// Meta publishes one rate at any context length, unlike Grok and Google.
	assert.Equal(t, standard.LongContextThreshold, 0,
		"Model API documents no long-context premium")
}

// Drift guard: a model added to the catalog without a modelcaps entry would
// have its reasoning parameters forwarded unclamped, and Muse Spark rejects
// "none" with a 400 rather than ignoring it.
func TestEveryCatalogModelHasCapabilities(t *testing.T) {
	for _, model := range Catalog().Models {
		if model.Kind != "" && model.Kind != "text" {
			continue
		}
		if model.ID == "" {
			continue // alias-only entries carry no id of their own
		}
		t.Run(model.ID, func(t *testing.T) {
			_, found := modelcaps.LookupEntry("meta", model.ID)
			assert.True(t, found,
				"model "+model.ID+" has no entry in the modelcaps Muse table")
		})
	}
}

// Muse Spark's ladder is open at neither end: "none" is a 400 and "max" is not
// offered, so a portable ModelSettings carrying either has to be clamped rather
// than forwarded.
func TestReasoningEffortIsClampedToTheMuseLadder(t *testing.T) {
	tests := []struct {
		requested llm.ReasoningEffort
		want      llm.ReasoningEffort
	}{
		{llm.ReasoningEffortNone, llm.ReasoningEffortMinimal},
		{llm.ReasoningEffortMinimal, llm.ReasoningEffortMinimal},
		{llm.ReasoningEffortMedium, llm.ReasoningEffortMedium},
		{llm.ReasoningEffortXHigh, llm.ReasoningEffortXHigh},
		{llm.ReasoningEffortMax, llm.ReasoningEffortXHigh},
	}
	for _, tt := range tests {
		t.Run(string(tt.requested), func(t *testing.T) {
			got, send := modelcaps.ResolveEffort("meta", ModelMuseSpark13, tt.requested, nil)
			assert.True(t, send, "Muse Spark always takes a reasoning parameter")
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestTemperatureIsAccepted(t *testing.T) {
	assert.True(t, modelcaps.AcceptsTemperature("meta", ModelMuseSpark13))
}
