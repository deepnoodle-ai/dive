package cli

import (
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
)

var (
	// Color scheme for workflow output
	headerStyle  = color.New(color.FgCyan, color.Bold)
	warningStyle = color.New(color.FgYellow, color.Bold)
	infoStyle    = color.New(color.FgCyan)
	stepStyle    = color.New(color.FgMagenta, color.Bold)
	inputStyle   = color.New(color.FgCyan)
	outputStyle  = color.New(color.FgGreen)
	timeStyle    = color.New(color.FgWhite, color.Faint)
	borderStyle  = color.New(color.FgWhite, color.Faint)
	mutedStyle   = color.New(color.FgHiBlack)
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
