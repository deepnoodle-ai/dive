package tui

import (
	"strings"
	"unicode/utf8"
)

// BorderStyle defines the characters used for drawing borders
type BorderStyle struct {
	TopLeft     string
	TopRight    string
	BottomLeft  string
	BottomRight string
	Horizontal  string
	Vertical    string
	Cross       string
	TopJoin     string
	BottomJoin  string
	LeftJoin    string
	RightJoin   string
}

// Predefined border styles
var (
	// SingleBorder uses single-line box drawing characters
	SingleBorder = BorderStyle{
		TopLeft:     "┌",
		TopRight:    "┐",
		BottomLeft:  "└",
		BottomRight: "┘",
		Horizontal:  "─",
		Vertical:    "│",
		Cross:       "┼",
		TopJoin:     "┬",
		BottomJoin:  "┴",
		LeftJoin:    "├",
		RightJoin:   "┤",
	}

	// DoubleBorder uses double-line box drawing characters
	DoubleBorder = BorderStyle{
		TopLeft:     "╔",
		TopRight:    "╗",
		BottomLeft:  "╚",
		BottomRight: "╝",
		Horizontal:  "═",
		Vertical:    "║",
		Cross:       "╬",
		TopJoin:     "╦",
		BottomJoin:  "╩",
		LeftJoin:    "╠",
		RightJoin:   "╣",
	}

	// RoundedBorder uses rounded corners
	RoundedBorder = BorderStyle{
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "╰",
		BottomRight: "╯",
		Horizontal:  "─",
		Vertical:    "│",
		Cross:       "┼",
		TopJoin:     "┬",
		BottomJoin:  "┴",
		LeftJoin:    "├",
		RightJoin:   "┤",
	}

	// ThickBorder uses thick box drawing characters
	ThickBorder = BorderStyle{
		TopLeft:     "┏",
		TopRight:    "┓",
		BottomLeft:  "┗",
		BottomRight: "┛",
		Horizontal:  "━",
		Vertical:    "┃",
		Cross:       "╋",
		TopJoin:     "┳",
		BottomJoin:  "┻",
		LeftJoin:    "┣",
		RightJoin:   "┫",
	}

	// ASCIIBorder uses ASCII characters for compatibility
	ASCIIBorder = BorderStyle{
		TopLeft:     "+",
		TopRight:    "+",
		BottomLeft:  "+",
		BottomRight: "+",
		Horizontal:  "-",
		Vertical:    "|",
		Cross:       "+",
		TopJoin:     "+",
		BottomJoin:  "+",
		LeftJoin:    "+",
		RightJoin:   "+",
	}
)

// Frame represents a bordered frame
type Frame struct {
	X           int
	Y           int
	Width       int
	Height      int
	Border      BorderStyle
	BorderStyle Style
	Title       string
	TitleStyle  Style
	TitleAlign  Alignment
}

// Alignment represents text alignment
type Alignment int

const (
	AlignLeft Alignment = iota
	AlignCenter
	AlignRight
)

// NewFrame creates a new frame with default border
func NewFrame(x, y, width, height int) *Frame {
	return &Frame{
		X:           x,
		Y:           y,
		Width:       width,
		Height:      height,
		Border:      SingleBorder,
		BorderStyle: NewStyle(),
		TitleStyle:  NewStyle(),
		TitleAlign:  AlignLeft,
	}
}

// WithBorderStyle sets the border style
func (f *Frame) WithBorderStyle(style BorderStyle) *Frame {
	f.Border = style
	return f
}

// WithColor sets the border color
func (f *Frame) WithColor(style Style) *Frame {
	f.BorderStyle = style
	return f
}

// WithTitle sets the frame title
func (f *Frame) WithTitle(title string, align Alignment) *Frame {
	f.Title = title
	f.TitleAlign = align
	return f
}

// WithTitleStyle sets the title style
func (f *Frame) WithTitleStyle(style Style) *Frame {
	f.TitleStyle = style
	return f
}

// Draw renders the frame to the terminal
func (f *Frame) Draw(t *Terminal) {
	if f.Width < 2 || f.Height < 2 {
		return
	}

	// Draw top border
	f.drawTopBorder(t)

	// Draw sides
	for i := 1; i < f.Height-1; i++ {
		t.MoveCursor(f.X, f.Y+i)
		t.Print(f.BorderStyle.Apply(f.Border.Vertical))
		t.MoveCursor(f.X+f.Width-1, f.Y+i)
		t.Print(f.BorderStyle.Apply(f.Border.Vertical))
	}

	// Draw bottom border
	f.drawBottomBorder(t)
}

