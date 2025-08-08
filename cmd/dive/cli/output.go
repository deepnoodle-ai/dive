package cli

import (
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

var (
	// Color scheme for workflow output
	headerStyle     = color.New(color.FgCyan, color.Bold)
	workflowSuccess = color.New(color.FgGreen, color.Bold)
	workflowError   = color.New(color.FgRed, color.Bold)
	warningStyle    = color.New(color.FgYellow, color.Bold)
	infoStyle       = color.New(color.FgCyan)
	stepStyle       = color.New(color.FgMagenta, color.Bold)
	inputStyle      = color.New(color.FgCyan)
	outputStyle     = color.New(color.FgGreen)
	timeStyle       = color.New(color.FgWhite, color.Faint)
	borderStyle     = color.New(color.FgWhite, color.Faint)
	mutedStyle      = color.New(color.FgHiBlack)
)

const (
	// Special characters for styling workflow output
	boxTopLeft     = "┌"
	boxTopRight    = "┐"
	boxBottomLeft  = "└"
	boxBottomRight = "┘"
	boxHorizontal  = "─"
	boxVertical    = "│"
	boxTeeDown     = "┬"
	boxTeeUp       = "┴"
	boxTeeRight    = "├"
	boxTeeLeft     = "┤"
	boxCross       = "┼"
	bullet         = "•"
	arrow          = "→"
	checkmark      = "✓"
	xmark          = "✗"
	hourglass      = "⏳"
	rocket         = "🚀"
	gear           = "⚙️"
)

// getTerminalWidth returns the terminal width, with a reasonable default
func getTerminalWidth() int {
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	return 120 // reasonable default
}

// wrapText wraps text to fit within the specified width
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		wordLen := displayWidth(word)
		currentLen := displayWidth(currentLine.String())

		// If adding this word would exceed the width, start a new line
		if currentLen > 0 && currentLen+1+wordLen > width {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
		}

		// Add word to current line
		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}

	// Add the last line if it has content
	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}

// stripANSI removes ANSI escape sequences from text for length calculation
func stripANSI(text string) string {
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

// displayWidth calculates the actual display width of text, accounting for wide characters
func displayWidth(text string) int {
	plainText := stripANSI(text)
	return runewidth.StringWidth(plainText)
}
