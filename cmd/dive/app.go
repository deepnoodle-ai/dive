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
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/tui"
)

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
	Role    string      // "user", "assistant", or "system"
	Content string      // Text content
	Time    time.Time
	Type    MessageType

	// Tool call fields (when Type == MessageTypeToolCall)
	ToolID          string
	ToolName        string
	ToolInput       string   // Full JSON input for display formatting
	ToolResult      string   // Display summary (first line or truncated)
	ToolResultLines []string // Full result lines for expansion display
	ToolError       bool
	ToolDone        bool
}

// ConfirmState tracks pending tool confirmation
type ConfirmState struct {
	Pending    bool
	ToolName   string
	Summary    string
	Input      []byte
	ResultChan chan bool
}

// SelectState tracks pending single-selection prompt
type SelectState struct {
	Pending      bool
	Title        string
	Message      string
	Options      []dive.SelectOption
	SelectedIdx  int // Currently highlighted option
	ResultChan   chan *dive.SelectResponse
}

// MultiSelectState tracks pending multi-selection prompt
type MultiSelectState struct {
	Pending     bool
	Title       string
	Message     string
	Options     []dive.SelectOption
	Selected    []bool // Which options are selected
	CursorIdx   int    // Currently highlighted option
	MinSelect   int
	MaxSelect   int
	ResultChan  chan *dive.MultiSelectResponse
}

// InputState tracks pending text input prompt
type InputState struct {
	Pending     bool
	Title       string
	Message     string
	Placeholder string
	Default     string
	Value       string // Current input value
	Multiline   bool
	ResultChan  chan *dive.InputResponse
}

// AutocompleteState tracks file path autocomplete
type AutocompleteState struct {
	Active   bool     // Whether autocomplete dropdown is showing
	Prefix   string   // Text being matched (after last @)
	StartIdx int      // Position of @ in input string (byte index)
	Matches  []string // Matching file paths (relative to workspace)
	Selected int      // Currently selected match index
}

// App is the main TUI application
type App struct {
	mu sync.RWMutex

	agent        *dive.StandardAgent
	workspaceDir string
	modelName    string

	// Chat state
	messages              []Message
	input                 string
	scrollY               int
	currentMessage        *Message
	streamingMessageIndex int

	// Tool call tracking
	toolCallIndex        map[string]int
	needNewTextMessage   bool // set after tool calls to create new text message

	// Command history
	history      []string
	historyIndex int
	savedInput   string

	// UI state
	termWidth  int
	termHeight int
	frame      uint64

	// Processing state
	processing          bool
	thinking            bool
	processingStartTime time.Time // When processing started, for elapsed time display

	// Todo list state
	todos       []Todo
	showTodos   bool // Whether to show the todo list

	// Confirmation state
	confirm ConfirmState

	// User interaction states
	selectState      SelectState
	multiSelectState MultiSelectState
	inputState       InputState

	// File autocomplete state
	autocomplete AutocompleteState

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

}

// NewApp creates a new TUI application
func NewApp(agent *dive.StandardAgent, workspaceDir, modelName string) *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		agent:         agent,
		workspaceDir:  workspaceDir,
		modelName:     modelName,
		messages:      make([]Message, 0),
		toolCallIndex: make(map[string]int),
		history:       make([]string, 0),
		historyIndex:  -1,
		scrollY:       999999,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Initialize is called when the app starts
func (a *App) Initialize() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Shorten workspace path for display
	wsDisplay := a.workspaceDir
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(wsDisplay, home) {
		wsDisplay = "~" + wsDisplay[len(home):]
	}

	// Build intro message - rendered specially in render.go
	a.messages = append(a.messages, Message{
		Role:    "intro",
		Content: a.modelName + "\n" + wsDisplay,
		Time:    time.Now(),
		Type:    MessageTypeText,
	})
}

// Destroy is called when the app exits
func (a *App) Destroy() {
	a.cancel()
}

// HandleEvent processes TUI events
func (a *App) HandleEvent(event tui.Event) []tui.Cmd {
	switch e := event.(type) {
	case tui.KeyEvent:
		return a.handleKeyEvent(e)

	case tui.MouseEvent:
		return a.handleMouseEvent(e)

	case tui.ResizeEvent:
		a.mu.Lock()
		a.termWidth = e.Width
		a.termHeight = e.Height
		a.mu.Unlock()
		return nil

	case tui.TickEvent:
		a.mu.Lock()
		a.frame = e.Frame
		a.mu.Unlock()
		return nil

	case ResponseEvent:
		return a.handleResponseEvent(e)
	}

	return nil
}

