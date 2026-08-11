// Package modelcatalog defines the embedded provider catalog format used by Dive.
package modelcatalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

const SchemaVersion = 1

var (
	goNamePattern = regexp.MustCompile(`^Model[A-Za-z0-9_]+$`)
	datePattern   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// Catalog is the validated representation of one provider's catalog.json.
type Catalog struct {
	SchemaVersion int       `json:"schema_version"`
	Provider      string    `json:"provider"`
	Sources       []Source  `json:"sources"`
	Models        []Model   `json:"models"`
	Pricing       Pricing   `json:"pricing"`
	FeatureFlags  []Feature `json:"feature_flags,omitempty"`
}

// Source identifies an authoritative upstream page or index watched for drift.
type Source struct {
	Name              string   `json:"name"`
	URL               string   `json:"url"`
	Kind              string   `json:"kind,omitempty"`
	DiscoveryPatterns []string `json:"discovery_patterns,omitempty"`
}

// Model describes a model constant plus optional runtime and CLI metadata.
type Model struct {
	GoName        string   `json:"go_name"`
	GoType        string   `json:"go_type,omitempty"`
	ID            string   `json:"id,omitempty"`
	AliasOf       string   `json:"alias_of,omitempty"`
	Kind          string   `json:"kind,omitempty"`
	DisplayName   string   `json:"display_name,omitempty"`
	Description   string   `json:"description,omitempty"`
	ContextWindow int      `json:"context_window,omitempty"`
	Recommended   bool     `json:"recommended,omitempty"`
	CLIOrder      int      `json:"cli_order,omitempty"`
	Default       bool     `json:"default,omitempty"`
	Lifecycle     string   `json:"lifecycle,omitempty"`
	Deprecated    string   `json:"deprecated,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Adapters      []string `json:"adapters,omitempty"`
}

// Feature describes a provider-level opt-in, beta header, or rollout flag.
type Feature struct {
	GoName      string   `json:"go_name"`
	ID          string   `json:"id"`
	AliasOf     string   `json:"alias_of,omitempty"`
	Deprecated  string   `json:"deprecated,omitempty"`
	Description string   `json:"description,omitempty"`
	Lifecycle   string   `json:"lifecycle,omitempty"`
	Models      []string `json:"models,omitempty"`
}

// Pricing groups the price tables generated into provider packages.
type Pricing struct {
	Text      []TextPrice      `json:"text,omitempty"`
	FastText  []TextPrice      `json:"fast_text,omitempty"`
	Image     []ImagePrice     `json:"image,omitempty"`
	Embedding []EmbeddingPrice `json:"embedding,omitempty"`
}

// TextPrice uses decimal strings so source prices survive JSON and code generation exactly.
type TextPrice struct {
	Model                        string            `json:"model"`
	InputPrice                   string            `json:"input_price_per_1m_tokens"`
	OutputPrice                  string            `json:"output_price_per_1m_tokens"`
	LongContextThreshold         int               `json:"long_context_threshold_tokens,omitempty"`
	LongContextInputPrice        string            `json:"long_context_input_price_per_1m_tokens,omitempty"`
	LongContextCacheReadPrice    string            `json:"long_context_cache_read_price_per_1m_tokens,omitempty"`
	LongContextOutputPrice       string            `json:"long_context_output_price_per_1m_tokens,omitempty"`
	CacheReadPrice               string            `json:"cache_read_price_per_1m_tokens,omitempty"`
	CacheReadPriceAboveThreshold string            `json:"cache_read_price_above_threshold_per_1m_tokens,omitempty"`
	CacheReadPriceThreshold      int               `json:"cache_read_price_threshold_tokens,omitempty"`
	CacheWritePrice              string            `json:"cache_write_price_per_1m_tokens,omitempty"`
	InputPriceByModality         map[string]string `json:"input_price_per_1m_tokens_by_modality,omitempty"`
	OutputPriceByModality        map[string]string `json:"output_price_per_1m_tokens_by_modality,omitempty"`
	CacheReadPriceByModality     map[string]string `json:"cache_read_price_per_1m_tokens_by_modality,omitempty"`
	NonGlobalPriceMultiplier     string            `json:"non_global_price_multiplier,omitempty"`
	Currency                     string            `json:"currency"`
	UpdatedAt                    string            `json:"updated_at"`
	Note                         string            `json:"note,omitempty"`
}

// ImagePrice describes per-image list pricing.
type ImagePrice struct {
	Model     string `json:"model"`
	Price     string `json:"price_per_image"`
	MaxSize   string `json:"max_size,omitempty"`
	Currency  string `json:"currency"`
	UpdatedAt string `json:"updated_at"`
	Note      string `json:"note,omitempty"`
}

// EmbeddingPrice describes per-million-token embedding pricing.
type EmbeddingPrice struct {
	Model     string `json:"model"`
	Price     string `json:"price_per_1m_tokens"`
	Currency  string `json:"currency"`
	UpdatedAt string `json:"updated_at"`
	Note      string `json:"note,omitempty"`
}

// Parse decodes and validates an embedded provider catalog.
func Parse(expectedProvider string, data []byte) (Catalog, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var catalog Catalog
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("decode provider catalog: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Catalog{}, err
	}
	if err := catalog.validate(expectedProvider); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

// MustParse is intended for package-level initialization of checked-in embeds.
func MustParse(expectedProvider string, data []byte) Catalog {
	catalog, err := Parse(expectedProvider, data)
	if err != nil {
		panic(err)
	}
	return catalog
}

// Clone returns a deep copy that callers may modify without changing package state.
func (c Catalog) Clone() Catalog {
	clone := c
	clone.Sources = slices.Clone(c.Sources)
	for i := range clone.Sources {
		clone.Sources[i].DiscoveryPatterns = slices.Clone(c.Sources[i].DiscoveryPatterns)
	}
	clone.Models = slices.Clone(c.Models)
	for i := range clone.Models {
		clone.Models[i].Capabilities = slices.Clone(c.Models[i].Capabilities)
		clone.Models[i].Adapters = slices.Clone(c.Models[i].Adapters)
	}
	clone.FeatureFlags = slices.Clone(c.FeatureFlags)
	for i := range clone.FeatureFlags {
		clone.FeatureFlags[i].Models = slices.Clone(c.FeatureFlags[i].Models)
	}
	clone.Pricing.Text = slices.Clone(c.Pricing.Text)
	clone.Pricing.FastText = slices.Clone(c.Pricing.FastText)
	for _, prices := range [][]TextPrice{
		clone.Pricing.Text,
		clone.Pricing.FastText,
	} {
		for i := range prices {
			prices[i].InputPriceByModality = maps.Clone(prices[i].InputPriceByModality)
			prices[i].OutputPriceByModality = maps.Clone(prices[i].OutputPriceByModality)
			prices[i].CacheReadPriceByModality = maps.Clone(prices[i].CacheReadPriceByModality)
		}
	}
	clone.Pricing.Image = slices.Clone(c.Pricing.Image)
	clone.Pricing.Embedding = slices.Clone(c.Pricing.Embedding)
	return clone
}

// RecommendedModels returns CLI recommendations in their declared order.
func (c Catalog) RecommendedModels() []Model {
	models := make([]Model, 0)
	for _, model := range c.Models {
		if model.Recommended {
			models = append(models, model)
		}
	}
	sort.SliceStable(models, func(i, j int) bool {
		return models[i].CLIOrder < models[j].CLIOrder
	})
	return models
}

// DefaultModel returns the canonical default model definition.
func (c Catalog) DefaultModel() (Model, bool) {
	for _, model := range c.Models {
		if model.Default {
			return model, true
		}
	}
	return Model{}, false
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode provider catalog: multiple JSON values")
		}
		return fmt.Errorf("decode provider catalog trailer: %w", err)
	}
	return nil
}

func (c Catalog) validate(expectedProvider string) error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("provider catalog schema_version must be %d", SchemaVersion)
	}
	if c.Provider == "" || c.Provider != expectedProvider {
		return fmt.Errorf("provider catalog provider %q does not match %q", c.Provider, expectedProvider)
	}
	if len(c.Models) == 0 {
		return errors.New("provider catalog must contain at least one model")
	}
	if err := validateSources(c.Sources); err != nil {
		return err
	}
	if err := validateModels(c.Models); err != nil {
		return err
	}
	if err := validateTextPrices("text", c.Pricing.Text); err != nil {
		return err
	}
	if err := validateTextPrices("fast_text", c.Pricing.FastText); err != nil {
		return err
	}
	if err := validateImagePrices(c.Pricing.Image); err != nil {
		return err
	}
	if err := validateEmbeddingPrices(c.Pricing.Embedding); err != nil {
		return err
	}
	return validateFeatures(c.FeatureFlags)
}

func validateSources(sources []Source) error {
	seen := map[string]bool{}
	for _, source := range sources {
		if source.Name == "" || source.URL == "" {
			return errors.New("provider catalog sources require name and url")
		}
		if seen[source.Name] {
			return fmt.Errorf("duplicate provider catalog source %q", source.Name)
		}
		seen[source.Name] = true
		parsed, err := url.ParseRequestURI(source.URL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("invalid provider catalog source URL %q", source.URL)
		}
	}
	return nil
}

func validateModels(models []Model) error {
	byName := map[string]Model{}
	ids := map[string]string{}
	defaultCount := 0
	for _, model := range models {
		if !goNamePattern.MatchString(model.GoName) {
			return fmt.Errorf("invalid model go_name %q", model.GoName)
		}
		if model.GoType != "" && model.GoType != "openaisdk.ChatModel" {
			return fmt.Errorf("model %s has unsupported go_type %q", model.GoName, model.GoType)
		}
		if _, ok := byName[model.GoName]; ok {
			return fmt.Errorf("duplicate model go_name %q", model.GoName)
		}
		if (model.ID == "") == (model.AliasOf == "") {
			return fmt.Errorf("model %s must set exactly one of id or alias_of", model.GoName)
		}
		if model.ID != "" {
			if previous, ok := ids[model.ID]; ok {
				return fmt.Errorf("models %s and %s share id %q", previous, model.GoName, model.ID)
			}
			ids[model.ID] = model.GoName
		}
		if model.ContextWindow < 0 || model.CLIOrder < 0 {
			return fmt.Errorf("model %s has a negative context_window or cli_order", model.GoName)
		}
		if model.Recommended && (model.ID == "" || model.DisplayName == "" || model.Description == "" || model.CLIOrder == 0) {
			return fmt.Errorf("recommended model %s requires id, display_name, description, and cli_order", model.GoName)
		}
		if model.Default {
			defaultCount++
		}
		byName[model.GoName] = model
	}
	if defaultCount != 1 {
		return fmt.Errorf("provider catalog requires exactly one default model, got %d", defaultCount)
	}
	for _, model := range models {
		if model.AliasOf == "" {
			continue
		}
		target, ok := byName[model.AliasOf]
		if !ok {
			return fmt.Errorf("model %s aliases unknown model %s", model.GoName, model.AliasOf)
		}
		if target.AliasOf != "" {
			return fmt.Errorf("model %s aliases another alias %s", model.GoName, model.AliasOf)
		}
	}
	return nil
}

func validateTextPrices(name string, prices []TextPrice) error {
	seen := map[string]bool{}
	for _, price := range prices {
		if err := validatePriceCommon(name, price.Model, price.Currency, price.UpdatedAt, seen); err != nil {
			return err
		}
		for field, value := range map[string]string{
			"input_price_per_1m_tokens":                      price.InputPrice,
			"output_price_per_1m_tokens":                     price.OutputPrice,
			"long_context_input_price_per_1m_tokens":         price.LongContextInputPrice,
			"long_context_cache_read_price_per_1m_tokens":    price.LongContextCacheReadPrice,
			"long_context_output_price_per_1m_tokens":        price.LongContextOutputPrice,
			"cache_read_price_per_1m_tokens":                 price.CacheReadPrice,
			"cache_read_price_above_threshold_per_1m_tokens": price.CacheReadPriceAboveThreshold,
			"cache_write_price_per_1m_tokens":                price.CacheWritePrice,
			"non_global_price_multiplier":                    price.NonGlobalPriceMultiplier,
		} {
			if err := validateDecimal(name, price.Model, field, value, field == "input_price_per_1m_tokens" || field == "output_price_per_1m_tokens"); err != nil {
				return err
			}
		}
		for field, prices := range map[string]map[string]string{
			"input_price_per_1m_tokens_by_modality":      price.InputPriceByModality,
			"output_price_per_1m_tokens_by_modality":     price.OutputPriceByModality,
			"cache_read_price_per_1m_tokens_by_modality": price.CacheReadPriceByModality,
		} {
			for modality, value := range prices {
				if modality == "" || strings.ToLower(modality) != modality {
					return fmt.Errorf("%s pricing for %s has an invalid modality %q in %s", name, price.Model, modality, field)
				}
				if err := validateDecimal(name, price.Model, field+"/"+modality, value, true); err != nil {
					return err
				}
			}
		}
		if price.NonGlobalPriceMultiplier != "" {
			multiplier, _ := strconv.ParseFloat(price.NonGlobalPriceMultiplier, 64)
			if multiplier <= 0 {
				return fmt.Errorf("%s pricing for %s requires a positive non-global price multiplier", name, price.Model)
			}
		}
		hasThreshold := price.CacheReadPriceThreshold != 0
		hasPriceAboveThreshold := price.CacheReadPriceAboveThreshold != ""
		if hasThreshold != hasPriceAboveThreshold || price.CacheReadPriceThreshold < 0 {
			return fmt.Errorf("%s pricing for %s requires a positive cache-read threshold and above-threshold price together", name, price.Model)
		}
		hasLongThreshold := price.LongContextThreshold != 0
		hasLongPrices := price.LongContextInputPrice != "" || price.LongContextCacheReadPrice != "" || price.LongContextOutputPrice != ""
		if hasLongThreshold != hasLongPrices || price.LongContextThreshold < 0 {
			return fmt.Errorf("%s pricing for %s requires a positive long-context threshold and long-context prices together", name, price.Model)
		}
		if hasLongThreshold && (price.LongContextInputPrice == "" || price.LongContextCacheReadPrice == "" || price.LongContextOutputPrice == "") {
			return fmt.Errorf("%s pricing for %s requires input, cache-read, and output long-context prices", name, price.Model)
		}
	}
	return nil
}

func validateImagePrices(prices []ImagePrice) error {
	seen := map[string]bool{}
	for _, price := range prices {
		if err := validatePriceCommon("image", price.Model, price.Currency, price.UpdatedAt, seen); err != nil {
			return err
		}
		if err := validateDecimal("image", price.Model, "price_per_image", price.Price, true); err != nil {
			return err
		}
	}
	return nil
}

func validateEmbeddingPrices(prices []EmbeddingPrice) error {
	seen := map[string]bool{}
	for _, price := range prices {
		if err := validatePriceCommon("embedding", price.Model, price.Currency, price.UpdatedAt, seen); err != nil {
			return err
		}
		if err := validateDecimal("embedding", price.Model, "price_per_1m_tokens", price.Price, true); err != nil {
			return err
		}
	}
	return nil
}

func validatePriceCommon(table, model, currency, updatedAt string, seen map[string]bool) error {
	if model == "" || currency == "" || updatedAt == "" {
		return fmt.Errorf("%s pricing entries require model, currency, and updated_at", table)
	}
	if seen[model] {
		return fmt.Errorf("duplicate %s pricing for model %q", table, model)
	}
	seen[model] = true
	if !datePattern.MatchString(updatedAt) {
		return fmt.Errorf("%s pricing for %s has invalid updated_at %q", table, model, updatedAt)
	}
	if _, err := time.Parse("2006-01-02", updatedAt); err != nil {
		return fmt.Errorf("%s pricing for %s has invalid updated_at %q", table, model, updatedAt)
	}
	return nil
}

func validateDecimal(table, model, field, value string, required bool) error {
	if value == "" && !required {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || strings.HasPrefix(value, "+") {
		return fmt.Errorf("%s pricing for %s has invalid %s %q", table, model, field, value)
	}
	return nil
}

func validateFeatures(features []Feature) error {
	seenIDs := map[string]bool{}
	seenNames := map[string]Feature{}
	for _, feature := range features {
		if !goNamePattern.MatchString(strings.Replace(feature.GoName, "Feature", "Model", 1)) || !strings.HasPrefix(feature.GoName, "Feature") {
			return fmt.Errorf("invalid provider feature go_name %q", feature.GoName)
		}
		if _, ok := seenNames[feature.GoName]; ok {
			return fmt.Errorf("duplicate provider feature go_name %q", feature.GoName)
		}
		if (feature.ID == "") == (feature.AliasOf == "") {
			return fmt.Errorf("feature %s must set exactly one of id or alias_of", feature.GoName)
		}
		if feature.ID != "" && seenIDs[feature.ID] {
			return fmt.Errorf("duplicate provider feature flag %q", feature.ID)
		}
		if feature.ID != "" {
			seenIDs[feature.ID] = true
		}
		seenNames[feature.GoName] = feature
	}
	for _, feature := range features {
		if feature.AliasOf == "" {
			continue
		}
		target, ok := seenNames[feature.AliasOf]
		if !ok || target.AliasOf != "" {
			return fmt.Errorf("feature %s aliases invalid target %s", feature.GoName, feature.AliasOf)
		}
	}
	return nil
}
