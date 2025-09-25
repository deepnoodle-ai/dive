package tui

import (
	"sync"
)

// AnimatedLayout extends Layout with animation capabilities
type AnimatedLayout struct {
	*Layout
	animator        *Animator
	animatedHeader  *AnimatedMultiLine
	animatedFooter  *AnimatedMultiLine
	animatedContent *AnimatedMultiLine
	mu              sync.RWMutex
	headerLines     int
	footerLines     int
	contentLines    int
}

// NewAnimatedLayout creates a new layout with animation support
func NewAnimatedLayout(terminal *Terminal, fps int) *AnimatedLayout {
	layout := NewLayout(terminal)
	animator := NewAnimator(terminal, fps)

	al := &AnimatedLayout{
		Layout:   layout,
		animator: animator,
	}

	return al
}

// SetAnimatedHeader configures an animated header area
func (al *AnimatedLayout) SetAnimatedHeader(lines int) *AnimatedLayout {
	al.mu.Lock()
	defer al.mu.Unlock()

	width, _ := al.terminal.Size()
	al.headerLines = lines
	al.animatedHeader = NewAnimatedMultiLine(0, 0, width)
	al.animator.AddElement(al.animatedHeader)

	// Update layout to account for animated header
	al.updateContentArea()
	return al
}

// SetAnimatedFooter configures an animated footer area
func (al *AnimatedLayout) SetAnimatedFooter(lines int) *AnimatedLayout {
	al.mu.Lock()
	defer al.mu.Unlock()

	width, height := al.terminal.Size()
	al.footerLines = lines
	al.animatedFooter = NewAnimatedMultiLine(0, height-lines, width)
	al.animator.AddElement(al.animatedFooter)

	// Update layout to account for animated footer
	al.updateContentArea()
	return al
}

// SetAnimatedContent configures an animated content area above input
func (al *AnimatedLayout) SetAnimatedContent(lines int) *AnimatedLayout {
	al.mu.Lock()
	defer al.mu.Unlock()

	width, _ := al.terminal.Size()
	al.contentLines = lines

	// Position content area after header but before input area
	contentY := al.headerLines
	al.animatedContent = NewAnimatedMultiLine(0, contentY, width)
	al.animator.AddElement(al.animatedContent)

	al.updateContentArea()
	return al
}

// SetHeaderLine sets content and animation for a header line
func (al *AnimatedLayout) SetHeaderLine(index int, text string, animation TextAnimation) {
	al.mu.RLock()
	defer al.mu.RUnlock()
	if al.animatedHeader != nil {
		al.animatedHeader.SetLine(index, text, animation)
	}
}

// SetFooterLine sets content and animation for a footer line
func (al *AnimatedLayout) SetFooterLine(index int, text string, animation TextAnimation) {
	al.mu.RLock()
	defer al.mu.RUnlock()
	if al.animatedFooter != nil {
		al.animatedFooter.SetLine(index, text, animation)
	}
}

// SetContentLine sets content and animation for a content line
func (al *AnimatedLayout) SetContentLine(index int, text string, animation TextAnimation) {
	al.mu.RLock()
	defer al.mu.RUnlock()
	if al.animatedContent != nil {
		al.animatedContent.SetLine(index, text, animation)
	}
}

// AddHeaderLine adds a new animated header line
func (al *AnimatedLayout) AddHeaderLine(text string, animation TextAnimation) {
	al.mu.RLock()
	defer al.mu.RUnlock()
	if al.animatedHeader != nil {
		al.animatedHeader.AddLine(text, animation)
	}
}

// AddFooterLine adds a new animated footer line
func (al *AnimatedLayout) AddFooterLine(text string, animation TextAnimation) {
	al.mu.RLock()
	defer al.mu.RUnlock()
	if al.animatedFooter != nil {
		al.animatedFooter.AddLine(text, animation)
	}
}

// AddContentLine adds a new animated content line
func (al *AnimatedLayout) AddContentLine(text string, animation TextAnimation) {
	al.mu.RLock()
	defer al.mu.RUnlock()
	if al.animatedContent != nil {
		al.animatedContent.AddLine(text, animation)
	}
}

// ClearHeaderLines removes all header lines
func (al *AnimatedLayout) ClearHeaderLines() {
	al.mu.RLock()
	defer al.mu.RUnlock()
	if al.animatedHeader != nil {
		al.animatedHeader.ClearLines()
	}
}

// ClearFooterLines removes all footer lines
func (al *AnimatedLayout) ClearFooterLines() {
	al.mu.RLock()
	defer al.mu.RUnlock()
	if al.animatedFooter != nil {
		al.animatedFooter.ClearLines()
	}
}