func (a *App) handleKeyEvent(e tui.KeyEvent) []tui.Cmd {
	// Global keybindings
	switch {
	case e.Key == tui.KeyCtrlC, e.Key == tui.KeyCtrlD:
		return []tui.Cmd{tui.Quit()}
	case e.Key == tui.KeyCtrlT:
		// Toggle todos visibility
		a.mu.Lock()
		a.showTodos = !a.showTodos
		a.mu.Unlock()
		return nil
	}

	// Handle various interactive modes
	a.mu.RLock()
	inConfirm := a.confirm.Pending
	inSelect := a.selectState.Pending
	inMultiSelect := a.multiSelectState.Pending
	inInput := a.inputState.Pending
	a.mu.RUnlock()

	if inConfirm {
		return a.handleConfirmKey(e)
	}
	if inSelect {
		return a.handleSelectKey(e)
	}
	if inMultiSelect {
		return a.handleMultiSelectKey(e)
	}
	if inInput {
		return a.handleInputPromptKey(e)
	}

	return a.handleInputKey(e)
}

func (a *App) handleConfirmKey(e tui.KeyEvent) []tui.Cmd {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch {
	case e.Rune == 'y' || e.Rune == 'Y':
		if a.confirm.ResultChan != nil {
			a.confirm.ResultChan <- true
		}
		a.confirm = ConfirmState{}
		return nil

	case e.Rune == 'n' || e.Rune == 'N' || e.Key == tui.KeyEscape:
		if a.confirm.ResultChan != nil {
			a.confirm.ResultChan <- false
		}
		a.confirm = ConfirmState{}
		return nil
	}

	return nil
}

func (a *App) handleSelectKey(e tui.KeyEvent) []tui.Cmd {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch {
	case e.Key == tui.KeyArrowUp, e.Rune == 'k':
		if a.selectState.SelectedIdx > 0 {
			a.selectState.SelectedIdx--
		}
		return nil

	case e.Key == tui.KeyArrowDown, e.Rune == 'j':
		if a.selectState.SelectedIdx < len(a.selectState.Options)-1 {
			a.selectState.SelectedIdx++
		}
		return nil

	case e.Key == tui.KeyEnter:
		if a.selectState.ResultChan != nil && len(a.selectState.Options) > 0 {
			a.selectState.ResultChan <- &dive.SelectResponse{
				Value: a.selectState.Options[a.selectState.SelectedIdx].Value,
			}
		}
		a.selectState = SelectState{}
		return nil

	case e.Key == tui.KeyEscape, e.Rune == 'q':
		if a.selectState.ResultChan != nil {
			a.selectState.ResultChan <- &dive.SelectResponse{Canceled: true}
		}
		a.selectState = SelectState{}
		return nil
	}

	// Handle number keys for quick selection (1-9)
	if e.Rune >= '1' && e.Rune <= '9' {
		idx := int(e.Rune - '1')
		if idx < len(a.selectState.Options) {
			if a.selectState.ResultChan != nil {
				a.selectState.ResultChan <- &dive.SelectResponse{
					Value: a.selectState.Options[idx].Value,
				}
			}
			a.selectState = SelectState{}
		}
	}

	return nil
}

