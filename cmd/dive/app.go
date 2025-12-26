package main

import (
	"bufio"
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
	"github.com/deepnoodle-ai/wonton/terminal"
	"github.com/deepnoodle-ai/wonton/tui"
	"golang.org/x/term"
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
	Role    string      // "user", "assistant", "system", or "intro"
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

// AutocompleteState tracks file path autocomplete
type AutocompleteState struct {
	Active        bool     // Whether autocomplete dropdown is showing
	Prefix        string   // Text being matched (after last @)
	StartIdx      int      // Position of @ in input string (byte index)
	Matches       []string // Matching file paths (relative to workspace)
	Selected      int      // Currently selected match index
	PrevLineCount int      // Number of dropdown lines shown in previous render (for clearing)
}

// App is the main CLI application
type App struct {
	mu sync.RWMutex

	agent        *dive.StandardAgent
	workspaceDir string
	modelName    string

	// Chat state
	messages              []Message
	streamingMessageIndex int
	currentMessage        *Message

	// Tool call tracking
	toolCallIndex      map[string]int
	needNewTextMessage bool // set after tool calls to create new text message

	// Command history
	history      []string
	historyIndex int
	savedInput   string // saved input when navigating history

	// UI state
	frame               uint64
	processing          bool
	thinking            bool
	processingStartTime time.Time

	// Todo list state
	todos     []Todo
	showTodos bool

	// Live printer for streaming updates
	live   *tui.LivePrinter
	ticker *time.Ticker
	done   chan struct{}

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Terminal state for raw input
	oldState *term.State

	// File autocomplete state
	autocomplete AutocompleteState
}

// NewApp creates a new CLI application
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
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Run starts the CLI application
func (a *App) Run() error {
	// Print header and intro once at startup
	a.printHeader()
	a.printIntro()

	// Main input loop
	for {
		// Print complete input area (divider + prompt + divider + footer)
		// Cursor is positioned after prompt for typing
		a.printInputArea()

		// Read user input
		input, err := a.readInput()
		if err != nil {
			return err
		}

		// Clear entire input area
		a.clearInputArea(input)

		if input == "" {
			continue
		}

		// Handle special commands
		if strings.HasPrefix(input, "/") {
			if a.handleCommand(input) {
				continue
			}
		}

		// Process the message
		if err := a.processMessage(input); err != nil {
			if err == context.Canceled {
				fmt.Println("\n(interrupted)")
				continue
			}
			return err
		}
	}
}

// printHeader prints the header once at startup
func (a *App) printHeader() {
	header := tui.Group(
		tui.Text(" Dive ").Bold().Fg(tui.ColorCyan),
		tui.Spacer(),
		tui.Text(" ready ").Success(),
	)
	tui.Print(header)
	tui.Print(tui.Divider())
}

// printIntro prints the intro/splash screen
func (a *App) printIntro() {
	// Shorten workspace path for display
	wsDisplay := a.workspaceDir
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(wsDisplay, home) {
		wsDisplay = "~" + wsDisplay[len(home):]
	}

	// Build intro message - same as TUI
	msg := Message{
		Role:    "intro",
		Content: a.modelName + "\n" + wsDisplay,
		Time:    time.Now(),
		Type:    MessageTypeText,
	}
	a.messages = append(a.messages, msg)

	// Print the intro view
	view := a.introView(msg)
	tui.Print(tui.Padding(1, view))
}

// printInputArea prints the input prompt area
func (a *App) printInputArea() {
	tui.Newline()
	tui.Print(tui.Divider())
	fmt.Print("\n") // Move to next line for input
}

// readInput reads a line of input from the user with interactive autocomplete
func (a *App) readInput() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return a.readLineFallback()
	}

	// Put terminal in raw mode
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return a.readLineFallback()
	}
	defer term.Restore(fd, oldState)

	// Use wonton's KeyDecoder for proper input handling
	decoder := terminal.NewKeyDecoder(os.Stdin)

	var input []byte
	a.autocomplete = AutocompleteState{}
	a.historyIndex = -1
	a.savedInput = ""

	for {
		// Render current state
		a.renderInputLine(string(input))

		// Read next event
		event, err := decoder.ReadEvent()
		if err != nil {
			return "", err
		}

		keyEvent, ok := event.(terminal.KeyEvent)
		if !ok {
			continue // Ignore mouse events for now
		}

		// Handle paste
		if keyEvent.Paste != "" {
			input = append(input, []byte(keyEvent.Paste)...)
			continue
		}

		// Handle special keys
		switch keyEvent.Key {
		case terminal.KeyCtrlC:
			a.clearAutocompleteDisplay()
			return "", context.Canceled

		case terminal.KeyCtrlD:
			a.clearAutocompleteDisplay()
			return "", context.Canceled

		case terminal.KeyEnter:
			if keyEvent.Shift {
				// Shift+Enter: insert newline
				input = append(input, '\n')
				continue
			}
			if a.autocomplete.Active && len(a.autocomplete.Matches) > 0 {
				// Accept selected autocomplete
				selected := a.autocomplete.Matches[a.autocomplete.Selected]
				input = append(input[:a.autocomplete.StartIdx], []byte("@"+selected)...)
				prevLines := a.autocomplete.PrevLineCount
				a.autocomplete = AutocompleteState{}
				a.autocomplete.PrevLineCount = prevLines
				continue
			}
			// Submit input
			a.clearAutocompleteDisplay()
			fmt.Print("\r\n")
			return strings.TrimSpace(string(input)), nil

		case terminal.KeyTab:
			if a.autocomplete.Active && len(a.autocomplete.Matches) > 0 {
				selected := a.autocomplete.Matches[a.autocomplete.Selected]
				input = append(input[:a.autocomplete.StartIdx], []byte("@"+selected+" ")...)
				prevLines := a.autocomplete.PrevLineCount
				a.autocomplete = AutocompleteState{}
				a.autocomplete.PrevLineCount = prevLines
			}
			continue

		case terminal.KeyEscape:
			if a.autocomplete.Active {
				prevLines := a.autocomplete.PrevLineCount
				a.autocomplete = AutocompleteState{}
				a.autocomplete.PrevLineCount = prevLines
			}
			continue

		case terminal.KeyBackspace:
			if len(input) > 0 {
				input = input[:len(input)-1]
				a.historyIndex = -1
				if a.autocomplete.Active {
					if len(input) <= a.autocomplete.StartIdx {
						prevLines := a.autocomplete.PrevLineCount
						a.autocomplete = AutocompleteState{}
						a.autocomplete.PrevLineCount = prevLines
					} else {
						a.autocomplete.Prefix = string(input[a.autocomplete.StartIdx+1:])
						a.updateAutocompleteMatches()
					}
				}
			}
			continue

		case terminal.KeyArrowUp:
			if a.autocomplete.Active && len(a.autocomplete.Matches) > 0 {
				if a.autocomplete.Selected > 0 {
					a.autocomplete.Selected--
				}
			} else if len(a.history) > 0 {
				if a.historyIndex == -1 {
					a.savedInput = string(input)
					a.historyIndex = len(a.history) - 1
				} else if a.historyIndex > 0 {
					a.historyIndex--
				}
				input = []byte(a.history[a.historyIndex])
			}
			continue

		case terminal.KeyArrowDown:
			if a.autocomplete.Active && len(a.autocomplete.Matches) > 0 {
				if a.autocomplete.Selected < len(a.autocomplete.Matches)-1 {
					a.autocomplete.Selected++
				}
			} else if a.historyIndex != -1 {
				if a.historyIndex < len(a.history)-1 {
					a.historyIndex++
					input = []byte(a.history[a.historyIndex])
				} else {
					a.historyIndex = -1
					input = []byte(a.savedInput)
				}
			}
			continue

		case terminal.KeyCtrlJ:
			// Ctrl+J: insert newline (alternative to Shift+Enter)
			input = append(input, '\n')
			continue
		}

		// Handle regular character input
		if keyEvent.Key == terminal.KeyUnknown && keyEvent.Rune != 0 {
			r := keyEvent.Rune
			input = append(input, string(r)...)
			a.historyIndex = -1

			if r == '@' {
				// Start autocomplete
				a.autocomplete.Active = true
				a.autocomplete.StartIdx = len(input) - 1
				a.autocomplete.Prefix = ""
				a.autocomplete.Selected = 0
				a.autocomplete.Matches = nil
			} else if a.autocomplete.Active {
				// Check if this character ends autocomplete
				if r == ' ' || strings.ContainsRune("?!,.:;'\")}]>", r) {
					prevLines := a.autocomplete.PrevLineCount
					a.autocomplete = AutocompleteState{}
					a.autocomplete.PrevLineCount = prevLines
				} else {
					a.autocomplete.Prefix = string(input[a.autocomplete.StartIdx+1:])
					a.updateAutocompleteMatches()
				}
			}
		}
	}
}

