package tui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

// Key represents special keyboard keys
type Key int

const (
	KeyEnter Key = iota
	KeyTab
	KeyBackspace
	KeyEscape
	KeyArrowUp
	KeyArrowDown
	KeyArrowLeft
	KeyArrowRight
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyDelete
	KeyInsert
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyCtrlA
	KeyCtrlB
	KeyCtrlC
	KeyCtrlD
	KeyCtrlE
	KeyCtrlF
	KeyCtrlG
	KeyCtrlH
	KeyCtrlI
	KeyCtrlJ
	KeyCtrlK
	KeyCtrlL
	KeyCtrlM
	KeyCtrlN
	KeyCtrlO
	KeyCtrlP
	KeyCtrlQ
	KeyCtrlR
	KeyCtrlS
	KeyCtrlT
	KeyCtrlU
	KeyCtrlV
	KeyCtrlW
	KeyCtrlX
	KeyCtrlY
	KeyCtrlZ
	KeyUnknown
)

// KeyEvent represents a keyboard event
type KeyEvent struct {
	Key  Key
	Rune rune
	Alt  bool
	Ctrl bool
}

// Input handles user input with borders and hotkeys
type Input struct {
	terminal        *Terminal
	prompt          string
	promptStyle     Style
	inputStyle      Style
	borderStyle     BorderStyle
	borderColor     Style
	showBorder      bool
	history         []string
	historyIndex    int
	hotkeys         map[Key]func()
	beforeLine      bool
	afterLine       bool
	lineStyle       Style
	placeholder     string
	maxLength       int
	mask            rune
	multiline       bool
	suggestions     []string
	showSuggestions bool
}

// NewInput creates a new input handler
func NewInput(terminal *Terminal) *Input {
	return &Input{
		terminal:        terminal,
		prompt:          "> ",
		promptStyle:     NewStyle().WithForeground(ColorCyan),
		inputStyle:      NewStyle(),
		borderStyle:     SingleBorder,
		borderColor:     NewStyle().WithForeground(ColorBlue),
		showBorder:      false,
		history:         make([]string, 0),
		historyIndex:    -1,
		hotkeys:         make(map[Key]func()),
		beforeLine:      false,
		afterLine:       false,
		lineStyle:       NewStyle().WithForeground(ColorBrightBlack),
		maxLength:       0,
		mask:            0,
		multiline:       false,
		suggestions:     make([]string, 0),
		showSuggestions: false,
	}
}

// WithPrompt sets the input prompt
func (i *Input) WithPrompt(prompt string, style Style) *Input {
	i.prompt = prompt
	i.promptStyle = style
	return i
}

// WithBorder enables bordered input
func (i *Input) WithBorder(border BorderStyle, color Style) *Input {
	i.showBorder = true
	i.borderStyle = border
	i.borderColor = color
	return i
}

// WithLines adds horizontal lines before and after input
func (i *Input) WithLines(before, after bool, style Style) *Input {
	i.beforeLine = before
	i.afterLine = after
	i.lineStyle = style
	return i
}

// WithPlaceholder sets placeholder text
func (i *Input) WithPlaceholder(placeholder string) *Input {
	i.placeholder = placeholder
	return i
}

// WithMaxLength sets maximum input length
func (i *Input) WithMaxLength(length int) *Input {
	i.maxLength = length
	return i
}

// WithMask sets a mask character for password input
func (i *Input) WithMask(mask rune) *Input {
	i.mask = mask
	return i
}

// EnableMultiline enables multiline input
func (i *Input) EnableMultiline() *Input {
	i.multiline = true
	return i
}

// SetHotkey registers a hotkey handler
func (i *Input) SetHotkey(key Key, handler func()) *Input {
	i.hotkeys[key] = handler
	return i
}

// AddToHistory adds input to history
func (i *Input) AddToHistory(input string) {
	if input != "" && (len(i.history) == 0 || i.history[len(i.history)-1] != input) {
		i.history = append(i.history, input)
	}
	i.historyIndex = len(i.history)
}

