package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/deepnoodle-ai/wonton/tui"
)

// spinnerSpeed controls how many frames per spinner character change (higher = slower)
const spinnerSpeed = 6

// diveMarkdownTheme returns a custom markdown theme matching Claude Code styling
func diveMarkdownTheme() tui.MarkdownTheme {
	theme := tui.DefaultMarkdownTheme()
	// Light purple for inline code (like Claude Code)
	theme.CodeStyle = tui.NewStyle().WithFgRGB(tui.RGB{R: 180, G: 140, B: 220})
	return theme
}

// View returns the current view tree
func (a *App) View() tui.View {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Check if we're in any interactive prompt mode
	hasPrompt := a.confirm.Pending || a.selectState.Pending ||
		a.multiSelectState.Pending || a.inputState.Pending

	if hasPrompt {
		return tui.Stack(
			tui.MaxHeight(1, a.headerView()),
			tui.MaxHeight(1, tui.Divider()),
			a.messagesView(),
			tui.MaxHeight(1, tui.Divider()),
			a.promptView(),
			tui.MaxHeight(1, tui.Divider()),
			tui.MaxHeight(1, a.footerView()),
		)
	}

	// Check if autocomplete is active
	if a.autocomplete.Active && len(a.autocomplete.Matches) > 0 {
		return tui.Stack(
			tui.MaxHeight(1, a.headerView()),
			tui.MaxHeight(1, tui.Divider()),
			a.messagesView(),
			tui.MaxHeight(1, tui.Divider()),
			a.inputView(),
			tui.MaxHeight(1, tui.Divider()),
			a.autocompleteView(),
		)
	}

	return tui.Stack(
		tui.MaxHeight(1, a.headerView()),
		tui.MaxHeight(1, tui.Divider()),
		a.messagesView(),
		tui.MaxHeight(1, tui.Divider()),
		a.inputView(),
		tui.MaxHeight(1, tui.Divider()),
		tui.MaxHeight(1, a.footerView()),
	)
}

func (a *App) headerView() tui.View {
	title := tui.Text(" Dive ").Bold().Fg(tui.ColorCyan)

	status := tui.IfElse(a.thinking,
		tui.Group(
			tui.Loading(a.frame).CharSet(tui.SpinnerBounce.Frames).Speed(spinnerSpeed).Fg(tui.ColorCyan),
			tui.Text(" thinking").Animate(tui.Slide(3, tui.NewRGB(80, 80, 80), tui.NewRGB(80, 200, 220))),
		),
		tui.IfElse(a.processing,
			tui.Text(" processing ").Muted(),
			tui.Text(" ready ").Success(),
		),
	)

	return tui.Group(
		title,
		tui.Spacer(),
		status,
	)
}