// readLineFallback reads a line when not in a terminal
func (a *App) readLineFallback() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", context.Canceled
	}
	return strings.TrimSpace(scanner.Text()), nil
}

// renderInputLine renders the input line, divider, and autocomplete dropdown
func (a *App) renderInputLine(input string) {
	// Get terminal width for divider
	fd := int(os.Stdout.Fd())
	width, _, err := term.GetSize(fd)
	if err != nil || width <= 0 {
		width = 80
	}
	divider := strings.Repeat("─", width)

	// Move cursor to start of line and clear
	fmt.Print("\r")
	tui.ClearLine()

	// Print prompt and input
	fmt.Print(" > " + input)

	// Calculate how many lines we'll show below input (divider + autocomplete)
	currentLineCount := 1 // Always have divider
	if a.autocomplete.Active && len(a.autocomplete.Matches) > 0 {
		acLines := len(a.autocomplete.Matches)
		if acLines > 5 {
			acLines = 6 // 5 matches + "... +N more"
		}
		currentLineCount += acLines
	}

	// Clear any extra lines from previous render
	if a.autocomplete.PrevLineCount > currentLineCount {
		for i := 0; i < a.autocomplete.PrevLineCount; i++ {
			fmt.Print("\n")
			tui.ClearLine()
		}
		for i := 0; i < a.autocomplete.PrevLineCount; i++ {
			fmt.Print("\033[A")
		}
	}

	// Always print divider first (right below input)
	fmt.Print("\n")
	tui.ClearLine()
	fmt.Printf("\r\033[90m%s\033[0m", divider)

	// If autocomplete active, show dropdown below divider
	if a.autocomplete.Active && len(a.autocomplete.Matches) > 0 {
		maxShow := 5
		if len(a.autocomplete.Matches) < maxShow {
			maxShow = len(a.autocomplete.Matches)
		}
		for i := 0; i < maxShow; i++ {
			fmt.Print("\n")
			tui.ClearLine()
			match := a.autocomplete.Matches[i]
			if i == a.autocomplete.Selected {
				fmt.Printf("\r   \033[7m %s \033[0m", match) // Inverted colors for selected
			} else {
				fmt.Printf("\r    %s", match)
			}
		}
		if len(a.autocomplete.Matches) > maxShow {
			fmt.Print("\n")
			tui.ClearLine()
			fmt.Printf("\r    ... +%d more", len(a.autocomplete.Matches)-maxShow)
			maxShow++
		}

		// Move cursor back up to input line (over divider + autocomplete)
		for i := 0; i < maxShow+1; i++ {
			fmt.Print("\033[A")
		}
		// Position cursor at end of input
		fmt.Printf("\r\033[%dC", 3+len(input))
		a.autocomplete.PrevLineCount = maxShow + 1
	} else {
		// No autocomplete - just divider shown
		// Clear any extra previous lines below divider
		if a.autocomplete.PrevLineCount > 1 {
			for i := 1; i < a.autocomplete.PrevLineCount; i++ {
				fmt.Print("\n")
				tui.ClearLine()
			}
			for i := 1; i < a.autocomplete.PrevLineCount; i++ {
				fmt.Print("\033[A")
			}
		}

		// Move back up to input line (over divider)
		fmt.Print("\033[A")
		// Position cursor at end of input
		fmt.Printf("\r\033[%dC", 3+len(input))
		a.autocomplete.PrevLineCount = 1
	}
}