// SetSuggestions sets autocomplete suggestions
func (i *Input) SetSuggestions(suggestions []string) *Input {
	i.suggestions = suggestions
	i.showSuggestions = len(suggestions) > 0
	return i
}

// Read reads user input with all configured features
func (i *Input) Read() (string, error) {
	// Draw input area
	i.drawInputArea()

	// Enable raw mode for key detection
	if err := i.terminal.EnableRawMode(); err != nil {
		return "", err
	}
	defer i.terminal.DisableRawMode()

	var buffer []rune
	cursorPos := 0

	for {
		// Read key event
		event := i.readKeyEvent()

		// Check hotkeys
		if handler, ok := i.hotkeys[event.Key]; ok {
			handler()
			continue
		}

		switch event.Key {
		case KeyEnter:
			if !i.multiline || (i.multiline && event.Ctrl) {
				i.clearInputArea()
				result := string(buffer)
				i.AddToHistory(result)
				fmt.Println()
				return result, nil
			}
			// In multiline mode, add newline
			buffer = i.insertRune(buffer, cursorPos, '\n')
			cursorPos++

		case KeyBackspace:
			if cursorPos > 0 && len(buffer) > 0 {
				buffer = i.deleteRune(buffer, cursorPos-1)
				cursorPos--
			}

		case KeyDelete:
			if cursorPos < len(buffer) {
				buffer = i.deleteRune(buffer, cursorPos)
			}

		case KeyArrowLeft:
			if cursorPos > 0 {
				cursorPos--
			}

		case KeyArrowRight:
			if cursorPos < len(buffer) {
				cursorPos++
			}

		case KeyArrowUp:
			if len(i.history) > 0 && i.historyIndex > 0 {
				i.historyIndex--
				buffer = []rune(i.history[i.historyIndex])
				cursorPos = len(buffer)
			}

		case KeyArrowDown:
			if i.historyIndex < len(i.history)-1 {
				i.historyIndex++
				buffer = []rune(i.history[i.historyIndex])
				cursorPos = len(buffer)
			} else if i.historyIndex == len(i.history)-1 {
				i.historyIndex = len(i.history)
				buffer = []rune{}
				cursorPos = 0
			}

		case KeyHome:
			cursorPos = 0

		case KeyEnd:
			cursorPos = len(buffer)

		case KeyEscape:
			i.clearInputArea()
			return "", fmt.Errorf("input cancelled")

		case KeyTab:
			// Handle autocomplete
			if i.showSuggestions && len(i.suggestions) > 0 {
				current := string(buffer)
				for _, suggestion := range i.suggestions {
					if strings.HasPrefix(suggestion, current) {
						buffer = []rune(suggestion)
						cursorPos = len(buffer)
						break
					}
				}
			}

		default:
			// Regular character input
			if event.Rune != 0 {
				if i.maxLength == 0 || len(buffer) < i.maxLength {
					buffer = i.insertRune(buffer, cursorPos, event.Rune)
					cursorPos++
				}
			}
		}

		// Update display
		i.updateDisplay(buffer, cursorPos)

		// Show suggestions if applicable
		if i.showSuggestions {
			i.drawSuggestions(string(buffer))
		}
	}
}

// ReadLine reads a single line of input (simplified version)
func (i *Input) ReadLine() (string, error) {
	if i.beforeLine {
		i.drawHorizontalLine()
	}

	// Show prompt
	fmt.Print(i.promptStyle.Apply(i.prompt))

	// Read input
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		input := scanner.Text()

		if i.afterLine {
			i.drawHorizontalLine()
		}

		i.AddToHistory(input)
		return input, nil
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("no input")
}

