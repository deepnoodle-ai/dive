package tui

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"golang.org/x/term"
)

// Terminal represents a terminal interface with advanced capabilities
type Terminal struct {
	mu          sync.Mutex
	width       int
	height      int
	savedCursor Position
	altScreen   bool
	rawMode     bool
	oldState    *term.State
	fd          int
}

// Position represents a cursor position
type Position struct {
	X int
	Y int
}

// NewTerminal creates a new terminal instance
func NewTerminal() (*Terminal, error) {
	fd := int(os.Stdout.Fd())
	width, height, err := term.GetSize(fd)
	if err != nil {
		return nil, fmt.Errorf("failed to get terminal size: %w", err)
	}

	return &Terminal{
		fd:     fd,
		width:  width,
		height: height,
	}, nil
}

// Size returns the terminal dimensions
func (t *Terminal) Size() (width, height int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.width, t.height
}

// RefreshSize updates the cached terminal size
func (t *Terminal) RefreshSize() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	width, height, err := term.GetSize(t.fd)
	if err != nil {
		return err
	}
	t.width = width
	t.height = height
	return nil
}

// Clear clears the entire screen
func (t *Terminal) Clear() {
	fmt.Print("\033[2J")
}

// ClearLine clears the current line
func (t *Terminal) ClearLine() {
	fmt.Print("\033[2K")
}

// ClearToEndOfLine clears from cursor to end of line
func (t *Terminal) ClearToEndOfLine() {
	fmt.Print("\033[K")
}

// MoveCursor moves the cursor to the specified position
func (t *Terminal) MoveCursor(x, y int) {
	fmt.Printf("\033[%d;%dH", y+1, x+1)
}

// MoveCursorUp moves the cursor up by n lines
func (t *Terminal) MoveCursorUp(n int) {
	if n > 0 {
		fmt.Printf("\033[%dA", n)
	}
}

// MoveCursorDown moves the cursor down by n lines
func (t *Terminal) MoveCursorDown(n int) {
	if n > 0 {
		fmt.Printf("\033[%dB", n)
	}
}

// MoveCursorRight moves the cursor right by n columns
func (t *Terminal) MoveCursorRight(n int) {
	if n > 0 {
		fmt.Printf("\033[%dC", n)
	}
}

// MoveCursorLeft moves the cursor left by n columns
func (t *Terminal) MoveCursorLeft(n int) {
	if n > 0 {
		fmt.Printf("\033[%dD", n)
	}
}

// SaveCursor saves the current cursor position
func (t *Terminal) SaveCursor() {
	fmt.Print("\033[s")
}

// RestoreCursor restores the saved cursor position
func (t *Terminal) RestoreCursor() {
	fmt.Print("\033[u")
}

// HideCursor hides the cursor
func (t *Terminal) HideCursor() {
	fmt.Print("\033[?25l")
}

// ShowCursor shows the cursor
func (t *Terminal) ShowCursor() {
	fmt.Print("\033[?25h")
}

// EnableAlternateScreen switches to the alternate screen buffer
func (t *Terminal) EnableAlternateScreen() {
	if !t.altScreen {
		fmt.Print("\033[?1049h")
		t.altScreen = true
	}
}

// DisableAlternateScreen switches back to the main screen buffer
func (t *Terminal) DisableAlternateScreen() {
	if t.altScreen {
		fmt.Print("\033[?1049l")
		t.altScreen = false
	}
}

// EnableRawMode enables raw terminal mode
func (t *Terminal) EnableRawMode() error {
	if t.rawMode {
		return nil
	}

	oldState, err := term.MakeRaw(t.fd)
	if err != nil {
		return fmt.Errorf("failed to enable raw mode: %w", err)
	}

	t.oldState = oldState
	t.rawMode = true
	return nil
}

// DisableRawMode disables raw terminal mode
func (t *Terminal) DisableRawMode() error {
	if !t.rawMode {
		return nil
	}

	if t.oldState != nil {
		if err := term.Restore(t.fd, t.oldState); err != nil {
			return fmt.Errorf("failed to restore terminal: %w", err)
		}
		t.oldState = nil
	}

	t.rawMode = false
	return nil
}

// Print outputs text at the current cursor position
func (t *Terminal) Print(text string) {
	fmt.Print(text)
}

// Println outputs text with a newline
func (t *Terminal) Println(text string) {
	fmt.Println(text)
}

// PrintAt prints text at a specific position
func (t *Terminal) PrintAt(x, y int, text string) {
	t.MoveCursor(x, y)
	fmt.Print(text)
}

// Fill fills a rectangular area with a character
func (t *Terminal) Fill(x, y, width, height int, char rune) {
	line := strings.Repeat(string(char), width)
	for i := 0; i < height; i++ {
		t.PrintAt(x, y+i, line)
	}
}

// Reset resets all terminal attributes
func (t *Terminal) Reset() {
	fmt.Print("\033[0m")
}

// Flush ensures all output is written
func (t *Terminal) Flush() {
	os.Stdout.Sync()
}

// Close cleans up terminal state
func (t *Terminal) Close() error {
	t.ShowCursor()
	t.DisableAlternateScreen()
	t.DisableRawMode()
	t.Reset()
	t.Flush()
	return nil
}