func (f *Frame) drawTopBorder(t *Terminal) {
	line := f.Border.TopLeft + strings.Repeat(f.Border.Horizontal, f.Width-2) + f.Border.TopRight

	// Insert title if present
	if f.Title != "" {
		titleWithSpaces := " " + f.Title + " "
		titleLen := utf8.RuneCountInString(titleWithSpaces)

		if titleLen < f.Width-2 {
			var pos int
			switch f.TitleAlign {
			case AlignLeft:
				pos = 1 // Start right after the top-left corner
			case AlignCenter:
				pos = ((f.Width - titleLen) / 2)
				if pos < 1 {
					pos = 1
				}
			case AlignRight:
				pos = f.Width - titleLen - 1 // End before the top-right corner
				if pos < 1 {
					pos = 1
				}
			}

			lineRunes := []rune(line)
			titleRunes := []rune(titleWithSpaces)
			for i, r := range titleRunes {
				if pos+i < len(lineRunes)-1 { // Don't overwrite the top-right corner
					lineRunes[pos+i] = r
				}
			}
			line = string(lineRunes)
		}
	}

	t.MoveCursor(f.X, f.Y)

	// Apply styles
	if f.Title != "" && !f.TitleStyle.IsEmpty() {
		// Split the line to apply different styles
		parts := f.splitLineForTitle(line)
		t.Print(f.BorderStyle.Apply(parts[0]))
		t.Print(f.TitleStyle.Apply(parts[1]))
		t.Print(f.BorderStyle.Apply(parts[2]))
	} else {
		t.Print(f.BorderStyle.Apply(line))
	}
}

func (f *Frame) splitLineForTitle(line string) []string {
	if f.Title == "" {
		return []string{line, "", ""}
	}

	titleWithSpaces := " " + f.Title + " "
	// Find the title in the line (it was placed by drawTopBorder)
	// We need to work with runes to handle multi-byte characters properly
	lineRunes := []rune(line)
	titleRunes := []rune(titleWithSpaces)

	// Find where the title starts
	titleStart := -1
	for i := 0; i <= len(lineRunes)-len(titleRunes); i++ {
		match := true
		for j := 0; j < len(titleRunes); j++ {
			if lineRunes[i+j] != titleRunes[j] {
				match = false
				break
			}
		}
		if match {
			titleStart = i
			break
		}
	}

	if titleStart == -1 {
		return []string{line, "", ""}
	}

	before := string(lineRunes[:titleStart])
	title := titleWithSpaces
	after := string(lineRunes[titleStart+len(titleRunes):])

	return []string{before, title, after}
}

func (f *Frame) drawBottomBorder(t *Terminal) {
	line := f.Border.BottomLeft + strings.Repeat(f.Border.Horizontal, f.Width-2) + f.Border.BottomRight
	t.MoveCursor(f.X, f.Y+f.Height-1)
	t.Print(f.BorderStyle.Apply(line))
}

// Clear clears the content inside the frame (not the border)
func (f *Frame) Clear(t *Terminal) {
	emptyLine := strings.Repeat(" ", f.Width-2)
	for i := 1; i < f.Height-1; i++ {
		t.MoveCursor(f.X+1, f.Y+i)
		t.Print(emptyLine)
	}
}

// Box is a simpler API for drawing boxes
type Box struct {
	Content     []string
	Border      BorderStyle
	BorderStyle Style
	Padding     int
}

// NewBox creates a new box with content
func NewBox(content []string) *Box {
	return &Box{
		Content:     content,
		Border:      SingleBorder,
		BorderStyle: NewStyle(),
		Padding:     1,
	}
}

// Draw renders the box at the specified position
func (b *Box) Draw(t *Terminal, x, y int) {
	if len(b.Content) == 0 {
		return
	}

	// Calculate dimensions
	maxWidth := 0
	for _, line := range b.Content {
		if len := utf8.RuneCountInString(line); len > maxWidth {
			maxWidth = len
		}
	}

	width := maxWidth + 2 + (b.Padding * 2)
	height := len(b.Content) + 2 + (b.Padding * 2)

	// Draw the complete box line by line to avoid corruption
	// This ensures each line is drawn atomically

	// Top border
	topLine := b.Border.TopLeft + strings.Repeat(b.Border.Horizontal, width-2) + b.Border.TopRight
	t.MoveCursor(x, y)
	t.Print(b.BorderStyle.Apply(topLine))

	// Content lines with side borders
	for i := 1; i < height-1; i++ {
		t.MoveCursor(x, y+i)

		// Left border
		line := b.BorderStyle.Apply(b.Border.Vertical)

		// Content or padding
		contentIndex := i - 1 - b.Padding
		if contentIndex >= 0 && contentIndex < len(b.Content) {
			// Add leading padding
			line += strings.Repeat(" ", b.Padding)

			// Add content (truncated if necessary)
			contentLine := b.Content[contentIndex]
			maxLineWidth := width - 2 - (b.Padding * 2)
			if utf8.RuneCountInString(contentLine) > maxLineWidth {
				lineRunes := []rune(contentLine)
				contentLine = string(lineRunes[:maxLineWidth])
			}
			line += contentLine

			// Add trailing padding to fill the width
			contentWidth := utf8.RuneCountInString(contentLine)
			padding := width - 2 - b.Padding - contentWidth
			line += strings.Repeat(" ", padding)
		} else {
			// Just padding/empty space
			line += strings.Repeat(" ", width-2)
		}

		// Right border
		line += b.BorderStyle.Apply(b.Border.Vertical)

		t.Print(line)
	}

	// Bottom border
	bottomLine := b.Border.BottomLeft + strings.Repeat(b.Border.Horizontal, width-2) + b.Border.BottomRight
	t.MoveCursor(x, y+height-1)
	t.Print(b.BorderStyle.Apply(bottomLine))
}