func (i *Input) drawInputArea() {
	width, _ := i.terminal.Size()

	if i.beforeLine {
		i.drawHorizontalLine()
	}

	if i.showBorder {
		// Draw bordered input area
		frame := NewFrame(0, 0, width, 3).
			WithBorderStyle(i.borderStyle).
			WithColor(i.borderColor)
		frame.Draw(i.terminal)
		i.terminal.MoveCursor(2, 1)
	}

	// Show prompt
	fmt.Print(i.promptStyle.Apply(i.prompt))

	// Show placeholder if needed
	if i.placeholder != "" {
		placeholderStyle := NewStyle().WithForeground(ColorBrightBlack)
		fmt.Print(placeholderStyle.Apply(i.placeholder))
		i.terminal.MoveCursorLeft(utf8.RuneCountInString(i.placeholder))
	}
}

func (i *Input) clearInputArea() {
	if i.showBorder {
		// Clear bordered area
		i.terminal.MoveCursor(0, 0)
		for j := 0; j < 3; j++ {
			i.terminal.ClearLine()
			i.terminal.MoveCursorDown(1)
		}
		i.terminal.MoveCursor(0, 0)
	} else {
		i.terminal.ClearLine()
	}
}

func (i *Input) updateDisplay(buffer []rune, cursorPos int) {
	// Clear current line
	i.terminal.MoveCursorLeft(1000)
	i.terminal.ClearToEndOfLine()

	// Redraw prompt
	fmt.Print(i.promptStyle.Apply(i.prompt))

	// Draw input text
	text := string(buffer)
	if i.mask != 0 {
		text = strings.Repeat(string(i.mask), len(buffer))
	}
	fmt.Print(i.inputStyle.Apply(text))

	// Position cursor
	if cursorPos < len(buffer) {
		i.terminal.MoveCursorLeft(len(buffer) - cursorPos)
	}
}

func (i *Input) drawHorizontalLine() {
	width, _ := i.terminal.Size()
	line := strings.Repeat("─", width)
	fmt.Println(i.lineStyle.Apply(line))
}

func (i *Input) drawSuggestions(current string) {
	// Clear suggestion area
	i.terminal.MoveCursorDown(1)
	i.terminal.ClearLine()

	matches := []string{}
	for _, suggestion := range i.suggestions {
		if strings.HasPrefix(suggestion, current) && suggestion != current {
			matches = append(matches, suggestion)
		}
	}

	if len(matches) > 0 {
		suggestionStyle := NewStyle().WithForeground(ColorBrightBlack)
		fmt.Print(suggestionStyle.Apply("  Suggestions: "))
		for idx, match := range matches {
			if idx > 2 { // Show max 3 suggestions
				break
			}
			if idx > 0 {
				fmt.Print(", ")
			}
			fmt.Print(match)
		}
	}

	// Move back to input line
	i.terminal.MoveCursorUp(1)
	i.terminal.MoveCursorRight(len(i.prompt) + len(current))
}

func (i *Input) insertRune(buffer []rune, pos int, r rune) []rune {
	if pos < 0 || pos > len(buffer) {
		return buffer
	}

	result := make([]rune, 0, len(buffer)+1)
	result = append(result, buffer[:pos]...)
	result = append(result, r)
	result = append(result, buffer[pos:]...)
	return result
}

func (i *Input) deleteRune(buffer []rune, pos int) []rune {
	if pos < 0 || pos >= len(buffer) {
		return buffer
	}

	result := make([]rune, 0, len(buffer)-1)
	result = append(result, buffer[:pos]...)
	result = append(result, buffer[pos+1:]...)
	return result
}

