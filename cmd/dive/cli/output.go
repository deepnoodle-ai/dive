package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diveagents/dive/environment"
	"github.com/diveagents/dive/workflow"
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

// WorkflowFormatter handles attractive formatting for workflow execution
type WorkflowFormatter struct {
	showTimestamps bool
	showProgress   bool
}

// NewWorkflowFormatter creates a new formatter
func NewWorkflowFormatter() *WorkflowFormatter {
	return &WorkflowFormatter{
		showTimestamps: true,
		showProgress:   true,
	}
}

// PrintWorkflowHeader displays the initial workflow information
func (f *WorkflowFormatter) PrintWorkflowHeader(w *workflow.Workflow, inputs map[string]interface{}) {
	fmt.Println()
	f.printBox(fmt.Sprintf("%s %s", rocket, headerStyle.Sprintf("Starting Workflow: %s", w.Name())))

	if w.Description() != "" {
		fmt.Printf("   %s\n", infoStyle.Sprint(w.Description()))
	}

	if len(inputs) > 0 {
		fmt.Println()
		fmt.Printf("   %s\n", inputStyle.Sprint("Inputs:"))
		for key, value := range inputs {
			fmt.Printf("     %s %s = %v\n", bullet, inputStyle.Sprint(key), value)
		}
	}

	fmt.Println()
}

// PrintStepStart displays when a step begins
func (f *WorkflowFormatter) PrintStepStart(stepName, stepType string) {
	icon := gear
	if stepType == "prompt" {
		icon = "💭"
	} else if stepType == "action" {
		icon = "⚡"
	}

	timestamp := ""
	if f.showTimestamps {
		timestamp = timeStyle.Sprintf(" [%s]", time.Now().Format("15:04:05"))
	}

	fmt.Printf("   %s %s %s%s\n",
		icon,
		stepStyle.Sprintf("Step: %s", stepName),
		infoStyle.Sprintf("(%s)", stepType),
		timestamp,
	)
}

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

// PrintStepOutput displays step output with attractive formatting
func (f *WorkflowFormatter) PrintStepOutput(stepName, content string) {
	if strings.TrimSpace(content) == "" {
		fmt.Println()
		fmt.Printf("     %s %s\n", checkmark, workflowSuccess.Sprint("Completed (no output)"))
		fmt.Println() // Add spacing after step completion
		return
	}

	fmt.Println() // Add spacing before output
	fmt.Printf("     %s %s\n", arrow, outputStyle.Sprint("Output:"))

	termWidth := getTerminalWidth()

	// Account for indentation (5 spaces) and borders (2 chars) and some padding
	maxContentWidth := termWidth - 10
	if maxContentWidth < 40 {
		maxContentWidth = 40
	}
	if maxContentWidth > 100 {
		maxContentWidth = 100
	}

	// Split content into lines and wrap long lines
	inputLines := strings.Split(strings.TrimSpace(content), "\n")
	var wrappedLines []string

	for _, line := range inputLines {
		if strings.TrimSpace(line) == "" {
			wrappedLines = append(wrappedLines, "")
		} else {
			wrapped := wrapText(line, maxContentWidth)
			wrappedLines = append(wrappedLines, wrapped...)
		}
	}

	// Find the longest line for box sizing
	maxLineWidth := 0
	for _, line := range wrappedLines {
		lineWidth := displayWidth(line)
		if lineWidth > maxLineWidth {
			maxLineWidth = lineWidth
		}
	}

	// Ensure minimum width and cap maximum
	boxWidth := maxLineWidth
	if boxWidth < 20 {
		boxWidth = 20
	}
	if boxWidth > maxContentWidth {
		boxWidth = maxContentWidth
	}

	// Top border
	fmt.Printf("     %s%s%s\n",
		borderStyle.Sprint("╭"),
		borderStyle.Sprint(strings.Repeat("─", boxWidth+2)), // +2 for padding
		borderStyle.Sprint("╮"))

	// Format content lines with proper borders
	for _, line := range wrappedLines {
		lineLen := displayWidth(line)
		padding := strings.Repeat(" ", boxWidth-lineLen)

		fmt.Printf("     %s %s%s %s\n",
			borderStyle.Sprint("│"),
			line,
			padding,
			borderStyle.Sprint("│"))
	}

	// Bottom border
	fmt.Printf("     %s%s%s\n",
		borderStyle.Sprint("╰"),
		borderStyle.Sprint(strings.Repeat("─", boxWidth+2)),
		borderStyle.Sprint("╯"))

	fmt.Printf("     %s %s\n", checkmark, workflowSuccess.Sprint("Step completed"))
	fmt.Println()
}

