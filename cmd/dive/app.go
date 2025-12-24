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

	// Handle confirmation mode
	a.mu.RLock()
	inConfirm := a.confirm.Pending
	a.mu.RUnlock()

	if inConfirm {
		return a.handleConfirmKey(e)
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
		switch e.Button {
		case tui.MouseButtonWheelUp:
			a.scrollY--
			if a.scrollY < 0 {
				a.scrollY = 0
			}
		case tui.MouseButtonWheelDown:
			a.scrollY++
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
		// Create response with event callback
		_, err := a.agent.CreateResponse(a.ctx,
			dive.WithInput(userInput),
			dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
				return a.handleAgentEvent(item)
			}),
		)

		// Signal completion
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
