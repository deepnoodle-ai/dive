package tui

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Button represents a clickable button component
type Button struct {
	X, Y       int
	Width      int
	Label      string
	Style      Style
	HoverStyle Style
	Focused    bool
	Hovered    bool
	OnClick    func()
}

// NewButton creates a new button
func NewButton(x, y int, label string, onClick func()) *Button {
	return &Button{
		X:          x,
		Y:          y,
		Width:      len(label) + 4, // Add padding
		Label:      label,
		Style:      NewStyle().WithBackground(ColorBlue).WithForeground(ColorWhite),
		HoverStyle: NewStyle().WithBackground(ColorCyan).WithForeground(ColorBlack).WithBold(),
		OnClick:    onClick,
	}
}

// Draw renders the button
func (b *Button) Draw(terminal *Terminal) {
	// Save current cursor position and hide cursor to prevent flash
	terminal.SaveCursor()
	terminal.HideCursor()

	style := b.Style
	if b.Hovered {
		style = b.HoverStyle
	}
	if b.Focused {
		style = style.WithUnderline()
	}

	// Draw button with borders
	buttonText := fmt.Sprintf(" %s ", b.Label)
	if len(buttonText) < b.Width {
		padding := b.Width - len(buttonText)
		buttonText += strings.Repeat(" ", padding)
	}

	terminal.MoveCursor(b.X, b.Y)
	terminal.Print(style.Apply("[" + buttonText + "]"))

	// Restore cursor position and show cursor
	terminal.RestoreCursor()
	terminal.ShowCursor()
}

// GetRegion returns the mouse region for this button
func (b *Button) GetRegion() *MouseRegion {
	return &MouseRegion{
		X:      b.X,
		Y:      b.Y,
		Width:  b.Width + 2, // Include brackets
		Height: 1,
		Label:  b.Label,
		Handler: func(event *MouseEvent) {
			if b.OnClick != nil {
				b.OnClick()
			}
		},
		HoverHandler: func(hovering bool) {
			b.Hovered = hovering
		},
	}
}

// TabCompleter handles tab completion functionality
type TabCompleter struct {
	suggestions       []string
	currentIndex      int
	Visible           bool
	maxVisible        int
	prefix            string
	X, Y              int
	Width             int
	selectedStyle     Style
	normalStyle       Style
	OnSelect          func(string)
	clearDropdown     bool
	lastDrawnLines    int
	savedContent      []string // Store content that dropdown overlays
}

// NewTabCompleter creates a new tab completer
func NewTabCompleter() *TabCompleter {
	return &TabCompleter{
		suggestions:   make([]string, 0),
		maxVisible:    5,
		currentIndex:  0,
		selectedStyle: NewStyle().WithBackground(ColorBlue).WithForeground(ColorWhite),
		normalStyle:   NewStyle().WithBackground(ColorBlack).WithForeground(ColorWhite),
	}
}

// SetSuggestions updates the list of suggestions
func (tc *TabCompleter) SetSuggestions(suggestions []string, prefix string) {
	tc.suggestions = suggestions
	tc.prefix = prefix
	tc.currentIndex = 0
	tc.Visible = len(suggestions) > 0
}

// Show displays the completion dropdown
func (tc *TabCompleter) Show(x, y int, width int) {
	tc.X = x
	tc.Y = y
	tc.Width = width
	tc.Visible = true
}

// Hide hides the completion dropdown
func (tc *TabCompleter) Hide() {
	tc.Visible = false
	tc.clearDropdown = true
}

// SelectNext moves to the next suggestion
func (tc *TabCompleter) SelectNext() {
	if len(tc.suggestions) > 0 {
		tc.currentIndex = (tc.currentIndex + 1) % len(tc.suggestions)
	}
}

// SelectPrev moves to the previous suggestion
func (tc *TabCompleter) SelectPrev() {
	if len(tc.suggestions) > 0 {
		tc.currentIndex--
		if tc.currentIndex < 0 {
			tc.currentIndex = len(tc.suggestions) - 1
		}
	}
}

// GetSelected returns the currently selected suggestion
func (tc *TabCompleter) GetSelected() string {
	if tc.currentIndex < len(tc.suggestions) {
		return tc.suggestions[tc.currentIndex]
	}
	return ""
}