// clearAutocompleteDisplay clears any autocomplete dropdown lines and divider
func (a *App) clearAutocompleteDisplay() {
	if a.autocomplete.PrevLineCount > 0 {
		// Move down and clear each line (autocomplete + divider)
		for i := 0; i < a.autocomplete.PrevLineCount; i++ {
			fmt.Print("\n")
			tui.ClearLine()
		}
		// Move back up
		for i := 0; i < a.autocomplete.PrevLineCount; i++ {
			fmt.Print("\033[A")
		}
		a.autocomplete.PrevLineCount = 0
	}
}

// updateAutocompleteMatches updates the autocomplete matches based on current prefix
func (a *App) updateAutocompleteMatches() {
	a.autocomplete.Matches = a.getFileMatches(a.autocomplete.Prefix)
	if a.autocomplete.Selected >= len(a.autocomplete.Matches) {
		a.autocomplete.Selected = 0
	}
}

// clearLines clears n lines above cursor
func (a *App) clearLines(n int) {
	for i := 0; i < n; i++ {
		tui.MoveCursorUp(1)
		tui.ClearLine()
	}
}

// clearInputArea clears the input area (blank + divider + input + autocomplete/divider below)
func (a *App) clearInputArea(input string) {
	inputLines := a.calculateInputLines(input)
	// blank (1) + divider above (1) + input line(s) (N) + lines below (autocomplete + divider)
	totalLines := 2 + inputLines + a.autocomplete.PrevLineCount
	a.clearLines(totalLines)
	a.autocomplete.PrevLineCount = 0
}

