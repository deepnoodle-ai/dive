package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/compaction"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/dive/skill"
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

type streamThinkingEvent struct {
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
	userInput      string
	expanded       string
	fromBackground bool // true when triggered by a native background task completion
}

type processingEndEvent struct {
	baseEvent
	err       error
	lastUsage *llm.Usage // Final LLM usage for context tracking
}

// usageUpdateEvent is sent each time the LLM returns usage data,
// allowing the status line to show live token counts during an interaction.
type usageUpdateEvent struct {
	baseEvent
	usage *llm.Usage
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
	// midTurn is true when compaction happened inside a turn's tool loop (via
	// MidTurnCompactionHook) rather than between turns.
	midTurn bool
}

// toolStreamEvent carries a chunk of streaming output for a tool call.
// Used by any tool that produces incremental output (Bash stdout, sub-agents).
type toolStreamEvent struct {
	baseEvent
	toolCallID string
	text       string
}

// toolProgressEvent carries a structured progress snapshot for a tool call.
// Distinct from toolStreamEvent (text deltas): each snapshot is latest-wins and
// surfaces a short status line while the tool runs (e.g. "47 lines · 2.2 KB").
type toolProgressEvent struct {
	baseEvent
	toolCallID string
	display    string
}

// nativeBgTasksReadyEvent is sent when one or more native dive.BackgroundTaskHandle
// goroutines have all completed, signaling the agent should re-enter with results.
type nativeBgTasksReadyEvent struct {
	baseEvent
	handles []*dive.BackgroundTaskHandle
	results map[string]*dive.ToolResult
}

type monitorNotificationEvent struct {
	baseEvent
	description string
	lines       []string
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
	Role    string // "user", "assistant", "reasoning", "system", "context", or "intro"
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
	ToolProgress    string   // Transient structured-progress line (live view only)
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
	ConfirmQuestion          string // Custom question text (e.g., "Do you want to make this edit to tmp.txt?")

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
	agent        *dive.Agent
	sessionStore session.Store
	workspaceDir string
	modelName    string
	skills       *skill.Loader

	// Session management
	resumeSessionID string           // Session ID to resume (from --resume flag)
	currentSession  *session.Session // Current active session

	// InlineApp runner
	runner *tui.InlineApp

	// Input state
	inputText    string
	historyIndex int // -1 when not navigating history

	// Chat state
	messages              []Message
	streamingMessageIndex int
	thinkingMessageIndex  int
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

	// Streaming buffers (flushed on tick for batched updates)
	streamBuffer         string
	thinkingStreamBuffer string

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Initial prompt to submit on startup
	initialPrompt string
	// Content attached to the first outbound user message only.
	startupAttachment string
	firstUserSent     bool

	// Ctrl+C exit confirmation state
	lastCtrlC    time.Time
	showExitHint bool

	// API endpoint for model creation (preserved for model switching)
	apiEndpoint string

	// Compaction configuration and state
	compactionConfig         *compaction.CompactionConfig
	lastCompactionEvent      *compaction.CompactionEvent
	compactionEventTime      time.Time
	showCompactionStats      bool
	compactionStatsStartTime time.Time

	// Status line state
	lastUsage        *llm.Usage // Most recent LLM usage (for context %)
	interactionUsage *llm.Usage // Usage for the current interaction (reset per turn)
	sessionUsage     *llm.Usage // Cumulative usage across all interactions
	contextWindowMax int        // Max context window tokens for the model

	// Streaming tool output state
	toolStreamBuffers map[string]string // tool call ID -> accumulated streaming text

	// pendingNativeBgResults queues native background results that arrived while
	// the agent was already processing; drained at the end of each turn.
	pendingNativeBgResults []nativeBgTasksReadyEvent

	// pendingMonitorNotifications queues monitor line batches that arrived
	// while processing; drained one at a time at the end of each turn.
	pendingMonitorNotifications []monitorNotificationEvent
}

// NewApp creates a new CLI application
func NewApp(
	agent *dive.Agent,
	sessionStore session.Store,
	workspaceDir, modelName string,
	initialPrompt string,
	compactionConfig *compaction.CompactionConfig,
	resumeSessionID string,
	skills *skill.Loader,
	apiEndpoint string,
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
		agent:                 agent,
		sessionStore:          sessionStore,
		workspaceDir:          workspaceDir,
		modelName:             modelName,
		resumeSessionID:       resumeSessionID,
		skills:                skills,
		apiEndpoint:           apiEndpoint,
		messages:              make([]Message, 0),
		streamingMessageIndex: -1,
		thinkingMessageIndex:  -1,
		toolCallIndex:         make(map[string]int),
		toolTitles:            toolTitles,
		history:               make([]string, 0),
		historyIndex:          -1,
		ctx:                   ctx,
		cancel:                cancel,
		initialPrompt:         initialPrompt,
		compactionConfig:      compactionConfig,
		contextWindowMax:      contextWindowForModel(modelName),
		toolStreamBuffers:     make(map[string]string),
	}
}

