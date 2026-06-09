package main

import (
	"os"
	"strings"
)

// modelInfo describes a known model for display and context window lookup.
type modelInfo struct {
	Pattern       string // substring to match against model ID
	Label         string // human-friendly display name (empty = use raw model ID)
	ContextWindow int    // max context window in tokens
}

// modelCatalog is the shared catalog of known models, checked in order.
// More specific patterns must appear before broader ones.
var modelCatalog = []modelInfo{
	// Anthropic models
	{"claude-fable-5", "Fable 5", 1_000_000},
	{"claude-mythos-5", "Mythos 5", 1_000_000},
	{"claude-opus-4-8", "Opus 4.8", 1_000_000},
	{"claude-opus-4-7", "Opus 4.7", 1_000_000},
	{"claude-opus-4-6", "Opus 4.6", 1_000_000},
	{"claude-opus-4-5", "Opus 4.5", 1_000_000},
	{"claude-opus-4", "", 1_000_000},
	{"claude-sonnet-4-6", "Sonnet 4.6", 1_000_000},
	{"claude-sonnet-4-5", "Sonnet 4.5", 1_000_000},
	{"claude-sonnet-4", "", 1_000_000},
	{"claude-haiku-4-5", "Haiku 4.5", 200_000},
	{"claude-haiku-4", "", 200_000},
	{"claude-3-5-sonnet", "Sonnet 3.5", 200_000},
	{"claude-3-5-haiku", "Haiku 3.5", 200_000},
	{"claude", "", 200_000},

	// Google models
	{"gemini-3.1-pro-preview", "Gemini 3.1 Pro", 1_000_000},
	{"gemini-3.1-pro", "Gemini 3.1 Pro", 1_000_000},
	{"gemini-3.1-flash", "Gemini 3.1 Flash", 1_000_000},
	{"gemini-3.5-flash", "Gemini 3.5 Flash", 1_000_000},
	{"gemini-3-flash-preview", "Gemini 3 Flash", 1_000_000},
	{"gemini-3-flash", "Gemini 3 Flash", 1_000_000},
	{"gemini-3", "", 1_000_000},
	{"gemini-2.5-pro", "Gemini 2.5 Pro", 1_000_000},
	{"gemini-2.5-flash", "Gemini 2.5 Flash", 1_000_000},
	{"gemini-2.5", "", 1_000_000},
	{"gemini", "", 1_000_000},

	// OpenAI models
	{"gpt-5.5", "GPT-5.5", 1_050_000},
	{"gpt-5.4-mini", "GPT-5.4 Mini", 400_000},
	{"gpt-5.4-nano", "GPT-5.4 Nano", 400_000},
	{"gpt-5.4", "GPT-5.4", 1_050_000},
	{"gpt-5.3-codex-spark", "GPT-5.3 Codex Spark", 1_000_000},
	{"gpt-5.3-codex", "GPT-5.3 Codex", 1_000_000},
	{"gpt-5.3", "GPT-5.3", 1_000_000},
	{"gpt-5.2", "GPT-5.2", 1_000_000},
	{"gpt-5.1-mini", "GPT-5.1 Mini", 1_000_000},
	{"gpt-5.1-codex", "GPT-5.1 Codex", 1_000_000},
	{"gpt-5.1", "GPT-5.1", 1_000_000},
	{"gpt-5-mini", "GPT-5 Mini", 1_000_000},
	{"gpt-5", "", 1_000_000},
	{"gpt-4o", "GPT-4o", 128_000},
	{"gpt-4", "", 128_000},
	{"codex-mini", "Codex Mini", 200_000},
	{"o4-mini", "o4-mini", 200_000},
	{"o3-mini", "o3-mini", 200_000},
	{"o3", "o3", 200_000},

	// Grok models
	{"grok-4.3", "Grok 4.3", 1_000_000},
	{"grok-build-0.1", "Grok Build", 256_000},
	{"grok-4.20-0309-reasoning", "Grok 4.20", 1_000_000},
	{"grok-4.20", "Grok 4.20", 1_000_000},
	{"grok-4-1-fast", "Grok 4.1 Fast", 131_072},
	{"grok-4", "Grok 4", 131_072},
	{"grok-3-mini", "Grok 3 Mini", 131_072},
	{"grok-3", "Grok 3", 131_072},
	{"grok-code", "Grok Code", 131_072},
	{"grok", "", 131_072},

	// Mistral models
	{"devstral-small-latest", "Devstral Small", 128_000},
	{"devstral", "", 128_000},
	{"mistral-large-latest", "Mistral Large", 128_000},
	{"mistral-small-latest", "Mistral Small", 128_000},
	{"mistral", "", 128_000},
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

// providerCatalog is the authoritative list of providers and their recommended models.
var providerCatalog = []providerInfo{
	{
		Name:    "Anthropic",
		EnvVars: []string{"ANTHROPIC_API_KEY"},
		Models: []modelChoice{
			{"claude-fable-5", "Fable 5", "Most capable for demanding reasoning and long-horizon work"},
			{"claude-opus-4-8", "Opus 4.8", "Most capable Opus-tier model for complex work"},
			{"claude-sonnet-4-6", "Sonnet 4.6", "Best for everyday tasks"},
			{"claude-haiku-4-5", "Haiku 4.5", "Fastest for quick answers"},
		},
	},
	{
		Name:    "Google",
		EnvVars: []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"},
		Models: []modelChoice{
			{"gemini-3.1-pro-preview", "Gemini 3.1 Pro", "Google's latest flagship model"},
			{"gemini-2.5-pro", "Gemini 2.5 Pro", "Strong all-around model"},
			{"gemini-3.5-flash", "Gemini 3.5 Flash", "Frontier intelligence at high speed"},
		},
	},
	{
		Name:    "OpenAI",
		EnvVars: []string{"OPENAI_API_KEY"},
		Models: []modelChoice{
			{"gpt-5.5", "GPT-5.5", "Flagship model for complex reasoning and coding"},
			{"gpt-5.4", "GPT-5.4", "More affordable model for professional work"},
			{"gpt-5.4-mini", "GPT-5.4 Mini", "Fast mini model for coding and subagents"},
			{"gpt-5.4-nano", "GPT-5.4 Nano", "Lowest-cost GPT-5.4-class model"},
		},
	},
	{
		Name:    "Grok",
		EnvVars: []string{"XAI_API_KEY", "GROK_API_KEY"},
		Models: []modelChoice{
			{"grok-4.3", "Grok 4.3", "xAI's most intelligent and fastest model"},
			{"grok-4.20-0309-reasoning", "Grok 4.20", "Reasoning model with 1M context"},
			{"grok-build-0.1", "Grok Build", "Optimized for coding tasks"},
		},
	},
	{
		Name:    "Mistral",
		EnvVars: []string{"MISTRAL_API_KEY"},
		Models: []modelChoice{
			{"mistral-large-latest", "Mistral Large", "Flagship model"},
			{"mistral-small-latest", "Mistral Small", "Fast and efficient"},
			{"devstral-small-latest", "Devstral Small", "Optimized for coding tasks"},
		},
	},
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
