package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/tui"
)

// SessionPickerResult holds the result of the session picker
type SessionPickerResult struct {
	SessionID string
	Canceled  bool
}

// SessionPickerApp implements the session picker TUI
type SessionPickerApp struct {
	sessions     []*dive.Session
	selectedIdx  int
	filter       string
	result       *SessionPickerResult
	workspaceDir string
}

// RunSessionPicker displays an interactive session picker and returns the selected session ID
func RunSessionPicker(repo dive.SessionRepository, filter string, workspaceDir string) (*SessionPickerResult, error) {
	ctx := context.Background()

	// List sessions from repository
	listResult, err := repo.ListSessions(ctx, &dive.ListSessionsInput{Limit: 50})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(listResult.Items) == 0 {
		return &SessionPickerResult{Canceled: true}, nil
	}

	// Filter sessions if a filter is provided
	var sessions []*dive.Session
	if filter != "" {
		filterLower := strings.ToLower(filter)
		for _, s := range listResult.Items {
			// Match against title, ID, or workspace
			if strings.Contains(strings.ToLower(s.Title), filterLower) ||
				strings.Contains(strings.ToLower(s.ID), filterLower) ||
				(s.Metadata != nil && strings.Contains(strings.ToLower(fmt.Sprintf("%v", s.Metadata["workspace"])), filterLower)) {
				sessions = append(sessions, s)
			}
		}
	} else {
		sessions = listResult.Items
	}

	if len(sessions) == 0 {
		fmt.Printf("No sessions found matching %q\n", filter)
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
	runner := tui.NewInlineApp(tui.InlineAppConfig{
		FPS:           30,
		KittyKeyboard: true,
	})

	if err := runner.Run(picker); err != nil {
		return nil, err
	}

	return picker.result, nil
}

// LiveView implements tui.InlineApplication
func (p *SessionPickerApp) LiveView() tui.View {
	views := []tui.View{
		tui.Text(""),
		tui.Text(" Select a session to resume:").Bold(),
		tui.Text(""),
	}

	// Show sessions (max 10 at a time with scrolling)
	startIdx := 0
	if p.selectedIdx >= 10 {
		startIdx = p.selectedIdx - 9
	}
	endIdx := startIdx + 10
	if endIdx > len(p.sessions) {
		endIdx = len(p.sessions)
	}

	for i := startIdx; i < endIdx; i++ {
		session := p.sessions[i]
		views = append(views, p.sessionItemView(session, i == p.selectedIdx))
	}

	// Show scroll indicator if needed
	if len(p.sessions) > 10 {
		views = append(views, tui.Text(""))
		views = append(views, tui.Text(" (%d/%d sessions)", endIdx, len(p.sessions)).Hint())
	}

	views = append(views, tui.Text(""))
	views = append(views, tui.Text(" ↑/↓ to navigate, Enter to select, Esc to cancel").Hint())

	return tui.Stack(views...)
}

// sessionItemView creates the view for a single session item
func (p *SessionPickerApp) sessionItemView(session *dive.Session, selected bool) tui.View {
	// Build the session display
	timeAgo := formatTimeAgo(session.UpdatedAt)

	// Get title or generate from first message
	title := session.Title
	if title == "" && len(session.Messages) > 0 {
		// Use first user message as title
		for _, msg := range session.Messages {
			if msg.Role == "user" {
				title = msg.Text()
				break
			}
		}
	}
	if title == "" {
		title = "Untitled session"
	}

	// Truncate title if too long
	if len(title) > 50 {
		title = title[:47] + "..."
	}

	// Get workspace if available
	var workspace string
	if session.Metadata != nil {
		if ws, ok := session.Metadata["workspace"].(string); ok {
			workspace = shortenPath(ws)
		}
	}

	// Build the item view
	if selected {
		line1 := tui.Group(
			tui.Text(" ❯ ").Fg(tui.ColorCyan),
			tui.Text("[%s] ", timeAgo).Hint(),
			tui.Text("%s", title).Fg(tui.ColorCyan),
		)

		var line2 tui.View
		if workspace != "" {
			line2 = tui.Text("     %s (%d messages)", workspace, len(session.Messages)).Hint()
		} else {
			line2 = tui.Text("     %d messages", len(session.Messages)).Hint()
		}

		return tui.Stack(line1, line2)
	}

	line1 := tui.Group(
		tui.Text("   "),
		tui.Text("[%s] ", timeAgo).Hint(),
		tui.Text("%s", title),
	)

	var line2 tui.View
	if workspace != "" {
		line2 = tui.Text("     %s (%d messages)", workspace, len(session.Messages)).Hint()
	} else {
		line2 = tui.Text("     %d messages", len(session.Messages)).Hint()
	}

	return tui.Stack(line1, line2)
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

		// Handle number keys 1-9 for quick selection
		if e.Rune >= '1' && e.Rune <= '9' {
			idx := int(e.Rune - '1')
			if idx < len(p.sessions) {
				p.result.SessionID = p.sessions[idx].ID
				return []tui.Cmd{tui.Quit()}
			}
		}
	}
	return nil
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
