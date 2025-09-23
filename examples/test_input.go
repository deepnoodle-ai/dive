package main

import (
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/tui"
)

func main() {
	// Create terminal
	terminal, err := tui.NewTerminal()
	if err != nil {
		log.Fatal("Failed to initialize terminal:", err)
	}
	defer terminal.Close()

	fmt.Println("\n=== Input Test Program ===\n")

	// Test 1: Regular input
	fmt.Println("Test 1: Regular Input")
	input1 := tui.NewInput(terminal).
		WithPrompt("Name: ", tui.NewStyle().WithForeground(tui.ColorCyan))

	name, err := input1.ReadBasic()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("You entered: %s\n\n", name)
	}

	// Test 2: Masked password input
	fmt.Println("Test 2: Password Input (should show * for each character)")
	input2 := tui.NewInput(terminal).
		WithPrompt("Password: ", tui.NewStyle().WithForeground(tui.ColorRed)).
		WithMask('*')

	password, err := input2.ReadSecure()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Password entered: %s (length: %d)\n\n", password, len(password))
	}

	// Test 3: Suggestions/Autocomplete
	fmt.Println("Test 3: Autocomplete (type 'go' or 'py' and press Tab)")
	languages := []string{"golang", "python", "javascript", "typescript", "rust"}
	input3 := tui.NewInput(terminal).
		WithPrompt("Language: ", tui.NewStyle().WithForeground(tui.ColorGreen)).
		SetSuggestions(languages)

	lang, err := input3.ReadWithSuggestions()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Selected: %s\n", lang)
	}

	fmt.Println("\n✓ Test complete!")
}