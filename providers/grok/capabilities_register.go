package grok

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

var grokVerificationScopes = []modelcaps.VerificationScope{
	modelcaps.VerificationXAIResponses,
}

var grokControlsSpecs = map[string]controlsSpec{
	"grok-4.6":                     {entryPrefix: "grok-4.6", scopes: grokVerificationScopes},
	"grok-4.5":                     {entryPrefix: "grok-4.5", scopes: grokVerificationScopes},
	"grok-4.3":                     {entryPrefix: "grok-4.3", scopes: grokVerificationScopes},
	"grok-4.20-0309-reasoning":     {entryPrefix: "grok-4.20-0309-reasoning", scopes: grokVerificationScopes},
	"grok-4.20-0309-non-reasoning": {entryPrefix: "grok-4.20-0309-non-reasoning", scopes: grokVerificationScopes},
	"grok-4.20-multi-agent-0309":   {entryPrefix: "grok-4.20-multi-agent", scopes: grokVerificationScopes},
	"grok-build-0.1":               {entryPrefix: "grok-build", scopes: grokVerificationScopes},
	"grok-4-1-fast-reasoning":      {entryPrefix: "grok-4", scopes: grokVerificationScopes},
	"grok-4-1-fast-non-reasoning":  {entryPrefix: "grok-4", scopes: grokVerificationScopes},
	"grok-4-fast-reasoning":        {entryPrefix: "grok-4", scopes: grokVerificationScopes},
	"grok-4-fast-non-reasoning":    {entryPrefix: "grok-4", scopes: grokVerificationScopes},
	"grok-4-0709":                  {entryPrefix: "grok-4", scopes: grokVerificationScopes},
	"grok-4":                       {entryPrefix: "grok-4", scopes: grokVerificationScopes},
	"grok-4-latest":                {entryPrefix: "grok-4", scopes: grokVerificationScopes},
	"grok-3":                       {entryPrefix: "grok-3", scopes: grokVerificationScopes},
	"grok-3-latest":                {entryPrefix: "grok-3", scopes: grokVerificationScopes},
	"grok-3-mini":                  {entryPrefix: "grok-3", scopes: grokVerificationScopes},
	"grok-code-fast-1":             {entryPrefix: "grok-code-fast", scopes: grokVerificationScopes},
}

func init() {
	modelcaps.MustRegister("grok", modelcaps.Resolver{
		Controls: modelControlsFor,
		Preview:  previewModelControls,
	})
}

func modelControlsFor(model string) (modelcaps.ModelControls, bool) {
	canonical := normalizeControlsModel(model)
	spec, ok := grokControlsSpecs[canonical]
	if !ok {
		return modelcaps.ModelControls{}, false
	}
	entry, ok := modelcaps.LookupEntry("grok", canonical)
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
	return responsescontrol.Plan("grok", config.Model, &config), true
}

func normalizeControlsModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.TrimPrefix(model, "x-ai/")
}
