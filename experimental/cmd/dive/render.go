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
	// Speed badge — fast mode bills at a different rate, so surface it.
	if a.lastUsage != nil && a.lastUsage.Speed == "fast" {
		parts = append(parts, tui.Text(" ⚡fast").Style(
			tui.NewStyle().WithFgRGB(tui.RGB{R: 230, G: 190, B: 80}).WithBold()))
	}

	rows := []tui.View{tui.Group(parts...)}

	// Line 2: context-window usage bar (shown after the first LLM response).
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
		rows = append(rows, tui.Group(
			tui.Text(" ").Style(mutedStyle),
			bar,
			tui.Text(" %d%% context", contextPct).Style(mutedStyle),
		))
	}

	// Tokens panel: a clearly-labeled per-scope breakdown of input, cache
	// reads (hits) vs writes (misses), output, and hit rate.
	if panel := a.tokensPanelView(); panel != nil {
		rows = append(rows, panel)
	}

	if len(rows) == 1 {
		return rows[0]
	}
	return tui.Stack(rows...).Gap(0)
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

// tokensPanelView renders a clearly-labeled token + cache breakdown table, one
// row per scope (turn / session). It makes cache reads (hits, cheap) and cache
// writes (misses, premium) explicit and adds a per-scope hit rate, so cache
// thrash is immediately visible. Returns nil before the first response.
//
//	tokens       input   cache read   cache write   output    hit
//	turn          1.2k        13.5k          0.5k       53     96%
//	session       4.8k        54.0k          1.2k      892     98%
func (a *App) tokensPanelView() tui.View {
	turn := a.interactionUsage
	if turn == nil || !hasUsage(turn) {
		return nil
	}
	sess := a.sessionUsage
	showSession := sess != nil && hasUsage(sess) && a.sessionUsageDiffers()
	showReasoning := turn.ReasoningTokens > 0 || (showSession && sess.ReasoningTokens > 0)
	showCost := turn.Cost != nil || (showSession && sess.Cost != nil)

	headerStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 110, G: 110, B: 120})
	scopeStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 160, G: 160, B: 170}).WithBold()
	valStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 220, G: 220, B: 230}).WithBold()
	readStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 110, G: 200, B: 130}).WithBold() // green: cheap hits
	writeStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 225, G: 175, B: 80}).WithBold() // amber: premium writes
	costStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 120, G: 205, B: 150}).WithBold() // green: money

	const (
		labelW = 10
		inW    = 8
		readW  = 13
		writeW = 14
		outW   = 9
		reasW  = 9
		hitW   = 7
		costW  = 10
	)

	left := func(w int, s string, st tui.Style) tui.View {
		return tui.Width(w, tui.Stack(tui.Text("%s", s).Style(st)).Align(tui.AlignLeft))
	}
	right := func(w int, s string, st tui.Style) tui.View {
		return tui.Width(w, tui.Stack(tui.Text("%s", s).Style(st)).Align(tui.AlignRight))
	}

	header := []tui.View{
		left(labelW, " tokens", headerStyle),
		right(inW, "input", headerStyle),
		right(readW, "cache read", headerStyle),
		right(writeW, "cache write", headerStyle),
		right(outW, "output", headerStyle),
	}
	if showReasoning {
		header = append(header, right(reasW, "reason", headerStyle))
	}
	header = append(header, right(hitW, "hit", headerStyle))
	if showCost {
		header = append(header, right(costW, "cost", headerStyle))
	}

	dataRow := func(label string, u *llm.Usage) tui.View {
		cells := []tui.View{
			left(labelW, " "+label, scopeStyle),
			right(inW, formatTokenCount(u.InputTokens), valStyle),
			right(readW, formatTokenCount(u.CacheReadInputTokens), readStyle),
			right(writeW, formatTokenCount(u.CacheCreationInputTokens), writeStyle),
			right(outW, formatTokenCount(u.OutputTokens), valStyle),
		}
		if showReasoning {
			cells = append(cells, right(reasW, formatTokenCount(u.ReasoningTokens), valStyle))
		}
		rate, ok := cacheHitRate(u)
		cells = append(cells, right(hitW, rate, cacheHitStyle(u, ok)))
		if showCost {
			cells = append(cells, right(costW, costString(u), costStyle))
		}
		return tui.Group(cells...)
	}

	rows := []tui.View{
		tui.Group(header...),
		dataRow("turn", turn),
	}
	if showSession {
		rows = append(rows, dataRow("session", sess))
	}
	return tui.Stack(rows...).Gap(0)
}

