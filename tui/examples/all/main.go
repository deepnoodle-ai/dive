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

type ComprehensiveDemo struct {
	terminal      *tui.Terminal
	screenManager *tui.ScreenManager
	mouseHandler  *tui.MouseHandler

	// Multiple input fields
	inputs        []string
	selectedInput int
	cursorPos     []int

	// Interactive components
	buttons      []*tui.Button
	tabCompleter *tui.TabCompleter
	clickedWords map[string]tui.RGB

	// State
	running       bool
	currentMode   string
	notifications []string
	mu            sync.Mutex

	// Demo data
	cpuUsage       int
	memoryUsage    int
	networkSpeed   float64
	taskProgress   int
	spinnerFrame   int
	saveInProgress bool
}

func NewComprehensiveDemo() (*ComprehensiveDemo, error) {
	terminal, err := tui.NewTerminal()
	if err != nil {
		return nil, err
	}

	screenManager := tui.NewScreenManager(terminal, 15)

	return &ComprehensiveDemo{
		terminal:      terminal,
		screenManager: screenManager,
		mouseHandler:  tui.NewMouseHandler(),
		tabCompleter:  tui.NewTabCompleter(),
		inputs:        make([]string, 4),
		cursorPos:     make([]int, 4),
		selectedInput: 0,
		running:       true,
		currentMode:   "NORMAL",
		notifications: []string{},
		clickedWords:  make(map[string]tui.RGB),
		cpuUsage:      45,
		memoryUsage:   62,
		networkSpeed:  1.2,
		taskProgress:  0,
	}, nil
}

func (d *ComprehensiveDemo) Setup() {
	// Enable alternate screen, mouse tracking, and show cursor
	d.terminal.EnableAlternateScreen()
	d.terminal.EnableMouseTracking()
	d.terminal.ShowCursor()

	width, height := d.terminal.Size()

	// Define all screen regions
	d.screenManager.DefineRegion("title", 0, 0, width, 2, false)
	d.screenManager.DefineRegion("status", 0, 2, width, 2, false)
	d.screenManager.DefineRegion("clickable", 0, 4, width, 2, false)
	d.screenManager.DefineRegion("content", 0, 6, width, 4, false)
	d.screenManager.DefineRegion("buttons", 0, 10, width, 2, false)
	d.screenManager.DefineRegion("input", 0, 12, width, 4, true)
	d.screenManager.DefineRegion("completions", 0, 16, width, 4, false)
	d.screenManager.DefineRegion("notifications", 0, 20, width, 3, false)

	footerY := 23
	footerHeight := height - footerY - 1
	if footerHeight > 3 {
		footerHeight = 3
	}
	d.screenManager.DefineRegion("footer", 0, footerY, width, footerHeight, false)

	// Create interactive components
	d.createButtons()
	d.setupTabCompleter()

	// Initialize all regions
	d.initializeRegions()

	// Start the screen manager
	d.screenManager.Start()
}

func (d *ComprehensiveDemo) createButtons() {
	// Create interactive buttons
	d.buttons = []*tui.Button{
		tui.NewButton(5, 11, "Execute", func() {
			d.addNotification("▶ Executing: " + d.inputs[d.selectedInput])
		}),
		tui.NewButton(18, 11, "Clear", func() {
			for i := range d.inputs {
				d.inputs[i] = ""
				d.cursorPos[i] = 0
			}
			d.updateInputFields()
			d.addNotification("✨ All fields cleared")
		}),
		tui.NewButton(29, 11, "Save", func() {
			d.saveInProgress = true
			d.updateStatusBar()
			go func() {
				time.Sleep(2 * time.Second)
				d.saveInProgress = false
				d.updateStatusBar()
				d.addNotification("💾 Configuration saved")
			}()
		}),
	}

	// Add button regions to mouse handler
	for _, btn := range d.buttons {
		d.mouseHandler.AddRegion(btn.GetRegion())
	}
}

func (d *ComprehensiveDemo) setupTabCompleter() {
	commands := []string{
		"build", "test", "run", "install", "clean",
		"deploy", "debug", "profile", "benchmark",
		"commit", "push", "pull", "merge", "rebase",
	}

	d.tabCompleter.OnSelect = func(suggestion string) {
		d.inputs[d.selectedInput] = suggestion + " "
		d.cursorPos[d.selectedInput] = len(d.inputs[d.selectedInput])
		d.tabCompleter.Hide()
		d.tabCompleter.Draw(d.terminal) // Clear the dropdown
		d.updateInputFields()
	}

	d.tabCompleter.SetSuggestions(commands, "")
}

