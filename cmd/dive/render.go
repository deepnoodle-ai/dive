package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/deepnoodle-ai/wonton/tui"
)

// diveMarkdownTheme returns a custom markdown theme matching Claude Code styling
func diveMarkdownTheme() tui.MarkdownTheme {
	theme := tui.DefaultMarkdownTheme()
	// Light purple for inline code (like Claude Code)
	theme.CodeStyle = tui.NewStyle().WithFgRGB(tui.RGB{R: 180, G: 140, B: 220})
	return theme
}

// messageView returns the view for a message (with animations for live updates)
func (a *App) messageView(msg Message, index int) tui.View {
	switch msg.Type {
	case MessageTypeToolCall:
		return a.toolCallView(msg)
	default:
		return a.textMessageView(msg, index)
	}
}

// textMessageView renders a text message (simplified for live updates - no markdown parsing)
func (a *App) textMessageView(msg Message, index int) tui.View {
	switch msg.Role {
	case "intro":
		return a.introView(msg)

	case "user":
		return tui.Text("> %s", msg.Content).Wrap().Style(
			tui.NewStyle().WithBgRGB(tui.RGB{R: 40, G: 40, B: 45}).WithItalic(),
		)

	case "assistant":
		content := strings.TrimRight(msg.Content, "\n")
		if content == "" {
			return nil
		}
		// Use simple text for live view (faster than markdown parsing)
		return tui.Text("⏺ %s", content).Wrap()

	case "system":
		return tui.Text("%s", msg.Content).Wrap().Warning()

	default:
		return tui.Text("%s", msg.Content).Wrap()
	}
}

// introView renders the splash screen with dive branding
func (a *App) introView(msg Message) tui.View {
	// Parse model and workspace from content
	parts := strings.SplitN(msg.Content, "\n", 2)
	model := parts[0]
	workspace := ""
	if len(parts) > 1 {
		workspace = parts[1]
	}

	// ASCII art logo
	artLines := []string{
		"  ██████╗ ██╗██╗   ██╗███████╗",
		"  ██╔══██╗██║██║   ██║██╔════╝",
		"  ██║  ██║██║██║   ██║█████╗  ",
		"  ██║  ██║██║╚██╗ ██╔╝██╔══╝  ",
		"  ██████╔╝██║ ╚████╔╝ ███████╗",
		"  ╚═════╝ ╚═╝  ╚═══╝  ╚══════╝",
	}

	// Find max width for consistent gradient
	maxWidth := 0
	for _, line := range artLines {
		if w := len([]rune(line)); w > maxWidth {
			maxWidth = w
		}
	}

	// Build logo with gradient
	logoViews := make([]tui.View, len(artLines))
	for row, line := range artLines {
		runes := []rune(line)
		charViews := make([]tui.View, len(runes))

		for col, r := range runes {
			t := float64(col) / float64(maxWidth-1)
			color := interpolateGradient(t)
			charViews[col] = tui.Text("%c", r).Style(tui.NewStyle().WithFgRGB(color))
		}

		logoViews[row] = tui.Group(charViews...)
	}

	// Style constants
	accentColor := tui.RGB{R: 80, G: 200, B: 235}
	mutedColor := tui.RGB{R: 140, G: 140, B: 155}
	dimColor := tui.RGB{R: 100, G: 100, B: 115}

	version := tui.Group(
		tui.Text("  ◆ ").Style(tui.NewStyle().WithFgRGB(accentColor)),
		tui.Text("v0.1.0").Style(tui.NewStyle().WithFgRGB(tui.RGB{R: 90, G: 90, B: 100})),
	)

	modelLine := tui.Group(
		tui.Text("  ◆ ").Style(tui.NewStyle().WithFgRGB(accentColor)),
		tui.Text("%s", model).Style(tui.NewStyle().WithFgRGB(mutedColor)),
	)

	workspaceLine := tui.Group(
		tui.Text("  ◆ ").Style(tui.NewStyle().WithFgRGB(accentColor)),
		tui.Text("%s", workspace).Style(tui.NewStyle().WithFgRGB(dimColor)),
	)

	// Combine all views
	var views []tui.View
	views = append(views, tui.Text(""))
	views = append(views, logoViews...)
	views = append(views, tui.Text(""))
	views = append(views, version)
	views = append(views, modelLine)
	views = append(views, workspaceLine)
	views = append(views, tui.Text(""))
	views = append(views, tui.Text(""))
	return tui.Stack(views...).Gap(0)
}

