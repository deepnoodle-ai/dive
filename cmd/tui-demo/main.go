package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/tui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--help" {
		showHelp()
		return
	}

	// Create terminal
	terminal, err := tui.NewTerminal()
	if err != nil {
		fmt.Printf("Error creating terminal: %v\n", err)
		return
	}

	// Show menu first
	showMenu()

	// Get user choice
	choice := getUserChoice()

	switch choice {
	case 1:
		runBasicAnimationDemo(terminal)
	case 2:
		runInteractiveDemo(terminal)
	case 3:
		runStatusBarDemo(terminal)
	case 4:
		runFullFeaturedDemo(terminal)
	case 5:
		runColorTestDemo(terminal)
	case 6:
		runClassicTUIDemo(terminal)
	default:
		fmt.Println("Invalid choice. Running basic demo...")
		runBasicAnimationDemo(terminal)
	}
}

func showHelp() {
	fmt.Println("🚀 Dive TUI Animation Demo")
	fmt.Println("==========================")
	fmt.Println("")
	fmt.Println("This demo showcases the animation capabilities of the Dive TUI package:")
	fmt.Println("")
	fmt.Println("Features demonstrated:")
	fmt.Println("  • Multi-line animated content above input area")
	fmt.Println("  • Rainbow text animations with configurable speed")
	fmt.Println("  • Animated status bars and footers")
	fmt.Println("  • Real-time content updates during user input")
	fmt.Println("  • 30+ FPS smooth animations")
	fmt.Println("  • Full RGB color support")
	fmt.Println("")
	fmt.Println("Usage: ./tui-demo")
	fmt.Println("       ./tui-demo --help")
}

func showMenu() {
	fmt.Println("🎨 Dive TUI Demo - Complete Feature Showcase")
	fmt.Println("=============================================")
	fmt.Println("")
	fmt.Println("Choose a demo to run:")
	fmt.Println("  1. Basic Animation Features")
	fmt.Println("  2. Interactive Input with Animations")
	fmt.Println("  3. Animated Status Bar Demo")
	fmt.Println("  4. Full-Featured Demo (All Features)")
	fmt.Println("  5. Color and Gradient Test")
	fmt.Println("  6. Classic TUI Components")
	fmt.Println("")
	fmt.Print("Enter your choice (1-6): ")
}

func getUserChoice() int {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		choice, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
		if err != nil {
			return 1
		}
		return choice
	}
	return 1
}

func runBasicAnimationDemo(terminal *tui.Terminal) {
	fmt.Println("\n🎯 Basic Animation Features Demo")
	fmt.Println("Press Enter to continue or 'q' to quit")

	// Enable alternate screen
	terminal.EnableAlternateScreen()
	terminal.HideCursor()
	defer func() {
		terminal.ShowCursor()
		terminal.DisableAlternateScreen()
	}()

	// Create animated layout
	layout := tui.NewAnimatedInputLayout(terminal, 30)

	// Set up header
	layout.SetAnimatedHeader(2)
	layout.SetHeaderLine(0, "🌈 Basic Animation Demo", tui.CreateRainbowText("🌈 Basic Animation Demo", 20))
	layout.SetHeaderLine(1, "Watch the colors flow!", tui.CreateReverseRainbowText("Watch the colors flow!", 25))

	// Set up content area
	layout.SetAnimatedContent(3)
	layout.SetContentLine(0, "Rainbow Text Animation", tui.CreateRainbowText("Rainbow Text Animation", 15))
	layout.SetContentLine(1, "Pulsing Status Indicator", tui.CreatePulseText(tui.NewRGB(0, 255, 100), 40))
	layout.SetContentLine(2, "Reverse Rainbow Effect", tui.CreateReverseRainbowText("Reverse Rainbow Effect", 18))

	// Set up footer
	layout.SetAnimatedFooter(1)
	layout.SetFooterLine(0, "Press Enter to continue or 'q' to quit", nil)

	// Set prompt
	layout.SetPrompt("Command: ", tui.NewStyle().WithForeground(tui.ColorCyan).WithBold())

	// Start animations
	layout.StartAnimations()
	defer layout.StopAnimations()

	// Initial draw
	terminal.Clear()
	layout.DrawPrompt()

	// Input loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		if scanner.Scan() {
			input := strings.TrimSpace(scanner.Text())
			if input == "q" || input == "quit" {
				break
			}
			layout.DrawPrompt()
		}
	}

	fmt.Println("\n✅ Basic animation demo completed!")
}