// shutdown cleans up resources before exit.
func (a *App) shutdown() {}

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
			Prompt(" ❯ ").
			PromptStyle(tui.NewStyle().WithFgRGB(tui.RGB{R: 100, G: 100, B: 110})).
			Placeholder("Type a message... (@filename for autocomplete)").
			Multiline(true).
			MaxHeight(10).
			OnChange(func(string) {
				// A focused input consumes its own keystrokes (wonton
				// >= v0.0.35), so refresh autocomplete on edits here
				// rather than from HandleEvent.
				a.updateAutocomplete()
			}).
			OnKey(func(e tui.KeyEvent) bool {
				// Claim the keys the input would otherwise consume so
				// autocomplete navigation and history recall still work
				// while the input is focused.
				if a.handleInputNavKey(e) {
					a.updateAutocomplete()
					return true
				}
				return false
			}).
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

	// Show status line when autocomplete is not active
	if len(a.autocompleteMatches) == 0 {
		views = append(views, a.statusLineView())
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
		confirmTextStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 198, G: 198, B: 210})
		confirmInfoStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 198, G: 198, B: 210}).WithItalic()

		// Upper divider (solid line)
		views = append(views, tui.Divider().Char('─').Style(purpleStyle))

		// Title and subtitle
		views = append(views, tui.Text(" %s", a.dialogState.Title).Style(confirmTextStyle.WithBold()))
		if a.dialogState.Message != "" {
			views = append(views, tui.Text(" %s", a.dialogState.Message).Style(confirmInfoStyle))
		}

		// Content preview with dashed dividers
		if a.dialogState.ContentPreview != "" {
			views = append(views, tui.Divider().Char('╌').Style(purpleStyle))
			for _, line := range strings.Split(a.dialogState.ContentPreview, "\n") {
				switch isDiffLine(line) {
				case "+":
					views = append(views, tui.Text(" %s", line).Success())
				case "-":
					views = append(views, tui.Text(" %s", line).Error())
				default:
					views = append(views, tui.Text(" %s", line).Style(confirmTextStyle))
				}
			}
			views = append(views, tui.Divider().Char('╌').Style(purpleStyle))
		} else {
			views = append(views, tui.Divider().Char('╌').Style(purpleStyle))
		}

		// Question
		question := "Do you want to proceed?"
		if a.dialogState.ConfirmQuestion != "" {
			question = a.dialogState.ConfirmQuestion
		}
		views = append(views, tui.Text(" %s", question).Style(confirmTextStyle.WithBold()))

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
				Option(fmt.Sprintf("Yes, allow all %s during this session (shift+tab)", allowLabel)).
				Option("No").
				CursorStyle(purpleStyle).
				HintText("").
				OnSelect(func(idx int, inputText string) {
					switch idx {
					case 0: // Yes
						confirmChan <- ConfirmResult{Approved: true}
					case 1: // Yes, allow all session
						confirmChan <- ConfirmResult{Approved: true, AllowSession: true}
					case 2: // No
						confirmChan <- ConfirmResult{Approved: false}
					}
					a.hideActiveDialog()
				}).
				OnCancel(func() {
					confirmChan <- ConfirmResult{Approved: false}
					a.hideActiveDialog()
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
				a.hideActiveDialog()
			}).
			OnCancel(func() {
				selectChan <- SelectResult{Index: -1}
				a.hideActiveDialog()
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
					a.hideActiveDialog()
				}),
		)
		views = append(views, tui.Text(" Press Enter to confirm, Esc to cancel").Hint())
	}

	return tui.Stack(views...)
}

// hideActiveDialog closes the current dialog via the event loop and restores
// focus to the main input.
func (a *App) hideActiveDialog() {
	if a.runner == nil {
		if a.dialogState != nil {
			a.dialogState.Active = false
			a.dialogState = nil
		}
		return
	}
	a.runner.SendEvent(hideDialogEvent{baseEvent: newBaseEvent()})
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
	case streamThinkingEvent:
		a.handleStreamThinking(e.text)
	case toolCallEvent:
		a.handleToolCall(e.call)
	case toolResultEvent:
		a.handleToolResult(e.result)
	case usageUpdateEvent:
		if a.interactionUsage != nil {
			a.interactionUsage.Add(e.usage)
		}
		if a.sessionUsage != nil {
			a.sessionUsage.Add(e.usage)
		}
	case processingEndEvent:
		if e.lastUsage != nil {
			a.lastUsage = e.lastUsage
		}
		a.handleProcessingEnd(e.err)
	case compactionEvent:
		a.handleCompaction(e.event, e.midTurn)
	case toolStreamEvent:
		a.handleToolStream(e)
	case toolProgressEvent:
		a.handleToolProgress(e)
	case nativeBgTasksReadyEvent:
		a.handleNativeBgTasksReady(e)
	case monitorNotificationEvent:
		a.handleMonitorNotification(e)
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
		if a.dialogState != nil {
			a.dialogState.Active = false
		}
		a.dialogState = nil
		return []tui.Cmd{tui.Focus("main-input")}
	case modelSwitchEvent:
		a.switchModel(e.modelID)
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
			a.hideActiveDialog()
		case e.Key == tui.KeyEnter:
			// Return selected indices based on CheckboxList state
			var selected []int
			for i, checked := range a.dialogState.MultiSelectChecked {
				if checked {
					selected = append(selected, i)
				}
			}
			a.dialogState.MultiSelectChan <- selected
			a.hideActiveDialog()
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
			a.hideActiveDialog()
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

	// Autocomplete navigation and command-history recall. These normally
	// arrive through the focused input's OnKey hook; consulting them here too
	// covers any key that reaches the app directly.
	if a.handleInputNavKey(e) {
		return nil
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
				a.shutdown()
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
	}

	return nil
}

// handleInputNavKey processes autocomplete navigation and command-history
// recall for the main input, returning true when it consumes the key. Since
// wonton >= v0.0.35 a focused InputField owns its keystrokes, this is wired
// through the input's OnKey hook; handleKeyEvent also consults it for keys
// that reach the app directly. Returning false lets the input handle the key
// itself (e.g. moving the cursor when there is no history to recall).
func (a *App) handleInputNavKey(e tui.KeyEvent) bool {
	// Autocomplete navigation takes precedence while suggestions are shown.
	if len(a.autocompleteMatches) > 0 {
		switch e.Key {
		case tui.KeyArrowUp:
			if a.autocompleteIndex > 0 {
				a.autocompleteIndex--
			}
			return true
		case tui.KeyArrowDown:
			if a.autocompleteIndex < len(a.autocompleteMatches)-1 {
				a.autocompleteIndex++
			}
			return true
		case tui.KeyTab:
			a.selectAutocomplete()
			return true
		case tui.KeyEscape:
			a.clearAutocomplete()
			return true
		}
	}

	// Command-history recall on Up/Down when the input is idle.
	switch e.Key {
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
			return true
		}
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
			return true
		}
	}

	return false
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
	includeStartupAttachment := !a.firstUserSent
	a.firstUserSent = true
	go a.processMessageAsync(trimmed, includeStartupAttachment)
}

// processMessageAsync handles message processing in background.
// Sends events to the main event loop instead of modifying state directly.
func (a *App) processMessageAsync(input string, includeStartupAttachment bool) {
	// Expand file references (uses only immutable workspaceDir)
	expanded, extraContent, err := a.expandFileReferences(input)
	if err != nil {
		a.runner.Printf("Warning: %s", err.Error())
	}
	if includeStartupAttachment {
		expanded = appendAttachedContent(expanded, a.startupAttachment)
	}

	// Send start event to set up state in the main goroutine
	a.runner.SendEvent(processingStartEvent{baseEvent: newBaseEvent(), userInput: input, expanded: expanded})
	a.runAgent(expanded, extraContent)
}

