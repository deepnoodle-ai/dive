package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/tui"
	"golang.org/x/term"
)

type InteractiveDemo struct {
	terminal      *tui.Terminal
	screenManager *tui.ScreenManager
	mouseHandler  *tui.MouseHandler

	// UI Components
	buttons       []*tui.Button
	tabCompleter  *tui.TabCompleter
	radioGroup    *tui.RadioGroup

	// State
	currentInput  string
	cursorPos     int
	commandHistory []string
	notifications []string
	clickedWords  map[string]tui.RGB
	selectedTheme string

	running bool
	mu      sync.Mutex
}

func NewInteractiveDemo() (*InteractiveDemo, error) {
	terminal, err := tui.NewTerminal()
	if err != nil {
		return nil, err
	}

	return &InteractiveDemo{
		terminal:      terminal,
		screenManager: tui.NewScreenManager(terminal, 15),
		mouseHandler:  tui.NewMouseHandler(),
		tabCompleter:  tui.NewTabCompleter(),
		clickedWords:  make(map[string]tui.RGB),
		commandHistory: make([]string, 0),
		notifications: make([]string, 0),
		running:       true,
		selectedTheme: "Default",
	}, nil
}

func (d *InteractiveDemo) Setup() {
	// Enable alternate screen and mouse tracking
	d.terminal.EnableAlternateScreen()
	d.terminal.EnableMouseTracking()
	d.terminal.ShowCursor()

	width, height := d.terminal.Size()

	// Define screen regions
	d.screenManager.DefineRegion("title", 0, 0, width, 2, false)
	d.screenManager.DefineRegion("clickable", 0, 2, width, 4, false)
	d.screenManager.DefineRegion("buttons", 0, 6, width, 3, false)
	d.screenManager.DefineRegion("radio", 0, 9, width, 4, false)
	d.screenManager.DefineRegion("input", 0, 13, width, 2, true)
	d.screenManager.DefineRegion("completions", 0, 15, width, 7, false)
	d.screenManager.DefineRegion("notifications", 0, 22, width, 3, false)

	// Footer at bottom
	footerY := height - 2
	if footerY > 25 {
		footerY = 25
	}
	d.screenManager.DefineRegion("footer", 0, footerY, width, 2, false)

	// Create UI components
	d.createButtons()
	d.createRadioGroup()
	d.setupTabCompleter()

	// Initialize display
	d.initializeDisplay()

	// Start screen manager
	d.screenManager.Start()
}

func (d *InteractiveDemo) createButtons() {
	// Create interactive buttons
	d.buttons = []*tui.Button{
		tui.NewButton(5, 7, "Run Command", func() {
			d.addNotification("▶ Running command: " + d.currentInput)
			d.commandHistory = append(d.commandHistory, d.currentInput)
			d.currentInput = ""
			d.cursorPos = 0
			d.updateInput()
		}),
		tui.NewButton(22, 7, "Clear Input", func() {
			d.currentInput = ""
			d.cursorPos = 0
			d.tabCompleter.Hide()
			d.updateInput()
			d.addNotification("✨ Input cleared")
		}),
		tui.NewButton(39, 7, "Show Help", func() {
			d.addNotification("ℹ️  Use TAB for completions, click words to color them!")
		}),
	}

	// Add button regions to mouse handler
	for _, btn := range d.buttons {
		d.mouseHandler.AddRegion(btn.GetRegion())
	}
}

func (d *InteractiveDemo) createRadioGroup() {
	themes := []string{"Default", "Dark Mode", "High Contrast", "Solarized"}
	d.radioGroup = tui.NewRadioGroup(5, 10, themes)
	d.radioGroup.OnChange = func(index int, theme string) {
		d.selectedTheme = theme
		d.addNotification(fmt.Sprintf("🎨 Theme changed to: %s", theme))
		d.updateColors()
	}

	// Add radio button regions
	for _, region := range d.radioGroup.GetRegions() {
		d.mouseHandler.AddRegion(region)
	}
}

