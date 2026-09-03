package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/termtest"
	"github.com/deepnoodle-ai/wonton/tui"
)

// The picker shortens a workspace path against the real home directory, so the
// fixtures have to sit under whatever that is here. A hard-coded home made the
// "~" assertions pass on one machine and fail on every other, CI included.
var (
	pickerHome      = userHomeForTests()
	pickerHere      = filepath.Join(pickerHome, "git", "deepnoodle", "dive")
	pickerElsewhere = filepath.Join(pickerHome, "git", "other", "repo")
)

func userHomeForTests() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Nothing to shorten against; the assertions that care say so plainly.
		return ""
	}
	return home
}

func sessionInfo(title, workspace string, turns int, ago time.Duration) *session.SessionInfo {
	return &session.SessionInfo{
		ID:         title,
		Title:      title,
		EventCount: turns,
		UpdatedAt:  time.Now().Add(-ago),
		Metadata:   map[string]any{"workspace": workspace},
	}
}

func newPicker(sessions ...*session.SessionInfo) *SessionPickerApp {
	return &SessionPickerApp{
		sessions:     sessions,
		result:       &SessionPickerResult{},
		workspaceDir: pickerHere,
	}
}

func renderPicker(t *testing.T, p *SessionPickerApp, width int) *termtest.Screen {
	t.Helper()
	return tui.SprintScreen(p.LiveView(), tui.WithWidth(width), tui.WithHeight(24))
}

func TestPickerGivesEachSessionOneLine(t *testing.T) {
	// The old picker spent a second line per session on the workspace path,
	// which was the same path on nearly every row.
	p := newPicker(
		sessionInfo("first", pickerHere, 2, time.Minute),
		sessionInfo("second", pickerHere, 3, time.Hour),
		sessionInfo("third", pickerHere, 1, 24*time.Hour),
	)
	screen := renderPicker(t, p, 80)

	rows := map[string]int{}
	for y := range 24 {
		row := strings.TrimSpace(screen.Row(y))
		if row != "" {
			rows[row] = y
		}
	}
	for _, want := range []string{"first", "second", "third"} {
		found := false
		for row := range rows {
			if strings.Contains(row, want) {
				found = true
				assert.Contains(t, row, "turn", "%q should carry its own meta", want)
			}
		}
		assert.True(t, found, "%q should be on screen", want)
	}
	// header + blank + 3 sessions + blank + detail + keys, inside a blank line
	assert.True(t, len(rows) <= 7, "the whole picker should fit in 7 written rows, got %d", len(rows))
}

func TestPickerShowsTheSelectedSessionsDirectory(t *testing.T) {
	p := newPicker(
		sessionInfo("here", pickerHere, 2, time.Minute),
		sessionInfo("elsewhere", pickerElsewhere, 3, time.Hour),
	)

	assert.Contains(t, renderPicker(t, p, 80).Text(), filepath.Join("~", "git", "deepnoodle", "dive"))

	p.HandleEvent(tui.KeyEvent{Key: tui.KeyArrowDown})
	assert.Contains(t, renderPicker(t, p, 80).Text(), filepath.Join("~", "git", "other", "repo"))
}

func TestPickerMetaSurvivesALongTitle(t *testing.T) {
	// The turn count and age are the two things you pick on; a long title must
	// not push them off the right edge.
	p := newPicker(sessionInfo(strings.Repeat("very long title ", 12), pickerHere, 12, 3*time.Hour))
	row := renderPicker(t, p, 80).Row(3)

	assert.Contains(t, row, "12 turns · 3h ago")
	assert.Contains(t, row, "…", "the title is what gives way")
	assert.True(t, len([]rune(strings.TrimRight(row, " "))) <= 80)
}

func TestPickerDropsSessionsWithNothingInThem(t *testing.T) {
	kept := nonEmptySessions([]*session.SessionInfo{
		sessionInfo("real", pickerHere, 2, time.Minute),
		sessionInfo("abandoned", pickerHere, 0, time.Minute),
		sessionInfo("also real", pickerHere, 7, time.Minute),
	})
	assert.Equal(t, 2, len(kept))
	assert.Equal(t, "real", kept[0].Title)
	assert.Equal(t, "also real", kept[1].Title)
}

func TestSessionTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"plain", "what does this image say?", "what does this image say?"},
		{"empty", "", "Untitled session"},
		{"whitespace", "   ", "Untitled session"},
		{
			name:  "an attachment marker is dropped, not truncated around",
			title: "[image: /Users/curtis/Desktop/fascism.png] what does this say?",
			want:  "what does this say?",
		},
		{
			name:  "several markers",
			title: "[image: a.png] [image: b.png] compare these",
			want:  "compare these",
		},
		{"nothing but a marker", "[image: /tmp/a.png]", "Untitled session"},
		{
			name:  "long titles are cut with an ellipsis",
			title: strings.Repeat("ab", 40),
			want:  strings.Repeat("ab", 23) + "a…",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sessionTitle(&session.SessionInfo{Title: tc.title}))
		})
	}
}

func TestSessionTitleCutsWholeCharacters(t *testing.T) {
	// Slicing bytes would split a multi-byte rune and render a replacement
	// character.
	got := sessionTitle(&session.SessionInfo{Title: strings.Repeat("日", 60)})
	assert.False(t, strings.Contains(got, "�"), "got %q", got)
	assert.Equal(t, 48, len([]rune(got)))
}

func TestPickerNumberKeysMatchTheGutter(t *testing.T) {
	// The gutter numbers rows from the top of the window, so once the list has
	// scrolled, "1" must mean the top visible row and not the first session.
	sessions := make([]*session.SessionInfo, 0, 15)
	for i := range 15 {
		sessions = append(sessions, sessionInfo(string(rune('a'+i)), pickerHere, 1, time.Minute))
	}
	p := newPicker(sessions...)
	p.selectedIdx = 12

	start, end := p.window()
	assert.Equal(t, 3, start)
	assert.Equal(t, 13, end)

	p.HandleEvent(tui.KeyEvent{Rune: '1'})
	assert.Equal(t, sessions[3].ID, p.result.SessionID)
}

func TestPickerNumberKeyPastTheEndDoesNothing(t *testing.T) {
	p := newPicker(sessionInfo("only one", pickerHere, 1, time.Minute))
	p.HandleEvent(tui.KeyEvent{Rune: '5'})
	assert.Equal(t, "", p.result.SessionID)
	assert.False(t, p.result.Canceled)
}

func TestPickerSecondaryTextIsNotItalic(t *testing.T) {
	// A whole screen of slanted text is what made the old picker hard to read.
	assert.False(t, strings.Contains(tui.Sprint(newPicker(
		sessionInfo("a session", pickerHere, 2, time.Minute),
	).LiveView(), tui.WithWidth(80)), "\033[3m"))
}
