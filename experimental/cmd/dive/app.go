package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/compaction"
	"github.com/deepnoodle-ai/dive/experimental/session"
	"github.com/deepnoodle-ai/dive/experimental/slashcmd"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/tui"
)

// Custom events for state changes from background goroutines.
// These are sent via runner.SendEvent() and handled in HandleEvent(),
// ensuring all state mutations happen in the main event loop goroutine.
// All events implement tui.Event by embedding baseEvent.

type baseEvent struct {
	time time.Time
}

func (e baseEvent) Timestamp() time.Time { return e.time }

func newBaseEvent() baseEvent { return baseEvent{time: time.Now()} }

type streamTextEvent struct {
	baseEvent
	text string
}

type toolCallEvent struct {
	baseEvent
	call *llm.ToolUseContent
}

type toolResultEvent struct {
	baseEvent
	result *dive.ToolCallResult
}

type processingStartEvent struct {
	baseEvent
	userInput string
	expanded  string
}

type processingEndEvent struct {
	baseEvent
	err error
}

type showDialogEvent struct {
	baseEvent
	dialog *DialogState
}

type hideDialogEvent struct {
	baseEvent
}

type initialPromptEvent struct {
	baseEvent
	prompt string
}

// compactionEvent is sent when context compaction occurs.
// This allows the UI to display compaction status and statistics.
type compactionEvent struct {
	baseEvent
	event *compaction.CompactionEvent
}

// MessageType distinguishes regular messages from tool calls
type MessageType int

const (
	MessageTypeText MessageType = iota
	MessageTypeToolCall
)

// TodoStatus represents the status of a todo item
type TodoStatus int

const (
	TodoStatusPending TodoStatus = iota
	TodoStatusInProgress
	TodoStatusCompleted
)

// Todo represents a task item
type Todo struct {
	Content    string     // What needs to be done (imperative: "Run tests")
	ActiveForm string     // Present continuous form (shown when active: "Running tests")
	Status     TodoStatus // pending, in_progress, completed
}

// Message represents a chat message
type Message struct {
	Role    string // "user", "assistant", "system", or "intro"
	Content string // Text content
	Time    time.Time
	Type    MessageType

	// Tool call fields (when Type == MessageTypeToolCall)
	ToolID          string
	ToolName        string
	ToolTitle       string   // Human-readable display name from tool annotations
	ToolInput       string   // Full JSON input for display formatting
	ToolResult      string   // Display summary (first line or truncated)
	ToolResultLines []string // Full result lines for expansion display
	ToolReadLines   int      // Line count for read_file results
	ToolError       bool
	ToolDone        bool
}

// DialogType represents the type of tool dialog
type DialogType int

const (
	DialogTypeConfirm DialogType = iota
	DialogTypeSelect
	DialogTypeMultiSelect
	DialogTypeInput
)

// ConfirmResult represents the result of a confirmation dialog
type ConfirmResult struct {
	Approved     bool   // User approved the action
	AllowSession bool   // User wants to allow all similar actions this session
	Feedback     string // User feedback when denied (for "tell what to do differently")
}

// SelectResult represents the result of a select dialog
type SelectResult struct {
	Index     int    // Selected option index (-1 for cancel)
	OtherText string // Custom text if user selected "Other"
}

// DialogState holds state for all tool dialogs
type DialogState struct {
	Type           DialogType
	Active         bool
	Title          string
	Message        string
	ContentPreview string

	// For confirm (Claude Code style)
	ConfirmChan              chan ConfirmResult
	ConfirmSelectedIdx       int    // Currently selected option (0=Yes, 1=Allow session, 2=Feedback)
	ConfirmFeedback          string // User feedback text (for PromptChoice input option)
	ConfirmToolCategoryLabel string // Human-readable label (e.g., "bash commands", "file edits")

	// For select (uses PromptChoice with "Other" input option)
	Options         []DialogOption
	DefaultIndex    int
	SelectIndex     int               // State for PromptChoice
	SelectOtherText string            // State for PromptChoice "Other" input
	SelectChan      chan SelectResult // Carries index and optional "Other" text

	// For multi-select
	MultiSelectChan    chan []int // nil for cancel
	MultiSelectChecked []bool     // State for CheckboxList
	MultiSelectCursor  int        // Cursor for CheckboxList

	// For input
	DefaultValue string
	InputValue   string
	InputChan    chan string // empty for cancel
}

// DialogOption represents an option in select/multi-select dialogs
type DialogOption struct {
	Label       string
	Description string
	Value       string
	Selected    bool
}

// App is the main CLI application.
// All state is accessed only from the main event loop goroutine (via LiveView/HandleEvent),
// except for immutable fields (agent, workspaceDir, modelName, runner).
// Background goroutines send events via runner.SendEvent() for state changes.
type App struct {
	agent         *dive.Agent
	sessionRepo   session.Repository
	workspaceDir  string
	modelName     string
	commandLoader *slashcmd.Loader

	// Session management
	resumeSessionID  string // Session ID to resume (from --resume flag)
	currentSessionID string // Current active session ID

	// InlineApp runner
	runner *tui.InlineApp

	// Input state
	inputText    string
	historyIndex int // -1 when not navigating history

	// Chat state
	messages              []Message
	streamingMessageIndex int
	currentMessage        *Message

	// Tool call tracking
	toolCallIndex      map[string]int
	toolTitles         map[string]string // tool name -> display title
	needNewTextMessage bool              // set after tool calls to create new text message

	// Command history
	history []string

	// UI state
	frame               uint64
	processing          bool
	processingStartTime time.Time

	// Todo list state
	todos     []Todo
	showTodos bool

	// Tool dialog state (confirmations, selections, input)
	dialogState *DialogState

	// Autocomplete state
	autocompleteMatches []string
	autocompleteIndex   int
	autocompletePrefix  string
	autocompleteType    string // "file" or "command"

	// Streaming text buffer (flushed on tick for batched updates)
	streamBuffer string

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Initial prompt to submit on startup
	initialPrompt string

	// Ctrl+C exit confirmation state
	lastCtrlC    time.Time
	showExitHint bool

	// Compaction configuration and state
	compactionConfig         *compaction.CompactionConfig
	lastCompactionEvent      *compaction.CompactionEvent
	compactionEventTime      time.Time
	showCompactionStats      bool
	compactionStatsStartTime time.Time
}

// NewApp creates a new CLI application
func NewApp(
	agent *dive.Agent,
	sessionRepo session.Repository,
	workspaceDir, modelName string,
	initialPrompt string,
	compactionConfig *compaction.CompactionConfig,
	resumeSessionID string,
	commandLoader *slashcmd.Loader,
) *App {
	ctx, cancel := context.WithCancel(context.Background())

	// Build tool name -> title map from agent's tools
	toolTitles := make(map[string]string)
	for _, tool := range agent.Tools() {
		title := tool.Name() // Default to name
		if annotations := tool.Annotations(); annotations != nil && annotations.Title != "" {
			title = annotations.Title
		}
		toolTitles[tool.Name()] = title
	}

	return &App{
		agent:            agent,
		sessionRepo:      sessionRepo,
		workspaceDir:     workspaceDir,
		modelName:        modelName,
		resumeSessionID:  resumeSessionID,
		commandLoader:    commandLoader,
		messages:         make([]Message, 0),
		toolCallIndex:    make(map[string]int),
		toolTitles:       toolTitles,
		history:          make([]string, 0),
		historyIndex:     -1,
		ctx:              ctx,
		cancel:           cancel,
		initialPrompt:    initialPrompt,
		compactionConfig: compactionConfig,
	}
}

