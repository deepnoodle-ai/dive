package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/settings"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/cli"
)

// CLI settings resolution: flag > env > settings file > built-in default.
// The settings file is ~/.dive/settings.json merged with the project tiers
// (.dive/settings.json, .dive/settings.local.json); see the settings package.

func loadEffectiveCLISettings(workspaceDir string) *settings.Settings {
	eff, err := settings.LoadEffectiveSettings(workspaceDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load settings: %v\n", err)
		return &settings.Settings{}
	}
	return eff
}

// resolveModelName picks the model with flag > env > settings > autodetect.
//
// wonton's IsSet covers env-supplied values too, so an explicit flag is only
// distinguishable when its value differs from the environment's. When they
// coincide the label says "env" — the value is the same either way.
func resolveModelName(ctx *cli.Context, eff *settings.Settings) (name, source string) {
	if ctx.IsSet("model") {
		if name := strings.TrimSpace(ctx.String("model")); name != "" {
			if env, ok := os.LookupEnv("DIVE_MODEL"); ok && strings.TrimSpace(env) == name {
				return name, "env"
			}
			return name, "flag"
		}
	}
	if env, ok := os.LookupEnv("DIVE_MODEL"); ok && strings.TrimSpace(env) != "" {
		return strings.TrimSpace(env), "env"
	}
	if eff != nil && eff.Model != nil && strings.TrimSpace(*eff.Model) != "" {
		return strings.TrimSpace(*eff.Model), "settings"
	}
	return getDefaultModel(), "autodetect"
}

// resolveThinkingEffort picks the effort with flag > env > settings > medium.
// An explicitly empty value omits the parameter entirely (for non-reasoning
// models).
func resolveThinkingEffort(ctx *cli.Context, eff *settings.Settings) (llm.ReasoningEffort, string) {
	if ctx.IsSet("thinking-effort") {
		value := ctx.String("thinking-effort")
		if env, ok := os.LookupEnv("DIVE_THINKING_EFFORT"); ok && env == value {
			return parseThinkingEffort(value), "env"
		}
		return parseThinkingEffort(value), "flag"
	}
	if env, ok := os.LookupEnv("DIVE_THINKING_EFFORT"); ok {
		// Explicit empty env omits the parameter, same as the flag.
		if strings.TrimSpace(env) == "" {
			return "", "env"
		}
		return parseThinkingEffort(env), "env"
	}
	if eff != nil && eff.ThinkingEffort != nil {
		if strings.TrimSpace(*eff.ThinkingEffort) == "" {
			return "", "settings"
		}
		return parseThinkingEffort(*eff.ThinkingEffort), "settings"
	}
	return llm.ReasoningEffortMedium, "default"
}

// resolveShowThinking picks the thinking display with flag > env > settings.
func resolveShowThinking(ctx *cli.Context, eff *settings.Settings) (bool, string) {
	if ctx.IsSet("show-thinking") {
		value := ctx.Bool("show-thinking")
		if env, ok := os.LookupEnv("DIVE_SHOW_THINKING"); ok {
			if b, err := strconv.ParseBool(strings.TrimSpace(env)); err == nil && b == value {
				return value, "env"
			}
		}
		return value, "flag"
	}
	if env, ok := os.LookupEnv("DIVE_SHOW_THINKING"); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(env)); err == nil {
			return b, "env"
		}
	}
	if eff != nil && eff.ShowThinking != nil {
		return *eff.ShowThinking, "settings"
	}
	return false, "default"
}

// resolveShowDetailedUsage picks the usage-panel mode with flag > env > settings.
// False (default) shows only the session total cost on the status line; true
// renders the full token breakdown table under it.
func resolveShowDetailedUsage(ctx *cli.Context, eff *settings.Settings) (bool, string) {
	if ctx != nil && ctx.IsSet("show-detailed-usage") {
		value := ctx.Bool("show-detailed-usage")
		if env, ok := os.LookupEnv("DIVE_SHOW_DETAILED_USAGE"); ok {
			if b, err := strconv.ParseBool(strings.TrimSpace(env)); err == nil && b == value {
				return value, "env"
			}
		}
		return value, "flag"
	}
	if env, ok := os.LookupEnv("DIVE_SHOW_DETAILED_USAGE"); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(env)); err == nil {
			return b, "env"
		}
	}
	if eff != nil && eff.ShowDetailedUsage != nil {
		return *eff.ShowDetailedUsage, "settings"
	}
	return false, "default"
}

// buildCLIModelSettings constructs ModelSettings from already-resolved values.
func buildCLIModelSettings(maxTokens int, tempSet bool, temp float64, showThinking bool, effort llm.ReasoningEffort) (*dive.ModelSettings, bool) {
	modelSettings := &dive.ModelSettings{
		MaxTokens: &maxTokens,
	}
	if showThinking {
		modelSettings.Thinking = llm.ThinkingTypeAdaptive
		modelSettings.ThinkingDisplay = llm.ThinkingDisplaySummarized
	}
	if effort != "" {
		modelSettings.ReasoningEffort = effort
	}
	if tempSet {
		t := temp
		modelSettings.Temperature = &t
	}
	return modelSettings, showThinking
}

// snapshotCLIModelSettings returns a point-in-time copy for a newly spawned
// subagent. Slash commands mutate the main agent's settings pointer in place;
// sharing it would also change subagents that are already running.
func snapshotCLIModelSettings(current *dive.ModelSettings) *dive.ModelSettings {
	if current == nil {
		return nil
	}
	snapshot := *current
	return &snapshot
}

func currentSubagentDefaults(app *App, initialModel llm.LLM, initialSettings *dive.ModelSettings) (llm.LLM, *dive.ModelSettings) {
	model := initialModel
	modelSettings := snapshotCLIModelSettings(initialSettings)
	if app != nil {
		model = app.agent.Model()
		modelSettings = snapshotCLIModelSettings(app.modelSettings)
	}
	return model, modelSettings
}

// newCLIModelSettingsWith resolves settings-file defaults underneath the
// flag/env values newCLIModelSettings already handles. A nil eff skips the
// settings tier (used by tests).
func newCLIModelSettingsWith(ctx *cli.Context, eff *settings.Settings) (*dive.ModelSettings, bool) {
	effort, _ := resolveThinkingEffort(ctx, eff)
	showThinking, _ := resolveShowThinking(ctx, eff)
	maxTokens := ctx.Int("max-tokens")
	tempSet := ctx.IsSet("temperature")
	var temp float64
	if tempSet {
		temp = ctx.Float64("temperature")
	}
	return buildCLIModelSettings(maxTokens, tempSet, temp, showThinking, effort)
}

func settingsStringPtr(s string) *string { return &s }

func settingsBoolPtr(b bool) *bool { return &b }

// saveUserModel persists the model default. Warns rather than failing the switch.
func saveUserModel(model string) error {
	return settings.SaveUserSettings(&settings.Settings{Model: settingsStringPtr(model)})
}

func saveUserEffort(effort llm.ReasoningEffort) error {
	return settings.SaveUserSettings(&settings.Settings{ThinkingEffort: settingsStringPtr(string(effort))})
}

func saveUserThinking(show bool) error {
	return settings.SaveUserSettings(&settings.Settings{ShowThinking: settingsBoolPtr(show)})
}

func saveUserDetailedUsage(show bool) error {
	return settings.SaveUserSettings(&settings.Settings{ShowDetailedUsage: settingsBoolPtr(show)})
}
