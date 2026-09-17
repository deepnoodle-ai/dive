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

// imageCatalog is a catalog whose only price row is an image row, with the
// billing shape left to the caller to fill in.
func imageCatalog(row string) []byte {
	return []byte(`{
      "schema_version": 1,
      "provider": "test",
      "models": [{"go_name":"ModelPrimary","id":"primary","default":true}],
      "pricing": {"image":[` + row + `]}
    }`)
}

func TestParseAcceptsEitherImageBillingShape(t *testing.T) {
	perImage, err := Parse("test", imageCatalog(`{
      "model":"primary",
      "price_per_image":"0.040",
      "max_size":"1024x1024",
      "currency":"USD",
      "updated_at":"2026-08-08"
    }`))
	assert.NoError(t, err)
	assert.Equal(t, "0.040", perImage.Pricing.Image[0].Price)
	assert.Nil(t, perImage.Pricing.Image[0].TokenPricing)

	perToken, err := Parse("test", imageCatalog(`{
      "model":"primary",
      "token_pricing":{
        "input_price_per_1m_tokens":"2.00",
        "output_price_per_1m_tokens":"12.00",
        "output_price_per_1m_tokens_by_modality":{"image":"120.00"}
      },
      "currency":"USD",
      "updated_at":"2026-08-08"
    }`))
	assert.NoError(t, err)
	assert.Equal(t, "", perToken.Pricing.Image[0].Price)
	assert.Equal(t, "120.00", perToken.Pricing.Image[0].TokenPricing.OutputPriceByModality["image"])
}

// Carrying both shapes lets the two drift apart, and carrying neither leaves the
// row priceless. Either one is a data bug worth failing the build over.
func TestParseRejectsAmbiguousImageBilling(t *testing.T) {
	both := imageCatalog(`{
      "model":"primary",
      "price_per_image":"0.134",
      "token_pricing":{
        "input_price_per_1m_tokens":"2.00",
        "output_price_per_1m_tokens":"12.00"
      },
      "currency":"USD",
      "updated_at":"2026-08-08"
    }`)
	_, err := Parse("test", both)
	assert.Error(t, err)

	neither := imageCatalog(`{"model":"primary","currency":"USD","updated_at":"2026-08-08"}`)
	_, err = Parse("test", neither)
	assert.Error(t, err)
}

// max_size names the resolution a per-image price is quoted at, so it means
// nothing next to a token rate that applies at every size.
func TestParseRejectsMaxSizeOnTokenBilledImage(t *testing.T) {
	_, err := Parse("test", imageCatalog(`{
      "model":"primary",
      "max_size":"4096x4096",
      "token_pricing":{
        "input_price_per_1m_tokens":"2.00",
        "output_price_per_1m_tokens":"12.00"
      },
      "currency":"USD",
      "updated_at":"2026-08-08"
    }`))
	assert.Error(t, err)
}

// Clone must detach the nested token rates too, or a caller mutating its copy
// reaches back into the embedded package-level catalog.
func TestCloneDetachesImageTokenPricing(t *testing.T) {
	catalog, err := Parse("test", imageCatalog(`{
      "model":"primary",
      "token_pricing":{
        "input_price_per_1m_tokens":"2.00",
        "output_price_per_1m_tokens":"12.00",
        "output_price_per_1m_tokens_by_modality":{"image":"120.00"}
      },
      "currency":"USD",
      "updated_at":"2026-08-08"
    }`))
	assert.NoError(t, err)

	clone := catalog.Clone()
	clone.Pricing.Image[0].TokenPricing.OutputPriceByModality["image"] = "1.00"
	assert.Equal(t, "120.00", catalog.Pricing.Image[0].TokenPricing.OutputPriceByModality["image"])
}