// ClearContentLines removes all content lines
func (al *AnimatedLayout) ClearContentLines() {
	al.mu.RLock()
	defer al.mu.RUnlock()
	if al.animatedContent != nil {
		al.animatedContent.ClearLines()
	}
}

// StartAnimations begins all animations
func (al *AnimatedLayout) StartAnimations() {
	al.animator.Start()
}

// StopAnimations stops all animations
func (al *AnimatedLayout) StopAnimations() {
	al.animator.Stop()
}

// GetInputArea returns the Y position and height available for input
func (al *AnimatedLayout) GetInputArea() (y, height int) {
	al.mu.RLock()
	defer al.mu.RUnlock()

	_, termHeight := al.terminal.Size()

	inputY := al.headerLines + al.contentLines
	inputHeight := termHeight - al.headerLines - al.contentLines - al.footerLines

	return inputY, inputHeight
}

func (al *AnimatedLayout) updateContentArea() {
	_, height := al.terminal.Size()

	// Calculate available space for input
	usedSpace := al.headerLines + al.contentLines + al.footerLines
	inputHeight := height - usedSpace

	// Update the underlying layout
	al.Layout.contentY = al.headerLines + al.contentLines
	al.Layout.contentHeight = inputHeight

	// Update footer position if it exists
	if al.animatedFooter != nil {
		footerY := height - al.footerLines
		al.animatedFooter.SetPosition(0, footerY)
	}
}

// AnimatedStatusBar represents an animated status bar
type AnimatedStatusBar struct {
	x, y         int
	width        int
	items        []*AnimatedStatusItem
	separator    string
	background   RGB
	mu           sync.RWMutex
	currentFrame uint64
}

// AnimatedStatusItem represents a single status item with animation
type AnimatedStatusItem struct {
	Key       string
	Value     string
	Icon      string
	Animation TextAnimation
	Style     Style
}

// NewAnimatedStatusBar creates a new animated status bar
func NewAnimatedStatusBar(x, y, width int) *AnimatedStatusBar {
	return &AnimatedStatusBar{
		x:          x,
		y:          y,
		width:      width,
		items:      make([]*AnimatedStatusItem, 0),
		separator:  " │ ",
		background: NewRGB(40, 40, 40),
	}
}

// AddItem adds a status item with optional animation
func (asb *AnimatedStatusBar) AddItem(key, value, icon string, animation TextAnimation, style Style) {
	asb.mu.Lock()
	defer asb.mu.Unlock()

	item := &AnimatedStatusItem{
		Key:       key,
		Value:     value,
		Icon:      icon,
		Animation: animation,
		Style:     style,
	}

	asb.items = append(asb.items, item)
}

// UpdateItem updates an existing status item
func (asb *AnimatedStatusBar) UpdateItem(index int, key, value string) {
	asb.mu.Lock()
	defer asb.mu.Unlock()

	if index >= 0 && index < len(asb.items) {
		asb.items[index].Key = key
		asb.items[index].Value = value
	}
}

// Update updates the animation frame
func (asb *AnimatedStatusBar) Update(frame uint64) {
	asb.mu.Lock()
	defer asb.mu.Unlock()
	asb.currentFrame = frame
}

