package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/deepnoodle-ai/dive/experimental/compaction"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/tui"
)

// Shared accent color palette (teal family, anchored to the splash diamond color)
var (
	accentBright = tui.RGB{R: 80, G: 200, B: 235}
	accentMid    = tui.RGB{R: 70, G: 175, B: 210}
	accentDim    = tui.RGB{R: 60, G: 155, B: 185}
	accentMuted  = tui.RGB{R: 55, G: 135, B: 165}
)

// diveMarkdownTheme returns a custom markdown theme with complementary colors
func diveMarkdownTheme() tui.MarkdownTheme {
	theme := tui.DefaultMarkdownTheme()

	// Cyan-blue headers with decreasing brightness for hierarchy
	theme.H1Style = tui.NewStyle().WithBold().WithUnderline().WithFgRGB(accentBright)
	theme.H2Style = tui.NewStyle().WithBold().WithFgRGB(accentMid)
	theme.H3Style = tui.NewStyle().WithBold().WithFgRGB(accentDim)
	theme.H4Style = tui.NewStyle().WithBold().WithFgRGB(accentMuted)

	// Light purple for inline code (like Claude Code)
	theme.CodeStyle = tui.NewStyle().WithFgRGB(tui.RGB{R: 180, G: 140, B: 220})

	return theme
}

// statusLineView renders the status line above the input area.
// Shows: model name, directory, git branch, context %, elapsed time.
func (a *App) statusLineView() tui.View {
	mutedColor := tui.RGB{R: 100, G: 100, B: 110}
	accentStyle := tui.NewStyle().WithFgRGB(accentDim)
	mutedStyle := tui.NewStyle().WithFgRGB(mutedColor)

	// Line 1: model in directory on branch
	parts := []tui.View{
		tui.Text(" %s", a.modelDisplayName()).Style(accentStyle),
	}
	dirName := filepath.Base(a.workspaceDir)
	parts = append(parts,
		tui.Text(" in ").Style(mutedStyle),
		tui.Text("%s", dirName).Style(tui.NewStyle().WithFgRGB(tui.RGB{R: 200, G: 200, B: 210}).WithBold()),
	)
	if branch := detectGitBranch(a.workspaceDir); branch != "" {
		parts = append(parts,
			tui.Text(" on ").Style(mutedStyle),
			tui.Text("%s", branch).Style(accentStyle),
		)
	}
	// Right-aligned token usage lines (turn above session)
	sectionStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 160, G: 160, B: 170}).WithBold()
	var usageLines []tui.View
	if u := a.interactionUsage; u != nil && hasUsage(u) {
		usageLines = append(usageLines, tui.Group(
			tui.Width(9, tui.Stack(tui.Text("turn").Style(sectionStyle)).Align(tui.AlignRight)),
			tui.Text("  "),
			usageView(u),
		))
	}
	if u := a.sessionUsage; u != nil && hasUsage(u) && a.sessionUsageDiffers() {
		usageLines = append(usageLines, tui.Group(
			tui.Width(9, tui.Stack(tui.Text("session").Style(sectionStyle)).Align(tui.AlignRight)),
			tui.Text("  "),
			usageView(u),
		))
	}

	// Line 2: progress bar with context %
	var line2LeftParts []tui.View
	line2LeftParts = append(line2LeftParts, tui.Text(" ").Style(mutedStyle))

	// Context usage progress bar (show after first LLM response)
	if a.lastUsage != nil {
		contextPct := a.contextPercent()
		barColor := accentDim
		if contextPct > 75 {
			barColor = tui.RGB{R: 200, G: 150, B: 60}
		}
		if contextPct > 90 {
			barColor = tui.RGB{R: 200, G: 70, B: 70}
		}
		bar := tui.Progress(contextPct, 100).
			Width(20).
			HidePercent().
			Style(tui.NewStyle().WithFgRGB(barColor)).
			EmptyStyle(tui.NewStyle().WithFgRGB(tui.RGB{R: 80, G: 80, B: 90}))
		line2LeftParts = append(line2LeftParts, bar)
		line2LeftParts = append(line2LeftParts, tui.Text(" %d%%", contextPct).Style(mutedStyle))
	}

	hasLine2Left := len(line2LeftParts) > 1
	hasUsageLines := len(usageLines) > 0

	if !hasLine2Left && !hasUsageLines {
		return tui.Group(parts...)
	}

	// Build the line 1 row: model info on left, turn usage on right
	line1Row := tui.Group(parts...)
	if len(usageLines) > 0 {
		line1Row = tui.Group(tui.Group(parts...), tui.Spacer(), usageLines[0], tui.Text(" "))
	}

	// Build the line 2 row: context bar on left, session usage on right
	if hasLine2Left || len(usageLines) > 1 {
		var line2Left tui.View
		if hasLine2Left {
			line2Left = tui.Group(line2LeftParts...)
		} else {
			line2Left = tui.Text("")
		}
		var line2Row tui.View
		if len(usageLines) > 1 {
			line2Row = tui.Group(line2Left, tui.Spacer(), usageLines[1], tui.Text(" "))
		} else {
			line2Row = line2Left
		}
		return tui.Stack(line1Row, line2Row).Gap(0)
	}

	return line1Row
}