func (a *App) handleMultiSelectKey(e tui.KeyEvent) []tui.Cmd {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch {
	case e.Key == tui.KeyArrowUp, e.Rune == 'k':
		if a.multiSelectState.CursorIdx > 0 {
			a.multiSelectState.CursorIdx--
		}
		return nil

	case e.Key == tui.KeyArrowDown, e.Rune == 'j':
		if a.multiSelectState.CursorIdx < len(a.multiSelectState.Options)-1 {
			a.multiSelectState.CursorIdx++
		}
		return nil

	case e.Rune == ' ':
		// Toggle selection
		idx := a.multiSelectState.CursorIdx
		if idx < len(a.multiSelectState.Selected) {
			a.multiSelectState.Selected[idx] = !a.multiSelectState.Selected[idx]
		}
		return nil

	case e.Key == tui.KeyEnter:
		// Collect selected values
		var values []string
		for i, opt := range a.multiSelectState.Options {
			if i < len(a.multiSelectState.Selected) && a.multiSelectState.Selected[i] {
				values = append(values, opt.Value)
			}
		}
		// Check minimum selection
		if len(values) < a.multiSelectState.MinSelect {
			// Don't allow submission if minimum not met
			return nil
		}
		if a.multiSelectState.ResultChan != nil {
			a.multiSelectState.ResultChan <- &dive.MultiSelectResponse{Values: values}
		}
		a.multiSelectState = MultiSelectState{}
		return nil

	case e.Key == tui.KeyEscape, e.Rune == 'q':
		if a.multiSelectState.ResultChan != nil {
			a.multiSelectState.ResultChan <- &dive.MultiSelectResponse{Canceled: true}
		}
		a.multiSelectState = MultiSelectState{}
		return nil
	}

	// Handle number keys for quick toggle (1-9)
	if e.Rune >= '1' && e.Rune <= '9' {
		idx := int(e.Rune - '1')
		if idx < len(a.multiSelectState.Selected) {
			a.multiSelectState.Selected[idx] = !a.multiSelectState.Selected[idx]
		}
	}

	return nil
}

func (a *App) handleInputPromptKey(e tui.KeyEvent) []tui.Cmd {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch {
	case e.Key == tui.KeyEnter:
		if a.inputState.Multiline && e.Shift {
			a.inputState.Value += "\n"
			return nil
		}
		// Submit input
		value := a.inputState.Value
		if value == "" {
			value = a.inputState.Default
		}
		if a.inputState.ResultChan != nil {
			a.inputState.ResultChan <- &dive.InputResponse{Value: value}
		}
		a.inputState = InputState{}
		return nil

	case e.Key == tui.KeyEscape:
		if a.inputState.ResultChan != nil {
			a.inputState.ResultChan <- &dive.InputResponse{Canceled: true}
		}
		a.inputState = InputState{}
		return nil

	case e.Key == tui.KeyBackspace:
		if len(a.inputState.Value) > 0 {
			a.inputState.Value = a.inputState.Value[:len(a.inputState.Value)-1]
		}
		return nil

	default:
		if e.Rune != 0 {
			a.inputState.Value += string(e.Rune)
		}
	}

	return nil
}