// calculateInputLines calculates how many terminal lines the input takes
func (a *App) calculateInputLines(input string) int {
	fd := int(os.Stdout.Fd())
	width, _, err := term.GetSize(fd)
	if err != nil || width <= 0 {
		width = 80 // fallback
	}

	promptLen := 3 // " > "
	totalLen := promptLen + len(input)
	lines := (totalLen + width - 1) / width // ceiling division
	if lines < 1 {
		lines = 1
	}
	return lines
}

// readSingleKey reads a single keypress (for y/n prompts)
// Returns the rune for printable keys, or special values:
// - '\r' or '\n' for Enter
// - 0x1b for Escape (standalone, not as part of escape sequence)
// - 0 for special keys (arrows, function keys, etc.) that should be ignored
func (a *App) readSingleKey() (rune, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return a.readSingleKeyFallback()
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return a.readSingleKeyFallback()
	}
	defer term.Restore(fd, oldState)

	// Use KeyDecoder to properly handle escape sequences
	decoder := terminal.NewKeyDecoder(os.Stdin)
	event, err := decoder.ReadEvent()
	if err != nil {
		return 0, err
	}

	keyEvent, ok := event.(terminal.KeyEvent)
	if !ok {
		return 0, nil // Mouse event, ignore
	}

	// Handle special keys
	switch keyEvent.Key {
	case terminal.KeyEnter:
		return '\r', nil
	case terminal.KeyEscape:
		return 0x1b, nil
	case terminal.KeyUnknown:
		// Regular character
		return keyEvent.Rune, nil
	default:
		// Arrow keys, function keys, etc. - return 0 to indicate "ignore"
		return 0, nil
	}
}

// readSingleKeyFallback reads input when terminal isn't available
func (a *App) readSingleKeyFallback() (rune, error) {
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return 0, context.Canceled
	}
	input := strings.TrimSpace(scanner.Text())
	if len(input) > 0 {
		return rune(input[0]), nil
	}
	return 0, nil
}

func (a *App) handleCommand(input string) bool {
	switch input {
	case "/quit", "/exit", "/q":
		a.cancel()
		os.Exit(0)
		return true
	case "/clear":
		tui.ClearScreen()
		a.printHeader()
		a.printIntro()
		return true
	case "/todos", "/t":
		a.mu.Lock()
		a.showTodos = !a.showTodos
		a.mu.Unlock()
		if a.showTodos {
			a.printTodos()
		}
		return true
	case "/help", "/?":
		a.printHelp()
		return true
	}
	return false
}

func (a *App) printHelp() {
	help := tui.Stack(
		tui.Text(""),
		tui.Text("Commands:").Bold(),
		tui.Text("  /quit, /q     Exit"),
		tui.Text("  /clear        Clear screen"),
		tui.Text("  /todos, /t    Toggle todo list"),
		tui.Text("  /help, /?     Show this help"),
		tui.Text(""),
		tui.Text("Input:").Bold(),
		tui.Text("  @filename     Include file contents"),
		tui.Text("  Enter         Send message"),
		tui.Text("  Ctrl+C        Exit"),
		tui.Text(""),
	)
	tui.Print(help)
}

func (a *App) printTodos() {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.todos) == 0 {
		fmt.Println("No todos.")
		return
	}

	view := a.todoListView()
	if view != nil {
		tui.Print(view)
	}
}