// formatTokenCount formats a token count for display (e.g. 1234 -> "1.2k", 56 -> "56").
func formatTokenCount(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// hasUsage returns true if the usage has any non-zero token counts.
func hasUsage(u *llm.Usage) bool {
	return u.InputTokens > 0 || u.OutputTokens > 0 || u.CacheReadInputTokens > 0 || u.CacheCreationInputTokens > 0
}

// usageView renders a compact token usage display with right-aligned number columns:
//   "in:  13.7k  cache:  13.5k  out:    53"
func usageView(u *llm.Usage) tui.View {
	labelStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 100, B: 110})
	valueStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 220, G: 220, B: 230}).WithBold()
	totalIn := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	col := func(val string) tui.View {
		return tui.Width(6, tui.Stack(tui.Text("%s", val).Style(valueStyle)).Align(tui.AlignRight))
	}
	return tui.Group(
		tui.Text("in: ").Style(labelStyle), col(formatTokenCount(totalIn)),
		tui.Text("  cache: ").Style(labelStyle), col(formatTokenCount(u.CacheReadInputTokens)),
		tui.Text("  out: ").Style(labelStyle), col(formatTokenCount(u.OutputTokens)),
	)
}

// sessionUsageDiffers returns true if session usage differs from interaction usage
// (i.e. there have been multiple interactions). No point showing both if they're identical.
func (a *App) sessionUsageDiffers() bool {
	s, i := a.sessionUsage, a.interactionUsage
	if s == nil || i == nil {
		return false
	}
	return s.InputTokens != i.InputTokens ||
		s.OutputTokens != i.OutputTokens ||
		s.CacheReadInputTokens != i.CacheReadInputTokens ||
		s.CacheCreationInputTokens != i.CacheCreationInputTokens
}

// modelDisplayName returns a human-friendly model name.
func (a *App) modelDisplayName() string {
	if info := lookupModel(a.modelName); info != nil && info.Label != "" {
		return info.Label
	}
	return a.modelName
}

// contextPercent returns the context usage as a percentage (0-100), or 0 if unknown.
func (a *App) contextPercent() int {
	if a.lastUsage == nil || a.contextWindowMax <= 0 {
		return 0
	}
	tokens := compaction.CalculateContextTokens(a.lastUsage)
	pct := (tokens * 100) / a.contextWindowMax
	if pct > 100 {
		pct = 100
	}
	return pct
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
		bg := tui.RGB{R: 48, G: 48, B: 54}
		caret := tui.Text("❯ ").Style(
			tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 100, B: 110}).WithBgRGB(bg),
		)
		text := tui.Text("%s", msg.Content).Wrap().Style(
			tui.NewStyle().WithBgRGB(bg),
		)
		return tui.Group(caret, text, tui.Fill(' ').BgRGB(bg.R, bg.G, bg.B))

	case "assistant":
		content := strings.TrimRight(msg.Content, "\n")
		if content == "" {
			return nil
		}
		// Hanging indent: ⏺ prefix is fixed, text fills remaining width
		// so wrapped lines align to column 2 (like Claude Code)
		return tui.Group(
			tui.Text("⏺ "),
			tui.Text("%s", content).Wrap().Flex(1),
		)

	case "system":
		return tui.Text("%s", msg.Content).Wrap().Warning()

	default:
		return tui.Text("%s", msg.Content).Wrap()
	}
}