// Draw renders the tab completion dropdown
func (tc *TabCompleter) Draw(terminal *Terminal) {
	// Handle clearing if dropdown was hidden
	if tc.clearDropdown && tc.lastDrawnLines > 0 {
		for i := 0; i <= tc.lastDrawnLines; i++ {
			terminal.MoveCursor(0, tc.Y + 1 + i) // Clear from start of line
			terminal.ClearLine() // Clear entire line
		}
		tc.clearDropdown = false
		tc.lastDrawnLines = 0
		tc.savedContent = nil
		return
	}

	if !tc.Visible || len(tc.suggestions) == 0 {
		return
	}

	// Save cursor position and hide it to prevent flashing
	terminal.SaveCursor()
	terminal.HideCursor()

	// Force a flush to ensure any pending output is displayed first
	terminal.Flush()

	// Use a background color to ensure dropdown is visible over other content
	dropdownBg := ColorBlack

	// Calculate visible range
	start := 0
	end := len(tc.suggestions)
	if end > tc.maxVisible {
		// Show window around current selection
		start = tc.currentIndex - tc.maxVisible/2
		if start < 0 {
			start = 0
		}
		end = start + tc.maxVisible
		if end > len(tc.suggestions) {
			end = len(tc.suggestions)
			start = end - tc.maxVisible
		}
	}

	// Track how many lines we're drawing for cleanup later
	tc.lastDrawnLines = (end - start) + 1 // +1 for bottom border

	// Save content that will be overwritten (for proper layering)
	if tc.savedContent == nil {
		tc.savedContent = make([]string, tc.lastDrawnLines)
	}

	// Draw dropdown box with proper clearing
	for i := start; i < end; i++ {
		y := tc.Y + 1 + (i - start)

		// First, save cursor and clear the entire line properly
		terminal.SaveCursor()
		terminal.MoveCursor(0, y) // Move to start of line
		terminal.ClearLine() // Clear entire line
		terminal.MoveCursor(tc.X, y) // Move back to dropdown position
		terminal.RestoreCursor()
		terminal.MoveCursor(tc.X, y) // Position for drawing

		// Format suggestion
		suggestion := tc.suggestions[i]
		displayWidth := runewidth.StringWidth(suggestion)
		if displayWidth > tc.Width-4 { // account for indicator and borders
			// Truncate to fit width
			truncated := runewidth.Truncate(suggestion, tc.Width-7, "...")
			suggestion = truncated
		}

		// Apply style with solid background for visibility
		style := tc.normalStyle.WithBackground(dropdownBg)
		indicator := "  "
		if i == tc.currentIndex {
			style = tc.selectedStyle.WithBackground(dropdownBg)
			indicator = "▶ "
		}

		// Draw suggestion with border
		line := fmt.Sprintf("│%s%s", indicator, suggestion)
		// Calculate actual display width for proper padding
		lineWidth := runewidth.StringWidth(line)
		padding := tc.Width - lineWidth - 1 // -1 for the closing border
		if padding > 0 {
			line += strings.Repeat(" ", padding)
		}
		line += "│"

		// Print the line, ensuring full width is covered with background
		terminal.Print(style.Apply(line))
	}

	// Draw bottom border
	y := tc.Y + 1 + (end - start)
	terminal.SaveCursor()
	terminal.MoveCursor(0, y) // Move to start of line
	terminal.ClearLine() // Clear entire line
	terminal.MoveCursor(tc.X, y) // Move back to dropdown position
	terminal.RestoreCursor()
	terminal.MoveCursor(tc.X, y)
	border := "└" + strings.Repeat("─", tc.Width-2) + "┘"
	// Ensure border has background for visibility
	borderStyle := tc.normalStyle.WithBackground(dropdownBg)
	terminal.Print(borderStyle.Apply(border))

	// Show scroll indicators if needed
	if start > 0 {
		terminal.MoveCursor(tc.X+tc.Width-3, tc.Y+1)
		terminal.Print("↑")
	}
	if end < len(tc.suggestions) {
		terminal.MoveCursor(tc.X+tc.Width-3, y-1)
		terminal.Print("↓")
	}

	// Restore cursor position and visibility
	terminal.RestoreCursor()
	terminal.ShowCursor()

	// Force flush to ensure dropdown is displayed immediately
	terminal.Flush()
}

// GetRegions returns mouse regions for clickable suggestions
func (tc *TabCompleter) GetRegions() []*MouseRegion {
	if !tc.Visible || len(tc.suggestions) == 0 {
		return nil
	}

	regions := make([]*MouseRegion, 0)

	// Calculate visible range
	start := 0
	end := len(tc.suggestions)
	if end > tc.maxVisible {
		start = tc.currentIndex - tc.maxVisible/2
		if start < 0 {
			start = 0
		}
		end = start + tc.maxVisible
		if end > len(tc.suggestions) {
			end = len(tc.suggestions)
			start = end - tc.maxVisible
		}
	}

	// Create regions for each visible suggestion
	for i := start; i < end; i++ {
		idx := i // Capture for closure
		y := tc.Y + 1 + (i - start)

		region := &MouseRegion{
			X:      tc.X,
			Y:      y,
			Width:  tc.Width,
			Height: 1,
			Label:  tc.suggestions[idx],
			Handler: func(event *MouseEvent) {
				tc.currentIndex = idx
				if tc.OnSelect != nil {
					tc.OnSelect(tc.suggestions[idx])
				}
			},
			HoverHandler: func(hovering bool) {
				if hovering {
					tc.currentIndex = idx
				}
			},
		}
		regions = append(regions, region)
	}

	return regions
}

// RadioGroup represents a group of radio buttons
type RadioGroup struct {
	X, Y         int
	Options      []string
	Selected     int
	Style        Style
	SelectedStyle Style
	OnChange     func(int, string)
}

// NewRadioGroup creates a new radio button group
func NewRadioGroup(x, y int, options []string) *RadioGroup {
	return &RadioGroup{
		X:             x,
		Y:             y,
		Options:       options,
		Selected:      0,
		Style:         NewStyle(),
		SelectedStyle: NewStyle().WithForeground(ColorGreen).WithBold(),
	}
}

// Draw renders the radio group
func (rg *RadioGroup) Draw(terminal *Terminal) {
	for i, option := range rg.Options {
		terminal.MoveCursor(rg.X, rg.Y+i)

		style := rg.Style
		indicator := "○"
		if i == rg.Selected {
			style = rg.SelectedStyle
			indicator = "●"
		}

		terminal.Print(style.Apply(fmt.Sprintf(" %s %s", indicator, option)))
	}
}

// GetRegions returns mouse regions for the radio buttons
func (rg *RadioGroup) GetRegions() []*MouseRegion {
	regions := make([]*MouseRegion, 0)

	for i, option := range rg.Options {
		idx := i // Capture for closure

		region := &MouseRegion{
			X:      rg.X,
			Y:      rg.Y + i,
			Width:  len(option) + 4,
			Height: 1,
			Label:  option,
			Handler: func(event *MouseEvent) {
				rg.Selected = idx
				if rg.OnChange != nil {
					rg.OnChange(idx, rg.Options[idx])
				}
			},
		}
		regions = append(regions, region)
	}

	return regions
}