func (d *InteractiveDemo) setupTabCompleter() {
	// Commands for tab completion
	commands := []string{
		"build", "test", "run", "install", "clean",
		"deploy", "debug", "profile", "benchmark",
		"commit", "push", "pull", "merge", "rebase",
		"status", "diff", "log", "branch", "checkout",
	}

	d.tabCompleter.OnSelect = func(suggestion string) {
		d.currentInput = suggestion + " "
		d.cursorPos = len(d.currentInput)
		d.tabCompleter.Hide()
		d.updateInput()
	}

	// Store for later use
	d.tabCompleter.SetSuggestions(commands, "")
}

func (d *InteractiveDemo) initializeDisplay() {
	// Title
	d.screenManager.UpdateRegion("title", 0,
		"🖱️  Interactive TUI Demo - Mouse, Tab Completion & Buttons",
		tui.CreateRainbowText("🖱️  Interactive TUI Demo - Mouse, Tab Completion & Buttons", 12))
	d.screenManager.UpdateRegion("title", 1,
		"═══════════════════════════════════════════════════════════",
		nil)

	// Clickable text area
	d.updateClickableText()

	// Buttons
	d.updateButtons()

	// Radio buttons
	d.updateRadioButtons()

	// Input area
	d.updateInput()

	// Footer
	d.screenManager.UpdateRegion("footer", 0,
		"TAB: Show completions | Click words to color | Click buttons to interact",
		nil)
	d.screenManager.UpdateRegion("footer", 1,
		"↑↓: Navigate completions | Enter: Accept | ESC: Cancel | Ctrl+C: Exit",
		nil)
}

func (d *InteractiveDemo) updateClickableText() {
	// Create clickable words
	words := []string{"Click", "these", "words", "to", "change", "their", "colors!"}

	d.screenManager.UpdateRegion("clickable", 0,
		"📝 Clickable Text Area:", nil)

	// Build the text with colored words
	var textLine strings.Builder
	textLine.WriteString("   ")

	for i, word := range words {
		// Check if word has been clicked and has a color
		if color, exists := d.clickedWords[word]; exists {
			textLine.WriteString(color.Apply(word, false))
		} else {
			textLine.WriteString(word)
		}

		if i < len(words)-1 {
			textLine.WriteString(" ")
		}
	}

	d.screenManager.UpdateRegion("clickable", 1, textLine.String(), nil)

	// Add mouse regions for each word
	x := 3 // Starting x position
	y := 3 // Line position

	for _, word := range words {
		wordCopy := word // Capture for closure
		region := &tui.MouseRegion{
			X:      x,
			Y:      y,
			Width:  len(word),
			Height: 1,
			Label:  word,
			Handler: func(event *tui.MouseEvent) {
				// Cycle through colors on click
				colors := []tui.RGB{
					tui.NewRGB(255, 100, 100),
					tui.NewRGB(100, 255, 100),
					tui.NewRGB(100, 100, 255),
					tui.NewRGB(255, 255, 100),
					tui.NewRGB(255, 100, 255),
					tui.NewRGB(100, 255, 255),
				}

				// Get next color
				currentIdx := 0
				if existingColor, exists := d.clickedWords[wordCopy]; exists {
					for i, c := range colors {
						if c == existingColor {
							currentIdx = (i + 1) % len(colors)
							break
						}
					}
				}

				d.clickedWords[wordCopy] = colors[currentIdx]
				d.updateClickableText()
				d.addNotification(fmt.Sprintf("🎨 Colored '%s'", wordCopy))
			},
		}
		d.mouseHandler.AddRegion(region)
		x += len(word) + 1
	}

	// Add example sentence
	d.screenManager.UpdateRegion("clickable", 2,
		"   Try clicking the words above to see them change color!", nil)
}

func (d *InteractiveDemo) updateButtons() {
	// Clear and redraw button area
	d.screenManager.UpdateRegion("buttons", 0, "🔘 Interactive Buttons:", nil)

	// Draw buttons manually for now (would be better integrated with screenManager)
	go func() {
		d.terminal.MoveCursor(0, 7)
		for _, btn := range d.buttons {
			btn.Draw(d.terminal)
		}
	}()
}

