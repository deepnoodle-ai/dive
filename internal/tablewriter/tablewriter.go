package tablewriter

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// Writer represents a table writer that formats data into an ASCII table
type Writer struct {
	out        io.Writer
	headers    []string
	rows       [][]string
	widths     []int
	maxColumns int
}

// stripANSI removes ANSI escape sequences from a string
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// displayWidth returns the display width of a string (excluding ANSI codes)
func displayWidth(s string) int {
	return len(stripANSI(s))
}

// NewWriter creates a new table writer
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		out:    w,
		rows:   make([][]string, 0),
		widths: make([]int, 0),
	}
}

// SetHeader sets the table headers
func (t *Writer) SetHeader(headers []string) {
	t.headers = headers
	t.maxColumns = len(headers)
	t.updateWidths(headers)
}

// Header is an alias for SetHeader to match the interface
func (t *Writer) Header(headers []string) {
	t.SetHeader(headers)
}

// Append adds a new row to the table
func (t *Writer) Append(row []string) {
	t.rows = append(t.rows, row)
	t.updateWidths(row)
}

// updateWidths updates the column widths based on the provided row
func (t *Writer) updateWidths(row []string) {
	// If headers are set, limit to header column count
	limit := len(row)
	if t.maxColumns > 0 && limit > t.maxColumns {
		limit = t.maxColumns
	}

	for i := 0; i < limit; i++ {
		if i >= len(t.widths) {
			t.widths = append(t.widths, 0)
		}
		if i < len(row) {
			width := displayWidth(row[i])
			if width > t.widths[i] {
				t.widths[i] = width
			}
		}
	}

	// Update maxColumns if no headers were set
	if t.maxColumns == 0 && len(t.widths) > t.maxColumns {
		t.maxColumns = len(t.widths)
	}
}

// Render outputs the table to the writer
func (t *Writer) Render() {
	if len(t.headers) == 0 && len(t.rows) == 0 {
		return
	}

	// Print top border
	t.printBorder()

	// Print headers if they exist
	if len(t.headers) > 0 {
		t.printRow(t.headers)
		t.printBorder()
	}

	// Print rows
	for _, row := range t.rows {
		t.printRow(row)
	}

	// Print bottom border
	t.printBorder()
}

// printBorder prints a horizontal border line
func (t *Writer) printBorder() {
	fmt.Fprint(t.out, "+")
	for _, width := range t.widths {
		fmt.Fprint(t.out, strings.Repeat("-", width+2))
		fmt.Fprint(t.out, "+")
	}
	fmt.Fprintln(t.out)
}

// printRow prints a single row with proper padding
func (t *Writer) printRow(row []string) {
	fmt.Fprint(t.out, "|")
	numCols := len(t.widths)

	for i := 0; i < numCols; i++ {
		cell := ""
		if i < len(row) {
			cell = row[i]
		}
		// Calculate padding needed based on display width
		cellWidth := displayWidth(cell)
		padding := t.widths[i] - cellWidth
		fmt.Fprintf(t.out, " %s%s |", cell, strings.Repeat(" ", padding))
	}
	fmt.Fprintln(t.out)
}
