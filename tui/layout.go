package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Layout manages the overall terminal layout with header, footer, and content area
type Layout struct {
	terminal       *Terminal
	header         *Header
	footer         *Footer
	contentY       int
	contentHeight  int
	autoRefresh    bool
	refreshTicker  *time.Ticker
	stopRefresh    chan bool
}

// Header represents the top section of the terminal
type Header struct {
	Left       string
	Center     string
	Right      string
	Style      Style
	Background Style
	Border     bool
	BorderStyle BorderStyle
	Height     int
}

// Footer represents the bottom section of the terminal
type Footer struct {
	Left       string
	Center     string
	Right      string
	Style      Style
	Background Style
	Border     bool
	BorderStyle BorderStyle
	Height     int
	StatusBar  bool
	StatusItems []StatusItem
}

// StatusItem represents an item in the status bar
type StatusItem struct {
	Key   string
	Value string
	Style Style
	Icon  string
}

// NewLayout creates a new layout manager
func NewLayout(terminal *Terminal) *Layout {
	_, height := terminal.Size()
	return &Layout{
		terminal:      terminal,
		contentY:      0,
		contentHeight: height,
	}
}

// SetHeader configures the header
func (l *Layout) SetHeader(header *Header) *Layout {
	l.header = header
	if header != nil && header.Height == 0 {
		header.Height = 1
		if header.Border {
			header.Height = 3
		}
	}
	l.updateContentArea()
	return l
}

// SetFooter configures the footer
func (l *Layout) SetFooter(footer *Footer) *Layout {
	l.footer = footer
	if footer != nil && footer.Height == 0 {
		footer.Height = 1
		if footer.Border {
			footer.Height = 3
		}
	}
	l.updateContentArea()
	return l
}

// EnableAutoRefresh enables automatic refresh of header/footer
func (l *Layout) EnableAutoRefresh(interval time.Duration) *Layout {
	if l.autoRefresh {
		return l
	}

	l.autoRefresh = true
	l.refreshTicker = time.NewTicker(interval)
	l.stopRefresh = make(chan bool)

	go func() {
		for {
			select {
			case <-l.refreshTicker.C:
				l.Refresh()
			case <-l.stopRefresh:
				return
			}
		}
	}()

	return l
}

// DisableAutoRefresh stops automatic refresh
func (l *Layout) DisableAutoRefresh() {
	if l.autoRefresh && l.refreshTicker != nil {
		l.refreshTicker.Stop()
		close(l.stopRefresh)
		l.autoRefresh = false
	}
}

// Draw renders the entire layout
func (l *Layout) Draw() {
	l.terminal.Clear()

	if l.header != nil {
		l.drawHeader()
	}

	if l.footer != nil {
		l.drawFooter()
	}

	// Position cursor in content area
	l.terminal.MoveCursor(0, l.contentY)
}

// Refresh updates the header and footer
func (l *Layout) Refresh() {
	// Save current cursor position
	l.terminal.SaveCursor()

	if l.header != nil {
		l.drawHeader()
	}

	if l.footer != nil {
		l.drawFooter()
	}

	// Restore cursor position
	l.terminal.RestoreCursor()
}

// ContentArea returns the Y position and height of the content area
func (l *Layout) ContentArea() (y, height int) {
	return l.contentY, l.contentHeight
}

// PrintInContent prints text in the content area
func (l *Layout) PrintInContent(text string) {
	l.terminal.MoveCursor(0, l.contentY)
	fmt.Print(text)
}

func (l *Layout) updateContentArea() {
	_, height := l.terminal.Size()

	contentY := 0
	contentHeight := height

	if l.header != nil {
		contentY += l.header.Height
		contentHeight -= l.header.Height
	}

	if l.footer != nil {
		contentHeight -= l.footer.Height
	}

	l.contentY = contentY
	l.contentHeight = contentHeight
}