func (a *App) handleInputKey(e tui.KeyEvent) []tui.Cmd {
	switch {
	case e.Key == tui.KeyEnter:
		if e.Shift {
			// Shift+Enter adds a newline
			a.mu.Lock()
			a.input += "\n"
			a.autocomplete = AutocompleteState{} // Cancel autocomplete on newline
			a.mu.Unlock()
		} else {
			// If autocomplete is active, Enter selects the item
			a.mu.Lock()
			if a.autocomplete.Active && len(a.autocomplete.Matches) > 0 {
				selected := a.autocomplete.Matches[a.autocomplete.Selected]
				a.input = a.input[:a.autocomplete.StartIdx] + "@" + selected
				a.autocomplete = AutocompleteState{}
				a.mu.Unlock()
				return nil
			}
			a.mu.Unlock()
			// Plain Enter sends the message
			return a.sendMessage()
		}
		return nil

	case e.Key == tui.KeyTab:
		// Tab accepts autocomplete selection
		a.mu.Lock()
		if a.autocomplete.Active && len(a.autocomplete.Matches) > 0 {
			selected := a.autocomplete.Matches[a.autocomplete.Selected]
			a.input = a.input[:a.autocomplete.StartIdx] + "@" + selected
			a.autocomplete = AutocompleteState{}
		}
		a.mu.Unlock()
		return nil

	case e.Key == tui.KeyEscape:
		// Escape cancels autocomplete
		a.mu.Lock()
		if a.autocomplete.Active {
			a.autocomplete = AutocompleteState{}
			a.mu.Unlock()
			return nil
		}
		a.mu.Unlock()
		return nil

	case e.Key == tui.KeyBackspace:
		a.mu.Lock()
		if len(a.input) > 0 {
			a.input = a.input[:len(a.input)-1]

			// Update autocomplete state
			if a.autocomplete.Active {
				if len(a.input) <= a.autocomplete.StartIdx {
					// Deleted past the @, deactivate
					a.autocomplete = AutocompleteState{}
				} else {
					// Update prefix
					a.autocomplete.Prefix = a.input[a.autocomplete.StartIdx+1:]
					a.updateAutocompleteMatches()
				}
			}
		}
		a.historyIndex = -1
		a.mu.Unlock()
		return nil

	case e.Key == tui.KeyArrowUp:
		a.mu.Lock()
		// If autocomplete is active, navigate suggestions
		if a.autocomplete.Active && len(a.autocomplete.Matches) > 0 {
			if a.autocomplete.Selected > 0 {
				a.autocomplete.Selected--
			}
			a.mu.Unlock()
			return nil
		}
		// Otherwise, navigate history
		if len(a.history) > 0 {
			if a.historyIndex == -1 {
				a.savedInput = a.input
				a.historyIndex = len(a.history) - 1
			} else if a.historyIndex > 0 {
				a.historyIndex--
			}
			a.input = a.history[a.historyIndex]
		}
		a.mu.Unlock()
		return nil

	case e.Key == tui.KeyArrowDown:
		a.mu.Lock()
		// If autocomplete is active, navigate suggestions
		if a.autocomplete.Active && len(a.autocomplete.Matches) > 0 {
			if a.autocomplete.Selected < len(a.autocomplete.Matches)-1 {
				a.autocomplete.Selected++
			}
			a.mu.Unlock()
			return nil
		}
		// Otherwise, navigate history
		if a.historyIndex != -1 {
			if a.historyIndex < len(a.history)-1 {
				a.historyIndex++
				a.input = a.history[a.historyIndex]
			} else {
				a.historyIndex = -1
				a.input = a.savedInput
			}
		}
		a.mu.Unlock()
		return nil

	case e.Key == tui.KeyPageUp:
		a.mu.Lock()
		a.scrollY -= 10
		if a.scrollY < 0 {
			a.scrollY = 0
		}
		a.mu.Unlock()
		return nil

	case e.Key == tui.KeyPageDown:
		a.mu.Lock()
		a.scrollY += 10
		a.mu.Unlock()
		return nil

	case e.Rune != 0:
		a.mu.Lock()
		a.input += string(e.Rune)
		a.historyIndex = -1

		// Handle autocomplete trigger and updates
		if e.Rune == '@' {
			// Start autocomplete mode
			a.autocomplete.Active = true
			a.autocomplete.StartIdx = len(a.input) - 1
			a.autocomplete.Prefix = ""
			a.autocomplete.Selected = 0
			a.autocomplete.Matches = nil
		} else if a.autocomplete.Active {
			// Update prefix with text after @
			a.autocomplete.Prefix = a.input[a.autocomplete.StartIdx+1:]
			a.updateAutocompleteMatches()

			// Deactivate on space (file path complete)
			if e.Rune == ' ' {
				a.autocomplete = AutocompleteState{}
			}
		}
		a.mu.Unlock()
		return nil
	}

	return nil
}

func (a *App) handleMouseEvent(e tui.MouseEvent) []tui.Cmd {
	if e.Type == tui.MouseScroll {
		a.mu.Lock()
		// Use DeltaY for scroll amount (negative=up, positive=down)
		// Scroll 2 lines per wheel notch
		scrollAmount := 2
		if e.DeltaY < 0 {
			// Scroll up - decrease scrollY to see earlier content
			a.scrollY -= scrollAmount
			if a.scrollY < 0 {
				a.scrollY = 0
			}
		} else if e.DeltaY > 0 {
			// Scroll down - increase scrollY to see later content
			a.scrollY += scrollAmount
		}
		a.mu.Unlock()
	}
	return nil
}