func (d *ComprehensiveDemo) updateClickableWords() {
	words := []string{"Click", "these", "words", "to", "change", "colors!"}

	var textLine strings.Builder
	textLine.WriteString("🖱️ Clickable: ")

	for i, word := range words {
		if color, exists := d.clickedWords[word]; exists {
			textLine.WriteString(color.Apply(word, false))
		} else {
			textLine.WriteString(word)
		}

		if i < len(words)-1 {
			textLine.WriteString(" ")
		}
	}

	d.screenManager.UpdateRegion("clickable", 0, textLine.String(), nil)
	d.screenManager.UpdateRegion("clickable", 1, "   Try clicking the words above!", nil)

	// Clear and re-add mouse regions
	d.mouseHandler.ClearRegions()

	// Re-add button regions
	for _, btn := range d.buttons {
		d.mouseHandler.AddRegion(btn.GetRegion())
	}

	// Add word regions
	x := 14 // After "Clickable: "
	y := 4

	for _, word := range words {
		wordCopy := word
		region := &tui.MouseRegion{
			X:      x,
			Y:      y,
			Width:  len(word),
			Height: 1,
			Label:  word,
			Handler: func(event *tui.MouseEvent) {
				colors := []tui.RGB{
					tui.NewRGB(255, 100, 100),
					tui.NewRGB(100, 255, 100),
					tui.NewRGB(100, 100, 255),
					tui.NewRGB(255, 255, 100),
				}

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
				d.updateClickableWords()
				d.addNotification(fmt.Sprintf("🎨 Colored '%s'", wordCopy))
			},
		}
		d.mouseHandler.AddRegion(region)
		x += len(word) + 1
	}
}

func (d *ComprehensiveDemo) updateButtons() {
	d.screenManager.UpdateRegion("buttons", 0, "🔘 Interactive Buttons (click them!):", nil)

	// Draw buttons
	go func() {
		time.Sleep(10 * time.Millisecond) // Small delay to ensure screen is ready
		for _, btn := range d.buttons {
			btn.Draw(d.terminal)
		}
	}()
}

func (d *ComprehensiveDemo) initializeRegions() {
	// Title
	titleText := d.getSpinner() + " Dive TUI - Complete Demo with Mouse! " + d.getSpinner()
	d.screenManager.UpdateRegion("title", 0, titleText,
		tui.CreateRainbowText(titleText, 15))
	d.screenManager.UpdateRegion("title", 1,
		"════════════════════════════════════════",
		nil)

	// Status
	d.updateStatusBar()

	// Clickable words
	d.updateClickableWords()

	// Content
	d.screenManager.UpdateRegion("content", 0, "📊 System Metrics:", nil)
	d.updateMetrics()

	// Buttons
	d.updateButtons()

	// Input fields
	d.updateInputFields()

	// Footer
	d.screenManager.UpdateRegion("footer", 0,
		"🖱️ Click words/buttons | TAB: Completions | ↑↓: Navigate",
		nil)
	d.screenManager.UpdateRegion("footer", 1,
		"Enter: Accept | ESC: Cancel | Ctrl+C: Exit",
		nil)
}

func (d *ComprehensiveDemo) updateStatusBar() {
	modeColor := tui.NewRGB(0, 255, 100)
	if d.currentMode == "INSERT" {
		modeColor = tui.NewRGB(255, 100, 0)
	}

	saveIndicator := ""
	if d.saveInProgress {
		saveIndicator = " " + d.getSpinner() + " Saving..."
	}

	statusText := fmt.Sprintf("Mode: %s | Field: %d/4 | Time: %s%s",
		d.currentMode,
		d.selectedInput+1,
		time.Now().Format("15:04:05"),
		saveIndicator)

	d.screenManager.UpdateRegion("status", 0, statusText, tui.CreatePulseText(modeColor, 30))

	progress := d.taskProgress % 101
	filled := progress / 5
	progressBar := strings.Repeat("█", filled) + strings.Repeat("░", 20-filled)
	progressText := fmt.Sprintf("Progress: [%s] %d%%", progressBar, progress)
	d.screenManager.UpdateRegion("status", 1, progressText, nil)
}