func (l *Layout) drawHeader() {
	width, _ := l.terminal.Size()
	l.terminal.MoveCursor(0, 0)

	if l.header.Border {
		// Draw bordered header
		frame := NewFrame(0, 0, width, l.header.Height).
			WithBorderStyle(l.header.BorderStyle).
			WithColor(l.header.Style)
		frame.Draw(l.terminal)

		// Draw content inside border
		l.terminal.MoveCursor(1, 1)
		l.drawHeaderContent(width - 2)
	} else {
		// Draw header without border
		l.drawHeaderContent(width)
	}
}

func (l *Layout) drawHeaderContent(width int) {
	// Apply background if set
	if !l.header.Background.IsEmpty() {
		bg := strings.Repeat(" ", width)
		fmt.Print(l.header.Background.Apply(bg))
		l.terminal.MoveCursorLeft(width)
	}

	// Calculate sections
	leftLen := utf8.RuneCountInString(l.header.Left)
	centerLen := utf8.RuneCountInString(l.header.Center)
	rightLen := utf8.RuneCountInString(l.header.Right)

	// Draw left section
	if l.header.Left != "" {
		fmt.Print(l.header.Style.Apply(l.header.Left))
	}

	// Draw center section
	if l.header.Center != "" {
		centerPos := (width - centerLen) / 2
		if centerPos > leftLen {
			l.terminal.MoveCursorRight(centerPos - leftLen)
			fmt.Print(l.header.Style.Apply(l.header.Center))
		}
	}

	// Draw right section
	if l.header.Right != "" {
		rightPos := width - rightLen
		l.terminal.MoveCursor(rightPos, l.terminal.savedCursor.Y)
		fmt.Print(l.header.Style.Apply(l.header.Right))
	}
}

func (l *Layout) drawFooter() {
	width, height := l.terminal.Size()
	footerY := height - l.footer.Height

	l.terminal.MoveCursor(0, footerY)

	if l.footer.Border {
		// Draw bordered footer
		frame := NewFrame(0, footerY, width, l.footer.Height).
			WithBorderStyle(l.footer.BorderStyle).
			WithColor(l.footer.Style)
		frame.Draw(l.terminal)

		// Draw content inside border
		l.terminal.MoveCursor(1, footerY + 1)
		if l.footer.StatusBar {
			l.drawStatusBar(width - 2)
		} else {
			l.drawFooterContent(width - 2)
		}
	} else {
		// Draw footer without border
		if l.footer.StatusBar {
			l.drawStatusBar(width)
		} else {
			l.drawFooterContent(width)
		}
	}
}

func (l *Layout) drawFooterContent(width int) {
	// Apply background if set
	if !l.footer.Background.IsEmpty() {
		bg := strings.Repeat(" ", width)
		fmt.Print(l.footer.Background.Apply(bg))
		l.terminal.MoveCursorLeft(width)
	}

	// Calculate sections
	leftLen := utf8.RuneCountInString(l.footer.Left)
	centerLen := utf8.RuneCountInString(l.footer.Center)
	rightLen := utf8.RuneCountInString(l.footer.Right)

	// Draw left section
	if l.footer.Left != "" {
		fmt.Print(l.footer.Style.Apply(l.footer.Left))
	}

	// Draw center section
	if l.footer.Center != "" {
		centerPos := (width - centerLen) / 2
		if centerPos > leftLen {
			l.terminal.MoveCursorRight(centerPos - leftLen)
			fmt.Print(l.footer.Style.Apply(l.footer.Center))
		}
	}

	// Draw right section
	if l.footer.Right != "" {
		rightPos := width - rightLen
		l.terminal.MoveCursor(rightPos, l.terminal.savedCursor.Y)
		fmt.Print(l.footer.Style.Apply(l.footer.Right))
	}
}

