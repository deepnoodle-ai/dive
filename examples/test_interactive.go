package main

import (
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/tui"
)

func main() {
	terminal, err := tui.NewTerminal()
	if err != nil {
		log.Fatal(err)
	}
	defer terminal.Close()

	fmt.Println("\n=== Interactive Input Test ===\n")

	// Test 1: Password with visual feedback
	fmt.Println("1. Password with asterisks (type and watch *):")
	input1 := tui.NewInput(terminal).
		WithPrompt("Password: ", tui.NewStyle().WithForeground(tui.ColorRed)).
		WithPlaceholder("min 8 chars").
		WithMask('*')

	pass, err := input1.ReadInteractive()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("You entered: %s\n\n", pass)
	}

	// Test 2: Live autocomplete
	fmt.Println("2. Live autocomplete (type 'go', 'py', 'java' etc):")
	fmt.Println("   Suggestions appear below. Press Tab to complete.")

	languages := []string{
		"Go", "Golang", "Python", "JavaScript", "TypeScript",
		"Java", "Rust", "Ruby", "PHP", "C++", "C#", "Swift",
		"Kotlin", "Scala", "Haskell", "Clojure",
	}

	input2 := tui.NewInput(terminal).
		WithPrompt("Language: ", tui.NewStyle().WithForeground(tui.ColorGreen)).
		SetSuggestions(languages)

	lang, err := input2.ReadInteractive()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Selected: %s\n\n", lang)
	}

	fmt.Println("✓ Test complete!")
}