// interpolateGradient returns a color along the dive gradient
func interpolateGradient(t float64) tui.RGB {
	type colorStop struct {
		pos   float64
		color tui.RGB
	}
	stops := []colorStop{
		{0.0, tui.RGB{R: 80, G: 220, B: 240}},
		{0.35, tui.RGB{R: 70, G: 150, B: 230}},
		{0.65, tui.RGB{R: 100, G: 120, B: 210}},
		{1.0, tui.RGB{R: 140, G: 100, B: 200}},
	}

	var lower, upper colorStop
	for i := 0; i < len(stops)-1; i++ {
		if t >= stops[i].pos && t <= stops[i+1].pos {
			lower = stops[i]
			upper = stops[i+1]
			break
		}
	}

	if t <= 0 {
		return stops[0].color
	}
	if t >= 1 {
		return stops[len(stops)-1].color
	}

	localT := (t - lower.pos) / (upper.pos - lower.pos)
	return tui.RGB{
		R: uint8(float64(lower.color.R) + localT*(float64(upper.color.R)-float64(lower.color.R))),
		G: uint8(float64(lower.color.G) + localT*(float64(upper.color.G)-float64(lower.color.G))),
		B: uint8(float64(lower.color.B) + localT*(float64(upper.color.B)-float64(lower.color.B))),
	}
}

// toolCallView renders a tool call message (with animation)
func (a *App) toolCallView(msg Message) tui.View {
	var statusView tui.View
	if msg.ToolDone {
		if msg.ToolError {
			statusView = tui.Text("⏺").Error()
		} else {
			statusView = tui.Text("⏺").Success()
		}
	} else {
		statusView = tui.Text("⏺").Animate(tui.Pulse(tui.NewRGB(80, 160, 220), 8).Brightness(0.3, 1.0))
	}

	toolCall := formatToolCall(msg.ToolTitle, msg.ToolName, msg.ToolInput)
	header := tui.Group(statusView, tui.Text(" %s", toolCall))

	if msg.ToolDone && len(msg.ToolResultLines) > 0 {
		resultView := a.formatToolResultView(msg)
		if resultView != nil {
			return tui.Stack(header, resultView).Gap(0)
		}
	}

	return header
}

// toolResultStyle returns the style for tool result text (brighter than muted)
func toolResultStyle() tui.Style {
	return tui.NewStyle().WithFgRGB(tui.RGB{R: 140, G: 140, B: 150})
}

// formatToolResultView formats tool result
func (a *App) formatToolResultView(msg Message) tui.View {
	// Special handling for read_file - show line count
	if msg.ToolName == "read_file" && msg.ToolReadLines > 0 {
		resultText := fmt.Sprintf("Read %d lines", msg.ToolReadLines)
		if msg.ToolError {
			return tui.Text("  ⎿  %s", resultText).Error()
		}
		return tui.Text("  ⎿  %s", resultText).Style(toolResultStyle())
	}

	lines := msg.ToolResultLines
	if len(lines) == 0 {
		return nil
	}

	// Check for diff output
	if isDiffResult(strings.Join(lines, "\n")) {
		return renderDiffResult(strings.Join(lines, "\n"))
	}

	views := make([]tui.View, 0, 4)

	firstLine := lines[0]
	if len(firstLine) > 80 {
		firstLine = firstLine[:77] + "..."
	}
	if msg.ToolError {
		views = append(views, tui.Text("  ⎿  %s", firstLine).Error())
	} else {
		views = append(views, tui.Text("  ⎿  %s", firstLine).Style(toolResultStyle()))
	}

	if len(lines) > 1 {
		views = append(views, tui.Text("     … +%d lines", len(lines)-1).Hint())
	}

	return tui.Stack(views...).Gap(0)
}

