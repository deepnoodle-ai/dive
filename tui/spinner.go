package tui

import (
	"fmt"
	"sync"
	"time"
)

// Spinner represents an animated loading spinner
type Spinner struct {
	frames   []string
	interval time.Duration
	style    Style
	message  string
	mu       sync.Mutex
	active   bool
	done     chan struct{}
	terminal *Terminal
}

// SpinnerStyle defines different spinner animations
type SpinnerStyle struct {
	Frames   []string
	Interval time.Duration
}

// Predefined spinner styles
var (
	SpinnerDots = SpinnerStyle{
		Frames:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		Interval: 80 * time.Millisecond,
	}

	SpinnerLine = SpinnerStyle{
		Frames:   []string{"-", "\\", "|", "/"},
		Interval: 100 * time.Millisecond,
	}

	SpinnerArrows = SpinnerStyle{
		Frames:   []string{"←", "↖", "↑", "↗", "→", "↘", "↓", "↙"},
		Interval: 100 * time.Millisecond,
	}

	SpinnerCircle = SpinnerStyle{
		Frames:   []string{"◐", "◓", "◑", "◒"},
		Interval: 120 * time.Millisecond,
	}

	SpinnerSquare = SpinnerStyle{
		Frames:   []string{"◰", "◳", "◲", "◱"},
		Interval: 100 * time.Millisecond,
	}

	SpinnerBounce = SpinnerStyle{
		Frames:   []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
		Interval: 80 * time.Millisecond,
	}

	SpinnerBar = SpinnerStyle{
		Frames: []string{
			"[    ]",
			"[=   ]",
			"[==  ]",
			"[=== ]",
			"[====]",
			"[ ===]",
			"[  ==]",
			"[   =]",
		},
		Interval: 80 * time.Millisecond,
	}

	SpinnerStars = SpinnerStyle{
		Frames: []string{
			"✶",
			"✸",
			"✹",
			"✺",
			"✹",
			"✷",
		},
		Interval: 120 * time.Millisecond,
	}

	SpinnerStarField = SpinnerStyle{
		Frames: []string{
			"·   ",
			"*·  ",
			"✦*· ",
			"✧✦*·",
			"·✧✦*",
			" ·✧✦",
			"  ·✧",
			"   ·",
		},
		Interval: 100 * time.Millisecond,
	}

	SpinnerAsterisk = SpinnerStyle{
		Frames: []string{
			"*",
			"⁎",
			"✱",
			"✲",
			"✳",
			"✴",
			"✵",
			"✶",
			"✷",
			"✸",
			"✹",
			"✺",
		},
		Interval: 100 * time.Millisecond,
	}

	SpinnerSparkle = SpinnerStyle{
		Frames: []string{
			"✨",
			"💫",
			"⭐",
			"🌟",
			"✨",
			"💫",
		},
		Interval: 150 * time.Millisecond,
	}
)

// NewSpinner creates a new spinner
func NewSpinner(terminal *Terminal, style SpinnerStyle) *Spinner {
	return &Spinner{
		frames:   style.Frames,
		interval: style.Interval,
		style:    NewStyle(),
		terminal: terminal,
		done:     make(chan struct{}),
	}
}

// WithStyle sets the spinner style
func (s *Spinner) WithStyle(style Style) *Spinner {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.style = style
	return s
}

// WithMessage sets the spinner message
func (s *Spinner) WithMessage(message string) *Spinner {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
	return s
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.mu.Unlock()

	go s.animate()
}

// Stop stops the spinner animation
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	close(s.done)
	s.mu.Unlock()

	// Clear the spinner line
	s.terminal.ClearLine()
}

// Success stops the spinner with a success message
func (s *Spinner) Success(message string) {
	s.Stop()
	s.terminal.MoveCursorLeft(1000) // Move to start of line
	s.terminal.ClearLine()
	successStyle := NewStyle().WithForeground(ColorGreen)
	fmt.Println(successStyle.Apply("✓ " + message))
}

// Error stops the spinner with an error message
func (s *Spinner) Error(message string) {
	s.Stop()
	s.terminal.MoveCursorLeft(1000) // Move to start of line
	s.terminal.ClearLine()
	errorStyle := NewStyle().WithForeground(ColorRed)
	fmt.Println(errorStyle.Apply("✗ " + message))
}

func (s *Spinner) animate() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			message := s.message
			style := s.style
			s.mu.Unlock()

			s.terminal.MoveCursorLeft(1000) // Move to start of line
			s.terminal.ClearLine()

			output := style.Apply(s.frames[frame])
			if message != "" {
				output += " " + message
			}
			fmt.Print(output)

			frame = (frame + 1) % len(s.frames)
		}
	}
}

// ProgressBar represents a progress bar
type ProgressBar struct {
	total       int
	current     int
	width       int
	style       Style
	fillChar    string
	emptyChar   string
	showPercent bool
	showNumbers bool
	message     string
	terminal    *Terminal
	mu          sync.Mutex
}