func (d *InteractiveDemo) updateRadioButtons() {
	d.screenManager.UpdateRegion("radio", 0, "🎨 Theme Selection:", nil)

	// Draw radio group
	go func() {
		d.radioGroup.Draw(d.terminal)
	}()
}

func (d *InteractiveDemo) updateInput() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Input prompt
	prompt := "$ "
	inputLine := prompt + d.currentInput

	// Show cursor position
	d.screenManager.UpdateRegion("input", 0, inputLine, nil)

	// Update cursor position
	d.screenManager.SetCursorPosition(len(prompt)+d.cursorPos, 13)
}

func (d *InteractiveDemo) handleTabCompletion() {
	if d.currentInput == "" {
		return
	}

	// Find matching suggestions
	suggestions := []string{}
	for _, cmd := range []string{
		"build", "test", "run", "install", "clean",
		"deploy", "debug", "profile", "benchmark",
		"commit", "push", "pull", "merge", "rebase",
		"status", "diff", "log", "branch", "checkout",
	} {
		if strings.HasPrefix(cmd, d.currentInput) {
			suggestions = append(suggestions, cmd)
		}
	}

	if len(suggestions) > 0 {
		d.tabCompleter.SetSuggestions(suggestions, d.currentInput)
		d.tabCompleter.Show(2, 13, 40)

		// Draw the completer
		go func() {
			d.tabCompleter.Draw(d.terminal)
		}()

		// Add clickable regions for suggestions
		for _, region := range d.tabCompleter.GetRegions() {
			d.mouseHandler.AddRegion(region)
		}
	} else {
		d.tabCompleter.Hide()
	}
}

func (d *InteractiveDemo) updateColors() {
	// Update UI colors based on theme
	switch d.selectedTheme {
	case "Dark Mode":
		for _, btn := range d.buttons {
			btn.Style = tui.NewStyle().WithBackground(tui.ColorBlack).WithForeground(tui.ColorWhite)
			btn.HoverStyle = tui.NewStyle().WithBackground(tui.ColorBrightBlack).WithForeground(tui.ColorCyan)
		}
	case "High Contrast":
		for _, btn := range d.buttons {
			btn.Style = tui.NewStyle().WithBackground(tui.ColorYellow).WithForeground(tui.ColorBlack)
			btn.HoverStyle = tui.NewStyle().WithBackground(tui.ColorWhite).WithForeground(tui.ColorBlack)
		}
	case "Solarized":
		for _, btn := range d.buttons {
			btn.Style = tui.NewStyle().WithBackground(tui.ColorGreen).WithForeground(tui.ColorBlack)
			btn.HoverStyle = tui.NewStyle().WithBackground(tui.ColorCyan).WithForeground(tui.ColorBlack)
		}
	default:
		for _, btn := range d.buttons {
			btn.Style = tui.NewStyle().WithBackground(tui.ColorBlue).WithForeground(tui.ColorWhite)
			btn.HoverStyle = tui.NewStyle().WithBackground(tui.ColorCyan).WithForeground(tui.ColorBlack)
		}
	}
	d.updateButtons()
}

func (d *InteractiveDemo) addNotification(msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	timestamp := time.Now().Format("15:04:05")
	notification := fmt.Sprintf("[%s] %s", timestamp, msg)

	d.notifications = append(d.notifications, notification)
	if len(d.notifications) > 3 {
		d.notifications = d.notifications[len(d.notifications)-3:]
	}

	// Update display
	for i := 0; i < 3; i++ {
		if i < len(d.notifications) {
			d.screenManager.UpdateRegion("notifications", i, d.notifications[i], nil)
		} else {
			d.screenManager.UpdateRegion("notifications", i, "", nil)
		}
	}
}

