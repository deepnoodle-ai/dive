package main

import (
	"context"
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

// Message represents a chat message
type Message struct {
	Role    string      // "user", "assistant", or "system"
	Content string      // Text content
	Time    time.Time
	Type    MessageType

	// Tool call fields (when Type == MessageTypeToolCall)
	ToolID     string
	ToolName   string
	ToolInput  string
	ToolResult string
	ToolError  bool
	ToolDone   bool
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

// App is the main TUI application
type App struct {
	mu sync.RWMutex

	agent        *dive.StandardAgent
	workspaceDir string

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
	processing bool
	thinking   bool

	// Confirmation state
	confirm ConfirmState

	// User interaction states
	selectState      SelectState
	multiSelectState MultiSelectState
	inputState       InputState

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

}

// NewApp creates a new TUI application
func NewApp(agent *dive.StandardAgent, workspaceDir string) *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		agent:         agent,
		workspaceDir:  workspaceDir,
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

	a.messages = append(a.messages, Message{
		Role:    "system",
		Content: "Welcome to Dive! I'm your AI coding assistant.\n\nType your message and press Enter to send. Use Shift+Enter for newlines.\nPress Ctrl+C or Ctrl+D to exit.",
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
			a.mu.Unlock()
		} else {
			// Plain Enter sends the message
			return a.sendMessage()
		}
		return nil

	case e.Key == tui.KeyBackspace:
		a.mu.Lock()
		if len(a.input) > 0 {
			a.input = a.input[:len(a.input)-1]
		}
		a.historyIndex = -1
		a.mu.Unlock()
		return nil

	case e.Key == tui.KeyArrowUp:
		a.mu.Lock()
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

	// Add to history
	a.history = append(a.history, a.input)
	a.historyIndex = -1
	a.savedInput = ""

	// Add user message
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
	a.input = ""
	a.mu.Unlock()

	// Start async response generation
	return []tui.Cmd{a.generateResponse(trimmed)}
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

	msg := Message{
		Role:      "assistant",
		Time:      time.Now(),
		Type:      MessageTypeToolCall,
		ToolID:    call.ID,
		ToolName:  call.Name,
		ToolInput: truncateString(string(call.Input), 200),
		ToolDone:  false,
	}
	a.messages = append(a.messages, msg)
	a.toolCallIndex[call.ID] = len(a.messages) - 1
	a.needNewTextMessage = true // next text should go to a new message
	a.scrollY = 999999
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
						display = truncateString(c.Text, 100)
						break
					}
				}
			}
			a.messages[idx].ToolResult = display
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