func (a *App) messagesView() tui.View {
	if len(a.messages) == 0 {
		return tui.Text("No messages yet.").Wrap().Muted().Center()
	}

	// Build message views
	messagesStack := tui.ForEach(a.messages, func(msg Message, i int) tui.View {
		return a.messageView(msg, i)
	}).Gap(1)

	// Add todo list if visible and has items
	if a.showTodos && len(a.todos) > 0 {
		todoView := a.todoListView()
		if todoView != nil {
			return tui.Scroll(
				tui.Padding(1,
					tui.Stack(messagesStack, todoView).Gap(1),
				),
				&a.scrollY,
			).Bottom()
		}
	}

	return tui.Scroll(
		tui.Padding(1, messagesStack),
		&a.scrollY,
	).Bottom() // Chat-style scrolling - anchor to bottom
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
	switch msg.Role {
	case "intro":
		return a.introView(msg)

	case "user":
		// User messages: italic with dark gray background
		return tui.Text("> %s", msg.Content).Wrap().
			Style(tui.NewStyle().WithItalic().WithBgRGB(tui.RGB{R: 50, G: 50, B: 50})).
			FillBg()

	case "assistant":
		// Assistant messages: bullet prefix, markdown content
		content := strings.TrimRight(msg.Content, "\n")
		if content == "" {
			// Show thinking spinner for current streaming message, otherwise empty
			return tui.If(a.thinking && index == a.streamingMessageIndex,
				tui.Group(
					tui.Loading(a.frame).CharSet(tui.SpinnerBounce.Frames).Speed(spinnerSpeed).Fg(tui.ColorCyan),
					tui.Text(" Thinking...").Animate(tui.Slide(3, tui.NewRGB(80, 80, 80), tui.NewRGB(80, 200, 220))),
				),
			)
		}
		// Prefix content with bullet point like Claude Code
		return tui.Markdown("⏺ "+content, nil).Theme(diveMarkdownTheme())

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

	// ASCII art logo - using block characters for density
	artLines := []string{
		"  ██████╗ ██╗██╗   ██╗███████╗",
		"  ██╔══██╗██║██║   ██║██╔════╝",
		"  ██║  ██║██║██║   ██║█████╗  ",
		"  ██║  ██║██║╚██╗ ██╔╝██╔══╝  ",
		"  ██████╔╝██║ ╚████╔╝ ███████╗",
		"  ╚═════╝ ╚═╝  ╚═══╝  ╚══════╝",
	}

	// Find max width for consistent gradient across all lines
	maxWidth := 0
	for _, line := range artLines {
		if w := len([]rune(line)); w > maxWidth {
			maxWidth = w
		}
	}

	// Build logo with per-character horizontal gradient (like Gemini CLI)
	logoViews := make([]tui.View, len(artLines))

	for row, line := range artLines {
		runes := []rune(line)
		charViews := make([]tui.View, len(runes))

		for col, r := range runes {
			// Horizontal gradient: 0.0 = left, 1.0 = right
			t := float64(col) / float64(maxWidth-1)
			color := interpolateGradient(t)
			charViews[col] = tui.Text("%c", r).Style(tui.NewStyle().WithFgRGB(color))
		}

		logoViews[row] = tui.Group(charViews...)
	}

	// Version styled subtly
	version := tui.Text("  v0.1.0").Style(tui.NewStyle().WithFgRGB(tui.RGB{R: 90, G: 90, B: 100}))

	// Accent colors matching gradient
	accentColor := tui.RGB{R: 80, G: 200, B: 235} // Cyan from gradient
	mutedColor := tui.RGB{R: 140, G: 140, B: 155}
	dimColor := tui.RGB{R: 100, G: 100, B: 115}

	// Model info
	modelLine := tui.Group(
		tui.Text("  ◆ ").Style(tui.NewStyle().WithFgRGB(accentColor)),
		tui.Text("%s", model).Style(tui.NewStyle().WithFgRGB(mutedColor)),
	)

	// Workspace info
	workspaceLine := tui.Group(
		tui.Text("  ◇ ").Style(tui.NewStyle().WithFgRGB(dimColor)),
		tui.Text("%s", workspace).Style(tui.NewStyle().WithFgRGB(dimColor)),
	)

	// Tagline
	tagline := tui.Text("  Your AI coding companion").Style(tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 130, B: 150}).WithItalic())

	// Combine all views
	views := make([]tui.View, 0, len(logoViews)+6)
	views = append(views, tui.Text(""))
	views = append(views, logoViews...)
	views = append(views, version)
	views = append(views, tui.Text(""))
	views = append(views, modelLine)
	views = append(views, workspaceLine)
	views = append(views, tui.Text(""))
	views = append(views, tagline)
	views = append(views, tui.Text(""))

	return tui.Stack(views...).Gap(0)
}

// interpolateGradient returns a color along the dive gradient (0.0 = left/surface, 1.0 = right/deep)
func interpolateGradient(t float64) tui.RGB {
	// Vibrant gradient: cyan -> blue -> purple (like diving from surface to depths)
	type colorStop struct {
		pos   float64
		color tui.RGB
	}
	stops := []colorStop{
		{0.0, tui.RGB{R: 80, G: 220, B: 240}},   // Bright cyan (surface)
		{0.35, tui.RGB{R: 70, G: 150, B: 230}},  // Ocean blue
		{0.65, tui.RGB{R: 100, G: 120, B: 210}}, // Blue-purple
		{1.0, tui.RGB{R: 140, G: 100, B: 200}},  // Deep purple (abyss)
	}

	// Find the two stops we're between
	var lower, upper colorStop
	for i := 0; i < len(stops)-1; i++ {
		if t >= stops[i].pos && t <= stops[i+1].pos {
			lower = stops[i]
			upper = stops[i+1]
			break
		}
	}

	// Handle edge cases
	if t <= 0 {
		return stops[0].color
	}
	if t >= 1 {
		return stops[len(stops)-1].color
	}

	// Interpolate between the two stops
	localT := (t - lower.pos) / (upper.pos - lower.pos)

	return tui.RGB{
		R: uint8(float64(lower.color.R) + localT*(float64(upper.color.R)-float64(lower.color.R))),
		G: uint8(float64(lower.color.G) + localT*(float64(upper.color.G)-float64(lower.color.G))),
		B: uint8(float64(lower.color.B) + localT*(float64(upper.color.B)-float64(lower.color.B))),
	}
}