func (a *App) processMessage(input string) error {
	a.mu.Lock()
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || a.processing {
		a.mu.Unlock()
		return nil
	}

	// Expand file references
	expanded, err := a.expandFileReferences(trimmed)
	if err != nil {
		// Show warning but continue
		warning := tui.Text("Warning: %s", err.Error()).Warning()
		tui.Print(tui.Padding(1, warning))
	}

	// Add to history
	a.history = append(a.history, input)
	a.historyIndex = -1

	// Add user message
	userMsg := Message{
		Role:    "user",
		Content: trimmed,
		Time:    time.Now(),
		Type:    MessageTypeText,
	}
	a.messages = append(a.messages, userMsg)

	// Print user message
	a.mu.Unlock()
	tui.Print(tui.Padding(1, a.textMessageView(userMsg, len(a.messages)-1)))
	a.mu.Lock()

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
	a.thinking = true
	a.processingStartTime = time.Now()
	a.mu.Unlock()

	// Start live updates
	a.startLiveUpdates()

	// Create response with streaming
	_, err = a.agent.CreateResponse(a.ctx,
		dive.WithInput(expanded),
		dive.WithThreadID("main"),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			return a.handleAgentEvent(item)
		}),
	)

	// Stop live updates - content remains in scroll buffer
	a.stopLiveUpdates(false)

	// Handle completion
	a.mu.Lock()
	a.processing = false
	a.thinking = false
	a.currentMessage = nil
	a.toolCallIndex = make(map[string]int)

	if err != nil {
		errMsg := Message{
			Role:    "system",
			Content: "Error: " + err.Error(),
			Time:    time.Now(),
			Type:    MessageTypeText,
		}
		a.messages = append(a.messages, errMsg)
		a.mu.Unlock()
		// Print error to scroll history
		tui.Print(tui.Padding(1, tui.Text(errMsg.Content).Warning()))
	} else {
		a.mu.Unlock()
	}

	// Note: Live view content is already in scroll buffer, no need to reprint

	return nil
}

func (a *App) startLiveUpdates() {
	a.live = tui.NewLivePrinter()
	a.done = make(chan struct{})
	a.ticker = time.NewTicker(time.Second / 30) // 30 FPS

	// Initial render
	a.updateLiveView()

	// Start animation ticker
	done := a.done
	ticker := a.ticker
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				a.mu.Lock()
				a.frame++
				a.mu.Unlock()
				a.updateLiveView()
			}
		}
	}()
}

// stopLiveUpdates stops the live update loop.
// If clear is true, the live region is cleared (for temporary pauses like confirmations).
// If clear is false, the final state is rendered and left on screen (for normal completion).
func (a *App) stopLiveUpdates(clear bool) {
	// Signal goroutine to exit first, before touching ticker
	if a.done != nil {
		close(a.done)
		a.done = nil
	}
	if a.ticker != nil {
		a.ticker.Stop()
		a.ticker = nil
	}
	if a.live != nil {
		if clear {
			a.live.Clear()
		} else {
			// Final render to ensure latest state is displayed (removes thinking indicator)
			a.updateLiveView()
			a.live.Stop()
		}
		a.live = nil
	}
}

func (a *App) updateLiveView() {
	if a.live == nil {
		return
	}

	a.mu.RLock()
	view := a.buildLiveView()
	a.mu.RUnlock()

	a.live.Update(view)
}

// buildLiveView creates the view for live updates during streaming
func (a *App) buildLiveView() tui.View {
	views := make([]tui.View, 0)

	// Find start of current interaction (last user message)
	startIdx := 0
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].Role == "user" {
			startIdx = i + 1 // Start after user message
			break
		}
	}

	// Show thinking indicator if no content yet
	hasContent := false
	for i := startIdx; i < len(a.messages); i++ {
		if a.messages[i].Type == MessageTypeToolCall || a.messages[i].Content != "" {
			hasContent = true
			break
		}
	}

	if a.thinking && !hasContent {
		elapsed := time.Since(a.processingStartTime)
		views = append(views, tui.Group(
			tui.Loading(a.frame).CharSet(tui.SpinnerBounce.Frames).Speed(6).Fg(tui.ColorCyan),
			tui.Text(" thinking").Animate(tui.Slide(3, tui.NewRGB(80, 80, 80), tui.NewRGB(80, 200, 220))),
			tui.Text(" (%s)", formatDuration(elapsed)).Hint(),
		))
	}

	// Render messages from current interaction
	for i := startIdx; i < len(a.messages); i++ {
		msg := a.messages[i]
		view := a.messageView(msg, i)
		if view != nil {
			views = append(views, view)
		}
	}

	// Show todos if active
	if a.showTodos && len(a.todos) > 0 {
		views = append(views, a.todoListView())
	}

	if len(views) == 0 {
		return tui.Text("")
	}

	return tui.PaddingLTRB(1, 1, 1, 0, tui.Stack(views...).Gap(1))
}