func runInteractiveDemo(terminal *tui.Terminal) {
	fmt.Println("\n💬 Interactive Input with Animations Demo")
	fmt.Println("Type commands while animations run. Try 'help', 'status', 'rainbow', or 'q' to quit")

	// Enable alternate screen
	terminal.EnableAlternateScreen()
	terminal.HideCursor()
	defer func() {
		terminal.ShowCursor()
		terminal.DisableAlternateScreen()
	}()

	// Create animated layout
	layout := tui.NewAnimatedInputLayout(terminal, 30)

	// Set up header
	layout.SetAnimatedHeader(2)
	layout.SetHeaderLine(0, "🎮 Interactive Demo", tui.CreateRainbowText("🎮 Interactive Demo", 20))
	layout.SetHeaderLine(1, "Type commands below!", tui.CreatePulseText(tui.NewRGB(255, 255, 0), 30))

	// Set up content area
	layout.SetAnimatedContent(4)
	layout.SetContentLine(0, "Ready for commands...", tui.CreatePulseText(tui.NewRGB(0, 255, 0), 35))
	layout.SetContentLine(1, "Type 'help' for available commands", nil)
	layout.SetContentLine(2, "Last command: none", nil)
	layout.SetContentLine(3, "Command counter: 0", tui.CreateRainbowText("Command counter: 0", 25))

	// Set up footer
	layout.SetAnimatedFooter(1)
	layout.SetFooterLine(0, "Available: help, status, rainbow, clear, q", nil)

	// Set prompt
	layout.SetPrompt("💬 ", tui.NewStyle().WithForeground(tui.ColorCyan).WithBold())

	// Start animations
	layout.StartAnimations()
	defer layout.StopAnimations()

	// Initial draw
	terminal.Clear()
	layout.DrawPrompt()

	commandCount := 0
	scanner := bufio.NewScanner(os.Stdin)

	for {
		if scanner.Scan() {
			input := strings.TrimSpace(scanner.Text())
			commandCount++

			switch strings.ToLower(input) {
			case "q", "quit", "exit":
				return
			case "help":
				layout.SetContentLine(0, "Help: Available commands", tui.CreateRainbowText("Help: Available commands", 15))
				layout.SetContentLine(1, "• help - Show this help", nil)
				layout.SetContentLine(2, "• status - Show system status", nil)
			case "status":
				layout.SetContentLine(0, "System Status: Online", tui.CreatePulseText(tui.NewRGB(0, 255, 0), 30))
				layout.SetContentLine(1, "Memory: 67% used", tui.CreatePulseText(tui.NewRGB(255, 200, 0), 40))
				layout.SetContentLine(2, "CPU: 45% used", tui.CreatePulseText(tui.NewRGB(0, 255, 255), 35))
			case "rainbow":
				layout.SetContentLine(0, "🌈 Rainbow mode activated!", tui.CreateRainbowText("🌈 Rainbow mode activated!", 10))
				layout.SetContentLine(1, "Everything is colorful now!", tui.CreateReverseRainbowText("Everything is colorful now!", 12))
				layout.SetContentLine(2, "✨ Sparkles and magic! ✨", tui.CreateRainbowText("✨ Sparkles and magic! ✨", 8))
			case "clear":
				layout.SetContentLine(0, "Screen cleared!", nil)
				layout.SetContentLine(1, "", nil)
				layout.SetContentLine(2, "", nil)
			default:
				layout.SetContentLine(0, fmt.Sprintf("Unknown command: %s", input), tui.CreatePulseText(tui.NewRGB(255, 100, 100), 25))
				layout.SetContentLine(1, "Type 'help' for available commands", nil)
				layout.SetContentLine(2, fmt.Sprintf("Last command: %s", input), nil)
			}

			layout.SetContentLine(3, fmt.Sprintf("Command counter: %d", commandCount), tui.CreateRainbowText(fmt.Sprintf("Command counter: %d", commandCount), 25))
			layout.DrawPrompt()
		}
	}
}

