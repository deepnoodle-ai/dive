package modelcatalog

import (
	"strings"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

const validCatalog = `{
  "schema_version": 1,
  "provider": "test",
  "sources": [{"name": "models", "url": "https://example.com/models"}],
  "models": [
    {
      "go_name": "ModelPrimary",
      "id": "model-primary",
      "display_name": "Primary",
      "description": "Recommended model",
      "context_window": 1000,
      "recommended": true,
      "cli_order": 1,
      "default": true
    },
    {"go_name": "ModelAlias", "alias_of": "ModelPrimary", "deprecated": "Use ModelPrimary."}
  ],
  "pricing": {
    "text": [{
      "model": "model-primary",
      "input_price_per_1m_tokens": "1.25",
      "output_price_per_1m_tokens": "5.00",
      "currency": "USD",
      "updated_at": "2026-08-08"
    }]
  },
  "feature_flags": [{"go_name": "FeatureExample", "id": "feature-2026-08-08"}]
}`

func TestParseAndCatalogHelpers(t *testing.T) {
	catalog, err := Parse("test", []byte(validCatalog))
	assert.NoError(t, err)
	assert.Equal(t, "model-primary", catalog.Models[0].ID)
	assert.Equal(t, "ModelPrimary", catalog.Models[1].AliasOf)
	assert.Equal(t, "model-primary", catalog.RecommendedModels()[0].ID)
	model, ok := catalog.DefaultModel()
	assert.True(t, ok)
	assert.Equal(t, "model-primary", model.ID)

	clone := catalog.Clone()
	clone.Models[0].ID = "changed"
	assert.Equal(t, "model-primary", catalog.Models[0].ID)
}

func TestCloneDetachesNestedSlicesAndAcceptsZeroValue(t *testing.T) {
	catalog := Catalog{
		Models:       []Model{{GoName: "ModelPrimary", Capabilities: []string{"text"}}},
		FeatureFlags: []Feature{{GoName: "FeatureExample", Models: []string{"model-primary"}}},
		Sources:      []Source{{Name: "models", DiscoveryPatterns: []string{"/models"}}},
	}

	clone := catalog.Clone()
	clone.Models[0].Capabilities[0] = "changed"
	clone.FeatureFlags[0].Models[0] = "changed"
	clone.Sources[0].DiscoveryPatterns[0] = "changed"

	assert.Equal(t, "text", catalog.Models[0].Capabilities[0])
	assert.Equal(t, "model-primary", catalog.FeatureFlags[0].Models[0])
	assert.Equal(t, "/models", catalog.Sources[0].DiscoveryPatterns[0])

	// A copy method must not depend on the value being valid.
	assert.Equal(t, Catalog{}, Catalog{}.Clone())
}

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := Parse("test", []byte(`{"schema_version":1,"provider":"test","unknown":true}`))
	assert.Error(t, err)
}

func TestParseRejectsWrongProvider(t *testing.T) {
	_, err := Parse("other", []byte(validCatalog))
	assert.Error(t, err)
}

func TestParseRejectsInvalidAlias(t *testing.T) {
	invalid := []byte(`{
      "schema_version": 1,
      "provider": "test",
      "models": [
        {"go_name":"ModelPrimary","id":"primary","default":true},
        {"go_name":"ModelAlias","alias_of":"ModelMissing"}
      ],
      "pricing": {}
    }`)
	_, err := Parse("test", invalid)
	assert.Error(t, err)
}

func TestParseRejectsInvalidPrice(t *testing.T) {
	invalid := []byte(`{
      "schema_version": 1,
      "provider": "test",
      "models": [{"go_name":"ModelPrimary","id":"primary","default":true}],
      "pricing": {"text":[{
        "model":"primary",
        "input_price_per_1m_tokens":"free",
        "output_price_per_1m_tokens":"1",
        "currency":"USD",
        "updated_at":"2026-08-08"
      }]}
    }`)
	_, err := Parse("test", invalid)
	assert.Error(t, err)
}

func TestParseRejectsIncompleteCacheReadPriceTier(t *testing.T) {
	invalid := strings.Replace(
		validCatalog,
		`"output_price_per_1m_tokens": "5.00"`,
		`"output_price_per_1m_tokens": "5.00", "cache_read_price_threshold_tokens": 200000`,
		1,
	)
	_, err := Parse("test", []byte(invalid))
	assert.Error(t, err)
}

func TestParseRejectsIncompleteLongContextPriceTier(t *testing.T) {
	invalid := strings.Replace(
		validCatalog,
		`"output_price_per_1m_tokens": "5.00"`,
		`"output_price_per_1m_tokens": "5.00", "long_context_threshold_tokens": 200000, "long_context_input_price_per_1m_tokens": "2.50"`,
		1,
	)
	_, err := Parse("test", []byte(invalid))
	assert.Error(t, err)
}

func TestParseRejectsLongContextCacheWriteWithoutStandardRate(t *testing.T) {
	invalid := strings.Replace(
		validCatalog,
		`"output_price_per_1m_tokens": "5.00"`,
		`"output_price_per_1m_tokens": "5.00", "long_context_threshold_tokens": 200000, "long_context_input_price_per_1m_tokens": "2.50", "long_context_cache_read_price_per_1m_tokens": "0.25", "long_context_output_price_per_1m_tokens": "10.00", "long_context_cache_write_price_per_1m_tokens": "3.00"`,
		1,
	)
	_, err := Parse("test", []byte(invalid))
	assert.Error(t, err)
}

func TestParseRejectsNonFinitePrices(t *testing.T) {
	for _, value := range []string{"NaN", "Inf"} {
		t.Run(value, func(t *testing.T) {
			invalid := strings.Replace(validCatalog, `"input_price_per_1m_tokens": "1.25"`, `"input_price_per_1m_tokens": "`+value+`"`, 1)
			_, err := Parse("test", []byte(invalid))
			assert.Error(t, err)
		})
	}
}

func TestParseRejectsHostlessSourceURL(t *testing.T) {
	invalid := strings.Replace(validCatalog, "https://example.com/models", "https:///models", 1)
	_, err := Parse("test", []byte(invalid))
	assert.Error(t, err)
}