func (a *App) toolCallView(msg Message) tui.View {
	// Status indicator: ⏺ prefix with color based on state
	// Use blinking animation for pending tool calls
	var statusView tui.View
	if msg.ToolDone {
		if msg.ToolError {
			statusView = tui.Text("⏺").Error()
		} else {
			statusView = tui.Text("⏺").Success()
		}
	} else {
		// Pulsing effect for pending tool calls
		statusView = tui.Text("⏺").Animate(tui.Pulse(tui.NewRGB(80, 160, 220), 8).Brightness(0.3, 1.0))
	}

	// Format tool call like: ToolName(param: value, param: value)
	// Special handling for bash tool to show command directly
	toolCall := formatToolCall(msg.ToolName, msg.ToolInput)
	callView := tui.Text(" %s", toolCall)

	// Header line: ⏺ ToolName(params...)
	header := tui.Group(
		statusView,
		callView,
	)

	// Result view (only shown when done with result)
	if msg.ToolDone && len(msg.ToolResultLines) > 0 {
		resultView := a.formatToolResultView(msg)
		if resultView != nil {
			return tui.Stack(header, resultView).Gap(0)
		}
	}

	return header
}

// formatToolResultView formats tool result with expandable multi-line display
func (a *App) formatToolResultView(msg Message) tui.View {
	lines := msg.ToolResultLines
	if len(lines) == 0 {
		return nil
	}

	// Check if this is a diff result
	if isDiffResult(strings.Join(lines, "\n")) {
		return renderDiffResult(strings.Join(lines, "\n"))
	}

	views := make([]tui.View, 0, 4)

	// Show first line
	firstLine := lines[0]
	if len(firstLine) > 80 {
		firstLine = firstLine[:77] + "..."
	}
	if msg.ToolError {
		views = append(views, tui.Text("  ⎿  %s", firstLine).Error())
	} else {
		views = append(views, tui.Text("  ⎿  %s", firstLine).Muted())
	}

	// Show additional lines indicator if there are more lines
	if len(lines) > 1 {
		extraLines := len(lines) - 1
		views = append(views, tui.Text("     … +%d lines", extraLines).Hint())
	}

	return tui.Stack(views...).Gap(0)
}

// formatToolCall formats a tool call like: ToolName(param: value, param: value)
// For bash tool, shows: Bash(command here) like Claude Code
func formatToolCall(name, inputJSON string) string {
	if inputJSON == "" {
		return name + "()"
	}

	var params map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &params); err != nil {
		// If not valid JSON, just show the tool name
		return name + "(...)"
	}

	if len(params) == 0 {
		return name + "()"
	}

	// Special handling for bash tool - show command directly like Claude Code
	if name == "bash" || name == "Bash" {
		if cmd, ok := params["command"].(string); ok {
			// Show full command (truncated if very long)
			displayCmd := cmd
			if len(displayCmd) > 100 {
				displayCmd = displayCmd[:97] + "..."
			}
			// Replace newlines with spaces for single-line display
			displayCmd = strings.ReplaceAll(displayCmd, "\n", " ")
			return "Bash(" + displayCmd + ")"
		}
	}

	// Special handling for TodoWrite - just show item count
	if name == "TodoWrite" || name == "todo_write" {
		if todos, ok := params["todos"].([]any); ok {
			return fmt.Sprintf("TodoWrite(%d items)", len(todos))
		}
		return "TodoWrite()"
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

	return name + "(" + strings.Join(parts, ", ") + ")"
}

// formatParamValue formats a parameter value for display
func formatParamValue(v any) string {
	switch val := v.(type) {
	case string:
		// Truncate long strings and quote them
		if len(val) > 40 {
			val = val[:37] + "..."
		}
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%v", val)
	case float64:
		// JSON numbers are float64
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
	// Check for diff markers in any line
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "+ ") || strings.HasPrefix(trimmed, "- ") {
			return true
		}
	}
	return false
}

// Display limits for diff rendering
const (
	maxDisplayLines   = 50  // Max lines to render in diff view
	maxDisplayLineLen = 120 // Max chars per line in display
)

// truncateDisplayLine limits line length for display
func truncateDisplayLine(line string) string {
	if len(line) > maxDisplayLineLen {
		return line[:maxDisplayLineLen-3] + "..."
	}
	return line
}