func (a *App) sendMessage() []tui.Cmd {
	a.mu.Lock()
	trimmed := strings.TrimSpace(a.input)
	if trimmed == "" || a.processing {
		a.mu.Unlock()
		return nil
	}

	// Clear autocomplete state
	a.autocomplete = AutocompleteState{}

	// Expand file references
	expanded, err := a.expandFileReferences(trimmed)
	if err != nil {
		// Show error to user but still allow sending
		a.messages = append(a.messages, Message{
			Role:    "system",
			Content: "Warning: " + err.Error(),
			Time:    time.Now(),
			Type:    MessageTypeText,
		})
	}

	// Add to history (original input, not expanded)
	a.history = append(a.history, a.input)
	a.historyIndex = -1
	a.savedInput = ""

	// Add user message (show original, send expanded)
	a.messages = append(a.messages, Message{
		Role:    "user",
		Content: trimmed,
		Time:    time.Now(),
		Type:    MessageTypeText,
	})
	a.scrollY = 999999

	// Prepare for streaming response
	a.currentMessage = &Message{
		Role:    "assistant",
		Content: "",
		Time:    time.Now(),
		Type:    MessageTypeText,
	}
	a.messages = append(a.messages, *a.currentMessage)
	a.streamingMessageIndex = len(a.messages) - 1
	a.needNewTextMessage = false // reset for new response

	a.processing = true
	a.thinking = true
	a.processingStartTime = time.Now()
	a.input = ""
	a.mu.Unlock()

	// Start async response generation with expanded content
	return []tui.Cmd{a.generateResponse(expanded)}
}

func (a *App) generateResponse(userInput string) tui.Cmd {
	return func() tui.Event {
		_, err := a.agent.CreateResponse(a.ctx,
			dive.WithInput(userInput),
			dive.WithThreadID("main"),
			dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
				return a.handleAgentEvent(item)
			}),
		)
		return ResponseEvent{Time: time.Now(), Done: true, Error: err}
	}
}

func (a *App) handleAgentEvent(item *dive.ResponseItem) error {
	switch item.Type {
	case dive.ResponseItemTypeModelEvent:
		// Handle streaming text
		if item.Event != nil && item.Event.Delta != nil {
			if text := item.Event.Delta.Text; text != "" {
				a.appendToStreamingMessage(text)
			}
		}

	case dive.ResponseItemTypeToolCall:
		// Tool call starting - mark that we'll need a new text message after this
		a.addToolCall(item.ToolCall)

	case dive.ResponseItemTypeToolCallResult:
		// Tool call completed
		a.updateToolCallResult(item.ToolCallResult)

	case dive.ResponseItemTypeMessage:
		// Complete message - we rely on streaming deltas, so ignore this
		// to prevent overwriting accumulated content
	}
	return nil
}

func (a *App) appendToStreamingMessage(text string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if we need to start a new streaming message:
	// - No current message (first streaming)
	// - Index out of bounds (shouldn't happen but be defensive)
	// - Flag set after tool calls
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

	a.messages[a.streamingMessageIndex].Content += text
	a.thinking = false
	a.scrollY = 999999
}

func (a *App) addToolCall(call *llm.ToolUseContent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if this is a TodoWrite tool call and parse todos
	if call.Name == "todo_write" || call.Name == "TodoWrite" {
		a.parseTodoWriteInput(call.Input)
	}

	msg := Message{
		Role:      "assistant",
		Time:      time.Now(),
		Type:      MessageTypeToolCall,
		ToolID:    call.ID,
		ToolName:  call.Name,
		ToolInput: string(call.Input), // Store full input for formatting
		ToolDone:  false,
	}
	a.messages = append(a.messages, msg)
	a.toolCallIndex[call.ID] = len(a.messages) - 1
	a.needNewTextMessage = true // next text should go to a new message
	a.scrollY = 999999
}

// parseTodoWriteInput parses the TodoWrite tool input and updates the todos list
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

	// Convert to our Todo type
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
	// Show todos when they're updated
	if len(a.todos) > 0 {
		a.showTodos = true
	}
}

func (a *App) updateToolCallResult(result *dive.ToolCallResult) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if idx, ok := a.toolCallIndex[result.ID]; ok && idx < len(a.messages) {
		a.messages[idx].ToolDone = true
		if result.Result != nil {
			if result.Result.IsError {
				a.messages[idx].ToolError = true
			}
			// Get display text or first text content
			display := result.Result.Display
			if display == "" && len(result.Result.Content) > 0 {
				for _, c := range result.Result.Content {
					if c.Type == dive.ToolResultContentTypeText {
						display = c.Text
						break
					}
				}
			}
			// Store full result lines for expandable display
			if display != "" {
				a.messages[idx].ToolResultLines = strings.Split(display, "\n")
			}
			// Store first line as summary
			if len(a.messages[idx].ToolResultLines) > 0 {
				a.messages[idx].ToolResult = a.messages[idx].ToolResultLines[0]
			}
		}
	}
	a.scrollY = 999999
}