func (l *Layout) drawStatusBar(width int) {
	// Apply background
	bg := l.footer.Background
	if bg.IsEmpty() {
		bg = NewStyle().WithBackground(ColorBrightBlack)
	}

	// Clear the line with background
	bgLine := strings.Repeat(" ", width)
	fmt.Print(bg.Apply(bgLine))
	l.terminal.MoveCursorLeft(width)

	// Draw status items
	currentX := 0
	for i, item := range l.footer.StatusItems {
		if currentX >= width {
			break
		}

		// Add separator if not first item
		if i > 0 {
			sep := " │ "
			sepStyle := NewStyle().WithForeground(ColorBrightBlack)
			fmt.Print(bg.Apply(sepStyle.Apply(sep)))
			currentX += 3
		}

		// Draw icon if present
		if item.Icon != "" {
			fmt.Print(bg.Apply(item.Style.Apply(item.Icon + " ")))
			currentX += utf8.RuneCountInString(item.Icon) + 1
		}

		// Draw key
		if item.Key != "" {
			keyStyle := item.Style
			if keyStyle.IsEmpty() {
				keyStyle = NewStyle().WithBold()
			}
			fmt.Print(bg.Apply(keyStyle.Apply(item.Key + ": ")))
			currentX += utf8.RuneCountInString(item.Key) + 2
		}

		// Draw value
		valueStyle := NewStyle().WithForeground(ColorWhite)
		fmt.Print(bg.Apply(valueStyle.Apply(item.Value)))
		currentX += utf8.RuneCountInString(item.Value)
	}
}

// SimpleHeader creates a simple header with title
func SimpleHeader(title string, style Style) *Header {
	return &Header{
		Center: title,
		Style:  style,
		Height: 1,
	}
}

// BorderedHeader creates a bordered header
func BorderedHeader(title string, style Style) *Header {
	return &Header{
		Center:      title,
		Style:       style,
		Border:      true,
		BorderStyle: SingleBorder,
		Height:      3,
	}
}

// SimpleFooter creates a simple footer
func SimpleFooter(left, center, right string, style Style) *Footer {
	return &Footer{
		Left:   left,
		Center: center,
		Right:  right,
		Style:  style,
		Height: 1,
	}
}

// StatusBarFooter creates a status bar footer
func StatusBarFooter(items []StatusItem) *Footer {
	return &Footer{
		StatusBar:   true,
		StatusItems: items,
		Height:      1,
	}
}

// Screen represents a full-screen application
type Screen struct {
	terminal *Terminal
	layout   *Layout
	widgets  []Widget
	active   bool
}

// Widget represents a UI widget that can be rendered
type Widget interface {
	Draw(t *Terminal)
	HandleKey(event KeyEvent) bool
}

// NewScreen creates a new full-screen application
func NewScreen(terminal *Terminal) *Screen {
	return &Screen{
		terminal: terminal,
		layout:   NewLayout(terminal),
		widgets:  make([]Widget, 0),
	}
}

// SetLayout sets the screen layout
func (s *Screen) SetLayout(layout *Layout) *Screen {
	s.layout = layout
	return s
}

// AddWidget adds a widget to the screen
func (s *Screen) AddWidget(widget Widget) *Screen {
	s.widgets = append(s.widgets, widget)
	return s
}

// Run starts the screen application
func (s *Screen) Run() error {
	// Enter alternate screen
	s.terminal.EnableAlternateScreen()
	s.terminal.HideCursor()
	defer func() {
		s.terminal.ShowCursor()
		s.terminal.DisableAlternateScreen()
	}()

	// Enable raw mode for input
	if err := s.terminal.EnableRawMode(); err != nil {
		return err
	}
	defer s.terminal.DisableRawMode()

	// Draw initial layout
	s.layout.Draw()

	// Draw widgets
	for _, widget := range s.widgets {
		widget.Draw(s.terminal)
	}

	s.active = true

	// Main event loop
	input := NewInput(s.terminal)
	for s.active {
		event := input.readKeyEvent()

		// Check for quit keys
		if event.Key == KeyEscape || event.Key == KeyCtrlC {
			s.active = false
			break
		}

		// Pass event to widgets
		for _, widget := range s.widgets {
			if widget.HandleKey(event) {
				break
			}
		}

		// Refresh screen
		s.layout.Refresh()
		for _, widget := range s.widgets {
			widget.Draw(s.terminal)
		}
	}

	return nil
}

// Stop stops the screen application
func (s *Screen) Stop() {
	s.active = false
}