// renderDiffResult renders a diff result with colored lines
func renderDiffResult(result string) tui.View {
	// Handle empty result
	if result == "" {
		return nil
	}

	lines := strings.Split(result, "\n")

	// Limit total lines to prevent UI issues
	truncated := false
	if len(lines) > maxDisplayLines {
		lines = lines[:maxDisplayLines]
		truncated = true
	}

	// Safety check - need at least one line to render
	if len(lines) == 0 {
		return nil
	}

	views := make([]tui.View, 0, len(lines)+2)

	// First line is the summary (e.g., "Added 1 line, removed 1 line")
	if len(lines) > 0 {
		views = append(views, tui.Text("  ⎿  %s", truncateDisplayLine(lines[0])).Muted())
	}

	// Render each diff line with appropriate coloring
	for i := 1; i < len(lines); i++ {
		line := truncateDisplayLine(lines[i])
		trimmed := strings.TrimSpace(lines[i])

		var lineView tui.View
		if strings.HasPrefix(trimmed, "+ ") {
			// Added line - green
			lineView = tui.Text("      %s", line).Success()
		} else if strings.HasPrefix(trimmed, "- ") {
			// Removed line - red
			lineView = tui.Text("      %s", line).Error()
		} else {
			// Context line - muted
			lineView = tui.Text("      %s", line).Muted()
		}
		views = append(views, lineView)
	}

	if truncated {
		views = append(views, tui.Text("      ... (output truncated)").Hint())
	}

	return tui.Stack(views...).Gap(0)
}

func (a *App) inputView() tui.View {
	// Check if we're in confirmation mode
	if a.confirm.Pending {
		return a.confirmView()
	}

	// Normal input mode - single line, compact
	prompt := tui.Text(" > ").Info().Bold()

	inputContent := tui.IfElse(a.processing,
		tui.Text("(processing...)").Muted(),
		tui.Text("%s█", a.input), // Static cursor (no blinking to reduce flicker)
	)

	return tui.Group(prompt, inputContent)
}

// autocompleteView renders the file autocomplete dropdown
func (a *App) autocompleteView() tui.View {
	if !a.autocomplete.Active || len(a.autocomplete.Matches) == 0 {
		return nil
	}

	// Fixed height of 10 lines
	const fixedHeight = 10
	views := make([]tui.View, fixedHeight)

	for i := 0; i < fixedHeight; i++ {
		if i < len(a.autocomplete.Matches) {
			path := a.autocomplete.Matches[i]
			prefix := "  "
			style := tui.NewStyle()
			if i == a.autocomplete.Selected {
				// Selected item: light purple
				prefix = "> "
				style = style.WithBold().WithFgRGB(tui.RGB{R: 180, G: 140, B: 220})
			} else {
				// Other items: light gray
				style = style.WithFgRGB(tui.RGB{R: 140, G: 140, B: 140})
			}

			// Truncate long paths
			displayPath := path
			if len(displayPath) > 60 {
				displayPath = "..." + displayPath[len(displayPath)-57:]
			}

			views[i] = tui.Text(" %s%s", prefix, displayPath).Style(style)
		} else {
			// Empty line to maintain fixed height
			views[i] = tui.Text("")
		}
	}

	return tui.Stack(views...).Gap(0)
}

func (a *App) confirmView() tui.View {
	// Compact confirmation prompt
	return tui.Group(
		tui.Text(" Confirm: ").Warning(),
		tui.Text("%s", a.confirm.ToolName).Bold(),
		tui.Text(" - %s ", a.confirm.Summary).Muted(),
		tui.Text("[y/n] ").Bold(),
	)
}

// promptView renders the current interactive prompt with prominent styling
func (a *App) promptView() tui.View {
	if a.confirm.Pending {
		return a.confirmPromptView()
	}
	if a.selectState.Pending {
		return a.selectPromptView()
	}
	if a.multiSelectState.Pending {
		return a.multiSelectPromptView()
	}
	if a.inputState.Pending {
		return a.inputPromptView()
	}
	return tui.Text("")
}

// confirmPromptView renders a prominent confirmation prompt
func (a *App) confirmPromptView() tui.View {
	title := a.confirm.Summary
	if title == "" {
		title = fmt.Sprintf("Execute %s?", a.confirm.ToolName)
	}

	return tui.Padding(1,
		tui.Stack(
			tui.Text(" CONFIRM ").Bold().
				Style(tui.NewStyle().WithBgRGB(tui.RGB{R: 200, G: 150, B: 50}).WithFgRGB(tui.RGB{R: 0, G: 0, B: 0})),
			tui.Text(""),
			tui.Text(" %s", title).Bold(),
			tui.Text(" Tool: %s", a.confirm.ToolName).Muted(),
			tui.Text(""),
			tui.Group(
				tui.Text(" Press "),
				tui.Text("y").Bold().Success(),
				tui.Text(" to confirm, "),
				tui.Text("n").Bold().Error(),
				tui.Text(" to cancel"),
			),
		),
	)
}