// printRecentMessages prints the messages from the current interaction to scroll history
func (a *App) printRecentMessages() {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Find start of current interaction
	startIdx := 0
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].Role == "user" {
			startIdx = i + 1 // Start after user message (already printed)
			break
		}
	}

	// Print each message
	for i := startIdx; i < len(a.messages); i++ {
		msg := a.messages[i]
		view := a.messageViewStatic(msg, i)
		if view != nil {
			tui.Print(tui.Padding(1, view))
		}
	}

	// Print todos if visible
	if a.showTodos && len(a.todos) > 0 {
		view := a.todoListViewStatic()
		if view != nil {
			tui.Print(view)
		}
	}
}

func (a *App) handleAgentEvent(item *dive.ResponseItem) error {
	switch item.Type {
	case dive.ResponseItemTypeModelEvent:
		if item.Event != nil && item.Event.Delta != nil {
			if text := item.Event.Delta.Text; text != "" {
				a.appendToStreamingMessage(text)
			}
		}

	case dive.ResponseItemTypeToolCall:
		a.addToolCall(item.ToolCall)

	case dive.ResponseItemTypeToolCallResult:
		a.updateToolCallResult(item.ToolCallResult)
	}
	return nil
}

func (a *App) appendToStreamingMessage(text string) {
	a.mu.Lock()
	defer a.mu.Unlock()

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
}

func (a *App) addToolCall(call *llm.ToolUseContent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Got content, no longer thinking
	a.thinking = false

	// Parse TodoWrite tool calls
	if call.Name == "todo_write" || call.Name == "TodoWrite" {
		a.parseTodoWriteInput(call.Input)
	}

	msg := Message{
		Role:      "assistant",
		Time:      time.Now(),
		Type:      MessageTypeToolCall,
		ToolID:    call.ID,
		ToolName:  call.Name,
		ToolInput: string(call.Input),
		ToolDone:  false,
	}
	a.messages = append(a.messages, msg)
	a.toolCallIndex[call.ID] = len(a.messages) - 1
	a.needNewTextMessage = true
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

func (a *App) updateToolCallResult(result *dive.ToolCallResult) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if idx, ok := a.toolCallIndex[result.ID]; ok && idx < len(a.messages) {
		a.messages[idx].ToolDone = true
		if result.Result != nil {
			if result.Result.IsError {
				a.messages[idx].ToolError = true
			}
			display := result.Result.Display
			if display == "" && len(result.Result.Content) > 0 {
				for _, c := range result.Result.Content {
					if c.Type == dive.ToolResultContentTypeText {
						display = c.Text
						break
					}
				}
			}
			if display != "" {
				a.messages[idx].ToolResultLines = strings.Split(display, "\n")
			}
			if len(a.messages[idx].ToolResultLines) > 0 {
				a.messages[idx].ToolResult = a.messages[idx].ToolResultLines[0]
			}
		}
	}
}