func (d *ComprehensiveDemo) updateMetrics() {
	cpuBar := d.createMeter(d.cpuUsage, 100)
	cpuText := fmt.Sprintf("   CPU: %s %3d%%", cpuBar, d.cpuUsage)
	d.screenManager.UpdateRegion("content", 1, cpuText, nil)

	memBar := d.createMeter(d.memoryUsage, 100)
	memText := fmt.Sprintf("   RAM: %s %3d%%", memBar, d.memoryUsage)
	d.screenManager.UpdateRegion("content", 2, memText, nil)

	netText := fmt.Sprintf("   NET: %s %.1f GB/s", d.getSpinner(), d.networkSpeed)
	d.screenManager.UpdateRegion("content", 3, netText, nil)
}

func (d *ComprehensiveDemo) createMeter(value, max int) string {
	width := 20
	filled := (value * width) / max

	bar := "["
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "·"
		}
	}
	bar += "]"
	return bar
}

func (d *ComprehensiveDemo) updateInputFields() {
	d.mu.Lock()
	defer d.mu.Unlock()

	labels := []string{"Name    ", "Email   ", "Project ", "Command "}

	for i := 0; i < 4; i++ {
		text := labels[i] + ": " + d.inputs[i]

		if i == d.selectedInput {
			style := tui.NewStyle().WithBackground(tui.ColorGreen).WithForeground(tui.ColorBlack)
			text = style.Apply(text)
		}

		d.screenManager.UpdateRegion("input", i, text, nil)
	}

	cursorX := 10 + d.cursorPos[d.selectedInput]
	cursorY := 12 + d.selectedInput
	d.screenManager.SetCursorPosition(cursorX, cursorY)
}

func (d *ComprehensiveDemo) handleTabCompletion() {
	if d.inputs[d.selectedInput] == "" {
		return
	}

	suggestions := []string{}
	commands := []string{
		"build", "test", "run", "install", "clean",
		"deploy", "debug", "profile", "benchmark",
		"commit", "push", "pull", "merge", "rebase",
	}

	for _, cmd := range commands {
		if strings.HasPrefix(cmd, d.inputs[d.selectedInput]) {
			suggestions = append(suggestions, cmd)
		}
	}

	if len(suggestions) > 0 {
		d.tabCompleter.SetSuggestions(suggestions, d.inputs[d.selectedInput])
		d.tabCompleter.Show(10, 12+d.selectedInput, 30)

		go func() {
			d.tabCompleter.Draw(d.terminal)
		}()

		for _, region := range d.tabCompleter.GetRegions() {
			d.mouseHandler.AddRegion(region)
		}
	} else {
		d.tabCompleter.Hide()
	}
}

func (d *ComprehensiveDemo) addNotification(msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	notification := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)

	d.notifications = append(d.notifications, notification)
	if len(d.notifications) > 3 {
		d.notifications = d.notifications[len(d.notifications)-3:]
	}

	for i := 0; i < 3; i++ {
		if i < len(d.notifications) {
			d.screenManager.UpdateRegion("notifications", i, d.notifications[i], nil)
		} else {
			d.screenManager.UpdateRegion("notifications", i, "", nil)
		}
	}
}

func (d *ComprehensiveDemo) getSpinner() string {
	spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return spinners[d.spinnerFrame%len(spinners)]
}

func (d *ComprehensiveDemo) StartBackgroundTasks() {
	// Spinner updater
	go func() {
		for d.running {
			time.Sleep(150 * time.Millisecond)
			d.spinnerFrame++

			if d.spinnerFrame%3 == 0 {
				titleText := d.getSpinner() + " Dive TUI - Complete Demo with Mouse! " + d.getSpinner()
				d.screenManager.UpdateRegion("title", 0, titleText,
					tui.CreateRainbowText(titleText, 15))
			}
		}
	}()

	// Metric updater
	go func() {
		for d.running {
			time.Sleep(750 * time.Millisecond)

			d.cpuUsage = 40 + int(time.Now().Unix()%40)
			d.memoryUsage = 50 + int(time.Now().Unix()%30)
			d.networkSpeed = 0.5 + float64(time.Now().Unix()%20)/10.0
			d.taskProgress = (d.taskProgress + 2) % 101

			d.updateMetrics()
			d.updateStatusBar()
		}
	}()
}