// LiveView implements tui.InlineApplication - returns the live region view.
// Called only from the main event loop goroutine - no locking needed.
func (a *App) LiveView() tui.View {
	views := make([]tui.View, 0)

	// Show tool dialog if active
	if a.dialogState != nil && a.dialogState.Active {
		views = append(views, tui.Text(""))
		views = append(views, a.dialogView())
		return tui.Stack(views...)
	}

	// Show streaming content during processing
	if a.processing {
		views = append(views, tui.Text(""))
		liveContent := a.buildLiveView()
		if liveContent != nil {
			views = append(views, liveContent)
		}
	}

	// Show todos if visible and not processing
	if !a.processing && a.showTodos && len(a.todos) > 0 {
		views = append(views, tui.Text(""))
		views = append(views, a.todoListView())
	}

	// Always add spacing before divider (separates from scrollback or live content)
	views = append(views, tui.Text(""))

	// Input area
	views = append(views, tui.Divider())
	views = append(views,
		tui.InputField(&a.inputText).
			ID("main-input").
			Prompt(" > ").
			PromptStyle(tui.NewStyle().WithForeground(tui.ColorCyan)).
			Placeholder("Type a message... (@filename for autocomplete)").
			Multiline(true).
			MaxHeight(10).
			OnSubmit(func(value string) {
				a.submitInput(value)
			}),
	)
	views = append(views, tui.Divider())

	// Show autocomplete options, compaction stats, or exit hint below the bottom divider
	// Only reserve space when autocomplete is active (collapses otherwise)
	footerViews := make([]tui.View, 0, 8)

	if len(a.autocompleteMatches) > 0 {
		count := len(a.autocompleteMatches)
		if count > 8 {
			count = 8
		}
		prefix := "@"
		if a.autocompleteType == "command" {
			prefix = "/"
		}
		for i := 0; i < count; i++ {
			match := a.autocompleteMatches[i]
			if i == a.autocompleteIndex {
				footerViews = append(footerViews, tui.Text(" ❯ %s%s", prefix, match).Fg(tui.ColorCyan))
			} else {
				footerViews = append(footerViews, tui.Text("   %s%s", prefix, match).Hint())
			}
		}
		// Pad to 8 lines only when autocomplete is active (stable height during selection)
		for len(footerViews) < 8 {
			footerViews = append(footerViews, tui.Text(""))
		}
	} else if a.showCompactionStats && a.lastCompactionEvent != nil {
		footerViews = append(footerViews, tui.Group(
			tui.Text(" ⚡").Fg(tui.ColorYellow),
			tui.Text(" Context compacted:").Hint(),
			tui.Text(" %d → %d tokens", a.lastCompactionEvent.TokensBefore, a.lastCompactionEvent.TokensAfter),
			tui.Text(" (%d messages summarized)", a.lastCompactionEvent.MessagesCompacted).Hint(),
		))
	} else if a.showExitHint {
		footerViews = append(footerViews, tui.Text(" Press Ctrl+C again to exit").Hint())
	}

	// Minimum padding at bottom for visual breathing room
	for len(footerViews) < 2 {
		footerViews = append(footerViews, tui.Text(""))
	}

	views = append(views, footerViews...)

	if len(views) == 0 {
		return tui.Text("")
	}

	return tui.Stack(views...).Gap(0)
}

// Purple color for Claude Code style UI elements
var purpleColor = tui.RGB{R: 180, G: 140, B: 220}

// dialogView builds the view for tool dialogs
func (a *App) dialogView() tui.View {
	if a.dialogState == nil {
		return nil
	}

	views := []tui.View{}

	switch a.dialogState.Type {
	case DialogTypeConfirm:
		// Claude Code style confirmation dialog using PromptChoice
		purpleStyle := tui.NewStyle().WithFgRGB(purpleColor)

		// Upper divider (purple, dashed)
		views = append(views, tui.Divider().Char('╌').Style(purpleStyle))

		// Title (purple, bold)
		views = append(views, tui.Text(" %s", a.dialogState.Title).Style(purpleStyle.WithBold()))

		// Content preview with dashed dividers
		if a.dialogState.ContentPreview != "" {
			views = append(views, tui.Divider().Char('╌').Style(purpleStyle))
			for _, line := range strings.Split(a.dialogState.ContentPreview, "\n") {
				views = append(views, tui.Text(" %s", line).Hint())
			}
		}

		// Lower divider (purple, dashed)
		views = append(views, tui.Divider().Char('╌').Style(purpleStyle))

		// Question
		views = append(views, tui.Text(" Do you want to proceed?").Muted())

		// Build option labels
		allowLabel := "file edits" // Default fallback
		if a.dialogState.ConfirmToolCategoryLabel != "" {
			allowLabel = a.dialogState.ConfirmToolCategoryLabel
		}

		// Use PromptChoice for the selection UI
		confirmChan := a.dialogState.ConfirmChan
		views = append(views,
			tui.PromptChoice(&a.dialogState.ConfirmSelectedIdx, &a.dialogState.ConfirmFeedback).
				ID("confirm-dialog").
				Option("Yes").
				Option(fmt.Sprintf("Yes, allow all %s during this session", allowLabel)).
				InputOption("Tell Dive what to do differently...").
				CursorStyle(purpleStyle).
				HintText("").
				OnSelect(func(idx int, inputText string) {
					switch idx {
					case 0: // Yes
						confirmChan <- ConfirmResult{Approved: true}
					case 1: // Yes, allow all session
						confirmChan <- ConfirmResult{Approved: true, AllowSession: true}
					case 2: // Feedback
						if inputText != "" {
							confirmChan <- ConfirmResult{Approved: false, Feedback: inputText}
						} else {
							// Empty feedback treated as cancel
							confirmChan <- ConfirmResult{Approved: false}
						}
					}
					a.dialogState.Active = false
				}).
				OnCancel(func() {
					confirmChan <- ConfirmResult{Approved: false}
					a.dialogState.Active = false
				}),
		)

		views = append(views, tui.Text(""))
		views = append(views, tui.Text(" Esc to cancel").Hint())

		return tui.Stack(views...)

	case DialogTypeSelect:
		// Non-confirm dialogs use original header style
		views = append(views,
			tui.Divider(),
			tui.Text(" %s", a.dialogState.Title).Bold(),
		)
		if a.dialogState.Message != "" {
			views = append(views, tui.Text(" %s", a.dialogState.Message).Muted())
		}
		if a.dialogState.ContentPreview != "" {
			views = append(views, tui.Divider().Char('-'))
			for _, line := range strings.Split(a.dialogState.ContentPreview, "\n") {
				views = append(views, tui.Text(" %s", line).Hint())
			}
		}
		views = append(views, tui.Text(""))

		// Use PromptChoice with options + "Other" input option
		selectChan := a.dialogState.SelectChan
		numOptions := len(a.dialogState.Options)
		promptChoice := tui.PromptChoice(&a.dialogState.SelectIndex, &a.dialogState.SelectOtherText).
			ID("select-list")

		// Add all predefined options
		for _, opt := range a.dialogState.Options {
			label := opt.Label
			if opt.Description != "" {
				label = fmt.Sprintf("%s - %s", opt.Label, opt.Description)
			}
			promptChoice = promptChoice.Option(label)
		}

		// Add "Other" input option
		promptChoice = promptChoice.
			InputOption("Other...").
			OnSelect(func(idx int, inputText string) {
				if idx == numOptions {
					// User selected "Other" and typed custom text
					if inputText != "" {
						selectChan <- SelectResult{Index: -1, OtherText: inputText}
					} else {
						// Empty "Other" text treated as cancel
						selectChan <- SelectResult{Index: -1}
					}
				} else {
					selectChan <- SelectResult{Index: idx}
				}
				a.dialogState.Active = false
			}).
			OnCancel(func() {
				selectChan <- SelectResult{Index: -1}
				a.dialogState.Active = false
			})

		views = append(views, promptChoice)

		views = append(views, tui.Text(""))
		views = append(views, tui.Text(" Use arrow keys to navigate, Enter to select, Esc to cancel").Hint())

	case DialogTypeMultiSelect:
		// Header
		views = append(views,
			tui.Divider(),
			tui.Text(" %s", a.dialogState.Title).Bold(),
		)
		if a.dialogState.Message != "" {
			views = append(views, tui.Text(" %s", a.dialogState.Message).Muted())
		}
		views = append(views, tui.Text(""))

		// Build list items for CheckboxList
		items := make([]tui.ListItem, len(a.dialogState.Options))
		for i, opt := range a.dialogState.Options {
			label := opt.Label
			if opt.Description != "" {
				label = fmt.Sprintf("%s - %s", opt.Label, opt.Description)
			}
			items[i] = tui.ListItem{Label: label, Value: opt.Value}
		}

		views = append(views,
			tui.CheckboxList(items, a.dialogState.MultiSelectChecked, &a.dialogState.MultiSelectCursor).
				ID("multiselect-list"),
		)

		views = append(views, tui.Text(""))
		views = append(views, tui.Text(" Use arrow keys to navigate, Space to toggle, Enter to confirm, Esc to cancel").Hint())

	case DialogTypeInput:
		// Header
		views = append(views,
			tui.Divider(),
			tui.Text(" %s", a.dialogState.Title).Bold(),
		)
		if a.dialogState.Message != "" {
			views = append(views, tui.Text(" %s", a.dialogState.Message).Muted())
		}
		views = append(views, tui.Text(""))
		// Note: InputField handles its own Enter key via OnSubmit
		// Escape is handled by handleDialogKey
		inputChan := a.dialogState.InputChan
		defaultValue := a.dialogState.DefaultValue
		views = append(views,
			tui.InputField(&a.dialogState.InputValue).
				ID("dialog-input").
				Prompt(" > ").
				PromptStyle(tui.NewStyle().WithForeground(tui.ColorCyan)).
				Placeholder(defaultValue).
				Width(60).
				OnSubmit(func(value string) {
					if value == "" {
						value = defaultValue
					}
					inputChan <- value
					a.dialogState.Active = false
				}),
		)
		views = append(views, tui.Text(" Press Enter to confirm, Esc to cancel").Hint())
	}

	return tui.Stack(views...)
}