// selectPromptView renders a prominent single-select prompt
func (a *App) selectPromptView() tui.View {
	title := a.selectState.Title
	if title == "" {
		title = "Select an option"
	}

	views := []tui.View{
		tui.Text(" SELECT ").Bold().
			Style(tui.NewStyle().WithBgRGB(tui.RGB{R: 80, G: 150, B: 220}).WithFgRGB(tui.RGB{R: 255, G: 255, B: 255})),
		tui.Text(""),
		tui.Text(" %s", title).Bold(),
	}

	if a.selectState.Message != "" {
		views = append(views, tui.Text(" %s", a.selectState.Message).Muted())
	}
	views = append(views, tui.Text(""))

	// Render options
	for i, opt := range a.selectState.Options {
		prefix := "  "
		style := tui.NewStyle()
		if i == a.selectState.SelectedIdx {
			prefix = "> "
			style = style.WithBold().WithFgRGB(tui.RGB{R: 80, G: 200, B: 220})
		}

		optView := tui.Text(" %s%d) %s", prefix, i+1, opt.Label).Style(style)
		if opt.Description != "" && i == a.selectState.SelectedIdx {
			views = append(views, tui.Group(optView, tui.Text(" - %s", opt.Description).Muted()))
		} else {
			views = append(views, optView)
		}
	}

	views = append(views, tui.Text(""))
	views = append(views, tui.Text(" ↑/↓: navigate  Enter: select  Esc: cancel").Hint())

	return tui.Padding(1, tui.Stack(views...))
}

// multiSelectPromptView renders a prominent multi-select prompt
func (a *App) multiSelectPromptView() tui.View {
	title := a.multiSelectState.Title
	if title == "" {
		title = "Select options"
	}

	views := []tui.View{
		tui.Text(" MULTI-SELECT ").Bold().
			Style(tui.NewStyle().WithBgRGB(tui.RGB{R: 150, G: 80, B: 180}).WithFgRGB(tui.RGB{R: 255, G: 255, B: 255})),
		tui.Text(""),
		tui.Text(" %s", title).Bold(),
	}

	if a.multiSelectState.Message != "" {
		views = append(views, tui.Text(" %s", a.multiSelectState.Message).Muted())
	}

	// Show min/max constraints
	if a.multiSelectState.MinSelect > 0 || a.multiSelectState.MaxSelect > 0 {
		constraint := ""
		if a.multiSelectState.MaxSelect > 0 {
			constraint = fmt.Sprintf(" (select %d-%d)", a.multiSelectState.MinSelect, a.multiSelectState.MaxSelect)
		} else if a.multiSelectState.MinSelect > 0 {
			constraint = fmt.Sprintf(" (select at least %d)", a.multiSelectState.MinSelect)
		}
		views = append(views, tui.Text("%s", constraint).Muted())
	}
	views = append(views, tui.Text(""))

	// Render options with checkboxes
	for i, opt := range a.multiSelectState.Options {
		checkbox := "[ ]"
		if i < len(a.multiSelectState.Selected) && a.multiSelectState.Selected[i] {
			checkbox = "[x]"
		}

		prefix := "  "
		style := tui.NewStyle()
		if i == a.multiSelectState.CursorIdx {
			prefix = "> "
			style = style.WithBold().WithFgRGB(tui.RGB{R: 180, G: 120, B: 220})
		}

		optView := tui.Text(" %s%s %d) %s", prefix, checkbox, i+1, opt.Label).Style(style)
		if opt.Description != "" && i == a.multiSelectState.CursorIdx {
			views = append(views, tui.Group(optView, tui.Text(" - %s", opt.Description).Muted()))
		} else {
			views = append(views, optView)
		}
	}

	views = append(views, tui.Text(""))
	views = append(views, tui.Text(" ↑/↓: navigate  Space: toggle  Enter: confirm  Esc: cancel").Hint())

	return tui.Padding(1, tui.Stack(views...))
}

