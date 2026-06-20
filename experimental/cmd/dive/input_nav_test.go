package main

import (
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/tui"
)

// These tests lock in the contract the main input's OnKey hook relies on:
// since wonton >= v0.0.35 a focused InputField owns its keystrokes, Dive routes
// autocomplete navigation and history recall through handleInputNavKey, which
// must report whether it consumed the key (true) or the input should handle it
// itself (false, e.g. moving the cursor).

func newTestApp() *App {
	return NewApp(&dive.Agent{}, nil, "/tmp/test", "test-model", "", nil, "", nil, "")
}

func TestHandleInputNavKey_HistoryRecall(t *testing.T) {
	a := newTestApp()
	a.history = []string{"first", "second"}

	// Up recalls the newest entry, then older ones.
	assert.True(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowUp}), "Up should be consumed for history")
	assert.Equal(t, "second", a.inputText)
	assert.True(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowUp}))
	assert.Equal(t, "first", a.inputText)

	// Down walks forward and clears past the newest entry.
	assert.True(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowDown}))
	assert.Equal(t, "second", a.inputText)
	assert.True(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowDown}))
	assert.Equal(t, "", a.inputText)
	assert.Equal(t, -1, a.historyIndex)
}

func TestHandleInputNavKey_PassthroughWhenNoHistory(t *testing.T) {
	a := newTestApp()
	// No history and no autocomplete: the key is not consumed, so the focused
	// input gets it (e.g. to move the cursor within a multiline draft).
	assert.False(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowUp}))
	assert.False(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowDown}))
	assert.False(t, a.handleInputNavKey(tui.KeyEvent{Rune: 'x'}))
}

func TestHandleInputNavKey_HistoryNotRecalledWhileProcessing(t *testing.T) {
	a := newTestApp()
	a.history = []string{"first"}
	a.processing = true
	// While a turn is in flight, Up is left to the input rather than recalling.
	assert.False(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowUp}))
	assert.Equal(t, "", a.inputText)
}

func TestHandleInputNavKey_AutocompleteNavigation(t *testing.T) {
	a := newTestApp()
	a.autocompleteMatches = []string{"a", "b", "c"}
	a.autocompleteIndex = 0

	assert.True(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowDown}))
	assert.Equal(t, 1, a.autocompleteIndex)
	assert.True(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowDown}))
	assert.Equal(t, 2, a.autocompleteIndex)
	// Already at the last match: consumed but index stays in range.
	assert.True(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowDown}))
	assert.Equal(t, 2, a.autocompleteIndex)
	assert.True(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowUp}))
	assert.Equal(t, 1, a.autocompleteIndex)
}

func TestHandleInputNavKey_AutocompleteTabSelects(t *testing.T) {
	a := newTestApp()
	a.inputText = "/he"
	a.autocompleteType = "command"
	a.autocompleteMatches = []string{"help"}
	a.autocompleteIndex = 0

	assert.True(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyTab}))
	assert.Equal(t, "/help ", a.inputText)
	assert.Equal(t, 0, len(a.autocompleteMatches), "selecting clears autocomplete")
}

func TestHandleInputNavKey_AutocompleteTakesPrecedenceOverHistory(t *testing.T) {
	a := newTestApp()
	a.history = []string{"old"}
	a.autocompleteMatches = []string{"a", "b"}
	a.autocompleteIndex = 0

	// Down moves the autocomplete cursor, not history, while suggestions show.
	assert.True(t, a.handleInputNavKey(tui.KeyEvent{Key: tui.KeyArrowDown}))
	assert.Equal(t, 1, a.autocompleteIndex)
	assert.Equal(t, "", a.inputText)
}
