package openai

import (
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/internal/responsescontrol"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
)

type controlsSpec struct {
	entryPrefix string
	scopes      []modelcaps.VerificationScope
}

var openAIControlsSpecs = map[string]controlsSpec{
	"gpt-5.6":               {entryPrefix: "gpt-5.6", scopes: openAIVerificationScopes},
	"gpt-5.6-sol":           {entryPrefix: "gpt-5.6-sol", scopes: openAIVerificationScopes},
	"gpt-5.6-terra":         {entryPrefix: "gpt-5.6-terra", scopes: openAIVerificationScopes},
	"gpt-5.6-luna":          {entryPrefix: "gpt-5.6-luna", scopes: openAIVerificationScopes},
	"gpt-5.5":               {entryPrefix: "gpt-5.5", scopes: openAIVerificationScopes},
	"gpt-5.4":               {entryPrefix: "gpt-5.4", scopes: openAIVerificationScopes},
	"gpt-5.4-mini":          {entryPrefix: "gpt-5.4-mini", scopes: openAIVerificationScopes},
	"gpt-5.4-nano":          {entryPrefix: "gpt-5.4-nano", scopes: openAIVerificationScopes},
	"gpt-5.3-chat-latest":   {entryPrefix: "gpt-5.3-chat", scopes: openAIVerificationScopes},
	"gpt-5.2":               {entryPrefix: "gpt-5.2", scopes: openAIVerificationScopes},
	"gpt-5.2-pro":           {entryPrefix: "gpt-5.2-pro", scopes: openAIVerificationScopes},
	"gpt-5.1":               {entryPrefix: "gpt-5.1", scopes: openAIVerificationScopes},
	"gpt-5":                 {entryPrefix: "gpt-5", scopes: openAIVerificationScopes},
	"gpt-5-pro":             {entryPrefix: "gpt-5-pro", scopes: openAIVerificationScopes},
	"gpt-5-mini":            {entryPrefix: "gpt-5-mini", scopes: openAIVerificationScopes},
	"gpt-5-nano":            {entryPrefix: "gpt-5-nano", scopes: openAIVerificationScopes},
	"gpt-4.1":               {entryPrefix: "gpt-4.1", scopes: openAIVerificationScopes},
	"gpt-4o":                {entryPrefix: "gpt-4o", scopes: openAIVerificationScopes},
	"o3":                    {entryPrefix: "o3", scopes: openAIVerificationScopes},
	"o3-pro":                {entryPrefix: "o3-pro", scopes: openAIVerificationScopes},
	"o3-mini":               {entryPrefix: "o3-mini", scopes: openAIVerificationScopes},
	"o4-mini":               {entryPrefix: "o4-mini", scopes: openAIVerificationScopes},
	"o3-deep-research":      {entryPrefix: "o3-deep-research"},
	"o4-mini-deep-research": {entryPrefix: "o4-mini-deep-research"},
	"gpt-5.3-codex":         {entryPrefix: "gpt-5.3-codex", scopes: openAIVerificationScopes},
	"gpt-5.2-codex":         {entryPrefix: "gpt-5.2-codex"},
	"gpt-5.1-codex-max":     {entryPrefix: "gpt-5.1-codex-max"},
	"gpt-5.1-codex":         {entryPrefix: "gpt-5.1-codex"},
	"gpt-5-codex":           {entryPrefix: "gpt-5-codex"},
	"gpt-5-codex-mini":      {entryPrefix: "gpt-5-codex-mini"},
	"codex-mini-latest":     {entryPrefix: "codex-mini-latest"},
	"codex-ask":             {entryPrefix: "codex-ask"},
}

var openAIVerificationScopes = []modelcaps.VerificationScope{
	modelcaps.VerificationOpenAIResponses,
}

func init() {
	modelcaps.MustRegister("openai", modelcaps.Resolver{
		Controls: modelControlsFor,
		Preview:  previewModelControls,
	})
}

func modelControlsFor(model string) (modelcaps.ModelControls, bool) {
	canonical := normalizeControlsModel(model, "openai/")
	spec, ok := openAIControlsSpecs[canonical]
	if !ok {
		return modelcaps.ModelControls{}, false
	}
	entry, ok := modelcaps.LookupEntry("openai", canonical)
	if !ok || entry.Prefix != spec.entryPrefix {
		return modelcaps.ModelControls{}, false
	}
	return modelcaps.ModelControls{
		Model:       canonical,
		Temperature: entry.Caps.Temperature,
		Reasoning: modelcaps.ReasoningControls{
			NativeEfforts: entry.Caps.Efforts,
		},
		VerificationScopes: spec.scopes,
	}, true
}

func previewModelControls(config llm.Config) (modelcaps.Plan, bool) {
	controls, ok := modelControlsFor(config.Model)
	if !ok {
		return modelcaps.Plan{}, false
	}
	config.Model = controls.Model
	config.Logger = nil
	return responsescontrol.Plan("openai", config.Model, &config), true
}

func normalizeControlsModel(model, qualifier string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.TrimPrefix(model, qualifier)
}
