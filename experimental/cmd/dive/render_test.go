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

func TestCacheHitRate(t *testing.T) {
	// Healthy: mostly reads.
	rate, ok := cacheHitRate(&llm.Usage{CacheReadInputTokens: 13500, CacheCreationInputTokens: 500})
	assert.True(t, ok, "rate should be defined when caching occurred")
	assert.Equal(t, "96%", rate)

	// Thrash: writes dominate reads (the Heartbeat failure mode).
	rate, ok = cacheHitRate(&llm.Usage{CacheReadInputTokens: 1000, CacheCreationInputTokens: 9000})
	assert.True(t, ok)
	assert.Equal(t, "10%", rate)

	// No caching at all: undefined, rendered as a dash.
	rate, ok = cacheHitRate(&llm.Usage{InputTokens: 1000})
	assert.False(t, ok, "rate should be undefined with no cache activity")
	assert.Equal(t, "—", rate)
}

func TestTokensPanelView_NilWhenNoUsage(t *testing.T) {
	app := newTestApp()
	app.interactionUsage = &llm.Usage{}
	assert.Nil(t, app.tokensPanelView(), "panel should be nil before any tokens are recorded")
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
	assert.True(t, strings.Contains(text, "96%"), "turn hit rate should be shown")
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
