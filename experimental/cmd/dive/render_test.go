package main

import (
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/tui"
)

func TestFormatTokenCount(t *testing.T) {
	assert.Equal(t, "56", formatTokenCount(56))
	assert.Equal(t, "1.2k", formatTokenCount(1234))
	assert.Equal(t, "13.5k", formatTokenCount(13500))
	assert.Equal(t, "1.0M", formatTokenCount(1000000))
}

func TestIntroViewUsesCompactThreeLineStartupText(t *testing.T) {
	app := newTestApp()
	app.reasoningEffort = llm.ReasoningEffortMedium
	intro := app.appendIntro()

	screen := tui.SprintScreen(app.introView(app.messages[intro]), tui.WithWidth(80))
	lines := strings.Split(strings.TrimRight(screen.Text(), "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}

	assert.Equal(t, 3, len(lines))
	assert.Equal(t, "Dive v"+cliVersion, lines[0])
	assert.Equal(t, "test-model with medium effort", lines[1])
	assert.Equal(t, "/tmp/test", lines[2])
	assert.NotContains(t, screen.Text(), "█")
	assert.NotContains(t, screen.Text(), "model:")
	assert.NotContains(t, screen.Text(), "directory:")
	assert.NotContains(t, screen.Text(), "╭")
	assert.NotContains(t, screen.Text(), "╰")
}

func TestDiveMarkdownThemeUsesSemanticPalette(t *testing.T) {
	theme := diveMarkdownTheme()

	assertStyleColor := func(name string, style tui.Style, want tui.RGB) {
		t.Helper()
		assert.NotNil(t, style.FgRGB, "%s should use an explicit RGB color", name)
		assert.Equal(t, *style.FgRGB, want, "%s color", name)
	}

	assertStyleColor("h1", theme.H1Style, accentBright)
	assertStyleColor("h3", theme.H3Style, accentMid)
	assertStyleColor("h6", theme.H6Style, dimText)
	assertStyleColor("bold", theme.BoldStyle, primaryText)
	assertStyleColor("inline code", theme.CodeStyle, codeText)
	assertStyleColor("link", theme.LinkStyle, accentBright)
	assertStyleColor("blockquote", theme.BlockQuoteStyle, dimText)
	assertStyleColor("code block", theme.CodeBlockStyle, primaryText)
	assertStyleColor("rule", theme.HorizontalRuleStyle, fadedText)
	assertStyleColor("table border", theme.TableBorderStyle, fadedText)
	assert.Equal(t, theme.SyntaxTheme, "github-dark")
	assert.NotEqual(t, codeText, warningText, "inline literals should not look like warnings")
}

func TestAutocompleteOptionStyleIsReadableActionableText(t *testing.T) {
	style := autocompleteOptionStyle()

	assert.NotNil(t, style.FgRGB)
	assert.Equal(t, mutedText, *style.FgRGB)
	assert.False(t, style.Italic, "file and command choices should not look like explanatory hints")
}

func TestFormatCacheCreationTokens(t *testing.T) {
	assert.Equal(t, "0", formatCacheCreationTokens(&llm.Usage{}), "reported zero should remain zero")
	assert.Equal(t, "2.0k", formatCacheCreationTokens(&llm.Usage{CacheCreationInputTokens: 2000}))
	assert.Equal(t, "—", formatCacheCreationTokens(&llm.Usage{
		CacheCreationInputTokensUnavailable: true,
	}), "an unavailable provider metric must not look like a measured zero")
}

func TestCacheHitRate(t *testing.T) {
	// Healthy: mostly reads.
	rate, ok := cacheHitRate(&llm.Usage{CacheReadInputTokens: 13500, CacheCreationInputTokens: 500})
	assert.True(t, ok, "rate should be defined when caching occurred")
	assert.Equal(t, "96%", rate)

	// Thrash: writes dominate reads (the Heartbeat failure mode).
	rate, ok = cacheHitRate(&llm.Usage{CacheReadInputTokens: 1000, CacheCreationInputTokens: 9000})
	assert.True(t, ok)
	assert.Equal(t, "10%", rate)

	// Reads are measured against the full prompt, not just cache activity.
	rate, ok = cacheHitRate(&llm.Usage{InputTokens: 46469, CacheReadInputTokens: 3923})
	assert.True(t, ok)
	assert.Equal(t, "7%", rate)

	// No caching at all: undefined, rendered as a dash.
	rate, ok = cacheHitRate(&llm.Usage{InputTokens: 1000})
	assert.False(t, ok, "rate should be undefined with no cache activity")
	assert.Equal(t, "—", rate)
}

func TestCacheHitStyle_UsesTotalInputTokens(t *testing.T) {
	style := cacheHitStyle(&llm.Usage{InputTokens: 21, CacheReadInputTokens: 79}, true)
	assert.NotNil(t, style.FgRGB)
	assert.Equal(t, warningText, *style.FgRGB,
		"79% of the full prompt should use the amber style")
}

func TestTokensPanelView_NilWhenNoUsage(t *testing.T) {
	app := newTestApp()
	app.interactionUsage = &llm.Usage{}
	assert.Nil(t, app.tokensPanelView(), "panel should be nil before any tokens are recorded")
}

func TestTokensPanelView_ShowsUnavailableCacheWritesWithoutTokenCounts(t *testing.T) {
	app := newTestApp()
	app.interactionUsage = &llm.Usage{CacheCreationInputTokensUnavailable: true}

	panel := app.tokensPanelView()
	assert.NotNil(t, panel)
	text := tui.Sprint(panel, tui.WithWidth(100))
	assert.True(t, strings.Contains(text, "cache write"))
	assert.True(t, strings.Contains(text, "—"))

	report := app.usageReportView()
	assert.NotNil(t, report)
	reportText := tui.Sprint(report, tui.WithWidth(100))
	assert.True(t, strings.Contains(reportText, "cache write"))
	assert.True(t, strings.Contains(reportText, "—"))
}

func TestTokensPanelView_ShowsCacheReadsWritesAndHitRate(t *testing.T) {
	app := newTestApp()
	app.interactionUsage = &llm.Usage{
		InputTokens:              1200,
		CacheReadInputTokens:     13500,
		CacheCreationInputTokens: 500,
		OutputTokens:             53,
	}
	app.sessionUsage = &llm.Usage{
		InputTokens:              4800,
		CacheReadInputTokens:     54000,
		CacheCreationInputTokens: 1200,
		OutputTokens:             892,
	}

	panel := app.tokensPanelView()
	assert.NotNil(t, panel, "panel should render when usage is present")

	text := tui.Sprint(panel, tui.WithWidth(100))

	// Labels make hits vs misses unambiguous.
	assert.True(t, strings.Contains(text, "cache read"), "should label cache reads")
	assert.True(t, strings.Contains(text, "cache write"), "should label cache writes")
	assert.True(t, strings.Contains(text, "hit"), "should show a hit-rate column")

	// Both scopes are present and differ.
	assert.True(t, strings.Contains(text, "turn"), "should show the turn row")
	assert.True(t, strings.Contains(text, "session"), "should show the session row")

	// The cache-write count (the previously hidden miss signal) is now visible.
	assert.True(t, strings.Contains(text, "500"), "cache write tokens should be shown")
	// And the per-scope hit rate.
	assert.True(t, strings.Contains(text, "88%"), "turn hit rate should be shown")
}

func TestFormatCost(t *testing.T) {
	assert.Equal(t, "$0", formatCost(0))
	assert.Equal(t, "$0.0021", formatCost(0.0021))
	assert.Equal(t, "$0.043", formatCost(0.043))
	assert.Equal(t, "$1.27", formatCost(1.273))
}

func TestCostString(t *testing.T) {
	assert.Equal(t, "—", costString(&llm.Usage{}), "unknown cost should render as a dash")
	assert.Equal(t, "$0", costString(&llm.Usage{Cost: &llm.Cost{Total: 0}}), "known zero cost is $0, not a dash")
	assert.Equal(t, "$1.27", costString(&llm.Usage{Cost: &llm.Cost{Total: 1.273}}))
}

func TestStatusLineShowsOnlySessionCostByDefault(t *testing.T) {
	app := newTestApp()
	app.gitBranch = "main"
	app.reasoningEffort = llm.ReasoningEffortHigh
	app.lastUsage = &llm.Usage{InputTokens: 100}
	app.interactionUsage = &llm.Usage{
		InputTokens: 100,
		Cost:        &llm.Cost{Total: 0.25},
	}
	app.sessionUsage = &llm.Usage{
		InputTokens: 400,
		Cost:        &llm.Cost{Total: 1.273},
	}

	text := tui.SprintScreen(app.statusLineView(), tui.WithWidth(120), tui.WithHeight(10)).Text()
	assert.Contains(t, text, "test-model · high in test on main")
	assert.Contains(t, text, "$1.27")
	assert.NotContains(t, text, "cache read")
	assert.NotContains(t, text, "turn")

	app.showDetailedUsage = true
	text = tui.SprintScreen(app.statusLineView(), tui.WithWidth(120), tui.WithHeight(10)).Text()
	assert.Contains(t, text, "cache read")
	assert.Contains(t, text, "turn")
	assert.Contains(t, text, "session")
}

func TestSessionTotalCostFallsBackToCurrentTurn(t *testing.T) {
	app := newTestApp()
	app.interactionUsage = &llm.Usage{Cost: &llm.Cost{Total: 0.043}}
	assert.Equal(t, "$0.043", app.sessionTotalCostString())

	app.sessionUsage = &llm.Usage{Cost: &llm.Cost{Total: 1.273}}
	assert.Equal(t, "$1.27", app.sessionTotalCostString())
}

func TestTokensPanelView_ShowsCostWhenPresent(t *testing.T) {
	app := newTestApp()
	app.interactionUsage = &llm.Usage{
		InputTokens:  1234,
		OutputTokens: 53,
		Cost:         &llm.Cost{Total: 0.0149, Currency: "USD"},
	}
	text := tui.Sprint(app.tokensPanelView(), tui.WithWidth(100))
	assert.True(t, strings.Contains(text, "cost"), "should show a cost column header")
	assert.True(t, strings.Contains(text, "$0.015"), "should show the formatted turn cost")
}

func TestTokensPanelView_NoCostColumnWhenUnknown(t *testing.T) {
	app := newTestApp()
	app.interactionUsage = &llm.Usage{InputTokens: 1234, OutputTokens: 53} // no Cost
	text := tui.Sprint(app.tokensPanelView(), tui.WithWidth(100))
	assert.False(t, strings.Contains(text, "cost"), "cost column should be hidden when cost is unknown")
}

func TestUsageReportView_NilWhenNoUsage(t *testing.T) {
	app := newTestApp()
	assert.Nil(t, app.usageReportView(), "report should be nil before any tokens are recorded")
}

func TestUsageReportView_SessionOnly(t *testing.T) {
	// When only session usage is present (empty/nil turn), the report must
	// still render the session column rather than hide it behind an empty turn.
	app := newTestApp()
	app.interactionUsage = &llm.Usage{} // empty turn
	app.sessionUsage = &llm.Usage{InputTokens: 4800, OutputTokens: 892}

	view := app.usageReportView()
	assert.NotNil(t, view, "report should render when only session has usage")
	text := tui.Sprint(view, tui.WithWidth(100))
	assert.True(t, strings.Contains(text, "session"), "should show the session column")
	assert.True(t, strings.Contains(text, "4.8k"), "should show session input tokens")
}

func TestUsageReportView_IncludesTotalsAndLegend(t *testing.T) {
	app := newTestApp()
	app.interactionUsage = &llm.Usage{
		InputTokens:              1234,
		CacheReadInputTokens:     13500,
		CacheCreationInputTokens: 500,
		OutputTokens:             53,
	}
	app.sessionUsage = app.interactionUsage.Copy() // same => single column

	view := app.usageReportView()
	assert.NotNil(t, view)

	text := tui.Sprint(view, tui.WithWidth(100))
	assert.True(t, strings.Contains(text, "Token usage"), "should have a heading")
	assert.True(t, strings.Contains(text, "cache write"), "should break out cache writes")
	assert.True(t, strings.Contains(text, "total input"), "should show total input")
	assert.True(t, strings.Contains(text, "cache hit"), "should show the hit rate")
	// total input = 1234 + 13500 + 500 = 15234 -> "15.2k"
	assert.True(t, strings.Contains(text, "15.2k"), "should sum cached + uncached input")
	// Legend explaining the metrics is present.
	assert.True(t, strings.Contains(text, "served from cache"), "should include the legend")
}
