package tui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// readKeyEventBlocking reads a key event, blocking until one is available
func (i *Input) readKeyEventBlocking() KeyEvent {
	buf := make([]byte, 16)

	// Keep trying to read until we get data
	var n int
	var err error
	for {
		n, err = os.Stdin.Read(buf)
		if err != nil && err != io.EOF {
			return KeyEvent{Key: KeyUnknown}
		}
		if n > 0 {
			break
		}
		// If we got 0 bytes but no error, continue waiting
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
			// Could be escape key or start of escape sequence
			// Try to read more bytes with a short timeout
			n2, _ := os.Stdin.Read(buf[1:])
			if n2 == 0 {
				// Just escape key
				return KeyEvent{Key: KeyEscape}
			}
			n += n2
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
		case '3':
			if n >= 4 && buf[3] == '~' {
				return KeyEvent{Key: KeyDelete}
			}
		case '5':
			if n >= 4 && buf[3] == '~' {
				return KeyEvent{Key: KeyPageUp}
			}
		case '6':
			if n >= 4 && buf[3] == '~' {
				return KeyEvent{Key: KeyPageDown}
			}
		}

		// Function keys
		if n >= 5 && buf[2] == '1' {
			switch string(buf[3:5]) {
			case "1~":
				return KeyEvent{Key: KeyF1}
			case "2~":
				return KeyEvent{Key: KeyF2}
			case "3~":
				return KeyEvent{Key: KeyF3}
			case "4~":
				return KeyEvent{Key: KeyF4}
			case "5~":
				return KeyEvent{Key: KeyF5}
			case "7~":
				return KeyEvent{Key: KeyF6}
			case "8~":
				return KeyEvent{Key: KeyF7}
			case "9~":
				return KeyEvent{Key: KeyF8}
			}
		}
		if n >= 5 && buf[2] == '2' {
			switch string(buf[3:5]) {
			case "0~":
				return KeyEvent{Key: KeyF9}
			case "1~":
				return KeyEvent{Key: KeyF10}
			case "3~":
				return KeyEvent{Key: KeyF11}
			case "4~":
				return KeyEvent{Key: KeyF12}
			}
		}
	}

	// UTF-8 character
	if n > 0 {
		r, _ := utf8.DecodeRune(buf[:n])
		if r != utf8.RuneError {
			return KeyEvent{Rune: r}
		}
	}

	return KeyEvent{Key: KeyUnknown}
}

// ReadLineSimple reads a line with simplified handling
func (i *Input) ReadLineSimple() (string, error) {
	// Draw initial prompt area
	if i.beforeLine {
		i.drawHorizontalLine()
	}

	// Show prompt
	fmt.Print(i.promptStyle.Apply(i.prompt))

	// Show placeholder if empty
	if i.placeholder != "" {
		placeholderStyle := NewStyle().WithForeground(ColorBrightBlack)
		fmt.Print(placeholderStyle.Apply(i.placeholder))
		// Move cursor back to start of placeholder
		i.terminal.MoveCursorLeft(utf8.RuneCountInString(i.placeholder))
	}

	// Enable raw mode for better key handling
	if err := i.terminal.EnableRawMode(); err != nil {
		return "", fmt.Errorf("failed to enable raw mode: %w", err)
	}
	defer func() {
		i.terminal.DisableRawMode()
		// Ensure we're on a new line when done
		fmt.Println()
	}()

	var buffer []rune
	cursorPos := 0

	// If we have suggestions, reserve space and show them
	if i.showSuggestions && len(i.suggestions) > 0 {
		// Reserve a line for suggestions
		fmt.Println()
		i.terminal.MoveCursorUp(1)
		i.terminal.SaveCursor()
		i.terminal.MoveCursorDown(1)
		i.drawInitialSuggestions()
		i.terminal.RestoreCursor()
	}

	for {
		// Read key with blocking
		event := i.readKeyEventBlocking()

		// Skip unknown keys
		if event.Key == KeyUnknown && event.Rune == 0 {
			continue
		}

		switch event.Key {
		case KeyEnter:
			// Clear suggestions line if it exists
			if i.showSuggestions && len(i.suggestions) > 0 {
				i.terminal.MoveCursorDown(1)
				i.terminal.ClearLine()
				i.terminal.MoveCursorUp(1)
			}

			if i.afterLine {
				fmt.Println()
				i.drawHorizontalLine()
			}

			result := string(buffer)
			i.AddToHistory(result)
			return result, nil

		case KeyBackspace:
			if cursorPos > 0 && len(buffer) > 0 {
				// Delete character before cursor
				buffer = append(buffer[:cursorPos-1], buffer[cursorPos:]...)
				cursorPos--

				// Redraw entire line
				i.redrawInputLine(buffer, cursorPos)

				// Update suggestions
				if i.showSuggestions {
					i.updateSuggestionsLine(string(buffer))
				}
			}

		case KeyDelete:
			if cursorPos < len(buffer) {
				buffer = append(buffer[:cursorPos], buffer[cursorPos+1:]...)
				i.redrawInputLine(buffer, cursorPos)

				if i.showSuggestions {
					i.updateSuggestionsLine(string(buffer))
				}
			}

		case KeyArrowLeft:
			if cursorPos > 0 {
				i.terminal.MoveCursorLeft(1)
				cursorPos--
			}

		case KeyArrowRight:
			if cursorPos < len(buffer) {
				i.terminal.MoveCursorRight(1)
				cursorPos++
			}

		case KeyTab:
			// Autocomplete with first matching suggestion
			if i.showSuggestions && len(i.suggestions) > 0 {
				current := string(buffer)
				for _, suggestion := range i.suggestions {
					if strings.HasPrefix(suggestion, current) && suggestion != current {
						buffer = []rune(suggestion)
						cursorPos = len(buffer)
						i.redrawInputLine(buffer, cursorPos)
						i.updateSuggestionsLine(string(buffer))
						break
					}
				}
			}

		case KeyEscape:
			// Clear suggestions if shown
			if i.showSuggestions && len(i.suggestions) > 0 {
				i.terminal.MoveCursorDown(1)
				i.terminal.ClearLine()
				i.terminal.MoveCursorUp(1)
			}
			return "", fmt.Errorf("input cancelled")

		case KeyHome:
			if cursorPos > 0 {
				i.terminal.MoveCursorLeft(cursorPos)
				cursorPos = 0
			}

		case KeyEnd:
			if cursorPos < len(buffer) {
				i.terminal.MoveCursorRight(len(buffer) - cursorPos)
				cursorPos = len(buffer)
			}

		default:
			// Regular character input
			if event.Rune != 0 && event.Rune >= 32 && event.Rune < 127 {
				if i.maxLength == 0 || len(buffer) < i.maxLength {
					// Insert character at cursor position
					newBuffer := make([]rune, 0, len(buffer)+1)
					newBuffer = append(newBuffer, buffer[:cursorPos]...)
					newBuffer = append(newBuffer, event.Rune)
					newBuffer = append(newBuffer, buffer[cursorPos:]...)
					buffer = newBuffer
					cursorPos++

					// Redraw line
					i.redrawInputLine(buffer, cursorPos)

					// Update suggestions
					if i.showSuggestions {
						i.updateSuggestionsLine(string(buffer))
					}
				}
			}
		}
	}
}

