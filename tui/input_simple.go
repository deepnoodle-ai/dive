package tui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"

	"golang.org/x/term"
)

// ReadSecure reads input with optional masking (for passwords)
func (i *Input) ReadSecure() (string, error) {
	// Draw prompt
	if i.beforeLine {
		i.drawHorizontalLine()
	}

	fmt.Print(i.promptStyle.Apply(i.prompt))

	// Show placeholder if set
	if i.placeholder != "" && i.mask == 0 {
		placeholderStyle := NewStyle().WithForeground(ColorBrightBlack)
		fmt.Print(placeholderStyle.Apply(i.placeholder))
		i.terminal.MoveCursorLeft(utf8.RuneCountInString(i.placeholder))
	}

	var result string
	var err error

	if i.mask != 0 {
		// Use terminal package for secure password input
		fd := int(syscall.Stdin)

		// Read password with masking
		bytePassword, err := term.ReadPassword(fd)
		if err != nil {
			return "", err
		}
		result = string(bytePassword)
		fmt.Println() // term.ReadPassword doesn't add newline
	} else {
		// Regular input with buffered reader
		reader := bufio.NewReader(os.Stdin)
		result, err = reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		// Trim newline
		result = strings.TrimSuffix(result, "\n")
		result = strings.TrimSuffix(result, "\r")
	}

	if i.afterLine {
		i.drawHorizontalLine()
	}

	i.AddToHistory(result)
	return result, nil
}

// ReadWithSuggestions reads input with inline suggestions
func (i *Input) ReadWithSuggestions() (string, error) {
	// Draw prompt
	if i.beforeLine {
		i.drawHorizontalLine()
	}

	fmt.Print(i.promptStyle.Apply(i.prompt))

	// Show available suggestions if any
	if i.showSuggestions && len(i.suggestions) > 0 {
		fmt.Println()
		suggestionStyle := NewStyle().WithForeground(ColorBrightBlack)
		fmt.Print(suggestionStyle.Apply("  Available: "))
		for idx, s := range i.suggestions {
			if idx > 0 {
				fmt.Print(", ")
			}
			fmt.Print(s)
			if idx >= 4 { // Show max 5
				if len(i.suggestions) > 5 {
					fmt.Printf(" (+%d more)", len(i.suggestions)-5)
				}
				break
			}
		}
		fmt.Println()
		fmt.Print(i.promptStyle.Apply(i.prompt))
	}

	// Regular input
	reader := bufio.NewReader(os.Stdin)
	result, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	// Trim newline
	result = strings.TrimSuffix(result, "\n")
	result = strings.TrimSuffix(result, "\r")

	// Check if input matches a suggestion prefix and offer completion
	if i.showSuggestions && len(i.suggestions) > 0 && result != "" {
		matches := []string{}
		for _, s := range i.suggestions {
			if strings.HasPrefix(strings.ToLower(s), strings.ToLower(result)) {
				matches = append(matches, s)
			}
		}

		if len(matches) == 1 && matches[0] != result {
			// Single match, offer to complete
			fmt.Printf("  → Did you mean '%s'? (y/n) ", matches[0])
			confirm := bufio.NewReader(os.Stdin)
			response, _ := confirm.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response == "y" || response == "yes" {
				result = matches[0]
			}
		} else if len(matches) > 1 {
			// Multiple matches, show them
			fmt.Println("  → Multiple matches found:")
			for idx, m := range matches {
				fmt.Printf("     %d. %s\n", idx+1, m)
				if idx >= 4 {
					break
				}
			}
		}
	}

	if i.afterLine {
		i.drawHorizontalLine()
	}

	i.AddToHistory(result)
	return result, nil
}

// ReadBasic performs basic line reading without any special features
func (i *Input) ReadBasic() (string, error) {
	// Draw prompt
	if i.beforeLine {
		i.drawHorizontalLine()
	}

	fmt.Print(i.promptStyle.Apply(i.prompt))

	// Show placeholder
	if i.placeholder != "" {
		placeholderStyle := NewStyle().WithForeground(ColorBrightBlack)
		fmt.Print(placeholderStyle.Apply("(" + i.placeholder + ") "))
	}

	// Use scanner for simple line reading
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		result := scanner.Text()

		if i.afterLine {
			i.drawHorizontalLine()
		}

		i.AddToHistory(result)
		return result, nil
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("no input")
}