// ConfirmTool prompts the user to confirm a tool execution
func (a *App) ConfirmTool(ctx context.Context, toolName, summary string, input []byte) (bool, error) {
	// Clear and stop live updates (we'll restart after confirmation)
	a.stopLiveUpdates(true)

	// Parse input JSON to build better summary and preview
	var parsed map[string]interface{}
	var contentPreview string
	var actionSummary string

	if len(input) > 0 {
		json.Unmarshal(input, &parsed)
	}

	// Build tool-specific summary and preview
	switch {
	case strings.Contains(toolName, "bash") || strings.Contains(toolName, "command"):
		if cmd, ok := parsed["command"].(string); ok {
			if len(cmd) > 80 {
				cmd = cmd[:77] + "..."
			}
			actionSummary = fmt.Sprintf("Run: %s", cmd)
		} else {
			actionSummary = summary
		}

	case strings.Contains(toolName, "write") || strings.Contains(toolName, "edit"):
		if filePath, ok := parsed["file_path"].(string); ok {
			actionSummary = fmt.Sprintf("Write to %s", filePath)
		} else if filePath, ok := parsed["filePath"].(string); ok {
			actionSummary = fmt.Sprintf("Write to %s", filePath)
		} else {
			actionSummary = summary
		}
		if content, ok := parsed["content"].(string); ok {
			lines := strings.Split(content, "\n")
			if len(lines) > 10 {
				contentPreview = strings.Join(lines[:10], "\n") + "\n..."
			} else {
				contentPreview = content
			}
		}

	case strings.Contains(toolName, "read"):
		if filePath, ok := parsed["file_path"].(string); ok {
			actionSummary = fmt.Sprintf("Read %s", filePath)
		} else if filePath, ok := parsed["filePath"].(string); ok {
			actionSummary = fmt.Sprintf("Read %s", filePath)
		} else {
			actionSummary = summary
		}

	default:
		actionSummary = summary
		if len(parsed) > 0 {
			var params []string
			for k, v := range parsed {
				valStr := fmt.Sprintf("%v", v)
				if len(valStr) > 50 {
					valStr = valStr[:47] + "..."
				}
				params = append(params, fmt.Sprintf("%s: %s", k, valStr))
			}
			if len(params) > 0 {
				contentPreview = strings.Join(params, "\n")
			}
		}
	}

	if actionSummary == "" {
		actionSummary = fmt.Sprintf("Execute %s", toolName)
	}

	// Build confirmation view using tui Stack
	views := []tui.View{
		tui.Divider(),
		tui.Text(" %s", actionSummary).Bold(),
	}

	if contentPreview != "" {
		views = append(views, tui.Divider().Char('-'))
		for _, line := range strings.Split(contentPreview, "\n") {
			views = append(views, tui.Text(" %s", line).Hint())
		}
	}

	views = append(views,
		tui.Divider().Char('-'),
		tui.Group(
			tui.Text(" ❯ ").Fg(tui.ColorCyan),
			tui.Text("Yes").Bold(),
			tui.Text(" (enter/y)  ").Hint(),
			tui.Text("No").Hint(),
			tui.Text(" (n/esc)").Hint(),
		),
	)

	// Count lines for clearing later
	lineCount := 5 // newline + divider + summary + dashed divider + options + trailing newline
	if contentPreview != "" {
		lineCount += 1 + len(strings.Split(contentPreview, "\n")) // dashed divider + content lines
	}

	// Print the confirmation UI as a stacked view
	// Ensure we start on a fresh line at column 0
	tui.Newline()
	tui.MoveToLineStart()
	tui.Print(tui.Stack(views...))
	tui.Newline() // Ensure we end on a fresh line

	// Hide cursor during confirmation
	tui.HideCursor()
	defer tui.ShowCursor()

	// Read keypresses until we get a valid one
	var approved bool
	for {
		key, err := a.readSingleKey()
		if err != nil {
			return false, err
		}

		switch key {
		case 0:
			continue // Ignore special keys
		case '\r', '\n', 'y', 'Y':
			approved = true
		case 'n', 'N', 0x1b:
			approved = false
		default:
			continue
		}
		break
	}

	// Clear the confirmation UI
	for i := 0; i < lineCount; i++ {
		tui.MoveCursorUp(1)
		tui.ClearLine()
	}

	// Restart live updates if still processing
	if a.processing {
		a.startLiveUpdates()
	}

	return approved, nil
}

