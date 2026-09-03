package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/tui"
)

// SessionPickerResult holds the result of the session picker
type SessionPickerResult struct {
	SessionID string
	Canceled  bool
}

// SessionPickerApp implements the session picker TUI
type SessionPickerApp struct {
	sessions     []*session.SessionInfo
	selectedIdx  int
	filter       string
	result       *SessionPickerResult
	workspaceDir string
}

// RunSessionPicker displays an interactive session picker and returns the selected session ID
func RunSessionPicker(store session.Store, filter string, workspaceDir string) (*SessionPickerResult, error) {
	ctx := context.Background()

	// List sessions from store
	listResult, err := store.List(ctx, &session.ListOptions{Limit: 50})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(listResult.Sessions) == 0 {
		return &SessionPickerResult{Canceled: true}, nil
	}

	// Filter sessions if a filter is provided
	var sessions []*session.SessionInfo
	if filter != "" {
		filterLower := strings.ToLower(filter)
		for _, s := range listResult.Sessions {
			// Match against title, ID, or workspace
			workspace := ""
			if s.Metadata != nil {
				if ws, ok := s.Metadata["workspace"].(string); ok {
					workspace = ws
				}
			}
			if strings.Contains(strings.ToLower(s.Title), filterLower) ||
				strings.Contains(strings.ToLower(s.ID), filterLower) ||
				strings.Contains(strings.ToLower(workspace), filterLower) {
				sessions = append(sessions, s)
			}
		}
	} else {
		sessions = listResult.Sessions
	}

	// A session with no turns is one that was opened and abandoned; there is
	// nothing in it to resume. They were four of the ten rows on screen.
	if withTurns := nonEmptySessions(sessions); len(withTurns) > 0 {
		sessions = withTurns
	}

	if len(sessions) == 0 {
		if filter != "" {
			return nil, fmt.Errorf("no sessions found matching %q", filter)
		}
		return &SessionPickerResult{Canceled: true}, nil
	}

	// Create picker app
	picker := &SessionPickerApp{
		sessions:     sessions,
		selectedIdx:  0,
		filter:       filter,
		result:       &SessionPickerResult{},
		workspaceDir: workspaceDir,
	}

	// Run the picker
	runner := tui.NewInlineApp(
		tui.WithInlineFPS(30),
		tui.WithInlineKittyKeyboard(true),
	)

	if err := runner.Run(picker); err != nil {
		return nil, err
	}

	return picker.result, nil
}

// visibleRows is how many sessions the picker shows at once. The picker runs
// in the inline live region, so every row it reserves is a row of the user's
// scrollback it paints over.
const visibleRows = 10

// window returns the slice of sessions currently on screen, keeping the
// selection inside it.
func (p *SessionPickerApp) window() (start, end int) {
	start = 0
	if p.selectedIdx >= visibleRows {
		start = p.selectedIdx - visibleRows + 1
	}
	end = min(start+visibleRows, len(p.sessions))
	return start, end
}

// LiveView implements tui.InlineApplication
func (p *SessionPickerApp) LiveView() tui.View {
	start, end := p.window()

	header := tui.Group(
		tui.Text(" Resume a session").Bold(),
		tui.Spacer().Flex(1),
		tui.Text("%d of %d ", end-start, len(p.sessions)).Style(metaStyle()),
	)

	views := []tui.View{tui.Text(""), header, tui.Text("")}
	for i := start; i < end; i++ {
		views = append(views, p.sessionItemView(p.sessions[i], i-start, i == p.selectedIdx))
	}

	views = append(views,
		tui.Text(""),
		p.detailView(),
		tui.Text(" ↑↓ navigate · 1-9 jump · Enter resume · Esc cancel").Style(metaStyle()),
	)
	return tui.Stack(views...)
}

// detailView is a fixed line under the list carrying the selected session's
// directory. Keeping it out of the rows is what lets the list be one line per
// session: the path was the same on nearly every row, and repeating it 10
// times said nothing while costing 10 lines.
func (p *SessionPickerApp) detailView() tui.View {
	if p.selectedIdx < 0 || p.selectedIdx >= len(p.sessions) {
		return tui.Text("")
	}
	ws := sessionWorkspace(p.sessions[p.selectedIdx])
	if ws == "" {
		return tui.Text("")
	}
	return tui.Text(" %s", shortenPath(ws)).Style(metaStyle())
}

