package tui

import (
	"fmt"
)

// MouseEvent represents a mouse interaction
type MouseEvent struct {
	X, Y   int
	Button MouseButton
	Type   MouseEventType
}

// MouseButton represents which mouse button was pressed
type MouseButton int

const (
	MouseLeft MouseButton = iota
	MouseMiddle
	MouseRight
	MouseRelease
	MouseWheelUp
	MouseWheelDown
)

// MouseEventType represents the type of mouse event
type MouseEventType int

const (
	MouseClick MouseEventType = iota
	MouseDoubleClick
	MouseDrag
	MouseMove
	MouseScroll
)

// EnableMouseTracking enables mouse event reporting in the terminal
func (t *Terminal) EnableMouseTracking() {
	// Enable SGR extended mouse mode (supports coordinates beyond 223)
	fmt.Print("\033[?1006h")
	// Enable mouse tracking - report button press and release
	fmt.Print("\033[?1000h")
	// Enable mouse motion tracking
	fmt.Print("\033[?1002h")
	// Alternative: use 1003 for all motion events
	// fmt.Print("\033[?1003h")
}

// DisableMouseTracking disables mouse event reporting
func (t *Terminal) DisableMouseTracking() {
	fmt.Print("\033[?1000l")
	fmt.Print("\033[?1002l")
	fmt.Print("\033[?1006l")
}

// ParseMouseEvent parses a mouse event from terminal input
func ParseMouseEvent(seq []byte) (*MouseEvent, error) {
	// SGR format: \033[<button>;x;yM (press) or m (release)
	// Example: \033[<0;10;5M for left click at (10,5)

	if len(seq) < 6 {
		return nil, fmt.Errorf("invalid mouse sequence")
	}

	// Parse the sequence
	var button, x, y int
	var action byte

	// Look for SGR format
	if seq[0] == '<' {
		// Parse SGR format: <button>;x;y[Mm]
		end := 1
		for end < len(seq) && seq[end] != 'M' && seq[end] != 'm' {
			end++
		}
		if end >= len(seq) {
			return nil, fmt.Errorf("incomplete mouse sequence")
		}

		action = seq[end]
		coords := string(seq[1:end])

		if _, err := fmt.Sscanf(coords, "%d;%d;%d", &button, &x, &y); err != nil {
			return nil, err
		}
	} else {
		// Legacy format - limited to coordinates < 223
		if len(seq) != 3 {
			return nil, fmt.Errorf("invalid legacy mouse sequence")
		}
		button = int(seq[0]) - 32
		x = int(seq[1]) - 32
		y = int(seq[2]) - 32
		action = 'M' // Legacy is always press
	}

	// Convert to 0-based coordinates
	x--
	y--

	event := &MouseEvent{
		X: x,
		Y: y,
	}

	// Determine button and type
	if action == 'm' {
		event.Button = MouseRelease
		event.Type = MouseClick
	} else {
		switch button & 3 {
		case 0:
			event.Button = MouseLeft
		case 1:
			event.Button = MouseMiddle
		case 2:
			event.Button = MouseRight
		case 3:
			event.Button = MouseRelease
		}

		// Check for wheel events
		if button&64 != 0 {
			if button&1 != 0 {
				event.Button = MouseWheelDown
			} else {
				event.Button = MouseWheelUp
			}
			event.Type = MouseScroll
		} else if button&32 != 0 {
			event.Type = MouseDrag
		} else {
			event.Type = MouseClick
		}
	}

	return event, nil
}

// MouseRegion represents a clickable area on screen
type MouseRegion struct {
	X, Y          int
	Width, Height int
	Handler       func(event *MouseEvent)
	HoverHandler  func(hovering bool)
	Label         string
}

// Contains checks if a point is within the region
func (r *MouseRegion) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.Width &&
		y >= r.Y && y < r.Y+r.Height
}

// MouseHandler manages mouse regions and events
type MouseHandler struct {
	regions      []*MouseRegion
	activeRegion *MouseRegion
}

// NewMouseHandler creates a new mouse handler
func NewMouseHandler() *MouseHandler {
	return &MouseHandler{
		regions: make([]*MouseRegion, 0),
	}
}

// AddRegion adds a clickable region
func (h *MouseHandler) AddRegion(region *MouseRegion) {
	h.regions = append(h.regions, region)
}

// ClearRegions removes all regions
func (h *MouseHandler) ClearRegions() {
	h.regions = h.regions[:0]
	h.activeRegion = nil
}

// HandleEvent processes a mouse event
func (h *MouseHandler) HandleEvent(event *MouseEvent) {
	// Find which region was clicked
	for _, region := range h.regions {
		if region.Contains(event.X, event.Y) {
			// Handle hover state changes
			if h.activeRegion != region {
				if h.activeRegion != nil && h.activeRegion.HoverHandler != nil {
					h.activeRegion.HoverHandler(false)
				}
				h.activeRegion = region
				if region.HoverHandler != nil {
					region.HoverHandler(true)
				}
			}

			// Handle click
			if event.Type == MouseClick && region.Handler != nil {
				region.Handler(event)
			}
			return
		}
	}

	// No region found - clear active
	if h.activeRegion != nil && h.activeRegion.HoverHandler != nil {
		h.activeRegion.HoverHandler(false)
	}
	h.activeRegion = nil
}