// Helper function to redraw the entire input line
func (i *Input) redrawInputLine(buffer []rune, cursorPos int) {
	// Move to start of input (after prompt)
	promptLen := utf8.RuneCountInString(i.prompt)
	i.terminal.MoveCursorLeft(1000)
	i.terminal.MoveCursorRight(promptLen)

	// Clear from cursor to end of line
	i.terminal.ClearToEndOfLine()

	// Draw the buffer with masking if needed
	if i.mask != 0 {
		maskedStr := strings.Repeat(string(i.mask), len(buffer))
		fmt.Print(maskedStr)
		// Position cursor
		i.terminal.MoveCursorLeft(len(buffer) - cursorPos)
	} else {
		fmt.Print(string(buffer))
		// Position cursor
		if cursorPos < len(buffer) {
			i.terminal.MoveCursorLeft(len(buffer) - cursorPos)
		}
	}
}

// Helper to draw initial suggestions
func (i *Input) drawInitialSuggestions() {
	if len(i.suggestions) == 0 {
		return
	}

	i.terminal.ClearLine()
	suggestionStyle := NewStyle().WithForeground(ColorBrightBlack)
	fmt.Print(suggestionStyle.Apply("  Suggestions: "))

	count := 0
	for idx, suggestion := range i.suggestions {
		if count >= 5 {
			break
		}
		if idx > 0 {
			fmt.Print(suggestionStyle.Apply(" | "))
		}
		fmt.Print(suggestionStyle.Apply(suggestion))
		count++
	}
}

// Helper to update suggestions line
func (i *Input) updateSuggestionsLine(current string) {
	if !i.showSuggestions || len(i.suggestions) == 0 {
		return
	}

	// Save cursor
	i.terminal.SaveCursor()

	// Move to suggestions line
	i.terminal.MoveCursorDown(1)
	i.terminal.MoveCursorLeft(1000)
	i.terminal.ClearLine()

	// Find matching suggestions
	var matches []string
	for _, suggestion := range i.suggestions {
		if current == "" || strings.HasPrefix(strings.ToLower(suggestion), strings.ToLower(current)) {
			matches = append(matches, suggestion)
			if len(matches) >= 5 {
				break
			}
		}
	}

	if len(matches) > 0 {
		suggestionStyle := NewStyle().WithForeground(ColorBrightBlack)
		highlightStyle := NewStyle().WithForeground(ColorCyan)

		fmt.Print(suggestionStyle.Apply("  Suggestions: "))

		for idx, match := range matches {
			if idx > 0 {
				fmt.Print(suggestionStyle.Apply(" | "))
			}

			// Highlight matching prefix
			if current != "" && strings.HasPrefix(strings.ToLower(match), strings.ToLower(current)) {
				fmt.Print(highlightStyle.Apply(match[:len(current)]))
				fmt.Print(suggestionStyle.Apply(match[len(current):]))
			} else {
				fmt.Print(suggestionStyle.Apply(match))
			}
		}
	} else {
		noMatchStyle := NewStyle().WithForeground(ColorBrightBlack).WithItalic()
		fmt.Print(noMatchStyle.Apply("  No matching suggestions"))
	}

	// Restore cursor
	i.terminal.RestoreCursor()
}