// HandleEvent implements tui.EventHandler - handles all events.
// Called only from the main event loop goroutine - no locking needed.
func (a *App) HandleEvent(event tui.Event) []tui.Cmd {
	switch e := event.(type) {
	case tui.KeyEvent:
		cmds := a.handleKeyEvent(e)
		// Update autocomplete after key events (input may have changed)
		a.updateAutocomplete()
		return cmds
	case tui.TickEvent:
		a.frame = e.Frame
		// Flush any buffered streaming text (batches updates to 30 FPS max)
		a.flushStreamBuffer()
		// Clear exit hint after 2 seconds
		if a.showExitHint && time.Since(a.lastCtrlC) >= 2*time.Second {
			a.showExitHint = false
		}
		// Clear compaction stats after 5 seconds
		if a.showCompactionStats && time.Since(a.compactionStatsStartTime) >= 5*time.Second {
			a.showCompactionStats = false
		}

	// Custom events from background goroutines
	case processingStartEvent:
		a.handleProcessingStart(e)
	case streamTextEvent:
		a.handleStreamText(e.text)
	case toolCallEvent:
		a.handleToolCall(e.call)
	case toolResultEvent:
		a.handleToolResult(e.result)
	case processingEndEvent:
		a.handleProcessingEnd(e.err)
	case compactionEvent:
		a.handleCompaction(e.event)
	case showDialogEvent:
		a.dialogState = e.dialog
		// Focus the dialog so it receives key events
		if e.dialog.Type == DialogTypeConfirm {
			return []tui.Cmd{tui.Focus("confirm-dialog")}
		} else if e.dialog.Type == DialogTypeSelect {
			return []tui.Cmd{tui.Focus("select-list")}
		} else if e.dialog.Type == DialogTypeMultiSelect {
			return []tui.Cmd{tui.Focus("multiselect-list")}
		} else if e.dialog.Type == DialogTypeInput {
			return []tui.Cmd{tui.Focus("dialog-input")}
		}
	case hideDialogEvent:
		a.dialogState = nil
		return []tui.Cmd{tui.Focus("main-input")}
	case initialPromptEvent:
		a.submitInput(e.prompt)
	}
	return nil
}

// handleDialogKey handles key events for dialogs
func (a *App) handleDialogKey(e tui.KeyEvent) []tui.Cmd {
	switch a.dialogState.Type {
	case DialogTypeConfirm:
		// PromptChoice handles most key events (arrows, numbers, Enter, Escape).
		// We avoid global y/n shortcuts as they interfere with free-form input.
		return nil

	case DialogTypeSelect:
		// PromptChoice handles all key events (arrows, numbers, Enter, Escape).
		return nil

	case DialogTypeMultiSelect:
		// CheckboxList handles arrow keys and Space
		switch {
		case e.Key == tui.KeyEscape:
			a.dialogState.MultiSelectChan <- nil
			a.dialogState.Active = false
		case e.Key == tui.KeyEnter:
			// Return selected indices based on CheckboxList state
			var selected []int
			for i, checked := range a.dialogState.MultiSelectChecked {
				if checked {
					selected = append(selected, i)
				}
			}
			a.dialogState.MultiSelectChan <- selected
			a.dialogState.Active = false
		case e.Rune >= '1' && e.Rune <= '9':
			// Shortcut to toggle items by number
			idx := int(e.Rune - '1')
			if idx < len(a.dialogState.MultiSelectChecked) {
				a.dialogState.MultiSelectChecked[idx] = !a.dialogState.MultiSelectChecked[idx]
			}
		}

	case DialogTypeInput:
		// Only handle Escape - Enter is handled by InputField's OnSubmit callback
		if e.Key == tui.KeyEscape {
			a.dialogState.InputChan <- ""
			a.dialogState.Active = false
		}
		// Other keys are handled by InputField
	}

	return nil
}