// inputPromptView renders a prominent text input prompt
func (a *App) inputPromptView() tui.View {
	title := a.inputState.Title
	if title == "" {
		title = "Enter input"
	}

	views := []tui.View{
		tui.Text(" INPUT ").Bold().
			Style(tui.NewStyle().WithBgRGB(tui.RGB{R: 80, G: 180, B: 120}).WithFgRGB(tui.RGB{R: 255, G: 255, B: 255})),
		tui.Text(""),
		tui.Text(" %s", title).Bold(),
	}

	if a.inputState.Message != "" {
		views = append(views, tui.Text(" %s", a.inputState.Message).Muted())
	}
	views = append(views, tui.Text(""))

	// Show input field with cursor
	inputDisplay := a.inputState.Value
	if inputDisplay == "" && a.inputState.Placeholder != "" {
		views = append(views, tui.Text(" > %s█", a.inputState.Placeholder).Muted())
	} else {
		views = append(views, tui.Text(" > %s█", inputDisplay))
	}

	if a.inputState.Default != "" && a.inputState.Value == "" {
		views = append(views, tui.Text(" (default: %s)", a.inputState.Default).Hint())
	}

	views = append(views, tui.Text(""))
	hint := " Enter: submit  Esc: cancel"
	if a.inputState.Multiline {
		hint = " Shift+Enter: newline  Enter: submit  Esc: cancel"
	}
	views = append(views, tui.Text("%s", hint).Hint())

	return tui.Padding(1, tui.Stack(views...))
}

func (a *App) footerView() tui.View {
	// Help hints based on current mode
	hints := tui.IfElse(a.confirm.Pending,
		tui.Text(" y/n: confirm ").Hint(),
		tui.Group(
			tui.Text(" Enter: send ").Hint(),
			tui.Text(" Shift+Enter: newline ").Hint(),
			tui.Text(" PgUp/PgDn: scroll ").Hint(),
		),
	)

	// Truncate workspace path if too long
	wsLabel := a.workspaceDir
	if len(wsLabel) > 30 {
		wsLabel = "..." + wsLabel[len(wsLabel)-27:]
	}

	return tui.Group(
		hints,
		tui.Spacer(),
		tui.Text(" Ctrl+C: exit ").Hint(),
		tui.Text(" %s ", wsLabel).Hint(),
	)
}

// todoStatusView renders the active todo status line like Claude Code
// Format: ✽ Updating formatToolCall for Bash… (ctrl+t to hide todos · 2m 57s)
func (a *App) todoStatusView() tui.View {
	// Find the in-progress todo
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

	// Calculate elapsed time
	elapsed := time.Since(a.processingStartTime)
	elapsedStr := formatDuration(elapsed)

	// Build the status line
	return tui.Group(
		tui.Text("✽").Animate(tui.Pulse(tui.NewRGB(180, 140, 220), 20).Brightness(0.4, 1.0)),
		tui.Text(" %s…", activeTodo.ActiveForm).Style(tui.NewStyle().WithFgRGB(tui.RGB{R: 180, G: 140, B: 220})),
		tui.Text(" (ctrl+t to hide todos · %s)", elapsedStr).Hint(),
	)
}

// todoListView renders the todo list with checkboxes
func (a *App) todoListView() tui.View {
	if len(a.todos) == 0 {
		return nil
	}

	views := make([]tui.View, 0, len(a.todos)+1)

	// Add the status line first
	if statusView := a.todoStatusView(); statusView != nil {
		views = append(views, statusView)
	}

	// Build the todo list
	for _, todo := range a.todos {
		var todoView tui.View
		switch todo.Status {
		case TodoStatusCompleted:
			// Muted checkbox + strikethrough content
			mutedStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 100, B: 100})
			todoView = tui.Group(
				tui.Text("  ⎿  ☒ ").Style(mutedStyle),
				tui.Text("%s", todo.Content).Style(mutedStyle.WithStrikethrough()),
			)
		case TodoStatusInProgress:
			// Purple, bold
			style := tui.NewStyle().WithFgRGB(tui.RGB{R: 180, G: 140, B: 220}).WithBold()
			todoView = tui.Text("  ⎿  ☐ %s", todo.Content).Style(style)
		default: // Pending
			// Slightly muted
			style := tui.NewStyle().WithFgRGB(tui.RGB{R: 140, G: 140, B: 140})
			todoView = tui.Text("  ⎿  ☐ %s", todo.Content).Style(style)
		}

		views = append(views, todoView)
	}

	return tui.Stack(views...).Gap(0)
}

// formatDuration formats a duration in a human-readable way (e.g., "2m 57s")
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