func runStatusBarDemo(terminal *tui.Terminal) {
	fmt.Println("\n📊 Animated Status Bar Demo")
	fmt.Println("Watch the animated status bars update in real-time")

	// Enable alternate screen
	terminal.EnableAlternateScreen()
	terminal.HideCursor()
	defer func() {
		terminal.ShowCursor()
		terminal.DisableAlternateScreen()
	}()

	// Create animated layout
	layout := tui.NewAnimatedInputLayout(terminal, 30)

	// Set up header
	layout.SetAnimatedHeader(1)
	layout.SetHeaderLine(0, "📊 Status Bar Animation Demo", tui.CreateRainbowText("📊 Status Bar Animation Demo", 22))

	// Set up content area
	layout.SetAnimatedContent(5)
	layout.SetContentLine(0, "System Monitoring Dashboard", tui.CreateRainbowText("System Monitoring Dashboard", 18))
	layout.SetContentLine(1, "CPU Usage: 45%", tui.CreatePulseText(tui.NewRGB(255, 200, 0), 30))
	layout.SetContentLine(2, "Memory: 67%", tui.CreatePulseText(tui.NewRGB(0, 255, 200), 35))
	layout.SetContentLine(3, "Network: 1.2GB/s", tui.CreateRainbowText("Network: 1.2GB/s", 20))
	layout.SetContentLine(4, "Disk I/O: 45MB/s", tui.CreatePulseText(tui.NewRGB(255, 0, 255), 40))

	// Set up animated footer with status bar
	layout.SetAnimatedFooter(2)
	width, _ := terminal.Size()

	statusBar := tui.NewAnimatedStatusBar(0, 0, width)
	statusBar.AddItem("CPU", "45%", "⚡", tui.CreatePulseText(tui.NewRGB(255, 200, 0), 25), tui.NewStyle())
	statusBar.AddItem("RAM", "67%", "🧠", tui.CreateRainbowText("67%", 15), tui.NewStyle())
	statusBar.AddItem("NET", "1.2GB/s", "🌐", tui.CreatePulseText(tui.NewRGB(0, 255, 255), 30), tui.NewStyle())
	statusBar.AddItem("DISK", "45MB/s", "💾", tui.CreateRainbowText("45MB/s", 18), tui.NewStyle())

	// Position status bar in footer
	_, termHeight := terminal.Size()
	statusBar.SetPosition(0, termHeight-2)
	layout.GetAnimator().AddElement(statusBar)

	layout.SetFooterLine(1, "Press Enter to continue or 'q' to quit", nil)

	// Set prompt
	layout.SetPrompt("Monitor: ", tui.NewStyle().WithForeground(tui.ColorGreen).WithBold())

	// Start animations
	layout.StartAnimations()
	defer layout.StopAnimations()

	// Initial draw
	terminal.Clear()
	layout.DrawPrompt()

	// Update values periodically
	go func() {
		counter := 0
		for {
			time.Sleep(800 * time.Millisecond)
			counter++

			// Update CPU
			cpu := 40 + (counter % 25)
			layout.SetContentLine(1, fmt.Sprintf("CPU Usage: %d%%", cpu), tui.CreatePulseText(tui.NewRGB(255, 200, 0), 30))
			statusBar.UpdateItem(0, "CPU", fmt.Sprintf("%d%%", cpu))

			// Update Memory
			memory := 60 + (counter % 15)
			layout.SetContentLine(2, fmt.Sprintf("Memory: %d%%", memory), tui.CreatePulseText(tui.NewRGB(0, 255, 200), 35))
			statusBar.UpdateItem(1, "RAM", fmt.Sprintf("%d%%", memory))

			// Update Network
			network := fmt.Sprintf("%.1fGB/s", 1.0+float64(counter%10)*0.2)
			layout.SetContentLine(3, fmt.Sprintf("Network: %s", network), tui.CreateRainbowText(fmt.Sprintf("Network: %s", network), 20))
			statusBar.UpdateItem(2, "NET", network)

			// Update Disk
			disk := 30 + (counter % 40)
			layout.SetContentLine(4, fmt.Sprintf("Disk I/O: %dMB/s", disk), tui.CreatePulseText(tui.NewRGB(255, 0, 255), 40))
			statusBar.UpdateItem(3, "DISK", fmt.Sprintf("%dMB/s", disk))
		}
	}()

	// Input loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		if scanner.Scan() {
			input := strings.TrimSpace(scanner.Text())
			if input == "q" || input == "quit" {
				break
			}
			layout.DrawPrompt()
		}
	}

	fmt.Println("\n✅ Status bar demo completed!")
}