// updateAutocomplete updates autocomplete state based on current input
func (a *App) updateAutocomplete() {
	// Check for command autocomplete (/ at start of input)
	if strings.HasPrefix(a.inputText, "/") {
		prefix := a.inputText[1:]

		// Check if there's a space (command completed)
		if strings.Contains(prefix, " ") || strings.Contains(prefix, "\n") {
			a.clearAutocomplete()
			return
		}

		// Only update matches if prefix changed or type changed
		if prefix != a.autocompletePrefix || a.autocompleteType != "command" {
			a.autocompletePrefix = prefix
			a.autocompleteType = "command"
			a.autocompleteMatches = a.getCommandMatches(prefix)
			if len(a.autocompleteMatches) > 8 {
				a.autocompleteMatches = a.autocompleteMatches[:8]
			}
			a.autocompleteIndex = 0
		}
		return
	}

	// Check for file autocomplete (@ anywhere in input)
	lastAt := strings.LastIndex(a.inputText, "@")
	if lastAt < 0 {
		a.clearAutocomplete()
		return
	}

	// Extract prefix after @
	prefix := a.inputText[lastAt+1:]

	// Check if there's a space after the @ (autocomplete completed)
	if strings.Contains(prefix, " ") || strings.Contains(prefix, "\n") {
		a.clearAutocomplete()
		return
	}

	// Only update matches if prefix changed or type changed
	if prefix != a.autocompletePrefix || a.autocompleteType != "file" {
		a.autocompletePrefix = prefix
		a.autocompleteType = "file"
		a.autocompleteMatches = a.getFileMatches(prefix)
		if len(a.autocompleteMatches) > 8 {
			a.autocompleteMatches = a.autocompleteMatches[:8]
		}
		a.autocompleteIndex = 0
	}
}

// clearAutocomplete resets autocomplete state
func (a *App) clearAutocomplete() {
	a.autocompleteMatches = nil
	a.autocompleteIndex = 0
	a.autocompletePrefix = ""
	a.autocompleteType = ""
}

// selectAutocomplete selects the current autocomplete option
func (a *App) selectAutocomplete() bool {
	if len(a.autocompleteMatches) == 0 || a.autocompleteIndex >= len(a.autocompleteMatches) {
		return false
	}

	selected := a.autocompleteMatches[a.autocompleteIndex]

	if a.autocompleteType == "command" {
		// Replace entire input with selected command (add space for args)
		a.inputText = "/" + selected + " "
	} else {
		// Find the last @ and replace prefix with selected match
		lastAt := strings.LastIndex(a.inputText, "@")
		if lastAt >= 0 {
			a.inputText = a.inputText[:lastAt+1] + selected + " "
		}
	}

	a.clearAutocomplete()
	return true
}

// handleKeyEvent processes keyboard input
func (a *App) handleKeyEvent(e tui.KeyEvent) []tui.Cmd {
	// Handle dialogs
	if a.dialogState != nil && a.dialogState.Active {
		return a.handleDialogKey(e)
	}

	// Handle autocomplete navigation
	if len(a.autocompleteMatches) > 0 {
		switch e.Key {
		case tui.KeyArrowUp:
			if a.autocompleteIndex > 0 {
				a.autocompleteIndex--
			}
			return nil
		case tui.KeyArrowDown:
			if a.autocompleteIndex < len(a.autocompleteMatches)-1 {
				a.autocompleteIndex++
			}
			return nil
		case tui.KeyTab:
			a.selectAutocomplete()
			return nil
		case tui.KeyEscape:
			a.clearAutocomplete()
			return nil
		}
	}

	// Handle global keys
	switch e.Key {
	case tui.KeyCtrlC:
		if a.processing {
			a.cancel()
		} else {
			// Require two Ctrl+C presses within 2 seconds to exit
			now := time.Now()
			if a.showExitHint && now.Sub(a.lastCtrlC) < 2*time.Second {
				return []tui.Cmd{tui.Quit()}
			}
			a.lastCtrlC = now
			a.showExitHint = true
		}
		return nil
	case tui.KeyEscape:
		if a.processing {
			a.cancel()
		}
		return nil
	case tui.KeyArrowUp:
		// History navigation (only when no autocomplete and input is empty or navigating)
		if !a.processing && len(a.history) > 0 && len(a.autocompleteMatches) == 0 {
			if a.historyIndex < 0 {
				a.historyIndex = len(a.history) - 1
			} else if a.historyIndex > 0 {
				a.historyIndex--
			}
			if a.historyIndex >= 0 && a.historyIndex < len(a.history) {
				a.inputText = a.history[a.historyIndex]
			}
		}
		return nil
	case tui.KeyArrowDown:
		// History navigation (only when no autocomplete)
		if !a.processing && a.historyIndex >= 0 && len(a.autocompleteMatches) == 0 {
			a.historyIndex++
			if a.historyIndex >= len(a.history) {
				a.historyIndex = -1
				a.inputText = ""
			} else {
				a.inputText = a.history[a.historyIndex]
			}
		}
		return nil
	}

	return nil
}

// submitInput handles input submission
func (a *App) submitInput(value string) {
	// If autocomplete is active, select instead of submitting
	if len(a.autocompleteMatches) > 0 {
		a.selectAutocomplete()
		return
	}

	trimmed := strings.TrimSpace(value)
	if trimmed == "" || a.processing {
		return
	}

	a.inputText = "" // Clear input
	a.historyIndex = -1
	a.history = append(a.history, trimmed)

	// Handle commands
	if strings.HasPrefix(trimmed, "/") {
		if a.handleCommand(trimmed) {
			return
		}
	}

	// Process message asynchronously
	go a.processMessageAsync(trimmed)
}

// processMessageAsync handles message processing in background.
// Sends events to the main event loop instead of modifying state directly.
func (a *App) processMessageAsync(input string) {
	// Expand file references (uses only immutable workspaceDir)
	expanded, err := a.expandFileReferences(input)
	if err != nil {
		a.runner.Printf("Warning: %s", err.Error())
	}

	// Send start event to set up state in the main goroutine
	a.runner.SendEvent(processingStartEvent{baseEvent: newBaseEvent(), userInput: input, expanded: expanded})
	a.runAgent(expanded)
}

// processCommandAsync handles slash command processing in background.
// displayText is what the user typed (e.g., "/explain go.mod")
// expanded is the full prompt to send to the agent
func (a *App) processCommandAsync(displayText, expanded string) {
	// Expand file references in the expanded text
	expandedWithFiles, err := a.expandFileReferences(expanded)
	if err != nil {
		a.runner.Printf("Warning: %s", err.Error())
	}

	// Send start event with display text, but send expanded text to agent
	a.runner.SendEvent(processingStartEvent{baseEvent: newBaseEvent(), userInput: displayText, expanded: expandedWithFiles})
	a.runAgent(expandedWithFiles)
}

// runAgent runs the agent with the given input
func (a *App) runAgent(expanded string) {

	// Track the last usage from the final LLM call (for accurate context size)
	// resp.Usage is accumulated across tool iterations which inflates context size
	var lastUsage *llm.Usage

	// Determine session ID to use
	sessionID := a.currentSessionID
	if sessionID == "" && a.resumeSessionID != "" {
		sessionID = a.resumeSessionID
	}

	// Build options for CreateResponse
	opts := []dive.CreateResponseOption{
		dive.WithInput(expanded),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			// Send events for each agent response item
			switch item.Type {
			case dive.ResponseItemTypeModelEvent:
				if item.Event != nil && item.Event.Delta != nil {
					if text := item.Event.Delta.Text; text != "" {
						a.runner.SendEvent(streamTextEvent{baseEvent: newBaseEvent(), text: text})
					}
				}
			case dive.ResponseItemTypeMessage:
				// Track usage from each LLM response (last one = actual context size)
				if item.Usage != nil {
					lastUsage = item.Usage
				}
			case dive.ResponseItemTypeToolCall:
				a.runner.SendEvent(toolCallEvent{baseEvent: newBaseEvent(), call: item.ToolCall})
			case dive.ResponseItemTypeToolCallResult:
				a.runner.SendEvent(toolResultEvent{baseEvent: newBaseEvent(), result: item.ToolCallResult})
			}
			return nil
		}),
	}

	// Track if this is a new session (for metadata update)
	isNewSession := sessionID == ""

	// Create response with streaming
	resp, err := a.agent.CreateResponse(a.ctx, opts...)

	// Update session metadata for new sessions
	if err == nil && resp != nil && a.sessionRepo != nil && isNewSession && a.currentSessionID != "" {
		a.updateSessionMetadata(expanded)
	}

	// Check for compaction after successful response
	if err == nil && resp != nil && a.compactionConfig != nil && a.sessionRepo != nil {
		a.checkAndPerformCompaction(lastUsage)
	}

	// Send completion event
	a.runner.SendEvent(processingEndEvent{baseEvent: newBaseEvent(), err: err})
}

