package tui

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// ReadInteractive reads input with real-time feedback for masking and suggestions
func (i *Input) ReadInteractive() (string, error) {
	// Draw initial setup
	if i.beforeLine {
		i.drawHorizontalLine()
	}

	// Show prompt
	fmt.Print(i.promptStyle.Apply(i.prompt))

	// Show placeholder
	if i.placeholder != "" {
		placeholderStyle := NewStyle().WithForeground(ColorBrightBlack)
		if i.mask != 0 {
			// For masked input, show placeholder as hint
			maskedPlaceholder := strings.Repeat(string(i.mask), utf8.RuneCountInString(i.placeholder))
			fmt.Print(placeholderStyle.Apply(maskedPlaceholder))
			i.terminal.MoveCursorLeft(len(maskedPlaceholder))
		} else {
			fmt.Print(placeholderStyle.Apply(i.placeholder))
			i.terminal.MoveCursorLeft(utf8.RuneCountInString(i.placeholder))
		}
	}

	// Reserve space for suggestions if needed
	if i.showSuggestions && len(i.suggestions) > 0 {
		fmt.Println()              // Create space for suggestions
		i.terminal.MoveCursorUp(1) // Move back to input line
		i.terminal.MoveCursorRight(utf8.RuneCountInString(i.prompt))
	}

	// Get the file descriptor for stdin
	fd := int(os.Stdin.Fd())

	// Save the current terminal state
	oldState, err := term.GetState(fd)
	if err != nil {
		return "", fmt.Errorf("failed to get terminal state: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Set terminal to raw mode
	rawState, err := term.MakeRaw(fd)
	if err != nil {
		return "", fmt.Errorf("failed to set raw mode: %w", err)
	}
	_ = rawState // We'll restore using oldState instead

	var buffer []rune
	var displayBuffer []rune // What we show on screen
	cursorPos := 0

	// Initial suggestions display
	if i.showSuggestions && len(i.suggestions) > 0 {
		i.showSuggestionsBelow("", i.suggestions)
	}

	for {
		// Read a single byte
		buf := make([]byte, 1)
		_, err := os.Stdin.Read(buf)
		if err != nil {
			return "", err
		}

		switch buf[0] {
		case 0x0D, 0x0A: // Enter
			// Clean up suggestions line
			if i.showSuggestions && len(i.suggestions) > 0 {
				i.terminal.MoveCursorDown(1)
				i.terminal.ClearLine()
				i.terminal.MoveCursorUp(1)
			}

			// Restore terminal and move to new line
			term.Restore(fd, oldState)
			fmt.Println()

			if i.afterLine {
				i.drawHorizontalLine()
			}

			result := string(buffer)
			i.AddToHistory(result)
			return result, nil

		case 0x7F, 0x08: // Backspace
			if cursorPos > 0 && len(buffer) > 0 {
				// Remove character from buffer
				buffer = append(buffer[:cursorPos-1], buffer[cursorPos:]...)
				displayBuffer = append(displayBuffer[:cursorPos-1], displayBuffer[cursorPos:]...)
				cursorPos--

				// Redisplay the line
				i.redrawInteractiveLine(displayBuffer, cursorPos)

				// Update suggestions
				if i.showSuggestions {
					i.updateSuggestionsLive(string(buffer))
				}
			}

		case 0x1B: // Escape or arrow keys
			// Try to read more for escape sequences
			seq := make([]byte, 2)
			n, _ := os.Stdin.Read(seq)
			if n == 2 && seq[0] == '[' {
				switch seq[1] {
				case 'D': // Left arrow
					if cursorPos > 0 {
						i.terminal.MoveCursorLeft(1)
						cursorPos--
					}
				case 'C': // Right arrow
					if cursorPos < len(buffer) {
						i.terminal.MoveCursorRight(1)
						cursorPos++
					}
				}
			} else {
				// Just escape - cancel
				if i.showSuggestions && len(i.suggestions) > 0 {
					i.terminal.MoveCursorDown(1)
					i.terminal.ClearLine()
					i.terminal.MoveCursorUp(1)
				}
				term.Restore(fd, oldState)
				fmt.Println()
				return "", fmt.Errorf("cancelled")
			}

		case 0x09: // Tab - autocomplete
			if i.showSuggestions && len(i.suggestions) > 0 {
				current := string(buffer)
				for _, suggestion := range i.suggestions {
					if strings.HasPrefix(strings.ToLower(suggestion), strings.ToLower(current)) && suggestion != current {
						// Replace buffer with suggestion
						buffer = []rune(suggestion)
						if i.mask != 0 {
							displayBuffer = []rune(strings.Repeat(string(i.mask), len(buffer)))
						} else {
							displayBuffer = buffer
						}
						cursorPos = len(buffer)

						// Redraw line
						i.redrawInteractiveLine(displayBuffer, cursorPos)

						// Update suggestions
						i.updateSuggestionsLive(string(buffer))
						break
					}
				}
			}

		case 0x03: // Ctrl-C
			if i.showSuggestions && len(i.suggestions) > 0 {
				i.terminal.MoveCursorDown(1)
				i.terminal.ClearLine()
				i.terminal.MoveCursorUp(1)
			}
			term.Restore(fd, oldState)
			fmt.Println()
			return "", fmt.Errorf("interrupted")

		default:
			// Regular character
			if buf[0] >= 32 && buf[0] < 127 {
				if i.maxLength == 0 || len(buffer) < i.maxLength {
					// Add to buffer
					newBuffer := make([]rune, 0, len(buffer)+1)
					newBuffer = append(newBuffer, buffer[:cursorPos]...)
					newBuffer = append(newBuffer, rune(buf[0]))
					newBuffer = append(newBuffer, buffer[cursorPos:]...)
					buffer = newBuffer

					// Update display buffer
					if i.mask != 0 {
						newDisplay := make([]rune, 0, len(displayBuffer)+1)
						newDisplay = append(newDisplay, displayBuffer[:cursorPos]...)
						newDisplay = append(newDisplay, i.mask)
						newDisplay = append(newDisplay, displayBuffer[cursorPos:]...)
						displayBuffer = newDisplay
					} else {
						displayBuffer = buffer
					}

					// Show the character (or mask)
					if i.mask != 0 {
						fmt.Print(string(i.mask))
					} else {
						fmt.Print(string(rune(buf[0])))
					}

					cursorPos++

					// If not at end, redraw rest of line
					if cursorPos < len(buffer) {
						i.terminal.SaveCursor()
						fmt.Print(string(displayBuffer[cursorPos:]))
						i.terminal.RestoreCursor()
					}

					// Update suggestions
					if i.showSuggestions {
						i.updateSuggestionsLive(string(buffer))
					}
				}
			}
		}
	}
}

// redrawInteractiveLine redraws the current input line
func (i *Input) redrawInteractiveLine(displayBuffer []rune, cursorPos int) {
	// Move to start of input (after prompt)
	promptLen := utf8.RuneCountInString(i.prompt)
	i.terminal.MoveCursorLeft(1000)
	i.terminal.MoveCursorRight(promptLen)

	// Clear line from cursor
	i.terminal.ClearToEndOfLine()

	// Draw buffer
	fmt.Print(string(displayBuffer))

	// Position cursor
	if cursorPos < len(displayBuffer) {
		i.terminal.MoveCursorLeft(len(displayBuffer) - cursorPos)
	}
}

// showSuggestionsBelow displays suggestions on the line below
func (i *Input) showSuggestionsBelow(current string, allSuggestions []string) {
	i.terminal.SaveCursor()
	i.terminal.MoveCursorDown(1)
	i.terminal.MoveCursorLeft(1000)
	i.terminal.ClearLine()

	// Find matching suggestions
	var matches []string
	if current == "" {
		// Show first 5 suggestions
		for idx, s := range allSuggestions {
			if idx >= 5 {
				break
			}
			matches = append(matches, s)
		}
	} else {
		// Show matching suggestions
		for _, s := range allSuggestions {
			if strings.HasPrefix(strings.ToLower(s), strings.ToLower(current)) {
				matches = append(matches, s)
				if len(matches) >= 5 {
					break
				}
			}
		}
	}

	if len(matches) > 0 {
		suggStyle := NewStyle().WithForeground(ColorBrightBlack)
		matchStyle := NewStyle().WithForeground(ColorCyan)

		fmt.Print(suggStyle.Apply("  → "))

		for idx, match := range matches {
			if idx > 0 {
				fmt.Print(suggStyle.Apply(" | "))
			}

			if current != "" && strings.HasPrefix(strings.ToLower(match), strings.ToLower(current)) {
				// Highlight matching part
				matchLen := len(current)
				if matchLen > len(match) {
					matchLen = len(match)
				}
				fmt.Print(matchStyle.Apply(match[:matchLen]))
				if matchLen < len(match) {
					fmt.Print(suggStyle.Apply(match[matchLen:]))
				}
			} else {
				fmt.Print(suggStyle.Apply(match))
			}
		}

		if len(allSuggestions) > len(matches) && len(matches) == 5 {
			fmt.Print(suggStyle.Apply(" ..."))
		}
	} else if current != "" {
		noMatchStyle := NewStyle().WithForeground(ColorBrightBlack).WithItalic()
		fmt.Print(noMatchStyle.Apply("  → No matches"))
	}

	i.terminal.RestoreCursor()
}

// updateSuggestionsLive updates the suggestions display in real-time
func (i *Input) updateSuggestionsLive(current string) {
	if !i.showSuggestions || len(i.suggestions) == 0 {
		return
	}

	i.showSuggestionsBelow(current, i.suggestions)
}
