package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/tui"
)

func main() {
	// Create terminal
	terminal, err := tui.NewTerminal()
	if err != nil {
		fmt.Printf("Error creating terminal: %v\n", err)
		return
	}

	// Enable alternate screen for clean demo
	terminal.EnableAlternateScreen()
	terminal.HideCursor()
	defer func() {
		terminal.ShowCursor()
		terminal.DisableAlternateScreen()
	}()

	// Create animated layout with 30 FPS
	layout := tui.NewAnimatedInputLayout(terminal, 30)

	// Set up animated header (2 lines)
	layout.SetAnimatedHeader(2)
	layout.SetHeaderLine(0, "🚀 Dive TUI Animation Demo", tui.CreateRainbowText("🚀 Dive TUI Animation Demo", 20))
	layout.SetHeaderLine(1, "Multiple animation types in action!", tui.CreateReverseRainbowText("Multiple animation types in action!", 25))

	// Set up animated content area (4 lines above input)
	layout.SetAnimatedContent(4)
	layout.SetContentLine(0, "Status: Processing...", tui.CreatePulseText(tui.NewRGB(0, 255, 100), 40))
	layout.SetContentLine(1, "Progress: ████████████████████", tui.CreateRainbowText("Progress: ████████████████████", 15))
	layout.SetContentLine(2, "Connections: 42 active", nil) // Static line
	layout.SetContentLine(3, "🌟 Sparkles everywhere! 🌟", tui.CreateRainbowText("🌟 Sparkles everywhere! 🌟", 10))

	// Set up animated footer (3 lines)
	layout.SetAnimatedFooter(3)
	layout.AddFooterLine("Press 'q' to quit | Type anything to see input", nil)

	// Create animated status bar for second footer line
	statusBar := tui.NewAnimatedStatusBar(0, 0, 80)
	statusBar.AddItem("CPU", "85%", "⚡", tui.CreatePulseText(tui.NewRGB(255, 100, 0), 30), tui.NewStyle())
	statusBar.AddItem("Memory", "67%", "🧠", tui.CreateRainbowText("67%", 20), tui.NewStyle())
	statusBar.AddItem("Network", "1.2GB/s", "🌐", tui.CreatePulseText(tui.NewRGB(0, 255, 255), 25), tui.NewStyle())
	layout.GetAnimator().AddElement(statusBar)

	layout.AddFooterLine("Rainbow footer text flowing like water", tui.CreateRainbowText("Rainbow footer text flowing like water", 18))

	// Set input prompt
	layout.SetPrompt("💬 Enter command: ", tui.NewStyle().WithForeground(tui.ColorCyan).WithBold())

	// Start animations
	layout.StartAnimations()
	defer layout.StopAnimations()

	// Initial draw
	terminal.Clear()
	layout.DrawPrompt()

	// Set up input handling
	scanner := bufio.NewScanner(os.Stdin)
	inputText := ""

	// Update status bar position to be in the correct footer line
	_, termHeight := terminal.Size()
	statusBar.SetPosition(0, termHeight-2)

	// Demo loop with periodic updates
	go func() {
		counter := 0
		for {
			time.Sleep(500 * time.Millisecond)
			counter++

			// Update content lines periodically to show dynamic content
			switch counter % 8 {
			case 0:
				layout.SetContentLine(0, "Status: Initializing...", tui.CreatePulseText(tui.NewRGB(255, 255, 0), 40))
			case 1:
				layout.SetContentLine(0, "Status: Loading modules...", tui.CreatePulseText(tui.NewRGB(255, 165, 0), 40))
			case 2:
				layout.SetContentLine(0, "Status: Connecting...", tui.CreatePulseText(tui.NewRGB(0, 255, 255), 40))
			case 3:
				layout.SetContentLine(0, "Status: Processing data...", tui.CreatePulseText(tui.NewRGB(0, 255, 100), 40))
			case 4:
				layout.SetContentLine(0, "Status: Optimizing...", tui.CreatePulseText(tui.NewRGB(255, 0, 255), 40))
			case 5:
				layout.SetContentLine(0, "Status: Finalizing...", tui.CreatePulseText(tui.NewRGB(100, 255, 100), 40))
			case 6:
				layout.SetContentLine(0, "Status: Complete!", tui.CreateRainbowText("Status: Complete!", 15))
			case 7:
				layout.SetContentLine(0, "Status: Ready for input", tui.CreatePulseText(tui.NewRGB(0, 255, 100), 40))
			}

			// Update progress bar
			progress := (counter % 20) + 1
			progressBar := strings.Repeat("█", progress) + strings.Repeat("░", 20-progress)
			layout.SetContentLine(1, fmt.Sprintf("Progress: %s %d%%", progressBar, progress*5), tui.CreateRainbowText(fmt.Sprintf("Progress: %s %d%%", progressBar, progress*5), 12))

			// Update connections count
			connections := 40 + (counter % 10)
			layout.SetContentLine(2, fmt.Sprintf("Connections: %d active", connections), nil)

			// Update status bar values
			cpu := 70 + (counter % 30)
			memory := 60 + (counter % 20)
			network := fmt.Sprintf("%.1fGB/s", 1.0+float64(counter%10)*0.1)

			statusBar.UpdateItem(0, "CPU", fmt.Sprintf("%d%%", cpu))
			statusBar.UpdateItem(1, "Memory", fmt.Sprintf("%d%%", memory))
			statusBar.UpdateItem(2, "Network", network)
		}
	}()

	fmt.Println("\n🎨 Welcome to the Dive TUI Animation Demo!")
	fmt.Println("You should see:")
	fmt.Println("✨ Rainbow animated header text")
	fmt.Println("📊 Animated status indicators in the content area")
	fmt.Println("🌈 Moving progress bars")
	fmt.Println("📱 Animated status bar in the footer")
	fmt.Println("💫 Rainbow footer text")
	fmt.Println("\nType anything and press Enter, or 'q' to quit...")

	// Simple input loop for demo
	for {
		// Position cursor for input
		x, y := layout.GetInputPosition()
		terminal.MoveCursor(x, y)

		// Read input
		if scanner.Scan() {
			inputText = scanner.Text()

			if inputText == "q" || inputText == "quit" {
				break
			}

			// Echo the input in the content area
			layout.SetContentLine(3, fmt.Sprintf("📝 You typed: %s", inputText), tui.CreateRainbowText(fmt.Sprintf("📝 You typed: %s", inputText), 15))

			// Redraw prompt
			layout.DrawPrompt()
		}
	}

	fmt.Println("\n👋 Thanks for trying the animation demo!")
}