// formatToolCall formats a tool call for display
// title is the human-readable display name, apiName is the actual tool name for special handling
func formatToolCall(title, apiName, inputJSON string) string {
	if inputJSON == "" {
		return title + "()"
	}

	var params map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &params); err != nil {
		return title + "(...)"
	}

	if len(params) == 0 {
		return title + "()"
	}

	// Special handling for bash tool
	if apiName == "bash" {
		if cmd, ok := params["command"].(string); ok {
			displayCmd := cmd
			if len(displayCmd) > 100 {
				displayCmd = displayCmd[:97] + "..."
			}
			displayCmd = strings.ReplaceAll(displayCmd, "\n", " ")
			return title + "(" + displayCmd + ")"
		}
	}

	// Special handling for read_file - show as Read(filepath)
	if apiName == "read_file" {
		if filePath, ok := params["file_path"].(string); ok {
			return fmt.Sprintf("Read(%s)", filePath)
		}
		return "Read()"
	}

	// Special handling for list_directory - show as List Directory(path)
	if apiName == "list_directory" {
		if path, ok := params["path"].(string); ok {
			return fmt.Sprintf("List Directory(%s)", path)
		}
		return "List Directory()"
	}

	// Special handling for todo_write
	if apiName == "todo_write" {
		if todos, ok := params["todos"].([]any); ok {
			return fmt.Sprintf("%s(%d items)", title, len(todos))
		}
		return title + "()"
	}

	// Sort keys for consistent ordering
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Format each parameter
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := params[k]
		parts = append(parts, fmt.Sprintf("%s: %s", k, formatParamValue(v)))
	}

	return title + "(" + strings.Join(parts, ", ") + ")"
}

// formatParamValue formats a parameter value
func formatParamValue(v any) string {
	switch val := v.(type) {
	case string:
		if len(val) > 40 {
			val = val[:37] + "..."
		}
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%v", val)
	case float64:
		if val == float64(int(val)) {
			return fmt.Sprintf("%d", int(val))
		}
		return fmt.Sprintf("%g", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// isDiffResult checks if the result looks like a diff output
func isDiffResult(result string) bool {
	lines := strings.Split(result, "\n")
	if len(lines) < 2 {
		return false
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "+ ") || strings.HasPrefix(trimmed, "- ") {
			return true
		}
	}
	return false
}

const (
	maxDisplayLines   = 50
	maxDisplayLineLen = 120
)

func truncateDisplayLine(line string) string {
	if len(line) > maxDisplayLineLen {
		return line[:maxDisplayLineLen-3] + "..."
	}
	return line
}

// renderDiffResult renders a diff result with colored lines
func renderDiffResult(result string) tui.View {
	if result == "" {
		return nil
	}

	lines := strings.Split(result, "\n")

	truncated := false
	if len(lines) > maxDisplayLines {
		lines = lines[:maxDisplayLines]
		truncated = true
	}

	if len(lines) == 0 {
		return nil
	}

	views := make([]tui.View, 0, len(lines)+2)

	if len(lines) > 0 {
		views = append(views, tui.Text("  ⎿  %s", truncateDisplayLine(lines[0])).Style(toolResultStyle()))
	}

	for i := 1; i < len(lines); i++ {
		line := truncateDisplayLine(lines[i])
		trimmed := strings.TrimSpace(lines[i])

		var lineView tui.View
		if strings.HasPrefix(trimmed, "+ ") {
			lineView = tui.Text("      %s", line).Success()
		} else if strings.HasPrefix(trimmed, "- ") {
			lineView = tui.Text("      %s", line).Error()
		} else {
			lineView = tui.Text("      %s", line).Style(toolResultStyle())
		}
		views = append(views, lineView)
	}

	if truncated {
		views = append(views, tui.Text("      ... (output truncated)").Hint())
	}

	return tui.Stack(views...).Gap(0)
}

// todoStatusView renders the active todo status line
func (a *App) todoStatusView() tui.View {
	var activeTodo *Todo
	for i := range a.todos {
		if a.todos[i].Status == TodoStatusInProgress {
			activeTodo = &a.todos[i]
			break
		}
	}

	if activeTodo == nil {
		return nil
	}

	elapsed := time.Since(a.processingStartTime)
	return tui.Group(
		tui.Text("✽").Animate(tui.Pulse(tui.NewRGB(180, 140, 220), 20).Brightness(0.4, 1.0)),
		tui.Text(" %s…", activeTodo.ActiveForm).Style(tui.NewStyle().WithFgRGB(tui.RGB{R: 180, G: 140, B: 220})),
		tui.Text(" (%s)", formatDuration(elapsed)).Hint(),
	)
}

// todoListView renders the todo list (with animations)
func (a *App) todoListView() tui.View {
	if len(a.todos) == 0 {
		return nil
	}

	views := make([]tui.View, 0, len(a.todos)+1)

	if statusView := a.todoStatusView(); statusView != nil {
		views = append(views, statusView)
	}

	for _, todo := range a.todos {
		var todoView tui.View
		switch todo.Status {
		case TodoStatusCompleted:
			mutedStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 100, B: 100})
			todoView = tui.Group(
				tui.Text("  ⎿  ☒ ").Style(mutedStyle),
				tui.Text("%s", todo.Content).Style(mutedStyle.WithStrikethrough()),
			)
		case TodoStatusInProgress:
			style := tui.NewStyle().WithFgRGB(tui.RGB{R: 180, G: 140, B: 220}).WithBold()
			todoView = tui.Text("  ⎿  ☐ %s", todo.Content).Style(style)
		default:
			style := tui.NewStyle().WithFgRGB(tui.RGB{R: 140, G: 140, B: 140})
			todoView = tui.Text("  ⎿  ☐ %s", todo.Content).Style(style)
		}
		views = append(views, todoView)
	}

	return tui.Stack(views...).Gap(0)
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// Static versions for scroll history (no animations)

func (a *App) messageViewStatic(msg Message) tui.View {
	switch msg.Type {
	case MessageTypeToolCall:
		return a.toolCallViewStatic(msg)
	default:
		return a.textMessageViewStatic(msg)
	}
}

func (a *App) textMessageViewStatic(msg Message) tui.View {
	switch msg.Role {
	case "user":
		return tui.Text("> %s", msg.Content).Wrap().Style(
			tui.NewStyle().WithBgRGB(tui.RGB{R: 40, G: 40, B: 45}).WithItalic(),
		)

	case "assistant":
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			return nil
		}
		return tui.Markdown("⏺ "+content, nil).Theme(diveMarkdownTheme())

	case "system":
		return tui.Text("%s", msg.Content).Wrap().Warning()

	default:
		return tui.Text("%s", msg.Content).Wrap()
	}
}

