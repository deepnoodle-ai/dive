// Skills example demonstrates how to load and use skills with a Dive agent.
//
// Skills are markdown-based instruction sets that extend agent behavior. They
// can be auto-invoked by the agent based on task context or manually triggered
// by users via /name syntax.
//
// This example:
//   - Loads skills from .dive/skills/ in the example directory
//   - Configures the agent with skill support (tool, catalog, hooks)
//   - Sends a prompt that triggers the code-reviewer skill
//
// Usage:
//
//	cd examples
//	go run ./skills_example
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/skill"
	"github.com/deepnoodle-ai/dive/toolkit"
)

func main() {
	ctx := context.Background()

	// Determine the example directory (where .dive/skills/ lives)
	exampleDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		exampleDir = "."
	}
	// When run via "go run", use the source directory
	if _, err := os.Stat(filepath.Join(exampleDir, ".dive")); os.IsNotExist(err) {
		exampleDir = "."
		if _, err := os.Stat(filepath.Join(exampleDir, ".dive")); os.IsNotExist(err) {
			exampleDir = "skills_example"
		}
	}

	// Load skills from the example's .dive/skills/ directory
	skills, err := skill.Load(ctx, skill.LoaderOptions{
		ProjectDir: exampleDir,
	})
	if err != nil {
		log.Fatalf("Failed to load skills: %v", err)
	}
	fmt.Printf("Loaded %d skill(s)\n", skills.Count())
	for _, s := range skills.List() {
		fmt.Printf("  - %s: %s\n", s.Name, s.Description)
	}

	// Set up tools
	tools := []dive.Tool{
		toolkit.NewReadFileTool(),
		toolkit.NewGlobTool(),
		toolkit.NewGrepTool(),
	}

	// Configure agent with skill support
	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: "You are a helpful software engineering assistant.",
		Model:        anthropic.New(),
		Tools:        tools,
		Extensions:   []dive.Extension{skills},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Send a prompt that should trigger the code-reviewer skill
	response, err := agent.CreateResponse(ctx,
		dive.WithInput("Review the main.go file in this directory"),
	)
	if err != nil {
		log.Fatalf("Agent error: %v", err)
	}
	fmt.Println(response.OutputText())
}
