package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ReadLineEnhanced reads a line with support for masking and suggestions
func (i *Input) ReadLineEnhanced() (string, error) {
	// Draw initial prompt area
	if i.beforeLine {
		i.drawHorizontalLine()
	}

	// Show prompt
	fmt.Print(i.promptStyle.Apply(i.prompt))

	// Show placeholder if empty
	if i.placeholder != "" && len(i.suggestions) == 0 {
		placeholderStyle := NewStyle().WithForeground(ColorBrightBlack)
		fmt.Print(placeholderStyle.Apply(i.placeholder))
		// Move cursor back to start of placeholder
		i.terminal.MoveCursorLeft(utf8.RuneCountInString(i.placeholder))
	}

	// Enable raw mode for better key handling
	if err := i.terminal.EnableRawMode(); err != nil {
		return "", err
	}
	defer i.terminal.DisableRawMode()

	var buffer []rune
	cursorPos := 0
	suggestionIndex := -1

	// If we have suggestions, show them initially
	if i.showSuggestions && len(i.suggestions) > 0 {
		i.drawSuggestionsBelow("")
	}

	for {
		// Read single key
		event := i.readKeyEvent()

		switch event.Key {
		case KeyEnter:
			// Clear suggestions if shown
			if i.showSuggestions && len(i.suggestions) > 0 {
				i.clearSuggestionsBelow()
			}

			if i.afterLine {
				fmt.Println()
				i.drawHorizontalLine()
			} else {
				fmt.Println()
			}

			result := string(buffer)
			i.AddToHistory(result)
			return result, nil

		case KeyBackspace:
			if cursorPos > 0 && len(buffer) > 0 {
				// Move cursor left
				i.terminal.MoveCursorLeft(1)

				// Delete character from buffer
				buffer = append(buffer[:cursorPos-1], buffer[cursorPos:]...)
				cursorPos--

				// Redraw the line from cursor position
				i.redrawFromCursor(buffer, cursorPos)

				// Update suggestions
				if i.showSuggestions && len(i.suggestions) > 0 {
					i.updateSuggestionsBelow(string(buffer))
				}
			}

		case KeyDelete:
			if cursorPos < len(buffer) {
				buffer = append(buffer[:cursorPos], buffer[cursorPos+1:]...)
				i.redrawFromCursor(buffer, cursorPos)

				// Update suggestions
				if i.showSuggestions && len(i.suggestions) > 0 {
					i.updateSuggestionsBelow(string(buffer))
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

		case KeyArrowUp:
			// Navigate through suggestions if available
			if i.showSuggestions && len(i.suggestions) > 0 {
				if suggestionIndex < len(i.suggestions)-1 {
					suggestionIndex++
					buffer = []rune(i.suggestions[suggestionIndex])
					cursorPos = len(buffer)
					i.redrawLine(buffer)
					i.highlightSuggestion(suggestionIndex)
				}
			} else if len(i.history) > 0 && i.historyIndex > 0 {
				// Navigate history
				i.historyIndex--
				buffer = []rune(i.history[i.historyIndex])
				cursorPos = len(buffer)
				i.redrawLine(buffer)
			}

		case KeyArrowDown:
			// Navigate through suggestions if available
			if i.showSuggestions && len(i.suggestions) > 0 {
				if suggestionIndex > -1 {
					suggestionIndex--
					if suggestionIndex >= 0 {
						buffer = []rune(i.suggestions[suggestionIndex])
					} else {
						buffer = []rune{}
					}
					cursorPos = len(buffer)
					i.redrawLine(buffer)
					i.highlightSuggestion(suggestionIndex)
				}
			} else if i.historyIndex < len(i.history)-1 {
				// Navigate history
				i.historyIndex++
				buffer = []rune(i.history[i.historyIndex])
				cursorPos = len(buffer)
				i.redrawLine(buffer)
			} else if i.historyIndex == len(i.history)-1 {
				i.historyIndex = len(i.history)
				buffer = []rune{}
				cursorPos = 0
				i.redrawLine(buffer)
			}

		case KeyTab:
			// Autocomplete with first matching suggestion
			if i.showSuggestions && len(i.suggestions) > 0 {
				current := string(buffer)
				for _, suggestion := range i.suggestions {
					if strings.HasPrefix(suggestion, current) && suggestion != current {
						buffer = []rune(suggestion)
						cursorPos = len(buffer)
						i.redrawLine(buffer)
						i.updateSuggestionsBelow(string(buffer))
						break
					}
				}
			}

		case KeyEscape:
			// Clear suggestions if shown
			if i.showSuggestions && len(i.suggestions) > 0 {
				i.clearSuggestionsBelow()
			}
			fmt.Println()
			return "", fmt.Errorf("input cancelled")

		case KeyHome:
			i.terminal.MoveCursorLeft(cursorPos)
			cursorPos = 0

		case KeyEnd:
			i.terminal.MoveCursorRight(len(buffer) - cursorPos)
			cursorPos = len(buffer)

		default:
			// Regular character input
			if event.Rune != 0 && event.Rune >= 32 && event.Rune < 127 {
				if i.maxLength == 0 || len(buffer) < i.maxLength {
					// Insert character at cursor position
					buffer = append(buffer[:cursorPos], append([]rune{event.Rune}, buffer[cursorPos:]...)...)

					// Display the character (or mask)
					if i.mask != 0 {
						fmt.Print(string(i.mask))
					} else {
						fmt.Print(string(event.Rune))
					}

					cursorPos++

					// If not at end, redraw the rest
					if cursorPos < len(buffer) {
						i.redrawFromCursor(buffer, cursorPos)
					}

					// Update suggestions
					if i.showSuggestions && len(i.suggestions) > 0 {
						i.updateSuggestionsBelow(string(buffer))
					}
				}
			}
		}
	}
}

// redrawLine clears and redraws the entire input line
func (i *Input) redrawLine(buffer []rune) {
	// Move to start of input (after prompt)
	i.terminal.MoveCursorLeft(1000)
	i.terminal.MoveCursorRight(utf8.RuneCountInString(i.prompt))

	// Clear to end of line
	i.terminal.ClearToEndOfLine()

	// Draw the buffer
	if i.mask != 0 {
		fmt.Print(strings.Repeat(string(i.mask), len(buffer)))
	} else {
		fmt.Print(string(buffer))
	}
}

// redrawFromCursor redraws the line from the current cursor position
func (i *Input) redrawFromCursor(buffer []rune, cursorPos int) {
	// Save cursor
	i.terminal.SaveCursor()

	// Clear from cursor to end
	i.terminal.ClearToEndOfLine()

	// Draw remaining buffer
	if cursorPos < len(buffer) {
		remaining := buffer[cursorPos:]
		if i.mask != 0 {
			fmt.Print(strings.Repeat(string(i.mask), len(remaining)))
		} else {
			fmt.Print(string(remaining))
		}
	}

	// Restore cursor
	i.terminal.RestoreCursor()
}

// drawSuggestionsBelow draws suggestions below the input line
func (i *Input) drawSuggestionsBelow(current string) {
	// Save cursor position
	i.terminal.SaveCursor()

	// Move down and clear line
	fmt.Println()
	i.terminal.ClearLine()

	// Find matching suggestions
	var matches []string
	for _, suggestion := range i.suggestions {
		if current == "" || strings.HasPrefix(suggestion, current) {
			matches = append(matches, suggestion)
			if len(matches) >= 5 { // Show max 5 suggestions
				break
			}
		}
	}

	if len(matches) > 0 {
		suggestionStyle := NewStyle().WithForeground(ColorBrightBlack)
		fmt.Print(suggestionStyle.Apply("  Suggestions: "))

		for idx, match := range matches {
			if idx > 0 {
				fmt.Print(suggestionStyle.Apply(" | "))
			}

			// Highlight the part that matches
			if current != "" && strings.HasPrefix(match, current) {
				matchStyle := NewStyle().WithForeground(ColorCyan)
				fmt.Print(matchStyle.Apply(current))
				fmt.Print(suggestionStyle.Apply(match[len(current):]))
			} else {
				fmt.Print(suggestionStyle.Apply(match))
			}
		}
	}

	// Restore cursor position
	i.terminal.RestoreCursor()
}

// updateSuggestionsBelow updates the suggestions display
func (i *Input) updateSuggestionsBelow(current string) {
	// Save cursor
	i.terminal.SaveCursor()

	// Move down to suggestions line
	i.terminal.MoveCursorDown(1)
	i.terminal.MoveCursorLeft(1000)
	i.terminal.ClearLine()

	// Find and display matching suggestions
	var matches []string
	for _, suggestion := range i.suggestions {
		if current == "" || strings.HasPrefix(suggestion, current) {
			matches = append(matches, suggestion)
			if len(matches) >= 5 {
				break
			}
		}
	}

	if len(matches) > 0 {
		suggestionStyle := NewStyle().WithForeground(ColorBrightBlack)
		fmt.Print(suggestionStyle.Apply("  Suggestions: "))

		for idx, match := range matches {
			if idx > 0 {
				fmt.Print(suggestionStyle.Apply(" | "))
			}

			// Highlight matching part
			if current != "" && strings.HasPrefix(match, current) {
				matchStyle := NewStyle().WithForeground(ColorCyan)
				fmt.Print(matchStyle.Apply(current))
				fmt.Print(suggestionStyle.Apply(match[len(current):]))
			} else {
				fmt.Print(suggestionStyle.Apply(match))
			}
		}
	}

	// Restore cursor
	i.terminal.RestoreCursor()
}

// clearSuggestionsBelow clears the suggestions line
func (i *Input) clearSuggestionsBelow() {
	i.terminal.SaveCursor()
	i.terminal.MoveCursorDown(1)
	i.terminal.ClearLine()
	i.terminal.RestoreCursor()
}

// highlightSuggestion highlights a specific suggestion
func (i *Input) highlightSuggestion(index int) {
	// This would need more complex implementation to highlight specific suggestion
	// For now, just update the display
	i.updateSuggestionsBelow(string(i.suggestions[index]))
}