func (a *App) toolCallViewStatic(msg Message) tui.View {
	var statusView tui.View
	if msg.ToolDone {
		if msg.ToolError {
			statusView = tui.Text("⏺").Error()
		} else {
			statusView = tui.Text("⏺").Success()
		}
	} else {
		statusView = tui.Text("⏺").Fg(tui.ColorCyan)
	}

	toolCall := formatToolCall(msg.ToolTitle, msg.ToolName, msg.ToolInput)
	header := tui.Group(statusView, tui.Text(" %s", toolCall))

	if msg.ToolDone && len(msg.ToolResultLines) > 0 {
		resultView := a.formatToolResultView(msg)
		if resultView != nil {
			return tui.Stack(header, resultView).Gap(0)
		}
	}

	return header
}

func (a *App) todoListViewStatic() tui.View {
	if len(a.todos) == 0 {
		return nil
	}

	views := make([]tui.View, 0, len(a.todos))

	for _, todo := range a.todos {
		var todoView tui.View
		switch todo.Status {
		case TodoStatusCompleted:
			mutedStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 100, B: 100})
			todoView = tui.Group(
				tui.Text("  ⎿  ☒ ").Style(mutedStyle),
				tui.Text("%s", todo.Content).Style(mutedStyle.WithStrikethrough()),
			)
		case TodoStatusInProgress:
			style := tui.NewStyle().WithFgRGB(tui.RGB{R: 180, G: 140, B: 220}).WithBold()
			todoView = tui.Text("  ⎿  ☐ %s", todo.Content).Style(style)
		default:
			style := tui.NewStyle().WithFgRGB(tui.RGB{R: 140, G: 140, B: 140})
			todoView = tui.Text("  ⎿  ☐ %s", todo.Content).Style(style)
		}
		views = append(views, todoView)
	}

	return tui.Stack(views...).Gap(0)
}