// updateSessionMetadata updates a session with workspace and title information
func (a *App) updateSessionMetadata(firstMessage string) {
	session, err := a.sessionRepo.GetSession(a.ctx, a.currentSessionID)
	if err != nil || session == nil {
		return
	}

	// Only update if metadata is not already set
	needsUpdate := false

	// Set workspace if not already set
	if session.Metadata == nil {
		session.Metadata = make(map[string]interface{})
	}
	if _, hasWorkspace := session.Metadata["workspace"]; !hasWorkspace {
		session.Metadata["workspace"] = a.workspaceDir
		needsUpdate = true
	}

	// Generate title from first message if not already set
	if session.Title == "" {
		session.Title = generateSessionTitle(firstMessage)
		needsUpdate = true
	}

	if needsUpdate {
		_ = a.sessionRepo.PutSession(a.ctx, session)
	}
}

// generateSessionTitle creates a short title from the first user message
func generateSessionTitle(message string) string {
	// Clean up the message
	title := strings.TrimSpace(message)

	// Remove file references
	title = strings.ReplaceAll(title, "<file path=", "")

	// Take first line only
	if idx := strings.Index(title, "\n"); idx > 0 {
		title = title[:idx]
	}

	// Truncate to reasonable length
	if len(title) > 60 {
		// Try to truncate at word boundary
		truncated := title[:60]
		if lastSpace := strings.LastIndex(truncated, " "); lastSpace > 40 {
			title = truncated[:lastSpace] + "..."
		} else {
			title = truncated + "..."
		}
	}

	if title == "" {
		title = "Untitled session"
	}

	return title
}

// checkAndPerformCompaction checks if compaction is needed and performs it.
// Uses lastUsage (from final LLM call) for accurate context size measurement.
func (a *App) checkAndPerformCompaction(lastUsage *llm.Usage) {
	if lastUsage == nil || a.currentSessionID == "" {
		return
	}

	// Get threshold
	threshold := a.compactionConfig.ContextTokenThreshold
	if threshold <= 0 {
		threshold = compaction.DefaultContextTokenThreshold
	}

	// Get the session to check message count
	session, err := a.sessionRepo.GetSession(a.ctx, a.currentSessionID)
	if err != nil || session == nil {
		return
	}

	// Check if compaction should trigger
	if !compaction.ShouldCompact(lastUsage, len(session.Messages), threshold) {
		return
	}

	// Get the model for compaction
	model := a.compactionConfig.Model
	if model == nil {
		model = a.agent.Model()
	}

	// Calculate tokens before (from last LLM call = actual context size)
	tokensBefore := compaction.CalculateContextTokens(lastUsage)

	// Perform compaction
	compactedMsgs, event, err := compaction.CompactMessages(
		a.ctx,
		model,
		session.Messages,
		"", // System prompt - can be empty for summary generation
		a.compactionConfig.SummaryPrompt,
		tokensBefore,
	)
	if err != nil {
		// Log warning but don't fail
		return
	}

	// Update session with compacted messages
	session.Messages = compactedMsgs

	if err := a.sessionRepo.PutSession(a.ctx, session); err != nil {
		return
	}

	// Send compaction event to UI
	a.runner.SendEvent(compactionEvent{baseEvent: newBaseEvent(), event: event})
}

// Event handlers for background goroutine events

func (a *App) handleProcessingStart(e processingStartEvent) {
	userInput := strings.TrimSpace(e.userInput)

	// Add user message
	userMsg := Message{
		Role:    "user",
		Content: userInput,
		Time:    time.Now(),
		Type:    MessageTypeText,
	}
	a.messages = append(a.messages, userMsg)

	// Print user message to scrollback
	a.runner.Print(tui.Stack(tui.Text(""), a.textMessageView(userMsg, len(a.messages)-1)))

	// Prepare for streaming response
	a.currentMessage = &Message{
		Role:    "assistant",
		Content: "",
		Time:    time.Now(),
		Type:    MessageTypeText,
	}
	a.messages = append(a.messages, *a.currentMessage)
	a.streamingMessageIndex = len(a.messages) - 1
	a.needNewTextMessage = false

	a.processing = true
	a.processingStartTime = time.Now()
}

func (a *App) handleStreamText(text string) {
	// Buffer text for batched updates (flushed on tick)
	a.streamBuffer += text
}

func (a *App) flushStreamBuffer() {
	if a.streamBuffer == "" {
		return
	}

	needNewMessage := a.streamingMessageIndex < 0 ||
		a.streamingMessageIndex >= len(a.messages) ||
		a.needNewTextMessage

	if needNewMessage {
		a.messages = append(a.messages, Message{
			Role:    "assistant",
			Content: "",
			Time:    time.Now(),
			Type:    MessageTypeText,
		})
		a.streamingMessageIndex = len(a.messages) - 1
		a.needNewTextMessage = false
	}

	a.messages[a.streamingMessageIndex].Content += a.streamBuffer
	a.streamBuffer = ""
}

func (a *App) handleToolCall(call *llm.ToolUseContent) {
	// Parse TodoWrite tool calls
	if call.Name == "TodoWrite" {
		a.parseTodoWriteInput(call.Input)
	}

	// Look up display title, default to name if not found
	toolTitle := call.Name
	if title, ok := a.toolTitles[call.Name]; ok {
		toolTitle = title
	}

	msg := Message{
		Role:      "assistant",
		Time:      time.Now(),
		Type:      MessageTypeToolCall,
		ToolID:    call.ID,
		ToolName:  call.Name,
		ToolTitle: toolTitle,
		ToolInput: string(call.Input),
		ToolDone:  false,
	}
	a.messages = append(a.messages, msg)
	a.toolCallIndex[call.ID] = len(a.messages) - 1
	a.needNewTextMessage = true
}

func (a *App) handleToolResult(result *dive.ToolCallResult) {
	if idx, ok := a.toolCallIndex[result.ID]; ok && idx < len(a.messages) {
		a.messages[idx].ToolDone = true
		if result.Result != nil {
			if result.Result.IsError {
				a.messages[idx].ToolError = true
			}
			display := result.Result.Display
			var textContent string
			if len(result.Result.Content) > 0 {
				for _, c := range result.Result.Content {
					if c.Type == dive.ToolResultContentTypeText {
						textContent = c.Text
						break
					}
				}
			}
			if display == "" {
				display = textContent
			}
			if display != "" {
				a.messages[idx].ToolResultLines = strings.Split(display, "\n")
			}
			if len(a.messages[idx].ToolResultLines) > 0 {
				a.messages[idx].ToolResult = a.messages[idx].ToolResultLines[0]
			}
			// For Read tool, count lines in the actual content
			if strings.ToLower(a.messages[idx].ToolName) == "read" && textContent != "" {
				a.messages[idx].ToolReadLines = strings.Count(textContent, "\n") + 1
			}
		}
	}
}

