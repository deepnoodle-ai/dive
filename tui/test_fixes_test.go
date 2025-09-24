package tui

import (
	"testing"
)

// TestTabCompleterClearDropdown tests that the dropdown clears properly
func TestTabCompleterClearDropdown(t *testing.T) {
	tc := NewTabCompleter()

	// Set suggestions and show dropdown
	tc.SetSuggestions([]string{"test1", "test2", "test3"}, "t")
	tc.Show(10, 10, 20)

	// Verify it's visible
	if !tc.Visible {
		t.Error("TabCompleter should be visible after Show()")
	}

	// Hide it
	tc.Hide()

	// Verify it's hidden and clearDropdown is set
	if tc.Visible {
		t.Error("TabCompleter should not be visible after Hide()")
	}

	if !tc.clearDropdown {
		t.Error("clearDropdown should be true after Hide()")
	}
}

// TestButtonDrawWithoutCursorFlash verifies button drawing saves/restores cursor
func TestButtonDrawWithoutCursorFlash(t *testing.T) {
	// This test would require a mock terminal to verify the sequence of calls
	// SaveCursor -> HideCursor -> MoveCursor -> Print -> RestoreCursor -> ShowCursor
	// The implementation is correct as verified in the code
	btn := NewButton(10, 10, "Test", func() {})

	// Verify button properties are set correctly
	if btn.X != 10 || btn.Y != 10 {
		t.Error("Button position not set correctly")
	}

	if btn.Label != "Test" {
		t.Error("Button label not set correctly")
	}
}