package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/deepnoodle-ai/wonton/tui"
)

// spinnerSpeed controls how many frames per spinner character change (higher = slower)
const spinnerSpeed = 12

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

	var status tui.View
	if a.thinking {
		status = tui.Loading(a.frame).Speed(spinnerSpeed).Label("thinking").Fg(tui.ColorCyan)
	} else if a.processing {
		status = tui.Text(" processing ").Dim()
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
		return tui.Stack(
			tui.WrappedText("No messages yet.").Dim().Center(),
		).Flex(1)
	}

	views := make([]tui.View, 0, len(a.messages))
	for i, msg := range a.messages {
		if v := a.messageView(msg, i); v != nil {
			views = append(views, v)
		}
	}

	return tui.Stack(
		tui.Scroll(
			tui.Stack(views...).Gap(1).Padding(1),
			&a.scrollY,
		),
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
		// User messages: italic with gray background
		return tui.WrappedText(msg.Content).
			Style(tui.NewStyle().WithItalic().WithBackground(tui.ColorBrightBlack)).
			FillBg()

	case "assistant":
		// Assistant messages: bullet prefix, markdown content
		content := strings.TrimRight(msg.Content, "\n")
		if content == "" {
			if a.thinking && index == a.streamingMessageIndex {
				return tui.Loading(a.frame).Speed(spinnerSpeed).Label("Thinking...")
			}
			// Skip empty assistant messages (e.g., before tool calls)
			return nil
		}
		// Prefix content with bullet point like Claude Code
		return tui.Markdown("⏺ "+content, nil)

	case "system":
		return tui.WrappedText(msg.Content).Fg(tui.ColorYellow)

	default:
		return tui.WrappedText(msg.Content)
	}
}

func (a *App) toolCallView(msg Message) tui.View {
	// Status indicator: ⏺ prefix with color based on state
	var statusView tui.View
	if msg.ToolDone {
		if msg.ToolError {
			statusView = tui.Text("⏺").Fg(tui.ColorRed)
		} else {
			statusView = tui.Text("⏺").Fg(tui.ColorGreen)
		}
	} else {
		statusView = tui.Text("⏺").Fg(tui.ColorCyan)
	}

	// Format tool call like: ToolName(param: value, param: value)
	toolCall := formatToolCall(msg.ToolName, msg.ToolInput)
	callView := tui.Text(" %s", toolCall)

	// Header line: ⏺ ToolName(params...)
	header := tui.Group(
		statusView,
		callView,
	)

	views := []tui.View{header}

	// Result (if done)
	if msg.ToolDone && msg.ToolResult != "" {
		resultText := formatToolResult(msg.ToolResult)
		var resultView tui.View
		if msg.ToolError {
			resultView = tui.Text("  ⎿  %s", resultText).Fg(tui.ColorRed)
		} else {
			resultView = tui.Text("  ⎿  %s", resultText).Dim()
		}
		views = append(views, resultView)
	}

	return tui.Stack(views...).Gap(0)
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
	prompt := tui.Text(" > ").Fg(tui.ColorCyan).Bold()

	inputText := a.input
	if a.processing {
		inputText = "(processing...)"
	}

	var inputContent tui.View
	if a.processing {
		inputContent = tui.Text("%s", inputText).Dim()
	} else {
		// Static cursor (no blinking to reduce flicker)
		inputContent = tui.Text("%s█", inputText)
	}

	return tui.Group(
		prompt,
		inputContent,
	)
}

func (a *App) confirmView() tui.View {
	// Compact confirmation prompt
	return tui.Group(
		tui.Text(" Confirm: ").Fg(tui.ColorYellow),
		tui.Text("%s", a.confirm.ToolName).Bold(),
		tui.Text(" - %s ", a.confirm.Summary).Dim(),
		tui.Text("[y/n] ").Bold(),
	)
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