// processCommandAsync handles slash command processing in background.
// displayText is what the user typed (e.g., "/explain go.mod")
// expanded is the full prompt to send to the agent
func (a *App) processCommandAsync(displayText, expanded string, includeStartupAttachment bool) {
	// Expand file references in the expanded text
	expandedWithFiles, extraContent, err := a.expandFileReferences(expanded)
	if err != nil {
		a.runner.Printf("Warning: %s", err.Error())
	}
	if includeStartupAttachment {
		expandedWithFiles = appendAttachedContent(expandedWithFiles, a.startupAttachment)
	}

	// Send start event with display text, but send expanded text to agent
	a.runner.SendEvent(processingStartEvent{baseEvent: newBaseEvent(), userInput: displayText, expanded: expandedWithFiles})
	a.runAgent(expandedWithFiles, extraContent)
}

// agentEventCallback returns a dive.EventCallback that routes agent response
// items to the appropriate UI events. lastUsage is updated in-place as LLM
// usage events arrive.
func (a *App) agentEventCallback(lastUsage **llm.Usage) dive.EventCallback {
	return func(ctx context.Context, item *dive.ResponseItem) error {
		switch item.Type {
		case dive.ResponseItemTypeModelEvent:
			if item.Event != nil && item.Event.Delta != nil {
				if thinking := item.Event.Delta.Thinking; thinking != "" {
					a.runner.SendEvent(streamThinkingEvent{baseEvent: newBaseEvent(), text: thinking})
				}
				if text := item.Event.Delta.Text; text != "" {
					a.runner.SendEvent(streamTextEvent{baseEvent: newBaseEvent(), text: text})
				}
			}
		case dive.ResponseItemTypeMessage:
			if item.Usage != nil {
				*lastUsage = item.Usage
				a.runner.SendEvent(usageUpdateEvent{baseEvent: newBaseEvent(), usage: item.Usage.Copy()})
			}
		case dive.ResponseItemTypeToolCall:
			a.runner.SendEvent(toolCallEvent{baseEvent: newBaseEvent(), call: item.ToolCall})
		case dive.ResponseItemTypeToolCallResult:
			a.runner.SendEvent(toolResultEvent{baseEvent: newBaseEvent(), result: item.ToolCallResult})
		case dive.ResponseItemTypeToolStream:
			if item.ToolStream != nil {
				a.runner.SendEvent(toolStreamEvent{
					baseEvent:  newBaseEvent(),
					toolCallID: item.ToolStream.ToolCallID,
					text:       item.ToolStream.Text,
				})
			}
		case dive.ResponseItemTypeToolProgress:
			if item.ToolProgress != nil && item.ToolProgress.Progress != nil {
				a.runner.SendEvent(toolProgressEvent{
					baseEvent:  newBaseEvent(),
					toolCallID: item.ToolProgress.ToolCallID,
					display:    item.ToolProgress.Progress.Display,
				})
			}
		}
		return nil
	}
}

// runAgent runs the agent with the given input. If extra content blocks are
// provided (images, documents, etc.), they are sent as native content blocks
// alongside the text in a single user message.
func (a *App) runAgent(expanded string, extraContent []llm.Content) {

	// Track the last usage from the final LLM call (for accurate context size)
	// resp.Usage is accumulated across tool iterations which inflates context size
	var lastUsage *llm.Usage

	// Build the input option: use WithMessages when extra content blocks are
	// present so they are sent as native content blocks rather than text.
	var inputOpt dive.CreateResponseOption
	if len(extraContent) > 0 {
		content := []llm.Content{&llm.TextContent{Text: expanded}}
		content = append(content, extraContent...)
		inputOpt = dive.WithMessages(llm.NewUserMessage(content...))
	} else {
		inputOpt = dive.WithInput(expanded)
	}

	// Build options for CreateResponse
	opts := []dive.CreateResponseOption{
		inputOpt,
		dive.WithSession(a.currentSession),
		dive.WithEventCallback(a.agentEventCallback(&lastUsage)),
	}

	// Track if this is a new session (for metadata update)
	isNewSession := a.currentSession.EventCount() <= 1

	// Create response with streaming
	resp, err := a.agent.CreateResponse(a.ctx, opts...)

	// Register any native background tasks so their results are auto-delivered.
	if err == nil && resp != nil && len(resp.BackgroundTasks) > 0 {
		a.startNativeBgTaskWatcher(resp.BackgroundTasks)
	}

	// Update session metadata for new sessions
	if err == nil && resp != nil && a.sessionStore != nil && isNewSession {
		a.updateSessionMetadata(expanded)
	}

	// Check for compaction after successful response
	if err == nil && resp != nil && a.compactionConfig != nil && a.sessionStore != nil {
		a.checkAndPerformCompaction(lastUsage)
	}

	// Send completion event with usage info
	a.runner.SendEvent(processingEndEvent{baseEvent: newBaseEvent(), err: err, lastUsage: lastUsage})
}