// introView renders a bordered splash header (Codex-style)
func (a *App) introView(msg Message) tui.View {
	// Parse lines from content: model, workspace, optional session info
	lines := strings.Split(msg.Content, "\n")
	model := ""
	workspace := ""
	var extras []string
	if len(lines) > 0 {
		model = lines[0]
	}
	if len(lines) > 1 {
		workspace = lines[1]
	}
	if len(lines) > 2 {
		extras = lines[2:]
	}

	titleStyle := tui.NewStyle().WithFgRGB(accentBright).WithBold()
	mutedStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 140, G: 140, B: 155})
	labelStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 100, B: 110})

	// Box content
	content := []tui.View{
		tui.Group(
			tui.Text("Dive").Style(titleStyle),
			tui.Text(" (v0.1.0)").Style(mutedStyle),
		),
		tui.Text(""),
		tui.Group(
			tui.Text("model:     ").Style(labelStyle),
			tui.Text("%s", model).Style(mutedStyle),
		),
		tui.Group(
			tui.Text("directory: ").Style(labelStyle),
			tui.Text("%s", workspace).Style(mutedStyle),
		),
	}

	for _, extra := range extras {
		if extra != "" {
			content = append(content, tui.Text("%s", extra).Style(mutedStyle))
		}
	}

	// Compute box width from longest visible line (using display width for Unicode)
	textWidths := []int{
		runewidth.StringWidth("Dive (v0.1.0)"),
		runewidth.StringWidth("model:     ") + runewidth.StringWidth(model),
		runewidth.StringWidth("directory: ") + runewidth.StringWidth(workspace),
	}
	for _, extra := range extras {
		textWidths = append(textWidths, runewidth.StringWidth(extra))
	}
	maxLen := 0
	for _, w := range textWidths {
		if w > maxLen {
			maxLen = w
		}
	}
	boxWidth := maxLen + 2 + 2 // 2 for border chars + 2 for inner spacing

	return tui.Width(boxWidth, tui.Bordered(tui.Stack(content...)).
		Border(&tui.RoundedBorder).
		BorderFg(tui.ColorBrightBlack))
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

	if len(msg.ToolResultLines) > 0 {
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
	// Special handling for Read tool - show line count
	if msg.ToolName == "Read" && msg.ToolReadLines > 0 {
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

	// Special handling for Bash tool
	if apiName == "Bash" {
		if cmd, ok := params["command"].(string); ok {
			displayCmd := cmd
			if len(displayCmd) > 100 {
				displayCmd = displayCmd[:97] + "..."
			}
			displayCmd = strings.ReplaceAll(displayCmd, "\n", " ")
			return title + "(" + displayCmd + ")"
		}
	}

	// Special handling for file tools - show as title(filepath)
	switch apiName {
	case "Read", "Write", "Edit":
		if filePath, ok := params["file_path"].(string); ok {
			return fmt.Sprintf("%s(%s)", title, filePath)
		}
		return title + "()"
	case "ListDirectory":
		if path, ok := params["path"].(string); ok {
			return fmt.Sprintf("%s(%s)", title, path)
		}
		return title + "()"
	}

	// Special handling for TodoWrite tool
	if apiName == "TodoWrite" {
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

// isDiffLine checks if a line is a diff addition or removal.
// Recognizes numbered format ("  42 + code" / "  42 - code") and plain
// preview format ("  + code" / "  - code"). Returns "+", "-", or "".
func isDiffLine(line string) string {
	trimmed := strings.TrimSpace(line)

	// Plain preview format from buildEditDiffPreview: "  + code" / "  - code"
	if strings.HasPrefix(trimmed, "+ ") || trimmed == "+" {
		return "+"
	}
	if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
		return "-"
	}

	// Numbered format: "  42 + code" / "  42 - code"
	for i, ch := range trimmed {
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == ' ' && i > 0 {
			rest := trimmed[i:]
			if strings.HasPrefix(rest, " + ") {
				return "+"
			}
			if strings.HasPrefix(rest, " - ") {
				return "-"
			}
		}
		break
	}
	return ""
}

// isDiffResult checks if the result looks like a diff output
func isDiffResult(result string) bool {
	lines := strings.Split(result, "\n")
	if len(lines) < 2 {
		return false
	}
	for _, line := range lines {
		if isDiffLine(line) != "" {
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

		var lineView tui.View
		switch isDiffLine(lines[i]) {
		case "+":
			lineView = tui.Text("      %s", line).Success()
		case "-":
			lineView = tui.Text("      %s", line).Error()
		default:
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
		bg := tui.RGB{R: 70, G: 70, B: 78}
		caret := tui.Text("❯ ").Style(
			tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 100, B: 110}).WithBgRGB(bg),
		)
		text := tui.Text("%s", msg.Content).Wrap().Style(
			tui.NewStyle().WithBgRGB(bg),
		)
		return tui.Group(caret, text, tui.Fill(' ').BgRGB(bg.R, bg.G, bg.B))

	case "assistant":
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			return nil
		}
		// Hanging indent: ⏺ overlaid at (0,0), markdown indented 2 spaces
		// so all lines (including wrapped) align to column 2 (like Claude Code)
		return tui.ZStack(
			tui.PaddingLTRB(2, 0, 0, 0, tui.Markdown(content, nil).Theme(diveMarkdownTheme())),
			tui.Text("⏺"),
		).Align(tui.AlignLeft)

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

	if len(msg.ToolResultLines) > 0 {
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