// handleCompaction processes a compaction event and updates UI state.
// The compaction notification is shown in the live view for 3 seconds,
// and detailed stats are displayed in the footer for 5 seconds.
func (a *App) handleCompaction(event *compaction.CompactionEvent) {
	a.lastCompactionEvent = event
	a.compactionEventTime = time.Now()
	a.showCompactionStats = true
	a.compactionStatsStartTime = time.Now()
}

func (a *App) handleProcessingEnd(err error) {
	// Flush any remaining buffered text
	a.flushStreamBuffer()

	if err != nil && err != context.Canceled {
		errMsg := Message{
			Role:    "system",
			Content: "Error: " + err.Error(),
			Time:    time.Now(),
			Type:    MessageTypeText,
		}
		a.messages = append(a.messages, errMsg)
	}

	// Clear processing state BEFORE printing to scrollback.
	// This ensures the live view rendered inside Print() matches the final state
	// (without thinking animation), preventing orphaned blank lines.
	a.processing = false
	a.currentMessage = nil
	a.toolCallIndex = make(map[string]int)

	// Now print to scrollback - the live view will be re-rendered with correct height
	a.printRecentMessagesToScrollback()
}

// printRecentMessagesToScrollback prints the messages from current interaction to scrollback
func (a *App) printRecentMessagesToScrollback() {
	// Find start of current interaction
	startIdx := 0
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].Role == "user" {
			startIdx = i + 1 // Start after user message (already printed)
			break
		}
	}

	// Collect all message views (add blank line before each)
	messageViews := []tui.View{}
	for i := startIdx; i < len(a.messages); i++ {
		msg := a.messages[i]
		view := a.messageViewStatic(msg)
		if view != nil {
			messageViews = append(messageViews, tui.Text(""), view)
		}
	}

	// Print all messages as a single view
	if len(messageViews) > 0 {
		a.runner.Print(tui.Stack(messageViews...))
	}
}

// Run starts the CLI application
func (a *App) Run() error {
	// Create InlineApp runner with 30 FPS for animations
	a.runner = tui.NewInlineApp(tui.InlineAppConfig{
		FPS:            30,
		BracketedPaste: true,
		KittyKeyboard:  true,
	})

	// If resuming a session, print the conversation history instead of the intro
	if a.resumeSessionID != "" {
		a.printSessionHistoryToScrollback()
	} else {
		// Print intro to scrollback for new sessions
		a.printIntroToScrollback()
	}

	// Submit initial prompt if provided (after a brief delay to let the UI initialize)
	if a.initialPrompt != "" {
		go func() {
			time.Sleep(50 * time.Millisecond)
			a.runner.SendEvent(initialPromptEvent{baseEvent: newBaseEvent(), prompt: a.initialPrompt})
		}()
	}

	// Run the inline app (blocks until quit)
	return a.runner.Run(a)
}

// printIntroToScrollback prints the intro/splash screen to scrollback.
// Uses tui.Print directly - suitable for startup before the runner event loop starts.
func (a *App) printIntroToScrollback() {
	view := a.buildIntroView()
	tui.Print(tui.PaddingHV(1, 0, view))
}

// printIntroViaRunner prints the intro/splash screen using the runner's Print method.
// Uses a.runner.Print - required when the runner event loop is active (e.g., after /clear).
func (a *App) printIntroViaRunner() {
	view := a.buildIntroView()
	a.runner.Print(tui.PaddingHV(1, 0, view))
}

// buildIntroView creates the intro view and adds it to messages.
func (a *App) buildIntroView() tui.View {
	// Shorten workspace path for display
	wsDisplay := a.workspaceDir
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(wsDisplay, home) {
		wsDisplay = "~" + wsDisplay[len(home):]
	}

	// Build intro content
	content := a.modelName + "\n" + wsDisplay

	// Add session info if resuming
	if a.resumeSessionID != "" {
		content += "\nResuming session: " + a.resumeSessionID
	}

	// Build intro message
	msg := Message{
		Role:    "intro",
		Content: content,
		Time:    time.Now(),
		Type:    MessageTypeText,
	}

	a.messages = append(a.messages, msg)

	return a.introView(msg)
}

// printSessionHistoryToScrollback prints the conversation history from a resumed session
// to the scrollback buffer, so it appears as if continuing from where we left off.
func (a *App) printSessionHistoryToScrollback() {
	if a.resumeSessionID == "" {
		return
	}
	if a.sessionRepo == nil {
		return
	}

	// Load the session from the repository
	session, err := a.sessionRepo.GetSession(a.ctx, a.resumeSessionID)
	if err != nil {
		return
	}

	// Build a map of tool use ID -> tool result for matching
	toolResults := make(map[string]*llm.ToolResultContent)
	for _, msg := range session.Messages {
		for _, content := range msg.Content {
			if result, ok := content.(*llm.ToolResultContent); ok {
				toolResults[result.ToolUseID] = result
			}
		}
	}

	// Convert and print each message
	messageViews := []tui.View{}
	for _, msg := range session.Messages {
		views := a.convertLLMMessageToViews(msg, toolResults)
		messageViews = append(messageViews, views...)
	}

	// Print all messages as a single view
	if len(messageViews) > 0 {
		tui.Print(tui.Stack(messageViews...))
	}
}

// convertLLMMessageToViews converts an llm.Message to app Message views for display.
func (a *App) convertLLMMessageToViews(msg *llm.Message, toolResults map[string]*llm.ToolResultContent) []tui.View {
	var views []tui.View

	for _, content := range msg.Content {
		switch c := content.(type) {
		case *llm.TextContent:
			if c.Text == "" {
				continue
			}
			appMsg := Message{
				Role:    string(msg.Role),
				Content: c.Text,
				Time:    time.Now(),
				Type:    MessageTypeText,
			}
			view := a.textMessageViewStatic(appMsg)
			if view != nil {
				views = append(views, tui.Text(""), view)
			}

		case *llm.ToolUseContent:
			// Find the corresponding result if available
			result := toolResults[c.ID]

			toolTitle := a.toolTitles[c.Name]
			if toolTitle == "" {
				toolTitle = c.Name
			}

			appMsg := Message{
				Role:      "assistant",
				Time:      time.Now(),
				Type:      MessageTypeToolCall,
				ToolID:    c.ID,
				ToolName:  c.Name,
				ToolTitle: toolTitle,
				ToolInput: string(c.Input),
				ToolDone:  result != nil,
			}

			// If we have a result, extract its content
			if result != nil {
				appMsg.ToolError = result.IsError
				resultText := extractToolResultText(result)
				if resultText != "" {
					appMsg.ToolResultLines = strings.Split(resultText, "\n")
					if len(appMsg.ToolResultLines) > 0 {
						appMsg.ToolResult = appMsg.ToolResultLines[0]
					}
				}
			}

			view := a.toolCallViewStatic(appMsg)
			if view != nil {
				views = append(views, tui.Text(""), view)
			}

		case *llm.SummaryContent:
			// Show compaction summaries as system messages
			if c.Summary != "" {
				appMsg := Message{
					Role:    "system",
					Content: "[Context compacted]",
					Time:    time.Now(),
					Type:    MessageTypeText,
				}
				view := a.textMessageViewStatic(appMsg)
				if view != nil {
					views = append(views, tui.Text(""), view)
				}
			}
		}
	}

	return views
}

