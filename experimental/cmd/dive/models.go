package main

import (
	"os"
	"sort"
	"strings"

	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/providers/google"
	"github.com/deepnoodle-ai/dive/providers/grok"
	"github.com/deepnoodle-ai/dive/providers/mistral"
	"github.com/deepnoodle-ai/dive/providers/modelcatalog"
	"github.com/deepnoodle-ai/dive/providers/ollama"
	"github.com/deepnoodle-ai/dive/providers/openai"
	"github.com/deepnoodle-ai/dive/providers/openrouter"
)

// modelInfo describes a known model for display and context window lookup.
type modelInfo struct {
	Pattern       string // substring to match against model ID
	Label         string // human-friendly display name (empty = use raw model ID)
	ContextWindow int    // max context window in tokens
}

var embeddedProviderCatalogs = []modelcatalog.Catalog{
	anthropic.Catalog(),
	google.Catalog(),
	openai.Catalog(),
	grok.Catalog(),
	mistral.Catalog(),
	openrouter.Catalog(),
	ollama.Catalog(),
}

// modelCatalog is built from embedded model metadata alone; a model the
// catalogs do not list is unknown, and the UI hides the context bar for it.
// Longer IDs are checked first so a dated variant beats its family prefix.
var modelCatalog = buildModelCatalog(embeddedProviderCatalogs)

func buildModelCatalog(catalogs []modelcatalog.Catalog) []modelInfo {
	models := make([]modelInfo, 0)
	seen := map[string]bool{}
	for _, catalog := range catalogs {
		for _, model := range catalog.Models {
			if model.ID == "" || model.ContextWindow == 0 || seen[model.ID] {
				continue
			}
			seen[model.ID] = true
			models = append(models, modelInfo{
				Pattern:       model.ID,
				Label:         model.DisplayName,
				ContextWindow: model.ContextWindow,
			})
		}
	}
	sort.SliceStable(models, func(i, j int) bool {
		return len(models[i].Pattern) > len(models[j].Pattern)
	})
	return models
}

// lookupModel finds the first matching catalog entry for a model ID.
func lookupModel(model string) *modelInfo {
	for i := range modelCatalog {
		if strings.Contains(model, modelCatalog[i].Pattern) {
			return &modelCatalog[i]
		}
	}
	return nil
}

// contextWindowForModel returns the max context window size (in tokens) for known models.
// Returns 0 for unknown models; the UI hides the context bar when 0.
func contextWindowForModel(model string) int {
	if info := lookupModel(model); info != nil {
		return info.ContextWindow
	}
	return 0
}

// providerInfo describes a provider and its available models for the CLI.
type providerInfo struct {
	Name    string        // display name (e.g. "Anthropic")
	EnvVars []string      // environment variables that enable this provider
	Models  []modelChoice // selectable models
}

// Available returns true if any of the provider's environment variables are set.
func (p providerInfo) Available() bool {
	for _, env := range p.EnvVars {
		if os.Getenv(env) != "" {
			return true
		}
	}
	return false
}

// modelChoice represents a selectable model in the /model dialog and models list.
type modelChoice struct {
	ModelID     string // model ID passed to createModel (e.g. "claude-opus-4-6")
	Label       string // display name (e.g. "Opus 4.6")
	Description string // short description
}

func providerInfoFromCatalog(
	name string,
	envVars []string,
	catalog modelcatalog.Catalog,
) providerInfo {
	models := catalog.RecommendedModels()
	choices := make([]modelChoice, 0, len(models))
	for _, model := range models {
		choices = append(choices, modelChoice{
			ModelID:     model.ID,
			Label:       model.DisplayName,
			Description: model.Description,
		})
	}
	return providerInfo{Name: name, EnvVars: envVars, Models: choices}
}

// providerCatalog is derived from each provider's embedded recommended models.
var providerCatalog = []providerInfo{
	providerInfoFromCatalog("Anthropic", []string{"ANTHROPIC_API_KEY"}, anthropic.Catalog()),
	providerInfoFromCatalog("Google", []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"}, google.Catalog()),
	providerInfoFromCatalog("OpenAI", []string{"OPENAI_API_KEY"}, openai.Catalog()),
	providerInfoFromCatalog("Grok", []string{"XAI_API_KEY", "GROK_API_KEY"}, grok.Catalog()),
	providerInfoFromCatalog("Mistral", []string{"MISTRAL_API_KEY"}, mistral.Catalog()),
}

// availableModelChoices returns model choices that the user can actually use,
// based on which API keys are set.
func availableModelChoices() []modelChoice {
	var choices []modelChoice
	for _, p := range providerCatalog {
		if p.Available() {
			choices = append(choices, p.Models...)
		}
	}
	return choices
}