// updateSessionMetadata updates a session with workspace and title information
func (a *App) updateSessionMetadata(firstMessage string) {
	sess := a.currentSession
	if sess == nil {
		return
	}

	needsUpdate := false

	// Set workspace if not already set
	meta := sess.Metadata()
	if meta == nil || meta["workspace"] == nil {
		sess.SetMetadata("workspace", a.workspaceDir)
		needsUpdate = true
	}

	// Generate title from first message if not already set
	if sess.Title() == "" {
		sess.SetTitle(generateSessionTitle(firstMessage))
		needsUpdate = true
	}

	if needsUpdate && a.sessionStore != nil {
		_ = a.sessionStore.Put(a.ctx, sess)
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

// repeatedCompactionWarnThreshold is the checkpoint count past which the CLI
// nudges the user toward a fresh session — repeated lossy compactions compound
// and degrade accuracy.
const repeatedCompactionWarnThreshold = 3

// warnIfManyCompactions nudges the user to start a fresh session once a thread
// has been compacted repeatedly.
func (a *App) warnIfManyCompactions() {
	if a.currentSession == nil {
		return
	}
	records, err := a.currentSession.CompactionHistory(a.ctx)
	if err != nil || len(records) < repeatedCompactionWarnThreshold {
		return
	}
	a.runner.Printf("Note: this conversation has been compacted %d times. "+
		"Repeated compactions lose detail and can reduce accuracy — consider starting a fresh session.",
		len(records))
}

// checkAndPerformCompaction checks if compaction is needed and performs it.
// Uses lastUsage (from final LLM call) for accurate context size measurement.
func (a *App) checkAndPerformCompaction(lastUsage *llm.Usage) {
	if lastUsage == nil || a.currentSession == nil {
		return
	}

	// Get threshold
	threshold := a.compactionConfig.ContextTokenThreshold
	if threshold <= 0 {
		threshold = compaction.DefaultContextTokenThreshold
	}

	// Get message count from session
	msgs, err := a.currentSession.Messages(a.ctx)
	if err != nil {
		return
	}

	// Check if compaction should trigger
	if !compaction.ShouldCompact(lastUsage, len(msgs), threshold) {
		return
	}

	// Get the model for compaction
	model := a.compactionConfig.Model
	if model == nil {
		model = a.agent.Model()
	}

	// Calculate tokens before (from last LLM call = actual context size)
	tokensBefore := compaction.CalculateContextTokens(lastUsage)

	// Perform compaction using the session's Compact method
	err = a.currentSession.Compact(a.ctx, func(ctx context.Context, messages []*llm.Message) ([]*llm.Message, error) {
		compactedMsgs, _, err := compaction.CompactMessages(
			ctx,
			model,
			messages,
			"",
			a.compactionConfig.SummaryPrompt,
			tokensBefore,
		)
		return compactedMsgs, err
	})
	if err != nil {
		return
	}
	a.warnIfManyCompactions()

	// Calculate rough tokens after for the event
	compactedMsgs, _ := a.currentSession.Messages(a.ctx)
	tokensAfter := 0
	for _, msg := range compactedMsgs {
		for _, c := range msg.Content {
			if tc, ok := c.(*llm.TextContent); ok && tc.Text != "" {
				tokensAfter += len(tc.Text) / 4
			}
		}
	}

	// Send compaction event to UI
	a.runner.SendEvent(compactionEvent{baseEvent: newBaseEvent(), event: &compaction.CompactionEvent{
		TokensBefore:      tokensBefore,
		TokensAfter:       tokensAfter,
		MessagesCompacted: len(msgs) - len(compactedMsgs),
	}})
}

// Event handlers for background goroutine events

func (a *App) handleProcessingStart(e processingStartEvent) {
	userInput := strings.TrimSpace(e.userInput)

	role := "user"
	if e.fromBackground {
		role = "context"
	}
	userMsg := Message{
		Role:    role,
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
	a.thinkingMessageIndex = -1
	a.needNewTextMessage = false

	a.processing = true
	a.processingStartTime = time.Now()
	a.interactionUsage = &llm.Usage{}
	if a.sessionUsage == nil {
		a.sessionUsage = &llm.Usage{}
	}
}

func (a *App) handleStreamText(text string) {
	a.flushThinkingStreamBuffer()
	a.thinkingMessageIndex = -1
	// Buffer text for batched updates (flushed on tick)
	a.streamBuffer += text
}

func (a *App) handleStreamThinking(text string) {
	a.flushStreamBuffer()
	a.needNewTextMessage = true
	a.thinkingStreamBuffer += text
}

func (a *App) flushStreamingBuffers() {
	a.flushThinkingStreamBuffer()
	a.flushStreamBuffer()
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

func (a *App) flushThinkingStreamBuffer() {
	if a.thinkingStreamBuffer == "" {
		return
	}

	needNewMessage := a.thinkingMessageIndex < 0 ||
		a.thinkingMessageIndex >= len(a.messages) ||
		a.messages[a.thinkingMessageIndex].Role != "reasoning"

	if needNewMessage {
		a.messages = append(a.messages, Message{
			Role:    "reasoning",
			Content: "",
			Time:    time.Now(),
			Type:    MessageTypeText,
		})
		a.thinkingMessageIndex = len(a.messages) - 1
	}

	a.messages[a.thinkingMessageIndex].Content += a.thinkingStreamBuffer
	a.thinkingStreamBuffer = ""
}

func (a *App) handleToolCall(call *llm.ToolUseContent) {
	a.flushStreamingBuffers()

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
	a.thinkingMessageIndex = -1
}

func (a *App) handleToolResult(result *dive.ToolCallResult) {
	if idx, ok := a.toolCallIndex[result.ID]; ok && idx < len(a.messages) {
		a.messages[idx].ToolDone = true
		if result.Result != nil {
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
			statusText := textContent
			if statusText == "" {
				statusText = display
			}
			a.messages[idx].ToolError = shouldDisplayToolError(
				a.messages[idx].ToolName,
				result.Result.IsError,
				statusText,
			)
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

// shouldDisplayToolError determines whether a tool result should render as an error.
// AskUserQuestion can return valid custom responses that may be flagged as protocol
// errors upstream, so we treat valid AskUser JSON output as non-error for UI status.
func shouldDisplayToolError(_ string, isError bool, _ string) bool {
	return isError
}

// handleCompaction processes a compaction event and updates UI state.
// The compaction notification is shown in the live view for 3 seconds,
// and detailed stats are displayed in the footer for 5 seconds.
func (a *App) handleCompaction(event *compaction.CompactionEvent, midTurn bool) {
	a.lastCompactionEvent = event
	// Reset usage so the context bar reflects post-compaction state.
	// It will be repopulated on the next LLM response.
	a.lastUsage = nil
	if midTurn {
		// Mid-turn compaction happens while the agent is still working, so
		// surface it as a scrollback line rather than the transient footer
		// stats used between turns.
		a.runner.Printf(" ⚡ Context compacted mid-turn: ~%d → ~%d tokens (%d messages summarized)",
			event.TokensBefore, event.TokensAfter, event.MessagesCompacted)
		return
	}
	a.compactionEventTime = time.Now()
	a.showCompactionStats = true
	a.compactionStatsStartTime = time.Now()
}

// notifyMidTurnCompaction surfaces a mid-turn compaction (from
// MidTurnCompactionHook) in the UI. Safe to call from the agent's goroutine —
// it just enqueues a UI event.
func (a *App) notifyMidTurnCompaction(event *compaction.CompactionEvent) {
	a.runner.SendEvent(compactionEvent{baseEvent: newBaseEvent(), event: event, midTurn: true})
}

func (a *App) handleProcessingEnd(err error) {
	// Flush any remaining buffered model content.
	a.flushStreamingBuffers()

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
	a.streamingMessageIndex = -1
	a.thinkingMessageIndex = -1
	a.toolCallIndex = make(map[string]int)
	a.toolStreamBuffers = make(map[string]string)
	a.streamBuffer = ""
	a.thinkingStreamBuffer = ""

	// Now print to scrollback - the live view will be re-rendered with correct height
	a.printRecentMessagesToScrollback()

	// Drain any native background results that arrived while processing.
	if len(a.pendingNativeBgResults) > 0 {
		pending := a.pendingNativeBgResults
		a.pendingNativeBgResults = nil
		var allHandles []*dive.BackgroundTaskHandle
		allResults := make(map[string]*dive.ToolResult)
		for _, e := range pending {
			allHandles = append(allHandles, e.handles...)
			for k, v := range e.results {
				allResults[k] = v
			}
		}
		go a.runBgContinuation(allHandles, allResults)
	}

	// Drain one pending monitor notification (the next one will be drained
	// after that turn completes, and so on).
	if len(a.pendingMonitorNotifications) > 0 {
		e := a.pendingMonitorNotifications[0]
		a.pendingMonitorNotifications = a.pendingMonitorNotifications[1:]
		go a.runMonitorNotification(e.description, e.lines)
	}
}

// handleToolStream updates a tool call's result line with streaming output.
// Used by any tool that produces incremental output (Bash stdout, sub-agents).
func (a *App) handleToolStream(e toolStreamEvent) {
	idx, ok := a.toolCallIndex[e.toolCallID]
	if !ok {
		return
	}
	buf := a.toolStreamBuffers[e.toolCallID] + e.text
	if len(buf) > 500 {
		buf = buf[len(buf)-500:]
		// Skip to the next valid UTF-8 boundary to avoid garbled display
		for len(buf) > 0 && !utf8.RuneStart(buf[0]) {
			buf = buf[1:]
		}
	}
	a.toolStreamBuffers[e.toolCallID] = buf
	if last := lastNonEmptyLine(buf); last != "" {
		if len(last) > 80 {
			last = last[:77] + "..."
		}
		a.messages[idx].ToolResultLines = []string{last}
	}
}

// handleToolProgress stores a tool's latest structured-progress snapshot so the
// live view can render a transient status line while the tool runs. The
// snapshot is latest-wins (each event replaces the prior one) and is dropped
// from the static scrollback once the tool completes.
func (a *App) handleToolProgress(e toolProgressEvent) {
	idx, ok := a.toolCallIndex[e.toolCallID]
	if !ok {
		return
	}
	display := e.display
	if len(display) > 80 {
		display = display[:77] + "..."
	}
	a.messages[idx].ToolProgress = display
}

// startNativeBgTaskWatcher starts a goroutine that awaits all tasks and sends
// a nativeBgTasksReadyEvent when they complete. Discards results if the app
// context is already cancelled (shutdown).
func (a *App) startNativeBgTaskWatcher(tasks []*dive.BackgroundTaskHandle) {
	go func() {
		results, _ := dive.AwaitBackgroundTasks(a.ctx, tasks)
		if a.ctx.Err() != nil {
			return
		}
		a.runner.SendEvent(nativeBgTasksReadyEvent{
			baseEvent: newBaseEvent(),
			handles:   tasks,
			results:   results,
		})
	}()
}

// handleNativeBgTasksReady is called when native background tasks complete.
// If the agent is currently processing, the results are queued and delivered
// at the end of the current turn. Otherwise a continuation is started immediately.
func (a *App) handleNativeBgTasksReady(e nativeBgTasksReadyEvent) {
	if a.processing {
		a.pendingNativeBgResults = append(a.pendingNativeBgResults, e)
		return
	}
	go a.runBgContinuation(e.handles, e.results)
}

// runBgContinuation re-enters the agent with completed background task results.
// It is called from a goroutine, mirrors runAgent's flow, and handles chained
// background tasks (continuations can themselves return BackgroundTasks).
func (a *App) runBgContinuation(handles []*dive.BackgroundTaskHandle, results map[string]*dive.ToolResult) {
	// Build a compact display label for the synthetic "user" turn header.
	var descs []string
	for _, h := range handles {
		if h.Description != "" {
			descs = append(descs, h.Description)
		}
	}
	display := "background task completed"
	if len(descs) > 0 {
		display = strings.Join(descs, ", ") + " completed"
	}

	a.runner.SendEvent(processingStartEvent{
		baseEvent:      newBaseEvent(),
		userInput:      display,
		expanded:       display,
		fromBackground: true,
	})

	var lastUsage *llm.Usage
	opts := []dive.CreateResponseOption{
		dive.WithBackgroundResults(handles, results),
		dive.WithSession(a.currentSession),
		dive.WithEventCallback(a.agentEventCallback(&lastUsage)),
	}

	resp, err := a.agent.CreateResponse(a.ctx, opts...)

	if err == nil && resp != nil && len(resp.BackgroundTasks) > 0 {
		a.startNativeBgTaskWatcher(resp.BackgroundTasks)
	}
	if err == nil && resp != nil && a.compactionConfig != nil && a.sessionStore != nil {
		a.checkAndPerformCompaction(lastUsage)
	}

	a.runner.SendEvent(processingEndEvent{baseEvent: newBaseEvent(), err: err, lastUsage: lastUsage})
}

// handleMonitorNotification is called when a monitor batch arrives.
// Queues if processing; otherwise starts a continuation immediately.
func (a *App) handleMonitorNotification(e monitorNotificationEvent) {
	if a.processing {
		a.pendingMonitorNotifications = append(a.pendingMonitorNotifications, e)
		return
	}
	go a.runMonitorNotification(e.description, e.lines)
}

// runMonitorNotification injects a monitor line batch into the conversation.
func (a *App) runMonitorNotification(description string, lines []string) {
	content := fmt.Sprintf("Monitor: %s\n%s", description, strings.Join(lines, "\n"))

	a.runner.SendEvent(processingStartEvent{
		baseEvent:      newBaseEvent(),
		userInput:      content,
		expanded:       content,
		fromBackground: true,
	})

	var lastUsage *llm.Usage
	opts := []dive.CreateResponseOption{
		dive.WithInput(content),
		dive.WithSession(a.currentSession),
		dive.WithEventCallback(a.agentEventCallback(&lastUsage)),
	}

	resp, err := a.agent.CreateResponse(a.ctx, opts...)
	if err == nil && resp != nil && len(resp.BackgroundTasks) > 0 {
		a.startNativeBgTaskWatcher(resp.BackgroundTasks)
	}
	if err == nil && resp != nil && a.compactionConfig != nil && a.sessionStore != nil {
		a.checkAndPerformCompaction(lastUsage)
	}

	a.runner.SendEvent(processingEndEvent{baseEvent: newBaseEvent(), err: err, lastUsage: lastUsage})
}

// lastNonEmptyLine returns the last non-empty line from text.
func lastNonEmptyLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return ""
}

// printRecentMessagesToScrollback prints the messages from current interaction to scrollback
func (a *App) printRecentMessagesToScrollback() {
	// Find start of current interaction
	startIdx := 0
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].Role == "user" || a.messages[i].Role == "context" {
			startIdx = i + 1 // Start after user/context message (already printed)
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
	a.runner = tui.NewInlineApp(
		tui.WithInlineFPS(30),
		tui.WithInlineBracketedPaste(true),
		tui.WithInlineKittyKeyboard(true),
	)

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
func (a *App) printIntroToScrollback() {
	view := a.buildIntroView()
	tui.Print(view)
	fmt.Println() // Newline so the live region doesn't overwrite the last line
}

// printIntroViaRunner prints the intro/splash screen using the runner's Print method.
func (a *App) printIntroViaRunner() {
	view := a.buildIntroView()
	a.runner.Print(view)
}

// buildIntroView creates the intro view and adds it to messages.
func (a *App) buildIntroView() tui.View {
	wsDisplay := a.workspaceDir
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(wsDisplay, home) {
		wsDisplay = "~" + wsDisplay[len(home):]
	}

	content := a.modelDisplayName() + "\n" + wsDisplay
	if a.resumeSessionID != "" {
		content += "\nResuming session: " + a.resumeSessionID
	}

	a.messages = append(a.messages, Message{
		Role:    "intro",
		Content: content,
		Time:    time.Now(),
		Type:    MessageTypeText,
	})

	return a.introView(a.messages[len(a.messages)-1])
}

// printSessionHistoryToScrollback prints the conversation history from a resumed session
// to the scrollback buffer, so it appears as if continuing from where we left off.
func (a *App) printSessionHistoryToScrollback() {
	if a.resumeSessionID == "" || a.currentSession == nil {
		return
	}

	// Load messages from the session
	sessionMsgs, err := a.currentSession.Messages(a.ctx)
	if err != nil {
		return
	}

	// Build a map of tool use ID -> tool result for matching
	toolResults := make(map[string]*llm.ToolResultContent)
	for _, msg := range sessionMsgs {
		for _, content := range msg.Content {
			if result, ok := content.(*llm.ToolResultContent); ok {
				toolResults[result.ToolUseID] = result
			}
		}
	}

	// Convert and print each message
	messageViews := []tui.View{}
	for _, msg := range sessionMsgs {
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

		case *llm.ThinkingContent:
			if strings.TrimSpace(c.Thinking) == "" {
				continue
			}
			appMsg := Message{
				Role:    "reasoning",
				Content: c.Thinking,
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
				resultText := extractToolResultText(result)
				appMsg.ToolError = shouldDisplayToolError(c.Name, result.IsError, resultText)
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
		if a.currentSession != nil && a.sessionStore != nil {
			if err := a.sessionStore.Delete(a.ctx, a.currentSession.ID()); err != nil {
				// Log error but continue
				a.runner.Printf("Warning: failed to clear conversation: %v", err)
			}
		}

		// Create a new session
		newID := newSessionID()
		newSess, err := a.sessionStore.Open(a.ctx, newID)
		if err != nil {
			a.runner.Printf("Warning: failed to create new session: %v", err)
		} else {
			a.currentSession = newSess
		}

		// Clear local message state
		a.messages = make([]Message, 0)
		a.todos = nil
		a.toolCallIndex = make(map[string]int)
		a.needNewTextMessage = false
		a.currentMessage = nil
		a.streamingMessageIndex = -1
		a.thinkingMessageIndex = -1
		a.streamBuffer = ""
		a.thinkingStreamBuffer = ""
		a.firstUserSent = false
		a.lastUsage = nil
		a.sessionUsage = nil
		a.interactionUsage = nil

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

	case "model":
		a.handleModelCommand(cmdArgs)
		return true

	case "usage", "cost":
		a.printUsageReport()
		return true
	}

	// Check for custom slash commands and skills
	if a.skills != nil {
		if cmd, ok := a.skills.Get(cmdName); ok {
			// Expand argument placeholders (with shell expansion for local skills)
			expanded, expandErr := cmd.Expand(context.Background(), cmdArgs, skill.WithShellExpansion(true))
			if expandErr != nil {
				a.runner.Printf("Warning: expansion error: %v", expandErr)
			}

			// Warn about unsupported model override (agent doesn't support per-request model changes)
			if cmd.Config.Model != "" {
				a.runner.Printf("Note: Model override '%s' specified but not yet supported in CLI", cmd.Config.Model)
			}

			// Build display text with command name and args
			displayCmd := "/" + cmdName
			if cmdArgs != "" {
				displayCmd = "/" + cmdName + " " + cmdArgs
			}

			// Send the expanded instructions to the agent (async like regular messages)
			includeStartupAttachment := !a.firstUserSent
			a.firstUserSent = true
			go a.processCommandAsync(displayCmd, expanded, includeStartupAttachment)
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

	if a.currentSession == nil || a.sessionStore == nil {
		a.runner.Printf("No conversation to compact.")
		return
	}

	// Get current messages
	msgs, err := a.currentSession.Messages(a.ctx)
	if err != nil {
		a.runner.Printf("No conversation to compact.")
		return
	}

	if len(msgs) < 2 {
		a.runner.Printf("Not enough messages to compact (need at least 2).")
		return
	}

	a.runner.Printf("Compacting conversation...")

	// Calculate tokens before compaction (estimate)
	tokensBefore := 0
	for _, msg := range msgs {
		for _, c := range msg.Content {
			if tc, ok := c.(*llm.TextContent); ok && tc.Text != "" {
				tokensBefore += len(tc.Text) / 4 // rough estimate
			}
		}
	}

	// Perform compaction using session's Compact method
	summaryPrompt := a.compactionConfig.SummaryPrompt
	if summaryPrompt == "" {
		summaryPrompt = compaction.DefaultCompactionSummaryPrompt
	}

	var event *compaction.CompactionEvent
	err = a.currentSession.Compact(a.ctx, func(ctx context.Context, messages []*llm.Message) ([]*llm.Message, error) {
		compactedMsgs, evt, err := compaction.CompactMessages(
			ctx,
			a.compactionConfig.Model,
			messages,
			"",
			summaryPrompt,
			tokensBefore,
		)
		event = evt
		return compactedMsgs, err
	})
	if err != nil {
		a.runner.Printf("Compaction failed: %v", err)
		return
	}

	// Show stats
	if event != nil {
		a.runner.Printf("Compacted: ~%d -> ~%d tokens", event.TokensBefore, event.TokensAfter)
	}
	a.warnIfManyCompactions()
}

func (a *App) printHelp() {
	views := []tui.View{
		tui.Text(""),
		tui.Text("Built-in Commands:").Bold(),
		tui.Text("  /quit, /q      Exit"),
		tui.Text("  /clear         Clear conversation and screen"),
		tui.Text("  /compact       Compact conversation to save context"),
		tui.Text("  /model         Switch model"),
		tui.Text("  /todos, /t     Toggle todo list"),
		tui.Text("  /usage, /cost  Show token & cache usage breakdown"),
		tui.Text("  /help, /?      Show this help"),
	}

	// List custom commands if any
	if a.skills != nil && a.skills.Count() > 0 {
		views = append(views,
			tui.Text(""),
			tui.Text("Custom Commands & Skills:").Bold(),
		)
		for _, cmd := range a.skills.List() {
			line := fmt.Sprintf("  /%s", cmd.Name)
			if cmd.Config.ArgumentHint != "" {
				line += " " + cmd.Config.ArgumentHint
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

// printUsageReport prints the detailed token + cache usage breakdown to
// scrollback, or a placeholder when nothing has been recorded yet.
func (a *App) printUsageReport() {
	view := a.usageReportView()
	if view == nil {
		a.runner.Printf("No token usage recorded yet.")
		return
	}
	a.runner.Print(view)
}

// usageReportView builds a detailed token + cache usage breakdown (turn and,
// when it differs, session) followed by a short legend. It is the persistent,
// fully-labeled counterpart to the compact status-line panel. Returns nil when
// no usage has been recorded yet.
func (a *App) usageReportView() tui.View {
	turn := a.interactionUsage
	sess := a.sessionUsage
	turnHas := turn != nil && hasUsage(turn)
	sessHas := sess != nil && hasUsage(sess)
	if !turnHas && !sessHas {
		return nil
	}
	if turn == nil {
		turn = &llm.Usage{}
	}
	if sess == nil {
		sess = &llm.Usage{}
	}

	labelStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 110, G: 110, B: 120})
	rowLabelStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 160, G: 160, B: 170})
	valStyle := tui.NewStyle().WithFgRGB(tui.RGB{R: 220, G: 220, B: 230}).WithBold()

	const (
		labelW = 16
		colW   = 12
	)
	left := func(w int, s string, st tui.Style) tui.View {
		return tui.Width(w, tui.Stack(tui.Text("%s", s).Style(st)).Align(tui.AlignLeft))
	}
	right := func(w int, s string, st tui.Style) tui.View {
		return tui.Width(w, tui.Stack(tui.Text("%s", s).Style(st)).Align(tui.AlignRight))
	}

	type scopeCol struct {
		name string
		u    *llm.Usage
	}
	// Only show scopes that actually have usage, so session-only data is not
	// hidden behind an empty turn column.
	var cols []scopeCol
	if turnHas {
		cols = append(cols, scopeCol{"turn", turn})
	}
	if sessHas && (!turnHas || a.sessionUsageDiffers()) {
		cols = append(cols, scopeCol{"session", sess})
	}

	tokRow := func(label string, get func(*llm.Usage) int) tui.View {
		cells := []tui.View{left(labelW, "  "+label, rowLabelStyle)}
		for _, c := range cols {
			cells = append(cells, right(colW, formatTokenCount(get(c.u)), valStyle))
		}
		return tui.Group(cells...)
	}

	headerCells := []tui.View{left(labelW, "", labelStyle)}
	for _, c := range cols {
		headerCells = append(headerCells, right(colW, c.name, labelStyle))
	}

	views := []tui.View{
		tui.Text(""),
		tui.Text("Token usage").Bold(),
		tui.Group(headerCells...),
		tokRow("input", func(u *llm.Usage) int { return u.InputTokens }),
		tokRow("cache read", func(u *llm.Usage) int { return u.CacheReadInputTokens }),
		tokRow("cache write", func(u *llm.Usage) int { return u.CacheCreationInputTokens }),
		tokRow("output", func(u *llm.Usage) int { return u.OutputTokens }),
	}
	if turn.ReasoningTokens > 0 || sess.ReasoningTokens > 0 {
		views = append(views, tokRow("reasoning", func(u *llm.Usage) int { return u.ReasoningTokens }))
	}
	views = append(views, tokRow("total input", func(u *llm.Usage) int {
		return u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
	}))

	hitCells := []tui.View{left(labelW, "  cache hit", rowLabelStyle)}
	for _, c := range cols {
		rate, _ := cacheHitRate(c.u)
		hitCells = append(hitCells, right(colW, rate, valStyle))
	}
	views = append(views, tui.Group(hitCells...))

	hasCost := false
	for _, c := range cols {
		if c.u.Cost != nil {
			hasCost = true
		}
	}
	if hasCost {
		costCells := []tui.View{left(labelW, "  est. cost", rowLabelStyle)}
		for _, c := range cols {
			costCells = append(costCells, right(colW, costString(c.u), valStyle))
		}
		views = append(views, tui.Group(costCells...))
	}

	views = append(views,
		tui.Text(""),
		tui.Text("  cache read  — prompt tokens served from cache (cheap, ~0.1x)").Style(labelStyle),
		tui.Text("  cache write — prompt tokens written to cache (premium, 1.25-2x)").Style(labelStyle),
		tui.Text("  cache hit   — cache read / (cache read + cache write)").Style(labelStyle),
	)
	if hasCost {
		views = append(views, tui.Text("  est. cost   — estimated at list prices; not a bill").Style(labelStyle))
	}
	if a.lastUsage != nil && a.lastUsage.Speed != "" {
		views = append(views, tui.Text("  speed       — %s mode", a.lastUsage.Speed).Style(labelStyle))
	}
	views = append(views, tui.Text(""))

	return tui.Stack(views...)
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

// fileMediaTypes maps file extensions to MIME types for files that should be
// sent as native content blocks rather than inlined as text.
var fileMediaTypes = map[string]string{
	// Images → ImageContent
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	// Documents → DocumentContent (base64)
	".pdf": "application/pdf",
	// Videos (base64 — provider support varies)
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
}

// textDocExtensions lists extensions that should be sent as DocumentContent
// with a text source rather than inlined in the prompt as XML.
var textDocExtensions = map[string]bool{
	".csv":  true,
	".tsv":  true,
	".xml":  true,
	".html": true,
	".htm":  true,
}

// expandFileReferences expands @filepath references in the input.
// Text files are inlined as XML tags in the returned string. Images, PDFs,
// videos, and structured text files are returned as separate content blocks
// for native API support.
func (a *App) expandFileReferences(input string) (string, []llm.Content, error) {
	// Match @ followed by path characters (exclude common punctuation that might follow)
	re := regexp.MustCompile(`@([^\s@]+)`)

	var lastErr error
	var contentBlocks []llm.Content
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

		// Guard against reading excessively large files into memory.
		const maxFileSize = 100 * 1024 * 1024 // 100 MB
		info, err := os.Stat(fullPath)
		if err != nil {
			lastErr = fmt.Errorf("cannot stat %s: %w", path, err)
			return match
		}
		if info.Size() > maxFileSize {
			lastErr = fmt.Errorf("file too large to attach: %s (%s)", path, formatBytes(int(info.Size())))
			return match
		}

		data, err := os.ReadFile(fullPath)
		if err != nil {
			lastErr = fmt.Errorf("cannot read %s: %w", path, err)
			return match
		}

		ext := strings.ToLower(filepath.Ext(path))
		filename := filepath.Base(path)

		// Images → ImageContent
		if mediaType, ok := fileMediaTypes[ext]; ok && strings.HasPrefix(mediaType, "image/") {
			contentBlocks = append(contentBlocks, &llm.ImageContent{
				Source: &llm.ContentSource{
					Type:      llm.ContentSourceTypeBase64,
					MediaType: mediaType,
					Data:      base64.StdEncoding.EncodeToString(data),
				},
			})
			return fmt.Sprintf("[image: %s]%s", path, trailing)
		}

		// PDFs and videos → DocumentContent (base64)
		if mediaType, ok := fileMediaTypes[ext]; ok {
			contentBlocks = append(contentBlocks, &llm.DocumentContent{
				Source: &llm.ContentSource{
					Type:      llm.ContentSourceTypeBase64,
					MediaType: mediaType,
					Data:      base64.StdEncoding.EncodeToString(data),
				},
				Title: filename,
			})
			return fmt.Sprintf("[document: %s]%s", path, trailing)
		}

		// Structured text files → DocumentContent (text source)
		if textDocExtensions[ext] {
			contentBlocks = append(contentBlocks, &llm.DocumentContent{
				Source: &llm.ContentSource{
					Type:      llm.ContentSourceTypeText,
					MediaType: "text/plain",
					Data:      string(data),
				},
				Title: filename,
			})
			return fmt.Sprintf("[document: %s]%s", path, trailing)
		}

		// Default: inline as XML-style tag
		return fmt.Sprintf("\n<file path=\"%s\">\n%s\n</file>%s\n", path, string(data), trailing)
	})

	return result, contentBlocks, lastErr
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
	builtins := []string{"clear", "compact", "cost", "help", "model", "quit", "todos", "usage"}

	var matches []string

	// Add matching built-in commands
	for _, cmd := range builtins {
		if strings.HasPrefix(cmd, prefix) {
			matches = append(matches, cmd)
		}
	}

	// Add matching custom commands from loader
	if a.skills != nil {
		for _, cmd := range a.skills.List() {
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

// detectGitBranch returns the current git branch name, or empty string if not in a repo.
func detectGitBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Model types, catalog, and selection logic are in models.go.

// handleModelCommand shows the model selection dialog or switches directly if
// a model name is provided as an argument.
func (a *App) handleModelCommand(args string) {
	args = strings.TrimSpace(args)

	// Direct model switch if argument provided
	if args != "" {
		a.switchModel(args)
		return
	}

	// Build choices
	choices := availableModelChoices()
	if len(choices) == 0 {
		a.runner.Printf("No models available. Set an API key (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.)")
		return
	}

	// Show dialog via event so focus gets set properly
	selectChan := make(chan SelectResult, 1)

	// Find current model index
	currentIdx := -1
	options := make([]DialogOption, len(choices))
	for i, c := range choices {
		label := c.Label
		if c.ModelID == a.modelName {
			label += " ✓"
			currentIdx = i
		}
		options[i] = DialogOption{
			Label:       label,
			Description: c.Description,
			Value:       c.ModelID,
		}
	}

	defaultIdx := 0
	if currentIdx >= 0 {
		defaultIdx = currentIdx
	}

	dialog := &DialogState{
		Type:         DialogTypeSelect,
		Active:       true,
		Title:        "Select model",
		Message:      "Switch between models. Applies to this session.",
		Options:      options,
		DefaultIndex: defaultIdx,
		SelectIndex:  defaultIdx,
		SelectChan:   selectChan,
	}

	a.runner.SendEvent(showDialogEvent{
		baseEvent: newBaseEvent(),
		dialog:    dialog,
	})

	// Wait for selection in background
	go func() {
		result := <-selectChan
		if result.Index >= 0 && result.Index < len(choices) {
			modelID := choices[result.Index].ModelID
			a.runner.SendEvent(modelSwitchEvent{
				baseEvent: newBaseEvent(),
				modelID:   modelID,
			})
		} else if result.OtherText != "" {
			a.runner.SendEvent(modelSwitchEvent{
				baseEvent: newBaseEvent(),
				modelID:   result.OtherText,
			})
		}
	}()
}

// modelSwitchEvent is sent when the user selects a model from the /model dialog.
type modelSwitchEvent struct {
	baseEvent
	modelID string
}

// switchModel changes the agent's model to the given model ID.
func (a *App) switchModel(modelID string) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return
	}
	if modelID == a.modelName {
		a.runner.Printf("Already using %s", a.modelDisplayName())
		return
	}

	oldName := a.modelName
	newModel := createModel(modelID, a.apiEndpoint)
	if newModel == nil {
		a.runner.Printf("Unknown model: %s", modelID)
		return
	}
	a.agent.SetModel(newModel)
	a.modelName = modelID
	a.contextWindowMax = contextWindowForModel(modelID)

	// Update compaction model so compaction uses the new model
	if a.compactionConfig != nil {
		a.compactionConfig.Model = newModel
	}

	// Update the model line in the system prompt so the agent knows its identity
	if sp := a.agent.SystemPrompt(); sp != "" {
		oldLine := fmt.Sprintf("- Model: %s", oldName)
		newLine := fmt.Sprintf("- Model: %s", modelID)
		if strings.Contains(sp, oldLine) {
			a.agent.SetSystemPrompt(strings.Replace(sp, oldLine, newLine, 1))
		}
	}

	a.runner.Printf("Switched to %s", a.modelDisplayName())
}