// cacheHitRate returns the share of cacheable prompt tokens served from cache
// (reads) versus freshly written (writes), as a percentage string. ok is false
// when no caching occurred for the scope (zero denominator).
func cacheHitRate(u *llm.Usage) (string, bool) {
	denom := u.CacheReadInputTokens + u.CacheCreationInputTokens
	if denom == 0 {
		return "—", false
	}
	pct := (u.CacheReadInputTokens * 100) / denom
	return fmt.Sprintf("%d%%", pct), true
}

// cacheHitStyle colors the hit rate by health: green (>=80%), amber (50-79%),
// red (<50%, i.e. cache thrash). Muted when no caching occurred.
func cacheHitStyle(u *llm.Usage, ok bool) tui.Style {
	if !ok {
		return tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 100, B: 110})
	}
	denom := u.CacheReadInputTokens + u.CacheCreationInputTokens
	pct := (u.CacheReadInputTokens * 100) / denom
	switch {
	case pct >= 80:
		return tui.NewStyle().WithFgRGB(tui.RGB{R: 110, G: 200, B: 130}).WithBold()
	case pct >= 50:
		return tui.NewStyle().WithFgRGB(tui.RGB{R: 225, G: 175, B: 80}).WithBold()
	default:
		return tui.NewStyle().WithFgRGB(tui.RGB{R: 220, G: 90, B: 90}).WithBold()
	}
}

// costString renders a usage's estimated total cost, or an em dash when cost is
// unknown (no pricing for the model) — distinct from a known $0 (local models).
func costString(u *llm.Usage) string {
	if u.Cost == nil {
		return "—"
	}
	return formatCost(u.Cost.Total)
}

// formatCost formats a USD amount with precision that scales to the magnitude,
// so sub-cent estimates stay legible (e.g. "$0.0021", "$0.043", "$1.27").
func formatCost(c float64) string {
	switch {
	case c <= 0:
		return "$0"
	case c < 0.01:
		return fmt.Sprintf("$%.4f", c)
	case c < 1:
		return fmt.Sprintf("$%.3f", c)
	default:
		return fmt.Sprintf("$%.2f", c)
	}
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
		s.CacheCreationInputTokens != i.CacheCreationInputTokens ||
		s.ReasoningTokens != i.ReasoningTokens
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

	case "reasoning":
		content := strings.TrimRight(msg.Content, "\n")
		if content == "" {
			return nil
		}
		reasoningStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 125, G: 125, B: 138})
		return tui.Group(
			tui.Text("◌ ").Style(reasoningStyle),
			tui.Text("%s", content).Wrap().Style(reasoningStyle).Flex(1),
		)

	case "context":
		dimStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 100, B: 110})
		return tui.Group(
			tui.Text("↩ ").Style(dimStyle),
			tui.Text("%s", msg.Content).Wrap().Style(dimStyle),
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

	views := []tui.View{header}

	// While the tool is still running, surface its latest structured-progress
	// snapshot. This is the ReportProgress channel — distinct from the streamed
	// result line below — and is dropped once the tool completes.
	if !msg.ToolDone && msg.ToolProgress != "" {
		views = append(views, tui.Text("  ↳ %s", msg.ToolProgress).Hint())
	}

	if len(msg.ToolResultLines) > 0 {
		if resultView := a.formatToolResultView(msg); resultView != nil {
			views = append(views, resultView)
		}
	}

	if len(views) == 1 {
		return header
	}
	return tui.Stack(views...).Gap(0)
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

	case "reasoning":
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			return nil
		}
		reasoningStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 125, G: 125, B: 138})
		return tui.Group(
			tui.Text("◌ ").Style(reasoningStyle),
			tui.Text("%s", content).Wrap().Style(reasoningStyle).Flex(1),
		)

	case "context":
		dimStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 100, B: 110})
		return tui.Group(
			tui.Text("↩ ").Style(dimStyle),
			tui.Text("%s", msg.Content).Wrap().Style(dimStyle),
		)

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