func (d *ComprehensiveDemo) Run() error {
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

	d.terminal.Clear()

	// Start background tasks
	d.StartBackgroundTasks()

	// Initial draw
	d.updateButtons()
	d.updateClickableWords()

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
					// Redraw components after mouse event
					d.updateButtons()
					d.updateClickableWords()
					continue
				}
			}

			// Arrow keys
			if n >= 3 && buf[2] >= 'A' && buf[2] <= 'D' {
				if d.tabCompleter.Visible {
					switch buf[2] {
					case 'A': // Up
						d.tabCompleter.SelectPrev()
						d.tabCompleter.Draw(d.terminal)
					case 'B': // Down
						d.tabCompleter.SelectNext()
						d.tabCompleter.Draw(d.terminal)
					}
				} else {
					// Navigate fields
					switch buf[2] {
					case 'A': // Up
						if d.selectedInput > 0 {
							d.selectedInput--
							d.updateInputFields()
						}
					case 'B': // Down
						if d.selectedInput < 3 {
							d.selectedInput++
							d.updateInputFields()
						}
					case 'C': // Right
						if d.cursorPos[d.selectedInput] < len(d.inputs[d.selectedInput]) {
							d.cursorPos[d.selectedInput]++
							d.updateInputFields()
						}
					case 'D': // Left
						if d.cursorPos[d.selectedInput] > 0 {
							d.cursorPos[d.selectedInput]--
							d.updateInputFields()
						}
					}
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
				selected := d.tabCompleter.GetSelected()
				if selected != "" {
					d.inputs[d.selectedInput] = selected + " "
					d.cursorPos[d.selectedInput] = len(d.inputs[d.selectedInput])
					d.tabCompleter.Hide()
					d.tabCompleter.Draw(d.terminal) // Clear the dropdown
					d.updateInputFields()
				}
			} else if d.inputs[d.selectedInput] != "" {
				d.handleTabCompletion()
			} else {
				d.selectedInput = (d.selectedInput + 1) % 4
				d.updateInputFields()
			}

		case 27: // ESC
			if d.tabCompleter.Visible {
				d.tabCompleter.Hide()
				d.tabCompleter.Draw(d.terminal) // Clear the dropdown
				d.updateClickableWords()        // Re-add mouse regions
			} else {
				d.inputs[d.selectedInput] = ""
				d.cursorPos[d.selectedInput] = 0
				d.updateInputFields()
			}

		case 13: // Enter
			if d.tabCompleter.Visible {
				selected := d.tabCompleter.GetSelected()
				if selected != "" {
					d.inputs[d.selectedInput] = selected + " "
					d.cursorPos[d.selectedInput] = len(d.inputs[d.selectedInput])
				}
				d.tabCompleter.Hide()
				d.tabCompleter.Draw(d.terminal) // Clear the dropdown
			} else if d.inputs[d.selectedInput] != "" {
				d.addNotification("Executed: " + d.inputs[d.selectedInput])
			}
			d.selectedInput = (d.selectedInput + 1) % 4
			d.updateInputFields()

		case 127, 8: // Backspace
			if d.cursorPos[d.selectedInput] > 0 {
				d.inputs[d.selectedInput] = d.inputs[d.selectedInput][:d.cursorPos[d.selectedInput]-1] +
					d.inputs[d.selectedInput][d.cursorPos[d.selectedInput]:]
				d.cursorPos[d.selectedInput]--
				d.updateInputFields()
			}

		default:
			if buf[0] >= 32 && buf[0] < 127 {
				d.inputs[d.selectedInput] = d.inputs[d.selectedInput][:d.cursorPos[d.selectedInput]] +
					string(buf[0]) + d.inputs[d.selectedInput][d.cursorPos[d.selectedInput]:]
				d.cursorPos[d.selectedInput]++
				d.updateInputFields()

				if d.tabCompleter.Visible {
					d.tabCompleter.Hide()
					d.tabCompleter.Draw(d.terminal) // Clear the dropdown
				}
			}
		}
	}

	return nil
}

func (d *ComprehensiveDemo) Cleanup() {
	d.running = false
	d.screenManager.Stop()
	time.Sleep(100 * time.Millisecond)
}

func main() {
	fmt.Println("\n🚀 Comprehensive TUI Demo with Mouse Support!")
	fmt.Println("=" + strings.Repeat("=", 50))
	fmt.Println("\nFeatures:")
	fmt.Println("✅ Click on words to change their colors")
	fmt.Println("✅ Click on buttons (Execute, Clear, Save)")
	fmt.Println("✅ Tab completion (type 'b' then TAB)")
	fmt.Println("✅ Arrow keys for navigation")
	fmt.Println("✅ Real-time metrics and animations")
	fmt.Println("\nMake sure your terminal supports mouse events!")
	fmt.Println("Starting in 2 seconds...")
	time.Sleep(2 * time.Second)

	demo, err := NewComprehensiveDemo()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	demo.Setup()
	defer demo.Cleanup()

	if err := demo.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	fmt.Println("\n✨ Thanks for trying the comprehensive demo!")
}