// metaStyle is the picker's secondary text: timestamps, counts, key hints.
// Upright, not italic — a whole screen of slanted text is what made the old
// picker hard to read.
func metaStyle() tui.Style {
	return tui.NewStyle().WithFgRGB(dimText)
}

// sessionItemView renders one session as a single line:
//
//	▌1  what does this image say?   ~/other/repo      3 turns · 6w ago
//
// One line rather than two, because the second line was the workspace path and
// it is the same path on nearly every row. The path now appears only when it
// differs from where the picker was launched, which is the only time it tells
// the user anything.
func (p *SessionPickerApp) sessionItemView(info *session.SessionInfo, row int, selected bool) tui.View {
	titleStyle := tui.NewStyle().WithFgRGB(primaryText)
	marker := tui.Text("  ")
	gutter := tui.Text("%d ", row+1).Style(metaStyle())
	if row >= 9 {
		gutter = tui.Text("  ").Style(metaStyle())
	}
	if selected {
		titleStyle = tui.NewStyle().WithFgRGB(accentBright).WithBold()
		marker = tui.Text("▌ ").Style(tui.NewStyle().WithFgRGB(accentBright))
		gutter = tui.Text("%d ", row+1).Style(tui.NewStyle().WithFgRGB(accentDim))
	}

	parts := []tui.View{marker, gutter, tui.Text("%s", sessionTitle(info)).Style(titleStyle)}

	parts = append(parts,
		tui.Spacer().Flex(1),
		tui.Text("  %d turn%s · %s ", info.EventCount, pluralSuffix(info.EventCount), formatTimeAgo(info.UpdatedAt)).
			Style(metaStyle()),
	)
	return tui.Group(parts...)
}

// sessionTitle is what the session is about, in one line. Titles are taken
// from the first message, so an attachment can push the actual words off the
// end; the marker is dropped rather than truncated around.
func sessionTitle(info *session.SessionInfo) string {
	title := strings.TrimSpace(info.Title)
	for strings.HasPrefix(title, "[image:") {
		close := strings.Index(title, "]")
		if close < 0 {
			break
		}
		title = strings.TrimSpace(title[close+1:])
	}
	if title == "" {
		return "Untitled session"
	}
	// Truncate by runes: slicing bytes splits multi-byte characters.
	// 48 leaves room for the gutter and the right-hand meta at 80 columns,
	// which is the narrowest terminal worth laying out for.
	if r := []rune(title); len(r) > 48 {
		return strings.TrimRight(string(r[:47]), " ") + "…"
	}
	return title
}

// sessionWorkspace reads the workspace path a session was started in.
func sessionWorkspace(info *session.SessionInfo) string {
	if info.Metadata == nil {
		return ""
	}
	ws, _ := info.Metadata["workspace"].(string)
	return ws
}

// HandleEvent implements tui.EventHandler
func (p *SessionPickerApp) HandleEvent(event tui.Event) []tui.Cmd {
	switch e := event.(type) {
	case tui.KeyEvent:
		switch e.Key {
		case tui.KeyArrowUp:
			if p.selectedIdx > 0 {
				p.selectedIdx--
			}
		case tui.KeyArrowDown:
			if p.selectedIdx < len(p.sessions)-1 {
				p.selectedIdx++
			}
		case tui.KeyEnter:
			p.result.SessionID = p.sessions[p.selectedIdx].ID
			return []tui.Cmd{tui.Quit()}
		case tui.KeyEscape, tui.KeyCtrlC:
			p.result.Canceled = true
			return []tui.Cmd{tui.Quit()}
		}

		// Number keys jump to a row by its gutter number, which is why the
		// gutter is numbered from the top of the window rather than the list.
		if e.Rune >= '1' && e.Rune <= '9' {
			start, end := p.window()
			if idx := start + int(e.Rune-'1'); idx < end {
				p.result.SessionID = p.sessions[idx].ID
				return []tui.Cmd{tui.Quit()}
			}
		}
	}
	return nil
}

// nonEmptySessions drops sessions that hold no turns.
func nonEmptySessions(sessions []*session.SessionInfo) []*session.SessionInfo {
	kept := make([]*session.SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		if s.EventCount > 0 {
			kept = append(kept, s)
		}
	}
	return kept
}

// formatTimeAgo formats a time as a human-readable "time ago" string
func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	case duration < 7*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	default:
		weeks := int(duration.Hours() / 24 / 7)
		if weeks == 1 {
			return "1w ago"
		}
		return fmt.Sprintf("%dw ago", weeks)
	}
}

// shortenPath shortens a path by replacing home directory with ~
func shortenPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
