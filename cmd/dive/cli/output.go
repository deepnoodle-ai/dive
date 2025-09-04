package cli

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// StripANSI removes ANSI escape sequences from text for length calculation
func StripANSI(text string) string {
	// Improved ANSI escape sequence removal
	result := strings.Builder{}
	inEscape := false

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		// Check for ANSI escape sequence start (\x1b[ or \033[)
		if r == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			inEscape = true
			i++ // skip the '['
			continue
		}

		if inEscape {
			// Skip characters until we find a letter (end of escape sequence)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}

		result.WriteRune(r)
	}

	return result.String()
}

// DisplayWidth calculates the actual display width of text, accounting for wide characters
func DisplayWidth(text string) int {
	plainText := StripANSI(text)
	return runewidth.StringWidth(plainText)
}
