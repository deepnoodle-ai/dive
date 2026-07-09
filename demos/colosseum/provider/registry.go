// Package provider maps a short, human-friendly key ("claude", "gpt",
// "gemini", "grok") to a Dive llm.LLM, with cost-aware default model tiers.
//
// This is the ONLY package in the demo that imports concrete provider packages.
// The arena and engine depend on the abstract llm.LLM interface, so adding a new
// contestant is a one-line entry here — the rest of the program never changes.
// That single-interface-for-every-provider property is precisely what The
// Colosseum exists to show off.
package provider

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/providers/google"
	"github.com/deepnoodle-ai/dive/providers/grok"
	"github.com/deepnoodle-ai/dive/providers/openai"
)

// Spec describes one contestant provider: how to build its model and which
// model tiers to use by default.
type Spec struct {
	Key     string   // canonical key, e.g. "claude"
	Aliases []string // alternate names accepted on the command line
	EnvKeys []string // API-key env vars; any one being set enables the provider
	Cheap   string   // default model for bulk / leaderboard runs (cost-aware)
	Premium string   // model for showcase matches (--premium)

	newLLM func(model string) llm.LLM
}

// New constructs an llm.LLM for this provider using the given model id.
func (s *Spec) New(model string) llm.LLM { return s.newLLM(model) }

// registry is the canonical contestant table. The default model tiers favour
// cheap variants so a full leaderboard run does not run up a large bill; pass
// --premium (or an explicit --model override) for showcase matches.
var registry = []*Spec{
	{
		Key:     "claude",
		Aliases: []string{"anthropic"},
		EnvKeys: []string{"ANTHROPIC_API_KEY"},
		Cheap:   anthropic.ModelClaudeHaiku45,
		Premium: anthropic.ModelClaudeOpus48,
		newLLM:  func(m string) llm.LLM { return anthropic.New(anthropic.WithModel(m)) },
	},
	{
		Key:     "gpt",
		Aliases: []string{"openai", "chatgpt", "gpt5"},
		EnvKeys: []string{"OPENAI_API_KEY"},
		Cheap:   openai.ModelGPT54Mini,
		Premium: openai.ModelGPT54,
		newLLM:  func(m string) llm.LLM { return openai.New(openai.WithModel(m)) },
	},
	{
		Key:     "gemini",
		Aliases: []string{"google"},
		EnvKeys: []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"},
		Cheap:   google.ModelGemini25Flash,
		Premium: google.ModelGemini25Pro,
		newLLM:  func(m string) llm.LLM { return google.New(google.WithModel(m)) },
	},
	{
		Key:     "grok",
		Aliases: []string{"xai"},
		EnvKeys: []string{"XAI_API_KEY", "GROK_API_KEY"},
		Cheap:   grok.ModelGrok41FastNonReasoning,
		Premium: grok.ModelGrok45,
		newLLM:  func(m string) llm.LLM { return grok.New(grok.WithModel(m)) },
	},
}

// Resolve looks up a provider by canonical key or alias (case-insensitive).
func Resolve(name string) (*Spec, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, s := range registry {
		if s.Key == name {
			return s, true
		}
		for _, a := range s.Aliases {
			if a == name {
				return s, true
			}
		}
	}
	return nil, false
}

// Keys returns the canonical provider keys, sorted, for help text and errors.
func Keys() []string {
	keys := make([]string, 0, len(registry))
	for _, s := range registry {
		keys = append(keys, s.Key)
	}
	sort.Strings(keys)
	return keys
}

// ModelFor selects the model id for a provider given the run options:
// an explicit override wins, otherwise the premium or cheap default tier.
func (s *Spec) ModelFor(override string, premium bool) string {
	switch {
	case override != "":
		return override
	case premium:
		return s.Premium
	default:
		return s.Cheap
	}
}

// ErrUnknownProvider is returned by Resolve callers when a name is not in the
// registry; provided as a helper for clear CLI errors.
func ErrUnknownProvider(name string) error {
	return fmt.Errorf("unknown provider %q (known: %s)", name, strings.Join(Keys(), ", "))
}

// EnvSatisfied reports whether at least one acceptable API-key env var is set
// for this provider, returning the accepted names for use in error messages.
func (s *Spec) EnvSatisfied() (bool, []string) {
	for _, k := range s.EnvKeys {
		if os.Getenv(k) != "" {
			return true, s.EnvKeys
		}
	}
	return false, s.EnvKeys
}