func (a *App) handleResponseEvent(e ResponseEvent) []tui.Cmd {
	a.mu.Lock()
	defer a.mu.Unlock()

	if e.Done {
		a.processing = false
		a.thinking = false
		a.currentMessage = nil
		a.toolCallIndex = make(map[string]int)

		if e.Error != nil {
			// Add error message
			a.messages = append(a.messages, Message{
				Role:    "system",
				Content: "Error: " + e.Error.Error(),
				Time:    time.Now(),
				Type:    MessageTypeText,
			})
		}
		a.scrollY = 999999
	}

	return nil
}

// ConfirmTool prompts the user to confirm a tool execution
func (a *App) ConfirmTool(ctx context.Context, toolName, summary string, input []byte) (bool, error) {
	resultChan := make(chan bool, 1)

	a.mu.Lock()
	a.confirm = ConfirmState{
		Pending:    true,
		ToolName:   toolName,
		Summary:    summary,
		Input:      input,
		ResultChan: resultChan,
	}
	a.mu.Unlock()

	// Wait for user response or context cancellation
	select {
	case result := <-resultChan:
		return result, nil
	case <-ctx.Done():
		a.mu.Lock()
		a.confirm = ConfirmState{}
		a.mu.Unlock()
		return false, ctx.Err()
	}
}

// SelectTool prompts the user to select one option
func (a *App) SelectTool(ctx context.Context, req *dive.SelectRequest) (*dive.SelectResponse, error) {
	resultChan := make(chan *dive.SelectResponse, 1)

	// Find default index
	defaultIdx := 0
	for i, opt := range req.Options {
		if opt.Default {
			defaultIdx = i
			break
		}
	}

	a.mu.Lock()
	a.selectState = SelectState{
		Pending:     true,
		Title:       req.Title,
		Message:     req.Message,
		Options:     req.Options,
		SelectedIdx: defaultIdx,
		ResultChan:  resultChan,
	}
	a.mu.Unlock()

	select {
	case result := <-resultChan:
		return result, nil
	case <-ctx.Done():
		a.mu.Lock()
		a.selectState = SelectState{}
		a.mu.Unlock()
		return &dive.SelectResponse{Canceled: true}, ctx.Err()
	}
}

// MultiSelectTool prompts the user to select multiple options
func (a *App) MultiSelectTool(ctx context.Context, req *dive.MultiSelectRequest) (*dive.MultiSelectResponse, error) {
	resultChan := make(chan *dive.MultiSelectResponse, 1)

	// Initialize selected state from defaults
	selected := make([]bool, len(req.Options))
	for i, opt := range req.Options {
		selected[i] = opt.Default
	}

	a.mu.Lock()
	a.multiSelectState = MultiSelectState{
		Pending:    true,
		Title:      req.Title,
		Message:    req.Message,
		Options:    req.Options,
		Selected:   selected,
		CursorIdx:  0,
		MinSelect:  req.MinSelect,
		MaxSelect:  req.MaxSelect,
		ResultChan: resultChan,
	}
	a.mu.Unlock()

	select {
	case result := <-resultChan:
		return result, nil
	case <-ctx.Done():
		a.mu.Lock()
		a.multiSelectState = MultiSelectState{}
		a.mu.Unlock()
		return &dive.MultiSelectResponse{Canceled: true}, ctx.Err()
	}
}

// InputTool prompts the user for text input
func (a *App) InputTool(ctx context.Context, req *dive.InputRequest) (*dive.InputResponse, error) {
	resultChan := make(chan *dive.InputResponse, 1)

	a.mu.Lock()
	a.inputState = InputState{
		Pending:     true,
		Title:       req.Title,
		Message:     req.Message,
		Placeholder: req.Placeholder,
		Default:     req.Default,
		Value:       req.Default,
		Multiline:   req.Multiline,
		ResultChan:  resultChan,
	}
	a.mu.Unlock()

	select {
	case result := <-resultChan:
		return result, nil
	case <-ctx.Done():
		a.mu.Lock()
		a.inputState = InputState{}
		a.mu.Unlock()
		return &dive.InputResponse{Canceled: true}, ctx.Err()
	}
}

// ResponseEvent signals response completion
type ResponseEvent struct {
	Time  time.Time
	Done  bool
	Error error
}

