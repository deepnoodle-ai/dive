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
		return tui.Stack(
			tui.Text("No messages yet.").Wrap().Muted().Center(),
		).Flex(1)
	}

	return tui.Stack(
		tui.Scroll(
			tui.Padding(1,
				tui.ForEach(a.messages, func(msg Message, i int) tui.View {
					return a.messageView(msg, i)
				}).Gap(1),
			),
			&a.scrollY,
		).Bottom(), // Chat-style scrolling - anchor to bottom
	).Flex(1)
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
