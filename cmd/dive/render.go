package main

import (
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/wonton/tui"
)

// Spinner frames for loading animation
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// View returns the current view tree
func (a *App) View() tui.View {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return tui.Stack(
		a.headerView(),
		tui.Divider(),
		a.messagesView(),
		tui.Divider(),
		a.inputView(),
		a.footerView(),
	)
}

func (a *App) headerView() tui.View {
	title := tui.Text(" Dive ").Bold().Fg(tui.ColorCyan)

	statusParts := []string{}
	if a.processing {
		statusParts = append(statusParts, "processing")
	}
	if a.thinking {
		spinner := spinnerFrames[a.tickCount%len(spinnerFrames)]
		statusParts = append(statusParts, spinner+" thinking")
	}

	var status tui.View
	if len(statusParts) > 0 {
		status = tui.Text(" %s ", strings.Join(statusParts, " | ")).Dim()
	} else {
		status = tui.Text(" ready ").Success()
	}

	return tui.Group(
		title,
		tui.Spacer(),
		status,
	)
}

func (a *App) messagesView() tui.View {
	if len(a.messages) == 0 {
		return tui.WrappedText("No messages yet.").Dim().Center()
	}

	views := make([]tui.View, 0, len(a.messages))
	for i, msg := range a.messages {
		views = append(views, a.messageView(msg, i))
	}

	// Scroll view is already flexible, no need to wrap in Stack
	return tui.Scroll(
		tui.Stack(views...).Gap(1).Padding(1),
		&a.scrollY,
	)
}

func (a *App) messageView(msg Message, index int) tui.View {
	switch msg.Type {
	case MessageTypeToolCall:
		return a.toolCallView(msg)
	default:
		return a.textMessageView(msg, index)
	}
}

func (a *App) textMessageView(msg Message, index int) tui.View {
	var roleView tui.View

	switch msg.Role {
	case "user":
		roleView = tui.Text("You").Bold().Fg(tui.ColorGreen)
	case "assistant":
		roleView = tui.Text("Dive").Bold().Fg(tui.ColorCyan)
	case "system":
		roleView = tui.Text("System").Bold().Fg(tui.ColorYellow)
	default:
		roleView = tui.Text(msg.Role).Bold()
	}

	content := msg.Content

	// Show thinking indicator for empty streaming message
	if msg.Role == "assistant" && content == "" && a.thinking && index == a.streamingMessageIndex {
		spinner := spinnerFrames[a.tickCount%len(spinnerFrames)]
		content = spinner + " Thinking..."
	}

	// Render content as markdown for assistant messages
	var contentView tui.View
	if msg.Role == "assistant" && content != "" {
		contentView = tui.Markdown(content, nil)
	} else if msg.Role == "system" {
		contentView = tui.WrappedText(content).Fg(tui.ColorYellow)
	} else {
		contentView = tui.WrappedText(content)
	}

	return tui.Stack(
		roleView,
		contentView,
	).Gap(0)
}

func (a *App) toolCallView(msg Message) tui.View {
	// Status indicator
	var statusText string
	var statusColor tui.Color
	if msg.ToolDone {
		if msg.ToolError {
			statusText = "✗"
			statusColor = tui.ColorRed
		} else {
			statusText = "✓"
			statusColor = tui.ColorGreen
		}
	} else {
		statusText = spinnerFrames[a.tickCount%len(spinnerFrames)]
		statusColor = tui.ColorYellow
	}

	statusView := tui.Text(statusText).Fg(statusColor)

	// Tool name
	nameView := tui.Text(msg.ToolName).Bold()

	// Header line
	header := tui.Group(
		statusView,
		tui.Text(" "),
		nameView,
	)

	views := []tui.View{header}

	// Input preview (truncated)
	if msg.ToolInput != "" {
		inputView := tui.Text("  %s", msg.ToolInput).Dim()
		views = append(views, inputView)
	}

	// Result (if done)
	if msg.ToolDone && msg.ToolResult != "" {
		var resultView tui.View
		if msg.ToolError {
			resultView = tui.Text("  %s", msg.ToolResult).Fg(tui.ColorRed)
		} else {
			resultView = tui.Text("  %s", msg.ToolResult).Dim()
		}
		views = append(views, resultView)
	}

	return tui.Stack(views...).Gap(0)
}

func (a *App) inputView() tui.View {
	// Check if we're in confirmation mode
	if a.confirm.Pending {
		return a.confirmView()
	}

	// Normal input mode
	prompt := tui.Text("> ").Fg(tui.ColorCyan).Bold()

	inputText := a.input
	if a.processing {
		inputText = "(processing...)"
	}

	var inputContent tui.View
	if a.processing {
		inputContent = tui.Text(inputText).Dim()
	} else {
		// Static cursor (no blinking to reduce flicker)
		inputContent = tui.Text("%s█", inputText)
	}

	return tui.Group(
		prompt,
		inputContent,
	).Padding(1)
}

func (a *App) confirmView() tui.View {
	header := tui.Text(" Confirm: %s ", a.confirm.ToolName).Bold().Fg(tui.ColorYellow)
	summary := tui.Text("  %s", a.confirm.Summary)
	prompt := tui.Text("  Allow? [y/n] ").Bold()

	return tui.Stack(
		header,
		summary,
		prompt,
	).Gap(0).Padding(1)
}

func (a *App) footerView() tui.View {
	var parts []tui.View

	if a.confirm.Pending {
		parts = append(parts, tui.Text(" y/n: confirm ").Dim())
	} else {
		parts = append(parts, tui.Text(" Enter: send ").Dim())
		parts = append(parts, tui.Text(" Shift+Enter: newline ").Dim())
		parts = append(parts, tui.Text(" PgUp/PgDn: scroll ").Dim())
	}

	parts = append(parts, tui.Spacer())
	parts = append(parts, tui.Text(" Ctrl+C: exit ").Dim())

	// Show workspace
	wsLabel := a.workspaceDir
	if len(wsLabel) > 30 {
		wsLabel = "..." + wsLabel[len(wsLabel)-27:]
	}
	parts = append(parts, tui.Text(" %s ", wsLabel).Dim())

	return tui.Group(parts...)
}

// wrapText wraps text to the specified width
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if len(line) <= width {
			lines = append(lines, line)
			continue
		}

		// Wrap long lines
		for len(line) > width {
			// Try to break at a space
			breakPoint := width
			for i := width - 1; i > width/2; i-- {
				if line[i] == ' ' {
					breakPoint = i
					break
				}
			}
			lines = append(lines, line[:breakPoint])
			line = strings.TrimLeft(line[breakPoint:], " ")
		}
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

// formatInput formats the input JSON for display
func formatInput(input string, maxLen int) string {
	// Remove newlines and excess whitespace
	formatted := strings.ReplaceAll(input, "\n", " ")
	formatted = strings.ReplaceAll(formatted, "\t", " ")

	// Collapse multiple spaces
	for strings.Contains(formatted, "  ") {
		formatted = strings.ReplaceAll(formatted, "  ", " ")
	}

	formatted = strings.TrimSpace(formatted)

	if len(formatted) > maxLen {
		return formatted[:maxLen-3] + "..."
	}
	return formatted
}

// countLines counts the number of lines in text
func countLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

// pluralize returns singular or plural form based on count
func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}
