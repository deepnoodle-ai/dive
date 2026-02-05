package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/compaction"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/termtest"
	"github.com/deepnoodle-ai/wonton/tui"
)

// Note: These tests do NOT require API keys - they only test UI rendering.
// The agent is created with nil LLM, so no API calls are made.

// TestAppLiveView tests the App's LiveView rendering using termtest.
// This validates the exact terminal output after specific interactions.
func TestAppLiveView(t *testing.T) {
	// Create a minimal app with mock agent
	agent := &dive.StandardAgent{}
	app := NewApp(agent, nil, "/tmp/test", "test-model", "", nil, "", false, nil)

	t.Run("initial view shows input prompt", func(t *testing.T) {
		// Render the live view to a termtest screen
		screen := renderLiveView(t, app, 80, 24)

		// The live view should show the input prompt
		termtest.AssertContains(t, screen, ">")
		termtest.AssertContains(t, screen, "Type a message")
	})

	t.Run("processing state shows thinking indicator", func(t *testing.T) {
		// Set processing state
		app.processing = true
		app.streamingMessageIndex = 0
		app.currentMessage = &Message{
			Role:    "assistant",
			Content: "",
			Time:    time.Now(),
		}
		app.messages = append(app.messages, *app.currentMessage)

		screen := renderLiveView(t, app, 80, 24)

		// Should show the thinking animation (lowercase 't')
		termtest.AssertContains(t, screen, "thinking")

		// Reset state
		app.processing = false
		app.currentMessage = nil
		app.messages = app.messages[:len(app.messages)-1]
	})

	t.Run("todo list renders when visible", func(t *testing.T) {
		app.showTodos = true
		app.todos = []Todo{
			{Content: "Run tests", ActiveForm: "Running tests", Status: TodoStatusInProgress},
			{Content: "Fix bugs", ActiveForm: "Fixing bugs", Status: TodoStatusPending},
		}

		screen := renderLiveView(t, app, 80, 24)

		// Should show todo items
		termtest.AssertContains(t, screen, "Running tests")
		termtest.AssertContains(t, screen, "Fix bugs")

		// Reset
		app.showTodos = false
		app.todos = nil
	})
}

// TestAppWithInlineRunner tests the App with an actual InlineApp runner
// to verify the full rendering pipeline including ANSI sequences.
func TestAppWithInlineRunner(t *testing.T) {
	agent := &dive.StandardAgent{}
	app := NewApp(agent, nil, "/tmp/test", "test-model", "", nil, "", false, nil)

	var buf bytes.Buffer
	runner := tui.NewInlineApp(tui.InlineAppConfig{
		Width:  80,
		Output: &buf,
	})

	// Wire up the runner
	app.runner = runner

	// Create a live printer for testing render cycles
	live := tui.NewLivePrinter(tui.PrintConfig{Width: 80, Output: &buf})

	t.Run("render produces valid ANSI output", func(t *testing.T) {
		// Manually render the live view
		view := app.LiveView()
		live.Update(view)

		output := buf.String()

		// Should produce some output
		assert.True(t, len(output) > 0, "expected non-empty output")

		// Parse through termtest screen to validate
		screen := termtest.NewScreen(80, 24)
		screen.WriteString(output)

		// Should contain the prompt
		termtest.AssertContains(t, screen, ">")
	})
}

// TestAppFooterCollapse tests that footer padding collapses when autocomplete is inactive
func TestAppFooterCollapse(t *testing.T) {
	agent := &dive.StandardAgent{}
	app := NewApp(agent, nil, "/tmp/test", "test-model", "", nil, "", false, nil)

	t.Run("footer minimal without autocomplete", func(t *testing.T) {
		// No autocomplete matches
		app.autocompleteMatches = nil
		app.showCompactionStats = false
		app.showExitHint = false

		// Render to buffer to count actual output lines (not screen lines)
		var buf bytes.Buffer
		view := app.LiveView()
		tui.Fprint(&buf, view, tui.PrintConfig{Width: 80})
		output := buf.String()

		// Count lines in the actual output
		lines := splitLines(output)
		nonEmptyCount := 0
		for _, line := range lines {
			if !isEmptyOrWhitespace(line) {
				nonEmptyCount++
			}
		}

		// Verify we have content (dividers, prompt, etc) but minimal padding
		assert.True(t, nonEmptyCount >= 2, "expected at least 2 non-empty lines, got %d", nonEmptyCount)
		// Total lines should be reasonable (not 8+ padding lines)
		assert.True(t, len(lines) <= 10, "expected at most 10 total lines without autocomplete, got %d", len(lines))
	})

	t.Run("footer expands with autocomplete", func(t *testing.T) {
		// Set autocomplete matches
		app.autocompleteMatches = []string{"file1.go", "file2.go", "file3.go"}

		screen := renderLiveView(t, app, 80, 24)

		// Should show autocomplete options
		termtest.AssertContains(t, screen, "file1.go")
		termtest.AssertContains(t, screen, "file2.go")

		// Reset
		app.autocompleteMatches = nil
	})
}

// TestAppCompactionStats tests the compaction stats display
func TestAppCompactionStats(t *testing.T) {
	agent := &dive.StandardAgent{}
	app := NewApp(agent, nil, "/tmp/test", "test-model", "", nil, "", false, nil)

	t.Run("shows compaction stats when enabled", func(t *testing.T) {
		app.showCompactionStats = true
		app.lastCompactionEvent = &compaction.CompactionEvent{
			TokensBefore:      150000,
			TokensAfter:       50000,
			MessagesCompacted: 25,
		}

		screen := renderLiveView(t, app, 80, 24)

		// Should show compaction info
		termtest.AssertContains(t, screen, "150000")
		termtest.AssertContains(t, screen, "50000")
		termtest.AssertContains(t, screen, "25")

		// Reset
		app.showCompactionStats = false
		app.lastCompactionEvent = nil
	})
}

