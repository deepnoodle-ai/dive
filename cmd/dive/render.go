package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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

	return tui.Scroll(
		tui.Padding(1,
			tui.ForEach(a.messages, func(msg Message, i int) tui.View {
				return a.messageView(msg, i)
			}).Gap(1),
		),
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
	case "user":
		// User messages: italic with dark gray background
		return tui.Text("%s", msg.Content).Wrap().
			Style(tui.NewStyle().WithItalic().WithBgRGB(tui.RGB{R: 40, G: 40, B: 40})).
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

func (a *App) toolCallView(msg Message) tui.View {
	// Status indicator: ⏺ prefix with color based on state
	statusView := tui.IfElse(msg.ToolDone,
		tui.IfElse(msg.ToolError,
			tui.Text("⏺").Error(),
			tui.Text("⏺").Success(),
		),
		tui.Text("⏺").Info(),
	)

	// Format tool call like: ToolName(param: value, param: value)
	toolCall := formatToolCall(msg.ToolName, msg.ToolInput)
	callView := tui.Text(" %s", toolCall)

	// Header line: ⏺ ToolName(params...)
	header := tui.Group(
		statusView,
		callView,
	)

	// Result view (only shown when done with result)
	resultText := formatToolResult(msg.ToolResult)
	resultView := tui.If(msg.ToolDone && msg.ToolResult != "",
		tui.IfElse(msg.ToolError,
			tui.Text("  ⎿  %s", resultText).Error(),
			tui.Text("  ⎿  %s", resultText).Muted(),
		),
	)

	return tui.Stack(header, resultView).Gap(0)
}

// formatToolCall formats a tool call like: ToolName(param: value, param: value)
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

// formatToolResult formats a tool result for display
func formatToolResult(result string) string {
	// Count lines
	lines := strings.Count(result, "\n") + 1
	if lines == 1 && len(result) <= 60 {
		return result
	}

	// Summarize multi-line or long results
	if lines > 1 {
		return fmt.Sprintf("(%d lines)", lines)
	}
	return result[:57] + "..."
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