func (i *Input) readKeyEvent() KeyEvent {
	// This is a simplified key event reader
	// In production, you'd want more sophisticated key detection

	buf := make([]byte, 16)
	n, _ := os.Stdin.Read(buf)

	if n == 0 {
		return KeyEvent{Key: KeyUnknown}
	}

	// Check for special keys
	if n == 1 {
		switch buf[0] {
		case 0x0D, 0x0A: // Enter
			return KeyEvent{Key: KeyEnter}
		case 0x09: // Tab
			return KeyEvent{Key: KeyTab}
		case 0x7F, 0x08: // Backspace
			return KeyEvent{Key: KeyBackspace}
		case 0x1B: // Escape
			return KeyEvent{Key: KeyEscape}
		case 0x01: // Ctrl-A
			return KeyEvent{Key: KeyCtrlA}
		case 0x03: // Ctrl-C
			return KeyEvent{Key: KeyCtrlC}
		case 0x04: // Ctrl-D
			return KeyEvent{Key: KeyCtrlD}
		case 0x05: // Ctrl-E
			return KeyEvent{Key: KeyCtrlE}
		case 0x06: // Ctrl-F
			return KeyEvent{Key: KeyCtrlF}
		case 0x0B: // Ctrl-K
			return KeyEvent{Key: KeyCtrlK}
		case 0x0C: // Ctrl-L
			return KeyEvent{Key: KeyCtrlL}
		case 0x0E: // Ctrl-N
			return KeyEvent{Key: KeyCtrlN}
		case 0x10: // Ctrl-P
			return KeyEvent{Key: KeyCtrlP}
		case 0x12: // Ctrl-R
			return KeyEvent{Key: KeyCtrlR}
		case 0x13: // Ctrl-S
			return KeyEvent{Key: KeyCtrlS}
		case 0x14: // Ctrl-T
			return KeyEvent{Key: KeyCtrlT}
		case 0x15: // Ctrl-U
			return KeyEvent{Key: KeyCtrlU}
		case 0x16: // Ctrl-V
			return KeyEvent{Key: KeyCtrlV}
		case 0x17: // Ctrl-W
			return KeyEvent{Key: KeyCtrlW}
		case 0x18: // Ctrl-X
			return KeyEvent{Key: KeyCtrlX}
		case 0x19: // Ctrl-Y
			return KeyEvent{Key: KeyCtrlY}
		case 0x1A: // Ctrl-Z
			return KeyEvent{Key: KeyCtrlZ}
		default:
			if buf[0] >= 0x20 && buf[0] < 0x7F {
				return KeyEvent{Rune: rune(buf[0])}
			}
		}
	}

	// Check for escape sequences (arrows, function keys, etc.)
	if n >= 3 && buf[0] == 0x1B && buf[1] == '[' {
		switch buf[2] {
		case 'A':
			return KeyEvent{Key: KeyArrowUp}
		case 'B':
			return KeyEvent{Key: KeyArrowDown}
		case 'C':
			return KeyEvent{Key: KeyArrowRight}
		case 'D':
			return KeyEvent{Key: KeyArrowLeft}
		case 'H':
			return KeyEvent{Key: KeyHome}
		case 'F':
			return KeyEvent{Key: KeyEnd}
		}

		// Function keys
		if n >= 4 && buf[2] >= '1' && buf[2] <= '2' {
			switch string(buf[2:4]) {
			case "11":
				return KeyEvent{Key: KeyF1}
			case "12":
				return KeyEvent{Key: KeyF2}
			case "13":
				return KeyEvent{Key: KeyF3}
			case "14":
				return KeyEvent{Key: KeyF4}
			case "15":
				return KeyEvent{Key: KeyF5}
			case "17":
				return KeyEvent{Key: KeyF6}
			case "18":
				return KeyEvent{Key: KeyF7}
			case "19":
				return KeyEvent{Key: KeyF8}
			case "20":
				return KeyEvent{Key: KeyF9}
			case "21":
				return KeyEvent{Key: KeyF10}
			case "23":
				return KeyEvent{Key: KeyF11}
			case "24":
				return KeyEvent{Key: KeyF12}
			}
		}
	}

	// UTF-8 character
	r, _ := utf8.DecodeRune(buf[:n])
	return KeyEvent{Rune: r}
}