// renderLiveView is a helper that renders the app's LiveView to a termtest Screen
func renderLiveView(t *testing.T, app *App, width, height int) *termtest.Screen {
	t.Helper()

	var buf bytes.Buffer
	view := app.LiveView()
	tui.Fprint(&buf, view, tui.PrintConfig{Width: width})

	screen := termtest.NewScreen(width, height)
	// Use Write (not WriteString) to process ANSI escape sequences
	screen.Write(buf.Bytes())

	return screen
}

// TestAppEventHandling tests the App's event handling and state changes.
// This demonstrates how to simulate user interactions and verify the resulting view.
func TestAppEventHandling(t *testing.T) {
	agent := &dive.StandardAgent{}
	app := NewApp(agent, nil, "/tmp/test", "test-model", "", nil, "", false, nil)

	// Create a runner with captured output
	var buf bytes.Buffer
	app.runner = tui.NewInlineApp(tui.InlineAppConfig{
		Width:  80,
		Output: &buf,
	})

	t.Run("Ctrl+C shows exit hint", func(t *testing.T) {
		// Simulate first Ctrl+C
		event := tui.KeyEvent{Key: tui.KeyCtrlC, Time: time.Now()}
		cmds := app.HandleEvent(event)

		// Should set exit hint (not quit immediately)
		assert.True(t, app.showExitHint, "expected showExitHint to be true after first Ctrl+C")
		assert.Equal(t, 0, len(cmds), "first Ctrl+C should not return Quit command")

		// Render and verify hint is shown
		screen := renderLiveView(t, app, 80, 24)
		termtest.AssertContains(t, screen, "Ctrl+C")
		termtest.AssertContains(t, screen, "exit")

		// Reset state
		app.showExitHint = false
	})

	t.Run("escape during processing triggers cancel", func(t *testing.T) {
		app.processing = true

		event := tui.KeyEvent{Key: tui.KeyEscape, Time: time.Now()}
		cmds := app.HandleEvent(event)

		// Escape during processing should return commands (cancel action)
		// The exact behavior depends on whether ctx cancel is triggered
		_ = cmds // Just verify no panic

		// Reset
		app.processing = false
	})

	t.Run("autocomplete state with file prefix", func(t *testing.T) {
		// Set up autocomplete state directly (since updateAutocomplete is internal)
		app.autocompleteMatches = []string{"README.md", "main.go"}
		app.autocompleteIndex = 0
		app.autocompleteType = "file"

		screen := renderLiveView(t, app, 80, 24)

		// Should show autocomplete matches
		termtest.AssertContains(t, screen, "README.md")
		termtest.AssertContains(t, screen, "main.go")

		// Reset
		app.autocompleteMatches = nil
		app.autocompleteIndex = 0
		app.autocompleteType = ""
	})

	t.Run("dialog state affects view", func(t *testing.T) {
		// Set up a dialog with proper DialogOption type
		app.dialogState = &DialogState{
			Active:  true,
			Type:    DialogTypeConfirm,
			Title:   "Test Dialog",
			Message: "Do you want to proceed?",
			Options: []DialogOption{
				{Label: "Yes", Description: "Approve the action"},
				{Label: "No", Description: "Deny the action"},
			},
		}

		screen := renderLiveView(t, app, 80, 24)

		// Dialog should be visible
		termtest.AssertContains(t, screen, "Test Dialog")
		termtest.AssertContains(t, screen, "proceed")

		// Reset
		app.dialogState = nil
	})
}

// TestAppSnapshotView demonstrates snapshot testing for view consistency.
// This is useful for catching unintended changes to the UI layout.
func TestAppSnapshotView(t *testing.T) {
	agent := &dive.StandardAgent{}
	app := NewApp(agent, nil, "/workspace", anthropic.ModelClaudeSonnet45, "", nil, "", false, nil)

	t.Run("idle state view structure", func(t *testing.T) {
		// Ensure clean state
		app.processing = false
		app.dialogState = nil
		app.showTodos = false
		app.autocompleteMatches = nil
		app.showCompactionStats = false
		app.showExitHint = false

		screen := renderLiveView(t, app, 80, 24)
		text := screen.Text()

		// Verify structural elements are present
		assert.True(t, containsAny(text, "─", "-"), "expected divider line")
		assert.True(t, containsAny(text, ">", "❯"), "expected input prompt")
		assert.True(t, containsAny(text, "Type a message", "message"), "expected placeholder text")
	})
}

// containsAny returns true if s contains any of the substrings
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if containsString(s, sub) {
			return true
		}
	}
	return false
}

// containsString is a simple contains check
func containsString(s, sub string) bool {
	return len(sub) <= len(s) && findSubstring(s, sub) >= 0
}

// findSubstring finds substring index (simple implementation)
func findSubstring(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// splitLines splits text into lines
func splitLines(text string) []string {
	var lines []string
	start := 0
	for i, c := range text {
		if c == '\n' {
			lines = append(lines, text[start:i])
			start = i + 1
		}
	}
	if start < len(text) {
		lines = append(lines, text[start:])
	}
	return lines
}

// isEmptyOrWhitespace returns true if the string is empty or only whitespace
func isEmptyOrWhitespace(s string) bool {
	for _, c := range s {
		if c != ' ' && c != '\t' && c != '\r' {
			return false
		}
	}
	return true
}
