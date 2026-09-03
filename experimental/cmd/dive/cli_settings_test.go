package main

import (
	"os"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/compaction"
	"github.com/deepnoodle-ai/dive/experimental/settings"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/cli"
	"github.com/deepnoodle-ai/wonton/tui"
)

func testStrPtr(s string) *string { return &s }

func testBoolPtr(b bool) *bool { return &b }

// unsetTestEnv removes a variable for the test's duration, restoring it after.
// t.Setenv(key, "") is not equivalent: a present-but-empty variable still
// counts as set for bool flags (and for some string flags) in the CLI parser.
func unsetTestEnv(t *testing.T, key string) {
	t.Helper()
	if value, ok := os.LookupEnv(key); ok {
		t.Cleanup(func() { os.Setenv(key, value) })
	} else {
		t.Cleanup(func() { os.Unsetenv(key) })
	}
	os.Unsetenv(key)
}

// captureSettingsCtx builds a CLI context with the real flag definitions and
// captures it for the resolution helpers.
func captureSettingsCtx(t *testing.T, opts ...cli.TestOption) *cli.Context {
	t.Helper()
	app := cli.New("test")
	var captured *cli.Context
	app.Main().
		Flags(
			cli.String("model", "m").
				Env("DIVE_MODEL"),
			cli.Bool("show-thinking").
				Default(false).
				Env("DIVE_SHOW_THINKING"),
			cli.Bool("show-detailed-usage").
				Default(false).
				Env("DIVE_SHOW_DETAILED_USAGE"),
			cli.String("thinking-effort").
				Default(string(llm.ReasoningEffortMedium)).
				Env("DIVE_THINKING_EFFORT"),
		).
		Run(func(ctx *cli.Context) error {
			captured = ctx
			return nil
		})
	result := app.Test(t, opts...)
	assert.True(t, result.Success(), result.Stderr)
	if captured == nil {
		t.Fatal("context was not captured")
	}
	return captured
}

func TestResolveModelNamePrecedence(t *testing.T) {
	t.Run("flag beats env and settings", func(t *testing.T) {
		ctx := captureSettingsCtx(t,
			cli.TestArgs("--model", "flag-model"),
			cli.TestEnv("DIVE_MODEL", "env-model"),
		)
		name, source := resolveModelName(ctx, &settings.Settings{Model: testStrPtr("settings-model")})
		assert.Equal(t, "flag-model", name)
		assert.Equal(t, "flag", source)
	})

	t.Run("env beats settings", func(t *testing.T) {
		ctx := captureSettingsCtx(t, cli.TestEnv("DIVE_MODEL", "env-model"))
		name, source := resolveModelName(ctx, &settings.Settings{Model: testStrPtr("settings-model")})
		assert.Equal(t, "env-model", name)
		assert.Equal(t, "env", source)
	})

	t.Run("settings beats autodetect", func(t *testing.T) {
		unsetTestEnv(t, "DIVE_MODEL")
		ctx := captureSettingsCtx(t)
		name, source := resolveModelName(ctx, &settings.Settings{Model: testStrPtr("settings-model")})
		assert.Equal(t, "settings-model", name)
		assert.Equal(t, "settings", source)
	})
}

func TestResolveThinkingEffortPrecedence(t *testing.T) {
	t.Run("unset defaults to medium", func(t *testing.T) {
		unsetTestEnv(t, "DIVE_THINKING_EFFORT")
		ctx := captureSettingsCtx(t)
		effort, source := resolveThinkingEffort(ctx, nil)
		assert.Equal(t, llm.ReasoningEffortMedium, effort)
		assert.Equal(t, "default", source)
	})

	t.Run("settings apply when flag and env are unset", func(t *testing.T) {
		unsetTestEnv(t, "DIVE_THINKING_EFFORT")
		ctx := captureSettingsCtx(t)
		effort, source := resolveThinkingEffort(ctx,
			&settings.Settings{ThinkingEffort: testStrPtr("high")})
		assert.Equal(t, llm.ReasoningEffortHigh, effort)
		assert.Equal(t, "settings", source)
	})

	t.Run("env beats settings", func(t *testing.T) {
		ctx := captureSettingsCtx(t, cli.TestEnv("DIVE_THINKING_EFFORT", "xhigh"))
		effort, source := resolveThinkingEffort(ctx,
			&settings.Settings{ThinkingEffort: testStrPtr("low")})
		assert.Equal(t, llm.ReasoningEffortXHigh, effort)
		assert.Equal(t, "env", source)
	})

	t.Run("explicit empty flag omits the parameter", func(t *testing.T) {
		ctx := captureSettingsCtx(t, cli.TestArgs("--thinking-effort", ""))
		effort, source := resolveThinkingEffort(ctx,
			&settings.Settings{ThinkingEffort: testStrPtr("high")})
		assert.Equal(t, llm.ReasoningEffort(""), effort)
		assert.Equal(t, "flag", source)
	})
}