func runFullFeaturedDemo(terminal *tui.Terminal) {
	fmt.Println("\n🚀 Full-Featured Animation Demo")
	fmt.Println("This demo showcases ALL animation features together")

	// Enable alternate screen
	terminal.EnableAlternateScreen()
	terminal.HideCursor()
	defer func() {
		terminal.ShowCursor()
		terminal.DisableAlternateScreen()
	}()

	// Create animated layout with high FPS for smooth animations
	layout := tui.NewAnimatedInputLayout(terminal, 30)

	// Set up animated header (3 lines)
	layout.SetAnimatedHeader(3)
	layout.SetHeaderLine(0, "🚀 Dive TUI - Full Animation Demo", tui.CreateRainbowText("🚀 Dive TUI - Full Animation Demo", 18))
	layout.SetHeaderLine(1, "Multiple animations running simultaneously", tui.CreateReverseRainbowText("Multiple animations running simultaneously", 22))
	layout.SetHeaderLine(2, "Real-time updates while you type!", tui.CreatePulseText(tui.NewRGB(255, 255, 0), 25))

	// Set up animated content area (6 lines above input)
	layout.SetAnimatedContent(6)
	layout.SetContentLine(0, "System Status: Initializing...", tui.CreatePulseText(tui.NewRGB(255, 200, 0), 35))
	layout.SetContentLine(1, "Progress: ████████████████████ 100%", tui.CreateRainbowText("Progress: ████████████████████ 100%", 12))
	layout.SetContentLine(2, "Active Connections: 42", nil)
	layout.SetContentLine(3, "🌟 Animated Elements: Header, Content, Footer", tui.CreateRainbowText("🌟 Animated Elements: Header, Content, Footer", 15))
	layout.SetContentLine(4, "💫 Color Effects: Rainbow, Pulse, Gradients", tui.CreateReverseRainbowText("💫 Color Effects: Rainbow, Pulse, Gradients", 14))
	layout.SetContentLine(5, "User Input: Ready", tui.CreatePulseText(tui.NewRGB(0, 255, 100), 30))

	// Set up animated footer (3 lines)
	layout.SetAnimatedFooter(3)

	// Create animated status bar
	width, _ := terminal.Size()
	statusBar := tui.NewAnimatedStatusBar(0, 0, width)
	statusBar.AddItem("FPS", "30", "🎯", tui.CreateRainbowText("30", 20), tui.NewStyle())
	statusBar.AddItem("Animations", "12", "✨", tui.CreatePulseText(tui.NewRGB(255, 0, 255), 25), tui.NewStyle())
	statusBar.AddItem("Colors", "16M", "🎨", tui.CreateRainbowText("16M", 18), tui.NewStyle())
	statusBar.AddItem("Status", "Running", "🚀", tui.CreatePulseText(tui.NewRGB(0, 255, 0), 30), tui.NewStyle())

	// Position status bar in footer
	_, termHeight := terminal.Size()
	statusBar.SetPosition(0, termHeight-3)
	layout.GetAnimator().AddElement(statusBar)

	layout.SetFooterLine(1, "Commands: help, status, demo, rainbow, clear, q", nil)
	layout.SetFooterLine(2, "🌈 Rainbow footer flowing like liquid light", tui.CreateRainbowText("🌈 Rainbow footer flowing like liquid light", 16))

	// Set prompt with animation
	layout.SetPrompt("🎮 CMD: ", tui.NewStyle().WithForeground(tui.ColorCyan).WithBold())

	// Start animations
	layout.StartAnimations()
	defer layout.StopAnimations()

	// Initial draw
	terminal.Clear()
	layout.DrawPrompt()

	// Dynamic updates
	go func() {
		counter := 0
		statuses := []string{
			"Initializing systems...",
			"Loading modules...",
			"Connecting to services...",
			"Processing data...",
			"Optimizing performance...",
			"Ready for input!",
		}

		for {
			time.Sleep(1500 * time.Millisecond)
			counter++

			// Cycle through different statuses
			statusIndex := counter % len(statuses)
			status := statuses[statusIndex]

			var statusColor tui.RGB
			switch statusIndex {
			case 0:
				statusColor = tui.NewRGB(255, 255, 0) // Yellow
			case 1:
				statusColor = tui.NewRGB(255, 165, 0) // Orange
			case 2:
				statusColor = tui.NewRGB(0, 255, 255) // Cyan
			case 3:
				statusColor = tui.NewRGB(255, 0, 255) // Magenta
			case 4:
				statusColor = tui.NewRGB(255, 100, 0) // Red-orange
			case 5:
				statusColor = tui.NewRGB(0, 255, 0)   // Green
			}

			layout.SetContentLine(0, fmt.Sprintf("System Status: %s", status), tui.CreatePulseText(statusColor, 35))

			// Update progress bar
			progress := ((counter % 20) + 1) * 5
			progressBar := strings.Repeat("█", progress/5) + strings.Repeat("░", 20-progress/5)
			layout.SetContentLine(1, fmt.Sprintf("Progress: %s %d%%", progressBar, progress), tui.CreateRainbowText(fmt.Sprintf("Progress: %s %d%%", progressBar, progress), 12))

			// Update connections
			connections := 35 + (counter % 15)
			layout.SetContentLine(2, fmt.Sprintf("Active Connections: %d", connections), nil)

			// Update status bar
			animations := 8 + (counter % 8)
			statusBar.UpdateItem(1, "Animations", fmt.Sprintf("%d", animations))
		}
	}()

	// Input handling
	scanner := bufio.NewScanner(os.Stdin)
	inputCount := 0

	for {
		if scanner.Scan() {
			input := strings.TrimSpace(scanner.Text())
			inputCount++

			switch strings.ToLower(input) {
			case "q", "quit", "exit":
				return
			case "help":
				layout.SetContentLine(3, "📖 Help: Available commands shown in footer", tui.CreateRainbowText("📖 Help: Available commands shown in footer", 15))
				layout.SetContentLine(4, "✨ All animation features are active!", tui.CreatePulseText(tui.NewRGB(0, 255, 255), 20))
			case "status":
				layout.SetContentLine(3, "📊 Status: All systems operational", tui.CreatePulseText(tui.NewRGB(0, 255, 0), 25))
				layout.SetContentLine(4, "🎯 30 FPS animations running smoothly", tui.CreateRainbowText("🎯 30 FPS animations running smoothly", 18))
			case "demo":
				layout.SetContentLine(3, "🎮 Demo Mode: Showcasing all features", tui.CreateRainbowText("🎮 Demo Mode: Showcasing all features", 12))
				layout.SetContentLine(4, "🌈 Header + Content + Footer animations", tui.CreateReverseRainbowText("🌈 Header + Content + Footer animations", 14))
			case "rainbow":
				layout.SetContentLine(3, "🌈 RAINBOW MODE ACTIVATED! 🌈", tui.CreateRainbowText("🌈 RAINBOW MODE ACTIVATED! 🌈", 8))
				layout.SetContentLine(4, "✨ Everything is rainbow now! ✨", tui.CreateReverseRainbowText("✨ Everything is rainbow now! ✨", 10))
			case "clear":
				layout.SetContentLine(3, "🧹 Display cleared", nil)
				layout.SetContentLine(4, "", nil)
			default:
				if input != "" {
					layout.SetContentLine(3, fmt.Sprintf("💬 You typed: %s", input), tui.CreateRainbowText(fmt.Sprintf("💬 You typed: %s", input), 15))
					layout.SetContentLine(4, fmt.Sprintf("📝 Input #%d received", inputCount), tui.CreatePulseText(tui.NewRGB(255, 255, 0), 25))
				}
			}

			layout.SetContentLine(5, fmt.Sprintf("User Input: %d commands entered", inputCount), tui.CreatePulseText(tui.NewRGB(0, 255, 100), 30))
			layout.DrawPrompt()
		}
	}
}

