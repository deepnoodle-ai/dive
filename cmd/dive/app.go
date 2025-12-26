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

	// Scanner for line input
	scanner *bufio.Scanner
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
		scanner:       bufio.NewScanner(os.Stdin),
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

// printInputArea prints the complete input area with cursor positioned for typing
// Uses cursor save/restore so footer is visible while typing
func (a *App) printInputArea() {
	tui.Newline()                       // Blank line
	tui.Print(tui.Divider())            // Divider above input
	tui.Print(tui.Text(" > ").Fg(tui.ColorCyan))
	tui.SaveCursor()                    // Save cursor position
	tui.Newline()                       // Move to next line
	tui.Print(tui.Divider())            // Divider below input
	footer := tui.Group(
		tui.Text(" Enter: send ").Hint(),
		tui.Text(" @file: include ").Hint(),
		tui.Text(" /help: commands ").Hint(),
		tui.Spacer(),
		tui.Text(" Ctrl+C: exit ").Hint(),
	)
	tui.Print(footer)
	tui.RestoreCursor()                 // Restore cursor to after prompt
}

// readInput reads a line of input from the user
func (a *App) readInput() (string, error) {
	if !a.scanner.Scan() {
		if err := a.scanner.Err(); err != nil {
			return "", err
		}
		return "", context.Canceled // EOF
	}
	return strings.TrimSpace(a.scanner.Text()), nil
}

// clearLines clears n lines above cursor
func (a *App) clearLines(n int) {
	for i := 0; i < n; i++ {
		tui.MoveCursorUp(1)
		tui.ClearLine()
	}
}

// clearInputArea clears the input area (blank + divider + input + divider + footer)
func (a *App) clearInputArea(input string) {
	inputLines := a.calculateInputLines(input)
	// blank (1) + divider above (1) + input (N) + divider below (1) + footer (1) = 4 + N
	totalLines := 4 + inputLines
	// After Enter, cursor is on divider-below line. Move to footer line, then clear up.
	tui.MoveCursorDown(1) // Move down 1 line to footer
	a.clearLines(totalLines)
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
func (a *App) readSingleKey() (rune, error) {
	// Put terminal in raw mode
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			// Fallback to line input
			return a.readSingleKeyFallback()
		}
		defer term.Restore(fd, oldState)

		// Read single byte
		buf := make([]byte, 1)
		_, err = os.Stdin.Read(buf)
		if err != nil {
			return 0, err
		}
		fmt.Println() // Move to next line after keypress
		return rune(buf[0]), nil
	}

	return a.readSingleKeyFallback()
}

// readSingleKeyFallback reads input when terminal isn't available
func (a *App) readSingleKeyFallback() (rune, error) {
	if !a.scanner.Scan() {
		return 0, context.Canceled
	}
	input := strings.TrimSpace(a.scanner.Text())
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

	// Stop live updates
	a.stopLiveUpdates()

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
	}
	a.mu.Unlock()

	// Print final output to scroll history
	a.printRecentMessages()

	return nil
}

func (a *App) startLiveUpdates() {
	a.live = tui.NewLivePrinter()
	a.done = make(chan struct{})
	a.ticker = time.NewTicker(time.Second / 30) // 30 FPS

	// Initial render
	a.updateLiveView()

	// Start animation ticker
	go func() {
		for {
			select {
			case <-a.ticker.C:
				a.mu.Lock()
				a.frame++
				a.mu.Unlock()
				a.updateLiveView()
			case <-a.done:
				return
			}
		}
	}()
}

func (a *App) stopLiveUpdates() {
	if a.ticker != nil {
		a.ticker.Stop()
	}
	if a.done != nil {
		close(a.done)
	}
	if a.live != nil {
		a.live.Clear()
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

	return tui.Padding(1, tui.Stack(views...).Gap(1))
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
	// Pause live updates
	if a.live != nil {
		a.live.Clear()
	}

	// Print current state
	a.printRecentMessages()

	// Print confirmation prompt
	confirmView := tui.Padding(1,
		tui.Stack(
			tui.Text(" CONFIRM ").Bold().
				Style(tui.NewStyle().WithBgRGB(tui.RGB{R: 200, G: 150, B: 50}).WithFgRGB(tui.RGB{R: 0, G: 0, B: 0})),
			tui.Text(""),
			tui.Text(" %s", summary).Bold(),
			tui.Text(" Tool: %s", toolName).Muted(),
			tui.Text(""),
			tui.Group(
				tui.Text(" Press "),
				tui.Text("y").Bold().Success(),
				tui.Text(" to confirm, "),
				tui.Text("n").Bold().Error(),
				tui.Text(" to cancel "),
			),
		),
	)
	tui.Print(confirmView)

	// Read single keypress
	key, err := a.readSingleKey()
	if err != nil {
		return false, err
	}

	approved := key == 'y' || key == 'Y'

	// Resume live updates if still processing
	if a.processing {
		a.live = tui.NewLivePrinter()
		a.updateLiveView()
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
	re := regexp.MustCompile(`@([^\s]+)`)

	var lastErr error
	result := re.ReplaceAllStringFunc(input, func(match string) string {
		path := match[1:] // Remove @

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

		// Format as XML-style tag
		return fmt.Sprintf("\n<file path=\"%s\">\n%s\n</file>\n", path, string(content))
	})

	return result, lastErr
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