// Draw renders the animated status bar
func (asb *AnimatedStatusBar) Draw(terminal *Terminal) {
	asb.mu.RLock()
	defer asb.mu.RUnlock()

	terminal.MoveCursor(asb.x, asb.y)

	// Clear the line with background
	bgLine := make([]byte, asb.width)
	for i := range bgLine {
		bgLine[i] = ' '
	}
	terminal.Print(asb.background.Apply(string(bgLine), true))
	terminal.MoveCursor(asb.x, asb.y)

	currentX := 0
	for i, item := range asb.items {
		if currentX >= asb.width {
			break
		}

		// Add separator if not first item
		if i > 0 {
			sepStyle := NewStyle().WithForeground(ColorBrightBlack)
			separatorText := asb.background.Apply(sepStyle.Apply(asb.separator), true)
			terminal.Print(separatorText)
			currentX += len(asb.separator)
		}

		// Draw icon if present
		if item.Icon != "" {
			iconText := item.Icon + " "
			if item.Animation != nil {
				// Apply animation to icon
				styledIcon := ""
				runes := []rune(iconText)
				for j, r := range runes {
					style := item.Animation.GetStyle(asb.currentFrame, j, len(runes))
					styledIcon += style.Apply(string(r))
				}
				terminal.Print(asb.background.Apply(styledIcon, true))
			} else {
				terminal.Print(asb.background.Apply(item.Style.Apply(iconText), true))
			}
			currentX += len(iconText)
		}

		// Draw key
		if item.Key != "" {
			keyText := item.Key + ": "
			keyStyle := item.Style
			if keyStyle.IsEmpty() {
				keyStyle = NewStyle().WithBold()
			}
			terminal.Print(asb.background.Apply(keyStyle.Apply(keyText), true))
			currentX += len(keyText)
		}

		// Draw value
		if item.Value != "" {
			if item.Animation != nil {
				// Apply animation to value
				styledValue := ""
				runes := []rune(item.Value)
				for j, r := range runes {
					switch anim := item.Animation.(type) {
					case *RainbowAnimation:
						colors := SmoothRainbow(anim.Length)
						offset := int(asb.currentFrame) / anim.Speed
						if anim.Reversed {
							offset = -offset
						}
						rainbowPos := (j + offset) % len(colors)
						if rainbowPos < 0 {
							rainbowPos += len(colors)
						}
						rgb := colors[rainbowPos]
						styledValue += rgb.Apply(string(r), false)
					default:
						style := item.Animation.GetStyle(asb.currentFrame, j, len(runes))
						styledValue += style.Apply(string(r))
					}
				}
				terminal.Print(asb.background.Apply(styledValue, true))
			} else {
				valueStyle := NewStyle().WithForeground(ColorWhite)
				terminal.Print(asb.background.Apply(valueStyle.Apply(item.Value), true))
			}
			currentX += len(item.Value)
		}
	}
}

// Position returns the status bar position
func (asb *AnimatedStatusBar) Position() (x, y int) {
	asb.mu.RLock()
	defer asb.mu.RUnlock()
	return asb.x, asb.y
}

// Dimensions returns the status bar dimensions
func (asb *AnimatedStatusBar) Dimensions() (width, height int) {
	asb.mu.RLock()
	defer asb.mu.RUnlock()
	return asb.width, 1
}

// SetPosition updates the status bar position
func (asb *AnimatedStatusBar) SetPosition(x, y int) {
	asb.mu.Lock()
	defer asb.mu.Unlock()
	asb.x = x
	asb.y = y
}

// CreateRainbowText creates a helper function for rainbow text animation
func CreateRainbowText(text string, speed int) TextAnimation {
	return &RainbowAnimation{
		Speed:    speed,
		Length:   len([]rune(text)),
		Reversed: false,
	}
}

// CreateReverseRainbowText creates a helper function for reverse rainbow text animation
func CreateReverseRainbowText(text string, speed int) TextAnimation {
	return &RainbowAnimation{
		Speed:    speed,
		Length:   len([]rune(text)),
		Reversed: true,
	}
}

// CreatePulseText creates a helper function for pulsing text animation
func CreatePulseText(color RGB, speed int) TextAnimation {
	return &PulseAnimation{
		Speed:         speed,
		Color:         color,
		MinBrightness: 0.3,
		MaxBrightness: 1.0,
	}
}

// AnimatedInputLayout combines animated layout with input handling
type AnimatedInputLayout struct {
	*AnimatedLayout
	inputY      int
	inputHeight int
	inputPrompt string
	promptStyle Style
}

// NewAnimatedInputLayout creates a layout optimized for input with animations
func NewAnimatedInputLayout(terminal *Terminal, fps int) *AnimatedInputLayout {
	al := NewAnimatedLayout(terminal, fps)
	return &AnimatedInputLayout{
		AnimatedLayout: al,
		promptStyle:    NewStyle().WithForeground(ColorCyan).WithBold(),
	}
}

// SetPrompt sets the input prompt
func (ail *AnimatedInputLayout) SetPrompt(prompt string, style Style) {
	ail.inputPrompt = prompt
	ail.promptStyle = style
}

// GetInputPosition returns where input should be positioned
func (ail *AnimatedInputLayout) GetInputPosition() (x, y int) {
	promptLen := len([]rune(ail.inputPrompt))
	inputY, _ := ail.GetInputArea()
	return promptLen, inputY
}

// DrawPrompt draws the input prompt
func (ail *AnimatedInputLayout) DrawPrompt() {
	_, inputY := ail.GetInputArea()
	ail.terminal.MoveCursor(0, inputY)
	ail.terminal.ClearLine()
	ail.terminal.Print(ail.promptStyle.Apply(ail.inputPrompt))
}

// GetAnimator returns the underlying animator for adding custom elements
func (ail *AnimatedInputLayout) GetAnimator() *Animator {
	return ail.AnimatedLayout.animator
}