func TestResolveShowThinkingPrecedence(t *testing.T) {
	t.Run("defaults to false", func(t *testing.T) {
		unsetTestEnv(t, "DIVE_SHOW_THINKING")
		ctx := captureSettingsCtx(t)
		show, source := resolveShowThinking(ctx, nil)
		assert.False(t, show)
		assert.Equal(t, "default", source)
	})

	t.Run("settings apply when unset", func(t *testing.T) {
		unsetTestEnv(t, "DIVE_SHOW_THINKING")
		ctx := captureSettingsCtx(t)
		show, source := resolveShowThinking(ctx,
			&settings.Settings{ShowThinking: testBoolPtr(true)})
		assert.True(t, show)
		assert.Equal(t, "settings", source)
	})

	t.Run("env beats settings", func(t *testing.T) {
		ctx := captureSettingsCtx(t, cli.TestEnv("DIVE_SHOW_THINKING", "false"))
		show, source := resolveShowThinking(ctx,
			&settings.Settings{ShowThinking: testBoolPtr(true)})
		assert.False(t, show)
		assert.Equal(t, "env", source)
	})
}

func TestResolveShowDetailedUsageDefaultsCompact(t *testing.T) {
	unsetTestEnv(t, "DIVE_SHOW_DETAILED_USAGE")
	ctx := captureSettingsCtx(t)
	show, source := resolveShowDetailedUsage(ctx, nil)
	assert.False(t, show)
	assert.Equal(t, "default", source)

	ctx = captureSettingsCtx(t)
	show, source = resolveShowDetailedUsage(ctx,
		&settings.Settings{ShowDetailedUsage: testBoolPtr(true)})
	assert.True(t, show)
	assert.Equal(t, "settings", source)
}

func TestTrimTrailingWhitespacePerLine(t *testing.T) {
	assert.Equal(t, "a\nb\nc",
		trimTrailingWhitespacePerLine("a   \nb\t\nc"))
	assert.Equal(t, "  indented\nplain",
		trimTrailingWhitespacePerLine("  indented   \nplain"))
}

func TestSetEffortUpdatesLiveModelSettings(t *testing.T) {
	app, _ := newFakeApp(t)
	app.modelSettings = &dive.ModelSettings{}
	app.setEffort(llm.ReasoningEffortHigh, "session")
	assert.Equal(t, llm.ReasoningEffortHigh, app.reasoningEffort)
	assert.Equal(t, llm.ReasoningEffortHigh, app.modelSettings.ReasoningEffort)
	assert.Equal(t, "session", app.effortSource)
}

func TestSetThinkingUpdatesLiveModelSettings(t *testing.T) {
	app, _ := newFakeApp(t)
	app.modelSettings = &dive.ModelSettings{}

	app.setThinking(true, "session")
	assert.True(t, app.showThinking)
	assert.Equal(t, llm.ThinkingTypeAdaptive, app.modelSettings.Thinking)
	assert.Equal(t, llm.ThinkingDisplaySummarized, app.modelSettings.ThinkingDisplay)
	assert.Equal(t, "session", app.thinkingSource)

	app.setThinking(false, "session")
	assert.False(t, app.showThinking)
	assert.Equal(t, llm.ThinkingType(""), app.modelSettings.Thinking)
	assert.Equal(t, llm.ThinkingDisplay(""), app.modelSettings.ThinkingDisplay)
}

func TestDetailedUsageTracksItsEffectiveSource(t *testing.T) {
	app, _ := newFakeApp(t)
	app.setDetailedUsage(true, "settings")
	assert.True(t, app.showDetailedUsage)
	assert.Equal(t, "settings", app.detailedUsageSource)
}

func TestSnapshotCLIModelSettingsDoesNotShareMutableState(t *testing.T) {
	current := &dive.ModelSettings{
		ReasoningEffort: llm.ReasoningEffortHigh,
		Thinking:        llm.ThinkingTypeAdaptive,
	}
	snapshot := snapshotCLIModelSettings(current)
	assert.NotNil(t, snapshot)

	current.ReasoningEffort = llm.ReasoningEffortLow
	current.Thinking = ""
	assert.Equal(t, llm.ReasoningEffortHigh, snapshot.ReasoningEffort)
	assert.Equal(t, llm.ThinkingTypeAdaptive, snapshot.Thinking)
}