func runColorTestDemo(terminal *tui.Terminal) {
	fmt.Println("\n🎨 Color and Gradient Test Demo")
	fmt.Println("Testing RGB colors and gradient generation")

	// Test RGB colors
	fmt.Println("\n1. Rainbow Gradient Test:")
	colors := tui.SmoothRainbow(20)
	for i, color := range colors {
		fmt.Print(color.Apply("█", false))
		if i%10 == 9 {
			fmt.Print(" ")
		}
	}
	fmt.Println()

	fmt.Println("\n2. Multi-Color Gradient Test:")
	stops := []tui.RGB{
		tui.NewRGB(255, 0, 0),   // Red
		tui.NewRGB(255, 255, 0), // Yellow
		tui.NewRGB(0, 255, 0),   // Green
		tui.NewRGB(0, 255, 255), // Cyan
		tui.NewRGB(0, 0, 255),   // Blue
		tui.NewRGB(255, 0, 255), // Magenta
	}
	multiGrad := tui.MultiGradient(stops, 30)
	for _, color := range multiGrad {
		fmt.Print(color.Apply("●", false))
	}
	fmt.Println()

	fmt.Println("\n3. Classic Rainbow Test:")
	classicRainbow := tui.RainbowGradient(25)
	for _, color := range classicRainbow {
		fmt.Print(color.Apply("▲", false))
	}
	fmt.Println()

	fmt.Println("\n4. Two-Color Gradient Test:")
	start := tui.NewRGB(255, 0, 100)
	end := tui.NewRGB(100, 0, 255)
	gradient := tui.Gradient(start, end, 15)
	for _, color := range gradient {
		fmt.Print(color.Apply("♦", false))
	}
	fmt.Println()

	fmt.Println("\n✅ Color test completed!")
	fmt.Println("All RGB color functions are working correctly.")
}