// PrintStepError displays step error
func (f *WorkflowFormatter) PrintStepError(stepName string, err error) {
	fmt.Printf("     %s %s: %s\n", xmark, workflowError.Sprint("Error"), err.Error())
	fmt.Println()
}

// PrintWorkflowComplete displays successful completion
func (f *WorkflowFormatter) PrintWorkflowComplete(duration time.Duration) {
	f.printBox(fmt.Sprintf("%s %s", checkmark, workflowSuccess.Sprintf("Workflow Completed Successfully")))
	fmt.Printf("   %s Total time: %s\n", hourglass, timeStyle.Sprint(duration.Round(time.Millisecond*100)))
	fmt.Println()
}

// PrintWorkflowError displays workflow failure
func (f *WorkflowFormatter) PrintWorkflowError(err error, duration time.Duration) {
	fmt.Printf("\n💥 %s (%s)\n", workflowError.Sprint("Workflow failed"), duration.Round(time.Millisecond))
	fmt.Println(workflowError.Sprint(err))
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

// printBox creates an attractive bordered box around text
func (f *WorkflowFormatter) printBox(text string) {
	// Calculate the actual display width of the text
	contentWidth := displayWidth(text)

	// Ensure minimum width
	if contentWidth < 20 {
		contentWidth = 20
	}

	padding := 2
	totalWidth := contentWidth + padding*2

	// Top border
	fmt.Printf("   %s%s%s\n",
		borderStyle.Sprint(boxTopLeft),
		borderStyle.Sprint(strings.Repeat(boxHorizontal, totalWidth)),
		borderStyle.Sprint(boxTopRight))

	// Content with padding and proper right border
	leftPadding := strings.Repeat(" ", padding)
	rightPadding := strings.Repeat(" ", padding)
	fmt.Printf("   %s%s%s%s%s\n",
		borderStyle.Sprint(boxVertical),
		leftPadding,
		text,
		rightPadding,
		borderStyle.Sprint(boxVertical))

	// Bottom border
	fmt.Printf("   %s%s%s\n",
		borderStyle.Sprint(boxBottomLeft),
		borderStyle.Sprint(strings.Repeat(boxHorizontal, totalWidth)),
		borderStyle.Sprint(boxBottomRight))
}

// PrintExecutionStats displays execution statistics
func (f *WorkflowFormatter) PrintExecutionStats(stats environment.ExecutionStats) {
	if stats.TotalPaths > 1 {
		fmt.Printf("   %s Execution paths: %d total, %d completed, %d failed\n",
			infoStyle.Sprint("📊"),
			stats.TotalPaths,
			stats.CompletedPaths,
			stats.FailedPaths)
	}
}

func (f *WorkflowFormatter) PrintExecutionID(id string) {
	fmt.Printf("🚀 Execution ID: %s\n", id)
}

func (f *WorkflowFormatter) PrintExecutionNextSteps(id string) {
	fmt.Printf("\n🔎 To inspect the execution, run:\n")
	fmt.Printf("   %s\n", mutedStyle.Sprintf("dive executions show %s --events", id))
}

// Implement the environment.WorkflowPathFormatter interface
func (f *WorkflowFormatter) OnPathStart(path *environment.PathState) {
	// Let's not print path start events for now, it's too noisy
}

func (f *WorkflowFormatter) OnPathComplete(path *environment.PathState, err error) {
}