// Timestamp implements the tui.Event interface
func (e ResponseEvent) Timestamp() time.Time { return e.Time }

func truncateString(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

// fuzzyMatch returns a score for how well the pattern matches the text.
// Higher scores are better matches. Returns -1 if no match.
func fuzzyMatch(pattern, text string) int {
	pattern = strings.ToLower(pattern)
	text = strings.ToLower(text)

	if len(pattern) == 0 {
		return 0
	}
	if len(pattern) > len(text) {
		return -1
	}

	// Check if pattern characters appear in order in text
	patternIdx := 0
	score := 0
	lastMatchIdx := -1
	consecutiveBonus := 0

	for i := 0; i < len(text) && patternIdx < len(pattern); i++ {
		if text[i] == pattern[patternIdx] {
			// Base score for match
			score += 10

			// Bonus for consecutive matches
			if lastMatchIdx == i-1 {
				consecutiveBonus++
				score += consecutiveBonus * 5
			} else {
				consecutiveBonus = 0
			}

			// Bonus for matching at start of text or after separator
			if i == 0 || text[i-1] == '/' || text[i-1] == '_' || text[i-1] == '-' || text[i-1] == '.' {
				score += 15
			}

			// Bonus for matching filename (after last /)
			lastSlash := strings.LastIndex(text, "/")
			if i > lastSlash {
				score += 5
			}

			lastMatchIdx = i
			patternIdx++
		}
	}

	// All pattern characters must match
	if patternIdx < len(pattern) {
		return -1
	}

	// Bonus for shorter paths (prefer less nested files)
	score -= strings.Count(text, "/") * 2

	// Bonus for shorter overall length
	score -= len(text) / 10

	return score
}

// updateAutocompleteMatches finds files matching the current prefix using fuzzy search
func (a *App) updateAutocompleteMatches() {
	// Requires lock to already be held
	prefix := a.autocomplete.Prefix
	if prefix == "" {
		a.autocomplete.Matches = nil
		return
	}

	// Directories to exclude
	excludeDirs := map[string]bool{
		".git":         true,
		"node_modules": true,
		"vendor":       true,
		"__pycache__":  true,
		".venv":        true,
		"dist":         true,
		"build":        true,
		".idea":        true,
		".vscode":      true,
	}

	type scoredMatch struct {
		path  string
		score int
	}
	var matches []scoredMatch

	// Walk the workspace directory
	_ = filepath.Walk(a.workspaceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Get relative path
		relPath, err := filepath.Rel(a.workspaceDir, path)
		if err != nil || relPath == "." {
			return nil
		}

		// Skip excluded directories
		if info.IsDir() {
			if excludeDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Fuzzy match against path
		score := fuzzyMatch(prefix, relPath)
		if score >= 0 {
			matches = append(matches, scoredMatch{path: relPath, score: score})
		}

		// Limit candidates collected
		if len(matches) >= 100 {
			return filepath.SkipAll
		}

		return nil
	})

	// Sort by score (highest first)
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		// Tie-breaker: shorter paths first
		return len(matches[i].path) < len(matches[j].path)
	})

	// Extract paths and limit to 10
	result := make([]string, 0, 10)
	for i := 0; i < len(matches) && i < 10; i++ {
		result = append(result, matches[i].path)
	}

	a.autocomplete.Matches = result
	// Reset selection if out of bounds
	if a.autocomplete.Selected >= len(result) {
		a.autocomplete.Selected = 0
	}
}

// expandFileReferences expands @filepath references in the input to file contents
func (a *App) expandFileReferences(input string) (string, error) {
	// Regex to find @filepath patterns (match until whitespace or end)
	re := regexp.MustCompile(`@([^\s]+)`)

	var lastErr error
	result := re.ReplaceAllStringFunc(input, func(match string) string {
		path := match[1:] // Remove @

		// Only allow relative paths (security)
		if filepath.IsAbs(path) {
			lastErr = fmt.Errorf("absolute paths not allowed: %s", path)
			return match
		}

		// Resolve path relative to workspace
		fullPath := filepath.Join(a.workspaceDir, path)

		// Verify path is within workspace (prevent directory traversal)
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
			return match // Keep original on error
		}

		// Format as XML-style tag
		return fmt.Sprintf("\n<file path=\"%s\">\n%s\n</file>\n", path, string(content))
	})

	return result, lastErr
}