func runClassicTUIDemo(terminal *tui.Terminal) {
	fmt.Println("\n🎯 Classic TUI Components Demo")
	fmt.Println("Demonstrating traditional TUI elements")

	// Basic colors demo
	fmt.Println("\n🎨 Colors and Styles:")
	style := tui.NewStyle().WithForeground(tui.ColorRed).WithBold()
	fmt.Println("  " + style.Apply("Bold Red Text"))

	// Progress bar demo
	fmt.Println("\n📊 Progress Bar:")
	progress := tui.NewProgressBar(terminal, 50)
	for i := 0; i <= 50; i += 5 {
		progress.Update(i, "Processing...")
		time.Sleep(100 * time.Millisecond)
	}
	progress.Complete("Done!")

	// Spinner demo
	fmt.Println("\n⏳ Spinner:")
	spinner := tui.NewSpinner(terminal, tui.SpinnerDots)
	spinner.WithMessage("Loading...").Start()
	time.Sleep(3 * time.Second)
	spinner.Success("Complete!")

	// Layout demo
	fmt.Println("\n📋 Layout:")
	layout := tui.NewLayout(terminal)
	header := tui.SimpleHeader("Demo Header", tui.NewStyle().WithForeground(tui.ColorCyan))
	footer := tui.SimpleFooter("Footer", "Center", "Right", tui.NewStyle())
	layout.SetHeader(header).SetFooter(footer)

	fmt.Println("Layout components created successfully!")
	fmt.Println("\n✅ Classic demo completed!")
}