// SelectTool prompts the user to select one option
func (a *App) SelectTool(ctx context.Context, req *dive.SelectRequest) (*dive.SelectResponse, error) {
	// Pause live updates
	if a.live != nil {
		a.live.Clear()
	}

	// Build options view
	views := []tui.View{
		tui.Text(" SELECT ").Bold().
			Style(tui.NewStyle().WithBgRGB(tui.RGB{R: 80, G: 150, B: 220}).WithFgRGB(tui.RGB{R: 255, G: 255, B: 255})),
		tui.Text(""),
		tui.Text(" %s", req.Title).Bold(),
	}

	if req.Message != "" {
		views = append(views, tui.Text(" %s", req.Message).Muted())
	}
	views = append(views, tui.Text(""))

	defaultIdx := 0
	for i, opt := range req.Options {
		marker := "  "
		if opt.Default {
			marker = "* "
			defaultIdx = i
		}
		optView := tui.Text(" %s%d) %s", marker, i+1, opt.Label)
		if opt.Description != "" {
			views = append(views, tui.Group(optView, tui.Text(" - %s", opt.Description).Muted()))
		} else {
			views = append(views, optView)
		}
	}

	views = append(views, tui.Text(""))
	views = append(views, tui.Text(" Enter number (or press Enter for default): ").Hint())

	tui.Print(tui.Padding(1, tui.Stack(views...)))

	// Read selection
	input, err := a.readInput()
	if err != nil {
		return &dive.SelectResponse{Canceled: true}, err
	}

	if input == "" {
		// Use default
		return &dive.SelectResponse{Value: req.Options[defaultIdx].Value}, nil
	}

	// Parse number
	var idx int
	if _, err := fmt.Sscanf(input, "%d", &idx); err == nil {
		if idx >= 1 && idx <= len(req.Options) {
			return &dive.SelectResponse{Value: req.Options[idx-1].Value}, nil
		}
	}

	return &dive.SelectResponse{Canceled: true}, nil
}

// MultiSelectTool prompts for multiple selections
func (a *App) MultiSelectTool(ctx context.Context, req *dive.MultiSelectRequest) (*dive.MultiSelectResponse, error) {
	// Pause live updates
	if a.live != nil {
		a.live.Clear()
	}

	// Build options view
	views := []tui.View{
		tui.Text(" MULTI-SELECT ").Bold().
			Style(tui.NewStyle().WithBgRGB(tui.RGB{R: 150, G: 80, B: 180}).WithFgRGB(tui.RGB{R: 255, G: 255, B: 255})),
		tui.Text(""),
		tui.Text(" %s", req.Title).Bold(),
	}

	if req.Message != "" {
		views = append(views, tui.Text(" %s", req.Message).Muted())
	}
	views = append(views, tui.Text(""))

	for i, opt := range req.Options {
		checkbox := "[ ]"
		if opt.Default {
			checkbox = "[x]"
		}
		views = append(views, tui.Text("  %s %d) %s", checkbox, i+1, opt.Label))
	}

	views = append(views, tui.Text(""))
	views = append(views, tui.Text(" Enter numbers separated by commas (e.g., 1,3,5) or Enter for defaults: ").Hint())

	tui.Print(tui.Padding(1, tui.Stack(views...)))

	// Read selection
	input, err := a.readInput()
	if err != nil {
		return &dive.MultiSelectResponse{Canceled: true}, err
	}

	if input == "" {
		// Use defaults
		var values []string
		for _, opt := range req.Options {
			if opt.Default {
				values = append(values, opt.Value)
			}
		}
		return &dive.MultiSelectResponse{Values: values}, nil
	}

	// Parse comma-separated numbers
	var values []string
	for _, part := range strings.Split(input, ",") {
		var idx int
		if _, err := fmt.Sscanf(strings.TrimSpace(part), "%d", &idx); err == nil {
			if idx >= 1 && idx <= len(req.Options) {
				values = append(values, req.Options[idx-1].Value)
			}
		}
	}

	return &dive.MultiSelectResponse{Values: values}, nil
}

// InputTool prompts for text input
func (a *App) InputTool(ctx context.Context, req *dive.InputRequest) (*dive.InputResponse, error) {
	// Pause live updates
	if a.live != nil {
		a.live.Clear()
	}

	views := []tui.View{
		tui.Text(" INPUT ").Bold().
			Style(tui.NewStyle().WithBgRGB(tui.RGB{R: 80, G: 180, B: 120}).WithFgRGB(tui.RGB{R: 255, G: 255, B: 255})),
		tui.Text(""),
		tui.Text(" %s", req.Title).Bold(),
	}

	if req.Message != "" {
		views = append(views, tui.Text(" %s", req.Message).Muted())
	}
	if req.Default != "" {
		views = append(views, tui.Text(" (default: %s)", req.Default).Hint())
	}
	views = append(views, tui.Text(""))

	tui.Print(tui.Padding(1, tui.Stack(views...)))
	fmt.Print("  > ")

	input, err := a.readInput()
	if err != nil {
		return &dive.InputResponse{Canceled: true}, err
	}

	if input == "" {
		input = req.Default
	}
	return &dive.InputResponse{Value: input}, nil
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
