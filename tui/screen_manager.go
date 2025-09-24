package tui

import (
	"sync"
	"time"
)

// ScreenRegion represents a rectangular area of the screen
type ScreenRegion struct {
	X, Y          int
	Width, Height int
	Content       []string
	Animations    []TextAnimation
	Protected     bool // If true, this region cannot be overwritten by animations
}

// ScreenManager coordinates all screen updates to prevent race conditions
type ScreenManager struct {
	terminal       *Terminal
	regions        map[string]*ScreenRegion
	mu             sync.RWMutex
	drawMutex      sync.Mutex // Ensures only one draw operation at a time
	cursorX, cursorY int       // Where the cursor should be positioned
	cursorVisible  bool
	running        bool
	updateChan     chan struct{}
	frameCounter   uint64
	fps            int
}

// NewScreenManager creates a new screen manager
func NewScreenManager(terminal *Terminal, fps int) *ScreenManager {
	return &ScreenManager{
		terminal:      terminal,
		regions:       make(map[string]*ScreenRegion),
		updateChan:    make(chan struct{}, 100), // Buffered to avoid blocking
		cursorVisible: true,
		fps:           fps,
	}
}

// DefineRegion creates or updates a screen region
func (sm *ScreenManager) DefineRegion(name string, x, y, width, height int, protected bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.regions[name] = &ScreenRegion{
		X:         x,
		Y:         y,
		Width:     width,
		Height:    height,
		Protected: protected,
		Content:   make([]string, height),
	}
}

// UpdateRegion updates the content of a region
func (sm *ScreenManager) UpdateRegion(name string, lineIndex int, content string, animation TextAnimation) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if region, exists := sm.regions[name]; exists {
		if lineIndex >= 0 && lineIndex < len(region.Content) {
			region.Content[lineIndex] = content

			// Ensure animations slice is large enough
			for len(region.Animations) <= lineIndex {
				region.Animations = append(region.Animations, nil)
			}
			region.Animations[lineIndex] = animation
		}
	}

	// Trigger a redraw
	select {
	case sm.updateChan <- struct{}{}:
	default:
		// Channel is full, skip this update signal
	}
}

// SetCursorPosition sets where the cursor should be positioned
func (sm *ScreenManager) SetCursorPosition(x, y int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cursorX = x
	sm.cursorY = y
}

// Start begins the managed drawing loop
func (sm *ScreenManager) Start() {
	sm.mu.Lock()
	if sm.running {
		sm.mu.Unlock()
		return
	}
	sm.running = true
	sm.mu.Unlock()

	go sm.drawLoop()
}

// Stop stops the drawing loop
func (sm *ScreenManager) Stop() {
	sm.mu.Lock()
	sm.running = false
	sm.mu.Unlock()
}

// drawLoop is the single drawing loop that coordinates all screen updates
func (sm *ScreenManager) drawLoop() {
	ticker := time.NewTicker(time.Second / time.Duration(sm.fps))
	defer ticker.Stop()

	lastDraw := time.Now()
	minDrawInterval := time.Millisecond * 50 // Don't redraw more than 20 times per second

	for sm.running {
		select {
		case <-ticker.C:
			sm.draw()
			lastDraw = time.Now()
		case <-sm.updateChan:
			// Batch rapid updates - only draw if enough time has passed
			if time.Since(lastDraw) > minDrawInterval {
				sm.draw()
				lastDraw = time.Now()
			}
			// Drain any additional updates in the channel
			for len(sm.updateChan) > 0 {
				<-sm.updateChan
			}
		}
	}
}

// draw performs a single coordinated draw of all regions
func (sm *ScreenManager) draw() {
	// Lock drawing to prevent concurrent draws
	sm.drawMutex.Lock()
	defer sm.drawMutex.Unlock()

	// Get current frame counter
	sm.frameCounter++
	frame := sm.frameCounter

	// Draw all regions
	sm.mu.RLock()
	regions := make(map[string]*ScreenRegion)
	for name, region := range sm.regions {
		regions[name] = region
	}
	cursorX, cursorY := sm.cursorX, sm.cursorY
	sm.mu.RUnlock()

	// Draw each region (without hiding cursor - less flicker)
	for _, region := range regions {
		sm.drawRegion(region, frame)
	}

	// Just position cursor - don't hide/show
	sm.terminal.MoveCursor(cursorX, cursorY)
	sm.terminal.Flush()
}

// drawRegion draws a single region
func (sm *ScreenManager) drawRegion(region *ScreenRegion, frame uint64) {
	for i, content := range region.Content {
		if content == "" {
			continue
		}

		// Move to the line position
		sm.terminal.MoveCursor(region.X, region.Y + i)

		// Clear to end of line from this position
		sm.terminal.ClearToEndOfLine()

		// Draw the content
		if i < len(region.Animations) && region.Animations[i] != nil {
			// Draw with animation
			sm.drawAnimatedText(content, region.Animations[i], frame)
		} else {
			// Draw static text
			sm.terminal.Print(content)
		}
	}
}

// drawAnimatedText draws text with animation
func (sm *ScreenManager) drawAnimatedText(text string, animation TextAnimation, frame uint64) {
	runes := []rune(text)
	totalChars := len(runes)

	for i, r := range runes {
		switch anim := animation.(type) {
		case *RainbowAnimation:
			colors := SmoothRainbow(anim.Length)
			offset := int(frame) / anim.Speed
			if anim.Reversed {
				offset = -offset
			}
			rainbowPos := (i + offset) % len(colors)
			if rainbowPos < 0 {
				rainbowPos += len(colors)
			}
			rgb := colors[rainbowPos]
			sm.terminal.Print(rgb.Apply(string(r), false))

		case *PulseAnimation:
			// Calculate pulse brightness
			pulseTime := float64(frame) / float64(anim.Speed)
			brightness := anim.MinBrightness +
				(anim.MaxBrightness-anim.MinBrightness) *
				(0.5 + 0.5 * Sine(pulseTime))

			// Apply brightness to color
			adjustedColor := RGB{
				R: uint8(float64(anim.Color.R) * brightness),
				G: uint8(float64(anim.Color.G) * brightness),
				B: uint8(float64(anim.Color.B) * brightness),
			}
			sm.terminal.Print(adjustedColor.Apply(string(r), false))

		default:
			style := animation.GetStyle(frame, i, totalChars)
			sm.terminal.Print(style.Apply(string(r)))
		}
	}
}

// Sine helper for pulse calculations
func Sine(x float64) float64 {
	// Simple sine approximation
	x = x - float64(int(x/6.28318)) * 6.28318 // Normalize to 0-2π
	if x < 3.14159 {
		return 4*x*(3.14159-x) / (3.14159*3.14159)
	}
	x = x - 3.14159
	return -4*x*(3.14159-x) / (3.14159*3.14159)
}