func (d *InteractiveDemo) Run() error {
	// Set terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() {
		term.Restore(int(os.Stdin.Fd()), oldState)
		d.terminal.DisableMouseTracking()
		d.terminal.DisableAlternateScreen()
		d.terminal.ShowCursor()
	}()

	// Clear screen
	d.terminal.Clear()

	// Refresh buttons and radio initially
	d.updateButtons()
	d.updateRadioButtons()

	// Main event loop
	buf := make([]byte, 20)
	for d.running {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		// Check for mouse events
		if buf[0] == 27 && n > 2 && buf[1] == '[' {
			if buf[2] == '<' {
				// SGR mouse event
				event, err := tui.ParseMouseEvent(buf[2:n])
				if err == nil {
					d.mouseHandler.HandleEvent(event)

					// Redraw buttons after mouse event
					d.updateButtons()
					d.updateRadioButtons()
					continue
				}
			}

			// Arrow keys in tab completion
			if d.tabCompleter.Visible && n >= 3 {
				switch buf[2] {
				case 'A': // Up
					d.tabCompleter.SelectPrev()
					d.tabCompleter.Draw(d.terminal)
				case 'B': // Down
					d.tabCompleter.SelectNext()
					d.tabCompleter.Draw(d.terminal)
				}
				continue
			}
		}

		// Regular keyboard input
		switch buf[0] {
		case 3: // Ctrl+C
			d.running = false

		case 9: // TAB
			if d.tabCompleter.Visible {
				// Accept current selection
				selected := d.tabCompleter.GetSelected()
				if selected != "" {
					d.currentInput = selected + " "
					d.cursorPos = len(d.currentInput)
					d.tabCompleter.Hide()
					d.updateInput()
				}
			} else {
				// Show completions
				d.handleTabCompletion()
			}

		case 27: // ESC
			if n == 1 {
				// Just ESC - hide completions
				d.tabCompleter.Hide()
				// Clear completion regions
				d.mouseHandler.ClearRegions()
				// Re-add persistent regions
				d.createButtons()
				d.createRadioGroup()
				d.updateClickableText()
			}

		case 13: // Enter
			if d.tabCompleter.Visible {
				// Accept completion
				selected := d.tabCompleter.GetSelected()
				if selected != "" {
					d.currentInput = selected + " "
					d.cursorPos = len(d.currentInput)
				}
				d.tabCompleter.Hide()
			} else if d.currentInput != "" {
				// Execute command
				d.commandHistory = append(d.commandHistory, d.currentInput)
				d.addNotification("▶ Executed: " + d.currentInput)
				d.currentInput = ""
				d.cursorPos = 0
			}
			d.updateInput()

		case 127, 8: // Backspace
			if d.cursorPos > 0 {
				d.currentInput = d.currentInput[:d.cursorPos-1] + d.currentInput[d.cursorPos:]
				d.cursorPos--
				d.updateInput()

				// Update completions if Visible
				if d.tabCompleter.Visible {
					d.handleTabCompletion()
				}
			}

		default:
			// Regular character
			if buf[0] >= 32 && buf[0] < 127 {
				d.currentInput = d.currentInput[:d.cursorPos] + string(buf[0]) + d.currentInput[d.cursorPos:]
				d.cursorPos++
				d.updateInput()

				// Hide completions on new input
				if d.tabCompleter.Visible {
					d.tabCompleter.Hide()
				}
			}
		}
	}

	return nil
}

func (d *InteractiveDemo) Cleanup() {
	d.running = false
	d.screenManager.Stop()
}

func main() {
	fmt.Println("\n🖱️  Interactive TUI Demo")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println("\nThis demo showcases:")
	fmt.Println("✅ Mouse support - click on words to change colors")
	fmt.Println("✅ Interactive buttons with hover effects")
	fmt.Println("✅ Tab completion with dropdown suggestions")
	fmt.Println("✅ Radio button groups for selection")
	fmt.Println("✅ Real-time notifications")
	fmt.Println("\nStarting in 2 seconds...")
	time.Sleep(2 * time.Second)

	demo, err := NewInteractiveDemo()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	demo.Setup()
	defer demo.Cleanup()

	if err := demo.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	fmt.Println("\n✨ Thanks for trying the interactive features!")
}