// extractToolResultText extracts text content from a tool result.
func extractToolResultText(result *llm.ToolResultContent) string {
	if result.Content == nil {
		return ""
	}

	switch content := result.Content.(type) {
	case string:
		return content
	case []byte:
		return string(content)
	case []interface{}:
		// Array of content objects
		var texts []string
		for _, item := range content {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["type"].(string); ok && t == "text" {
					if text, ok := m["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		return strings.Join(texts, "\n")
	default:
		// Try JSON marshaling as fallback
		data, err := json.Marshal(content)
		if err != nil {
			return ""
		}
		return string(data)
	}
}

func (a *App) handleCommand(input string) bool {
	// Parse command name and arguments
	parts := strings.SplitN(strings.TrimPrefix(input, "/"), " ", 2)
	cmdName := parts[0]
	var cmdArgs string
	if len(parts) > 1 {
		cmdArgs = parts[1]
	}

	// Handle built-in commands first
	switch cmdName {
	case "quit", "exit", "q":
		a.runner.Stop()
		return true

	case "clear":
		// Clear scrollback
		a.runner.ClearScrollback()

		// Reset conversation state by deleting the current session
		if a.currentSessionID != "" && a.sessionRepo != nil {
			if err := a.sessionRepo.DeleteSession(a.ctx, a.currentSessionID); err != nil {
				// Log error but continue
				a.runner.Printf("Warning: failed to clear conversation: %v", err)
			}
		}

		// Clear local message state and reset session
		a.messages = make([]Message, 0)
		a.todos = nil
		a.toolCallIndex = make(map[string]int)
		a.needNewTextMessage = false
		a.currentMessage = nil
		a.streamingMessageIndex = 0
		a.currentSessionID = ""

		// Show fresh intro using runner.Print (not tui.Print) since we're
		// in the middle of the running InlineApp event loop
		a.printIntroViaRunner()
		return true

	case "compact":
		a.handleCompactCommand()
		return true

	case "todos", "t":
		a.showTodos = !a.showTodos
		if a.showTodos {
			a.printTodosToScrollback()
		}
		return true

	case "help", "?":
		a.printHelp()
		return true
	}

	// Check for custom slash commands
	if a.commandLoader != nil {
		if cmd, ok := a.commandLoader.GetCommand(cmdName); ok {
			// Expand argument placeholders
			expanded := cmd.ExpandArguments(cmdArgs)

			// Warn about unsupported model override (agent doesn't support per-request model changes)
			if cmd.Model != "" {
				a.runner.Printf("Note: Model override '%s' specified but not yet supported in CLI", cmd.Model)
			}

			// Build display text with command name and args
			displayCmd := "/" + cmdName
			if cmdArgs != "" {
				displayCmd = "/" + cmdName + " " + cmdArgs
			}

			// Send the expanded instructions to the agent (async like regular messages)
			go a.processCommandAsync(displayCmd, expanded)
			return true
		}
	}

	// Unknown command - show error
	a.runner.Printf("Unknown command: /%s (try /help)", cmdName)
	return true
}

// handleCompactCommand performs manual compaction of the conversation
func (a *App) handleCompactCommand() {
	if a.compactionConfig == nil {
		a.runner.Printf("Compaction is disabled.")
		return
	}

	if a.currentSessionID == "" || a.sessionRepo == nil {
		a.runner.Printf("No conversation to compact.")
		return
	}

	// Get current session
	session, err := a.sessionRepo.GetSession(a.ctx, a.currentSessionID)
	if err != nil {
		a.runner.Printf("No conversation to compact.")
		return
	}

	if len(session.Messages) < 2 {
		a.runner.Printf("Not enough messages to compact (need at least 2).")
		return
	}

	a.runner.Printf("Compacting conversation...")

	// Calculate tokens before compaction (estimate)
	tokensBefore := 0
	for _, msg := range session.Messages {
		for _, c := range msg.Content {
			if tc, ok := c.(*llm.TextContent); ok && tc.Text != "" {
				tokensBefore += len(tc.Text) / 4 // rough estimate
			}
		}
	}

	// Perform compaction
	summaryPrompt := a.compactionConfig.SummaryPrompt
	if summaryPrompt == "" {
		summaryPrompt = compaction.DefaultCompactionSummaryPrompt
	}
	compactedMsgs, event, err := compaction.CompactMessages(
		a.ctx,
		a.compactionConfig.Model,
		session.Messages,
		"", // System prompt not needed for compaction
		summaryPrompt,
		tokensBefore,
	)
	if err != nil {
		a.runner.Printf("Compaction failed: %v", err)
		return
	}

	// Update session with compacted messages
	session.Messages = compactedMsgs
	if err := a.sessionRepo.PutSession(a.ctx, session); err != nil {
		a.runner.Printf("Failed to save compacted session: %v", err)
		return
	}

	// Show stats
	a.runner.Printf("Compacted: ~%d -> ~%d tokens", event.TokensBefore, event.TokensAfter)
}

func (a *App) printHelp() {
	views := []tui.View{
		tui.Text(""),
		tui.Text("Built-in Commands:").Bold(),
		tui.Text("  /quit, /q      Exit"),
		tui.Text("  /clear         Clear conversation and screen"),
		tui.Text("  /compact       Compact conversation to save context"),
		tui.Text("  /todos, /t     Toggle todo list"),
		tui.Text("  /help, /?      Show this help"),
	}

	// List custom commands if any
	if a.commandLoader != nil && a.commandLoader.CommandCount() > 0 {
		views = append(views,
			tui.Text(""),
			tui.Text("Custom Commands:").Bold(),
		)
		for _, cmd := range a.commandLoader.ListCommands() {
			line := fmt.Sprintf("  /%s", cmd.Name)
			if cmd.ArgumentHint != "" {
				line += " " + cmd.ArgumentHint
			}
			views = append(views, tui.Text("%s", line))
			if cmd.Description != "" {
				views = append(views, tui.Text("      %s", cmd.Description).Hint())
			}
		}
	}

	views = append(views,
		tui.Text(""),
		tui.Text("Input:").Bold(),
		tui.Text("  @filename      Include file contents"),
		tui.Text("  Enter          Send message"),
		tui.Text("  Shift+Enter    New line"),
		tui.Text("  Ctrl+C twice   Exit"),
		tui.Text(""),
	)

	a.runner.Print(tui.Stack(views...))
}

func (a *App) printTodosToScrollback() {
	if len(a.todos) == 0 {
		a.runner.Printf("No todos.")
		return
	}

	view := a.todoListViewStatic()
	if view != nil {
		a.runner.Print(view)
	}
}

// buildLiveView creates the view for live updates during streaming.
// Shows a simple progress indicator to keep the live region height stable.
// Full markdown is rendered to scrollback when the response completes.
func (a *App) buildLiveView() tui.View {
	views := make([]tui.View, 0)

	elapsed := time.Since(a.processingStartTime)

	// Show recent tool calls (last 3 max, in chronological order)
	// Includes both completed and in-progress tool calls so users can see
	// long-running operations like subagent tasks while they're working
	var toolViews []tui.View
	for i := len(a.messages) - 1; i >= 0; i-- {
		msg := a.messages[i]
		if msg.Role == "user" {
			break
		}
		if msg.Type == MessageTypeToolCall {
			view := a.toolCallView(msg)
			if view != nil {
				toolViews = append([]tui.View{view}, toolViews...) // prepend for chronological order
			}
			if len(toolViews) >= 3 {
				break
			}
		}
	}
	views = append(views, toolViews...)

	// Show compaction notification (if recent)
	if a.lastCompactionEvent != nil && time.Since(a.compactionEventTime) < 3*time.Second {
		views = append(views, tui.Group(
			tui.Text("⚡").Fg(tui.ColorYellow),
			tui.Text(" Context compacted").Fg(tui.ColorYellow),
			tui.Text(" %d → %d tokens, %d messages summarized",
				a.lastCompactionEvent.TokensBefore,
				a.lastCompactionEvent.TokensAfter,
				a.lastCompactionEvent.MessagesCompacted).Hint(),
		))
	}

	// Show generation progress indicator (below tool calls)
	if a.streamingMessageIndex >= 0 {
		views = append(views, tui.Group(
			tui.Loading(a.frame).CharSet(tui.SpinnerBounce.Frames).Speed(6).Fg(tui.ColorCyan),
			tui.Text(" thinking").Animate(tui.Slide(3, tui.NewRGB(80, 80, 80), tui.NewRGB(80, 200, 220))),
			tui.Text(" (%s)", formatDuration(elapsed)).Hint(),
			tui.Text("  ").Hint(),
			tui.Text("esc to interrupt").Hint(),
		))
	}

	// Show todos if active
	if a.showTodos && len(a.todos) > 0 {
		views = append(views, a.todoListView())
	}

	if len(views) == 0 {
		return tui.Text("")
	}

	// Use only horizontal padding to avoid extra lines when live view is removed
	return tui.PaddingLTRB(1, 0, 1, 0, tui.Stack(views...).Gap(1))
}

func (a *App) parseTodoWriteInput(input []byte) {
	var todoInput struct {
		Todos []struct {
			Content    string `json:"content"`
			Status     string `json:"status"`
			ActiveForm string `json:"activeForm"`
		} `json:"todos"`
	}

	if err := json.Unmarshal(input, &todoInput); err != nil {
		return
	}

	a.todos = make([]Todo, 0, len(todoInput.Todos))
	for _, t := range todoInput.Todos {
		status := TodoStatusPending
		switch t.Status {
		case "in_progress":
			status = TodoStatusInProgress
		case "completed":
			status = TodoStatusCompleted
		}
		a.todos = append(a.todos, Todo{
			Content:    t.Content,
			ActiveForm: t.ActiveForm,
			Status:     status,
		})
	}
	if len(a.todos) > 0 {
		a.showTodos = true
	}
}

// expandFileReferences expands @filepath references in the input to file contents
func (a *App) expandFileReferences(input string) (string, error) {
	// Match @ followed by path characters (exclude common punctuation that might follow)
	re := regexp.MustCompile(`@([^\s@]+)`)

	var lastErr error
	result := re.ReplaceAllStringFunc(input, func(match string) string {
		path := match[1:] // Remove @

		// Strip trailing punctuation that's likely not part of the filename
		path, trailing := stripTrailingPunctuation(path)

		// Only allow relative paths
		if filepath.IsAbs(path) {
			lastErr = fmt.Errorf("absolute paths not allowed: %s", path)
			return match
		}

		// Resolve path relative to workspace
		fullPath := filepath.Join(a.workspaceDir, path)

		// Verify path is within workspace
		absPath, err := filepath.Abs(fullPath)
		if err != nil {
			lastErr = fmt.Errorf("invalid path %s: %w", path, err)
			return match
		}
		absWorkspace, _ := filepath.Abs(a.workspaceDir)
		if !strings.HasPrefix(absPath, absWorkspace) {
			lastErr = fmt.Errorf("path outside workspace: %s", path)
			return match
		}

		// Read file
		content, err := os.ReadFile(fullPath)
		if err != nil {
			lastErr = fmt.Errorf("cannot read %s: %w", path, err)
			return match
		}

		// Format as XML-style tag, preserving trailing punctuation
		return fmt.Sprintf("\n<file path=\"%s\">\n%s\n</file>%s\n", path, string(content), trailing)
	})

	return result, lastErr
}

// stripTrailingPunctuation removes trailing punctuation from a path and returns both parts
func stripTrailingPunctuation(path string) (string, string) {
	// Punctuation that commonly follows file references in sentences
	punctuation := "?!,.:;'\")}]>"
	trailing := ""
	for len(path) > 0 && strings.ContainsRune(punctuation, rune(path[len(path)-1])) {
		trailing = string(path[len(path)-1]) + trailing
		path = path[:len(path)-1]
	}
	return path, trailing
}

// fuzzyMatch returns a score for how well the pattern matches the text
func fuzzyMatch(pattern, text string) int {
	pattern = strings.ToLower(pattern)
	text = strings.ToLower(text)

	if len(pattern) == 0 {
		return 0
	}
	if len(pattern) > len(text) {
		return -1
	}

	patternIdx := 0
	score := 0
	lastMatchIdx := -1
	consecutiveBonus := 0

	for i := 0; i < len(text) && patternIdx < len(pattern); i++ {
		if text[i] == pattern[patternIdx] {
			score += 10
			if lastMatchIdx == i-1 {
				consecutiveBonus++
				score += consecutiveBonus * 5
			} else {
				consecutiveBonus = 0
			}
			if i == 0 || text[i-1] == '/' || text[i-1] == '_' || text[i-1] == '-' || text[i-1] == '.' {
				score += 15
			}
			lastSlash := strings.LastIndex(text, "/")
			if i > lastSlash {
				score += 5
			}
			lastMatchIdx = i
			patternIdx++
		}
	}

	if patternIdx < len(pattern) {
		return -1
	}

	score -= strings.Count(text, "/") * 2
	score -= len(text) / 10

	return score
}

// getCommandMatches returns slash commands matching the prefix for autocomplete
func (a *App) getCommandMatches(prefix string) []string {
	// Built-in commands
	builtins := []string{"clear", "compact", "help", "quit", "todos"}

	var matches []string

	// Add matching built-in commands
	for _, cmd := range builtins {
		if strings.HasPrefix(cmd, prefix) {
			matches = append(matches, cmd)
		}
	}

	// Add matching custom commands from loader
	if a.commandLoader != nil {
		for _, cmd := range a.commandLoader.ListCommands() {
			if strings.HasPrefix(cmd.Name, prefix) {
				matches = append(matches, cmd.Name)
			}
		}
	}

	// Sort alphabetically
	sort.Strings(matches)

	return matches
}

// getFileMatches returns files matching the prefix for autocomplete
func (a *App) getFileMatches(prefix string) []string {
	if prefix == "" {
		return nil
	}

	excludeDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true,
		"__pycache__": true, ".venv": true, "dist": true,
		"build": true, ".idea": true, ".vscode": true,
	}

	type scoredMatch struct {
		path  string
		score int
	}
	var matches []scoredMatch

	_ = filepath.Walk(a.workspaceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(a.workspaceDir, path)
		if err != nil || relPath == "." {
			return nil
		}

		if info.IsDir() {
			if excludeDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		score := fuzzyMatch(prefix, relPath)
		if score >= 0 {
			matches = append(matches, scoredMatch{path: relPath, score: score})
		}

		if len(matches) >= 100 {
			return filepath.SkipAll
		}

		return nil
	})

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return len(matches[i].path) < len(matches[j].path)
	})

	result := make([]string, 0, 10)
	for i := 0; i < len(matches) && i < 10; i++ {
		result = append(result, matches[i].path)
	}

	return result
}