func TestCurrentSubagentDefaultsUseLiveParentState(t *testing.T) {
	app, _ := newFakeApp(t)
	initialModel := createModel("mistral-small-latest", "")
	liveModel := createModel("gpt-5.6-sol", "")
	app.agent.SetModel(liveModel)
	app.modelSettings = &dive.ModelSettings{ReasoningEffort: llm.ReasoningEffortXHigh}

	model, modelSettings := currentSubagentDefaults(
		app,
		initialModel,
		&dive.ModelSettings{ReasoningEffort: llm.ReasoningEffortLow},
	)

	assert.True(t, model == liveModel, "new subagents should inherit the switched parent model")
	assert.Equal(t, llm.ReasoningEffortXHigh, modelSettings.ReasoningEffort)
	assert.True(t, modelSettings != app.modelSettings, "subagents should receive a settings snapshot")
}

func TestModelSwitchRefreshesAutomaticCompactionConfiguration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	app, _ := newFakeApp(t)
	app.modelName = "mistral-small-latest"
	app.compactionConfig = &compaction.CompactionConfig{
		ContextTokenThreshold: compactionThreshold(0, app.modelName),
		Model:                 createModel(app.modelName, ""),
	}

	app.switchModel("gpt-5.6-sol")

	assert.Equal(t, compactionThreshold(0, "gpt-5.6-sol"), app.compactionConfig.ContextTokenThreshold)
	assert.Equal(t, app.agent.Model(), app.compactionConfig.Model)
}

func TestModelSwitchPreservesExplicitCompactionThreshold(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	app, _ := newFakeApp(t)
	app.modelName = "mistral-small-latest"
	app.compactionThresholdExplicit = true
	app.compactionConfig = &compaction.CompactionConfig{
		ContextTokenThreshold: 42_000,
		Model:                 createModel(app.modelName, ""),
	}

	app.switchModel("gpt-5.6-sol")

	assert.Equal(t, 42_000, app.compactionConfig.ContextTokenThreshold)
	assert.Equal(t, app.agent.Model(), app.compactionConfig.Model)
}

func TestSelectingActiveModelStillPersistsIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	app, _ := newFakeApp(t)
	app.modelName = "gpt-5.6-sol"
	app.modelSource = "autodetect"

	app.switchModel("gpt-5.6-sol")

	eff, err := settings.LoadEffectiveSettings("")
	assert.NoError(t, err)
	assert.NotNil(t, eff.Model)
	assert.Equal(t, "gpt-5.6-sol", *eff.Model)
	assert.Equal(t, "session", app.modelSource)
}

func TestEffortCommandPersistsToSandboxedHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	app, _ := newFakeApp(t)
	app.modelSettings = &dive.ModelSettings{}

	app.handleEffortCommand("high")
	assert.Equal(t, llm.ReasoningEffortHigh, app.reasoningEffort)

	eff, err := settings.LoadEffectiveSettings("")
	assert.NoError(t, err)
	if eff.ThinkingEffort == nil {
		t.Fatal("expected thinking_effort to be persisted")
	}
	assert.Equal(t, "high", *eff.ThinkingEffort)
}

func TestThinkingCommandPersistsToSandboxedHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	app, _ := newFakeApp(t)
	app.modelSettings = &dive.ModelSettings{}

	app.handleThinkingCommand("on")
	assert.True(t, app.showThinking)

	eff, err := settings.LoadEffectiveSettings("")
	assert.NoError(t, err)
	if eff.ShowThinking == nil {
		t.Fatal("expected show_thinking to be persisted")
	}
	assert.True(t, *eff.ShowThinking)
}

func TestUsageCommandTogglesDetailedPanel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	app, _ := newFakeApp(t)

	app.handleUsageCommand("full")
	assert.True(t, app.showDetailedUsage)
	assert.Equal(t, "session", app.detailedUsageSource)

	app.handleUsageCommand("brief")
	assert.False(t, app.showDetailedUsage)
}

func TestStatusShowsSourcesForEveryPersistedSetting(t *testing.T) {
	app, _ := newFakeApp(t)
	app.modelSource = "flag"
	app.effortSource = "settings"
	app.thinkingSource = "env"
	app.detailedUsageSource = "session"

	app.handleStatusCommand()
	view := app.messages[len(app.messages)-1].View
	text := tui.Sprint(view, tui.WithWidth(100))
	assert.Contains(t, text, "model:           test-model (from flag)")
	assert.Contains(t, text, "thinking effort: unset (parameter omitted) (from settings)")
	assert.Contains(t, text, "thinking display: off (from env)")
	assert.Contains(t, text, "detailed usage:   off (from session)")
}
