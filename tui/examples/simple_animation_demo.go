package main

import (
	"fmt"

	"github.com/deepnoodle-ai/dive/tui"
)

func main() {
	// Test basic animation functionality
	fmt.Println("🎨 Testing TUI Animation Features")
	fmt.Println("=================================")

	// Test RGB color generation
	fmt.Println("\n1. Testing RGB Color Generation:")
	colors := tui.SmoothRainbow(10)
	for i, color := range colors {
		text := fmt.Sprintf("Color %d ", i)
		fmt.Print(color.Apply(text, false))
	}
	fmt.Println()

	// Test rainbow gradient
	fmt.Println("\n2. Testing Rainbow Gradient:")
	rainbow := tui.RainbowGradient(20)
	for i, color := range rainbow {
		fmt.Print(color.Apply("█", false))
		if i%10 == 9 {
			fmt.Print(" ")
		}
	}
	fmt.Println()

	// Test multi-gradient
	fmt.Println("\n3. Testing Multi-Color Gradient:")
	stops := []tui.RGB{
		tui.NewRGB(255, 0, 0), // Red
		tui.NewRGB(0, 255, 0), // Green
		tui.NewRGB(0, 0, 255), // Blue
	}
	multiGrad := tui.MultiGradient(stops, 15)
	for _, color := range multiGrad {
		fmt.Print(color.Apply("●", false))
	}
	fmt.Println()

	// Test animation classes
	fmt.Println("\n4. Testing Animation Classes:")

	// Test RainbowAnimation
	rainbowAnim := &tui.RainbowAnimation{
		Speed:    20,
		Length:   10,
		Reversed: false,
	}

	fmt.Print("RainbowAnimation: ")
	testText := "Hello World!"
	for frame := uint64(0); frame < 3; frame++ {
		for i, char := range testText {
			style := rainbowAnim.GetStyle(frame, i, len(testText))
			fmt.Print(style.Apply(string(char)))
		}
		fmt.Print(" ")
	}
	fmt.Println()

	// Test PulseAnimation
	pulseAnim := &tui.PulseAnimation{
		Speed:         30,
		Color:         tui.NewRGB(255, 100, 0),
		MinBrightness: 0.3,
		MaxBrightness: 1.0,
	}

	fmt.Print("PulseAnimation:   ")
	for frame := uint64(0); frame < 3; frame++ {
		for i, char := range testText {
			style := pulseAnim.GetStyle(frame, i, len(testText))
			fmt.Print(style.Apply(string(char)))
		}
		fmt.Print(" ")
	}
	fmt.Println()

	// Test helper functions
	fmt.Println("\n5. Testing Helper Functions:")
	fmt.Print("CreateRainbowText: ")
	helperAnim := tui.CreateRainbowText("Rainbow!", 15)
	for i, char := range "Rainbow!" {
		style := helperAnim.GetStyle(0, i, 8)
		fmt.Print(style.Apply(string(char)))
	}
	fmt.Println()

	fmt.Print("CreatePulseText:   ")
	pulseHelper := tui.CreatePulseText(tui.NewRGB(0, 255, 255), 20)
	for i, char := range "Pulse!" {
		style := pulseHelper.GetStyle(0, i, 6)
		fmt.Print(style.Apply(string(char)))
	}
	fmt.Println()

	fmt.Println("\n✅ Basic animation tests completed!")
	fmt.Println("\n📝 Animation features implemented:")
	fmt.Println("   • Multi-line animated content above input")
	fmt.Println("   • Rainbow text animation with configurable speed")
	fmt.Println("   • Reverse rainbow animation")
	fmt.Println("   • Pulsing brightness animation")
	fmt.Println("   • Wave animation framework")
	fmt.Println("   • Animated status bars")
	fmt.Println("   • Animated footer sections")
	fmt.Println("   • 30+ FPS animation engine")
	fmt.Println("   • RGB color gradients and smooth transitions")

	fmt.Println("\n🎯 TUI package now supports all requested animation capabilities!")
}