// NewProgressBar creates a new progress bar
func NewProgressBar(terminal *Terminal, total int) *ProgressBar {
	return &ProgressBar{
		total:       total,
		width:       40,
		style:       NewStyle().WithForeground(ColorCyan),
		fillChar:    "█",
		emptyChar:   "░",
		showPercent: true,
		showNumbers: false,
		terminal:    terminal,
	}
}

// WithWidth sets the progress bar width
func (p *ProgressBar) WithWidth(width int) *ProgressBar {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.width = width
	return p
}

// WithStyle sets the progress bar style
func (p *ProgressBar) WithStyle(style Style) *ProgressBar {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.style = style
	return p
}

// WithChars sets the fill and empty characters
func (p *ProgressBar) WithChars(fill, empty string) *ProgressBar {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fillChar = fill
	p.emptyChar = empty
	return p
}

// ShowNumbers enables showing current/total numbers
func (p *ProgressBar) ShowNumbers() *ProgressBar {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.showNumbers = true
	return p
}

// Update updates the progress bar
func (p *ProgressBar) Update(current int, message string) {
	p.mu.Lock()
	p.current = current
	if current > p.total {
		p.current = p.total
	}
	p.message = message
	p.mu.Unlock()

	p.draw()
}

// Increment increments the progress by 1
func (p *ProgressBar) Increment(message string) {
	p.mu.Lock()
	p.current++
	if p.current > p.total {
		p.current = p.total
	}
	p.message = message
	p.mu.Unlock()

	p.draw()
}

// Complete marks the progress as complete
func (p *ProgressBar) Complete(message string) {
	p.Update(p.total, message)
	fmt.Println() // Move to next line
}

func (p *ProgressBar) draw() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.terminal.MoveCursorLeft(1000) // Move to start of line
	p.terminal.ClearLine()

	// Calculate fill
	percent := float64(p.current) / float64(p.total)
	filled := int(percent * float64(p.width))

	// Build bar
	bar := "["
	for i := 0; i < p.width; i++ {
		if i < filled {
			bar += p.style.Apply(p.fillChar)
		} else {
			bar += p.emptyChar
		}
	}
	bar += "]"

	// Add percentage
	output := bar
	if p.showPercent {
		output += fmt.Sprintf(" %3.0f%%", percent*100)
	}

	// Add numbers
	if p.showNumbers {
		output += fmt.Sprintf(" (%d/%d)", p.current, p.total)
	}

	// Add message
	if p.message != "" {
		output += " " + p.message
	}

	fmt.Print(output)
}

// MultiProgress manages multiple progress items
type MultiProgress struct {
	items    []*ProgressItem
	terminal *Terminal
	mu       sync.Mutex
	startY   int
}

// ProgressItem represents a single progress item in MultiProgress
type ProgressItem struct {
	ID          string
	Message     string
	Current     int
	Total       int
	Style       Style
	SpinnerOnly bool
	spinner     *Spinner
	bar         *ProgressBar
}

// NewMultiProgress creates a new multi-progress manager
func NewMultiProgress(terminal *Terminal) *MultiProgress {
	return &MultiProgress{
		terminal: terminal,
		items:    make([]*ProgressItem, 0),
	}
}

// Add adds a new progress item
func (m *MultiProgress) Add(id string, total int, spinnerOnly bool) *ProgressItem {
	m.mu.Lock()
	defer m.mu.Unlock()

	item := &ProgressItem{
		ID:          id,
		Total:       total,
		Style:       NewStyle(),
		SpinnerOnly: spinnerOnly,
	}

	if spinnerOnly {
		item.spinner = NewSpinner(m.terminal, SpinnerDots)
	} else {
		item.bar = NewProgressBar(m.terminal, total)
	}

	m.items = append(m.items, item)
	return item
}

// Update updates a progress item
func (m *MultiProgress) Update(id string, current int, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, item := range m.items {
		if item.ID == id {
			item.Current = current
			item.Message = message
			m.drawItem(i, item)
			return
		}
	}
}

// Start begins rendering all progress items
func (m *MultiProgress) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Save current position
	m.terminal.SaveCursor()
	_, y := m.terminal.Size()
	m.startY = y

	// Reserve space for all items
	for range m.items {
		fmt.Println()
	}
}

// Stop stops all progress items
func (m *MultiProgress) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop all spinners
	for _, item := range m.items {
		if item.spinner != nil {
			item.spinner.Stop()
		}
	}
}

func (m *MultiProgress) drawItem(index int, item *ProgressItem) {
	// Move to the item's line
	m.terminal.MoveCursor(0, m.startY+index)
	m.terminal.ClearLine()

	if item.SpinnerOnly {
		// Draw spinner
		if item.spinner != nil {
			item.spinner.WithMessage(item.Message)
		}
	} else {
		// Draw progress bar
		if item.bar != nil {
			item.bar.Update(item.Current, item.Message)
		}
	}
}
